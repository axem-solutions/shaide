package stacks

import (
	"context"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func DeployMonitoring(rt *core.Runtime) error {
	storage, err := monitoringStorageClasses(rt)
	if err != nil {
		return err
	}

	monitoringStack := monitoring.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectMonitoring),
		stack.Options{
			Platform:   cluster.Provider(rt.Bootstrap.Provider),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		storage,
	)

	deployer, err := newStackDeployer(rt, monitoringStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	_, err = deployer.Deploy(context.Background())
	if err != nil {
		return err
	}
	return nil
}

// monitoringStorageClasses resolves the StorageClass for the Loki and the
// Prometheus PVC.
//
// A PVC that already exists keeps its class: storageClassName is immutable, so
// a different answer could only fail the upgrade. The operator is asked once,
// and only for the volumes that do not exist yet, with the class already in use
// pre-selected so both volumes stay on the same class by default.
func monitoringStorageClasses(rt *core.Runtime) (monitoring.Options, error) {
	lokiClass, lokiFound, err := existingPVCStorageClass(rt, monitoring.Namespace, monitoring.LokiPVCName)
	if err != nil {
		return monitoring.Options{}, err
	}
	promClass, promFound, err := existingPVCStorageClass(rt, monitoring.Namespace, monitoring.PrometheusPVCName)
	if err != nil {
		return monitoring.Options{}, err
	}

	if lokiFound {
		rt.Detailf("keeping StorageClass %q of existing PVC %s", displayStorageClass(lokiClass), monitoring.LokiPVCName)
	}
	if promFound {
		rt.Detailf("keeping StorageClass %q of existing PVC %s", displayStorageClass(promClass), monitoring.PrometheusPVCName)
	}

	options := monitoring.Options{
		LokiStorageClass:       lokiClass,
		PrometheusStorageClass: promClass,
	}
	if lokiFound && promFound {
		return options, nil
	}

	preferred := lokiClass
	if !lokiFound {
		preferred = promClass
	}

	selected, err := promptStorageClass(rt, "StorageClass for monitoring PVCs (Loki, Prometheus)", preferred)
	if err != nil {
		return monitoring.Options{}, err
	}

	if !lokiFound {
		options.LokiStorageClass = selected
	}
	if !promFound {
		options.PrometheusStorageClass = selected
	}
	return options, nil
}

func displayStorageClass(class string) string {
	if class == "" {
		return clusterDefaultStorageClassLabel
	}
	return class
}
