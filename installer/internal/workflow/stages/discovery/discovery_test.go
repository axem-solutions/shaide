package discovery

import (
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/harbor/auth"
	workflowcore "github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCloudModeIncludesUpdates(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		mode     workflowcore.Installation
		want     bool
	}{
		{name: "azure install", platform: "azure", mode: workflowcore.Install, want: true},
		{name: "azure update", platform: "azure", mode: workflowcore.Update, want: true},
		{name: "gcp update", platform: "gcp", mode: workflowcore.Update, want: true},
		{name: "on-prem install", platform: "on-prem", mode: workflowcore.Install, want: false},
		{name: "unknown platform", platform: "", mode: workflowcore.Update, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &workflowcore.Runtime{GlobalState: workflowcore.NewGlobalState()}
			rt.Bootstrap.CloudPlatform = tt.platform
			rt.Discovery.Mode = tt.mode

			if got := CloudMode(rt); got != tt.want {
				t.Fatalf("CloudMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadExistingHarborCredentials(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "harbor-core",
			Namespace: "harbor",
		},
		Data: map[string][]byte{
			"HARBOR_ADMIN_PASSWORD": []byte("existing-admin-password"),
		},
	})

	rt := &workflowcore.Runtime{GlobalState: workflowcore.NewGlobalState()}
	rt.Cluster.Client = client
	rt.Bootstrap.Config.Harbor.Namespace = "harbor"
	rt.Bootstrap.Config.Harbor.Service = "harbor"
	rt.Discovery.Auth = auth.Credentials{
		Username: "robot$k8s-harbor-sa",
		Password: "existing-robot-password",
	}

	loadExistingHarborCredentials(rt)

	if got := rt.Discovery.AdminPassword; got != "existing-admin-password" {
		t.Fatalf("AdminPassword = %q, want existing secret value", got)
	}
	if got := rt.Discovery.RobotPassword; got != "existing-robot-password" {
		t.Fatalf("RobotPassword = %q, want pull-secret password", got)
	}
}

func TestLoadExistingHarborCredentialsFallsBackToPromptForMissingAdminSecret(t *testing.T) {
	rt := &workflowcore.Runtime{GlobalState: workflowcore.NewGlobalState()}
	rt.Cluster.Client = fake.NewSimpleClientset()
	rt.Bootstrap.Config.Harbor.Namespace = "harbor"
	rt.Bootstrap.Config.Harbor.Service = "harbor"
	rt.Discovery.Auth = auth.Credentials{Password: "existing-robot-password"}

	loadExistingHarborCredentials(rt)

	if rt.Discovery.AdminPassword != "" {
		t.Fatalf("AdminPassword = %q, want empty so DeployHarbor prompts", rt.Discovery.AdminPassword)
	}
	if got := rt.Discovery.RobotPassword; got != "existing-robot-password" {
		t.Fatalf("RobotPassword = %q, want pull-secret password", got)
	}
}
