package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
	"github.com/axem-solutions/ai_platform/installer/internal/config/resources"
)

const (
	KubeconfigPathEnv = "KUBECONFIG"
	PrivateKeyPathEnv = "PRIVATE_KEY_PATH"

	// ModelManifestPathEnv overrides where the model manifest is read from.
	// The manifest is supplied at runtime rather than baked into the image, so
	// this is the escape hatch for keeping it somewhere other than the default
	// path under the storage mount.
	ModelManifestPathEnv = "MODEL_MANIFEST_PATH"
	HFTokenEnv           = "HF_TOKEN"

	GHCRUserEnv  = "GHCR_USERNAME"
	GHCRTokenEnv = "GHCR_TOKEN"

	DockerHubUserEnv  = "DOCKERHUB_USERNAME"
	DockerHubTokenEnv = "DOCKERHUB_PASSWORD"

	PulumiConfigPassEnv = "PULUMI_CONFIG_PASSPHRASE"

	// Resource limits bound what a model transfer takes of the machine. They
	// are percentages of what the process may actually use, so the same
	// defaults suit a laptop and a build server.
	CPUPercentEnv      = "RESOURCE_CPU_PERCENT"
	MemoryPercentEnv   = "RESOURCE_MEMORY_PERCENT"
	HighPerformanceEnv = "TRANSFER_HIGH_PERFORMANCE"
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
	Resources   Resources
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

// Resources is the share of the machine a transfer may use.
type Resources struct {
	Limits resources.Limits

	// HighPerformance lets Xet scale to the whole host, ignoring Limits. It
	// belongs on a machine that is doing nothing else.
	HighPerformance bool
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

	if value := env(ModelManifestPathEnv); value != "" {
		paths.ModelManifestPath = filepath.Clean(value)
	}

	hfToken := env(HFTokenEnv)

	resourceLimits := resources.Limits{
		CPUPercent:    envInt(CPUPercentEnv),
		MemoryPercent: envInt(MemoryPercentEnv),
	}

	return Config{
		Paths: paths,
		Resources: Resources{
			Limits:          resourceLimits,
			HighPerformance: envBool(HighPerformanceEnv),
		},
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

// envInt reads an optional numeric setting. An unset or unparseable value
// leaves the default in place rather than failing the run over a tuning knob.
func envInt(key string) int {
	value, err := strconv.Atoi(env(key))
	if err != nil {
		return 0
	}

	return value
}

// envBool treats the usual affirmatives as set, so the variable reads the same
// whether it is written as 1, true or yes.
func envBool(key string) bool {
	switch strings.ToLower(env(key)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
