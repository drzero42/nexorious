# CLI `api-key` subcommand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `nexorious api-key generate|list|revoke` (alias `keys`) so a logged-in user manages their API keys from the CLI by calling the existing `/api/auth/api-keys` endpoints with their stored Bearer key.

**Architecture:** Pure-CLI change. Three new `cliclient` methods (`ListAPIKeys`, `CreateAPIKeyWithBearer`, plus an exported `APIKey` type) wrap the endpoints; revoke reuses the existing `RevokeAPIKeyWithBearer`. A new `cmd/nexorious/api_key.go` holds the cobra parent + three subcommands, all authenticating with the key from `clicfg`. The logout "clear stored key" step is extracted into a shared helper so self-revoke can reuse it. No server or database changes.

**Tech Stack:** Go 1.25, `spf13/cobra`, stdlib `net/http`, `text/tabwriter`, `encoding/json`; tests with stdlib `testing` + `net/http/httptest`.

**Spec:** `docs/superpowers/specs/2026-06-01-issue-626-cli-api-key-subcommand-design.md`

---

### Task 1: `cliclient` — `APIKey` type, `ListAPIKeys`, `CreateAPIKeyWithBearer`

**Files:**
- Modify: `internal/cliclient/client.go`
- Test: `internal/cliclient/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/cliclient/client_test.go`:

```go
func TestListAPIKeys(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"k1","name":"laptop","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	keys, err := New(srv.URL).ListAPIKeys("nxr_secret")
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}
	if len(keys) != 1 || keys[0].ID != "k1" || keys[0].Name != "laptop" {
		t.Fatalf("keys = %+v, want one key k1/laptop", keys)
	}
	if keys[0].LastUsedAt != nil {
		t.Fatalf("LastUsedAt = %v, want nil", keys[0].LastUsedAt)
	}
}

func TestCreateAPIKeyWithBearer(t *testing.T) {
	var gotAuth string
	var gotBody map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k2","name":"ci","scopes":"read","key":"nxr_rawkey","created_at":"2026-01-01T00:00:00Z","expires_at":null}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	key, err := New(srv.URL).CreateAPIKeyWithBearer("nxr_secret", "ci", "read", nil)
	if err != nil {
		t.Fatalf("CreateAPIKeyWithBearer: %v", err)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}
	if _, ok := gotBody["expires_at"]; ok {
		t.Fatalf("expires_at should be omitted when nil, got body %+v", gotBody)
	}
	if gotBody["name"] != "ci" || gotBody["scopes"] != "read" {
		t.Fatalf("body = %+v, want name=ci scopes=read", gotBody)
	}
	if key.Key != "nxr_rawkey" || key.ID != "k2" {
		t.Fatalf("key = %+v, want raw key nxr_rawkey id k2", key)
	}
}

func TestCreateAPIKeyWithBearerSendsExpiry(t *testing.T) {
	var gotBody map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k3","name":"temp","scopes":"write","key":"nxr_x","created_at":"2026-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	exp := "2027-01-01T00:00:00Z"
	if _, err := New(srv.URL).CreateAPIKeyWithBearer("nxr_secret", "temp", "write", &exp); err != nil {
		t.Fatalf("CreateAPIKeyWithBearer: %v", err)
	}
	if gotBody["expires_at"] != exp {
		t.Fatalf("expires_at = %q, want %q", gotBody["expires_at"], exp)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cliclient/ -run 'TestListAPIKeys|TestCreateAPIKeyWithBearer' -v`
Expected: FAIL — compile error `undefined: ... ListAPIKeys / CreateAPIKeyWithBearer / APIKey`.

- [ ] **Step 3: Add the `APIKey` type and two methods**

In `internal/cliclient/client.go`, add (after the existing `createAPIKeyResp` type / `CreateAPIKey` method):

```go
// APIKey describes one API key as returned by the /api/auth/api-keys endpoints.
// Key is only populated by CreateAPIKeyWithBearer (the raw value is shown once at
// creation); list responses never include it.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     string     `json:"scopes"`
	Key        string     `json:"key,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// ListAPIKeys returns the caller's non-revoked API keys, authenticating with the
// key itself as a Bearer token.
func (c *Client) ListAPIKeys(key string) ([]APIKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/auth/api-keys", nil)
	if err != nil {
		return nil, fmt.Errorf("build list keys request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list keys request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, httpError(resp)
	}
	var out []APIKey
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode list keys response: %w", err)
	}
	return out, nil
}

