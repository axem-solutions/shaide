package stacks

import (
	"context"
	"slices"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func gatewayClass(name, controller string, accepted bool) *unstructured.Unstructured {
	status := "False"
	if accepted {
		status = "True"
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "GatewayClass",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"controllerName": controller},
		"status": map[string]any{"conditions": []any{
			map[string]any{"type": "Accepted", "status": status},
		}},
	}}
}

func sharedGateway(class, issuer string) *unstructured.Unstructured {
	metadata := map[string]any{"name": gateway.SharedGatewayName, "namespace": gateway.SharedGatewayNamespace}
	if issuer != "" {
		metadata["annotations"] = map[string]any{clusterIssuerAnnotation: issuer}
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata":   metadata,
		"spec":       map[string]any{"gatewayClassName": class},
	}}
}

func gatewayAPIClient(t *testing.T, objects map[schema.GroupVersionResource][]*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	t.Helper()

	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			gatewayGVR:       "GatewayList",
			gatewayClassGVR:  "GatewayClassList",
			clusterIssuerGVR: "ClusterIssuerList",
		},
	)
	for gvr, items := range objects {
		for _, item := range items {
			if _, err := client.Resource(gvr).Namespace(item.GetNamespace()).
				Create(context.Background(), item, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return client
}

// dev-polandcentral: AGC was once installed and its class is still accepted,
// but the shared Gateway runs on Istio. The class in use must be the answer
// offered, or accepting the default moves the Gateway to AGC.
func TestGatewayClassChoicePrefersTheClassInUse(t *testing.T) {
	options, current := gatewayClassChoice(
		[]string{"azure-alb-external", "istio"},
		"istio",
		gateway.DefaultGatewayClassName("azure"),
	)

	if want := []string{"azure-alb-external", "istio"}; !slices.Equal(options, want) {
		t.Errorf("options = %v, want %v", options, want)
	}
	if current != "istio" {
		t.Errorf("current = %q, want the class in use %q", current, "istio")
	}
}

func TestGatewayClassChoiceFallsBackToThePlatformDefault(t *testing.T) {
	_, current := gatewayClassChoice([]string{"azure-alb-external"}, "", "azure-alb-external")

	if current != "azure-alb-external" {
		t.Errorf("current = %q, want the platform default", current)
	}
}

// A fresh cluster has no GatewayClass until the gateway stack installs Istio,
// so Istio is the one option and the question is not worth asking.
func TestGatewayClassChoiceOnAFreshCluster(t *testing.T) {
	options, current := gatewayClassChoice(nil, "", "azure-alb-external")

	if want := []string{"istio"}; !slices.Equal(options, want) {
		t.Errorf("options = %v, want %v", options, want)
	}
	if current != "istio" {
		t.Errorf("current = %q, want %q", current, "istio")
	}
}

// Only classes a controller accepted can serve the Gateway, and Istio's class
// for unmanaged gateways cannot serve the shared one.
func TestAcceptedGatewayClasses(t *testing.T) {
	dyn := gatewayAPIClient(t, map[schema.GroupVersionResource][]*unstructured.Unstructured{
		gatewayClassGVR: {
			gatewayClass("azure-alb-external", "alb.networking.azure.io/alb-controller", true),
			gatewayClass("istio", "istio.io/gateway-controller", true),
			gatewayClass("istio-remote", unmanagedIstioController, true),
			gatewayClass("broken", "example.com/controller", false),
		},
	})

	got, err := acceptedGatewayClasses(context.Background(), dyn)
	if err != nil {
		t.Fatalf("acceptedGatewayClasses() error = %v", err)
	}
	slices.Sort(got)

	if want := []string{"azure-alb-external", "istio"}; !slices.Equal(got, want) {
		t.Errorf("accepted = %v, want %v", got, want)
	}
}

func TestReadSharedGateway(t *testing.T) {
	dyn := gatewayAPIClient(t, map[schema.GroupVersionResource][]*unstructured.Unstructured{
		gatewayGVR: {sharedGateway("istio", "letsencrypt")},
	})

	got, err := readSharedGateway(context.Background(), dyn)
	if err != nil {
		t.Fatalf("readSharedGateway() error = %v", err)
	}
	if got.class != "istio" || got.issuer != "letsencrypt" {
		t.Errorf("live = %+v, want class istio and issuer letsencrypt", got)
	}
}

// A fresh cluster has no Gateway yet; that is not an error.
func TestReadSharedGatewayWhenAbsent(t *testing.T) {
	got, err := readSharedGateway(context.Background(), gatewayAPIClient(t, nil))
	if err != nil {
		t.Fatalf("readSharedGateway() error = %v", err)
	}
	if got != (liveGateway{}) {
		t.Errorf("live = %+v, want nothing", got)
	}
}

// A new Gateway starts on plain HTTP; an update keeps the issuer it has.
func TestIssuerChoice(t *testing.T) {
	issuers := []string{"letsencrypt", "letsencrypt-staging"}

	options, current := issuerChoice(issuers, "")
	if want := []string{httpOnlyIssuerLabel, "letsencrypt", "letsencrypt-staging"}; !slices.Equal(options, want) {
		t.Errorf("options = %v, want %v", options, want)
	}
	if current != httpOnlyIssuerLabel {
		t.Errorf("current = %q, want %q for a new Gateway", current, httpOnlyIssuerLabel)
	}

	if _, current := issuerChoice(issuers, "letsencrypt"); current != "letsencrypt" {
		t.Errorf("current = %q, want the issuer in use", current)
	}

	// An issuer that no longer exists cannot be kept.
	if _, current := issuerChoice(issuers, "removed"); current != httpOnlyIssuerLabel {
		t.Errorf("current = %q, want %q for a missing issuer", current, httpOnlyIssuerLabel)
	}
}

// Keeping the name of the ApplicationLoadBalancer in the Gateway's namespace
// keeps its frontend, and with it the Gateway's address.
func TestDiscoverALBKeepsTheExistingName(t *testing.T) {
	got := discoverALB(context.Background(),
		albClient(applicationLoadBalancer("alb-westeurope", subnetA)),
		fake.NewSimpleClientset(aksNode()),
		discardf,
	)

	if got.ALBName != "alb-westeurope" {
		t.Errorf("ALBName = %q, want %q", got.ALBName, "alb-westeurope")
	}
}
