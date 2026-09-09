package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	kubernetes "github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const (
	deploymentFolder = "deployments"
	modelFolder      = "models"
	gaiPrefix        = "gaie-"
	msPrefix         = "ms-"

	DefaultLLMdChartPath      = "../upstream/llm-d/llm-d-infra/charts/llm-d-infra"
	DefaultGaieLocalChartPath = "charts/inferencepool"
)

// ModelSource describes where pre-loaded model weights are stored.
// Pulumi uses this to create the PVC + ORAS pull Job and override
// modelArtifacts.uri in the Helm chart.
type ModelSource struct {
	HarborRef    string // Harbor OCI ref, e.g. "harbor.../ai-models/nomic:1.5.0"
	ModelUri     string // path within the PVC, e.g. "hub/org/model-name"
	StorageSize  string // e.g. "5Gi"
	StorageClass string // optional: overrides cluster default StorageClass (e.g. "hyperdisk-balanced" for g4 nodes)
	HostpathNode string // on-prem only: node hostname for the hostpath PV (e.g. "srv3rke2w2")
	HostpathDir  string // on-prem only: absolute path on the node; defaults to /var/lib/hostpath/models/<slug>
}

type Values struct {
	Platform   platform.Platform
	Kubernetes kubernetes.Connection

	Models []Model

	HarborHostname string              // internal Harbor registry hostname (e.g. harbor.internal.lan)
	HarborUser     string              // Harbor robot account name (e.g. robot$k8s-puller)
	HarborToken    pulumi.StringOutput // Harbor robot secret; valid only when HarborTokenSet is true
	HarborTokenSet bool                // true when harborToken is present in stack config
	LLMdChartPath  string
}

type Model struct {
	Namespace   string
	ReleaseName string
	ModelName   string
	Slug        string
	IsEmbedder  bool

	NodeSelector  map[string]string
	GPUToleration *Toleration // nil = no toleration injected via extraConfig

	GaieValuesPath string
	MsValuesPath   string

	// GaieLocalChartPath is the absolute path to the bundled inferencepool
	// Helm chart. Used on the on-prem path where the chart is loaded from
	// disk (instead of pulled from OCI). Resolved to an absolute path so
	// helm v3 Release inside the Pulumi automation subprocess (whose cwd
	// is not the project workdir) can find it.
	GaieLocalChartPath string

	Platform    platform.Platform
	ModelSource *ModelSource // nil = no PV/PVC managed by Pulumi

	// On-prem / air-gap fields (copied from stack-level Values; empty on cloud)
	Kubernetes     kubernetes.Connection
	HarborHostname string // internal Harbor hostname; used to derive on-prem ORAS image path
}

func (m Model) validate(msSlug string) error {
	if m.GaieValuesPath == "" {
		return fmt.Errorf("no %s* subdirectory found", gaiPrefix)
	}
	if m.MsValuesPath == "" {
		return fmt.Errorf("no %s* subdirectory found", msPrefix)
	}
	if m.Slug != msSlug {
		gaieName := gaiPrefix + m.Slug
		msName := msPrefix + msSlug
		return fmt.Errorf("slug mismatch: %s vs %s (both must use the same slug)", gaieName, msName)
	}
	if m.Slug == "" {
		return fmt.Errorf("invalid empty slug")
	}
	if len(m.Slug) > 47 {
		return fmt.Errorf("invalid slug %q: too long (%d > 47)", m.Slug, len(m.Slug))
	}
	if !slugPattern.MatchString(m.Slug) {
		return fmt.Errorf("invalid slug %q: use lowercase alphanumerics and '-', start/end with alphanumeric", m.Slug)
	}

	if _, err := os.Stat(m.GaieValuesPath); err != nil {
		return fmt.Errorf("gaie values file not found: %s", m.GaieValuesPath)
	}
	if _, err := os.Stat(m.MsValuesPath); err != nil {
		return fmt.Errorf("model service values file not found: %s", m.MsValuesPath)
	}

	return nil
}

