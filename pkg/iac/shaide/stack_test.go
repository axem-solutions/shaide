package shaide

import (
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

// The installer places app-shaide on the nodes it labelled for CPU work, so
// the label it chose has to reach the stack config.
func TestInstallerOptionsCarryNodeSelectorToStackConfig(t *testing.T) {
	stack := newTestStack(Options{
		NodeSelectorKey: "axem.dev/workload-cpu",
		NodeSelector:    "true",
	})

	resolved, err := stack.Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	for key, want := range map[string]string{
		"app-shaide:nodeSelectorKey": "axem.dev/workload-cpu",
		"app-shaide:nodeSelector":    "true",
	} {
		if got := resolved[key].Value; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// Without installer values the keys are not written, so a value set by hand
// in Pulumi.<stack>.yaml survives.
func TestEmptyNodeSelectorIsNotWritten(t *testing.T) {
	resolved, err := newTestStack(Options{}).Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	for _, key := range []string{"app-shaide:nodeSelectorKey", "app-shaide:nodeSelector"} {
		if _, ok := resolved[key]; ok {
			t.Errorf("%s was written without an installer value", key)
		}
	}
}
