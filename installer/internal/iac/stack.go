package iac

import (
	"context"
	"fmt"
	"io"

	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// StackDeployerOptions contains installer-wide Pulumi settings for a
// declarative stack. Project identity, project directory, config schema, and
// the inline Pulumi program are all owned by Stack.
type StackDeployerOptions struct {
	Stack       stackpkg.Stack
	Prompter    stackconfig.Prompter
	StateDir    string
	Passphrase  string
	Logger      io.Writer
	Destroy     bool
	SkipRefresh bool
}

// StackDeployer adapts a stack.Stack to the installer's Pulumi deployer. It
// retains the resolved config for workflow stages that must forward a value to
// later stages.
type StackDeployer struct {
	stack          stackpkg.Stack
	resolvedConfig auto.ConfigMap
	deployer       *Deployer
}

func NewStackDeployer(opts StackDeployerOptions) (*StackDeployer, error) {
	if opts.Stack == nil {
		return nil, fmt.Errorf("stack is required")
	}

	config := opts.Stack.Config()
	resolvedConfig, err := config.Resolve(opts.Prompter)
	if err != nil {
		return nil, fmt.Errorf("resolve %s stack config: %w", config.StackName(), err)
	}

	deployer, err := NewDeployer(DeployerOptions{
		ProjectName: config.ProjectName(),
		StackName:   config.StackName(),
		WorkDir:     config.ProjectDir(),
		StateDir:    opts.StateDir,
		Config:      resolvedConfig,
		Passphrase:  opts.Passphrase,
		Destroy:     opts.Destroy,
		SkipRefresh: opts.SkipRefresh,
		Logger:      opts.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create %s stack deployer: %w", config.StackName(), err)
	}

	return &StackDeployer{
		stack:          opts.Stack,
		resolvedConfig: resolvedConfig,
		deployer:       deployer,
	}, nil
}

func (d *StackDeployer) ResolvedConfig() auto.ConfigMap {
	return d.resolvedConfig
}

func (d *StackDeployer) Deploy(ctx context.Context) (*auto.UpResult, error) {
	return d.deployer.Deploy(ctx, d.stack.Deploy)
}
