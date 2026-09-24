package stacks

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	subnetA = "/subscriptions/0000/resourceGroups/rg-dev/providers/Microsoft.Network/virtualNetworks/vnet-dev/subnets/snet-alb-dev"
	subnetB = "/subscriptions/0000/resourceGroups/rg-dev/providers/Microsoft.Network/virtualNetworks/vnet-dev/subnets/snet-alb-other"
)

func applicationLoadBalancer(name string, associations ...string) *unstructured.Unstructured {
	values := make([]any, 0, len(associations))
	for _, association := range associations {
		values = append(values, association)
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "alb.networking.azure.io/v1",
		"kind":       "ApplicationLoadBalancer",
		"metadata":   map[string]any{"name": name, "namespace": "gateway-system"},
		"spec":       map[string]any{"associations": values},
	}}
}

// albClient creates the objects through applicationLoadBalancerGVR. Seeding
// them through the constructor would file them under a plural guessed from the
// kind ("applicationloadbalancers"), which is not the CRD's resource name and
// would let a wrong GVR pass.
func albClient(objects ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{applicationLoadBalancerGVR: "ApplicationLoadBalancerList"},
	)
	for _, object := range objects {
		if _, err := client.Resource(applicationLoadBalancerGVR).Namespace(object.GetNamespace()).
			Create(context.Background(), object, metav1.CreateOptions{}); err != nil {
			panic(err)
		}
	}
	return client
}

func aksNode() *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "aks-node-0",
		Labels: map[string]string{
			nodeNetworkSubscriptionLabel:  "0000",
			nodeNetworkResourceGroupLabel: "rg-dev",
			nodeNetworkVNetLabel:          "vnet-dev",
		},
	}}
}

func discardf(string, ...any) {}

// The existing association pre-fills the prompt, so accepting the default on
// an update keeps the load balancer.
func TestDiscoverALBSubnetPrefillsExistingAssociation(t *testing.T) {
	got := discoverALBSubnet(context.Background(),
		albClient(applicationLoadBalancer("shared-alb", subnetA)),
		fake.NewSimpleClientset(aksNode()),
		discardf,
	)

	if got.ALBSubnetID != subnetA {
		t.Errorf("ALBSubnetID = %q, want %q", got.ALBSubnetID, subnetA)
	}
}

// The state that took westeurope offline: an association of "" is not a
// subnet and must not be offered as one.
func TestDiscoverALBSubnetIgnoresEmptyAssociation(t *testing.T) {
	got := discoverALBSubnet(context.Background(),
		albClient(applicationLoadBalancer("shared-alb", "")),
		fake.NewSimpleClientset(aksNode()),
		discardf,
	)

	if got.ALBSubnetID != "" {
		t.Errorf("ALBSubnetID = %q, want nothing pre-filled", got.ALBSubnetID)
	}
	want := "/subscriptions/0000/resourceGroups/rg-dev/providers/Microsoft.Network/virtualNetworks/vnet-dev/subnets/<subnet>"
	if got.ALBSubnetPlaceholder != want {
		t.Errorf("ALBSubnetPlaceholder = %q, want %q", got.ALBSubnetPlaceholder, want)
	}
}

// With several subnets in use there is no single right answer to pre-fill.
func TestDiscoverALBSubnetAmbiguous(t *testing.T) {
	got := discoverALBSubnet(context.Background(),
		albClient(applicationLoadBalancer("a", subnetA), applicationLoadBalancer("b", subnetB)),
		fake.NewSimpleClientset(aksNode()),
		discardf,
	)

	if got.ALBSubnetID != "" {
		t.Errorf("ALBSubnetID = %q, want nothing pre-filled", got.ALBSubnetID)
	}
}

func TestDiscoverALBSubnetWithoutNodeLabels(t *testing.T) {
	got := discoverALBSubnet(context.Background(),
		albClient(),
		fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "plain"}}),
		discardf,
	)

	if got.ALBSubnetID != "" || got.ALBSubnetPlaceholder != "" {
		t.Errorf("options = %+v, want nothing discovered", got)
	}
}

// The ALB controller's CRD serves the singular "applicationloadbalancer" as its
// resource name (see `kubectl api-resources`). The regular plural returns
// NotFound, which reads as "no ALB on this cluster" and silently disables the
// pre-fill, so the name is pinned here.
func TestApplicationLoadBalancerResourceName(t *testing.T) {
	if applicationLoadBalancerGVR.Resource != "applicationloadbalancer" {
		t.Errorf("resource = %q, want the CRD's served name %q", applicationLoadBalancerGVR.Resource, "applicationloadbalancer")
	}
}
