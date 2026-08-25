package cloudprovider

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// AzureProvider deploys Azure/AKS-specific resources for the shaide stack.
// Pod-level Workload Identity (label + ServiceAccount annotation) is wired directly
// in components/shaide/deploy.go and platform.CreateShaideServiceAccount, since it
// applies regardless of which cloud-specific resources this provider creates.
type AzureProvider struct{}

// ProvisionStorage is a no-op for Azure — AKS uses the disk.csi.azure.com dynamic
// provisioner; PersistentVolumes are created automatically.
func (p *AzureProvider) ProvisionStorage(_ *pulumi.Context, _ *runtime.DeploymentContext, _ appconfig.Config) ([]pulumi.Resource, error) {
	return nil, nil
}

// PostDeployService is a no-op for Azure. Both current Azure stacks set infraStackRef,
// so the shaide-server Service is ClusterIP + HTTPRoute, never a standalone Azure Load
// Balancer — an LB health-probe annotation would be unused. Revisit if a non-gateway
// Azure stack config is introduced.
func (p *AzureProvider) PostDeployService(_ *pulumi.Context, _ *runtime.DeploymentContext, _ string, _ pulumi.Resource) error {
	return nil
}
