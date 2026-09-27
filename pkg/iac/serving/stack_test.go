package serving

import (
	"os"
	"path/filepath"
	"testing"

	servingconfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

func stackOptions() stackpkg.Options {
	return stackpkg.Options{Platform: cluster.Azure, Kubeconfig: "/.kube/config", Context: "aks-test"}
}

func packagedProject(t *testing.T, models map[string]string) string {
	t.Helper()

	project := t.TempDir()
	for name, category := range models {
		slug := "slug-" + category
		for _, dir := range []string{"gaie-" + slug, "ms-" + slug} {
			path := filepath.Join(project, "deployments", "models", category, name, dir)
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "values.yaml"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return project
}

// The stack places each model by where its values are packaged, on the GPU
// pool, with its weights at hub/<ID> in the volume the ORAS Job fills.
func TestModelsInputPlacesModels(t *testing.T) {
	project := packagedProject(t, map[string]string{
		"GPT-OSS-20B": servingconfig.CategoryGenerative,
		"BGE-M3":      servingconfig.CategoryEmbedder,
	})

	input := modelsInput(project, []Model{
		{Name: "GPT-OSS-20B", ID: "openai/gpt-oss-20b", HarborRef: "harbor/ai-models/gpt-oss-20b:1.0.0", StorageSize: "70Gi"},
		{Name: "BGE-M3", ID: "BAAI/bge-m3", HarborRef: "harbor/ai-models/bge-m3:1.0.0"},
	}, nil)

	if len(input.Generative) != 1 || len(input.Embedder) != 1 {
		t.Fatalf("input = %+v, want one generative and one embedder model", input)
	}

	generative := input.Generative[0]
	if generative.ModelSource.ModelUri != "hub/openai/gpt-oss-20b" {
		t.Errorf("ModelUri = %q, want hub/openai/gpt-oss-20b", generative.ModelSource.ModelUri)
	}
	if got := generative.NodeSelector["nodegroup"]; got != "generative" {
		t.Errorf("node selector = %v, want the GPU pool", generative.NodeSelector)
	}
	if !generative.Enabled || generative.ModelSource.HarborRef != "harbor/ai-models/gpt-oss-20b:1.0.0" {
		t.Errorf("generative = %+v, want enabled with its Harbor reference", generative)
	}
}

// Serving a model from the wrong values directory would deploy the wrong
// runtime, so a model with no packaged values is dropped and logged.
func TestModelsInputDropsUnpackagedModels(t *testing.T) {
	project := packagedProject(t, nil)

	var logged []string
	input := modelsInput(project, []Model{{Name: "not-packaged"}}, func(format string, args ...any) {
		logged = append(logged, format)
	})

	if len(input.Generative)+len(input.Embedder) != 0 {
		t.Errorf("input = %+v, want the model dropped", input)
	}
	if len(logged) != 1 {
		t.Errorf("logged %d messages, want the skip logged", len(logged))
	}
}

func TestModelVolumes(t *testing.T) {
	project := packagedProject(t, map[string]string{"GPT-OSS-20B": servingconfig.CategoryGenerative})

	volumes, err := ModelVolumes(project, []Model{{Name: "GPT-OSS-20B"}, {Name: "not-packaged"}})
	if err != nil {
		t.Fatalf("ModelVolumes() error = %v", err)
	}

	want := ModelVolume{Model: "GPT-OSS-20B", Namespace: "llm-d-slug-generative", Claim: "slug-generative-model"}
	if len(volumes) != 1 || volumes[0] != want {
		t.Errorf("volumes = %+v, want [%+v]", volumes, want)
	}
}

func TestORASImageIsPickedFromTheMirroredImages(t *testing.T) {
	mirrored := "harbor.harbor.svc.cluster.local/services/oras-project/oras:v1.3.1"
	stack := NewStack(t.TempDir(), stackOptions(), Options{Images: map[string]string{
		servingconfig.ORASImageName: mirrored,
		"istio/pilot":               "harbor.harbor.svc.cluster.local/services/istio/pilot:1.28.1",
	}})

	resolved, err := stack.Config().Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := resolved["app-serving:orasImage"].Value; got != mirrored {
		t.Errorf("orasImage = %q, want %q", got, mirrored)
	}
}
