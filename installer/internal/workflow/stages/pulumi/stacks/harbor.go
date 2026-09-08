package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func DeployHarbor(rt *core.Runtime) error {
	harborStack := harbor.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectHarbor),
		stack.Options{
			Platform:   platform.Platform(rt.Bootstrap.CloudPlatform),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
	)

	deployer, err := newStackDeployer(rt, harborStack, stackDeploymentOptions{
		SkipRefresh: true,
	})
	if err != nil {
		return err
	}

	adminPassword, ok := harborStack.AdminPassword(deployer.ResolvedConfig())
	if !ok {
		return fmt.Errorf("harbor admin password is missing")
	}

	rt.Discovery.AdminPassword = adminPassword

	_, err = deployer.Deploy(context.Background())
	if err != nil {
		return err
	}

	return nil
}
