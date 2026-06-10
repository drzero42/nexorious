# `nexorious version` Update Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `nexorious version` report whether a newer release is available, reusing `internal/services/updatecheck` from PR #914.

**Architecture:** The cobra `version` command prints the build version first, then (unless skipped) calls `updatecheck.Client.FetchLatest` against the GitHub API with a 3-second context deadline and renders one result line. A package-level client factory in `cmd/nexorious` lets tests point the command at an `httptest` server. Skip conditions: `--no-check` flag, `UPDATE_CHECK_ENABLED=false` env var, or a non-semver running version (`dev` builds).

**Tech Stack:** Go, cobra, `internal/services/updatecheck` (GitHub client + `golang.org/x/mod/semver` compare), `net/http/httptest` for tests.

**Spec:** `docs/superpowers/specs/2026-06-10-version-update-check-design.md`

## File Structure

- Modify: `internal/services/updatecheck/compare.go` — add exported `IsValidVersion`
- Modify: `internal/services/updatecheck/compare_test.go` — test for `IsValidVersion`
- Modify: `cmd/nexorious/version.go` — flag, skip logic, `checkForUpdate` helper
- Create: `cmd/nexorious/version_test.go` — command-level and helper tests
- Modify: `cmd/nexorious/main_test.go:125` — existing version test must pass `--no-check`
- Modify: `docs/admin-guide.md:182` — extend the `UPDATE_CHECK_ENABLED` table row

---

### Task 1: `updatecheck.IsValidVersion`

The CLI must skip the network call entirely for non-semver builds (`dev`). The normalize logic already lives unexported in `compare.go`; expose a tiny validity helper instead of duplicating it.

**Files:**
- Modify: `internal/services/updatecheck/compare.go`
- Test: `internal/services/updatecheck/compare_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/updatecheck/compare_test.go`:

```go
func TestIsValidVersion(t *testing.T) {
	cases := []struct {
		name string
		v    string
		want bool
	}{
		{"plain semver", "0.9.0", true},
		{"v-prefixed", "v0.9.0", true},
		{"prerelease", "1.2.3-rc1", true},
		{"dev build", "dev", false},
		{"empty", "", false},
		{"garbage", "not-a-version", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidVersion(tc.v); got != tc.want {
				t.Errorf("IsValidVersion(%q) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/updatecheck/... -run TestIsValidVersion -v`
Expected: FAIL — `undefined: IsValidVersion` (compile error).

- [ ] **Step 3: Implement `IsValidVersion`**

Append to `internal/services/updatecheck/compare.go`:

```go
// IsValidVersion reports whether v is valid semver (with or without the
// leading "v"). Callers use it to skip update checks for non-release builds
// such as "dev".
func IsValidVersion(v string) bool {
	return semver.IsValid(normalize(v))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/services/updatecheck/... -run TestIsValidVersion -v`
