package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const (
	deploymentFolder = "deployments"
	modelFolder      = "models"
	gaiPrefix        = "gaie-"
	msPrefix         = "ms-"
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

type Config struct {
	Models []Model

	ShaideDebugProxy bool

	// On-prem / air-gap fields (all optional)
	Kubeconfig     string              // path to kubeconfig; empty = KUBECONFIG env / ~/.kube/config
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

	llmdChartPath string

	// GaieLocalChartPath is the absolute path to the bundled inferencepool
	// Helm chart. Used on the on-prem path where the chart is loaded from
	// disk (instead of pulled from OCI). Resolved to an absolute path so
	// helm v3 Release inside the Pulumi automation subprocess (whose cwd
	// is not the project workdir) can find it.
	GaieLocalChartPath string

	CloudProvider string       // "cloud" or "on-prem"
	ModelSource   *ModelSource // nil = no PV/PVC managed by Pulumi

	// On-prem / air-gap fields (copied from stack-level Config; empty on cloud)
	Kubeconfig     string // path to kubeconfig; empty = KUBECONFIG env / ~/.kube/config
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

// Load reads the stack config and returns a single Config containing one Model per enabled
// entry under models.generative / models.embedder. All models on the same stack share
// cluster-wide settings (kubeconfig, Harbor credentials, nodeSelector). Each model gets its
// own namespace and release names derived from the slug discovered in its
// deployments/models/<category>/<modelName>/ directory.

type Logf func(format string, args ...any)

func Load(ctx *pulumi.Context, dir string, logf Logf) (Config, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	in, err := loadStack(ctx)
	if err != nil {
		return Config{}, err
	}

	return buildConfig(in, dir, logf)
}

func buildConfig(stack stackInput, dir string, logf Logf) (Config, error) {

	wd, err := os.Getwd()
	if err != nil {
		logf("buildConfig: failed to get working directory: %v", err)
	} else {
		logf("buildConfig: current working directory: %s", wd)
	}

	totalModels := len(stack.Models.Generative) + len(stack.Models.Embedder)
	if totalModels == 0 {
		return Config{}, fmt.Errorf("models must be non-empty")
	}

	categories := []struct {
		kind   string
		models []modelInput
	}{
		{kind: "generative", models: stack.Models.Generative},
		{kind: "embedder", models: stack.Models.Embedder},
	}

	models := make([]Model, 0, totalModels)

	for _, category := range categories {
		for _, model := range category.models {
			logf("Model is: %s", model.Name)
			if !model.Enabled {
				continue
			}

			resolved, err := resolveModelPaths(category.kind, model.Name, dir, logf)
			if err != nil {
				return Config{}, fmt.Errorf("resolve model paths for %q: %w", model.Name, err)
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
					storageClass = stack.ModelStorageClass
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
				GPUToleration: stack.GPUToleration,
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
				GaieLocalChartPath: filepath.Join(dir, "charts", "inferencepool"),

				CloudProvider:  string(stack.CloudProvider),
				ModelSource:    ms,
				HarborHostname: stack.HarborHostname,
			})
		}
	}

	var llmdPath string
	if stack.LLMdChartPath == "" {
		llmdPath = "./../upstream/llm-d/llm-d-infra/charts/llm-d-infra"
	} else {
		llmdPath = filepath.Join(dir, stack.LLMdChartPath)
	}

	return Config{
		Models:         models,
		Kubeconfig:     stack.Kubeconfig,
		HarborHostname: stack.HarborHostname,
		HarborUser:     stack.HarborUser,
		HarborToken:    stack.HarborToken,
		HarborTokenSet: stack.HarborTokenSet,
		LLMdChartPath:  llmdPath,
	}, nil

}
