package legacy

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const legacyLabel = "ms-example-llm-d-modelservice"

// deployment mirrors how the charts labeled their objects: the retired
// modelservice chart put llm-d.ai/model on the pod template only, with Helm's
// own labels on the Deployment.
func deployment(name, model string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "llm-d-example",
			Labels: map[string]string{"helm.sh/chart": "llm-d-modelservice-v0.4.12"},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{modelLabel: model, "llm-d.ai/role": "decode"}},
			},
		},
	}
}

// Only the retired chart's Deployments go; the new model server, labeled
// differently, stays.
func TestRemoveModelServersDeletesOnlyTheLegacyOnes(t *testing.T) {
	client := fake.NewClientset(
		deployment("ms-example-llm-d-modelservice-decode", legacyLabel),
		deployment("model-example", "model-example"),
	)

	var logged int
	if err := RemoveModelServers(context.Background(), client, "llm-d-example", legacyLabel, func(string, ...any) { logged++ }); err != nil {
		t.Fatal(err)
	}

	left, err := client.AppsV1().Deployments("llm-d-example").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(left.Items) != 1 || left.Items[0].Name != "model-example" {
		t.Errorf("remaining = %+v, want only the new model server", left.Items)
	}
	if logged != 1 {
		t.Errorf("logged %d removals, want 1", logged)
	}
}

func TestRemoveModelServersIsANoOpWithoutLegacy(t *testing.T) {
	client := fake.NewClientset()
	if err := RemoveModelServers(context.Background(), client, "llm-d-example", legacyLabel, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunningIgnoresFinishedPods(t *testing.T) {
	pods := []corev1.Pod{
		{Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{Status: corev1.PodStatus{Phase: corev1.PodSucceeded}},
		{Status: corev1.PodStatus{Phase: corev1.PodFailed}},
	}
	if got := running(pods); len(got) != 1 {
		t.Errorf("running = %d pods, want 1", len(got))
	}
}
