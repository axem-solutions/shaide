// webapp: end-user facing web application.
// Deployment (no persistent storage) and ClusterIP Service.
// Reachable only from within the cluster.
package webapp

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Deploy creates the webapp Deployment and ClusterIP Service.
func Deploy(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Config) error {
	name := cfg.Services.WebApp
	image := cfg.Images.WebApp

	_, err := appsv1.NewDeployment(ctx, name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels(name, "webapp"),
			Annotations: pulumi.StringMap{
				"pulumi.com/patchForce": pulumi.String("true"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Strategy: &appsv1.DeploymentStrategyArgs{
				Type: pulumi.String("Recreate"),
			},
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: deps.Labels(name),
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: deps.MetaLabels(name, "webapp"),
				},
				Spec: &corev1.PodSpecArgs{
					NodeSelector: cfg.NodeSelectorFor(cfg.NodeSelectorWebApp),
					ImagePullSecrets: corev1.LocalObjectReferenceArray{
						&corev1.LocalObjectReferenceArgs{
							Name: pulumi.String(deps.RegistrySecretName),
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:            pulumi.String(name),
							Image:           pulumi.String(image),
							ImagePullPolicy: pulumi.String("Always"),
							Env: corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name:  pulumi.String("SHAIDE_SERVER_FQDN"),
									Value: pulumi.String("shaide-server"),
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("SHAIDE_SERVER_PORT"),
									Value: pulumi.String("80"),
								},
							},
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(8787),
									Name:          pulumi.String("http"),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("128Mi"),
								},
							},
						},
					},
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt, deps.RegistryDeps, pulumi.DeleteBeforeReplace(true))
	if err != nil {
		return err
	}

	// ClusterIP — reachable only from within the cluster
	_, err = corev1.NewService(ctx, "webapp-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels(name, "webapp"),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: deps.Labels(name),
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(8787),
					TargetPort: pulumi.Int(8787),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt)
	if err != nil {
		return err
	}

	return nil
}
