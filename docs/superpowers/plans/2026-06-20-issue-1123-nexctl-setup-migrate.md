# `nexctl setup` + `nexctl migrate` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `nexctl` a `setup` command group (create-admin + backup restore, full parity with the web setup UI) and a `migrate` command, relocate the misplaced REST-client `setup` out of the `nexorious` server binary, and ship `nexctl` in the container image.

**Architecture:** The setup/migrate zones are unauthenticated, pre-bootstrap REST endpoints. New `cliclient` methods wrap the three setup-zone backup endpoints; the health/migrate/admin orchestration that today lives privately in `cmd/nexorious/setup.go` is reimplemented as exported functions in `internal/cliauth` so both `nexctl setup` and `nexctl migrate` consume one copy. `nexorious setup` is then deleted, and the container image gains `nexctl`.

**Tech Stack:** Go 1.26, cobra, `internal/cliclient` (REST), `internal/cliauth` (bootstrap orchestration), `internal/cliui` (TTY/prompt/JSON helpers), `internal/clicfg` (profile config), `httptest` for tests.

## Global Constraints

- Go: standard conventions; errors wrapped with `fmt.Errorf("context: %w", err)`, never panicked.
- `errcheck` runs with `check-blank`: every discarded error must be handled or annotated; in non-test code `defer func() { _ = x.Close() }()` is covered by the `std-error-handling` preset.
- `gosec` enabled: annotate operator-supplied file paths with `//nolint:gosec // <reason>` (test files are exempt).
- The `nexorious` server binary must **never** link `nexctl`-only deps; this work touches only shared `internal/` packages already linked by both binaries (`cliclient`, `cliauth`, `cliui`, `clicfg`).
- Commit type for the feature is `feat:` (NOT `feat!`) — capability is preserved, only its host binary changes; the breaking removal of `nexorious setup` is documented, not flagged as a major bump.
- After Go changes that remove/rename callers, run `make deadcode` (`go run golang.org/x/tools/cmd/deadcode@latest -test ./...`) and reconcile new entries.
- Pre-push hooks run the full `go test ./...`; per-task you run the targeted tests shown.

---

## File Structure

- **Modify** `internal/cliclient/client.go` — add the no-auth-header guard to `doBearer`; add `SetupListBackups`, `SetupRestoreFromDisk`, and the `SetupBackupEntry`/`SetupBackupManifest` types.
- **Modify** `internal/cliclient/import.go` — add the no-auth-header guard to `doBearerMultipart`.
- **Modify** `internal/cliclient/backup.go` — add `SetupRestoreUpload` (lives with the other restore wrappers).
- **Create** `internal/cliclient/setup_test.go` — tests for the three new methods + the no-auth-header assertion.
- **Create** `internal/cliauth/setup.go` — exported `Preflight`, `RunMigrateAndWait`, `MigrationFailedErr`, `ReportSetupResult` + poll constants.
- **Create** `internal/cliauth/setup_test.go` — unit tests for the orchestration against `httptest`.
- **Create** `cmd/nexctl/setup.go` — the `setup` command group (`admin`, `backups`, `restore`) + password helpers.
- **Create** `cmd/nexctl/setup_test.go` — command tests (ported + extended from the nexorious version).
- **Create** `cmd/nexctl/migrate.go` — the `migrate` / `migrate status` commands.
- **Create** `cmd/nexctl/migrate_test.go` — command tests.
- **Modify** `cmd/nexctl/main.go` — register `newSetupCmd()` + `newMigrateCmd()`; add `resolveServerURL`.
- **Delete** `cmd/nexorious/setup.go`, `cmd/nexorious/setup_cmd_test.go`; **modify** `cmd/nexorious/main.go` (unregister).
- **Modify** `Dockerfile` — build/ship `nexctl`.
- **Modify** `docs/admin-guide.md`, `docs/user-guide.md`, `CLAUDE.md`.

---

## Task 1: cliclient — no-auth-header guard + setup-zone backup methods

**Files:**
- Modify: `internal/cliclient/client.go` (the `doBearer` header line; add types + two methods)
- Modify: `internal/cliclient/import.go` (the `doBearerMultipart` header line)
- Modify: `internal/cliclient/backup.go` (add `SetupRestoreUpload`)
- Test: `internal/cliclient/setup_test.go`

**Interfaces:**
- Consumes: existing `doBearer(method, path, key string, body, out any) error`, `doBearerMultipart(method, path, key, filename string, body io.Reader, fields map[string]string, out any) error`, `New(baseURL string) *Client`.
- Produces:
  - `type SetupBackupManifest struct { CreatedAt, AppVersion, MigrationVersion, BackupType string; Stats struct{ Users, Games, Tags int } }`
  - `type SetupBackupEntry struct { Filename string; SizeBytes int64; ModTime string; Restorable bool; Reason string; Manifest *SetupBackupManifest }`
  - `func (c *Client) SetupListBackups() ([]SetupBackupEntry, error)`
  - `func (c *Client) SetupRestoreFromDisk(filename string) error`
  - `func (c *Client) SetupRestoreUpload(filename string, body io.Reader) error`

- [ ] **Step 1: Write the failing tests**

Create `internal/cliclient/setup_test.go`:

