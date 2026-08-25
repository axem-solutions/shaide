package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/bundle"
	orasclient "github.com/axem-solutions/ai_platform/installer/internal/oras/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

const (
	dockerManifestMediaType = "application/vnd.docker.distribution.manifest.v2+json"
	dockerIndexMediaType    = "application/vnd.docker.distribution.manifest.list.v2+json"

	cudaVersionEnv   = "CUDA_VERSION"
	nvidiaRequireEnv = "NVIDIA_REQUIRE_CUDA"
)

type Platform struct {
	OS           string
	Architecture string
	Variant      string
}

func (p Platform) Equal(candidate *ocispec.Platform) bool {
	if candidate == nil {
		return false
	}

	if candidate.OS != p.OS {
		return false
	}

	if candidate.Architecture != p.Architecture {
		return false
	}

	if p.Variant != "" &&
		candidate.Variant != p.Variant {
		return false
	}

	return true
}

// InspectImage is the ORAS equivalent of:
//
// skopeo inspect --config docker://ghcr.io/llm-d/llm-d-cuda:v0.7.0
//
// It resolves the image reference, selects the manifest matching the requested platform
// reads the image configuration and returns the given environment variables as name-value pairs.
func InspectImage(ctx context.Context, image bundle.Image, platform Platform, envVars ...string) (map[string]string, error) {
	client := orasclient.NewClient(orasclient.ClientOptions{})

	repo, err := client.NewSourceRepository(image)
	if err != nil {
		return nil, fmt.Errorf("create remote repository for %s:%s: %w", image.Name, image.Tag, err)
	}

	root, err := repo.Resolve(ctx, image.Tag)
	if err != nil {
		return nil, fmt.Errorf("resolve image %s:%s: %w", image.Name, image.Tag, err)
	}

	descriptor, err := selectDescriptor(ctx, repo, root, platform)
	if err != nil {
		return nil, fmt.Errorf("select image descriptor: %w", err)
	}

	manifest, err := fetchContent[ocispec.Manifest](ctx, repo, descriptor)
	if err != nil {
		return nil, fmt.Errorf("fetch image manifest: %w", err)
	}

	imageConfig, err := fetchContent[ocispec.Image](ctx, repo, manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("fetch image config: %w", err)
	}

	requested := make(map[string]struct{}, len(envVars))
	for _, name := range envVars {
		requested[name] = struct{}{}
	}

	result := make(map[string]string, len(envVars))

	for _, entry := range imageConfig.Config.Env {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}

		if _, ok := requested[name]; ok {
			result[name] = value
		}
	}

	return result, nil
}

func selectDescriptor(ctx context.Context, fetcher content.Fetcher, root ocispec.Descriptor, target Platform) (ocispec.Descriptor, error) {
	switch root.MediaType {
	case ocispec.MediaTypeImageManifest, dockerManifestMediaType:
		return root, nil
	case ocispec.MediaTypeImageIndex, dockerIndexMediaType:
		index, err := fetchContent[ocispec.Index](ctx, fetcher, root)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("fetch index: %w", err)
		}

		for _, candidate := range index.Manifests {
			if target.Equal(candidate.Platform) {
				return candidate, nil
			}
		}

		return ocispec.Descriptor{}, fmt.Errorf("image does not contain platform %s/%s", target.OS, target.Architecture)
	default:
		return ocispec.Descriptor{}, fmt.Errorf("unsupported image media type %q", root.MediaType)
	}
}

func fetchContent[T any](ctx context.Context, fetcher content.Fetcher, descriptor ocispec.Descriptor) (T, error) {
	var result T
	data, err := content.FetchAll(ctx, fetcher, descriptor)
	if err != nil {
		return result, fmt.Errorf("fetch: %w", err)
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("decode: %w", err)
	}

	return result, nil
}
