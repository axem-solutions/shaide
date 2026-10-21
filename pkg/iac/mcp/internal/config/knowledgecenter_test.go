package appconfig

import (
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

const kcMCPImage = "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide_knowledge_center_mcp:v0.2.0"

func kcSources(enabled bool) Sources {
	return Sources{
		KnowledgeCenterEnabled: &enabled,
		KnowledgeCenter:        knowledgecentertest.Credentials(),
		Images: map[string]string{
			knowledgecenter.MCPImageName: kcMCPImage,
			// Mirrored for app-shaide; app-mcp must ignore it.
			knowledgecenter.ImageName: "harbor/kc:v1",
		},
	}
}

func resolveMCP(sources Sources) (map[string]string, map[string]bool, error) {
	cfg := New("/projects/app-mcp", stack.Options{}, sources)
	prompter := &stubPrompter{}
	resolved, err := cfg.Resolve(prompter)
	if err != nil {
		return nil, nil, err
	}
	if len(prompter.asked) != 0 {
		return nil, nil, errAsked(prompter.asked)
	}
	values := map[string]string{}
	secret := map[string]bool{}
	for key, value := range resolved {
		values[key] = value.Value
		secret[key] = value.Secret
	}
	return values, secret, nil
}

type errAsked []string

func (e errAsked) Error() string { return "operator was asked " + strings.Join(e, ", ") }

// KC-MCP is the only MCP workload a Knowledge Center installation needs; it
// must not require unrelated generic datasources.
func TestKnowledgeCenterAloneNeedsNoDatasources(t *testing.T) {
	values, _, err := resolveMCP(kcSources(true))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, ok := values["app-mcp:datasources"]; ok {
		t.Error("app-mcp:datasources was written without any datasource")
	}
	if values["app-mcp:knowledgeCenterEnabled"] != "true" {
		t.Errorf("knowledgeCenterEnabled = %q", values["app-mcp:knowledgeCenterEnabled"])
	}
}

// Without KC the stack behaves as before: it still needs a datasource.
func TestDatasourcesStillRequiredWithoutKnowledgeCenter(t *testing.T) {
	if _, _, err := resolveMCP(kcSources(false)); err == nil {
		t.Fatal("Resolve() succeeded with neither KC nor datasources")
	}

	disabled := false
	values, _, err := resolveMCP(Sources{KnowledgeCenterEnabled: &disabled, Datasources: []Datasource{datasource()}})
	if err != nil {
		t.Fatalf("Resolve() error = %v for generic datasources only", err)
	}
	for _, key := range []string{"app-mcp:knowledgeCenterMcpImage", "app-mcp:kcGatewayToken", "app-mcp:kcScopedToken", "app-mcp:kcContextPublicKey"} {
		if _, ok := values[key]; ok {
			t.Errorf("%s written without KC", key)
		}
	}
}

// KC and generic datasources run side by side.
func TestKnowledgeCenterKeepsGenericDatasources(t *testing.T) {
	sources := kcSources(true)
	sources.Datasources = []Datasource{datasource()}
	values, _, err := resolveMCP(sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !strings.HasPrefix(values["app-mcp:datasources"], "[") {
		t.Errorf("datasources = %q", values["app-mcp:datasources"])
	}
}

// app-mcp receives only what KC-MCP verifies and presents: the public key and
// its two hop tokens. Never the signing key, the internal token, or storage
// credentials.
func TestOnlyKCMCPCredentialsAreWritten(t *testing.T) {
	sources := kcSources(true)
	values, secret, err := resolveMCP(sources)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	c := sources.KnowledgeCenter
	for key, want := range map[string]string{
		"app-mcp:knowledgeCenterMcpImage": kcMCPImage,
		"app-mcp:kcGatewayToken":          c.GatewayToken,
		"app-mcp:kcScopedToken":           c.ScopedToken,
		"app-mcp:kcContextPublicKey":      c.ContextPublicKey,
	} {
		if values[key] != want {
			t.Errorf("%s not written verbatim", key)
		}
	}
	for _, key := range []string{"app-mcp:kcGatewayToken", "app-mcp:kcScopedToken", "app-mcp:kcContextPublicKey"} {
		if !secret[key] {
			t.Errorf("%s not marked secret", key)
		}
	}
	for key, value := range values {
		for _, forbidden := range []string{c.ContextSigningKey, c.InternalToken, c.WebhookToken, c.EmbeddingPassword} {
			if value == forbidden {
				t.Errorf("%s carries a credential KC-MCP must not hold", key)
			}
		}
		if strings.Contains(key, "s3") || strings.Contains(strings.ToLower(key), "storage") {
			t.Errorf("%s: storage settings reached app-mcp", key)
		}
	}
}

func TestEnabledKCMCPRequiresImageAndCredentials(t *testing.T) {
	for key, mutate := range map[string]func(*Sources){
		"knowledgeCenterMcpImage": func(s *Sources) { delete(s.Images, knowledgecenter.MCPImageName) },
		"kcGatewayToken":          func(s *Sources) { s.KnowledgeCenter.GatewayToken = "" },
		"kcScopedToken":           func(s *Sources) { s.KnowledgeCenter.ScopedToken = "" },
		"kcContextPublicKey":      func(s *Sources) { s.KnowledgeCenter.ContextPublicKey = "" },
	} {
		t.Run(key, func(t *testing.T) {
			sources := kcSources(true)
			mutate(&sources)
			_, _, err := resolveMCP(sources)
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Resolve() error = %v, want %s reported", err, key)
			}
		})
	}
}

// KC-MCP itself refuses a gateway token equal to its scoped token.
func TestKCMCPCredentialRules(t *testing.T) {
	for name, mutate := range map[string]func(*knowledgecenter.Credentials){
		"gateway equals scoped": func(c *knowledgecenter.Credentials) { c.ScopedToken = c.GatewayToken },
		"padded gateway":        func(c *knowledgecenter.Credentials) { c.GatewayToken += "=" },
		"wrapped scoped":        func(c *knowledgecenter.Credentials) { c.ScopedToken = c.ScopedToken[:10] + "\n" + c.ScopedToken[10:] },
		"private key as public": func(c *knowledgecenter.Credentials) { c.ContextPublicKey = c.ContextSigningKey },
	} {
		t.Run(name, func(t *testing.T) {
			sources := kcSources(true)
			mutate(&sources.KnowledgeCenter)
			_, _, err := resolveMCP(sources)
			if err == nil {
				t.Fatal("Resolve() accepted invalid KC-MCP credentials")
			}
			if strings.Contains(err.Error(), sources.KnowledgeCenter.GatewayToken) {
				t.Fatalf("error echoes a token: %v", err)
			}
		})
	}
}

// The namespace is a program default: the installer does not overwrite a
// stack that pinned the legacy mcp-gateway namespace.
func TestNamespaceIsLeftToTheStack(t *testing.T) {
	values, _, err := resolveMCP(kcSources(true))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, ok := values["app-mcp:namespace"]; ok {
		t.Error("app-mcp:namespace was written; a pinned legacy namespace would be moved")
	}
	v := Values{}
	applyDefaults(&v)
	if v.Namespace != "app-mcp" {
		t.Errorf("default namespace = %q, want app-mcp", v.Namespace)
	}
}

func TestValidateKnowledgeCenterPlacement(t *testing.T) {
	ok := Values{Namespace: "app-mcp", ShaideNamespace: "app-shaide", KnowledgeCenterEnabled: true}
	ok.Images.KnowledgeCenterMCP = kcMCPImage
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	legacy := ok
	legacy.Namespace = "mcp-gateway"
	if err := legacy.Validate(); err == nil || !strings.Contains(err.Error(), "mcp-gateway") {
		t.Errorf("Validate() error = %v, want the legacy namespace reported", err)
	}

	otherShaide := ok
	otherShaide.ShaideNamespace = "elsewhere"
	if err := otherShaide.Validate(); err == nil {
		t.Error("Validate() accepted KC callers outside app-shaide")
	}

	noImage := ok
	noImage.Images.KnowledgeCenterMCP = ""
	if err := noImage.Validate(); err == nil {
		t.Error("Validate() accepted KC-MCP without an image")
	}

	withoutKC := Values{Namespace: "mcp-gateway", Datasources: []Datasource{{Name: "jira", Image: "harbor/jira:v1"}}}
	if err := withoutKC.Validate(); err != nil {
		t.Errorf("Validate() rejected a KC-less legacy stack: %v", err)
	}
}

func TestKCMCPResourceDefaults(t *testing.T) {
	v := Values{}
	applyDefaults(&v)
	r := v.KnowledgeCenterMCP.Resources
	if r.CPURequest == "" || r.MemoryRequest == "" || r.CPULimit == "" || r.MemoryLimit == "" {
		t.Errorf("resources = %+v", r)
	}
}
