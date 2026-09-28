package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
)

// A missing model manifest is named where the operator will look for it: in
// the storage directory they mounted, not by its path inside the container.
func TestMissingModelManifestIsNamedInTheStorageDirectory(t *testing.T) {
	imageManifest := filepath.Join(t.TempDir(), "images.yaml")
	if err := os.WriteFile(imageManifest, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	storage := paths.NewPaths(t.TempDir())
	err := LoadOptions{
		ImageManifestPath: imageManifest,
		ModelManifestPath: storage.ModelManifestPath,
		ImagesDir:         t.TempDir(),
		Describe:          storage.Describe,
	}.checkInputs()

	if err == nil {
		t.Fatal("checkInputs() accepted a missing model manifest")
	}
	message := err.Error()
	if !strings.Contains(message, "model manifest manifests/models.yaml in the storage directory does not exist") {
		t.Errorf("error = %q, want the manifest named in the storage directory", message)
	}
	if strings.Contains(message, storage.StorageRoot) {
		t.Errorf("error = %q, names the container path", message)
	}
	if !strings.Contains(message, "MODEL_MANIFEST_PATH") {
		t.Errorf("error = %q, want the override mentioned", message)
	}
}
