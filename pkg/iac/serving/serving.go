package serving

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/embeddingservice"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/gaie"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/httproute"
	llmdinfra "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/llmd-infra"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/components/modelservice"
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/platform"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployAppServing(ctx *pulumi.Context, dir string, logf appConfig.Logf) error {
	config, err := appConfig.Load(ctx, dir, logf)
	if err != nil {
		return err
	}

	// --- K8s Provider (shared across all models on this cluster) ---
	// When kubeconfig is set in stack config (e.g. rke2-cluster.yaml), create an
	// explicit provider targeting that cluster. Otherwise the default provider
	// (KUBECONFIG env / ~/.kube/config) is used — identical to cloud stacks.
	k8sProviderArgs := &kubernetes.ProviderArgs{}
	if config.Kubeconfig != "" {
		k8sProviderArgs.Kubeconfig = pulumi.StringPtr(config.Kubeconfig)
	}
	k8sProvider, err := kubernetes.NewProvider(ctx, "app-serving-k8s", k8sProviderArgs)
	if err != nil {
		return err
	}
	providerOpt := pulumi.Provider(k8sProvider)

	// Deploy each model into its own namespace on the same cluster.
	for _, model := range config.Models {
		llmdNamespace, err := corev1.NewNamespace(ctx, model.Namespace, &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:   pulumi.String(model.Namespace),
				Labels: model.MetaLabels(),
			},
		}, providerOpt)
		if err != nil {
			return err
		}

		// Create Harbor pull secret when running against an air-gapped on-prem cluster.
		// Images are served from the internal Harbor registry; pods reference "harbor-creds"
		// in their imagePullSecrets (configured via chart values in the on-prem model folders).
		if config.HarborTokenSet {
			if _, err := platform.CreateHarborPullSecret(ctx, &config, model, llmdNamespace, providerOpt); err != nil {
				return err
			}
		}

		// Create PVC (and optionally an ORAS pull Job) for pre-loaded model weights,
		// when model.ModelSource is set (harborRef pulled via ORAS into the PVC).
		var modelPVC *corev1.PersistentVolumeClaim
		var orasJob pulumi.Resource
		if model.ModelSource != nil {
			modelPVC, orasJob, err = platform.CreateModelStorage(ctx, model, llmdNamespace, providerOpt)
			if err != nil {
				return err
			}
		}

		llmdInfra, err := llmdinfra.Deploy(ctx, model, config.LLMdChartPath, providerOpt)
		if err != nil {
			return err
		}

		gaieRelease, err := gaie.Deploy(ctx, llmdInfra, model, providerOpt)
		if err != nil {
			return err
		}

		modelServiceRelease, err := modelservice.Deploy(ctx, gaieRelease, model, modelPVC, orasJob, providerOpt)
		if err != nil {
			return err
		}

		// Embedder models are reached directly via a ClusterIP Service on port 8200.
		// The Gateway/HTTPRoute path does not support embedding-style requests.
		// Generative models continue to use the HTTPRoute → GAIE InferencePool path.
		if model.IsEmbedder {
			if err := embeddingservice.Deploy(ctx, modelServiceRelease, model, providerOpt); err != nil {
				return err
			}
		} else {
			if err := httproute.Deploy(ctx, modelServiceRelease, model, providerOpt); err != nil {
				return err
			}
		}
	}

	return nil
}
