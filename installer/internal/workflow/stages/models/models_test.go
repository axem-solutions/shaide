package models

import (
	"io"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type choosingReporter struct {
	prompt  core.ChoicePrompt
	actions []string
}

func (r *choosingReporter) Choose(prompt core.ChoicePrompt) ([]string, error) {
	r.prompt = prompt
	return r.actions, nil
}

func (*choosingReporter) Select(_ string, current string, _ []string) (string, error) {
	return current, nil
}
func (*choosingReporter) MultiSelect(string, []string) ([]string, error) { return nil, nil }
func (*choosingReporter) Input(string, string, string) (string, error)   { return "", nil }
func (*choosingReporter) ProgressModel(core.ModelProgress)               {}

func model(name, slug string) catalog.Model {
	return catalog.Model{Name: name, Slug: slug, Category: catalog.CategoryGenerative, HarborTag: "6cee5e81ee83"}
}

func pod(namespace, name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func newRuntime(reporter core.Reporter, models ...catalog.Model) *core.Runtime {
	rt := core.NewContext(logger.NewWithWriter(io.Discard), reporter)
	rt.Bootstrap.Catalog.Models = models
	return rt
}

func TestDetectInstalledModelsReadsBothLabelDomains(t *testing.T) {
	rt := newRuntime(&choosingReporter{},
		model("GPT-OSS-20B", "gpt-oss-20b"),
		model("BGE-M3", "bge-m3"),
		model("GLM-4.7-Flash", "glm-4-7-flash"),
	)
	rt.Cluster.Client = fake.NewClientset(
		pod("llm-d-gpt-oss-20b", "decode", map[string]string{"axem.dev/model-slug": "gpt-oss-20b"}),
		pod("llm-d-bge-m3", "decode", map[string]string{"axem.ai/model-slug": "bge-m3"}),
		pod("llm-d-other", "decode", map[string]string{"axem.dev/model-slug": "not-in-catalog"}),
		pod("default", "unrelated", nil),
	)

	if err := detectInstalledModels(rt); err != nil {
		t.Fatalf("detectInstalledModels: %v", err)
	}

	want := map[string]bool{"gpt-oss-20b": true, "bge-m3": true}
	if len(rt.Models.Installed) != len(want) {
		t.Fatalf("installed = %v, want %v", rt.Models.Installed, want)
	}
	for slug := range want {
		if !rt.Models.Installed[slug] {
			t.Errorf("%s not detected as installed", slug)
		}
	}
}

func TestSelectModelsOffersActionsByStatus(t *testing.T) {
	reporter := &choosingReporter{actions: []string{ActionKeep, ActionNone}}
	rt := newRuntime(reporter, model("GPT-OSS-20B", "gpt-oss-20b"), model("BGE-M3", "bge-m3"))
	rt.Models.Installed = map[string]bool{"gpt-oss-20b": true}

	if err := selectModels(rt); err != nil {
		t.Fatalf("selectModels: %v", err)
	}

	installed, available := reporter.prompt.Rows[0], reporter.prompt.Rows[1]
	if installed.Current != ActionKeep || len(installed.Options) != 2 || installed.Options[1] != ActionUninstall {
		t.Errorf("installed row = %+v, want keep/uninstall starting at keep", installed)
	}
	if available.Current != ActionNone || len(available.Options) != 2 || available.Options[1] != ActionInstall {
		t.Errorf("available row = %+v, want -/install starting at -", available)
	}
	if installed.Cells[4] != "Installed" || available.Cells[4] != "Available" {
		t.Errorf("status cells = %q, %q", installed.Cells[4], available.Cells[4])
	}
}

func TestSelectModelsSplitsServeAndUninstall(t *testing.T) {
	reporter := &choosingReporter{actions: []string{ActionKeep, ActionUninstall, ActionInstall, ActionNone}}
	rt := newRuntime(reporter,
		model("Kept", "kept"),
		model("Removed", "removed"),
		model("Added", "added"),
		model("Ignored", "ignored"),
	)
	rt.Models.Installed = map[string]bool{"kept": true, "removed": true}

	if err := selectModels(rt); err != nil {
		t.Fatalf("selectModels: %v", err)
	}

	if names(rt.Models.Serve) != "Kept,Added" {
		t.Errorf("serve = %s, want Kept,Added", names(rt.Models.Serve))
	}
	if names(rt.Models.Uninstall) != "Removed" {
		t.Errorf("uninstall = %s, want Removed", names(rt.Models.Uninstall))
	}
}

func TestSelectModelsRejectsAShortReply(t *testing.T) {
	reporter := &choosingReporter{actions: []string{ActionInstall}}
	rt := newRuntime(reporter, model("A", "a"), model("B", "b"))

	if err := selectModels(rt); err == nil {
		t.Fatal("selectModels accepted one action for two models")
	}
}

func names(models []catalog.Model) string {
	out := ""
	for i, m := range models {
		if i > 0 {
			out += ","
		}
		out += m.Name
	}
	return out
}
