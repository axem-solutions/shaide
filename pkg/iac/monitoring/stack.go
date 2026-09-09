package monitoring

import (
	monitoringconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Stack struct {
	config monitoringconfig.Config
}

func NewStack(projectDir string, options stack.Options) *Stack {
	return &Stack{config: monitoringconfig.New(projectDir, options)}
}

func (s *Stack) Config() stack.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployMonitoring(ctx, s.config)
}

var _ stack.Stack = (*Stack)(nil)
