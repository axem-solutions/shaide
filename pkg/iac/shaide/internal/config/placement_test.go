package appconfig

import (
	"testing"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
)

// A placement makes every component prefer the assigned pools, one preferred
// term per placement term, whatever the per-component selector says.
func TestPlacementReplacesTheNodeSelector(t *testing.T) {
	values := Values{
		NodeSelectorKey: "nodegroup",
		NodeSelector:    "cpu",
		Placement: []PlacementTerm{
			{Key: "nodegroup", Values: []string{"cpu-b", "cpu-a"}},
			{Key: "kubernetes.io/hostname", Values: []string{"server2"}},
		},
	}

	affinity := values.NodeAffinityFor("override")
	if affinity == nil {
		t.Fatal("NodeAffinityFor() = nil, want the placement")
	}
	if affinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		t.Error("the placement is a hard requirement; app-shaide must only prefer it")
	}
	if got := len(affinity.PreferredDuringSchedulingIgnoredDuringExecution.(corev1.PreferredSchedulingTermArray)); got != 2 {
		t.Errorf("preferred terms = %d, want one per placement term", got)
	}
}
