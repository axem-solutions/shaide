package iac

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	westeurope    = "aks-axem-dev-westeurope"
	polandcentral = "aks-axem-dev-polandcentral"
)

func testKubeconfig() *clientcmdapi.Config {
	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"we": {Server: "https://we.example.azmk8s.io:443"},
			"pc": {Server: "https://pc.example.azmk8s.io:443"},
		},
		Contexts: map[string]*clientcmdapi.Context{
			westeurope:         {Cluster: "we"},
			polandcentral:      {Cluster: "pc"},
			"westeurope-alias": {Cluster: "we"},
		},
		CurrentContext: polandcentral,
	}
}

const providerURN = "urn:pulumi:shaide::app-shaide::pulumi:providers:kubernetes::app-shaide-k8s"

func provider(id, context string, deleted bool) apitype.ResourceV3 {
	inputs := map[string]any{"kubeconfig": "/.kube/config"}
	if context != "" {
		inputs["context"] = context
	}
	return apitype.ResourceV3{
		URN:    resource.URN(providerURN),
		ID:     resource.ID(id),
		Type:   tokens.Type(kubernetesProviderType),
		Inputs: inputs,
		Delete: deleted,
	}
}

func managed(kind, name, providerID string, deleted bool) apitype.ResourceV3 {
	return apitype.ResourceV3{
		URN:      resource.URN("urn:pulumi:shaide::app-shaide::" + kind + "::" + name),
		Type:     tokens.Type(kind),
		Provider: providerURN + "::" + providerID,
		Delete:   deleted,
	}
}

func TestSameClusterIsClean(t *testing.T) {
	report := inspectClusterTarget([]apitype.ResourceV3{
		provider("p1", westeurope, false),
		managed("kubernetes:apps/v1:Deployment", "control-panel", "p1", false),
	}, testKubeconfig(), westeurope)

	if len(report.Mismatched)+len(report.Unverified)+len(report.PendingDeletes) != 0 {
		t.Errorf("report = %+v, want nothing to report", report)
	}
}

// A context renamed in the kubeconfig still names the same API server, so it
// is not a move.
func TestRenamedContextForSameServerIsClean(t *testing.T) {
	report := inspectClusterTarget([]apitype.ResourceV3{
		provider("p1", "westeurope-alias", false),
	}, testKubeconfig(), westeurope)

	if len(report.Mismatched)+len(report.Unverified) != 0 {
		t.Errorf("report = %+v, want the alias accepted", report)
	}
}

// Reusing one cluster's state dir for another: deploying would replace every
// resource onto the selected cluster and delete the originals.
func TestOtherClusterIsMismatched(t *testing.T) {
	report := inspectClusterTarget([]apitype.ResourceV3{
		provider("p1", polandcentral, false),
	}, testKubeconfig(), westeurope)

	if len(report.Mismatched) != 1 {
		t.Fatalf("mismatched = %v, want the polandcentral provider", report.Mismatched)
	}
	for _, want := range []string{polandcentral, "pc.example", westeurope, "we.example"} {
		if !strings.Contains(report.Mismatched[0], want) {
			t.Errorf("message %q does not name %q", report.Mismatched[0], want)
		}
	}
}

// A provider recorded without a context used whatever current-context the
// kubeconfig had at the time, which cannot be established afterwards.
func TestProviderWithoutContextIsUnverified(t *testing.T) {
	report := inspectClusterTarget([]apitype.ResourceV3{
		provider("p1", "", false),
	}, testKubeconfig(), westeurope)

	if len(report.Unverified) != 1 || len(report.Mismatched) != 0 {
		t.Errorf("report = %+v, want the provider unverified", report)
	}
}

func TestUnknownContextIsUnverified(t *testing.T) {
	report := inspectClusterTarget([]apitype.ResourceV3{
		provider("p1", "deleted-cluster", false),
	}, testKubeconfig(), westeurope)

	if len(report.Unverified) != 1 || !strings.Contains(report.Unverified[0], "deleted-cluster") {
		t.Errorf("unverified = %v, want the unknown context named", report.Unverified)
	}
}

// The state the westeurope incident left behind: an interrupted replacement
// with the live provider on no context and the westeurope originals pending
// deletion. The next update would delete those first.
func TestInterruptedReplacementReportsPendingDeletes(t *testing.T) {
	report := inspectClusterTarget([]apitype.ResourceV3{
		provider("new", "", false),
		provider("old", westeurope, true),
		managed("kubernetes:core/v1:Namespace", "app-shaide", "new", false),
		managed("kubernetes:core/v1:Namespace", "app-shaide", "old", true),
		managed("kubernetes:apps/v1:StatefulSet", "shaide-server", "old", true),
	}, testKubeconfig(), westeurope)

	if len(report.Unverified) != 1 {
		t.Errorf("unverified = %v, want the live provider without a context", report.Unverified)
	}
	if len(report.Mismatched) != 0 {
		t.Errorf("mismatched = %v; a provider pending deletion is not the live target", report.Mismatched)
	}

	want := []string{
		"Namespace app-shaide (on " + westeurope + ")",
		"StatefulSet shaide-server (on " + westeurope + ")",
	}
	if len(report.PendingDeletes) != len(want) {
		t.Fatalf("pending deletes = %v, want %v", report.PendingDeletes, want)
	}
	for i := range want {
		if report.PendingDeletes[i] != want[i] {
			t.Errorf("pending delete %d = %q, want %q", i, report.PendingDeletes[i], want[i])
		}
	}
}

func TestDeploymentResourcesOfNewStack(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		resources, err := deploymentResources(apitype.UntypedDeployment{Deployment: json.RawMessage(raw)})
		if err != nil || len(resources) != 0 {
			t.Errorf("deploymentResources(%q) = %v, %v; want none", raw, resources, err)
		}
	}
}

type scriptedConfirmer struct {
	answer  string
	current string
}

func (c *scriptedConfirmer) Select(_ string, current string, _ []string) (string, error) {
	c.current = current
	return c.answer, nil
}

func TestConfirmDefaultsToAbort(t *testing.T) {
	confirmer := &scriptedConfirmer{answer: confirmAbort}
	d := &Deployer{StackName: "shaide", Target: &ClusterTarget{Context: westeurope}, Confirmer: confirmer}

	err := d.confirm("question")
	if confirmer.current != confirmAbort {
		t.Errorf("pre-selected = %q, want %q", confirmer.current, confirmAbort)
	}
	var targetErr *ClusterTargetError
	if !errors.As(err, &targetErr) {
		t.Fatalf("confirm() error = %v, want a ClusterTargetError on Abort", err)
	}
}

func TestConfirmContinue(t *testing.T) {
	d := &Deployer{StackName: "shaide", Target: &ClusterTarget{Context: westeurope}, Confirmer: &scriptedConfirmer{answer: confirmContinue}}

	if err := d.confirm("question"); err != nil {
		t.Errorf("confirm() error = %v, want nil on Continue", err)
	}
}

// Without a prompt the question cannot be asked, so the safe answer is taken.
func TestConfirmWithoutPromptAborts(t *testing.T) {
	d := &Deployer{StackName: "shaide", Target: &ClusterTarget{Context: westeurope}}

	var targetErr *ClusterTargetError
	if err := d.confirm("question"); !errors.As(err, &targetErr) {
		t.Errorf("confirm() error = %v, want a ClusterTargetError", err)
	}
}
