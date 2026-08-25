package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ReadSecretKey(ctx context.Context, client kubernetes.Interface, namespace, secretName, secretKey string) ([]byte, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, secretName, err)
	}

	value, ok := secret.Data[secretKey]
	if !ok || len(value) == 0 {
		return nil, fmt.Errorf("secret %s/%s missing key %q", namespace, secretName, secretKey)
	}

	return value, nil
}
