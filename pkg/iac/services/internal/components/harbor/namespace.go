package harbor

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func createNamespace(ctx *pulumi.Context, provider pulumi.ProviderResource) (*corev1.Namespace, error) {
	return corev1.NewNamespace(ctx, "harbor-namespace", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String("harbor"),
		},
	}, pulumi.Provider(provider))
}
