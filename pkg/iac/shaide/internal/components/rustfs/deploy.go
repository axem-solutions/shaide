// rustfs: S3-compatible object storage.
// StatefulSet with PVC and ClusterIP Service (port 9000 plus optional 9001 console).
// Credentials sourced from shaide-config ConfigMap and shaide-secrets Secret.
package rustfs

import (
	"fmt"

	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// rustfsUID is the UID/GID the rustfs container runs as.
// Update this if the upstream image changes its user ID.
const rustfsUID = 10001

// deployRustfs creates the rustfs StatefulSet and ClusterIP Service.
// Provides S3-compatible object storage for shaide-server.
func Deploy(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Config) error {
	name := cfg.Services.Rustfs
	image := cfg.Images.Rustfs
	enableConsole := cfg.RustEnv.ConsoleEnabled

	args := pulumi.StringArray{pulumi.String("/data")}
	ports := corev1.ContainerPortArray{
		&corev1.ContainerPortArgs{
			ContainerPort: pulumi.Int(9000),
			Name:          pulumi.String("s3"),
		},
	}
	env := corev1.EnvVarArray{
		&corev1.EnvVarArgs{
			// rustfs 1.0.0-alpha.82 reads ACCESS_KEY/SECRET_KEY.
			// Keep ROOT_* too for compatibility across versions.
			Name: pulumi.String("RUSTFS_ACCESS_KEY"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelectorArgs{
					Name: pulumi.String("shaide-config"),
					Key:  pulumi.String("S3_USER"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("RUSTFS_SECRET_KEY"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pulumi.String("shaide-secrets"),
					Key:  pulumi.String("S3_PASSWORD"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("RUSTFS_ROOT_USER"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelectorArgs{
					Name: pulumi.String("shaide-config"),
					Key:  pulumi.String("S3_USER"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name: pulumi.String("RUSTFS_ROOT_PASSWORD"),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pulumi.String("shaide-secrets"),
					Key:  pulumi.String("S3_PASSWORD"),
				},
			},
		},
		&corev1.EnvVarArgs{
			Name:  pulumi.String("RUSTFS_NOTIFY_WEBHOOK_ENABLE_SHAIDE"),
			Value: pulumi.String(cfg.RustEnv.WebhookEnableShaide),
		},
		&corev1.EnvVarArgs{
			Name:  pulumi.String("RUSTFS_NOTIFY_WEBHOOK_ENDPOINT_SHAIDE"),
			Value: pulumi.String(cfg.RustEnv.WebhookEndpointShaide),
		},
		&corev1.EnvVarArgs{
			Name:  pulumi.String("RUSTFS_NOTIFY_WEBHOOK_QUEUE_DIR_SHAIDE"),
			Value: pulumi.String(cfg.RustEnv.WebhookQueueDirShaide),
		},
	}
	servicePorts := corev1.ServicePortArray{
		&corev1.ServicePortArgs{
			Name:       pulumi.String("s3"),
			Port:       pulumi.Int(9000),
			TargetPort: pulumi.Int(9000),
			Protocol:   pulumi.String("TCP"),
		},
	}

	if enableConsole {
		args = pulumi.StringArray{pulumi.String("--console-enable"), pulumi.String("/data")}
		ports = append(ports, &corev1.ContainerPortArgs{
			ContainerPort: pulumi.Int(9001),
			Name:          pulumi.String("console"),
		})
		env = append(corev1.EnvVarArray{
			&corev1.EnvVarArgs{
				Name:  pulumi.String("RUSTFS_CONSOLE_ENABLE"),
				Value: pulumi.String("true"),
			},
		}, env...)
		servicePorts = append(servicePorts, &corev1.ServicePortArgs{
			Name:       pulumi.String("console"),
			Port:       pulumi.Int(9001),
			TargetPort: pulumi.Int(9001),
			Protocol:   pulumi.String("TCP"),
		})
	}

	rustfsPVCSpec := &corev1.PersistentVolumeClaimSpecArgs{
		AccessModes: pulumi.StringArray{pulumi.String("ReadWriteOnce")},
		Resources: &corev1.VolumeResourceRequirementsArgs{
			Requests: pulumi.StringMap{"storage": pulumi.String(cfg.RustfsPVSize)},
		},
	}
	if cfg.StorageClassName != "" {
		rustfsPVCSpec.StorageClassName = pulumi.StringPtr(cfg.StorageClassName)
	}

	_, err := appsv1.NewStatefulSet(ctx, name, &appsv1.StatefulSetArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels(name, "object-storage"),
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
					Labels: deps.MetaLabels(name, "object-storage"),
				},
				Spec: &corev1.PodSpecArgs{
					NodeSelector: cfg.NodeSelectorFor(cfg.NodeSelectorRustfs),
					SecurityContext: &corev1.PodSecurityContextArgs{
						RunAsUser:  pulumi.Int(rustfsUID),
						RunAsGroup: pulumi.Int(rustfsUID),
						FsGroup:    pulumi.Int(rustfsUID),
					},
					// rustfs runs as UID/GID 10001 and expects /data and /logs
					// to have mode 0755. PVC mounts default to root ownership and
					// emptyDir mounts default to 0777. This init container runs as
					// root to set the correct ownership and permissions before the
					// main container starts.
					InitContainers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String("fix-permissions"),
							Image:   pulumi.String(cfg.Images.Busybox),
							Command: pulumi.StringArray{pulumi.String("sh"), pulumi.String("-c"), pulumi.String(fmt.Sprintf("chmod 755 /logs && chown %d:%d /logs && chmod 755 /data && chown %d:%d /data", rustfsUID, rustfsUID, rustfsUID, rustfsUID))},
							SecurityContext: &corev1.SecurityContextArgs{
								RunAsUser: pulumi.Int(0), // required to chown/chmod
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String(name + "-data"),
									MountPath: pulumi.String("/data"),
								},
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("logs"),
									MountPath: pulumi.String("/logs"),
								},
							},
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:            pulumi.String(name),
							Image:           pulumi.String(image),
							ImagePullPolicy: pulumi.String("Always"),
							Args:            args,
							Ports:           ports,
							Env:             env,
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("256Mi"),
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String(name + "-data"),
									MountPath: pulumi.String("/data"),
								},
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("logs"),
									MountPath: pulumi.String("/logs"),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("logs"),
							EmptyDir: &corev1.EmptyDirVolumeSourceArgs{
								Medium: pulumi.String(""),
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
					Spec: rustfsPVCSpec,
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt, deps.ConfigDeps, deps.StorageDeps)
	if err != nil {
		return err
	}

	// ClusterIP — S3 API on port 9000, plus optional console on 9001, internal only
	_, err = corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels(name, "object-storage"),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: deps.Labels(name),
			Ports:    servicePorts,
		},
	}, deps.ProviderOpt, deps.NsOpt)
	if err != nil {
		return err
	}

	return nil
}
