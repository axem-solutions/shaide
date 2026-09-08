package config

import (
	"fmt"
	"path/filepath"

	kubernetes "github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	DefaultNamespace = "harbor"
	DefaultChartPath = "./charts/harbor-1.18.2.tgz"
)

type Values struct {
	Platform   platform.Platform
	Kubernetes kubernetes.Connection

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

func (c Config) Load(ctx *pulumi.Context) (Values, error) {
	values, err := c.definition.Load(ctx)
	if err != nil {
		return Values{}, fmt.Errorf("load stack config: %w", err)
	}

	values.Harbor.Projects = []string{
		"ai-models",
		"shaide",
		"services",
	}

	if err := c.applyDefaults(&values); err != nil {
		return Values{}, fmt.Errorf("apply defaults: %w", err)
	}

	// Resolve after defaults are applied so the default chart path is anchored
	// just like an explicitly configured relative path.
	values.Harbor.ChartPath = resolveProjectPath(c.ProjectDir(), values.Harbor.ChartPath)

	if err := c.Validate(values); err != nil {
		return Values{}, fmt.Errorf("validate Harbor config: %w", err)
	}

	return values, nil
}

func (c Config) Validate(cfg Values) error {
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

func (c Config) applyDefaults(cfg *Values) error {
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
