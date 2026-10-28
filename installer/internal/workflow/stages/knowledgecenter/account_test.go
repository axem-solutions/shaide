package knowledgecenter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer is the part of shaide-server's user API the installer uses.
type fakeServer struct {
	mu        sync.Mutex
	passwords map[string]string
	created   []map[string]any
	// unavailable answers this many requests with 503 first.
	unavailable int
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.unavailable > 0 {
		f.unavailable--
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	switch r.Method + " " + r.URL.Path {
	case "POST /v1/login":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if password, ok := f.passwords[body["username"]]; !ok || password != body["password"] {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "token-" + body["username"]})

	case "GET /v1/users":
		if r.Header.Get("Authorization") != "Bearer token-admin" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		users := []map[string]any{}
		for name := range f.passwords {
			users = append(users, map[string]any{"id": 1, "username": name, "expiry": "9999-12-31T23:59:59Z"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"users": users})

	case "POST /v1/user":
		if r.Header.Get("Authorization") != "Bearer token-admin" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.created = append(f.created, body)
		f.passwords[body["username"].(string)] = body["password"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 2, "username": body["username"]})

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newServer(t *testing.T, fake *fakeServer) *shaideServer {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return &shaideServer{baseURL: srv.URL, client: srv.Client(), delay: time.Millisecond, logf: t.Logf}
}

// A first install creates the account as an ordinary, non-expiring user.
func TestEnsureAccountCreatesAMissingAccount(t *testing.T) {
	fake := &fakeServer{passwords: map[string]string{"admin": "admin-pass"}}

	created, err := newServer(t, fake).ensureAccount(context.Background(), "admin-pass", "knowledge-center", "kc-pass")
	if err != nil {
		t.Fatalf("ensureAccount() error = %v", err)
	}
	if !created || len(fake.created) != 1 {
		t.Fatalf("created = %v with %d requests, want one account created", created, len(fake.created))
	}
	request := fake.created[0]
	if request["username"] != "knowledge-center" || request["password"] != "kc-pass" {
		t.Errorf("create request = %v", request)
	}
	if expiry, _ := time.Parse(time.RFC3339, request["expiry"].(string)); expiry.Year() != 9999 {
		t.Errorf("expiry = %v, want a non-expiring account", request["expiry"])
	}
}

// An update finds the account from the first install and leaves it alone.
func TestEnsureAccountKeepsAnAccountThatSignsIn(t *testing.T) {
	fake := &fakeServer{passwords: map[string]string{"admin": "admin-pass", "knowledge-center": "kc-pass"}}

	created, err := newServer(t, fake).ensureAccount(context.Background(), "admin-pass", "knowledge-center", "kc-pass")
	if err != nil {
		t.Fatalf("ensureAccount() error = %v", err)
	}
	if created || len(fake.created) != 0 {
		t.Error("an existing account was created again")
	}
}

// An account of that name with another password would make every upload
// fail, so it stops the run with what to do about it.
func TestEnsureAccountReportsAnAccountWithAnotherPassword(t *testing.T) {
	fake := &fakeServer{passwords: map[string]string{"admin": "admin-pass", "knowledge-center": "other"}}

	_, err := newServer(t, fake).ensureAccount(context.Background(), "admin-pass", "knowledge-center", "kc-pass")
	if err == nil || !strings.Contains(err.Error(), "kcEmbeddingPassword") {
		t.Fatalf("ensureAccount() error = %v, want one pointing at kcEmbeddingPassword", err)
	}
	if strings.Contains(err.Error(), "kc-pass") {
		t.Error("the error echoes the password")
	}
}

// shaide-server may still be restarting after this run's config change.
func TestEnsureAccountWaitsForTheServer(t *testing.T) {
	fake := &fakeServer{passwords: map[string]string{"admin": "admin-pass"}, unavailable: 3}

	if _, err := newServer(t, fake).ensureAccount(context.Background(), "admin-pass", "knowledge-center", "kc-pass"); err != nil {
		t.Fatalf("ensureAccount() error = %v", err)
	}
}

// A wrong admin password does not improve with retries.
func TestEnsureAccountDoesNotRetryARejectedAdmin(t *testing.T) {
	fake := &fakeServer{passwords: map[string]string{"admin": "admin-pass"}}
	server := newServer(t, fake)
	server.delay = time.Hour

	_, err := server.ensureAccount(context.Background(), "wrong", "knowledge-center", "kc-pass")
	if err == nil || !strings.Contains(err.Error(), "sign in to shaide-server as admin") {
		t.Fatalf("ensureAccount() error = %v, want the admin sign-in to fail", err)
	}
}
