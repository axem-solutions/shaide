package metallb

import (
	apiextensions "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// createIPPool creates a MetalLB IPAddressPool and L2Advertisement in the
// metallb-system namespace. Both resources are applied after the Helm release
// so that the MetalLB CRDs are registered before the CRs are created.
//
// nodeHostname pins L2 announcements to a single node's speaker via nodeSelectors,
// preventing non-deterministic speaker elections across the cluster.
func createIPPool(
	ctx *pulumi.Context,
	ipPool string,
	nodeHostname string,
	l2Interface string,
	provider pulumi.ProviderResource,
	dependsOn []pulumi.Resource,
) error {
	opts := []pulumi.ResourceOption{
		pulumi.Provider(provider),
		pulumi.DependsOn(dependsOn),
	}

	pool, err := apiextensions.NewCustomResource(ctx, "metallb-ippool", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("metallb.io/v1beta1"),
		Kind:       pulumi.String("IPAddressPool"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("default-pool"),
			Namespace: pulumi.String("metallb-system"),
		},
		OtherFields: map[string]interface{}{
			"spec": map[string]interface{}{
				"addresses": []interface{}{ipPool},
			},
		},
	}, opts...)
	if err != nil {
		return err
	}

	l2AdvertSpec := map[string]interface{}{
		"ipAddressPools": []interface{}{"default-pool"},
		"nodeSelectors": []interface{}{
			map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"kubernetes.io/hostname": nodeHostname,
				},
			},
		},
	}
	if l2Interface != "" {
		l2AdvertSpec["interfaces"] = []interface{}{l2Interface}
	}

	_, err = apiextensions.NewCustomResource(ctx, "metallb-l2advert", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("metallb.io/v1beta1"),
		Kind:       pulumi.String("L2Advertisement"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("default-l2advert"),
			Namespace: pulumi.String("metallb-system"),
		},
		OtherFields: map[string]interface{}{
			"spec": l2AdvertSpec,
		},
	}, append(opts, pulumi.DependsOn([]pulumi.Resource{pool}))...)

	return err
}
