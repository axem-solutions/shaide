package kubernetes

import (
	pulumikubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
)

func NewProvider(ctx *pulumi.Context, connection connection.Connection) (*pulumikubernetes.Provider, error) {
	args := &pulumikubernetes.ProviderArgs{}

	if connection.KubeconfigPath != "" {
		args.Kubeconfig = pulumi.StringPtr(connection.KubeconfigPath)
	}

	if connection.Context != "" {
		args.Context = pulumi.StringPtr(connection.Context)
	}

	return pulumikubernetes.NewProvider(ctx, "k8s", args)
}
