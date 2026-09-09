package appconfig

import (
	"testing"

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

func datasource() Datasource {
	return Datasource{Name: "atlassian"}
}

func TestDefinitionValidates(t *testing.T) {
	cfg := New("/projects/app-mcp", stack.Options{}, Sources{Datasources: []Datasource{datasource()}})
	if err := cfg.Definition().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// Nothing is asked for: MCP data sources are selected from what has been
// published, not typed in, and the rest are defaults.
func TestNothingIsPrompted(t *testing.T) {
	cfg := New("/projects/app-mcp", stack.Options{Kubeconfig: "/.kube/config"}, Sources{
		Datasources: []Datasource{datasource()},
	})

	prompter := &stubPrompter{}
	if _, err := cfg.Resolve(prompter); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(prompter.asked) != 0 {
		t.Errorf("operator was asked %v; the MCP stack has nothing to ask", prompter.asked)
	}
}

// The stack cannot run without at least one data source, so an empty selection
// must fail at resolve time rather than inside the Pulumi program.
func TestDatasourcesAreRequired(t *testing.T) {
	cfg := New("/projects/app-mcp", stack.Options{}, Sources{})

	if _, err := cfg.Resolve(&stubPrompter{}); err == nil {
		t.Fatal("Resolve() succeeded with no data sources")
	}
}

// The MCP RBAC Role is bound to the Shaide Server ServiceAccount, so these must
// match what app-shaide deployed.
func TestShaideBindingDefaults(t *testing.T) {
	cfg := New("/projects/app-mcp", stack.Options{}, Sources{Datasources: []Datasource{datasource()}})

	resolved, err := cfg.Resolve(&stubPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	for key, want := range map[string]string{
		"app-mcp:namespace":                DefaultNamespace,
		"app-mcp:shaideNamespace":          DefaultShaideNamespace,
		"app-mcp:shaideServiceAccountName": DefaultShaideSAName,
	} {
		if resolved[key].Value != want {
			t.Errorf("%s = %q, want %q", key, resolved[key].Value, want)
		}
	}
}

// A data source list reaches the stack as JSON, which RequireObject reads back.
func TestDatasourcesAreEncoded(t *testing.T) {
	cfg := New("/projects/app-mcp", stack.Options{}, Sources{Datasources: []Datasource{datasource()}})

	resolved, err := cfg.Resolve(&stubPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	value := resolved["app-mcp:datasources"].Value
	if value == "" || value[0] != '[' {
		t.Errorf("datasources = %q, want a JSON array", value)
	}
}
