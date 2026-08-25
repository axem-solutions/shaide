package repository

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	localerrdef "github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote/auth"
)

type chunk struct {
	Location string
	Offset   int64
	Size     int64
	End      int64
}

type chunkContent struct {
	size int64
	open func() (io.ReadCloser, error)
}

func (c *chunkContent) Open() (io.ReadCloser, error) {
	return c.open()
}

func prepareChunkContent(content io.ReadSeeker, offset int64, size int64, spool bool) (*chunkContent, func(), error) {
	if spool {
		return spoolChunkToTempFile(content, offset, size)
	}
	return directChunkContent(content, offset, size)
}

func directChunkContent(content io.ReadSeeker, offset int64, size int64) (*chunkContent, func(), error) {
	if offset < 0 {
		return nil, nil, fmt.Errorf("invalid chunk offset %d", offset)
	}
	if size <= 0 {
		return nil, nil, fmt.Errorf("invalid chunk size %d", size)
	}

	var open func() (io.ReadCloser, error)
	if readerAt, ok := content.(io.ReaderAt); ok {
		open = func() (io.ReadCloser, error) {
			return io.NopCloser(io.NewSectionReader(readerAt, offset, size)), nil
		}
	} else {
		open = func() (io.ReadCloser, error) {
			if _, err := content.Seek(offset, io.SeekStart); err != nil {
				return nil, fmt.Errorf("seek source chunk to offset %d: %w", offset, err)
			}
			return io.NopCloser(io.LimitReader(content, size)), nil
		}
	}

	return &chunkContent{size: size, open: open}, func() {}, nil
}

func spoolChunkToTempFile(content io.ReadSeeker, offset int64, size int64) (*chunkContent, func(), error) {
	if offset < 0 {
		return nil, nil, fmt.Errorf("invalid chunk offset %d", offset)
	}
	if size <= 0 {
		return nil, nil, fmt.Errorf("invalid chunk size %d", size)
	}

	if _, err := content.Seek(offset, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("seek source chunk to offset %d: %w", offset, err)
	}

	file, err := os.CreateTemp("", "oras-chunk-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create chunk temp file: %w", err)
	}

	path := file.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}

	written, copyErr := io.CopyN(file, content, size)
	closeErr := file.Close()

	if copyErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf(
			"copy source chunk offset=%d size=%d written=%d: %w",
			offset,
			size,
			written,
			copyErr,
		)
	}
	if closeErr != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close chunk temp file: %w", closeErr)
	}
	if written != size {
		cleanup()
		return nil, nil, fmt.Errorf(
			"short chunk spool offset=%d expected=%d written=%d",
			offset,
			size,
			written,
		)
	}

	open := func() (io.ReadCloser, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open chunk temp file: %w", err)
		}
		return file, nil
	}

	return &chunkContent{size: size, open: open}, cleanup, nil
}

func (r *Repository) pushBlobChunked(ctx context.Context, desc ocispec.Descriptor, content io.ReadSeeker) error {
	ctx = auth.AppendRepositoryScope(ctx, r.Repo.Reference, auth.ActionPull, auth.ActionPush)

	current, err := r.startOrResumeBlobUpload(ctx, desc)
	if err != nil {
		return err
	}

	reportedOffset := int64(0)

	setReportedOffset := func(offset int64) {
		delta := offset - reportedOffset
		if delta == 0 {
			return
		}

		r.reportProgress(delta)
		reportedOffset = offset
	}
	setReportedOffset(current.Offset)

	for current.Offset < desc.Size {
		current = nextChunk(current.Location, current.Offset, desc.Size, r.opts.ChunkSize)

		if err := r.uploadChunkWithRetry(ctx, &current, desc, content, func() { setReportedOffset(0) }); err != nil {
			return err
		}

		setReportedOffset(current.Offset)
	}

	if err := r.putBlobUpload(ctx, current.Location, desc); err != nil {
		return fmt.Errorf("finalize upload: %w", err)
	}
	if err := r.deleteUploadState(desc); err != nil {
		return fmt.Errorf("%w: delete upload state after finalize: %w", localerrdef.ErrUploadStateFailure, err)
	}

	r.logf("oras chunk push: finalized upload digest=%s", desc.Digest)

	return nil
}

func (r *Repository) startOrResumeBlobUpload(ctx context.Context, desc ocispec.Descriptor) (chunk, error) {
	state, err := r.loadUploadState(desc)
	if err != nil {
		return chunk{}, fmt.Errorf("%w: load upload state: %w", localerrdef.ErrUploadStateFailure, err)
	}

	if state != nil {
		r.logf("oras chunk push: found upload state %s", state)

		offset, location, err := r.getBlobUpload(ctx, state.Location)
		if err != nil {
			if err := r.deleteUploadState(desc); err != nil {
				return chunk{}, fmt.Errorf("%w: delete stale upload state: %w", localerrdef.ErrUploadStateFailure, err)
			}
			r.logf("oras chunk push: deleting stale upload state digest=%s error=%v", desc.Digest, err)
		}

		resumeChunk := chunk{Location: location, Offset: offset}

		if err := r.saveUploadState(desc, resumeChunk.Offset, resumeChunk.Location); err != nil {
			return chunk{}, fmt.Errorf("%w: save resumed upload state: %w", localerrdef.ErrUploadStateFailure, err)
		}
		r.logf("oras chunk push: resumed upload digest=%s offset=%d size=%d", desc.Digest, resumeChunk.Offset, desc.Size)

		return resumeChunk, nil
	}

	location, err := r.postBlobUpload(ctx)
	if err != nil {
		return chunk{}, err
	}

	newChunk := chunk{Location: location}

	if err := r.saveUploadState(desc, newChunk.Offset, newChunk.Location); err != nil {
		return chunk{}, fmt.Errorf("%w: save new upload state: %w", localerrdef.ErrUploadStateFailure, err)
	}
	r.logf("oras chunk push: started upload digest=%s size=%d", desc.Digest, desc.Size)

	return newChunk, nil
}

