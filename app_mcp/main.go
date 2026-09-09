// App MCP application stack orchestrator.
//
// The deployment itself lives in pkg/iac/mcp so the installer can drive it
// through the shared stack framework; this module is only the Pulumi entry
// point for running the stack directly.
package main

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Run by the Pulumi CLI, so the working directory is already the
		// project directory and relative paths resolve against it.
		return mcp.DeployAppMCP(ctx, ".")
	})
}
