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

	// NodeSelector names the node labels the model's pods require, for
	// example its own pool's. Empty uses the stack's default GPU pool.
	NodeSelector map[string]string
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

// modelsInput places each model by the category its packaged values are in,
// on the GPU pool, with its weights under hub/<ID> in its volume. A model with
// no packaged values is dropped rather than guessed at: serving it from the
// wrong values directory would deploy the wrong runtime.
// nodeSelector is the model's own selector, or the default GPU pool's.
func nodeSelector(model Model) map[string]string {
	if len(model.NodeSelector) == 0 {
		return servingconfig.DefaultInferenceNodeSelector()
	}

	selector := make(map[string]string, len(model.NodeSelector))
	for key, value := range model.NodeSelector {
		selector[key] = value
	}

	return selector
}

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
			Name:         model.Name,
			Enabled:      true,
			NodeSelector: nodeSelector(model),
			ModelSource: &servingconfig.ModelSourceInput{
				HarborRef:    model.HarborRef,
				ModelUri:     path.Join("hub", model.ID),
				StorageSize:  model.StorageSize,
				StorageClass: model.StorageClass,
			},
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
