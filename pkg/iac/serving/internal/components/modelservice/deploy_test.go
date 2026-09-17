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

func TestWorkloadValuesUseRecreateStrategy(t *testing.T) {
	workload := workloadValues(pulumi.Map{})

	strategy, ok := workload["strategy"].(pulumi.Map)
	if !ok {
		t.Fatalf("strategy = %T, want pulumi.Map", workload["strategy"])
	}

	strategyType, ok := strategy["type"].(pulumi.String)
	if !ok {
		t.Fatalf("strategy.type = %T, want pulumi.String", strategy["type"])
	}
	if string(strategyType) != "Recreate" {
		t.Fatalf("strategy.type = %q, want Recreate", strategyType)
	}
}
