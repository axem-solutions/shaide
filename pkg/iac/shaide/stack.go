package shaide

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options contains values known by the installer. Empty values are not
// written, allowing an existing Pulumi stack value to be retained.
type Options struct {
	GatewayHostname string

	// Images maps an upstream image name, as the image manifest lists it
	// (e.g. "qdrant/qdrant"), to the reference the cluster pulls it from. The
	// stack picks the images it deploys and ignores the rest.
	Images map[string]string
}

type Stack struct {
	config appconfig.Config
}

func NewStack(projectDir string, common stackpkg.Options, options ...Options) *Stack {
	var shaideOptions Options
	if len(options) > 0 {
		shaideOptions = options[0]
	}

	return &Stack{config: appconfig.New(
		projectDir,
		common,
		appconfig.Sources{
			GatewayHostname: shaideOptions.GatewayHostname,
			Images:          shaideOptions.Images,
		},
	)}
}

func (s *Stack) Config() stackpkg.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployAppShaide(ctx, s.config)
}

var _ stackpkg.Stack = (*Stack)(nil)

// Namespace and ServiceAccountName are the app-shaide values other stacks must
// agree with. The MCP RBAC Role is bound to this ServiceAccount in this
// namespace, and a mismatch surfaces as a 403 on the Kubernetes watch rather
// than as a deployment failure.
const (
	Namespace          = appconfig.DefaultNamespace
	ServiceAccountName = appconfig.DefaultServiceAccount
)
