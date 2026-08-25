package pulumi

import (
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/pulumi/stacks"
)

func Stage() core.Stage {
	return core.Stage{
		Name: "deploy AI platform",
		Steps: []core.Step{
			{
				Name:    "Deploy Gateway Provider",
				Run:     stacks.DeployGatewayProvider,
				Recover: RecoverGatewayProvider,
			},
			{
				Name:    "Deploy App-Serving ",
				Run:     stacks.DeployAppServing,
				Recover: RecoverAppServing,
			},
			{
				Name:    "Deploy App-Shaide ",
				Run:     stacks.DeployAppShaide,
				Recover: RecoverAppShaide,
			},
			{
				Name:    "Deploy Monitoring",
				Run:     stacks.DeployMonitoring,
				Recover: RecoverMonitoring,
			},
		}}
}
