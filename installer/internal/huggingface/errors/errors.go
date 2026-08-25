package errors

import (
	stderrors "errors"
	"fmt"
	"os/exec"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
)

type ErrorKind = httpapi.ErrorKind

const (
	ErrCLIUnavailable ErrorKind = "cli_unavailable"
	ErrCache          ErrorKind = "cache"
)

type Error struct {
	Kind     ErrorKind
	Op       string
	RepoID   string
	Revision string
	Err      error
}

func (e *Error) Error() string {
	target := e.RepoID
	if e.Revision != "" {
		target = fmt.Sprintf("%s@%s", e.RepoID, e.Revision)
	}

	if target != "" {
		if e.Err != nil {
			return fmt.Sprintf("huggingface %s failed for %s: %v", e.Op, target, e.Err)
		}
		return fmt.Sprintf("huggingface %s failed for %s: %v", e.Op, target, e.Err)
	}

	if e.Err != nil {
		return fmt.Sprintf("huggingface %s failed: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("huggingface %s failed: %v", e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func ClassifyError(err error) ErrorKind {
	if stderrors.Is(err, exec.ErrNotFound) {
		return ErrCLIUnavailable
	}
	var cliErr *CliError
	if stderrors.As(err, &cliErr) {
		return classifyCLIError(cliErr)
	}

	return ErrorKind(httpapi.ClassifyError(err))
}

func UserMessage(kind ErrorKind) string {
	switch kind {
	case httpapi.ErrAuth:
		return "Hugging Face token is invalid or has no access"
	case httpapi.ErrNotFound:
		return "Hugging Face model was not found"
	case httpapi.ErrRateLimited:
		return "Hugging Face API rate limit was reached"
	case httpapi.ErrNetwork:
		return "Hugging Face API is unreachable"
	case ErrCLIUnavailable:
		return "Hugging Face CLI was not found"
	case ErrCache:
		return "Hugging Face cache or disk error"
	default:
		return "Hugging Face request failed"
	}
}
