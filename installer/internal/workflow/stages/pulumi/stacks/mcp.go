package stacks

import (
	"context"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

// DeployAppMCP deploys the MCP server namespace and its data source runtimes.
//
// The step is not registered in the stage yet: the stack requires at least one
// data source, and nothing supplies them. The intended flow is for an operator
// to publish an MCP server image into Harbor and select from what is
// published, at which point the selection becomes mcp.Options.Datasources and
// this gains a When gating it on a non-empty selection, as app-serving does
// for models.
func DeployAppMCP(rt *core.Runtime) error {
	mcpStack := mcp.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppMCP),
		stackpkg.Options{
			Platform:   platform.Platform(rt.Bootstrap.CloudPlatform),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		mcp.Options{
			// The RBAC Role is bound to the Shaide Server ServiceAccount, so
			// both must match what app-shaide deployed.
			ShaideNamespace: shaide.Namespace,
			ShaideSAName:    shaide.ServiceAccountName,
		},
	)

	deployer, err := newStackDeployer(rt, mcpStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	if _, err := deployer.Deploy(context.Background()); err != nil {
		return err
	}

	return nil
}

// ServesMCPDatasources reports whether anything is selected to run in the MCP
// namespace. It gates the step once a data source selection exists.
func ServesMCPDatasources(rt *core.Runtime) bool {
	return false
}
