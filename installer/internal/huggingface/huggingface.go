package huggingface

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/config/resources"
	"github.com/axem-solutions/ai_platform/installer/internal/config/storage"
	"github.com/axem-solutions/ai_platform/installer/internal/huggingface/api"
	"github.com/axem-solutions/ai_platform/installer/internal/huggingface/cache"
	"github.com/axem-solutions/ai_platform/installer/internal/huggingface/cli"
	hferrors "github.com/axem-solutions/ai_platform/installer/internal/huggingface/errors"
	"github.com/axem-solutions/ai_platform/installer/internal/huggingface/files"
	"github.com/axem-solutions/ai_platform/installer/internal/huggingface/utils"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
)

const (
	metadataTimeout = 15 * time.Second
)

const (
	DefaultCLI      = "huggingface-cli"
	defaultRevision = "main"
	hubDirName      = "hub"
	xetDirName      = "xet"
	modelDirPrefix  = "models--"
	sentinelName    = ".complete"
)

type Downloader struct {
	cache        cache.Manager
	client       *api.Client
	cli          cli.Runner
	storageCheck storage.Checker
	logf         func(format string, args ...any)
	progressf    func(progress.Event)
}

type Options struct {
	Token        string
	CacheDir     string
	StorageCheck storage.Checker
	Logf         func(format string, args ...any)
	Progressf    func(progress.Event)

	// Budget bounds what a transfer takes of the machine.
	Budget resources.Budget

	// HighPerformance lets Xet scale to the whole host, ignoring Budget.
	HighPerformance bool
}

type Model struct {
	ID       string
	Revision string

	Dependencies []Dependency
}

type Dependency struct {
	ID       string
	Revision string
}

type Repository struct {
	ID       string
	Revision string
}

type Progress struct {
	RepoID     string
	Bytes      int64
	TotalBytes int64
	Files      int
	TotalFiles int
	Percent    int
	Done       bool
}

type StorageEstimate struct {
	TotalBytes   int64
	TotalFiles   int
	Repositories []RepositoryMetadata
}

// download is the part of a repository revision the installer fetches: the
// files the model server loads.
type download struct {
	Files []string
	Bytes int64
}

// RepositoryMetadata describes a repository revision. Bytes and Files count
// the files the installer downloads, not the whole repository.
type RepositoryMetadata struct {
	ID       string
	Revision string
	SHA      string
	Bytes    int64
	Files    int
}

func NewDownloader(opts Options) (*Downloader, error) {
	if opts.Progressf == nil {
		return nil, fmt.Errorf("huggingface: progress callback is required")
	}
	if opts.Logf == nil {
		return nil, fmt.Errorf("huggingface: log callback is required")
	}

	cacheManager := cache.New(cache.Config{
		CacheDir:       opts.CacheDir,
		HubDirName:     hubDirName,
		XetDirName:     xetDirName,
		ModelDirPrefix: modelDirPrefix,
		SentinelName:   sentinelName,
		RefName:        defaultRevision,
	})

	return &Downloader{
		cache:        cacheManager,
		client:       api.New(opts.Token, defaultRevision),
		cli:          cli.New(cliConfig(opts)),
		storageCheck: opts.StorageCheck,
		logf:         opts.Logf,
		progressf:    opts.Progressf,
	}, nil
}

func (d *Downloader) DownloadModel(ctx context.Context, model Model) error {
	repos := convertModel(model)

	for _, repo := range repos {
		d.logf("downloading model %s", repo.ID)

		if err := d.downloadRepository(ctx, repo); err != nil {
			return fmt.Errorf("download Hugging Face repository %s: %w", repo.ID, err)
		}
	}

	return nil
}

func (d *Downloader) downloadRepository(ctx context.Context, repo Repository) error {
	state, err := d.cache.Prepare(repo.ID, normalizeRevision(repo.Revision))
	if err != nil {
		return &hferrors.Error{
			Kind:   hferrors.ErrCache,
			Op:     "prepare cache",
			RepoID: repo.ID,
			Err:    err,
		}
	}
	if state.Skip {
		d.detailf("using cached model %s@%s", repo.ID, state.Revision)
		return d.trimCache(repo, state)
	}

	plan, err := d.fetchDownload(ctx, repo)
	if err != nil {
		return &hferrors.Error{
			Kind:     hferrors.ClassifyError(err),
			Op:       "fetch metadata",
			RepoID:   repo.ID,
			Revision: repo.Revision,
			Err:      err,
		}
	}

	current := utils.Current(state.ModelDir)

	storageReq := storage.Requirement{
		Phase:    "model download",
		Target:   repo.ID,
		Expected: plan.Bytes,
		Reusable: current.Bytes,
	}

	if err := d.storageCheck(storageReq); err != nil {
		return &hferrors.Error{
			Kind:     hferrors.ErrCache,
			Op:       "check download storage",
			RepoID:   repo.ID,
			Revision: repo.Revision,
			Err:      err,
		}
	}

	if err := d.runDownload(ctx, repo, state, plan); err != nil {
		return &hferrors.Error{
			Kind:     hferrors.ClassifyError(err),
			Op:       "download",
			RepoID:   repo.ID,
			Revision: repo.Revision,
			Err:      err,
		}
	}

	if err := d.cache.Complete(state); err != nil {
		return &hferrors.Error{
			Kind:   hferrors.ErrCache,
			Op:     "complete cache",
			RepoID: repo.ID,
			Err:    err,
		}
	}

	return d.trimCache(repo, state)
}

