package harbor

import (
	harborconfig "github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Stack struct {
	config harborconfig.Config
}

func NewStack(projectDir string, options stack.Options) *Stack {
	return &Stack{config: harborconfig.New(projectDir, options)}
}

func (s *Stack) Config() stack.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployHarbor(ctx, s.config)
}

func (s *Stack) AdminPassword(values auto.ConfigMap) (string, bool) {
	value, ok := values[s.Config().Definition().Key(harborconfig.KeyAdminPassword)]
	return value.Value, ok
}

var _ stack.Stack = (*Stack)(nil)
