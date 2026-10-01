package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ReadConfigMapData returns a ConfigMap's data. A missing ConfigMap is not an
// error: found is false.
func ReadConfigMapData(ctx context.Context, client kubernetes.Interface, namespace, name string) (data map[string]string, found bool, err error) {
	configMap, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read ConfigMap %s/%s: %w", namespace, name, err)
	}

	return configMap.Data, true, nil
}

// WriteConfigMapData creates the ConfigMap, or replaces the data of an
// existing one.
func WriteConfigMapData(ctx context.Context, client kubernetes.Interface, namespace, name string, labels, data map[string]string) error {
	configMaps := client.CoreV1().ConfigMaps(namespace)

	existing, err := configMaps.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = configMaps.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
			Data:       data,
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ConfigMap %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read ConfigMap %s/%s: %w", namespace, name, err)
	}

	existing.Data = data
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for key, value := range labels {
		existing.Labels[key] = value
	}

	if _, err := configMaps.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update ConfigMap %s/%s: %w", namespace, name, err)
	}

	return nil
}
