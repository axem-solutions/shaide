package chart

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/config"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	ServiceName = "harbor"
	ReleaseName = "harbor"
)

func Deploy(
	ctx *pulumi.Context,
	provider *kubernetes.Provider,
	namespace *corev1.Namespace,
	cfg config.Config,
	storageDependencies []pulumi.Resource,
) (*helmv3.Release, error) {
	deps := make(
		[]pulumi.Resource,
		0,
		1+len(storageDependencies),
	)

	deps = append(deps, namespace)
	deps = append(deps, storageDependencies...)

	return helmv3.NewRelease(
		ctx,
		ReleaseName,
		&helmv3.ReleaseArgs{
			Chart:     pulumi.String(cfg.Harbor.ChartPath),
			Namespace: pulumi.String(cfg.Harbor.Namespace),
			Name:      pulumi.String(ReleaseName),

			Values: BuildValues(cfg),

			WaitForJobs: pulumi.Bool(true),
			Timeout:     pulumi.Int(600),
		},
		pulumi.Provider(provider),
		pulumi.DependsOn(deps),
	)
}
