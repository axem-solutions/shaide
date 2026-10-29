package mcp

import (
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"os"
	"strings"
	"testing"
)

// This bounds the platform handoff; mocks do not simulate prior Pulumi state
// or deletions, and this test never invokes an installer or a destroy operation.
func TestKnowledgeCenterOnlyDisableRequiresTargetedTeardown(t *testing.T) {
	before := mustDeploy(t, kcOptions(true))
	before.Must(t, typeDeployment, "kc-mcp")
	before.Must(t, typeSecret, "kc-mcp-context")
	// The ordinary false+empty path must remain invalid, not become a
	// permissive empty desired state for teardown.
	stack := NewStack("/projects/app-mcp", stackpkg.Options{}, kcOptions(false))
	if _, err := stack.Config().Resolve(noPrompts{}); err == nil || !strings.Contains(err.Error(), "datasources") {
		t.Fatalf("false+empty Resolve must reject: %v", err)
	}
	cfg := directConfig()
	cfg["app-mcp:knowledgeCenterEnabled"] = auto.ConfigValue{Value: "false"}
	assertDirectRejected(t, cfg, "datasources")
	// With generic workloads retained, ordinary redeployment stays valid.
	after := mustDeploy(t, kcOptions(false, generic()))
	after.Must(t, typeDeployment, "mcp-jira")
	if _, ok := after.Find(typeDeployment, "kc-mcp"); ok {
		t.Error("disabled program still selects KC-MCP")
	}
	doc, err := os.ReadFile("../../../docs/operations/knowledge-center.md")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(doc), "## Disabling")
	if start < 0 {
		t.Fatal("missing disabling contract")
	}
	section := string(doc)[start:]
	if end := strings.Index(section[3:], "\n## "); end >= 0 {
		section = section[:end+3]
	}
	for _, required := range []string{"DestroyOnly", "previously managed", "Pulumi state", "generic", "skip"} {
		if !strings.Contains(section, required) {
			t.Errorf("disabling handoff omits %q", required)
		}
	}
}
