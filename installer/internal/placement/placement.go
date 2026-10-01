// Package placement decides where shaide's workloads run: on the node pools
// the operator assigns to each model and to CPU work.
//
// A pool is identified by a label the platform maintains itself, so workloads
// are matched on it rather than on labels the installer would have to write:
// a node a pool replaces or adds carries it from the start, and the cluster
// autoscaler knows it from the pool definition.
package placement

import (
	"sort"
	"strings"
)

// PoolKeys are the labels that name a node's pool, in the order they are
// looked for: the managed node pools of AKS, EKS and GKE, then the
// nodegroup label the on-prem inventory sets.
var PoolKeys = []string{
	"kubernetes.azure.com/agentpool",
	"eks.amazonaws.com/nodegroup",
	"cloud.google.com/gke-nodepool",
	"nodegroup",
}

// HostnameKey names a node without any pool label: it is a pool of its own.
const HostnameKey = "kubernetes.io/hostname"

// GPUTaintKey is the taint GPU nodes may carry to keep other workloads off
// them. The serving stack's model workloads tolerate it, so a pool tainted
// with it can still serve a model.
const GPUTaintKey = "nvidia.com/gpu"

// Pool identifies a node pool by the label naming it and its value.
type Pool struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// PoolOf returns the pool a node belongs to.
func PoolOf(nodeName string, labels map[string]string) Pool {
	for _, key := range PoolKeys {
		if name := labels[key]; name != "" {
			return Pool{Key: key, Name: name}
		}
	}

	if hostname := labels[HostnameKey]; hostname != "" {
		return Pool{Key: HostnameKey, Name: hostname}
	}

	return Pool{Key: HostnameKey, Name: nodeName}
}

// Kind names what the pool is, for the operator.
func (p Pool) Kind() string {
	switch p.Key {
	case "kubernetes.azure.com/agentpool":
		return "AKS node pool"
	case "eks.amazonaws.com/nodegroup":
		return "EKS node group"
	case "cloud.google.com/gke-nodepool":
		return "GKE node pool"
	case "nodegroup":
		return "node group"
	default:
		return "single node"
	}
}

func (p Pool) String() string {
	return p.Key + "=" + p.Name
}

// Term admits the nodes whose Key label is one of Values.
type Term struct {
	Key    string
	Values []string
}

// Terms turns pools into placement terms, one per pool label, with the pool
// names sorted so the stacks see a stable placement. A node matching any term
// qualifies.
func Terms(pools []Pool) []Term {
	names := map[string][]string{}
	for _, pool := range pools {
		names[pool.Key] = append(names[pool.Key], pool.Name)
	}

	keys := make([]string, 0, len(names))
	for key := range names {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	terms := make([]Term, 0, len(keys))
	for _, key := range keys {
		values := names[key]
		sort.Strings(values)
		terms = append(terms, Term{Key: key, Values: values})
	}

	return terms
}

// Assignment is the operator's choice of pools: per model slug, and for CPU
// work.
type Assignment struct {
	Models map[string][]Pool `json:"models"`
	CPU    []Pool            `json:"cpu"`
}

// Legacy node labels written by earlier installers to place workloads. They
// are no longer read for placement, and are removed from the nodes the
// installer lists.
const (
	legacyWorkloadPrefix = "axem.dev/workload-"
	legacyModelPrefix    = "axem.dev/model-"
)

// IsLegacyLabel reports whether a node label is one an earlier installer
// wrote to place workloads.
func IsLegacyLabel(key string) bool {
	return strings.HasPrefix(key, legacyWorkloadPrefix) || strings.HasPrefix(key, legacyModelPrefix)
}

// LegacyModelLabel is the label an earlier installer put on a model's nodes.
func LegacyModelLabel(slug string) string {
	return legacyModelPrefix + slug
}

// LegacyCPULabel is the label an earlier installer put on CPU nodes.
const LegacyCPULabel = legacyWorkloadPrefix + "cpu"
