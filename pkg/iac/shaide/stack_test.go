package shaide

import (
	"encoding/json"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

// answeringPrompter accepts every default and answers every input, so the
// generated secrets resolve without a terminal.
type answeringPrompter struct{}

func (answeringPrompter) Input(_, _, defaultValue string) (string, error) {
	if defaultValue != "" {
		return defaultValue, nil
	}
	return "answer", nil
}

func (answeringPrompter) Select(_, current string, _ []string) (string, error) {
	return current, nil
}

func (answeringPrompter) MultiSelect(string, []string) ([]string, error) { return nil, nil }

func newTestStack(options Options) *Stack {
	options.GatewayHostname = "shaide.example.com"
	options.Images = map[string]string{
		"axem-solutions/shaide_server":        "harbor/shaide/shaide_server:v1",
		"axem-solutions/shaide_control_panel": "harbor/shaide/shaide_control_panel:v1",
		"axem-solutions/shaide-webapp":        "harbor/shaide/shaide-webapp:v1",
		"rustfs/rustfs":                       "harbor/shaide/rustfs:v1",
		"qdrant/qdrant":                       "harbor/shaide/qdrant:v1",
		"busybox":                             "harbor/shaide/busybox:v1",
	}

	return NewStack("/projects/app-shaide", stackpkg.Options{
		Platform:   cluster.Azure,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	}, options)
}

// The installer places app-shaide on the node pools assigned to CPU work, so
// the pools it chose have to reach the stack config.
func TestInstallerOptionsCarryPlacementToStackConfig(t *testing.T) {
	stack := newTestStack(Options{
		Placement: []PlacementTerm{
			{Key: "kubernetes.azure.com/agentpool", Values: []string{"shaide"}},
		},
	})

	resolved, err := stack.Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	value, ok := resolved["app-shaide:placement"]
	if !ok {
		t.Fatal("app-shaide:placement was not written")
	}

	var got []PlacementTerm
	if err := json.Unmarshal([]byte(value.Value), &got); err != nil {
		t.Fatalf("decode placement %q: %v", value.Value, err)
	}
	if len(got) != 1 || got[0].Key != "kubernetes.azure.com/agentpool" || len(got[0].Values) != 1 || got[0].Values[0] != "shaide" {
		t.Errorf("placement = %+v, want the CPU pool", got)
	}
}

// Without installer values the key is not written, so a node selector set by
// hand in Pulumi.<stack>.yaml stays in effect.
func TestEmptyPlacementIsNotWritten(t *testing.T) {
	resolved, err := newTestStack(Options{}).Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if _, ok := resolved["app-shaide:placement"]; ok {
		t.Error("app-shaide:placement was written without an installer value")
	}
}
