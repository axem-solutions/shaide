package shaide

import (
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// The installer reuses the credentials an earlier run stored, so reading them
// back must find every key Resolve wrote.
func TestKnowledgeCenterCredentialsRoundTripThroughStackConfig(t *testing.T) {
	enabled := true
	want := knowledgecentertest.Credentials()
	images := testImages()
	images[KnowledgeCenterImage] = "harbor/shaide/shaide_knowledge_center:v1"

	stack := newTestStack(Options{
		Images:                 images,
		KnowledgeCenterEnabled: &enabled,
		KnowledgeCenter:        want,
	})

	resolved, err := stack.Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if got := stack.KnowledgeCenterCredentials(resolved); got != want {
		t.Error("KnowledgeCenterCredentials() did not read back the resolved credentials")
	}
}

// A stack that never had the Knowledge Center has none to reuse.
func TestKnowledgeCenterCredentialsOfAStackWithout(t *testing.T) {
	if got := newTestStack(Options{}).KnowledgeCenterCredentials(auto.ConfigMap{}); got != (knowledgecenter.Credentials{}) {
		t.Errorf("KnowledgeCenterCredentials() = %+v, want none", got)
	}
}

// The installer signs in as admin with the password this run resolved.
func TestAdminPasswordReadsTheResolvedSecret(t *testing.T) {
	stack := newTestStack(Options{})

	resolved, err := stack.Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	password, ok := stack.AdminPassword(resolved)
	if !ok || password != "answer" {
		t.Errorf("AdminPassword() = %q, %v; want the prompted answer", password, ok)
	}
}
