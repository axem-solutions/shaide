package projects

import (
	"fmt"
	"os"
	"path/filepath"
)

type PrepareOptions struct {
	// SourceDir contains the immutable Pulumi projects packaged in the installer image.
	SourceDir string

	// DestinationDir is the writable Pulumi projects directory on mounted storage.
	DestinationDir string
}

func (opts PrepareOptions) Validate() error {
	if opts.SourceDir == "" {
		return fmt.Errorf("projects source directory is required")
	}

	if opts.DestinationDir == "" {
		return fmt.Errorf("projects destination directory is required")
	}

	return nil
}

// Prepare replaces the runtime Pulumi projects with the projects packaged
// in the installer image, then validates the expected files.
func Prepare(opts PrepareOptions) error {
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("invalid prepare options: %w", err)
	}

	srcDir := filepath.Clean(opts.SourceDir)
	dstDir := filepath.Clean(opts.DestinationDir)

	info, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("stat projects source %q: %w", srcDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("projects source %q is not a directory", srcDir)
	}

	// Runtime project files are disposable. Persistent Pulumi stack config and
	// backend state live outside this directory and are not affected.
	if err := os.RemoveAll(dstDir); err != nil {
		return fmt.Errorf("remove projects destination %q: %w", dstDir, err)
	}

	if err := os.CopyFS(dstDir, os.DirFS(srcDir)); err != nil {
		return fmt.Errorf("copy projects from %q to %q: %w", srcDir, dstDir, err)
	}

	return Validate(dstDir)
}

// Validate ensures every Pulumi project expected by the installer is present.
func Validate(dstDir string) error {
	required := []string{
		"app-serving/Pulumi.yaml",
		"app-shaide/Pulumi.yaml",
		"cloud-harbor/Pulumi.yaml",
		"gateway-provider/Pulumi.yaml",
		"monitoring/Pulumi.yaml",
	}

	for _, relativePath := range required {
		path := filepath.Join(dstDir, relativePath)

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("validate project file %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("project file %q is not a regular file", path)
		}
	}

	return nil
}