```go
package cliclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupListBackups(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header = %q; want empty (setup zone is unauthenticated)", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backups":[{"filename":"b.tar.gz","size_bytes":2048,"mtime":"2026-06-20T09:30:15Z","restorable":true,"manifest":{"app_version":"0.90.0","migration_version":"v0.90.0","backup_type":"manual","stats":{"users":1,"games":42,"tags":3}}}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	entries, err := New(srv.URL).SetupListBackups()
	if err != nil {
		t.Fatalf("SetupListBackups: %v", err)
	}
	if len(entries) != 1 || entries[0].Filename != "b.tar.gz" || entries[0].SizeBytes != 2048 || !entries[0].Restorable {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Manifest == nil || entries[0].Manifest.Stats.Games != 42 {
		t.Fatalf("manifest = %+v", entries[0].Manifest)
	}
}

func TestSetupListBackupsForbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"restore during setup is only available when no users exist"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := New(srv.URL).SetupListBackups()
	if err == nil || !strings.Contains(err.Error(), "no users exist") {
		t.Fatalf("err = %v; want forbidden message", err)
	}
}

func TestSetupRestoreFromDisk(t *testing.T) {
	var gotBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/restore/disk", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header = %q; want empty", got)
		}
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"message":"Backup restored successfully."}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := New(srv.URL).SetupRestoreFromDisk("b.tar.gz"); err != nil {
		t.Fatalf("SetupRestoreFromDisk: %v", err)
	}
	if !strings.Contains(gotBody, `"filename":"b.tar.gz"`) {
		t.Fatalf("body = %q; want filename field", gotBody)
	}
}

func TestSetupRestoreUpload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/restore", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header = %q; want empty", got)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type = %q; want multipart", r.Header.Get("Content-Type"))
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = f.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := New(srv.URL).SetupRestoreUpload("b.tar.gz", strings.NewReader("ARCHIVE-BYTES")); err != nil {
		t.Fatalf("SetupRestoreUpload: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cliclient/ -run 'TestSetup(ListBackups|RestoreFromDisk|RestoreUpload)' -v`
Expected: FAIL to compile — `SetupListBackups`, `SetupRestoreFromDisk`, `SetupRestoreUpload` undefined.

- [ ] **Step 3: Add the no-auth-header guard to `doBearer`**

In `internal/cliclient/client.go`, find in `doBearer`:

```go
	req.Header.Set("Authorization", "Bearer "+key)
```

Replace with:

```go
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
```

- [ ] **Step 4: Add the no-auth-header guard to `doBearerMultipart`**

In `internal/cliclient/import.go`, find in `doBearerMultipart`:

```go
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+key)
```

Replace with:

```go
	req.Header.Set("Content-Type", contentType)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
```

- [ ] **Step 5: Add the types + `SetupListBackups` + `SetupRestoreFromDisk` to `client.go`**

Append to `internal/cliclient/client.go` (it already imports `net/http`; no new imports needed):

```go
// SetupBackupManifest is the manifest sub-object of a setup-zone backup entry.
type SetupBackupManifest struct {
	CreatedAt        string `json:"created_at"`
	AppVersion       string `json:"app_version"`
	MigrationVersion string `json:"migration_version"`
	BackupType       string `json:"backup_type"`
	Stats            struct {
		Users int `json:"users"`
		Games int `json:"games"`
		Tags  int `json:"tags"`
	} `json:"stats"`
}

// SetupBackupEntry is one candidate archive from GET /api/auth/setup/backups.
type SetupBackupEntry struct {
	Filename   string               `json:"filename"`
	SizeBytes  int64                `json:"size_bytes"`
	ModTime    string               `json:"mtime"`
	Restorable bool                 `json:"restorable"`
	Reason     string               `json:"reason,omitempty"`
	Manifest   *SetupBackupManifest `json:"manifest,omitempty"`
}

// SetupListBackups lists candidate on-disk backup archives during initial
// setup via GET /api/auth/setup/backups. The endpoint is unauthenticated
// (pre-bootstrap), so no API key is sent.
func (c *Client) SetupListBackups() ([]SetupBackupEntry, error) {
	var env struct {
		Backups []SetupBackupEntry `json:"backups"`
	}
	if err := c.doBearer(http.MethodGet, "/api/auth/setup/backups", "", nil, &env); err != nil {
		return nil, err
	}
	return env.Backups, nil
}

// SetupRestoreFromDisk restores a fresh instance from a named on-disk backup
// via POST /api/auth/setup/restore/disk. Unauthenticated (pre-bootstrap).
func (c *Client) SetupRestoreFromDisk(filename string) error {
	return c.doBearer(http.MethodPost, "/api/auth/setup/restore/disk", "", map[string]string{"filename": filename}, nil)
}
```

- [ ] **Step 6: Add `SetupRestoreUpload` to `backup.go`**

Append to `internal/cliclient/backup.go` (it already imports `io` and `net/http`):

```go
// SetupRestoreUpload uploads a backup archive and restores a fresh instance
// from it via POST /api/auth/setup/restore (multipart field "file"). The body
// is streamed, so the caller can pass an *os.File directly. Unauthenticated
// (pre-bootstrap), so no API key is sent.
func (c *Client) SetupRestoreUpload(filename string, body io.Reader) error {
	return c.doBearerMultipart(http.MethodPost, "/api/auth/setup/restore", "", filename, body, nil, nil)
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/cliclient/ -run 'TestSetup(ListBackups|RestoreFromDisk|RestoreUpload)' -v`
Expected: PASS (all four tests).

- [ ] **Step 8: Verify the auth-guard change didn't break existing cliclient tests**

Run: `go test ./internal/cliclient/`
Expected: ok (all existing authenticated tests still send their Bearer header — they pass a non-empty key).

- [ ] **Step 9: Commit**

```bash
git add internal/cliclient/client.go internal/cliclient/import.go internal/cliclient/backup.go internal/cliclient/setup_test.go
git commit -m "feat(cliclient): add setup-zone backup methods and skip auth header on empty key"
```

---

## Task 2: cliauth — exported setup/migrate orchestration

**Files:**
- Create: `internal/cliauth/setup.go`
- Test: `internal/cliauth/setup_test.go`

**Interfaces:**
- Consumes: `*cliclient.Client` methods `Health() (string, error)`, `RunMigrations() error`, `MigrationStatus() (state, detail string, err error)`; `*cliclient.SetupResult{StatusCode int; Location, Message string}`.
- Produces:
  - `func Preflight(out io.Writer, client *cliclient.Client, url string) error`
  - `func RunMigrateAndWait(out io.Writer, client *cliclient.Client) error`
  - `func MigrationFailedErr(client *cliclient.Client) error`
  - `func ReportSetupResult(out io.Writer, username string, res *cliclient.SetupResult) error`

- [ ] **Step 1: Write the failing tests**

Create `internal/cliauth/setup_test.go`:

