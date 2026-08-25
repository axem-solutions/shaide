package prometheus

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	values := pulumi.Map{
		"server": pulumi.Map{
			"retention":        pulumi.String("15d"),
			"persistentVolume": serverPersistence(cfg),
			"resources": pulumi.Map{
				"requests": pulumi.Map{"cpu": pulumi.String("250m"), "memory": pulumi.String("512Mi")},
				"limits":   pulumi.Map{"cpu": pulumi.String("1"), "memory": pulumi.String("1Gi")},
			},
		},
		// No alerting rules are defined yet — Alertmanager sits idle without them.
		"alertmanager": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"prometheus-pushgateway": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"kube-state-metrics": pulumi.Map{
			"resources": pulumi.Map{
				"requests": pulumi.Map{"cpu": pulumi.String("50m"), "memory": pulumi.String("64Mi")},
				"limits":   pulumi.Map{"cpu": pulumi.String("200m"), "memory": pulumi.String("256Mi")},
			},
		},
		"prometheus-node-exporter": pulumi.Map{
			"resources": pulumi.Map{
				"requests": pulumi.Map{"cpu": pulumi.String("50m"), "memory": pulumi.String("32Mi")},
				"limits":   pulumi.Map{"cpu": pulumi.String("200m"), "memory": pulumi.String("128Mi")},
			},
		},
	}

	_, err := helmv3.NewRelease(ctx, "prometheus", &helmv3.ReleaseArgs{
		Name:        pulumi.String("prometheus"),
		Chart:       pulumi.String(cfg.Prometheus.ChartPath),
		Namespace:   pulumi.String(cfg.Namespace),
		Values:      values,
		WaitForJobs: pulumi.Bool(true),
		Timeout:     pulumi.Int(300),
	}, opts...)
	return err
}

func serverPersistence(cfg appconfig.Config) pulumi.Map {
	m := pulumi.Map{
		"enabled": pulumi.Bool(true),
		"size":    pulumi.String("10Gi"),
	}
	if cfg.Prometheus.StorageClass != "" {
		m["storageClass"] = pulumi.String(cfg.Prometheus.StorageClass)
	}
	return m
}
