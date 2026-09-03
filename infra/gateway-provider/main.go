package main

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Pulumi.yaml lives in deployments while the bundled CRDs live at the
		// gateway-provider project root.
		return gateway.DeployGatewayProvider(ctx, "..")
	})
}