```go
package cliauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func TestPreflightOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := Preflight(&bytes.Buffer{}, cliclient.New(srv.URL), srv.URL); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
}

func TestPreflightRunsMigrations(t *testing.T) {
	ran := false
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "needs_migration"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	var out bytes.Buffer
	if err := Preflight(&out, cliclient.New(srv.URL), srv.URL); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if !ran {
		t.Fatal("migrations were not triggered")
	}
	if !strings.Contains(out.String(), "Migrations complete.") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestPreflightMigrationFailedSurfacesDetail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "migration_failed"})
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "migration_failed", "error": "migration 003 failed: boom"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := Preflight(&bytes.Buffer{}, cliclient.New(srv.URL), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "previously failed") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v; want previously-failed + detail", err)
	}
}

func TestReportSetupResultCreated(t *testing.T) {
	var out bytes.Buffer
	err := ReportSetupResult(&out, "admin", &cliclient.SetupResult{StatusCode: http.StatusCreated})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if !strings.Contains(out.String(), `Admin user "admin" created.`) {
		t.Fatalf("out = %q", out.String())
	}
}

func TestReportSetupResultRedirectToMigrate(t *testing.T) {
	err := ReportSetupResult(&bytes.Buffer{}, "admin", &cliclient.SetupResult{StatusCode: http.StatusFound, Location: "/migrate"})
	if err == nil || !strings.Contains(err.Error(), `run "nexctl migrate"`) {
		t.Fatalf("err = %v; want nexctl-migrate hint", err)
	}
}

func TestReportSetupResultForbidden(t *testing.T) {
	err := ReportSetupResult(&bytes.Buffer{}, "admin", &cliclient.SetupResult{StatusCode: http.StatusForbidden})
	if err == nil || !strings.Contains(err.Error(), "already complete") {
		t.Fatalf("err = %v; want already-complete", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cliauth/ -run 'TestPreflight|TestReportSetupResult' -v`
Expected: FAIL to compile — `Preflight`, `ReportSetupResult` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/cliauth/setup.go`:

```go
package cliauth

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/drzero42/nexorious/internal/cliclient"
)

const (
	migratePollInterval = 1 * time.Second
	migrateTimeout      = 5 * time.Minute
)

// Preflight checks server health before credentials are read. Pending
// migrations (and a migration another process is already running) are applied
// and waited on automatically so a fresh instance comes up in one command. A
// migration that previously *failed* aborts with the failure detail rather than
// blindly retrying a broken migration.
func Preflight(out io.Writer, client *cliclient.Client, url string) error {
	status, err := client.Health()
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	switch status {
	case "ok":
		return nil
	case "db_unavailable":
		return fmt.Errorf("database is unavailable")
	case "needs_migration", "migrating":
		return RunMigrateAndWait(out, client)
	case "migration_failed":
		return MigrationFailedErr(client)
	default:
		return fmt.Errorf("server is not ready (status: %s)", status)
	}
}

// MigrationFailedErr builds the abort error for a server stuck in
// migration_failed, surfacing the failure detail when available.
func MigrationFailedErr(client *cliclient.Client) error {
	_, detail, err := client.MigrationStatus()
	if err == nil && detail != "" {
		return fmt.Errorf("migrations previously failed: %s — resolve the underlying problem (check the server logs) before retrying", detail)
	}
	return fmt.Errorf("migrations previously failed — resolve the underlying problem (check the server logs) before retrying")
}

// RunMigrateAndWait triggers migrations on the server and polls until the
// server reports ready, or fails / times out. Driven over HTTP so the running
// server's own migrator applies them.
func RunMigrateAndWait(out io.Writer, client *cliclient.Client) error {
	fmt.Fprintln(out, "Running pending migrations...")
	if err := client.RunMigrations(); err != nil {
		return fmt.Errorf("start migrations: %w", err)
	}
	deadline := time.Now().Add(migrateTimeout)
	for {
		state, detail, err := client.MigrationStatus()
		if err != nil {
			return fmt.Errorf("poll migration status: %w", err)
		}
		switch state {
		case "ready":
			fmt.Fprintln(out, "Migrations complete.")
			return nil
		case "migration_failed":
			if detail != "" {
				return fmt.Errorf("migrations failed: %s", detail)
			}
			return fmt.Errorf("migrations failed — check the server logs")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for migrations (last state: %s)", migrateTimeout, state)
		}
		time.Sleep(migratePollInterval)
	}
}

