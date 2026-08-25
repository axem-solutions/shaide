package kube

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
		want       string
	}{
		{
			// Verified against aks-axem-dev-polandcentral.
			name:       "aks",
			providerID: "azure:///subscriptions/798052f0-f1e0-43aa-99d3-2a60a98801f2/resourceGroups/mc-aks-axem-dev-polandcentral/providers/Microsoft.Compute/virtualMachineScaleSets/aks-system-26278782-vmss/virtualMachines/0",
			want:       PlatformAzure,
		},
		{
			name:       "gke",
			providerID: "gce://axem-471708/europe-west1-b/gke-dev-cluster-default-pool-a1b2c3d4-xyz",
			want:       PlatformGCP,
		},
		{
			name:       "eks",
			providerID: "aws:///eu-central-1a/i-0abc123def456789",
			want:       PlatformAWS,
		},
		{
			// No cloud-controller-manager: RKE2, k3s, kubeadm on bare metal.
			name:       "on-prem empty providerID",
			providerID: "",
			want:       PlatformOnPrem,
		},
		{
			name:       "on-prem non-cloud scheme",
			providerID: "rke2://srv2rke2w1",
			want:       PlatformOnPrem,
		},
		{
			name:       "whitespace only",
			providerID: "   ",
			want:       PlatformOnPrem,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := platformForProviderID(tt.providerID); got != tt.want {
				t.Errorf("platformForProviderID(%q) = %q, want %q", tt.providerID, got, tt.want)
			}
		})
	}
}

func TestIsCloud(t *testing.T) {
	cloud := []string{PlatformAzure, PlatformGCP, PlatformAWS}
	for _, p := range cloud {
		if !IsCloud(p) {
			t.Errorf("IsCloud(%q) = false, want true", p)
		}
	}

	// An unset platform must not be treated as cloud: that would skip the
	// on-prem preload on a cluster we failed to identify.
	for _, p := range []string{PlatformOnPrem, ""} {
		if IsCloud(p) {
			t.Errorf("IsCloud(%q) = true, want false", p)
		}
	}
}

func TestDetectPlatform(t *testing.T) {
	node := func(providerID string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Spec:       corev1.NodeSpec{ProviderID: providerID},
		}
	}

	t.Run("reads providerID from a node", func(t *testing.T) {
		client := fake.NewSimpleClientset(node("azure:///subscriptions/x/resourceGroups/y"))
		got, err := DetectPlatform(context.Background(), client)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != PlatformAzure {
			t.Errorf("DetectPlatform() = %q, want %q", got, PlatformAzure)
		}
	})

	t.Run("no nodes is an error, not a silent on-prem", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		if _, err := DetectPlatform(context.Background(), client); err == nil {
			t.Error("expected an error for a cluster with no nodes, got nil")
		}
	})
}
