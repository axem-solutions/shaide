package stacks

import (
	"context"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/iac/decoder"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployMonitoring(rt *core.Runtime) error {
	workdir := rt.Bootstrap.Bundle.PulumiWorkDir
	monitorDir := filepath.Join(workdir, "monitoring")

	stackFile := filepath.Join(monitorDir, "Pulumi.yaml")

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

	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectMonitoring,
		StackName:   stackMonitoring,
		WorkDir:     monitorDir,
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Logger:      rt.Logger.Writer(),
		Config:      stackConfig,
		Destroy:     true,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
	})
	if err != nil {
		return err
	}

	// Inline Automation API programs retain the installer's process working
	// directory. Pass the project directory explicitly so bundled chart paths
	// are resolved relative to deployments/monitoring, not the launch directory.
	_, err = deployer.Deploy(context.Background(), func(ctx *pulumi.Context) error {
		return monitoring.DeployMonitoring(ctx, monitorDir)
	})
	if err != nil {
		return err
	}
	return nil
}
