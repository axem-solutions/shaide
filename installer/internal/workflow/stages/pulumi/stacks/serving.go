package stacks

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
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
	workDir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppServing)

	storageClass, err := promptModelStorageClass(rt)
	if err != nil {
		return err
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

	servingStack := serving.NewStack(
		workDir,
		stackpkg.Options{
			Platform:   platform.Platform(rt.Bootstrap.CloudPlatform),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		serving.Options{
			HarborUser:        rt.Discovery.Auth.Username,
			HarborToken:       rt.Discovery.Auth.Password,
			ModelStorageClass: storageClass,
			Logf:              rt.Detailf,
		},
	)

	deployer, err := newStackDeployer(rt, servingStack, stackDeploymentOptions{
		Destroy: destroy,
	})
	if err != nil {
		return err
	}

	_, err = deployer.Deploy(context.Background())
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
