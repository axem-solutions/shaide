package discovery

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

type selectReporter struct {
	selectTitle   string
	selectCurrent string
	selected      string
	inputDefault  string
}

func (r *selectReporter) Select(title string, current string, _ []string) (string, error) {
	r.selectTitle = title
	r.selectCurrent = current
	return r.selected, nil
}

func (r *selectReporter) Input(_ string, _ string, defaultValue string) (string, error) {
	r.inputDefault = defaultValue
	return defaultValue, nil
}

func (*selectReporter) MultiSelect(string, []string) ([]string, error) { return nil, nil }
func (*selectReporter) Choose(core.ChoicePrompt) ([]string, error)     { return nil, nil }
func (*selectReporter) ProgressModel(core.ModelProgress)               {}

func newDiscoveryRuntime(reporter core.Reporter, objects ...runtime.Object) *core.Runtime {
	rt := core.NewContext(logger.NewWithWriter(io.Discard), reporter)
	rt.Bootstrap.Config.Harbor.Namespace = "harbor"
	rt.Bootstrap.Config.Harbor.Service = "harbor"
	rt.Cluster.Client = fake.NewClientset(objects...)
	return rt
}

func newService(namespace, name string, labels map[string]string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func TestCheckResourcesInstallsHarborOnNewCluster(t *testing.T) {
	rt := newDiscoveryRuntime(&selectReporter{},
		newService("default", "kubernetes", nil),
	)

	if err := CheckResources(rt); err != nil {
		t.Fatalf("CheckResources: %v", err)
	}
	if rt.Discovery.Mode != core.Install {
		t.Errorf("mode = %v, want %v", rt.Discovery.Mode, core.Install)
	}
}

func TestCheckResourcesAsksWhenHarborRunsInAnotherNamespace(t *testing.T) {
	rt := newDiscoveryRuntime(&selectReporter{},
		newService("registry", "harbor-core", map[string]string{"app": "harbor"}),
		newService("mirror", "harbor", map[string]string{"app.kubernetes.io/name": "harbor"}),
	)

	err := CheckResources(rt)

	var discoveryErr harborDiscoveryError
	if !errors.As(err, &discoveryErr) {
		t.Fatalf("CheckResources error = %v, want a harborDiscoveryError", err)
	}
	if want := []string{"mirror", "registry"}; !slices.Equal(discoveryErr.Candidates, want) {
		t.Errorf("candidates = %v, want %v", discoveryErr.Candidates, want)
	}
	if rt.Discovery.Mode != core.Update {
		t.Errorf("mode = %v, want it unchanged (%v)", rt.Discovery.Mode, core.Update)
	}
}

func TestRecoverHarborNamespaceOffersFoundNamespace(t *testing.T) {
	reporter := &selectReporter{selected: "Choose another namespace"}
	rt := newDiscoveryRuntime(reporter)
	runErr := harborDiscoveryError{
		Kind:         harborNamespace,
		Problem:      missing,
		ResourceName: "harbor",
		Candidates:   []string{"registry"},
	}

	action, err := recoverCheckResources(rt, runErr)
	if err != nil {
		t.Fatalf("recoverCheckResources: %v", err)
	}

	if action != core.RecoveryRetryStep {
		t.Errorf("action = %v, want %v", action, core.RecoveryRetryStep)
	}
	if !strings.Contains(reporter.selectTitle, "registry") {
		t.Errorf("prompt %q does not name the namespace Harbor runs in", reporter.selectTitle)
	}
	if reporter.selectCurrent != "Choose another namespace" {
		t.Errorf("preselected = %q, want %q", reporter.selectCurrent, "Choose another namespace")
	}
	if reporter.inputDefault != "registry" {
		t.Errorf("namespace default = %q, want %q", reporter.inputDefault, "registry")
	}
	if rt.Bootstrap.Config.Harbor.Namespace != "registry" {
		t.Errorf("namespace = %q, want %q", rt.Bootstrap.Config.Harbor.Namespace, "registry")
	}
}
