package uploader

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"
)

func (u *Uploader) UploadImages(ctx context.Context, imageDir string, images []catalog.Image) error {
	for _, image := range images {
		u.logf("uploading image %s", image.Name)

		artifact, err := u.prepareImageArtifact(ctx, imageDir, image)
		if err != nil {
			return uploadError("prepare image artifact", image.Name, err)
		}

		if _, err := u.push(ctx, artifact); err != nil {
			return uploadError("push image artifact", image.Name, err)
		}
	}

	return nil
}

func (u *Uploader) prepareImageArtifact(ctx context.Context, imageDir string, image catalog.Image) (Artifact, error) {
	// Example:
	//
	//   source: archive://api-server.tar
	//   source: dockerhub://library/nginx:1.25
	//   source: ghcr://my-org/api-server:v1.2.3
	switch image.Source {
	case catalog.ImageSourceArchive:
		return u.archiveArtifact(ctx, imageDir, image)
	case catalog.ImageSourceDockerHub, catalog.ImageSourceGitHub, catalog.ImageSourceNVCR, catalog.ImageSourceQuay, catalog.ImageSourceRegistryK8s:
		return u.remoteArtifact(ctx, image)
	}

	return Artifact{}, fmt.Errorf("unsupported image source %q", image.Source)
}

func (u *Uploader) archiveArtifact(ctx context.Context, imageDir string, image catalog.Image) (Artifact, error) {
	archiveFile := image.FileName()
	path := filepath.Join(imageDir, archiveFile)

	source, err := oci.NewFromTar(ctx, path)
	if err != nil {
		return Artifact{}, fmt.Errorf("open docker image archive: %w", err)
	}

	sourceRef, manifest, err := resolveImage(ctx, source, image)
	if err != nil {
		return Artifact{}, fmt.Errorf("resolve image: %w", err)
	}

	u.logf("resolved docker image archive %s as %s", image.Source, sourceRef)

	bytes, files, err := graphSize(ctx, source, manifest)
	if err != nil {
		return Artifact{}, fmt.Errorf("artifact size: %w", err)
	}

	return Artifact{
		Source:    source,
		SourceRef: sourceRef,

		Project: image.Project,
		Name:    image.Name,
		Tag:     image.Tag,

		ProgressID: fmt.Sprintf("%s:%s", image.Name, image.Tag),
		Phase:      progress.PhaseUploading,
		Bytes:      bytes,
		Files:      files,
	}, nil
}

func (u *Uploader) remoteArtifact(ctx context.Context, image catalog.Image) (Artifact, error) {
	source, err := u.client.NewSourceRepository(image)
	if err != nil {
		return Artifact{}, fmt.Errorf("create remote source repository %q: %w", image.Source, err)
	}

	manifest, err := source.Resolve(ctx, image.Tag)
	if err != nil {
		return Artifact{}, fmt.Errorf("resolve remote image ref %q: %w", image.Source, err)
	}

	bytes, files, err := graphSize(ctx, source, manifest)
	if err != nil {
		return Artifact{}, fmt.Errorf("calculate remote image artifact size: %w", err)
	}

	return Artifact{
		Source:      source,
		SourceRef:   image.Tag,
		SpoolChunks: true,

		Project: image.Project,
		Name:    image.Name,
		Tag:     image.Tag,

		ProgressID: image.Name,
		Phase:      progress.PhaseUploading,
		Bytes:      bytes,
		Files:      files,
	}, nil
}

// resolveImage resolves the OCI descriptor for image inside an archive
// produced by `docker save -o <file> <project>/<name>:<tag>`
func resolveImage(ctx context.Context, source *oci.ReadOnlyStore, image catalog.Image) (string, ocispec.Descriptor, error) {
	ref := targetRef(image.Project, image.Name, image.Tag)

	manifest, err := source.Resolve(ctx, ref)
	if err != nil {
		return "", ocispec.Descriptor{}, fmt.Errorf("%w: resolve archive ref %q: %w", errdef.ErrArtifactBuildFailure, ref, err)
	}

	return ref, manifest, nil
}
