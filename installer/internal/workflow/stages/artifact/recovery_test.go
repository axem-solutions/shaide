package artifact

import (
	"fmt"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	oras "github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

// fakeReporter answers prompts from a script and records what it was asked, so
// a test can assert which credentials the operator was sent to fix.
type fakeReporter struct {
	selectAnswer string
	inputAnswers []string

	selectTitles  []string
	selectOptions [][]string
	inputTitles   []string
}

func (r *fakeReporter) Select(title string, _ string, options []string) (string, error) {
	r.selectTitles = append(r.selectTitles, title)
	r.selectOptions = append(r.selectOptions, options)
	return r.selectAnswer, nil
}

func (r *fakeReporter) MultiSelect(string, []string) ([]string, error) { return nil, nil }

func (r *fakeReporter) ProgressModel(core.ModelProgress) {}

func (r *fakeReporter) Input(title string, _ string, _ string) (string, error) {
	r.inputTitles = append(r.inputTitles, title)
	if len(r.inputAnswers) == 0 {
		return "", fmt.Errorf("unexpected Input(%q)", title)
	}
	answer := r.inputAnswers[0]
	r.inputAnswers = r.inputAnswers[1:]
	return answer, nil
}

func newTestRuntime(reporter core.Reporter) *core.Runtime {
	return &core.Runtime{
		Reporter:         reporter,
		GlobalState:      core.NewGlobalState(),
		ActiveStageState: core.NewActiveStageState(),
	}
}

func sourceAuthError(registry string) error {
	return &oras.Error{
		Kind:     httpapi.ErrAuth,
		Op:       "prepare image artifact",
		Target:   "axem-solutions/shaide_control_panel",
		Registry: registry,
		Scope:    oras.ScopeSource,
	}
}

// The bug this guards: a 401 from ghcr.io was routed to the Harbor credential
// prompt, so entering a Harbor password was the only offered fix for a failure
// Harbor had no part in. The installer mirrors public images only, so there is
// no credential to enter for a source registry either.
func TestRecoverArtifactUploadSourceAuthOffersRetryAndAbortOnly(t *testing.T) {
	reporter := &fakeReporter{selectAnswer: "Retry"}
	rt := newTestRuntime(reporter)

	action, err := recoverArtifactUpload(rt, sourceAuthError("ghcr.io"))
	if err != nil {
		t.Fatalf("recoverArtifactUpload() error = %v", err)
	}
	if action != core.RecoveryRetryStep {
		t.Errorf("action = %v, want RecoveryRetryStep", action)
	}

	if len(reporter.inputTitles) != 0 {
		t.Errorf("prompted for credentials on a source-registry failure: %v", reporter.inputTitles)
	}
	if rt.Discovery.Auth.Password != "" {
		t.Errorf("Harbor credentials were overwritten by a source-registry failure")
	}

	if len(reporter.selectOptions) == 0 {
		t.Fatal("no prompt was shown")
	}
	want := []string{"Retry", "Abort"}
	got := reporter.selectOptions[0]
	if len(got) != len(want) {
		t.Fatalf("options = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("options = %v, want %v", got, want)
		}
	}
}

// The prompt has to say which registry refused and what to do about it,
// because neither is recoverable from inside the installer.
func TestRecoverArtifactUploadSourceAuthPromptIsActionable(t *testing.T) {
	reporter := &fakeReporter{selectAnswer: "Abort"}
	rt := newTestRuntime(reporter)

	if _, err := recoverArtifactUpload(rt, sourceAuthError("ghcr.io")); err != nil {
		t.Fatalf("recoverArtifactUpload() error = %v", err)
	}

	title := reporter.selectTitles[0]
	for _, want := range []string{"ghcr.io", "axem-solutions/shaide_control_panel", "publicly readable"} {
		if !strings.Contains(title, want) {
			t.Errorf("prompt %q does not mention %q", title, want)
		}
	}
	if strings.Contains(title, "Harbor") {
		t.Errorf("prompt %q blames Harbor for an upstream failure", title)
	}
}

// A registry with no special handling must behave the same way: the failure is
// upstream regardless of which registry it came from.
func TestRecoverArtifactUploadSourceAuthUniformAcrossRegistries(t *testing.T) {
	for _, registry := range []string{"docker.io", "auth.docker.io", "nvcr.io", "quay.io", "registry.k8s.io"} {
		t.Run(registry, func(t *testing.T) {
			reporter := &fakeReporter{selectAnswer: "Abort"}
			rt := newTestRuntime(reporter)

			action, err := recoverArtifactUpload(rt, sourceAuthError(registry))
			if err != nil {
				t.Fatalf("recoverArtifactUpload() error = %v", err)
			}
			if action != core.RecoveryFail {
				t.Errorf("action = %v, want RecoveryFail", action)
			}
			if len(reporter.inputTitles) != 0 {
				t.Errorf("prompted for credentials: %v", reporter.inputTitles)
			}
		})
	}
}

// Harbor push failures must keep their existing recovery path.
func TestRecoverArtifactUploadTargetAuthStillPromptsHarbor(t *testing.T) {
	reporter := &fakeReporter{
		selectAnswer: "Enter new credentials",
		inputAnswers: []string{"admin", "harbor-password"},
	}
	rt := newTestRuntime(reporter)

	targetErr := &oras.Error{
		Kind:     httpapi.ErrAuth,
		Op:       "push image artifact",
		Target:   "axem-solutions/shaide_server",
		Registry: "127.0.0.1:5000",
		Scope:    oras.ScopeTarget,
	}

	action, err := recoverArtifactUpload(rt, targetErr)
	if err != nil {
		t.Fatalf("recoverArtifactUpload() error = %v", err)
	}
	if action != core.RecoveryRetryStep {
		t.Errorf("action = %v, want RecoveryRetryStep", action)
	}
	if rt.Discovery.Auth.Password != "harbor-password" {
		t.Errorf("Harbor password = %q, want the entered value", rt.Discovery.Auth.Password)
	}
}
