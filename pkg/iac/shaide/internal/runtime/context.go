package runtime

import (
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DeploymentContext carries shared Pulumi options for all app-shaide components.
type DeploymentContext struct {
	ProviderOpt        pulumi.ResourceOption
	NsOpt              pulumi.ResourceOption
	ConfigDeps         pulumi.ResourceOption
	RegistryDeps       pulumi.ResourceOption
	ServiceAccountDeps pulumi.ResourceOption
	// StorageDeps is populated by the cloud provider's ProvisionStorage call.
	// StatefulSets depend on it so PersistentVolumes exist before PVCs are created.
	// Defaults to an empty DependsOn (no-op) for clouds with dynamic provisioning.
	StorageDeps        pulumi.ResourceOption
	RegistrySecretName string
	Labels             func(string) pulumi.StringMap
	MetaLabels         func(name, component string) pulumi.StringMap
}

// NewDeploymentContext constructs deployment wiring shared across all components.
func NewDeploymentContext(
	providerOpt pulumi.ResourceOption,
	nsOpt pulumi.ResourceOption,
	configDeps pulumi.ResourceOption,
	registryDeps pulumi.ResourceOption,
	serviceAccountDeps pulumi.ResourceOption,
	registrySecretName string,
) *DeploymentContext {
	return &DeploymentContext{
		ProviderOpt:        providerOpt,
		NsOpt:              nsOpt,
		ConfigDeps:         configDeps,
		RegistryDeps:       registryDeps,
		ServiceAccountDeps: serviceAccountDeps,
		StorageDeps:        pulumi.DependsOn([]pulumi.Resource{}),
		RegistrySecretName: registrySecretName,
		Labels: func(name string) pulumi.StringMap {
			return pulumi.StringMap{
				"app.kubernetes.io/name":    pulumi.String(name),
				"app.kubernetes.io/part-of": pulumi.String("app-shaide"),
			}
		},
		MetaLabels: func(name, component string) pulumi.StringMap {
			return pulumi.StringMap{
				"app.kubernetes.io/name":       pulumi.String(name),
				"app.kubernetes.io/part-of":    pulumi.String("app-shaide"),
				"app.kubernetes.io/component":  pulumi.String(component),
				"app.kubernetes.io/managed-by": pulumi.String("pulumi"),
				"axem.dev/platform":            pulumi.String("ai-platform"),
			}
		},
	}
}

// PodAntiAffinityFor builds a soft (preferred) podAntiAffinity spreading a component's own
// replicas across nodes, once there's more than one — keyed on the same
// app.kubernetes.io/name label the component's own Selector already uses, so it only ever
// avoids co-locating with its own pods, never another component's.
func (deps *DeploymentContext) PodAntiAffinityFor(name string) *corev1.PodAntiAffinityArgs {
	return &corev1.PodAntiAffinityArgs{
		PreferredDuringSchedulingIgnoredDuringExecution: corev1.WeightedPodAffinityTermArray{
			&corev1.WeightedPodAffinityTermArgs{
				Weight: pulumi.Int(100),
				PodAffinityTerm: &corev1.PodAffinityTermArgs{
					LabelSelector: &metav1.LabelSelectorArgs{
						MatchLabels: deps.Labels(name),
					},
					TopologyKey: pulumi.String("kubernetes.io/hostname"),
				},
			},
		},
	}
}
