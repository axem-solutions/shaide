package bootstrap

import (
	"fmt"
	"os"

	"github.com/axem-solutions/ai_platform/installer/internal/config"
	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/config/projects"
	"github.com/axem-solutions/ai_platform/installer/internal/config/storage"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"golang.org/x/term"
)

func Stage() core.Stage {
	return core.Stage{
		Name: "bootstrap",
		Steps: []core.Step{
			{
				Name: "require active terminal",
				Run:  requireActiveTerminal,
			},
			{
				Name: "require valid config",
				Run:  requireValidConfig,
			},
			{
				Name: "require mounted storage",
				Run:  requireMountedStorage,
			},
			{
				Name: "prepare installer storage",
				Run:  prepareStorage,
			},
			{
				Name: "prepare installer assets",
				Run:  prepareAssets,
			},
		},
	}
}

func requireActiveTerminal(rt *core.Runtime) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("not an active terminal")
	}

	return nil
}

func requireMountedStorage(rt *core.Runtime) error {
	storageRoot := rt.Bootstrap.Config.Paths.StorageRoot

	mounted, err := storage.IsMountPoint(storageRoot)
	if err != nil {
		return fmt.Errorf("check storage mount point %q: %w", storageRoot, err)
	}

	if !mounted {
		rt.Detailf("storage root %q is not a mount point", storageRoot)

		confirm, err := rt.Reporter.Select(
			"No persistent storage under installer. Are you sure you want to continue?",
			"No",
			[]string{"No", "Yes"},
		)
		if err != nil {
			return err
		}

		if confirm == "Yes" {
			return nil
		}

		return fmt.Errorf("persistent storage mount is required to continue")
	}

	rt.Detailf("storage root %q is a mount point", storageRoot)

	return nil
}

func prepareStorage(rt *core.Runtime) error {
	storageDirs := rt.Bootstrap.Config.Paths.StorageDirs()
	tmpDir := rt.Bootstrap.Config.Paths.Temp

	if err := storage.EnsureDirs(storageDirs); err != nil {
		return err
	}
	if err := storage.UseTempDir(tmpDir); err != nil {
		return fmt.Errorf("configure installer temp storage: %w", err)
	}

	rt.Detailf("installer storage directories are ready; temp=%q", tmpDir)
	return nil
}

func prepareAssets(rt *core.Runtime) error {
	paths := rt.Bootstrap.Config.Paths

	if err := projects.Prepare(projects.PrepareOptions{
		SourceDir:      paths.ProjectsSourceDir,
		DestinationDir: paths.ProjectsDir,
	}); err != nil {
		return fmt.Errorf("prepare Pulumi projects: %w", err)
	}

	loadedCatalog, err := catalog.Load(
		catalog.LoadOptions{
			ImageManifestPath: paths.ImageManifestPath,
			ModelManifestPath: paths.ModelManifestPath,
			ImagesDir:         paths.ImagesDir,
		})
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	rt.Bootstrap.Catalog = loadedCatalog

	return nil
}

func requireValidConfig(rt *core.Runtime) error {
	cfg := config.Load()

	rt.Bootstrap.Config = cfg

	token, err := requireInput(rt, rt.Bootstrap.Config.HuggingFace.Token, "Hugging Face token")
	if err != nil {
		return err
	}

	rt.Bootstrap.Config.HuggingFace.Token = token

	passphrase, err := requireInput(rt, rt.Bootstrap.Config.Pulumi.ConfigPassphrase, "Pulumi passphrase")
	if err != nil {
		return err
	}

	rt.Bootstrap.Config.Pulumi.ConfigPassphrase = passphrase

	return nil
}

func requireInput(rt *core.Runtime, current, label string) (string, error) {
	if current != "" {
		return current, nil
	}

	for {
		value, err := rt.Reporter.Input(label, "", "")
		if err != nil {
			return "", err
		}

		if value != "" {
			return value, nil
		}
	}
}
