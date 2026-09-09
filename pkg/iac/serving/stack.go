package serving

import (
	servingconfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options contains values known by the installer but specific to app-serving.
// Empty values are not written, allowing an existing Pulumi stack value to be
// retained.
// Model is one model the installer selected to serve. Category picks the
// values directory the stack loads, under deployments/models/<category>/<name>.
type Model struct {
	Name         string
	Category     string
	NodeSelector map[string]string

	HarborRef    string
	ModelURI     string
	StorageSize  string
	StorageClass string
}

const (
	CategoryGenerative = "generative"
	CategoryEmbedder   = "embedder"
)

type Options struct {
	// Models is the selection to serve. Empty leaves the stack's models key
	// untouched.
	Models []Model

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
			Models:            modelsInput(servingOptions.Models),
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

// modelsInput splits the selection into the two categories the stack expects.
// An unrecognised category is dropped rather than guessed at: serving a model
// from the wrong values directory would deploy the wrong runtime.
func modelsInput(models []Model) servingconfig.ModelsInput {
	var input servingconfig.ModelsInput

	for _, model := range models {
		entry := servingconfig.ModelInput{
			Name:         model.Name,
			Enabled:      true,
			NodeSelector: model.NodeSelector,
			ModelSource: &servingconfig.ModelSourceInput{
				HarborRef:    model.HarborRef,
				ModelUri:     model.ModelURI,
				StorageSize:  model.StorageSize,
				StorageClass: model.StorageClass,
			},
		}

		switch model.Category {
		case CategoryGenerative:
			input.Generative = append(input.Generative, entry)
		case CategoryEmbedder:
			input.Embedder = append(input.Embedder, entry)
		}
	}

	return input
}
