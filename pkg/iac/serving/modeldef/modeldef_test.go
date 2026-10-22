package modeldef

import (
	"strings"
	"testing"
)

const minimal = `
slug: example
model:
  name: org/example
  size: 10Gi
`

func TestParseAppliesDefaults(t *testing.T) {
	def, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}

	if def.Server.Engine != EngineVLLM || def.ServerReplicas() != 1 || def.RouterReplicas() != 1 {
		t.Errorf("defaults = %q, %d, %d", def.Server.Engine, def.ServerReplicas(), def.RouterReplicas())
	}
	if gpus, err := def.GPUsPerPod(); err != nil || gpus != 0 {
		t.Errorf("GPUsPerPod() = %d, %v, want 0 without a GPU limit", gpus, err)
	}
}

func TestParseReadsTheServer(t *testing.T) {
	def, err := Parse([]byte(minimal + `
revision: "0123456789abcdef0123"
dependencies:
  - {id: org/tokenizer, revision: "abcdefabcdefabcdef12"}
server:
  replicas: 2
  args: ["--max-model-len", "4096"]
  env:
    - {name: VLLM_LOGGING_LEVEL, value: INFO}
  resources:
    limits: {nvidia.com/gpu: "2", memory: 40Gi}
  runtimeMetadata: true
router:
  replicas: 2
`))
	if err != nil {
		t.Fatal(err)
	}

	if gpus, err := def.GPUsPerPod(); err != nil || gpus != 2 {
		t.Errorf("GPUsPerPod() = %d, %v, want 2", gpus, err)
	}
	if def.ServerReplicas() != 2 || def.RouterReplicas() != 2 || !def.Server.RuntimeMetadata {
		t.Errorf("server = %+v, router = %+v", def.Server, def.Router)
	}
	if len(def.Dependencies) != 1 || def.Dependencies[0].ID != "org/tokenizer" || def.Revision == "" {
		t.Errorf("pins = %q %+v", def.Revision, def.Dependencies)
	}
	if def.Server.Env[0].Name != "VLLM_LOGGING_LEVEL" {
		t.Errorf("env = %+v", def.Server.Env)
	}
}

func TestParseRejectsInvalidDefinitions(t *testing.T) {
	cases := map[string]string{
		"unknown field":         minimal + "servre: {}\n",
		"missing slug":          "model: {name: org/x, size: 1Gi}\n",
		"invalid slug":          strings.Replace(minimal, "slug: example", "slug: Example_Model", 1),
		"missing name":          "slug: example\nmodel: {size: 1Gi}\n",
		"invalid size":          strings.Replace(minimal, "10Gi", "ten", 1),
		"unknown engine":        minimal + "server: {engine: sglang}\n",
		"sim with GPUs":         minimal + "server: {engine: sim, resources: {limits: {nvidia.com/gpu: 1}}}\n",
		"sim with metadata":     minimal + "server: {engine: sim, image: sim, runtimeMetadata: true}\n",
		"fractional GPUs":       minimal + "server: {resources: {limits: {nvidia.com/gpu: 500m}}}\n",
		"no router replicas":    minimal + "router: {replicas: 0}\n",
		"negative server count": minimal + "server: {replicas: -1}\n",
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(data)); err == nil {
				t.Error("Parse() accepted an invalid definition")
			}
		})
	}
}

// The simulator needs neither an image, which the stack supplies, nor a
// volume size, as it has no weights.
func TestParseAcceptsBareSimulator(t *testing.T) {
	def, err := Parse([]byte("slug: sim\nmodel: {name: shaide/simulator}\nserver: {engine: sim}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !def.Simulated() {
		t.Error("Simulated() = false for engine sim")
	}
	if gpus, _ := def.GPUsPerPod(); gpus != 0 {
		t.Errorf("GPUsPerPod() = %d, want 0", gpus)
	}
}

func TestValidateSlugLength(t *testing.T) {
	if err := ValidateSlug(strings.Repeat("a", maxSlugLength)); err != nil {
		t.Errorf("a %d character slug: %v", maxSlugLength, err)
	}
	if err := ValidateSlug(strings.Repeat("a", maxSlugLength+1)); err == nil {
		t.Error("an overlong slug was accepted")
	}
}
