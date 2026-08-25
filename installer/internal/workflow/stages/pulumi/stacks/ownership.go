package stacks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// chartResource identifies a single Kubernetes resource managed by a Helm chart.
// The GVR is used with the dynamic client (handles CRDs and core types uniformly).
type chartResource struct {
	gvr       schema.GroupVersionResource
	name      string
	namespace string // "" for cluster-scoped
	// forceDestroy: when true, skip the take-ownership patch and always
	// destroy the resource on conflict. Required for CRDs because Pulumi
	// v4 Helm Chart bypasses Helm metadata checks for CRD install (the
	// CRD lifecycle uses helm's pre-install hook path).
	forceDestroy bool
}

// Helm-managed metadata. Adding these to a pre-existing K8s resource causes
// `helm install` to treat the resource as part of the named release (UPDATE
// instead of CREATE), which is how take-ownership works without --take-ownership.
const (
	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"
	helmManagedByLabel             = "app.kubernetes.io/managed-by"
	helmManagedByValue             = "Helm"
)

// ensureChartOwnership implements the installer's four-rule resource policy
// for a Helm chart's known resources:
//  1. Rule 1 (own): if a resource is missing OR already annotated for this
//     release, do nothing - helm install will create or update it.
//  2. Rule 2 (take): if a resource exists with wrong/missing Helm metadata,
//     patch it to point at our release. helm install will then adopt and
//     update it instead of failing with "already exists".
//  3. Rule 3 (destroy): if patching fails for any reason, delete the resource
//     so the subsequent install creates it cleanly.
//  4. Rule 4 (error): if delete also fails, return an error.
//
// Idempotent: re-running over already-owned resources is a no-op.
func ensureChartOwnership(
	rt *core.Runtime,
	releaseName, releaseNamespace string,
	resources []chartResource,
) error {
	dyn, err := dynamic.NewForConfig(rt.Cluster.RESTConfig)
	if err != nil {
		return fmt.Errorf("build dynamic client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, r := range resources {
		if err := ownOrDestroy(ctx, rt, dyn, releaseName, releaseNamespace, r); err != nil {
			return fmt.Errorf("ownership policy failed for %s %s/%s: %w",
				r.gvr.Resource, namespaceOrCluster(r.namespace), r.name, err)
		}
	}
	return nil
}

// ownOrDestroy walks the 4-rule policy for a single resource.
func ownOrDestroy(
	ctx context.Context,
	rt *core.Runtime,
	dyn dynamic.Interface,
	releaseName, releaseNamespace string,
	r chartResource,
) error {
	client := resourceClient(dyn, r)

	existing, err := client.Get(ctx, r.name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		// Rule 1 (no resource = installer will create) - nothing to do.
		return nil
	}
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}

	// Already owned by our release? Skip.
	annotations := existing.GetAnnotations()
	if annotations[helmReleaseNameAnnotation] == releaseName &&
		annotations[helmReleaseNamespaceAnnotation] == releaseNamespace &&
		existing.GetLabels()[helmManagedByLabel] == helmManagedByValue &&
		!r.forceDestroy {
		rt.Detailf("  %s %s: already owned by release %s, skipping",
			r.gvr.Resource, r.name, releaseName)
		return nil
	}

	// Rule 2: try to take ownership by patching metadata. Skipped for
	// resources we know Pulumi v4 won't adopt from metadata (CRDs).
	if !r.forceDestroy {
		if patchErr := patchHelmOwnership(ctx, client, r.name, releaseName, releaseNamespace); patchErr == nil {
			rt.Detailf("  %s %s: ownership taken by release %s",
				r.gvr.Resource, r.name, releaseName)
			return nil
		} else {
			rt.Detailf("  %s %s: take-ownership patch failed (%v); falling back to destroy",
				r.gvr.Resource, r.name, patchErr)
		}
	}

	// Rule 3: destroy and let installer recreate. For CRDs this also
	// deletes any CR instances of that type (Kubernetes garbage collection).
	if deleteErr := client.Delete(ctx, r.name, metav1.DeleteOptions{}); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
		// Rule 4: surface the error - the install will fail without intervention.
		return fmt.Errorf("delete failed: %w", deleteErr)
	}
	if r.forceDestroy {
		rt.Detailf("  %s %s: destroyed (forced; chart will recreate)", r.gvr.Resource, r.name)
	} else {
		rt.Detailf("  %s %s: destroyed (chart will recreate)", r.gvr.Resource, r.name)
	}
	return nil
}

