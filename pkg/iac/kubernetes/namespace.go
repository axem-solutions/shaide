package kubernetes

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type NamespaceOptions struct {
	Labels pulumi.StringMap
}

// CreateNamespace creates a namespace with Kubernetes metadata and Pulumi
// resource options.
func CreateNamespace(
	ctx *pulumi.Context,
	namespace string,
	options NamespaceOptions,
	opts ...pulumi.ResourceOption,
) (*corev1.Namespace, error) {
	return corev1.NewNamespace(
		ctx,
		namespace,
		&corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:   pulumi.String(namespace),
				Labels: options.Labels,
			},
		},
		opts...,
	)
}
