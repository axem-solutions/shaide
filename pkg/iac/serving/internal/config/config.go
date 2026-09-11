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

	gaiePrefix = "gaie-"
	msPrefix   = "ms-"

	DefaultLLMdChartPath      = "../upstream/llm-d/llm-d-infra/charts/llm-d-infra"
	DefaultGaieLocalChartPath = "charts/inferencepool"
)

type Values struct {
	Platform   platform.Platform
	Kubernetes kubernetes.Connection

	Harbor struct {
		Hostname string
		User     string
		Token    pulumi.StringOutput
		TokenSet bool
	}

	LLMd struct {
		ChartPath string
	}

	Toleration *Toleration

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

	ModelSource *ModelSource `json:"modelSource"`
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
	Slug               string
	GaieValuesPath     string
	MsValuesPath       string
	GaieLocalChartPath string
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
	if values.LLMd.ChartPath == "" {
		values.LLMd.ChartPath = DefaultLLMdChartPath
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
	values.LLMd.ChartPath = resolveProjectPath(c.ProjectDir(), values.LLMd.ChartPath)

	gaieLocalChartPath := resolveProjectPath(c.ProjectDir(), DefaultGaieLocalChartPath)

	if err := c.resolveModels("generative", values.Models.Generative, gaieLocalChartPath); err != nil {
		return err
	}

	if err := c.resolveModels("embedder", values.Models.Embedder, gaieLocalChartPath); err != nil {
		return err
	}

	return nil
}

func (c Config) resolveModels(category string, models []Model, gaieLocalChartPath string) error {
	for i := range models {
		model := &models[i]

		if !model.Enabled {
			continue
		}

		paths, err := resolveModelPaths(category, model.Name, c.ProjectDir())
		if err != nil {
			return fmt.Errorf("resolve model paths for %q: %w", model.Name, err)
		}

		paths.GaieLocalChartPath = gaieLocalChartPath
		model.ModelPaths = paths

		if model.Namespace == "" {
			model.Namespace = "llm-d-" + model.Slug
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

	if strings.TrimSpace(values.LLMd.ChartPath) == "" {
		return fmt.Errorf("llm-d chart path cannot be empty")
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

	if values.Platform == platform.OnPrem && strings.TrimSpace(values.Kubernetes.KubeconfigPath) == "" {
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
	if err := paths.validateSlug(); err != nil {
		return err
	}

	if paths.GaieValuesPath == "" {
		return fmt.Errorf("gaie values path cannot be empty")
	}

	if paths.MsValuesPath == "" {
		return fmt.Errorf("model service values path cannot be empty")
	}

	if paths.GaieLocalChartPath == "" {
		return fmt.Errorf("gaie local chart path cannot be empty")
	}

	if _, err := os.Stat(paths.GaieValuesPath); err != nil {
		return fmt.Errorf("gaie values file not found: %s", paths.GaieValuesPath)
	}

	if _, err := os.Stat(paths.MsValuesPath); err != nil {
		return fmt.Errorf("model service values file not found: %s", paths.MsValuesPath)
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
	if values.Platform == platform.OnPrem {
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

func resolveModelPaths(category string, modelName string, projectDir string) (ModelPaths, error) {
	modelDir := filepath.Join(projectDir, deploymentFolder, modelFolder, category, modelName)

	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return ModelPaths{}, fmt.Errorf("read model directory %q: %w", modelDir, err)
	}

	var paths ModelPaths
	var msSlug string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		switch {
		case strings.HasPrefix(name, gaiePrefix):
			if paths.GaieValuesPath != "" {
				return ModelPaths{}, fmt.Errorf("multiple %s* subdirectories in %q", gaiePrefix, modelDir)
			}

			paths.Slug = strings.TrimPrefix(name, gaiePrefix)
			paths.GaieValuesPath = filepath.Join(modelDir, name, "values.yaml")
		case strings.HasPrefix(name, msPrefix):
			if paths.MsValuesPath != "" {
				return ModelPaths{}, fmt.Errorf("multiple %s* subdirectories in %q", msPrefix, modelDir)
			}

			msSlug = strings.TrimPrefix(name, msPrefix)
			paths.MsValuesPath = filepath.Join(modelDir, name, "values.yaml")
		}
	}

	if err := paths.validateResolved(modelDir, msSlug); err != nil {
		return ModelPaths{}, err
	}

	return paths, nil
}

func (paths ModelPaths) validateResolved(modelDir string, msSlug string) error {
	if paths.GaieValuesPath == "" {
		return fmt.Errorf("no %s* subdirectory found in %q", gaiePrefix, modelDir)
	}

	if paths.MsValuesPath == "" {
		return fmt.Errorf("no %s* subdirectory found in %q", msPrefix, modelDir)
	}
	if paths.Slug != msSlug {
		return fmt.Errorf("slug mismatch: %s%s vs %s%s", gaiePrefix, paths.Slug, msPrefix, msSlug)
	}

	if err := paths.validateSlug(); err != nil {
		return err
	}

	if _, err := os.Stat(paths.GaieValuesPath); err != nil {
		return fmt.Errorf("gaie values file not found: %s", paths.GaieValuesPath)
	}

	if _, err := os.Stat(paths.MsValuesPath); err != nil {
		return fmt.Errorf("model service values file not found: %s", paths.MsValuesPath)
	}

	return nil
}

func (paths ModelPaths) validateSlug() error {
	if paths.Slug == "" {
		return fmt.Errorf("slug cannot be empty")
	}

	if len(paths.Slug) > 47 {
		return fmt.Errorf("invalid slug %q: too long (%d > 47)", paths.Slug, len(paths.Slug))
	}

	if !slugPattern.MatchString(paths.Slug) {
		return fmt.Errorf("invalid slug %q: use lowercase alphanumerics and '-', start/end with alphanumeric", paths.Slug)
	}

	return nil
}
