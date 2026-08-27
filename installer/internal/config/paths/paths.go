package paths

import "path/filepath"

const (
	// Mount the kubeconfig to:
	// -v "${KUBECONFIG_PATH}:/.kube/config"
	defaultKubeconfigPath = "/.kube/config"

	// DefaultBindMount is the root of installer-owned writable storage.
	defaultBindMount = "/var/shaide-installer"

	// TODO: add comment
	defaultProjectsSourceDir = "/opt/shaide-installer/projects"
	defaultManifestsDir      = "/opt/shaide-installer/manifests"
	defaultImagesDir         = "/opt/shaide-installer/images"
)

type Paths struct {
	Kubeconfig string

	// Read-only files copied into the Docker image.
	ProjectsSourceDir string
	ManifestsDir      string
	ImagesDir         string

	// Writable installer storage.
	StorageRoot   string
	ProjectsDir   string
	ModelCache    string
	UploadState   string
	ArtifactCache string
	PulumiState   string
	Logs          string
	Temp          string
}

func DefaultPaths() Paths {
	return NewPaths(defaultBindMount)
}

func NewPaths(storageRoot string) Paths {
	storageRoot = filepath.Clean(storageRoot)

	return Paths{
		Kubeconfig: defaultKubeconfigPath,

		ProjectsSourceDir: defaultProjectsSourceDir,
		ManifestsDir:      defaultManifestsDir,
		ImagesDir:         defaultImagesDir,

		StorageRoot:   storageRoot,
		ProjectsDir:   filepath.Join(storageRoot, "projects"),
		ModelCache:    filepath.Join(storageRoot, "model-cache"),
		UploadState:   filepath.Join(storageRoot, "upload-state"),
		ArtifactCache: filepath.Join(storageRoot, "artifact-cache"),
		PulumiState:   filepath.Join(storageRoot, "pulumi-state"),
		Logs:          filepath.Join(storageRoot, "logs"),
		Temp:          filepath.Join(storageRoot, "tmp"),
	}
}

func (p Paths) StorageDirs() []string {
	return []string{
		p.StorageRoot,
		p.ModelCache,
		p.UploadState,
		p.ArtifactCache,
		p.PulumiState,
		p.Logs,
		p.Temp,
	}
}
