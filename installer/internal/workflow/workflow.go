package workflow

import (
	"fmt"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/installer/internal/ui"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/artifact"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/bootstrap"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/discovery"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/kubernetes"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/pulumi"
)

func Run() error {
	tui := ui.New()
	log := logger.NewWithWriter(tui.Writer)
	rt := core.NewContext(log, tui.Reporter)

	go func() {
		err := execution(rt, defaultStages())
		tui.SendDone(err)
		tui.CloseLogs()
	}()

	return tui.Run()
}

func defaultStages() []core.Stage {
	return []core.Stage{
		bootstrap.Stage(),
		kubernetes.Stage(),
		discovery.Stage(),
		artifact.Stage(),
		pulumi.Stage(),
	}
}

func execution(rt *core.Runtime, stages []core.Stage) error {
	for _, stage := range stages {
		if err := runStage(rt, stage); err != nil {
			return fmt.Errorf("%s: %w", stage.Name, err)
		}
	}

	return nil
}

func runStage(rt *core.Runtime, stage core.Stage) (err error) {
	stageStartedAt := time.Now()
	runner := newRunner(rt, stage)

	runner.begin()

	defer func() {
		cleanupErr := runner.cleanup()
		runner.reset()

		if err == nil && cleanupErr != nil {
			err = cleanupErr
		}
	}()

	for runner.hasNextStep() {
		if err := runner.runCurrentStep(); err != nil {
			return err
		}
	}

	rt.StageDone(stage.Name, time.Since(stageStartedAt))
	return nil
}

type Runner struct {
	rt        *core.Runtime
	stage     core.Stage
	stepIndex int
}

func newRunner(rt *core.Runtime, stage core.Stage) *Runner {
	return &Runner{
		rt:    rt,
		stage: stage,
	}
}

func (r *Runner) begin() {
	var data any
	if r.stage.NewState != nil {
		data = r.stage.NewState()
	}

	r.rt.Begin(r.stage.Name, data, len(r.stage.Steps))
	r.rt.StageStart(r.stage.Name, len(r.stage.Steps))
}

func (r *Runner) cleanup() error {
	if r.stage.Cleanup == nil {
		return nil
	}

	if err := r.stage.Cleanup(r.rt); err != nil {
		return fmt.Errorf("cleanup stage %q: %w", r.stage.Name, err)
	}

	return nil
}

func (r *Runner) reset() {
	r.rt.Reset()
}

func (r *Runner) hasNextStep() bool {
	return r.stepIndex < len(r.stage.Steps)
}

func (r *Runner) runCurrentStep() error {
	step := r.stage.Steps[r.stepIndex]
	stepStartedAt := time.Now()

	r.rt.StepStart(r.stepIndex+1, len(r.stage.Steps), step.Name)

	if step.When != nil && !step.When(r.rt) {
		r.rt.Detailf("skipped")
		r.stepIndex++
		return nil
	}

	if err := step.Run(r.rt); err != nil {
		r.rt.Errorf("%v", err)
		return r.recoverStep(step, err)
	}

	r.rt.StepDone(time.Since(stepStartedAt))
	r.stepIndex++
	return nil
}

func (r *Runner) recoverStep(step core.Step, runErr error) error {
	if step.Recover == nil {
		return fmt.Errorf("%s: %w", step.Name, runErr)
	}

	action, recoverErr := step.Recover(r.rt, runErr)
	if recoverErr != nil {
		return fmt.Errorf("%s: %w", step.Name, recoverErr)
	}

	switch action {
	case core.RecoveryContinue:
		r.rt.Detailf("continue %s after error:", step.Name)
		r.stepIndex++
		return nil

	case core.RecoveryRetryStep:
		r.rt.Detailf("retry step %s", step.Name)
		return nil

	case core.RecoveryRestartStage:
		r.rt.Detailf("retry stage %s", r.stage.Name)
		if err := r.cleanup(); err != nil {
			return err
		}

		r.reset()
		r.begin()
		r.stepIndex = 0
		return nil

	case core.RecoveryFail:
		return fmt.Errorf("%s: %w", step.Name, runErr)

	default:
		return fmt.Errorf(
			"%s: unknown recovery action %d after error: %w",
			step.Name,
			action,
			runErr,
		)
	}
}
