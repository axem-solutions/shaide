package pulumi

import (
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

func stepNamed(t *testing.T, name string) core.Step {
	t.Helper()

	for _, step := range Stage().Steps {
		if step.Name == name {
			return step
		}
	}

	t.Fatalf("stage has no step named %q", name)
	return core.Step{}
}

func runtimeWithSelection(serve, uninstall []catalog.Model) *core.Runtime {
	rt := &core.Runtime{
		GlobalState:      core.NewGlobalState(),
		ActiveStageState: core.NewActiveStageState(),
	}
	rt.Models.Serve = serve
	rt.Models.Uninstall = uninstall

	return rt
}

// A cluster that serves no models, and has none to remove, must not reach the
// app-serving stack: it refuses to configure without one, and that failure
// ends the whole run before App-Shaide and Monitoring are deployed.
func TestAppServingRunsOnlyWithSomethingToDo(t *testing.T) {
	step := stepNamed(t, "Deploy App-Serving ")

	if step.When == nil {
		t.Fatal("the app-serving step has no condition; it would run with nothing to serve")
	}

	model := catalog.Model{ID: "nomic-ai/nomic-embed-text-v1.5"}

	tests := []struct {
		name      string
		serve     []catalog.Model
		uninstall []catalog.Model
		want      bool
	}{
		{name: "nothing selected", want: false},
		{name: "empty selection", serve: []catalog.Model{}, uninstall: []catalog.Model{}, want: false},
		{name: "one model to serve", serve: []catalog.Model{model}, want: true},
		{name: "last model uninstalled", uninstall: []catalog.Model{model}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := step.When(runtimeWithSelection(test.serve, test.uninstall)); got != test.want {
				t.Errorf("When() = %v, want %v", got, test.want)
			}
		})
	}
}

// Skipping serving must not skip the rest of the platform: those stacks are
// what a cluster with no on-cluster models still needs.
func TestOtherStacksAreNotGatedOnModels(t *testing.T) {
	for _, name := range []string{"Deploy Gateway Provider", "Deploy App-Shaide ", "Deploy Monitoring"} {
		t.Run(name, func(t *testing.T) {
			if step := stepNamed(t, name); step.When != nil {
				t.Errorf("%q is gated; it should run regardless of the model selection", name)
			}
		})
	}
}
