package serving

import (
	"path"

	servingconfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Model is one model the installer selected to serve, as the model manifest
// describes it. Where it runs and how its weights are laid out is the stack's
// business.
type Model struct {
	// Name is the packaged values directory, under
	// deployments/models/<category>/<name>.
	Name string

	// ID is the Hugging Face repository, e.g. "openai/gpt-oss-20b". The
	// weights land at hub/<ID> inside the model's volume.
	ID string

	// HarborRef is where the weights were mirrored.
	HarborRef string

	StorageSize string

	// StorageClass pins the model's volume to a class. Empty uses
	// Options.ModelStorageClass.
	StorageClass string

	// Placement admits the nodes the model's pods may run on, for example
	// the node pools assigned to it. Empty uses the stack's default GPU pool.
	Placement []PlacementTerm
}

// PlacementTerm admits the nodes whose Key label is one of Values, e.g. a node
// pool label and the pools' names. A model's terms are alternatives: a node
// matching any of them qualifies.
type PlacementTerm struct {
	Key    string
	Values []string
}

// ModelVolume is the PersistentVolumeClaim holding a model's weights.
type ModelVolume struct {
	Model     string
	Namespace string
	Claim     string
}

// Options contains values known by the installer. Empty values are not
// written, allowing an existing Pulumi stack value to be retained.
type Options struct {
	// Models is the selection to serve. Empty leaves the stack's models key
	// untouched.
	Models []Model

	// Images maps an upstream image name, as the image manifest lists it, to
	// the reference the cluster pulls it from. The stack picks the images it
	// deploys and ignores the rest.
	Images map[string]string

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
			Models:            modelsInput(projectDir, servingOptions.Models, servingOptions.Logf),
			HarborHostname:    servingOptions.HarborHostname,
			HarborUser:        servingOptions.HarborUser,
			HarborToken:       servingOptions.HarborToken,
			ModelStorageClass: servingOptions.ModelStorageClass,
			ORASImage:         servingOptions.Images[servingconfig.ORASImageName],
		},
	)}
}

func (s *Stack) Config() stackpkg.Config {
	return s.config.Config
}

func (s *Stack) Deploy(ctx *pulumi.Context) error {
	return deployAppServing(ctx, s.config)
}

var _ stackpkg.Stack = (*Stack)(nil)

// ModelVolumes names the volumes the given models' weights live in, so the
// installer can keep the class of those that already exist: a
// PersistentVolumeClaim's class cannot change. Models the project packages no
// values for are left out, as the stack does not deploy them.
func ModelVolumes(projectDir string, models []Model) ([]ModelVolume, error) {
	var volumes []ModelVolume
	for _, model := range models {
		category, ok := servingconfig.ModelCategory(projectDir, model.Name)
		if !ok {
			continue
		}

		namespace, claim, err := servingconfig.ModelVolume(projectDir, category, model.Name)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, ModelVolume{Model: model.Name, Namespace: namespace, Claim: claim})
	}

	return volumes, nil
}

// placement is the model's own placement, or nil when the stack's default GPU
// pool applies.
func placement(model Model) []servingconfig.PlacementTerm {
	if len(model.Placement) == 0 {
		return nil
	}

	terms := make([]servingconfig.PlacementTerm, 0, len(model.Placement))
	for _, term := range model.Placement {
		terms = append(terms, servingconfig.PlacementTerm{
			Key:    term.Key,
			Values: append([]string(nil), term.Values...),
		})
	}

	return terms
}

// modelsInput places each model by the category its packaged values are in,
// on its own placement or the GPU pool, with its weights under hub/<ID> in its
// volume. A model with no packaged values is dropped rather than guessed at:
// serving it from the wrong values directory would deploy the wrong runtime.
func modelsInput(projectDir string, models []Model, logf func(format string, args ...any)) servingconfig.ModelsInput {
	var input servingconfig.ModelsInput

	for _, model := range models {
		category, ok := servingconfig.ModelCategory(projectDir, model.Name)
		if !ok {
			if logf != nil {
				logf("model %q has no packaged values directory; skipping it for serving", model.Name)
			}
			continue
		}

		entry := servingconfig.ModelInput{
			Name:      model.Name,
			Enabled:   true,
			Placement: placement(model),
			ModelSource: &servingconfig.ModelSourceInput{
				HarborRef:    model.HarborRef,
				ModelUri:     path.Join("hub", model.ID),
				StorageSize:  model.StorageSize,
				StorageClass: model.StorageClass,
			},
		}

		if len(entry.Placement) == 0 {
			entry.NodeSelector = servingconfig.DefaultInferenceNodeSelector()
		}

		switch category {
		case servingconfig.CategoryGenerative:
			input.Generative = append(input.Generative, entry)
		case servingconfig.CategoryEmbedder:
			input.Embedder = append(input.Embedder, entry)
		}
	}

	return input
}
