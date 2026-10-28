package iac

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// StateResources returns the resources the stack's current Pulumi state
// holds, before this run changes anything. A stack that has never been
// deployed returns none.
func (d *StackDeployer) StateResources(ctx context.Context) ([]apitype.ResourceV3, error) {
	return d.deployer.StateResources(ctx, d.stack.Deploy)
}

// StateResources is StackDeployer.StateResources for a stack whose config is
// not resolved, e.g. one that is about to be destroyed.
func (d *Deployer) StateResources(ctx context.Context, deployment func(ctx *pulumi.Context) error) ([]apitype.ResourceV3, error) {
	stack, err := d.prepareStack(ctx, deployment)
	if err != nil {
		return nil, err
	}

	exported, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("export %s stack state: %w", d.StackName, err)
	}
	if len(exported.Deployment) == 0 || string(exported.Deployment) == "null" {
		return nil, nil
	}

	var state apitype.DeploymentV3
	if err := json.Unmarshal(exported.Deployment, &state); err != nil {
		return nil, fmt.Errorf("read %s stack state: %w", d.StackName, err)
	}
	return state.Resources, nil
}

// Exists reports whether an earlier run created the stack. It does not create
// it.
func (d *Deployer) Exists(ctx context.Context, deployment func(ctx *pulumi.Context) error) (bool, error) {
	if _, err := os.Stat(d.StateDir); os.IsNotExist(err) {
		return false, nil
	}

	_, err := auto.SelectStackInlineSource(
		ctx,
		d.StackName,
		d.ProjectName,
		deployment,
		auto.WorkDir(d.WorkDir),
		auto.SecretsProvider("passphrase"),
		auto.EnvVars(d.env()),
	)
	if auto.IsSelectStack404Error(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select %s stack: %w", d.StackName, err)
	}
	return true, nil
}

// StoredConfig returns the config the stack holds from earlier runs, with
// secrets decrypted. A stack that does not exist yet has none.
func (d *Deployer) StoredConfig(ctx context.Context, deployment func(ctx *pulumi.Context) error) (auto.ConfigMap, error) {
	stack, err := d.openStack(ctx, deployment)
	if err != nil {
		return nil, err
	}

	config, err := stack.GetAllConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("read %s stack config: %w", d.StackName, err)
	}
	return config, nil
}
