package llmdinfra

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helm_v4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(
	ctx *pulumi.Context,
	cfg appConfig.Values,
	model appConfig.Model,
	category string,
	opts ...pulumi.ResourceOption,
) (*helm_v4.Chart, error) {
	releaseName := model.InfraReleaseName()

	gaieReleaseName := model.GaieReleaseName()
	gaieEppHost := pulumi.Sprintf("%s-epp.%s.svc.cluster.local", gaieReleaseName, model.Namespace)

	gatewayValues := pulumi.Map{
		"gateway": pulumi.Map{
			"gatewayClassName": pulumi.String("istio"),
			"gatewayParameters": pulumi.Map{
				"logLevel": pulumi.String("error"),
			},
			// Explicitly set listeners without unsupported 'path' field for Gateway v1
			"listeners": pulumi.Array{
				pulumi.Map{
					"name":     pulumi.String("default"),
					"port":     pulumi.Int(80),
					"protocol": pulumi.String("HTTP"),
					"allowedRoutes": pulumi.Map{
						"namespaces": pulumi.Map{
							"from": pulumi.String("All"),
						},
					},
				},
			},
			"service": pulumi.Map{
				"type": pulumi.String("ClusterIP"),
			},
			"destinationRule": pulumi.Map{
				"enabled": pulumi.Bool(true),
				"host":    gaieEppHost,
				"trafficPolicy": pulumi.Map{
					"tls": pulumi.Map{
						"mode":               pulumi.String("SIMPLE"),
						"insecureSkipVerify": pulumi.Bool(true),
					},
				},
			},
		},
	}
	chartOpts := opts
	release, err := helm_v4.NewChart(ctx, releaseName, &helm_v4.ChartArgs{
		Chart:     pulumi.String(cfg.LLMd.ChartPath),
		Namespace: pulumi.String(model.Namespace),
		Name:      pulumi.String(releaseName),
		SkipAwait: pulumi.Bool(false),
		Values:    gatewayValues,
	}, chartOpts...)
	if err != nil {
		return nil, err
	}

	// Helm sub-providers use the same kubeconfig and context as the stack-level
	// provider while retaining their historical server-side apply setting.
	serverSideApply := false
	serviceProvider, err := iackube.NewProvider(ctx, cfg.Kubernetes, iackube.ProviderOptions{
		Name:                  "service-provider-" + model.ReleasePostFix(),
		EnableServerSideApply: &serverSideApply,
	})
	if err != nil {
		return nil, err
	}

	gatewayServiceName := fmt.Sprintf("llmd-gateway-%s", model.Slug)
	gatewayServiceFQDN := pulumi.Sprintf("%s-istio.%s.svc.cluster.local", model.GatewayName(), model.Namespace)
	svcOpts := append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{release}), pulumi.Provider(serviceProvider)}, opts...)
	_, err = corev1.NewService(ctx, gatewayServiceName+"-"+model.ReleasePostFix(), &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(gatewayServiceName),
			Namespace: pulumi.String(model.Namespace),
			// Labeled so a client with cluster-wide Service read access —
			// e.g. shaide-server — can discover this model's chat-completion
			// endpoint directly via label selector, without needing to know
			// the Istio-generated Service naming convention it wraps.
			Labels: model.MetaLabels(category),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:         pulumi.String("ExternalName"),
			ExternalName: gatewayServiceFQDN,
		},
	}, svcOpts...)
	if err != nil {
		return nil, err
	}

	return release, nil
}
