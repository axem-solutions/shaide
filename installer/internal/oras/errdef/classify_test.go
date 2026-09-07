package errdef

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// A dropped connection used to classify as unknown, which the artifact stage
// treats as fatal: the run aborted instead of offering a retry, discarding
// upload state that could have resumed the blob.
func TestClassifyErrorNetworkFailures(t *testing.T) {
	patchURL := "http://127.0.0.1:5000/v2/services/llm-d/llm-d-cuda/blobs/uploads/7d77f287"

	tests := []struct {
		name string
		err  error
	}{
		{
			// The exact shape from a port-forward dying mid-upload: the PATCH
			// fails with EOF, and the status check that follows cannot dial.
			name: "chunk patch eof then refused status check",
			err: fmt.Errorf("%w; recover failed chunk: %v",
				fmt.Errorf("upload blob chunk: %w", &url.Error{Op: "Patch", URL: patchURL, Err: io.EOF}),
				fmt.Errorf("check upload status: %w", &url.Error{
					Op:  "Get",
					URL: patchURL,
					Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
				})),
		},
		{
			name: "bare eof",
			err:  fmt.Errorf("copy source chunk: %w", io.EOF),
		},
		{
			name: "unexpected eof",
			err:  fmt.Errorf("copy source chunk: %w", io.ErrUnexpectedEOF),
		},
		{
			name: "connection reset",
			err:  fmt.Errorf("push: %w", &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET}),
		},
		{
			name: "broken pipe",
			err:  fmt.Errorf("push: %w", syscall.EPIPE),
		},
		{
			name: "dial timeout",
			err:  &url.Error{Op: "Get", URL: patchURL, Err: &net.OpError{Op: "dial", Err: syscall.ETIMEDOUT}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyError(test.err); got != httpapi.ErrNetwork {
				t.Errorf("ClassifyError() = %q, want %q", got, httpapi.ErrNetwork)
			}
		})
	}
}

// A registry that answered must still be classified by its answer: the network
// check runs last so it cannot mask an auth or not-found response.
func TestClassifyErrorRegistryResponseWins(t *testing.T) {
	tokenURL, err := url.Parse("https://ghcr.io/token?scope=repository:x:pull")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{
			name: "unauthorized response",
			err: fmt.Errorf("resolve: %w", &errcode.ErrorResponse{
				Method: "GET", URL: tokenURL, StatusCode: 401,
			}),
			want: httpapi.ErrAuth,
		},
		{
			name: "not found response",
			err: fmt.Errorf("resolve: %w", &errcode.ErrorResponse{
				Method: "GET", URL: tokenURL, StatusCode: 404,
			}),
			want: httpapi.ErrNotFound,
		},
		{
			name: "server error is still network",
			err: fmt.Errorf("resolve: %w", &errcode.ErrorResponse{
				Method: "GET", URL: tokenURL, StatusCode: 503,
			}),
			want: httpapi.ErrNetwork,
		},
		{
			name: "upload state failure",
			err:  fmt.Errorf("%w: save upload state", ErrUploadStateFailure),
			want: ErrUploadState,
		},
		{
			name: "plain error stays unknown",
			err:  fmt.Errorf("registry offset moved backwards: got=1 start=2"),
			want: httpapi.ErrUnknown,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyError(test.err); got != test.want {
				t.Errorf("ClassifyError() = %q, want %q", got, test.want)
			}
		})
	}
}

// The failing host is recoverable from a transport error too, so a dropped
// connection is attributed to the registry that went away.
func TestRegistryHostFromTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "harbor port-forward",
			err: &url.Error{
				Op:  "Patch",
				URL: "http://127.0.0.1:5000/v2/services/llm-d/llm-d-cuda/blobs/uploads/x",
				Err: io.EOF,
			},
			want: "127.0.0.1:5000",
		},
		{
			name: "source registry",
			err:  fmt.Errorf("copy source chunk: %w", &url.Error{Op: "Get", URL: "https://ghcr.io/v2/x/blobs/sha256:y", Err: io.EOF}),
			want: "ghcr.io",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RegistryHost(test.err); got != test.want {
				t.Errorf("RegistryHost() = %q, want %q", got, test.want)
			}
		})
	}
}
