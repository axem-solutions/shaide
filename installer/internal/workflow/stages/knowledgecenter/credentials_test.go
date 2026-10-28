package knowledgecenter

import (
	"strings"
	"testing"

	kc "github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
)

func complete(t *testing.T, retained kc.Credentials) kc.Credentials {
	t.Helper()
	creds, err := CompleteCredentials(retained)
	if err != nil {
		t.Fatalf("CompleteCredentials() error = %v", err)
	}
	return creds
}

// A first install generates every credential, in the formats both stacks
// accept.
func TestCompleteCredentialsGeneratesValidCredentials(t *testing.T) {
	creds := complete(t, kc.Credentials{})

	if creds.EmbeddingUsername != EmbeddingUsername {
		t.Errorf("EmbeddingUsername = %q, want %q", creds.EmbeddingUsername, EmbeddingUsername)
	}
	for name, err := range map[string]error{
		"internal token":   kc.ValidateServiceToken(creds.InternalToken),
		"webhook token":    kc.ValidateServiceToken(creds.WebhookToken),
		"gateway token":    kc.ValidateGatewayToken(creds.GatewayToken),
		"scoped token":     kc.ValidateHopToken(creds.ScopedToken),
		"embedding pass":   kc.ValidatePassword(creds.EmbeddingPassword),
		"context key pair": kc.ValidateKeyPair(creds.ContextSigningKey, creds.ContextPublicKey),
	} {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if !strings.HasPrefix(creds.ContextSigningKey, "-----BEGIN PRIVATE KEY-----") {
		t.Error("the signing key is not PKCS#8 PEM")
	}
}

// Each run generates fresh values, so two installations never share one.
func TestCompleteCredentialsGeneratesDistinctValues(t *testing.T) {
	first, second := complete(t, kc.Credentials{}), complete(t, kc.Credentials{})

	if first.InternalToken == second.InternalToken || first.ContextSigningKey == second.ContextSigningKey {
		t.Error("two runs generated the same credentials")
	}
}

// An update keeps what the running deployments use and fills in only what is
// missing.
func TestCompleteCredentialsKeepsRetainedValues(t *testing.T) {
	retained := complete(t, kc.Credentials{})
	partial := retained
	partial.WebhookToken = ""

	got := complete(t, partial)

	if got.InternalToken != retained.InternalToken ||
		got.GatewayToken != retained.GatewayToken ||
		got.ScopedToken != retained.ScopedToken ||
		got.EmbeddingUsername != retained.EmbeddingUsername ||
		got.EmbeddingPassword != retained.EmbeddingPassword ||
		got.ContextSigningKey != retained.ContextSigningKey ||
		got.ContextPublicKey != retained.ContextPublicKey {
		t.Error("a retained credential was replaced")
	}
	if got.WebhookToken == "" || got.WebhookToken == retained.WebhookToken {
		t.Error("the missing webhook token was not generated")
	}
}

// A key pair is only usable whole, so a missing or mismatched half replaces
// both.
func TestCompleteCredentialsReplacesABrokenKeyPair(t *testing.T) {
	retained := complete(t, kc.Credentials{})
	other := complete(t, kc.Credentials{})

	for name, broken := range map[string]kc.Credentials{
		"missing public key": func() kc.Credentials { c := retained; c.ContextPublicKey = ""; return c }(),
		"mismatched halves":  func() kc.Credentials { c := retained; c.ContextPublicKey = other.ContextPublicKey; return c }(),
	} {
		t.Run(name, func(t *testing.T) {
			got := complete(t, broken)

			if got.ContextSigningKey == retained.ContextSigningKey {
				t.Error("the signing key of a broken pair was kept")
			}
			if err := kc.ValidateKeyPair(got.ContextSigningKey, got.ContextPublicKey); err != nil {
				t.Errorf("the replaced pair does not match: %v", err)
			}
		})
	}
}

// A retained value that no longer passes the stacks' rules fails here,
// naming the field and not echoing the value.
func TestCompleteCredentialsRejectsInvalidRetainedValues(t *testing.T) {
	retained := complete(t, kc.Credentials{})

	for name, tc := range map[string]struct {
		creds kc.Credentials
		field string
	}{
		"short token":  {func() kc.Credentials { c := retained; c.InternalToken = "short"; return c }(), kc.FieldInternalToken},
		"reused token": {func() kc.Credentials { c := retained; c.ScopedToken = c.GatewayToken; return c }(), kc.FieldScopedToken},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CompleteCredentials(tc.creds)
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("CompleteCredentials() error = %v, want one naming %s", err, tc.field)
			}
			if strings.Contains(err.Error(), retained.GatewayToken) {
				t.Error("the error echoes a credential")
			}
		})
	}
}
