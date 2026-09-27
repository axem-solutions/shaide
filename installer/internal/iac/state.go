package iac

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

// StateResources returns the resources the stack's current Pulumi state
// holds, before this run changes anything. A stack that has never been
// deployed returns none.
func (d *StackDeployer) StateResources(ctx context.Context) ([]apitype.ResourceV3, error) {
	stack, err := d.deployer.prepareStack(ctx, d.stack.Deploy)
	if err != nil {
		return nil, err
	}

	exported, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("export %s stack state: %w", d.deployer.StackName, err)
	}
	if len(exported.Deployment) == 0 || string(exported.Deployment) == "null" {
		return nil, nil
	}

	var deployment apitype.DeploymentV3
	if err := json.Unmarshal(exported.Deployment, &deployment); err != nil {
		return nil, fmt.Errorf("read %s stack state: %w", d.deployer.StackName, err)
	}
	return deployment.Resources, nil
}
