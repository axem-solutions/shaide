package cloudprovider

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// GenericProvider is the fallback for any unrecognised cloud.
// No cloud-specific resources are created — standard Kubernetes primitives suffice.
type GenericProvider struct{}

func (p *GenericProvider) ProvisionStorage(_ *pulumi.Context, _ *runtime.DeploymentContext, _ appconfig.Config) ([]pulumi.Resource, error) {
	return nil, nil
}

func (p *GenericProvider) PostDeployService(_ *pulumi.Context, _ *runtime.DeploymentContext, _ string, _ pulumi.Resource) error {
	return nil
}
