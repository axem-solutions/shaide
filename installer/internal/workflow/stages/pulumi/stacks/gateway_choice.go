package stacks

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	gatewayGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}
	gatewayClassGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gatewayclasses",
	}
	clusterIssuerGVR = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "clusterissuers",
	}
)

const (
	// Istio's class for gateways it does not deploy itself. It cannot serve
	// the shared Gateway, so it is not offered.
	unmanagedIstioController = "istio.io/unmanaged-gateway"

	clusterIssuerAnnotation = "cert-manager.io/cluster-issuer"

	gatewayLookupTimeout = 15 * time.Second

	httpOnlyIssuerLabel = "(none, HTTP only)"
)

// liveGateway is what the shared Gateway already on the cluster uses. An
// update pre-selects it, so accepting the defaults keeps the Gateway as is.
type liveGateway struct {
	class  string
	issuer string
}

func readSharedGateway(ctx context.Context, dyn dynamic.Interface) (liveGateway, error) {
	ctx, cancel := context.WithTimeout(ctx, gatewayLookupTimeout)
	defer cancel()

	object, err := dyn.Resource(gatewayGVR).Namespace(gateway.SharedGatewayNamespace).
		Get(ctx, gateway.SharedGatewayName, metav1.GetOptions{})
	if err != nil {
		if isResourceNotServed(err) {
			return liveGateway{}, nil
		}
		return liveGateway{}, err
	}

	class, _, _ := unstructured.NestedString(object.Object, "spec", "gatewayClassName")

	return liveGateway{
		class:  class,
		issuer: object.GetAnnotations()[clusterIssuerAnnotation],
	}, nil
}

// selectGatewayClass picks the class of the shared Gateway from those the
// cluster accepts. It asks only when there is a choice to make.
func selectGatewayClass(
	ctx context.Context,
	rt *core.Runtime,
	dyn dynamic.Interface,
	platform cluster.Provider,
	inUse string,
) (string, error) {
	accepted, err := acceptedGatewayClasses(ctx, dyn)
	if err != nil {
		rt.Detailf("could not list GatewayClasses (%v); offering the platform default and Istio", err)
		accepted = []string{gateway.DefaultGatewayClassName(platform)}
	}

	options, current := gatewayClassChoice(accepted, inUse, gateway.DefaultGatewayClassName(platform))
	if len(options) == 1 {
		rt.Detailf("using GatewayClass %q, the only one available", options[0])
		return options[0], nil
	}

	return rt.Reporter.Select("Gateway class", current, options)
}

// acceptedGatewayClasses lists the classes whose controller accepted them. A
// cluster without the Gateway API CRDs has none yet.
func acceptedGatewayClasses(ctx context.Context, dyn dynamic.Interface) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, gatewayLookupTimeout)
	defer cancel()

	list, err := dyn.Resource(gatewayClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isResourceNotServed(err) {
			return nil, nil
		}
		return nil, err
	}

	var accepted []string
	for _, item := range list.Items {
		controller, _, _ := unstructured.NestedString(item.Object, "spec", "controllerName")
		if controller == unmanagedIstioController || !hasTrueCondition(item.Object, "Accepted") {
			continue
		}
		accepted = append(accepted, item.GetName())
	}

	return accepted, nil
}

// gatewayClassChoice builds the picker. Istio is always offered because the
// gateway stack installs it, so its class exists once the stack has run even
// on a cluster that has no GatewayClass yet. The class in use is pre-selected,
// then the platform default, then Istio.
func gatewayClassChoice(accepted []string, inUse, platformDefault string) (options []string, current string) {
	options = append([]string{gateway.IstioGatewayClassName}, accepted...)
	sort.Strings(options)
	options = slices.Compact(options)

	for _, candidate := range []string{inUse, platformDefault} {
		if candidate != "" && slices.Contains(options, candidate) {
			return options, candidate
		}
	}

	return options, gateway.IstioGatewayClassName
}

// selectCertManagerIssuer picks the ClusterIssuer for Gateway TLS. Without any
// ClusterIssuer the only possible answer is plain HTTP, so nothing is asked.
func selectCertManagerIssuer(
	ctx context.Context,
	rt *core.Runtime,
	dyn dynamic.Interface,
	inUse string,
) (string, error) {
	title := "cert-manager ClusterIssuer for Gateway TLS"

	issuers, err := clusterIssuers(ctx, dyn)
	if err != nil {
		rt.Detailf("could not list ClusterIssuers (%v); prompting free-form", err)
		value, err := rt.Reporter.Input(title+" (empty serves HTTP only)", "letsencrypt", inUse)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(value), nil
	}

	if len(issuers) == 0 {
		rt.Detailf("no cert-manager ClusterIssuer on the cluster; the Gateway serves HTTP only")
		return "", nil
	}

	options, current := issuerChoice(issuers, inUse)

	selected, err := rt.Reporter.Select(title, current, options)
	if err != nil {
		return "", err
	}
	if selected == httpOnlyIssuerLabel {
		return "", nil
	}
	return selected, nil
}

// clusterIssuers lists the ClusterIssuers. A cluster without cert-manager has
// none.
func clusterIssuers(ctx context.Context, dyn dynamic.Interface) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, gatewayLookupTimeout)
	defer cancel()

	list, err := dyn.Resource(clusterIssuerGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if isResourceNotServed(err) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	sort.Strings(names)

	return names, nil
}

// issuerChoice pre-selects the issuer in use. A new Gateway starts on plain
// HTTP: an issuer only succeeds once DNS points at the Gateway, which cannot
// be true before the Gateway has an address.
func issuerChoice(issuers []string, inUse string) (options []string, current string) {
	options = append([]string{httpOnlyIssuerLabel}, issuers...)

	if inUse != "" && slices.Contains(issuers, inUse) {
		return options, inUse
	}
	return options, httpOnlyIssuerLabel
}

func hasTrueCondition(object map[string]any, conditionType string) bool {
	conditions, _, _ := unstructured.NestedSlice(object, "status", "conditions")
	for _, value := range conditions {
		condition, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == conditionType && condition["status"] == "True" {
			return true
		}
	}
	return false
}
