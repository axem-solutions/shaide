package config

import (
	"reflect"
	"testing"
)

// Each placement term becomes its own nodeSelectorTerm, so a model can run on
// pools named by different labels, with the pool names sorted for stable diffs.
func TestPlacementTermsAreAlternatives(t *testing.T) {
	m := Model{
		NodeSelector: map[string]string{"nodegroup": "generative"},
		Placement: []PlacementTerm{
			{Key: "nodegroup", Values: []string{"gpu-pro", "gpu"}},
			{Key: "kubernetes.io/hostname", Values: []string{"server4"}},
		},
	}

	want := [][]nodeRequirement{
		{{key: "nodegroup", values: []string{"gpu", "gpu-pro"}}},
		{{key: "kubernetes.io/hostname", values: []string{"server4"}}},
	}
	if got := m.placementTerms(); !reflect.DeepEqual(got, want) {
		t.Errorf("placementTerms() = %+v, want %+v", got, want)
	}

	if m.NodeAffinityArgs() == nil {
		t.Error("NodeAffinityArgs() = nil, want the placement")
	}
}

// Without a placement the node selector applies as one term ANDing its keys,
// as it did before placements existed.
func TestNodeSelectorIsOneTerm(t *testing.T) {
	m := Model{NodeSelector: map[string]string{"nodegroup": "generative", "gpu-type": "nvidia-a10"}}

	want := [][]nodeRequirement{{
		{key: "gpu-type", values: []string{"nvidia-a10"}},
		{key: "nodegroup", values: []string{"generative"}},
	}}
	if got := m.placementTerms(); !reflect.DeepEqual(got, want) {
		t.Errorf("placementTerms() = %+v, want %+v", got, want)
	}
}

func TestNoPlacementAddsNoAffinity(t *testing.T) {
	m := Model{}

	if m.NodeAffinityArgs() != nil || len(m.NodeAffinityMap()) != 0 {
		t.Error("a model without placement got a node affinity")
	}
}
