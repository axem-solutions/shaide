package serving

import (
	servingconfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options contains values known by the installer but specific to app-serving.
// Empty values are not written, allowing an existing Pulumi stack value to be
// retained.
type Options struct {
	HarborHostname    string
	HarborUser        string
	HarborToken       string
	ModelStorageClass string
	Logf              func(format string, args ...any)
}

type Stack struct {
	config servingconfig.Config
}

func NewStack(projectDir string, common stackpkg.Options, options ...Options) *Stack {
	var servingOptions Options
	if len(options) > 0 {
		servingOptions = options[0]
	}

	return &Stack{config: servingconfig.New(
		projectDir,
		common,
		servingconfig.Sources{
			HarborHostname:    servingOptions.HarborHostname,
			HarborUser:        servingOptions.HarborUser,
			HarborToken:       servingOptions.HarborToken,
			ModelStorageClass: servingOptions.ModelStorageClass,
		},
		servingOptions.Logf,
	)}
}

func (s *Stack) Config() stackpkg.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployAppServing(ctx, s.config)
}

var _ stackpkg.Stack = (*Stack)(nil)