// resolveModelPaths scans deployments/models/{modelName}/ for gaie-* and ms-* subdirectories
// and returns the values file paths and the slug extracted from directory names.
func resolveModelPaths(category, modelName string, dir string, logf Logf) (Model, error) {
	var modelDir string
	if dir == "" {
		modelDir = filepath.Join(".", deploymentFolder, modelFolder, category, modelName)
	} else {
		modelDir = filepath.Join(dir, deploymentFolder, modelFolder, category, modelName)
	}

	logf("modelDir is %s", modelDir)

	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return Model{}, fmt.Errorf("read model directory %q: %w", modelDir, err)
	}

	var model Model
	var msSlug string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		switch {
		case strings.HasPrefix(name, gaiPrefix):
			if model.GaieValuesPath != "" {
				return Model{}, fmt.Errorf("multiple %s* subdirectories in %q", gaiPrefix, modelDir)
			}
			model.GaieValuesPath = filepath.Join(modelDir, name, "values.yaml")
			model.Slug = strings.TrimPrefix(name, gaiPrefix)

		case strings.HasPrefix(name, msPrefix):
			if model.MsValuesPath != "" {
				return Model{}, fmt.Errorf("multiple %s* subdirectories in %q", msPrefix, modelDir)
			}
			model.MsValuesPath = filepath.Join(modelDir, name, "values.yaml")
			msSlug = strings.TrimPrefix(name, msPrefix)
		}
	}

	if err = model.validate(msSlug); err != nil {
		return Model{}, fmt.Errorf("validate model config for %q: %w", modelDir, err)
	}

	return model, nil
}

// Load reads the stack config and returns a single Values containing one Model per enabled
// entry under models.generative / models.embedder. All models on the same stack share
// cluster-wide settings (kubeconfig, Harbor credentials, nodeSelector). Each model gets its
// own namespace and release names derived from the slug discovered in its
// deployments/models/<category>/<modelName>/ directory.

type Logf func(format string, args ...any)

func (c Config) Load(ctx *pulumi.Context) (Values, error) {
	input, err := c.definition.Load(ctx)
	if err != nil {
		return Values{}, fmt.Errorf("load stack config: %w", err)
	}

	c.applyDefaults(&input)

	values, err := c.buildValues(input)
	if err != nil {
		return Values{}, fmt.Errorf("build app-serving config: %w", err)
	}

	if err := c.Validate(values); err != nil {
		return Values{}, fmt.Errorf("validate app-serving config: %w", err)
	}

	return values, nil
}

func (c Config) applyDefaults(input *stackInput) {
	if input.LLMdChartPath == "" {
		input.LLMdChartPath = DefaultLLMdChartPath
	}
}

func (c Config) Validate(cfg Values) error {
	if err := cfg.Platform.Validate(); err != nil {
		return err
	}

	if len(cfg.Models) == 0 {
		return fmt.Errorf("at least one model must be enabled")
	}

	if strings.TrimSpace(cfg.LLMdChartPath) == "" {
		return fmt.Errorf("llm-d chart path cannot be empty")
	}

	requiresHarbor := cfg.Platform == platform.OnPrem || cfg.hasModelSource()
	if requiresHarbor {
		if strings.TrimSpace(cfg.HarborHostname) == "" {
			return fmt.Errorf("harborHostname is required for on-prem or modelSource deployments")
		}
		if strings.TrimSpace(cfg.HarborUser) == "" {
			return fmt.Errorf("harborUser is required for on-prem or modelSource deployments")
		}
		if !cfg.HarborTokenSet {
			return fmt.Errorf("harborToken is required for on-prem or modelSource deployments")
		}
	}

	if cfg.Platform == platform.OnPrem && strings.TrimSpace(cfg.Kubernetes.KubeconfigPath) == "" {
		return fmt.Errorf("kubeconfig is required for %q", cfg.Platform)
	}

	return nil
}

