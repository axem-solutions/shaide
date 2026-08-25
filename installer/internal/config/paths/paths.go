package paths

import "path/filepath"

const (
	// Mount the kubeconfig to:
	// -v "${KUBECONFIG_PATH}:/.kube/config"
	defaultKubeconfigPath = "/.kube/config"

	// Mount the bundle to:
	// -v "${BUNDLE_PATH}:/.bundle/bundle.tar.gz:ro"
	defaultBundlePath = "/.bundle/bundle.tar.gz"

	// DefaultBindMount is the root of installer-owned writable storage.
	DefaultBindMount = "/var/shaide-installer"
)

type Paths struct {
	Kubeconfig    string
	BundleArchive string

	StorageRoot   string
	Bundle        string
	ModelCache    string
	UploadState   string
	ArtifactCache string
	PulumiState   string
	Logs          string
	Temp          string
}

func DefaultPaths() Paths {
	return NewPaths(DefaultBindMount)
}

func NewPaths(storageRoot string) Paths {
	storageRoot = filepath.Clean(storageRoot)

	return Paths{
		Kubeconfig:    defaultKubeconfigPath,
		BundleArchive: defaultBundlePath,
		StorageRoot:   storageRoot,
		Bundle:        filepath.Join(storageRoot, "bundle"),
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
