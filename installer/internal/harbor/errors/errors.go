package errors

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
)

type Error struct {
	Kind       httpapi.ErrorKind
	Op         string
	Project    string
	Repository string
	Err        error
}

func (e *Error) Error() string {
	target := e.Project
	if e.Repository != "" {
		target = fmt.Sprintf("%s/%s", e.Project, e.Repository)
	}

	if target != "" {
		return fmt.Sprintf("harbor %s failed for %s: %v", e.Op, target, e.Err)
	}

	return fmt.Sprintf("harbor %s failed: %v", e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func ClassifyError(err error) httpapi.ErrorKind {
	return httpapi.ClassifyError(err)
}

func UserMessage(kind httpapi.ErrorKind) string {
	switch kind {
	case httpapi.ErrAuth:
		return "Harbor credentials are invalid or do not have API access"
	case httpapi.ErrNotFound:
		return "Harbor resource was not found"
	case httpapi.ErrRateLimited:
		return "Harbor API rate limit was reached"
	case httpapi.ErrNetwork:
		return "Harbor API is unreachable"
	default:
		return "Harbor API request failed"
	}
}
