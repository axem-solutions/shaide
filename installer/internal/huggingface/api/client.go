package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
)

type Client struct {
	http            *httpapi.Client
	defaultRevision string
}

func New(token, defaultRevision string) *Client {
	parsed, _ := url.Parse("https://huggingface.co")

	return &Client{
		defaultRevision: defaultRevision,
		http: &httpapi.Client{
			BaseURL:    parsed,
			HTTPClient: http.DefaultClient,
			Configure: func(req *http.Request) {
				if strings.TrimSpace(token) != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
			},
		},
	}
}
