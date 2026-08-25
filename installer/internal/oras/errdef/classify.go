package errdef

import (
	"context"
	"errors"
	"net/http"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	oraserrdef "oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func ClassifyError(err error) ErrorKind {
	if errors.Is(err, ErrUploadStateFailure) {
		return ErrUploadState
	}
	if errors.Is(err, ErrLocalModelCacheFailure) {
		return ErrLocalModelCache
	}
	if errors.Is(err, ErrArtifactBuildFailure) {
		return ErrArtifactBuild
	}

	var respErr *errcode.ErrorResponse
	if errors.As(err, &respErr) {
		if kind := classifyRegistryErrors(respErr.Errors); kind != "" {
			return kind
		}
		switch respErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return httpapi.ErrAuth
		case http.StatusNotFound:
			return httpapi.ErrNotFound
		case http.StatusTooManyRequests:
			return httpapi.ErrRateLimited
		case http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType:
			return ErrUnsupported
		default:
			if respErr.StatusCode >= 500 {
				return httpapi.ErrNetwork
			}
			return httpapi.ErrUnknown
		}
	}
	switch {
	case errors.Is(err, oraserrdef.ErrInvalidReference):
		return httpapi.ErrNotFound
	case errors.Is(err, oraserrdef.ErrNotFound):
		return httpapi.ErrNotFound
	case errors.Is(err, oraserrdef.ErrUnsupported), errors.Is(err, oraserrdef.ErrUnsupportedVersion):
		return ErrUnsupported
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return httpapi.ErrNetwork
	}
	return httpapi.ErrUnknown
}

func classifyRegistryErrors(registryErrors errcode.Errors) ErrorKind {
	for _, registryErr := range registryErrors {
		switch registryErr.Code {
		case errcode.ErrorCodeUnauthorized,
			errcode.ErrorCodeDenied:
			return httpapi.ErrAuth

		case errcode.ErrorCodeNameUnknown,
			errcode.ErrorCodeManifestUnknown,
			errcode.ErrorCodeBlobUnknown:
			return httpapi.ErrNotFound

		case errcode.ErrorCodeNameInvalid:
			return httpapi.ErrNotFound

		case errcode.ErrorCodeBlobUploadInvalid,
			errcode.ErrorCodeBlobUploadUnknown:
			return ErrUploadState

		case errcode.ErrorCodeDigestInvalid,
			errcode.ErrorCodeSizeInvalid,
			errcode.ErrorCodeManifestInvalid,
			errcode.ErrorCodeManifestBlobUnknown:
			return ErrArtifactBuild

		case errcode.ErrorCodeUnsupported:
			return ErrUnsupported
		}
	}

	return ""
}
