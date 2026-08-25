// MCP runtime secrets.
package mcpsecret

import (
	appconfig "app_mcp/internal/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const Name = "mcp-secrets"

func Deploy(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) (*corev1.Secret, error) {
	secretData := pulumi.StringMap{}
	if cfg.Secrets.HasAtlassianOAuthClientSecret {
		secretData["ATLASSIAN_OAUTH_CLIENT_SECRET"] = cfg.Secrets.AtlassianOAuthClientSecret
	}
	if len(secretData) == 0 {
		return nil, nil
	}

	return corev1.NewSecret(ctx, Name, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(Name),
			Namespace: pulumi.String(cfg.Namespace),
		},
		StringData: secretData,
	}, opts...)
}
