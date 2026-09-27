package gateway

import (
	gatewayconfig "github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/workflow"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Names the installer needs to read the live Gateway and offer its classes.
const (
	SharedGatewayName      = gatewayconfig.SharedGatewayName
	SharedGatewayNamespace = gatewayconfig.DefaultGatewayNamespace

	AGCGatewayClassName   = gatewayconfig.AGCGatewayClassName
	IstioGatewayClassName = gatewayconfig.IstioGatewayClassName
)

// DefaultGatewayClassName is the class a platform most commonly uses. It is
// the suggestion when the cluster gives no better answer.
func DefaultGatewayClassName(platform cluster.Provider) string {
	return gatewayconfig.DefaultGatewayClassName(platform)
}

// Options are values the installer discovers from the cluster.
type Options struct {
	// GatewayClassName is the class picked from those the cluster accepts.
	GatewayClassName string

	// ALBName is the ApplicationLoadBalancer already serving an AGC Gateway.
	ALBName string

	// ALBSubnetID pre-fills the Application Gateway for Containers subnet,
	// typically with the association already on the cluster.
	ALBSubnetID string

	// ALBSubnetPlaceholder hints at the expected subnet ID when there is
	// nothing to pre-fill.
	ALBSubnetPlaceholder string

	// CertManagerIssuer is the ClusterIssuer picked for Gateway TLS. Empty
	// serves plain HTTP.
	CertManagerIssuer string
}

type Stack struct {
	config gatewayconfig.Config
	owned  workflow.OwnedObjects
}

// SetStateResources tells the stack which resources its Pulumi state holds, so
// the Istio ownership sweep can tell this stack's objects from ones another
// Pulumi deployment wrote. Without it the sweep assumes any Pulumi-written
// object is this stack's.
func (s *Stack) SetStateResources(resources []apitype.ResourceV3) {
	s.owned = workflow.OwnedObjectsFromState(resources)
}

func NewStack(projectDir string, options stack.Options, opts Options) *Stack {
	return &Stack{config: gatewayconfig.New(projectDir, options, gatewayconfig.Sources{
		GatewayClassName:     opts.GatewayClassName,
		ALBName:              opts.ALBName,
		ALBSubnetID:          opts.ALBSubnetID,
		ALBSubnetPlaceholder: opts.ALBSubnetPlaceholder,
		CertManagerIssuer:    opts.CertManagerIssuer,
	})}
}

func (s *Stack) Config() stack.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployGatewayProvider(ctx, s.config, s.owned)
}

// Hostname reports the Gateway hostname that was resolved for this deployment.
// Later stages point their HTTPRoutes at the same host.
func (s *Stack) Hostname(values auto.ConfigMap) (string, bool) {
	value, ok := values[s.Config().Definition().Key(gatewayconfig.KeyGatewayHostname)]
	return value.Value, ok
}

var _ stack.Stack = (*Stack)(nil)
