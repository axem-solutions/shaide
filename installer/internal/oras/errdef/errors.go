package errdef

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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

type Error struct {
	Kind   ErrorKind
	Op     string
	Target string
	Err    error
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

func UserMessage(kind ErrorKind) string {
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
