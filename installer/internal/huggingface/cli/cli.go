package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	hferrors "github.com/axem-solutions/ai_platform/installer/internal/huggingface/errors"
)

type Config struct {
	Path            string
	CacheDir        string
	Token           string
	XetDirName      string
	DefaultRevision string
}

type Runner struct {
	Config
}

type DownloadRequest struct {
	RepoID   string
	Revision string
	HubDir   string
}

func New(config Config) Runner {
	return Runner{Config: config}
}

func (r Runner) Download(ctx context.Context, request DownloadRequest) error {
	if err := r.check(); err != nil {
		return err
	}

	request = r.normalizeDownloadRequest(request)
	if request.RepoID == "" {
		return fmt.Errorf("repo id is required")
	}
	if request.HubDir == "" {
		return fmt.Errorf("hub cache directory is required")
	}

	cmd := exec.CommandContext(ctx, r.Path, downloadArgs(request)...)
	cmd.Env = r.env()

	output, err := cmd.CombinedOutput()
	if err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		return &hferrors.CliError{
			RepoID:   request.RepoID,
			ExitCode: exitCode,
			Output:   string(output),
			Err:      err,
		}
	}

	return nil
}

func (r Runner) check() error {
	if strings.TrimSpace(r.Path) == "" {
		return fmt.Errorf("CLI path is required")
	}
	if strings.TrimSpace(r.CacheDir) == "" {
		return fmt.Errorf("cache directory is required")
	}
	if strings.TrimSpace(r.XetDirName) == "" {
		return fmt.Errorf("xet directory name is required")
	}
	if strings.TrimSpace(r.DefaultRevision) == "" {
		return fmt.Errorf("default revision is required")
	}
	if _, err := exec.LookPath(r.Path); err != nil {
		return fmt.Errorf("%s not found: %w", r.Path, err)
	}
	return nil
}

func downloadArgs(request DownloadRequest) []string {
	return []string{
		"download",
		"--cache-dir", request.HubDir,
		"--revision", request.Revision,
		request.RepoID,
	}
}

func (r Runner) normalizeDownloadRequest(request DownloadRequest) DownloadRequest {
	request.RepoID = strings.TrimSpace(request.RepoID)
	request.Revision = strings.TrimSpace(request.Revision)
	request.HubDir = strings.TrimSpace(request.HubDir)

	if request.Revision == "" {
		request.Revision = r.DefaultRevision
	}

	return request
}

func (r Runner) env() []string {
	return append(os.Environ(),
		"HF_TOKEN="+r.Token,
		"HF_HOME="+r.CacheDir,
		"HF_HUB_DISABLE_PROGRESS_BARS=1",
		"HF_HUB_ENABLE_HF_TRANSFER=0",
		"HF_XET_CACHE="+filepath.Join(r.CacheDir, r.XetDirName),
		"HF_XET_HIGH_PERFORMANCE=1",
	)
}
