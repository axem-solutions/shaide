package stacks

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config"
	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func DeployAppShaide(rt *core.Runtime) error {
	workDir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppShaide)
	stateDir := rt.Bootstrap.Config.Paths.PulumiState

	deployConfig := auto.ConfigMap{
		pulumiConfigKey(projectAppShaide, "kubeconfig"): {
			Value: rt.Cluster.ConfigPath,
		},
	}

	// Reuse the gateway hostname captured during the gateway-provider stage so
	// shaide can route via the shared Gateway without a StackReference to an
	// external infra stack. Empty value falls back to the bundle stack file
	// default (e.g. on-prem flow where gateway-provider doesn't set it).
	if rt.Bootstrap.GatewayHostname != "" {
		deployConfig[pulumiConfigKey(projectAppShaide, "gatewayHostname")] = auto.ConfigValue{
			Value: rt.Bootstrap.GatewayHostname,
		}
	}

	// Derive shaide:cloudProvider from the platform picked at the gateway-provider
	// stage. The shaide program uses this to choose provider-specific resources
	// (e.g. GKE HealthCheckPolicy is only created when cloudProvider is "gcp").
	if rt.Bootstrap.CloudPlatform != "" {
		deployConfig[pulumiConfigKey(projectAppShaide, "cloudProvider")] = auto.ConfigValue{
			Value: rt.Bootstrap.CloudPlatform,
		}
	}

	for {
		value, err := rt.Reporter.Input("Shaide admin password", "", "admin")
		if err != nil {
			return err
		}
		if value != "" {
			rt.Pulumi.ShaideAdmiPassword = value
			break
		}
	}

	if rt.Pulumi.ShaideAdmiPassword != "" {
		deployConfig[pulumiConfigKey(projectAppShaide, "adminAuthKey")] = auto.ConfigValue{
			Value:  rt.Pulumi.ShaideAdmiPassword,
			Secret: true,
		}
		deployConfig[pulumiConfigKey(projectAppShaide, "s3Password")] = auto.ConfigValue{
			Value:  rt.Pulumi.ShaideAdmiPassword,
			Secret: true,
		}
	}

	ghcrToken, err := shaidePullCredential(rt, workDir)
	if err != nil {
		return err
	}

	deployConfig[pulumiConfigKey(projectAppShaide, "ghcrToken")] = auto.ConfigValue{
		Value:  ghcrToken,
		Secret: true,
	}

	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectAppShaide,
		StackName:   stackAppShaide,
		WorkDir:     workDir,
		StateDir:    stateDir,
		Logger:      rt.Logger.Writer(),
		Config:      deployConfig,
		Destroy:     false,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
	})
	if err != nil {
		return err
	}

	_, err = deployer.Deploy(context.Background(), shaide.DeployAppShaide)
	if err != nil {
		return err
	}
	return nil
}

// shaidePullCredential resolves the password for the shaide image pull secret.
//
// The secret authenticates against the registry CreateGHCRSecret targets: the
// internal Harbor when the stack sets harborHostname, ghcr.io otherwise. The
// credential must match that registry — a Harbor password sent to ghcr.io is
// rejected with 403, and because a dockerconfigjson entry makes containerd
// attempt basic auth rather than an anonymous pull, that breaks even public
// images.
//
// GHCR_TOKEN takes precedence in both cases so a cluster can always be pointed
// at explicit credentials.
func shaidePullCredential(rt *core.Runtime, projectDir string) (string, error) {
	if token := strings.TrimSpace(rt.Bootstrap.Config.Registries.GHCR.Password); token != "" {
		rt.Detailf("using GHCR_TOKEN env var as shaide image pull credential")
		return token, nil
	}

	stackFile := stackConfigFile(projectDir, stackAppShaide)
	harborHostname, err := stackConfigString(stackFile, pulumiConfigKey(projectAppShaide, "harborHostname"))
	if err != nil {
		return "", err
	}

	if harborHostname != "" {
		if rt.Discovery.Auth.Password == "" {
			return "", fmt.Errorf(
				"shaide images are served from Harbor (%s) but no Harbor credential is available",
				harborHostname,
			)
		}
		rt.Detailf("using Harbor robot password as shaide image pull credential")
		return rt.Discovery.Auth.Password, nil
	}

	return "", fmt.Errorf(
		"shaide images resolve at ghcr.io (%s sets no harborHostname), so a Harbor "+
			"credential cannot authenticate them: set %s (and %s) before running the installer",
		filepath.Base(stackFile), config.GHCRTokenEnv, config.GHCRUserEnv,
	)
}
