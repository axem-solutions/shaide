package sharedgateway

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var gatewayClassGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "gatewayclasses",
}

// validateGatewayClass checks that the configured class is accepted by the
// target cluster. A missing GatewayClass CRD or an empty accepted-class list is
// allowed because the same Pulumi update may still be installing the CRD and
// controller that creates the class.
func validateGatewayClass(
	ctx context.Context,
	kube connection.Connection,
	configured string,
	allowMissing bool,
) error {
	validationCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	restConfig, err := connection.BuildRestConfig(kube)
	if err != nil {
		return fmt.Errorf("build Kubernetes REST config: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("build dynamic client: %w", err)
	}

	list, err := dyn.Resource(gatewayClassGVR).List(validationCtx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		// Validation is best-effort: the CRD/controller may be part of this
		// update, and restricted installer credentials may not allow listing
		// cluster-scoped resources. Kubernetes will still validate the Gateway
		// when Pulumi applies it.
		return nil
	}

	accepted := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		if isAccepted(item.Object) && item.GetName() != "" {
			accepted = append(accepted, item.GetName())
		}
	}
	if len(accepted) == 0 {
		return nil
	}

	sort.Strings(accepted)
	if !slices.Contains(accepted, configured) {
		if allowMissing {
			return nil
		}
		return fmt.Errorf(
			"configured GatewayClass %q is not accepted by the cluster; accepted classes: %s",
			configured,
			strings.Join(accepted, ", "),
		)
	}

	return nil
}

func isAccepted(obj map[string]any) bool {
	status, ok := obj["status"].(map[string]any)
	if !ok {
		return false
	}

	conditions, ok := status["conditions"].([]any)
	if !ok {
		return false
	}

	for _, value := range conditions {
		condition, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == "Accepted" && condition["status"] == "True" {
			return true
		}
	}

	return false
}
