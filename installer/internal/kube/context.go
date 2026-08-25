package kube

import (
	"fmt"
	"sort"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ContextOptions struct {
	Context   string
	Available []string
}

func LoadRawConfig(kubeconfigPath string) (*clientcmdapi.Config, error) {
	cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
	}

	return cfg, nil
}

func LoadContextOptions(kubeconfigPath string) (ContextOptions, error) {
	cfg, err := LoadRawConfig(kubeconfigPath)
	if err != nil {
		return ContextOptions{}, err
	}

	if len(cfg.Contexts) == 0 {
		return ContextOptions{}, fmt.Errorf("no contexts found in kubeconfig %q", kubeconfigPath)
	}

	available := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		available = append(available, name)
	}
	sort.Strings(available)

	current := cfg.CurrentContext
	if current == "" {
		current = available[0]
	}

	return ContextOptions{
		Context:   current,
		Available: available,
	}, nil
}
