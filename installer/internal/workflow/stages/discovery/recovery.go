package discovery

import (
	"errors"
	"fmt"

	harborerrors "github.com/axem-solutions/ai_platform/installer/internal/harbor/errors"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

func recoverCheckResources(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	var discoveryErr harborDiscoveryError
	if !errors.As(runErr, &discoveryErr) {
		return core.RecoveryFail, nil
	}

	switch discoveryErr.Kind {
	case harborNamespace:
		return recoverHarborNamespace(rt)
	case harborService:
		return recoverHarborService(rt)
	case harborSecret:
		return recoverHarborSecret(rt)
	default:
		return core.RecoveryFail, nil
	}
}

func recoverHarborNamespace(rt *core.Runtime) (core.RecoveryAction, error) {
	msg := fmt.Sprintf(
		"Harbor namespace %q was not found",
		rt.Bootstrap.Config.Harbor.Namespace,
	)
	options := []string{
		"Fresh install",
		"Choose another namespace",
		"Abort",
	}

	selected, promptErr := rt.Reporter.Select(msg, "Fresh install", options)
	if promptErr != nil {
		return core.RecoveryFail, promptErr
	}

	switch selected {
	case "Fresh install":
		rt.Discovery.Mode = core.Install
		return core.RecoveryContinue, nil

	case "Choose another namespace":
		ns, inputErr := rt.Reporter.Input(
			"Harbor namespace",
			"harbor",
			rt.Bootstrap.Config.Harbor.Namespace,
		)
		if inputErr != nil {
			return core.RecoveryFail, inputErr
		}

		rt.Bootstrap.Config.Harbor.Namespace = ns
		return core.RecoveryRetryStep, nil

	default:
		return core.RecoveryFail, nil
	}
}

func recoverHarborService(rt *core.Runtime) (core.RecoveryAction, error) {
	msg := fmt.Sprintf(
		"Harbor service %q is not usable in namespace %q",
		rt.Bootstrap.Config.Harbor.Service,
		rt.Bootstrap.Config.Harbor.Namespace,
	)
	options := []string{
		"Enter service name",
		"Choose another namespace",
		"Fresh install",
		"Abort",
	}

	selected, promptErr := rt.Reporter.Select(msg, "Enter service name", options)

	if promptErr != nil {
		return core.RecoveryFail, promptErr
	}

	switch selected {
	case "Enter service name":
		svc, inputErr := rt.Reporter.Input("Harbor service", "harbor", rt.Bootstrap.Config.Harbor.Service)
		if inputErr != nil {
			return core.RecoveryFail, inputErr
		}

		rt.Bootstrap.Config.Harbor.Service = svc
		return core.RecoveryRetryStep, nil

	case "Choose another namespace":
		ns, inputErr := rt.Reporter.Input("Harbor namespace", "harbor", rt.Bootstrap.Config.Harbor.Namespace)
		if inputErr != nil {
			return core.RecoveryFail, inputErr
		}

		rt.Bootstrap.Config.Harbor.Namespace = ns
		return core.RecoveryRetryStep, nil

	case "Fresh install":
		rt.Discovery.Mode = core.Install
		return core.RecoveryContinue, nil

	default:
		return core.RecoveryFail, nil
	}
}

func recoverHarborSecret(rt *core.Runtime) (core.RecoveryAction, error) {
	msg := fmt.Sprintf(
		"Harbor secret %q is not usable in namespace %q",
		rt.Bootstrap.Config.Harbor.PullSecret,
		rt.Bootstrap.Config.Harbor.Namespace,
	)
	options := []string{
		"Enter secret name",
		"Choose another namespace",
		"Fresh install",
		"Abort",
	}

	selected, promptErr := rt.Reporter.Select(msg, "Enter secret name", options)
	if promptErr != nil {
		return core.RecoveryFail, promptErr
	}

	switch selected {
	case "Enter secret name":
		secret, inputErr := rt.Reporter.Input(
			"Harbor pull secret",
			"harbor-pull-secret",
			rt.Bootstrap.Config.Harbor.PullSecret,
		)
		if inputErr != nil {
			return core.RecoveryFail, inputErr
		}

		rt.Bootstrap.Config.Harbor.PullSecret = secret
		return core.RecoveryRetryStep, nil

	case "Choose another namespace":
		ns, inputErr := rt.Reporter.Input(
			"Harbor namespace",
			"harbor",
			rt.Bootstrap.Config.Harbor.Namespace,
		)
		if inputErr != nil {
			return core.RecoveryFail, inputErr
		}

		rt.Bootstrap.Config.Harbor.Namespace = ns
		return core.RecoveryRetryStep, nil

	case "Fresh install":
		rt.Discovery.Mode = core.Install
		return core.RecoveryContinue, nil

	default:
		return core.RecoveryFail, nil
	}
}

func recoverScrapeHarbor(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	var harborErr *harborerrors.Error
	if !errors.As(runErr, &harborErr) {
		return core.RecoveryFail, nil
	}

	target := harborErr.Project
	if harborErr.Repository != "" {
		target = fmt.Sprintf("%s/%s", harborErr.Project, harborErr.Repository)
	}

	title := fmt.Sprintf(
		"Could not %s from Harbor %q: %s",
		harborErr.Op,
		target,
		harborerrors.UserMessage(harborErr.Kind),
	)
	options := []string{
		"Retry",
		"Continue without Harbor models",
		"Fresh install",
		"Abort",
	}

	selected, promptErr := rt.Reporter.Select(title, "Retry", options)
	if promptErr != nil {
		return core.RecoveryFail, promptErr
	}

	switch selected {
	case "Retry":
		return core.RecoveryRetryStep, nil

	case "Continue without Harbor models":
		return core.RecoveryContinue, nil

	case "Fresh install":
		rt.Discovery.Mode = core.Install
		return core.RecoveryContinue, nil

	default:
		return core.RecoveryFail, nil
	}

}

func recoverPreloadHarbor(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	options := []string{
		"Retry",
		"Abort",
	}

	selected, promptErr := rt.Reporter.Select("Harbor image preload failed", "Retry", options)
	if promptErr != nil {
		return core.RecoveryFail, promptErr
	}

	if selected == "Retry" {
		return core.RecoveryRetryStep, nil
	}

	return core.RecoveryFail, nil
}
