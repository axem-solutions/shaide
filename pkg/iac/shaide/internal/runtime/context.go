package runtime

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

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
	Labels     func(string) pulumi.StringMap
	MetaLabels func(name, component string) pulumi.StringMap
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
