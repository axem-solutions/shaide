package router

import (
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func input() Input {
	return Input{
		Definition:   modeldef.Definition{Slug: "example"},
		Image:        "harbor.harbor.svc.cluster.local/services/llm-d/llm-d-router-endpoint-picker:v1.0.0",
		PoolSelector: map[string]string{"llm-d.ai/model": "model-example"},
	}
}

func epp(t *testing.T, values map[string]any) map[string]any {
	t.Helper()
	return values["router"].(map[string]any)["epp"].(map[string]any)
}

// The chart joins registry/repository:tag, so a mirrored reference must come
// back out unchanged, and its tag must be pinned: the chart defaults to main.
func TestValuesPinTheMirroredImage(t *testing.T) {
	values, err := Values(input())
	if err != nil {
		t.Fatal(err)
	}

	image := epp(t, values)["image"].(map[string]any)
	got := image["registry"].(string) + "/" + image["repository"].(string) + ":" + image["tag"].(string)
	if got != input().Image {
		t.Errorf("image = %q, want %q", got, input().Image)
	}
}

func TestValuesRejectUnpinnedImages(t *testing.T) {
	for _, image := range []string{"llm-d-router-endpoint-picker", "ghcr.io/llm-d/epp", "ghcr.io/llm-d/epp@sha256:abc"} {
		in := input()
		in.Image = image
		if _, err := Values(in); err == nil {
			t.Errorf("Values(%q) succeeded, want an error", image)
		}
	}
}

// The pool selects the model's pods on vLLM's port; an unreachable EPP must
// not take the model down.
func TestValuesTargetTheModelServers(t *testing.T) {
	values, err := Values(input())
	if err != nil {
		t.Fatal(err)
	}

	router := values["router"].(map[string]any)
	servers := router["modelServers"].(map[string]any)
	if servers["matchLabels"].(map[string]any)["llm-d.ai/model"] != "model-example" {
		t.Errorf("matchLabels = %v", servers["matchLabels"])
	}
	port := servers["targetPorts"].([]any)[0].(map[string]any)["number"]
	if port != ModelServerPort {
		t.Errorf("target port = %v, want %d", port, ModelServerPort)
	}
	if router["inferencePool"].(map[string]any)["failureMode"] != "FailOpen" {
		t.Error("failureMode is not FailOpen")
	}
	if values["httpRoute"].(map[string]any)["create"] != false {
		t.Error("the chart must not create an HTTPRoute; the platform owns it")
	}
	if values["provider"].(map[string]any)["name"] != "istio" {
		t.Error("provider is not istio")
	}
}

func TestValuesRenderTheBaselineConfig(t *testing.T) {
	values, err := Values(input())
	if err != nil {
		t.Fatal(err)
	}

	config := epp(t, values)["pluginsCustomConfig"].(map[string]any)[pluginsConfigFile].(string)
	if config != DefaultPluginsConfig || !strings.Contains(config, "apiVersion: llm-d.ai/v1\n") {
		t.Errorf("plugins config = %q", config)
	}
	if epp(t, values)["pluginsConfigFile"] != pluginsConfigFile {
		t.Error("pluginsConfigFile does not select the rendered config")
	}
}

func TestValuesUseAModelsOwnRouterSettings(t *testing.T) {
	replicas := int32(3)
	in := input()
	in.PullSecret = "harbor-creds"
	in.Definition.Router = modeldef.Router{
		Replicas:      &replicas,
		PluginsConfig: "apiVersion: llm-d.ai/v1\nkind: EndpointPickerConfig",
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
		},
	}

	values, err := Values(in)
	if err != nil {
		t.Fatal(err)
	}

	e := epp(t, values)
	if e["replicas"] != int32(3) {
		t.Errorf("replicas = %v", e["replicas"])
	}
	strategy := e["deploymentStrategy"].(map[string]any)
	if strategy["type"] != "RollingUpdate" || strategy["rollingUpdate"].(map[string]any)["maxUnavailable"] != int32(2) {
		t.Errorf("strategy = %v, want a rolling update tolerating the two standbys", strategy)
	}
	if e["resources"].(map[string]any)["requests"].(map[string]any)["cpu"] != "500m" {
		t.Errorf("resources = %v", e["resources"])
	}
	if !strings.HasSuffix(e["pluginsCustomConfig"].(map[string]any)[pluginsConfigFile].(string), "kind: EndpointPickerConfig\n") {
		t.Error("custom plugins config not used")
	}
	secrets := values["router"].(map[string]any)["imagePullSecrets"].([]any)
	if secrets[0].(map[string]any)["name"] != "harbor-creds" {
		t.Errorf("imagePullSecrets = %v", secrets)
	}
}

// One replica keeps the chart's Recreate strategy: there is no standby to
// hand over to.
func TestValuesKeepRecreateForOneReplica(t *testing.T) {
	values, err := Values(input())
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := epp(t, values)["deploymentStrategy"]; ok {
		t.Error("a single replica got a deployment strategy override")
	}
	if _, ok := values["router"].(map[string]any)["imagePullSecrets"]; ok {
		t.Error("imagePullSecrets set without a pull secret")
	}
}
