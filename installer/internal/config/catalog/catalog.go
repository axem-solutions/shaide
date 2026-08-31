package catalog

import (
	"fmt"
	"os"
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

	return Catalog{
		ImagesDir:     opts.ImagesDir,
		ServiceImages: images.Services,
		HarborImages:  images.Harbor,
		Models:        models.Models,
	}, nil
}
