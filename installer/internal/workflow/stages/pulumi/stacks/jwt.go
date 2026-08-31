package stacks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	shaideSecretsName = "shaide-secrets"
	jwtSecretKey      = "JWT_SECRET"
	jwtSecretBytes    = 32
)

// resolveShaideJWTSecret preserves the JWT signing key across installer
// reruns. A fresh or partially deployed namespace receives a new random key;
// an existing deployment reuses the live Secret value so active tokens are
// not invalidated by an update.
func resolveShaideJWTSecret(ctx context.Context, client kubernetes.Interface, namespace string) (string, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, shaideSecretsName, metav1.GetOptions{})
	if err == nil {
		if value := strings.TrimSpace(string(secret.Data[jwtSecretKey])); value != "" {
			return value, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("read existing Shaide JWT secret: %w", err)
	}

	raw := make([]byte, jwtSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate Shaide JWT secret: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}
