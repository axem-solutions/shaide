package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func resolveServing(t *testing.T, sources Sources) map[string]string {
	t.Helper()

	cfg := New("/projects/app-serving", stack.Options{
		Platform:   cluster.Azure,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	}, sources)

	resolved, err := cfg.Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	values := map[string]string{}
	for key, value := range resolved {
		values[key] = value.Value
	}
	return values
}

// Model workloads and their pull Jobs tolerate the taint reserving the GPU
// pool. The stack owns that policy; the installer no longer supplies it.
func TestGPUTolerationIsTheInferencePolicy(t *testing.T) {
	value, ok := resolveServing(t, Sources{})["app-serving:gpuToleration"]
	if !ok {
		t.Fatal("app-serving:gpuToleration was not written")
	}

	var got Toleration
	if err := json.Unmarshal([]byte(value), &got); err != nil {
		t.Fatalf("decode gpuToleration %q: %v", value, err)
	}
	if got != DefaultGPUToleration {
		t.Fatalf("gpuToleration = %+v, want %+v", got, DefaultGPUToleration)
	}
}

// The model-pull Job runs the ORAS image from where the installer mirrored it,
// and from upstream when nothing was mirrored.
func TestORASImage(t *testing.T) {
	mirrored := "harbor.harbor.svc.cluster.local/services/oras-project/oras:v1.3.1"

	if got := resolveServing(t, Sources{ORASImage: mirrored})["app-serving:orasImage"]; got != mirrored {
		t.Errorf("orasImage = %q, want the mirrored %q", got, mirrored)
	}
	if got := resolveServing(t, Sources{})["app-serving:orasImage"]; got != DefaultORASImage {
		t.Errorf("orasImage = %q, want the upstream %q", got, DefaultORASImage)
	}
}

func packageModel(t *testing.T, projectDir, category, name, slug string) {
	t.Helper()

	for _, dir := range []string{"gaie-" + slug, "ms-" + slug} {
		path := filepath.Join(projectDir, deploymentFolder, modelFolder, category, name, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "values.yaml"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestModelCategory(t *testing.T) {
	project := t.TempDir()
	packageModel(t, project, CategoryGenerative, "GPT-OSS-20B", "gpt-oss-20b")
	packageModel(t, project, CategoryEmbedder, "BGE-M3", "bge-m3")

	for name, want := range map[string]string{"GPT-OSS-20B": CategoryGenerative, "BGE-M3": CategoryEmbedder} {
		if got, ok := ModelCategory(project, name); !ok || got != want {
			t.Errorf("ModelCategory(%q) = %q, %t, want %q", name, got, ok, want)
		}
	}
	if _, ok := ModelCategory(project, "not-packaged"); ok {
		t.Error("ModelCategory found a model the project does not package")
	}
}

// The volume names are what the stack creates, so an installer lookup by them
// finds the real PersistentVolumeClaim.
func TestModelVolumeMatchesTheDeployedNames(t *testing.T) {
	project := t.TempDir()
	packageModel(t, project, CategoryGenerative, "GPT-OSS-20B", "gpt-oss-20b")

	namespace, claim, err := ModelVolume(project, CategoryGenerative, "GPT-OSS-20B")
	if err != nil {
		t.Fatalf("ModelVolume() error = %v", err)
	}

	model := Model{ModelPaths: ModelPaths{Slug: "gpt-oss-20b"}}
	if namespace != "llm-d-gpt-oss-20b" || claim != model.ClaimName() {
		t.Errorf("volume = %s/%s, want llm-d-gpt-oss-20b/%s", namespace, claim, model.ClaimName())
	}
}
