// Package router deploys a model's llm-d router: the InferencePool and its
// endpoint picker (EPP), from the vendored llm-d-router-gateway chart. The
// model's Istio Gateway is the proxy; it asks the EPP which pod to send each
// request to.
package router

import (
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ModelServerPort is the port the InferencePool sends requests to.
const ModelServerPort = 8000

// pluginsConfigFile names the EPP configuration the platform renders.
const pluginsConfigFile = "shaide-plugins.yaml"

// DefaultPluginsConfig is llm-d v1.0's optimized baseline: prefix-cache
// affinity with a saturation override, and prefill token load balancing. Both
// plugins run with their built-in defaults. The affinity filter's
// peakPrefillThroughput default is calibrated for Qwen3-32B on H100 (TP=2);
// models on other hardware can set their own through router.pluginsConfig.
const DefaultPluginsConfig = `apiVersion: llm-d.ai/v1
kind: EndpointPickerConfig
plugins:
- type: approx-prefix-cache-producer
- type: inflight-load-producer
- type: prefix-cache-affinity-filter
- type: token-load-scorer
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: prefix-cache-affinity-filter
  - pluginRef: token-load-scorer
`

// defaultResources request what one EPP needs at modest load and fits a
// 4 vCPU node, which llm-d's base values (4 CPU, 8Gi) never do. CPU is not
// limited, so the EPP bursts under load; a model serving heavy traffic sets
// router.resources in its model.yaml.
func defaultResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
	}
}

// destinationRuleConnectionPool lifts Istio's default connection limits on
// the Gateway's ext-proc calls to the EPP. Every inference request holds one
// open for its whole duration, and streamed responses can last many minutes.
func destinationRuleConnectionPool() map[string]any {
	return map[string]any{
		"http": map[string]any{
			"http1MaxPendingRequests":  256000,
			"maxRequestsPerConnection": 256000,
			"http2MaxRequests":         256000,
			"idleTimeout":              "900s",
		},
		"tcp": map[string]any{
			"maxConnections":        256000,
			"maxConnectionDuration": "1800s",
			"connectTimeout":        "900s",
		},
	}
}

// Input is what the chart values derive from.
type Input struct {
	Definition modeldef.Definition

	// Image is the EPP image reference, name:tag.
	Image string

	// PoolSelector selects the model server pods.
	PoolSelector map[string]string

	// PullSecret names the image pull secret, or is empty.
	PullSecret string
}

// Values renders the llm-d-router-gateway chart values.
func Values(in Input) (map[string]any, error) {
	registry, repository, tag, err := splitImage(in.Image)
	if err != nil {
		return nil, fmt.Errorf("router image: %w", err)
	}

	if len(in.PoolSelector) == 0 {
		return nil, fmt.Errorf("the InferencePool needs a pod selector")
	}

	resources := defaultResources()
	if in.Definition.Router.Resources != nil {
		resources = *in.Definition.Router.Resources
	}

	pluginsConfig := DefaultPluginsConfig
	if custom := strings.TrimSpace(in.Definition.Router.PluginsConfig); custom != "" {
		pluginsConfig = custom + "\n"
	}

	matchLabels := map[string]any{}
	for key, value := range in.PoolSelector {
		matchLabels[key] = value
	}

	replicas := in.Definition.RouterReplicas()

	epp := map[string]any{
		"replicas": replicas,
		"image": map[string]any{
			"registry":   registry,
			"repository": repository,
			"tag":        tag,
			"pullPolicy": "IfNotPresent",
		},
		"flags": map[string]any{
			"v": 1,
		},
		"pluginsConfigFile": pluginsConfigFile,
		"pluginsCustomConfig": map[string]any{
			pluginsConfigFile: pluginsConfig,
		},
		"resources": resourcesValues(resources),
	}

	// More than one replica runs leader-elected: the standbys stay NotReady,
	// so the chart's default Recreate strategy never reports the Deployment
	// available and the release never finishes. A rolling update that may
	// leave all standbys unavailable lets it complete while keeping them out
	// of the Service.
	if replicas > 1 {
		epp["deploymentStrategy"] = map[string]any{
			"type": "RollingUpdate",
			"rollingUpdate": map[string]any{
				"maxUnavailable": replicas - 1,
				"maxSurge":       0,
			},
		}
	}

	router := map[string]any{
		"epp": epp,
		"inferencePool": map[string]any{
			// An unreachable EPP sends requests straight to a pod instead of
			// failing them: the model stays available, only unbalanced.
			"failureMode": "FailOpen",
		},
		"modelServers": map[string]any{
			"type":     "vllm",
			"protocol": "http",
			"targetPorts": []any{
				map[string]any{"number": ModelServerPort},
			},
			"matchLabels": matchLabels,
		},
	}

	if in.PullSecret != "" {
		router["imagePullSecrets"] = []any{map[string]any{"name": in.PullSecret}}
	}

	return map[string]any{
		"router": router,
		"provider": map[string]any{
			"name": "istio",
			"istio": map[string]any{
				"destinationRule": map[string]any{
					"trafficPolicy": map[string]any{
						"connectionPool": destinationRuleConnectionPool(),
					},
				},
			},
		},
		// The platform creates its own HTTPRoute, with the timeouts
		// streamed responses need.
		"httpRoute": map[string]any{
			"create": false,
		},
	}, nil
}

// splitImage splits registry/path/name:tag into the chart's registry,
// repository and tag. The chart joins them back as registry/repository:tag.
func splitImage(image string) (registry, repository, tag string, err error) {
	if strings.Contains(image, "@") {
		return "", "", "", fmt.Errorf("%q: digest references are not supported, use a tag", image)
	}

	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if slash <= 0 || colon < slash {
		return "", "", "", fmt.Errorf("%q: want registry/repository:tag", image)
	}

	registry, repository, tag = image[:slash], image[slash+1:colon], image[colon+1:]
	if repository == "" || tag == "" {
		return "", "", "", fmt.Errorf("%q: want registry/repository:tag", image)
	}

	return registry, repository, tag, nil
}

func resourcesValues(resources corev1.ResourceRequirements) map[string]any {
	values := map[string]any{}
	if list := resourceListValues(resources.Requests); len(list) > 0 {
		values["requests"] = list
	}
	if list := resourceListValues(resources.Limits); len(list) > 0 {
		values["limits"] = list
	}
	return values
}

func resourceListValues(list corev1.ResourceList) map[string]any {
	values := map[string]any{}
	for name, quantity := range list {
		values[string(name)] = quantity.String()
	}
	return values
}
