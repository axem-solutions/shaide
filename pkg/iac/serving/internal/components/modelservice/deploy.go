package modelservice

import (
	"fmt"
	"strings"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(
	ctx *pulumi.Context,
	gaie pulumi.Resource,
	model appConfig.Model,
	modelPVC *corev1.PersistentVolumeClaim,
	orasJob pulumi.Resource,
	opts ...pulumi.ResourceOption,
) (*helmv4.Chart, error) {
	// Sub-provider with server-side apply disabled; inherits kubeconfig from the
	// stack-level provider when kubeconfig is explicitly configured.
	helmProviderArgs := &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(false),
	}
	if model.Kubeconfig != "" {
		helmProviderArgs.Kubeconfig = pulumi.StringPtr(model.Kubeconfig)
	}

	helmProvider, err := kubernetes.NewProvider(
		ctx,
		"modelservice-provider-"+model.ReleasePostFix(),
		helmProviderArgs,
	)
	if err != nil {
		return nil, err
	}

	podConfig := pulumi.Map{
		"nodeSelector": model.NodeSelectorMap(),
	}

	if t := model.GPUToleration; t != nil {
		podConfig["tolerations"] = pulumi.Array{
			pulumi.Map{
				"key":      pulumi.String(t.Key),
				"operator": pulumi.String(t.Operator),
				"value":    pulumi.String(t.Value),
				"effect":   pulumi.String(t.Effect),
			},
		}
	}

	decodeValues := pulumi.Map{
		"extraConfig": podConfig,
	}

	prefillValues := pulumi.Map{
		"extraConfig": podConfig,
	}

	modelServiceValues := pulumi.Map{
		"decode":  decodeValues,
		"prefill": prefillValues,
	}

	chartDeps := []pulumi.Resource{gaie}

	modelArtifacts := pulumi.Map{
		"labels": model.MetaLabels(),
	}
	if model.ModelSource != nil {
		// Override modelArtifacts.uri to load weights from the pre-populated PVC.
		pvcName := model.Slug + "-model"
		uri := fmt.Sprintf("pvc+hf://%s/%s", pvcName, model.ModelSource.ModelUri)

		modelArtifacts["uri"] = pulumi.String(uri)

		chartDeps = append(chartDeps, modelPVC)

		// The ORAS pull Job must complete before the pod starts; otherwise the pod
		// finds an empty PVC under HF_HUB_OFFLINE=1.
		if orasJob != nil {
			chartDeps = append(chartDeps, orasJob)
		}
	}
	modelServiceValues["modelArtifacts"] = modelArtifacts

	modelServiceReleaseName := model.ModelServiceReleaseName()

	chartOpts := append(
		[]pulumi.ResourceOption{
			pulumi.DependsOn(chartDeps),
			pulumi.Provider(helmProvider),
		},
		opts...,
	)

	// vLLM/modelservice pods can spend a long time becoming Ready while pulling
	// CUDA images and initializing model weights on fresh nodes.
	chartOpts = append(chartOpts, pulumi.Timeouts(&pulumi.CustomTimeouts{
		Create: "30m",
		Update: "30m",
		Delete: "10m",
	}))

	// Single-replica GPU model pods cannot safely use the Kubernetes default
	// RollingUpdate strategy, because the old pod holds the GPU while the new pod
	// waits in Pending. This transformation forces decode/prefill deployments to
	// terminate the old pod before creating the replacement.
	chartOpts = append(chartOpts, pulumi.Transformations([]pulumi.ResourceTransformation{
		singleReplicaGPURolloutStrategy,
	}))

	modelService, err := helmv4.NewChart(
		ctx,
		modelServiceReleaseName,
		&helmv4.ChartArgs{
			Namespace: pulumi.String(model.Namespace),
			Name:      pulumi.String(modelServiceReleaseName),
			Chart:     pulumi.String("llm-d-modelservice"),
			Version:   pulumi.String("v0.4.12"),
			RepositoryOpts: &helmv4.RepositoryOptsArgs{
				Repo: pulumi.String("https://llm-d-incubation.github.io/llm-d-modelservice/"),
			},
			ValueYamlFiles: pulumi.AssetOrArchiveArray{
				pulumi.NewFileAsset(model.MsValuesPath),
			},
			Values: modelServiceValues,
		},
		chartOpts...,
	)
	if err != nil {
		return nil, err
	}

	return modelService, nil
}

func singleReplicaGPURolloutStrategy(args *pulumi.ResourceTransformationArgs) *pulumi.ResourceTransformationResult {
	if args.Type != "kubernetes:apps/v1:Deployment" {
		return nil
	}

	props, ok := args.Props.(kubernetes.UntypedArgs)
	if !ok {
		return nil
	}

	metadataName := ""
	if metadata, ok := props["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			metadataName = name
		}
	}

	resourceName := args.Name

	target := strings.Contains(resourceName, "modelservice-decode") ||
		strings.Contains(resourceName, "modelservice-prefill") ||
		strings.HasSuffix(resourceName, "-decode") ||
		strings.HasSuffix(resourceName, "-prefill") ||
		strings.Contains(metadataName, "modelservice-decode") ||
		strings.Contains(metadataName, "modelservice-prefill") ||
		strings.HasSuffix(metadataName, "-decode") ||
		strings.HasSuffix(metadataName, "-prefill")

	if !target {
		return nil
	}

	spec, ok := props["spec"].(map[string]interface{})
	if !ok {
		spec = map[string]interface{}{}
		props["spec"] = spec
	}

	// Recreate is the correct rollout strategy for single-replica, single-GPU
	// vLLM deployments. It prevents a new pod from being created while the old
	// pod still holds the only available GPU.
	spec["strategy"] = map[string]interface{}{
		"type": "Recreate",
	}

	return &pulumi.ResourceTransformationResult{
		Props: props,
		Opts:  args.Opts,
	}
}
