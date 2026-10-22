package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	kubernetes "github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	deploymentFolder = "deployments"
	modelFolder      = "models"

	// DefaultRouterChartPath is the llm-d-router-gateway chart vendored in
	// the project, relative to the project directory. It is used on every
	// platform, so deploying needs no access to the chart registry.
	DefaultRouterChartPath = "charts/llm-d-router-gateway"
)

type Values struct {
	Platform   cluster.Provider
	Kubernetes kubernetes.Connection

	Harbor struct {
		Hostname string
		User     string
		Token    pulumi.StringOutput
		TokenSet bool
	}

	// RouterChartPath is the resolved llm-d-router-gateway chart directory.
	RouterChartPath string

	Toleration *Toleration

	// ORASImage pulls model weights into their volume.
	ORASImage string

	// ModelServerImage runs vLLM for models that do not name an image.
	ModelServerImage string

	// RouterImage is the llm-d router's endpoint picker (EPP).
	RouterImage string

	// SimulatorImage runs the models with engine sim that do not name an
	// image.
	SimulatorImage string

	Models struct {
		Generative []Model `json:"generative"`
		Embedder   []Model `json:"embedder"`
	}

	// Default StorageClass used when modelSource.storageClass is empty.
	ModelStorageClass string
}

type Model struct {
	ModelPaths `json:"-"`

	Name         string            `json:"name"`
	Enabled      bool              `json:"enabled"`
	Namespace    string            `json:"nameSpace"`
	ReleaseName  string            `json:"releaseName"`
	NodeSelector map[string]string `json:"nodeSelector"`
	// Placement, when set, replaces NodeSelector.
	Placement []PlacementTerm `json:"placement,omitempty"`

	ModelSource *ModelSource `json:"modelSource"`

	// Definition is the packaged model.yaml, loaded when the config resolves.
	Definition modeldef.Definition `json:"-"`
}

// ModelSource describes model weights pre-loaded from an OCI artifact into a
// persistent volume before the model service starts.
type ModelSource struct {
	HarborRef    string `json:"harborRef"`
	ModelUri     string `json:"modelUri"`
	StorageSize  string `json:"storageSize"`
	StorageClass string `json:"storageClass"`
	HostpathNode string `json:"hostpathNode"`
	HostpathDir  string `json:"hostpathDir"`
}

type ModelPaths struct {
	Slug           string
	DefinitionPath string
}

func (c Config) Load(ctx *pulumi.Context) (Values, error) {
	values, err := c.definition.Load(ctx)
	if err != nil {
		return Values{}, fmt.Errorf("load stack config: %w", err)
	}

	c.applyDefaults(&values)

	if err := c.resolve(&values); err != nil {
		return Values{}, fmt.Errorf("resolve app-serving config: %w", err)
	}

	if err := c.Validate(values); err != nil {
		return Values{}, fmt.Errorf("validate app-serving config: %w", err)
	}

	return values, nil
}

func (c Config) applyDefaults(values *Values) {
	if values.ORASImage == "" {
		values.ORASImage = DefaultORASImage
	}

	if values.ModelServerImage == "" {
		values.ModelServerImage = DefaultModelServerImage
	}

	if values.RouterImage == "" {
		values.RouterImage = DefaultRouterImage
	}

	if values.SimulatorImage == "" {
		values.SimulatorImage = DefaultSimulatorImage
	}

	applyModelDefaults := func(models []Model) {
		for i := range models {
			model := &models[i]

			if !model.Enabled || model.ModelSource == nil {
				continue
			}

			if model.ModelSource.StorageClass == "" {
				model.ModelSource.StorageClass = values.ModelStorageClass
			}
		}
	}

	applyModelDefaults(values.Models.Generative)
	applyModelDefaults(values.Models.Embedder)
}

func (c Config) resolve(values *Values) error {
	values.RouterChartPath = resolveProjectPath(c.ProjectDir(), DefaultRouterChartPath)

	if err := c.resolveModels(CategoryGenerative, values.Models.Generative); err != nil {
		return err
	}

	if err := c.resolveModels(CategoryEmbedder, values.Models.Embedder); err != nil {
		return err
	}

	return nil
}

