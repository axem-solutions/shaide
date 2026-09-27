package iac

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// object is a managed resource as the state records it, with its live
// identity in the outputs.
func object(apiVersion, kind, namespace, name, uid, providerID string) apitype.ResourceV3 {
	metadata := map[string]any{"name": name, "uid": uid}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	return apitype.ResourceV3{
		URN:      resource.URN("urn:pulumi:shaide::app-shaide::kubernetes:" + kind + "::" + name),
		Type:     tokens.Type("kubernetes:" + apiVersion + ":" + kind),
		Provider: providerURN + "::" + providerID,
		Outputs: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   metadata,
		},
	}
}

// fakeCluster answers lookups from the objects it holds, keyed by String().
type fakeCluster struct {
	uids    map[string]string
	failing map[string]bool
	looked  []string
}

func (c *fakeCluster) lookup(_ context.Context, object recordedObject) (string, bool, error) {
	c.looked = append(c.looked, object.String())
	if c.failing[object.String()] {
		return "", false, errors.New("forbidden")
	}
	uid, ok := c.uids[object.String()]
	return uid, ok, nil
}

func TestProviderObjectsSelectsTheProvidersLiveObjects(t *testing.T) {
	deleted := object("v1", "ConfigMap", "app-shaide", "old", "u-old", "p1")
	deleted.Delete = true
	noUID := object("v1", "ConfigMap", "app-shaide", "no-uid", "", "p1")

	got := providerObjects([]apitype.ResourceV3{
		provider("p1", "", false),
		object("v1", "Namespace", "", "app-shaide", "u-ns", "p1"),
		object("v1", "Service", "app-shaide", "shaide-server", "u-svc", "p2"),
		deleted,
		noUID,
	}, providerURN+"::p1")

	if len(got) != 1 || got[0].String() != "Namespace app-shaide" || got[0].UID != "u-ns" {
		t.Errorf("objects = %+v, want only the live, UID-bearing object of p1", got)
	}
}

func TestVerifyByUID(t *testing.T) {
	ns := recordedObject{APIVersion: "v1", Kind: "Namespace", Name: "app-shaide", UID: "u-ns"}
	svc := recordedObject{APIVersion: "v1", Kind: "Service", Namespace: "app-shaide", Name: "shaide-server", UID: "u-svc"}

	tests := []struct {
		name    string
		cluster *fakeCluster
		objects []recordedObject
		want    uidVerdict
	}{
		{
			name:    "a matching UID proves the cluster",
			cluster: &fakeCluster{uids: map[string]string{ns.String(): "u-ns"}},
			objects: []recordedObject{ns, svc},
			want:    uidSameCluster,
		},
		{
			// An object recreated by hand on the same cluster has a new UID;
			// another object's match still proves the cluster.
			name:    "one recreated object does not outweigh a match",
			cluster: &fakeCluster{uids: map[string]string{ns.String(): "u-recreated", svc.String(): "u-svc"}},
			objects: []recordedObject{ns, svc},
			want:    uidSameCluster,
		},
		{
			// The westeurope incident: the same names exist on the other
			// cluster, created there independently.
			name:    "only different UIDs is another cluster",
			cluster: &fakeCluster{uids: map[string]string{ns.String(): "u-other"}},
			objects: []recordedObject{ns, svc},
			want:    uidOtherCluster,
		},
		{
			name:    "missing objects decide nothing",
			cluster: &fakeCluster{},
			objects: []recordedObject{ns, svc},
			want:    uidUndecided,
		},
		{
			name:    "unreadable objects decide nothing",
			cluster: &fakeCluster{failing: map[string]bool{ns.String(): true}},
			objects: []recordedObject{ns},
			want:    uidUndecided,
		},
		{
			name:    "no recorded UIDs decide nothing",
			cluster: &fakeCluster{},
			want:    uidUndecided,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, detail := verifyByUID(context.Background(), test.cluster.lookup, test.objects)
			if got != test.want {
				t.Errorf("verdict = %d (%s), want %d", got, detail, test.want)
			}
		})
	}
}