// ReportSetupResult maps a SetupResult to user-facing output and an error
// (non-nil => non-zero exit). A nil error means success.
func ReportSetupResult(out io.Writer, username string, res *cliclient.SetupResult) error {
	switch res.StatusCode {
	case http.StatusCreated:
		fmt.Fprintf(out, "Admin user %q created.\n", username)
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("setup already complete; an admin user already exists")
	case http.StatusBadRequest:
		if res.Message != "" {
			return errors.New(res.Message)
		}
		return errors.New("invalid request")
	case http.StatusFound, http.StatusMovedPermanently, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		switch {
		case strings.HasPrefix(res.Location, "/migrate"):
			return fmt.Errorf("migrations are pending — run \"nexctl migrate\" first")
		case strings.HasPrefix(res.Location, "/db-error"):
			return fmt.Errorf("database is unavailable")
		default:
			return fmt.Errorf("server redirected to %q; setup is not currently available", res.Location)
		}
	default:
		return fmt.Errorf("unexpected response from server: %d", res.StatusCode)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cliauth/ -run 'TestPreflight|TestReportSetupResult' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cliauth/setup.go internal/cliauth/setup_test.go
git commit -m "feat(cliauth): export setup/migrate bootstrap orchestration"
```

---

## Task 3: `nexctl setup admin`

**Files:**
- Create: `cmd/nexctl/setup.go`
- Modify: `cmd/nexctl/main.go` (add `resolveServerURL`, register `newSetupCmd`)
- Test: `cmd/nexctl/setup_test.go`

**Interfaces:**
- Consumes: `cliauth.Preflight`, `cliauth.ReportSetupResult`, `cliauth.LoginAndStoreKey`, `cliauth.DefaultServerURL`; `cliclient.New`, `(*Client).SetupAdmin`; `cliui.Prompt`, `cliui.ReadPassword`; `clicfg.Load`, `(*Config).ProfileNamed`; existing `profileName(cmd, cfg)`, `flagBool(cmd, name)`.
- Produces (used by Tasks 4 & 5):
  - `func newSetupCmd() *cobra.Command`
  - `func resolveServerURL(cmd *cobra.Command) string` (in `main.go`)
  - `func confirmInteractivePassword(read func(label string) (string, error)) (string, error)`

- [ ] **Step 1: Add `resolveServerURL` to `cmd/nexctl/main.go`**

Add the import `"github.com/drzero42/nexorious/internal/cliauth"` to `main.go`'s import block, then add:

```go
// resolveServerURL resolves the target server URL for unauthenticated
// (setup/migrate) commands: the --url flag if set, else the current profile's
// stored URL, else cliauth.DefaultServerURL. Unlike resolveProfile, no API key
// is required — the setup and migration zones are pre-bootstrap.
func resolveServerURL(cmd *cobra.Command) string {
	if u, _ := cmd.Flags().GetString("url"); u != "" { //nolint:errcheck // absent flag yields ""
		return u
	}
	if cfg, err := clicfg.Load(); err == nil {
		if p, ok := cfg.ProfileNamed(profileName(cmd, cfg)); ok && p.URL != "" {
			return p.URL
		}
	}
	return cliauth.DefaultServerURL
}
```

Then register the new commands in `newRootCmd()` after `newMCPCmd()`:

```go
	root.AddCommand(newSetupCmd())
	root.AddCommand(newMigrateCmd())
```

- [ ] **Step 2: Write the failing tests**

Create `cmd/nexctl/setup_test.go`:

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

// runNexctl executes the given args with piped stdin and an isolated config home.
func runNexctl(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestConfirmInteractivePasswordMismatch(t *testing.T) {
	entries := []string{"first-secret", "second-secret"}
	i := 0
	read := func(string) (string, error) { v := entries[i]; i++; return v, nil }
	if _, err := confirmInteractivePassword(read); err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestConfirmInteractivePasswordMatch(t *testing.T) {
	read := func(string) (string, error) { return "supersecret", nil }
	pw, err := confirmInteractivePassword(read)
	if err != nil || pw != "supersecret" {
		t.Fatalf("pw=%q err=%v; want supersecret", pw, err)
	}
}

func TestConfirmInteractivePasswordBothEmpty(t *testing.T) {
	read := func(string) (string, error) { return "", nil }
	_, err := confirmInteractivePassword(read)
	if err == nil || !strings.Contains(err.Error(), "password is required") {
		t.Fatalf("err = %v; want password-required", err)
	}
}

// setupAdminStub serves /health (ok), a configurable /api/auth/setup/admin.
type setupAdminStub struct {
	adminStatus   int
	adminMessage  string
	adminLocation string
}

func (s setupAdminStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) {
		code := s.adminStatus
		if code == 0 {
			code = http.StatusCreated
		}
		if s.adminLocation != "" {
			w.Header().Set("Location", s.adminLocation)
		}
		w.WriteHeader(code)
		if s.adminMessage != "" {
			_ = json.NewEncoder(w).Encode(map[string]string{"message": s.adminMessage})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetupAdminCreates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := setupAdminStub{}.server(t)
	out, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err != nil {
		t.Fatalf("setup admin: %v\n%s", err, out)
	}
	if !strings.Contains(out, `Admin user "admin" created.`) {
		t.Fatalf("out = %q", out)
	}
}

func TestSetupAdminAlreadyComplete(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := setupAdminStub{adminStatus: http.StatusForbidden, adminMessage: "setup already complete"}.server(t)
	_, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("err = %v; want already-complete", err)
	}
}

func TestSetupAdminMissingUsernameWithPasswordStdin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", "http://127.0.0.1:1", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "--username is required") {
		t.Fatalf("err = %v; want username-required", err)
	}
}

func TestSetupAdminNoPasswordSourceNonTTY(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runNexctl(t, "", "setup", "admin", "--url", "http://127.0.0.1:1", "--username", "admin")
	if err == nil || !strings.Contains(err.Error(), "no password") {
		t.Fatalf("err = %v; want no-password", err)
	}
}

// loginStub serves admin-setup plus the full login bootstrap for --login.
func loginStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "admin"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetupAdminLoginStoresKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := loginStub(t)
	out, err := runNexctl(t, "supersecret\n", "setup", "admin", "--url", srv.URL, "--username", "admin", "--password-stdin", "--login")
	if err != nil {
		t.Fatalf("setup admin --login: %v\n%s", err, out)
	}
	if !strings.Contains(out, `Admin user "admin" created.`) || !strings.Contains(out, "Logged in to") {
		t.Fatalf("out = %q", out)
	}
	cfg, err := clicfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key != "nxr_minted" || p.KeyID != "key-1" {
		t.Fatalf("profile = %+v ok=%v", p, ok)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./cmd/nexctl/ -run 'TestSetupAdmin|TestConfirmInteractivePassword' -v`
Expected: FAIL to compile — `newSetupCmd`, `confirmInteractivePassword` undefined.

- [ ] **Step 4: Write `cmd/nexctl/setup.go` (group + admin)**

Create `cmd/nexctl/setup.go`:

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

	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Bootstrap a fresh instance (create admin / restore backup) — pre-auth",
		Long: "Bootstrap a fresh, user-less Nexorious instance by driving the\n" +
			"unauthenticated setup-zone endpoints over HTTP: create the first admin\n" +
			"user, or restore from a backup (disaster recovery). Intended to run from\n" +
			"a workstation or via `kubectl exec` into a fresh instance.",
	}
	cmd.AddCommand(newSetupAdminCmd())
	return cmd
}

