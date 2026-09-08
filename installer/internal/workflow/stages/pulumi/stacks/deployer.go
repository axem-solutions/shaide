package stacks

import (
	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

type stackDeploymentOptions struct {
	Destroy     bool
	SkipRefresh bool
}

func newStackDeployer(
	rt *core.Runtime,
	deploymentStack stackpkg.Stack,
	opts stackDeploymentOptions,
) (*iac.StackDeployer, error) {
	return iac.NewStackDeployer(iac.StackDeployerOptions{
		Stack:       deploymentStack,
		Prompter:    rt.Reporter,
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
		Logger:      rt.Logger.Writer(),
		Destroy:     opts.Destroy,
		SkipRefresh: opts.SkipRefresh,
	})
}
