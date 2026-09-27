package stacks

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/axem-solutions/ai_platform/installer/internal/logger"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving"
)

// packagedModels lays out the values directories the installer image ships:
// deployments/models/<category>/<name>/{gaie,ms}-<slug>/values.yaml, with the
// slug being the lowercased name.
func packagedModels(t *testing.T, names map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, category := range names {
		slug := strings.ToLower(name)
		for _, release := range []string{"gaie-" + slug, "ms-" + slug} {
			dir := filepath.Join(root, projectAppServing, "deployments", "models", category, name, release)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("create %s: %v", dir, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "values.yaml"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
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

// The manifest already records where a model was mirrored and which
// repository it is, so the installer passes both; the stack derives the rest.
func TestSelectedModelsCarryTheManifest(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})
	rt := runtimeWith(t, projects, []catalog.Model{gptOSS()})

	models := selectedModels(rt)
	if len(models) != 1 {
		t.Fatalf("selected %d models, want 1", len(models))
	}

	want := serving.Model{
		Name:        "GPT-OSS-20B",
		ID:          "openai/gpt-oss-20b",
		HarborRef:   "harbor.harbor.svc.cluster.local/ai-models/gpt-oss-20b:1.0.0",
		StorageSize: "70Gi",
	}
	if models[0] != want {
		t.Errorf("model = %+v, want %+v", models[0], want)
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

func modelPVC(namespace, name, class string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &class},
	}
}

func servingStorageRuntime(t *testing.T, reporter *storageReporter, objects ...runtime.Object) (*core.Runtime, string) {
	t.Helper()

	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative", "BGE-M3": "embedder"})
	rt := storageRuntime(reporter, append(objects,
		storageClass("managed-csi", true),
		storageClass("premium", false),
	)...)
	return rt, filepath.Join(projects, projectAppServing)
}

func servingModels() []serving.Model {
	return []serving.Model{{Name: "GPT-OSS-20B"}, {Name: "BGE-M3"}}
}

// On an update, a model whose volume exists keeps its class, and nothing is
// asked when every model has one.
func TestModelStorageKeepsExistingVolumes(t *testing.T) {
	reporter := &storageReporter{}
	rt, workDir := servingStorageRuntime(t, reporter,
		modelPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "premium"),
		modelPVC("llm-d-bge-m3", "bge-m3-model", "managed-csi"),
	)

	models, err := modelStorageClasses(rt, workDir, "azure", servingModels(), false)
	if err != nil {
		t.Fatalf("modelStorageClasses() error = %v", err)
	}
	if reporter.asked != 0 {
		t.Errorf("asked %d times, want no question", reporter.asked)
	}
	if models[0].StorageClass != "premium" || models[1].StorageClass != "managed-csi" {
		t.Errorf("classes = %q, %q, want each volume's own", models[0].StorageClass, models[1].StorageClass)
	}
}

// A new model is asked about, pre-selecting the class already in use, and the
// answer does not reach the model whose volume exists.
func TestModelStorageAsksOnlyForNewVolumes(t *testing.T) {
	reporter := &storageReporter{}
	rt, workDir := servingStorageRuntime(t, reporter,
		modelPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "premium"),
	)

	models, err := modelStorageClasses(rt, workDir, "azure", servingModels(), false)
	if err != nil {
		t.Fatalf("modelStorageClasses() error = %v", err)
	}
	if reporter.asked != 1 || reporter.current != "premium" {
		t.Errorf("asked %d times pre-selecting %q, want once pre-selecting premium", reporter.asked, reporter.current)
	}
	if models[0].StorageClass != "premium" || models[1].StorageClass != "premium" {
		t.Errorf("classes = %q, %q, want premium for both", models[0].StorageClass, models[1].StorageClass)
	}
}

// Recreating deletes the volumes, so every model is asked about again.
func TestModelStorageAsksAgainWhenRecreating(t *testing.T) {
	reporter := &storageReporter{answer: "managed-csi (cluster default)"}
	rt, workDir := servingStorageRuntime(t, reporter,
		modelPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "premium"),
		modelPVC("llm-d-bge-m3", "bge-m3-model", "premium"),
	)

	models, err := modelStorageClasses(rt, workDir, "azure", servingModels(), true)
	if err != nil {
		t.Fatalf("modelStorageClasses() error = %v", err)
	}
	if reporter.asked != 1 || reporter.current != "premium" {
		t.Errorf("asked %d times pre-selecting %q, want once pre-selecting premium", reporter.asked, reporter.current)
	}
	if models[0].StorageClass != "managed-csi" || models[1].StorageClass != "managed-csi" {
		t.Errorf("classes = %q, %q, want the new answer for both", models[0].StorageClass, models[1].StorageClass)
	}
}

// A class the manifest pins is used as is.
func TestModelStorageLeavesPinnedClasses(t *testing.T) {
	reporter := &storageReporter{}
	rt, workDir := servingStorageRuntime(t, reporter)

	models := servingModels()
	models[0].StorageClass = "hyperdisk-balanced"
	models[1].StorageClass = "premium"

	got, err := modelStorageClasses(rt, workDir, "azure", models, false)
	if err != nil {
		t.Fatalf("modelStorageClasses() error = %v", err)
	}
	if reporter.asked != 0 || got[0].StorageClass != "hyperdisk-balanced" {
		t.Errorf("asked %d, class %q, want the pinned class and no question", reporter.asked, got[0].StorageClass)
	}
}

// On-prem the stack puts every model volume on the hostpath class, so the
// answer could not matter and nothing is asked.
func TestModelStorageIsNotAskedOnPrem(t *testing.T) {
	reporter := &storageReporter{}
	rt, workDir := servingStorageRuntime(t, reporter)

	if _, err := modelStorageClasses(rt, workDir, "on-prem", servingModels(), false); err != nil {
		t.Fatalf("modelStorageClasses() error = %v", err)
	}
	if reporter.asked != 0 {
		t.Errorf("asked %d times on-prem, want no question", reporter.asked)
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
