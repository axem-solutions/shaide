package mcp

import (
	"testing"

	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// The installer destroys an app-mcp stack it deployed for KC-MCP alone, so it
// has to tell one that also runs generic data sources.
func TestHasDatasources(t *testing.T) {
	newStack := func(options Options) *Stack {
		return NewStack("/projects/app-mcp", stackpkg.Options{Kubeconfig: "/.kube/config", Context: "test"}, options)
	}
	resolve := func(options Options) auto.ConfigMap {
		t.Helper()
		resolved, err := newStack(options).Config().Resolve(noPrompts{})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		return resolved
	}

	stack := newStack(Options{})
	for _, tc := range []struct {
		name   string
		values auto.ConfigMap
		want   bool
	}{
		{"KC-MCP only", resolve(kcOptions(true)), false},
		{"with a generic data source", resolve(kcOptions(true, generic())), true},
		{"nothing stored", auto.ConfigMap{}, false},
		{"empty list", auto.ConfigMap{"app-mcp:datasources": {Value: "[]"}}, false},
		{"unreadable", auto.ConfigMap{"app-mcp:datasources": {Value: "not json"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stack.HasDatasources(tc.values); got != tc.want {
				t.Errorf("HasDatasources() = %v, want %v", got, tc.want)
			}
		})
	}
}
