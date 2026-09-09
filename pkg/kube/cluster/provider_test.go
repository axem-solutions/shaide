package cluster

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestProviderForProviderID(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		want       Provider
	}{
		{"Azure", "azure:///subscriptions/example", Azure},
		{"GCP", "gce://project/zone/node", GCP},
		{"AWS", "aws:///zone/instance", AWS},
		{"empty is on-prem", "", OnPrem},
		{"unknown is on-prem", "rke2://node", OnPrem},
		{"whitespace is ignored", "  azure:///subscriptions/example  ", Azure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerForProviderID(tt.providerID); got != tt.want {
				t.Fatalf("providerForProviderID(%q) = %q, want %q", tt.providerID, got, tt.want)
			}
		})
	}
}

func TestProviderIsCloud(t *testing.T) {
	for _, value := range []Provider{Azure, GCP, AWS} {
		if !value.IsCloud() {
			t.Errorf("%q should be a cloud provider", value)
		}
	}

	for _, value := range []Provider{OnPrem, ""} {
		if value.IsCloud() {
			t.Errorf("%q should not be a cloud provider", value)
		}
	}
}

func TestDetectProvider(t *testing.T) {
	t.Run("detects the node provider", func(t *testing.T) {
		client := fake.NewClientset(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Spec:       corev1.NodeSpec{ProviderID: "azure:///subscriptions/example"},
		})

		got, err := DetectProvider(context.Background(), client)
		if err != nil {
			t.Fatalf("DetectProvider() error = %v", err)
		}
		if got != Azure {
			t.Errorf("DetectProvider() = %q, want %q", got, Azure)
		}
	})

	t.Run("requires at least one node", func(t *testing.T) {
		client := fake.NewClientset()
		if _, err := DetectProvider(context.Background(), client); err == nil {
			t.Fatal("DetectProvider() error = nil, want an error")
		}
	})
}
