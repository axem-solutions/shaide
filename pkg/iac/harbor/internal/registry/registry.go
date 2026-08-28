package registry

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func EnableHTTPSFastFail(
	ctx *pulumi.Context,
	provider *kubernetes.Provider,
	namespace string,
	serviceName string,
	release *helmv3.Release,
) error {
	_, err := corev1.NewServicePatch(
		ctx,
		"harbor-https-fast-fail",
		&corev1.ServicePatchArgs{
			Metadata: &metav1.ObjectMetaPatchArgs{
				Name: pulumi.String(serviceName),
				Namespace: pulumi.String(
					namespace,
				),
			},

			Spec: &corev1.ServiceSpecPatchArgs{
				Ports: corev1.ServicePortPatchArray{
					&corev1.ServicePortPatchArgs{
						Name: pulumi.String(
							"https-passthrough",
						),
						Port: pulumi.Int(443),
						TargetPort: pulumi.Int(
							8080,
						),
						Protocol: pulumi.String(
							"TCP",
						),
					},
				},
			},
		},
		pulumi.Provider(provider),
		pulumi.DependsOn([]pulumi.Resource{
			release,
		}),
	)

	return err
}