// CreateAPIKeyWithBearer mints a key authenticating with an existing key as a
// Bearer token (used by `api-key generate`). When expiresAt is non-nil it is sent
// as the request's expires_at (the server validates the RFC3339 format). The
// returned APIKey includes the raw Key, shown exactly once.
func (c *Client) CreateAPIKeyWithBearer(key, name, scopes string, expiresAt *string) (APIKey, error) {
	body := map[string]string{"name": name, "scopes": scopes}
	if expiresAt != nil {
		body["expires_at"] = *expiresAt
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return APIKey{}, fmt.Errorf("marshal create key: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/api-keys", bytes.NewReader(payload))
	if err != nil {
		return APIKey{}, fmt.Errorf("build create key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return APIKey{}, fmt.Errorf("create key request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return APIKey{}, httpError(resp)
	}
	var out APIKey
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return APIKey{}, fmt.Errorf("decode create key response: %w", err)
	}
	return out, nil
}
```

(`bytes`, `encoding/json`, `fmt`, `net/http`, `time` are already imported in this file.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cliclient/ -run 'TestListAPIKeys|TestCreateAPIKeyWithBearer' -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/client.go internal/cliclient/client_test.go
git commit -m "feat: cliclient ListAPIKeys and CreateAPIKeyWithBearer (#626)"
```

---

### Task 2: Extract `clearStoredKey` helper from logout

**Files:**
- Modify: `cmd/nexorious/logout.go`
- Test: `cmd/nexorious/logout_test.go` (existing tests must still pass — no new test needed; this is a behavior-preserving refactor verified by the existing `TestLogoutRevokesAndClearsKey`)

- [ ] **Step 1: Add the helper and use it in `runLogout`**

In `cmd/nexorious/logout.go`, replace the body of `runLogout` from the `// Best-effort` comment onward, and add the helper. The full file becomes:

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
		client := cliclient.New(p.URL)
		if err := client.RevokeAPIKeyWithBearer(p.Key, p.KeyID); err != nil {
			fmt.Fprintf(out, "warning: could not revoke key on server: %v\n", err)
		}
	}

	if err := clearStoredKey(cfg); err != nil {
		return err
	}

	fmt.Fprintf(out, "Logged out of %s.\n", p.URL)
	return nil
}

// clearStoredKey removes the stored API key (Key/KeyID/KeyName) from the current
// profile and saves the config, leaving the CLI logged out. URL and username are
// retained. Used by `logout` and by `api-key revoke` when revoking the CLI's own
// key. It does not touch the server.
func clearStoredKey(cfg *clicfg.Config) error {
	p, _ := cfg.CurrentProfile()
	p.Key = ""
	p.KeyID = ""
	p.KeyName = ""
	cfg.SetProfile(cfg.CurrentName(), p)
	if err := clicfg.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Run the existing logout tests to verify they still pass**

Run: `go test ./cmd/nexorious/ -run TestLogout -v`
Expected: PASS (`TestLogoutRevokesAndClearsKey`, `TestLogoutNoStoredKey`).

- [ ] **Step 3: Commit**

```bash
git add cmd/nexorious/logout.go
git commit -m "refactor: extract clearStoredKey helper from logout (#626)"
```

---

### Task 3: `api-key` parent + `generate` subcommand (and register in root)

**Files:**
- Create: `cmd/nexorious/api_key.go`
- Modify: `cmd/nexorious/main.go`
- Test: `cmd/nexorious/api_key_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/nexorious/api_key_test.go`:

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

// seedProfile writes a logged-in config pointing at srvURL and returns nothing;
// callers set XDG_CONFIG_HOME to a temp dir first.
func seedProfile(t *testing.T, srvURL string) {
	t.Helper()
	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{
		URL: srvURL, Username: "alice", KeyName: "cli@host", KeyID: "self-key", Key: "nxr_secret",
	})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// runCmd executes the root command with args and returns combined output.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestGenerateNotLoggedIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := runCmd(t, "api-key", "generate", "--name", "x"); err == nil {
		t.Fatal("expected error when not logged in")
	}
}

func TestGenerateHappyPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var gotBody map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // dup-name check
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k9","name":"ci","scopes":"write","key":"nxr_rawkey","created_at":"2026-01-01T00:00:00Z","expires_at":null}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "generate", "--name", "ci")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "nxr_rawkey") {
		t.Fatalf("output missing raw key: %s", out)
	}
	if !strings.Contains(out, "k9") || !strings.Contains(out, "never") {
		t.Fatalf("output missing id/expiry: %s", out)
	}
	if gotBody["scopes"] != "write" {
		t.Fatalf("default scopes = %q, want write", gotBody["scopes"])
	}
}

func TestGenerateInvalidScopes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seedProfile(t, "http://unused.example")
	out, err := runCmd(t, "api-key", "generate", "--name", "x", "--scopes", "admin")
	if err == nil {
		t.Fatalf("expected scopes validation error, output: %s", out)
	}
}

func TestGenerateMissingName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seedProfile(t, "http://unused.example")
	if _, err := runCmd(t, "api-key", "generate"); err == nil {
		t.Fatal("expected error when --name omitted")
	}
}

func TestGenerateDuplicateNameWarns(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"k1","name":"ci","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k2","name":"ci","scopes":"write","key":"nxr_rawkey","created_at":"2026-01-01T00:00:00Z","expires_at":null}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "generate", "--name", "ci")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "already exists") {
		t.Fatalf("expected dup-name warning, got: %s", out)
	}
	if !strings.Contains(out, "nxr_rawkey") {
		t.Fatalf("key should still be created: %s", out)
	}
}

func TestGenerateServerError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "expires_at must be RFC3339"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "generate", "--name", "x", "--expires-at", "nope")
	if err == nil {
		t.Fatalf("expected server error, output: %s", out)
	}
	if !strings.Contains(err.Error(), "expires_at must be RFC3339") {
		t.Fatalf("error = %v, want it to surface the server message", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/nexorious/ -run TestGenerate -v`
Expected: FAIL — compile error `undefined: newAPIKeyCmd` (referenced once Task 3 registers it; until then `api-key` is an unknown command and `newRootCmd` doesn't compile against the new helpers). Compile failure is the expected red.

- [ ] **Step 3: Create `cmd/nexorious/api_key.go` with the parent + generate**

```go
package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// newAPIKeyCmd returns the `api-key` parent command (aliased `keys`).
func newAPIKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "api-key",
		Aliases: []string{"keys"},
		Short:   "Manage your API keys on a Nexorious server",
	}
	cmd.AddCommand(newAPIKeyGenerateCmd())
	cmd.AddCommand(newAPIKeyListCmd())
	cmd.AddCommand(newAPIKeyRevokeCmd())
	return cmd
}

// currentProfile loads the CLI config and returns the active profile, erroring
// with a login hint if there is no stored API key.
func currentProfile() (clicfg.Profile, *clicfg.Config, error) {
	cfg, err := clicfg.Load()
	if err != nil {
		return clicfg.Profile{}, nil, err
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key == "" {
		return clicfg.Profile{}, nil, fmt.Errorf("not logged in (run `nexorious login` first)")
	}
	return p, cfg, nil
}

// formatNullableTime renders a *time.Time in local time, or zero when nil.
func formatNullableTime(t *time.Time, zero string) string {
	if t == nil {
		return zero
	}
	return t.Local().Format("2006-01-02 15:04")
}

func newAPIKeyGenerateCmd() *cobra.Command {
	var name, scopes, expiresAt string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Create a new API key and print it once",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGenerate(cmd, name, scopes, expiresAt)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Label for the key (required)")
	cmd.Flags().StringVar(&scopes, "scopes", "write", "Key scopes: read or write")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "Optional expiry as an RFC3339 timestamp")
	return cmd
}

