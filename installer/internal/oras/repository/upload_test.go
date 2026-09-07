package repository

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testBlob(size int) []byte {
	blob := make([]byte, size)
	for i := range blob {
		blob[i] = byte(i % 251)
	}
	return blob
}

func readChunk(t *testing.T, content *chunkContent) []byte {
	t.Helper()

	reader, err := content.Open()
	if err != nil {
		t.Fatalf("open chunk: %v", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}

	return body
}

func tempFileCount(t *testing.T) int {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "oras-chunk-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}

	return len(matches)
}

// Walking a blob chunk by chunk must return the same bytes the source holds,
// whether a chunk came from the prefetch or was spooled on the spot.
func TestChunkPrefetcherServesEveryChunk(t *testing.T) {
	const chunkSize = 1024
	blob := testBlob(chunkSize*3 + 500)

	for _, spool := range []bool{true, false} {
		name := "spooled"
		if !spool {
			name = "direct"
		}

		t.Run(name, func(t *testing.T) {
			prefetcher := newChunkPrefetcher(
				bytes.NewReader(blob),
				int64(len(blob)),
				ChunkedUploadOptions{ChunkSize: chunkSize, SpoolChunks: spool},
			)
			defer prefetcher.close()

			for offset := 0; offset < len(blob); offset += chunkSize {
				size := chunkSize
				if remaining := len(blob) - offset; remaining < size {
					size = remaining
				}

				content, cleanup, err := prefetcher.fetch(int64(offset), int64(size))
				if err != nil {
					t.Fatalf("fetch(%d, %d): %v", offset, size, err)
				}

				if got := readChunk(t, content); !bytes.Equal(got, blob[offset:offset+size]) {
					t.Fatalf("chunk at offset %d does not match the source", offset)
				}

				cleanup()
			}
		})
	}
}

// A retry or a recovered offset asks for a range the prefetch does not cover.
// The stale copy must be discarded, not served.
func TestChunkPrefetcherDiscardsMismatchedRange(t *testing.T) {
	const chunkSize = 1024
	blob := testBlob(chunkSize * 4)

	prefetcher := newChunkPrefetcher(
		bytes.NewReader(blob),
		int64(len(blob)),
		ChunkedUploadOptions{ChunkSize: chunkSize, SpoolChunks: true},
	)
	defer prefetcher.close()

	content, cleanup, err := prefetcher.fetch(0, chunkSize)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	cleanup()
	_ = content

	// The registry accepted part of the chunk, so the next range starts inside
	// what was prefetched rather than at the chunk boundary.
	const recoveredOffset = chunkSize + 256
	const recoveredSize = chunkSize - 256

	recovered, cleanup, err := prefetcher.fetch(recoveredOffset, recoveredSize)
	if err != nil {
		t.Fatalf("recovered fetch: %v", err)
	}
	defer cleanup()

	want := blob[recoveredOffset : recoveredOffset+recoveredSize]
	if got := readChunk(t, recovered); !bytes.Equal(got, want) {
		t.Fatalf("recovered chunk does not match the source range")
	}
}

// The point of the prefetch: the next chunk is read from the source while the
// caller is still busy with the current one.
func TestChunkPrefetcherReadsAhead(t *testing.T) {
	const chunkSize = 1024
	blob := testBlob(chunkSize * 2)

	source := &slowReader{ReadSeeker: bytes.NewReader(blob), delay: 40 * time.Millisecond}
	prefetcher := newChunkPrefetcher(
		source,
		int64(len(blob)),
		ChunkedUploadOptions{ChunkSize: chunkSize, SpoolChunks: true},
	)
	defer prefetcher.close()

	first, cleanup, err := prefetcher.fetch(0, chunkSize)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	_ = first
	cleanup()

	// Stand in for the upload of chunk one; the second chunk should be spooled
	// during this window rather than after it.
	time.Sleep(80 * time.Millisecond)

	start := time.Now()
	second, cleanup, err := prefetcher.fetch(chunkSize, chunkSize)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	defer cleanup()
	elapsed := time.Since(start)

	if elapsed > 20*time.Millisecond {
		t.Errorf("second fetch took %v; it was not read ahead", elapsed)
	}
	if got := readChunk(t, second); !bytes.Equal(got, blob[chunkSize:]) {
		t.Fatalf("prefetched chunk does not match the source")
	}
}

// An abandoned prefetch must not leave its temp file behind.
func TestChunkPrefetcherCloseRemovesPendingSpool(t *testing.T) {
	const chunkSize = 1024
	blob := testBlob(chunkSize * 4)

	before := tempFileCount(t)

	prefetcher := newChunkPrefetcher(
		bytes.NewReader(blob),
		int64(len(blob)),
		ChunkedUploadOptions{ChunkSize: chunkSize, SpoolChunks: true},
	)

	_, cleanup, err := prefetcher.fetch(0, chunkSize)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	cleanup()

	// A pending prefetch for the second chunk is now in flight and never taken.
	prefetcher.close()

	if after := tempFileCount(t); after != before {
		t.Errorf("temp file count = %d, want %d: a spooled chunk was leaked", after, before)
	}
}

type slowReader struct {
	io.ReadSeeker
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	time.Sleep(r.delay)
	return r.ReadSeeker.Read(p)
}
