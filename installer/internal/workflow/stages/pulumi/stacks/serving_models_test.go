package stacks

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/logger"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving"
)

// packagedModels lays out the values directories the installer image ships, so
// the category lookup has something real to find.
func packagedModels(t *testing.T, names map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, category := range names {
		dir := filepath.Join(root, projectAppServing, "deployments", "models", category, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	return root
}

func runtimeWith(t *testing.T, projectsDir string, models []catalog.Model) *core.Runtime {
	t.Helper()

	rt := &core.Runtime{
		Logger:           logger.NewWithWriter(io.Discard),
		GlobalState:      core.NewGlobalState(),
		ActiveStageState: core.NewActiveStageState(),
	}
	rt.Bootstrap.Config.Paths = paths.Paths{ProjectsDir: projectsDir}
	rt.Bootstrap.Config.Harbor.Service = "harbor"
	rt.Bootstrap.Config.Harbor.Namespace = "harbor"
	rt.Bootstrap.Catalog.Models = models

	return rt
}

func gptOSS() catalog.Model {
	return catalog.Model{
		ID:            "openai/gpt-oss-20b",
		HarborProject: "ai-models",
		HarborName:    "gpt-oss-20b",
		HarborTag:     "1.0.0",
		Serving: &catalog.Serving{
			Name:         "GPT-OSS-20B",
			NodeSelector: "generative",
			StorageSize:  "70Gi",
		},
	}
}

// The manifest already records where a model was mirrored, so the reference the
// cluster pulls and the path inside the volume are derived, not configured.
func TestSelectedModelsDerivesHarborRefAndURI(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})
	rt := runtimeWith(t, projects, []catalog.Model{gptOSS()})

	models := selectedModels(rt)
	if len(models) != 1 {
		t.Fatalf("selected %d models, want 1", len(models))
	}

	model := models[0]
	if want := "harbor.harbor.svc.cluster.local/ai-models/gpt-oss-20b:1.0.0"; model.HarborRef != want {
		t.Errorf("HarborRef = %q, want %q", model.HarborRef, want)
	}
	if want := "hub/openai/gpt-oss-20b"; model.ModelURI != want {
		t.Errorf("ModelURI = %q, want %q", model.ModelURI, want)
	}
	if model.Category != serving.CategoryGenerative {
		t.Errorf("Category = %q, want it found from the packaged directory", model.Category)
	}
	if got := model.NodeSelector["nodegroup"]; got != "generative" {
		t.Errorf("NodeSelector = %v, want the nodegroup pool", model.NodeSelector)
	}
	if model.StorageSize != "70Gi" {
		t.Errorf("StorageSize = %q, want 70Gi", model.StorageSize)
	}
}

// A model without a serving block is mirrored into Harbor but not deployed,
// which is what a cluster that publishes models for another consumer wants.
func TestModelsWithoutServingAreNotDeployed(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})
	mirrorOnly := catalog.Model{ID: "nomic-ai/nomic-embed-text-v1.5", HarborName: "nomic"}

	rt := runtimeWith(t, projects, []catalog.Model{mirrorOnly})

	if models := selectedModels(rt); len(models) != 0 {
		t.Errorf("selected %v, want nothing to deploy", models)
	}
}

// Naming a model the image does not package would deploy the wrong runtime or
// none at all, so it is skipped rather than passed through.
func TestUnknownModelDirectoryIsSkipped(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})

	unknown := gptOSS()
	unknown.Serving.Name = "Model-That-Is-Not-Packaged"

	rt := runtimeWith(t, projects, []catalog.Model{unknown})

	if models := selectedModels(rt); len(models) != 0 {
		t.Errorf("selected %v, want the unpackaged model skipped", models)
	}
}

// An embedder must be found in its own category directory.
func TestEmbedderCategoryIsFound(t *testing.T) {
	projects := packagedModels(t, map[string]string{"BGE-M3": "embedder"})

	embedder := gptOSS()
	embedder.Serving.Name = "BGE-M3"

	rt := runtimeWith(t, projects, []catalog.Model{embedder})

	models := selectedModels(rt)
	if len(models) != 1 || models[0].Category != serving.CategoryEmbedder {
		t.Fatalf("selected %v, want one embedder", models)
	}
}

// The installer currently has one explicit scheduling class for inference.
// A manifest value must not move a model onto a general-purpose node until the
// installer gains a more sophisticated placement policy.
func TestInferenceNodeSelectorIsInstallerManaged(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})

	model := gptOSS()
	model.Serving.NodeSelector = "some-other-pool"

	rt := runtimeWith(t, projects, []catalog.Model{model})

	if got := selectedModels(rt)[0].NodeSelector; got[inferenceNodeSelectorKey] != inferenceNodeSelectorValue {
		t.Errorf("NodeSelector = %v, want %s=%s", got, inferenceNodeSelectorKey, inferenceNodeSelectorValue)
	}
}

func TestInferenceGPUTolerationMatchesReservedPoolTaint(t *testing.T) {
	toleration := inferenceGPUToleration()

	if toleration.Key != "nvidia.com/gpu" || toleration.Operator != "Equal" ||
		toleration.Value != "present" || toleration.Effect != "NoSchedule" {
		t.Fatalf("inference toleration = %+v, want the reserved GPU pool taint", toleration)
	}
}

// A model with a modelSource is pulled from Harbor by the cluster, so the stack
// is rejected without the registry address. The installer knows it, and until
// it passed it the deployment failed at Pulumi runtime with
// "harborHostname is required for on-prem or modelSource deployments".
func TestServingOptionsCarryTheRegistryAddress(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})
	rt := runtimeWith(t, projects, []catalog.Model{gptOSS()})

	registry := harborRegistryHostname(rt)
	if registry != "harbor.harbor.svc.cluster.local" {
		t.Fatalf("registry = %q, want the in-cluster Harbor address", registry)
	}

	// The same address the model references are built from, so a model can
	// always be pulled from where the stack is told to look.
	models := selectedModels(rt)
	if len(models) != 1 {
		t.Fatalf("selected %d models, want 1", len(models))
	}
	if !strings.HasPrefix(models[0].HarborRef, registry+"/") {
		t.Errorf("HarborRef %q does not point at %q", models[0].HarborRef, registry)
	}
}