func newSetupAdminCmd() *cobra.Command {
	var username string
	var passwordStdin, login bool
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Create the first admin user on a fresh instance",
		Long: "Create the first admin user by driving POST /api/auth/setup/admin.\n" +
			"Pending database migrations are applied automatically first. Pass --login\n" +
			"to also log in and store an API key so subsequent commands are ready.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetupAdmin(cmd, username, passwordStdin, login)
		},
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	cmd.Flags().StringVar(&username, "username", "", "Admin username (prompted if omitted; required with --password-stdin)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin instead of prompting")
	cmd.Flags().BoolVar(&login, "login", false, "After creating the admin, log in with the same credentials and store an API key")
	return cmd
}

func runSetupAdmin(cmd *cobra.Command, username string, passwordStdin, login bool) error {
	out := cmd.OutOrStdout()
	src := cmd.InOrStdin()
	in := bufio.NewReader(src)

	stdinIsTTY := false
	if f, ok := src.(*os.File); ok {
		stdinIsTTY = term.IsTerminal(int(f.Fd()))
	}
	if !passwordStdin && !stdinIsTTY {
		return fmt.Errorf("no password: pass --password-stdin to read it from stdin, or run interactively")
	}
	if passwordStdin && username == "" {
		return fmt.Errorf("--username is required with --password-stdin")
	}

	url := resolveServerURL(cmd)
	client := cliclient.New(url)

	if err := cliauth.Preflight(out, client, url); err != nil {
		return err
	}

	if username == "" {
		var err error
		username, err = cliui.Prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
		if username == "" {
			return fmt.Errorf("username is required")
		}
	}

	password, err := resolveSetupPassword(in, src, out, passwordStdin)
	if err != nil {
		return err
	}

	res, err := client.SetupAdmin(username, password)
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	if err := cliauth.ReportSetupResult(out, username, res); err != nil {
		return err
	}

	if login {
		cfg, err := clicfg.Load()
		if err != nil {
			return fmt.Errorf("admin created, but loading CLI config for --login failed (run \"nexctl account login\"): %w", err)
		}
		if err := cliauth.LoginAndStoreKey(out, client, cfg, profileName(cmd, cfg), url, username, password); err != nil {
			return fmt.Errorf("admin created, but --login failed (run \"nexctl account login\"): %w", err)
		}
	}
	return nil
}

// resolveSetupPassword returns the admin password. With passwordStdin it reads a
// single line from in. Otherwise it prompts twice (no echo on a TTY) and requires
// the entries to match.
func resolveSetupPassword(in *bufio.Reader, src io.Reader, out io.Writer, passwordStdin bool) (string, error) {
	if passwordStdin {
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		pw := strings.TrimSpace(line)
		if pw == "" {
			return "", fmt.Errorf("empty password on stdin")
		}
		return pw, nil
	}
	read := func(label string) (string, error) {
		return cliui.ReadPassword(in, src, out, label)
	}
	return confirmInteractivePassword(read)
}