func EstimateStorage(ctx context.Context, token string, models []Model) (StorageEstimate, error) {
	client := api.New(token, defaultRevision)
	repos := uniqueRepositories(models)

	estimate := StorageEstimate{
		Repositories: make([]RepositoryMetadata, 0, len(repos)),
	}

	for _, repo := range repos {
		reqCtx, cancel := context.WithTimeout(ctx, metadataTimeout)
		metadata, err := client.FetchMetadata(reqCtx, repo.ID, repo.Revision)
		cancel()
		if err != nil {
			return StorageEstimate{}, fmt.Errorf("fetch metadata for %s@%s: %w", repo.ID, repo.Revision, err)
		}

		plan := planDownload(metadata)

		estimate.TotalBytes += plan.Bytes
		estimate.TotalFiles += len(plan.Files)
		estimate.Repositories = append(estimate.Repositories, RepositoryMetadata{
			ID:       repo.ID,
			Revision: repo.Revision,
			SHA:      metadata.SHA,
			Bytes:    plan.Bytes,
			Files:    len(plan.Files),
		})
	}

	return estimate, nil
}

func (d *Downloader) fetchDownload(ctx context.Context, repo Repository) (download, error) {
	reqCtx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()

	metadata, err := d.client.FetchMetadata(reqCtx, repo.ID, repo.Revision)
	if err != nil {
		return download{}, err
	}

	plan := planDownload(metadata)
	if skipped := metadata.TotalFiles - len(plan.Files); skipped > 0 {
		d.detailf(
			"skipping %d files of %s that the model server does not load (%s)",
			skipped,
			repo.ID,
			storage.FormatBytes(metadata.TotalBytes-plan.Bytes),
		)
	}

	return plan, nil
}

// planDownload picks the files of the revision that the model server loads.
func planDownload(metadata api.Metadata) download {
	paths := make([]string, 0, len(metadata.Siblings))
	sizes := make(map[string]int64, len(metadata.Siblings))
	for _, sibling := range metadata.Siblings {
		paths = append(paths, sibling.Filename)
		sizes[sibling.Filename] = sibling.Size
	}

	plan := download{Files: files.Select(paths)}
	for _, path := range plan.Files {
		if size := sizes[path]; size > 0 {
			plan.Bytes += size
		}
	}

	return plan
}

func (d *Downloader) runDownload(ctx context.Context, repo Repository, state cache.State, plan download) error {
	tracker := progress.NewTracker(
		progress.PhaseDownloading,
		repo.ID,
		plan.Bytes,
		len(plan.Files),
		d.progressf,
	)

	tracker.Start()

	pollCtx, cancelPoll := context.WithCancel(ctx)
	pollDone := make(chan struct{})

	go func() {
		defer close(pollDone)

		tracker.Poll(pollCtx, progress.DefaultTick, func() progress.Snapshot {
			info := utils.Current(state.ModelDir)

			return progress.Snapshot{
				Bytes: info.Bytes,
				Files: info.Files,
			}
		})
	}()

	err := d.cli.Download(ctx, cli.DownloadRequest{
		RepoID:   repo.ID,
		Revision: repo.Revision,
		HubDir:   d.cache.HubDir(),
		Files:    plan.Files,
	})

	cancelPoll()
	<-pollDone

	if err != nil {
		tracker.Stop()
		return err
	}

	info := utils.Current(state.ModelDir)
	tracker.SetSnapshot(progress.Snapshot{
		Bytes: info.Bytes,
		Files: info.Files,
	})

	tracker.Finish()

	return nil
}

// trimCache leaves the pinned revision's files that the model server loads in
// the cache, so nothing else is packed into the model artifact or kept on the
// provisioning host. It drops the other files of a cache an earlier installer
// filled with the whole repository, and what earlier pins of the model left.
func (d *Downloader) trimCache(repo Repository, state cache.State) error {
	removed, err := d.cache.RemoveUnselected(state, files.Select)
	if err != nil {
		return &hferrors.Error{
			Kind:   hferrors.ErrCache,
			Op:     "remove files the model server does not load",
			RepoID: repo.ID,
			Err:    err,
		}
	}
	if len(removed) > 0 {
		d.detailf("removed %d files of %s that the model server does not load from the cache", len(removed), repo.ID)
	}

	freed, err := d.cache.PruneOtherRevisions(state)
	if err != nil {
		return &hferrors.Error{
			Kind:   hferrors.ErrCache,
			Op:     "prune superseded revisions",
			RepoID: repo.ID,
			Err:    err,
		}
	}
	if freed > 0 {
		d.detailf("freed %s of unused files in the cache of %s", storage.FormatBytes(freed), repo.ID)
	}

	return nil
}

func (d *Downloader) detailf(format string, args ...any) {
	d.logf(format, args...)
}

func cliConfig(opts Options) cli.Config {
	return cli.Config{
		Path:            DefaultCLI,
		CacheDir:        opts.CacheDir,
		Token:           opts.Token,
		XetDirName:      xetDirName,
		DefaultRevision: defaultRevision,
		Budget:          opts.Budget,
		HighPerformance: opts.HighPerformance,
	}
}

func convertModel(model Model) []Repository {
	repos := []Repository{{
		ID:       model.ID,
		Revision: model.Revision,
	}}

	for _, dep := range model.Dependencies {
		repos = append(repos, Repository{
			ID:       dep.ID,
			Revision: dep.Revision,
		})
	}

	return repos
}

func uniqueRepositories(models []Model) []Repository {
	seen := make(map[string]struct{})
	repos := make([]Repository, 0, len(models))

	for _, model := range models {
		for _, repo := range convertModel(model) {
			repo.ID = strings.TrimSpace(repo.ID)
			repo.Revision = normalizeRevision(repo.Revision)

			key := repo.ID + "\x00" + repo.Revision
			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}
			repos = append(repos, repo)
		}
	}

	return repos
}

func normalizeRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return defaultRevision
	}
	return revision
}
