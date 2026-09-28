// Package placement names the node labels that decide where shaide's
// workloads run, so the installer that sets them and the stacks that match on
// them cannot drift apart.
//
// Every label is a boolean key set to "true": a node carries one per role, and
// a node that serves two roles (a GPU split between two models) carries both.
package placement

import (
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
)

const (
	// Prefix is the domain of every label the installer manages. The node
	// assignment step removes managed labels it did not choose, so nothing
	// else may use these prefixes.
	Prefix = "axem.dev/"

	workloadPrefix = Prefix + "workload-"
	modelPrefix    = Prefix + "model-"

	// CPULabel marks nodes for the platform's CPU workloads: app-shaide,
	// app-mcp, and anything else that needs no GPU.
	CPULabel = workloadPrefix + "cpu"

	GenerativeLabel = workloadPrefix + "generative"
	EmbeddingLabel  = workloadPrefix + "embedding"

	// Value is the value of every placement label.
	Value = "true"

	// GPUTaintKey is the taint GPU nodes may carry to keep other workloads
	// off them. The serving stack's model workloads tolerate it, so a node
	// tainted with it can still serve a model.
	GPUTaintKey = "nvidia.com/gpu"
)

// ModelLabel marks the nodes of one model's pool. The slug is at most 47
// characters, so the label name stays within Kubernetes' 63.
func ModelLabel(slug string) string {
	return modelPrefix + slug
}

// ClassLabel is the workload class label of a model category.
func ClassLabel(category string) string {
	if category == catalog.CategoryEmbedder {
		return EmbeddingLabel
	}

	return GenerativeLabel
}

// ModelSelector is the node selector of a model: its own pool, and the class
// the pool serves.
func ModelSelector(model catalog.Model) map[string]string {
	return map[string]string{
		ModelLabel(model.Slug):     Value,
		ClassLabel(model.Category): Value,
	}
}

// Managed reports whether a node label is one the installer owns.
func Managed(key string) bool {
	return strings.HasPrefix(key, workloadPrefix) || strings.HasPrefix(key, modelPrefix)
}

// ModelSlug returns the slug of a model pool label.
func ModelSlug(key string) (string, bool) {
	if !strings.HasPrefix(key, modelPrefix) {
		return "", false
	}

	return strings.TrimPrefix(key, modelPrefix), true
}
