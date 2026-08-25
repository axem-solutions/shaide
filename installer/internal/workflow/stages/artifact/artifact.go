package artifact

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/bundle"
	"github.com/axem-solutions/ai_platform/installer/internal/config/storage"
	harborapi "github.com/axem-solutions/ai_platform/installer/internal/harbor/api"
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
				Name: "Select models to download",
				Run:  selectModelsToDownload,
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
				Name:    "Delete models",
				Run:     deleteModels,
				Recover: recoverDeleteModels,
			},
		},
		Cleanup: ClosePortForward,
	}
}

// STEP 1 - check models in Harbor
func checkModelArtifacts(rt *core.Runtime) error {
	rt.Artifact.ModelOptions = nil

	client := orasapi.NewClient(artifactClientOptions(rt))

	for _, model := range rt.Bootstrap.Bundle.Models {
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

		label := modelOptionLabel(model)
		rt.Artifact.ModelOptions = append(rt.Artifact.ModelOptions, core.ModelOption{
			Label: label,
			Model: model,
		})
	}

	return nil
}

// STEP 2 - UserInput: ai-models
func selectModelsToDownload(rt *core.Runtime) error {
	if len(rt.Artifact.ModelOptions) == 0 {
		rt.Detailf("all manifest models already exist in Harbor")
		return nil
	}

	options := make([]string, 0, len(rt.Artifact.ModelOptions))
	for _, option := range rt.Artifact.ModelOptions {
		options = append(options, option.Label)
	}

	var selected []string
	for {
		var err error
		selected, err = rt.Reporter.MultiSelect(
			"Select models to download from manifest",
			options,
		)
		if err != nil {
			return err
		}

		if len(selected) > 0 {
			break
		}

		confirm, err := rt.Reporter.Select(
			"No model selected. Are you sure you want to continue?",
			"No",
			[]string{"No", "Yes"},
		)
		if err != nil {
			return err
		}

		if confirm == "Yes" {
			break
		}
	}

	selectedSet := make(map[string]struct{}, len(selected))
	for _, label := range selected {
		selectedSet[label] = struct{}{}
	}

	for _, option := range rt.Artifact.ModelOptions {
		if _, ok := selectedSet[option.Label]; !ok {
			continue
		}

		rt.Artifact.SelectedModels = append(rt.Artifact.SelectedModels, option.Model)
	}
	if len(rt.Artifact.SelectedModels) == 0 {
		rt.Detailf("no models selected for download")
	}

	return nil
}

// STEP 3 - Download models from HuggingFace
func downloadModels(rt *core.Runtime) error {
	if len(rt.Artifact.SelectedModels) == 0 {
		return nil
	}

	downloader, err := huggingface.NewDownloader(huggingface.Options{
		Token:    rt.Bootstrap.Config.HuggingFace.Token,
		CacheDir: rt.Bootstrap.Config.Paths.ModelCache,
		Logf:     rt.Detailf,

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

	for _, model := range rt.Artifact.SelectedModels {
		err := downloader.DownloadModel(context.Background(), huggingFaceModel(model))
		if err != nil {
			return err
		}

	}

	return nil
}

// STEP 4 - Upload models to Harbor via oras
func uploadModels(rt *core.Runtime) error {
	if len(rt.Artifact.SelectedModels) == 0 {
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
		rt.Artifact.SelectedModels,
	)
}

// STEP 5 - Upload images to Harbor via oras
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
		rt.Bootstrap.Bundle.ImagesDir,
		rt.Bootstrap.Bundle.ServiceImages,
	)
}

func artifactUploader(rt *core.Runtime) (*oras.Uploader, error) {
	return oras.NewUploader(oras.UploaderOptions{
		Client:           artifactClientOptions(rt),
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
func artifactClientOptions(rt *core.Runtime) orasapi.ClientOptions {
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
	}
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

// STEP 6 - Delete models from Harbor
func deleteModels(rt *core.Runtime) error {
	shouldDelete, err := rt.Reporter.Select(
		"Do you want to delete models from Harbor?",
		"No",
		[]string{"No", "Yes"},
	)
	if err != nil {
		return err
	}
	if shouldDelete != "Yes" {
		rt.Detailf("skipping Harbor model deletion")
		return nil
	}

	client := rt.Discovery.Client

	repositories, err := harborapi.ListRepositories(context.Background(), client, "ai-models")
	if err != nil {
		return err
	}

	for _, repo := range repositories {
		rt.Detailf("%s", repo.Name)
	}

	options := make([]string, 0, len(repositories))
	for _, model := range repositories {
		options = append(options, model.Name)
	}

	if len(options) == 0 {
		rt.Detailf("No models available in Harbor")
		return nil
	}

	selected, err := rt.Reporter.MultiSelect(
		"Select models to delete from Harbor",
		options,
	)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		rt.Detailf("no models selected for deletion")
		return nil
	}

	selectedSet := make(map[string]struct{}, len(selected))
	for _, label := range selected {
		selectedSet[label] = struct{}{}
	}

	for _, repo := range repositories {
		if _, ok := selectedSet[repo.Name]; !ok {
			continue
		}

		parts := strings.Split(repo.Name, "/")

		if err := harborapi.DeleteRepository(
			context.Background(),
			client,
			parts[0],
			parts[1],
		); err != nil {
			return fmt.Errorf("delete Harbor model %s: %w", repo.Name, err)
		}

		rt.Detailf("deleted Harbor model repository %s/%s", "ai-models", repo.Name)
	}

	return nil
}

func modelExistsInHarbor(ctx context.Context, client *orasapi.Client, model bundle.Model, uploadDir string) (bool, error) {
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

func huggingFaceModel(model bundle.Model) huggingface.Model {
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

func modelOptionLabel(model bundle.Model) string {
	return fmt.Sprintf("%s:%s",
		model.HarborName,
		model.HarborTag,
	)
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
