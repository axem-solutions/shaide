package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
)

func (c *Client) FetchSizeInfo(ctx context.Context, repoID, revision string) (SizeInfo, error) {
	modelMetadata, err := c.FetchMetadata(ctx, repoID, revision)
	if err != nil {
		return SizeInfo{}, err
	}

	return sizeFromMetadata(modelMetadata), nil
}

func (c *Client) FetchMetadata(ctx context.Context, repoID, revision string) (Metadata, error) {
	request, err := newRequest(repoID, revision, c.defaultRevision)
	if err != nil {
		return Metadata{}, err
	}

	payload, err := c.fetchMetadataResponse(ctx, request)
	if err != nil {
		return Metadata{}, fmt.Errorf("fetch metadata for %s: %w", request.RepoID, err)
	}

	return FromResponse(request.Revision, payload), nil
}

func (c *Client) fetchMetadataResponse(
	ctx context.Context,
	request Request,
) (Response, error) {
	query := url.Values{
		"blobs":    {"true"},
		"revision": {request.Revision},
	}
	path := "/api/models/" + strings.ReplaceAll(url.PathEscape(request.RepoID), "%2F", "/")

	return httpapi.Request[Response](
		ctx,
		c.http,
		http.MethodGet,
		path,
		query,
		nil,
	)
}
