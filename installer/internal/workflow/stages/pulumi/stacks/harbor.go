package stacks

import (
	"context"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployHarbor(rt *core.Runtime) error {
	workdir := rt.Bootstrap.Bundle.PulumiWorkDir
	harborDir := filepath.Join(workdir, projectCloudHarbor)

	deployConfig := auto.ConfigMap{
		pulumiConfigKey(projectCloudHarbor, "kubeconfig"): {
			Value: rt.Cluster.ConfigPath,
		},
		pulumiConfigKey(projectCloudHarbor, "context"): {
			Value: rt.Cluster.SelectedContext,
		},
	}

	adminPassword := rt.Discovery.AdminPassword

	if adminPassword == "" {
		for {
			value, err := rt.Reporter.Input("Harbor admin password", "", "defaultPassword")
			if err != nil {
				return err
			}
			if value != "" {
				adminPassword = value
				break
			}
		}
	}

	// adminPassword and robotPassword belong to the "harbor" config namespace,
	// not the project one: the Harbor program reads them with
	// config.New(ctx, "harbor"), the same namespace Pulumi.harbor.yaml uses for
	// chartPath and namespace. kubeconfig and context above are read with
	// config.New(ctx, ""), which resolves to the project, so those stay as-is.
	rt.Discovery.AdminPassword = adminPassword
	deployConfig[pulumiConfigKey(configNamespaceHarbor, "adminPassword")] = auto.ConfigValue{
		Value:  adminPassword,
		Secret: true,
	}

	if rt.Discovery.RobotPassword != "" {
		deployConfig[pulumiConfigKey(configNamespaceHarbor, "robotPassword")] = auto.ConfigValue{
			Value:  rt.Discovery.RobotPassword,
			Secret: true,
		}
	}

	// &Deployer should be in a stage struct
	// Change stage struct and stage specific structs so those
	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectCloudHarbor,
		StackName:   stackCloudHarbor,
		WorkDir:     harborDir,
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Logger:      rt.Logger.Writer(),
		Config:      deployConfig,
		Destroy:     false,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
	})
	if err != nil {
		return err
	}

	// harborDir is passed explicitly: the program runs as an inline source, so
	// it inherits the installer's working directory rather than WorkDir, and
	// the relative chartPath from Pulumi.harbor.yaml has to be anchored here.
	_, err = deployer.Deploy(context.Background(), func(ctx *pulumi.Context) error {
		return harbor.DeployHarbor(ctx, harborDir)
	})
	if err != nil {
		return err
	}

	return nil
}
