package monitoring

import (
	monitoringconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Namespace is where the monitoring components are deployed.
const Namespace = monitoringconfig.DefaultNamespace

// PVCs the charts create for the persistent components. Their names follow
// from the release names in this stack (the Loki StatefulSet's "storage"
// claim template, and the Prometheus server's claim). The installer reads
// them to keep an existing volume's StorageClass, which cannot change.
const (
	LokiPVCName       = "storage-loki-0"
	PrometheusPVCName = "prometheus-server"
)

// Options are the values the installer resolves for the stack.
type Options struct {
	// Empty uses the cluster's default StorageClass.
	LokiStorageClass       string
	PrometheusStorageClass string
}

type Stack struct {
	config monitoringconfig.Config
}

func NewStack(projectDir string, options stack.Options, opts Options) *Stack {
	return &Stack{config: monitoringconfig.New(projectDir, options, monitoringconfig.Sources{
		LokiStorageClass:       opts.LokiStorageClass,
		PrometheusStorageClass: opts.PrometheusStorageClass,
	})}
}

func (s *Stack) Config() stack.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployMonitoring(ctx, s.config)
}

var _ stack.Stack = (*Stack)(nil)
