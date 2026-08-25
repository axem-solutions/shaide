package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/iac/decoder"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/services"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func DeployOnPremHarbor(rt *core.Runtime) error {
	workdir := rt.Bootstrap.Bundle.PulumiWorkDir
	onpremDir := filepath.Join(workdir, "harbor")
	stackFile := filepath.Join(onpremDir, "Pulumi.yaml")
	_, keys, err := decoder.LoadTemplateFile(stackFile)
	if err != nil {
		return err
	}

	stackConfig := auto.ConfigMap{}

	for _, key := range keys {
		configValue, err := resolveConfigKey(rt.Reporter, key.Key)
		if err != nil {
			return err
		}
		stackConfig[key.Name] = configValue
	}

	adminPassword, ok := stackConfig["harbor:adminPassword"]
	if !ok {
		return fmt.Errorf("harbor admin password is missing")
	}

	rt.Discovery.AdminPassword = adminPassword.Value

	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectOnPremHarbor,
		StackName:   stackOnPremHarbor,
		WorkDir:     onpremDir,
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Logger:      rt.Logger.Writer(),
		Config:      stackConfig,
		Destroy:     true,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
	})
	if err != nil {
		return err
	}

	_, err = deployer.Deploy(context.Background(), services.DeployServices)
	if err != nil {
		return err
	}
	return nil
}
