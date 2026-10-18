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

func testImages() map[string]string {
	return map[string]string{
		"axem-solutions/shaide_server":        "harbor/shaide/shaide_server:v1",
		"axem-solutions/shaide_control_panel": "harbor/shaide/shaide_control_panel:v1",
		"axem-solutions/shaide-webapp":        "harbor/shaide/shaide-webapp:v1",
		"rustfs/rustfs":                       "harbor/shaide/rustfs:v1",
		"qdrant/qdrant":                       "harbor/shaide/qdrant:v1",
		"busybox":                             "harbor/shaide/busybox:v1",
	}
}

// newTestStack fills in the installer values a deployment always has; images
// set by the caller are kept.
func newTestStack(options Options) *Stack {
	options.GatewayHostname = "shaide.example.com"
	if options.Images == nil {
		options.Images = testImages()
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

// The control panel reads the operator's answer from APP_ENABLED, so it has to
// reach the stack config whichever way it went.
func TestWebAppAnswerIsWritten(t *testing.T) {
	disabled, enabled := false, true

	for _, tc := range []struct {
		name   string
		answer *bool
		want   string
	}{
		{"declined", &disabled, "false"},
		{"accepted", &enabled, "true"},
		{"not asked", nil, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := newTestStack(Options{WebAppEnabled: tc.answer}).Config().Resolve(answeringPrompter{})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if got := resolved["app-shaide:webappEnabled"].Value; got != tc.want {
				t.Errorf("app-shaide:webappEnabled = %q, want %q", got, tc.want)
			}
		})
	}
}

// A declined webapp is not deployed, so its image is not required.
func TestDeclinedWebAppDoesNotRequireItsImage(t *testing.T) {
	disabled := false
	images := testImages()
	delete(images, "axem-solutions/shaide-webapp")

	resolved, err := newTestStack(Options{WebAppEnabled: &disabled, Images: images}).Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if _, ok := resolved["app-shaide:webappImage"]; ok {
		t.Error("app-shaide:webappImage was written for a declined webapp")
	}
}
