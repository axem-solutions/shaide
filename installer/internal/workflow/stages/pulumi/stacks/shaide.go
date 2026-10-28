package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/placement"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

func shaidePlacement(pools []placement.Pool) []shaide.PlacementTerm {
	terms := placement.Terms(pools)
	if len(terms) == 0 {
		return nil
	}

	out := make([]shaide.PlacementTerm, 0, len(terms))
	for _, term := range terms {
		out = append(out, shaide.PlacementTerm{Key: term.Key, Values: term.Values})
	}

	return out
}

func DeployAppShaide(rt *core.Runtime) error {
	if rt.KnowledgeCenter.Enabled {
		if err := knowledgeCenterCredentials(rt, newShaideStack(rt)); err != nil {
			return err
		}
	}

	shaideStack := newShaideStack(rt)

	deployer, err := newStackDeployer(rt, shaideStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	if _, err := deployer.Deploy(context.Background()); err != nil {
		return err
	}

	if !rt.KnowledgeCenter.Enabled {
		return nil
	}

	adminPassword, ok := shaideStack.AdminPassword(deployer.ResolvedConfig())
	if !ok {
		return fmt.Errorf("the app-shaide config has no admin password to create the Knowledge Center account with")
	}

	return knowledgecenter.EnsureEmbeddingAccount(rt, adminPassword, rt.KnowledgeCenter.Credentials)
}

// newShaideStack is app-shaide with this run's answers. The Knowledge Center
// credentials are empty until knowledgeCenterCredentials sets them.
func newShaideStack(rt *core.Runtime) *shaide.Stack {
	webAppEnabled := rt.WebApp.Enabled
	knowledgeCenterEnabled := rt.KnowledgeCenter.Enabled

	return shaide.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppShaide),
		stackpkg.Options{
			Platform:   cluster.Provider(rt.Bootstrap.Provider),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		shaide.Options{
			GatewayHostname:        rt.Bootstrap.GatewayHostname,
			WebAppEnabled:          &webAppEnabled,
			KnowledgeCenterEnabled: &knowledgeCenterEnabled,
			KnowledgeCenter:        rt.KnowledgeCenter.Credentials,
			Images:                 mirroredImages(rt.Bootstrap.Catalog.ServiceImages, harborRegistryHostname(rt)),

			// Prefer the node pools assigned to CPU work. The preference is
			// soft, so the components still schedule if those nodes are full.
			Placement: shaidePlacement(rt.Placement.CPU),
		},
	)
}
