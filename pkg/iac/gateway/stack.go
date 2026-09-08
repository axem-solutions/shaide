package gateway

import (
	gatewayconfig "github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Stack struct {
	config gatewayconfig.Config
}

func NewStack(projectDir string, options stack.Options) *Stack {
	return &Stack{config: gatewayconfig.New(projectDir, options)}
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