func runGenerate(cmd *cobra.Command, name, scopes, expiresAt string) error {
	out := cmd.OutOrStdout()
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if scopes != "read" && scopes != "write" {
		return fmt.Errorf("--scopes must be 'read' or 'write'")
	}

	p, _, err := currentProfile()
	if err != nil {
		return err
	}
	client := cliclient.New(p.URL)

	// Non-fatal warning if an active key already uses this name (names aren't unique).
	if existing, err := client.ListAPIKeys(p.Key); err == nil {
		for _, k := range existing {
			if k.Name == name {
				fmt.Fprintf(out, "warning: an API key named %q already exists\n", name)
				break
			}
		}
	}

	var expPtr *string
	if expiresAt != "" {
		expPtr = &expiresAt
	}
	key, err := client.CreateAPIKeyWithBearer(p.Key, name, scopes, expPtr)
	if err != nil {
		return fmt.Errorf("create API key failed: %w", err)
	}

	fmt.Fprintf(out, "API key created. Copy it now — it will not be shown again:\n\n  %s\n\n", key.Key)
	fmt.Fprintf(out, "id:      %s\nname:    %s\nscopes:  %s\nexpires: %s\n",
		key.ID, key.Name, key.Scopes, formatNullableTime(key.ExpiresAt, "never"))
	return nil
}
```

> Note: `newAPIKeyListCmd` and `newAPIKeyRevokeCmd` are referenced here but defined in Tasks 4 and 5. To keep this task compiling on its own, add temporary minimal stubs at the bottom of the file now and replace them in those tasks:
>
> ```go
> func newAPIKeyListCmd() *cobra.Command   { return &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error { return runListKeys(cmd, false) }} }
> func newAPIKeyRevokeCmd() *cobra.Command { return &cobra.Command{Use: "revoke"} }
> ```
>
> The `runListKeys` stub must also exist; add `func runListKeys(cmd *cobra.Command, asJSON bool) error { return fmt.Errorf("not implemented") }` temporarily. These three stubs are removed/replaced in Tasks 4 and 5.
>
> **Imports for this task:** `fmt`, `time`, `github.com/spf13/cobra`, `github.com/drzero42/nexorious/internal/clicfg`, `github.com/drzero42/nexorious/internal/cliclient`. (`time` is needed now because `formatNullableTime` takes `*time.Time`.) The remaining imports come with their code: `encoding/json` + `text/tabwriter` in Task 4, `bufio` + `strings` in Task 5. Keep the import block to exactly what the present code uses or the build fails on unused imports.

- [ ] **Step 4: Register the command in `main.go`**

In `cmd/nexorious/main.go`, after `root.AddCommand(newWhoamiCmd())`, add:

```go
	root.AddCommand(newAPIKeyCmd())
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/nexorious/ -run TestGenerate -v`
Expected: PASS (all five `TestGenerate*`).

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/api_key.go cmd/nexorious/api_key_test.go cmd/nexorious/main.go
git commit -m "feat: api-key generate subcommand (#626)"
```

---

### Task 4: `list` subcommand

**Files:**
- Modify: `cmd/nexorious/api_key.go` (replace the `newAPIKeyListCmd`/`runListKeys` stubs; add `text/tabwriter`, `encoding/json` imports)
- Test: `cmd/nexorious/api_key_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/nexorious/api_key_test.go`:

```go
func TestListTable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"k1","name":"laptop","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	for _, want := range []string{"ID", "NAME", "SCOPES", "k1", "laptop", "never"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "No API keys.") {
		t.Fatalf("output = %q, want 'No API keys.'", out)
	}
}

func TestListJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"k1","name":"laptop","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON array: %v\n%s", err, out)
	}
	if len(parsed) != 1 || parsed[0]["id"] != "k1" {
		t.Fatalf("parsed = %+v, want one key k1", parsed)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/nexorious/ -run TestList -v`
Expected: FAIL — the stub `runListKeys` returns `not implemented` / output lacks the table.

- [ ] **Step 3: Replace the `list` stub with the real implementation**

In `cmd/nexorious/api_key.go`, remove the temporary `newAPIKeyListCmd` and `runListKeys` stubs and add `encoding/json`, `text/tabwriter` to the import block, then add:

```go
func newAPIKeyListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your API keys",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runListKeys(cmd, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit raw JSON instead of a table")
	return cmd
}

func runListKeys(cmd *cobra.Command, asJSON bool) error {
	out := cmd.OutOrStdout()
	p, _, err := currentProfile()
	if err != nil {
		return err
	}
	keys, err := cliclient.New(p.URL).ListAPIKeys(p.Key)
	if err != nil {
		return fmt.Errorf("list API keys failed: %w", err)
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(keys); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
		return nil
	}

	if len(keys) == 0 {
		fmt.Fprintln(out, "No API keys.")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSCOPES\tCREATED\tLAST USED\tEXPIRES")
	for _, k := range keys {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			k.ID, k.Name, k.Scopes,
			k.CreatedAt.Local().Format("2006-01-02 15:04"),
			formatNullableTime(k.LastUsedAt, "never"),
			formatNullableTime(k.ExpiresAt, "–"),
		)
	}
	return tw.Flush()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/nexorious/ -run 'TestList|TestGenerate' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/api_key.go cmd/nexorious/api_key_test.go
git commit -m "feat: api-key list subcommand (#626)"
```

