package kube

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type NodeLabels struct {
	NodeName string
	Labels   map[string]string
}

func GetNodeLabels(ctx context.Context, client kubernetes.Interface, labelSelector string, requiredLabels ...string) ([]NodeLabels, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, v1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("no nodes have the label %s", labelSelector)
	}

	result := make([]NodeLabels, 0, len(nodes.Items))

	for _, node := range nodes.Items {
		nodeLabels := node.GetLabels()

		if missingLabels := missingLabels(nodeLabels, requiredLabels); len(missingLabels) != 0 {
			return nil, fmt.Errorf("node %q is missing labels: %s", node.Name, strings.Join(missingLabels, ", "))
		}

		selectedLabels := make(
			map[string]string,
			len(requiredLabels),
		)

		for _, label := range requiredLabels {
			selectedLabels[label] = nodeLabels[label]
		}

		result = append(
			result,
			NodeLabels{
				NodeName: node.Name,
				Labels:   selectedLabels,
			},
		)
	}

	return result, nil
}

func missingLabels(nodeLabels map[string]string, requiredLabels []string) []string {
	missingLabels := make([]string, 0)
	for _, label := range requiredLabels {
		if nodeLabels[label] == "" {
			missingLabels = append(missingLabels, label)
		}
	}

	return missingLabels
}
