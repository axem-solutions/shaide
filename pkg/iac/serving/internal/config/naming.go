package config

import (
	"sort"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// nodeAffinityMatchExpressions builds a sorted-by-key list of {key, In, [value]}
// matchExpressions from a nodeSelector-shaped map, so Pulumi diffs stay stable
// regardless of Go's randomized map iteration order.
func nodeAffinityMatchExpressions(selector map[string]string) []map[string]interface{} {
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	exprs := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		exprs = append(exprs, map[string]interface{}{
			"key":      k,
			"operator": "In",
			"values":   []string{selector[k]},
		})
	}
	return exprs
}

func (m *Model) ReleasePostFix() string {
	if m.ReleaseName == "" {
		return "sim"
	}
	return strings.TrimPrefix(m.ReleaseName, "infra-")
}

func (m *Model) InfraReleaseName() string {
	if m.ReleaseName == "" {
		return "infra-" + m.ReleasePostFix()
	}
	return m.ReleaseName
}

func (m *Model) GaieReleaseName() string {
	return "gaie-" + m.ReleasePostFix()
}

func (m *Model) ModelServiceReleaseName() string {
	return "ms-" + m.ReleasePostFix()
}

func (m *Model) GatewayName() string {
	return m.InfraReleaseName() + "-inference-gateway"
}

func (m *Model) HTTPRouteName() string {
	return "llm-d-" + m.ReleasePostFix()
}

func (m *Model) EmbeddingServiceName() string {
	return "ms-" + m.ReleasePostFix() + "-embeddings"
}

// ChatCompletionEndpoint is the URL a client reaches this model at, through
// the Istio Gateway. "-istio" is not app_serving's own naming choice — it's
// how Istio names the Service it auto-provisions for a Gateway API Gateway,
// confirmed against a real deployed cluster (kubectl get svc).
func (m *Model) ChatCompletionEndpoint() string {
	return "http://" + m.GatewayName() + "-istio." + m.Namespace + ".svc.cluster.local/v1/chat/completions"
}

// EmbeddingURL is the URL a client reaches this embedder model at, direct
// (no Gateway/GAIE — embedding requests bypass that path entirely).
func (m *Model) EmbeddingURL() string {
	return "http://" + m.EmbeddingServiceName() + "." + m.Namespace + ".svc.cluster.local:8200/v1/embeddings"
}

func (m Model) NodeSelectorMap() pulumi.Map {
	out := pulumi.Map{}
	for key, value := range m.NodeSelector {
		out[key] = pulumi.String(value)
	}
	return out
}

func (m Model) NodeSelectorStringMap() pulumi.StringMap {
	out := pulumi.StringMap{}
	for key, value := range m.NodeSelector {
		out[key] = pulumi.String(value)
	}
	return out
}

// NodeAffinityMap builds a hard (required) nodeAffinity as a raw pulumi.Map, for chart values
// that accept an arbitrary pod-spec passthrough (llm-d-modelservice's extraConfig).
// Renders to the same requiredDuringSchedulingIgnoredDuringExecution shape used everywhere
// else in this design. Returns an empty map when NodeSelector is empty, matching
// NodeSelectorMap's behavior of contributing nothing to the pod spec.
func (m Model) NodeAffinityMap() pulumi.Map {
	if len(m.NodeSelector) == 0 {
		return pulumi.Map{}
	}
	return pulumi.Map{
		"nodeAffinity": pulumi.Map{
			"requiredDuringSchedulingIgnoredDuringExecution": pulumi.Map{
				"nodeSelectorTerms": pulumi.Array{
					pulumi.Map{
						"matchExpressions": pulumi.Any(nodeAffinityMatchExpressions(m.NodeSelector)),
					},
				},
			},
		},
	}
}

// NodeAffinityArgs builds the same hard (required) nodeAffinity as *corev1.AffinityArgs, for
// components using typed corev1.PodSpecArgs directly instead of raw Helm values.
func (m Model) NodeAffinityArgs() *corev1.AffinityArgs {
	if len(m.NodeSelector) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m.NodeSelector))
	for k := range m.NodeSelector {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	exprs := make(corev1.NodeSelectorRequirementArray, 0, len(keys))
	for _, k := range keys {
		exprs = append(exprs, &corev1.NodeSelectorRequirementArgs{
			Key:      pulumi.String(k),
			Operator: pulumi.String("In"),
			Values:   pulumi.StringArray{pulumi.String(m.NodeSelector[k])},
		})
	}

	return &corev1.AffinityArgs{
		NodeAffinity: &corev1.NodeAffinityArgs{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelectorArgs{
				NodeSelectorTerms: corev1.NodeSelectorTermArray{
					&corev1.NodeSelectorTermArgs{
						MatchExpressions: exprs,
					},
				},
			},
		},
	}
}

func (m Model) Category() string {
	if m.IsEmbedder {
		return "embedder"
	}
	return "generative"
}

func (m Model) MetaLabels() pulumi.StringMap {
	labels := pulumi.StringMap{
		"app.kubernetes.io/part-of": pulumi.String("app-serving"),
		"axem.dev/platform":         pulumi.String("ai-platform"),
		"axem.dev/model-name":       pulumi.String(m.ModelName),
		"axem.dev/model-slug":       pulumi.String(m.Slug),
		"axem.dev/model-category":   pulumi.String(m.Category()),
	}
	if ng := m.NodeSelector["nodegroup"]; ng != "" {
		labels["axem.dev/nodegroup"] = pulumi.String(ng)
	}
	return labels
}
