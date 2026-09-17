package discovery

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReconcileNodeRegistryResources(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	if err := reconcileNodeRegistryNamespace(ctx, client); err != nil {
		t.Fatalf("reconcile namespace: %v", err)
	}
	if err := reconcileNodeRegistryDaemonSet(
		ctx,
		client,
		"harbor.harbor.svc.cluster.local",
		"10.96.214.170",
	); err != nil {
		t.Fatalf("reconcile DaemonSet: %v", err)
	}

	ns, err := client.CoreV1().Namespaces().Get(ctx, nodeRegistryNamespace, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if got := ns.Labels["pod-security.kubernetes.io/enforce"]; got != "privileged" {
		t.Fatalf("pod security label = %q, want privileged", got)
	}

	ds, err := client.AppsV1().DaemonSets(nodeRegistryNamespace).Get(ctx, nodeRegistryName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get DaemonSet: %v", err)
	}
	if len(ds.Spec.Template.Spec.Tolerations) != 1 || ds.Spec.Template.Spec.Tolerations[0].Operator != "Exists" {
		t.Fatalf("tolerations = %#v, want one Exists toleration", ds.Spec.Template.Spec.Tolerations)
	}
	script := ds.Spec.Template.Spec.Containers[0].Command[2]
	for _, want := range []string{"harbor.harbor.svc.cluster.local", "http://10.96.214.170"} {
		if !strings.Contains(script, want) {
			t.Fatalf("script does not contain %q: %s", want, script)
		}
	}

	if err := reconcileNodeRegistryDaemonSet(
		ctx,
		client,
		"harbor.harbor.svc.cluster.local",
		"10.96.214.171",
	); err != nil {
		t.Fatalf("update DaemonSet: %v", err)
	}
	ds, err = client.AppsV1().DaemonSets(nodeRegistryNamespace).Get(ctx, nodeRegistryName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated DaemonSet: %v", err)
	}
	if script = ds.Spec.Template.Spec.Containers[0].Command[2]; !strings.Contains(script, "http://10.96.214.171") {
		t.Fatalf("updated script does not contain new ClusterIP: %s", script)
	}
}
