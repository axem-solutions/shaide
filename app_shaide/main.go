// App Shaide application stack entry point for the Pulumi CLI.
//
// The program itself lives in pkg/iac/shaide, which the installer deploys
// through the same code path.
package main

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Run by the Pulumi CLI, so the working directory is already the
		// project directory and relative paths resolve against it.
		return shaide.DeployAppShaide(ctx, ".")
	})
}
