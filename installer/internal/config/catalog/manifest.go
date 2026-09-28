package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

// Model is a supported model packaged with the installer, read from its
// deployments/models/<category>/<Name>/ms-<slug>/values.yaml.
type Model struct {
	// Name is the model's directory, for example "GPT-OSS-20B".
	Name string
	// Category is the directory above it: CategoryGenerative or
	// CategoryEmbedder.
	Category string
	// Slug is the ms-<slug> suffix. app-serving names the model's namespace,
	// releases and pod labels after it.
	Slug string

	// ID and Revision pin the Hugging Face repository to download.
	ID           string
	Revision     string
	Dependencies []Dependency

	// StorageSize is the model volume size, for example "70Gi".
	StorageSize string
	// GPUsPerPod and Replicas describe what serving the model needs.
	GPUsPerPod int
	Replicas   int

	// Where the model artifact lives in Harbor, derived from Slug and Revision.
	HarborProject string
	HarborName    string
	HarborTag     string
}

type Dependency struct {
	ID       string `yaml:"id"`
	Revision string `yaml:"revision"`
}

type ImageSource string

const (
	ImageSourceArchive     ImageSource = "archive"
	ImageSourceDockerHub   ImageSource = "dockerhub"
	ImageSourceGitHub      ImageSource = "ghcr"
	ImageSourceNVCR        ImageSource = "nvcr"
	ImageSourceQuay        ImageSource = "quay"
	ImageSourceRegistryK8s ImageSource = "registry_k8s"
)

type Image struct {
	Source  ImageSource `yaml:"source"`
	Project string      `yaml:"project"`
	Name    string      `yaml:"name"`
	Tag     string      `yaml:"tag"`
	Size    int64       `yaml:"-"`
}

type imageManifest struct {
	Services []Image `yaml:"harbor_upload_images"`
	Harbor   []Image `yaml:"goharbor_images"`
}

func readManifest[T any](path string) (T, error) {
	var manifest T

	file, err := os.Open(path)
	if err != nil {
		return manifest, fmt.Errorf("open manifest %s: %w", path, err)
	}
	defer file.Close()

	if err := yaml.NewDecoder(file).Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("parse manifest %s: %w", path, err)
	}

	return manifest, nil
}

func resolveImageSizes(images []Image, imagesDir string) error {
	for i := range images {
		if images[i].Source != ImageSourceArchive {
			continue
		}

		archivePath := filepath.Join(imagesDir, images[i].FileName())

		info, err := os.Stat(archivePath)
		if err != nil {
			return fmt.Errorf("stat archive for image %s: %w", images[i].Ref(), err)
		}

		if info.IsDir() {
			return fmt.Errorf("archive for image %s is not a file: %s", images[i].Ref(), archivePath)
		}

		images[i].Size = info.Size()
	}

	return nil
}

func (i Image) Ref() string {
	return fmt.Sprintf("%s/%s:%s", i.Project, i.Name, i.Tag)
}

func (i Image) FileName() string {
	name := strings.ReplaceAll(i.Name, "/", "-")
	return fmt.Sprintf("%s-%s.tar", name, i.Tag)
}
