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
		"axem.ai/platform":          pulumi.String("ai-platform"),
		"axem.ai/model-name":        pulumi.String(m.ModelName),
		"axem.ai/model-slug":        pulumi.String(m.Slug),
		"axem.ai/model-category":    pulumi.String(m.Category()),
	}
	if ng := m.NodeSelector["nodegroup"]; ng != "" {
		labels["axem.ai/nodegroup"] = pulumi.String(ng)
	}
	return labels
}
