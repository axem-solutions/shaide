package preloader

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/preloader/remote"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
)

type Preloader struct {
	opts   PreloaderOptions
	remote remote.RemoteClient
}

func NewPreloader(opts PreloaderOptions) (*Preloader, error) {
	opts = applyOptions(opts)

	if err := validateOptions(opts); err != nil {
		return nil, err
	}

	remoteClient, err := remote.NewSSHClient(remote.SSHOptions{
		Host:           opts.Host,
		User:           opts.User,
		PrivateKeyPath: opts.PrivateKeyPath,
		Port:           opts.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("error initializing SSH client: %w", err)
	}

	return &Preloader{
		opts:   opts,
		remote: remoteClient,
	}, nil
}

func (p *Preloader) Close() error {
	if p == nil || p.remote == nil {
		return nil
	}

	return p.remote.Close()
}

func (p *Preloader) Preload(ctx context.Context, images []catalog.Image) error {
	harborImages, _, err := p.build(ctx, images)
	if err != nil {
		return err
	}

	return p.uploadImages(ctx, harborImages)
}

func (p *Preloader) build(ctx context.Context, images []catalog.Image) ([]catalog.Image, int64, error) {
	var totalBytes int64
	for _, image := range images {
		totalBytes += image.Size
	}

	if err := p.remoteHealthCheck(ctx); err != nil {
		return nil, -1, err
	}

	if err := p.checkDiskSpace(ctx, totalBytes); err != nil {
		return nil, -1, err
	}

	nodeImages, err := p.listImages(ctx)
	if err != nil {
		return nil, -1, err
	}

	missingImages := make([]catalog.Image, 0, len(images))

	for _, harborImage := range images {
		if _, ok := nodeImages[harborImage.Ref()]; !ok {
			p.opts.Logf("not present %s", harborImage.Ref())
			missingImages = append(missingImages, harborImage)
		}
	}

	return missingImages, totalBytes, nil
}

func (p *Preloader) uploadImages(ctx context.Context, images []catalog.Image) error {
	if len(images) == 0 {
		return nil
	}

	if err := p.prepareStageDir(ctx, p.opts.StagingDir); err != nil {
		return err
	}

	defer func() {
		if err := p.cleanStageDir(ctx, p.opts.StagingDir); err != nil {
			p.opts.Logf("%v", err)
		}
	}()

	return p.uploadAndImport(ctx, images)
}

func (p *Preloader) prepareStageDir(ctx context.Context, stageDir string) error {
	commands := []remote.Command{
		{Program: "sudo", Args: []string{"-n", "mkdir", "-p", stageDir}},
		{Program: "sudo", Args: []string{"-n", "chown", p.opts.User + ":", stageDir}},
		{Program: "sudo", Args: []string{"-n", "chmod", "700", stageDir}},
	}

	for _, command := range commands {
		stdout, stderr, err := p.remote.Run(ctx, command)
		if err != nil {
			return fmt.Errorf(
				"prepare staging directory %s failed on %q: %w; stdout=%q stderr=%q",
				stageDir,
				command.String(),
				err,
				stdout,
				stderr,
			)
		}
	}

	return nil
}

func (p *Preloader) cleanStageDir(ctx context.Context, stageDir string) error {
	if stageDir == "/" {
		return fmt.Errorf("invalid staging directory: %q", stageDir)
	}

	stdout, stderr, err := p.remote.Run(ctx, remote.Command{
		Program: "sudo",
		Args:    []string{"-n", "rm", "-rf", stageDir},
	})
	if err != nil {
		return fmt.Errorf(
			"clean staging directory %s failed: %w; stdout=%q stderr=%q",
			stageDir,
			err,
			stdout,
			stderr,
		)
	}

	return nil
}

func (p *Preloader) uploadAndImport(ctx context.Context, images []catalog.Image) error {
	for _, image := range images {
		remotePath, err := p.toNode(ctx, image)
		if err != nil {
			return err
		}

		if err := p.toContainerd(ctx, image, remotePath); err != nil {
			return err
		}
	}

	return nil
}

func (p *Preloader) toNode(ctx context.Context, image catalog.Image) (string, error) {
	if image.Source != catalog.ImageSourceArchive {
		return "", fmt.Errorf("unsupported Harbor image source %q", image.Source)
	}

	file := image.FileName()
	localPath := filepath.Join(p.opts.LocalDir, file)
	remotePath := filepath.Join(p.opts.StagingDir, image.Ref())

	p.opts.Logf("uploading Harbor image archive %s to %s", localPath, remotePath)

	tracker := p.startProgress(image.Ref(), progress.PhaseUploading, image.Size, 0)
	if err := p.remote.Upload(ctx, localPath, remotePath, 0644, tracker); err != nil {
		tracker.Stop()
		return "", fmt.Errorf(
			"upload Harbor image archive %s to %s failed: %w",
			localPath,
			remotePath,
			err,
		)
	}
	tracker.Finish()

	return remotePath, nil

}

func (p *Preloader) toContainerd(ctx context.Context, image catalog.Image, remotePath string) error {
	p.opts.Logf("importing Harbor image archive %s into containerd", remotePath)
	tracker := p.startProgress(image.Ref(), progress.PhaseImporting, 0, 1)

	stdout, stderr, err := p.remote.Run(ctx, p.ctr("images", "import", remotePath))
	if err != nil {
		tracker.Stop()
		return fmt.Errorf(
			"import Harbor image archive %s into containerd failed: %w; stdout=%q stderr=%q",
			remotePath,
			err,
			stdout,
			stderr,
		)
	}

	tracker.AddFiles(1)
	tracker.Finish()

	return nil
}

func (p *Preloader) startProgress(
	current string,
	phase progress.Phase,
	totalBytes int64,
	totalFiles int,
) *progress.Tracker {
	tracker := progress.NewTracker(
		phase,
		current,
		totalBytes,
		totalFiles,
		p.opts.Progressf,
	).Throttle(progress.DefaultReportEvery)

	tracker.Start()
	return tracker
}

func (p *Preloader) remoteHealthCheck(ctx context.Context) error {
	checks := []struct {
		name string
		fn   func() error
	}{
		{"SSH connectivity", func() error { return p.checkSSH(ctx) }},
		{"Ctr binary", func() error { return p.checkCtr(ctx) }},
		{"Containerd socket", func() error { return p.checkContainerdSocket(ctx) }},
		{"containerd version", func() error { return p.checkVersion(ctx) }},
	}

	for _, check := range checks {
		p.opts.Logf("checking %s", check.name)
		if err := check.fn(); err != nil {
			return fmt.Errorf("%s check failed: %w", check.name, err)
		}
	}
	return nil
}

func (p *Preloader) checkSSH(ctx context.Context) error {
	if stdout, stderr, err := p.remote.Run(ctx, remote.Command{Program: "true"}); err != nil {
		return fmt.Errorf("%w; stdout=%q stderr=%q", err, stdout, stderr)
	}
	return nil
}

func (p *Preloader) checkCtr(ctx context.Context) error {
	stdout, stderr, err := p.remote.Run(ctx, remote.Command{
		Program: "test",
		Args:    []string{"-x", p.opts.CtrPath},
	})
	if err != nil {
		return fmt.Errorf("%s is not executable: %w; stdout=%q stderr=%q", p.opts.CtrPath, err, stdout, stderr)
	}
	return nil
}

func (p *Preloader) checkContainerdSocket(ctx context.Context) error {
	stdout, stderr, err := p.remote.Run(ctx, remote.Command{
		Program: "test",
		Args:    []string{"-S", p.opts.ContainerdSocket},
	})
	if err != nil {
		return fmt.Errorf("not a socket: %s; stdout=%q stderr=%q", p.opts.ContainerdSocket, stdout, stderr)
	}
	return nil
}

func (p *Preloader) checkDiskSpace(ctx context.Context, requiredBytes int64) error {
	if requiredBytes < 0 {
		return nil
	}

	if stdout, stderr, err := p.remote.Run(ctx, remote.Command{
		Program: "sudo",
		Args:    []string{"-n", "mkdir", "-p", p.opts.StagingDir},
	}); err != nil {
		return fmt.Errorf("create staging base directory %s failed: %w; stdout=%q stderr=%q",
			p.opts.StagingDir,
			err,
			stdout,
			stderr,
		)
	}

	stdout, stderr, err := p.remote.Run(ctx, remote.Command{
		Program: "df",
		Args:    []string{"-B1", "--output=avail", p.opts.StagingDir},
	})
	if err != nil {
		return fmt.Errorf("check free disk under %s failed: %w; stdout=%q stderr=%q",
			p.opts.StagingDir,
			err,
			stdout,
			stderr,
		)
	}

	freeBytes, err := parseAvailableBytes(stdout)
	if err != nil {
		return fmt.Errorf("parse free disk bytes from df output %q: %w", stdout, err)
	}
	p.opts.Logf("free space: %d", freeBytes)

	if freeBytes < requiredBytes {
		return fmt.Errorf("insufficient disk space on Harbor node: free=%d required=%d staging_dir=%s",
			freeBytes,
			requiredBytes,
			p.opts.StagingDir,
		)
	}

	return nil
}

func (p *Preloader) checkVersion(ctx context.Context) error {
	stdout, stderr, err := p.remote.Run(ctx, p.ctr("version"))
	if err != nil {
		return fmt.Errorf("ctr cannot access containerd: %w, stdout=%q, stderr=%q",
			err,
			stdout,
			stderr,
		)
	}

	return nil
}

func parseAvailableBytes(raw string) (int64, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("expected df output with header and value, got %d line(s)", len(lines))
	}

	return strconv.ParseInt(strings.TrimSpace(lines[1]), 10, 64)
}

func (p *Preloader) ctr(args ...string) remote.Command {
	base := []string{
		"-n",
		p.opts.CtrPath,
		"--address",
		p.opts.ContainerdSocket,
		"--namespace",
		p.opts.ContainerdNamespace,
	}

	base = append(base, args...)

	return remote.Command{
		Program: "sudo",
		Args:    base,
	}
}

func (p *Preloader) listImages(ctx context.Context) (map[string]struct{}, error) {
	stdout, stderr, err := p.remote.Run(ctx, p.ctr("images", "list", "-q"))
	if err != nil {
		return nil, fmt.Errorf("list containerd images: %w; stdout=%q stderr=%q", err, stdout, stderr)
	}
	return parseNodeImages(stdout), nil
}

func parseNodeImages(raw string) map[string]struct{} {
	images := make(map[string]struct{})

	for line := range strings.SplitSeq(raw, "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" {
			continue
		}

		images[ref] = struct{}{}
	}

	return images
}
