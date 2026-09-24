package iac

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const kubernetesProviderType = "pulumi:providers:kubernetes"

// ClusterTarget is the cluster the operator selected for this run.
type ClusterTarget struct {
	KubeconfigPath string
	Context        string
}

// Confirmer asks the operator to choose between options.
type Confirmer interface {
	Select(title string, current string, options []string) (string, error)
}

const (
	confirmAbort    = "Abort"
	confirmContinue = "Continue"
)

// ClusterTargetError stops a deployment whose stack state belongs to a cluster
// other than the selected one. Retrying cannot help, so recovery fails fast.
type ClusterTargetError struct {
	Stack    string
	Selected string
	Reason   string
}

func (e *ClusterTargetError) Error() string {
	return fmt.Sprintf("stack %q: %s (selected context %q)", e.Stack, e.Reason, e.Selected)
}

// clusterReport is what the stack state says about where it will act.
type clusterReport struct {
	// Live Kubernetes providers recorded against a different cluster. Running
	// the update would replace every resource onto the selected cluster and
	// delete the originals from the recorded one.
	Mismatched []string

	// Live Kubernetes providers whose cluster cannot be established: recorded
	// without a context (the kubeconfig's current-context at the time), or
	// with a context the kubeconfig no longer knows.
	Unverified []string

	// Resources an earlier, interrupted update left pending deletion. The next
	// update deletes them before anything else.
	PendingDeletes []string
}

// checkClusterTarget refuses to deploy a stack whose state belongs to another
// cluster, and asks before deploying one it cannot verify or that carries
// pending deletions.
//
// Pulumi treats a Kubernetes provider that now points at a different cluster
// as a provider change and replaces every resource: it creates each one on the
// new cluster and deletes the original from the old one. Nothing in that plan
// looks unusual, so it has to be caught before the update starts.
func (d *Deployer) checkClusterTarget(ctx context.Context, stack auto.Stack) error {
	if d.Target == nil || d.Target.Context == "" {
		return nil
	}

	exported, err := stack.Export(ctx)
	if err != nil {
		return fmt.Errorf("export stack %q state: %w", d.StackName, err)
	}

	resources, err := deploymentResources(exported)
	if err != nil {
		return fmt.Errorf("read stack %q state: %w", d.StackName, err)
	}
	if len(resources) == 0 {
		return nil
	}

	kubeconfig, err := clientcmd.LoadFromFile(d.Target.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("load kubeconfig %q: %w", d.Target.KubeconfigPath, err)
	}

	report := inspectClusterTarget(resources, kubeconfig, d.Target.Context)

	if len(report.Mismatched) > 0 {
		for _, provider := range report.Mismatched {
			d.logf("stack %q: %s", d.StackName, provider)
		}
		return &ClusterTargetError{
			Stack:    d.StackName,
			Selected: d.Target.Context,
			Reason: "the stack state belongs to another cluster; deploying would move every resource " +
				"to the selected cluster and delete the originals. Use the state directory of the " +
				"selected cluster, or move this stack's state aside",
		}
	}

	if len(report.Unverified) > 0 {
		for _, provider := range report.Unverified {
			d.logf("stack %q: %s", d.StackName, provider)
		}
		if err := d.confirm(fmt.Sprintf(
			"Stack %s was deployed without a verifiable cluster (see Logs). Deploy it to %s anyway?",
			d.StackName, d.Target.Context,
		)); err != nil {
			return err
		}
	}

	if len(report.PendingDeletes) > 0 {
		d.logf("stack %q has %d resources pending deletion; the update deletes them first:", d.StackName, len(report.PendingDeletes))
		for _, resource := range report.PendingDeletes {
			d.logf("  %s", resource)
		}
		if err := d.confirm(fmt.Sprintf(
			"Stack %s will first delete %d resources left pending by an interrupted update (see Logs). Continue?",
			d.StackName, len(report.PendingDeletes),
		)); err != nil {
			return err
		}
	}

	return nil
}

// confirm defaults to Abort: every question here guards against deleting
// something, so the safe answer is the pre-selected one.
func (d *Deployer) confirm(title string) error {
	if d.Confirmer == nil {
		return &ClusterTargetError{
			Stack:    d.StackName,
			Selected: d.Target.Context,
			Reason:   "the stack state needs confirmation and no prompt is available",
		}
	}

	selected, err := d.Confirmer.Select(title, confirmAbort, []string{confirmAbort, confirmContinue})
	if err != nil {
		return err
	}
	if selected != confirmContinue {
		return &ClusterTargetError{
			Stack:    d.StackName,
			Selected: d.Target.Context,
			Reason:   "deployment aborted by the operator",
		}
	}

	return nil
}

