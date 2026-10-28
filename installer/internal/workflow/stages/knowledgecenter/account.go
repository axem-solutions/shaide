package knowledgecenter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	kc "github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	pkgkube "github.com/axem-solutions/ai_platform/pkg/kube"
)

const (
	// adminUsername is the account shaide-server creates for ADMIN_PASSWORD.
	adminUsername = "admin"

	requestTimeout = 30 * time.Second

	// shaide-server can still be rolling out the config this run changed, so
	// the first sign-in is retried for a while.
	signInAttempts = 12
	signInDelay    = 10 * time.Second
)

// accountExpiry is far enough out that the account does not expire, like the
// accounts shaide-server generates.
var accountExpiry = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)

// EnsureEmbeddingAccount creates the ordinary shaide user KC requests
// embeddings with, through a port-forward to shaide-server. An account that
// already exists is left as it is, after checking it signs in with the
// retained password.
func EnsureEmbeddingAccount(rt *core.Runtime, adminPassword string, creds kc.Credentials) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	target, err := pkgkube.NewServiceRef(ctx, rt.Cluster.Client, shaide.Namespace, shaide.ServerServiceName).ForwardTarget()
	if err != nil {
		return fmt.Errorf("find shaide-server to create the Knowledge Center account: %w", err)
	}

	forward, err := pkgkube.StartPortForward(ctx, rt.Cluster.RESTConfig, rt.Cluster.Client, pkgkube.ForwardRequest{
		Namespace:  shaide.Namespace,
		Service:    shaide.ServerServiceName,
		PodName:    target.PodName,
		RemotePort: target.TargetPort,
		Logf:       rt.Detailf,
	})
	if err != nil {
		return fmt.Errorf("port-forward to shaide-server: %w", err)
	}
	defer forward.Close()

	server := &shaideServer{
		baseURL: forward.Address(),
		client:  &http.Client{Timeout: requestTimeout},
		delay:   signInDelay,
		logf:    rt.Detailf,
	}

	created, err := server.ensureAccount(context.Background(), adminPassword, creds.EmbeddingUsername, creds.EmbeddingPassword)
	if err != nil {
		return err
	}
	if created {
		rt.Detailf("created the Knowledge Center account %q", creds.EmbeddingUsername)
	} else {
		rt.Detailf("the Knowledge Center account %q already exists", creds.EmbeddingUsername)
	}

	return nil
}

type shaideServer struct {
	baseURL string
	client  *http.Client
	delay   time.Duration
	logf    func(format string, args ...any)
}

type shaideUser struct {
	Username string `json:"username"`
}

// errRejected is a sign-in the server answered with 401 or 403.
var errRejected = errors.New("credentials rejected")

// ensureAccount reports whether it created the account.
func (s *shaideServer) ensureAccount(ctx context.Context, adminPassword, username, password string) (bool, error) {
	token, err := s.signInWithRetry(ctx, adminUsername, adminPassword)
	if err != nil {
		return false, fmt.Errorf("sign in to shaide-server as %s: %w", adminUsername, err)
	}

	var users struct {
		Users []shaideUser `json:"users"`
	}
	if err := s.do(ctx, http.MethodGet, "/v1/users", token, nil, &users); err != nil {
		return false, fmt.Errorf("list shaide users: %w", err)
	}

	exists := slices.ContainsFunc(users.Users, func(u shaideUser) bool { return u.Username == username })
	if exists {
		if _, err := s.signIn(ctx, username, password); err != nil {
			if errors.Is(err, errRejected) {
				return false, fmt.Errorf("the shaide user %q exists but does not sign in with the Knowledge Center's password; "+
					"delete the user or set its password to app-shaide:kcEmbeddingPassword, then run the installer again", username)
			}
			return false, fmt.Errorf("sign in as %s: %w", username, err)
		}
		return false, nil
	}

	request := map[string]any{
		"username": username,
		"password": password,
		"expiry":   accountExpiry.Format(time.RFC3339),
	}
	if err := s.do(ctx, http.MethodPost, "/v1/user", token, request, nil); err != nil {
		return false, fmt.Errorf("create the shaide user %q: %w", username, err)
	}

	return true, nil
}

// signInWithRetry retries until shaide-server answers. A rejection is not
// retried.
func (s *shaideServer) signInWithRetry(ctx context.Context, username, password string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= signInAttempts; attempt++ {
		token, err := s.signIn(ctx, username, password)
		if err == nil || errors.Is(err, errRejected) {
			return token, err
		}

		lastErr = err
		s.logf("shaide-server is not answering yet (%v); retrying", err)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(s.delay):
		}
	}
	return "", lastErr
}

func (s *shaideServer) signIn(ctx context.Context, username, password string) (string, error) {
	var response struct {
		AccessToken string `json:"access_token"`
	}
	err := s.do(ctx, http.MethodPost, "/v1/login", "", map[string]string{"username": username, "password": password}, &response)
	if err != nil {
		return "", err
	}
	if response.AccessToken == "" {
		return "", errors.New("the sign-in response has no access token")
	}
	return response.AccessToken, nil
}

func (s *shaideServer) do(ctx context.Context, method, path, token string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errRejected
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The body is not echoed: it can repeat the request.
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return nil
}
