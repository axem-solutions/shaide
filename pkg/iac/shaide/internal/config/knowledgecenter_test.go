package appconfig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

const kcImage = "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide_knowledge_center:v0.2.0"

// answers answers prompts by title, so a test can pin a prompted secret.
type answers map[string]string

func (a answers) Input(title, _, defaultValue string) (string, error) {
	if answer, ok := a[title]; ok {
		return answer, nil
	}
	return "prompted-" + strings.ReplaceAll(title, " ", "-") + "-0123456789abcdef", nil
}

func (a answers) Select(_, current string, _ []string) (string, error) { return current, nil }

func (a answers) MultiSelect(string, []string) ([]string, error) { return nil, nil }

func kcSources(enabled bool) Sources {
	sources := completeSources()
	sources.Images[knowledgecenter.ImageName] = kcImage
	sources.KnowledgeCenterEnabled = &enabled
	sources.KnowledgeCenter = knowledgecentertest.Credentials()
	return sources
}

func resolveKC(sources Sources, prompter answers) (map[string]string, map[string]bool, error) {
	cfg := New("/projects/app-shaide", stack.Options{Platform: cluster.Azure}, sources)
	resolved, err := cfg.Resolve(prompter)
	if err != nil {
		return nil, nil, err
	}
	values := map[string]string{}
	secret := map[string]bool{}
	for key, value := range resolved {
		values[key] = value.Value
		secret[key] = value.Secret
	}
	return values, secret, nil
}

var kcCredentialKeys = []string{
	"app-shaide:kcInternalToken",
	"app-shaide:kcWebhookToken",
	"app-shaide:kcGatewayToken",
	"app-shaide:kcScopedToken",
	"app-shaide:kcContextSigningKey",
	"app-shaide:kcContextPublicKey",
	"app-shaide:kcEmbeddingUsername",
	"app-shaide:kcEmbeddingPassword",
}

// The control panel reads the answer from KNOWLEDGE_CENTER_ENABLED, so it is
// written whichever way it went; unasked means not installed.
func TestKnowledgeCenterAnswerIsWritten(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources Sources
		want    string
	}{
		{"declined", kcSources(false), "false"},
		{"accepted", kcSources(true), "true"},
		{"not asked", completeSources(), "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values, _, err := resolveKC(tc.sources, answers{})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got := values["app-shaide:knowledgeCenterEnabled"]; got != tc.want {
				t.Errorf("knowledgeCenterEnabled = %q, want %q", got, tc.want)
			}
		})
	}
}

