package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
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

// LoadModels lists the supported models packaged under modelsDir, the
// app-serving project's deployments/models directory.
//
// A model is a <category>/<Name>/ directory holding a model.yaml that pins its
// Hugging Face revision, or a generative model the simulator serves. The app-serving stack deploys from the same file, so
// both read it with the shared modeldef package. Any other directory is
// reported as skipped rather than failing the run, so one incomplete model
// does not block installing the rest.
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
	path := modeldef.Path(filepath.Join(categoryDir, name))
	if _, err := os.Stat(path); err != nil {
		return Model{}, fmt.Errorf("no %s", modeldef.FileName)
	}

	def, err := modeldef.Load(path)
	if err != nil {
		return Model{}, err
	}

	gpus, err := def.GPUsPerPod()
	if err != nil {
		return Model{}, err
	}

	dependencies := make([]Dependency, 0, len(def.Dependencies))
	for _, dep := range def.Dependencies {
		dependencies = append(dependencies, Dependency{ID: dep.ID, Revision: dep.Revision})
	}

	if def.Simulated() {
		if category != CategoryGenerative {
			return Model{}, fmt.Errorf("the installer offers the simulator as a generative model only")
		}

		return Model{
			Name:      name,
			Category:  category,
			Slug:      def.Slug,
			ID:        strings.TrimSpace(def.Model.Name),
			Replicas:  int(def.ServerReplicas()),
			Simulated: true,
		}, nil
	}

	revision := strings.TrimSpace(def.Revision)

	model := Model{
		Name:          name,
		Category:      category,
		Slug:          def.Slug,
		ID:            strings.TrimSpace(def.Model.Name),
		Revision:      revision,
		Dependencies:  dependencies,
		StorageSize:   strings.TrimSpace(def.Model.Size),
		GPUsPerPod:    gpus,
		Replicas:      int(def.ServerReplicas()),
		HarborProject: ModelHarborProject,
		HarborName:    def.Slug,
		HarborTag:     harborTag(revision),
	}

	if err := model.validate(); err != nil {
		return Model{}, err
	}

	return model, nil
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
