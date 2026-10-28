package knowledgecenter

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	kc "github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
)

const (
	// EmbeddingUsername is the ordinary shaide user KC signs in as to request
	// embeddings.
	EmbeddingUsername = "knowledge-center"

	tokenBytes = 32
	keyBits    = 3072
)

// CompleteCredentials returns retained with every missing credential
// generated. Retained values are kept as they are, so an update does not
// rotate what the running deployments use. The key pair is replaced as a
// unit when either half is missing or they do not match.
func CompleteCredentials(retained kc.Credentials) (kc.Credentials, error) {
	creds := retained

	for _, field := range []*string{
		&creds.InternalToken,
		&creds.WebhookToken,
		&creds.GatewayToken,
		&creds.ScopedToken,
		&creds.EmbeddingPassword,
	} {
		if *field != "" {
			continue
		}
		token, err := randomToken()
		if err != nil {
			return kc.Credentials{}, err
		}
		*field = token
	}

	if creds.EmbeddingUsername == "" {
		creds.EmbeddingUsername = EmbeddingUsername
	}

	if kc.ValidateKeyPair(creds.ContextSigningKey, creds.ContextPublicKey) != nil {
		private, public, err := keyPair()
		if err != nil {
			return kc.Credentials{}, err
		}
		creds.ContextSigningKey, creds.ContextPublicKey = private, public
	}

	if err := validate(creds); err != nil {
		return kc.Credentials{}, err
	}
	return creds, nil
}

// randomToken is unpadded base64url of 32 random bytes, which satisfies both
// the service and the hop token formats.
func randomToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate Knowledge Center token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func keyPair() (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return "", "", fmt.Errorf("generate Knowledge Center context key: %w", err)
	}

	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("encode Knowledge Center context key: %w", err)
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("encode Knowledge Center context public key: %w", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})),
		nil
}

// validate applies the stacks' rules, so a retained value an operator edited
// into an invalid one fails here, naming the field, rather than in a stack.
func validate(c kc.Credentials) error {
	for _, check := range []struct {
		field    string
		value    string
		validate func(string) error
	}{
		{kc.FieldInternalToken, c.InternalToken, kc.ValidateServiceToken},
		{kc.FieldWebhookToken, c.WebhookToken, kc.ValidateServiceToken},
		{kc.FieldGatewayToken, c.GatewayToken, kc.ValidateGatewayToken},
		{kc.FieldScopedToken, c.ScopedToken, kc.ValidateHopToken},
		{kc.FieldEmbeddingUsername, c.EmbeddingUsername, kc.ValidateUsername},
		{kc.FieldEmbeddingPassword, c.EmbeddingPassword, kc.ValidatePassword},
	} {
		if err := check.validate(check.value); err != nil {
			return fmt.Errorf("Knowledge Center %s: %w", check.field, err)
		}
	}

	return kc.ValidateDistinct(map[string]string{
		kc.FieldInternalToken:     c.InternalToken,
		kc.FieldWebhookToken:      c.WebhookToken,
		kc.FieldGatewayToken:      c.GatewayToken,
		kc.FieldScopedToken:       c.ScopedToken,
		kc.FieldEmbeddingPassword: c.EmbeddingPassword,
	})
}
