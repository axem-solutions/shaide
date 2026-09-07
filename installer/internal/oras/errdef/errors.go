package errdef

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

type ErrorKind = httpapi.ErrorKind

const (
	ErrUploadState     ErrorKind = "upload_state"
	ErrLocalModelCache ErrorKind = "local_model_cache"
	ErrArtifactBuild   ErrorKind = "artifact_build"
	ErrUnsupported     ErrorKind = "unsupported"
)

var (
	ErrUploadStateFailure     = errors.New("upload verification failure")
	ErrLocalModelCacheFailure = errors.New("local model cache failure")
	ErrArtifactBuildFailure   = errors.New("artifact build failure")
)

// Scope says which side of a mirror failed. Every upload both pulls from an
// upstream registry and pushes to Harbor over the same oras client, so without
// this an auth failure on either leg looks identical — and a source-side 401
// was reported to the operator as a Harbor credential problem, offering a
// Harbor password prompt that could never fix it.
type Scope string

const (
	// ScopeTarget is the Harbor registry the installer pushes into.
	ScopeTarget Scope = "target"

	// ScopeSource is the upstream registry an image is pulled from.
	ScopeSource Scope = "source"

	// ScopeLocal is a failure with no registry behind it, such as a bad
	// archive or stale upload state.
	ScopeLocal Scope = "local"
)

type Error struct {
	Kind ErrorKind
	Op   string
	// Target is the artifact being uploaded, not a registry.
	Target string

	// Registry is the host that returned the failure, empty when the failure
	// did not come from a registry request.
	Registry string
	Scope    Scope

	Err error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("oras %s failed for %s: %v", e.Op, e.Target, e.Err)
	}

	return fmt.Sprintf("oras %s failed for %s", e.Op, e.Target)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// RegistryHost reports the host that produced an HTTP failure, or "" when the
// error did not come from a registry request.
//
// Both error types carry the request URL, so the failing registry is recovered
// from the error itself rather than threaded through every call site. Note the
// URL can belong to a registry's token service rather than the registry itself
// (Docker Hub answers challenges from auth.docker.io), so callers matching a
// host to credentials must treat those as the same registry.
func RegistryHost(err error) string {
	var respErr *errcode.ErrorResponse
	if errors.As(err, &respErr) && respErr.URL != nil {
		return respErr.URL.Host
	}

	var statusErr *httpapi.StatusError
	if errors.As(err, &statusErr) {
		if parsed, parseErr := url.Parse(statusErr.URL); parseErr == nil {
			return parsed.Host
		}
	}

	// A transport failure never reaches a registry response, but the request
	// URL survives on the *url.Error. Without this a dropped connection is
	// unattributed and reads as a Harbor problem even when the source registry
	// was the side that went away.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if parsed, parseErr := url.Parse(urlErr.URL); parseErr == nil {
			return parsed.Host
		}
	}

	return ""
}

// UserMessage describes a failure against Harbor. Callers holding an *Error
// should use its UserMessage method instead, which also handles source
// registries.
func UserMessage(kind ErrorKind) string {
	return targetMessage(kind)
}

// UserMessage describes the failure in terms of the registry that caused it.
func (e *Error) UserMessage() string {
	if e.Scope == ScopeSource {
		return sourceMessage(e.Kind, e.RegistryLabel())
	}

	return targetMessage(e.Kind)
}

// RegistryLabel names the failing registry for display, falling back to a
// generic phrase when the host could not be determined.
func (e *Error) RegistryLabel() string {
	if e.Registry == "" {
		return "the source registry"
	}

	return e.Registry
}

func targetMessage(kind ErrorKind) string {
	switch kind {
	case httpapi.ErrAuth:
		return "Harbor credentials are invalid or do not have push access"
	case httpapi.ErrNotFound:
		return "Harbor project or repository was not found"
	case httpapi.ErrRateLimited:
		return "Harbor registry rate limit was reached"
	case httpapi.ErrNetwork:
		return "Harbor registry is unreachable"
	case ErrUploadState:
		return "upload resume state is stale or could not be used"
	case ErrLocalModelCache:
		return "local model cache is missing or incomplete"
	case ErrArtifactBuild:
		return "model artifact could not be prepared for upload"
	case ErrUnsupported:
		return "Harbor registry does not support this model upload"
	default:
		return "artifact upload failed"
	}
}

// sourceMessage describes a failure pulling the image, before Harbor is
// involved at all. Harbor credentials are never the cause here.
func sourceMessage(kind ErrorKind, registry string) string {
	switch kind {
	case httpapi.ErrAuth:
		// The installer mirrors public images only, so a refused pull means
		// the image is not published publicly rather than that a credential
		// is wrong.
		return fmt.Sprintf("%s refused the pull: the image is not publicly readable", registry)
	case httpapi.ErrNotFound:
		return fmt.Sprintf("%s has no such repository or tag", registry)
	case httpapi.ErrRateLimited:
		return fmt.Sprintf("%s rate limit was reached", registry)
	case httpapi.ErrNetwork:
		return fmt.Sprintf("%s is unreachable", registry)
	default:
		return fmt.Sprintf("the image could not be read from %s", registry)
	}
}

// Copied from Oras SDK:
// maxErrorBytes specifies the default limit on how many response bytes are
// allowed in the server's error response.
// A typical error message is around 200 bytes. Hence, 8 KiB should be
// sufficient.
const maxErrorBytes int64 = 8 * 1024 // 8 KiB

// ParseErrorResponse parses the error returned by the remote registry.
func ParseErrorResponse(resp *http.Response) error {
	resultErr := &errcode.ErrorResponse{
		Method:     resp.Request.Method,
		URL:        resp.Request.URL,
		StatusCode: resp.StatusCode,
	}
	var body struct {
		Errors errcode.Errors `json:"errors"`
	}
	lr := io.LimitReader(resp.Body, maxErrorBytes)
	if err := json.NewDecoder(lr).Decode(&body); err == nil {
		resultErr.Errors = body.Errors
	}
	return resultErr
}
