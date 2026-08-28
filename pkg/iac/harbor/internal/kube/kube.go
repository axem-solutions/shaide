package kube

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewProvider(ctx *pulumi.Context, kubeconfigPath string, kubeContext string) (*kubernetes.Provider, error) {
	args := &kubernetes.ProviderArgs{}

	if kubeconfigPath != "" {
		args.Kubeconfig = pulumi.StringPtr(kubeconfigPath)
	}

	if kubeContext != "" {
		args.Context = pulumi.StringPtr(kubeContext)
	}

	return kubernetes.NewProvider(ctx, "k8s", args)
}
func NewNamespace(ctx *pulumi.Context, provider *kubernetes.Provider, name string) (*corev1.Namespace, error) {
	return corev1.NewNamespace(
		ctx,
		"harbor-namespace",
		&corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(name),
			},
		},
		pulumi.Provider(provider),
	)
}
