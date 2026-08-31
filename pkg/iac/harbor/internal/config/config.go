package config

import (
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	DefaultNamespace = "harbor"
	DefaultChartPath = "./charts/harbor-1.18.2.tgz"
)

type Config struct {
	Platform   platform.Platform
	Kubernetes connection.Connection

	Storage Storage
	Mirror  Mirror

	Harbor struct {
		AdminPassword pulumi.StringOutput
		Namespace     string
		ChartPath     string
		Projects      []string

		Robot struct {
			Configured bool
			Password   pulumi.StringOutput
		}
	}

	Network struct {
		RegistryHostname string
		StaticClusterIP  string

		NodeTrustEnabled     bool
		HTTPSFastFailEnabled bool
	}
}

func Load(ctx *pulumi.Context, projectDir string) (Config, error) {
	conf := pulumiconfig.New(ctx, "harbor")

	var cfg Config

	loadPlatform(conf, &cfg)

	if err := loadHarbor(conf, &cfg, projectDir); err != nil {
		return Config{}, err
	}

	loadKubernetes(conf, &cfg)
	loadStorage(conf, &cfg)
	loadNetwork(conf, &cfg)
	loadMirror(conf, &cfg)

	if err := applyDefaults(&cfg); err != nil {
		return Config{}, fmt.Errorf("apply defaults: %w", err)
	}

	// Resolve after defaults are applied so the default chart path is anchored
	// just like an explicitly configured relative path.
	cfg.Harbor.ChartPath = resolveProjectPath(projectDir, cfg.Harbor.ChartPath)

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate Harbor config: %w", err)
	}

	return cfg, nil
}

func loadPlatform(root *pulumiconfig.Config, cfg *Config) {
	cfg.Platform = platform.Platform(root.Get("platform"))
}

func loadKubernetes(root *pulumiconfig.Config, cfg *Config) {
	cfg.Kubernetes.KubeconfigPath = root.Get("kubeconfig")
	cfg.Kubernetes.Context = root.Get("context")
}

func loadHarbor(harbor *pulumiconfig.Config, cfg *Config, projectDir string) error {
	projects := []string{
		"ai-models",
		"shaide",
		"services",
	}

	cfg.Harbor.AdminPassword = harbor.RequireSecret("adminPassword")
	cfg.Harbor.Namespace = harbor.Get("namespace")
	cfg.Harbor.ChartPath = harbor.Get("chartPath")
	cfg.Harbor.Projects = projects

	loadRobot(harbor, cfg)

	return nil
}

func loadRobot(harbor *pulumiconfig.Config, cfg *Config) {
	if harbor.Get("robotPassword") == "" {
		return
	}

	cfg.Harbor.Robot.Configured = true
	cfg.Harbor.Robot.Password = harbor.RequireSecret("robotPassword")
}

func loadStorage(harbor *pulumiconfig.Config, cfg *Config) {
	cfg.Storage.Mode = StorageMode(harbor.Get("storageMode"))
	cfg.Storage.StorageClass = harbor.Get("storageClass")
	cfg.Storage.HostPathBase = harbor.Get("hostPathBase")
	cfg.Storage.NodeHostname = harbor.Get("nodeHostname")
}

func loadNetwork(harbor *pulumiconfig.Config, cfg *Config) {
	cfg.Network.RegistryHostname = harbor.Get("registryHostname")
	cfg.Network.StaticClusterIP = harbor.Get("staticClusterIP")
	cfg.Network.NodeTrustEnabled = harbor.GetBool("nodeTrustEnabled")
	cfg.Network.HTTPSFastFailEnabled = harbor.GetBool("httpsFastFailEnabled")
}

func loadMirror(harbor *pulumiconfig.Config, cfg *Config) {
	cfg.Mirror.Enabled = harbor.GetBool("mirrorEnabled")
	cfg.Mirror.PublicImages = harbor.Get("publicImages")

	cfg.Mirror.GHCR.Org = harbor.Get("ghcrOrg")
	cfg.Mirror.GHCR.User = harbor.Get("ghcrUser")
	cfg.Mirror.GHCR.Token = harbor.GetSecret("ghcrToken")
	cfg.Mirror.GHCR.SyncMode = SyncMode(harbor.Get("ghcrSyncMode"))
	cfg.Mirror.GHCR.MinVersions = harbor.Get("ghcrMinVersions")
	cfg.Mirror.GHCR.PinnedImages = harbor.Get("ghcrPinnedImages")
}

func (cfg Config) Validate() error {
	if err := cfg.Platform.Validate(); err != nil {
		return err
	}

	if err := cfg.Storage.Validate(); err != nil {
		return err
	}

	if err := cfg.Mirror.Validate(cfg.Harbor.Robot.Configured); err != nil {
		return err
	}

	if cfg.Harbor.Namespace == "" {
		return fmt.Errorf("Harbor namespace cannot be empty")
	}

	if cfg.Harbor.ChartPath == "" {
		return fmt.Errorf("Harbor chart path cannot be empty")
	}

	return nil
}

func applyDefaults(cfg *Config) error {
	if cfg.Storage.Mode == "" {
		mode, err := defaultStorageMode(cfg.Platform)
		if err != nil {
			return err
		}

		cfg.Storage.Mode = mode
	}

	if cfg.Harbor.Namespace == "" {
		cfg.Harbor.Namespace = DefaultNamespace
	}

	if cfg.Harbor.ChartPath == "" {
		cfg.Harbor.ChartPath = DefaultChartPath
	}

	if cfg.Storage.Mode == StorageModeHostPath &&
		cfg.Storage.HostPathBase == "" {
		cfg.Storage.HostPathBase = DefaultHostPathBase
	}

	if cfg.Mirror.GHCR.SyncMode == "" {
		cfg.Mirror.GHCR.SyncMode = SyncModeAll
	}

	if cfg.Network.RegistryHostname == "" {
		cfg.Network.RegistryHostname = fmt.Sprintf("harbor.%s.svc.cluster.local", cfg.Harbor.Namespace)
	}

	return nil
}

func resolveProjectPath(projectDir, path string) string {
	if path == "" {
		return ""
	}

	if !filepath.IsAbs(path) && projectDir != "" {
		path = filepath.Join(projectDir, path)
	}

	return filepath.Clean(path)
}

func loadProjects(conf *pulumiconfig.Config) ([]string, error) {
	var projects []string

	if err := conf.GetObject("projects", &projects); err != nil {
		return nil, fmt.Errorf("read harbor:projects: %w", err)
	}

	return projects, nil
}
