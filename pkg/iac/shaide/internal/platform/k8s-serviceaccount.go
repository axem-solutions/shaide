package platform

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateShaideServiceAccount creates a dedicated ServiceAccount for shaide-server.
func CreateShaideServiceAccount(
	ctx *pulumi.Context,
	cfg appconfig.Config,
	providerOpt, nsOpt pulumi.ResourceOption,
) (*corev1.ServiceAccount, error) {
	var annotations pulumi.StringMap
	if len(cfg.ServiceAccountAnnotations) > 0 {
		annotations = make(pulumi.StringMap, len(cfg.ServiceAccountAnnotations))
		for k, v := range cfg.ServiceAccountAnnotations {
			annotations[k] = pulumi.String(v)
		}
	}

	return corev1.NewServiceAccount(ctx, "shaide-server-sa", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        pulumi.String(cfg.ServiceAccountName),
			Namespace:   pulumi.String(cfg.Namespace),
			Annotations: annotations,
		},
	}, providerOpt, nsOpt)
}
