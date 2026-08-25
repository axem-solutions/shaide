package cache

import (
	"fmt"
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
	Revision       string
}

type Manager struct {
	Config
}

type State struct {
	ModelDir string
	Sentinel string
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

func (m Manager) EnsureRefsMain(modelDir string) error {
	refsMain := filepath.Join(modelDir, "refs", m.Revision)
	if content, err := os.ReadFile(refsMain); err == nil && strings.TrimSpace(string(content)) != "" {
		return nil
	}

	snapshotsDir := filepath.Join(modelDir, "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return fmt.Errorf("read snapshots directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(refsMain), 0o755); err != nil {
			return fmt.Errorf("create refs directory: %w", err)
		}
		if err := os.WriteFile(refsMain, []byte(entry.Name()), 0o644); err != nil {
			return fmt.Errorf("write refs/main: %w", err)
		}
		return nil
	}

	return fmt.Errorf("no snapshot found")
}

func (m Manager) Prepare(repoID string) (State, error) {
	if strings.TrimSpace(repoID) == "" {
		return State{}, fmt.Errorf("huggingface: model id is required")
	}

	modelDir := filepath.Join(m.HubDir(), m.DirectoryName(repoID))
	sentinel := filepath.Join(modelDir, m.SentinelName)

	if info, err := os.Stat(sentinel); err == nil && !info.IsDir() {
		if err := m.EnsureRefsMain(modelDir); err != nil {
			return State{}, fmt.Errorf("huggingface: cached model %s is incomplete: %w", repoID, err)
		}

		return State{
			ModelDir: modelDir,
			Sentinel: sentinel,
			Skip:     true,
		}, nil
	}

	if err := os.MkdirAll(m.HubDir(), 0o755); err != nil {
		return State{}, fmt.Errorf("huggingface: create cache directory %s: %w", m.HubDir(), err)
	}

	return State{
		ModelDir: modelDir,
		Sentinel: sentinel,
		Skip:     false,
	}, nil
}

func (m Manager) Complete(state State) error {
	if err := m.EnsureRefsMain(state.ModelDir); err != nil {
		return fmt.Errorf("huggingface: create refs/main: %w", err)
	}

	if err := os.WriteFile(state.Sentinel, nil, 0o644); err != nil {
		return fmt.Errorf("huggingface: write sentinel %s: %w", state.Sentinel, err)
	}

	return nil
}
