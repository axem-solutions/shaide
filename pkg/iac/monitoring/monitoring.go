package monitoring

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/alloy"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/dashboards"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/grafana"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/loki"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/components/prometheus"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DeployMonitoring deploys monitoring while resolving relative chart paths
// against projectDir. The installer must use this entry point because an
// inline Automation API program does not inherit auto.WorkDir as its process
// working directory.
func DeployMonitoring(ctx *pulumi.Context, projectDir string) error {
	return deployMonitoring(ctx, appconfig.New(projectDir, stack.Options{}))
}

func deployMonitoring(ctx *pulumi.Context, configuration appconfig.Config) error {
	cfg, err := configuration.Load(ctx)
	if err != nil {
		return fmt.Errorf("load monitoring config: %w", err)
	}

	// --- K8s Provider ---
	k8sProvider, err := iackube.NewProvider(ctx, cfg.Kubernetes, iackube.ProviderOptions{
		Name: "monitoring-k8s",
	})
	if err != nil {
		return fmt.Errorf("create Kubernetes provider: %w", err)
	}
	providerOpt := pulumi.Provider(k8sProvider)

	// --- Namespace ---
	ns, err := iackube.CreateNamespace(ctx, cfg.Namespace, providerOpt)
	if err != nil {
		return fmt.Errorf("create monitoring namespace: %w", err)
	}
	nsOpt := pulumi.DependsOn([]pulumi.Resource{ns})

	// --- Deploy components ---
	if cfg.Components["loki"] {
		if err := loki.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return fmt.Errorf("deploy Loki: %w", err)
		}
	}
	if cfg.Components["grafana"] {
		if err := grafana.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return fmt.Errorf("deploy Grafana: %w", err)
		}
	}
	if cfg.Components["alloy"] {
		if err := alloy.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return fmt.Errorf("deploy Alloy: %w", err)
		}
	}
	if cfg.Components["prometheus"] {
		if err := prometheus.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return fmt.Errorf("deploy Prometheus: %w", err)
		}
	}
	if cfg.Components["dashboards"] {
		if err := dashboards.Deploy(ctx, cfg, providerOpt, nsOpt); err != nil {
			return fmt.Errorf("deploy monitoring dashboards: %w", err)
		}
	}

	return nil
}
