package platform

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPlatformForProviderID(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		want       Platform
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
			if got := platformForProviderID(tt.providerID); got != tt.want {
				t.Fatalf("platformForProviderID(%q) = %q, want %q", tt.providerID, got, tt.want)
			}
		})
	}
}

func TestPlatformIsCloud(t *testing.T) {
	for _, value := range []Platform{Azure, GCP, AWS} {
		if !value.IsCloud() {
			t.Errorf("%q should be a cloud platform", value)
		}
	}

	for _, value := range []Platform{OnPrem, ""} {
		if value.IsCloud() {
			t.Errorf("%q should not be a cloud platform", value)
		}
	}
}

func TestDetect(t *testing.T) {
	t.Run("detects the node provider", func(t *testing.T) {
		client := fake.NewClientset(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Spec:       corev1.NodeSpec{ProviderID: "azure:///subscriptions/example"},
		})

		got, err := Detect(context.Background(), client)
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if got != Azure {
			t.Errorf("Detect() = %q, want %q", got, Azure)
		}
	})

	t.Run("requires at least one node", func(t *testing.T) {
		client := fake.NewClientset()
		if _, err := Detect(context.Background(), client); err == nil {
			t.Fatal("Detect() error = nil, want an error")
		}
	})
}
