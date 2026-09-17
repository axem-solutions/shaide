package discovery

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	nodeRegistryNamespace = "node-registry-config"
	nodeRegistryName      = "node-registry-config"
	nodeRegistryImage     = "busybox:1.36"
	nodeRegistryTimeout   = 90 * time.Second
)

// ensureHarborNodeAccess configures containerd on managed-cloud nodes to pull
// from Harbor's HTTP ClusterIP. Kubelet image pulls happen outside Pod DNS, so
// harbor.<namespace>.svc.cluster.local is otherwise not resolvable there.
//
// This runs even when Harbor was discovered rather than installed by this
// installer, which keeps an existing Harbor deployment usable by app-shaide.
func ensureHarborNodeAccess(rt *core.Runtime) error {
	if !platform.Platform(rt.Bootstrap.CloudPlatform).IsCloud() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), nodeRegistryTimeout)
	defer cancel()

	harborNamespace := rt.Bootstrap.Config.Harbor.Namespace
	harborService := rt.Bootstrap.Config.Harbor.Service
	svc, err := rt.Cluster.Client.CoreV1().Services(harborNamespace).Get(
		ctx,
		harborService,
		metav1.GetOptions{},
	)
	if err != nil {
		return fmt.Errorf("read Harbor service ClusterIP: %w", err)
	}
	if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == corev1.ClusterIPNone || net.ParseIP(svc.Spec.ClusterIP) == nil {
		return fmt.Errorf(
			"Harbor service %s/%s has no usable ClusterIP",
			harborNamespace,
			harborService,
		)
	}

	hostname := fmt.Sprintf("%s.%s.svc.cluster.local", harborService, harborNamespace)
	if err := reconcileNodeRegistryNamespace(ctx, rt.Cluster.Client); err != nil {
		return err
	}
	if err := reconcileNodeRegistryDaemonSet(ctx, rt.Cluster.Client, hostname, svc.Spec.ClusterIP); err != nil {
		return err
	}

	if err := waitForNodeRegistryDaemonSet(ctx, rt.Cluster.Client); err != nil {
		return err
	}

	rt.Detailf("Harbor node access ready for %s via %s", hostname, svc.Spec.ClusterIP)
	return nil
}

func reconcileNodeRegistryNamespace(ctx context.Context, client kubernetes.Interface) error {
	namespaces := client.CoreV1().Namespaces()
	ns, err := namespaces.Get(ctx, nodeRegistryNamespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = namespaces.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeRegistryNamespace,
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "privileged",
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create Harbor node-access namespace: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Harbor node-access namespace: %w", err)
	}

	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}
	if ns.Labels["pod-security.kubernetes.io/enforce"] == "privileged" {
		return nil
	}
	ns.Labels["pod-security.kubernetes.io/enforce"] = "privileged"
	if _, err := namespaces.Update(ctx, ns, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update Harbor node-access namespace: %w", err)
	}
	return nil
}

func reconcileNodeRegistryDaemonSet(ctx context.Context, client kubernetes.Interface, hostname, clusterIP string) error {
	daemonSets := client.AppsV1().DaemonSets(nodeRegistryNamespace)
	desired := nodeRegistryDaemonSet(hostname, clusterIP)
	current, err := daemonSets.Get(ctx, nodeRegistryName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := daemonSets.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create Harbor node-access DaemonSet: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Harbor node-access DaemonSet: %w", err)
	}

	desired.ResourceVersion = current.ResourceVersion
	if _, err := daemonSets.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update Harbor node-access DaemonSet: %w", err)
	}
	return nil
}

func waitForNodeRegistryDaemonSet(ctx context.Context, client kubernetes.Interface) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		ds, err := client.AppsV1().DaemonSets(nodeRegistryNamespace).Get(ctx, nodeRegistryName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("read Harbor node-access DaemonSet status: %w", err)
		}
		if ds.Status.DesiredNumberScheduled > 0 &&
			ds.Status.NumberAvailable == ds.Status.DesiredNumberScheduled &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"wait for Harbor node access on all nodes: %w",
				ctx.Err(),
			)
		case <-ticker.C:
		}
	}
}

func nodeRegistryDaemonSet(hostname, clusterIP string) *appsv1.DaemonSet {
	labels := map[string]string{"app": nodeRegistryName}
	script := fmt.Sprintf(`while true; do
  mkdir -p /host/certs.d/%[1]s
  cat > /host/certs.d/%[1]s/hosts.toml <<'TOML'
server = "http://%[2]s"

[host."http://%[2]s"]
  capabilities = ["pull", "resolve"]
TOML
  sleep 300
done`, hostname, clusterIP)

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeRegistryName,
			Namespace: nodeRegistryNamespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
					Containers: []corev1.Container{{
						Name:    "config-writer",
						Image:   nodeRegistryImage,
						Command: []string{"sh", "-c", script},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "certs-d",
							MountPath: "/host/certs.d",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "certs-d",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/etc/containerd/certs.d",
								Type: hostPathType(corev1.HostPathDirectoryOrCreate),
							},
						},
					}},
				},
			},
		},
	}
}

func hostPathType(value corev1.HostPathType) *corev1.HostPathType {
	return &value
}
