package modelservice

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(
	ctx *pulumi.Context,
	gaie pulumi.Resource,
	cfg appConfig.Values,
	model appConfig.Model,
	category string,
	modelPVC *corev1.PersistentVolumeClaim,
	orasJob pulumi.Resource,
	opts ...pulumi.ResourceOption,
) (*helmv4.Chart, error) {
	// The sub-provider targets the same kubeconfig and context as the stack.
	serverSideApply := false
	helmProvider, err := iackube.NewProvider(ctx, cfg.Kubernetes, iackube.ProviderOptions{
		Name:                  "modelservice-provider-" + model.ReleasePostFix(),
		EnableServerSideApply: &serverSideApply,
	})
	if err != nil {
		return nil, err
	}

	podConfig := pulumi.Map{
		"affinity": model.NodeAffinityMap(),
	}

	if t := cfg.Toleration; t != nil {
		podConfig["tolerations"] = pulumi.Array{
			pulumi.Map{
				"key":      pulumi.String(t.Key),
				"operator": pulumi.String(t.Operator),
				"value":    pulumi.String(t.Value),
				"effect":   pulumi.String(t.Effect),
			},
		}
	}

	decodeValues := workloadValues(podConfig)
	prefillValues := workloadValues(podConfig)

	modelServiceValues := values(decodeValues, prefillValues)

	chartDeps := []pulumi.Resource{gaie}

	modelArtifacts := pulumi.Map{
		"labels": model.MetaLabels(category),
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

func workloadValues(podConfig pulumi.Map) pulumi.Map {
	return pulumi.Map{
		"extraConfig": podConfig,
		// A rolling update deadlocks when the old replica holds the only GPU.
		// The llm-d chart exposes this value directly on its Deployment spec.
		"strategy": pulumi.Map{
			"type": pulumi.String("Recreate"),
		},
	}
}

func values(decodeValues, prefillValues pulumi.Map) pulumi.Map {
	return pulumi.Map{
		"decode":  decodeValues,
		"prefill": prefillValues,
		// The legacy routing sidecar image is no longer reliably available from
		// its upstream registry. Disable it temporarily and let the chart expose
		// vLLM directly on routing.servicePort.
		"routing": pulumi.Map{
			"proxy": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
		},
	}
}
