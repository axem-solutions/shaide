package artifact

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/config/resources"
	"github.com/axem-solutions/ai_platform/installer/internal/config/storage"
	harborapi "github.com/axem-solutions/ai_platform/installer/internal/harbor/api"
	harborerrors "github.com/axem-solutions/ai_platform/installer/internal/harbor/errors"
	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	"github.com/axem-solutions/ai_platform/installer/internal/huggingface"
	"github.com/axem-solutions/ai_platform/installer/internal/oras"
	orasapi "github.com/axem-solutions/ai_platform/installer/internal/oras/client"
	orasremote "github.com/axem-solutions/ai_platform/installer/internal/oras/repository"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/discovery"
)

func Stage() core.Stage {
	return core.Stage{
		Name: "populate Harbor",
		Steps: []core.Step{
			{
				Name:    "check model artifacts",
				Run:     checkModelArtifacts,
				Recover: recoverCheckModelArtifacts,
			},
			{
				Name: "report model storage",
				Run:  reportModelStorage,
			},
			{
				Name:    "Download models",
				Run:     downloadModels,
				Recover: recoverDownloadModels,
			},
			{
				Name:    "Upload models",
				Run:     uploadModels,
				Recover: recoverArtifactUpload,
			},
			{
				Name:    "Upload images",
				Run:     uploadImages,
				Recover: recoverArtifactUpload,
			},
			{
				Name:    "Delete uninstalled models",
				When:    uninstallsModels,
				Run:     deleteUninstalledModels,
				Recover: recoverDeleteModels,
			},
		},
		Cleanup: ClosePortForward,
	}
}

// STEP 1 - check which models to serve are missing from Harbor
func checkModelArtifacts(rt *core.Runtime) error {
	rt.Artifact.ToUpload = nil

	if len(rt.Models.Serve) == 0 {
		return nil
	}

	if err := discovery.RefreshPortForward(rt); err != nil {
		return err
	}

	clientOptions, err := artifactClientOptions(rt)
	if err != nil {
		return err
	}
	client := orasapi.NewClient(clientOptions)

	for _, model := range rt.Models.Serve {
		found, err := modelExistsInHarbor(context.Background(), client, model, rt.Bootstrap.Config.Paths.UploadState)
		if err != nil {
			return err
		}
		if found {
			rt.Detailf("model %s already exists in Harbor as %s/%s:%s",
				model.ID,
				model.HarborProject,
				model.HarborName,
				model.HarborTag,
			)
			continue
		}

		rt.Artifact.ToUpload = append(rt.Artifact.ToUpload, model)
	}

	if len(rt.Artifact.ToUpload) == 0 {
		rt.Detailf("every model to serve is already in Harbor")
	}

	return nil
}

// STEP 2 - Download models from HuggingFace
func downloadModels(rt *core.Runtime) error {
	if len(rt.Artifact.ToUpload) == 0 {
		return nil
	}

	budget, err := resources.Detect(rt.Bootstrap.Config.Resources.Limits)
	if err != nil {
		return fmt.Errorf("resolve transfer resource budget: %w", err)
	}

	if rt.Bootstrap.Config.Resources.HighPerformance {
		rt.Detailf("transfer budget: high performance, using the whole machine")
	} else {
		rt.Detailf("transfer budget: %s", budget)
	}

	downloader, err := huggingface.NewDownloader(huggingface.Options{
		Token:    rt.Bootstrap.Config.HuggingFace.Token,
		CacheDir: rt.Bootstrap.Config.Paths.ModelCache,
		Logf:     rt.Detailf,

		Budget:          budget,
		HighPerformance: rt.Bootstrap.Config.Resources.HighPerformance,

		StorageCheck: storage.NewChecker(
			rt.Bootstrap.Config.Paths.StorageRoot,
			rt.Detailf,
		),

		Progressf: func(e progress.Event) {
			rt.Reporter.ProgressModel(core.ModelProgress{
				ID:         fmt.Sprintf("%s\n %s", e.Phase, e.Current),
				Bytes:      e.Bytes,
				TotalBytes: e.TotalBytes,
				Files:      e.Files,
				TotalFiles: e.TotalFiles,
				Percent:    e.Percent,
				Done:       e.Done,
			})
		},
	})
	if err != nil {
		return err
	}

	for _, model := range rt.Artifact.ToUpload {
		err := downloader.DownloadModel(context.Background(), huggingFaceModel(model))
		if err != nil {
			return err
		}

	}

	return nil
}

// STEP 3 - Upload models to Harbor via oras
func uploadModels(rt *core.Runtime) error {
	if len(rt.Artifact.ToUpload) == 0 {
		return nil
	}
	if err := discovery.RefreshPortForward(rt); err != nil {
		return err
	}

	uploader, err := artifactUploader(rt)
	if err != nil {
		return err
	}

	hubDir := filepath.Join(rt.Bootstrap.Config.Paths.ModelCache, "hub")

	return uploader.UploadModels(
		context.Background(),
		hubDir,
		rt.Artifact.ToUpload,
	)
}

// STEP 4 - Upload images to Harbor via oras
func uploadImages(rt *core.Runtime) error {
	if err := discovery.RefreshPortForward(rt); err != nil {
		return err
	}

	uploader, err := artifactUploader(rt)
	if err != nil {
		return err
	}

	return uploader.UploadImages(
		context.Background(),
		rt.Bootstrap.Catalog.ImagesDir,
		rt.Bootstrap.Catalog.ServiceImages,
	)
}

