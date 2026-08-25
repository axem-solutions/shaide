package api

import (
	"net/http"
	"net/url"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/harbor/auth"
	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
)

type Client struct {
	Http *httpapi.Client
}

func NewClient(baseURL string, creds auth.Credentials) *Client {
	parsed, _ := url.Parse(baseURL)

	return &Client{
		Http: &httpapi.Client{
			BaseURL: parsed,
			HTTPClient: &http.Client{
				Timeout: 30 * time.Second,
			},
			Configure: func(req *http.Request) {
				req.SetBasicAuth(creds.Username, creds.Password)
			},
		},
	}
}
