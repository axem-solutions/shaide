// Package gateway deploys a model's Istio inference Gateway. It replaces the
// retired llm-d-infra chart and follows the llm-d v1.0 Istio gateway recipe
// (guides/recipes/gateway/istio).
package gateway

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	gatewayClassName = "istio"
	listenerPort     = 80
)

// proxyDeploymentPatch customizes the istio-proxy Deployment Istio creates for
// the Gateway. Logging stays at warn: trace or debug logging costs a large
// share of the proxy's CPU under load. The limits follow the llm-d v1.0 Istio
// recipe; the requests are lower than its 2 CPU / 4Gi so one proxy per model
// fits next to the platform on 4 vCPU nodes.
const proxyDeploymentPatch = `spec:
  template:
    spec:
      containers:
      - name: istio-proxy
        args:
          - proxy
          - router
          - --domain
          - $(POD_NAMESPACE).svc.cluster.local
          - --proxyLogLevel
          - warn
          - --proxyComponentLogLevel
          - misc:warn
          - --log_output_level
          - default:warn
        resources:
          limits:
            cpu: "8"
            memory: 16Gi
          requests:
            cpu: 500m
            memory: 1Gi
`

// proxyServicePatch keeps the Gateway in-cluster: clients reach it through
// the discovery Service, not a load balancer.
const proxyServicePatch = `spec:
  type: ClusterIP
`

// Deploy creates the Gateway and the ExternalName Service that shaide-server
// discovers the model by. It returns the Gateway, which routes attach to.
func Deploy(
	ctx *pulumi.Context,
	cfg appConfig.Values,
	model appConfig.Model,
	category string,
	namespace pulumi.Resource,
	opts ...pulumi.ResourceOption,
) (pulumi.Resource, error) {
	gatewayName := model.GatewayName()
	labels := pulumi.StringMap{
		"app.kubernetes.io/gateway":   pulumi.String(gatewayName),
		"app.kubernetes.io/component": pulumi.String("inference-gateway"),
	}

	dependsOnNamespace := append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{namespace})}, opts...)

	parameters, err := corev1.NewConfigMap(ctx, gatewayName+"-parameters", &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(gatewayName),
			Namespace: pulumi.String(model.Namespace),
			Labels:    labels,
		},
		Data: pulumi.StringMap{
			"deployment": pulumi.String(proxyDeploymentPatch),
			"service":    pulumi.String(proxyServicePatch),
		},
	}, dependsOnNamespace...)
	if err != nil {
		return nil, fmt.Errorf("create Gateway parameters: %w", err)
	}

	gatewayLabels := pulumi.StringMap{
		// Istio only calls the InferencePool's endpoint picker from Gateways
		// that opt in.
		"istio.io/enable-inference-extproc": pulumi.String("true"),
	}
	for key, value := range labels {
		gatewayLabels[key] = value
	}

	gateway, err := apiext.NewCustomResource(ctx, gatewayName, &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("Gateway"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(gatewayName),
			Namespace: pulumi.String(model.Namespace),
			Labels:    gatewayLabels,
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": map[string]any{
				"gatewayClassName": gatewayClassName,
				"listeners": []map[string]any{{
					"name":     "default",
					"port":     listenerPort,
					"protocol": "HTTP",
					"allowedRoutes": map[string]any{
						"namespaces": map[string]any{"from": "All"},
					},
				}},
				"infrastructure": map[string]any{
					"parametersRef": map[string]any{
						"group": "",
						"kind":  "ConfigMap",
						"name":  gatewayName,
					},
				},
			},
		},
	}, append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{parameters})}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("create Gateway: %w", err)
	}

	if err := deployDiscoveryService(ctx, cfg, model, category, gateway, opts...); err != nil {
		return nil, err
	}

	return gateway, nil
}

// deployDiscoveryService creates llmd-gateway-<slug>, an ExternalName Service
// for the Service Istio provisions for the Gateway. It is labeled so a client
// with cluster-wide Service read access, e.g. shaide-server, can discover the
// model's chat-completion endpoint by label selector without knowing Istio's
// naming convention.
//
// The resource and provider names predate the move off llm-d-infra and are
// kept, so existing stacks update the Service in place.
func deployDiscoveryService(
	ctx *pulumi.Context,
	cfg appConfig.Values,
	model appConfig.Model,
	category string,
	gateway pulumi.Resource,
	opts ...pulumi.ResourceOption,
) error {
	serverSideApply := false
	serviceProvider, err := iackube.NewProvider(ctx, cfg.Kubernetes, iackube.ProviderOptions{
		Name:                  "service-provider-" + model.ReleasePostFix(),
		EnableServerSideApply: &serverSideApply,
	})
	if err != nil {
		return err
	}

	serviceName := "llmd-gateway-" + model.Slug
	target := pulumi.Sprintf("%s.%s.svc.cluster.local", model.GatewayServiceName(), model.Namespace)

	svcOpts := append([]pulumi.ResourceOption{
		pulumi.DependsOn([]pulumi.Resource{gateway}),
		pulumi.Provider(serviceProvider),
	}, opts...)
	_, err = corev1.NewService(ctx, serviceName+"-"+model.ReleasePostFix(), &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(serviceName),
			Namespace: pulumi.String(model.Namespace),
			Labels:    model.MetaLabels(category),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:         pulumi.String("ExternalName"),
			ExternalName: target,
		},
	}, svcOpts...)
	if err != nil {
		return fmt.Errorf("create discovery Service: %w", err)
	}

	return nil
}
