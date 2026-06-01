# CLI Login API-Key Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `nexorious login`, `logout`, and `whoami` CLI commands that exchange a username/password for an API key, store it in a local YAML config, and authenticate subsequent calls with that key.

**Architecture:** Two new internal packages — `internal/clicfg` (XDG config load/save) and `internal/cliclient` (thin HTTP client over the existing `/api/auth/*` endpoints) — plus three cobra commands in `cmd/nexorious/`. Login reuses the browser session path: `POST /api/auth/login` yields a `session_id` cookie, that cookie mints an API key via `POST /api/auth/api-keys`, then the throwaway session is dropped via `POST /api/auth/logout`.

**Tech Stack:** Go 1.25, cobra, `gopkg.in/yaml.v3` (already indirect), `golang.org/x/term` (new), stdlib `net/http`, `net/http/httptest` for tests.

**Design doc:** `docs/superpowers/specs/2026-06-01-issue-627-cli-login-bootstrap-design.md`

**Module path:** `github.com/drzero42/nexorious`

**Endpoint contract (verified against `internal/api/router.go` + `auth.go`):**
- `POST /api/auth/login` — body `{"username","password"}` → **200**, sets `session_id` cookie.
- `POST /api/auth/api-keys` — cookie or Bearer auth; body `{"name","scopes"}` → **200**, `{"id","key",...}` (`key` is raw `nxr_…`, shown once).
- `DELETE /api/auth/api-keys/:id` — cookie or Bearer auth → **204 No Content**.
- `POST /api/auth/logout` — cookie auth → **200**.
- `GET /api/auth/me` — cookie or Bearer auth → **200**, `{"username",…}`.
- Echo error responses are JSON `{"message":"…"}`.

---

## File Structure

- `internal/clicfg/config.go` — `Config`/`Profile` types, `Path`, `Load`, `Save`, `CurrentProfile`, `SetProfile`.
- `internal/clicfg/config_test.go` — round-trip, XDG fallback, file mode, missing-file.
- `internal/cliclient/client.go` — `Client` + `Login`, `CreateAPIKey`, `RevokeAPIKeyWithCookie`, `RevokeAPIKeyWithBearer`, `Logout`, `Me`.
- `internal/cliclient/client_test.go` — each method against `httptest.Server`.
- `cmd/nexorious/login.go` — `login` command + password resolution.
- `cmd/nexorious/logout.go` — `logout` command.
- `cmd/nexorious/whoami.go` — `whoami` command.
- `cmd/nexorious/login_test.go` — end-to-end login against an `httptest` server.
- `cmd/nexorious/main.go:38-40` — register the three new commands.

---

## Task 1: `clicfg` package — config types and path resolution

**Files:**
- Create: `internal/clicfg/config.go`
- Test: `internal/clicfg/config_test.go`

- [ ] **Step 1: Write the failing test for Path + round-trip + file mode**

Create `internal/clicfg/config_test.go`:

```go
package clicfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := "/tmp/xdg-test/nexorious/config.yaml"
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/home-test")
	got, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := "/tmp/home-test/.config/nexorious/config.yaml"
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Profiles == nil {
		t.Fatal("Profiles map should be initialized, not nil")
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(cfg.Profiles))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &Config{}
	cfg.SetProfile("default", Profile{
		URL:      "http://localhost:8000",
		Username: "alice",
		KeyName:  "cli@host",
		KeyID:    "id-123",
		Key:      "nxr_secret",
	})
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Current != "default" {
		t.Fatalf("Current = %q, want default", got.Current)
	}
	p, ok := got.CurrentProfile()
	if !ok {
		t.Fatal("CurrentProfile not found")
	}
	if p.Key != "nxr_secret" || p.KeyID != "id-123" || p.URL != "http://localhost:8000" {
		t.Fatalf("round-trip mismatch: %+v", p)
	}
}

func TestSaveUsesOwnerOnlyPerms(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &Config{}
	cfg.SetProfile("default", Profile{Key: "nxr_secret"})
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir perm = %o, want 700", perm)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/clicfg/ -v`
Expected: FAIL — `undefined: Path` / `undefined: Config` (package doesn't compile yet).

- [ ] **Step 3: Implement `config.go`**

Create `internal/clicfg/config.go`:

```go
// Package clicfg reads and writes the Nexorious CLI's local config file.
//
// The file lives at $XDG_CONFIG_HOME/nexorious/config.yaml (falling back to
// ~/.config/nexorious/config.yaml) and stores a live API key, so it is written
// with owner-only permissions.
package clicfg

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultProfile = "default"

// Profile holds the credentials for one server.
type Profile struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	KeyName  string `yaml:"key_name"`
	KeyID    string `yaml:"key_id"`
	Key      string `yaml:"key"`
}

// Config is the whole config file: a set of named profiles and a pointer to the
// active one. Only a single profile is created today, but the schema leaves room
// for multiple without a breaking format change.
type Config struct {
	Current  string             `yaml:"current"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// Path returns the config file path, honoring XDG_CONFIG_HOME.
func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "nexorious", "config.yaml"), nil
}

