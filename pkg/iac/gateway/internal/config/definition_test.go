package config

import (
	"sort"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

type stubPrompter struct {
	asked []string
}

func (p *stubPrompter) Input(title, _, defaultValue string) (string, error) {
	p.asked = append(p.asked, title)
	return defaultValue, nil
}

func (p *stubPrompter) Select(title, current string, _ []string) (string, error) {
	p.asked = append(p.asked, title)
	return current, nil
}

func (p *stubPrompter) MultiSelect(title string, _ []string) ([]string, error) {
	p.asked = append(p.asked, title)
	return nil, nil
}

func resolveFor(t *testing.T, provider platform.Platform) (map[string]string, []string) {
	t.Helper()

	cfg := New("/projects/gateway-provider", stack.Options{
		Platform:   provider,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	})

	prompter := &stubPrompter{}
	resolved, err := cfg.Resolve(prompter)
	if err != nil {
		t.Fatalf("Resolve(%s) error = %v", provider, err)
	}

	values := map[string]string{}
	for key, value := range resolved {
		values[key] = value.Value
	}

	sort.Strings(prompter.asked)

	return values, prompter.asked
}

func TestDefinitionValidates(t *testing.T) {
	cfg := New("/projects/gateway-provider", stack.Options{Platform: platform.Azure})
	if err := cfg.Definition().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// Only values that must differ per cluster are asked for. Everything else is
// derived from the platform or shipped as a default, so an operator is not
// walked through settings that do not apply to their cluster.
func TestPromptsAreLimitedToPerClusterValues(t *testing.T) {
	tests := []struct {
		platform platform.Platform
		want     []string
	}{
		{
			platform: platform.Azure,
			want: []string{
				"Azure Application Gateway for Containers name",
				"Azure subnet resource ID for Application Gateway for Containers",
				"Gateway class name",
				"Gateway hostname",
				"cert-manager ClusterIssuer for Gateway TLS (empty serves HTTP only)",
			},
		},
		{
			// Application Gateway for Containers is Azure-only, so those two
			// questions must not appear anywhere else.
			platform: platform.GCP,
			want: []string{
				"Gateway class name",
				"Gateway hostname",
				"cert-manager ClusterIssuer for Gateway TLS (empty serves HTTP only)",
			},
		},
		{
			platform: platform.OnPrem,
			want: []string{
				"Gateway class name",
				"Gateway hostname",
				"cert-manager ClusterIssuer for Gateway TLS (empty serves HTTP only)",
			},
		},
	}

	for _, test := range tests {
		t.Run(string(test.platform), func(t *testing.T) {
			_, asked := resolveFor(t, test.platform)

			if len(asked) != len(test.want) {
				t.Fatalf("asked %v, want %v", asked, test.want)
			}
			for i := range test.want {
				if asked[i] != test.want[i] {
					t.Errorf("asked %v, want %v", asked, test.want)
					break
				}
			}
		})
	}
}

// Runtime state the installer already knows must be supplied, not asked for.
func TestRuntimeValuesAreInjected(t *testing.T) {
	values, _ := resolveFor(t, platform.Azure)

	want := map[string]string{
		"gateway-provider:cloudProvider": "azure",
		"gateway-provider:kubeconfig":    "/.kube/config",
		"gateway-provider:context":       "aks-test",
	}

	for key, expected := range want {
		if values[key] != expected {
			t.Errorf("%s = %q, want %q", key, values[key], expected)
		}
	}
}

// The installer image ships the CRDs beside the project, so an install must not
// reach GitHub for them.
func TestPackagedCRDPathsAreWritten(t *testing.T) {
	values, _ := resolveFor(t, platform.Azure)

	if got := values["gateway-provider:gatewayApiCrdsPath"]; got != PackagedGatewayAPICRDsPath {
		t.Errorf("gatewayApiCrdsPath = %q, want %q", got, PackagedGatewayAPICRDsPath)
	}
	if got := values["gateway-provider:gieCrdsPath"]; got != PackagedGIECRDsPath {
		t.Errorf("gieCrdsPath = %q, want %q", got, PackagedGIECRDsPath)
	}
}

// An unprompted, unset entry must not be written, so a value placed in the
// stack file by hand survives a later run.
func TestUnsetEntriesAreNotWritten(t *testing.T) {
	values, _ := resolveFor(t, platform.Azure)

	for _, key := range []string{
		"gateway-provider:gatewayStaticIPName",
		"gateway-provider:gatewayStaticIP",
		"gateway-provider:gatewayCertName",
		"gateway-provider:tlsCertAnnotation",
		"gateway-provider:infraStackRef",
	} {
		if _, ok := values[key]; ok {
			t.Errorf("%s was written; it should be left for the program default or a hand-set value", key)
		}
	}
}

// The platform default is offered as the prompt's answer, not applied behind
// the operator's back: Azure clusters split between Application Gateway for
// Containers and Istio, so the default is right for only some of them.
func TestGatewayClassDefaultsToThePlatformValue(t *testing.T) {
	tests := map[platform.Platform]string{
		platform.Azure:  "azure-alb-external",
		platform.GCP:    "gke-l7-regional-external-managed",
		platform.AWS:    "alb",
		platform.OnPrem: "istio",
	}

	for provider, want := range tests {
		t.Run(string(provider), func(t *testing.T) {
			values, asked := resolveFor(t, provider)

			if got := values["gateway-provider:gatewayClassName"]; got != want {
				t.Errorf("gatewayClassName = %q, want the %s default %q", got, provider, want)
			}

			var prompted bool
			for _, title := range asked {
				if title == "Gateway class name" {
					prompted = true
				}
			}
			if !prompted {
				t.Error("the gateway class was applied without asking")
			}
		})
	}
}

// Azure-only entries must not leak onto other platforms even as empty keys.
func TestALBKeysAreAzureOnly(t *testing.T) {
	values, _ := resolveFor(t, platform.GCP)

	for _, key := range []string{"gateway-provider:albName", "gateway-provider:albSubnetId"} {
		if _, ok := values[key]; ok {
			t.Errorf("%s was written on GCP", key)
		}
	}
}
