package catalog

import (
	"fmt"
	"path/filepath"
)

// TODO: add comments
type Catalog struct {
	ImagesDir string

	ServiceImages []Image

	HarborImages []Image

	Models []Model
}

type LoadOptions struct {
	ManifestsDir string
	ImagesDir    string
}

func (opts LoadOptions) Validate() error {
	if opts.ManifestsDir == "" {
		return fmt.Errorf("manifests directory is required")
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

	imageManifestPath := filepath.Join(opts.ManifestsDir, "images.yml")
	modelManifestPath := filepath.Join(opts.ManifestsDir, "models.yml")

	images, err := readManifest[imageManifest](imageManifestPath)
	if err != nil {
		return Catalog{}, fmt.Errorf("read image manifest: %w", err)
	}

	if err := resolveImageSizes(images.Services, opts.ImagesDir); err != nil {
		return Catalog{}, fmt.Errorf("resolve service image sizes: %w", err)
	}

	if err := resolveImageSizes(images.Harbor, opts.ImagesDir); err != nil {
		return Catalog{}, fmt.Errorf("resolve Harbor image sizes: %w", err)
	}

	models, err := readManifest[modelManifest](modelManifestPath)
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
