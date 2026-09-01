package config

import (
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

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
