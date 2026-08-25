package uploader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/bundle"
	"github.com/axem-solutions/ai_platform/installer/internal/config/storage"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/content/oci"
	oraserrdef "oras.land/oras-go/v2/errdef"
)

const modelArtifactType = "application/vnd.cnai.model"

func (u *Uploader) UploadModels(ctx context.Context, hubdir string, models []bundle.Model) error {
	for _, model := range models {
		u.logf("uploading model %s", model.HarborName)

		artifact, err := u.prepareModelArtifact(ctx, hubdir, model)
		if err != nil {
			return uploadError("prepare model artifact", model.HarborName, err)
		}

		if _, err := u.push(ctx, artifact); err != nil {
			return uploadError("push model artifact", model.HarborName, err)
		}
	}

	return nil
}

func (u *Uploader) prepareModelArtifact(ctx context.Context, hubdir string, model bundle.Model) (Artifact, error) {
	ref := targetRef(model.HarborProject, model.HarborName, model.HarborTag)

	cache, manifest, err := u.cacheArtifact(ctx, hubdir, model, ref)
	if err != nil {
		return Artifact{}, err
	}

	bytes, _, err := graphSize(ctx, cache, manifest)
	if err != nil {
		return Artifact{}, fmt.Errorf("calculate model artifact size: %w", err)
	}

	return Artifact{
		Source:      cache,
		SourceRef:   ref,
		SpoolChunks: false,

		Project: model.HarborProject,
		Name:    model.HarborName,
		Tag:     model.HarborTag,

		ProgressID: model.ID,
		Phase:      progress.PhaseUploading,
		Bytes:      bytes,
	}, nil
}

func (u *Uploader) cacheArtifact(ctx context.Context, hubDir string, model bundle.Model, ref string) (*oci.Store, ocispec.Descriptor, error) {
	cache, manifest, found, err := u.resolveCache(ctx, ref)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}

	if found {
		u.logf("using cached model artifact %s", ref)
		return cache, manifest, nil
	}

	u.logf("cached model artifact %s not found; building", ref)

	manifest, err = u.buildCache(ctx, hubDir, model, cache, ref)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}

	return cache, manifest, nil
}

func (u *Uploader) resolveCache(ctx context.Context, ref string) (*oci.Store, ocispec.Descriptor, bool, error) {
	if err := os.MkdirAll(u.artifactCacheDir, 0o755); err != nil {
		return nil, ocispec.Descriptor{}, false, fmt.Errorf("%w: create artifact cache dir: %w", errdef.ErrArtifactBuildFailure, err)
	}

	cache, err := oci.New(u.artifactCacheDir)
	if err != nil {
		return nil, ocispec.Descriptor{}, false, fmt.Errorf("open artifact cache: %w", err)
	}

	manifest, err := cache.Resolve(ctx, ref)
	if err != nil {
		if errors.Is(err, oraserrdef.ErrNotFound) {
			return cache, ocispec.Descriptor{}, false, nil
		}

		return nil, ocispec.Descriptor{}, false, fmt.Errorf("resolve cached artifact %s: %w", ref, err)
	}

	return cache, manifest, true, nil

}

func (u *Uploader) buildCache(ctx context.Context, hubDir string, model bundle.Model, cache *oci.Store, ref string) (ocispec.Descriptor, error) {
	if err := u.checkBuildStorage(hubDir, model, ref); err != nil {
		return ocispec.Descriptor{}, err
	}

	store, err := file.New(hubDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: create new file store: %w", errdef.ErrArtifactBuildFailure, err)
	}

	store.TarReproducible = true
	defer store.Close()

	built, err := u.packStore(ctx, hubDir, model, store)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	cached, err := oras.Copy(ctx, store, model.HarborTag, cache, ref, oras.CopyOptions{})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("copy artifact into OCI cache: %w", err)
	}

	u.logf("cached model artifact %s built_manifest=%s cached_manifest=%s", ref, built.Digest, cached.Digest)

	return cached, nil
}

