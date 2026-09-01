package main

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// The Pulumi CLI runs this program from the project directory, so local
		// Kustomize paths are resolved relative to that directory.
		return gateway.DeployGatewayProvider(ctx, ".")
	})
}
