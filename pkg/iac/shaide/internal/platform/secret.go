// Registry pull secret.
//
// For GCP/AWS/generic: authenticates against ghcr.io using ghcrUser/ghcrToken.
// For on-prem:         authenticates against the internal Harbor registry
//
//	(harborHostname) using the same ghcrUser/ghcrToken fields
//	— configure them with the Harbor robot account credentials.
//
// Set the token as a secret: pulumi config set --secret ghcrToken <value>
package platform

import (
	"encoding/json"

	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateGHCRSecret creates a kubernetes.io/dockerconfigjson pull secret.
// When cfg.HarborHostname is set the secret authenticates against Harbor
// (http://<harborHostname>); otherwise it authenticates against ghcr.io.
func CreateGHCRSecret(ctx *pulumi.Context, cfg appconfig.Config, providerOpt, nsOpt pulumi.ResourceOption) (*corev1.Secret, error) {
	registry := "ghcr.io"
	if cfg.HarborHostname != "" {
		registry = cfg.HarborHostname
	}

	dockerConfigJSON := cfg.Registry.GHCRToken.ApplyT(func(token string) (string, error) {
		dockerCfg := map[string]any{
			"auths": map[string]any{
				registry: map[string]string{
					"username": cfg.Registry.GHCRUser,
					"password": token,
				},
			},
		}
		b, err := json.Marshal(dockerCfg)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}).(pulumi.StringOutput)

	return corev1.NewSecret(ctx, "ghcr-creds", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("ghcr-creds"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Type: pulumi.String("kubernetes.io/dockerconfigjson"),
		StringData: pulumi.StringMap{
			".dockerconfigjson": dockerConfigJSON,
		},
	}, providerOpt, nsOpt)
}
