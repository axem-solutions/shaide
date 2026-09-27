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
	err := ensureIstioChartOwnership(context.Background(), nil, "istio-system", nil, nil)
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
		nil,
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
		nil,
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
		nil,
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

// TestOwnOrDestroyKeepsPulumiManagedResource covers the case that wiped Istio
// from a cluster: a resource created by a previous run of this stack carries a
// Pulumi field manager but no meta.helm.sh annotations, so the ownership sweep
// force-destroyed it while Pulumi - having refreshed beforehand - reported the
// stack unchanged and never recreated it.
func TestOwnOrDestroyKeepsPulumiManagedResource(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	object := testUnstructured("apiextensions.k8s.io/v1", "CustomResourceDefinition", "", "destinationrules.networking.istio.io")
	object.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "pulumi-resource-kubernetes", Operation: metav1.ManagedFieldsOperationApply},
	})
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)

	err := ownOrDestroy(
		context.Background(),
		nilSafeLogf,
		client,
		"istio-base",
		"istio-system",
		chartResource{
			gvr:          gvr,
			name:         "destinationrules.networking.istio.io",
			forceDestroy: true,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("ownOrDestroy() error = %v", err)
	}

	if _, err := client.Resource(gvr).Get(
		context.Background(),
		"destinationrules.networking.istio.io",
		metav1.GetOptions{},
	); err != nil {
		t.Fatalf("Pulumi-managed resource was not kept: %v", err)
	}
}

func TestManagedByPulumi(t *testing.T) {
	tests := []struct {
		name     string
		managers []string
		want     bool
	}{
		{name: "no managed fields", managers: nil, want: false},
		{name: "foreign helm install", managers: []string{"helm", "kubectl-client-side-apply"}, want: false},
		{name: "pulumi provider", managers: []string{"pulumi-resource-kubernetes"}, want: true},
		{name: "pulumi alongside controllers", managers: []string{"kube-controller-manager", "pulumi-resource-kubernetes"}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object := testUnstructured("v1", "ConfigMap", "istio-system", "istio")
			entries := make([]metav1.ManagedFieldsEntry, 0, len(test.managers))
			for _, manager := range test.managers {
				entries = append(entries, metav1.ManagedFieldsEntry{Manager: manager})
			}
			object.SetManagedFields(entries)

			if got := managedByPulumi(object); got != test.want {
				t.Errorf("managedByPulumi(%v) = %v, want %v", test.managers, got, test.want)
			}
		})
	}
}

func pulumiWrittenCRD(name string) *unstructured.Unstructured {
	object := testUnstructured("apiextensions.k8s.io/v1", "CustomResourceDefinition", "", name)
	object.SetManagedFields([]metav1.ManagedFieldsEntry{
		{Manager: "pulumi-kubernetes", Operation: metav1.ManagedFieldsOperationUpdate},
	})
	return object
}

// The trial-westeurope case: Istio written by an earlier Pulumi deployment
// that is not in this stack's state. The object is foreign to this stack, so
// the forced policy deletes it and the chart recreates it, instead of the
// chart failing with "already exists".
func TestOwnOrDestroyDestroysResourceFromAnotherPulumiStack(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	name := "virtualservices.networking.istio.io"
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), pulumiWrittenCRD(name))

	err := ownOrDestroy(context.Background(), nilSafeLogf, client, "istio-base", "istio-system",
		chartResource{gvr: gvr, name: name, forceDestroy: true},
		OwnedObjects{}, // a fresh state owns nothing
	)
	if err != nil {
		t.Fatalf("ownOrDestroy() error = %v", err)
	}

	if _, err := client.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("resource from another Pulumi deployment was kept, get error = %v", err)
	}
}

// The dev-westeurope and dev-polandcentral case: this stack wrote the object,
// so it stays and Pulumi updates it. This is the regression the Pulumi check
// was added for.
func TestOwnOrDestroyKeepsResourceInThisStacksState(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	name := "virtualservices.networking.istio.io"
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), pulumiWrittenCRD(name))
	owned := OwnedObjects{{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition", Name: name}: {}}

	err := ownOrDestroy(context.Background(), nilSafeLogf, client, "istio-base", "istio-system",
		chartResource{gvr: gvr, name: name, forceDestroy: true},
		owned,
	)
	if err != nil {
		t.Fatalf("ownOrDestroy() error = %v", err)
	}

	if _, err := client.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{}); err != nil {
		t.Fatalf("resource in this stack's state was not kept: %v", err)
	}
}