func (cfg Values) hasModelSource() bool {
	for _, model := range cfg.Models {
		if model.ModelSource != nil {
			return true
		}
	}
	return false
}

func (c Config) buildValues(input stackInput) (Values, error) {

	wd, err := os.Getwd()
	if err != nil {
		c.logf("buildConfig: failed to get working directory: %v", err)
	} else {
		c.logf("buildConfig: current working directory: %s", wd)
	}

	totalModels := len(input.Models.Generative) + len(input.Models.Embedder)
	if totalModels == 0 {
		return Values{}, fmt.Errorf("models must be non-empty")
	}

	categories := []struct {
		kind   string
		models []modelInput
	}{
		{kind: "generative", models: input.Models.Generative},
		{kind: "embedder", models: input.Models.Embedder},
	}

	models := make([]Model, 0, totalModels)

	for _, category := range categories {
		for _, model := range category.models {
			c.logf("Model is: %s", model.Name)
			if !model.Enabled {
				continue
			}

			resolved, err := resolveModelPaths(category.kind, model.Name, c.ProjectDir(), c.logf)
			if err != nil {
				return Values{}, fmt.Errorf("resolve model paths for %q: %w", model.Name, err)
			}

			if model.NameSpace == "" {
				model.NameSpace = "llm-d-" + resolved.Slug
			}
			if model.RelaseName == "" {
				model.RelaseName = "infra-" + resolved.Slug
			}

			var ms *ModelSource
			if model.ModelSource != nil {
				// Apply the stack-wide ModelStorageClass fallback (set via
				// installer prompt) when the model's own storageClass is empty.
				storageClass := model.ModelSource.StorageClass
				if storageClass == "" {
					storageClass = input.ModelStorageClass
				}
				ms = &ModelSource{
					HarborRef:    model.ModelSource.HarborRef,
					ModelUri:     model.ModelSource.ModelUri,
					StorageSize:  model.ModelSource.StorageSize,
					StorageClass: storageClass,
					HostpathNode: model.ModelSource.HostpathNode,
					HostpathDir:  model.ModelSource.HostpathDir,
				}
			}

			models = append(models, Model{
				ModelName:     model.Name,
				NodeSelector:  model.NodeSelector,
				GPUToleration: input.GPUToleration,
				ReleaseName:   model.RelaseName,
				Namespace:     model.NameSpace,

				Slug:           resolved.Slug,
				IsEmbedder:     category.kind == "embedder",
				GaieValuesPath: resolved.GaieValuesPath,
				MsValuesPath:   resolved.MsValuesPath,

				// Resolved absolute path to the bundled inferencepool chart.
				// Helm v3 Release runs inside the Pulumi automation subprocess
				// whose cwd is not the project workdir, so a relative path
				// like "./charts/inferencepool" wouldn't resolve. Joining
				// with `dir` (the project workdir) gives an absolute path.
				GaieLocalChartPath: resolveProjectPath(c.ProjectDir(), DefaultGaieLocalChartPath),

				Platform:       input.Platform,
				ModelSource:    ms,
				Kubernetes:     input.Kubernetes,
				HarborHostname: input.HarborHostname,
			})
		}
	}

	return Values{
		Platform:       input.Platform,
		Kubernetes:     input.Kubernetes,
		Models:         models,
		HarborHostname: input.HarborHostname,
		HarborUser:     input.HarborUser,
		HarborToken:    input.HarborToken,
		HarborTokenSet: input.HarborTokenSet,
		LLMdChartPath:  resolveProjectPath(c.ProjectDir(), input.LLMdChartPath),
	}, nil
}

func resolveProjectPath(projectDir, path string) string {
	if path == "" {
		return ""
	}

	if !filepath.IsAbs(path) && projectDir != "" {
		path = filepath.Join(projectDir, path)
	}

	return filepath.Clean(path)
}
