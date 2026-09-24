package gateway

import (
	gatewayconfig "github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options are values the installer discovers from the cluster.
type Options struct {
	// ALBSubnetID pre-fills the Application Gateway for Containers subnet,
	// typically with the association already on the cluster.
	ALBSubnetID string

	// ALBSubnetPlaceholder hints at the expected subnet ID when there is
	// nothing to pre-fill.
	ALBSubnetPlaceholder string
}

type Stack struct {
	config gatewayconfig.Config
}

func NewStack(projectDir string, options stack.Options, opts Options) *Stack {
	return &Stack{config: gatewayconfig.New(projectDir, options, gatewayconfig.Sources{
		ALBSubnetID:          opts.ALBSubnetID,
		ALBSubnetPlaceholder: opts.ALBSubnetPlaceholder,
	})}
}

func (s *Stack) Config() stack.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployGatewayProvider(ctx, s.config)
}

// Hostname reports the Gateway hostname that was resolved for this deployment.
// Later stages point their HTTPRoutes at the same host.
func (s *Stack) Hostname(values auto.ConfigMap) (string, bool) {
	value, ok := values[s.Config().Definition().Key(gatewayconfig.KeyGatewayHostname)]
	return value.Value, ok
}

var _ stack.Stack = (*Stack)(nil)
