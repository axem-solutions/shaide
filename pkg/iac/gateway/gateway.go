package gateway

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/components/crds"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/components/istio"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/components/sharedgateway"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployGatewayProvider(ctx *pulumi.Context, projectDir string) error {
	cfg, err := config.Load(ctx, projectDir)
	if err != nil {
		return fmt.Errorf("load gateway provider config: %w", err)
	}

	// Create an explicit Kubernetes provider targeting the configured cluster.
	// When kubeconfig is empty, pulumi-kubernetes falls back to KUBECONFIG and
	// then ~/.kube/config. Keep the historical logical name to avoid replacing
	// the provider and all resources that reference it during this refactor.
	provider, err := iackube.NewProvider(
		ctx,
		cfg.Kubernetes,
		iackube.ProviderOptions{Name: "gateway-provider-k8s"},
	)
	if err != nil {
		return fmt.Errorf("create Kubernetes provider: %w", err)
	}

	crdDeps, err := crds.Deploy(ctx, cfg, provider)
	if err != nil {
		return fmt.Errorf("deploy CRDs: %w", err)
	}

	if cfg.Istio.Enabled {
		if err := istio.Deploy(ctx, cfg, provider); err != nil {
			return fmt.Errorf("deploy Istio: %w", err)
		}
	}

	// The shared Gateway uses the gatewayClassName supplied by stack config.
	if err := sharedgateway.Deploy(ctx, cfg, provider, crdDeps); err != nil {
		return fmt.Errorf("deploy shared gateway: %w", err)
	}

	return nil
}
