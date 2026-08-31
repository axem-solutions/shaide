package stacks

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestResolveShaideJWTSecretReusesExistingValue(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: shaideSecretsName, Namespace: "app-shaide"},
		Data: map[string][]byte{
			jwtSecretKey: []byte("existing-jwt-secret"),
		},
	})

	got, err := resolveShaideJWTSecret(context.Background(), client, "app-shaide")
	if err != nil {
		t.Fatalf("resolveShaideJWTSecret() error = %v", err)
	}
	if got != "existing-jwt-secret" {
		t.Fatalf("resolveShaideJWTSecret() = %q, want existing value", got)
	}
}

func TestResolveShaideJWTSecretGeneratesStrongValueWhenMissing(t *testing.T) {
	client := fake.NewSimpleClientset()

	got, err := resolveShaideJWTSecret(context.Background(), client, "app-shaide")
	if err != nil {
		t.Fatalf("resolveShaideJWTSecret() error = %v", err)
	}

	raw, err := base64.RawURLEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("generated secret is not raw URL-safe base64: %v", err)
	}
	if len(raw) != jwtSecretBytes {
		t.Fatalf("generated secret contains %d bytes, want %d", len(raw), jwtSecretBytes)
	}
}

func TestResolveShaideJWTSecretDoesNotRotateOnReadFailure(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Resource: "secrets"},
			shaideSecretsName,
			errors.New("denied"),
		)
	})

	if _, err := resolveShaideJWTSecret(context.Background(), client, "app-shaide"); err == nil {
		t.Fatal("resolveShaideJWTSecret() error = nil, want read failure")
	}
}
