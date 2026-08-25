// allow-shaide-ingress NetworkPolicy.
//
// Namespace-level policy applied once to the mcp namespace.
// Permits ingress to all MCP Server pods exclusively from the app-shaide namespace.
// All other ingress is implicitly denied by the NetworkPolicy whitelist model.
package networkpolicy

import (
	appconfig "app_mcp/internal/config"

	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DeployIngress creates the allow-shaide-ingress NetworkPolicy in the mcp namespace.
// It selects all pods in the namespace (podSelector: {}) and allows ingress only from
// the app-shaide namespace — identified by the kubernetes.io/metadata.name label which
// Kubernetes automatically applies to every namespace.
func DeployIngress(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	_, err := networkingv1.NewNetworkPolicy(ctx, "allow-shaide-ingress", &networkingv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("allow-shaide-ingress"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Spec: &networkingv1.NetworkPolicySpecArgs{
			// Empty podSelector matches all pods in the mcp namespace.
			PodSelector: &metav1.LabelSelectorArgs{},
			PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
			Ingress: networkingv1.NetworkPolicyIngressRuleArray{
				&networkingv1.NetworkPolicyIngressRuleArgs{
					From: networkingv1.NetworkPolicyPeerArray{
						&networkingv1.NetworkPolicyPeerArgs{
							NamespaceSelector: &metav1.LabelSelectorArgs{
								MatchLabels: pulumi.StringMap{
									"kubernetes.io/metadata.name": pulumi.String(cfg.ShaideNamespace),
								},
							},
						},
					},
				},
			},
		},
	}, opts...)
	return err
}
