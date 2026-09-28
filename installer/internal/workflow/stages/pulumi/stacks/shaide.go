package stacks

import (
	"context"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

func DeployAppShaide(rt *core.Runtime) error {
	shaideStack := shaide.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppShaide),
		stackpkg.Options{
			Platform:   cluster.Provider(rt.Bootstrap.Provider),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		shaide.Options{
			GatewayHostname: rt.Bootstrap.GatewayHostname,
			Images:          mirroredImages(rt.Bootstrap.Catalog.ServiceImages, harborRegistryHostname(rt)),
		},
	)

	deployer, err := newStackDeployer(rt, shaideStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	if _, err := deployer.Deploy(context.Background()); err != nil {
		return err
	}

	return nil
}
