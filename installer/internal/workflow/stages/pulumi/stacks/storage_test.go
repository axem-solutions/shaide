package stacks

import (
	"io"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/monitoring"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// storageReporter records the StorageClass picker and answers it.
type storageReporter struct {
	answer  string // "" keeps the pre-selected option
	asked   int
	current string
	options []string
}

func (r *storageReporter) Select(_ string, current string, options []string) (string, error) {
	r.asked++
	r.current = current
	r.options = options
	if r.answer == "" {
		return current, nil
	}
	return r.answer, nil
}

func (*storageReporter) MultiSelect(string, []string) ([]string, error) { return nil, nil }

func (*storageReporter) Input(string, string, string) (string, error) { return "", nil }

func (*storageReporter) ProgressModel(core.ModelProgress) {}

func storageRuntime(reporter core.Reporter, objects ...runtime.Object) *core.Runtime {
	rt := core.NewContext(logger.NewWithWriter(io.Discard), reporter)
	rt.Cluster.Client = fake.NewSimpleClientset(objects...)
	return rt
}

func storageClass(name string, isDefault bool) *storagev1.StorageClass {
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if isDefault {
		sc.Annotations = map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}
	}
	return sc
}

func monitoringPVC(name, class string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: monitoring.Namespace},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &class},
	}
}

// The picker offers only classes that exist, so a typo can no longer leave a
// PVC Pending until the Helm release times out.
func TestStorageClassPickerListsClusterClasses(t *testing.T) {
	reporter := &storageReporter{}
	rt := storageRuntime(reporter, storageClass("default", true), storageClass("managed-csi", false))

	got, err := promptStorageClass(rt, "StorageClass", "")
	if err != nil {
		t.Fatalf("promptStorageClass() error = %v", err)
	}

	want := []string{clusterDefaultStorageClassLabel, "default (cluster default)", "managed-csi"}
	if len(reporter.options) != len(want) {
		t.Fatalf("options = %v, want %v", reporter.options, want)
	}
	for i := range want {
		if reporter.options[i] != want[i] {
			t.Fatalf("options = %v, want %v", reporter.options, want)
		}
	}
	if got != "default" {
		t.Errorf("pre-selected class = %q, want the cluster default %q", got, "default")
	}
}

func TestStorageClassPickerPreselectsPreferred(t *testing.T) {
	reporter := &storageReporter{}
	rt := storageRuntime(reporter, storageClass("default", true), storageClass("managed-csi", false))

	got, err := promptStorageClass(rt, "StorageClass", "managed-csi")
	if err != nil {
		t.Fatalf("promptStorageClass() error = %v", err)
	}
	if got != "managed-csi" {
		t.Errorf("selected = %q, want the preferred %q", got, "managed-csi")
	}
}

func TestStorageClassPickerClusterDefaultIsEmpty(t *testing.T) {
	reporter := &storageReporter{answer: clusterDefaultStorageClassLabel}
	rt := storageRuntime(reporter, storageClass("default", true))

	got, err := promptStorageClass(rt, "StorageClass", "")
	if err != nil {
		t.Fatalf("promptStorageClass() error = %v", err)
	}
	if got != "" {
		t.Errorf("selected = %q, want empty for the cluster default", got)
	}
}

// On an update both volumes exist, and their classes are immutable: keep them
// and do not ask.
func TestMonitoringKeepsExistingPVCClasses(t *testing.T) {
	reporter := &storageReporter{}
	rt := storageRuntime(reporter,
		storageClass("default", true),
		storageClass("managed-csi", false),
		monitoringPVC(monitoring.LokiPVCName, "managed-csi"),
		monitoringPVC(monitoring.PrometheusPVCName, "default"),
	)

	got, err := monitoringStorageClasses(rt)
	if err != nil {
		t.Fatalf("monitoringStorageClasses() error = %v", err)
	}
	if reporter.asked != 0 {
		t.Errorf("operator was asked %d times; existing classes cannot change", reporter.asked)
	}
	if got.LokiStorageClass != "managed-csi" || got.PrometheusStorageClass != "default" {
		t.Errorf("classes = %+v, want the existing PVCs' classes", got)
	}
}

// A fresh install asks once and uses the answer for both volumes.
func TestMonitoringAsksOnceOnInstall(t *testing.T) {
	reporter := &storageReporter{answer: "managed-csi"}
	rt := storageRuntime(reporter, storageClass("default", true), storageClass("managed-csi", false))

	got, err := monitoringStorageClasses(rt)
	if err != nil {
		t.Fatalf("monitoringStorageClasses() error = %v", err)
	}
	if reporter.asked != 1 {
		t.Errorf("operator was asked %d times, want once", reporter.asked)
	}
	if got.LokiStorageClass != "managed-csi" || got.PrometheusStorageClass != "managed-csi" {
		t.Errorf("classes = %+v, want the answer for both", got)
	}
}

// When only one volume exists, the question covers the other one and
// pre-selects the class already in use.
func TestMonitoringAsksOnlyForMissingPVC(t *testing.T) {
	reporter := &storageReporter{}
	rt := storageRuntime(reporter,
		storageClass("default", true),
		storageClass("managed-csi", false),
		monitoringPVC(monitoring.LokiPVCName, "managed-csi"),
	)

	got, err := monitoringStorageClasses(rt)
	if err != nil {
		t.Fatalf("monitoringStorageClasses() error = %v", err)
	}
	if reporter.current != "managed-csi" {
		t.Errorf("pre-selected = %q, want the existing Loki class %q", reporter.current, "managed-csi")
	}
	if got.LokiStorageClass != "managed-csi" || got.PrometheusStorageClass != "managed-csi" {
		t.Errorf("classes = %+v, want both on the existing class", got)
	}
}