---

### Task 5: `revoke` subcommand + `resolveKeyID`

**Files:**
- Modify: `cmd/nexorious/api_key.go` (replace the `newAPIKeyRevokeCmd` stub; add `bufio`, `strings` imports)
- Test: `cmd/nexorious/api_key_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/nexorious/api_key_test.go`:

```go
// revokeServer serves a list of the given keys and records DELETEs into revoked.
func revokeServer(t *testing.T, listJSON string, revoked *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(listJSON))
	})
	mux.HandleFunc("/api/auth/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		*revoked = append(*revoked, r.URL.Path[len("/api/auth/api-keys/"):])
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRevokeByID(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked []string
	srv := revokeServer(t, `[{"id":"k1","name":"laptop","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`, &revoked)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "revoke", "k1")
	if err != nil {
		t.Fatalf("revoke: %v\n%s", err, out)
	}
	if len(revoked) != 1 || revoked[0] != "k1" {
		t.Fatalf("revoked = %v, want [k1]", revoked)
	}
}

func TestRevokeByName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked []string
	srv := revokeServer(t, `[{"id":"k1","name":"laptop","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`, &revoked)
	seedProfile(t, srv.URL)

	if _, err := runCmd(t, "api-key", "revoke", "laptop"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if len(revoked) != 1 || revoked[0] != "k1" {
		t.Fatalf("revoked = %v, want [k1]", revoked)
	}
}

func TestRevokeAmbiguousName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked []string
	srv := revokeServer(t, `[{"id":"k1","name":"dup","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null},{"id":"k2","name":"dup","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`, &revoked)
	seedProfile(t, srv.URL)

	if _, err := runCmd(t, "api-key", "revoke", "dup"); err == nil {
		t.Fatal("expected ambiguous-name error")
	}
	if len(revoked) != 0 {
		t.Fatalf("nothing should be revoked on ambiguity, got %v", revoked)
	}
}

func TestRevokeUnknown(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked []string
	srv := revokeServer(t, `[]`, &revoked)
	seedProfile(t, srv.URL)

	if _, err := runCmd(t, "api-key", "revoke", "nope"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRevokeSelfWithYesLogsOut(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked []string
	// seedProfile stores KeyID "self-key"; list returns that id.
	srv := revokeServer(t, `[{"id":"self-key","name":"cli@host","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`, &revoked)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "revoke", "self-key", "--yes")
	if err != nil {
		t.Fatalf("revoke: %v\n%s", err, out)
	}
	if len(revoked) != 1 || revoked[0] != "self-key" {
		t.Fatalf("revoked = %v, want [self-key]", revoked)
	}
	if !strings.Contains(out, "logged out") {
		t.Fatalf("output = %q, want logged-out message", out)
	}
	cfg, _ := clicfg.Load()
	p, _ := cfg.CurrentProfile()
	if p.Key != "" || p.KeyID != "" {
		t.Fatalf("config not cleared after self-revoke: %+v", p)
	}
}

func TestRevokeSelfDeclinedAborts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked []string
	srv := revokeServer(t, `[{"id":"self-key","name":"cli@host","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`, &revoked)
	seedProfile(t, srv.URL)

	// Drive stdin with "n\n" to decline the prompt.
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"api-key", "revoke", "self-key"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected abort error when prompt declined")
	}
	if len(revoked) != 0 {
		t.Fatalf("nothing should be revoked when declined, got %v", revoked)
	}
	cfg, _ := clicfg.Load()
	p, _ := cfg.CurrentProfile()
	if p.Key == "" {
		t.Fatal("config should be untouched when aborted")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/nexorious/ -run TestRevoke -v`
Expected: FAIL — the `revoke` stub has no `RunE`, so `revoke` does nothing / `resolveKeyID` is undefined (compile error).

- [ ] **Step 3: Replace the `revoke` stub with the real implementation**

In `cmd/nexorious/api_key.go`, remove the temporary `newAPIKeyRevokeCmd` stub, add `bufio` and `strings` to the import block, and add:

```go
func newAPIKeyRevokeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke <id-or-name>",
		Short: "Revoke an API key by id or name (from `api-key list`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRevoke(cmd, args[0], yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false,
		"Skip the confirmation prompt when revoking the key this CLI is using")
	return cmd
}

