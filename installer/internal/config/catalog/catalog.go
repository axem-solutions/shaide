package catalog

import (
	"fmt"
)

// TODO: add comments
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

func Load(opts LoadOptions) (Catalog, error) {
	if err := opts.Validate(); err != nil {
		return Catalog{}, fmt.Errorf("invalid catalog options: %w", err)
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
