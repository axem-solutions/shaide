package config

import (
	"sort"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	k8scorev1 "k8s.io/api/core/v1"
)

// ClaimName is the PersistentVolumeClaim holding the model's weights.
func (m *Model) ClaimName() string {
	return modelClaim(m.Slug)
}

// ReleasePostFix names the model's resources: its slug, or a stack-specific
// suffix when the stack config sets releaseName (infra-<postfix>).
func (m *Model) ReleasePostFix() string {
	if m.ReleaseName == "" {
		return "sim"
	}
	return strings.TrimPrefix(m.ReleaseName, "infra-")
}

// RouterReleaseName is the llm-d-router-gateway Helm release. The chart names
// the InferencePool after it and the EPP Service "<release>-epp".
func (m *Model) RouterReleaseName() string {
	return "router-" + m.ReleasePostFix()
}

// RouterServiceName is the EPP Service the Gateway sends ext-proc calls to.
func (m *Model) RouterServiceName() string {
	return m.RouterReleaseName() + "-epp"
}

// InferencePoolName is the pool of model server pods the router picks from.
func (m *Model) InferencePoolName() string {
	return m.RouterReleaseName()
}

// GatewayName is the model's Istio Gateway.
func (m *Model) GatewayName() string {
	return "gateway-" + m.ReleasePostFix()
}

// GatewayServiceName is the Service Istio provisions for the Gateway: Istio
// names it "<gateway>-istio".
func (m *Model) GatewayServiceName() string {
	return m.GatewayName() + "-istio"
}

// ModelServerName is the model server Deployment. Its pods carry it as the
// llm-d.ai/model label, which the InferencePool and the embedding Service
// select on.
func (m *Model) ModelServerName() string {
	return "model-" + m.ReleasePostFix()
}

func (m *Model) HTTPRouteName() string {
	return "llm-d-" + m.ReleasePostFix()
}

func (m *Model) EmbeddingServiceName() string {
	return "ms-" + m.ReleasePostFix() + "-embeddings"
}

// LegacyModelServiceLabel is the llm-d.ai/model label the retired
// llm-d-modelservice chart put on its pods, used to find them for removal.
func (m *Model) LegacyModelServiceLabel() string {
	return "ms-" + m.ReleasePostFix() + "-llm-d-modelservice"
}

// ChatCompletionEndpoint is the URL a client reaches this model at, through
// the Istio Gateway.
func (m *Model) ChatCompletionEndpoint() string {
	return "http://" + m.GatewayServiceName() + "." + m.Namespace + ".svc.cluster.local/v1/chat/completions"
}

// EmbeddingURL is the URL a client reaches this embedder model at, through
// its embedding Service.
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

// NodeAffinity builds the model's hard (required) nodeAffinity for pod specs
// built from the Kubernetes API types, as the model server Deployment is.
// Returns nil when the model has no placement.
func (m Model) NodeAffinity() *k8scorev1.Affinity {
	terms := m.placementTerms()
	if len(terms) == 0 {
		return nil
	}

	nodeSelectorTerms := make([]k8scorev1.NodeSelectorTerm, 0, len(terms))
	for _, term := range terms {
		exprs := make([]k8scorev1.NodeSelectorRequirement, 0, len(term))
		for _, req := range term {
			exprs = append(exprs, k8scorev1.NodeSelectorRequirement{
				Key:      req.key,
				Operator: k8scorev1.NodeSelectorOpIn,
				Values:   append([]string(nil), req.values...),
			})
		}
		nodeSelectorTerms = append(nodeSelectorTerms, k8scorev1.NodeSelectorTerm{MatchExpressions: exprs})
	}

	return &k8scorev1.Affinity{
		NodeAffinity: &k8scorev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &k8scorev1.NodeSelector{
				NodeSelectorTerms: nodeSelectorTerms,
			},
		},
	}
}

// NodeAffinityArgs builds the same hard (required) nodeAffinity as *corev1.AffinityArgs, for
// components built from Pulumi's corev1.PodSpecArgs, such as the ORAS pull Job.
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

// MetaLabels identify the model's resources; shaide-server discovers models
// by them.
func (m Model) MetaLabels(category string) pulumi.StringMap {
	return pulumi.ToStringMap(m.MetaLabelValues(category))
}

// MetaLabelValues are the MetaLabels as plain strings, for objects built from
// the Kubernetes API types.
func (m Model) MetaLabelValues(category string) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/part-of": "app-serving",
		"axem.dev/platform":         "ai-platform",
		"axem.dev/model-name":       m.Name,
		"axem.dev/model-slug":       m.Slug,
		"axem.dev/model-category":   category,
	}
	if ng := m.NodeSelector["nodegroup"]; ng != "" {
		labels["axem.dev/nodegroup"] = ng
	}
	return labels
}
