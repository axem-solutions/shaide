package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gptOSSValues = `
shaide:
  revision: "6cee5e81ee83917806bbde320786a8fb61efebee"

modelArtifacts:
  name: "openai/gpt-oss-20b"
  uri: "hf://openai/gpt-oss-20b"
  size: 60Gi

decode:
  replicas: 2
  containers:
  - name: "vllm"
    resources:
      limits:
        nvidia.com/gpu: "1"
`

func writeModel(t *testing.T, root, category, name, msDir, values string) {
	t.Helper()

	dir := filepath.Join(root, category, name, msDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatalf("write values: %v", err)
	}
}

func TestLoadModelsReadsValuesFile(t *testing.T) {
	root := t.TempDir()
	writeModel(t, root, CategoryGenerative, "GPT-OSS-20B", "ms-gpt-oss-20b", gptOSSValues)

	models, skipped, err := LoadModels(root)
	if err != nil {
		t.Fatalf("LoadModels: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, want none", skipped)
	}
	if len(models) != 1 {
		t.Fatalf("loaded %d models, want 1", len(models))
	}

	got := models[0]
	want := Model{
		Name:          "GPT-OSS-20B",
		Category:      CategoryGenerative,
		Slug:          "gpt-oss-20b",
		ID:            "openai/gpt-oss-20b",
		Revision:      "6cee5e81ee83917806bbde320786a8fb61efebee",
		StorageSize:   "60Gi",
		GPUsPerPod:    1,
		Replicas:      2,
		HarborProject: ModelHarborProject,
		HarborName:    "gpt-oss-20b",
		HarborTag:     "6cee5e81ee83",
	}
	if got.Name != want.Name || got.Category != want.Category || got.Slug != want.Slug ||
		got.ID != want.ID || got.Revision != want.Revision || got.StorageSize != want.StorageSize ||
		got.GPUsPerPod != want.GPUsPerPod || got.Replicas != want.Replicas ||
		got.HarborProject != want.HarborProject || got.HarborName != want.HarborName ||
		got.HarborTag != want.HarborTag {
		t.Errorf("model = %+v\nwant    %+v", got, want)
	}
}

// A new pin must produce a new tag, or the "already in Harbor" check would
// keep serving the old weights.
func TestHarborTagFollowsRevision(t *testing.T) {
	root := t.TempDir()
	bumped := strings.Replace(gptOSSValues, "6cee5e81ee83917806bbde320786a8fb61efebee", "0123456789abcdef0123456789abcdef01234567", 1)
	writeModel(t, root, CategoryGenerative, "GPT-OSS-20B", "ms-gpt-oss-20b", bumped)

	models, _, err := LoadModels(root)
	if err != nil {
		t.Fatalf("LoadModels: %v", err)
	}
	if got := models[0].HarborTag; got != "0123456789ab" {
		t.Errorf("HarborTag = %q, want the new revision's prefix", got)
	}
}

func TestLoadModelsSkipsIncompleteDirectories(t *testing.T) {
	root := t.TempDir()
	writeModel(t, root, CategoryGenerative, "GPT-OSS-20B", "ms-gpt-oss-20b", gptOSSValues)

	unpinned := strings.Replace(gptOSSValues, `revision: "6cee5e81ee83917806bbde320786a8fb61efebee"`, "", 1)
	writeModel(t, root, CategoryGenerative, "Unpinned", "ms-unpinned", unpinned)

	branch := strings.Replace(gptOSSValues, "6cee5e81ee83917806bbde320786a8fb61efebee", "main", 1)
	writeModel(t, root, CategoryGenerative, "Branch", "ms-branch", branch)

	writeModel(t, root, CategoryEmbedder, "Broken", "ms-broken", "modelArtifacts: [")

	if err := os.MkdirAll(filepath.Join(root, CategoryEmbedder, "NoValues", "gaie-no-values"), 0o755); err != nil {
		t.Fatal(err)
	}

	models, skipped, err := LoadModels(root)
	if err != nil {
		t.Fatalf("LoadModels: %v", err)
	}
	if len(models) != 1 || models[0].Name != "GPT-OSS-20B" {
		t.Errorf("models = %v, want only GPT-OSS-20B", models)
	}

	reasons := map[string]string{}
	for _, s := range skipped {
		reasons[s.Dir] = s.Reason
	}
	for dir, want := range map[string]string{
		"generative/Unpinned": "shaide.revision is required",
		"generative/Branch":   "not a commit sha",
		"embedder/Broken":     "parse",
		"embedder/NoValues":   "no ms-*/values.yaml",
	} {
		if !strings.Contains(reasons[dir], want) {
			t.Errorf("skip reason for %s = %q, want it to mention %q", dir, reasons[dir], want)
		}
	}
}

func TestLoadModelsOrdersGenerativeFirstThenByName(t *testing.T) {
	root := t.TempDir()
	writeModel(t, root, CategoryEmbedder, "BGE-M3", "ms-bge-m3", gptOSSValues)
	writeModel(t, root, CategoryGenerative, "qwen", "ms-qwen", gptOSSValues)
	writeModel(t, root, CategoryGenerative, "GLM", "ms-glm", gptOSSValues)

	models, _, err := LoadModels(root)
	if err != nil {
		t.Fatalf("LoadModels: %v", err)
	}

	var names []string
	for _, model := range models {
		names = append(names, model.Name)
	}
	if got, want := strings.Join(names, ","), "GLM,qwen,BGE-M3"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
}

// Every model the installer image ships must be offered: a supported model
// that silently drops out of the selector cannot be installed at all.
func TestPackagedModelsAreAllLoadable(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "app_serving", "deployments", "models")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("packaged models not found at %s: %v", root, err)
	}

	models, skipped, err := LoadModels(root)
	if err != nil {
		t.Fatalf("LoadModels: %v", err)
	}

	for _, s := range skipped {
		if isExcludedVariant(s.Dir) {
			continue
		}
		t.Errorf("packaged model %s is not offered: %s", s.Dir, s.Reason)
	}

	if len(models) == 0 {
		t.Error("no packaged model loaded")
	}
}

// isExcludedVariant mirrors the patterns installer/build/Dockerfile.dockerignore
// keeps out of the installer image.
func isExcludedVariant(dir string) bool {
	name := filepath.Base(dir)
	return strings.HasPrefix(name, "sim-") ||
		strings.HasSuffix(name, "-rke2") ||
		strings.HasSuffix(name, "-janosmurai")
}