// confirmInteractivePassword prompts for the password twice via read and returns
// it only if both entries match and are non-empty.
func confirmInteractivePassword(read func(label string) (string, error)) (string, error) {
	first, err := read("Password: ")
	if err != nil {
		return "", err
	}
	second, err := read("Confirm password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", fmt.Errorf("passwords do not match")
	}
	if first == "" {
		return "", fmt.Errorf("password is required")
	}
	return first, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/nexctl/ -run 'TestSetupAdmin|TestConfirmInteractivePassword' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/nexctl/setup.go cmd/nexctl/setup_test.go cmd/nexctl/main.go
git commit -m "feat(nexctl): add 'setup admin' to bootstrap the first admin user"
```

---

## Task 4: `nexctl setup backups` + `nexctl setup restore`

**Files:**
- Modify: `cmd/nexctl/setup.go` (add two subcommands, register them)
- Test: `cmd/nexctl/setup_test.go` (append cases)

**Interfaces:**
- Consumes: `(*cliclient.Client).SetupListBackups`, `.SetupRestoreFromDisk`, `.SetupRestoreUpload`; `cliui.Confirm`, `cliui.EncodeJSON`; `humanBackupBytes` (from `backup.go`, same package); `resolveServerURL`, `flagBool`.
- Produces: `setup backups` and `setup restore` subcommands wired into `newSetupCmd()`.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/nexctl/setup_test.go`:

```go
func TestSetupBackupsList(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"backups":[{"filename":"b.tar.gz","size_bytes":2048,"mtime":"2026-06-20T09:30:15Z","restorable":true}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "setup", "backups", "--url", srv.URL)
	if err != nil {
		t.Fatalf("setup backups: %v\n%s", err, out)
	}
	if !strings.Contains(out, "b.tar.gz") || !strings.Contains(out, "FILENAME") {
		t.Fatalf("out = %q", out)
	}
}

func TestSetupBackupsQuiet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"backups":[{"filename":"b.tar.gz","size_bytes":2048,"restorable":true}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "setup", "backups", "--url", srv.URL, "-q")
	if err != nil {
		t.Fatalf("setup backups -q: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "b.tar.gz" {
		t.Fatalf("out = %q; want bare filename", out)
	}
}

func TestSetupRestoreRequiresExactlyOne(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := runNexctl(t, "y\n", "setup", "restore", "--url", "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("err = %v; want exactly-one error (neither name nor --file)", err)
	}
	_, err = runNexctl(t, "y\n", "setup", "restore", "x.tar.gz", "--file", "/tmp/x.tar.gz", "--url", "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("err = %v; want exactly-one error (both)", err)
	}
}

func TestSetupRestoreFromDiskConfirmed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/restore/disk", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "setup", "restore", "b.tar.gz", "--url", srv.URL, "-y")
	if err != nil {
		t.Fatalf("setup restore: %v\n%s", err, out)
	}
	if !called {
		t.Fatal("restore/disk endpoint was not called")
	}
	if !strings.Contains(out, "Backup restored") {
		t.Fatalf("out = %q", out)
	}
}

func TestSetupRestoreAborted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/restore/disk", func(w http.ResponseWriter, _ *http.Request) {
		called = true
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// "n\n" declines the confirm prompt.
	out, err := runNexctl(t, "n\n", "setup", "restore", "b.tar.gz", "--url", srv.URL)
	if err != nil {
		t.Fatalf("setup restore (declined): %v\n%s", err, out)
	}
	if called {
		t.Fatal("restore must not call the server after the user declines")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Fatalf("out = %q; want Aborted.", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/nexctl/ -run TestSetupBackups -v` and `go test ./cmd/nexctl/ -run TestSetupRestore -v`
Expected: FAIL — `unknown command "backups"` / `"restore"` for `setup` (commands not registered yet).

- [ ] **Step 3: Add the two subcommands and register them**

In `cmd/nexctl/setup.go`, add `"text/tabwriter"` to the imports, register the subcommands in `newSetupCmd()`:

```go
	cmd.AddCommand(newSetupAdminCmd(), newSetupBackupsCmd(), newSetupRestoreCmd())
```

Then append:

```go
func newSetupBackupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backups",
		Short: "List on-disk backups available for restore on a fresh instance",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			entries, err := cliclient.New(resolveServerURL(cmd)).SetupListBackups()
			if err != nil {
				return fmt.Errorf("list setup backups: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, entries)
			}
			if flagBool(cmd, "quiet") {
				for i := range entries {
					fmt.Fprintln(out, entries[i].Filename)
				}
				return nil
			}
			if len(entries) == 0 {
				fmt.Fprintln(out, "No backups found.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "FILENAME\tSIZE\tMODIFIED\tRESTORABLE\tREASON")
			for i := range entries {
				e := &entries[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n",
					e.Filename, humanBackupBytes(e.SizeBytes), e.ModTime, e.Restorable, e.Reason)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	return cmd
}

func newSetupRestoreCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "restore [<name>]",
		Short: "Restore a fresh instance from a backup (on-disk name or uploaded --file)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			hasName := len(args) == 1
			hasFile := cmd.Flags().Changed("file")
			if hasName == hasFile {
				return fmt.Errorf("specify exactly one of <name> or --file")
			}

			confirmMsg := "WARNING: Restoring will overwrite the database on this instance. Proceed?"
			ok, err := cliui.Confirm(in, out, confirmMsg, flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}

			c := cliclient.New(resolveServerURL(cmd))
			if hasFile {
				f, err := os.Open(filePath) //nolint:gosec // operator-supplied restore archive path
				if err != nil {
					return fmt.Errorf("open file: %w", err)
				}
				defer func() { _ = f.Close() }()
				filename := filePath
				if idx := strings.LastIndexByte(filePath, '/'); idx >= 0 {
					filename = filePath[idx+1:]
				}
				if err := c.SetupRestoreUpload(filename, f); err != nil {
					return fmt.Errorf("setup restore upload: %w", err)
				}
				fmt.Fprintln(out, "Backup restored. Log in with your restored credentials.")
				return nil
			}
			if err := c.SetupRestoreFromDisk(args[0]); err != nil {
				return fmt.Errorf("setup restore: %w", err)
			}
			fmt.Fprintln(out, "Backup restored. Log in with your restored credentials.")
			return nil
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "Path to a backup archive to upload and restore from")
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	return cmd
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/nexctl/ -run 'TestSetupBackups|TestSetupRestore' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/setup.go cmd/nexctl/setup_test.go
git commit -m "feat(nexctl): add 'setup backups' and 'setup restore' for fresh-instance recovery"
```

---

## Task 5: `nexctl migrate` + `nexctl migrate status`

**Files:**
- Create: `cmd/nexctl/migrate.go`
- Test: `cmd/nexctl/migrate_test.go`

**Interfaces:**
- Consumes: `(*cliclient.Client).MigrationStatus`, `cliauth.RunMigrateAndWait`, `cliauth.DefaultServerURL`; `resolveServerURL`, `flagBool`, `cliui.EncodeJSON`.
- Produces: `func newMigrateCmd() *cobra.Command` (registered in `main.go` in Task 3 Step 1).

- [ ] **Step 1: Write the failing tests**

Create `cmd/nexctl/migrate_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMigrateAlreadyReady(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ran := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) { ran = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "migrate", "--url", srv.URL)
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if ran {
		t.Fatal("migrate must not POST run when already ready")
	}
	if !strings.Contains(out, "No pending migrations.") {
		t.Fatalf("out = %q", out)
	}
}

func TestMigrateRunsAndWaits(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ran := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		state := "needs_migration"
		if ran {
			state = "ready"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"state": state})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "migrate", "--url", srv.URL)
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if !ran || !strings.Contains(out, "Migrations complete.") {
		t.Fatalf("ran=%v out=%q", ran, out)
	}
}

func TestMigrateStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "needs_migration", "pending_count": 2})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "migrate", "status", "--url", srv.URL)
	if err != nil {
		t.Fatalf("migrate status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "needs_migration") {
		t.Fatalf("out = %q", out)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/nexctl/ -run TestMigrate -v`
Expected: FAIL to compile — `newMigrateCmd` undefined (referenced from `main.go`, but its definition is missing).

- [ ] **Step 3: Write `cmd/nexctl/migrate.go`**

Create `cmd/nexctl/migrate.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run pending database migrations on a running server",
		Long: "Apply pending database migrations by driving POST /api/migrate/run and\n" +
			"polling until the server reports ready — the CLI equivalent of the web\n" +
			"migration UI's \"Run migrations\" button. Prints \"No pending migrations.\"\n" +
			"when the server is already up to date.",
		RunE: runNexctlMigrate,
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	cmd.AddCommand(newMigrateStatusCmd())
	return cmd
}

func runNexctlMigrate(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	url := resolveServerURL(cmd)
	client := cliclient.New(url)

	state, _, err := client.MigrationStatus()
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	switch state {
	case "ready":
		fmt.Fprintln(out, "No pending migrations.")
		return nil
	case "db_unavailable":
		return fmt.Errorf("database is unavailable")
	}
	return cliauth.RunMigrateAndWait(out, client)
}

func newMigrateStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the server's migration state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			state, detail, err := cliclient.New(resolveServerURL(cmd)).MigrationStatus()
			if err != nil {
				return fmt.Errorf("migration status: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, map[string]any{"state": state, "detail": detail})
			}
			fmt.Fprintf(out, "state: %s\n", state)
			if detail != "" {
				fmt.Fprintf(out, "detail: %s\n", detail)
			}
			return nil
		},
	}
	cmd.Flags().String("url", "", "Server URL (default "+cliauth.DefaultServerURL+")")
	return cmd
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/nexctl/ -run TestMigrate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/migrate.go cmd/nexctl/migrate_test.go
git commit -m "feat(nexctl): add 'migrate' and 'migrate status' commands"
```

---

## Task 6: Remove `nexorious setup` from the server binary

**Files:**
- Delete: `cmd/nexorious/setup.go`, `cmd/nexorious/setup_cmd_test.go`
- Modify: `cmd/nexorious/main.go` (unregister `newSetupCmd`)

**Interfaces:** none produced; this removes the duplicate orchestration now owned by `cliauth`.

- [ ] **Step 1: Confirm no other code in `cmd/nexorious` depends on the helpers being deleted**

Run: `grep -rn "confirmInteractivePassword\|resolveSetupPassword\|reportSetupResult\|runMigrateAndWait\|migrationFailedErr\|\bpreflight\b" cmd/nexorious/ --include='*.go' | grep -v setup`
Expected: no output. (If `promptSecret` appears anywhere, it lives in `reset_password.go` and stays — it is NOT being deleted.)

- [ ] **Step 2: Delete the two files**

```bash
git rm cmd/nexorious/setup.go cmd/nexorious/setup_cmd_test.go
```

- [ ] **Step 3: Unregister the command in `cmd/nexorious/main.go`**

In `cmd/nexorious/main.go`, find:

```go
	root.AddCommand(newSetupCmd())
```

Delete that line.

- [ ] **Step 4: Verify the server binary builds**

Run: `go build ./cmd/nexorious/`
Expected: builds cleanly (no `newSetupCmd` / undefined-symbol errors).

- [ ] **Step 5: Run deadcode to catch newly-orphaned exported symbols**

Run: `go run golang.org/x/tools/cmd/deadcode@latest -test ./... 2>&1 | grep -iE 'nexorious/cmd/nexorious|cliauth|cliclient' || echo "no new dead code in touched packages"`
Expected: no entries pointing at live code you just wrote. (`cliauth` exports `Preflight`/`RunMigrateAndWait`/`ReportSetupResult` — all reached from `cmd/nexctl`. `migrationFailedErr` is reached via `Preflight`.) Reconcile any genuine orphan against the diff before continuing.

- [ ] **Step 6: Run the full Go suite for the touched packages**

Run: `go test ./cmd/nexorious/ ./cmd/nexctl/ ./internal/cliauth/ ./internal/cliclient/`
Expected: ok for all four.

- [ ] **Step 7: Commit**

```bash
git add -A cmd/nexorious/
git commit -m "feat(nexorious): remove the REST-client 'setup' command (moved to nexctl)"
```

---

## Task 7: Ship `nexctl` in the container image

**Files:**
- Modify: `Dockerfile`

**Interfaces:** none.

- [ ] **Step 1: Build `nexctl` in the `go-build` stage**

In `Dockerfile`, find the `go build … -o /out/nexorious ./cmd/nexorious` step. Immediately after it (still in the `go-build` stage), add a second build:

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/nexctl \
      ./cmd/nexctl
```

- [ ] **Step 2: Ship `nexctl` in the `runtime-ci` (release) target**

In `Dockerfile`, find in the `runtime-ci` target:

```dockerfile
COPY --from=binaries --chown=nexorious:nexorious --chmod=0755 nexorious-linux-${TARGETARCH} /app/nexorious
```

Add immediately after it:

```dockerfile
COPY --from=binaries --chown=nexorious:nexorious --chmod=0755 nexctl-linux-${TARGETARCH} /usr/local/bin/nexctl
```

- [ ] **Step 3: Ship `nexctl` in the `runtime` (source-build) target**

In `Dockerfile`, find in the `runtime` target:

```dockerfile
COPY --from=go-build --chown=nexorious:nexorious --chmod=0755 /out/nexorious /app/nexorious
```

Add immediately after it:

```dockerfile
COPY --from=go-build --chown=nexorious:nexorious --chmod=0755 /out/nexctl /usr/local/bin/nexctl
```

- [ ] **Step 4: Build the image and verify `nexctl` is present and runnable**

Run:
```bash
docker build -t nexorious:nexctl-test . && \
docker run --rm --entrypoint /usr/local/bin/nexctl nexorious:nexctl-test version
```
Expected: the image builds and `nexctl version` prints a version line (confirms the binary is on the image and executable). If Docker is unavailable in the environment, skip the run and instead confirm the two `COPY` lines and the build step are present with `grep -n nexctl Dockerfile` (expect 3 matches: the build `-o /out/nexctl`, and the two `COPY … /usr/local/bin/nexctl`).

- [ ] **Step 5: Commit**

```bash
git add Dockerfile
git commit -m "feat(docker): ship nexctl in the container image for headless bootstrap"
```

---

## Task 8: Docs — repoint bootstrap instructions and document the new commands

**Files:**
- Modify: `docs/admin-guide.md`
- Modify: `docs/user-guide.md`
- Modify: `CLAUDE.md`

**Interfaces:** none.

- [ ] **Step 1: Repoint `docs/admin-guide.md`**

Open `docs/admin-guide.md`. Around line 252, replace the sentence pointing at `nexorious setup`:

> You can also create it from the host with `nexorious setup` — handy for scripted or headless installs.

with:

> You can also create it with `nexctl setup admin` — handy for scripted or headless installs. `nexctl` ships inside the container image, so you can run it via `kubectl exec`/`docker exec` on a fresh instance.

Around line 358, replace the `| `nexorious setup` | … |` command-table row with:

> `| nexctl setup admin | Create the first admin user against a running server — good for headless installs. Supports --username and reading the password from stdin (--password-stdin), runs pending migrations first, and can store an API key for you with --login. nexctl is available in the container image. |`

(Match the existing table's exact column formatting when editing.)

- [ ] **Step 2: Document the new commands in `docs/user-guide.md`**

Find the `nexctl` section in `docs/user-guide.md` (added in #1127). Add a short subsection covering the new pre-auth commands. Insert this Markdown:

```markdown
### Bootstrapping a fresh instance

These commands target the pre-authentication setup and migration zones, so they
do not need an API key. They resolve the server URL from `--url`, then the
current profile's stored URL, then the default (`http://localhost:8000`).

- `nexctl setup admin [--username U] [--password-stdin] [--login]` — create the
  first admin user on a fresh instance. Pending migrations are applied first.
  `--login` also logs in and stores an API key.
- `nexctl setup backups` — list on-disk backups available for restore.
- `nexctl setup restore --file PATH` — upload a backup archive and restore from it.
- `nexctl setup restore <name>` — restore from a named on-disk backup.
- `nexctl migrate` — apply pending migrations on a running server (the web
  migration UI's "Run migrations" button); `nexctl migrate status` shows state.

`nexctl` ships inside the container image, so on a containerized deployment you
can run these via `kubectl exec`/`docker exec` into a fresh instance.
```

- [ ] **Step 3: Update `CLAUDE.md` references**

In `CLAUDE.md`:

1. In the `cmd/nexorious/` bullet, the parenthetical currently reads `(auth/account commands have moved to nexctl)`. Extend it to: `(auth/account commands and the REST-client setup command have moved to nexctl; the server binary keeps the DB-direct migrate command)`.

2. In the `cmd/nexctl/` bullet, add `setup` and `migrate` to the command inventory near the start (after `mcp` in the list of groups): change `and \`mcp\` (config/serve) commands` to `\`mcp\` (config/serve), \`setup\` (admin/backups/restore — unauthenticated pre-bootstrap), and \`migrate\` (run/status — HTTP-driven against a running server) commands`. Then change the backup note `**setup-zone restore (\`/api/auth/setup/*\`, pre-bootstrap only) is intentionally excluded** — that belongs to \`nexorious setup\`.` to `the **authenticated** \`/api/admin/backups/*\` surface; the **unauthenticated** setup-zone restore (\`/api/auth/setup/*\`, pre-bootstrap) is the separate \`setup\` group.`

3. In the `internal/cliauth/` bullet, change `login-bootstrap shared by \`nexctl account login\` and \`nexorious setup --login\`` to `login-bootstrap plus the setup/migrate orchestration (\`Preflight\`/\`RunMigrateAndWait\`/\`ReportSetupResult\`) shared by \`nexctl account login\`, \`nexctl setup\`, and \`nexctl migrate\``.

- [ ] **Step 4: Verify no stale `nexorious setup` references remain in the embedded docs**

Run: `grep -rn "nexorious setup" docs/admin-guide.md docs/user-guide.md CLAUDE.md`
Expected: no output (the historical files under `docs/superpowers/` are allowed to keep their references and are NOT edited).

- [ ] **Step 5: Commit**

```bash
git add docs/admin-guide.md docs/user-guide.md CLAUDE.md
git commit -m "docs: point bootstrap at nexctl setup; document nexctl setup/migrate"
```

---

## Final verification

- [ ] **Step 1: Full Go suite**

Run: `go test ./...`
Expected: ok across all packages.

- [ ] **Step 2: Lint**

Run: `golangci-lint run ./internal/cliclient/ ./internal/cliauth/ ./cmd/nexctl/ ./cmd/nexorious/`
Expected: no findings.

- [ ] **Step 3: Deadcode sweep**

Run: `go run golang.org/x/tools/cmd/deadcode@latest -test ./... 2>&1 | grep -iE 'cliauth|cliclient|cmd/nexctl|cmd/nexorious' || echo clean`
Expected: `clean` (or only pre-existing, unrelated entries).

- [ ] **Step 4: Push and open the PR**

```bash
git push -u origin feat/issue-1123-nexctl-setup-migrate
```
PR body must include `Closes #1123` and a note that `nexorious setup` is removed and replaced by `nexctl setup admin` (migration note for any scripts). Commit/PR title prefix: `feat(nexctl): …`.

---

## Self-Review

**Spec coverage:**
- setup admin (create-admin + auto-migrate + --login) → Task 3. ✓
- setup backups (list on-disk) → Task 4. ✓
- setup restore --file (upload) + restore <name> (on-disk) → Task 4. ✓
- migrate / migrate status → Task 5. ✓
- URL/profile resolution (no key) → Task 3 (`resolveServerURL`). ✓
- Shared orchestration extraction into cliauth → Task 2. ✓
- cliclient methods + no-auth-header guard → Task 1. ✓
- Remove `nexorious setup` → Task 6. ✓
- Ship nexctl in container (both Dockerfile targets) → Task 7. ✓
- Docs (admin-guide, user-guide, CLAUDE.md) → Task 8. ✓
- Tests: cliclient httptest (Task 1), cliauth unit (Task 2), nexctl command tests (Tasks 3–5). ✓
- Loud destructive confirm on restore, `-y` bypass → Task 4. ✓

**Deviation from spec (intentional):** `migrate status` shows `state` + `detail` only; `pending_count` is not surfaced because `cliclient.MigrationStatus` returns `(state, detail)` and extending its signature would churn the shared callers for marginal benefit. Noted here; acceptable.

**Placeholder scan:** every code step contains complete code; no TBD/TODO/"handle errors appropriately". ✓

**Type consistency:** `SetupBackupEntry`/`SetupBackupManifest`, `SetupListBackups`/`SetupRestoreFromDisk`/`SetupRestoreUpload`, `Preflight`/`RunMigrateAndWait`/`MigrationFailedErr`/`ReportSetupResult`, `resolveServerURL`, `confirmInteractivePassword`, `newSetupCmd`/`newMigrateCmd` are named identically everywhere they appear across tasks. ✓
