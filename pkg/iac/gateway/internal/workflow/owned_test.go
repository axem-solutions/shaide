package workflow

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

func stateResource(typeToken, id string, deleted bool) apitype.ResourceV3 {
	return apitype.ResourceV3{Type: tokens.Type(typeToken), ID: resource.ID(id), Delete: deleted}
}

func TestOwnedObjectsFromState(t *testing.T) {
	owned := OwnedObjectsFromState([]apitype.ResourceV3{
		stateResource("kubernetes:apiextensions.k8s.io/v1:CustomResourceDefinition", "virtualservices.networking.istio.io", false),
		stateResource("kubernetes:core/v1:ServiceAccount", "istio-system/istiod", false),
		stateResource("kubernetes:apps/v1:Deployment", "istio-system/istiod", false),
		stateResource("kubernetes:admissionregistration.k8s.io/v1:ValidatingWebhookConfiguration", "istiod-default-validator", false),
		// Not objects, or no longer owned:
		stateResource("kubernetes:helm.sh/v4:Chart", "istio-base", false),
		stateResource("pulumi:providers:kubernetes", "ns-ready-provider", false),
		stateResource("pulumi:pulumi:Stack", "gateway-provider-gateway-provider", false),
		stateResource("kubernetes:core/v1:ConfigMap", "istio-system/values", true),
	})

	want := OwnedObjects{
		{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition", Name: "virtualservices.networking.istio.io"}:    {},
		{Group: "", Kind: "ServiceAccount", Namespace: "istio-system", Name: "istiod"}:                                    {},
		{Group: "apps", Kind: "Deployment", Namespace: "istio-system", Name: "istiod"}:                                    {},
		{Group: "admissionregistration.k8s.io", Kind: "ValidatingWebhookConfiguration", Name: "istiod-default-validator"}: {},
	}
	if len(owned) != len(want) {
		t.Fatalf("owned = %v, want %v", owned, want)
	}
	for key := range want {
		if _, ok := owned[key]; !ok {
			t.Errorf("missing %+v", key)
		}
	}
}

// A fresh state is not "unknown": it owns nothing, so every Pulumi-written
// object on the cluster is foreign to it.
func TestOwnedObjectsFromEmptyStateIsEmptyNotNil(t *testing.T) {
	if owned := OwnedObjectsFromState(nil); owned == nil || len(owned) != 0 {
		t.Errorf("OwnedObjectsFromState(nil) = %#v, want an empty non-nil set", owned)
	}
}
