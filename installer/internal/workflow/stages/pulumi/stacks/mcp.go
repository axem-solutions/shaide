package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/placement"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

// DeployAppMCP deploys the MCP server namespace with KC-MCP when the
// Knowledge Center is installed.
//
// KC-MCP is the only MCP server the installer runs: generic data sources are
// not selectable yet. So when the Knowledge Center is not installed, an
// app-mcp stack an earlier run deployed holds nothing but KC-MCP and is
// destroyed. One that has generic data sources configured was set up by hand
// and is left for the operator.
func DeployAppMCP(rt *core.Runtime) error {
	mcpStack := newMCPStack(rt)

	if !rt.KnowledgeCenter.Enabled {
		return removeAppMCP(rt, mcpStack)
	}

	deployer, err := newStackDeployer(rt, mcpStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	if _, err := deployer.Deploy(context.Background()); err != nil {
		return err
	}

	return nil
}

func newMCPStack(rt *core.Runtime) *mcp.Stack {
	knowledgeCenterEnabled := rt.KnowledgeCenter.Enabled

	return mcp.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppMCP),
		stackpkg.Options{
			Platform:   cluster.Provider(rt.Bootstrap.Provider),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		mcp.Options{
			// The RBAC Role is bound to the Shaide Server ServiceAccount, so
			// both must match what app-shaide deployed.
			ShaideNamespace: shaide.Namespace,
			ShaideSAName:    shaide.ServiceAccountName,

			// The same answer and credentials app-shaide got.
			KnowledgeCenterEnabled: &knowledgeCenterEnabled,
			KnowledgeCenter:        rt.KnowledgeCenter.Credentials,
			Images:                 mirroredImages(rt.Bootstrap.Catalog.ServiceImages, harborRegistryHostname(rt)),

			// Like app-shaide, prefer the node pools assigned to CPU work.
			Placement: mcpPlacement(rt.Placement.CPU),
		},
	)
}

// removeAppMCP destroys an app-mcp stack an earlier run deployed for KC-MCP.
// A stack that was never created, or has nothing deployed, needs nothing.
func removeAppMCP(rt *core.Runtime, mcpStack *mcp.Stack) error {
	deployer, err := newUnresolvedDeployer(rt, mcpStack)
	if err != nil {
		return err
	}

	ctx := context.Background()
	exists, err := deployer.Exists(ctx, mcpStack.Deploy)
	if err != nil {
		return err
	}
	if !exists {
		rt.Detailf("the Knowledge Center is not installed; no app-mcp stack to deploy or remove")
		return nil
	}

	resources, err := deployer.StateResources(ctx, mcpStack.Deploy)
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		rt.Detailf("the Knowledge Center is not installed; no app-mcp stack to deploy or remove")
		return nil
	}

	stored, err := deployer.StoredConfig(ctx, mcpStack.Deploy)
	if err != nil {
		return err
	}
	if mcpStack.HasDatasources(stored) {
		return fmt.Errorf("the app-mcp stack has MCP data sources configured besides KC-MCP; " +
			"the installer does not manage those, so remove KC-MCP from it by hand")
	}

	rt.Detailf("the Knowledge Center is not installed; destroying the app-mcp stack that ran KC-MCP")
	return deployer.DestroyOnly(ctx, mcpStack.Deploy)
}

func mcpPlacement(pools []placement.Pool) []mcp.PlacementTerm {
	terms := placement.Terms(pools)
	if len(terms) == 0 {
		return nil
	}

	out := make([]mcp.PlacementTerm, 0, len(terms))
	for _, term := range terms {
		out = append(out, mcp.PlacementTerm{Key: term.Key, Values: term.Values})
	}

	return out
}
