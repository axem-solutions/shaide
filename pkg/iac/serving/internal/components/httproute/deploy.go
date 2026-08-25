package httproute

import (
	"fmt"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Binds the Istio Gateway listener and targets the GAIE InferencePool.
func Deploy(ctx *pulumi.Context, dep pulumi.Resource, model appConfig.Model, opts ...pulumi.ResourceOption) error {
	routeName := model.HTTPRouteName()
	gaieReleaseName := model.GaieReleaseName()

	resourceName := fmt.Sprintf("llm-d-inference-scheduling-%s", model.Slug)
	_, err := apiext.NewCustomResource(ctx, resourceName, &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("HTTPRoute"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(routeName),
			Namespace: pulumi.String(model.Namespace),
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]interface{}{
				"parentRefs": []map[string]interface{}{
					{
						"group": "gateway.networking.k8s.io",
						"kind":  "Gateway",
						// Gateway name includes nodegroup for multi-stack isolation: "{releaseName}-{nodegroup}-inference-gateway"
						"name": model.GatewayName(),
					},
				},
				"rules": []map[string]interface{}{
					{
						"backendRefs": []map[string]interface{}{
							{
								"group": "inference.networking.k8s.io",
								"kind":  "InferencePool",
								// Use the GAIE chart release name as the InferencePool name (gaie-sim)
								"name":   gaieReleaseName,
								"port":   8000,
								"weight": 1,
							},
						},
						// Upstream sets timeouts to 0s; keep them for parity (Istio ignores unsupported fields)
						"timeouts": map[string]interface{}{
							"backendRequest": "0s",
							"request":        "0s",
						},
						"matches": []map[string]interface{}{
							{
								"path": map[string]interface{}{
									"type":  "PathPrefix",
									"value": "/",
								},
							},
						},
					},
				},
			},
		},
	}, append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{dep})}, opts...)...)
	return err
}
