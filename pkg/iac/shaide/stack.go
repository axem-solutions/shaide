package shaide

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options contains values known by the installer. Empty values are not
// written, allowing an existing Pulumi stack value to be retained, except nil
// feature flags which Resolve writes as false.
type Options struct {
	GatewayHostname string

	// WebAppEnabled says whether the webapp is installed. The control panel
	// learns the answer through its APP_ENABLED env var. Nil resolves to false.
	WebAppEnabled *bool

	// Placement names the nodes the components prefer (soft node affinity),
	// for example the node pools assigned to CPU work. A node matching any
	// term is preferred.
	Placement []PlacementTerm

	// Images maps an upstream image name, as the image manifest lists it
	// (e.g. "qdrant/qdrant"), to the reference the cluster pulls it from. The
	// stack picks the images it deploys and ignores the rest.
	Images map[string]string

	// KnowledgeCenterEnabled is the operator's single answer to whether the
	// Knowledge Center is installed. Pass the same answer to mcp.Options,
	// which runs KC-MCP. The control panel learns it through
	// KNOWLEDGE_CENTER_ENABLED. Nil resolves to false; pass the operator's
	// answer explicitly.
	KnowledgeCenterEnabled *bool

	// KnowledgeCenter are the credentials the installer generated once and
	// keeps; pass the same value to mcp.Options. Read only when the
	// Knowledge Center is installed.
	KnowledgeCenter knowledgecenter.Credentials
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
			WebAppEnabled:   shaideOptions.WebAppEnabled,
			Placement:       placement(shaideOptions.Placement),
			Images:          shaideOptions.Images,

			KnowledgeCenterEnabled: shaideOptions.KnowledgeCenterEnabled,
			KnowledgeCenter:        shaideOptions.KnowledgeCenter,
		},
	)}
}

// PlacementTerm admits the nodes whose Key label is one of Values, e.g. a node
// pool label and the pools' names.
type PlacementTerm struct {
	Key    string
	Values []string
}

func placement(terms []PlacementTerm) []appconfig.PlacementTerm {
	if len(terms) == 0 {
		return nil
	}

	out := make([]appconfig.PlacementTerm, 0, len(terms))
	for _, term := range terms {
		out = append(out, appconfig.PlacementTerm{Key: term.Key, Values: append([]string(nil), term.Values...)})
	}

	return out
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

// WebAppName names the webapp Deployment, so the installer can tell from the
// cluster whether an earlier install included it.
const WebAppName = appconfig.DefaultWebappService

// WebAppImage is the catalog name of the webapp image, so the installer can
// leave it out of the Harbor upload when the webapp is not installed.
const WebAppImage = appconfig.WebappImageName

// KnowledgeCenterImage is the catalog name of the KC image, so the installer
// can leave it out of the Harbor upload when KC is not installed.
const KnowledgeCenterImage = knowledgecenter.ImageName

// KnowledgeCenterName names the KC Deployment, so the installer can tell from
// the cluster whether an earlier install included it.
const KnowledgeCenterName = knowledgecenter.ServiceName
