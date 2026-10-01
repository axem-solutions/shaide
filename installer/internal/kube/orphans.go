package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// gcFinalizers are the finalizers the garbage collector puts on an object
// while it deletes the object's dependents. A pod has none, so dropping them
// lets nothing through that is still being waited for.
var gcFinalizers = map[string]bool{
	metav1.FinalizerDeleteDependents: true, // foregroundDeletion
	metav1.FinalizerOrphanDependents: true, // orphan
}

// OrphanedPod is a pod bound to a node that no longer exists.
type OrphanedPod struct {
	Name string
	Node string
}

// ReleaseOrphanedPods removes the pods of namespace that are bound to a node
// that no longer exists. Such a pod is never confirmed stopped, as no kubelet
// is left to do it, so it blocks whatever waits for it to go: a Deployment
// deleted in the foreground waits forever.
//
// Each is force-deleted, as `kubectl delete --force` does, and a
// garbage-collector finalizer still holding it is removed. A pod held by any
// other finalizer is left alone and returned as held.
func ReleaseOrphanedPods(ctx context.Context, client kubernetes.Interface, namespace string) (released, held []OrphanedPod, err error) {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("list pods in %s: %w", namespace, err)
	}

	nodeExists := map[string]bool{}

	for _, pod := range pods.Items {
		node := pod.Spec.NodeName
		if node == "" {
			continue
		}

		exists, checked := nodeExists[node]
		if !checked {
			_, err := client.CoreV1().Nodes().Get(ctx, node, metav1.GetOptions{})
			switch {
			case err == nil:
				exists = true
			case apierrors.IsNotFound(err):
				exists = false
			default:
				return released, held, fmt.Errorf("read node %s: %w", node, err)
			}
			nodeExists[node] = exists
		}
		if exists {
			continue
		}

		gone, err := releasePod(ctx, client, pod)
		if err != nil {
			return released, held, err
		}
		if gone {
			released = append(released, OrphanedPod{Name: pod.Name, Node: node})
		} else {
			held = append(held, OrphanedPod{Name: pod.Name, Node: node})
		}
	}

	return released, held, nil
}

// releasePod reports whether the pod is gone, or about to be once its
// garbage-collector finalizers are dropped.
func releasePod(ctx context.Context, client kubernetes.Interface, pod corev1.Pod) (bool, error) {
	pods := client.CoreV1().Pods(pod.Namespace)

	grace := int64(0)
	err := pods.Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	if err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("force-delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	current, err := pods.Get(ctx, pod.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	if len(current.Finalizers) == 0 {
		return true, nil
	}
	for _, finalizer := range current.Finalizers {
		if !gcFinalizers[finalizer] {
			return false, nil
		}
	}

	if _, err := pods.Patch(ctx, pod.Name, types.MergePatchType, []byte(`{"metadata":{"finalizers":null}}`), metav1.PatchOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("remove finalizers of pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	return true, nil
}
