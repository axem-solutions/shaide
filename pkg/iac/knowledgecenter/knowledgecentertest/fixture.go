// Package knowledgecentertest provides synthetic Knowledge Center credentials
// for tests. Deployment code must never import it: the installer supplies the
// real values.
package knowledgecentertest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"sync"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
)

var (
	once       sync.Once
	privatePEM string
	publicPEM  string
)

func keyPair() (string, string) {
	once.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		private, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			panic(err)
		}
		public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			panic(err)
		}
		privatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
		publicPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))
	})
	return privatePEM, publicPEM
}

// HopToken returns a valid unpadded base64url token built from fill.
func HopToken(fill byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = fill
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Credentials returns a complete, valid and mutually distinct set.
func Credentials() knowledgecenter.Credentials {
	private, public := keyPair()
	return knowledgecenter.Credentials{
		InternalToken:     "synthetic-internal-token-0123456789abcdef",
		WebhookToken:      "synthetic-webhook-token-0123456789abcdefg",
		GatewayToken:      HopToken('g'),
		ScopedToken:       HopToken('s'),
		ContextSigningKey: private,
		ContextPublicKey:  public,
		EmbeddingUsername: "kc-embedding",
		EmbeddingPassword: "synthetic-embedding-password",
	}
}

// OtherPublicKey returns a valid public key that does not match Credentials.
func OtherPublicKey() string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))
}