// A declined KC needs neither its image nor its credentials, and leaves no
// KC value behind in the stack config.
func TestDeclinedKnowledgeCenterRequiresNothing(t *testing.T) {
	disabled := false
	sources := completeSources() // no KC image, no credentials
	sources.KnowledgeCenterEnabled = &disabled

	values, _, err := resolveKC(sources, answers{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	for _, key := range append([]string{"app-shaide:knowledgeCenterImage"}, kcCredentialKeys...) {
		if _, ok := values[key]; ok {
			t.Errorf("%s was written for a declined Knowledge Center", key)
		}
	}
}

// Enabling KC adds no question: every value comes from the installer.
func TestKnowledgeCenterAddsNoPrompt(t *testing.T) {
	cfg := New("/projects/app-shaide", stack.Options{Platform: cluster.Azure}, kcSources(true))
	prompter := &stubPrompter{}
	if _, err := cfg.Resolve(prompter); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(prompter.asked) != 4 {
		t.Errorf("asked %v, want only the four core secrets", prompter.asked)
	}
}

func TestKnowledgeCenterImageAndCredentialsAreWritten(t *testing.T) {
	sources := kcSources(true)
	values, secret, err := resolveKC(sources, answers{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if got := values["app-shaide:knowledgeCenterImage"]; got != kcImage {
		t.Errorf("knowledgeCenterImage = %q, want the Harbor reference %q", got, kcImage)
	}

	c := sources.KnowledgeCenter
	for key, want := range map[string]string{
		"app-shaide:kcInternalToken":     c.InternalToken,
		"app-shaide:kcWebhookToken":      c.WebhookToken,
		"app-shaide:kcGatewayToken":      c.GatewayToken,
		"app-shaide:kcScopedToken":       c.ScopedToken,
		"app-shaide:kcContextSigningKey": c.ContextSigningKey,
		"app-shaide:kcContextPublicKey":  c.ContextPublicKey,
		"app-shaide:kcEmbeddingUsername": c.EmbeddingUsername,
		"app-shaide:kcEmbeddingPassword": c.EmbeddingPassword,
	} {
		if values[key] != want {
			t.Errorf("%s was not written verbatim", key)
		}
	}
	for _, key := range kcCredentialKeys {
		if key == "app-shaide:kcEmbeddingUsername" {
			continue
		}
		if !secret[key] {
			t.Errorf("%s was written without the secret flag", key)
		}
	}
}

func TestEnabledKnowledgeCenterRequiresItsImage(t *testing.T) {
	sources := kcSources(true)
	delete(sources.Images, knowledgecenter.ImageName)

	_, _, err := resolveKC(sources, answers{})
	if err == nil || !strings.Contains(err.Error(), "knowledgeCenterImage") {
		t.Fatalf("Resolve() error = %v, want the missing KC image named", err)
	}
}

func TestEnabledKnowledgeCenterRequiresEveryCredential(t *testing.T) {
	unsetters := map[string]func(*knowledgecenter.Credentials){
		"kcInternalToken":     func(c *knowledgecenter.Credentials) { c.InternalToken = "" },
		"kcWebhookToken":      func(c *knowledgecenter.Credentials) { c.WebhookToken = "" },
		"kcGatewayToken":      func(c *knowledgecenter.Credentials) { c.GatewayToken = "" },
		"kcScopedToken":       func(c *knowledgecenter.Credentials) { c.ScopedToken = "" },
		"kcContextSigningKey": func(c *knowledgecenter.Credentials) { c.ContextSigningKey = "" },
		"kcContextPublicKey":  func(c *knowledgecenter.Credentials) { c.ContextPublicKey = "" },
		"kcEmbeddingUsername": func(c *knowledgecenter.Credentials) { c.EmbeddingUsername = "" },
		"kcEmbeddingPassword": func(c *knowledgecenter.Credentials) { c.EmbeddingPassword = "" },
	}
	for key, unset := range unsetters {
		t.Run(key, func(t *testing.T) {
			sources := kcSources(true)
			unset(&sources.KnowledgeCenter)
			_, _, err := resolveKC(sources, answers{})
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Resolve() error = %v, want %s reported missing", err, key)
			}
		})
	}
}

func TestMalformedKnowledgeCenterCredentialsAreRejected(t *testing.T) {
	for name, mutate := range map[string]func(*knowledgecenter.Credentials){
		"short internal token": func(c *knowledgecenter.Credentials) { c.InternalToken = "too-short" },
		"spaced webhook token": func(c *knowledgecenter.Credentials) { c.WebhookToken = strings.Repeat("w", 32) + " x" },
		"padded gateway token": func(c *knowledgecenter.Credentials) { c.GatewayToken = c.GatewayToken + "=" },
		"short scoped token":   func(c *knowledgecenter.Credentials) { c.ScopedToken = c.ScopedToken[:20] },
		"bad signing key":      func(c *knowledgecenter.Credentials) { c.ContextSigningKey = "not a key" },
		"swapped keys": func(c *knowledgecenter.Credentials) {
			c.ContextSigningKey, c.ContextPublicKey = c.ContextPublicKey, c.ContextSigningKey
		},
		"mismatched pair": func(c *knowledgecenter.Credentials) { c.ContextPublicKey = knowledgecentertest.OtherPublicKey() },
		"spaced username": func(c *knowledgecenter.Credentials) { c.EmbeddingUsername = "kc user" },
	} {
		t.Run(name, func(t *testing.T) {
			sources := kcSources(true)
			mutate(&sources.KnowledgeCenter)
			if _, _, err := resolveKC(sources, answers{}); err == nil {
				t.Fatal("Resolve() accepted malformed credentials")
			}
		})
	}
}

// Each hop has its own credential, and none may double as the JWT secret or
// the admin password. The error names the fields, never the value.
func TestSharedKnowledgeCenterCredentialsAreRejected(t *testing.T) {
	jwt := "operator-chosen-jwt-secret-0123456789abcdef"
	admin := "operator-chosen-admin-password-0123456789"
	prompted := answers{"JWT signing secret": jwt, "Shaide admin password": admin}

	for name, mutate := range map[string]func(*knowledgecenter.Credentials){
		"internal equals webhook": func(c *knowledgecenter.Credentials) { c.WebhookToken = c.InternalToken },
		"gateway equals scoped":   func(c *knowledgecenter.Credentials) { c.ScopedToken = c.GatewayToken },
		"internal equals gateway": func(c *knowledgecenter.Credentials) { c.InternalToken = c.GatewayToken },
		"internal equals jwt":     func(c *knowledgecenter.Credentials) { c.InternalToken = jwt },
		"webhook equals jwt":      func(c *knowledgecenter.Credentials) { c.WebhookToken = jwt },
		"password equals admin":   func(c *knowledgecenter.Credentials) { c.EmbeddingPassword = admin },
		"password equals a token": func(c *knowledgecenter.Credentials) { c.EmbeddingPassword = c.InternalToken },
	} {
		t.Run(name, func(t *testing.T) {
			sources := kcSources(true)
			mutate(&sources.KnowledgeCenter)
			_, _, err := resolveKC(sources, prompted)
			if err == nil {
				t.Fatal("Resolve() accepted a shared credential")
			}
			for _, value := range []string{jwt, admin, sources.KnowledgeCenter.InternalToken, sources.KnowledgeCenter.GatewayToken} {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("error echoes a secret: %v", err)
				}
			}
		})
	}

	if _, _, err := resolveKC(kcSources(true), prompted); err != nil {
		t.Fatalf("Resolve() rejected distinct credentials: %v", err)
	}
}

// Disabled installations keep whatever credentials they have; the cross
// checks only apply to an enabled KC.
func TestDeclinedKnowledgeCenterIgnoresCredentials(t *testing.T) {
	sources := kcSources(false)
	sources.KnowledgeCenter.ScopedToken = sources.KnowledgeCenter.GatewayToken
	if _, _, err := resolveKC(sources, answers{}); err != nil {
		t.Fatalf("Resolve() error = %v for a declined KC", err)
	}
}

// The MCP namespace is not rewritten by the installer: an installation that
// pinned the legacy mcp-gateway namespace keeps it until it is migrated
// deliberately. The program default for new stacks is app-mcp.
func TestMCPNamespaceIsLeftToTheStack(t *testing.T) {
	values, _, err := resolveKC(kcSources(true), answers{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, ok := values["app-shaide:mcpNamespace"]; ok {
		t.Error("app-shaide:mcpNamespace was written; a hand-set legacy value would be overwritten")
	}

	v := Values{}
	applyDefaults(&v)
	if v.ShaideEnv.MCPNamespace != knowledgecenter.MCPNamespace {
		t.Errorf("default MCP namespace = %q, want %q", v.ShaideEnv.MCPNamespace, knowledgecenter.MCPNamespace)
	}
}

func TestKnowledgeCenterResourceDefaults(t *testing.T) {
	v := Values{}
	applyDefaults(&v)
	r := v.KnowledgeCenter.Resources
	if r.CPURequest == "" || r.MemoryRequest == "" || r.CPULimit == "" || r.MemoryLimit == "" {
		t.Errorf("resources = %+v, want every default set", r)
	}

	v = Values{KnowledgeCenter: KnowledgeCenterValues{Resources: Resources{MemoryLimit: "4Gi"}}}
	applyDefaults(&v)
	if v.KnowledgeCenter.Resources.MemoryLimit != "4Gi" {
		t.Errorf("a configured memory limit was replaced: %+v", v.KnowledgeCenter.Resources)
	}
}

// The deployment refuses to put KC next to a legacy MCP discovery namespace:
// shaide-server watches one namespace, and KC-MCP runs in app-mcp.
func TestValidateRejectsLegacyMCPNamespaceWithKnowledgeCenter(t *testing.T) {
	v := Values{Namespace: DefaultNamespace, KnowledgeCenterEnabled: true, ShaideEnv: AppEnv{MCPNamespace: "mcp-gateway"}}
	v.Images.KnowledgeCenter = kcImage
	err := v.Validate()
	if err == nil || !strings.Contains(err.Error(), "mcp-gateway") {
		t.Fatalf("Validate() error = %v, want the legacy namespace reported", err)
	}

	v.ShaideEnv.MCPNamespace = knowledgecenter.MCPNamespace
	if err := v.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	v.Namespace = "other"
	if err := v.Validate(); err == nil {
		t.Fatal("Validate() accepted KC outside app-shaide")
	}

	legacy := Values{Namespace: "other", ShaideEnv: AppEnv{MCPNamespace: "mcp-gateway"}}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("Validate() rejected a KC-less legacy installation: %v", err)
	}
}

func TestResolveSigningJWTContract(t *testing.T) {
	for _, kind := range []string{"exact", "trimmed", "distinct"} {
		t.Run(kind, func(t *testing.T) {
			sources := kcSources(true)
			signing := sources.KnowledgeCenter.ContextSigningKey + " \t\r\n"
			sources.KnowledgeCenter.ContextSigningKey = signing
			jwt := signing
			if kind == "trimmed" {
				jwt = " \t\n" + strings.TrimSpace(signing) + "\r\n "
			} else if kind == "distinct" {
				jwt = "  distinct-jwt-signing-secret-0123456789  "
			}
			values, secrets, err := resolveKC(sources, answers{"JWT signing secret": jwt})
			if kind == "distinct" {
				if err != nil {
					t.Fatal("distinct signing and JWT credentials rejected")
				}
				for key, want := range map[string]string{"app-shaide:kcContextSigningKey": signing, "app-shaide:jwtSecret": jwt} {
					if values[key] != want || !secrets[key] {
						t.Errorf("%s lost original bytes or Secret flag", key)
					}
				}
				return
			}
			if err == nil {
				t.Fatal("Resolve accepted signing PEM reused as JWT secret")
			}
			if !strings.Contains(err.Error(), "kcContextSigningKey and jwtSecret must differ") {
				t.Error("Resolve did not return the dedicated signing/JWT field-only error")
			}
			for _, value := range []string{signing, jwt, strings.TrimSpace(signing)} {
				if strings.Contains(err.Error(), value) {
					t.Error("Resolve error leaked signing/JWT credential")
				}
			}
		})
	}
}

func TestResolveSigningFileByteBoundary(t *testing.T) {
	for _, size := range []int{32768, 32769} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			sources := kcSources(true)
			signing := sources.KnowledgeCenter.ContextSigningKey
			signing += strings.Repeat(" ", size-len(signing))
			sources.KnowledgeCenter.ContextSigningKey = signing
			values, secrets, err := resolveKC(sources, answers{})
			if size == 32768 {
				if err != nil {
					t.Fatal("Resolve rejected valid matching 32768-byte signing PEM")
				}
				if values["app-shaide:kcContextSigningKey"] != signing || !secrets["app-shaide:kcContextSigningKey"] {
					t.Error("Resolve changed original signing bytes or lost Secret flag")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), knowledgecenter.FieldContextSigningKey+": must be at most 32768 bytes") {
				t.Error("Resolve did not reject original 32769-byte signing file")
			}
			if err != nil && strings.Contains(err.Error(), strings.TrimSpace(signing)) {
				t.Error("Resolve error leaked signing PEM")
			}
		})
	}
}
