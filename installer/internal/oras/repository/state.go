package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type uploadState struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
	Digest     string `json:"digest"`
	Size       int64  `json:"size"`
	Location   string `json:"location"`

	// LastKnownOffset is only diagnostic/progress metadata.
	// Do not use it as the source of truth for resume.
	LastKnownOffset int64 `json:"lastKnownOffset"`
}

func (r *Repository) loadUploadState(desc ocispec.Descriptor) (*uploadState, error) {
	data, err := os.ReadFile(r.statePath(desc))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var state uploadState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if !r.validUploadState(&state, desc) {
		return nil, nil
	}

	return &state, nil
}

func (r *Repository) saveUploadState(desc ocispec.Descriptor, uploadOffset int64, uploadLocation string) error {
	state := uploadState{
		Registry:        r.Repo.Reference.Registry,
		Repository:      r.Repo.Reference.Repository,
		Digest:          desc.Digest.String(),
		Size:            desc.Size,
		Location:        uploadLocation,
		LastKnownOffset: uploadOffset,
	}

	if err := os.MkdirAll(r.opts.StateDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.statePath(desc), data, 0o644)
}

func (r *Repository) deleteUploadState(desc ocispec.Descriptor) error {
	err := os.Remove(r.statePath(desc))
	if err == nil || os.IsNotExist(err) {
		return nil
	}

	return err
}

func (r *Repository) validUploadState(state *uploadState, desc ocispec.Descriptor) bool {
	if state.Registry != r.Repo.Reference.Registry {
		return false
	}
	if state.Repository != r.Repo.Reference.Repository {
		return false
	}
	if state.Digest != desc.Digest.String() {
		return false
	}
	if state.Size != desc.Size {
		return false
	}
	if state.Location == "" {
		return false
	}

	return true
}

func (r *Repository) statePath(desc ocispec.Descriptor) string {
	name := strings.ReplaceAll(desc.Digest.String(), ":", "-") + ".json"
	return filepath.Join(r.opts.StateDir, name)
}

func (s *uploadState) String() string {
	return fmt.Sprintf(
		"digest=%s size=%d location=%s lastKnownOffset=%d",
		s.Digest,
		s.Size,
		s.Location,
		s.LastKnownOffset,
	)
}
