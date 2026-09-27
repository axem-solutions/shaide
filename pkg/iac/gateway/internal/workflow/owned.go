package workflow

import (
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ObjectKey identifies a Kubernetes object independently of API version.
type ObjectKey struct {
	Group     string
	Kind      string
	Namespace string // "" for cluster-scoped objects
	Name      string
}

// OwnedObjects is the set of Kubernetes objects the current stack's state
// manages. A nil set means the state is unknown (the program runs outside the
// installer), in which case the sweep keeps its earlier assumption that any
// Pulumi-written object belongs to this stack.
type OwnedObjects map[ObjectKey]struct{}

func objectKey(object *unstructured.Unstructured) ObjectKey {
	gvk := object.GroupVersionKind()
	return ObjectKey{
		Group:     gvk.Group,
		Kind:      gvk.Kind,
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

// ownedByThisStack reports whether a Pulumi-written object belongs to the
// current stack. Only then may the sweep leave it alone: Pulumi will update it.
// An object another Pulumi deployment wrote is foreign to this stack, and the
// chart's create would fail with "already exists".
func (owned OwnedObjects) ownedByThisStack(object *unstructured.Unstructured) bool {
	if owned == nil {
		return true
	}
	_, ok := owned[objectKey(object)]
	return ok
}

// OwnedObjectsFromState extracts the Kubernetes objects a stack's state
// manages. A Kubernetes resource's type token names its group and kind
// ("kubernetes:apps/v1:Deployment", "kubernetes:core/v1:ServiceAccount"), and
// its ID is "namespace/name", or just "name" when cluster-scoped. Resources
// pending deletion are not owned any more, and a non-nil empty set means the
// stack owns nothing, which is what a fresh state says.
func OwnedObjectsFromState(resources []apitype.ResourceV3) OwnedObjects {
	owned := OwnedObjects{}
	for _, resource := range resources {
		if resource.Delete {
			continue
		}
		key, ok := stateObjectKey(string(resource.Type), resource.ID.String())
		if !ok {
			continue
		}
		owned[key] = struct{}{}
	}
	return owned
}

func stateObjectKey(typeToken, id string) (ObjectKey, bool) {
	parts := strings.Split(typeToken, ":")
	if len(parts) != 3 || parts[0] != "kubernetes" || id == "" {
		return ObjectKey{}, false
	}

	// "apps/v1" → "apps"; "core/v1" is the core group, which Kubernetes names "".
	group, _, _ := strings.Cut(parts[1], "/")
	if group == "core" {
		group = ""
	}
	// Components such as helm.sh/v4:Chart and kustomize are not objects.
	if group == "helm.sh" || strings.HasPrefix(parts[1], "kustomize") || strings.HasPrefix(parts[1], "yaml") {
		return ObjectKey{}, false
	}

	namespace, name, namespaced := strings.Cut(id, "/")
	if !namespaced {
		namespace, name = "", id
	}
	return ObjectKey{Group: group, Kind: parts[2], Namespace: namespace, Name: name}, true
}
