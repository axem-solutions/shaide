package main

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Run by the Pulumi CLI, so the working directory is already the
		// project directory and relative chart paths resolve against it.
		return harbor.DeployHarbor(ctx, ".")
	})
}