func (c Config) resolveModels(category string, models []Model) error {
	for i := range models {
		model := &models[i]

		if !model.Enabled {
			continue
		}

		paths, definition, err := resolveModelPaths(category, model.Name, c.ProjectDir())
		if err != nil {
			return fmt.Errorf("resolve model %q: %w", model.Name, err)
		}

		model.ModelPaths = paths
		model.Definition = definition

		if model.Namespace == "" {
			model.Namespace = modelNamespace(model.Slug)
		}

		if model.ReleaseName == "" {
			model.ReleaseName = "infra-" + model.Slug
		}
	}

	return nil
}

func (c Config) Validate(values Values) error {
	if err := values.Platform.Validate(); err != nil {
		return err
	}

	if strings.TrimSpace(values.RouterChartPath) == "" {
		return fmt.Errorf("router chart path cannot be empty")
	}

	if strings.TrimSpace(values.ModelServerImage) == "" {
		return fmt.Errorf("model server image cannot be empty")
	}

	if strings.TrimSpace(values.RouterImage) == "" {
		return fmt.Errorf("router image cannot be empty")
	}

	if strings.TrimSpace(values.SimulatorImage) == "" {
		return fmt.Errorf("simulator image cannot be empty")
	}

	if !values.hasEnabledModels() {
		return fmt.Errorf("at least one model must be enabled")
	}

	if err := validateModels(values.Models.Generative); err != nil {
		return fmt.Errorf("validate generative models: %w", err)
	}

	if err := validateModels(values.Models.Embedder); err != nil {
		return fmt.Errorf("validate embedder models: %w", err)
	}

	if values.requiresHarbor() {
		if strings.TrimSpace(values.Harbor.Hostname) == "" {
			return fmt.Errorf("harbor hostname is required for on-prem or modelSource deployments")
		}

		if strings.TrimSpace(values.Harbor.User) == "" {
			return fmt.Errorf("harbor user is required for on-prem or modelSource deployments")
		}

		if !values.Harbor.TokenSet {
			return fmt.Errorf("harbor token is required for on-prem or modelSource deployments")
		}
	}

	if values.Platform == cluster.OnPrem && strings.TrimSpace(values.Kubernetes.KubeconfigPath) == "" {
		return fmt.Errorf("kubeconfig is required for %q", values.Platform)
	}

	return nil
}

func validateModels(models []Model) error {
	for _, model := range models {
		if !model.Enabled {
			continue
		}

		if strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("model name cannot be empty")
		}

		if strings.TrimSpace(model.Namespace) == "" {
			return fmt.Errorf("namespace cannot be empty for model %q", model.Name)
		}

		if strings.TrimSpace(model.ReleaseName) == "" {
			return fmt.Errorf("release name cannot be empty for model %q", model.Name)
		}

		if err := model.ModelPaths.validate(); err != nil {
			return fmt.Errorf("validate model %q: %w", model.Name, err)
		}
	}

	return nil
}

func (values Values) hasEnabledModels() bool {
	for _, model := range values.Models.Generative {
		if model.Enabled {
			return true
		}
	}

	for _, model := range values.Models.Embedder {
		if model.Enabled {
			return true
		}
	}

	return false
}

func (paths ModelPaths) validate() error {
	if err := modeldef.ValidateSlug(paths.Slug); err != nil {
		return err
	}

	if paths.DefinitionPath == "" {
		return fmt.Errorf("model definition path cannot be empty")
	}

	if _, err := os.Stat(paths.DefinitionPath); err != nil {
		return fmt.Errorf("model definition not found: %s", paths.DefinitionPath)
	}

	return nil
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

func (values Values) requiresHarbor() bool {
	if values.Platform == cluster.OnPrem {
		return true
	}

	for _, model := range values.Models.Embedder {
		if model.Enabled && model.ModelSource != nil {
			return true
		}
	}

	for _, model := range values.Models.Generative {
		if model.Enabled && model.ModelSource != nil {
			return true
		}
	}

	return false
}

// resolveModelPaths loads the model.yaml packaged for the model. Its slug
// names the model's namespace, resources and labels.
func resolveModelPaths(category string, modelName string, projectDir string) (ModelPaths, modeldef.Definition, error) {
	path := modeldef.Path(filepath.Join(projectDir, deploymentFolder, modelFolder, category, modelName))

	definition, err := modeldef.Load(path)
	if err != nil {
		return ModelPaths{}, modeldef.Definition{}, err
	}

	return ModelPaths{Slug: definition.Slug, DefinitionPath: path}, definition, nil
}
