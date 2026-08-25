package preloader

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/config"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
)

const defaultImageCheckPattern = "goharbor/"

type PreloaderOptions struct {
	// Host is the SSH hostname or IP address of the target Harbor node.
	//
	// Required.
	Host string

	// User is the SSH username to connect to the target Harbor node.
	//
	// Required.
	User string

	// PrivateKeyPath is the local filesystem path to the SSh private key used
	// by the provisioning machine to connect to the Harbor node.
	//
	// Required.
	PrivateKeyPath string

	// Port is the SSH port on the target Harbor node.
	//
	// Optional.
	// Default: 22
	Port int

	// NodeName is the Kubernetes node name for the Harbor node.
	NodeName string

	// LocalImageDir is the local directory containing the image archives named
	// by the Harbor image manifest.
	//
	// Required when manifest file entries are relative paths.
	LocalImageDir string

	// StagingDir is the base directory on the Harbor node where image archives
	// are uploaded before import.
	//
	// Optional.
	// Default: /tmp/harbor-images.
	StagingDir string

	// LocalDir is the base directory where the images archives are downloaded:
	LocalDir string

	// ContainerdSocket is the path to the containerd socket on the Harbor node.
	//
	// RKE2/K3s default:
	//   /run/k3s/containerd/containerd.sock
	//
	// Optional.
	// Default: /run/k3s/containerd/containerd.sock.
	ContainerdSocket string

	// ContainerdNamespace is the containerd namespace where Kubernetes pod
	// images are stored.
	//
	// RKE2/Kubernetes uses:
	//  k8s.io
	//
	// Optional.
	// Default: k8s.io.
	ContainerdNamespace string

	// CtrPath is the path to the ctr binary on the Harbor node.
	// RKE2 commonly places ctr at:
	//   /var/lib/rancher/rke2/bin/ctr
	//
	// If empty, the preloader should discover it with command -v ctr and then
	// fall back to /var/lib/rancher/rke2/bin/ctr.
	// Optional.
	// Default: discover from PATH, then /var/lib/rancher/rke2/bin/ctr.
	CtrPath string

	// ImageCheckPattern is used as a broad containerd image-list match to
	// detect previously imported Harbor bootstrap images.
	//
	// Optional.
	// Default: goharbor/
	ImageCheckPattern string

	// Progressf receives Harbor node preload upload and import progress.
	Progressf func(progress.Event)

	Logf func(format string, args ...any)
}

func validateOptions(opts PreloaderOptions) error {
	if opts.Host == "" {
		return fmt.Errorf("host is required")
	}

	if opts.User == "" {
		return fmt.Errorf("SSH user is required")
	}

	if opts.PrivateKeyPath == "" {
		return fmt.Errorf("SSH private key path is required")
	}

	if opts.Port < 0 {
		return fmt.Errorf("SSH port must be positive, got %d", opts.Port)
	}

	if opts.Logf == nil {
		return fmt.Errorf("logging function is required")
	}

	if opts.LocalDir == "" {
		return fmt.Errorf("localDir is required")
	}

	return nil
}

func applyOptions(opts PreloaderOptions) PreloaderOptions {
	if opts.Port == 0 {
		opts.Port = config.DefaultSSHPort
	}

	if opts.StagingDir == "" {
		opts.StagingDir = config.DefaultStagingDir
	}

	if opts.ContainerdSocket == "" {
		opts.ContainerdSocket = config.DefaultContainerdSocket
	}

	if opts.ContainerdNamespace == "" {
		opts.ContainerdNamespace = config.DefaultContainerdNamespace
	}

	if opts.ImageCheckPattern == "" {
		opts.ImageCheckPattern = defaultImageCheckPattern
	}

	return opts
}