// The first match settles it, so a large stack is not read object by object.
func TestVerifyByUIDStopsAtTheFirstMatch(t *testing.T) {
	objects := []recordedObject{
		{APIVersion: "v1", Kind: "Namespace", Name: "a", UID: "u-a"},
		{APIVersion: "v1", Kind: "Namespace", Name: "b", UID: "u-b"},
	}
	cluster := &fakeCluster{uids: map[string]string{"Namespace a": "u-a", "Namespace b": "u-b"}}

	verifyByUID(context.Background(), cluster.lookup, objects)

	if len(cluster.looked) != 1 {
		t.Errorf("looked up %v, want only the first object", cluster.looked)
	}
}

// dev-polandcentral: app-shaide deployed before the context was recorded, and
// its objects are live on the selected cluster with the recorded UIDs. The
// stack is verified without asking.
func TestUnverifiedProviderIsSettledByMatchingUIDs(t *testing.T) {
	resources := []apitype.ResourceV3{
		provider("p1", "", false),
		object("v1", "Namespace", "", "app-shaide", "u-ns", "p1"),
	}
	report := inspectClusterTarget(resources, testKubeconfig(), polandcentral)

	var logged strings.Builder
	d := &Deployer{
		StackName: "shaide",
		Target:    &ClusterTarget{Context: polandcentral},
		Logger:    &logged,
		lookup:    (&fakeCluster{uids: map[string]string{"Namespace app-shaide": "u-ns"}}).lookup,
	}

	remaining, err := d.verifyByObjectUID(context.Background(), resources, report.Unverified)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("remaining = %v, err = %v, want the provider verified", remaining, err)
	}
	if !strings.Contains(logged.String(), "verified on") {
		t.Errorf("log = %q, want the verification recorded", logged.String())
	}
}

func TestUnverifiedProviderWithOnlyDifferentUIDsIsRefused(t *testing.T) {
	resources := []apitype.ResourceV3{
		provider("p1", "", false),
		object("v1", "Namespace", "", "app-shaide", "u-westeurope", "p1"),
	}
	report := inspectClusterTarget(resources, testKubeconfig(), polandcentral)

	d := &Deployer{
		StackName: "shaide",
		Target:    &ClusterTarget{Context: polandcentral},
		Logger:    &strings.Builder{},
		lookup:    (&fakeCluster{uids: map[string]string{"Namespace app-shaide": "u-polandcentral"}}).lookup,
	}

	_, err := d.verifyByObjectUID(context.Background(), resources, report.Unverified)
	var targetErr *ClusterTargetError
	if !errors.As(err, &targetErr) {
		t.Fatalf("err = %v, want a ClusterTargetError", err)
	}
}

// Without evidence either way the operator is still asked, now with the
// reason in the log.
func TestUnverifiedProviderWithMissingObjectsStillAsks(t *testing.T) {
	resources := []apitype.ResourceV3{
		provider("p1", "", false),
		object("v1", "Namespace", "", "app-shaide", "u-ns", "p1"),
	}
	report := inspectClusterTarget(resources, testKubeconfig(), polandcentral)

	d := &Deployer{
		StackName: "shaide",
		Target:    &ClusterTarget{Context: polandcentral},
		Logger:    &strings.Builder{},
		lookup:    (&fakeCluster{}).lookup,
	}

	remaining, err := d.verifyByObjectUID(context.Background(), resources, report.Unverified)
	if err != nil || len(remaining) != 1 {
		t.Fatalf("remaining = %v, err = %v, want the provider left to confirm", remaining, err)
	}
	if !strings.Contains(remaining[0].reason, "missing: Namespace app-shaide") {
		t.Errorf("reason = %q, want the missing object named", remaining[0].reason)
	}
}
