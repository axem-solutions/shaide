package cache

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	CacheDir       string
	HubDirName     string
	XetDirName     string
	ModelDirPrefix string
	SentinelName   string
	// RefName is the ref the serving runtime resolves, e.g. "main". It is
	// written to point at the pinned revision's snapshot.
	RefName string
}

type Manager struct {
	Config
}

type State struct {
	ModelDir string
	Sentinel string
	Revision string
	Skip     bool
}

func New(config Config) Manager {
	return Manager{Config: config}
}

func (m Manager) HubDir() string {
	return filepath.Join(m.CacheDir, m.HubDirName)
}

func (m Manager) DirectoryName(modelID string) string {
	return DirectoryName(modelID, m.ModelDirPrefix)
}

func DirectoryName(modelID, prefix string) string {
	return prefix + strings.ReplaceAll(modelID, "/", "--")
}

func snapshotDir(modelDir, revision string) string {
	return filepath.Join(modelDir, "snapshots", revision)
}

// EnsureRef points the ref at the snapshot of revision, so an offline
// runtime resolving the ref loads the pinned weights. Without a snapshot of
// that name, as when the revision is a branch, an existing ref is kept, or
// else the first snapshot is used.
func (m Manager) EnsureRef(modelDir, revision string) error {
	ref := filepath.Join(modelDir, "refs", m.RefName)

	target := revision
	if _, err := os.Stat(snapshotDir(modelDir, revision)); err != nil {
		if content, err := os.ReadFile(ref); err == nil && strings.TrimSpace(string(content)) != "" {
			return nil
		}

		entries, err := os.ReadDir(filepath.Join(modelDir, "snapshots"))
		if err != nil {
			return fmt.Errorf("read snapshots directory: %w", err)
		}

		target = ""
		for _, entry := range entries {
			if entry.IsDir() {
				target = entry.Name()
				break
			}
		}
		if target == "" {
			return fmt.Errorf("no snapshot found")
		}
	}

	if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
		return fmt.Errorf("create refs directory: %w", err)
	}
	if err := os.WriteFile(ref, []byte(target), 0o644); err != nil {
		return fmt.Errorf("write refs/%s: %w", m.RefName, err)
	}

	return nil
}

// Prepare reports whether revision of the model is already downloaded. The
// completion marker records the revision it was written for, so a model
// downloaded for an earlier pin is downloaded again. A marker written before
// it recorded one counts when the pinned revision's snapshot exists.
func (m Manager) Prepare(repoID, revision string) (State, error) {
	if strings.TrimSpace(repoID) == "" {
		return State{}, fmt.Errorf("huggingface: model id is required")
	}

	modelDir := filepath.Join(m.HubDir(), m.DirectoryName(repoID))
	state := State{
		ModelDir: modelDir,
		Sentinel: filepath.Join(modelDir, m.SentinelName),
		Revision: revision,
	}

	if m.complete(state) {
		if err := m.EnsureRef(modelDir, revision); err != nil {
			return State{}, fmt.Errorf("huggingface: cached model %s is incomplete: %w", repoID, err)
		}

		state.Skip = true
		return state, nil
	}

	if err := os.MkdirAll(m.HubDir(), 0o755); err != nil {
		return State{}, fmt.Errorf("huggingface: create cache directory %s: %w", m.HubDir(), err)
	}

	return state, nil
}

func (m Manager) complete(state State) bool {
	content, err := os.ReadFile(state.Sentinel)
	if err != nil {
		return false
	}

	recorded := strings.TrimSpace(string(content))
	if recorded != "" {
		return recorded == state.Revision
	}

	info, err := os.Stat(snapshotDir(state.ModelDir, state.Revision))
	return err == nil && info.IsDir()
}

// Complete marks the revision downloaded.
func (m Manager) Complete(state State) error {
	if err := m.EnsureRef(state.ModelDir, state.Revision); err != nil {
		return fmt.Errorf("huggingface: create refs/%s: %w", m.RefName, err)
	}

	if err := os.WriteFile(state.Sentinel, []byte(state.Revision), 0o644); err != nil {
		return fmt.Errorf("huggingface: write sentinel %s: %w", state.Sentinel, err)
	}

	return nil
}

// PruneOtherRevisions removes the snapshots of every other revision and the
// blobs only they use, so the model directory, which is packed as the model
// artifact, holds the pinned revision alone. It does nothing unless the
// pinned revision has a snapshot, and returns the bytes it freed.
func (m Manager) PruneOtherRevisions(state State) (int64, error) {
	pinned := snapshotDir(state.ModelDir, state.Revision)
	if info, err := os.Stat(pinned); err != nil || !info.IsDir() {
		return 0, nil
	}

	keep, err := referencedBlobs(pinned)
	if err != nil {
		return 0, fmt.Errorf("list blobs of %s: %w", pinned, err)
	}

	var freed int64

	snapshots, err := os.ReadDir(filepath.Join(state.ModelDir, "snapshots"))
	if err != nil {
		return 0, fmt.Errorf("read snapshots directory: %w", err)
	}
	for _, snapshot := range snapshots {
		if snapshot.Name() == state.Revision {
			continue
		}
		if err := os.RemoveAll(filepath.Join(state.ModelDir, "snapshots", snapshot.Name())); err != nil {
			return freed, fmt.Errorf("remove snapshot %s: %w", snapshot.Name(), err)
		}
	}

	blobsDir := filepath.Join(state.ModelDir, "blobs")
	blobs, err := os.ReadDir(blobsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return freed, nil
		}
		return freed, fmt.Errorf("read blobs directory: %w", err)
	}
	for _, blob := range blobs {
		if keep[blob.Name()] {
			continue
		}
		if info, err := blob.Info(); err == nil {
			freed += info.Size()
		}
		if err := os.Remove(filepath.Join(blobsDir, blob.Name())); err != nil {
			return freed, fmt.Errorf("remove blob %s: %w", blob.Name(), err)
		}
	}

	return freed, nil
}

// referencedBlobs names the blobs the snapshot's symlinks point at.
func referencedBlobs(snapshot string) (map[string]bool, error) {
	keep := map[string]bool{}

	err := filepath.WalkDir(snapshot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink == 0 {
			return nil
		}

		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		keep[filepath.Base(target)] = true

		return nil
	})

	return keep, err
}
