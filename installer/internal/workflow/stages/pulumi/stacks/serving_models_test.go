package stacks

import (
	"context"
	"io"
	"k8s.io/client-go/kubernetes/fake"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/axem-solutions/ai_platform/installer/internal/logger"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
	"github.com/axem-solutions/ai_platform/installer/internal/placement"
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
	rt.Models.Serve = models

	return rt
}

func gptOSS() catalog.Model {
	return catalog.Model{
		Name:          "GPT-OSS-20B",
		Category:      catalog.CategoryGenerative,
		Slug:          "gpt-oss-20b",
		ID:            "openai/gpt-oss-20b",
		Revision:      "6cee5e81ee83917806bbde320786a8fb61efebee",
		StorageSize:   "70Gi",
		HarborProject: "ai-models",
		HarborName:    "gpt-oss-20b",
		HarborTag:     "6cee5e81ee83",
	}
}

// The catalog already records where a model was mirrored and which
// repository it is, so the installer passes both; the stack derives the rest.
func TestSelectedModelsCarryTheCatalog(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})
	rt := runtimeWith(t, projects, []catalog.Model{gptOSS()})

	models := selectedModels(rt)
	if len(models) != 1 {
		t.Fatalf("selected %d models, want 1", len(models))
	}

	got := models[0]
	want := serving.Model{
		Name:        "GPT-OSS-20B",
		ID:          "openai/gpt-oss-20b",
		HarborRef:   "harbor.harbor.svc.cluster.local/ai-models/gpt-oss-20b:6cee5e81ee83",
		StorageSize: "70Gi",
	}
	if got.Name != want.Name || got.ID != want.ID || got.HarborRef != want.HarborRef ||
		got.StorageSize != want.StorageSize || got.StorageClass != "" {
		t.Errorf("model = %+v, want %+v", got, want)
	}
}

// Each model runs on its own pool, the nodes the node assignment stage
// labelled for it, which also carry the class of the pool.
func TestModelsRunOnTheirOwnPool(t *testing.T) {
	embedder := gptOSS()
	embedder.Name, embedder.Slug, embedder.Category = "BGE-M3", "bge-m3", catalog.CategoryEmbedder

	rt := runtimeWith(t, t.TempDir(), []catalog.Model{gptOSS(), embedder})
	rt.Placement = placement.Assignment{Models: map[string][]placement.Pool{
		"gpt-oss-20b": {
			{Key: "kubernetes.azure.com/agentpool", Name: "generative-b"},
			{Key: "kubernetes.azure.com/agentpool", Name: "generative"},
		},
		"bge-m3": {
			{Key: "nodegroup", Name: "gpu"},
			{Key: "kubernetes.io/hostname", Name: "server4"},
		},
	}}

	models := selectedModels(rt)

	want := [][]serving.PlacementTerm{
		{{Key: "kubernetes.azure.com/agentpool", Values: []string{"generative", "generative-b"}}},
		{
			{Key: "kubernetes.io/hostname", Values: []string{"server4"}},
			{Key: "nodegroup", Values: []string{"gpu"}},
		},
	}
	for i := range want {
		if !reflect.DeepEqual(models[i].Placement, want[i]) {
			t.Errorf("%s placement = %+v, want %+v", models[i].Name, models[i].Placement, want[i])
		}
	}
}

// A model without assigned pools gets no placement, and the stack falls back
// to its default GPU pool.
func TestModelWithoutPoolsHasNoPlacement(t *testing.T) {
	rt := runtimeWith(t, t.TempDir(), []catalog.Model{gptOSS()})

	if got := selectedModels(rt)[0].Placement; got != nil {
		t.Errorf("placement = %+v, want none", got)
	}
}

// Only the models chosen to serve reach the stack; the rest of the catalog
// does not.
func TestOnlyModelsToServeAreDeployed(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})

	rt := runtimeWith(t, projects, nil)
	rt.Bootstrap.Catalog.Models = []catalog.Model{gptOSS()}

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

// A class already set on a model is used as is.
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

// sizedPVC is an existing model volume that asked for request and was given
// capacity.
func sizedPVC(namespace, name, request, capacity string) *corev1.PersistentVolumeClaim {
	pvc := modelPVC(namespace, name, "managed-csi")
	pvc.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(request)}
	if capacity != "" {
		pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(capacity)}
	}
	return pvc
}

func sizedModels() []serving.Model {
	return []serving.Model{{Name: "GPT-OSS-20B", StorageSize: "60Gi"}, {Name: "BGE-M3", StorageSize: "10Gi"}}
}

