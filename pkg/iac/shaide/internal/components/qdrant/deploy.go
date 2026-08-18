// qdrant: vector database for RAG.
// StatefulSet with PVC and ClusterIP Service (REST 6333, gRPC 6334).
package qdrant

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// deployQdrant creates the qdrant StatefulSet and ClusterIP Service.
// Provides vector database for RAG capabilities.
func Deploy(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Config) error {
	name := cfg.Services.Qdrant
	image := cfg.Images.Qdrant

	qdrantPVCSpec := &corev1.PersistentVolumeClaimSpecArgs{
		AccessModes: pulumi.StringArray{pulumi.String("ReadWriteOnce")},
		Resources: &corev1.VolumeResourceRequirementsArgs{
			Requests: pulumi.StringMap{"storage": pulumi.String(cfg.QdrantPVSize)},
		},
	}
	if cfg.StorageClassName != "" {
		qdrantPVCSpec.StorageClassName = pulumi.StringPtr(cfg.StorageClassName)
	}

	_, err := appsv1.NewStatefulSet(ctx, name, &appsv1.StatefulSetArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels(name, "vector-database"),
			Annotations: pulumi.StringMap{
				"pulumi.com/patchForce": pulumi.String("true"),
			},
		},
		Spec: &appsv1.StatefulSetSpecArgs{
			Replicas:    pulumi.Int(1),
			ServiceName: pulumi.String(name),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: deps.Labels(name),
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: deps.MetaLabels(name, "vector-database"),
				},
				Spec: &corev1.PodSpecArgs{
					Affinity: &corev1.AffinityArgs{
						NodeAffinity:    cfg.NodeAffinityFor(cfg.NodeSelectorQdrant),
						PodAntiAffinity: deps.PodAntiAffinityFor(name),
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:            pulumi.String(name),
							Image:           pulumi.String(image),
							ImagePullPolicy: pulumi.String("Always"),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(6333),
									Name:          pulumi.String("rest"),
								},
								&corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(6334),
									Name:          pulumi.String("http"),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("256Mi"),
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String(name + "-data"),
									MountPath: pulumi.String("/qdrant/storage"),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: corev1.PersistentVolumeClaimTypeArray{
				&corev1.PersistentVolumeClaimTypeArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Name: pulumi.String(name + "-data"),
					},
					Spec: qdrantPVCSpec,
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt, deps.StorageDeps)
	if err != nil {
		return err
	}

	// ClusterIP — REST on 6333, gRPC on 6334, internal only
	_, err = corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels(name, "vector-database"),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: deps.Labels(name),
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("rest"),
					Port:       pulumi.Int(6333),
					TargetPort: pulumi.Int(6333),
					Protocol:   pulumi.String("TCP"),
				},
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(6334),
					TargetPort: pulumi.Int(6334),
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
