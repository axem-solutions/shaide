package stacks

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// clusterDefaultStorageClassLabel is the option label shown when the user wants
// to leave model PVCs without an explicit storageClass so Kubernetes uses the
// cluster default. Resolved to the empty string before injection.
const clusterDefaultStorageClassLabel = "(cluster default)"

// Deployment modes offered for the app-serving stack. The labels are shown
// verbatim in the picker, so they spell out the consequence of each choice.
const (
	servingModeUpdate   = "Update — keep model volumes"
	servingModeRecreate = "Recreate — destroy the stack, deleting model volumes"
)

func DeployAppServing(rt *core.Runtime) error {
	workdir := rt.Bootstrap.Bundle.PulumiWorkDir
	appServingDir := filepath.Join(workdir, projectAppServing)

	deployConfig := auto.ConfigMap{
		pulumiConfigKey(projectAppServing, "kubeconfig"): {
			Value: rt.Cluster.ConfigPath,
		},
	}

	if rt.Discovery.Auth.Password != "" {
		deployConfig[pulumiConfigKey(projectAppServing, "harborToken")] = auto.ConfigValue{
			Value:  rt.Discovery.Auth.Password,
			Secret: true,
		}
	}

	// Derive serving:cloudProvider from the platform picked at the gateway-provider
	// stage. The serving program only distinguishes "on-prem" (hostpath PVs) from
	// "cloud" (cluster-default StorageClass), so collapse the four-value platform
	// pick accordingly.
	servingCloudProvider := "cloud"
	if rt.Bootstrap.CloudPlatform == cloudProviderOnPrem {
		servingCloudProvider = "on-prem"
	}
	deployConfig[pulumiConfigKey(projectAppServing, "cloudProvider")] = auto.ConfigValue{
		Value: servingCloudProvider,
	}

	storageClass, err := promptModelStorageClass(rt)
	if err != nil {
		return err
	}
	if storageClass != "" {
		deployConfig[pulumiConfigKey(projectAppServing, "modelStorageClass")] = auto.ConfigValue{
			Value: storageClass,
		}
	}

	// Recreating tears the stack down before deploying it again, which deletes
	// the model PersistentVolumeClaims along with it — the weights have to be
	// pulled from Harbor again afterwards. Updating leaves the volumes in place
	// and patches the running resources, so it is the default.
	mode, err := rt.Reporter.Select(
		"How should app-serving be deployed?",
		servingModeUpdate,
		[]string{servingModeUpdate, servingModeRecreate},
	)
	if err != nil {
		return err
	}

	destroy := mode == servingModeRecreate

	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectAppServing,
		StackName:   stackAppServing,
		WorkDir:     filepath.Join(workdir, projectAppServing),
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Logger:      rt.Logger.Writer(),
		Config:      deployConfig,
		Destroy:     destroy,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
	})
	if err != nil {
		return err
	}

	_, err = deployer.Deploy(context.Background(), func(ctx *pulumi.Context) error {
		return serving.DeployAppServing(ctx, appServingDir, rt.Detailf)
	})
	if err != nil {
		return err
	}
	return nil
}

// promptModelStorageClass asks the user to pick a StorageClass for model PVCs.
// Options are the cluster's actual StorageClasses plus a "cluster default" entry
// (which resolves to empty string - K8s uses the cluster default).
// The default selection is the cluster-default option. Returns the selected
// StorageClass name, or empty string for cluster default.
func promptModelStorageClass(rt *core.Runtime) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	scList, err := rt.Cluster.Client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		// If we can't list (RBAC, transient), fall back to a free-form prompt.
		rt.Detailf("could not list cluster StorageClasses (%v); prompting free-form", err)
		value, err := rt.Reporter.Input(
			"StorageClass for model PVCs (leave empty for cluster default)",
			"", "",
		)
		if err != nil {
			return "", err
		}
		return value, nil
	}

	options := []string{clusterDefaultStorageClassLabel}
	defaultLabel := clusterDefaultStorageClassLabel
	for _, sc := range scList.Items {
		label := sc.Name
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			label = fmt.Sprintf("%s (cluster default)", sc.Name)
			defaultLabel = label
		}
		options = append(options, label)
	}

	selected, err := rt.Reporter.Select(
		"StorageClass for model PVCs",
		defaultLabel,
		options,
	)
	if err != nil {
		return "", err
	}

	if selected == clusterDefaultStorageClassLabel {
		return "", nil
	}
	// Strip the "(cluster default)" suffix to recover the bare StorageClass name.
	name := selected
	if suffix := " (cluster default)"; len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
		name = name[:len(name)-len(suffix)]
	}
	return name, nil
}
