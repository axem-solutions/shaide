package harbor

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/chart"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/kube"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/mirror"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/registry"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/setup"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/storage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployHarbor(ctx *pulumi.Context, projectDir string) error {
	cfg, err := config.Load(ctx, projectDir)
	if err != nil {
		return fmt.Errorf("load Harbor config: %w", err)
	}

	provider, err := kube.NewProvider(ctx, cfg.Kubernetes.KubeconfigPath, cfg.Kubernetes.Context)
	if err != nil {
		return fmt.Errorf("create Kubernetes provider: %w", err)
	}

	namespace, err := kube.NewNamespace(ctx, provider, cfg.Harbor.Namespace)
	if err != nil {
		return fmt.Errorf("create Harbor namespace: %w", err)
	}

	storageResult, err := storage.Prepare(ctx, provider, namespace, cfg.Storage)
	if err != nil {
		return fmt.Errorf("prepare Harbor storage: %w", err)
	}

	release, err := chart.Deploy(ctx, provider, namespace, cfg, storageResult.Dependencies)
	if err != nil {
		return fmt.Errorf("deploy Harbor chart: %w", err)
	}

	// Currently cloud-specific, but fundamentally usable by both.
	if cfg.Network.HTTPSFastFailEnabled {
		if err := registry.EnableHTTPSFastFail(
			ctx,
			provider,
			cfg.Harbor.Namespace,
			chart.ServiceName,
			release,
		); err != nil {
			return fmt.Errorf("configure Harbor HTTPS fast-fail: %w", err)
		}
	}

	// Pull secret + Harbor setup + image mirror require the robot account.
	if cfg.Harbor.Robot.Configured {
		setupResult, err := setup.Ensure(
			ctx,
			release,
			cfg,
		)
		if err != nil {
			return fmt.Errorf("configure Harbor: %w", err)
		}

		if _, err := registry.NewPullSecret(
			ctx,
			provider,
			namespace,
			cfg.Harbor.Namespace,
			cfg.Network.RegistryHostname,
			setupResult.Username,
			setupResult.Password,
			release,
		); err != nil {
			return fmt.Errorf("create Harbor pull secret: %w", err)
		}

		if cfg.Mirror.Enabled {
			if err := mirror.Deploy(
				ctx,
				provider,
				cfg.Mirror,
				cfg.Network.RegistryHostname,
				setupResult.Username,
				setupResult.Password,
			); err != nil {
				return fmt.Errorf("mirror Harbor images: %w", err)
			}
		}
	}

	if cfg.Network.NodeTrustEnabled {
		if err := registry.DeployNodeTrust(
			ctx,
			provider,
			release,
			cfg,
		); err != nil {
			return fmt.Errorf("configure Harbor node trust: %w", err)
		}
	}

	ctx.Export("harborReleaseName", release.Name)

	return nil
}
