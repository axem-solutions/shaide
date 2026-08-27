package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
)

const (
	KubeconfigPathEnv = "KUBECONFIG"
	PrivateKeyPathEnv = "PRIVATE_KEY_PATH"
	HFTokenEnv        = "HF_TOKEN"

	GHCRUserEnv  = "GHCR_USERNAME"
	GHCRTokenEnv = "GHCR_TOKEN"

	DockerHubUserEnv  = "DOCKERHUB_USERNAME"
	DockerHubTokenEnv = "DOCKERHUB_PASSWORD"

	PulumiConfigPassEnv = "PULUMI_CONFIG_PASSPHRASE"
)

const (
	// Default Harbor configs.
	// These values come from the Harbor Pulumi stack configuration.
	defaultHarborNamespace      = "harbor"
	defaultHarborServiceName    = "harbor"
	defaultHarborPullSecretName = "harbor-pull-secret"
	defaultHarborLocalPort      = 5000
	defaultHarborAIProject      = "ai-models"
	defaultHarborAdminPassword  = "admin"

	DefaultHarborRobotName     = "k8s-harbor-sa"
	DefaultHarborRobotFullName = "robot$k8s-harbor-sa"
)

const (
	// Default Harbor preload settings.
	DefaultSSHPort             = 22
	DefaultStagingDir          = "/tmp/harbor-images"
	DefaultContainerdSocket    = "/run/k3s/containerd/containerd.sock"
	DefaultContainerdNamespace = "k8s.io"
	DefaultCtrFallbackPath     = "/var/lib/rancher/rke2/bin/ctr"
)

type Config struct {
	Paths       paths.Paths
	Harbor      Harbor
	HuggingFace HuggingFace
	Registries  Registries
	Pulumi      Pulumi
	Preloader   Preloader
}

type Harbor struct {
	Namespace     string
	Service       string
	PullSecret    string
	LocalPort     int
	AIProject     string
	AdminPassword string
	RobotName     string
	RobotFullName string
	Projects      []string
}

type HuggingFace struct {
	Token string
}

type RegistryCredentials struct {
	Username string
	Password string
}

type Registries struct {
	GHCR      RegistryCredentials
	DockerHub RegistryCredentials
}

type Pulumi struct {
	ConfigPassphrase string
}

type Preloader struct {
	PrivateKeyFile      string
	SSHPort             int
	StagingDir          string
	ContainerdSocket    string
	ContainerdNamespace string
	CtrFallbackPath     string
}

func Load() Config {
	paths := paths.DefaultPaths()

	if value := env(KubeconfigPathEnv); value != "" {
		paths.Kubeconfig = filepath.Clean(value)
	}

	hfToken := env(HFTokenEnv)

	return Config{
		Paths: paths,
		Harbor: Harbor{
			Namespace:     defaultHarborNamespace,
			Service:       defaultHarborServiceName,
			PullSecret:    defaultHarborPullSecretName,
			LocalPort:     defaultHarborLocalPort,
			AIProject:     defaultHarborAIProject,
			AdminPassword: defaultHarborAdminPassword,
			RobotName:     DefaultHarborRobotName,
			RobotFullName: DefaultHarborRobotFullName,
			Projects:      []string{"ai-models", "shaide", "services"},
		},
		HuggingFace: HuggingFace{
			Token: hfToken,
		},
		Registries: Registries{
			GHCR: RegistryCredentials{
				Username: env(GHCRUserEnv),
				Password: env(GHCRTokenEnv),
			},
			DockerHub: RegistryCredentials{
				Username: env(DockerHubUserEnv),
				Password: env(DockerHubTokenEnv),
			},
		},
		Pulumi: Pulumi{
			ConfigPassphrase: env(PulumiConfigPassEnv),
		},
		Preloader: Preloader{
			PrivateKeyFile:      env(PrivateKeyPathEnv),
			SSHPort:             DefaultSSHPort,
			StagingDir:          DefaultStagingDir,
			ContainerdSocket:    DefaultContainerdSocket,
			ContainerdNamespace: DefaultContainerdNamespace,
			CtrFallbackPath:     DefaultCtrFallbackPath,
		},
	}
}

func env(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
