package knowledgecenter_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
)

// The installer keys its Harbor map by these names; a rename here silently
// drops the image from the deployment.
func TestImageNamesAreTheUpstreamPackages(t *testing.T) {
	if knowledgecenter.ImageName != "axem-solutions/shaide_knowledge_center" {
		t.Errorf("ImageName = %q", knowledgecenter.ImageName)
	}
	if knowledgecenter.MCPImageName != "axem-solutions/shaide_knowledge_center_mcp" {
		t.Errorf("MCPImageName = %q", knowledgecenter.MCPImageName)
	}
}

func TestNamespacesAreFixed(t *testing.T) {
	if knowledgecenter.Namespace != "app-shaide" || knowledgecenter.MCPNamespace != "app-mcp" {
		t.Errorf("namespaces = %q, %q", knowledgecenter.Namespace, knowledgecenter.MCPNamespace)
	}
}

// shaide-server compares the discovered KC-MCP URL with the signed target's
// origin, and KC-MCP checks the Host header, so all three must agree.
func TestServiceOriginsAgree(t *testing.T) {
	if got, want := knowledgecenter.MCPURL(), "http://kc-mcp.app-mcp.svc.cluster.local:8080"; got != want {
		t.Errorf("MCPURL() = %q, want %q", got, want)
	}
	if !strings.HasPrefix(knowledgecenter.MCPURL(), "http://"+knowledgecenter.MCPHost()+":") {
		t.Errorf("MCPHost() %q is not the host of MCPURL() %q", knowledgecenter.MCPHost(), knowledgecenter.MCPURL())
	}
	if got, want := knowledgecenter.URL(), "http://knowledge-center.app-shaide.svc.cluster.local:8080"; got != want {
		t.Errorf("URL() = %q, want %q", got, want)
	}
	if got, want := knowledgecenter.LocalURL(), "http://knowledge-center:8080"; got != want {
		t.Errorf("LocalURL() = %q, want %q", got, want)
	}
}

// The server's parser denies unknown fields and requires the KC audience to
// differ from kc-mcp:<server_id>.
func TestContextTargetsMatchTheServerSchema(t *testing.T) {
	var targets []map[string]string
	if err := json.Unmarshal([]byte(knowledgecenter.ContextTargetsJSON()), &targets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %v, want exactly KC-MCP", targets)
	}
	want := map[string]string{
		"server_id":          "knowledge-center",
		"url":                "http://kc-mcp.app-mcp.svc.cluster.local:8080",
		"issuer":             knowledgecenter.Issuer,
		"kc_audience":        "kc:knowledge-center",
		"kid":                "kc-context-v1",
		"signing_key_file":   "/run/secrets/kc-context/private.pem",
		"gateway_token_file": "/run/secrets/kc-context/gateway_token",
	}
	if len(targets[0]) != len(want) {
		t.Errorf("target fields = %v, want %v", targets[0], want)
	}
	for key, value := range want {
		if targets[0][key] != value {
			t.Errorf("%s = %q, want %q", key, targets[0][key], value)
		}
	}
	if targets[0]["kc_audience"] == "kc-mcp:"+targets[0]["server_id"] {
		t.Error("kc_audience equals the KC-MCP audience")
	}
}

func TestFixtureCredentialsAreValid(t *testing.T) {
	c := knowledgecentertest.Credentials()
	for name, err := range map[string]error{
		"internal": knowledgecenter.ValidateServiceToken(c.InternalToken),
		"webhook":  knowledgecenter.ValidateServiceToken(c.WebhookToken),
		"gateway":  knowledgecenter.ValidateHopToken(c.GatewayToken),
		"scoped":   knowledgecenter.ValidateHopToken(c.ScopedToken),
		"signing":  knowledgecenter.ValidateSigningKey(c.ContextSigningKey),
		"public":   knowledgecenter.ValidatePublicKey(c.ContextPublicKey),
		"pair":     knowledgecenter.ValidateKeyPair(c.ContextSigningKey, c.ContextPublicKey),
		"user":     knowledgecenter.ValidateUsername(c.EmbeddingUsername),
		"password": knowledgecenter.ValidatePassword(c.EmbeddingPassword),
	} {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestServiceTokenRules(t *testing.T) {
	for _, bad := range []string{"", "short", strings.Repeat("a", 31), strings.Repeat("a", 31) + " b"} {
		if knowledgecenter.ValidateServiceToken(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestHopTokenRules(t *testing.T) {
	token := knowledgecentertest.HopToken('x')
	for _, bad := range []string{
		"",
		// padded
		strings.Repeat("a", 64) + "=",
		// not base64
		"not base64 at all!",
		// decodes to fewer than 32 bytes
		token[:40],
		// the server tolerates one LF in a file, not in the value itself
		token + "\n",
		// a wrapped value; Go's decoder alone would skip the line break
		token[:20] + "\r\n" + token[20:],
		// standard rather than URL alphabet
		strings.Replace(token, "e", "+", 1),
	} {
		if knowledgecenter.ValidateHopToken(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestMismatchedKeyPairIsRejected(t *testing.T) {
	c := knowledgecentertest.Credentials()
	if err := knowledgecenter.ValidateKeyPair(c.ContextSigningKey, knowledgecentertest.OtherPublicKey()); err == nil {
		t.Fatal("accepted a public key of another pair")
	}
}

func TestKeysAreNotSwappable(t *testing.T) {
	c := knowledgecentertest.Credentials()
	if knowledgecenter.ValidateSigningKey(c.ContextPublicKey) == nil {
		t.Error("public key accepted as signing key")
	}
	if knowledgecenter.ValidatePublicKey(c.ContextSigningKey) == nil {
		t.Error("private key accepted as public key")
	}
}

// A shared value would let one leaked credential open another hop. Errors
// name the fields, never the values.
func TestDistinctRejectsSharedValuesWithoutEchoingThem(t *testing.T) {
	err := knowledgecenter.ValidateDistinct(map[string]string{
		"gateway token": "same-secret-value",
		"scoped token":  "same-secret-value",
		"other":         "different",
		"unset":         "",
		"also unset":    "",
	})
	if err == nil {
		t.Fatal("accepted a reused credential")
	}
	if strings.Contains(err.Error(), "same-secret-value") {
		t.Errorf("error echoes the secret: %v", err)
	}
	if !strings.Contains(err.Error(), "gateway token") || !strings.Contains(err.Error(), "scoped token") {
		t.Errorf("error does not name both fields: %v", err)
	}
}
