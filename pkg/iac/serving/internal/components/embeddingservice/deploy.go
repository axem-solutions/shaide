package embeddingservice

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/modelserver"
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ServicePort is where clients reach an embedder. It predates the llm-d v1.0
// move and stays, so clients built against it keep working; the Service
// forwards to the model server's own port.
const ServicePort = 8200

// Deploy creates a ClusterIP Service that selects the model server pods
// directly.
func Deploy(
	ctx *pulumi.Context,
	dependsOn []pulumi.Resource,
	model appConfig.Model,
	category string,
	opts ...pulumi.ResourceOption,
) error {
	resourceName := "embedding-svc-" + model.Slug
	_, err := corev1.NewService(ctx, resourceName, &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(model.EmbeddingServiceName()),
			Namespace: pulumi.String(model.Namespace),
			// Labeled (not just the Namespace) so a client with cluster-wide
			// Service read access, e.g. shaide-server, can discover this
			// model's endpoint directly via label selector instead of
			// reconstructing the naming convention.
			Labels: model.MetaLabels(category),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: pulumi.ToStringMap(modelserver.PodSelector(model)),
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(ServicePort),
					TargetPort: pulumi.Int(modelserver.Port),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, append([]pulumi.ResourceOption{pulumi.DependsOn(dependsOn)}, opts...)...)
	return err
}
