package embeddingservice

import (
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Deploy creates a ClusterIP Service that selects the decode pod(s) directly on port 8200.
// This is the only supported in-cluster path for embedding requests; the Gateway/HTTPRoute
// path does not work for embeddings.
func Deploy(ctx *pulumi.Context, dep pulumi.Resource, model appConfig.Model, opts ...pulumi.ResourceOption) error {
	svcName := model.EmbeddingServiceName()
	// Helm fullname for the modelservice release: "<releaseName>-llm-d-modelservice".
	// This matches the llm-d.ai/model label written by the llm-d-modelservice chart onto decode pods.
	modelLabel := model.ModelServiceReleaseName() + "-llm-d-modelservice"

	resourceName := "embedding-svc-" + model.Slug
	_, err := corev1.NewService(ctx, resourceName, &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(svcName),
			Namespace: pulumi.String(model.Namespace),
			// Labeled (not just the Namespace) so a client with cluster-wide
			// Service read access — e.g. shaide-server — can discover this
			// model's endpoint directly via label selector instead of
			// reconstructing the naming convention.
			Labels: model.MetaLabels(),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type: pulumi.String("ClusterIP"),
			Selector: pulumi.StringMap{
				"llm-d.ai/model": pulumi.String(modelLabel),
				"llm-d.ai/role":  pulumi.String("decode"),
			},
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(8200),
					TargetPort: pulumi.Int(8200),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{dep})}, opts...)...)
	return err
}
