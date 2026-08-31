package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/iac/decoder"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployHarbor(rt *core.Runtime) error {
	workDir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectHarbor)
	stackFile := filepath.Join(workDir, "Pulumi.yaml")

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

	stackConfig["harbor:platform"] = auto.ConfigValue{
		Value: rt.Bootstrap.CloudPlatform,
	}
	stackConfig["harbor:kubeconfig"] = auto.ConfigValue{
		Value: rt.Cluster.ConfigPath,
	}
	stackConfig["harbor:context"] = auto.ConfigValue{
		Value: rt.Cluster.SelectedContext,
	}

	adminPassword, ok := stackConfig["harbor:adminPassword"]
	if !ok {
		return fmt.Errorf("harbor admin password is missing")
	}

	rt.Discovery.AdminPassword = adminPassword.Value

	// &Deployer should be in a stage struct
	// Change stage struct and stage specific structs so those
	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectHarbor,
		StackName:   projectHarbor,
		WorkDir:     workDir,
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Logger:      rt.Logger.Writer(),
		Config:      stackConfig,
		Destroy:     false,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
		SkipRefresh: true,
	})
	if err != nil {
		return err
	}

	// harborDir is passed explicitly: the program runs as an inline source, so
	// it inherits the installer's working directory rather than WorkDir, and
	// the relative chartPath from Pulumi.harbor.yaml has to be anchored here.
	_, err = deployer.Deploy(context.Background(), func(ctx *pulumi.Context) error {
		return harbor.DeployHarbor(ctx, workDir)
	})
	if err != nil {
		return err
	}

	return nil
}
