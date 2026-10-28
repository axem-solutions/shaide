package mcp

import (
	"encoding/json"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options contains values known by the installer but specific to app-mcp.
// Empty values are not written, allowing an existing Pulumi stack value to be
// retained, except nil feature flags which Resolve writes as false.
//
// Datasources is the set of generic MCP servers to run. Nothing supplies it
// yet: the intended flow is for an operator to publish an MCP server image
// into Harbor and pick from what is published, so the list will come from the
// installer rather than from a prompt. With the Knowledge Center it may be
// empty.
type Options struct {
	Datasources      []appconfig.Datasource
	ImagePullSecrets []string
	ShaideNamespace  string
	ShaideSAName     string

	// KnowledgeCenterEnabled is the operator's single Knowledge Center
	// answer; pass the same value to shaide.Options. It runs KC-MCP here.
	// Nil resolves to false; pass the operator's answer explicitly.
	KnowledgeCenterEnabled *bool

	// KnowledgeCenter are the installer's credentials, the same value given
	// to shaide.Options. Only GatewayToken, ScopedToken and
	// ContextPublicKey are read.
	KnowledgeCenter knowledgecenter.Credentials

	// Images maps an upstream image name to the reference the cluster pulls
	// it from; only KnowledgeCenterMCPImage is read.
	Images map[string]string

	// Placement names the nodes KC-MCP prefers (soft node affinity), e.g.
	// the node pools assigned to CPU work.
	Placement []PlacementTerm
}

// PlacementTerm admits the nodes whose Key label is one of Values.
type PlacementTerm struct {
	Key    string
	Values []string
}

type Stack struct {
	config appconfig.Config
}

func NewStack(projectDir string, common stackpkg.Options, options ...Options) *Stack {
	var mcpOptions Options
	if len(options) > 0 {
		mcpOptions = options[0]
	}

	return &Stack{config: appconfig.New(
		projectDir,
		common,
		appconfig.Sources{
			Datasources:      mcpOptions.Datasources,
			ImagePullSecrets: mcpOptions.ImagePullSecrets,
			ShaideNamespace:  mcpOptions.ShaideNamespace,
			ShaideSAName:     mcpOptions.ShaideSAName,

			KnowledgeCenterEnabled: mcpOptions.KnowledgeCenterEnabled,
			KnowledgeCenter:        mcpOptions.KnowledgeCenter,
			Images:                 mcpOptions.Images,
			Placement:              placement(mcpOptions.Placement),
		},
	)}
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
	return deployAppMCP(ctx, s.config)
}

// HasDatasources reports whether values, e.g. the config an earlier run
// stored in the stack, configure any generic MCP server besides KC-MCP.
func (s *Stack) HasDatasources(values auto.ConfigMap) bool {
	value, ok := values[s.Config().Definition().Key(appconfig.KeyDatasources)]
	if !ok {
		return false
	}

	var datasources []json.RawMessage
	if err := json.Unmarshal([]byte(value.Value), &datasources); err != nil {
		// Unreadable is not proof of none.
		return true
	}
	return len(datasources) > 0
}

var _ stackpkg.Stack = (*Stack)(nil)

// Namespace is where app-mcp runs MCP servers, KC-MCP included, unless a
// stack pinned another one.
const Namespace = appconfig.DefaultNamespace

// KnowledgeCenterMCPImage is the catalog name of the KC-MCP image, so the
// installer can leave it out of the Harbor upload when KC is not installed.
const KnowledgeCenterMCPImage = knowledgecenter.MCPImageName

// KnowledgeCenterMCPName names the KC-MCP Deployment.
const KnowledgeCenterMCPName = knowledgecenter.MCPServiceName
