package kubernetes

import (
	pulumikubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
)

type ProviderOptions struct {
	Name                  string
	EnableServerSideApply *bool
}

func NewProvider(
	ctx *pulumi.Context,
	connection connection.Connection,
	options ProviderOptions,
	opts ...pulumi.ResourceOption,
) (*pulumikubernetes.Provider, error) {
	args := &pulumikubernetes.ProviderArgs{}

	if connection.KubeconfigPath != "" {
		args.Kubeconfig = pulumi.StringPtr(connection.KubeconfigPath)
	}

	if connection.Context != "" {
		args.Context = pulumi.StringPtr(connection.Context)
	}

	if options.EnableServerSideApply != nil {
		args.EnableServerSideApply = pulumi.BoolPtr(
			*options.EnableServerSideApply,
		)
	}

	name := options.Name
	if name == "" {
		name = "k8s"
	}

	return pulumikubernetes.NewProvider(
		ctx,
		name,
		args,
		opts...,
	)
}
