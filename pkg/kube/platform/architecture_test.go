package platform

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func node(name string, os string, arch string, unschedulable bool) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.NodeSpec{Unschedulable: unschedulable},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				OperatingSystem: os,
				Architecture:    arch,
			},
		},
	}
}

func TestDetectArchitectureSingle(t *testing.T) {
	client := fake.NewSimpleClientset(
		node("worker-1", "linux", "amd64", false),
		node("worker-2", "linux", "amd64", false),
		node("system-1", "linux", "amd64", false),
	)

	architecture, err := DetectArchitecture(context.Background(), client)
	if err != nil {
		t.Fatalf("DetectArchitecture() error = %v", err)
	}

	if got, want := architecture.String(), "linux/amd64"; got != want {
		t.Errorf("architecture = %q, want %q", got, want)
	}
}

// A node cordoned for removal must not contribute an architecture: nothing new
// will be scheduled onto it, so mirroring for it would be wasted.
func TestDetectArchitectureIgnoresUnschedulableNodes(t *testing.T) {
	client := fake.NewSimpleClientset(
		node("worker-1", "linux", "amd64", false),
		node("draining", "linux", "arm64", true),
	)

	architecture, err := DetectArchitecture(context.Background(), client)
	if err != nil {
		t.Fatalf("DetectArchitecture() error = %v", err)
	}

	if got, want := architecture.String(), "linux/amd64"; got != want {
		t.Errorf("architecture = %q, want %q", got, want)
	}
}

// Mixed clusters are refused rather than guessed at, and the message has to say
// what was found so the operator can act on it.
func TestDetectArchitectureRejectsMixedCluster(t *testing.T) {
	client := fake.NewSimpleClientset(
		node("worker-1", "linux", "amd64", false),
		node("worker-2", "linux", "arm64", false),
	)

	_, err := DetectArchitecture(context.Background(), client)
	if err == nil {
		t.Fatal("DetectArchitecture() succeeded on a mixed cluster")
	}

	for _, want := range []string{"linux/amd64", "linux/arm64", "not supported"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestDetectArchitecturesIsSortedAndDistinct(t *testing.T) {
	client := fake.NewSimpleClientset(
		node("a", "linux", "arm64", false),
		node("b", "linux", "amd64", false),
		node("c", "linux", "amd64", false),
	)

	found, err := DetectArchitectures(context.Background(), client)
	if err != nil {
		t.Fatalf("DetectArchitectures() error = %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("found %d architectures, want 2: %v", len(found), found)
	}
	if found[0].String() != "linux/amd64" || found[1].String() != "linux/arm64" {
		t.Errorf("architectures = %v, want a sorted [linux/amd64 linux/arm64]", found)
	}
}

func TestDetectArchitectureNoUsableNodes(t *testing.T) {
	tests := []struct {
		name  string
		nodes []runtime.Object
	}{
		{name: "no nodes"},
		{name: "all cordoned", nodes: []runtime.Object{node("a", "linux", "amd64", true)}},
		{name: "architecture unreported", nodes: []runtime.Object{node("a", "", "", false)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(test.nodes...)
			if _, err := DetectArchitecture(context.Background(), client); err == nil {
				t.Error("DetectArchitecture() succeeded with no usable node")
			}
		})
	}
}
