package llmdinfra

import (
	"fmt"
	"sort"
	"strings"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"

	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helm_v4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(ctx *pulumi.Context, model appConfig.Model, lldChartPath string, opts ...pulumi.ResourceOption) (*helm_v4.Chart, error) {
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
	// "./../upstream/llm-d/llm-d-infra/charts/llm-d-infra"
	chartOpts := opts
	release, err := helm_v4.NewChart(ctx, releaseName, &helm_v4.ChartArgs{
		Chart:     pulumi.String(lldChartPath),
		Namespace: pulumi.String(model.Namespace),
		Name:      pulumi.String(releaseName),
		SkipAwait: pulumi.Bool(false),
		Values:    gatewayValues,
	}, chartOpts...)
	if err != nil {
		return nil, err
	}

	// Sub-provider with server-side apply disabled; inherits kubeconfig from the
	// stack-level provider (set in main.go) when kubeconfig is explicitly configured.
	serviceProviderArgs := &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(false),
	}
	if model.Kubeconfig != "" {
		serviceProviderArgs.Kubeconfig = pulumi.StringPtr(model.Kubeconfig)
	}
	serviceProvider, err := kubernetes.NewProvider(ctx, "service-provider-"+model.ReleasePostFix(), serviceProviderArgs)
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
			Labels: model.MetaLabels(),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:         pulumi.String("ExternalName"),
			ExternalName: gatewayServiceFQDN,
		},
	}, svcOpts...)
	if err != nil {
		return nil, err
	}

	// Create ConfigMap for Gateway nodeSelector injection using Istio's GatewayClass defaults.
	// Uses Istio's GatewayClass-level customization via ConfigMap
	resourceName := fmt.Sprintf("gateway-defaults-%s", model.Slug)
	cmOpts := append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{release})}, opts...)
	_, err = corev1.NewConfigMap(ctx, resourceName, &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("gateway-defaults"),
			Namespace: pulumi.String(model.Namespace),
			Labels: pulumi.StringMap{
				"gateway.istio.io/defaults-for-class": pulumi.String("istio"),
			},
		},
		Data: pulumi.StringMap{
			"deployment": pulumi.String(`
spec:
  template:
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
` + renderNodeAffinityYaml(model.NodeSelector)),
		},
	}, cmOpts...)
	if err != nil {
		return nil, err
	}

	return release, nil
}

// renderNodeAffinityYaml renders selector as a sorted list of matchExpressions lines, one
// {key, In, [value]} entry per key, to be embedded under a requiredDuringSchedulingIgnoredDuringExecution
// nodeSelectorTerms/matchExpressions block. Sorted for a stable, diff-friendly rendering.
func renderNodeAffinityYaml(selector map[string]string) string {
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf(`              - {key: %s, operator: In, values: [%q]}`, key, selector[key]))
	}
	return strings.Join(lines, "\n")
}
