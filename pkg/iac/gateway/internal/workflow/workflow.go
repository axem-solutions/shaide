// Package workflow coordinates live-cluster preparation that must happen
// before the gateway provider registers Pulumi resources.
package workflow

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Prepare prepares pre-existing Istio resources for Helm ownership during an
// update that installs Istio.
func Prepare(ctx *pulumi.Context, cfg config.Config) error {
	if ctx.DryRun() || !cfg.Istio.Enabled {
		return nil
	}

	restConfig, err := connection.BuildRestConfig(cfg.Kubernetes)
	if err != nil {
		return fmt.Errorf("build Kubernetes REST config: %w", err)
	}

	writeLog := func(format string, args ...any) {
		_ = ctx.Log.Info(fmt.Sprintf(format, args...), nil)
	}

	if err := ensureIstioChartOwnership(
		ctx.Context(),
		restConfig,
		cfg.Istio.Namespace,
		writeLog,
	); err != nil {
		return fmt.Errorf("prepare Istio chart ownership: %w", err)
	}

	return nil
}
