package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"

	localerrdef "github.com/axem-solutions/ai_platform/installer/internal/oras/errdef"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func (r *Repository) blobUploadURL() string {
	return fmt.Sprintf(
		"%s://%s/v2/%s/blobs/uploads/",
		r.scheme(),
		r.Repo.Reference.Registry,
		r.Repo.Reference.Repository,
	)
}

func (r *Repository) postBlobUpload(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.blobUploadURL(), nil)
	if err != nil {
		return "", fmt.Errorf("create start-upload request: %w", err)
	}

	resp, err := r.doReq(req, http.StatusAccepted)
	if err != nil {
		return "", fmt.Errorf("start blob upload: %w", err)
	}
	defer resp.Body.Close()

	location, err := resp.Location()
	if err != nil {
		return "", err
	}

	return location.String(), nil
}

func (r *Repository) patchBlobUpload(ctx context.Context, chunk *chunk, content *chunkContent) (int64, string, error) {
	newBody := func() (io.ReadCloser, error) {
		return content.Open()
	}

	body, err := newBody()
	if err != nil {
		return 0, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, chunk.Location, body)
	if err != nil {
		_ = body.Close()
		return 0, "", fmt.Errorf("create patch-upload request: %w", err)
	}

	req.GetBody = newBody
	req.ContentLength = content.size
	// Registry upload PATCH URLs are stateful. If the response is lost after
	// Harbor commits the bytes, replaying the same URL/body will fail because
	// the upload offset has already advanced.
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", chunk.Offset, chunk.End))

	resp, err := r.doReq(req, http.StatusAccepted)
	if err != nil {
		return 0, "", fmt.Errorf("upload blob chunk: %w", err)
	}
	defer resp.Body.Close()

	nextOffset, err := nextOffsetFromRange(resp.Header.Get("Range"))
	if err != nil {
		return 0, "", fmt.Errorf("parse upload range: %w", err)
	}

	nextLocation, err := resp.Location()
	if err != nil {
		return 0, "", err
	}

	return nextOffset, nextLocation.String(), nil
}

func (r *Repository) putBlobUpload(ctx context.Context, location string, desc ocispec.Descriptor) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, location, nil)
	if err != nil {
		return fmt.Errorf("create complete-upload request: %w", err)
	}

	query := req.URL.Query()
	query.Set("digest", desc.Digest.String())
	req.URL.RawQuery = query.Encode()

	resp, err := r.doReq(req, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("complete blob upload: %w", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Docker-Content-Digest"); got != "" && got != desc.Digest.String() {
		return fmt.Errorf("registry committed unexpected digest: got=%s want=%s", got, desc.Digest)
	}

	return nil
}

func (r *Repository) getBlobUpload(ctx context.Context, location string) (int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return 0, "", fmt.Errorf("create upload-status request: %w", err)
	}

	resp, err := r.doReq(req, http.StatusNoContent)
	if err != nil {
		return 0, "", fmt.Errorf("get blob upload status: %w", err)
	}
	defer resp.Body.Close()

	nextOffset, err := nextOffsetFromRange(resp.Header.Get("Range"))
	if err != nil {
		return 0, "", fmt.Errorf("parse upload range: %w", err)
	}

	nextLocation, err := resp.Location()
	if err != nil {
		return 0, "", err
	}

	return nextOffset, nextLocation.String(), nil
}

func (r *Repository) doReq(req *http.Request, expectedStatus int) (*http.Response, error) {
	resp, err := r.Repo.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL, err)
	}

	if resp.StatusCode == expectedStatus {
		return resp, nil
	}

	defer resp.Body.Close()
	return nil, localerrdef.ParseErrorResponse(resp)
}
