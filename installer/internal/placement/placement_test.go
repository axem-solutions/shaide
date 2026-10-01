package placement

import (
	"reflect"
	"testing"
)

func TestPoolOf(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   Pool
	}{
		{"AKS", map[string]string{"kubernetes.azure.com/agentpool": "generative", "nodegroup": "generative"}, Pool{"kubernetes.azure.com/agentpool", "generative"}},
		{"EKS", map[string]string{"eks.amazonaws.com/nodegroup": "gpu"}, Pool{"eks.amazonaws.com/nodegroup", "gpu"}},
		{"GKE", map[string]string{"cloud.google.com/gke-nodepool": "l4"}, Pool{"cloud.google.com/gke-nodepool", "l4"}},
		{"on-prem node group", map[string]string{"nodegroup": "gpu-pro", "kubernetes.io/hostname": "server4"}, Pool{"nodegroup", "gpu-pro"}},
		{"on-prem single node", map[string]string{"kubernetes.io/hostname": "server4"}, Pool{"kubernetes.io/hostname", "server4"}},
		{"no labels", nil, Pool{"kubernetes.io/hostname", "node-name"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := PoolOf("node-name", test.labels); got != test.want {
				t.Errorf("PoolOf() = %+v, want %+v", got, test.want)
			}
		})
	}
}

// Pools named by the same label share a term; different labels are separate
// terms, which the stacks treat as alternatives.
func TestTerms(t *testing.T) {
	got := Terms([]Pool{
		{"nodegroup", "gpu-pro"},
		{"kubernetes.io/hostname", "server4"},
		{"nodegroup", "gpu"},
	})

	want := []Term{
		{Key: "kubernetes.io/hostname", Values: []string{"server4"}},
		{Key: "nodegroup", Values: []string{"gpu", "gpu-pro"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Terms() = %+v, want %+v", got, want)
	}

	if Terms(nil) == nil || len(Terms(nil)) != 0 {
		t.Error("Terms(nil) should be empty")
	}
}

func TestIsLegacyLabel(t *testing.T) {
	for key, want := range map[string]bool{
		"axem.dev/workload-cpu":          true,
		"axem.dev/model-gpt-oss-20b":     true,
		"axem.dev/platform":              false,
		"kubernetes.azure.com/agentpool": false,
	} {
		if got := IsLegacyLabel(key); got != want {
			t.Errorf("IsLegacyLabel(%q) = %v, want %v", key, got, want)
		}
	}
}
