package dashboards

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type dashboard struct {
	name string
	json string
}

func Deploy(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	items := []dashboard{
		{"app-shaide-logs", appShaideDashboard},
		{"app-serving-logs", appServingDashboard},
		{"error-logs", errorsDashboard},
		{"platform-overview", platformDashboard},
	}
	if cfg.Components["prometheus"] {
		items = append(items,
			dashboard{"cluster-nodes", clusterNodesDashboard},
			dashboard{"cluster-pods", clusterPodsDashboard},
		)
	}

	for _, d := range items {
		_, err := corev1.NewConfigMap(ctx, "dashboard-"+d.name, &corev1.ConfigMapArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("grafana-dashboard-" + d.name),
				Namespace: pulumi.String(cfg.Namespace),
				Labels: pulumi.StringMap{
					"grafana_dashboard": pulumi.String("1"),
				},
			},
			Data: pulumi.StringMap{
				d.name + ".json": pulumi.String(d.json),
			},
		}, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}