func (u *Uploader) packStore(ctx context.Context, hubDir string, model bundle.Model, store *file.Store) (ocispec.Descriptor, error) {
	dirs := modelDirs(model)

	tracker := progress.NewTracker(progress.PhaseBuilding, model.ID, int64(len(dirs)+1), 0, u.progressf)

	tracker.Start()
	defer tracker.Finish()

	layers := make([]ocispec.Descriptor, 0, len(dirs))
	for _, dir := range dirs {
		layer, err := u.addLayer(ctx, store, hubDir, dir)
		if err != nil {
			return ocispec.Descriptor{}, err
		}

		layers = append(layers, layer)

		u.logf("added layer %s from %s", layer.Digest, dir)
		tracker.AddFiles(1)
	}

	manifest, err := oras.PackManifest(
		ctx,
		store,
		oras.PackManifestVersion1_1,
		modelArtifactType,
		oras.PackManifestOptions{
			Layers: layers,
		},
	)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: pack manifest: %w", errdef.ErrArtifactBuildFailure, err)
	}
	if err := store.Tag(ctx, manifest, model.HarborTag); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: tag manifest %q: %w", errdef.ErrArtifactBuildFailure, model.HarborTag, err)
	}

	tracker.AddFiles(1)

	bytes, _, err := graphSize(ctx, store, manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("calculate built artifact size: %w", err)
	}

	u.logf(
		"built model artifact %s layers=%d size=%d manifest=%s",
		targetRef(model.HarborProject, model.HarborName, model.HarborTag),
		len(layers),
		bytes,
		manifest.Digest,
	)

	return manifest, nil
}

func (u *Uploader) addLayer(ctx context.Context, store *file.Store, hubDir string, dir string) (ocispec.Descriptor, error) {
	path := filepath.Join(hubDir, dir)

	if _, err := os.Stat(path); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: dir %s is missing: %w", errdef.ErrLocalModelCacheFailure, path, err)
	}

	layer, err := store.Add(ctx, dir, ocispec.MediaTypeImageLayerGzip, path)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: add model cache dir %s: %w", errdef.ErrArtifactBuildFailure, dir, err)
	}

	return layer, nil
}

func modelDirs(model bundle.Model) []string {
	dirs := []string{modelDir(model.ID)}

	for _, dep := range model.Dependencies {
		dirs = append(dirs, modelDir(dep.ID))
	}

	return dirs
}

func modelDir(id string) string {
	return "models--" + strings.ReplaceAll(id, "/", "--")
}

func (u *Uploader) checkBuildStorage(hubDir string, model bundle.Model, ref string) error {
	required, err := estimateBuildWorkingSpace(hubDir, modelDirs(model))
	if err != nil {
		return fmt.Errorf("estimate OCI artifact storage for %s: %w", ref, err)
	}

	return u.storageChecker(storage.Requirement{
		Phase:    "model artifact build",
		Target:   ref,
		Expected: required,
	})
}

const (
	tarBlockSize        int64 = 512
	tarEndBlocks              = 2
	gzipSafetyDivisor   int64 = 100
	artifactSafetyBytes int64 = 1 << 20
)

// Walk the model tree and estimate the size of the tar.gz artifact that will be created later.
func estimateBuildWorkingSpace(hubDir string, dirs []string) (int64, error) {
	var tarBytes int64
	for _, dir := range dirs {
		root := filepath.Join(hubDir, dir)
		if _, err := os.Stat(root); err != nil {
			return 0, fmt.Errorf("%w: dir %s is missing: %w", errdef.ErrLocalModelCacheFailure, root, err)
		}

		err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Account for tar headers and file padding.
			tarBytes += tarBlockSize
			if info.Mode().IsRegular() {
				tarBytes += roundUp(info.Size(), tarBlockSize)
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("walk model cache dir %s: %w", root, err)
		}

		// Tar archives end with two empty blocks.
		tarBytes += tarEndBlocks * tarBlockSize
	}

	// Leave some headroom for gzip and OCI metadata.
	return tarBytes + tarBytes/gzipSafetyDivisor + artifactSafetyBytes, nil
}

func roundUp(value, blockSize int64) int64 {
	if value <= 0 {
		return 0
	}
	return ((value + blockSize - 1) / blockSize) * blockSize
}
