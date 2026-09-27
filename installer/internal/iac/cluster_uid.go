package iac

import (
	"context"
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

// maxUIDLookups bounds how many recorded objects are read from the cluster.
// One matching UID settles the question, so the bound only matters when none
// match.
const maxUIDLookups = 20

// ObjectLookup reads the UID of a live object on the selected cluster. found
// is false when the object does not exist there.
type ObjectLookup func(ctx context.Context, object recordedObject) (uid string, found bool, err error)

// recordedObject is a Kubernetes object as the stack state recorded it.
type recordedObject struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	UID        string
}

func (o recordedObject) String() string {
	if o.Namespace == "" {
		return o.Kind + " " + o.Name
	}
	return o.Kind + " " + o.Namespace + "/" + o.Name
}

type uidVerdict int

const (
	// uidSameCluster: a recorded object is live on the selected cluster with
	// the same UID. UIDs are unique across clusters, so one match is proof.
	uidSameCluster uidVerdict = iota

	// uidOtherCluster: no recorded object matched, and some exist on the
	// selected cluster under a different UID. The same names were created
	// independently there, which is what another cluster's state looks like.
	uidOtherCluster

	// uidUndecided: nothing matched and nothing contradicted the state; the
	// objects are missing, unreadable, or the provider recorded none.
	uidUndecided
)

// providerObjects returns the live objects a provider created, as recorded in
// the state, for those whose UID was recorded.
func providerObjects(resources []apitype.ResourceV3, providerKey string) []recordedObject {
	var objects []recordedObject
	for _, resource := range resources {
		if resource.Delete || resource.Provider != providerKey {
			continue
		}

		metadata, _ := resource.Outputs["metadata"].(map[string]any)
		object := recordedObject{
			APIVersion: stringInput(resource.Outputs, "apiVersion"),
			Kind:       stringInput(resource.Outputs, "kind"),
			Namespace:  stringInput(metadata, "namespace"),
			Name:       stringInput(metadata, "name"),
			UID:        stringInput(metadata, "uid"),
		}
		if object.APIVersion == "" || object.Kind == "" || object.Name == "" || object.UID == "" {
			continue
		}
		objects = append(objects, object)
	}

	return objects
}

// verifyByUID compares recorded UIDs with the live objects until one matches.
// detail explains the verdict for the log.
func verifyByUID(ctx context.Context, lookup ObjectLookup, objects []recordedObject) (uidVerdict, string) {
	if len(objects) > maxUIDLookups {
		objects = objects[:maxUIDLookups]
	}

	var different, missing, unreadable []string
	for _, object := range objects {
		uid, found, err := lookup(ctx, object)
		switch {
		case err != nil:
			unreadable = append(unreadable, fmt.Sprintf("%s (%v)", object, err))
		case !found:
			missing = append(missing, object.String())
		case uid == object.UID:
			return uidSameCluster, fmt.Sprintf("%s has the recorded UID %s", object, uid)
		default:
			different = append(different, fmt.Sprintf("%s has UID %s, recorded %s", object, uid, object.UID))
		}
	}

	if len(different) > 0 {
		return uidOtherCluster, "no recorded object matched; " + strings.Join(different, "; ")
	}
	if len(objects) == 0 {
		return uidUndecided, "the state records no object UIDs to compare"
	}

	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}
	if len(unreadable) > 0 {
		parts = append(parts, "unreadable: "+strings.Join(unreadable, ", "))
	}
	return uidUndecided, "no recorded object could be compared; " + strings.Join(parts, "; ")
}

// clusterObjectLookup reads objects through the selected context, resolving
// each kind to its resource through discovery so custom resources work too.
func clusterObjectLookup(target ClusterTarget) (ObjectLookup, error) {
	restConfig, err := connection.BuildRestConfig(connection.Connection{
		KubeconfigPath: target.KubeconfigPath,
		Context:        target.Context,
	})
	if err != nil {
		return nil, fmt.Errorf("build Kubernetes client config: %w", err)
	}

	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	disco, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("build discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco))

	return func(ctx context.Context, object recordedObject) (string, bool, error) {
		gv, err := schema.ParseGroupVersion(object.APIVersion)
		if err != nil {
			return "", false, err
		}

		mapping, err := mapper.RESTMapping(gv.WithKind(object.Kind).GroupKind(), gv.Version)
		if err != nil {
			if meta.IsNoMatchError(err) {
				// The kind is not served, so no such object exists here.
				return "", false, nil
			}
			return "", false, err
		}

		resource := dyn.Resource(mapping.Resource)
		var live *unstructured.Unstructured
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			live, err = resource.Namespace(object.Namespace).Get(ctx, object.Name, metav1.GetOptions{})
		} else {
			live, err = resource.Get(ctx, object.Name, metav1.GetOptions{})
		}
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}

		return string(live.GetUID()), true, nil
	}, nil
}
