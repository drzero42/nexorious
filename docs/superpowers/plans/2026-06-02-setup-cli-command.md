# `nexorious setup` CLI Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `nexorious setup` cobra subcommand that creates the first admin user by driving the existing `POST /api/auth/setup/admin` HTTP endpoint, with an optional `--migrate` flag that runs pending migrations first via `POST /api/migrate/run`.

**Architecture:** `setup` is a pure HTTP client — it never opens the database. Two thin method pairs are added to `internal/cliclient` (`Health`/`SetupAdmin`, `RunMigrations`/`MigrationStatus`); `cmd/nexorious/setup.go` orchestrates preflight → optional migrate → credential resolution → admin creation. Migrations are run over HTTP (not DB-direct) because the running server caches migration state in memory and only learns it is migrated when its *own* migrator runs them.

**Tech Stack:** Go 1.25, cobra, `golang.org/x/term`, stdlib `net/http` + `net/http/httptest`, Echo v5 server (unchanged).

**Spec:** `docs/superpowers/specs/2026-06-02-setup-cli-command-design.md`

---

## File Structure

- **Modify** `internal/cliclient/client.go` — add `Health`, `SetupAdmin` (+ `SetupResult`), `RunMigrations`, `MigrationStatus`; set `CheckRedirect` in `New` so 3xx are observable.
- **Modify** `internal/cliclient/client_test.go` — unit tests for the four new methods.
- **Create** `cmd/nexorious/setup.go` — the `setup` command, password resolution, preflight + migrate-wait, result mapping.
- **Create** `cmd/nexorious/setup_cmd_test.go` — command-level tests against an `httptest.Server`. (NOT `setup_test.go` — that name is taken by the package's `TestMain`/DB harness.)
- **Modify** `cmd/nexorious/main.go` — register `newSetupCmd()`.
- **Modify** `DEV.md` — add a `setup` row to the CLI Subcommands table.

No new HTTP routes. `slumber.yaml` already contains `create_admin`, `migration_run`, and `migration_status`.

---

## Task 1: cliclient — `Health` and `SetupAdmin`

**Files:**
- Modify: `internal/cliclient/client.go`
- Test: `internal/cliclient/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/cliclient/client_test.go`:

```go
func TestHealthReturnsStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "needs_migration"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, err := New(srv.URL).Health()
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if status != "needs_migration" {
		t.Fatalf("status = %q; want needs_migration", status)
	}
}

func TestSetupAdminCreated(t *testing.T) {
	var gotUser, gotPass string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotUser, gotPass = body["username"], body["password"]
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-1"})
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "admin"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).SetupAdmin("admin", "supersecret")
	if err != nil {
		t.Fatalf("SetupAdmin: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("StatusCode = %d; want 201", res.StatusCode)
	}
	if gotUser != "admin" || gotPass != "supersecret" {
		t.Fatalf("server got user=%q pass=%q", gotUser, gotPass)
	}
}

func TestSetupAdminRedirectIsObservable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/migrate")
		w.WriteHeader(http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).SetupAdmin("admin", "supersecret")
	if err != nil {
		t.Fatalf("SetupAdmin: %v", err)
	}
	if res.StatusCode != http.StatusFound {
		t.Fatalf("StatusCode = %d; want 302 (redirect must not be followed)", res.StatusCode)
	}
	if res.Location != "/migrate" {
		t.Fatalf("Location = %q; want /migrate", res.Location)
	}
}

func TestSetupAdminForbiddenMessage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "setup already complete"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).SetupAdmin("admin", "supersecret")
	if err != nil {
		t.Fatalf("SetupAdmin: %v", err)
	}
	if res.StatusCode != http.StatusForbidden || res.Message != "setup already complete" {
		t.Fatalf("res = %+v; want 403 / setup already complete", res)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cliclient/ -run 'TestHealth|TestSetupAdmin' -v`
Expected: FAIL — `c.Health undefined` / `c.SetupAdmin undefined`.

- [ ] **Step 3: Implement `New` redirect behavior, `Health`, `SetupAdmin`**

In `internal/cliclient/client.go`, replace the `New` constructor:

```go
// New returns a Client for the given base URL (trailing slash trimmed). The
// client does not follow redirects: a gate's 302 is an observable response, so
// callers (e.g. setup) can read its Location instead of silently chasing it.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}
```

Then append the new methods (after `Me`, at end of file):

```go
type healthResp struct {
	Status string `json:"status"`
}

// Health performs the GET /health preflight and returns the reported status
// ("ok" when the server is ready, otherwise the app-state name such as
// "needs_migration" or "db_unavailable").
func (c *Client) Health() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return "", fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("health request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	var out healthResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode health response: %w", err)
	}
	return out.Status, nil
}

// SetupResult is the interpreted outcome of a setup-admin attempt. The caller
// maps StatusCode (and Location for a 3xx redirect) to a message and exit code.
type SetupResult struct {
	StatusCode int
	Location   string // Location header, set when StatusCode is a 3xx redirect
	Message    string // server {"message":...}, set for 4xx when present
}

// SetupAdmin posts the first-admin credentials to POST /api/auth/setup/admin.
// It returns a SetupResult for any HTTP response (including 3xx/4xx) so the
// caller can map the outcome; it returns a non-nil error only for transport
// failures (e.g. the server is unreachable). Redirects are not followed, so a
// gate's 302 is observable via Location.
func (c *Client) SetupAdmin(username, password string) (*SetupResult, error) {
	payload, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return nil, fmt.Errorf("marshal setup: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/setup/admin", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build setup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("setup request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	res := &SetupResult{StatusCode: resp.StatusCode}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		res.Location = resp.Header.Get("Location")
		return res, nil
	}
	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr == nil {
			var eb errorBody
			if json.Unmarshal(body, &eb) == nil {
				res.Message = eb.Message
			}
		}
	}
	return res, nil
}
```

Note: `bytes`, `encoding/json`, `io`, `net/http`, `strings`, `time` are already imported by this file.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cliclient/ -run 'TestHealth|TestSetupAdmin' -v`
Expected: PASS (4 tests). Also run the existing suite to confirm the `New` change didn't regress login/api-key tests:
Run: `go test ./internal/cliclient/`
Expected: ok.

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/client.go internal/cliclient/client_test.go
git commit -m "feat: add Health and SetupAdmin to cliclient"
```

---

## Task 2: cliclient — `RunMigrations` and `MigrationStatus`

**Files:**
- Modify: `internal/cliclient/client.go`
- Test: `internal/cliclient/client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/cliclient/client_test.go`:

```go
func TestRunMigrationsAcceptsStarted(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "migration started"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := New(srv.URL).RunMigrations(); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}

func TestRunMigrationsAcceptsAlreadyUpToDate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "already up to date"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := New(srv.URL).RunMigrations(); err != nil {
		t.Fatalf("RunMigrations(already up to date) should be nil, got: %v", err)
	}
}

func TestMigrationStatusReturnsState(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready", "pending_count": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	state, err := New(srv.URL).MigrationStatus()
	if err != nil {
		t.Fatalf("MigrationStatus: %v", err)
	}
	if state != "ready" {
		t.Fatalf("state = %q; want ready", state)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cliclient/ -run 'TestRunMigrations|TestMigrationStatus' -v`
Expected: FAIL — `c.RunMigrations undefined` / `c.MigrationStatus undefined`.

- [ ] **Step 3: Implement `RunMigrations` and `MigrationStatus`**

Append to `internal/cliclient/client.go`:

```go
// RunMigrations triggers POST /api/migrate/run on the running server, so the
// server's own migrator applies pending migrations and its in-memory state
// transitions to ready. 202 ("migration started"), 400 ("already up to date"),
// and 409 ("in progress") are all treated as success (nil) — the caller then
// polls MigrationStatus to learn the outcome. Other responses return an error.
func (c *Client) RunMigrations() error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/migrate/run", nil)
	if err != nil {
		return fmt.Errorf("build migrate request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusAccepted, http.StatusBadRequest, http.StatusConflict:
		return nil
	default:
		return httpError(resp)
	}
}

type migrationStatusResp struct {
	State        string `json:"state"`
	PendingCount int    `json:"pending_count"`
}

// MigrationStatus returns the server's migration state from
// GET /api/migrate/status ("needs_migration", "migrating", "ready",
// "migration_failed", or "db_unavailable").
func (c *Client) MigrationStatus() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/migrate/status", nil)
	if err != nil {
		return "", fmt.Errorf("build status request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	var out migrationStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode status response: %w", err)
	}
	return out.State, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cliclient/ -run 'TestRunMigrations|TestMigrationStatus' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/client.go internal/cliclient/client_test.go
git commit -m "feat: add RunMigrations and MigrationStatus to cliclient"
```

---

## Task 3: `setup` command (no `--migrate` yet) + registration

This task builds the command end-to-end for an already-migrated server: preflight requires `ok`, then it resolves credentials and creates the admin. The `--migrate` flag exists but its only effect here is wired in Task 4 (in this task, a non-`ok` status is always a fatal error).

**Files:**
- Create: `cmd/nexorious/setup.go`
- Modify: `cmd/nexorious/main.go:53` (the `AddCommand` block)
- Test: `cmd/nexorious/setup_cmd_test.go`

- [ ] **Step 1: Write the failing unit test for password confirmation**

Create `cmd/nexorious/setup_cmd_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfirmInteractivePasswordMismatch(t *testing.T) {
	entries := []string{"first-secret", "second-secret"}
	i := 0
	read := func(string) (string, error) {
		v := entries[i]
		i++
		return v, nil
	}
	_, err := confirmInteractivePassword(read)
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestConfirmInteractivePasswordMatch(t *testing.T) {
	read := func(string) (string, error) { return "supersecret", nil }
	pw, err := confirmInteractivePassword(read)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pw != "supersecret" {
		t.Fatalf("pw = %q; want supersecret", pw)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/nexorious/ -run TestConfirmInteractivePassword -v`
Expected: FAIL — `confirmInteractivePassword undefined`.

- [ ] **Step 3: Create `cmd/nexorious/setup.go`**

```go
package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/drzero42/nexorious/internal/cliclient"
)

var errPasswordMismatch = fmt.Errorf("passwords do not match")

const (
	migratePollInterval = 1 * time.Second
	migrateTimeout      = 5 * time.Minute
)

type setupOpts struct {
	url           string
	username      string
	passwordStdin bool
	migrateFirst  bool
}

// newSetupCmd returns the `setup` subcommand. It drives the server's existing
// POST /api/auth/setup/admin endpoint over HTTP to create the first admin user.
func newSetupCmd() *cobra.Command {
	var opts setupOpts
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create the first admin user on a running server",
		Long: "Create the first admin user by driving the server's setup endpoint over\n" +
			"HTTP. The server must already be running and reachable. Pass --migrate to\n" +
			"run any pending database migrations first, bringing a fresh instance up in\n" +
			"one command. Intended to be run via `docker exec` / `kubectl exec` into the\n" +
			"running container.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.url, "url", "", "Server URL (default "+defaultServerURL+")")
	cmd.Flags().StringVar(&opts.username, "username", "", "Admin username (prompted if omitted; required with --password-stdin)")
	cmd.Flags().BoolVar(&opts.passwordStdin, "password-stdin", false, "Read the password from stdin instead of prompting")
	cmd.Flags().BoolVar(&opts.migrateFirst, "migrate", false, "Run pending migrations before creating the admin")
	return cmd
}

func runSetup(cmd *cobra.Command, opts setupOpts) error {
	out := cmd.OutOrStdout()
	in := bufio.NewReader(cmd.InOrStdin())
	stdinIsTTY := term.IsTerminal(int(os.Stdin.Fd()))

	// Validate the input mode before touching the network.
	if !opts.passwordStdin && !stdinIsTTY {
		return fmt.Errorf("no password: pass --password-stdin to read it from stdin, or run interactively")
	}
	if opts.passwordStdin && opts.username == "" {
		return fmt.Errorf("--username is required with --password-stdin")
	}

	url := firstNonEmpty(opts.url, defaultServerURL)
	client := cliclient.New(url)

	if err := preflight(out, client, url, opts.migrateFirst); err != nil {
		return err
	}

	username := opts.username
	if username == "" { // interactive (TTY) path
		var err error
		username, err = prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
		if username == "" {
			return fmt.Errorf("username is required")
		}
	}

	password, err := resolveSetupPassword(in, out, opts.passwordStdin)
	if err != nil {
		return err
	}

	res, err := client.SetupAdmin(username, password)
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	return reportSetupResult(out, username, res)
}

// preflight checks server health before credentials are read. With
// migrateFirst it runs pending migrations and waits for the server to become
// ready; without it, a non-ready state is a fatal error.
func preflight(out io.Writer, client *cliclient.Client, url string, migrateFirst bool) error {
	status, err := client.Health()
	if err != nil {
		return fmt.Errorf("could not reach server at %s — is it running? (%w)", url, err)
	}
	switch status {
	case "ok":
		return nil
	case "db_unavailable":
		return fmt.Errorf("database is unavailable")
	default:
		// needs_migration, migration_failed, migrating, ...
		if !migrateFirst {
			return fmt.Errorf("migrations are pending — pass --migrate or run \"nexorious migrate\" first")
		}
		return runMigrateAndWait(out, client)
	}
}

// runMigrateAndWait triggers migrations on the server and polls until the
// server reports ready, or fails / times out. Implemented in Task 4.
func runMigrateAndWait(out io.Writer, client *cliclient.Client) error {
	return fmt.Errorf("--migrate not yet implemented")
}

// resolveSetupPassword returns the admin password. With passwordStdin it reads
// a single line from in (no confirmation, docker-login style). Otherwise it
// prompts twice with no echo on the TTY and requires the entries to match.
func resolveSetupPassword(in *bufio.Reader, out io.Writer, passwordStdin bool) (string, error) {
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
		fmt.Fprint(out, label)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return confirmInteractivePassword(read)
}

// confirmInteractivePassword prompts for the password twice via read and
// returns it only if both entries match. Factored out so the match/mismatch
// logic is unit-testable without a TTY.
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
		return "", errPasswordMismatch
	}
	if first == "" {
		return "", fmt.Errorf("password is required")
	}
	return first, nil
}

// reportSetupResult maps a SetupResult to user-facing output and an error
// (non-nil => non-zero exit). A nil error means success.
func reportSetupResult(out io.Writer, username string, res *cliclient.SetupResult) error {
	switch res.StatusCode {
	case http.StatusCreated:
		fmt.Fprintf(out, "Admin user %q created.\n", username)
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("setup already complete; an admin user already exists")
	case http.StatusBadRequest:
		if res.Message != "" {
			return fmt.Errorf("%s", res.Message)
		}
		return fmt.Errorf("invalid request")
	case http.StatusFound, http.StatusMovedPermanently, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		switch {
		case strings.HasPrefix(res.Location, "/migrate"):
			return fmt.Errorf("migrations are pending — run \"nexorious migrate\" first (or pass --migrate)")
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

Note: `firstNonEmpty`, `prompt`, and the `defaultServerURL` constant already exist in `cmd/nexorious/login.go` (same package) — reuse them, do not redeclare.

- [ ] **Step 4: Register the command in `main.go`**

In `cmd/nexorious/main.go`, add the line after `newResetPasswordCmd()`:

```go
	root.AddCommand(newResetPasswordCmd())
	root.AddCommand(newSetupCmd())
```

- [ ] **Step 5: Run the password unit tests to verify they pass**

Run: `go test ./cmd/nexorious/ -run TestConfirmInteractivePassword -v`
Expected: PASS (2 tests).

- [ ] **Step 6: Write the command-level tests**

Append to `cmd/nexorious/setup_cmd_test.go`. This helper builds a stub server with overridable handlers:

```go
// setupStub is a configurable httptest server for the setup command tests.
type setupStub struct {
	healthStatus string // value for /health "status" (default "ok")
	adminStatus  int    // status code for /api/auth/setup/admin (default 201)
	adminMessage string // {"message":...} body for 4xx admin responses
	adminLocation string // Location header for a 3xx admin response
}

func (s setupStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		status := s.healthStatus
		if status == "" {
			status = "ok"
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status})
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

// runSetupCmd executes `setup` with the given args and piped stdin.
func runSetupCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"setup"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestSetupCreatesAdmin(t *testing.T) {
	srv := setupStub{}.server(t)
	out, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err != nil {
		t.Fatalf("setup: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, `Admin user "admin" created.`) {
		t.Fatalf("output = %q; want success message", out)
	}
}

func TestSetupAlreadyComplete(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusForbidden, adminMessage: "setup already complete"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("err = %v; want already-complete error", err)
	}
}

func TestSetupValidationMessageSurfaced(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusBadRequest, adminMessage: "password must be at least 8 characters"}.server(t)
	_, err := runSetupCmd(t, "short\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "at least 8") {
		t.Fatalf("err = %v; want server validation message", err)
	}
}

