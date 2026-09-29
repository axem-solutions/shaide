package cache

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	oldRevision = "1111111111111111111111111111111111111111"
	newRevision = "2222222222222222222222222222222222222222"
)

func newManager(t *testing.T) Manager {
	t.Helper()

	return New(Config{
		CacheDir:       t.TempDir(),
		HubDirName:     "hub",
		ModelDirPrefix: "models--",
		SentinelName:   ".complete",
		RefName:        "main",
	})
}

// snapshot lays out a Hugging Face cache snapshot: each file a symlink into
// blobs/, as the hub client writes it.
func snapshot(t *testing.T, modelDir, revision string, files map[string]string) {
	t.Helper()

	for name, blob := range files {
		blobPath := filepath.Join(modelDir, "blobs", blob)
		if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(blobPath, []byte("content of "+blob), 0o644); err != nil {
			t.Fatal(err)
		}

		link := filepath.Join(modelDir, "snapshots", revision, name)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		target, err := filepath.Rel(filepath.Dir(link), blobPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
}

func readRef(t *testing.T, modelDir string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(modelDir, "refs", "main"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// A model downloaded for an earlier pin must be downloaded again, or the new
// tag would be published with the old weights.
func TestPrepareDownloadsAgainForANewRevision(t *testing.T) {
	m := newManager(t)

	state, err := m.Prepare("openai/gpt-oss-20b", oldRevision)
	if err != nil {
		t.Fatal(err)
	}
	snapshot(t, state.ModelDir, oldRevision, map[string]string{"config.json": "a"})
	if err := m.Complete(state); err != nil {
		t.Fatal(err)
	}

	again, err := m.Prepare("openai/gpt-oss-20b", oldRevision)
	if err != nil || !again.Skip {
		t.Fatalf("same revision: skip = %v, err = %v, want it skipped", again.Skip, err)
	}

	bumped, err := m.Prepare("openai/gpt-oss-20b", newRevision)
	if err != nil {
		t.Fatal(err)
	}
	if bumped.Skip {
		t.Error("a new revision was treated as already downloaded")
	}
}

// Caches written before the marker recorded its revision stay valid when
// they hold the pinned revision's snapshot.
func TestPrepareAcceptsALegacyMarkerForThePinnedSnapshot(t *testing.T) {
	m := newManager(t)
	modelDir := filepath.Join(m.HubDir(), "models--openai--gpt-oss-20b")
	snapshot(t, modelDir, oldRevision, map[string]string{"config.json": "a"})
	if err := os.WriteFile(filepath.Join(modelDir, ".complete"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := m.Prepare("openai/gpt-oss-20b", oldRevision)
	if err != nil || !state.Skip {
		t.Fatalf("pinned snapshot: skip = %v, err = %v, want it skipped", state.Skip, err)
	}
	if got := readRef(t, modelDir); got != oldRevision {
		t.Errorf("refs/main = %q, want the pinned revision", got)
	}

	other, err := m.Prepare("openai/gpt-oss-20b", newRevision)
	if err != nil || other.Skip {
		t.Errorf("other revision: skip = %v, err = %v, want a download", other.Skip, err)
	}
}

func TestPruneOtherRevisionsKeepsOnlyThePinnedOne(t *testing.T) {
	m := newManager(t)
	modelDir := filepath.Join(m.HubDir(), "models--openai--gpt-oss-20b")

	// The tokenizer did not change between the revisions, so both share its
	// blob; the weights did.
	snapshot(t, modelDir, oldRevision, map[string]string{
		"tokenizer.json":             "shared",
		"model.safetensors":          "old-weights",
		"original/model.safetensors": "old-original",
	})
	snapshot(t, modelDir, newRevision, map[string]string{
		"tokenizer.json":    "shared",
		"model.safetensors": "new-weights",
	})

	state := State{ModelDir: modelDir, Revision: newRevision}
	freed, err := m.PruneOtherRevisions(state)
	if err != nil {
		t.Fatalf("PruneOtherRevisions: %v", err)
	}
	if freed == 0 {
		t.Error("freed = 0, want the old weights counted")
	}

	for path, want := range map[string]bool{
		"snapshots/" + oldRevision:                     false,
		"snapshots/" + newRevision:                     true,
		"blobs/shared":                                 true,
		"blobs/new-weights":                            true,
		"blobs/old-weights":                            false,
		"blobs/old-original":                           false,
		"snapshots/" + newRevision + "/tokenizer.json": true,
	} {
		_, err := os.Stat(filepath.Join(modelDir, path))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists = %v, want %v", path, exists, want)
		}
	}
}

// Without the pinned snapshot, as for a branch revision, nothing is removed.
func TestPruneOtherRevisionsNeedsThePinnedSnapshot(t *testing.T) {
	m := newManager(t)
	modelDir := filepath.Join(m.HubDir(), "models--openai--gpt-oss-20b")
	snapshot(t, modelDir, oldRevision, map[string]string{"model.safetensors": "old-weights"})

	if _, err := m.PruneOtherRevisions(State{ModelDir: modelDir, Revision: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(modelDir, "blobs", "old-weights")); err != nil {
		t.Errorf("removed a blob without a pinned snapshot to keep: %v", err)
	}
}

// With several snapshots, the ref points at the pinned one, not whichever is
// listed first.
func TestCompletePointsTheRefAtThePinnedRevision(t *testing.T) {
	m := newManager(t)
	modelDir := filepath.Join(m.HubDir(), "models--openai--gpt-oss-20b")
	snapshot(t, modelDir, oldRevision, map[string]string{"a": "a"})
	snapshot(t, modelDir, newRevision, map[string]string{"b": "b"})

	state := State{ModelDir: modelDir, Sentinel: filepath.Join(modelDir, ".complete"), Revision: newRevision}
	if err := m.Complete(state); err != nil {
		t.Fatal(err)
	}

	if got := readRef(t, modelDir); got != newRevision {
		t.Errorf("refs/main = %q, want %q", got, newRevision)
	}
	content, _ := os.ReadFile(state.Sentinel)
	if string(content) != newRevision {
		t.Errorf(".complete = %q, want the revision recorded", content)
	}
}
