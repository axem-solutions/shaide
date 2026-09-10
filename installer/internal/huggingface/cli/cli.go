package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/resources"
	hferrors "github.com/axem-solutions/ai_platform/installer/internal/huggingface/errors"
)

type Config struct {
	Path            string
	CacheDir        string
	Token           string
	XetDirName      string
	DefaultRevision string

	// Budget bounds what the transfer may take of the machine. The zero value
	// leaves Xet to size itself, which is what it did before this was
	// configurable.
	Budget resources.Budget

	// HighPerformance opts into Xet's maximum-throughput mode. It ignores the
	// budget and scales to the whole host, so it belongs on a machine doing
	// nothing else.
	HighPerformance bool
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
	env := append(os.Environ(),
		"HF_TOKEN="+r.Token,
		"HF_HOME="+r.CacheDir,
		"HF_HUB_DISABLE_PROGRESS_BARS=1",
		"HF_HUB_ENABLE_HF_TRANSFER=0",
		"HF_XET_CACHE="+filepath.Join(r.CacheDir, r.XetDirName),
	)

	return append(env, r.transferEnv()...)
}

// transferEnv bounds what Xet takes of the machine.
//
// Left alone Xet sizes its worker pools and buffers from the host, and inside a
// container the host is not what the process may actually use: --cpus sets a
// time quota while every CPU stays visible, so a pool sized from the CPU count
// oversubscribes the quota and spends its time being throttled.
//
// High performance mode is the documented way to ask for everything, so it
// suppresses the caps rather than fighting them.
func (r Runner) transferEnv() []string {
	if r.HighPerformance {
		return []string{"HF_XET_HIGH_PERFORMANCE=1"}
	}

	if r.Budget.CPUs <= 0 {
		return nil
	}

	return []string{
		// Files fetched at once. Each carries its own buffers, so this is the
		// main lever on how much memory a transfer holds.
		fmt.Sprintf("HF_XET_DATA_MAX_CONCURRENT_FILE_DOWNLOADS=%d", r.Budget.CPUs),

		// Range requests in flight across those files.
		fmt.Sprintf("HF_XET_CLIENT_AC_MAX_DOWNLOAD_CONCURRENCY=%d", r.Budget.CPUs*rangeGetsPerCPU),

		// The on-disk chunk cache. Bounded by the memory share because the same
		// budget stands in for how much of the machine this transfer may take.
		fmt.Sprintf("HF_XET_CHUNK_CACHE_SIZE_BYTES=%d", r.Budget.Memory),
	}
}

// rangeGetsPerCPU keeps enough requests in flight to hide network latency
// without multiplying the buffers each one needs.
const rangeGetsPerCPU = 4
