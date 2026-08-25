package artifact

import (
	"errors"
	"fmt"
	"os"

	harborerrors "github.com/axem-solutions/ai_platform/installer/internal/harbor/errors"
	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	huggingface "github.com/axem-solutions/ai_platform/installer/internal/huggingface/errors"
	oras "github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

func recoverCheckModelArtifacts(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	kind := oras.ClassifyError(runErr)
	errMsg := fmt.Sprintf("Harbor model check failed. %s", oras.UserMessage(kind))

	switch kind {
	case httpapi.ErrAuth:
		return recoverModelUploadAuth(rt, errMsg)
	case httpapi.ErrNetwork, httpapi.ErrRateLimited:
		return recoverRetryableModelUpload(rt, errMsg)
	default:
		return core.RecoveryFail, nil
	}
}

// Recovery actions for HuggingFace model download
func recoverDownloadModels(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	var hfErr *huggingface.Error
	if !errors.As(runErr, &hfErr) {
		return core.RecoveryFail, nil
	}

	cause := huggingface.UserMessage(hfErr.Kind)
	errMsg := fmt.Sprintf("Download failed for\n %s.%s", hfErr.RepoID, cause)

	switch hfErr.Kind {
	case httpapi.ErrAuth:
		return recoverHuggingFaceAuth(rt, errMsg)
	case httpapi.ErrNotFound:
		return recoverMissingModel(rt)
	case httpapi.ErrRateLimited, httpapi.ErrNetwork, huggingface.ErrCLIUnavailable, huggingface.ErrCache:
		return recoverRetryableDownload(rt, errMsg)
	default:
		return core.RecoveryFail, nil
	}

}

func recoverHuggingFaceAuth(rt *core.Runtime, title string) (core.RecoveryAction, error) {

	options := []string{
		"Enter new token",
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(
		title,
		"Enter new token",
		options,
	)
	if err != nil {
		return core.RecoveryFail, err
	}

	switch selected {
	case "Enter new token":
		token, err := rt.Reporter.Input(
			"Hugging Face token",
			"hf_...",
			"",
		)
		if err != nil {
			return core.RecoveryFail, err
		}
		rt.Bootstrap.Config.HuggingFace.Token = token
		return core.RecoveryRetryStep, nil
	case "Retry":
		return core.RecoveryRetryStep, nil
	default:
		return core.RecoveryFail, nil

	}
}

func recoverMissingModel(rt *core.Runtime) (core.RecoveryAction, error) {
	title := "Hugging Face model was not found."
	options := []string{
		"Add models manually",
		"Abort",
	}

	_, err := rt.Reporter.Select(
		title,
		"Abort",
		options,
	)
	if err != nil {
		return core.RecoveryFail, err
	}

	return core.RecoveryFail, nil
}

func recoverRetryableDownload(rt *core.Runtime, title string) (core.RecoveryAction, error) {
	options := []string{
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(
		title,
		"Retry",
		options,
	)
	if err != nil {
		return core.RecoveryFail, err
	}

	if selected == "Retry" {
		return core.RecoveryRetryStep, nil
	}

	return core.RecoveryFail, nil
}

// Recovery actions for HuggingFace model upload
func recoverArtifactUpload(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	var orasErr *oras.Error
	if !errors.As(runErr, &orasErr) {
		return core.RecoveryFail, nil
	}

	cause := oras.UserMessage(orasErr.Kind)
	errMsg := fmt.Sprintf("Upload failed for %s.\n %s", orasErr.Target, cause)

	switch orasErr.Kind {
	case httpapi.ErrAuth:
		return recoverModelUploadAuth(rt, errMsg)
	case httpapi.ErrNetwork, httpapi.ErrRateLimited:
		return recoverRetryableModelUpload(rt, errMsg)
	case oras.ErrUploadState:
		return recoverModelUploadState(rt, errMsg)
	case oras.ErrArtifactBuild, oras.ErrUnsupported, oras.ErrLocalModelCache, httpapi.ErrNotFound:
		return recoverUpload(rt, errMsg)
	default:
		return core.RecoveryFail, nil
	}
}

func recoverDeleteModels(rt *core.Runtime, runErr error) (core.RecoveryAction, error) {
	var harborErr *harborerrors.Error
	if !errors.As(runErr, &harborErr) {
		return core.RecoveryFail, nil
	}

	title := fmt.Sprintf(
		"Failed to %s in Harbor for %s.\n %s",
		harborErr.Op,
		harborErrorTarget(harborErr),
		harborerrors.UserMessage(harborErr.Kind),
	)
	options := []string{
		"Skip delete models",
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(
		title,
		options[0],
		options,
	)
	if err != nil {
		return core.RecoveryFail, err
	}

	switch selected {
	case "Skip delete models":
		return core.RecoveryContinue, nil
	case "Retry":
		return core.RecoveryRetryStep, nil
	default:
		return core.RecoveryFail, nil
	}
}

func harborErrorTarget(err *harborerrors.Error) string {
	if err.Project == "" {
		return "the requested resource"
	}
	if err.Repository == "" {
		return fmt.Sprintf("project %s", err.Project)
	}
	return fmt.Sprintf("repository %s/%s", err.Project, err.Repository)
}

func recoverModelUploadAuth(rt *core.Runtime, title string) (core.RecoveryAction, error) {
	options := []string{
		"Enter new credentials",
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(
		title,
		options[0],
		options,
	)
	if err != nil {
		return core.RecoveryFail, err
	}

	switch selected {
	case "Enter new credentials":
		username, err := rt.Reporter.Input("Harbor username", "admin", rt.Discovery.Auth.Username)
		if err != nil {
			return core.RecoveryFail, err
		}

		password, err := rt.Reporter.Input("Harbor password", "", "")
		if err != nil {
			return core.RecoveryFail, err
		}

		rt.Discovery.Auth.Username = username
		rt.Discovery.Auth.Password = password
		return core.RecoveryRetryStep, nil
	case "Retry":
		return core.RecoveryRetryStep, nil
	default:
		return core.RecoveryFail, nil
	}
}

func recoverRetryableModelUpload(rt *core.Runtime, title string) (core.RecoveryAction, error) {
	options := []string{
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(
		title,
		options[0],
		options,
	)
	if err != nil {
		return core.RecoveryFail, err
	}

	if selected == "Retry" {
		return core.RecoveryRetryStep, nil
	}

	return core.RecoveryFail, nil
}

func recoverModelUploadState(rt *core.Runtime, title string) (core.RecoveryAction, error) {
	options := []string{
		"Clear upload state and retry",
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(
		title,
		options[0],
		options,
	)
	if err != nil {
		return core.RecoveryFail, err
	}

	switch selected {
	case "Clear upload state and retry":
		if err := os.RemoveAll(rt.Bootstrap.Config.Paths.UploadState); err != nil {
			return core.RecoveryFail, err
		}
		return core.RecoveryRetryStep, nil

	case "Retry":
		return core.RecoveryRetryStep, nil

	default:
		return core.RecoveryFail, nil
	}
}

func recoverUpload(rt *core.Runtime, title string) (core.RecoveryAction, error) {
	options := []string{
		"Retry",
		"Abort",
	}

	selected, err := rt.Reporter.Select(
		title,
		options[0],
		options,
	)

	if err != nil {
		return core.RecoveryFail, err
	}

	if selected == "Retry" {
		return core.RecoveryRetryStep, nil
	}

	return core.RecoveryFail, nil
}
