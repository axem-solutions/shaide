package stacks

import (
	"context"
	"fmt"
	"path/filepath"

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

// ServesModels reports whether anything is selected to serve on the cluster.
//
// An empty selection is a valid deployment, not a misconfiguration: a cluster
// with no GPU nodes that reaches third-party providers only needs the rest of
// the platform and no serving stack. The stack itself refuses to configure
// without at least one model ("models must be non-empty"), and that failure is
// unrecoverable, so the whole install used to stop here rather than skipping a
// stack it had nothing to put in.
//
// The model manifest stands in for the model selection UI: it lists the models
// the operator would have picked, so its being empty is the selection.
func ServesModels(rt *core.Runtime) bool {
	return len(rt.Bootstrap.Catalog.Models) > 0
}

func DeployAppServing(rt *core.Runtime) error {
	workDir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppServing)
	platform := cluster.Provider(rt.Bootstrap.Provider)

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

// selectedModels maps the model manifest onto the stack's model list. Only a
// model carrying a serving block is deployed: the rest are published to
// Harbor for another consumer to pull.
func selectedModels(rt *core.Runtime) []serving.Model {
	registry := harborRegistryHostname(rt)

	var models []serving.Model
	for _, model := range rt.Bootstrap.Catalog.Models {
		if model.Serving == nil {
			continue
		}

		models = append(models, serving.Model{
			Name: model.Serving.Name,
			ID:   model.ID,
			HarborRef: fmt.Sprintf(
				"%s/%s/%s:%s",
				registry, model.HarborProject, model.HarborName, model.HarborTag,
			),
			StorageSize:  model.Serving.StorageSize,
			StorageClass: model.Serving.StorageClass,
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
// class, which cannot take one. A class the manifest pins is left alone, and
// on-prem nothing is asked, since the stack puts every model volume on the
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
