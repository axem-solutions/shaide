package iac

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type DeployerOptions struct {
	// ProjectName is the Pulumi project name. It must match the "name" field
	// in the Pulumi.yaml file inside WorkDir.
	ProjectName string

	// StackName selects the Pulumi stack config file in WorkDir.
	StackName string

	// WorkDir is the Pulumi project directory. It must contain Pulumi.yaml.
	// When Pulumi.<stack>.yaml is missing, template defaults in Pulumi.yaml are
	// used to generate it before deployment.
	WorkDir string

	// StateDir is the local Pulumi backend directory.
	StateDir string

	// Config contains runtime stack config values to write before deployment.
	// These values overwrite matching keys in Pulumi.<stack>.yaml.
	Config auto.ConfigMap

	// Passphrase unlocks Pulumi's passphrase-based secrets provider.
	Passphrase string

	// Destroy runs pulumi destroy before pulumi up.
	Destroy bool

	SkipRefresh bool

	// Logger receives Pulumi progress output and deployer log messages.
	Logger io.Writer
}

type Deployer struct {
	ProjectName string
	StackName   string
	WorkDir     string
	StateDir    string
	Passphrase  string
	Config      auto.ConfigMap
	Logger      io.Writer
	Destroy     bool
	SkipRefresh bool
}

func NewDeployer(opts DeployerOptions) (*Deployer, error) {
	if opts.ProjectName == "" {
		return nil, fmt.Errorf("project name is required")
	}
	if opts.StackName == "" {
		return nil, fmt.Errorf("stack name is required")
	}
	if opts.WorkDir == "" {
		return nil, fmt.Errorf("workdir is required")
	}
	if opts.StateDir == "" {
		return nil, fmt.Errorf("stateDir is required")
	}
	if opts.Passphrase == "" {
		return nil, fmt.Errorf("passphrase is required")
	}

	return &Deployer{
		ProjectName: opts.ProjectName,
		StackName:   opts.StackName,
		WorkDir:     opts.WorkDir,
		StateDir:    opts.StateDir,
		Config:      opts.Config,
		Logger:      opts.Logger,
		Destroy:     opts.Destroy,
		Passphrase:  opts.Passphrase,
		SkipRefresh: opts.SkipRefresh,
	}, nil
}

func (d *Deployer) Deploy(ctx context.Context, deployment func(ctx *pulumi.Context) error) (*auto.UpResult, error) {
	stack, err := d.prepareStack(ctx, deployment)
	if err != nil {
		return nil, err
	}

	if d.Destroy {
		if _, err := d.destroy(ctx, stack); err != nil {
			return nil, err
		}
	}

	return d.up(ctx, stack)
}

func (d *Deployer) prepareStack(ctx context.Context, deployment func(ctx *pulumi.Context) error) (auto.Stack, error) {
	if err := os.MkdirAll(d.StateDir, 0o755); err != nil {
		return auto.Stack{}, fmt.Errorf("create pulumi state dir: %w", err)
	}

	stack, err := auto.UpsertStackInlineSource(
		ctx,
		d.StackName,
		d.ProjectName,
		deployment,
		auto.WorkDir(d.WorkDir),
		// TODO: this should be an enum when we want to install using cloud resources
		// awskms, azurekeyvault, gcpkms,
		auto.SecretsProvider("passphrase"),
		auto.EnvVars(d.env()),
	)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("upsert stack: %w", err)
	}

	if err := d.applyConfig(ctx, stack); err != nil {
		return auto.Stack{}, err
	}

	return stack, nil
}

func (d *Deployer) applyConfig(ctx context.Context, stack auto.Stack) error {
	if len(d.Config) == 0 {
		return nil
	}

	if err := stack.SetAllConfig(ctx, d.Config); err != nil {
		return fmt.Errorf("set stack config: %w", err)
	}

	return nil
}

func (d *Deployer) env() map[string]string {
	return map[string]string{
		"PULUMI_BACKEND_URL":                 "file://" + d.StateDir,
		"PULUMI_CONFIG_PASSPHRASE":           d.Passphrase,
		"PULUMI_K8S_UPSERT_EXISTING_OBJECTS": "true",
		"PULUMI_K8S_ENABLE_PATCH_FORCE":      "true",
	}
}

func (d *Deployer) up(ctx context.Context, stack auto.Stack) (*auto.UpResult, error) {
	result, err := withLockRetry(ctx, stack, "up", d.logf, d.wrapPulumiOperationError, func() (auto.UpResult, error) {
		return stack.Up(ctx, d.upOptions()...)
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (d *Deployer) destroy(ctx context.Context, stack auto.Stack) (*auto.DestroyResult, error) {
	result, err := withLockRetry(ctx, stack, "destroy", d.logf, d.wrapPulumiOperationError, func() (auto.DestroyResult, error) {
		return stack.Destroy(ctx, d.destroyOptions()...)
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func withLockRetry[T any](
	ctx context.Context,
	stack auto.Stack,
	operation string,
	logf func(format string, args ...any),
	wrapOperationError func(operation string, err error) error,
	deploy func() (T, error),
) (T, error) {
	result, err := deploy()
	if err == nil {
		return result, nil
	}

	var zero T

	if auto.IsConcurrentUpdateError(err) {
		logf("pulumi stack %q is locked; running pulumi cancel and retrying once", stack.Name())

		if cancelErr := stack.Cancel(ctx); cancelErr != nil {
			return zero, &StackLockedError{
				Stack: stack.Name(),
				Err:   fmt.Errorf("pulumi %s: %w; automatic pulumi cancel failed: %v", operation, err, cancelErr),
			}
		}

		logf("pulumi cancel completed for stack %q; retrying pulumi %s", stack.Name(), operation)

		result, err = deploy()
		if err == nil {
			return result, nil
		}

		if auto.IsConcurrentUpdateError(err) {
			return zero, &StackLockedError{
				Stack: stack.Name(),
				Err:   err,
			}
		}
	}

	return zero, wrapOperationError(operation, err)
}

func (d *Deployer) upOptions() []optup.Option {
	opts := []optup.Option{
		optup.Color("never"),
	}

	if !d.SkipRefresh {
		opts = append(opts, optup.Refresh())
	}

	if d.Logger != nil {
		opts = append(opts,
			optup.ProgressStreams(d.Logger),
			optup.ErrorProgressStreams(d.Logger),
		)
	}

	return opts
}

func (d *Deployer) destroyOptions() []optdestroy.Option {
	opts := []optdestroy.Option{
		optdestroy.Refresh(),
		optdestroy.Color("never"),
	}

	if d.Logger != nil {
		opts = append(opts,
			optdestroy.ProgressStreams(d.Logger),
			optdestroy.ErrorProgressStreams(d.Logger),
		)
	}

	return opts
}

func (d *Deployer) logf(format string, args ...any) {
	if d.Logger == nil {
		return
	}

	fmt.Fprintf(d.Logger, format+"\n", args...)
}
