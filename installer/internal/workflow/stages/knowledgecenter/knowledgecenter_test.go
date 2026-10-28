package knowledgecenter

import (
	"context"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func shaideDeployment(name string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: shaide.Namespace}}
}

// An update pre-selects what the cluster runs, so accepting the default keeps
// the earlier answer; a fresh install leaves the Knowledge Center out.
func TestInstalledFollowsTheCluster(t *testing.T) {
	for _, tc := range []struct {
		name    string
		objects []runtime.Object
		want    bool
	}{
		{"without Knowledge Center", nil, false},
		{"with only the webapp", []runtime.Object{shaideDeployment(shaide.WebAppName)}, false},
		{"with Knowledge Center", []runtime.Object{shaideDeployment(shaide.KnowledgeCenterName)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset(tc.objects...)

			if got := installed(context.Background(), client, t.Logf); got != tc.want {
				t.Errorf("installed() = %v, want %v", got, tc.want)
			}
		})
	}
}
