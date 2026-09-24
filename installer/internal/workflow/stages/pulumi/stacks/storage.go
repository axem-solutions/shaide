package stacks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// clusterDefaultStorageClassLabel is the option label shown when the user wants
// to leave PVCs without an explicit storageClass so Kubernetes uses the cluster
// default. Resolved to the empty string before injection.
const clusterDefaultStorageClassLabel = "(cluster default)"

// defaultClassSuffix marks the cluster's default StorageClass in the picker.
const defaultClassSuffix = " (cluster default)"

// promptStorageClass asks the user to pick a StorageClass for a stack's PVCs.
// Options are the cluster's actual StorageClasses plus a "cluster default"
// entry, which resolves to the empty string so Kubernetes applies the cluster
// default. preferred pre-selects a class when it exists on the cluster;
// otherwise the cluster default is pre-selected. Returns the selected
// StorageClass name, or the empty string for the cluster default.
func promptStorageClass(rt *core.Runtime, title, preferred string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	scList, err := rt.Cluster.Client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		// If we can't list (RBAC, transient), fall back to a free-form prompt.
		rt.Detailf("could not list cluster StorageClasses (%v); prompting free-form", err)
		value, err := rt.Reporter.Input(
			title+" (leave empty for cluster default)",
			"", preferred,
		)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(value), nil
	}

	options := []string{clusterDefaultStorageClassLabel}
	current := clusterDefaultStorageClassLabel
	preferredFound := false
	for _, sc := range scList.Items {
		label := sc.Name
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			label = sc.Name + defaultClassSuffix
			if !preferredFound {
				current = label
			}
		}
		if preferred != "" && sc.Name == preferred {
			current = label
			preferredFound = true
		}
		options = append(options, label)
	}

	selected, err := rt.Reporter.Select(title, current, options)
	if err != nil {
		return "", err
	}

	if selected == clusterDefaultStorageClassLabel {
		return "", nil
	}
	// Strip the "(cluster default)" suffix to recover the bare StorageClass name.
	return strings.TrimSuffix(selected, defaultClassSuffix), nil
}

// existingPVCStorageClass reports the StorageClass of a PVC that is already on
// the cluster. A PVC's class cannot change, so an update has to keep it rather
// than ask again. found is false when the PVC does not exist.
func existingPVCStorageClass(rt *core.Runtime, namespace, name string) (class string, found bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pvc, err := rt.Cluster.Client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read PVC %s/%s: %w", namespace, name, err)
	}

	if pvc.Spec.StorageClassName == nil {
		return "", true, nil
	}
	return *pvc.Spec.StorageClassName, true, nil
}
