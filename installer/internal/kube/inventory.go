package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// Labels and resources the NVIDIA GPU Operator publishes on GPU nodes.
const (
	gpuResource        corev1.ResourceName = "nvidia.com/gpu"
	gpuProductLabel                        = "nvidia.com/gpu.product"
	gpuMemoryLabel                         = "nvidia.com/gpu.memory"
	gpuMIGCapableLabel                     = "nvidia.com/mig.capable"
	instanceTypeLabel                      = "node.kubernetes.io/instance-type"
	controlPlaneLabel                      = "node-role.kubernetes.io/control-plane"

	// conditionTaintPrefix marks the taints Kubernetes itself sets for a
	// node condition (not ready, cordoned, under pressure). They are
	// transient and say nothing about what the node is for.
	conditionTaintPrefix = "node.kubernetes.io/"
)

// NodeInfo is what the installer shows about a node, so an operator can tell
// nodes apart without guessing.
type NodeInfo struct {
	Name         string
	Labels       map[string]string
	Taints       []corev1.Taint
	InstanceType string
	ControlPlane bool

	CPUMillis   int64
	MemoryBytes int64

	GPUs       int64
	GPUProduct string
	// GPUMemoryMiB is per GPU, as the GPU Operator reports it.
	GPUMemoryMiB int64
	MIGCapable   bool
}

// ListNodes returns every node of the cluster, sorted by name.
func ListNodes(ctx context.Context, client kubernetes.Interface) ([]NodeInfo, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	infos := make([]NodeInfo, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		infos = append(infos, nodeInfo(node))
	}

	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })

	return infos, nil
}

func nodeInfo(node corev1.Node) NodeInfo {
	labels := node.GetLabels()
	allocatable := node.Status.Allocatable

	info := NodeInfo{
		Name:         node.Name,
		Labels:       labels,
		Taints:       node.Spec.Taints,
		InstanceType: labels[instanceTypeLabel],
		GPUProduct:   labels[gpuProductLabel],
		MIGCapable:   labels[gpuMIGCapableLabel] == "true",
	}
	_, info.ControlPlane = labels[controlPlaneLabel]

	if cpu, ok := allocatable[corev1.ResourceCPU]; ok {
		info.CPUMillis = cpu.MilliValue()
	}
	if memory, ok := allocatable[corev1.ResourceMemory]; ok {
		info.MemoryBytes = memory.Value()
	}
	if gpus, ok := allocatable[gpuResource]; ok {
		info.GPUs = gpus.Value()
	}
	if mib, err := strconv.ParseInt(labels[gpuMemoryLabel], 10, 64); err == nil {
		info.GPUMemoryMiB = mib
	}

	return info
}

// ExclusiveTaint returns the first taint that keeps workloads off the node
// unless they tolerate it, ignoring the tolerated keys and the transient
// condition taints. A node with one is reserved for something else, such as
// a control plane or system add-ons.
func (n NodeInfo) ExclusiveTaint(tolerated ...string) (corev1.Taint, bool) {
	for _, taint := range n.Taints {
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		if strings.HasPrefix(taint.Key, conditionTaintPrefix) {
			continue
		}
		if contains(tolerated, taint.Key) {
			continue
		}

		return taint, true
	}

	return corev1.Taint{}, false
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}

	return false
}

// PatchNodeLabels sets and removes node labels in one merge patch. A nil
// value in the patch deletes the label.
func PatchNodeLabels(ctx context.Context, client kubernetes.Interface, node string, set map[string]string, remove []string) error {
	labels := make(map[string]any, len(set)+len(remove))
	for key, value := range set {
		labels[key] = value
	}
	for _, key := range remove {
		labels[key] = nil
	}

	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"labels": labels},
	})
	if err != nil {
		return fmt.Errorf("encode label patch for node %q: %w", node, err)
	}

	if _, err := client.CoreV1().Nodes().Patch(ctx, node, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("label node %q: %w", node, err)
	}

	return nil
}
