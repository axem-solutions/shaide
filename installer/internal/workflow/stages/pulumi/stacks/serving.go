package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/placement"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"k8s.io/apimachinery/pkg/api/resource"
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

	if err := releaseOrphanedModelPods(rt, workDir); err != nil {
		return err
	}

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

	models, err = modelStorageSizes(rt, workDir, models, destroy)
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

// releaseOrphanedModelPods removes model pods bound to a node that no longer
// exists, such as one a node pool replaced while the pod was being deleted.
// Nothing confirms such a pod stopped, so a recreate or uninstall would wait
// for its Deployment to be deleted until it timed out. Every supported
// model's namespace is checked, as an installed model may have been dropped
// from the selection.
func releaseOrphanedModelPods(rt *core.Runtime, workDir string) error {
	models := make([]serving.Model, 0, len(rt.Bootstrap.Catalog.Models))
	for _, model := range rt.Bootstrap.Catalog.Models {
		models = append(models, serving.Model{Name: model.Name})
	}

	volumes, err := serving.ModelVolumes(workDir, models)
	if err != nil {
		return fmt.Errorf("name model namespaces: %w", err)
	}

	seen := map[string]bool{}
	for _, volume := range volumes {
		if seen[volume.Namespace] {
			continue
		}
		seen[volume.Namespace] = true

		released, held, err := kube.ReleaseOrphanedPods(context.Background(), rt.Cluster.Client, volume.Namespace)
		if err != nil {
			return err
		}
		for _, pod := range released {
			rt.Detailf("removed pod %s/%s: its node %s no longer exists", volume.Namespace, pod.Name, pod.Node)
		}
		for _, pod := range held {
			rt.Detailf("pod %s/%s is on node %s, which no longer exists, and is held by a finalizer the installer does not remove; a recreate or uninstall will wait for it", volume.Namespace, pod.Name, pod.Node)
		}
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

// servingPlacement places a model on the node pools assigned to it.
func servingPlacement(pools []placement.Pool) []serving.PlacementTerm {
	terms := placement.Terms(pools)
	if len(terms) == 0 {
		return nil
	}

	out := make([]serving.PlacementTerm, 0, len(terms))
	for _, term := range terms {
		out = append(out, serving.PlacementTerm{Key: term.Key, Values: term.Values})
	}

	return out
}

// selectedModels maps the models to serve onto the stack's model list. The
// Harbor reference is derived from where the artifact stage put each model,
// and each model runs on the node pools assigned to it.
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
			StorageSize: model.StorageSize,
			Placement:   servingPlacement(rt.Placement.Models[model.Slug]),
		})
	}

	return models
}

// modelStorageSizes keeps an existing model volume from shrinking. A PVC can
// grow but never shrink, so on an update a model whose volume is larger than
// its configured size keeps the volume's size; a larger configured size still
// grows it. Recreating deletes the volumes first, so the configured size
// applies. Volumes created from the retired model manifest could be larger
// than the size the model's values now ask for.
func modelStorageSizes(rt *core.Runtime, workDir string, models []serving.Model, destroy bool) ([]serving.Model, error) {
	if destroy {
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

	for i, model := range models {
		volume, deployed := volumeOf[model.Name]
		if !deployed {
			continue
		}

		current, found, err := existingPVCSize(rt, volume.Namespace, volume.Claim)
		if err != nil {
			return nil, err
		}
		if !found || current.IsZero() {
			continue
		}

		if model.StorageSize != "" {
			wanted, err := resource.ParseQuantity(model.StorageSize)
			if err != nil {
				return nil, fmt.Errorf("storage size %q of model %s: %w", model.StorageSize, model.Name, err)
			}
			if wanted.Cmp(current) >= 0 {
				continue
			}
		}

		rt.Detailf(
			"keeping size %s of existing PVC %s/%s: the model asks for %s, and a volume cannot shrink",
			current.String(), volume.Namespace, volume.Claim, displayStorageSize(model.StorageSize),
		)
		models[i].StorageSize = current.String()
	}

	return models, nil
}

func displayStorageSize(size string) string {
	if size == "" {
		return "the default size"
	}
	return size
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
