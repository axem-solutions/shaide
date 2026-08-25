package harbor

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// deployNodeTrust makes every node's containerd trust and route to Harbor
// over plain HTTP, by writing a registry mirror config (hosts.toml) to each
// node's disk via a privileged DaemonSet. Requires a pinned harborIP — see
// buildHarborValues's staticClusterIP wiring, which is what makes this
// callable without reading the Service back out of the cluster.
func deployNodeTrust(
	ctx *pulumi.Context,
	k8sProvider *kubernetes.Provider,
	harborIP string,
	harborNamespaceName string,
	release *helmv3.Release,
) error {
	providerOpt := pulumi.Provider(k8sProvider)

	ns, err := corev1.NewNamespace(ctx, "node-registry-config-namespace", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(nodeTrustNamespace),
			Labels: pulumi.StringMap{
				"pod-security.kubernetes.io/enforce": pulumi.String("privileged"),
			},
		},
	}, providerOpt)
	if err != nil {
		return fmt.Errorf("node-registry-config namespace: %w", err)
	}

	harborHost := fmt.Sprintf("harbor.%s.svc.cluster.local", harborNamespaceName)
	certsDir := fmt.Sprintf("/host/certs.d/%s", harborHost)

	// Self-healing loop: rewrites hosts.toml every 300s instead of once, so a
	// node whose file gets deleted/corrupted self-corrects without needing its
	// pod rescheduled. See infra/cloud-harbor node-trust docs for the tradeoff
	// against a write-once-then-idle variant.
	script := fmt.Sprintf(nodeTrustScriptTemplate, certsDir, harborIP)

	labels := pulumi.StringMap{"app": pulumi.String(nodeTrustNamespace)}

	_, err = appsv1.NewDaemonSet(ctx, "node-registry-config", &appsv1.DaemonSetArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(nodeTrustNamespace),
			Namespace: pulumi.String(nodeTrustNamespace),
		},
		Spec: &appsv1.DaemonSetSpecArgs{
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: labels,
				},
				Spec: &corev1.PodSpecArgs{
					// Tolerate every taint so this schedules on every node,
					// including GPU/tainted nodes — see docs for why.
					Tolerations: corev1.TolerationArray{
						&corev1.TolerationArgs{
							Operator: pulumi.String("Exists"),
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("config-writer"),
							Image: pulumi.String("busybox:1.36"),
							Command: pulumi.StringArray{
								pulumi.String("sh"),
								pulumi.String("-c"),
								pulumi.String(script),
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("certs-d"),
									MountPath: pulumi.String("/host/certs.d"),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("certs-d"),
							HostPath: &corev1.HostPathVolumeSourceArgs{
								Path: pulumi.String("/etc/containerd/certs.d"),
								Type: pulumi.String("DirectoryOrCreate"),
							},
						},
					},
				},
			},
		},
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{ns, release}))
	if err != nil {
		return fmt.Errorf("node-registry-config daemonset: %w", err)
	}

	return nil
}
