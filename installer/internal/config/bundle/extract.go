package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/progress"
)

const (
	defaultDeploymentDir = "deployments"
	defaultImageDir      = "images"
	defaultManifestDir   = "manifests"
	defaultImageFile     = "images.yaml"
	defaultModelFile     = "models.yaml"

	bundleChecksumFile = "checksum.json"
)

type extractedBundle struct {
	ImagesDir         string
	PulumiWorkDir     string
	ModelManifestPath string
	ImageManifestPath string
}

type extractOptions struct {
	archivePath    string
	destinationDir string
	Logf           func(format string, args ...any)
	Progressf      func(progress.Event)
}

func bundlePaths(destinationDir string) extractedBundle {
	return extractedBundle{
		ImagesDir:         filepath.Join(destinationDir, defaultImageDir),
		PulumiWorkDir:     filepath.Join(destinationDir, defaultDeploymentDir),
		ModelManifestPath: filepath.Join(destinationDir, defaultManifestDir, defaultModelFile),
		ImageManifestPath: filepath.Join(destinationDir, defaultManifestDir, defaultImageFile),
	}
}

func (e extractedBundle) Validate() error {
	if err := checkDir(e.ImagesDir); err != nil {
		return err
	}

	if err := checkDir(e.PulumiWorkDir); err != nil {
		return err
	}

	if err := checkFile(e.ImageManifestPath); err != nil {
		return err
	}

	if err := checkFile(e.ModelManifestPath); err != nil {
		return err
	}

	return nil

}

func (opts extractOptions) validate() error {
	if opts.archivePath == "" {
		return fmt.Errorf("archive path is required")
	}

	if opts.destinationDir == "" {
		return fmt.Errorf("destination directory is required")
	}

	return nil
}

func extractBundle(opts extractOptions) (extractedBundle, error) {
	if err := opts.validate(); err != nil {
		return extractedBundle{}, fmt.Errorf("invalid extract options: %w", err)
	}

	if err := untarBundle(opts); err != nil {
		return extractedBundle{}, fmt.Errorf("extract bundle: %w", err)
	}

	bundle := bundlePaths(opts.destinationDir)

	if err := bundle.Validate(); err != nil {
		if err := resetDestination(opts.destinationDir); err != nil {
			return extractedBundle{}, err
		}
		return extractedBundle{}, fmt.Errorf("bundle is not valid: %w", err)
	}

	return bundle, nil
}

func untarBundle(opts extractOptions) error {
	file, err := os.Open(opts.archivePath)
	if err != nil {
		return fmt.Errorf("open bundle archive %s: %w", opts.archivePath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat bundle archive: %w", err)
	}

	tracker := progress.NewTracker(
		progress.PhaseExtracting,
		filepath.Base(opts.archivePath),
		info.Size(),
		0,
		opts.Progressf,
	).Throttle(progress.DefaultReportEvery)
	tracker.Start()

	gzipReader, err := gzip.NewReader(tracker.Reader(file))
	if err != nil {
		return fmt.Errorf("open bundle gzip stream: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	return extractTar(tarReader, opts)
}

func extractTar(tarReader *tar.Reader, opts extractOptions) error {
	reuse, checksum, err := checksumEntry(tarReader, opts)
	if err != nil {
		return fmt.Errorf("check bundle checksum: %w", err)
	}

	if reuse {
		return nil
	}

	if err := resetDestination(opts.destinationDir); err != nil {
		return err
	}

	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("read tar entry: %w", err)
		}

		targetPath, err := safeTargetPath(opts.destinationDir, header.Name)
		if err != nil {
			return fmt.Errorf("safe target path: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := extractFile(tarReader, targetPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported tar entry %q", header.Name)
		}
	}

	if err := os.WriteFile(filepath.Join(opts.destinationDir, bundleChecksumFile), checksum, 0o644); err != nil {
		return fmt.Errorf("write bundle checksum: %w", err)
	}

	return nil
}

func extractFile(reader io.Reader, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory %s: %w", filepath.Dir(targetPath), err)
	}

	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", targetPath, err)
	}

	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()

	if copyErr != nil {
		return fmt.Errorf("extract file %s: %w", targetPath, copyErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close file %s: %w", targetPath, closeErr)
	}

	return nil
}

func checksumEntry(tarReader *tar.Reader, opts extractOptions) (bool, []byte, error) {
	header, err := tarReader.Next()
	if err != nil {
		return false, nil, fmt.Errorf("read checksum entry: %w", err)
	}

	if filepath.Clean(header.Name) != bundleChecksumFile {
		return false, nil, fmt.Errorf("%s must be the first bundle entry", bundleChecksumFile)
	}

	if header.Typeflag != tar.TypeReg {
		return false, nil, fmt.Errorf("%s must be a regular file", bundleChecksumFile)
	}

	checksumData, err := io.ReadAll(tarReader)
	if err != nil {
		return false, nil, fmt.Errorf("read bundle checksum: %w", err)
	}

	existingChecksum, err := os.ReadFile(filepath.Join(opts.destinationDir, bundleChecksumFile))
	if err != nil {
		if os.IsNotExist(err) {
			return false, checksumData, nil
		}
		return false, nil, fmt.Errorf("read extracted checksum: %w", err)
	}

	return bytes.Equal(checksumData, existingChecksum), checksumData, nil

}

func safeTargetPath(destinationDir, entryName string) (string, error) {
	if filepath.IsAbs(entryName) {
		return "", fmt.Errorf("tar entry %q uses an absolute path", entryName)
	}

	targetPath := filepath.Join(destinationDir, filepath.Clean(entryName))

	relative, err := filepath.Rel(destinationDir, targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve tar entry %q: %w", entryName, err)
	}

	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("tar entry %q escapes destination directory", entryName)
	}

	return targetPath, nil
}

func resetDestination(destinationDir string) error {
	if err := os.RemoveAll(destinationDir); err != nil {
		return fmt.Errorf("remove existing bundle %s: %w", destinationDir, err)
	}

	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return fmt.Errorf("create bundle directory %s: %w", destinationDir, err)
	}

	return nil
}

func checkFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat file %s: %w", path, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%s is not a file", path)
	}

	return nil
}

func checkDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat directory %s: %w", path, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	return nil
}