func runRevoke(cmd *cobra.Command, idOrName string, yes bool) error {
	out := cmd.OutOrStdout()
	p, cfg, err := currentProfile()
	if err != nil {
		return err
	}
	client := cliclient.New(p.URL)

	keys, err := client.ListAPIKeys(p.Key)
	if err != nil {
		return fmt.Errorf("list API keys failed: %w", err)
	}
	targetID, err := resolveKeyID(keys, idOrName)
	if err != nil {
		return err
	}

	self := targetID == p.KeyID
	if self && !yes {
		fmt.Fprint(out, "Revoke the key this CLI is currently using? This will log you out. [y/N] ")
		answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted")
		}
	}

	if err := client.RevokeAPIKeyWithBearer(p.Key, targetID); err != nil {
		return fmt.Errorf("revoke failed: %w", err)
	}

	if self {
		url := p.URL
		if err := clearStoredKey(cfg); err != nil {
			return err
		}
		fmt.Fprintf(out, "Revoked API key %s.\nThat was the key this CLI was using — you have been logged out of %s.\n", targetID, url)
		return nil
	}

	fmt.Fprintf(out, "Revoked API key %s.\n", targetID)
	return nil
}

// resolveKeyID maps an id-or-name argument to a single key id. An exact id match
// wins; otherwise it matches active keys by name, requiring exactly one match.
func resolveKeyID(keys []cliclient.APIKey, idOrName string) (string, error) {
	for _, k := range keys {
		if k.ID == idOrName {
			return k.ID, nil
		}
	}
	var matches []string
	for _, k := range keys {
		if k.Name == idOrName {
			matches = append(matches, k.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no API key with id or name %q", idOrName)
	default:
		return "", fmt.Errorf("multiple active keys named %q; revoke by id instead (see `api-key list`)", idOrName)
	}
}
```

- [ ] **Step 4: Run the full command-package test suite**

Run: `go test ./cmd/nexorious/ -v`
Expected: PASS (all `TestGenerate*`, `TestList*`, `TestRevoke*`, plus the existing login/logout/whoami/version tests).

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/api_key.go cmd/nexorious/api_key_test.go
git commit -m "feat: api-key revoke subcommand (#626)"
```

---

### Task 6: Final verification & docs

**Files:**
- Verify only: `slumber.yaml` (no change expected), full build & tests.

- [ ] **Step 1: Confirm slumber already covers the endpoints**

Run: `grep -n "api-keys\|list_api_keys" slumber.yaml`
Expected: existing `create`/`list_api_keys`/revoke requests are present (lines ~64/287/296). No edit needed — the CLI adds no server routes. If `slumber` is on PATH, optionally run `slumber collection` and expect no error.

- [ ] **Step 2: Build the binary**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 3: Run the full Go test suite for the touched packages**

Run: `go test ./internal/cliclient/ ./cmd/nexorious/ -count=1`
Expected: `ok` for both packages.

- [ ] **Step 4: Lint the touched files**

Run: `golangci-lint run ./internal/cliclient/... ./cmd/nexorious/...`
Expected: no findings. (In particular, confirm no `errcheck` discards were introduced outside the allowed `Close`/`Fprint` family; the new code returns or handles every error.)

- [ ] **Step 5: Manual smoke test (optional, requires a running server + login)**

```bash
./nexorious api-key generate --name ci-test
./nexorious api-key list
./nexorious keys list            # alias works
./nexorious api-key revoke ci-test
```
Expected: generate prints a `nxr_...` key once; list shows it as a row; revoke by name removes it.

- [ ] **Step 6: No extra commit needed** unless Step 1/4 surfaced a fix; the feature commits from Tasks 1–5 stand.

---

## Notes for the implementer

- `ListAPIKeys` returns only non-revoked keys (the server filters `revoked_at IS NULL`), so name resolution in `revoke` is automatically scoped to active keys.
- The raw API key is printed by `generate` and **never** written to `clicfg` — only `login` stores a key, and that is out of scope here.
- The temporary stubs in Task 3 exist solely so each task compiles independently under TDD; Tasks 4 and 5 replace them. If you implement Tasks 3–5 in one sitting, you may skip the stubs and add the real `list`/`revoke` directly — but keep each task's tests green as you go.
- Self-revoke reuses `clearStoredKey` (Task 2) and must NOT call the server a second time (the key is already revoked; a second DELETE would 404).
