package modelserver

import (
	"fmt"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	yamlv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"k8s.io/apimachinery/pkg/runtime"
)

// Deploy creates the model server Deployment, and the runtime metadata
// ConfigMap when the model enables it. The Deployment is built from the
// Kubernetes API types and handed to Pulumi as a manifest, so it is the same
// object the unit tests check.
func Deploy(
	ctx *pulumi.Context,
	cfg appConfig.Values,
	model appConfig.Model,
	category string,
	pullSecret string,
	dependsOn []pulumi.Resource,
	opts ...pulumi.ResourceOption,
) (pulumi.Resource, error) {
	in := Input{
		Model:          model,
		Category:       category,
		Image:          cfg.ModelServerImage,
		SimulatorImage: cfg.SimulatorImage,
		Toleration:     cfg.Toleration,
		PullSecret:     pullSecret,
	}

	deps := append([]pulumi.Resource(nil), dependsOn...)

	if model.Definition.Server.RuntimeMetadata {
		in.MetadataConfigMap = MetadataConfigMapName(model.ModelServerName())

		configMap, err := corev1.NewConfigMap(ctx, model.ModelServerName()+"-runtime-metadata", &corev1.ConfigMapArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(in.MetadataConfigMap),
				Namespace: pulumi.String(model.Namespace),
			},
			Immutable: pulumi.Bool(true),
			Data:      pulumi.ToStringMap(MetadataModule()),
		}, append([]pulumi.ResourceOption{pulumi.DependsOn(dependsOn)}, opts...)...)
		if err != nil {
			return nil, fmt.Errorf("create runtime metadata ConfigMap: %w", err)
		}
		deps = append(deps, configMap)
	}

	deployment, err := Deployment(in)
	if err != nil {
		return nil, err
	}

	manifest, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deployment)
	if err != nil {
		return nil, fmt.Errorf("convert Deployment: %w", err)
	}
	delete(manifest, "status")
	if metadata, ok := manifest["metadata"].(map[string]any); ok {
		delete(metadata, "creationTimestamp")
	}
	if template, ok := manifest["spec"].(map[string]any)["template"].(map[string]any); ok {
		if metadata, ok := template["metadata"].(map[string]any); ok {
			delete(metadata, "creationTimestamp")
		}
	}

	group, err := yamlv2.NewConfigGroup(ctx, model.ModelServerName(), &yamlv2.ConfigGroupArgs{
		Objs: pulumi.Array{pulumi.ToMap(manifest)},
	}, append([]pulumi.ResourceOption{pulumi.DependsOn(deps)}, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("create model server Deployment: %w", err)
	}

	return group, nil
}
