package stacks

import (
	"context"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

// newUnresolvedDeployer opens a stack without resolving this run's config,
// to read what earlier runs stored or to destroy it.
func newUnresolvedDeployer(rt *core.Runtime, deploymentStack stackpkg.Stack) (*iac.Deployer, error) {
	config := deploymentStack.Config()

	return iac.NewDeployer(iac.DeployerOptions{
		ProjectName: config.ProjectName(),
		StackName:   config.StackName(),
		WorkDir:     config.ProjectDir(),
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
		Logger:      rt.Logger.Writer(),
		Confirmer:   rt.Reporter,
		Target: &iac.ClusterTarget{
			KubeconfigPath: rt.Cluster.ConfigPath,
			Context:        rt.Cluster.SelectedContext,
		},
	})
}

// knowledgeCenterCredentials are the credentials app-shaide holds from an
// earlier run with the Knowledge Center, completed with generated ones. The
// first install generates them all; later runs reuse them, including after
// KC was turned off in between, since a run without KC leaves them in the
// stack config.
func knowledgeCenterCredentials(rt *core.Runtime, shaideStack *shaide.Stack) error {
	deployer, err := newUnresolvedDeployer(rt, shaideStack)
	if err != nil {
		return err
	}

	stored, err := deployer.StoredConfig(context.Background(), shaideStack.Deploy)
	if err != nil {
		return err
	}

	retained := shaideStack.KnowledgeCenterCredentials(stored)
	if retained.InternalToken != "" {
		rt.Detailf("reusing the Knowledge Center credentials stored in the app-shaide stack")
	}

	credentials, err := knowledgecenter.CompleteCredentials(retained)
	if err != nil {
		return err
	}

	rt.KnowledgeCenter.Credentials = credentials
	return nil
}
