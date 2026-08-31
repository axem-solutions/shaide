package pulumi

import (
	"context"
	"errors"
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

func RecoverAppServing(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	return recoverStackError(rt, runErr, stackAppServing)
}

func RecoverGatewayProvider(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	return recoverStackError(rt, runErr, stackGatewayProvider)
}

func RecoverAppShaide(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	return recoverStackError(rt, runErr, stackAppShaide)
}

func RecoverHarbor(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	return recoverStackError(rt, runErr, stackCloudHarbor)
}

func RecoverMonitoring(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	return recoverStackError(rt, runErr, stackMonitoring)
}

func recoverStackError(rt *core.Runtime, runErr error, target stackTarget) (core.RecoveryAction, error) {
	rt.Detailf("%s failed: %s", displayStack(target), runErr)

	var lockedErr *iac.StackLockedError
	if errors.As(runErr, &lockedErr) {
		return recoverRetryAbort(
			rt,
			fmt.Sprintf("%s stack is locked after automatic cancel retry.", displayStack(target)),
		)
	}

	var timeoutErr *iac.OperationTimeoutError
	if errors.As(runErr, &timeoutErr) {
		return recoverRetryAbort(
			rt,
			fmt.Sprintf("%s Pulumi operation timed out.", displayStack(target)),
		)
	}

	var programErr *iac.ProgramError
	if errors.As(runErr, &programErr) {
		return recoverProgramError(rt, target, programErr)
	}

	if errors.Is(runErr, context.Canceled) {
		rt.Detailf("%s deployment was canceled", displayStack(target))
		return core.RecoveryFail, nil
	}

	return recoverRetryAbort(
		rt,
		fmt.Sprintf("%s Pulumi deployment failed. See Logs for details.", displayStack(target)),
	)
}

func recoverProgramError(rt *core.Runtime, target stackTarget, err *iac.ProgramError) (core.RecoveryAction, error) {
	switch err.Kind {
	case iac.ProgramCompilation:
		rt.Detailf("%s Pulumi program failed to compile", displayStack(target))
		return core.RecoveryFail, nil

	case iac.ProgramRuntime:
		rt.Detailf("%s Pulumi program failed at runtime", displayStack(target))
		return core.RecoveryFail, nil

	case iac.EngineUnexpected:
		rt.Detailf("%s Pulumi engine failed unexpectedly", displayStack(target))
		return core.RecoveryFail, nil

	default:
		rt.Detailf("%s Pulumi program failed", displayStack(target))
		return core.RecoveryFail, nil
	}
}

func recoverRetryAbort(rt *core.Runtime, title string) (core.RecoveryAction, error) {
	options := []string{
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(title, options[0], options)
	if err != nil {
		return core.RecoveryFail, err
	}

	if selected == "Retry" {
		return core.RecoveryRetryStep, nil
	}

	return core.RecoveryFail, nil
}

type stackTarget string

const (
	stackAppServing      stackTarget = "app-serving"
	stackGatewayProvider stackTarget = "gateway-provider"
	stackAppShaide       stackTarget = "app-shaide"
	stackCloudHarbor     stackTarget = "harbor"
	stackMonitoring      stackTarget = "monitoring"
)

func displayStack(target stackTarget) string {
	switch target {
	case stackAppServing:
		return "App-Serving"
	case stackGatewayProvider:
		return "Gateway Provider"
	case stackAppShaide:
		return "App-Shaide"
	case stackCloudHarbor:
		return "Harbor"
	case stackMonitoring:
		return "Monitoring"
	default:
		return string(target)
	}
}
