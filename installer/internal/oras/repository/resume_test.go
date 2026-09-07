package repository

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
)

const testDigest = "sha256:2ff1fec5a6d4a1319b2df10ac245a89f6de043887b5ec0bb733b3dda581ba36d"

func newTestRepository(t *testing.T, handler http.Handler) (*Repository, string, string, func()) {
	t.Helper()

	server := httptest.NewServer(handler)
	stateDir := t.TempDir()

	host := strings.TrimPrefix(server.URL, "http://")
	repo, err := remote.NewRepository(host + "/services/llm-d/llm-d-cuda")
	if err != nil {
		server.Close()
		t.Fatalf("new repository: %v", err)
	}
	repo.PlainHTTP = true
	// doReq calls r.Repo.Client directly; production wires this up in
	// client.remoteRepository.
	repo.Client = server.Client()

	return New(repo, ChunkedUploadOptions{
		ChunkSize: 1024,
		StateDir:  stateDir,
		Logf:      func(string, ...any) {},
	}), stateDir, server.URL, server.Close
}

func readState(t *testing.T, stateDir string) uploadState {
	t.Helper()

	name := strings.ReplaceAll(testDigest, ":", "-") + ".json"
	body, err := os.ReadFile(filepath.Join(stateDir, name))
	if err != nil {
		t.Fatalf("read upload state: %v", err)
	}

	var state uploadState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("decode upload state: %v", err)
	}

	return state
}

// An upload session that the registry has expired must restart cleanly.
//
// The old behaviour continued with the zero values from the failed status
// check, producing a chunk with no location. Every PATCH then failed with
// "unsupported protocol scheme", and because the empty location was written
// back, the state file kept the upload wedged on every later run.
func TestStartOrResumeBlobUploadRestartsExpiredSession(t *testing.T) {
	var posted bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/uploads/"):
			// Harbor has purged the session recorded in the state file.
			w.WriteHeader(http.StatusNotFound)

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/blobs/uploads/"):
			posted = true
			w.Header().Set("Location", "/v2/services/llm-d/llm-d-cuda/blobs/uploads/fresh-session")
			w.WriteHeader(http.StatusAccepted)

		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	repo, stateDir, _, closeServer := newTestRepository(t, handler)
	defer closeServer()

	desc := ocispec.Descriptor{Digest: testDigest, Size: 6557630703}

	if err := repo.saveUploadState(desc, 1610612736, "http://127.0.0.1:5000/v2/services/llm-d/llm-d-cuda/blobs/uploads/expired"); err != nil {
		t.Fatalf("seed upload state: %v", err)
	}

	current, err := repo.startOrResumeBlobUpload(context.Background(), desc)
	if err != nil {
		t.Fatalf("startOrResumeBlobUpload() error = %v", err)
	}

	if !posted {
		t.Error("no new upload was started after the recorded session expired")
	}
	if current.Location == "" {
		t.Fatal("resumed chunk has no location; every PATCH would fail with an empty URL")
	}
	if !strings.HasSuffix(current.Location, "fresh-session") {
		t.Errorf("location = %q, want the newly posted session", current.Location)
	}
	if current.Offset != 0 {
		t.Errorf("offset = %d, want 0 for a restarted upload", current.Offset)
	}

	if state := readState(t, stateDir); state.Location == "" {
		t.Error("an empty location was written back to the state file")
	}
}

// A state file already carrying an empty location must recover by itself
// rather than reproducing the failure on every run.
func TestStartOrResumeBlobUploadRecoversFromPoisonedState(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/blobs/uploads/") {
			w.Header().Set("Location", "/v2/services/llm-d/llm-d-cuda/blobs/uploads/fresh-session")
			w.WriteHeader(http.StatusAccepted)
			return
		}

		t.Errorf("unexpected request %s %s: an empty location must not be requested", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})

	repo, _, _, closeServer := newTestRepository(t, handler)
	defer closeServer()

	desc := ocispec.Descriptor{Digest: testDigest, Size: 6557630703}

	if err := repo.saveUploadState(desc, 0, ""); err != nil {
		t.Fatalf("seed upload state: %v", err)
	}

	current, err := repo.startOrResumeBlobUpload(context.Background(), desc)
	if err != nil {
		t.Fatalf("startOrResumeBlobUpload() error = %v", err)
	}

	if !strings.HasSuffix(current.Location, "fresh-session") {
		t.Errorf("location = %q, want a freshly posted session", current.Location)
	}
}

// A live session must still resume where it left off.
func TestStartOrResumeBlobUploadResumesLiveSession(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/uploads/") {
			w.Header().Set("Range", "0-1610612735")
			w.Header().Set("Location", "/v2/services/llm-d/llm-d-cuda/blobs/uploads/live-session")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		t.Errorf("unexpected request %s %s: a live session must not be restarted", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})

	repo, _, serverURL, closeServer := newTestRepository(t, handler)
	defer closeServer()

	desc := ocispec.Descriptor{Digest: testDigest, Size: 6557630703}

	live := serverURL + "/v2/services/llm-d/llm-d-cuda/blobs/uploads/live"
	if err := repo.saveUploadState(desc, 1610612736, live); err != nil {
		t.Fatalf("seed upload state: %v", err)
	}

	current, err := repo.startOrResumeBlobUpload(context.Background(), desc)
	if err != nil {
		t.Fatalf("startOrResumeBlobUpload() error = %v", err)
	}

	if current.Offset != 1610612736 {
		t.Errorf("offset = %d, want the recorded resume point", current.Offset)
	}
	if !strings.HasSuffix(current.Location, "live-session") {
		t.Errorf("location = %q, want the live session", current.Location)
	}
}
