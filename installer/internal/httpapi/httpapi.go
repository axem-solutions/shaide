package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
	Configure  func(*http.Request)
}

func Request[T any](
	ctx context.Context,
	client *Client,
	method, path string,
	query url.Values,
	body any,
) (T, error) {
	var zero T

	requestURL := buildURL(client.BaseURL, path, query)

	var reqBody io.Reader

	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return zero, fmt.Errorf("marshal request body: %w", err)
		}

		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reqBody)
	if err != nil {
		return zero, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if client.Configure != nil {
		client.Configure(req)
	}

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, &StatusError{
			StatusCode: resp.StatusCode,
			Method:     req.Method,
			URL:        req.URL.String(),
		}
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if errors.Is(err, io.EOF) {
			return result, nil
		}

		return zero, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

func buildURL(baseURL *url.URL, path string, query url.Values) *url.URL {
	requestURL := *baseURL
	requestURL.Path = path
	requestURL.RawQuery = query.Encode()
	return &requestURL
}
