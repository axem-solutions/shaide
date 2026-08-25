package uploader

import (
	"context"
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/config/storage"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/client"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/repository"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
)

const defaultChunkSize = 256 << 20

type UploaderOptions struct {
	// Client holds target Harbor settings and optional remote source credentials.
	Client client.ClientOptions

	// ChunkSize is the maximum bytes per OCI PATCH request.
	// Defaults to 500 MB.
	ChunkSize int64

	// StateDir stores resumable chunk-upload state.
	StateDir string

	// ArtifactCacheDir stores packaged model artifacts between runs.
	ArtifactCacheDir string

	StorageChecker storage.Checker

	// Logf receives upload/build log messages.
	Logf func(format string, args ...any)

	// Progressf receives artifact build and upload progress events.
	Progressf func(progress.Event)
}

type Uploader struct {
	client           client.Client
	chunkSize        int64
	stateDir         string
	artifactCacheDir string
	storageChecker   storage.Checker
	logf             func(format string, args ...any)
	progressf        func(progress.Event)
}

func NewUploader(opts UploaderOptions) (*Uploader, error) {
	if opts.Logf == nil {
		return nil, fmt.Errorf("logf is required")
	}
	if opts.Progressf == nil {
		return nil, fmt.Errorf("progress callback is required")
	}
	if opts.StateDir == "" {
		return nil, fmt.Errorf("stateDir is required")
	}
	if opts.ArtifactCacheDir == "" {
		return nil, fmt.Errorf("artifact cacheDir is required")
	}
	if opts.StorageChecker == nil {
		return nil, fmt.Errorf("storage checker is required")
	}

	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	return &Uploader{
		client:           *client.NewClient(opts.Client),
		chunkSize:        chunkSize,
		stateDir:         opts.StateDir,
		artifactCacheDir: opts.ArtifactCacheDir,
		storageChecker:   opts.StorageChecker,
		logf:             opts.Logf,
		progressf:        opts.Progressf,
	}, nil
}

// Artifact is the normalized input for the shared upload path
//
// Image and model workflows prepare the artifacts differently, but both eventually
// produces these same values
type Artifact struct {
	Source    oras.ReadOnlyTarget
	SourceRef string
	// SpoolChunks decouples a remote source registry response from retries
	// against the target registry. Local artifacts can be reopened directly.
	SpoolChunks bool

	Project string
	Name    string
	Tag     string

	ProgressID string
	Phase      progress.Phase
	Bytes      int64
	Files      int
}

func (a Artifact) Ref() string {
	return targetRef(a.Project, a.Name, a.Tag)
}

func (u *Uploader) push(ctx context.Context, a Artifact) (ocispec.Descriptor, error) {
	tracker := progress.NewTracker(a.Phase, a.ProgressID, a.Bytes, a.Files, u.progressf).Throttle(progress.DefaultReportEvery)

	var err error
	tracker.Start()
	defer func() {
		if err != nil {
			tracker.Stop()
			return
		}
		tracker.Finish()
	}()

	target, err := u.client.NewTargetRepository(a.Project, a.Name, repository.ChunkedUploadOptions{
		ChunkSize:   u.chunkSize,
		Logf:        u.logf,
		StateDir:    u.stateDir,
		SpoolChunks: a.SpoolChunks,
		Tracker:     tracker,
	})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create target repository: %w", err)
	}

	manifest, err := u.copy(ctx, a, target, tracker)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	u.logf("uploaded artifact %s manifest=%s", a.Ref(), manifest.Digest)

	return manifest, nil
}

func (u *Uploader) copy(ctx context.Context, artifact Artifact, target *repository.Repository, tracker *progress.Tracker) (ocispec.Descriptor, error) {
	u.logf("oras upload: copying source=%s target=%s tag=%s", artifact.SourceRef, artifact.Ref(), artifact.Tag)

	manifest, err := oras.Copy(
		ctx,
		artifact.Source,
		artifact.SourceRef,
		target,
		artifact.Tag,
		oras.CopyOptions{
			CopyGraphOptions: oras.CopyGraphOptions{
				Concurrency: 1,
				OnCopySkipped: func(ctx context.Context, desc ocispec.Descriptor) error {
					u.reuse("skipped", desc, tracker)
					return nil
				},
			},
		},
	)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("copy artifact to Harbor: %w", err)
	}

	if err := u.verify(ctx, target, artifact.Tag, artifact.Ref(), manifest); err != nil {
		return ocispec.Descriptor{}, err
	}

	return manifest, nil
}

func (u *Uploader) verify(ctx context.Context, target *repository.Repository, tag string, ref string, manifest ocispec.Descriptor) error {
	ok, err := target.ManifestExists(ctx, tag)
	if err != nil {
		return fmt.Errorf("verify manifest: %w", err)
	}

	if !ok {
		return fmt.Errorf(" %s manifest not found: %w", tag, errdef.ErrUploadStateFailure)
	}

	u.logf("oras upload: verified target=%s tag=%s manifest=%s", ref, tag, manifest.Digest)

	return nil
}

func graphSize(ctx context.Context, fetcher content.Fetcher, root ocispec.Descriptor) (int64, int, error) {
	seen := map[string]struct{}{}

	var totalBytes int64
	var totalFiles int

	var walk func(ocispec.Descriptor) error

	walk = func(desc ocispec.Descriptor) error {
		key := desc.Digest.String()
		if _, ok := seen[key]; ok {
			return nil
		}

		seen[key] = struct{}{}

		totalBytes += desc.Size
		totalFiles++

		children, err := content.Successors(ctx, fetcher, desc)
		if err != nil {
			return fmt.Errorf("find successors for %s: %w", desc.Digest, err)
		}

		for _, child := range children {
			if err := walk(child); err != nil {
				return err
			}
		}

		return nil
	}

	if err := walk(root); err != nil {
		return 0, 0, err
	}

	return totalBytes, totalFiles, nil
}

func (u *Uploader) reuse(action string, desc ocispec.Descriptor, tracker *progress.Tracker) {
	u.logf(
		"oras upload: %s digest=%s mediaType=%s size=%d",
		action,
		desc.Digest,
		desc.MediaType,
		desc.Size,
	)

	if tracker != nil {
		tracker.AddBytes(desc.Size)
	}
}

func targetRef(project string, name string, tag string) string {
	return fmt.Sprintf("%s/%s:%s", project, name, tag)
}

func uploadError(op string, target string, err error) error {
	return &errdef.Error{
		Kind:   errdef.ClassifyError(err),
		Op:     op,
		Target: target,
		Err:    err,
	}
}
