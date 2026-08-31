package shaide

import (
	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/cloudprovider"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/components/controlpanel"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/components/qdrant"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/components/rustfs"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/components/shaide"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/components/webapp"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/platform"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployAppShaide(ctx *pulumi.Context) error {
	appConfig := appconfig.Load(ctx)

	// --- K8s Provider ---
	k8sProviderArgs := &kubernetes.ProviderArgs{}
	if appConfig.Kubeconfig != "" {
		k8sProviderArgs.Kubeconfig = pulumi.StringPtr(appConfig.Kubeconfig)
	}
	k8sProvider, err := kubernetes.NewProvider(ctx, "app-shaide-k8s", k8sProviderArgs)
	if err != nil {
		return err
	}
	providerOpt := pulumi.Provider(k8sProvider)

	// --- Namespace ---
	ns, err := iackube.CreateNamespace(ctx, appConfig.Namespace, providerOpt)
	if err != nil {
		return err
	}
	nsOpt := pulumi.DependsOn([]pulumi.Resource{ns})

	// --- GHCR Registry Secret (for private shaide-server image) ---
	registrySecret, err := platform.CreateGHCRSecret(ctx, appConfig, providerOpt, nsOpt)
	if err != nil {
		return err
	}

	// --- ConfigMap and Secret ---
	shaideConfig, shaideSecrets, err := platform.CreateAppShaideConfig(ctx, appConfig, providerOpt, nsOpt)
	if err != nil {
		return err
	}

	// --- ServiceAccount and RBAC for shaide-server ---
	serviceAccount, err := platform.CreateShaideServiceAccount(ctx, appConfig, providerOpt, nsOpt)
	if err != nil {
		return err
	}
	shaideRBAC, err := platform.CreateShaideRBAC(ctx, appConfig, providerOpt)
	if err != nil {
		return err
	}
	serviceAccountDeps := []pulumi.Resource{serviceAccount}
	serviceAccountDeps = append(serviceAccountDeps, shaideRBAC...)

	deps := runtime.NewDeploymentContext(
		providerOpt,
		nsOpt,
		pulumi.DependsOn([]pulumi.Resource{shaideConfig, shaideSecrets}),
		pulumi.DependsOn([]pulumi.Resource{shaideConfig, shaideSecrets, registrySecret}),
		pulumi.DependsOn(serviceAccountDeps),
		"ghcr-creds",
	)

	// --- Cloud provider (selects cloud-specific resource implementations) ---
	cp := cloudprovider.New(appConfig.CloudProvider)

	// --- Storage provisioning (cloud-specific; no-op for dynamic provisioners) ---
	pvs, err := cp.ProvisionStorage(ctx, deps, appConfig)
	if err != nil {
		return err
	}
	if len(pvs) > 0 {
		deps.StorageDeps = pulumi.DependsOn(pvs)
	}

	// --- Deploy components ---
	if err := shaide.Deploy(ctx, deps, appConfig, cp); err != nil {
		return err
	}
	if err := controlpanel.Deploy(ctx, deps, appConfig); err != nil {
		return err
	}
	if err := webapp.Deploy(ctx, deps, appConfig); err != nil {
		return err
	}
	if err := rustfs.Deploy(ctx, deps, appConfig); err != nil {
		return err
	}
	if err := qdrant.Deploy(ctx, deps, appConfig); err != nil {
		return err
	}

	return nil
}