Expected: PASS (all 6 subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/services/updatecheck/compare.go internal/services/updatecheck/compare_test.go
git commit -m "feat: add updatecheck.IsValidVersion helper"
```

---

### Task 2: `checkForUpdate` helper in `cmd/nexorious`

Pure fetch-and-render function, fully testable with `httptest`. Tag normalization (`strings.TrimPrefix(rel.TagName, "v")`) matches `internal/scheduler/update_check.go:45`.

**Files:**
- Modify: `cmd/nexorious/version.go`
- Test: `cmd/nexorious/version_test.go` (create)

- [ ] **Step 1: Write the failing tests**

Create `cmd/nexorious/version_test.go`:

```go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// releaseServer returns an httptest server mimicking the GitHub
// "latest release" endpoint, and a counter of requests received.
func releaseServer(t *testing.T, status int, body string) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestCheckForUpdate(t *testing.T) {
	const release = `{"tag_name":"v1.1.0","html_url":"https://github.com/drzero42/nexorious/releases/tag/v1.1.0"}`

	t.Run("newer release available", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusOK, release)
		client := updatecheck.NewClientWithBaseURL(srv.URL)

		line, failed := checkForUpdate(context.Background(), client, "1.0.0")
		if failed {
			t.Fatalf("failed = true, want false; line = %q", line)
		}
		if !strings.Contains(line, "Update available: 1.1.0") ||
			!strings.Contains(line, "releases/tag/v1.1.0") {
			t.Errorf("line = %q, want update notice with version and URL", line)
		}
	})

	t.Run("up to date", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusOK, release)
		client := updatecheck.NewClientWithBaseURL(srv.URL)

		line, failed := checkForUpdate(context.Background(), client, "1.1.0")
		if failed {
			t.Fatalf("failed = true, want false; line = %q", line)
		}
		if line != "You are running the latest version." {
			t.Errorf("line = %q, want latest-version message", line)
		}
	})

	t.Run("API failure", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusInternalServerError, "")
		client := updatecheck.NewClientWithBaseURL(srv.URL)

		line, failed := checkForUpdate(context.Background(), client, "1.0.0")
		if !failed {
			t.Fatalf("failed = false, want true; line = %q", line)
		}
		if !strings.Contains(line, "update check failed") {
			t.Errorf("line = %q, want it to start with 'update check failed'", line)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/nexorious/... -run TestCheckForUpdate -v -short`
Expected: FAIL — `undefined: checkForUpdate` (compile error). (`-short` skips the container-backed tests; `TestMain` may still start the shared container — that is fine.)

- [ ] **Step 3: Implement `checkForUpdate`**

In `cmd/nexorious/version.go`, replace the entire file content with:

```go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// newVersionCmd returns the `version` subcommand. The values printed are
// injected at build time via -ldflags `-X main.version=... -X main.commit=...`.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "nexorious %s (%s)\n", version, commit)
		},
	}
}

