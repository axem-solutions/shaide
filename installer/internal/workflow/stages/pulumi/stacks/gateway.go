package stacks

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/iac/decoder"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployGatewayProvider(rt *core.Runtime) error {
	workdir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectGatewayProvider)
	stateDir := rt.Bootstrap.Config.Paths.PulumiState
	templateFile := filepath.Join(workdir, "Pulumi.yaml")
	stackFile := stackConfigFile(workdir, stackGatewayProvider)

	_, keys, err := decoder.LoadTemplateFile(templateFile)
	if err != nil {
		return err
	}

	// These values come from installer runtime discovery rather than user
	// input. Keep them in Pulumi.yaml so the project remains self-describing,
	// but do not prompt for them when the installer drives the deployment.
	runtimeConfig := auto.ConfigMap{
		pulumiConfigKey(projectGatewayProvider, "cloudProvider"): {
			Value: rt.Bootstrap.CloudPlatform,
		},
		pulumiConfigKey(projectGatewayProvider, "kubeconfig"): {
			Value: rt.Cluster.ConfigPath,
		},
		pulumiConfigKey(projectGatewayProvider, "context"): {
			Value: rt.Cluster.SelectedContext,
		},
	}

	deployConfig := auto.ConfigMap{}
	for _, key := range keys {
		if _, suppliedAtRuntime := runtimeConfig[key.Name]; suppliedAtRuntime {
			continue
		}

		existing, err := stackConfigString(stackFile, key.Name)
		if err != nil {
			return err
		}

		if existing != "" {
			key.Key.Default = existing
		}

		configValue, err := resolveConfigKey(rt.Reporter, key.Key)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", key.Name, err)
		}
		deployConfig[key.Name] = configValue
	}

	for key, value := range runtimeConfig {
		deployConfig[key] = value
	}

	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectGatewayProvider,
		StackName:   stackGatewayProvider,
		WorkDir:     workdir,
		StateDir:    stateDir,
		Logger:      rt.Logger.Writer(),
		Config:      deployConfig,
		Destroy:     false,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
	})
	if err != nil {
		return err
	}

	result, err := deployer.Deploy(context.Background(), func(ctx *pulumi.Context) error {
		return gateway.DeployGatewayProvider(ctx, workdir)
	})
	if err != nil {
		return err
	}

	// A StackReference can supply the hostname asynchronously. Prefer the
	// resolved Pulumi output when present so downstream projects receive the
	// same hostname as the shared Gateway.
	if output, ok := result.Outputs["gatewayHostname"]; ok {
		if hostname, ok := output.Value.(string); ok && strings.TrimSpace(hostname) != "" {
			rt.Bootstrap.GatewayHostname = strings.TrimSpace(hostname)
		}
	}

	return nil
}
