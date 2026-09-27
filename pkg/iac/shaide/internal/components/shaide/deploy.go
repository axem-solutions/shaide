// shaide-server: main application hub.
// StatefulSet with PVC (SQLite DB at /root/.config).
// Only externally reachable entrypoint in the app-shaide namespace, and only
// through the shared Gateway: the Service is always ClusterIP, and an HTTPRoute
// attaches it to the Gateway when infraStackRef or gatewayHostname is set.
// Without either, it is reachable only from inside the cluster, e.g. through
// kubectl port-forward during development.
package shaide

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/cloudprovider"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Deploy creates the shaide-server StatefulSet and its ClusterIP Service.
// If infraStackRef or gatewayHostname is set, an HTTPRoute routes traffic from
// the shared Gateway (in gateway-system) to this Service.
// Cloud-specific post-deploy resources are delegated to the provider.
func Deploy(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Values, provider cloudprovider.Provider) error {
	image := cfg.Images.ShaideServer

	podLabels := deps.MetaLabels("shaide-server", "server")
	if cfg.CloudProvider == "azure" {
		// Required for the AKS Workload Identity mutating webhook to project the
		// federated token volume into the pod.
		podLabels["azure.workload.identity/use"] = pulumi.String("true")
	}

	shaidePVCSpec := &corev1.PersistentVolumeClaimSpecArgs{
		AccessModes: pulumi.StringArray{pulumi.String("ReadWriteOnce")},
		Resources: &corev1.VolumeResourceRequirementsArgs{
			Requests: pulumi.StringMap{"storage": pulumi.String(cfg.ShaidePVSize)},
		},
	}
	if cfg.StorageClassName != "" {
		shaidePVCSpec.StorageClassName = pulumi.StringPtr(cfg.StorageClassName)
	}

	_, err := appsv1.NewStatefulSet(ctx, "shaide-server", &appsv1.StatefulSetArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-server"),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels("shaide-server", "server"),
			Annotations: pulumi.StringMap{
				"pulumi.com/patchForce": pulumi.String("true"),
			},
		},
		Spec: &appsv1.StatefulSetSpecArgs{
			Replicas:    pulumi.Int(1),
			ServiceName: pulumi.String("shaide-server"),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: deps.Labels("shaide-server"),
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: podLabels,
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: pulumi.String(cfg.ServiceAccountName),
					Affinity: &corev1.AffinityArgs{
						NodeAffinity:    cfg.NodeAffinityFor(cfg.NodeSelectorShaide),
						PodAntiAffinity: deps.PodAntiAffinityFor("shaide-server"),
					},
					SecurityContext: &corev1.PodSecurityContextArgs{
						FsGroup: pulumi.Int(1000),
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:            pulumi.String("shaide-server"),
							Image:           pulumi.String(image),
							ImagePullPolicy: pulumi.String("Always"),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(8080),
									Name:          pulumi.String("http"),
								},
							},
							EnvFrom: corev1.EnvFromSourceArray{
								&corev1.EnvFromSourceArgs{
									ConfigMapRef: &corev1.ConfigMapEnvSourceArgs{
										Name: pulumi.String("shaide-config"),
									},
								},
								&corev1.EnvFromSourceArgs{
									SecretRef: &corev1.SecretEnvSourceArgs{
										Name: pulumi.String("shaide-secrets"),
									},
								},
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.String("/v1/health"),
									Port: pulumi.Int(8080),
								},
								InitialDelaySeconds: pulumi.Int(5),
								PeriodSeconds:       pulumi.Int(10),
								TimeoutSeconds:      pulumi.Int(2),
								FailureThreshold:    pulumi.Int(3),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("128Mi"),
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("shaide-server-data"),
									MountPath: pulumi.String("/root/.config"),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: corev1.PersistentVolumeClaimTypeArray{
				&corev1.PersistentVolumeClaimTypeArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Name: pulumi.String("shaide-server-data"),
					},
					Spec: shaidePVCSpec,
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt, deps.ConfigDeps, deps.ServiceAccountDeps, deps.StorageDeps)
	if err != nil {
		return err
	}

	// shaide-server is never a LoadBalancer of its own. External traffic
	// enters through the shared Gateway, whose load balancer the platform
	// provides (a cloud LB, or MetalLB on-prem).
	useGateway := cfg.Routing.InfraStackRef != "" || cfg.Routing.GatewayHostname != ""

	shaideSvc, err := corev1.NewService(ctx, "shaide-server-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-server"),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    deps.MetaLabels("shaide-server", "server"),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: deps.Labels("shaide-server"),
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(80),
					TargetPort: pulumi.Int(8080),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt)
	if err != nil {
		return err
	}

	if useGateway {
		hostnameOut := getGatewayHostname(ctx, cfg.Routing.InfraStackRef, cfg.Routing.GatewayHostname)
		if err := createHTTPRoute(ctx, deps, cfg.Namespace, hostnameOut, cfg.Routing.GatewayName, cfg.Routing.GatewayNamespace, shaideSvc); err != nil {
			return err
		}
	}

	return provider.PostDeployService(ctx, deps, cfg.Namespace, shaideSvc)
}

// getGatewayHostname returns the gateway hostname either from a Pulumi StackReference
// (cloud stacks) or directly from a config value (on-prem stacks).
func getGatewayHostname(ctx *pulumi.Context, infraStackName, directHostname string) pulumi.StringOutput {
	if infraStackName != "" {
		ref, err := pulumi.NewStackReference(ctx, "infra-stack", &pulumi.StackReferenceArgs{
			Name: pulumi.String(infraStackName),
		})
		if err != nil {
			return pulumi.String("").ToStringOutput()
		}
		return ref.GetStringOutput(pulumi.String("gatewayHostname"))
	}
	return pulumi.String(directHostname).ToStringOutput()
}

// createHTTPRoute creates an HTTPRoute that routes traffic from the shared Gateway
// (in gateway-system namespace) to the shaide-server ClusterIP Service.
func createHTTPRoute(ctx *pulumi.Context, deps *runtime.DeploymentContext, namespace string, hostname pulumi.StringOutput, gatewayName, gatewayNamespace string, shaideSvc pulumi.Resource) error {
	hostnamesOutput := hostname.ApplyT(func(h string) []string {
		return []string{h}
	})

	_, err := apiext.NewCustomResource(ctx, "shaide-route", &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("HTTPRoute"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-route"),
			Namespace: pulumi.String(namespace),
			Annotations: pulumi.StringMap{
				"pulumi.com/patchForce": pulumi.String("true"),
			},
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]interface{}{
				"parentRefs": []map[string]interface{}{
					{
						"name":      gatewayName,
						"namespace": gatewayNamespace,
					},
				},
				"hostnames": hostnamesOutput,
				"rules": []map[string]interface{}{
					{
						"backendRefs": []map[string]interface{}{
							{
								"name": "shaide-server",
								"port": 80,
							},
						},
					},
				},
			},
		},
	}, deps.ProviderOpt, pulumi.DependsOn([]pulumi.Resource{shaideSvc}))
	return err
}
