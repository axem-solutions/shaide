package stacks

import (
	"context"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func DeployMonitoring(rt *core.Runtime) error {
	monitoringStack := monitoring.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectMonitoring),
		stack.Options{
			Platform:   platform.Platform(rt.Bootstrap.CloudPlatform),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
	)

	deployer, err := newStackDeployer(rt, monitoringStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	_, err = deployer.Deploy(context.Background())
	if err != nil {
		return err
	}
	return nil
}
