package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/progress"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oraserrdef "oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	defaultChunkUploadAttempts = 5
	defaultChunkRetryDelay     = 2 * time.Second
)

type ChunkedUploadOptions struct {
	ChunkSize        int64
	MaxChunkAttempts int
	StateDir         string
	ChunkRetryDelay  time.Duration
	SpoolChunks      bool

	Logf    func(format string, args ...any)
	Tracker *progress.Tracker
}

type Repository struct {
	Repo *remote.Repository

	opts ChunkedUploadOptions
}

func New(repo *remote.Repository, opts ChunkedUploadOptions) *Repository {
	if opts.MaxChunkAttempts <= 0 {
		opts.MaxChunkAttempts = defaultChunkUploadAttempts
	}
	if opts.ChunkRetryDelay <= 0 {
		opts.ChunkRetryDelay = defaultChunkRetryDelay
	}

	return &Repository{Repo: repo, opts: opts}
}

func (r *Repository) ManifestExists(ctx context.Context, tag string) (bool, error) {
	if _, err := r.Repo.Resolve(ctx, tag); err != nil {
		if errors.Is(err, oraserrdef.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) Fetch(ctx context.Context, d ocispec.Descriptor) (io.ReadCloser, error) {
	return r.Repo.Fetch(ctx, d)
}

func (r *Repository) Exists(ctx context.Context, d ocispec.Descriptor) (bool, error) {
	return r.Repo.Exists(ctx, d)
}

func (r *Repository) Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error) {
	return r.Repo.Resolve(ctx, ref)
}

func (r *Repository) Tag(ctx context.Context, d ocispec.Descriptor, ref string) error {
	return r.Repo.Tag(ctx, d, ref)
}

func (r *Repository) Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error {
	if r.useDefaultPush(expected) {
		r.logf(
			"oras push: default push digest=%s mediaType=%s size=%d",
			expected.Digest,
			expected.MediaType,
			expected.Size,
		)
		if r.opts.Tracker != nil {
			content = r.opts.Tracker.Reader(content)
		}
		return r.Repo.Push(ctx, expected, content)
	}

	r.logf(
		"oras push: chunk push digest=%s mediaType=%s size=%d chunk_size=%d spool_chunks=%t",
		expected.Digest,
		expected.MediaType,
		expected.Size,
		r.opts.ChunkSize,
		r.opts.SpoolChunks,
	)

	// We need a ReadSeeker so each PATCH can start from the byte offset
	// acknowledged by the registry.
	//
	// Local ORAS file stores return *os.File, and remote ORAS fetches can
	// return an io.ReadSeekCloser when the source registry supports byte
	// ranges. Plain HTTP response bodies still need to be spooled.
	rs, ok := content.(io.ReadSeeker)
	if !ok {
		r.logf("oras push: spooling non-seekable content digest=%s", expected.Digest)
		file, cleanup, err := spoolToTempFile(content)
		if err != nil {
			return fmt.Errorf("spool blob %s to temp file: %w", expected.Digest, err)
		}
		defer cleanup()

		rs = file
	}

	return r.pushBlobChunked(ctx, expected, rs)
}

func (r *Repository) reportProgress(bytes int64) {
	if r.opts.Tracker == nil || bytes == 0 {
		return
	}

	r.opts.Tracker.AddBytes(bytes)
}

func (r *Repository) logf(format string, args ...any) {
	if r.opts.Logf == nil {
		return
	}

	r.opts.Logf(format, args...)
}

func (r *Repository) scheme() string {
	if r.Repo.PlainHTTP {
		return "http"
	}

	return "https"
}

func (r *Repository) useDefaultPush(desc ocispec.Descriptor) bool {
	return desc.Size <= r.opts.ChunkSize || isManifestMediaType(desc.MediaType)
}

func isManifestMediaType(mediaType string) bool {
	switch mediaType {
	case "application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.oci.artifact.manifest.v1+json":
		return true
	default:
		return false
	}
}

func spoolToTempFile(r io.Reader) (*os.File, func(), error) {
	file, err := os.CreateTemp("", "oras-blob-*")
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
	}

	if _, err := io.Copy(file, r); err != nil {
		cleanup()
		return nil, nil, err
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, err
	}

	return file, cleanup, nil
}
