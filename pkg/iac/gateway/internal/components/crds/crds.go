package crds

import (
	"context"
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(
	ctx *pulumi.Context,
	cfg config.Config,
	provider *kubernetes.Provider,
) ([]pulumi.Resource, error) {
	var resources []pulumi.Resource

	opts := []pulumi.ResourceOption{
		pulumi.Provider(provider),
		forceOwnershipTransform(),
	}

	// Azure AGC ships and owns the Gateway API CRDs, so its platform default is
	// false. Other platforms install them unless explicitly configured not to.
	if cfg.CRDs.InstallGatewayAPI {
		gatewayAPI, err := kustomize.NewDirectory(
			ctx,
			"gateway-api-crds",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(cfg.CRDs.GatewayAPIPath),
			},
			opts...,
		)
		if err != nil {
			return nil, fmt.Errorf("deploy Gateway API CRDs: %w", err)
		}

		resources = append(resources, gatewayAPI)
	}

	gie, err := kustomize.NewDirectory(
		ctx,
		"gie-crds",
		kustomize.DirectoryArgs{
			Directory: pulumi.String(cfg.CRDs.GIEPath),
		},
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("deploy Gateway API inference extension CRDs: %w", err)
	}

	resources = append(resources, gie)

	return resources, nil
}

// forceOwnershipTransform allows Pulumi server-side apply to take ownership of
// fields already claimed by another Kubernetes field manager. For example,
// GKE's kube-addon-manager may own bundle-version annotations and spec.versions
// on Gateway API CRDs. patchForce is equivalent to kubectl apply
// --force-conflicts for these resource types.
func forceOwnershipTransform() pulumi.ResourceOption {
	return pulumi.Transforms([]pulumi.ResourceTransform{
		func(
			_ context.Context,
			args *pulumi.ResourceTransformArgs,
		) *pulumi.ResourceTransformResult {
			if !requiresForceOwnership(args.Type) {
				return unchanged(args)
			}

			addPatchForceAnnotation(args.Props)

			return unchanged(args)
		},
	})
}

func requiresForceOwnership(resourceType string) bool {
	switch resourceType {
	case "kubernetes:apiextensions.k8s.io/v1:CustomResourceDefinition",
		"kubernetes:admissionregistration.k8s.io/v1:ValidatingAdmissionPolicy",
		"kubernetes:admissionregistration.k8s.io/v1:ValidatingAdmissionPolicyBinding":
		return true

	default:
		return false
	}
}

func addPatchForceAnnotation(props pulumi.Map) {
	metaRaw, ok := props["metadata"]
	if !ok {
		return
	}

	metadata, ok := metaRaw.(pulumi.Map)
	if !ok {
		return
	}

	annotations := pulumi.Map{}

	if raw, ok := metadata["annotations"]; ok {
		if existing, ok := raw.(pulumi.Map); ok {
			for key, value := range existing {
				annotations[key] = value
			}
		}
	}

	annotations["pulumi.com/patchForce"] = pulumi.String("true")
	metadata["annotations"] = annotations
}

func unchanged(
	args *pulumi.ResourceTransformArgs,
) *pulumi.ResourceTransformResult {
	return &pulumi.ResourceTransformResult{
		Props: args.Props,
		Opts:  args.Opts,
	}
}
