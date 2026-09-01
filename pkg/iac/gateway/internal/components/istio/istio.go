package istio

import (
	"context"
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helm "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(ctx *pulumi.Context, cfg config.Config, provider *kubernetes.Provider) error {
	namespace, err := iackube.CreateNamespace(ctx, cfg.Istio.Namespace, pulumi.Provider(provider))
	if err != nil {
		return fmt.Errorf("create Istio namespace: %w", err)
	}

	serverSideApply := false
	// Use a provider that explicitly waits for the namespace and disables
	// server-side apply, matching the behavior of the original stack.
	namespaceProvider, err := iackube.NewProvider(
		ctx,
		cfg.Kubernetes,
		iackube.ProviderOptions{
			// Preserve the logical provider name so the refactor does not replace
			// the Istio charts and their child resources.
			Name:                  "ns-ready-provider",
			EnableServerSideApply: &serverSideApply,
		},
		pulumi.DependsOn([]pulumi.Resource{
			namespace,
		}),
	)
	if err != nil {
		return fmt.Errorf("create namespace-ready Kubernetes provider: %w", err)
	}

	base, err := deployBase(ctx, cfg, namespace, namespaceProvider)
	if err != nil {
		return fmt.Errorf("deploy istio-base: %w", err)
	}

	return deployIstiod(ctx, cfg, namespace, namespaceProvider, base)
}

func deployBase(
	ctx *pulumi.Context,
	cfg config.Config,
	namespace *corev1.Namespace,
	provider *kubernetes.Provider,
) (*helm.Chart, error) {
	return helm.NewChart(
		ctx,
		"istio-base",
		&helm.ChartArgs{
			Chart: pulumi.String("base"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String(
					"https://istio-release.storage.googleapis.com/charts",
				),
			},
			Version:   pulumi.String(cfg.Istio.Tag),
			Namespace: namespace.Metadata.Name(),
			Values: pulumi.Map{
				"defaultRevision": pulumi.String("default"),
			},
			SkipCrds: pulumi.Bool(false),
		},
		pulumi.Provider(provider),
		chartTransform(namespace),
	)
}

func deployIstiod(
	ctx *pulumi.Context,
	cfg config.Config,
	namespace *corev1.Namespace,
	provider *kubernetes.Provider,
	base *helm.Chart,
) error {
	_, err := helm.NewChart(
		ctx,
		"istiod",
		&helm.ChartArgs{
			Chart: pulumi.String("istiod"),
			RepositoryOpts: &helm.RepositoryOptsArgs{
				Repo: pulumi.String(
					"https://istio-release.storage.googleapis.com/charts",
				),
			},
			Version:   pulumi.String(cfg.Istio.Tag),
			Namespace: namespace.Metadata.Name(),
			Values: pulumi.Map{
				"meshConfig": pulumi.Map{
					"defaultConfig": pulumi.Map{
						"proxyMetadata": pulumi.Map{
							"ENABLE_GATEWAY_API_INFERENCE_EXTENSION": pulumi.String("true"),
						},
					},
				},
				"pilot": pulumi.Map{
					"env": pulumi.Map{
						"ENABLE_GATEWAY_API_INFERENCE_EXTENSION": pulumi.String("true"),
					},
					"resources": pulumi.Map{
						"requests": pulumi.Map{
							"cpu":    pulumi.String("250m"),
							"memory": pulumi.String("1Gi"),
						},
					},
				},
				"tag": pulumi.String(cfg.Istio.Tag),
				"hub": pulumi.String(cfg.Istio.Hub),
				"global": pulumi.Map{
					"hub": pulumi.String(cfg.Istio.Hub),
					"tag": pulumi.String(cfg.Istio.Tag),
				},
			},
			SkipAwait: pulumi.Bool(false),
		},
		pulumi.Provider(provider),
		pulumi.DependsOn([]pulumi.Resource{
			base,
		}),
		chartTransform(namespace),
	)
	if err != nil {
		return err
	}

	return nil
}

func chartTransform(
	namespace pulumi.Resource,
) pulumi.ResourceOption {
	return pulumi.Transforms([]pulumi.ResourceTransform{
		func(
			_ context.Context,
			args *pulumi.ResourceTransformArgs,
		) *pulumi.ResourceTransformResult {
			// Ensure every Helm child resource waits for the namespace to exist.
			args.Opts.DependsOn = append(
				args.Opts.DependsOn,
				namespace,
			)

			if args.Type ==
				"kubernetes:admissionregistration.k8s.io/v1:ValidatingWebhookConfiguration" {

				args.Opts.IgnoreChanges = append(
					args.Opts.IgnoreChanges,
					"webhooks[*].failurePolicy",
					"webhooks[*].clientConfig",
					"webhooks[*].admissionReviewVersions",
				)
			}

			return &pulumi.ResourceTransformResult{
				Props: args.Props,
				Opts:  args.Opts,
			}
		},
	})
}
