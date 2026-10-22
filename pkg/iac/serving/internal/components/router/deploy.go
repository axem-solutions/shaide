package router

import (
	"fmt"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Deploy installs the router release. The chart is the vendored
// llm-d-router-gateway (see app_serving/charts/README.md); a Helm release
// rather than rendered manifests, because the chart leaves the namespace of
// some objects to Helm.
func Deploy(
	ctx *pulumi.Context,
	cfg appConfig.Values,
	model appConfig.Model,
	poolSelector map[string]string,
	pullSecret string,
	dependsOn []pulumi.Resource,
	opts ...pulumi.ResourceOption,
) (*helmv3.Release, error) {
	values, err := Values(Input{
		Definition:   model.Definition,
		Image:        cfg.RouterImage,
		PoolSelector: poolSelector,
		PullSecret:   pullSecret,
	})
	if err != nil {
		return nil, err
	}

	releaseName := model.RouterReleaseName()
	releaseOpts := append([]pulumi.ResourceOption{pulumi.DependsOn(dependsOn)}, opts...)

	release, err := helmv3.NewRelease(ctx, releaseName, &helmv3.ReleaseArgs{
		Name:      pulumi.String(releaseName),
		Namespace: pulumi.String(model.Namespace),
		Chart:     pulumi.String(cfg.RouterChartPath),
		Values:    pulumi.ToMap(values),
	}, releaseOpts...)
	if err != nil {
		return nil, fmt.Errorf("install router release: %w", err)
	}

	return release, nil
}
