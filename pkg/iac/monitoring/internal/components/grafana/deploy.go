package grafana

import (
	"fmt"

	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	lokiURL := fmt.Sprintf("http://loki.%s.svc.cluster.local:3100", cfg.Namespace)

	datasources := pulumi.Array{
		pulumi.Map{
			"name":      pulumi.String("Loki"),
			"type":      pulumi.String("loki"),
			"url":       pulumi.String(lokiURL),
			"access":    pulumi.String("proxy"),
			"isDefault": pulumi.Bool(true),
		},
	}
	if cfg.Components["prometheus"] {
		promURL := fmt.Sprintf("http://prometheus-server.%s.svc.cluster.local", cfg.Namespace)
		datasources = append(datasources, pulumi.Map{
			"name":   pulumi.String("Prometheus"),
			"type":   pulumi.String("prometheus"),
			"url":    pulumi.String(promURL),
			"access": pulumi.String("proxy"),
		})
	}

	values := pulumi.Map{
		"adminPassword": cfg.Grafana.AdminPassword,
		"resources": pulumi.Map{
			// Memory raised from 128Mi/256Mi after a P1 OOMKill in Poland dev —
			// loading the bundled dashboards/plugins exceeded the 256Mi limit,
			// surfacing as browser-side 502s and plugin-load errors. 256Mi/512Mi
			// is the validated-stable baseline.
			"requests": pulumi.Map{"cpu": pulumi.String("100m"), "memory": pulumi.String("256Mi")},
			"limits":   pulumi.Map{"cpu": pulumi.String("500m"), "memory": pulumi.String("512Mi")},
		},
		// Grafana is embedded read-only in the app_shaide control panel via iframe
		// (see control-panel-dashboards.md). Access control is delegated to the
		// control panel's own session cookie, not Grafana login.
		"grafana.ini": pulumi.Map{
			"server": pulumi.Map{
				// Must match the browser-facing path the iframe actually loads
				// (the control panel's Next.js proxy strips the prefix before
				// forwarding to Grafana), not Grafana's internal ClusterIP.
				// Do NOT set serve_from_sub_path — the proxy handles the subpath,
				// not Grafana; enabling it causes a redirect loop.
				"root_url": pulumi.String("%(protocol)s://%(domain)s/control-panel/grafana/"),
			},
			"security": pulumi.Map{
				"allow_embedding": pulumi.Bool(true),
				// Comma-separated list of every browser-facing origin that will
				// load the embed. Add the production origin here once it exists.
				"csrf_trusted_origins": pulumi.String("http://localhost:8080"),
			},
			"auth.anonymous": pulumi.Map{
				"enabled":  pulumi.Bool(true),
				"org_role": pulumi.String("Viewer"),
			},
			"live": pulumi.Map{
				// Neither proxy hop in front of Grafana (shaide-server's Rust
				// reverse proxy, then the Next.js rewrite) tunnels WebSocket
				// upgrades, so Grafana Live fails and panels fall back to HTTP
				// polling anyway — disable it outright to avoid the misleading
				// "Origin not allowed" toast.
				"max_connections": pulumi.Int(0),
			},
		},
		"sidecar": pulumi.Map{
			"dashboards": pulumi.Map{
				"enabled":         pulumi.Bool(true),
				"label":           pulumi.String("grafana_dashboard"),
				"labelValue":      pulumi.String("1"),
				"searchNamespace": pulumi.String("ALL"),
			},
		},
		"datasources": pulumi.Map{
			"datasources.yaml": pulumi.Map{
				"apiVersion":  pulumi.Int(1),
				"datasources": datasources,
			},
		},
	}

	_, err := helmv3.NewRelease(ctx, "grafana", &helmv3.ReleaseArgs{
		Name:        pulumi.String("grafana"),
		Chart:       pulumi.String(cfg.Grafana.ChartPath),
		Namespace:   pulumi.String(cfg.Namespace),
		Values:      values,
		WaitForJobs: pulumi.Bool(true),
		Timeout:     pulumi.Int(300),
	}, opts...)
	return err
}
