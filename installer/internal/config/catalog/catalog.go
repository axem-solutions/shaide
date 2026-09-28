package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Catalog is the installer's inventory of what it can install: the container
// images to copy into Harbor, and the supported models the operator picks from.
//
// The image manifest ships inside the installer image. The models are the
// app-serving project's packaged model directories, so the list of supported
// models changes with the installer release.
type Catalog struct {
	ImagesDir string

	ServiceImages []Image

	HarborImages []Image

	Models []Model

	// SkippedModels are model directories that could not be read as a
	// supported model. They are reported, not fatal.
	SkippedModels []SkippedModel
}

type LoadOptions struct {
	ImageManifestPath string
	ImagesDir         string

	// Describe names a path for the operator, e.g. relative to the storage
	// directory they mounted. Nil shows the path as is.
	Describe func(path string) string

	// ModelsDir is the app-serving project's deployments/models directory.
	ModelsDir string
}

func (opts LoadOptions) describe(path string) string {
	if opts.Describe == nil {
		return path
	}
	return opts.Describe(path)
}

func (opts LoadOptions) Validate() error {
	if opts.ImageManifestPath == "" {
		return fmt.Errorf("image manifest path is required")
	}

	if opts.ImagesDir == "" {
		return fmt.Errorf("images directory is required")
	}

	if opts.ModelsDir == "" {
		return fmt.Errorf("models directory is required")
	}

	return nil
}

// checkInputs fails on a missing image manifest at bootstrap, with a message
// that says what it means. Without this it surfaces as a bare "no such file"
// from readManifest.
//
// ImagesDir is deliberately not checked here: it is only read for manifest
// entries whose source is "archive", and resolveImageSizes already reports a
// missing archive per image. Requiring the directory would fail every install
// that pulls all of its images from registries, which is the normal case.
func (opts LoadOptions) checkInputs() error {
	if _, err := os.Stat(opts.ImageManifestPath); err != nil {
		return fmt.Errorf(
			"image manifest %q is not readable: %w (it ships inside the installer image, which indicates a broken build)",
			opts.ImageManifestPath, err)
	}

	if _, err := os.Stat(opts.ModelsDir); err != nil {
		const cause = "it ships inside the installer image, which indicates a broken build"
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("models directory %s does not exist (%s)", opts.describe(opts.ModelsDir), cause)
		}
		return fmt.Errorf("models directory %s is not readable: %w (%s)", opts.describe(opts.ModelsDir), err, cause)
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

	models, skipped, err := LoadModels(opts.ModelsDir)
	if err != nil {
		return Catalog{}, fmt.Errorf("load models: %w", err)
	}

	return Catalog{
		ImagesDir:     opts.ImagesDir,
		ServiceImages: images.Services,
		HarborImages:  images.Harbor,
		Models:        models,
		SkippedModels: skipped,
	}, nil
}

// validate rejects a model that cannot be downloaded or served, so it is
// skipped at bootstrap rather than failing minutes into a run.
func (m Model) validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("modelArtifacts.name is required")
	}

	if strings.TrimSpace(m.StorageSize) == "" {
		return fmt.Errorf("modelArtifacts.size is required")
	}

	if err := validateRevision(m.Revision); err != nil {
		return err
	}

	for _, dep := range m.Dependencies {
		if strings.TrimSpace(dep.ID) == "" {
			return fmt.Errorf("shaide.dependencies: id is required")
		}
		if err := validateRevision(dep.Revision); err != nil {
			return fmt.Errorf("shaide.dependencies %s: %w", dep.ID, err)
		}
	}

	return nil
}

// minRevisionLength keeps the derived Harbor tag unambiguous.
const minRevisionLength = harborTagLength

// validateRevision requires a commit sha. The Harbor tag is derived from it,
// so a branch name would both drift and could produce an invalid tag.
func validateRevision(revision string) error {
	trimmed := strings.TrimSpace(revision)

	if trimmed == "" {
		return fmt.Errorf("shaide.revision is required; pin the Hugging Face commit to download")
	}

	if len(trimmed) < minRevisionLength || strings.Trim(strings.ToLower(trimmed), "0123456789abcdef") != "" {
		return fmt.Errorf(
			"revision %q is not a commit sha; pin a full sha such as 6cee5e81ee83917806bbde320786a8fb61efebee",
			revision,
		)
	}

	return nil
}
