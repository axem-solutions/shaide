package mcp

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options contains values known by the installer but specific to app-mcp.
// Empty values are not written, allowing an existing Pulumi stack value to be
// retained.
//
// Datasources is the set of MCP servers to run. Nothing supplies it yet: the
// intended flow is for an operator to publish an MCP server image into Harbor
// and pick from what is published, so the list will come from the installer
// rather than from a prompt.
type Options struct {
	Datasources      []appconfig.Datasource
	ImagePullSecrets []string
	ShaideNamespace  string
	ShaideSAName     string
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
		},
	)}
}

func (s *Stack) Config() stackpkg.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployAppMCP(ctx, s.config)
}

var _ stackpkg.Stack = (*Stack)(nil)
