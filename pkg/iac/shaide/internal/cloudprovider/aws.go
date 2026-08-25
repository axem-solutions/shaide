package cloudprovider

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// AWSProvider deploys AWS-specific resources for the shaide stack.
// LB behaviour is fully driven by lbAnnotations in stack config.
// EBS/EFS dynamic provisioning handles storage — no static PVs needed.
type AWSProvider struct{}

func (p *AWSProvider) ProvisionStorage(_ *pulumi.Context, _ *runtime.DeploymentContext, _ appconfig.Config) ([]pulumi.Resource, error) {
	return nil, nil
}

func (p *AWSProvider) PostDeployService(_ *pulumi.Context, _ *runtime.DeploymentContext, _ string, _ pulumi.Resource) error {
	return nil
}
