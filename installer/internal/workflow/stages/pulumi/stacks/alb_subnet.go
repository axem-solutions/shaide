package stacks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// The ALB controller's CRD registers its plural as the singular
// "applicationloadbalancer", not "applicationloadbalancers".
var applicationLoadBalancerGVR = schema.GroupVersionResource{
	Group:    "alb.networking.azure.io",
	Version:  "v1",
	Resource: "applicationloadbalancer",
}

// AKS labels every node with the network it sits in.
const (
	nodeNetworkSubscriptionLabel  = "kubernetes.azure.com/network-subscription"
	nodeNetworkResourceGroupLabel = "kubernetes.azure.com/network-resourcegroup"
	nodeNetworkVNetLabel          = "kubernetes.azure.com/network-name"
)

// discoverALBSubnet finds what the cluster says about the Application Gateway
// for Containers subnet.
//
// The subnet the live ApplicationLoadBalancer is associated with pre-fills the
// prompt: re-entering it on every update is how an empty answer once removed
// the load balancer. Without one, the nodes' VNet gives a placeholder that
// only leaves the subnet name open. The delegated subnet itself cannot be
// found from inside the cluster.
//
// Discovery is best effort: whatever cannot be read is left empty and the
// prompt falls back to asking.
func discoverALBSubnet(
	ctx context.Context,
	dyn dynamic.Interface,
	client kubernetes.Interface,
	logf func(format string, args ...any),
) gateway.Options {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var options gateway.Options

	associations, err := albAssociations(ctx, dyn)
	switch {
	case err != nil:
		logf("could not read ApplicationLoadBalancers (%v); the AGC subnet is not pre-filled", err)
	case len(associations) == 0:
		logf("no ApplicationLoadBalancer with a subnet association on the cluster; the AGC subnet is not pre-filled")
	case len(associations) == 1:
		options.ALBSubnetID = associations[0]
		logf("pre-filling the AGC subnet with the existing association %s", associations[0])
	case len(associations) > 1:
		logf("ApplicationLoadBalancers use several subnets (%s); the AGC subnet is not pre-filled",
			strings.Join(associations, ", "))
	}

	placeholder, err := vnetSubnetPlaceholder(ctx, client)
	if err != nil {
		logf("could not derive the cluster VNet from node labels (%v)", err)
	}
	options.ALBSubnetPlaceholder = placeholder

	return options
}

// albAssociations returns the distinct, non-empty subnet associations of the
// ApplicationLoadBalancers on the cluster. A cluster without the ALB
// controller has no such resource and simply yields none.
func albAssociations(ctx context.Context, dyn dynamic.Interface) ([]string, error) {
	list, err := dyn.Resource(applicationLoadBalancerGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isResourceNotServed(err) {
			return nil, nil
		}
		return nil, err
	}

	seen := map[string]struct{}{}
	for _, item := range list.Items {
		values, _, err := unstructured.NestedStringSlice(item.Object, "spec", "associations")
		if err != nil {
			continue
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				seen[value] = struct{}{}
			}
		}
	}

	associations := make([]string, 0, len(seen))
	for value := range seen {
		associations = append(associations, value)
	}
	sort.Strings(associations)

	return associations, nil
}

// vnetSubnetPlaceholder builds the subnet ID prefix of the VNet the nodes sit
// in, leaving the subnet name to the operator.
func vnetSubnetPlaceholder(ctx context.Context, client kubernetes.Interface) (string, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return "", err
	}
	if len(nodes.Items) == 0 {
		return "", nil
	}

	labels := nodes.Items[0].Labels
	subscription := labels[nodeNetworkSubscriptionLabel]
	resourceGroup := labels[nodeNetworkResourceGroupLabel]
	vnet := labels[nodeNetworkVNetLabel]
	if subscription == "" || resourceGroup == "" || vnet == "" {
		return "", nil
	}

	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/<subnet>",
		subscription, resourceGroup, vnet,
	), nil
}

// isResourceNotServed reports whether the API server does not serve the
// resource at all, which is what a cluster without the ALB CRD answers.
func isResourceNotServed(err error) bool {
	return apierrors.IsNotFound(err) || meta.IsNoMatchError(err)
}
