package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

type Model struct {
	ID            string       `yaml:"id"`
	Revision      string       `yaml:"revision"`
	HarborProject string       `yaml:"harbor_project"`
	HarborName    string       `yaml:"harbor_name"`
	HarborTag     string       `yaml:"harbor_tag"`
	Dependencies  []Dependency `yaml:"dependencies,omitempty"`

	// Serving describes how to run the model on the cluster. A model without
	// it is published to Harbor but not deployed, which is what a cluster that
	// mirrors models for another consumer wants.
	Serving *Serving `yaml:"serving,omitempty"`
}

// Serving holds what the model manifest cannot derive: which packaged values
// directory to deploy, and the two facts that depend on the target cluster.
type Serving struct {
	// Name is the directory under deployments/models/<category>/, for example
	// "GPT-OSS-20B". It also decides the category, since the installer looks
	// the name up among the packaged model directories.
	Name string `yaml:"name"`

	// NodeSelector is the nodegroup label value of the pool to run on.
	NodeSelector string `yaml:"node_selector"`

	// StorageSize is the model volume size, for example "70Gi".
	StorageSize string `yaml:"storage_size"`

	// StorageClass overrides the cluster default for this model's volume.
	StorageClass string `yaml:"storage_class,omitempty"`
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

type modelManifest struct {
	Models []Model `yaml:"models"`
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