func (r *Repository) uploadChunkWithRetry(ctx context.Context, current *chunk, desc ocispec.Descriptor, content io.ReadSeeker, onRestart func()) error {
	var lastErr error

	for attempt := 1; attempt <= r.opts.MaxChunkAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("upload canceled: %w", err)
		}

		attemptOffset := current.Offset
		attemptSize := current.End - current.Offset + 1
		if attemptSize <= 0 {
			return fmt.Errorf("invalid remaining chunk range offset=%d end=%d", current.Offset, current.End)
		}

		chunkBody, cleanup, err := prepareChunkContent(content, attemptOffset, attemptSize, r.opts.SpoolChunks)
		if err != nil {
			lastErr = err

			r.logf("oras chunk push: chunk spool failed digest=%s offset=%d end=%d attempt=%d/%d error=%v", desc.Digest, attemptOffset, current.End, attempt, r.opts.MaxChunkAttempts, err)

			if attempt < r.opts.MaxChunkAttempts {
				if err := r.waitForChunkRetry(ctx); err != nil {
					return err
				}
			}

			continue
		}

		nextOffset, nextLocation, err := r.patchBlobUpload(ctx, current, chunkBody)
		cleanup()
		if err != nil {
			lastErr = err

			r.logf("oras chunk push: chunk failed digest=%s offset=%d end=%d attempt=%d/%d error=%v", desc.Digest, attemptOffset, current.End, attempt, r.opts.MaxChunkAttempts, err)

			recovered, err := r.recoverOrRestartChunk(ctx, current, desc, attemptOffset, onRestart)
			if err != nil {
				return fmt.Errorf("%w; recover failed chunk: %v", lastErr, err)
			}
			if recovered {
				return nil
			}

			if attempt < r.opts.MaxChunkAttempts {
				if err := r.waitForChunkRetry(ctx); err != nil {
					return err
				}
			}

			continue
		}

		if err := validateNextOffset(nextOffset, attemptOffset, desc.Size); err != nil {
			return err
		}

		current.Offset = nextOffset
		current.Location = nextLocation
		if err := r.saveUploadState(desc, current.Offset, current.Location); err != nil {
			return fmt.Errorf("%w: save upload state after chunk: %w", localerrdef.ErrUploadStateFailure, err)
		}

		r.logf(
			"oras chunk push: chunk uploaded digest=%s offset=%d end=%d next_offset=%d size=%d attempt=%d/%d",
			desc.Digest,
			attemptOffset,
			current.End,
			current.Offset,
			desc.Size,
			attempt,
			r.opts.MaxChunkAttempts,
		)

		return nil
	}

	return fmt.Errorf("upload chunk after %d attempts: %w", r.opts.MaxChunkAttempts, lastErr)
}

// recoverOrRestartChunk resolves an ambiguous PATCH result before another
// attempt is allowed. It returns true when Harbor already accepted the chunk.
func (r *Repository) recoverOrRestartChunk(
	ctx context.Context,
	current *chunk,
	desc ocispec.Descriptor,
	attemptOffset int64,
	onRestart func(),
) (bool, error) {
	recovered, err := r.recoverChunk(ctx, current, attemptOffset, desc.Size)
	if err == nil {
		if err := r.saveUploadState(desc, current.Offset, current.Location); err != nil {
			return false, fmt.Errorf("%w: save recovered upload state: %w", localerrdef.ErrUploadStateFailure, err)
		}
		return recovered, nil
	}

	r.logf("oras chunk push: status check failed digest=%s error=%v", desc.Digest, err)
	if localerrdef.ClassifyError(err) != localerrdef.ErrUploadState {
		return false, fmt.Errorf("check upload status: %w", err)
	}

	location, err := r.postBlobUpload(ctx)
	if err != nil {
		return false, fmt.Errorf("restart blob upload: %w", err)
	}

	onRestart()
	*current = nextChunk(location, 0, desc.Size, r.opts.ChunkSize)
	if err := r.saveUploadState(desc, current.Offset, current.Location); err != nil {
		return false, fmt.Errorf("%w: save restarted upload state: %w", localerrdef.ErrUploadStateFailure, err)
	}

	r.logf("oras chunk push: restarted upload digest=%s after stale upload state", desc.Digest)
	return false, nil
}

// It returns true when the registry accepted bytes beyond the original chunk
// start. In that case, the caller can continue without repeating the PATCH.
func (r *Repository) recoverChunk(
	ctx context.Context,
	current *chunk,
	start int64,
	blobSize int64,
) (bool, error) {
	offset, location, err := r.getBlobUpload(ctx, current.Location)
	if err != nil {
		return false, err
	}

	if offset < start {
		return false, fmt.Errorf("registry offset moved backwards: got=%d start=%d", offset, start)
	}

	if offset > blobSize {
		return false, fmt.Errorf("registry offset exceeds blob size: got=%d size=%d", offset, blobSize)
	}

	current.Offset = offset
	current.Location = location

	return offset > start, nil
}

// waitForChunkRetry waits before the next PATCH attempt.
//
// The wait stops immediately if the context is canceled.
func (r *Repository) waitForChunkRetry(ctx context.Context) error {
	timer := time.NewTimer(r.opts.ChunkRetryDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil

	case <-ctx.Done():
		return fmt.Errorf("upload canceled: %w", ctx.Err())
	}
}
