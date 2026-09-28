package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
)

// A missing models directory is named where the operator will look for it: in
// the storage directory they mounted, not by its path inside the container.
func TestMissingModelsDirIsNamedInTheStorageDirectory(t *testing.T) {
	imageManifest := filepath.Join(t.TempDir(), "images.yaml")
	if err := os.WriteFile(imageManifest, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	storage := paths.NewPaths(t.TempDir())
	err := LoadOptions{
		ImageManifestPath: imageManifest,
		ImagesDir:         t.TempDir(),
		ModelsDir:         filepath.Join(storage.ProjectsDir, "app-serving", "deployments", "models"),
		Describe:          storage.Describe,
	}.checkInputs()

	if err == nil {
		t.Fatal("checkInputs() accepted a missing models directory")
	}
	message := err.Error()
	if !strings.Contains(message, "models directory projects/app-serving/deployments/models in the storage directory does not exist") {
		t.Errorf("error = %q, want the directory named in the storage directory", message)
	}
	if strings.Contains(message, storage.StorageRoot) {
		t.Errorf("error = %q, names the container path", message)
	}
}
