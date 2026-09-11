package modelservice

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestValuesDisableRoutingProxy(t *testing.T) {
	modelValues := values(pulumi.Map{}, pulumi.Map{})

	routing, ok := modelValues["routing"].(pulumi.Map)
	if !ok {
		t.Fatalf("routing = %T, want pulumi.Map", modelValues["routing"])
	}

	proxy, ok := routing["proxy"].(pulumi.Map)
	if !ok {
		t.Fatalf("routing.proxy = %T, want pulumi.Map", routing["proxy"])
	}

	enabled, ok := proxy["enabled"].(pulumi.Bool)
	if !ok {
		t.Fatalf("routing.proxy.enabled = %T, want pulumi.Bool", proxy["enabled"])
	}
	if bool(enabled) {
		t.Error("routing.proxy.enabled = true, want false")
	}
}
