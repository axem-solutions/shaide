package paths

import "path/filepath"

const (
	// Mount the kubeconfig to:
	// -v "${KUBECONFIG_PATH}:/.kube/config"
	defaultKubeconfigPath = "/.kube/config"

	// DefaultBindMount is the root of installer-owned writable storage.
	defaultBindMount = "/var/shaide-installer"

	// Read-only installer payload, baked into the image at build time.
	//
	// The Pulumi projects and the image manifest ship with the installer
	// because they change with the installer release. The model manifest does
	// not live here: models change per deployment, so it is supplied at runtime
	// under the writable storage root instead — see ModelManifestPath.
	defaultProjectsSourceDir = "/opt/shaide-installer/projects"
	defaultImageManifestPath = "/opt/shaide-installer/manifests/images.yaml"

	// Container image archives for manifest entries whose source is "archive".
	// Created by the image build so a missing archive fails on the file, not
	// on the directory.
	defaultImagesDir = "/opt/shaide-installer/images"
)

type Paths struct {
	Kubeconfig string

	// Read-only files copied into the Docker image.
	ProjectsSourceDir string
	ImageManifestPath string
	ImagesDir         string

	// Writable installer storage.
	StorageRoot string

	// ManifestsDir is created on every run so the operator has somewhere to
	// place the model manifest before starting the installer.
	ManifestsDir      string
	ModelManifestPath string
	ProjectsDir       string
	ModelCache        string
	UploadState       string
	ArtifactCache     string
	PulumiState       string
	Logs              string
	Temp              string
}

func DefaultPaths() Paths {
	return NewPaths(defaultBindMount)
}

func NewPaths(storageRoot string) Paths {
	storageRoot = filepath.Clean(storageRoot)

	return Paths{
		Kubeconfig: defaultKubeconfigPath,

		ProjectsSourceDir: defaultProjectsSourceDir,
		ImageManifestPath: defaultImageManifestPath,
		ImagesDir:         defaultImagesDir,

		StorageRoot:       storageRoot,
		ManifestsDir:      filepath.Join(storageRoot, "manifests"),
		ModelManifestPath: filepath.Join(storageRoot, "manifests", "models.yaml"),
		ProjectsDir:       filepath.Join(storageRoot, "projects"),
		ModelCache:        filepath.Join(storageRoot, "model-cache"),
		UploadState:       filepath.Join(storageRoot, "upload-state"),
		ArtifactCache:     filepath.Join(storageRoot, "artifact-cache"),
		PulumiState:       filepath.Join(storageRoot, "pulumi-state"),
		Logs:              filepath.Join(storageRoot, "logs"),
		Temp:              filepath.Join(storageRoot, "tmp"),
	}
}

func (p Paths) StorageDirs() []string {
	return []string{
		p.StorageRoot,
		p.ManifestsDir,
		p.ModelCache,
		p.UploadState,
		p.ArtifactCache,
		p.PulumiState,
		p.Logs,
		p.Temp,
	}
}
