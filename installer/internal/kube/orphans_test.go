package kube

import (
	"context"
	"sort"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const namespace = "llm-d-gpt-oss-20b"

func testPod(name, node string, finalizers ...string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Finalizers: finalizers},
		Spec:       corev1.PodSpec{NodeName: node},
	}
}

// keepPodsWithFinalizers makes deleting a pod with finalizers mark it deleted
// instead of removing it, as the API server does.
func keepPodsWithFinalizers(client *fake.Clientset) {
	client.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		name := action.(k8stesting.DeleteAction).GetName()
		obj, err := client.Tracker().Get(corev1.SchemeGroupVersion.WithResource("pods"), namespace, name)
		if err != nil {
			return false, nil, nil
		}

		pod := obj.(*corev1.Pod)
		if len(pod.Finalizers) == 0 {
			return false, nil, nil
		}

		now := metav1.Now()
		pod.DeletionTimestamp = &now
		return true, nil, client.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), pod, namespace)
	})
}

func podNames(pods []OrphanedPod) []string {
	var names []string
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	sort.Strings(names)
	return names
}

func TestReleaseOrphanedPods(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "live-node"}},
		testPod("on-live-node", "live-node"),
		testPod("pending", ""),
		testPod("orphan", "deleted-node"),
		testPod("orphan-foreground", "deleted-node", metav1.FinalizerDeleteDependents),
		testPod("orphan-foreign", "deleted-node", "example.com/protect"),
	)
	keepPodsWithFinalizers(client)

	released, held, err := ReleaseOrphanedPods(context.Background(), client, namespace)
	if err != nil {
		t.Fatalf("ReleaseOrphanedPods: %v", err)
	}

	if got := podNames(released); len(got) != 2 || got[0] != "orphan" || got[1] != "orphan-foreground" {
		t.Errorf("released = %v, want orphan and orphan-foreground", got)
	}
	if got := podNames(held); len(got) != 1 || got[0] != "orphan-foreign" {
		t.Errorf("held = %v, want orphan-foreign", got)
	}

	pods := client.CoreV1().Pods(namespace)
	ctx := context.Background()

	for _, name := range []string{"on-live-node", "pending"} {
		if _, err := pods.Get(ctx, name, metav1.GetOptions{}); err != nil {
			t.Errorf("%s was touched: %v", name, err)
		}
	}
	if _, err := pods.Get(ctx, "orphan", metav1.GetOptions{}); err == nil {
		t.Error("orphan was not deleted")
	}

	foreground, err := pods.Get(ctx, "orphan-foreground", metav1.GetOptions{})
	if err == nil && len(foreground.Finalizers) != 0 {
		t.Errorf("orphan-foreground finalizers = %v, want the garbage-collector finalizer removed", foreground.Finalizers)
	}

	foreign, err := pods.Get(ctx, "orphan-foreign", metav1.GetOptions{})
	if err != nil || len(foreign.Finalizers) != 1 {
		t.Errorf("orphan-foreign = %v, %v, want it kept with its finalizer", foreign, err)
	}
}

// A namespace that does not exist, as for a model never installed, has
// nothing to release.
func TestReleaseOrphanedPodsInAnEmptyNamespace(t *testing.T) {
	released, held, err := ReleaseOrphanedPods(context.Background(), fake.NewClientset(), "llm-d-not-installed")
	if err != nil || len(released) != 0 || len(held) != 0 {
		t.Errorf("released = %v, held = %v, err = %v, want nothing", released, held, err)
	}
}
