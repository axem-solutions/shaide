package models

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/embeddingservice"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/gaie"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/httproute"
	llmdinfra "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/llmd-infra"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/modelservice"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	generativeCategory = "generative"
	embedderCategory   = "embedder"
)

// Deploy deploys every enabled model. Generative and embedder models share the
// same resource chain; only their category label and final endpoint differ.
func Deploy(ctx *pulumi.Context, provider pulumi.ProviderResource, cfg config.Values) error {
	groups := []struct {
		category string
		models   []config.Model
	}{
		{category: generativeCategory, models: cfg.Models.Generative},
		{category: embedderCategory, models: cfg.Models.Embedder},
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

	if cfg.Harbor.TokenSet {
		if _, err := platform.CreateHarborPullSecret(
			ctx,
			cfg,
			model,
			namespace,
			providerOpt,
		); err != nil {
			return fmt.Errorf("create Harbor pull secret: %w", err)
		}
	}

	modelPVC, orasJob, err := platform.PrepareModelStorage(
		ctx,
		cfg,
		model,
		namespace,
		providerOpt,
	)
	if err != nil {
		return fmt.Errorf("prepare model storage: %w", err)
	}

	infra, err := llmdinfra.Deploy(ctx, cfg, model, category, providerOpt)
	if err != nil {
		return fmt.Errorf("deploy llm-d infrastructure: %w", err)
	}

	inferencePool, err := gaie.Deploy(ctx, infra, cfg, model, providerOpt)
	if err != nil {
		return fmt.Errorf("deploy inference extension: %w", err)
	}

	modelService, err := modelservice.Deploy(
		ctx,
		inferencePool,
		cfg,
		model,
		category,
		modelPVC,
		orasJob,
		providerOpt,
	)
	if err != nil {
		return fmt.Errorf("deploy model service: %w", err)
	}

	if category == embedderCategory {
		if err := embeddingservice.Deploy(ctx, modelService, model, category, providerOpt); err != nil {
			return fmt.Errorf("deploy embedding service: %w", err)
		}

		return nil
	}

	if err := httproute.Deploy(ctx, modelService, model, providerOpt); err != nil {
		return fmt.Errorf("deploy HTTP route: %w", err)
	}

	return nil
}
