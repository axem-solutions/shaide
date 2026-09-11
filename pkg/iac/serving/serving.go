package serving

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/models"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployAppServing(ctx *pulumi.Context, dir string) error {
	return deployAppServing(ctx, config.New(dir, stackpkg.Options{}, config.Sources{}))
}

func deployAppServing(ctx *pulumi.Context, configuration config.Config) error {
	cfg, err := configuration.Load(ctx)
	if err != nil {
		return fmt.Errorf("load app-serving config: %w", err)
	}

	provider, err := iackube.NewProvider(ctx, cfg.Kubernetes, iackube.ProviderOptions{
		Name: "app-serving-k8s",
	})
	if err != nil {
		return fmt.Errorf("create Kubernetes provider: %w", err)
	}

	if err := models.Deploy(ctx, provider, cfg); err != nil {
		return fmt.Errorf("deploy models: %w", err)
	}

	return nil
}
