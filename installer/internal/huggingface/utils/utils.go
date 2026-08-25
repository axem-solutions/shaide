package utils

import (
	"os"
	"path/filepath"
	"strings"
)

type Info struct {
	Bytes int64
	Files int
}

func Current(modelDir string) Info {
	if progress, ok := snapshotProgress(modelDir); ok {
		blobProgress := treeProgress(filepath.Join(modelDir, "blobs"), false)
		if blobProgress.Bytes > progress.Bytes {
			progress.Bytes = blobProgress.Bytes
		}
		return progress
	}

	return treeProgress(filepath.Join(modelDir, "blobs"), false)
}

func snapshotProgress(modelDir string) (Info, bool) {
	snapshotsDir := filepath.Join(modelDir, "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return Info{}, false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			return treeProgress(filepath.Join(snapshotsDir, entry.Name()), true), true
		}
	}

	return Info{}, false
}

func treeProgress(root string, followSymlinks bool) Info {
	var progress Info

	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}

		info, statErr := entry.Info()
		if followSymlinks {
			info, statErr = os.Stat(path)
		}
		if statErr != nil {
			return nil
		}

		if strings.HasSuffix(path, ".incomplete") {
			progress.Bytes += info.Size()
			return nil
		}

		progress.Files++
		progress.Bytes += info.Size()
		return nil
	})

	return progress
}
