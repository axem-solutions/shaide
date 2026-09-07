package platform

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Architecture is the OS and CPU architecture a cluster's nodes run, in the
// form used by OCI image manifests ("linux", "amd64").
type Architecture struct {
	OS   string
	Arch string
}

func (a Architecture) String() string {
	return a.OS + "/" + a.Arch
}

func (a Architecture) IsZero() bool {
	return a.OS == "" && a.Arch == ""
}

// DetectArchitecture resolves the single architecture a cluster runs, so that
// images are mirrored for the platform the cluster can actually schedule.
//
// Unlike Detect, which samples one node because every node in a cluster shares
// an infrastructure provider, this lists every node: whether the nodes agree is
// the question being asked.
//
// Nodes marked unschedulable are ignored. A node cordoned for removal would
// otherwise contribute an architecture that nothing will ever be scheduled onto.
//
// A cluster reporting more than one architecture is refused rather than
// guessed at: mirroring for one of them would leave pods scheduled onto the
// others without images, and picking for the operator hides that.
func DetectArchitecture(ctx context.Context, client kubernetes.Interface) (Architecture, error) {
	found, err := DetectArchitectures(ctx, client)
	if err != nil {
		return Architecture{}, err
	}

	if len(found) > 1 {
		return Architecture{}, fmt.Errorf(
			"cluster nodes report more than one architecture (%s); "+
				"mixed-architecture clusters are not supported",
			joinArchitectures(found),
		)
	}

	return found[0], nil
}

// DetectArchitectures returns the distinct architectures of the cluster's
// schedulable nodes, sorted for a stable message.
func DetectArchitectures(ctx context.Context, client kubernetes.Interface) ([]Architecture, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes for architecture detection: %w", err)
	}

	seen := map[Architecture]struct{}{}

	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}

		architecture := architectureOf(node)
		if architecture.IsZero() {
			continue
		}

		seen[architecture] = struct{}{}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("no schedulable node reports an architecture, cannot detect the cluster architecture")
	}

	found := make([]Architecture, 0, len(seen))
	for architecture := range seen {
		found = append(found, architecture)
	}

	sort.Slice(found, func(i, j int) bool {
		return found[i].String() < found[j].String()
	})

	return found, nil
}

// architectureOf reads the values kubelet reports for the node it runs on.
func architectureOf(node corev1.Node) Architecture {
	return Architecture{
		OS:   strings.TrimSpace(node.Status.NodeInfo.OperatingSystem),
		Arch: strings.TrimSpace(node.Status.NodeInfo.Architecture),
	}
}

func joinArchitectures(architectures []Architecture) string {
	names := make([]string, 0, len(architectures))
	for _, architecture := range architectures {
		names = append(names, architecture.String())
	}

	return strings.Join(names, ", ")
}
