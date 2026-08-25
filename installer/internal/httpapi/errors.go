package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
)

type StatusError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (e *StatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s %s returned %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
	}
	return fmt.Sprintf("%s %s returned %d", e.Method, e.URL, e.StatusCode)
}

func ReadStatusError(resp *http.Response) *StatusError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &StatusError{
		StatusCode: resp.StatusCode,
		Method:     resp.Request.Method,
		URL:        resp.Request.URL.String(),
		Body:       strings.TrimSpace(string(body)),
	}
}

type ErrorKind string

const (
	ErrAuth        ErrorKind = "auth"
	ErrNotFound    ErrorKind = "not_found"
	ErrRateLimited ErrorKind = "rate_limited"
	ErrNetwork     ErrorKind = "network"
	ErrUnknown     ErrorKind = "unknown"
)

func ClassifyError(err error) ErrorKind {
	if errors.Is(err, syscall.EPIPE) {
		return ErrNetwork
	}

	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return ErrAuth
		case http.StatusNotFound:
			return ErrNotFound
		case http.StatusTooManyRequests:
			return ErrRateLimited
		default:
			return ErrNetwork
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrNetwork
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return ErrNetwork
	}

	return ErrUnknown
}
