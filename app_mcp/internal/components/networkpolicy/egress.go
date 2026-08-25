// Egress NetworkPolicy per datasource.
//
// DeployEgress creates the mcp-<name>-egress NetworkPolicy for a single datasource.
// Skipped silently when ds.EgressCIDR is empty.
package networkpolicy

import (
	appconfig "app_mcp/internal/config"
	"fmt"

	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const defaultEgressPort = 443

// DeployEgress creates the mcp-<name>-egress NetworkPolicy for a single datasource.
// Skipped silently when ds.EgressCIDR is empty.
//
// When ds.EgressHost is set, it is stored as the annotation
// mcp.shaide/datasource-host on the NetworkPolicy for operator reference.
// Kubernetes NetworkPolicy ipBlock only accepts CIDR — hostnames cannot be used
// as policy selectors and are stored for documentation purposes only.
func DeployEgress(ctx *pulumi.Context, ds appconfig.Datasource, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	if ds.EgressCIDR == "" {
		return nil
	}

	name := fmt.Sprintf("mcp-%s-egress", ds.Name)
	egressPort := ds.EgressPort
	if egressPort == 0 {
		egressPort = defaultEgressPort
	}

	annotations := pulumi.StringMap{}
	if ds.EgressHost != "" {
		annotations["mcp.shaide/datasource-host"] = pulumi.String(ds.EgressHost)
	}

	_, err := networkingv1.NewNetworkPolicy(ctx, name, &networkingv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:        pulumi.String(name),
			Namespace:   pulumi.String(cfg.Namespace),
			Annotations: annotations,
		},
		Spec: &networkingv1.NetworkPolicySpecArgs{
			PodSelector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app.kubernetes.io/name": pulumi.String("mcp-" + ds.Name),
				},
			},
			PolicyTypes: pulumi.StringArray{pulumi.String("Egress")},
			Egress: networkingv1.NetworkPolicyEgressRuleArray{
				// Datasource traffic: TCP to the specific IP and port.
				&networkingv1.NetworkPolicyEgressRuleArgs{
					To: networkingv1.NetworkPolicyPeerArray{
						&networkingv1.NetworkPolicyPeerArgs{
							IpBlock: &networkingv1.IPBlockArgs{
								Cidr: pulumi.String(ds.EgressCIDR),
							},
						},
					},
					Ports: networkingv1.NetworkPolicyPortArray{
						&networkingv1.NetworkPolicyPortArgs{
							Protocol: pulumi.String("TCP"),
							Port:     pulumi.Int(egressPort),
						},
					},
				},
				// DNS: UDP and TCP port 53, unrestricted destination.
				// GKE NodeLocal DNSCache uses a node-local link-local address outside any
				// namespace — omitting `to` is the correct pattern for GKE clusters.
				&networkingv1.NetworkPolicyEgressRuleArgs{
					Ports: networkingv1.NetworkPolicyPortArray{
						&networkingv1.NetworkPolicyPortArgs{
							Protocol: pulumi.String("UDP"),
							Port:     pulumi.Int(53),
						},
						&networkingv1.NetworkPolicyPortArgs{
							Protocol: pulumi.String("TCP"),
							Port:     pulumi.Int(53),
						},
					},
				},
			},
		},
	}, opts...)
	return err
}