// A volume created larger than the model now asks for keeps its size: asking
// for less is rejected by the API server, since a volume cannot shrink.
func TestModelStorageSizeNeverShrinksAVolume(t *testing.T) {
	rt, workDir := servingStorageRuntime(t, &storageReporter{},
		sizedPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "70Gi", "70Gi"),
	)

	models, err := modelStorageSizes(rt, workDir, sizedModels(), false)
	if err != nil {
		t.Fatalf("modelStorageSizes() error = %v", err)
	}
	if models[0].StorageSize != "70Gi" {
		t.Errorf("GPT-OSS-20B size = %q, want the existing 70Gi kept", models[0].StorageSize)
	}
	if models[1].StorageSize != "10Gi" {
		t.Errorf("BGE-M3 size = %q, want its configured 10Gi, as it has no volume", models[1].StorageSize)
	}
}

// The capacity counts, not only the request: a provisioner may round a volume
// up, and the request may not go below what it was given.
func TestModelStorageSizeKeepsTheProvisionedCapacity(t *testing.T) {
	rt, workDir := servingStorageRuntime(t, &storageReporter{},
		sizedPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "60Gi", "64Gi"),
	)

	models, err := modelStorageSizes(rt, workDir, sizedModels(), false)
	if err != nil {
		t.Fatalf("modelStorageSizes() error = %v", err)
	}
	if models[0].StorageSize != "64Gi" {
		t.Errorf("size = %q, want the provisioned 64Gi", models[0].StorageSize)
	}
}

// A larger configured size still grows the volume.
func TestModelStorageSizeGrowsAVolume(t *testing.T) {
	rt, workDir := servingStorageRuntime(t, &storageReporter{},
		sizedPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "50Gi", "50Gi"),
	)

	models, err := modelStorageSizes(rt, workDir, sizedModels(), false)
	if err != nil {
		t.Fatalf("modelStorageSizes() error = %v", err)
	}
	if models[0].StorageSize != "60Gi" {
		t.Errorf("size = %q, want the configured 60Gi", models[0].StorageSize)
	}
}

// Recreating deletes the volumes first, so the configured size applies.
func TestModelStorageSizeIsConfiguredWhenRecreating(t *testing.T) {
	rt, workDir := servingStorageRuntime(t, &storageReporter{},
		sizedPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "70Gi", "70Gi"),
	)

	models, err := modelStorageSizes(rt, workDir, sizedModels(), true)
	if err != nil {
		t.Fatalf("modelStorageSizes() error = %v", err)
	}
	if models[0].StorageSize != "60Gi" {
		t.Errorf("size = %q, want the configured 60Gi when recreating", models[0].StorageSize)
	}
}

// A model without a configured size would get the stack default, which may be
// smaller than its volume, so it keeps the volume's size too.
func TestModelStorageSizeFillsAMissingSizeFromTheVolume(t *testing.T) {
	rt, workDir := servingStorageRuntime(t, &storageReporter{},
		sizedPVC("llm-d-gpt-oss-20b", "gpt-oss-20b-model", "70Gi", "70Gi"),
	)

	models := sizedModels()
	models[0].StorageSize = ""

	got, err := modelStorageSizes(rt, workDir, models, false)
	if err != nil {
		t.Fatalf("modelStorageSizes() error = %v", err)
	}
	if got[0].StorageSize != "70Gi" {
		t.Errorf("size = %q, want the volume's 70Gi", got[0].StorageSize)
	}
}

// Before deploying, a model pod left on a deleted node is removed, so a
// recreate does not wait for its Deployment until it times out.
func TestOrphanedModelPodsAreReleased(t *testing.T) {
	projects := packagedModels(t, map[string]string{"GPT-OSS-20B": "generative"})
	rt := runtimeWith(t, projects, nil)
	rt.Bootstrap.Catalog.Models = []catalog.Model{gptOSS()}
	rt.Cluster.Client = fake.NewClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "decode", Namespace: "llm-d-gpt-oss-20b"},
			Spec:       corev1.PodSpec{NodeName: "deleted-node"},
		},
	)

	if err := releaseOrphanedModelPods(rt, filepath.Join(projects, projectAppServing)); err != nil {
		t.Fatalf("releaseOrphanedModelPods: %v", err)
	}

	if _, err := rt.Cluster.Client.CoreV1().Pods("llm-d-gpt-oss-20b").Get(context.Background(), "decode", metav1.GetOptions{}); err == nil {
		t.Error("the pod on the deleted node was not removed")
	}
}
