package knowledgecenter

// NetworkPolicies deployed with the Knowledge Center.
//
// KC adds document content to RustFS and Qdrant, neither of which
// authenticates every client, so their ingress is narrowed to the pods that
// use them. Kubernetes allow rules are additive: these policies only take
// effect for the pods they select, and any broader policy selecting the same
// pods still admits its own traffic. app-shaide has no such policy today.
//
// The policies are deployed only with KC, so a KC-less installation keeps its
// network behavior.
//
//	knowledge-center  ingress: shaide-server pods, kc-mcp pods in app-mcp (8080)
//	                  egress:  qdrant (6334), rustfs (9000), shaide-server (8080), DNS
//	qdrant            ingress: shaide-server (workspace code search) and KC (6333, 6334)
//	rustfs            ingress: KC, and the RustFS client namespaces (default
//	                  monitoring, for Loki) (9000)
//
// RustFS notification delivery to shaide-server is egress from RustFS and
// ingress to shaide-server; neither is restricted here. Ports are pod ports:
// policies apply after Service translation, so shaide-server is 8080, not 80.

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	nameLabel        = "app.kubernetes.io/name"
	namespaceLabel   = "kubernetes.io/metadata.name"
	shaideServer     = "shaide-server"
	shaideServerPort = 8080
	qdrantRESTPort   = 6333
	qdrantGRPCPort   = 6334
	rustfsPort       = 9000
)

// DeployNetworkPolicies creates the policies listed above.
func DeployNetworkPolicies(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Values) error {
	for _, policy := range []struct {
		name string
		args *networkingv1.NetworkPolicySpecArgs
	}{
		{"knowledge-center", knowledgeCenterPolicy(cfg)},
		{cfg.Services.Qdrant + "-clients", qdrantPolicy(cfg)},
		{cfg.Services.Rustfs + "-clients", rustfsPolicy(cfg)},
	} {
		_, err := networkingv1.NewNetworkPolicy(ctx, policy.name+"-netpol", &networkingv1.NetworkPolicyArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(policy.name),
				Namespace: pulumi.String(cfg.Namespace),
				Labels:    deps.MetaLabels(policy.name, "network-policy"),
			},
			Spec: policy.args,
		}, deps.ProviderOpt, deps.NsOpt)
		if err != nil {
			return err
		}
	}
	return nil
}

func knowledgeCenterPolicy(cfg appconfig.Values) *networkingv1.NetworkPolicySpecArgs {
	return &networkingv1.NetworkPolicySpecArgs{
		PodSelector: podsNamed(name),
		PolicyTypes: pulumi.StringArray{pulumi.String("Ingress"), pulumi.String("Egress")},
		Ingress: networkingv1.NetworkPolicyIngressRuleArray{
			&networkingv1.NetworkPolicyIngressRuleArgs{
				From: networkingv1.NetworkPolicyPeerArray{
					&networkingv1.NetworkPolicyPeerArgs{PodSelector: podsNamed(shaideServer)},
					&networkingv1.NetworkPolicyPeerArgs{
						NamespaceSelector: namespaceNamed(knowledgecenter.MCPNamespace),
						PodSelector:       podsNamed(knowledgecenter.MCPServiceName),
					},
				},
				Ports: tcp(knowledgecenter.Port),
			},
		},
		Egress: networkingv1.NetworkPolicyEgressRuleArray{
			&networkingv1.NetworkPolicyEgressRuleArgs{
				To:    networkingv1.NetworkPolicyPeerArray{&networkingv1.NetworkPolicyPeerArgs{PodSelector: podsNamed(cfg.Services.Qdrant)}},
				Ports: tcp(qdrantGRPCPort),
			},
			&networkingv1.NetworkPolicyEgressRuleArgs{
				To:    networkingv1.NetworkPolicyPeerArray{&networkingv1.NetworkPolicyPeerArgs{PodSelector: podsNamed(cfg.Services.Rustfs)}},
				Ports: tcp(rustfsPort),
			},
			&networkingv1.NetworkPolicyEgressRuleArgs{
				To:    networkingv1.NetworkPolicyPeerArray{&networkingv1.NetworkPolicyPeerArgs{PodSelector: podsNamed(shaideServer)}},
				Ports: tcp(shaideServerPort),
			},
			dnsEgress(),
		},
	}
}

func qdrantPolicy(cfg appconfig.Values) *networkingv1.NetworkPolicySpecArgs {
	return &networkingv1.NetworkPolicySpecArgs{
		PodSelector: podsNamed(cfg.Services.Qdrant),
		PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
		Ingress: networkingv1.NetworkPolicyIngressRuleArray{
			&networkingv1.NetworkPolicyIngressRuleArgs{
				From: networkingv1.NetworkPolicyPeerArray{
					&networkingv1.NetworkPolicyPeerArgs{PodSelector: podsNamed(shaideServer)},
					&networkingv1.NetworkPolicyPeerArgs{PodSelector: podsNamed(name)},
				},
				Ports: tcp(qdrantRESTPort, qdrantGRPCPort),
			},
		},
	}
}

func rustfsPolicy(cfg appconfig.Values) *networkingv1.NetworkPolicySpecArgs {
	from := networkingv1.NetworkPolicyPeerArray{
		&networkingv1.NetworkPolicyPeerArgs{PodSelector: podsNamed(name)},
	}
	for _, namespace := range cfg.RustfsClientNamespaces {
		from = append(from, &networkingv1.NetworkPolicyPeerArgs{NamespaceSelector: namespaceNamed(namespace)})
	}

	return &networkingv1.NetworkPolicySpecArgs{
		PodSelector: podsNamed(cfg.Services.Rustfs),
		PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
		Ingress: networkingv1.NetworkPolicyIngressRuleArray{
			&networkingv1.NetworkPolicyIngressRuleArgs{
				From:  from,
				Ports: tcp(rustfsPort),
			},
		},
	}
}

func podsNamed(value string) *metav1.LabelSelectorArgs {
	return &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{nameLabel: pulumi.String(value)}}
}

func namespaceNamed(value string) *metav1.LabelSelectorArgs {
	return &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{namespaceLabel: pulumi.String(value)}}
}

func tcp(ports ...int) networkingv1.NetworkPolicyPortArray {
	out := networkingv1.NetworkPolicyPortArray{}
	for _, port := range ports {
		out = append(out, &networkingv1.NetworkPolicyPortArgs{
			Protocol: pulumi.String("TCP"),
			Port:     pulumi.Int(port),
		})
	}
	return out
}

// dnsEgress allows DNS to any destination. NodeLocal DNSCache listens on a
// link-local address outside any namespace, so the rule names no peer.
func dnsEgress() *networkingv1.NetworkPolicyEgressRuleArgs {
	return &networkingv1.NetworkPolicyEgressRuleArgs{
		Ports: networkingv1.NetworkPolicyPortArray{
			&networkingv1.NetworkPolicyPortArgs{Protocol: pulumi.String("UDP"), Port: pulumi.Int(53)},
			&networkingv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(53)},
		},
	}
}
