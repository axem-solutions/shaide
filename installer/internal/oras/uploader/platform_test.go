package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	oraserrdef "oras.land/oras-go/v2/errdef"
)

func TestUploadErrorPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform cluster.Platform
		wantName string
	}{
		{
			name:     "complete platform",
			platform: cluster.Platform{OS: "linux", Arch: "amd64"},
			wantName: "linux/amd64",
		},
		{
			name: "empty platform has no error label",
		},
		{
			name:     "missing architecture has no error label",
			platform: cluster.Platform{OS: "linux"},
		},
		{
			name:     "missing OS has no error label",
			platform: cluster.Platform{Arch: "amd64"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			u := &Uploader{platform: test.platform}

			err := u.uploadError("copy", "target", errors.New("connection reset"))
			var uploadErr *errdef.Error
			if !errors.As(err, &uploadErr) {
				t.Fatalf("uploadError() = %v, want structured upload error", err)
			}
			if uploadErr.Platform != test.wantName {
				t.Errorf("error platform = %q, want %q", uploadErr.Platform, test.wantName)
			}
		})
	}
}

// oras reports "no variant matches this platform" as a not-found, which would
// otherwise be rendered as a missing Harbor repository. The source tag resolved
// moments earlier, so the repository is there and the variant is not.
func TestCopyErrorNamesPlatformMismatch(t *testing.T) {
	u := &Uploader{platform: cluster.Platform{OS: "linux", Arch: "arm64"}}

	err := u.copyError(fmt.Errorf("resolve root: %w", oraserrdef.ErrNotFound))

	if !errors.Is(err, errdef.ErrPlatformUnavailable) {
		t.Fatalf("copyError() = %v, want it to wrap ErrPlatformUnavailable", err)
	}
	if kind := errdef.ClassifyError(err); kind != errdef.ErrPlatform {
		t.Errorf("ClassifyError() = %q, want %q", kind, errdef.ErrPlatform)
	}

	message := (&errdef.Error{Kind: errdef.ErrPlatform, Platform: u.platform.String()}).UserMessage()
	if message != "the image publishes no linux/arm64 variant" {
		t.Errorf("UserMessage() = %q", message)
	}
	if kind := errdef.ClassifyError(err); kind == httpapi.ErrNotFound {
		t.Error("a platform mismatch still classifies as not-found")
	}
}

// Without a target platform the old not-found behaviour must be untouched.
func TestCopyErrorLeavesOtherFailuresAlone(t *testing.T) {
	withPlatform := &Uploader{platform: cluster.Platform{OS: "linux", Arch: "amd64"}}
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

func TestCopyOptionsSelectClusterPlatform(t *testing.T) {
	ctx := context.Background()
	source := memory.New()
	pushJSON := func(mediaType string, value any) ocispec.Descriptor {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		desc := content.NewDescriptorFromBytes(mediaType, data)
		if err := source.Push(ctx, desc, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		return desc
	}

	manifests := make(map[string]ocispec.Descriptor)
	var variants []ocispec.Descriptor
	for _, arch := range []string{"amd64", "arm64"} {
		platform := ocispec.Platform{OS: "linux", Architecture: arch}
		config := pushJSON(ocispec.MediaTypeImageConfig, ocispec.Image{Platform: platform})
		manifest := pushJSON(ocispec.MediaTypeImageManifest, ocispec.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: ocispec.MediaTypeImageManifest,
			Config:    config,
			Layers:    []ocispec.Descriptor{},
		})
		manifest.Platform = &platform
		manifests[arch] = manifest
		variants = append(variants, manifest)
	}
	index := pushJSON(ocispec.MediaTypeImageIndex, ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: variants,
	})
	if err := source.Tag(ctx, index, "multi"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		platform cluster.Platform
		wantArch string
		wantErr  bool
	}{
		{"amd64 only", cluster.Platform{OS: "linux", Arch: "amd64"}, "amd64", false},
		{"arm64 only", cluster.Platform{OS: "linux", Arch: "arm64"}, "arm64", false},
		{"empty copies all", cluster.Platform{}, "", false},
		{"missing architecture copies all", cluster.Platform{OS: "linux"}, "", false},
		{"missing OS copies all", cluster.Platform{Arch: "amd64"}, "", false},
		{"unavailable platform", cluster.Platform{OS: "windows", Arch: "amd64"}, "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			u := &Uploader{platform: test.platform, logf: func(string, ...any) {}}
			target := memory.New()
			root, err := oras.Copy(ctx, source, "multi", target, "copied", u.copyOptions(nil))
			if test.wantErr {
				if !errors.Is(err, oraserrdef.ErrNotFound) {
					t.Fatalf("Copy() error = %v, want platform not found", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantRoot := index
			if test.wantArch != "" {
				wantRoot = manifests[test.wantArch]
			}
			if root.Digest != wantRoot.Digest {
				t.Errorf("copied root = %s, want %s", root.Digest, wantRoot.Digest)
			}
			for arch, manifest := range manifests {
				exists, err := target.Exists(ctx, manifest)
				if err != nil {
					t.Fatal(err)
				}
				if want := test.wantArch == "" || test.wantArch == arch; exists != want {
					t.Errorf("%s manifest copied = %v, want %v", arch, exists, want)
				}
			}
		})
	}
}
