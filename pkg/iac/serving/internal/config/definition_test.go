package config

import (
	"encoding/json"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func TestInstallerGPUTolerationIsWrittenToStackConfig(t *testing.T) {
	want := Toleration{
		Key:      "nvidia.com/gpu",
		Operator: "Equal",
		Value:    "present",
		Effect:   "NoSchedule",
	}
	cfg := New("/projects/app-serving", stack.Options{
		Platform:   cluster.Azure,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	}, Sources{GPUToleration: &want})

	resolved, err := cfg.Resolve(nil)
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

func TestUnsetGPUTolerationRemainsUnwritten(t *testing.T) {
	cfg := New("/projects/app-serving", stack.Options{
		Platform:   cluster.Azure,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	}, Sources{})

	resolved, err := cfg.Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, ok := resolved["app-serving:gpuToleration"]; ok {
		t.Fatal("unset app-serving:gpuToleration was written")
	}
}
