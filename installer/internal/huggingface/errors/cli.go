package errors

import (
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
)

type CliError struct {
	RepoID   string
	ExitCode int
	Output   string
	Err      error
}

func (e *CliError) Error() string {
	return fmt.Sprintf("download %s failed with exit code %d: %s", e.RepoID, e.ExitCode, e.Output)
}

func (e *CliError) Unwrap() error {
	return e.Err
}

func classifyCLIError(err *CliError) ErrorKind {
	output := strings.ToLower(err.Output)

	switch {
	case strings.Contains(output, "401"),
		strings.Contains(output, "unauthorized"),
		strings.Contains(output, "invalid username or password"),
		strings.Contains(output, "invalid token"):
		return httpapi.ErrAuth

	case strings.Contains(output, "repository not found"),
		strings.Contains(output, "404"),
		strings.Contains(output, "not found"):
		return httpapi.ErrNotFound

	case strings.Contains(output, "429"),
		strings.Contains(output, "rate limit"),
		strings.Contains(output, "too many requests"):
		return httpapi.ErrRateLimited

	case strings.Contains(output, "network"),
		strings.Contains(output, "connection"),
		strings.Contains(output, "timeout"),
		strings.Contains(output, "temporary failure"),
		strings.Contains(output, "name resolution"),
		strings.Contains(output, "failed to resolve"),
		strings.Contains(output, "no route to host"):
		return httpapi.ErrNetwork
	}

	// For `hf download`, a generic exit status 1 is usually retryable:
	// network, transient HF error, interrupted transfer, etc.
	if err.ExitCode != 0 {
		return httpapi.ErrNetwork
	}

	return httpapi.ErrUnknown
}
