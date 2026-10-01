package config

import (
	"sort"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ClaimName is the PersistentVolumeClaim holding the model's weights.
func (m *Model) ClaimName() string {
	return modelClaim(m.Slug)
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

// placementTerms is the model's placement as nodeSelectorTerms: one per
// Placement term, each matching its key against any of its values, or a
// single term ANDing the NodeSelector entries. Terms are alternatives, so a
// model placed on pools named by different labels can match any of them.
func (m Model) placementTerms() [][]nodeRequirement {
	if len(m.Placement) > 0 {
		terms := make([][]nodeRequirement, 0, len(m.Placement))
		for _, term := range m.Placement {
			values := append([]string(nil), term.Values...)
			sort.Strings(values)
			terms = append(terms, []nodeRequirement{{key: term.Key, values: values}})
		}
		return terms
	}

	if len(m.NodeSelector) == 0 {
		return nil
	}

	keys := make([]string, 0, len(m.NodeSelector))
	for k := range m.NodeSelector {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	term := make([]nodeRequirement, 0, len(keys))
	for _, k := range keys {
		term = append(term, nodeRequirement{key: k, values: []string{m.NodeSelector[k]}})
	}
	return [][]nodeRequirement{term}
}

type nodeRequirement struct {
	key    string
	values []string
}

// NodeAffinityMap builds a hard (required) nodeAffinity as a raw pulumi.Map, for chart values
// that accept an arbitrary pod-spec passthrough (llm-d-modelservice's extraConfig).
// Renders to the same requiredDuringSchedulingIgnoredDuringExecution shape used everywhere
// else in this design. Returns an empty map when the model has no placement, contributing
// nothing to the pod spec.
func (m Model) NodeAffinityMap() pulumi.Map {
	terms := m.placementTerms()
	if len(terms) == 0 {
		return pulumi.Map{}
	}

	nodeSelectorTerms := make(pulumi.Array, 0, len(terms))
	for _, term := range terms {
		exprs := make([]map[string]interface{}, 0, len(term))
		for _, req := range term {
			exprs = append(exprs, map[string]interface{}{
				"key":      req.key,
				"operator": "In",
				"values":   req.values,
			})
		}
		nodeSelectorTerms = append(nodeSelectorTerms, pulumi.Map{
			"matchExpressions": pulumi.Any(exprs),
		})
	}

	return pulumi.Map{
		"nodeAffinity": pulumi.Map{
			"requiredDuringSchedulingIgnoredDuringExecution": pulumi.Map{
				"nodeSelectorTerms": nodeSelectorTerms,
			},
		},
	}
}

// NodeAffinityArgs builds the same hard (required) nodeAffinity as *corev1.AffinityArgs, for
// components using typed corev1.PodSpecArgs directly instead of raw Helm values.
func (m Model) NodeAffinityArgs() *corev1.AffinityArgs {
	terms := m.placementTerms()
	if len(terms) == 0 {
		return nil
	}

	nodeSelectorTerms := make(corev1.NodeSelectorTermArray, 0, len(terms))
	for _, term := range terms {
		exprs := make(corev1.NodeSelectorRequirementArray, 0, len(term))
		for _, req := range term {
			exprs = append(exprs, &corev1.NodeSelectorRequirementArgs{
				Key:      pulumi.String(req.key),
				Operator: pulumi.String("In"),
				Values:   pulumi.ToStringArray(req.values),
			})
		}
		nodeSelectorTerms = append(nodeSelectorTerms, &corev1.NodeSelectorTermArgs{
			MatchExpressions: exprs,
		})
	}

	return &corev1.AffinityArgs{
		NodeAffinity: &corev1.NodeAffinityArgs{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelectorArgs{
				NodeSelectorTerms: nodeSelectorTerms,
			},
		},
	}
}

func (m Model) MetaLabels(category string) pulumi.StringMap {
	labels := pulumi.StringMap{
		"app.kubernetes.io/part-of": pulumi.String("app-serving"),
		"axem.dev/platform":         pulumi.String("ai-platform"),
		"axem.dev/model-name":       pulumi.String(m.Name),
		"axem.dev/model-slug":       pulumi.String(m.Slug),
		"axem.dev/model-category":   pulumi.String(category),
	}
	if ng := m.NodeSelector["nodegroup"]; ng != "" {
		labels["axem.dev/nodegroup"] = pulumi.String(ng)
	}
	return labels
}
