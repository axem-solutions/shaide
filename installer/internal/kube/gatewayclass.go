package kube

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// gatewayClassGVR identifies the Gateway API GatewayClass resource. It is a CRD,
// so it is not part of the typed clientset and has to be read dynamically.
var gatewayClassGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "gatewayclasses",
}

// ListGatewayClasses returns the names of the GatewayClasses the cluster
// accepts, sorted. A class whose Accepted condition is not True is skipped:
// selecting it would produce a Gateway that never programs.
//
// An empty result is not an error — the Gateway API CRDs may not be installed
// yet, in which case the caller should fall back to free-text entry.
func ListGatewayClasses(ctx context.Context, cfg *rest.Config) ([]string, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}

	list, err := dyn.Resource(gatewayClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list gatewayclasses: %w", err)
	}

	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		if !isGatewayClassAccepted(item.Object) {
			continue
		}
		if name := item.GetName(); name != "" {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	return names, nil
}

func isGatewayClassAccepted(obj map[string]any) bool {
	status, ok := obj["status"].(map[string]any)
	if !ok {
		return false
	}

	conditions, ok := status["conditions"].([]any)
	if !ok {
		return false
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == "Accepted" && cond["status"] == "True" {
			return true
		}
	}

	return false
}
