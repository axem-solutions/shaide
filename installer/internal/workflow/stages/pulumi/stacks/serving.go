package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/placement"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

// Deployment modes offered for the app-serving stack. The labels are shown
// verbatim in the picker, so they spell out the consequence of each choice.
const (
	servingModeUpdate   = "Update — keep model volumes"
	servingModeRecreate = "Recreate — destroy the stack, deleting model volumes"
)

// ServesModels reports whether app-serving has anything to do: models to
// serve, or installed models to remove.
//
// An empty selection is a valid deployment, not a misconfiguration: a cluster
// with no GPU nodes that reaches third-party providers only needs the rest of
// the platform and no serving stack.
func ServesModels(rt *core.Runtime) bool {
	return len(rt.Models.Serve) > 0 || len(rt.Models.Uninstall) > 0
}

func DeployAppServing(rt *core.Runtime) error {
	workDir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppServing)
	platform := cluster.Provider(rt.Bootstrap.Provider)

	// The stack refuses an empty model list, so uninstalling the last model
	// tears the stack down instead of updating it.
	if len(rt.Models.Serve) == 0 {
		return destroyAppServing(rt, workDir, platform)
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

	models, err := modelStorageClasses(rt, workDir, platform, selectedModels(rt), destroy)
	if err != nil {
		return err
	}

	registry := harborRegistryHostname(rt)
	servingStack := serving.NewStack(
		workDir,
		stackpkg.Options{
			Platform:   platform,
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		serving.Options{
			Models:         models,
			Images:         mirroredImages(rt.Bootstrap.Catalog.ServiceImages, registry),
			HarborHostname: registry,
			HarborUser:     rt.Discovery.Auth.Username,
			HarborToken:    rt.Discovery.Auth.Password,
			Logf:           rt.Detailf,
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

func destroyAppServing(rt *core.Runtime, workDir string, platform cluster.Provider) error {
	servingStack := serving.NewStack(
		workDir,
		stackpkg.Options{
			Platform:   platform,
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		serving.Options{Logf: rt.Detailf},
	)
	config := servingStack.Config()

	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: config.ProjectName(),
		StackName:   config.StackName(),
		WorkDir:     config.ProjectDir(),
		StateDir:    rt.Bootstrap.Config.Paths.PulumiState,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
		Logger:      rt.Logger.Writer(),
		Confirmer:   rt.Reporter,
		Target: &iac.ClusterTarget{
			KubeconfigPath: rt.Cluster.ConfigPath,
			Context:        rt.Cluster.SelectedContext,
		},
	})
	if err != nil {
		return err
	}

	rt.Detailf("no models left to serve; destroying app-serving")
	return deployer.DestroyOnly(context.Background(), servingStack.Deploy)
}

// selectedModels maps the models to serve onto the stack's model list. The
// Harbor reference is derived from where the artifact stage put each model,
// and each model runs on the pool the node assignment stage labelled for it.
func selectedModels(rt *core.Runtime) []serving.Model {
	registry := harborRegistryHostname(rt)

	models := make([]serving.Model, 0, len(rt.Models.Serve))
	for _, model := range rt.Models.Serve {
		models = append(models, serving.Model{
			Name: model.Name,
			ID:   model.ID,
			HarborRef: fmt.Sprintf(
				"%s/%s/%s:%s",
				registry, model.HarborProject, model.HarborName, model.HarborTag,
			),
			StorageSize:  model.StorageSize,
			NodeSelector: placement.ModelSelector(model),
		})
	}

	return models
}

// modelStorageClasses settles the StorageClass of each model's volume.
//
// A PVC's class cannot change, so on an update a model whose volume exists
// keeps its class and is not asked about. The operator is asked once, for the
// models without a volume, or for all of them when recreating, with the class
// already in use pre-selected. The answer is set on those models only: as the
// stack-wide default it would also reach an existing volume created without a
// class, which cannot take one. A class already set on a model is left alone,
// and on-prem nothing is asked, since the stack puts every model volume on the
// hostpath class there.
func modelStorageClasses(
	rt *core.Runtime,
	workDir string,
	platform cluster.Provider,
	models []serving.Model,
	destroy bool,
) ([]serving.Model, error) {
	if platform == cluster.OnPrem {
		return models, nil
	}

	volumes, err := serving.ModelVolumes(workDir, models)
	if err != nil {
		return nil, fmt.Errorf("name model volumes: %w", err)
	}
	volumeOf := make(map[string]serving.ModelVolume, len(volumes))
	for _, volume := range volumes {
		volumeOf[volume.Model] = volume
	}

	var unsettled []int
	preferred := ""
	for i, model := range models {
		volume, deployed := volumeOf[model.Name]
		if model.StorageClass != "" || !deployed {
			continue
		}

		class, found, err := existingPVCStorageClass(rt, volume.Namespace, volume.Claim)
		if err != nil {
			return nil, err
		}
		if found {
			preferred = class
		}
		if found && !destroy {
			rt.Detailf("keeping StorageClass %q of existing PVC %s/%s", displayStorageClass(class), volume.Namespace, volume.Claim)
			models[i].StorageClass = class
			continue
		}

		unsettled = append(unsettled, i)
	}

	if len(unsettled) == 0 {
		return models, nil
	}

	selected, err := promptStorageClass(rt, "StorageClass for model PVCs", preferred)
	if err != nil {
		return nil, err
	}
	for _, i := range unsettled {
		models[i].StorageClass = selected
	}

	return models, nil
}