func TestSetupRedirectToMigrate(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusFound, adminLocation: "/migrate"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "migrations are pending") {
		t.Fatalf("err = %v; want migrations-pending error", err)
	}
}

func TestSetupRedirectToDBError(t *testing.T) {
	srv := setupStub{adminStatus: http.StatusFound, adminLocation: "/db-error"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatalf("err = %v; want db-unavailable error", err)
	}
}

func TestSetupUnhealthyPreflightAborts(t *testing.T) {
	srv := setupStub{healthStatus: "needs_migration"}.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "--migrate") {
		t.Fatalf("err = %v; want pending-migrations preflight error", err)
	}
}

func TestSetupConnectionRefused(t *testing.T) {
	// A server that is created then immediately closed yields a refused connection.
	srv := setupStub{}.server(t)
	url := srv.URL
	srv.Close()
	_, err := runSetupCmd(t, "supersecret\n", "--url", url, "--username", "admin", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "could not reach server") {
		t.Fatalf("err = %v; want reach-server error", err)
	}
}

func TestSetupMissingUsernameWithPasswordStdin(t *testing.T) {
	// No server needed: flag validation happens before any network call.
	_, err := runSetupCmd(t, "supersecret\n", "--url", "http://127.0.0.1:1", "--password-stdin")
	if err == nil || !strings.Contains(err.Error(), "--username is required") {
		t.Fatalf("err = %v; want username-required error", err)
	}
}

