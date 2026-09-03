package workflow

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestEnsureIstioChartOwnershipRequiresRESTConfig(t *testing.T) {
	err := ensureIstioChartOwnership(context.Background(), nil, "istio-system", nil)
	if err == nil {
		t.Fatal("expected an error for a nil REST config")
	}
}

func TestOwnOrDestroyTakesOwnership(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	object := testUnstructured("v1", "ConfigMap", "istio-system", "istio")
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)

	err := ownOrDestroy(
		context.Background(),
		nilSafeLogf,
		client,
		"istiod",
		"istio-system",
		chartResource{gvr: gvr, name: "istio", namespace: "istio-system"},
	)
	if err != nil {
		t.Fatalf("ownOrDestroy() error = %v", err)
	}

	got, err := client.Resource(gvr).Namespace("istio-system").Get(
		context.Background(),
		"istio",
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("get adopted resource: %v", err)
	}

	if got.GetAnnotations()[helmReleaseNameAnnotation] != "istiod" {
		t.Errorf("release name annotation = %q, want %q", got.GetAnnotations()[helmReleaseNameAnnotation], "istiod")
	}
	if got.GetAnnotations()[helmReleaseNamespaceAnnotation] != "istio-system" {
		t.Errorf("release namespace annotation = %q, want %q", got.GetAnnotations()[helmReleaseNamespaceAnnotation], "istio-system")
	}
	if got.GetLabels()[helmManagedByLabel] != helmManagedByValue {
		t.Errorf("managed-by label = %q, want %q", got.GetLabels()[helmManagedByLabel], helmManagedByValue)
	}
}

func TestOwnOrDestroyForceDeletes(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	object := testUnstructured("v1", "ConfigMap", "istio-system", "istio")
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)

	err := ownOrDestroy(
		context.Background(),
		nilSafeLogf,
		client,
		"istiod",
		"istio-system",
		chartResource{
			gvr:          gvr,
			name:         "istio",
			namespace:    "istio-system",
			forceDestroy: true,
		},
	)
	if err != nil {
		t.Fatalf("ownOrDestroy() error = %v", err)
	}

	_, err = client.Resource(gvr).Namespace("istio-system").Get(
		context.Background(),
		"istio",
		metav1.GetOptions{},
	)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("get deleted resource error = %v, want NotFound", err)
	}
}

func TestOwnOrDestroyKeepsAlreadyOwnedForcedResource(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	object := testUnstructured("v1", "ConfigMap", "istio-system", "istio")
	object.SetAnnotations(map[string]string{
		helmReleaseNameAnnotation:      "istiod",
		helmReleaseNamespaceAnnotation: "istio-system",
	})
	object.SetLabels(map[string]string{helmManagedByLabel: helmManagedByValue})
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)

	err := ownOrDestroy(
		context.Background(),
		nilSafeLogf,
		client,
		"istiod",
		"istio-system",
		chartResource{
			gvr:          gvr,
			name:         "istio",
			namespace:    "istio-system",
			forceDestroy: true,
		},
	)
	if err != nil {
		t.Fatalf("ownOrDestroy() error = %v", err)
	}

	if _, err := client.Resource(gvr).Namespace("istio-system").Get(
		context.Background(),
		"istio",
		metav1.GetOptions{},
	); err != nil {
		t.Fatalf("already-owned resource was removed: %v", err)
	}
}

func testUnstructured(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}}
}

func nilSafeLogf(string, ...any) {}
