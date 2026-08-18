package config

import (
	"fmt"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type LokiConfig struct {
	Version       string
	S3Bucket      string
	ChartPath     string
	S3ClientImage string // aws-cli-compatible image for the bucket-creation Job
	StorageClass  string
}

type GrafanaConfig struct {
	Version       string
	AdminPassword pulumi.StringOutput
	ChartPath     string
}

type AlloyConfig struct {
	Version   string
	ChartPath string
}

type PrometheusConfig struct {
	Version      string
	ChartPath    string
	StorageClass string
}

type S3Config struct {
	Endpoint string
	User     string
	Password pulumi.StringOutput
}

// Config is the typed view of Pulumi stack config used by this stack.
type Config struct {
	Namespace     string
	CloudProvider string
	Kubeconfig    string
	Components    map[string]bool
	S3            S3Config
	Loki          LokiConfig
	Grafana       GrafanaConfig
	Alloy         AlloyConfig
	Prometheus    PrometheusConfig
}

func getWithDefault(cfg *pulumiconfig.Config, key, fallback string) string {
	v := cfg.Get(key)
	if v == "" {
		return fallback
	}
	return v
}

// resolveChartPath anchors a relative chart archive path to the Pulumi project
// directory. Inline Automation API programs execute in the installer's process,
// so their filesystem paths are not implicitly relative to auto.WorkDir.
func resolveChartPath(projectDir, chartPath string) string {
	if projectDir == "" || chartPath == "" || filepath.IsAbs(chartPath) {
		return chartPath
	}

	return filepath.Join(projectDir, chartPath)
}

func loadComponents(cfg *pulumiconfig.Config) map[string]bool {
	var list []string
	cfg.RequireObject("components", &list)
	set := make(map[string]bool, len(list))
	for _, c := range list {
		set[c] = true
	}
	return set
}

func Load(ctx *pulumi.Context, projectDir string) Config {
	cfg := pulumiconfig.New(ctx, "")

	return Config{
		Namespace:     cfg.Require("namespace"),
		CloudProvider: cfg.Require("cloudProvider"),
		Kubeconfig:    cfg.Get("kubeconfig"),
		Components:    loadComponents(cfg),
		S3: S3Config{
			Endpoint: cfg.Require("s3Endpoint"),
			User:     cfg.Require("s3User"),
			Password: cfg.RequireSecret("s3Password"),
		},
		Loki: LokiConfig{
			Version:       cfg.Require("lokiVersion"),
			S3Bucket:      cfg.Require("s3BucketLoki"),
			ChartPath:     resolveChartPath(projectDir, getWithDefault(cfg, "lokiChartPath", fmt.Sprintf("charts/loki-%s.tgz", cfg.Require("lokiVersion")))),
			S3ClientImage: getWithDefault(cfg, "s3ClientImage", "amazon/aws-cli:latest"),
			StorageClass:  cfg.Get("lokiStorageClass"),
		},
		Grafana: GrafanaConfig{
			Version:       cfg.Require("grafanaVersion"),
			AdminPassword: cfg.RequireSecret("grafanaAdminPassword"),
			ChartPath:     resolveChartPath(projectDir, getWithDefault(cfg, "grafanaChartPath", fmt.Sprintf("charts/grafana-%s.tgz", cfg.Require("grafanaVersion")))),
		},
		Alloy: AlloyConfig{
			Version:   cfg.Require("alloyVersion"),
			ChartPath: resolveChartPath(projectDir, getWithDefault(cfg, "alloyChartPath", fmt.Sprintf("charts/alloy-%s.tgz", cfg.Require("alloyVersion")))),
		},
		Prometheus: PrometheusConfig{
			Version:      cfg.Require("prometheusVersion"),
			ChartPath:    resolveChartPath(projectDir, getWithDefault(cfg, "prometheusChartPath", fmt.Sprintf("charts/prometheus-%s.tgz", cfg.Require("prometheusVersion")))),
			StorageClass: cfg.Get("prometheusStorageClass"),
		},
	}
}