func TestSetupNoPasswordSourceNonTTY(t *testing.T) {
	// Non-TTY stdin without --password-stdin: must error before any network call.
	_, err := runSetupCmd(t, "", "--url", "http://127.0.0.1:1", "--username", "admin")
	if err == nil || !strings.Contains(err.Error(), "no password") {
		t.Fatalf("err = %v; want no-password error", err)
	}
}
```

- [ ] **Step 7: Run the command tests to verify they pass**

Run: `go test ./cmd/nexorious/ -run TestSetup -v`
Expected: PASS (all `TestSetup*` and `TestConfirmInteractivePassword*`). `go test` stdin is not a TTY, so the `--password-stdin` paths and the non-TTY validation paths exercise as designed.

- [ ] **Step 8: Commit**

```bash
git add cmd/nexorious/setup.go cmd/nexorious/setup_cmd_test.go cmd/nexorious/main.go
git commit -m "feat: add setup CLI command driving setup-admin endpoint (#728)"
```

---

## Task 4: `--migrate` flow

Replace the stub `runMigrateAndWait` with the real HTTP-driven migrate-and-poll.

**Files:**
- Modify: `cmd/nexorious/setup.go` (`runMigrateAndWait`)
- Test: `cmd/nexorious/setup_cmd_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/nexorious/setup_cmd_test.go`. This stub serves the migrate endpoints; `/api/migrate/status` returns the configured terminal state once `run` has been called:

```go
// migrateStub serves /health (needs_migration), /api/migrate/run, and
// /api/migrate/status (which returns finalState after run is called), plus a
// 201 /api/auth/setup/admin.
type migrateStub struct {
	finalState string // "ready" or "migration_failed"
	ranMigrate bool
}

