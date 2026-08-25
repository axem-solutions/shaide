package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/bundle"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/repository"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	DockerHubRegistry   = "registry-1.docker.io"
	DockerHubAlias      = "docker.io"
	GHCRRegistry        = "ghcr.io"
	NVCRRegistry        = "nvcr.io"
	QuayRegistry        = "quay.io"
	RegistryK8sRegistry = "registry.k8s.io"
)

type Client struct {
	registry string
	http     *auth.Client
}

type ClientOptions struct {
	Registry string

	TargetCredentials Credential
	RemoteCredentials map[string]Credential
}

type Credential struct {
	Username string
	Password string
}

func NewClient(opts ClientOptions) *Client {
	return &Client{
		registry: opts.Registry,
		http:     newAuthClient(opts),
	}
}

func newAuthClient(opts ClientOptions) *auth.Client {
	credentials := registryCredentials(opts)

	return &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		// Credential is called by ORAS when a registry request requires authentication.
		// registry is the request host[:port]. Return EmptyCredential when no matching
		// credentials are configured so ORAS can attempt anonymous authentication.
		Credential: func(ctx context.Context, registry string) (auth.Credential, error) {
			credential, ok := lookup(credentials, registry)
			if !ok {
				return auth.EmptyCredential, nil
			}
			return authCredential(credential), nil
		},
	}
}

func registryCredentials(opts ClientOptions) map[string]Credential {
	credentials := map[string]Credential{}

	if hasCredential(opts.TargetCredentials) {
		credentials[opts.Registry] = opts.TargetCredentials
	}

	for registry, credential := range opts.RemoteCredentials {
		if hasCredential(credential) {
			credentials[registry] = credential
		}
	}
	return credentials
}

func lookup(credentials map[string]Credential, registry string) (Credential, bool) {
	credential, ok := credentials[registry]
	if ok {
		return credential, true
	}

	return Credential{}, false
}

func hasCredential(credential Credential) bool {
	return credential.Username != "" && credential.Password != ""
}

func authCredential(credential Credential) auth.Credential {
	return auth.Credential{
		Username: credential.Username,
		Password: credential.Password,
	}
}

func (c *Client) NewTargetRepository(project string, repositoryName string, opts repository.ChunkedUploadOptions) (*repository.Repository, error) {
	ref := fmt.Sprintf("%s/%s/%s", c.registry, project, repositoryName)

	repo, err := c.remoteRepository(ref, true)
	if err != nil {
		return nil, err
	}

	return repository.New(repo, opts), nil
}

func (c *Client) NewSourceRepository(image bundle.Image) (*remote.Repository, error) {
	registry, err := ParseRemote(image)
	if err != nil {
		return nil, err
	}

	repo, err := c.remoteRepository(registry, false)
	if err != nil {
		return nil, err
	}

	return repo, nil
}

func (c *Client) remoteRepository(ref string, plainHTTP bool) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid repository reference %s: %w", ref, err)
	}

	repo.Client = c.http
	repo.PlainHTTP = plainHTTP

	return repo, nil
}

// Ping verifies the registry can issue a repository-scoped pull token for the
// given repo and answer a manifest lookup. It deliberately exercises the same
// auth path as the real workload (a repository-scoped token), NOT a bare /v2/
// catalog request: Harbor grants the scopeless catalog token only to
// admin-level accounts and always denies it (401) to a repository-scoped robot
// account, so a /v2/ probe can never succeed with robot credentials. A nil
// return means the token service is up and the credentials are accepted; a 404
// (manifest absent) still counts as ready because the scoped token was issued.
// Warmup or outage surfaces as a non-nil error (401/5xx/timeout) for retry.
func (c *Client) Ping(ctx context.Context, project, name, tag string) error {
	repo, err := c.NewTargetRepository(project, name, repository.ChunkedUploadOptions{})
	if err != nil {
		return err
	}
	if _, err := repo.ManifestExists(ctx, tag); err != nil {
		return fmt.Errorf("registry token check %s/%s:%s: %w", project, name, tag, err)
	}
	return nil
}

func ParseRemote(image bundle.Image) (string, error) {
	switch image.Source {
	case bundle.ImageSourceDockerHub:
		return fmt.Sprintf("%s/%s", DockerHubRegistry, dockerHubRepository(image.Name)), nil
	case bundle.ImageSourceGitHub:
		return fmt.Sprintf("%s/%s", GHCRRegistry, image.Name), nil
	case bundle.ImageSourceNVCR:
		return fmt.Sprintf("%s/%s", NVCRRegistry, image.Name), nil
	case bundle.ImageSourceQuay:
		return fmt.Sprintf("%s/%s", QuayRegistry, image.Name), nil
	case bundle.ImageSourceRegistryK8s:
		return fmt.Sprintf("%s/%s", RegistryK8sRegistry, image.Name), nil
	default:
		return "", fmt.Errorf("unsupported remote image source %q", image.Source)
	}
}

func dockerHubRepository(name string) string {
	if strings.Contains(name, "/") {
		return name
	}

	return "library/" + name
}
