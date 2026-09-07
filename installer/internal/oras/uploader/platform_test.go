package uploader

import (
	"errors"
	"fmt"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oraserrdef "oras.land/oras-go/v2/errdef"
)

func TestTargetPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform ocispec.Platform
		wantNil  bool
		wantName string
	}{
		{
			name:     "complete platform",
			platform: ocispec.Platform{OS: "linux", Architecture: "amd64"},
			wantName: "linux/amd64",
		},
		{
			// Detection never ran; copying the whole graph is the old
			// behaviour and is safe, if wasteful.
			name:    "zero platform copies everything",
			wantNil: true,
		},
		{
			name:     "half a platform is not a platform",
			platform: ocispec.Platform{OS: "linux"},
			wantNil:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			u := &Uploader{platform: test.platform}

			got := u.targetPlatform()
			if test.wantNil {
				if got != nil {
					t.Fatalf("targetPlatform() = %v, want nil", got)
				}
				if name := u.platformName(); name != "" {
					t.Errorf("platformName() = %q, want empty", name)
				}
				return
			}

			if got == nil {
				t.Fatal("targetPlatform() = nil, want a platform")
			}
			if name := u.platformName(); name != test.wantName {
				t.Errorf("platformName() = %q, want %q", name, test.wantName)
			}
		})
	}
}

// oras reports "no variant matches this platform" as a not-found, which would
// otherwise be rendered as a missing Harbor repository. The source tag resolved
// moments earlier, so the repository is there and the variant is not.
func TestCopyErrorNamesPlatformMismatch(t *testing.T) {
	u := &Uploader{platform: ocispec.Platform{OS: "linux", Architecture: "arm64"}}

	err := u.copyError(fmt.Errorf("resolve root: %w", oraserrdef.ErrNotFound))

	if !errors.Is(err, errdef.ErrPlatformUnavailable) {
		t.Fatalf("copyError() = %v, want it to wrap ErrPlatformUnavailable", err)
	}
	if kind := errdef.ClassifyError(err); kind != errdef.ErrPlatform {
		t.Errorf("ClassifyError() = %q, want %q", kind, errdef.ErrPlatform)
	}

	message := (&errdef.Error{Kind: errdef.ErrPlatform, Platform: u.platformName()}).UserMessage()
	if message != "the image publishes no linux/arm64 variant" {
		t.Errorf("UserMessage() = %q", message)
	}
	if kind := errdef.ClassifyError(err); kind == httpapi.ErrNotFound {
		t.Error("a platform mismatch still classifies as not-found")
	}
}

// Without a target platform the old not-found behaviour must be untouched.
func TestCopyErrorLeavesOtherFailuresAlone(t *testing.T) {
	withPlatform := &Uploader{platform: ocispec.Platform{OS: "linux", Architecture: "amd64"}}
	noPlatform := &Uploader{}

	notFound := fmt.Errorf("resolve root: %w", oraserrdef.ErrNotFound)
	if err := noPlatform.copyError(notFound); errors.Is(err, errdef.ErrPlatformUnavailable) {
		t.Error("a not-found was blamed on a platform that was never selected")
	}

	other := errors.New("connection reset")
	if err := withPlatform.copyError(other); errors.Is(err, errdef.ErrPlatformUnavailable) {
		t.Error("an unrelated failure was blamed on the platform")
	}
}
