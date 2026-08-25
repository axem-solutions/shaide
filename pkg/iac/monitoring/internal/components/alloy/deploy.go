package alloy

import (
	"fmt"

	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// alloyConfigTemplate is the Alloy River config for Kubernetes pod log collection.
// loki.source.kubernetes streams logs via the Kubernetes API (pods/log) — no
// hostPath mount is required. The chart's default RBAC already covers the
// necessary pods/log and discovery.kubernetes permissions.
const alloyConfigTemplate = `
discovery.kubernetes "pods" {
  role = "pod"
}

discovery.relabel "pod_logs" {
  targets = discovery.kubernetes.pods.targets

  rule {
    source_labels = ["__meta_kubernetes_namespace"]
    target_label  = "namespace"
  }

  rule {
    source_labels = ["__meta_kubernetes_pod_name"]
    target_label  = "pod"
  }

  rule {
    source_labels = ["__meta_kubernetes_pod_container_name"]
    target_label  = "container"
  }

  rule {
    source_labels = ["__meta_kubernetes_namespace", "__meta_kubernetes_pod_name"]
    separator     = "/"
    target_label  = "job"
  }

  // app.kubernetes.io/name → app  (set by app_shaide on all workloads)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_name"]
    regex         = "(.+)"
    target_label  = "app"
  }

  // app.kubernetes.io/component → component  (set by app_shaide)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_component"]
    regex         = "(.+)"
    target_label  = "component"
  }

  // app.kubernetes.io/part-of → part_of  (set by app_shaide + app_serving)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_part_of"]
    regex         = "(.+)"
    target_label  = "part_of"
  }

  // app.kubernetes.io/managed-by → managed_by  (set by app_shaide + app_serving)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_managed_by"]
    regex         = "(.+)"
    target_label  = "managed_by"
  }

  // axem.ai/platform → platform  (set by app_shaide + app_serving)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_axem_ai_platform"]
    regex         = "(.+)"
    target_label  = "platform"
  }

  // axem.ai/model-slug → model_slug  (set by app_serving; absent on app_shaide pods)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_axem_ai_model_slug"]
    regex         = "(.+)"
    target_label  = "model_slug"
  }

  // axem.ai/model-category → model_category  (set by app_serving)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_axem_ai_model_category"]
    regex         = "(.+)"
    target_label  = "model_category"
  }

  // axem.ai/nodegroup → nodegroup  (absent on on-prem workload:gpu pods)
  rule {
    source_labels = ["__meta_kubernetes_pod_label_axem_ai_nodegroup"]
    regex         = "(.+)"
    target_label  = "nodegroup"
  }

  // Drop pods that don't belong to the AI Platform — keeps kube-system,
  // cert-manager, ingress controllers, and the monitoring stack itself out of Loki.
  rule {
    source_labels = ["__meta_kubernetes_pod_label_axem_ai_platform"]
    regex         = "ai-platform"
    action        = "keep"
  }
}

loki.source.kubernetes "pod_logs" {
  targets    = discovery.relabel.pod_logs.output
  forward_to = [loki.process.drop_noisy.receiver]
}

loki.process "drop_noisy" {
  forward_to = [loki.write.default.receiver]

  // Drop Kubernetes liveness/readiness probe and metrics scrape lines.
  stage.drop {
    expression          = "(?i)(GET|POST|PUT|DELETE|HEAD)\\s+/(healthz|readyz|health|livez|metrics|ready)"
    drop_counter_reason = "health_probe"
  }

  // Drop empty and whitespace-only lines.
  stage.drop {
    expression          = "^\\s*$"
    drop_counter_reason = "empty_line"
  }
}

loki.write "default" {
  endpoint {
    url = "%s"
  }
}
`

func Deploy(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	lokiURL := fmt.Sprintf("http://loki.%s.svc.cluster.local:3100/loki/api/v1/push", cfg.Namespace)

	values := pulumi.Map{
		"alloy": pulumi.Map{
			"configMap": pulumi.Map{
				"content": pulumi.String(fmt.Sprintf(alloyConfigTemplate, lokiURL)),
			},
			"resources": pulumi.Map{
				"requests": pulumi.Map{"cpu": pulumi.String("100m"), "memory": pulumi.String("128Mi")},
				"limits":   pulumi.Map{"cpu": pulumi.String("500m"), "memory": pulumi.String("512Mi")},
			},
		},
	}

	_, err := helmv3.NewRelease(ctx, "alloy", &helmv3.ReleaseArgs{
		Name:      pulumi.String("alloy"),
		Chart:     pulumi.String(cfg.Alloy.ChartPath),
		Namespace: pulumi.String(cfg.Namespace),
		Values:    values,
		Timeout:   pulumi.Int(300),
	}, opts...)
	return err
}
