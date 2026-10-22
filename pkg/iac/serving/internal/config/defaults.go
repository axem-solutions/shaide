package config

import (
	"os"
	"path/filepath"
)

// Categories a packaged model's values live under, in
// deployments/models/<category>/<name>.
const (
	CategoryGenerative = "generative"
	CategoryEmbedder   = "embedder"
)

// Inference scheduling policy. Model workloads are isolated on the GPU pool:
// they select its nodes and tolerate its taint, while the rest of the platform
// does not tolerate it and so stays off those nodes.
const (
	inferenceNodeSelectorKey   = "nodegroup"
	inferenceNodeSelectorValue = "generative"
)

// DefaultGPUToleration tolerates the NoSchedule taint reserving the GPU pool.
var DefaultGPUToleration = Toleration{
	Key:      "nvidia.com/gpu",
	Operator: "Equal",
	Value:    "present",
	Effect:   "NoSchedule",
}

// DefaultInferenceNodeSelector places a model on the GPU pool. A fresh map is
// returned so a caller cannot change the policy for every model.
func DefaultInferenceNodeSelector() map[string]string {
	return map[string]string{inferenceNodeSelectorKey: inferenceNodeSelectorValue}
}

// ORAS pulls model weights into their volume. ORASImageName is the upstream
// name the image manifest mirrors it under; DefaultORASImage is used when no
// mirrored reference is supplied, e.g. for a direct pulumi up.
const (
	ORASImageName    = "oras-project/oras"
	DefaultORASImage = "ghcr.io/oras-project/oras:v1.3.1"
)

// The model server and the llm-d router run these images. The names are the
// upstream ones the image manifest mirrors them under; the defaults are used
// when no mirrored reference is supplied. The versions follow llm-d v1.0:
// its GPU vLLM recipe and its router release.
const (
	ModelServerImageName    = "vllm/vllm-openai"
	DefaultModelServerImage = "docker.io/vllm/vllm-openai:v0.31.0"

	RouterImageName    = "llm-d/llm-d-router-endpoint-picker"
	DefaultRouterImage = "ghcr.io/llm-d/llm-d-router-endpoint-picker:v1.0.0"

	// SimulatorImageName is llm-d's vLLM simulator, which models with engine
	// sim run unless they name their own image.
	SimulatorImageName    = "llm-d/llm-d-inference-sim"
	DefaultSimulatorImage = "ghcr.io/llm-d/llm-d-inference-sim:v1.0.0"
)

// ModelCategory reports which category holds a packaged model's values, or
// false when the project packages no values for it.
func ModelCategory(projectDir, name string) (string, bool) {
	for _, category := range []string{CategoryGenerative, CategoryEmbedder} {
		info, err := os.Stat(filepath.Join(projectDir, deploymentFolder, modelFolder, category, name))
		if err == nil && info.IsDir() {
			return category, true
		}
	}

	return "", false
}

// ModelVolume names the namespace and PersistentVolumeClaim that hold a
// packaged model's weights. Both derive from the slug in its model.yaml, so
// only the project can name them.
func ModelVolume(projectDir, category, name string) (namespace, claim string, err error) {
	paths, _, err := resolveModelPaths(category, name, projectDir)
	if err != nil {
		return "", "", err
	}

	return modelNamespace(paths.Slug), modelClaim(paths.Slug), nil
}

func modelNamespace(slug string) string {
	return "llm-d-" + slug
}

func modelClaim(slug string) string {
	return slug + "-model"
}
