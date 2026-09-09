package config

import (
	"fmt"
	"path/filepath"
	"strings"

	kubernetes "github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	DefaultNamespace = "monitoring"

	DefaultS3Endpoint    = "http://rustfs.app-shaide.svc.cluster.local:9000"
	DefaultS3User        = "rustfsuser"
	DefaultS3BucketLoki  = "loki-chunks"
	DefaultS3ClientImage = "amazon/aws-cli:latest"

	DefaultLokiVersion       = "14.1.0"
	DefaultGrafanaVersion    = "12.3.2"
	DefaultAlloyVersion      = "1.8.1"
	DefaultPrometheusVersion = "29.21.0"

	DefaultStorageClass = "hostpath"
)

type Values struct {
	Platform   platform.Platform
	Kubernetes kubernetes.Connection

	Namespace  string
	Components map[string]bool

	Loki struct {
		Version   string
		ChartPath string

		S3Bucket      string
		S3ClientImage string // aws-cli-compatible image for the bucket-creation Job
		StorageClass  string
	}

	Grafana struct {
		Version   string
		ChartPath string

		AdminPassword pulumi.StringOutput
	}

	Alloy struct {
		Version   string
		ChartPath string
	}

	Prometheus struct {
		Version   string
		ChartPath string

		StorageClass string
	}

	S3 struct {
		Endpoint string
		User     string
		Password pulumi.StringOutput
	}
}

func (c Config) Load(ctx *pulumi.Context) (Values, error) {
	values, err := c.definition.Load(ctx)
	if err != nil {
		return Values{}, fmt.Errorf("load stack config: %w", err)
	}

	c.applyDefaults(&values)
	c.resolveChartPaths(&values)

	if err := c.Validate(values); err != nil {
		return Values{}, fmt.Errorf("validate monitoring config: %w", err)
	}

	return values, nil
}

func (c Config) Validate(cfg Values) error {
	if err := cfg.Platform.Validate(); err != nil {
		return err
	}

	if strings.TrimSpace(cfg.Namespace) == "" {
		return fmt.Errorf("monitoring namespace cannot be empty")
	}

	enabledComponents := 0
	for component, enabled := range cfg.Components {
		if _, ok := supportedComponents[component]; !ok {
			return fmt.Errorf("unsupported monitoring component %q", component)
		}

		if enabled {
			enabledComponents++
		}
	}

	if enabledComponents == 0 {
		return fmt.Errorf("at least one monitoring component must be enabled")
	}

	if cfg.Components[ComponentLoki] {
		if err := requireValues(
			requiredValue{"Loki version", cfg.Loki.Version},
			requiredValue{"Loki chart path", cfg.Loki.ChartPath},
			requiredValue{"Loki S3 bucket", cfg.Loki.S3Bucket},
			requiredValue{"S3 client image", cfg.Loki.S3ClientImage},
			requiredValue{"S3 endpoint", cfg.S3.Endpoint},
			requiredValue{"S3 user", cfg.S3.User},
		); err != nil {
			return err
		}
	}

	if cfg.Components[ComponentGrafana] {
		if err := requireValues(
			requiredValue{"Grafana version", cfg.Grafana.Version},
			requiredValue{"Grafana chart path", cfg.Grafana.ChartPath},
		); err != nil {
			return err
		}
	}

	if cfg.Components[ComponentAlloy] {
		if err := requireValues(
			requiredValue{"Alloy version", cfg.Alloy.Version},
			requiredValue{"Alloy chart path", cfg.Alloy.ChartPath},
		); err != nil {
			return err
		}
	}

	if cfg.Components[ComponentPrometheus] {
		if err := requireValues(
			requiredValue{"Prometheus version", cfg.Prometheus.Version},
			requiredValue{"Prometheus chart path", cfg.Prometheus.ChartPath},
		); err != nil {
			return err
		}
	}

	return nil
}

func (c Config) applyDefaults(cfg *Values) {
	if cfg.Namespace == "" {
		cfg.Namespace = DefaultNamespace
	}

	if len(cfg.Components) == 0 {
		cfg.Components = map[string]bool{ComponentLoki: true}
	}

	if cfg.S3.Endpoint == "" {
		cfg.S3.Endpoint = DefaultS3Endpoint
	}

	if cfg.S3.User == "" {
		cfg.S3.User = DefaultS3User
	}

	if cfg.Loki.S3Bucket == "" {
		cfg.Loki.S3Bucket = DefaultS3BucketLoki
	}

	if cfg.Loki.S3ClientImage == "" {
		cfg.Loki.S3ClientImage = DefaultS3ClientImage
	}

	if cfg.Loki.Version == "" {
		cfg.Loki.Version = DefaultLokiVersion
	}

	if cfg.Grafana.Version == "" {
		cfg.Grafana.Version = DefaultGrafanaVersion
	}

	if cfg.Alloy.Version == "" {
		cfg.Alloy.Version = DefaultAlloyVersion
	}

	if cfg.Prometheus.Version == "" {
		cfg.Prometheus.Version = DefaultPrometheusVersion
	}

	if cfg.Loki.ChartPath == "" {
		cfg.Loki.ChartPath = chartPath(ComponentLoki, cfg.Loki.Version)
	}

	if cfg.Grafana.ChartPath == "" {
		cfg.Grafana.ChartPath = chartPath(ComponentGrafana, cfg.Grafana.Version)
	}

	if cfg.Alloy.ChartPath == "" {
		cfg.Alloy.ChartPath = chartPath(ComponentAlloy, cfg.Alloy.Version)
	}

	if cfg.Prometheus.ChartPath == "" {
		cfg.Prometheus.ChartPath = chartPath(ComponentPrometheus, cfg.Prometheus.Version)
	}

	if cfg.Loki.StorageClass == "" {
		cfg.Loki.StorageClass = DefaultStorageClass
	}

	if cfg.Prometheus.StorageClass == "" {
		cfg.Prometheus.StorageClass = DefaultStorageClass
	}
}

func (c Config) resolveChartPaths(cfg *Values) {
	cfg.Loki.ChartPath = resolveProjectPath(c.ProjectDir(), cfg.Loki.ChartPath)
	cfg.Grafana.ChartPath = resolveProjectPath(c.ProjectDir(), cfg.Grafana.ChartPath)
	cfg.Alloy.ChartPath = resolveProjectPath(c.ProjectDir(), cfg.Alloy.ChartPath)
	cfg.Prometheus.ChartPath = resolveProjectPath(c.ProjectDir(), cfg.Prometheus.ChartPath)
}

func chartPath(component, version string) string {
	return fmt.Sprintf("charts/%s-%s.tgz", component, version)
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

type requiredValue struct {
	name  string
	value string
}

func requireValues(values ...requiredValue) error {
	for _, value := range values {
		if strings.TrimSpace(value.value) == "" {
			return fmt.Errorf("%s cannot be empty", value.name)
		}
	}

	return nil
}