// patchHelmOwnership adds the meta.helm.sh annotations + managed-by label.
// Uses strategic-merge patch via JSON Patch to set fields atomically.
func patchHelmOwnership(
	ctx context.Context,
	client dynamic.ResourceInterface,
	name, releaseName, releaseNamespace string,
) error {
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				helmReleaseNameAnnotation:      releaseName,
				helmReleaseNamespaceAnnotation: releaseNamespace,
			},
			"labels": map[string]string{
				helmManagedByLabel: helmManagedByValue,
			},
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}

	_, err = client.Patch(ctx, name, types.MergePatchType, data, metav1.PatchOptions{})
	return err
}

// resourceClient returns the dynamic ResourceInterface for the given resource,
// scoped to its namespace if namespaced, or cluster-wide if not.
func resourceClient(dyn dynamic.Interface, r chartResource) dynamic.ResourceInterface {
	if r.namespace == "" {
		return dyn.Resource(r.gvr)
	}
	return dyn.Resource(r.gvr).Namespace(r.namespace)
}

func namespaceOrCluster(ns string) string {
	if ns == "" {
		return "<cluster-scoped>"
	}
	return ns
}

// istioBaseChartResources lists resources managed by the upstream istio/base
// Helm chart (stable for Istio 1.28.x). All entries are forceDestroy=true
// because Pulumi v4 Helm Chart does not honor pre-existing Helm metadata for
// adoption - the only reliable path is to delete and let the chart recreate.
func istioBaseChartResources(istioNamespace string) []chartResource {
	return []chartResource{
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"},
			name: "istio-reader-service-account", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
			name: "istio-reader-clusterrole-" + istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
			name: "istio-reader-clusterrole-" + istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingwebhookconfigurations"},
			name: "istiod-default-validator", forceDestroy: true},
		// Istio CRDs - 13 stable types in 1.28.x
		istioCRD("authorizationpolicies.security.istio.io"),
		istioCRD("destinationrules.networking.istio.io"),
		istioCRD("envoyfilters.networking.istio.io"),
		istioCRD("gateways.networking.istio.io"),
		istioCRD("peerauthentications.security.istio.io"),
		istioCRD("proxyconfigs.networking.istio.io"),
		istioCRD("requestauthentications.security.istio.io"),
		istioCRD("serviceentries.networking.istio.io"),
		istioCRD("sidecars.networking.istio.io"),
		istioCRD("telemetries.telemetry.istio.io"),
		istioCRD("virtualservices.networking.istio.io"),
		istioCRD("wasmplugins.extensions.istio.io"),
		istioCRD("workloadentries.networking.istio.io"),
		istioCRD("workloadgroups.networking.istio.io"),
	}
}

// istiodChartResources lists resources managed by the upstream istio/istiod
// Helm chart (stable for Istio 1.28.x). All entries forceDestroy=true; see
// istioBaseChartResources comment for rationale.
func istiodChartResources(istioNamespace string) []chartResource {
	return []chartResource{
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"},
			name: "istiod", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
			name: "istiod", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
			name: "istio", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
			name: "istio-sidecar-injector", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"},
			name: "values", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			name: "istiod", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
			name: "istiod", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"},
			name: "istiod", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
			name: "istiod", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
			name: "istiod", namespace: istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
			name: "istiod-clusterrole-" + istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
			name: "istiod-clusterrole-" + istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
			name: "istiod-gateway-controller-" + istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
			name: "istiod-gateway-controller-" + istioNamespace, forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "mutatingwebhookconfigurations"},
			name: "istio-sidecar-injector", forceDestroy: true},
		{gvr: schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingwebhookconfigurations"},
			name: "istio-validator-istio-system", forceDestroy: true},
	}
}

func istioCRD(name string) chartResource {
	return chartResource{
		gvr:          schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"},
		name:         name,
		forceDestroy: true,
	}
}

// Sentinel error used by the ownership module so callers can distinguish "no
// existing install detected" from "real failure" if they want to.
var errNoExistingChart = errors.New("no existing chart installation detected")