// checkForUpdate fetches the latest release and renders the one-line result.
// failed=true means the line is a non-fatal error note destined for stderr.
func checkForUpdate(ctx context.Context, client *updatecheck.Client, running string) (line string, failed bool) {
	rel, err := client.FetchLatest(ctx)
	if err != nil {
		return fmt.Sprintf("update check failed: %v", err), true
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	if updatecheck.UpdateAvailable(running, latest) {
		return fmt.Sprintf("Update available: %s — %s", latest, rel.HTMLURL), false
	}
	return "You are running the latest version.", false
}
```

(The `Run` body is still the old one — wiring happens in Task 3.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/nexorious/... -run TestCheckForUpdate -v -short`
Expected: PASS (3 subtests).

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/version.go cmd/nexorious/version_test.go
git commit -m "feat: add update-check helper for the version subcommand"
```

---

### Task 3: Wire the check into the `version` command

Adds `--no-check`, the env opt-out, the dev-build skip, the 3s deadline, and a test seam (`newUpdateCheckClient` package var). Also fixes the pre-existing `TestVersionCmd_PrintsBuildVersion`, which sets `version = "1.2.3-test"` (valid semver) and would otherwise hit the real GitHub API once the check is default-on.

**Files:**
- Modify: `cmd/nexorious/version.go`
- Modify: `cmd/nexorious/main_test.go:125` (add `--no-check` to existing test args)
- Test: `cmd/nexorious/version_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/nexorious/version_test.go`:

```go
// runVersionCmd executes `nexorious version <args...>` with version/commit
// overridden, returning stdout and stderr separately.
func runVersionCmd(t *testing.T, ver string, args ...string) (stdout, stderr string) {
	t.Helper()
	prevVersion, prevCommit := version, commit
	version, commit = ver, "deadbeef"
	t.Cleanup(func() {
		version = prevVersion
		commit = prevCommit
	})

	root := newRootCmd()
	var out, errOut strings.Builder
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"version"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	return out.String(), errOut.String()
}

// pointClientAt overrides the command's client factory at the test server.
func pointClientAt(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := newUpdateCheckClient
	newUpdateCheckClient = func() *updatecheck.Client {
		return updatecheck.NewClientWithBaseURL(srv.URL)
	}
	t.Cleanup(func() { newUpdateCheckClient = prev })
}

func TestVersionCmd_UpdateCheck(t *testing.T) {
	const release = `{"tag_name":"v1.1.0","html_url":"https://github.com/drzero42/nexorious/releases/tag/v1.1.0"}`

	t.Run("reports available update", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)

		stdout, stderr := runVersionCmd(t, "1.0.0")
		if !strings.Contains(stdout, "nexorious 1.0.0 (deadbeef)") {
			t.Errorf("stdout = %q, want version line first", stdout)
		}
		if !strings.Contains(stdout, "Update available: 1.1.0") {
			t.Errorf("stdout = %q, want update notice", stdout)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("failure goes to stderr, exit 0", func(t *testing.T) {
		srv, _ := releaseServer(t, http.StatusInternalServerError, "")
		pointClientAt(t, srv)

		stdout, stderr := runVersionCmd(t, "1.0.0")
		if !strings.Contains(stdout, "nexorious 1.0.0") {
			t.Errorf("stdout = %q, want version line", stdout)
		}
		if strings.Contains(stdout, "update check failed") {
			t.Errorf("stdout = %q, failure note must not be on stdout", stdout)
		}
		if !strings.Contains(stderr, "update check failed") {
			t.Errorf("stderr = %q, want failure note", stderr)
		}
	})

	t.Run("--no-check skips the request", func(t *testing.T) {
		srv, requests := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)

		stdout, stderr := runVersionCmd(t, "1.0.0", "--no-check")
		if *requests != 0 {
			t.Errorf("requests = %d, want 0", *requests)
		}
		if want := "nexorious 1.0.0 (deadbeef)\n"; stdout != want {
			t.Errorf("stdout = %q, want only the version line %q", stdout, want)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("UPDATE_CHECK_ENABLED=false skips the request", func(t *testing.T) {
		srv, requests := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)
		t.Setenv("UPDATE_CHECK_ENABLED", "false")

		stdout, _ := runVersionCmd(t, "1.0.0")
		if *requests != 0 {
			t.Errorf("requests = %d, want 0", *requests)
		}
		if want := "nexorious 1.0.0 (deadbeef)\n"; stdout != want {
			t.Errorf("stdout = %q, want only the version line %q", stdout, want)
		}
	})

	t.Run("dev build skips the request", func(t *testing.T) {
		srv, requests := releaseServer(t, http.StatusOK, release)
		pointClientAt(t, srv)

		stdout, _ := runVersionCmd(t, "dev")
		if *requests != 0 {
			t.Errorf("requests = %d, want 0", *requests)
		}
		if want := "nexorious dev (deadbeef)\n"; stdout != want {
			t.Errorf("stdout = %q, want only the version line %q", stdout, want)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/nexorious/... -run TestVersionCmd_UpdateCheck -v -short`
Expected: FAIL — `undefined: newUpdateCheckClient` (compile error).

- [ ] **Step 3: Implement the wiring**

In `cmd/nexorious/version.go`, replace the entire file content with:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// updateCheckTimeout bounds the GitHub call so `version` stays snappy even
// when offline. Applied as a context deadline; the client's own 30s HTTP
// timeout stays as is, the shorter deadline wins.
const updateCheckTimeout = 3 * time.Second

// newUpdateCheckClient builds the GitHub release client. Package-level so
// tests can point the command at an httptest server.
var newUpdateCheckClient = updatecheck.NewClient

// newVersionCmd returns the `version` subcommand. The values printed are
// injected at build time via -ldflags `-X main.version=... -X main.commit=...`.
// After printing them it reports whether a newer release is available, unless
// --no-check is set, UPDATE_CHECK_ENABLED=false, or the build is not a
// release (non-semver version such as "dev").
func newVersionCmd() *cobra.Command {
	var noCheck bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "nexorious %s (%s)\n", version, commit)

			if noCheck || !updateCheckEnabled() || !updatecheck.IsValidVersion(version) {
				return
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), updateCheckTimeout)
			defer cancel()

			line, failed := checkForUpdate(ctx, newUpdateCheckClient(), version)
			if failed {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), line)
				return
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
		},
	}
	cmd.Flags().BoolVar(&noCheck, "no-check", false, "skip checking GitHub for a newer release")
	return cmd
}

// updateCheckEnabled mirrors the server's UPDATE_CHECK_ENABLED opt-out. The
// full internal/config struct cannot be parsed here (it requires DATABASE_URL
// etc.), so the variable is read directly. Unset or unparseable means enabled.
func updateCheckEnabled() bool {
	v, ok := os.LookupEnv("UPDATE_CHECK_ENABLED")
	if !ok {
		return true
	}
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return enabled
}

// checkForUpdate fetches the latest release and renders the one-line result.
// failed=true means the line is a non-fatal error note destined for stderr.
func checkForUpdate(ctx context.Context, client *updatecheck.Client, running string) (line string, failed bool) {
	rel, err := client.FetchLatest(ctx)
	if err != nil {
		return fmt.Sprintf("update check failed: %v", err), true
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	if updatecheck.UpdateAvailable(running, latest) {
		return fmt.Sprintf("Update available: %s — %s", latest, rel.HTMLURL), false
	}
	return "You are running the latest version.", false
}
```

- [ ] **Step 4: Fix the pre-existing version test**

In `cmd/nexorious/main_test.go`, `TestVersionCmd_PrintsBuildVersion` sets `version = "1.2.3-test"` — valid semver, so the command would now contact the real GitHub API. Change its args line:

```go
	root.SetArgs([]string{"version"})
```

to:

```go
	root.SetArgs([]string{"version", "--no-check"})
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/nexorious/... -run 'TestVersionCmd' -v -short`
Expected: PASS — `TestVersionCmd_UpdateCheck` (5 subtests) and `TestVersionCmd_PrintsBuildVersion`.

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/version.go cmd/nexorious/version_test.go cmd/nexorious/main_test.go
git commit -m "feat: report available updates in the version subcommand"
```

---

### Task 4: Document the CLI check in the admin guide

**Files:**
- Modify: `docs/admin-guide.md:182`

- [ ] **Step 1: Extend the `UPDATE_CHECK_ENABLED` row**

In `docs/admin-guide.md`, the env-var table row for `UPDATE_CHECK_ENABLED` ends with:

```
set `false` to disable it entirely. |
```

Replace that ending with:

```
set `false` to disable it entirely, which also disables the check performed by the `version` subcommand (one-off skips: `nexorious version --no-check`). |
```

- [ ] **Step 2: Commit**

```bash
git add docs/admin-guide.md
git commit -m "docs: document the version subcommand update check"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run the full affected test packages**

Run: `go test ./cmd/nexorious/... ./internal/services/updatecheck/... -timeout 600s`
Expected: PASS (the `cmd/nexorious` package starts its shared PostgreSQL test container — this is normal).

- [ ] **Step 2: Build and smoke-test by hand**

```bash
make build
./nexorious version
./nexorious version --no-check
UPDATE_CHECK_ENABLED=false ./nexorious version
```

Expected: a dev build prints only `nexorious dev (<commit>)` for all three (the version is non-semver, so even the first invocation makes no network call). To see the live path, override the version at build time:

```bash
go build -ldflags "-X main.version=0.0.1 -X main.commit=smoke" -o /tmp/nexorious-smoke ./cmd/nexorious
/tmp/nexorious-smoke version
```

Expected: version line, then `Update available: <latest> — https://github.com/drzero42/nexorious/releases/tag/v<latest>`.

- [ ] **Step 3: Commit any stragglers and prepare the PR**

The PR title (squash commit) should be:

```
feat: show available updates in the version subcommand
```
