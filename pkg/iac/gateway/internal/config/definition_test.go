package config

import (
	"sort"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
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

func resolveFor(t *testing.T, provider cluster.Provider) (map[string]string, []string) {
	t.Helper()

	cfg := New("/projects/gateway-provider", stack.Options{
		Platform:   provider,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	}, Sources{ALBSubnetID: testSubnetID})

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
	cfg := New("/projects/gateway-provider", stack.Options{Platform: cluster.Azure}, Sources{})
	if err := cfg.Definition().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// Only values that must differ per cluster are asked for. Everything else is
// derived from the platform or shipped as a default, so an operator is not
// walked through settings that do not apply to their cluster.
func TestPromptsAreLimitedToPerClusterValues(t *testing.T) {
	tests := []struct {
		platform cluster.Provider
		want     []string
	}{
		{
			platform: cluster.Azure,
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
			platform: cluster.GCP,
			want: []string{
				"Gateway class name",
				"Gateway hostname",
				"cert-manager ClusterIssuer for Gateway TLS (empty serves HTTP only)",
			},
		},
		{
			platform: cluster.OnPrem,
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
	values, _ := resolveFor(t, cluster.Azure)

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
	values, _ := resolveFor(t, cluster.Azure)

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
	values, _ := resolveFor(t, cluster.Azure)

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
	tests := map[cluster.Provider]string{
		cluster.Azure:  "azure-alb-external",
		cluster.GCP:    "gke-l7-regional-external-managed",
		cluster.AWS:    "alb",
		cluster.OnPrem: "istio",
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
	values, _ := resolveFor(t, cluster.GCP)

	for _, key := range []string{"gateway-provider:albName", "gateway-provider:albSubnetId"} {
		if _, ok := values[key]; ok {
			t.Errorf("%s was written on GCP", key)
		}
	}
}

const testSubnetID = "/subscriptions/0000/resourceGroups/rg-test/providers/Microsoft.Network/virtualNetworks/vnet-test/subnets/snet-alb-test"

// answeringPrompter answers every input prompt with the same value.
type answeringPrompter struct {
	answer string
}

func (p *answeringPrompter) Input(string, string, string) (string, error) { return p.answer, nil }

func (p *answeringPrompter) Select(_, current string, _ []string) (string, error) {
	return current, nil
}

func (p *answeringPrompter) MultiSelect(string, []string) ([]string, error) { return nil, nil }

func azureConfig(sources Sources) Config {
	return New("/projects/gateway-provider", stack.Options{
		Platform:   cluster.Azure,
		Kubeconfig: "/.kube/config",
		Context:    "aks-test",
	}, sources)
}

// An empty association makes the ALB controller delete the Application
// Gateway for Containers, so on Azure the subnet cannot be left empty.
func TestALBSubnetIsRequiredOnAzure(t *testing.T) {
	if _, err := azureConfig(Sources{}).Resolve(&answeringPrompter{answer: ""}); err == nil {
		t.Fatal("Resolve() accepted an empty subnet on Azure")
	}
}

func TestALBSubnetMustBeASubnetID(t *testing.T) {
	_, err := azureConfig(Sources{}).Resolve(&answeringPrompter{answer: "snet-alb-test"})
	if err == nil || !strings.Contains(err.Error(), "not a subnet resource ID") {
		t.Fatalf("Resolve() error = %v, want the subnet ID rejected", err)
	}
}

// On an update the association already in place is offered, so accepting the
// default keeps the platform reachable.
func TestALBSubnetIsPrefilled(t *testing.T) {
	prompter := &stubPrompter{}
	resolved, err := azureConfig(Sources{ALBSubnetID: testSubnetID}).Resolve(prompter)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := resolved["gateway-provider:albSubnetId"].Value; got != testSubnetID {
		t.Errorf("albSubnetId = %q, want the pre-filled %q", got, testSubnetID)
	}
}

func TestALBSubnetPlaceholder(t *testing.T) {
	hint := "/subscriptions/0000/resourceGroups/rg-test/providers/Microsoft.Network/virtualNetworks/vnet-test/subnets/<subnet>"
	for _, entry := range azureConfig(Sources{ALBSubnetPlaceholder: hint}).definition.Entries {
		if entry.Key == KeyALBSubnetID && entry.Prompt.Placeholder != hint {
			t.Errorf("placeholder = %q, want %q", entry.Prompt.Placeholder, hint)
		}
	}
}

func TestValidateSubnetID(t *testing.T) {
	for _, value := range []string{
		testSubnetID,
		"/subscriptions/0000/resourcegroups/rg-test/providers/microsoft.network/virtualnetworks/vnet-test/subnets/snet",
	} {
		if err := ValidateSubnetID(value); err != nil {
			t.Errorf("ValidateSubnetID(%q) = %v, want nil", value, err)
		}
	}
	for _, value := range []string{
		"snet-alb-test",
		"/subscriptions/0000/resourceGroups/rg-test/providers/Microsoft.Network/virtualNetworks/vnet-test",
		"/subscriptions/0000/resourceGroups/rg-test/providers/Microsoft.Network/virtualNetworks/vnet-test/subnets/",
		testSubnetID + "/extra",
	} {
		if err := ValidateSubnetID(value); err == nil {
			t.Errorf("ValidateSubnetID(%q) = nil, want an error", value)
		}
	}
}
