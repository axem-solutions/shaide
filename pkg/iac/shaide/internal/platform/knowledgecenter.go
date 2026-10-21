package platform

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Knowledge Center Secrets in app-shaide. Each workload gets only what it
// uses:
//
//	knowledge-center          env for KC: the internal token and the
//	                          embedding account password
//	knowledge-center-context  files for KC: the context public key and the
//	                          KC-MCP scoped token
//	kc-context-signer         files for shaide-server: the context signing
//	                          key and the KC-MCP gateway token
//
// The signing key exists only in kc-context-signer.
const (
	KCEnvSecretName         = "knowledge-center"
	KCContextSecretName     = "knowledge-center-context"
	ServerContextSecretName = "kc-context-signer"

	KCEnvInternalToken     = "KNOWLEDGE_CENTER_INTERNAL_TOKEN"
	KCEnvEmbeddingPassword = "KC_SHAIDE_PASSWORD"
)

// CreateKnowledgeCenterSecrets creates the KC Secrets. It creates nothing
// when the Knowledge Center is not installed.
func CreateKnowledgeCenterSecrets(ctx *pulumi.Context, cfg appconfig.Values, providerOpt, nsOpt pulumi.ResourceOption) ([]pulumi.Resource, error) {
	if !cfg.KnowledgeCenterEnabled {
		return nil, nil
	}

	var created []pulumi.Resource
	for _, secret := range []struct {
		name string
		data pulumi.StringMap
	}{
		{KCEnvSecretName, pulumi.StringMap{
			KCEnvInternalToken:     cfg.Secrets.KCInternalToken,
			KCEnvEmbeddingPassword: cfg.Secrets.KCEmbeddingPassword,
		}},
		{KCContextSecretName, pulumi.StringMap{
			knowledgecenter.PublicKeyFile:   cfg.Secrets.KCContextPublicKey,
			knowledgecenter.ScopedTokenFile: cfg.Secrets.KCScopedToken,
		}},
		{ServerContextSecretName, pulumi.StringMap{
			knowledgecenter.SigningKeyFile:   cfg.Secrets.KCContextSigningKey,
			knowledgecenter.GatewayTokenFile: cfg.Secrets.KCGatewayToken,
		}},
	} {
		resource, err := corev1.NewSecret(ctx, secret.name, &corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(secret.name),
				Namespace: pulumi.String(cfg.Namespace),
			},
			StringData: secret.data,
		}, providerOpt, nsOpt)
		if err != nil {
			return nil, err
		}
		created = append(created, resource)
	}

	return created, nil
}