func deploymentResources(exported apitype.UntypedDeployment) ([]apitype.ResourceV3, error) {
	if len(exported.Deployment) == 0 || string(exported.Deployment) == "null" {
		return nil, nil
	}

	var deployment apitype.DeploymentV3
	if err := json.Unmarshal(exported.Deployment, &deployment); err != nil {
		return nil, err
	}

	return deployment.Resources, nil
}

// inspectClusterTarget classifies the Kubernetes providers recorded in a stack
// state against the selected context.
func inspectClusterTarget(
	resources []apitype.ResourceV3,
	kubeconfig *clientcmdapi.Config,
	selected string,
) clusterReport {
	var report clusterReport

	// Provider references are "<urn>::<id>".
	providerContexts := map[string]string{}
	for _, resource := range resources {
		if string(resource.Type) != kubernetesProviderType {
			continue
		}

		recorded := stringInput(resource.Inputs, "context")
		providerContexts[string(resource.URN)+"::"+resource.ID.String()] = recorded

		if resource.Delete {
			continue
		}

		name := resourceName(resource)
		switch sameCluster(kubeconfig, recorded, selected) {
		case clusterDifferent:
			report.Mismatched = append(report.Mismatched, fmt.Sprintf(
				"provider %s targets context %q (%s), not %q (%s)",
				name, recorded, contextServer(kubeconfig, recorded), selected, contextServer(kubeconfig, selected),
			))
		case clusterUnknown:
			if recorded == "" {
				report.Unverified = append(report.Unverified, fmt.Sprintf(
					"provider %s has no recorded context; it targeted the kubeconfig's current-context at the time", name,
				))
			} else {
				report.Unverified = append(report.Unverified, fmt.Sprintf(
					"provider %s targets context %q, which the kubeconfig does not define", name, recorded,
				))
			}
		}
	}

	for _, resource := range resources {
		if !resource.Delete || string(resource.Type) == kubernetesProviderType {
			continue
		}

		target, ok := providerContexts[resource.Provider]
		switch {
		case !ok:
			target = "unknown provider"
		case target == "":
			target = "kubeconfig current-context"
		}
		report.PendingDeletes = append(report.PendingDeletes, fmt.Sprintf(
			"%s %s (on %s)", shortType(string(resource.Type)), resourceName(resource), target,
		))
	}
	sort.Strings(report.PendingDeletes)

	return report
}

type clusterMatch int

const (
	clusterSame clusterMatch = iota
	clusterDifferent
	clusterUnknown
)

// sameCluster compares two contexts by the API server they point at, so a
// renamed context for the same cluster is not reported as a move.
func sameCluster(kubeconfig *clientcmdapi.Config, recorded, selected string) clusterMatch {
	if recorded == "" {
		return clusterUnknown
	}
	if recorded == selected {
		return clusterSame
	}

	recordedServer := contextServer(kubeconfig, recorded)
	selectedServer := contextServer(kubeconfig, selected)
	if recordedServer == "" || selectedServer == "" {
		return clusterUnknown
	}
	if recordedServer == selectedServer {
		return clusterSame
	}

	return clusterDifferent
}

func contextServer(kubeconfig *clientcmdapi.Config, name string) string {
	if kubeconfig == nil {
		return ""
	}
	kubeContext, ok := kubeconfig.Contexts[name]
	if !ok {
		return ""
	}
	cluster, ok := kubeconfig.Clusters[kubeContext.Cluster]
	if !ok {
		return ""
	}

	return cluster.Server
}

func stringInput(inputs map[string]any, key string) string {
	value, ok := inputs[key].(string)
	if !ok {
		return ""
	}

	return value
}

// resourceName is the last URN segment, the resource's logical name.
func resourceName(resource apitype.ResourceV3) string {
	urn := string(resource.URN)
	if i := strings.LastIndex(urn, "::"); i >= 0 {
		return urn[i+2:]
	}

	return urn
}

// shortType turns "kubernetes:apps/v1:Deployment" into "Deployment".
func shortType(token string) string {
	if i := strings.LastIndex(token, ":"); i >= 0 {
		return token[i+1:]
	}

	return token
}
