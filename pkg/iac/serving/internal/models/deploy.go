package models

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/embeddingservice"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/gateway"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/httproute"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/modelserver"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/router"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/legacy"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/platform"
	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"k8s.io/client-go/kubernetes"
)

// Deploy deploys every enabled model. Generative and embedder models share the
// same resource chain; only their category label and final endpoint differ.
func Deploy(ctx *pulumi.Context, provider pulumi.ProviderResource, cfg config.Values) error {
	groups := []struct {
		category string
		models   []config.Model
	}{
		{category: config.CategoryGenerative, models: cfg.Models.Generative},
		{category: config.CategoryEmbedder, models: cfg.Models.Embedder},
	}

	if err := removeLegacyModelServers(ctx, cfg, groups[0].models, groups[1].models); err != nil {
		return err
	}

	for _, group := range groups {
		for _, model := range group.models {
			if !model.Enabled {
				continue
			}

			if err := deployModel(ctx, provider, cfg, model, group.category); err != nil {
				return fmt.Errorf("deploy %s model %q: %w", group.category, model.Name, err)
			}
		}
	}

	return nil
}

// removeLegacyModelServers frees the GPUs the retired llm-d-modelservice
// chart's pods hold, so the new model servers can start (see package legacy).
// A preview changes nothing on the cluster.
func removeLegacyModelServers(ctx *pulumi.Context, cfg config.Values, modelGroups ...[]config.Model) error {
	if ctx.DryRun() {
		return nil
	}

	restConfig, err := connection.BuildRestConfig(cfg.Kubernetes)
	if err != nil {
		return fmt.Errorf("build Kubernetes REST config: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("build Kubernetes client: %w", err)
	}

	logf := func(format string, args ...any) {
		_ = ctx.Log.Info(fmt.Sprintf(format, args...), nil)
	}

	for _, models := range modelGroups {
		for _, model := range models {
			if !model.Enabled {
				continue
			}

			if err := legacy.RemoveModelServers(ctx.Context(), client, model.Namespace, model.LegacyModelServiceLabel(), logf); err != nil {
				return fmt.Errorf("model %q: %w", model.Name, err)
			}
		}
	}

	return nil
}

func deployModel(ctx *pulumi.Context, provider pulumi.ProviderResource, cfg config.Values, model config.Model, category string) error {
	providerOpt := pulumi.Provider(provider)

	namespace, err := iackube.CreateNamespace(
		ctx,
		model.Namespace,
		iackube.NamespaceOptions{Labels: model.MetaLabels(category)},
		providerOpt,
	)
	if err != nil {
		return fmt.Errorf("create namespace: %w", err)
	}

	// Pods pull the mirrored images from Harbor with these credentials.
	pullSecret := ""
	serverDeps := []pulumi.Resource{namespace}
	routerDeps := []pulumi.Resource{namespace}

	if cfg.Harbor.TokenSet {
		secret, err := platform.CreateHarborPullSecret(ctx, cfg, model, namespace, providerOpt)
		if err != nil {
			return fmt.Errorf("create Harbor pull secret: %w", err)
		}
		pullSecret = platform.HarborPullSecretName
		serverDeps = append(serverDeps, secret)
		routerDeps = append(routerDeps, secret)
	}

	modelPVC, orasJob, err := platform.PrepareModelStorage(ctx, cfg, model, namespace, providerOpt)
	if err != nil {
		return fmt.Errorf("prepare model storage: %w", err)
	}

	// The pods must not start before the ORAS Job has filled the volume: with
	// HF_HUB_OFFLINE=1 vLLM would find an empty cache and exit.
	if modelPVC != nil {
		serverDeps = append(serverDeps, modelPVC)
	}
	if orasJob != nil {
		serverDeps = append(serverDeps, orasJob)
	}

	gw, err := gateway.Deploy(ctx, cfg, model, category, namespace, providerOpt)
	if err != nil {
		return fmt.Errorf("deploy gateway: %w", err)
	}

	routerRelease, err := router.Deploy(ctx, cfg, model, modelserver.PodSelector(model), pullSecret, routerDeps, providerOpt)
	if err != nil {
		return fmt.Errorf("deploy router: %w", err)
	}

	server, err := modelserver.Deploy(ctx, cfg, model, category, pullSecret, serverDeps, providerOpt)
	if err != nil {
		return fmt.Errorf("deploy model server: %w", err)
	}

	if category == config.CategoryEmbedder {
		if err := embeddingservice.Deploy(ctx, []pulumi.Resource{server}, model, category, providerOpt); err != nil {
			return fmt.Errorf("deploy embedding service: %w", err)
		}
	}

	// Embedders are routed through the Gateway too: the llm-d v1.0 router
	// parses /v1/embeddings.
	if err := httproute.Deploy(ctx, []pulumi.Resource{gw, routerRelease, server}, model, providerOpt); err != nil {
		return fmt.Errorf("deploy HTTP route: %w", err)
	}

	return nil
}
