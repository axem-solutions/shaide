// Package legacy clears what the retired llm-d-modelservice chart leaves in
// a model namespace before the llm-d v1.0 resources are deployed.
//
// Pulumi creates a model's new server Deployment before it deletes the old
// chart's, at the end of the update. On a GPU node the new pods would then
// wait for GPUs the old pods hold, and the update would time out. Deleting the
// old Deployments first frees the GPUs; Pulumi later finds them gone and drops
// them from its state.
package legacy

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	modelLabel   = "llm-d.ai/model"
	pollInterval = 5 * time.Second

	// The old pods run vLLM with the chart's default grace period, so they
	// are normally gone in well under this.
	removeTimeout = 10 * time.Minute
)

// Logf reports progress.
type Logf func(format string, args ...any)

// RemoveModelServers deletes the Deployments the retired chart created for a
// model and waits until their pods are gone. The chart put the model's
// llm-d.ai/model label only on the pods, not on the Deployments, so they are
// found by their pod template. It does nothing when there are none, so it is
// safe on every run.
func RemoveModelServers(ctx context.Context, client kubernetes.Interface, namespace, legacyLabel string, logf Logf) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	selector := labels.SelectorFromSet(labels.Set{modelLabel: legacyLabel}).String()

	all, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list Deployments in %s: %w", namespace, err)
	}

	var legacy []appsv1.Deployment
	for _, deployment := range all.Items {
		if deployment.Spec.Template.Labels[modelLabel] == legacyLabel {
			legacy = append(legacy, deployment)
		}
	}

	if len(legacy) == 0 {
		return nil
	}

	foreground := metav1.DeletePropagationForeground
	for _, deployment := range legacy {
		logf("removing legacy model server %s/%s to free its GPUs", namespace, deployment.Name)
		err := client.AppsV1().Deployments(namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{PropagationPolicy: &foreground})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete legacy Deployment %s/%s: %w", namespace, deployment.Name, err)
		}
	}

	return waitForPods(ctx, client, namespace, selector, legacy)
}

func waitForPods(ctx context.Context, client kubernetes.Interface, namespace, selector string, deleted []appsv1.Deployment) error {
	ctx, cancel := context.WithTimeout(ctx, removeTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return fmt.Errorf("list legacy model server pods in %s: %w", namespace, err)
		}

		if len(running(pods.Items)) == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("legacy model server pods in %s (%d Deployments) did not terminate within %s", namespace, len(deleted), removeTimeout)
		case <-ticker.C:
		}
	}
}

// running is the pods that may still hold a GPU.
func running(pods []corev1.Pod) []corev1.Pod {
	var result []corev1.Pod
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result = append(result, pod)
	}
	return result
}
