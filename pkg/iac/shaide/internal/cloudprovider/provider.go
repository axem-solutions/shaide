// Package cloudprovider defines the Provider interface for cloud-specific
// Kubernetes resources deployed alongside the shaide stack.
//
// Each cloud implements the interface; deploy.go calls it without knowing
// which cloud is active. New clouds add a file here — deploy.go never changes.
package cloudprovider

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Provider deploys cloud-specific resources that cannot be expressed
// in a cloud-agnostic way (e.g. GKE HealthCheckPolicy, hostPath PVs).
type Provider interface {
	// ProvisionStorage is called before any StatefulSet is created.
	// Implementations create cloud-specific storage resources (e.g. static
	// PersistentVolumes for on-prem hostPath). The returned resources are
	// added to StorageDeps so StatefulSets wait for them.
	// Return nil slice for clouds with dynamic provisioning (no-op).
	ProvisionStorage(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Config) ([]pulumi.Resource, error)

	// PostDeployService is called after the shaide-server Service is created.
	// Implementations may create cloud-native resources that target the Service.
	// Pass a no-op implementation for clouds that need nothing extra.
	PostDeployService(ctx *pulumi.Context, deps *runtime.DeploymentContext, namespace string, svc pulumi.Resource) error
}

// New returns the Provider for the given cloudProvider string.
// Unrecognised values fall back to GenericProvider (no-op).
func New(cloudProvider string) Provider {
	switch cloudProvider {
	case "gcp":
		return &GCPProvider{}
	case "aws":
		return &AWSProvider{}
	case "azure":
		return &AzureProvider{}
	case "on-prem":
		return &OnPremProvider{}
	default:
		return &GenericProvider{}
	}
}
