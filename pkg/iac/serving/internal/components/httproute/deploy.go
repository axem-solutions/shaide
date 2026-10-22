package httproute

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/router"
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Deploy binds the model's Gateway listener to its InferencePool, so the
// Gateway asks the router's endpoint picker where to send each request.
func Deploy(ctx *pulumi.Context, dependsOn []pulumi.Resource, model appConfig.Model, opts ...pulumi.ResourceOption) error {
	// The resource name predates the llm-d v1.0 move and is kept, so
	// existing stacks update the route in place.
	resourceName := fmt.Sprintf("llm-d-inference-scheduling-%s", model.Slug)

	_, err := apiext.NewCustomResource(ctx, resourceName, &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("HTTPRoute"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(model.HTTPRouteName()),
			Namespace: pulumi.String(model.Namespace),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]any{
				"parentRefs": []map[string]any{
					{
						"group": "gateway.networking.k8s.io",
						"kind":  "Gateway",
						"name":  model.GatewayName(),
					},
				},
				"rules": []map[string]any{
					{
						"backendRefs": []map[string]any{
							{
								"group":  "inference.networking.k8s.io",
								"kind":   "InferencePool",
								"name":   model.InferencePoolName(),
								"port":   router.ModelServerPort,
								"weight": 1,
							},
						},
						// No route timeout: streamed responses of long
						// generations run for many minutes.
						"timeouts": map[string]any{
							"backendRequest": "0s",
							"request":        "0s",
						},
						"matches": []map[string]any{
							{
								"path": map[string]any{
									"type":  "PathPrefix",
									"value": "/",
								},
							},
						},
					},
				},
			},
		},
	}, append([]pulumi.ResourceOption{pulumi.DependsOn(dependsOn)}, opts...)...)
	return err
}
