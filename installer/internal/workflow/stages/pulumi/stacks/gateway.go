package stacks

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func DeployGatewayProvider(rt *core.Runtime) error {
	gatewayStack := gateway.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectGatewayProvider),
		stack.Options{
			Platform:   platform.Platform(rt.Bootstrap.CloudPlatform),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
	)

	deployer, err := newStackDeployer(rt, gatewayStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	// Take the hostname the operator supplied up front so later stages have it
	// even when the Gateway itself does not export one.
	if hostname, ok := gatewayStack.Hostname(deployer.ResolvedConfig()); ok {
		if hostname = strings.TrimSpace(hostname); hostname != "" {
			rt.Bootstrap.GatewayHostname = hostname
		}
	}

	result, err := deployer.Deploy(context.Background())
	if err != nil {
		return err
	}

	// A StackReference can supply the hostname asynchronously. Prefer the
	// resolved Pulumi output when present so downstream projects receive the
	// same hostname as the shared Gateway.
	if output, ok := result.Outputs["gatewayHostname"]; ok {
		if hostname, ok := output.Value.(string); ok && strings.TrimSpace(hostname) != "" {
			rt.Bootstrap.GatewayHostname = strings.TrimSpace(hostname)
		}
	}

	return nil
}
