package platform

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Platform string

const (
	PlatformGCP    Platform = "gcp"
	PlatformAWS    Platform = "aws"
	PlatformAzure  Platform = "azure"
	PlatformOnPrem Platform = "on-prem"
)

// providerIDPrefixes maps the scheme of a Node's spec.providerID to a platform.
// The field is set by the cloud-controller-manager, so a cluster without one
// (RKE2, k3s, kubeadm on bare metal) leaves it empty and is treated as on-prem.
var providerIDPrefixes = []struct {
	prefix   string
	platform Platform
}{
	{"azure://", PlatformAzure},
	{"gce://", PlatformGCP},
	{"aws://", PlatformAWS},
}

func (p Platform) Validate() error {
	switch p {
	case PlatformOnPrem, PlatformGCP, PlatformAWS, PlatformAzure:
		return nil
	default:
		return fmt.Errorf(
			"invalid platform %q: expected %q, %q, %q, or %q",
			p,
			PlatformOnPrem,
			PlatformGCP,
			PlatformAWS,
			PlatformAzure,
		)
	}
}

// IsCloud reports whether the platform is a managed cloud provider.
func (p Platform) IsCloud() bool {
	switch p {
	case PlatformGCP, PlatformAWS, PlatformAzure:
		return true
	default:
		return false
	}
}

// Detect infers the target platform from the cluster's nodes.
//
// spec.providerID is populated by the cloud-controller-manager and encodes
// the cloud provider as a URI scheme:
//
//	azure:///subscriptions/<sub>/resourceGroups/<rg>/providers/Microsoft.Compute/...
//	gce://<project>/<zone>/<instance>
//	aws:///<zone>/<instance-id>
//
// On-prem distributions normally have no cloud-controller-manager, so
// providerID is empty or uses an unknown scheme and is treated as on-prem.
//
// Nodes are listed with a limit of 1 because every node in a cluster is
// expected to use the same infrastructure provider.
func Detect(ctx context.Context, client kubernetes.Interface) (Platform, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return "", fmt.Errorf("list nodes for platform detection: %w", err)
	}

	if len(nodes.Items) == 0 {
		return "", fmt.Errorf("cluster reports no nodes, cannot detect platform")
	}

	return platformForProviderID(nodes.Items[0].Spec.ProviderID), nil
}

func platformForProviderID(providerID string) Platform {
	id := strings.TrimSpace(providerID)

	for _, p := range providerIDPrefixes {
		if strings.HasPrefix(id, p.prefix) {
			return p.platform
		}
	}

	return PlatformOnPrem
}
