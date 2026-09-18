package catalog

import (
	"fmt"
	"os"
	"strings"
)

// Catalog is the installer's inventory of what to install: the container
// images to copy into Harbor, and the models to publish as OCI artifacts.
//
// It replaces the bundle's manifests. The image manifest ships inside the
// installer image; the model manifest is supplied at runtime under the storage
// mount, so adding a model does not require an image rebuild.
type Catalog struct {
	ImagesDir string

	ServiceImages []Image

	HarborImages []Image

	Models []Model
}

type LoadOptions struct {
	ImageManifestPath string
	ModelManifestPath string
	ImagesDir         string
}

func (opts LoadOptions) Validate() error {
	if opts.ImageManifestPath == "" {
		return fmt.Errorf("image manifest path is required")
	}

	if opts.ModelManifestPath == "" {
		return fmt.Errorf("model manifest path is required")
	}

	if opts.ImagesDir == "" {
		return fmt.Errorf("images directory is required")
	}

	return nil
}

// checkInputs fails on missing manifests at bootstrap, with a message that says
// what to do about it. Without this the model manifest surfaces as a bare
// "no such file" from readManifest, deep inside the run.
//
// ImagesDir is deliberately not checked here: it is only read for manifest
// entries whose source is "archive", and resolveImageSizes already reports a
// missing archive per image. Requiring the directory would fail every install
// that pulls all of its images from registries — which is the normal case.
func (opts LoadOptions) checkInputs() error {
	if _, err := os.Stat(opts.ImageManifestPath); err != nil {
		return fmt.Errorf(
			"image manifest %q is not readable: %w (it ships inside the installer image — this indicates a broken build)",
			opts.ImageManifestPath, err)
	}

	if _, err := os.Stat(opts.ModelManifestPath); err != nil {
		return fmt.Errorf(
			"model manifest %q is not readable: %w (place models.yaml there before running the installer, "+
				"or point %s at another location)",
			opts.ModelManifestPath, err, "MODEL_MANIFEST_PATH")
	}

	return nil
}

func Load(opts LoadOptions) (Catalog, error) {
	if err := opts.Validate(); err != nil {
		return Catalog{}, fmt.Errorf("invalid catalog options: %w", err)
	}

	if err := opts.checkInputs(); err != nil {
		return Catalog{}, err
	}

	images, err := readManifest[imageManifest](opts.ImageManifestPath)
	if err != nil {
		return Catalog{}, fmt.Errorf("read image manifest: %w", err)
	}

	if err := resolveImageSizes(images.Services, opts.ImagesDir); err != nil {
		return Catalog{}, fmt.Errorf("resolve service image sizes: %w", err)
	}

	if err := resolveImageSizes(images.Harbor, opts.ImagesDir); err != nil {
		return Catalog{}, fmt.Errorf("resolve Harbor image sizes: %w", err)
	}

	models, err := readManifest[modelManifest](opts.ModelManifestPath)
	if err != nil {
		return Catalog{}, fmt.Errorf("read model manifest: %w", err)
	}

	if err := validateModels(models.Models, opts.ModelManifestPath); err != nil {
		return Catalog{}, err
	}

	return Catalog{
		ImagesDir:     opts.ImagesDir,
		ServiceImages: images.Services,
		HarborImages:  images.Harbor,
		Models:        models.Models,
	}, nil
}

// validateModels rejects entries that cannot describe a real Hugging Face
// repository, so a bad manifest fails at bootstrap rather than after the
// storage check, minutes into a run.
func validateModels(models []Model, manifestPath string) error {
	for index, model := range models {
		if err := model.validate(); err != nil {
			return fmt.Errorf("%s: model %d (%s): %w", manifestPath, index+1, describeModel(model), err)
		}
	}

	return nil
}

func describeModel(model Model) string {
	if strings.TrimSpace(model.ID) != "" {
		return model.ID
	}

	return "unnamed"
}

func (m Model) validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("id is required")
	}

	return validateRevision(m.Revision)
}

// revisionCharacters are the ones git forbids in a ref name, plus the angle
// brackets that a placeholder left in the manifest tends to carry.
const revisionCharacters = " \t\n<>~^:?*[\\"

// validateRevision checks that a revision could name a commit, branch or tag.
// It deliberately accepts more than a commit sha: pinning is the documented
// advice, not a rule the installer enforces.
func validateRevision(revision string) error {
	trimmed := strings.TrimSpace(revision)

	if trimmed == "" {
		return fmt.Errorf("revision is required; pin the commit to download")
	}

	if strings.ContainsAny(trimmed, revisionCharacters) {
		return fmt.Errorf(
			"revision %q is not a commit, branch or tag; replace it with a commit sha such as 6cee5e81ee83917806bbde320786a8fb61efebee",
			revision,
		)
	}

	return nil
}
