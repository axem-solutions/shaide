package serving

import (
	"encoding/json"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

func TestInstallerOptionsCarryGPUTolerationToStackConfig(t *testing.T) {
	want := Toleration{
		Key:      "nvidia.com/gpu",
		Operator: "Equal",
		Value:    "present",
		Effect:   "NoSchedule",
	}
	stack := NewStack("/projects/app-serving", stackpkg.Options{
		Platform:   cluster.Azure,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	}, Options{GPUToleration: &want})

	resolved, err := stack.Config().Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	value, ok := resolved["app-serving:gpuToleration"]
	if !ok {
		t.Fatal("app-serving:gpuToleration was not written")
	}

	var got Toleration
	if err := json.Unmarshal([]byte(value.Value), &got); err != nil {
		t.Fatalf("decode gpuToleration %q: %v", value.Value, err)
	}
	if got != want {
		t.Fatalf("gpuToleration = %+v, want %+v", got, want)
	}
}
