package cloudprovider

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"

	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// GCPProvider deploys GKE-specific resources for the shaide stack.
type GCPProvider struct{}

// ProvisionStorage is a no-op for GCP — GKE uses the pd.csi.storage.gke.io
// dynamic provisioner; PersistentVolumes are created automatically.
func (p *GCPProvider) ProvisionStorage(_ *pulumi.Context, _ *runtime.DeploymentContext, _ appconfig.Config) ([]pulumi.Resource, error) {
	return nil, nil
}

// PostDeployService creates a HealthCheckPolicy so GKE configures the
// load balancer health check against the shaide-server /v1/health endpoint.
func (p *GCPProvider) PostDeployService(ctx *pulumi.Context, deps *runtime.DeploymentContext, namespace string, svc pulumi.Resource) error {
	_, err := apiext.NewCustomResource(ctx, "shaide-healthcheck", &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("networking.gke.io/v1"),
		Kind:       pulumi.String("HealthCheckPolicy"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-hc"),
			Namespace: pulumi.String(namespace),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]interface{}{
				"default": map[string]interface{}{
					"checkIntervalSec":   10,
					"timeoutSec":         5,
					"healthyThreshold":   1,
					"unhealthyThreshold": 3,
					"logConfig": map[string]interface{}{
						"enabled": true,
					},
					"config": map[string]interface{}{
						"type": "HTTP",
						"httpHealthCheck": map[string]interface{}{
							"portSpecification": "USE_SERVING_PORT",
							"requestPath":       "/v1/health",
						},
					},
				},
				"targetRef": map[string]interface{}{
					"group": "",
					"kind":  "Service",
					"name":  "shaide-server",
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt, pulumi.DependsOn([]pulumi.Resource{svc}))
	return err
}
