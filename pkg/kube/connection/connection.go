package connection

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultQPS   = 20
	defaultBurst = 40

	// defaultTimeout is a backstop against a hung connection, not a budget for
	// individual operations. rest.Config.Timeout applies to every request, so it
	// must stay above the longest per-call context in the installer for those
	// contexts to remain authoritative. Deleting a CRD that still has custom
	// resources cascades through garbage collection and can take minutes.
	defaultTimeout = 5 * time.Minute
)

type Connection struct {
	KubeconfigPath string
	Context        string
}

func BuildRestConfig(connection Connection) (*rest.Config, error) {
	// Starting from the real default loading rules (not a bare zero-value
	// struct) matters when KubeconfigPath is empty: the zero value has no
	// search path at all and fails outright, where the default rules fall
	// back to $KUBECONFIG then ~/.kube/config — the same resolution the
	// pulumi-kubernetes provider itself uses when kubeconfig is omitted.
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	if connection.KubeconfigPath != "" {
		loadingRules.ExplicitPath = connection.KubeconfigPath
	}

	overrides := &clientcmd.ConfigOverrides{
		CurrentContext: connection.Context,
	}

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		overrides,
	)

	cfg, err := cc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	cfg.Timeout = defaultTimeout
	cfg.QPS = defaultQPS
	cfg.Burst = defaultBurst

	return cfg, nil
}

func NewK8sClient(connection Connection) (*kubernetes.Clientset, *rest.Config, error) {
	cfg, err := BuildRestConfig(connection)
	if err != nil {
		return nil, nil, err
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create clientset: %w", err)
	}

	return cs, cfg, nil
}