// Load reads the config file. A missing file yields an empty Config rather than
// an error so first-time `login` works.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Profiles: map[string]Profile{}}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return &cfg, nil
}

// Save writes the config atomically (temp file + rename) with 0600 perms in a
// 0700 directory.
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	//nolint:errcheck // best-effort cleanup; the rename below is what matters
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// CurrentProfile returns the active profile and whether it exists.
func (c *Config) CurrentProfile() (Profile, bool) {
	name := c.Current
	if name == "" {
		name = defaultProfile
	}
	p, ok := c.Profiles[name]
	return p, ok
}

// CurrentName returns the active profile name, defaulting to "default".
func (c *Config) CurrentName() string {
	if c.Current == "" {
		return defaultProfile
	}
	return c.Current
}

// SetProfile stores a profile and marks it current.
func (c *Config) SetProfile(name string, p Profile) {
	if name == "" {
		name = defaultProfile
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = p
	c.Current = name
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/clicfg/ -v`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
git add internal/clicfg/
git commit -m "feat: add clicfg package for CLI config file"
```

---

## Task 2: `cliclient` package — HTTP client over the auth API

**Files:**
- Create: `internal/cliclient/client.go`
- Test: `internal/cliclient/client_test.go`

- [ ] **Step 1: Write the failing test against an httptest server**

Create `internal/cliclient/client_test.go`:

```go
package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer returns an httptest server emulating the subset of /api/auth/*
// the CLI uses, plus a record of what it received.
type captured struct {
	createCookie string
	revokeAuth   string
	revokeID     string
	logoutCookie string
	meAuth       string
}

func newTestServer(t *testing.T, cap *captured) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "alice" || body["password"] != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "incorrect username or password"})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-123"})
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})

	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if ck, _ := r.Cookie("session_id"); ck != nil {
			cap.createCookie = ck.Value
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-id-1", "key": "nxr_rawkey"})
	})

	mux.HandleFunc("/api/auth/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		cap.revokeID = r.URL.Path[len("/api/auth/api-keys/"):]
		if ck, _ := r.Cookie("session_id"); ck != nil {
			cap.revokeAuth = "cookie:" + ck.Value
		} else {
			cap.revokeAuth = r.Header.Get("Authorization")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if ck, _ := r.Cookie("session_id"); ck != nil {
			cap.logoutCookie = ck.Value
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})

	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		cap.meAuth = r.Header.Get("Authorization")
		if cap.meAuth != "Bearer nxr_rawkey" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "unauthorized"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginReturnsSessionCookie(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	sess, err := c.Login("alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sess != "sess-123" {
		t.Fatalf("session = %q, want sess-123", sess)
	}
}

func TestLoginBadCredsReturnsServerMessage(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	_, err := c.Login("alice", "wrong")
	if err == nil {
		t.Fatal("expected error for bad creds")
	}
	if got := err.Error(); got == "" || !contains(got, "incorrect username or password") {
		t.Fatalf("error = %q, want server message", got)
	}
}

func TestCreateAPIKeySendsCookieReturnsKey(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	key, id, err := c.CreateAPIKey("sess-123", "cli@host")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if key != "nxr_rawkey" || id != "key-id-1" {
		t.Fatalf("got key=%q id=%q", key, id)
	}
	if cap.createCookie != "sess-123" {
		t.Fatalf("server saw cookie %q, want sess-123", cap.createCookie)
	}
}

func TestRevokeWithCookie(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	if err := c.RevokeAPIKeyWithCookie("sess-123", "key-id-1"); err != nil {
		t.Fatalf("RevokeAPIKeyWithCookie: %v", err)
	}
	if cap.revokeID != "key-id-1" {
		t.Fatalf("revoked id = %q", cap.revokeID)
	}
	if cap.revokeAuth != "cookie:sess-123" {
		t.Fatalf("revoke auth = %q, want cookie:sess-123", cap.revokeAuth)
	}
}

func TestRevokeWithBearer(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	if err := c.RevokeAPIKeyWithBearer("nxr_rawkey", "key-id-1"); err != nil {
		t.Fatalf("RevokeAPIKeyWithBearer: %v", err)
	}
	if cap.revokeAuth != "Bearer nxr_rawkey" {
		t.Fatalf("revoke auth = %q, want Bearer nxr_rawkey", cap.revokeAuth)
	}
}

func TestLogoutSendsCookie(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	if err := c.Logout("sess-123"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if cap.logoutCookie != "sess-123" {
		t.Fatalf("logout cookie = %q", cap.logoutCookie)
	}
}

func TestMeReturnsUsername(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	user, err := c.Me("nxr_rawkey")
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if user != "alice" {
		t.Fatalf("user = %q, want alice", user)
	}
}

func TestMeUnauthorized(t *testing.T) {
	var cap captured
	srv := newTestServer(t, &cap)
	c := New(srv.URL)

	_, err := c.Me("nxr_wrong")
	if err == nil {
		t.Fatal("expected error for bad key")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cliclient/ -v`
Expected: FAIL — `undefined: New` (package doesn't compile yet).

- [ ] **Step 3: Implement `client.go`**

Create `internal/cliclient/client.go`:

```go
// Package cliclient is a thin HTTP client over the Nexorious /api/auth/*
// endpoints used by the CLI to bootstrap and manage an API key.
package cliclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "session_id"

// Client talks to one Nexorious server.
type Client struct {
	baseURL string
	hc      *http.Client
}

// New returns a Client for the given base URL (trailing slash trimmed).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 30 * time.Second},
	}
}

type errorBody struct {
	Message string `json:"message"`
}

// httpError decodes an Echo error response (`{"message":"…"}`) into a readable
// error including the status code.
func httpError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("server returned %d (failed reading body: %w)", resp.StatusCode, err)
	}
	var eb errorBody
	if json.Unmarshal(body, &eb) == nil && eb.Message != "" {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, eb.Message)
	}
	return fmt.Errorf("server returned %d", resp.StatusCode)
}

// Login posts credentials and returns the raw session_id cookie value. The value
// is read straight off the response (not via a cookie jar) so a Secure-flagged
// cookie issued over http://localhost is still usable for the follow-up calls.
func (c *Client) Login(username, password string) (string, error) {
	payload, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return "", fmt.Errorf("marshal login: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookieName {
			return ck.Value, nil
		}
	}
	return "", fmt.Errorf("login succeeded but no %s cookie was returned", sessionCookieName)
}

type createAPIKeyResp struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// CreateAPIKey mints a write-scoped key named `name`, authenticating with the
// session cookie. Returns the raw key and its server-side id.
func (c *Client) CreateAPIKey(sessionID, name string) (string, string, error) {
	payload, err := json.Marshal(map[string]string{"name": name, "scopes": "write"})
	if err != nil {
		return "", "", fmt.Errorf("marshal create key: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/api-keys", bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("build create key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("create key request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", httpError(resp)
	}
	var out createAPIKeyResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode create key response: %w", err)
	}
	return out.Key, out.ID, nil
}

// revoke issues DELETE /api/auth/api-keys/:id with caller-supplied auth.
func (c *Client) revoke(keyID string, auth func(*http.Request)) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/auth/api-keys/"+keyID, nil)
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	auth(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("revoke request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return httpError(resp)
	}
	return nil
}

// RevokeAPIKeyWithCookie revokes a key using a session cookie (used during
// login rotation, before the new key exists).
func (c *Client) RevokeAPIKeyWithCookie(sessionID, keyID string) error {
	return c.revoke(keyID, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	})
}

// RevokeAPIKeyWithBearer revokes a key using the key itself as a Bearer token
// (used by logout).
func (c *Client) RevokeAPIKeyWithBearer(key, keyID string) error {
	return c.revoke(keyID, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+key)
	})
}

// Logout drops the throwaway session created during login.
func (c *Client) Logout(sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/logout", nil)
	if err != nil {
		return fmt.Errorf("build logout request: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return httpError(resp)
	}
	return nil
}

type meResp struct {
	Username string `json:"username"`
}

// Me returns the authenticated username for the given API key.
func (c *Client) Me(key string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/auth/me", nil)
	if err != nil {
		return "", fmt.Errorf("build me request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("me request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	var out meResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode me response: %w", err)
	}
	return out.Username, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/cliclient/ -v`
Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/
git commit -m "feat: add cliclient HTTP client for CLI auth"
```

---

## Task 3: Add `golang.org/x/term` dependency

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `nix/package.nix` (vendorHash)

- [ ] **Step 1: Add the dependency**

Run:
```bash
go get golang.org/x/term@latest
go mod tidy
```
Expected: `golang.org/x/term` appears in `go.mod`'s require block as a direct dependency, and `gopkg.in/yaml.v3` is promoted from `// indirect` to a direct require.

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 3: Update the Nix vendorHash**

The Go module set changed, so `vendorHash` in `nix/package.nix` is now stale. Per CLAUDE.md → "Nix Flake Maintenance":

```bash
# Temporarily set vendorHash = pkgs.lib.fakeHash; in nix/package.nix, then:
nix build .#nexorious 2>&1 | grep "got:"
# paste the "got:" hash into nix/package.nix → vendorHash
```

If `nix` is not available in this environment, leave a note in the commit body that `vendorHash` must be refreshed and skip the nix build — CI will surface the correct hash. Do NOT guess a hash.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum nix/package.nix
git commit -m "build: add golang.org/x/term dependency"
```

---

## Task 4: `login` command

**Files:**
- Create: `cmd/nexorious/login.go`
- Create: `cmd/nexorious/login_test.go`

- [ ] **Step 1: Write the failing end-to-end test**

Create `cmd/nexorious/login_test.go`. It stands up an httptest server emulating the three endpoints, points `--url` at it, supplies the password via `NEXORIOUS_PASSWORD`, runs the command, and asserts the config file was written with the minted key. (`XDG_CONFIG_HOME` is redirected to a temp dir so the real user config is untouched.)

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func loginTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginWritesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := loginTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\noutput: %s", err, out.String())
	}

	cfg, err := clicfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := cfg.CurrentProfile()
	if !ok {
		t.Fatal("no current profile after login")
	}
	if p.Key != "nxr_minted" || p.KeyID != "key-1" {
		t.Fatalf("stored profile = %+v", p)
	}
	if p.URL != srv.URL || p.Username != "alice" {
		t.Fatalf("stored profile url/username = %+v", p)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/nexorious/ -run TestLoginWritesConfig -v`
Expected: FAIL — `unknown command "login"` (command not registered yet).

- [ ] **Step 3: Implement `login.go`**

Create `cmd/nexorious/login.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

const defaultServerURL = "http://localhost:8000"

// newLoginCmd returns the `login` subcommand.
func newLoginCmd() *cobra.Command {
	var urlFlag, usernameFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a Nexorious server and store an API key",
		Long: "Exchange a username and password for an API key and store it in the\n" +
			"local config file (" + configPathHint() + "). Subsequent commands use the\n" +
			"stored key. The password is read from the NEXORIOUS_PASSWORD environment\n" +
			"variable when set, otherwise prompted for interactively.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, urlFlag, usernameFlag)
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "Server URL (prompted if omitted)")
	cmd.Flags().StringVar(&usernameFlag, "username", "", "Username (prompted if omitted)")
	return cmd
}

func runLogin(cmd *cobra.Command, urlFlag, usernameFlag string) error {
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	existing, _ := cfg.CurrentProfile()

	in := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	url := firstNonEmpty(urlFlag, existing.URL)
	if url == "" {
		url, err = prompt(in, out, fmt.Sprintf("Server URL [%s]: ", defaultServerURL))
		if err != nil {
			return err
		}
		url = firstNonEmpty(url, defaultServerURL)
	}

	username := firstNonEmpty(usernameFlag, existing.Username)
	if username == "" {
		username, err = prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}

	password, err := readPassword(out)
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	client := cliclient.New(url)

	sessionID, err := client.Login(username, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Rotate: revoke the previously stored key (if any) before minting a new one.
	if existing.KeyID != "" {
		if err := client.RevokeAPIKeyWithCookie(sessionID, existing.KeyID); err != nil {
			fmt.Fprintf(out, "warning: could not revoke previous key %s: %v\n", existing.KeyID, err)
		}
	}

	keyName := "cli@" + hostname()
	key, keyID, err := client.CreateAPIKey(sessionID, keyName)
	if err != nil {
		return fmt.Errorf("create API key failed: %w", err)
	}

	// Drop the throwaway session; failure here is non-fatal.
	if err := client.Logout(sessionID); err != nil {
		fmt.Fprintf(out, "warning: could not close bootstrap session: %v\n", err)
	}

	cfg.SetProfile(cfg.CurrentName(), clicfg.Profile{
		URL:      url,
		Username: username,
		KeyName:  keyName,
		KeyID:    keyID,
		Key:      key,
	})
	if err := clicfg.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(out, "Logged in to %s as %s.\nStored API key %q (%s).\n",
		url, username, keyName, maskKey(key))
	return nil
}

// readPassword reads the password from NEXORIOUS_PASSWORD, or prompts without
// echo on a TTY, or reads a plain line from stdin when not a TTY (e.g. piped).
func readPassword(out io.Writer) (string, error) {
	if env := os.Getenv("NEXORIOUS_PASSWORD"); env != "" {
		return env, nil
	}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(out, "Password: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func prompt(in *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown-host"
	}
	return h
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "…" + key[len(key)-4:]
}

func configPathHint() string {
	p, err := clicfg.Path()
	if err != nil {
		return "~/.config/nexorious/config.yaml"
	}
	return p
}
```

- [ ] **Step 4: Register the command**

Modify `cmd/nexorious/main.go`. After the existing `root.AddCommand(newVersionCmd())` line (around line 40), add:

```go
	root.AddCommand(newLoginCmd())
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./cmd/nexorious/ -run TestLoginWritesConfig -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/login.go cmd/nexorious/login_test.go cmd/nexorious/main.go
git commit -m "feat: add nexorious login command"
```

---

## Task 5: `logout` command

**Files:**
- Create: `cmd/nexorious/logout.go`
- Create: `cmd/nexorious/logout_test.go`
- Modify: `cmd/nexorious/main.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/nexorious/logout_test.go`:

```go
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestLogoutRevokesAndClearsKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var revokedID, gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		revokedID = r.URL.Path[len("/api/auth/api-keys/"):]
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Seed a logged-in config.
	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{
		URL: srv.URL, Username: "alice", KeyName: "cli@host", KeyID: "key-1", Key: "nxr_secret",
	})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"logout"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logout: %v\n%s", err, out.String())
	}

	if revokedID != "key-1" {
		t.Fatalf("revoked id = %q, want key-1", revokedID)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}

	got, err := clicfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, _ := got.CurrentProfile()
	if p.Key != "" || p.KeyID != "" || p.KeyName != "" {
		t.Fatalf("credentials not cleared: %+v", p)
	}
	if p.URL != srv.URL || p.Username != "alice" {
		t.Fatalf("url/username should be retained: %+v", p)
	}
}

func TestLogoutNoStoredKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"logout"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error when not logged in")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/nexorious/ -run TestLogout -v`
Expected: FAIL — `unknown command "logout"`.

- [ ] **Step 3: Implement `logout.go`**

Create `cmd/nexorious/logout.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// newLogoutCmd returns the `logout` subcommand.
func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the stored API key and clear it from config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogout(cmd)
		},
	}
}

func runLogout(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key == "" {
		return fmt.Errorf("not logged in (no stored API key)")
	}

	// Best-effort server-side revocation; a failure still clears local config.
	if p.KeyID != "" {
		if err := cliclient.New(p.URL).RevokeAPIKeyWithBearer(p.Key, p.KeyID); err != nil {
			fmt.Fprintf(out, "warning: could not revoke key on server: %v\n", err)
		}
	}

	p.Key = ""
	p.KeyID = ""
	p.KeyName = ""
	cfg.SetProfile(cfg.CurrentName(), p)
	if err := clicfg.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Fprintf(out, "Logged out of %s.\n", p.URL)
	return nil
}
```

- [ ] **Step 4: Register the command**

Modify `cmd/nexorious/main.go`. After `root.AddCommand(newLoginCmd())`, add:

```go
	root.AddCommand(newLogoutCmd())
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./cmd/nexorious/ -run TestLogout -v`
Expected: PASS (both logout tests).

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/logout.go cmd/nexorious/logout_test.go cmd/nexorious/main.go
git commit -m "feat: add nexorious logout command"
```

---

## Task 6: `whoami` command

**Files:**
- Create: `cmd/nexorious/whoami.go`
- Create: `cmd/nexorious/whoami_test.go`
- Modify: `cmd/nexorious/main.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/nexorious/whoami_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestWhoamiPrintsUser(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer nxr_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "unauthorized"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{URL: srv.URL, Username: "alice", Key: "nxr_secret", KeyID: "k1"})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"whoami"})
	if err := root.Execute(); err != nil {
		t.Fatalf("whoami: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "alice") {
		t.Fatalf("output missing username: %q", out.String())
	}
}

func TestWhoamiNotLoggedIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"whoami"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error when not logged in")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/nexorious/ -run TestWhoami -v`
Expected: FAIL — `unknown command "whoami"`.

- [ ] **Step 3: Implement `whoami.go`**

Create `cmd/nexorious/whoami.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// newWhoamiCmd returns the `whoami` subcommand.
func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the user authenticated by the stored API key",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWhoami(cmd)
		},
	}
}

func runWhoami(cmd *cobra.Command) error {
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key == "" {
		return fmt.Errorf("not logged in (run `nexorious login` first)")
	}

	username, err := cliclient.New(p.URL).Me(p.Key)
	if err != nil {
		return fmt.Errorf("could not verify stored key (it may be revoked or expired): %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s @ %s\n", username, p.URL)
	return nil
}
```

- [ ] **Step 4: Register the command**

Modify `cmd/nexorious/main.go`. After `root.AddCommand(newLogoutCmd())`, add:

```go
	root.AddCommand(newWhoamiCmd())
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./cmd/nexorious/ -run TestWhoami -v`
Expected: PASS (both whoami tests).

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/whoami.go cmd/nexorious/whoami_test.go cmd/nexorious/main.go
git commit -m "feat: add nexorious whoami command"
```

---

## Task 7: Full verification

- [ ] **Step 1: Run the whole Go test suite**

Run: `go test -timeout 600s ./...`
Expected: PASS across all packages.

- [ ] **Step 2: Lint**

Run: `golangci-lint run`
Expected: no findings. If `errcheck` (check-blank) flags any `_ =` discard not covered by the `std-error-handling` preset, handle it or add a one-line `//nolint:errcheck // <reason>` as the codebase convention allows.

- [ ] **Step 3: Build the binary and smoke-test help text**

Run:
```bash
go build ./... && go run ./cmd/nexorious login --help
```
Expected: build succeeds; `login --help` prints the flags `--url` and `--username` and mentions `NEXORIOUS_PASSWORD`.

- [ ] **Step 4: Final commit if anything changed**

```bash
git status
# commit any lint fixups
```

---

## Notes for the implementer

- **No `slumber.yaml` changes** — these commands add no server routes; they consume existing endpoints.
- **No new server code** — purely additive CLI. Do not touch `internal/api` or `internal/auth`.
- **errcheck convention** — see CLAUDE.md "Known Gotchas". The `defer func(){ _ = resp.Body.Close() }()` pattern is covered by the `std-error-handling` preset and needs no annotation; the `os.Remove` cleanup in `clicfg.Save` carries an explicit `//nolint:errcheck`.
- **Do not guess the Nix `vendorHash`** (Task 3) — derive it from `nix build` output or let CI report it.
- **README/docs:** out of scope for this plan; can be a follow-up.
