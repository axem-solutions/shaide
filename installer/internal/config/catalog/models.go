package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v2"
)

// Model categories, named after the directories under deployments/models/.
const (
	CategoryGenerative = "generative"
	CategoryEmbedder   = "embedder"
)

const (
	// ModelHarborProject is the Harbor project every model artifact is pushed
	// into.
	ModelHarborProject = "ai-models"

	// harborTagLength is how much of the pinned revision names the artifact.
	// Twelve characters of a commit sha is what git itself considers
	// unambiguous for large repositories.
	harborTagLength = 12

	modelServicePrefix = "ms-"
	gpuResource        = "nvidia.com/gpu"
)

// modelCategories lists the directories scanned for supported models, in the
// order the selector shows them.
var modelCategories = []string{CategoryGenerative, CategoryEmbedder}

// SkippedModel is a directory under deployments/models/ that does not describe
// an installable model, with the reason it was left out.
type SkippedModel struct {
	Dir    string
	Reason string
}

// modelValues is the part of a model's ms-*/values.yaml the installer reads.
// The file itself is the llm-d-modelservice chart's values; the shaide block
// is ignored by the chart, whose schema allows unknown top-level keys.
type modelValues struct {
	Shaide struct {
		Revision     string       `yaml:"revision"`
		Dependencies []Dependency `yaml:"dependencies"`
	} `yaml:"shaide"`

	ModelArtifacts struct {
		Name string `yaml:"name"`
		Size string `yaml:"size"`
	} `yaml:"modelArtifacts"`

	Decode struct {
		Replicas   int `yaml:"replicas"`
		Containers []struct {
			Resources struct {
				Limits map[string]any `yaml:"limits"`
			} `yaml:"resources"`
		} `yaml:"containers"`
	} `yaml:"decode"`
}

// LoadModels lists the supported models packaged under modelsDir, the
// app-serving project's deployments/models directory.
//
// A model is a <category>/<Name>/ directory with exactly one ms-<slug>/
// subdirectory holding a values.yaml that pins its Hugging Face revision. Any
// other directory is reported as skipped rather than failing the run, so one
// incomplete model does not block installing the rest.
func LoadModels(modelsDir string) ([]Model, []SkippedModel, error) {
	var models []Model
	var skipped []SkippedModel

	for _, category := range modelCategories {
		categoryDir := filepath.Join(modelsDir, category)

		entries, err := os.ReadDir(categoryDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, fmt.Errorf("read model directory %q: %w", categoryDir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			model, err := loadModel(categoryDir, category, entry.Name())
			if err != nil {
				skipped = append(skipped, SkippedModel{
					Dir:    filepath.Join(category, entry.Name()),
					Reason: err.Error(),
				})
				continue
			}

			models = append(models, model)
		}
	}

	sort.SliceStable(models, func(i, j int) bool {
		if models[i].Category != models[j].Category {
			return categoryRank(models[i].Category) < categoryRank(models[j].Category)
		}
		return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
	})

	return models, skipped, nil
}

func loadModel(categoryDir, category, name string) (Model, error) {
	slug, valuesPath, err := findModelValues(filepath.Join(categoryDir, name))
	if err != nil {
		return Model{}, err
	}

	data, err := os.ReadFile(valuesPath)
	if err != nil {
		return Model{}, fmt.Errorf("read %s: %w", valuesPath, err)
	}

	var values modelValues
	if err := yaml.Unmarshal(data, &values); err != nil {
		return Model{}, fmt.Errorf("parse %s: %w", valuesPath, err)
	}

	gpus, err := gpusPerPod(values)
	if err != nil {
		return Model{}, err
	}

	revision := strings.TrimSpace(values.Shaide.Revision)

	model := Model{
		Name:          name,
		Category:      category,
		Slug:          slug,
		ID:            strings.TrimSpace(values.ModelArtifacts.Name),
		Revision:      revision,
		Dependencies:  values.Shaide.Dependencies,
		StorageSize:   strings.TrimSpace(values.ModelArtifacts.Size),
		GPUsPerPod:    gpus,
		Replicas:      values.Decode.Replicas,
		HarborProject: ModelHarborProject,
		HarborName:    slug,
		HarborTag:     harborTag(revision),
	}

	if err := model.validate(); err != nil {
		return Model{}, err
	}

	return model, nil
}

// findModelValues returns the slug and values file of the model's single
// ms-<slug>/ subdirectory. app-serving names the model's namespace, release and
// labels after the same slug.
func findModelValues(modelDir string) (string, string, error) {
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", modelDir, err)
	}

	var slug, valuesPath string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), modelServicePrefix) {
			continue
		}

		candidate := filepath.Join(modelDir, entry.Name(), "values.yaml")
		if _, err := os.Stat(candidate); err != nil {
			continue
		}

		if valuesPath != "" {
			return "", "", fmt.Errorf("more than one %s*/values.yaml", modelServicePrefix)
		}

		slug = strings.TrimPrefix(entry.Name(), modelServicePrefix)
		valuesPath = candidate
	}

	if valuesPath == "" {
		return "", "", fmt.Errorf("no %s*/values.yaml", modelServicePrefix)
	}

	return slug, valuesPath, nil
}

// gpusPerPod reads the GPU limit of the first decode container. A model
// without one runs on CPU only, which is valid for the simulator images.
func gpusPerPod(values modelValues) (int, error) {
	if len(values.Decode.Containers) == 0 {
		return 0, nil
	}

	raw, ok := values.Decode.Containers[0].Resources.Limits[gpuResource]
	if !ok {
		return 0, nil
	}

	count, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw)))
	if err != nil {
		return 0, fmt.Errorf("decode GPU limit %q is not a whole number", fmt.Sprint(raw))
	}

	return count, nil
}

// harborTag names the artifact after the revision it was built from, so a new
// pin is a new tag and the "already in Harbor" check cannot mistake old
// weights for new ones.
func harborTag(revision string) string {
	if len(revision) > harborTagLength {
		return revision[:harborTagLength]
	}

	return revision
}

func categoryRank(category string) int {
	for i, known := range modelCategories {
		if known == category {
			return i
		}
	}

	return len(modelCategories)
}
