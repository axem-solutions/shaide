package monitoring

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/alloy"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/dashboards"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/grafana"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/loki"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/prometheus"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/platform"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DeployMonitoring deploys monitoring while resolving relative chart paths
// against projectDir. The installer must use this entry point because an
// inline Automation API program does not inherit auto.WorkDir as its process
// working directory.
func DeployMonitoring(ctx *pulumi.Context, projectDir string) error {
	cfg := appconfig.Load(ctx, projectDir)

	// --- K8s Provider ---
	k8sProviderArgs := &kubernetes.ProviderArgs{}
	if cfg.Kubeconfig != "" {
		k8sProviderArgs.Kubeconfig = pulumi.StringPtr(cfg.Kubeconfig)
	}
	k8sProvider, err := kubernetes.NewProvider(ctx, "monitoring-k8s", k8sProviderArgs)
	if err != nil {
		return err
	}
	providerOpt := pulumi.Provider(k8sProvider)

	// --- Namespace ---
	ns, err := platform.CreateNamespace(ctx, cfg.Namespace, providerOpt)
	if err != nil {
		return err
	}
	nsOpt := pulumi.DependsOn([]pulumi.Resource{ns})

	// --- Deploy components ---
	if cfg.Components["loki"] {
		if err := loki.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return err
		}
	}
	if cfg.Components["grafana"] {
		if err := grafana.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return err
		}
	}
	if cfg.Components["alloy"] {
		if err := alloy.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return err
		}
	}
	if cfg.Components["prometheus"] {
		if err := prometheus.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return err
		}
	}
	if cfg.Components["dashboards"] {
		if err := dashboards.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return err
		}
	}

	return nil
}
