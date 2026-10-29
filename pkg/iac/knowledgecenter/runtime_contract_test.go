package knowledgecenter_test

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
	"strings"
	"testing"
)

func TestSigningPEMEnvelope(t *testing.T) {
	key := knowledgecentertest.Credentials().ContextSigningKey
	block, _ := pem.Decode([]byte(key))
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pkcs1 := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(parsed.(*rsa.PrivateKey))}))
	if err := knowledgecenter.ValidateSigningKey(pkcs1); err != nil {
		t.Fatalf("valid PKCS#1 key: %v", err)
	}
	for name, value := range map[string]string{
		"leading-space": " " + key, "leading-newline": "\n" + key, "leading-material": "comment\n" + key,
		"trailing-material": key + "comment", "second-block": key + key,
		"pem-headers": strings.Replace(key, "-----BEGIN PRIVATE KEY-----\n", "-----BEGIN PRIVATE KEY-----\nComment: unsupported\n\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := knowledgecenter.ValidateSigningKey(value); err == nil {
				t.Error("accepted signing envelope rejected by server")
			}
		})
	}
	for name, value := range map[string]string{"original": key, "trailing-whitespace": key + " \t\r\n", "no-final-LF": strings.TrimSuffix(key, "\n")} {
		t.Run(name, func(t *testing.T) {
			if err := knowledgecenter.ValidateSigningKey(value); err != nil {
				t.Fatalf("valid envelope: %v", err)
			}
		})
	}
}

func TestGatewayTokenFileSize(t *testing.T) {
	for _, n := range []int{3071, 3072, 3073} {
		token := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("g", n)))
		err := knowledgecenter.ValidateGatewayToken(token)
		if (err != nil) != (len(token) > 4096) {
			t.Errorf("encoded bytes=%d decoded bytes=%d rejected=%t", len(token), n, err != nil)
		}
		if knowledgecenter.ValidateHopToken(token) != nil {
			t.Error("scoped token incorrectly restricted by gateway file bound")
		}
		if knowledgecenter.ValidateServiceToken(token) != nil {
			t.Error("service token incorrectly restricted by gateway file bound")
		}
	}
	if knowledgecenter.ValidateGatewayToken(strings.Repeat("g", 4097)) == nil {
		t.Error("accepted 4097-byte gateway file")
	}
}

func TestSigningFileByteBoundary(t *testing.T) {
	c := knowledgecentertest.Credentials()
	for _, size := range []int{32768, 32769} {
		for _, padding := range []string{"trailing", "body", "multibyte"} {
			t.Run(fmt.Sprintf("%d/%s", size, padding), func(t *testing.T) {
				key := c.ContextSigningKey
				space := strings.Repeat(" ", size-len(key))
				if padding == "body" {
					key = strings.Replace(key, "-----BEGIN PRIVATE KEY-----", "-----BEGIN PRIVATE KEY-----"+space, 1)
				} else if padding == "multibyte" {
					key += strings.Repeat("\u2003", (size-len(key))/3) + strings.Repeat(" ", (size-len(key))%3)
				} else {
					key += space
				}
				if len(key) != size {
					t.Fatal("fixture does not have requested original byte length")
				}
				for name, check := range map[string]func() error{
					"private": func() error { return knowledgecenter.ValidateSigningKey(key) },
					"pair":    func() error { return knowledgecenter.ValidateKeyPair(key, c.ContextPublicKey) },
				} {
					err := check()
					if size == 32768 {
						if err != nil {
							t.Errorf("%s rejected valid 32768-byte signing PEM", name)
						}
					} else if err == nil || !strings.Contains(err.Error(), "must be at most 32768 bytes") {
						t.Errorf("%s did not reject original bytes above signing file limit", name)
					}
					if err != nil && strings.Contains(err.Error(), strings.TrimSpace(c.ContextSigningKey)) {
						t.Error("validator error leaked signing PEM")
					}
				}
			})
		}
	}
	// The byte ceiling precedes parsing, and is specific to private files.
	if err := knowledgecenter.ValidateSigningKey(strings.Repeat("x", 32769)); err == nil || !strings.Contains(err.Error(), "must be at most 32768 bytes") {
		t.Error("oversized private input reached parsing instead of the original-byte guard")
	}
	if err := knowledgecenter.ValidatePublicKey(c.ContextPublicKey + strings.Repeat(" ", 32769)); err != nil {
		t.Error("public key incorrectly restricted by private-file ceiling")
	}
}
