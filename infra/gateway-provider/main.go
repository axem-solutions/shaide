package main

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		return gateway.DeployGatewayProvider(ctx)
	})
}
