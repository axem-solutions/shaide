package bundle

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/progress"
)

// Prepared contains the bundle data available after successful preparation.
type Prepared struct {
	// ImagesDir contains the extracted OCI image archives.
	ImagesDir string

	// PulumiWorkDir contains the Pulumi projects and stack files from the bundle.
	PulumiWorkDir string

	// ServiceImages contains service images declared by the bundle.
	ServiceImages []Image

	// HarborImages contains Harbor images declared by the bundle.
	HarborImages []Image

	// Models contains models declared by the bundle.
	Models []Model
}

// PrepareOptions configures bundle preparation.
type PrepareOptions struct {
	// ArchivePath is the path to the mounted installer bundle archive.
	ArchivePath string

	// Destination is the directory where the bundle is extracted.
	Destination string

	// Logf receives detailed bundle preparation messages.
	Logf func(format string, args ...any)

	// Progressf receives bundle extraction progress events.
	Progressf func(progress.Event)
}

func (opts PrepareOptions) Validate() error {
	if opts.ArchivePath == "" {
		return fmt.Errorf("bundle archive path is required")
	}

	if opts.Destination == "" {
		return fmt.Errorf("bundle destination is required")
	}

	return nil
}

func Prepare(opts PrepareOptions) (Prepared, error) {
	if err := opts.Validate(); err != nil {
		return Prepared{}, fmt.Errorf("invalid opts: %w", err)
	}

	extracted, err := extractBundle(extractOptions{
		archivePath:    opts.ArchivePath,
		destinationDir: opts.Destination,
		Logf:           opts.Logf,
		Progressf:      opts.Progressf,
	})
	if err != nil {
		return Prepared{}, fmt.Errorf("extract bundle: %w", err)
	}

	images, err := readManifest[imageManifest](extracted.ImageManifestPath)
	if err != nil {
		return Prepared{}, fmt.Errorf("read image manifest: %w", err)
	}

	if err := resolveImageSizes(images.Services, extracted.ImagesDir); err != nil {
		return Prepared{}, fmt.Errorf("resolve service image sizes: %w", err)
	}
	if err := resolveImageSizes(images.Harbor, extracted.ImagesDir); err != nil {
		return Prepared{}, fmt.Errorf("resolve Harbor image sizes: %w", err)
	}

	models, err := readManifest[modelManifest](extracted.ModelManifestPath)
	if err != nil {
		return Prepared{}, fmt.Errorf("read model manifest: %w", err)
	}

	return Prepared{
		ImagesDir:     extracted.ImagesDir,
		PulumiWorkDir: extracted.PulumiWorkDir,
		ServiceImages: images.Services,
		HarborImages:  images.Harbor,
		Models:        models.Models,
	}, nil
}
