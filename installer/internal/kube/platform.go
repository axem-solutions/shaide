package kube

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Cloud platform identifiers. These are the same values the gateway-provider
// stage offers in its platform Select and that the app-serving / app-shaide
// stacks read as their cloudProvider config.
const (
	PlatformGCP    = "gcp"
	PlatformAWS    = "aws"
	PlatformAzure  = "azure"
	PlatformOnPrem = "on-prem"
)

// providerIDPrefixes maps the scheme of a Node's spec.providerID to a platform.
// The field is set by the cloud-controller-manager, so a cluster without one
// (RKE2, k3s, kubeadm on bare metal) leaves it empty and is treated as on-prem.
var providerIDPrefixes = []struct {
	prefix   string
	platform string
}{
	{"azure://", PlatformAzure},
	{"gce://", PlatformGCP},
	{"aws://", PlatformAWS},
}

// DetectPlatform infers the target platform from the cluster's nodes.
//
// spec.providerID is the canonical signal: it is populated by the
// cloud-controller-manager and encodes the provider as a URI scheme, e.g.
//
//	azure:///subscriptions/<sub>/resourceGroups/<rg>/providers/Microsoft.Compute/...
//	gce://<project>/<zone>/<instance>
//	aws:///<zone>/<instance-id>
//
// On-prem distributions have no cloud-controller-manager, so providerID is
// empty (or carries a non-cloud scheme such as rke2://) and we report on-prem.
//
// Nodes are listed with a limit of 1: every node in a cluster shares the same
// provider, so one is enough and it keeps the call cheap.
func DetectPlatform(ctx context.Context, client kubernetes.Interface) (string, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return "", fmt.Errorf("list nodes for platform detection: %w", err)
	}

	if len(nodes.Items) == 0 {
		return "", fmt.Errorf("cluster reports no nodes, cannot detect platform")
	}

	return platformForProviderID(nodes.Items[0].Spec.ProviderID), nil
}

func platformForProviderID(providerID string) string {
	id := strings.TrimSpace(providerID)
	for _, p := range providerIDPrefixes {
		if strings.HasPrefix(id, p.prefix) {
			return p.platform
		}
	}

	return PlatformOnPrem
}

// IsCloud reports whether the platform is a managed cloud provider, i.e. one
// where nodes can pull from public registries and no image side-loading is
// required.
func IsCloud(platform string) bool {
	return platform != PlatformOnPrem && platform != ""
}
