package gaie

import (
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// gaieOciChart is the upstream inferencepool chart reference (requires internet on provisioner).
	gaieOciChart = "oci://registry.k8s.io/gateway-api-inference-extension/charts/inferencepool"
	// gaieLocalChart is the offline path used for on-prem air-gapped deployments.
	// Download once (from app_serving/):
	//   helm pull oci://registry.k8s.io/gateway-api-inference-extension/charts/inferencepool \
	//     --version v1.2.0 --untar --untardir charts/
	gaieLocalChart = "./charts/inferencepool"
	gaieChartVersion = "v1.2.0"
)

func Deploy(ctx *pulumi.Context, infraSim pulumi.Resource, model appConfig.Model, opts ...pulumi.ResourceOption) (*helmv3.Release, error) {
	gaieReleaseName := model.GaieReleaseName()
	gaieEppHost := pulumi.Sprintf("%s-epp.%s.svc.cluster.local", gaieReleaseName, model.Namespace)

	gaieValues := pulumi.Map{
		"inferenceExtension": pulumi.Map{
			"flags": pulumi.Map{
				"v": pulumi.Int(1),
			},
		},
		"provider": pulumi.Map{
			"name": pulumi.String("istio"),
			"istio": pulumi.Map{
				"destinationRule": pulumi.Map{
					"host": gaieEppHost,
					"trafficPolicy": pulumi.Map{
						"connectionPool": pulumi.Map{
							"http": pulumi.Map{
								"http1MaxPendingRequests":  pulumi.Int(256000),
								"maxRequestsPerConnection": pulumi.Int(256000),
								"http2MaxRequests":         pulumi.Int(256000),
								"idleTimeout":              pulumi.String("900s"),
							},
							"tcp": pulumi.Map{
								"maxConnections":        pulumi.Int(256000),
								"maxConnectionDuration": pulumi.String("1800s"),
								"connectTimeout":        pulumi.String("900s"),
							},
						},
					},
				},
			},
		},
	}

	chart := gaieOciChart
	chartVersion := pulumi.StringPtr(gaieChartVersion)
	if model.CloudProvider == "on-prem" {
		// Air-gapped: use the locally committed chart; no internet access needed.
		// Version is inferred from the local chart's Chart.yaml.
		// Prefer the absolute path resolved by the config layer (works inside
		// Pulumi automation API where cwd != project workdir); fall back to
		// the legacy relative path for direct `pulumi up` from the project dir.
		if model.GaieLocalChartPath != "" {
			chart = model.GaieLocalChartPath
		} else {
			chart = gaieLocalChart
		}
		chartVersion = nil
	}

	releaseOpts := append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{infraSim})}, opts...)
	gaieSim, err := helmv3.NewRelease(ctx, gaieReleaseName, &helmv3.ReleaseArgs{
		Namespace: pulumi.String(model.Namespace),
		Name:      pulumi.String(gaieReleaseName),
		Chart:     pulumi.String(chart),
		Version:   chartVersion,
		ValueYamlFiles: pulumi.AssetOrArchiveArray{
			pulumi.NewFileAsset(model.GaieValuesPath),
		},
		Values: gaieValues,
	}, releaseOpts...)
	if err != nil {
		return nil, err
	}

	return gaieSim, nil
}
