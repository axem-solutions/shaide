package errdef

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func TestRegistryHost(t *testing.T) {
	tokenURL, err := url.Parse("https://ghcr.io/token?scope=repository%3Aaxem-solutions%2Fshaide_control_panel%3Apull")
	if err != nil {
		t.Fatalf("parse token url: %v", err)
	}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "registry error response",
			err:  &errcode.ErrorResponse{Method: "GET", URL: tokenURL, StatusCode: 401},
			want: "ghcr.io",
		},
		{
			// The uploader never sees a bare registry error: oras wraps it in
			// the manifest request that triggered the token fetch.
			name: "wrapped registry error response",
			err: fmt.Errorf("resolve remote image ref %q: %w", "ghcr",
				fmt.Errorf("HEAD %q: %w", "https://ghcr.io/v2/x/manifests/dev",
					&errcode.ErrorResponse{Method: "GET", URL: tokenURL, StatusCode: 401})),
			want: "ghcr.io",
		},
		{
			name: "http status error",
			err:  &httpapi.StatusError{StatusCode: 401, Method: "GET", URL: "http://127.0.0.1:5000/v2/shaide/x/manifests/dev"},
			want: "127.0.0.1:5000",
		},
		{
			name: "non-http error",
			err:  fmt.Errorf("open docker image archive: no such file"),
			want: "",
		},
		{
			name: "error response without url",
			err:  &errcode.ErrorResponse{StatusCode: 401},
			want: "",
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

// A source-side rejection must not be reported as a Harbor problem: that
// wording sent operators to check Harbor credentials for a failure that
// happened before Harbor was contacted.
func TestErrorUserMessageNamesFailingRegistry(t *testing.T) {
	sourceErr := &Error{
		Kind:     httpapi.ErrAuth,
		Op:       "prepare image artifact",
		Target:   "axem-solutions/shaide_control_panel",
		Registry: "ghcr.io",
		Scope:    ScopeSource,
	}

	message := sourceErr.UserMessage()
	if !strings.Contains(message, "ghcr.io") {
		t.Errorf("source message %q does not name the failing registry", message)
	}
	if strings.Contains(message, "Harbor") {
		t.Errorf("source message %q blames Harbor for an upstream failure", message)
	}

	targetErr := &Error{Kind: httpapi.ErrAuth, Scope: ScopeTarget, Registry: "127.0.0.1:5000"}
	if got, want := targetErr.UserMessage(), UserMessage(httpapi.ErrAuth); got != want {
		t.Errorf("target message = %q, want the Harbor message %q", got, want)
	}
}

func TestErrorRegistryLabelFallsBack(t *testing.T) {
	unknown := &Error{Kind: httpapi.ErrAuth, Scope: ScopeSource}
	if got := unknown.RegistryLabel(); got != "the source registry" {
		t.Errorf("RegistryLabel() = %q, want a generic fallback", got)
	}
	if strings.Contains(unknown.UserMessage(), "Harbor") {
		t.Errorf("unattributed source failure still blames Harbor: %q", unknown.UserMessage())
	}
}

// Local failures keep the existing wording: they are not registry problems and
// the operator should not be pointed at any registry.
func TestLocalScopeKeepsExistingMessages(t *testing.T) {
	local := &Error{Kind: ErrUploadState, Scope: ScopeLocal}
	if got, want := local.UserMessage(), UserMessage(ErrUploadState); got != want {
		t.Errorf("local message = %q, want %q", got, want)
	}
}
