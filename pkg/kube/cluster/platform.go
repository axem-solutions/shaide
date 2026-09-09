package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Platform is the OS and CPU architecture a cluster's nodes run, in the
// form used by OCI image manifests ("linux", "amd64").
type Platform struct {
	OS   string
	Arch string
}

func (a Platform) String() string {
	return a.OS + "/" + a.Arch
}

func (a Platform) IsValid() bool {
	return !(a.OS == "" && a.Arch == "")
}

// DetectPlatform resolves the single platform a cluster runs, so that
// images are mirrored for the platform the cluster can actually schedule.
//
// Nodes marked unschedulable are ignored. A node cordoned for removal would
// otherwise contribute a platform that nothing will ever be scheduled onto.
//
// A cluster reporting more than one platform is refused rather than
// guessed at: mirroring for one of them would leave pods scheduled onto the
// others without images, and picking for the operator hides that.
func DetectPlatform(ctx context.Context, client kubernetes.Interface) (Platform, error) {
	found, err := DetectPlatforms(ctx, client)
	if err != nil {
		return Platform{}, err
	}

	if len(found) > 1 {
		return Platform{}, fmt.Errorf(
			"cluster nodes report more than one platform (%s); "+
				"mixed-platform clusters are not supported",
			joinPlatforms(found),
		)
	}

	return found[0], nil
}

// DetectPlatforms returns the distinct platforms of the cluster's
// schedulable nodes, sorted for a stable message.
func DetectPlatforms(ctx context.Context, client kubernetes.Interface) ([]Platform, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes for platform detection: %w", err)
	}

	seen := map[Platform]struct{}{}

	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}

		platform := platformOf(node)
		if !platform.IsValid() {
			continue
		}

		seen[platform] = struct{}{}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("no schedulable node reports a platform, cannot detect the cluster platform")
	}

	found := make([]Platform, 0, len(seen))
	for platform := range seen {
		found = append(found, platform)
	}

	sort.Slice(found, func(i, j int) bool {
		return found[i].String() < found[j].String()
	})

	return found, nil
}

// platformOf reads the values kubelet reports for the node it runs on.
func platformOf(node corev1.Node) Platform {
	return Platform{
		OS:   strings.TrimSpace(node.Status.NodeInfo.OperatingSystem),
		Arch: strings.TrimSpace(node.Status.NodeInfo.Architecture),
	}
}

func joinPlatforms(platforms []Platform) string {
	names := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		names = append(names, platform.String())
	}

	return strings.Join(names, ", ")
}
