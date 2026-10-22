// Package modeldef reads the model definitions packaged under app-serving's
// deployments/models/<category>/<Name>/model.yaml. The installer reads them to
// offer and download models, the app-serving stack to deploy them, so both
// share this one schema.
package modeldef

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

// FileName is the definition file inside a model's directory.
const FileName = "model.yaml"

// GPUResource is the extended resource a model server requests GPUs with.
const GPUResource corev1.ResourceName = "nvidia.com/gpu"

// maxSlugLength keeps every name derived from the slug within the 63
// character limit of Kubernetes names and labels.
const maxSlugLength = 47

var slugPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Engine selects how the model server container is started.
type Engine string

const (
	// EngineVLLM runs `vllm serve <model>`. It is the default.
	EngineVLLM Engine = "vllm"
	// EngineSim runs llm-d-inference-sim: the image's own entrypoint with
	// vLLM-compatible flags. It needs no weights and no GPU.
	EngineSim Engine = "sim"
)

// Definition describes one servable model.
type Definition struct {
	// Slug names the model's namespace, resources and pod labels.
	Slug string `json:"slug"`

	// Revision pins the Hugging Face commit to download. It also derives
	// the Harbor tag of the mirrored weights.
	Revision string `json:"revision,omitempty"`

	// Dependencies are further Hugging Face repositories the model needs,
	// for example a separate tokenizer.
	Dependencies []Dependency `json:"dependencies,omitempty"`

	Model  Model  `json:"model"`
	Server Server `json:"server"`
	Router Router `json:"router,omitempty"`
}

// Dependency is a Hugging Face repository pinned to a commit.
type Dependency struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

// Model names the weights and the volume holding them.
type Model struct {
	// Name is the Hugging Face repository, e.g. "openai/gpt-oss-20b". It is
	// also the model name clients send in requests.
	Name string `json:"name"`

	// Size of the model volume, e.g. "40Gi".
	Size string `json:"size"`
}

// Server is the model server workload. Fields not set here follow the llm-d
// v1.0 single-host recipe: port 8000, /health liveness, /v1/models readiness
// and startup probes, and a graceful drain on shutdown.
type Server struct {
	Engine Engine `json:"engine,omitempty"`

	// Image overrides the stack's image for the engine: vLLM, or the
	// simulator for EngineSim.
	Image string `json:"image,omitempty"`

	// Replicas defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`

	// Args are appended to the arguments the platform sets (model, port,
	// served model name).
	Args []string `json:"args,omitempty"`

	Env          []corev1.EnvVar             `json:"env,omitempty"`
	Resources    corev1.ResourceRequirements `json:"resources,omitempty"`
	Volumes      []corev1.Volume             `json:"volumes,omitempty"`
	VolumeMounts []corev1.VolumeMount        `json:"volumeMounts,omitempty"`

	// Tolerations are added to the stack's GPU toleration.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// StartupProbe overrides the default, which allows 30 minutes for
	// loading weights and compiling kernels.
	StartupProbe *corev1.Probe `json:"startupProbe,omitempty"`

	// RuntimeMetadata serves GET /v1/shaide/metadata through the platform's
	// vLLM middleware. Only EngineVLLM supports it.
	RuntimeMetadata bool `json:"runtimeMetadata,omitempty"`
}

// Router configures the model's llm-d router (EPP).
type Router struct {
	// Replicas defaults to 1. More replicas run in leader-elected HA mode.
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources override the EPP container's requests and limits.
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// PluginsConfig replaces the platform's EndpointPickerConfig with a
	// complete llm-d.ai/v1 EndpointPickerConfig document.
	PluginsConfig string `json:"pluginsConfig,omitempty"`
}

// Load reads and validates the definition at path.
func Load(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read %s: %w", path, err)
	}

	def, err := Parse(data)
	if err != nil {
		return Definition{}, fmt.Errorf("%s: %w", path, err)
	}

	return def, nil
}

// Parse decodes and validates a definition. Unknown fields are rejected so a
// misspelled key fails the install instead of being silently dropped.
func Parse(data []byte) (Definition, error) {
	var def Definition
	if err := yaml.UnmarshalStrict(data, &def); err != nil {
		return Definition{}, fmt.Errorf("parse model definition: %w", err)
	}

	if def.Server.Engine == "" {
		def.Server.Engine = EngineVLLM
	}

	if err := def.Validate(); err != nil {
		return Definition{}, err
	}

	return def, nil
}

// Path is the definition file of the model packaged in modelDir.
func Path(modelDir string) string {
	return filepath.Join(modelDir, FileName)
}

// Validate checks the fields every consumer relies on. The installer checks
// the download pins itself, as only it needs them.
func (d Definition) Validate() error {
	if err := ValidateSlug(d.Slug); err != nil {
		return err
	}

	if strings.TrimSpace(d.Model.Name) == "" {
		return fmt.Errorf("model.name is required")
	}

	// The simulator has no weights to store.
	if strings.TrimSpace(d.Model.Size) == "" && !d.Simulated() {
		return fmt.Errorf("model.size is required")
	}

	if d.Model.Size != "" {
		if _, err := resource.ParseQuantity(d.Model.Size); err != nil {
			return fmt.Errorf("model.size %q: %w", d.Model.Size, err)
		}
	}

	switch d.Server.Engine {
	case EngineVLLM:
	case EngineSim:
		if d.Server.RuntimeMetadata {
			return fmt.Errorf("server.runtimeMetadata requires engine %q", EngineVLLM)
		}
		if _, ok := d.Server.Resources.Limits[GPUResource]; ok {
			return fmt.Errorf("engine %q runs without GPUs; remove the %s limit", EngineSim, GPUResource)
		}
	default:
		return fmt.Errorf("server.engine %q is not one of %q, %q", d.Server.Engine, EngineVLLM, EngineSim)
	}

	if r := d.Server.Replicas; r != nil && *r < 0 {
		return fmt.Errorf("server.replicas cannot be negative")
	}

	if r := d.Router.Replicas; r != nil && *r < 1 {
		return fmt.Errorf("router.replicas must be at least 1")
	}

	if _, err := d.GPUsPerPod(); err != nil {
		return err
	}

	return nil
}

// ValidateSlug checks that a slug can name Kubernetes objects.
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug is required")
	}

	if len(slug) > maxSlugLength {
		return fmt.Errorf("invalid slug %q: too long (%d > %d)", slug, len(slug), maxSlugLength)
	}

	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("invalid slug %q: use lowercase alphanumerics and '-', start/end with alphanumeric", slug)
	}

	return nil
}

// ServerReplicas is the number of model server pods.
func (d Definition) ServerReplicas() int32 {
	if d.Server.Replicas == nil {
		return 1
	}
	return *d.Server.Replicas
}

// RouterReplicas is the number of EPP pods.
func (d Definition) RouterReplicas() int32 {
	if d.Router.Replicas == nil {
		return 1
	}
	return *d.Router.Replicas
}

// Simulated reports whether the model runs on the simulator, which serves
// generated responses without weights.
func (d Definition) Simulated() bool {
	return d.Server.Engine == EngineSim
}

// GPUsPerPod is the model server's GPU limit. A model without one runs on CPU
// only, which is valid for the simulator.
func (d Definition) GPUsPerPod() (int, error) {
	quantity, ok := d.Server.Resources.Limits[GPUResource]
	if !ok {
		return 0, nil
	}

	count, err := strconv.Atoi(quantity.String())
	if err != nil {
		return 0, fmt.Errorf("server GPU limit %q is not a whole number", quantity.String())
	}

	return count, nil
}