func artifactUploader(rt *core.Runtime) (*oras.Uploader, error) {
	clientOptions, err := artifactClientOptions(rt)
	if err != nil {
		return nil, err
	}

	return oras.NewUploader(oras.UploaderOptions{
		Client:           clientOptions,
		Platform:         rt.Cluster.Platform,
		ChunkSize:        128 << 20,
		StateDir:         rt.Bootstrap.Config.Paths.UploadState,
		ArtifactCacheDir: rt.Bootstrap.Config.Paths.ArtifactCache,
		Logf:             rt.Detailf,

		StorageChecker: storage.NewChecker(
			rt.Bootstrap.Config.Paths.StorageRoot,
			rt.Detailf,
		),

		Progressf: func(p progress.Event) {
			rt.Reporter.ProgressModel(core.ModelProgress{
				ID:         fmt.Sprintf("%s\n%s", p.Phase, p.Current),
				Percent:    p.Percent,
				Bytes:      p.Bytes,
				TotalBytes: p.TotalBytes,
				Files:      0,
				TotalFiles: 0,
				Done:       p.Done,
			})
		},
	})
}

func artifactClientOptions(rt *core.Runtime) (orasapi.ClientOptions, error) {
	if rt.Discovery.HarborForward == nil {
		return orasapi.ClientOptions{}, fmt.Errorf("harbor port-forward is not initialized")
	}

	return orasapi.ClientOptions{
		Registry: fmt.Sprintf(
			"127.0.0.1:%d",
			rt.Discovery.HarborForward.LocalPort(),
		),
		TargetCredentials: orasapi.Credential{
			Username: strings.TrimSpace(rt.Discovery.Auth.Username),
			Password: strings.TrimSpace(rt.Discovery.Auth.Password),
		},
		RemoteCredentials: remoteSourceCredentials(rt),
	}, nil
}

func remoteSourceCredentials(rt *core.Runtime) map[string]orasapi.Credential {
	credentials := map[string]orasapi.Credential{}

	ghcr := orasapi.Credential{
		Username: rt.Bootstrap.Config.Registries.GHCR.Username,
		Password: rt.Bootstrap.Config.Registries.GHCR.Password,
	}
	if ghcr.Username != "" && ghcr.Password != "" {
		credentials[orasapi.GHCRRegistry] = ghcr
	}

	dockerHub := orasapi.Credential{
		Username: rt.Bootstrap.Config.Registries.DockerHub.Username,
		Password: rt.Bootstrap.Config.Registries.DockerHub.Password,
	}
	if dockerHub.Username != "" && dockerHub.Password != "" {
		credentials[orasapi.DockerHubRegistry] = dockerHub
	}

	if len(credentials) == 0 {
		return nil
	}

	return credentials
}

func uninstallsModels(rt *core.Runtime) bool {
	return len(rt.Models.Uninstall) > 0
}

// STEP 5 - Delete the artifacts of uninstalled models from Harbor
//
// A repository that is already gone counts as deleted, so a run that failed
// after this step can uninstall the same model again.
func deleteUninstalledModels(rt *core.Runtime) error {
	if err := discovery.RefreshPortForward(rt); err != nil {
		return err
	}

	for _, model := range rt.Models.Uninstall {
		err := harborapi.DeleteRepository(
			context.Background(),
			rt.Discovery.Client,
			model.HarborProject,
			model.HarborName,
		)
		if isNotFound(err) {
			rt.Detailf("Harbor has no %s/%s to delete", model.HarborProject, model.HarborName)
			continue
		}
		if err != nil {
			return fmt.Errorf("delete Harbor model %s/%s: %w", model.HarborProject, model.HarborName, err)
		}

		rt.Detailf("deleted Harbor model repository %s/%s", model.HarborProject, model.HarborName)
	}

	return nil
}

func isNotFound(err error) bool {
	var harborErr *harborerrors.Error
	return errors.As(err, &harborErr) && harborErr.Kind == httpapi.ErrNotFound
}

func modelExistsInHarbor(ctx context.Context, client *orasapi.Client, model catalog.Model, uploadDir string) (bool, error) {
	repository, err := client.NewTargetRepository(
		model.HarborProject,
		model.HarborName,
		orasremote.ChunkedUploadOptions{
			StateDir: uploadDir,
		},
	)
	if err != nil {
		return false, fmt.Errorf("create Harbor repository target: %w", err)
	}

	exists, err := repository.ManifestExists(ctx, model.HarborTag)
	if err != nil {
		return false, fmt.Errorf("check Harbor manifest %s/%s:%s: %w",
			model.HarborProject,
			model.HarborName,
			model.HarborTag,
			err,
		)
	}

	return exists, nil
}

func huggingFaceModel(model catalog.Model) huggingface.Model {
	deps := make([]huggingface.Dependency, 0, len(model.Dependencies))
	for _, dep := range model.Dependencies {
		deps = append(deps, huggingface.Dependency{
			ID:       dep.ID,
			Revision: dep.Revision,
		})
	}

	return huggingface.Model{
		ID:           model.ID,
		Revision:     model.Revision,
		Dependencies: deps,
	}
}

func ClosePortForward(rt *core.Runtime) error {
	if rt.Discovery.HarborForward == nil {
		return nil
	}

	rt.Discovery.HarborForward.Close()
	rt.Discovery.HarborForward = nil
	rt.Discovery.Client = nil

	return nil
}