func (s *migrateStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "needs_migration"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		s.ranMigrate = true
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "migration started"})
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		state := "needs_migration"
		if s.ranMigrate {
			state = s.finalState
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"state": state, "pending_count": 0})
	})
	mux.HandleFunc("/api/auth/setup/admin", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSetupMigrateHappyPath(t *testing.T) {
	stub := &migrateStub{finalState: "ready"}
	srv := stub.server(t)
	out, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin", "--migrate")
	if err != nil {
		t.Fatalf("setup --migrate: %v\noutput: %s", err, out)
	}
	if !stub.ranMigrate {
		t.Fatal("migrations were not triggered")
	}
	if !strings.Contains(out, "Migrations complete.") || !strings.Contains(out, `Admin user "admin" created.`) {
		t.Fatalf("output = %q; want migrate + create messages", out)
	}
}

func TestSetupMigrateFailed(t *testing.T) {
	stub := &migrateStub{finalState: "migration_failed"}
	srv := stub.server(t)
	_, err := runSetupCmd(t, "supersecret\n", "--url", srv.URL, "--username", "admin", "--password-stdin", "--migrate")
	if err == nil || !strings.Contains(err.Error(), "migrations failed") {
		t.Fatalf("err = %v; want migrations-failed error", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/nexorious/ -run 'TestSetupMigrate' -v`
Expected: FAIL — `TestSetupMigrateHappyPath` fails with "--migrate not yet implemented".

- [ ] **Step 3: Implement `runMigrateAndWait`**

In `cmd/nexorious/setup.go`, replace the stub:

```go
// runMigrateAndWait triggers migrations on the server and polls until the
// server reports ready, or fails / times out. Driven over HTTP so the running
// server's own migrator applies them (a DB-direct migration would leave the
// running server's cached state stale; see the design doc).
func runMigrateAndWait(out io.Writer, client *cliclient.Client) error {
	fmt.Fprintln(out, "Running pending migrations...")
	if err := client.RunMigrations(); err != nil {
		return fmt.Errorf("start migrations: %w", err)
	}
	deadline := time.Now().Add(migrateTimeout)
	for {
		state, err := client.MigrationStatus()
		if err != nil {
			return fmt.Errorf("poll migration status: %w", err)
		}
		switch state {
		case "ready":
			fmt.Fprintln(out, "Migrations complete.")
			return nil
		case "migration_failed":
			return fmt.Errorf("migrations failed — check the server logs")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for migrations (last state: %s)", migrateTimeout, state)
		}
		time.Sleep(migratePollInterval)
	}
}
```

The loop checks status before sleeping, so the tests (whose stub returns the terminal state on the first poll) complete without waiting.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/nexorious/ -run 'TestSetupMigrate' -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Run the whole command suite**

Run: `go test ./cmd/nexorious/ -run 'TestSetup|TestConfirmInteractivePassword'`
Expected: ok.

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/setup.go cmd/nexorious/setup_cmd_test.go
git commit -m "feat: add --migrate flag to setup command (#728)"
```

---

## Task 5: Docs + final verification

**Files:**
- Modify: `DEV.md` (CLI Subcommands table)

- [ ] **Step 1: Add the `setup` row to the CLI table**

In `DEV.md`, in the CLI Subcommands table, add this row immediately after the `reset-password` row:

```markdown
| `setup`          | Create the first admin user on a running server (via HTTP)     |
```

- [ ] **Step 2: Verify the full Go suite and lint**

Run: `go build ./... && go test ./internal/cliclient/ ./cmd/nexorious/`
Expected: builds; both packages `ok`.

Run: `golangci-lint run ./internal/cliclient/... ./cmd/nexorious/...`
Expected: `0 issues`. (Pay attention to `errcheck`/`check-blank`: there are no bare `_ =` error discards in the new code; the `defer func() { _ = resp.Body.Close() }()` pattern is covered by the `std-error-handling` preset, matching the existing file.)

- [ ] **Step 3: Manual smoke test (optional, against a real server)**

```bash
make && ./nexorious serve   # in one shell, against a fresh empty DB
# in another shell:
echo "supersecret" | ./nexorious setup --username admin --password-stdin --migrate
# expect: "Running pending migrations..." / "Migrations complete." / 'Admin user "admin" created.'
```

- [ ] **Step 4: Commit**

```bash
git add DEV.md
git commit -m "docs: document setup CLI subcommand"
```

---

## Self-Review Notes (for the implementer)

- **Spec coverage:** command + flags (Task 3), `--password-stdin` & confirmation rules (Task 3), preflight branching (Task 3 + 4), outcome mapping table (Task 3 `reportSetupResult`), `--migrate` HTTP flow (Task 4), four `cliclient` methods + redirect observability (Tasks 1–2), DEV.md (Task 5). slumber/login unchanged per spec.
- **Type consistency:** `SetupResult{StatusCode, Location, Message}` defined in Task 1 is consumed unchanged by `reportSetupResult` in Task 3. `Health`/`MigrationStatus` return `(string, error)`; `RunMigrations`/`SetupAdmin` signatures match their call sites. `confirmInteractivePassword(read func(string)(string,error))` is defined and tested in Task 3.
- **Known gotcha:** tests live in `setup_cmd_test.go`, NOT `setup_test.go` (the latter owns the package `TestMain`/DB harness). The setup command never opens the DB, so it does not use that harness.
