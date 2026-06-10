# Update Check (GitHub Release) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show "A newer version is available" in the sidebar footer and emit a once-per-release `admin.version.available` notification when the running build is behind the latest stable GitHub release (issue #899).

**Architecture:** A new `internal/services/updatecheck` package holds the pure pieces (semver compare via `golang.org/x/mod/semver`, GitHub `releases/latest` client, mutex-guarded `State`). A River periodic worker (`internal/scheduler/update_check.go`, every 6h, `RunOnStart: true`) fetches the latest release, stores it in `State`, and emits the notify event (deduped on version). `/api/version` reads the shared `State` — no network in the request path. The frontend extends `VersionInfo` and renders the link in a new `VersionFooter` component. A backfill migration subscribes existing admins to the new default-on event type (precedent: `20260601000004`).

**Tech Stack:** Go (Echo v5, River, Bun, `golang.org/x/mod/semver` — new dependency), React/TanStack Query/Vitest, SQL migration, Nix `vendorHash` bump.

**Notes / accepted behaviors:**
- All existing periodic jobs use `RunOnStart: false`; this one deliberately uses `true` so the banner appears shortly after boot. The DedupKey keeps the notification once-per-release.
- Events are pruned after `NOTIFY_EVENTS_RETENTION_DAYS` (90d). If an instance stays outdated longer than that, the dedup row disappears and the same release re-notifies. Accepted (acts as a reminder).
- `latest_version`/`release_url` in `/api/version` are populated **only** when `update_available` is true (matches issue spec: empty when disabled or no comparison possible).

---

## Pre-work: branch

- [ ] **Create the feature branch**

```bash
git checkout -b 899-update-check
git add docs/superpowers/plans/2026-06-09-update-check.md
git commit -m "docs: add update-check implementation plan"
```

---

### Task 1: Semver comparison (`updatecheck.UpdateAvailable`)

**Files:**
- Create: `internal/services/updatecheck/compare.go`
- Test: `internal/services/updatecheck/compare_test.go`
- Modify: `go.mod` / `go.sum` (new dep `golang.org/x/mod`)

- [ ] **Step 1: Add the dependency**

```bash
go get golang.org/x/mod
```

- [ ] **Step 2: Write the failing test**

Create `internal/services/updatecheck/compare_test.go`:

```go
package updatecheck

import "testing"

func TestUpdateAvailable(t *testing.T) {
	cases := []struct {
		name             string
		running, latest  string
		want             bool
	}{
		{"newer patch", "0.9.0", "0.9.1", true},
		{"newer minor", "0.9.0", "0.10.0", true},
		{"newer major", "0.9.0", "1.0.0", true},
		{"equal", "0.9.0", "0.9.0", false},
		{"older latest", "0.10.0", "0.9.0", false},
		{"v-prefixed running", "v0.9.0", "0.10.0", true},
		{"v-prefixed latest", "0.9.0", "v0.10.0", true},
		{"both v-prefixed equal", "v0.9.0", "v0.9.0", false},
		{"dev running", "dev", "0.10.0", false},
		{"garbage running", "not-a-version", "0.10.0", false},
		{"empty running", "", "0.10.0", false},
		{"garbage latest", "0.9.0", "next", false},
		{"empty latest", "0.9.0", "", false},
		{"prerelease latest older than stable", "1.0.0", "1.1.0-rc1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UpdateAvailable(tc.running, tc.latest); got != tc.want {
				t.Errorf("UpdateAvailable(%q, %q) = %v, want %v", tc.running, tc.latest, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/services/updatecheck/... -run TestUpdateAvailable -v`
Expected: FAIL (compile error — `UpdateAvailable` undefined)

- [ ] **Step 4: Write the implementation**

Create `internal/services/updatecheck/compare.go`:

```go
// Package updatecheck checks GitHub for a newer Nexorious release. The
// periodic worker (internal/scheduler) fetches and stores the result; the
// /api/version handler reads it — no network call in the request path.
package updatecheck

import (
	"strings"

	"golang.org/x/mod/semver"
)

// UpdateAvailable reports whether latest is a strictly newer semver than
// running. Returns false when either side is not valid semver (e.g. a "dev"
// build), so non-release builds never claim an update.
func UpdateAvailable(running, latest string) bool {
	r, l := normalize(running), normalize(latest)
	if !semver.IsValid(r) || !semver.IsValid(l) {
		return false
	}
	return semver.Compare(l, r) > 0
}

// normalize adds the leading "v" that x/mod/semver requires.
func normalize(v string) string {
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/services/updatecheck/... -run TestUpdateAvailable -v`
Expected: PASS (all subtests)

- [ ] **Step 6: Tidy and commit**

```bash
go mod tidy
git add go.mod go.sum internal/services/updatecheck/
git commit -m "feat: add updatecheck semver comparison"
```

---

### Task 2: GitHub release client + shared State

**Files:**
- Create: `internal/services/updatecheck/client.go`
- Create: `internal/services/updatecheck/state.go`
- Test: `internal/services/updatecheck/client_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/services/updatecheck/client_test.go`:

```go
package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/drzero42/nexorious/releases/latest" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q, want application/vnd.github+json", got)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected a User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.10.0","html_url":"https://github.com/drzero42/nexorious/releases/tag/v0.10.0"}`))
	}))
	defer srv.Close()

	c := NewClientWithBaseURL(srv.URL)
	rel, err := c.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if rel.TagName != "v0.10.0" {
		t.Errorf("TagName = %q, want v0.10.0", rel.TagName)
	}
	if rel.HTMLURL != "https://github.com/drzero42/nexorious/releases/tag/v0.10.0" {
		t.Errorf("HTMLURL = %q", rel.HTMLURL)
	}
}

func TestFetchLatest_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewClientWithBaseURL(srv.URL).FetchLatest(context.Background()); err == nil {
		t.Fatal("expected error on 404, got nil")
	}
}

func TestFetchLatest_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	if _, err := NewClientWithBaseURL(srv.URL).FetchLatest(context.Background()); err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

func TestFetchLatest_EmptyTagName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"","html_url":"x"}`))
	}))
	defer srv.Close()

	if _, err := NewClientWithBaseURL(srv.URL).FetchLatest(context.Background()); err == nil {
		t.Fatal("expected error on empty tag_name, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/updatecheck/... -run TestFetchLatest -v`
Expected: FAIL (compile error — `NewClientWithBaseURL` undefined)

- [ ] **Step 3: Write the client**

Create `internal/services/updatecheck/client.go`:

```go
package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Release is the subset of the GitHub "latest release" API response we need.
// The endpoint returns only the latest stable release (drafts and
// pre-releases excluded).
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Client fetches the latest Nexorious release from the GitHub API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient returns a Client pointed at the real GitHub API.
func NewClient() *Client {
	return NewClientWithBaseURL("https://api.github.com")
}

// NewClientWithBaseURL returns a Client with a custom API base URL (tests).
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

// FetchLatest returns the latest stable release of drzero42/nexorious.
func (c *Client) FetchLatest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/repos/drzero42/nexorious/releases/latest", nil)
	if err != nil {
		return Release{}, fmt.Errorf("updatecheck: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nexorious-update-check")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("updatecheck: fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("updatecheck: GitHub returned status %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("updatecheck: decode release: %w", err)
	}
	if rel.TagName == "" {
		return Release{}, fmt.Errorf("updatecheck: release response has empty tag_name")
	}
	return rel, nil
}
```

- [ ] **Step 4: Write the State**

Create `internal/services/updatecheck/state.go` (thin mutex wrapper — no dedicated test per project testing policy; it is exercised by the worker and router tests):

```go
package updatecheck

import "sync"

// State holds the most recent successful update-check result. Written by the
// periodic worker, read by the /api/version handler. Safe for concurrent use.
// Zero value (via NewState) means "no check has succeeded yet".
type State struct {
	mu            sync.RWMutex
	latestVersion string // normalized, no leading "v" (e.g. "0.10.0")
	releaseURL    string
}

// NewState returns an empty State.
func NewState() *State {
	return &State{}
}

// Set stores the latest known release. Called only on successful fetches, so
// a failed run leaves the last good value in place.
func (s *State) Set(latestVersion, releaseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestVersion = latestVersion
	s.releaseURL = releaseURL
}

// Latest returns the stored latest version (no leading "v") and release URL.
// Both are empty until the first successful check.
func (s *State) Latest() (latestVersion, releaseURL string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestVersion, s.releaseURL
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/services/updatecheck/... -v`
Expected: PASS (compare + all 4 FetchLatest tests)

- [ ] **Step 6: Commit**

```bash
git add internal/services/updatecheck/
git commit -m "feat: add updatecheck GitHub client and shared state"
```

---

### Task 3: Notify event type `admin.version.available`

**Files:**
- Modify: `internal/notify/registry.go` (constant + registry entry)
- Modify: `internal/notify/payloads.go` (payload struct)
- Modify: `internal/notify/formatters.go` (Format case)
- Modify: `internal/notify/formatters_test.go` (samplePayloads + wantRender)
- Modify: `internal/notify/registry_test.go` (expected-types + defaults lists)

- [ ] **Step 1: Write the failing tests first**

In `internal/notify/formatters_test.go`, add to `samplePayloads`:

```go
	TypeAdminVersionAvailable: VersionAvailablePayload{CurrentVersion: "0.9.0", AvailableVersion: "0.10.0", ReleaseURL: "https://github.com/drzero42/nexorious/releases/tag/v0.10.0"},
```

and to `wantRender`:

```go
	TypeAdminVersionAvailable: {"New version available", "Nexorious 0.10.0 is available (you are running 0.9.0): https://github.com/drzero42/nexorious/releases/tag/v0.10.0"},
```

In `internal/notify/registry_test.go`:
- add `"admin.version.available"` to the `want` slice in `TestRegistryHasExpectedTypes`;
- add `"admin.version.available"` to the **default-on** list in `TestDefaultSubscriptionsAreFailuresOnly` (the first loop, alongside `admin.backup.failed` etc.).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/notify/... -run 'TestRegistryHasExpectedTypes|TestDefaultSubscriptionsAreFailuresOnly|TestFormat_AllRegisteredTypesRoundTrip' -v`
Expected: FAIL (compile error — `TypeAdminVersionAvailable` / `VersionAvailablePayload` undefined)

- [ ] **Step 3: Implement registry, payload, formatter**

`internal/notify/registry.go` — add to the type constants block (after `TypeAdminMaintFailed`):

```go
	TypeAdminVersionAvailable   = "admin.version.available"
```

and to the `registry` slice (after the Maintenance entries):

```go
	{TypeAdminVersionAvailable, ScopeAdmin, "Updates", "New version available", true},
```

`internal/notify/payloads.go` — add at the end:

```go
// VersionAvailablePayload announces that a newer release than the running
// build is available on GitHub.
type VersionAvailablePayload struct {
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version"`
	ReleaseURL       string `json:"release_url"`
}
```

`internal/notify/formatters.go` — add a case to the `switch eventType` (after the `TypeAdminMaint*` cases, matching the file's style):

```go
	case TypeAdminVersionAvailable:
		var p VersionAvailablePayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "New version available"
		if decodeErr != nil {
			body = "A newer version of Nexorious is available."
		} else {
			body = fmt.Sprintf("Nexorious %s is available (you are running %s): %s",
				fallback(p.AvailableVersion, "a newer version"),
				fallback(p.CurrentVersion, "an unknown version"),
				fallback(p.ReleaseURL, "https://github.com/drzero42/nexorious/releases"))
		}
```

- [ ] **Step 4: Run the notify package tests**

Run: `go test ./internal/notify/... -v`
Expected: PASS (round-trip test now covers the new type; registry tests pass)

- [ ] **Step 5: Commit**

```bash
git add internal/notify/
git commit -m "feat: add admin.version.available notification event type"
```

---

### Task 4: Backfill migration for existing admins

The new type is `DefaultOn: true`, but `SeedDefaultSubscriptions` only runs at user creation — existing admins need a backfill (precedent: `20260601000004_backfill_default_subscriptions`).

**Files:**
- Create: `internal/db/migrations/20260609000001_backfill_version_subscription.up.sql`
- Create: `internal/db/migrations/20260609000001_backfill_version_subscription.down.sql`

- [ ] **Step 1: Write the up migration**

`internal/db/migrations/20260609000001_backfill_version_subscription.up.sql`:

```sql
-- Subscribe existing admins to the new admin.version.available event type.
-- New users are seeded at creation time (see notify.SeedDefaultSubscriptions);
-- this covers admins created before the type existed.
-- Idempotent: ON CONFLICT DO NOTHING skips admins already subscribed.

INSERT INTO notification_subscriptions (user_id, event_type, created_at)
SELECT u.id, 'admin.version.available', now()
FROM users u
WHERE u.is_admin = true
ON CONFLICT (user_id, event_type) DO NOTHING;
```

- [ ] **Step 2: Write the down migration**

`internal/db/migrations/20260609000001_backfill_version_subscription.down.sql` (matches the no-op precedent of `20260601000004`):

```sql
-- No-op: the backfill cannot be safely reversed (backfilled rows are
-- indistinguishable from subscriptions the user has since chosen to keep).
SELECT 1;
```

- [ ] **Step 3: Verify migrations still build/discover**

Run: `go test ./internal/db/... 2>&1 | tail -5` (migration discovery is exercised by any package TestMain that runs migrations; a build is the minimum gate)
Run: `go build ./...`
Expected: builds cleanly; no duplicate-prefix errors.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260609000001_backfill_version_subscription.up.sql internal/db/migrations/20260609000001_backfill_version_subscription.down.sql
git commit -m "feat: backfill admin.version.available subscription for existing admins"
```

---

### Task 5: Config field `UpdateCheckEnabled`

**Files:**
- Modify: `internal/config/config.go`

No dedicated test — a plain `caarlos0/env` field parse is a thin wrapper per the project testing policy.

- [ ] **Step 1: Add the field**

In `internal/config/config.go`, in the `Application` section (after `Debug    bool   \`env:"DEBUG"\``):

```go
	// UpdateCheckEnabled controls the periodic GitHub release check that powers
	// the "newer version available" sidebar notice and admin notification.
	// Set false to disable the check entirely (no outbound GitHub requests).
	UpdateCheckEnabled bool `env:"UPDATE_CHECK_ENABLED" envDefault:"true"`
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add UPDATE_CHECK_ENABLED config flag"
```

---

### Task 6: `CheckForUpdatesWorker` (River worker)

**Files:**
- Create: `internal/scheduler/update_check.go`
- Test: `internal/scheduler/update_check_test.go` (uses the package's shared `testDB`)

- [ ] **Step 1: Write the failing tests**

Create `internal/scheduler/update_check_test.go`:

```go
package scheduler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/drzero42/nexorious/internal/scheduler"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// newGitHubStub returns an httptest server mimicking the GitHub
// releases/latest endpoint, plus a request counter.
func newGitHubStub(t *testing.T, tagName, htmlURL string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tagName + `","html_url":"` + htmlURL + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func countVersionEvents(t *testing.T) int {
	t.Helper()
	var n int
	err := testDB.NewRaw(
		`SELECT count(*) FROM events WHERE type = 'admin.version.available'`,
	).Scan(context.Background(), &n)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func TestCheckForUpdates_EmitsOncePerRelease(t *testing.T) {
	truncateAllTables(t)
	srv, _ := newGitHubStub(t, "v9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "0.1.0",
		Enabled:        true,
	}

	// Two runs — the dedup key must collapse them into one event.
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if got := countVersionEvents(t); got != 1 {
		t.Errorf("events = %d, want exactly 1 (deduped)", got)
	}
	latest, url := st.Latest()
	if latest != "9.9.9" {
		t.Errorf("state latest = %q, want 9.9.9 (v stripped)", latest)
	}
	if url != "https://github.com/drzero42/nexorious/releases/tag/v9.9.9" {
		t.Errorf("state url = %q", url)
	}
}

func TestCheckForUpdates_UpToDateEmitsNothing(t *testing.T) {
	truncateAllTables(t)
	srv, _ := newGitHubStub(t, "v0.1.0", "https://github.com/drzero42/nexorious/releases/tag/v0.1.0")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "0.1.0",
		Enabled:        true,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if got := countVersionEvents(t); got != 0 {
		t.Errorf("events = %d, want 0", got)
	}
	if latest, _ := st.Latest(); latest != "0.1.0" {
		t.Errorf("state latest = %q, want 0.1.0 (state still updated)", latest)
	}
}

func TestCheckForUpdates_DevVersionEmitsNothing(t *testing.T) {
	truncateAllTables(t)
	srv, _ := newGitHubStub(t, "v9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "dev",
		Enabled:        true,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if got := countVersionEvents(t); got != 0 {
		t.Errorf("events = %d, want 0 for dev build", got)
	}
}

func TestCheckForUpdates_DisabledShortCircuits(t *testing.T) {
	truncateAllTables(t)
	srv, calls := newGitHubStub(t, "v9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")

	st := updatecheck.NewState()
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(srv.URL),
		RunningVersion: "0.1.0",
		Enabled:        false,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if calls.Load() != 0 {
		t.Errorf("GitHub fetches = %d, want 0 when disabled", calls.Load())
	}
	if got := countVersionEvents(t); got != 0 {
		t.Errorf("events = %d, want 0 when disabled", got)
	}
	if latest, _ := st.Latest(); latest != "" {
		t.Errorf("state latest = %q, want empty when disabled", latest)
	}
}

func TestCheckForUpdates_FetchFailureKeepsLastGoodState(t *testing.T) {
	truncateAllTables(t)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)

	st := updatecheck.NewState()
	st.Set("0.2.0", "https://github.com/drzero42/nexorious/releases/tag/v0.2.0")
	w := &scheduler.CheckForUpdatesWorker{
		DB:             testDB,
		State:          st,
		Client:         updatecheck.NewClientWithBaseURL(failing.URL),
		RunningVersion: "0.1.0",
		Enabled:        true,
	}
	if err := w.Work(context.Background(), nil); err != nil {
		t.Fatalf("Work must not error on fetch failure, got: %v", err)
	}
	if latest, _ := st.Latest(); latest != "0.2.0" {
		t.Errorf("state latest = %q, want last good value 0.2.0", latest)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scheduler/... -run TestCheckForUpdates -v`
Expected: FAIL (compile error — `CheckForUpdatesWorker` undefined)

- [ ] **Step 3: Implement the worker**

Create `internal/scheduler/update_check.go`:

```go
package scheduler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// ── CheckForUpdates ───────────────────────────────────────────────────────────

type CheckForUpdatesArgs struct{}

func (CheckForUpdatesArgs) Kind() string { return "check_for_updates" }

// CheckForUpdatesWorker fetches the latest stable GitHub release, stores it
// in the shared State (read by /api/version), and emits an
// admin.version.available event once per release when the running build is
// behind. Failures are logged and never fail the run.
type CheckForUpdatesWorker struct {
	river.WorkerDefaults[CheckForUpdatesArgs]
	DB             *bun.DB
	State          *updatecheck.State
	Client         *updatecheck.Client
	RunningVersion string
	Enabled        bool
}

func (w *CheckForUpdatesWorker) Work(ctx context.Context, _ *river.Job[CheckForUpdatesArgs]) error {
	if !w.Enabled {
		return nil
	}

	rel, err := w.Client.FetchLatest(ctx)
	if err != nil {
		slog.Warn("update check: fetch latest release failed", "err", err)
		return nil
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	w.State.Set(latest, rel.HTMLURL)

	if !updatecheck.UpdateAvailable(w.RunningVersion, latest) {
		return nil
	}

	notify.Emit(ctx, w.DB, notify.EmitParams{
		Type:  notify.TypeAdminVersionAvailable,
		Scope: notify.ScopeAdmin,
		Payload: notify.VersionAvailablePayload{
			CurrentVersion:   w.RunningVersion,
			AvailableVersion: latest,
			ReleaseURL:       rel.HTMLURL,
		},
		DedupKey: notify.TypeAdminVersionAvailable + ":" + latest,
	})
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/scheduler/... -run TestCheckForUpdates -v`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/update_check.go internal/scheduler/update_check_test.go
git commit -m "feat: add periodic update-check worker"
```

---

### Task 7: Register worker + periodic job (serve.go, BuildPeriodicJobs)

**Files:**
- Modify: `internal/scheduler/scheduler.go` (`BuildPeriodicJobs`)
- Modify: `cmd/nexorious/serve.go` (worker registration — **two** sites: initial startup AND `RebuildServices`)

- [ ] **Step 1: Add the periodic job to `BuildPeriodicJobs`**

In `internal/scheduler/scheduler.go`, change the `return []*river.PeriodicJob{ ... }` in `BuildPeriodicJobs` to build a slice and conditionally append (keep every existing entry exactly as-is):

```go
	jobs := []*river.PeriodicJob{
		// ... all existing entries unchanged ...
	}

	if cfg.UpdateCheckEnabled {
		// Unlike the other periodic jobs, RunOnStart is true so the sidebar
		// notice appears shortly after boot; the per-release dedup key keeps
		// the admin notification at once per release regardless.
		jobs = append(jobs, river.NewPeriodicJob(
			mustCron("0 */6 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CheckForUpdatesArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	return jobs
```

- [ ] **Step 2: Wire up serve.go (startup path)**

In `cmd/nexorious/serve.go`, before `workers := river.NewWorkers()` (~line 194), construct the shared pieces:

```go
	updateState := updatecheck.NewState()
	updateClient := updatecheck.NewClient()
```

Add the worker registration alongside the other scheduler workers (after the `notify.PruneEventsWorker` line ~218):

```go
	river.AddWorker(workers, &scheduler.CheckForUpdatesWorker{
		DB:             db,
		State:          updateState,
		Client:         updateClient,
		RunningVersion: version,
		Enabled:        cfg.UpdateCheckEnabled,
	})
```

Add the import `"github.com/drzero42/nexorious/internal/services/updatecheck"` to serve.go's import block.

- [ ] **Step 3: Wire up serve.go (RebuildServices path)**

Inside `RebuildServices` (after the `notify.PruneEventsWorker` registration on `newWorkers`, ~line 307), add the same worker with `newDB` (reuse the same `updateState`/`updateClient` — they are DB-independent):

```go
			river.AddWorker(newWorkers, &scheduler.CheckForUpdatesWorker{
				DB:             newDB,
				State:          updateState,
				Client:         updateClient,
				RunningVersion: version,
				Enabled:        cfg.UpdateCheckEnabled,
			})
```

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: success. (`updateState` is passed to `api.New` in Task 8 — until then it is used by both worker registrations, so no unused-variable error.)

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/scheduler.go cmd/nexorious/serve.go
git commit -m "feat: register update-check periodic job and worker"
```

---

### Task 8: Extend `GET /api/version`

**Files:**
- Modify: `internal/api/router.go` (`New`, `registerRoutes`, the `/api/version` handler)
- Modify: `cmd/nexorious/serve.go` (pass `updateState` to `api.New`)
- Modify (mechanical, add `nil` arg): `internal/api/import_test.go:107`, `internal/api/auth_test.go:71,83,277`, `internal/api/games_test.go:212,427`, `internal/api/backup_test.go:29`, `internal/api/router_test.go:19,35` and any other `api.New(` call sites (`grep -rn "api.New(" internal/ cmd/`)
- Test: `internal/api/router_test.go` (new version-endpoint tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/router_test.go` (note: `testCfg()` lives in the package's test helpers — set `UpdateCheckEnabled` explicitly on the returned config):

```go
func getVersion(t *testing.T, e http.Handler) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/version = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func TestVersionEndpoint_UpdateAvailable(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = true
	st := updatecheck.NewState()
	st.Set("9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "0.1.0", "abc1234", st)

	body := getVersion(t, e)
	if body["update_available"] != true {
		t.Errorf("update_available = %v, want true", body["update_available"])
	}
	if body["latest_version"] != "9.9.9" {
		t.Errorf("latest_version = %v, want 9.9.9", body["latest_version"])
	}
	if body["release_url"] != "https://github.com/drzero42/nexorious/releases/tag/v9.9.9" {
		t.Errorf("release_url = %v", body["release_url"])
	}
	if body["update_check_enabled"] != true {
		t.Errorf("update_check_enabled = %v, want true", body["update_check_enabled"])
	}
	if body["version"] != "0.1.0" || body["commit"] != "abc1234" {
		t.Errorf("version/commit = %v/%v", body["version"], body["commit"])
	}
}

func TestVersionEndpoint_DevBuildNeverClaimsUpdate(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = true
	st := updatecheck.NewState()
	st.Set("9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "dev", "unknown", st)

	body := getVersion(t, e)
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false for dev build", body["update_available"])
	}
	if body["latest_version"] != "" || body["release_url"] != "" {
		t.Errorf("latest_version/release_url = %v/%v, want empty", body["latest_version"], body["release_url"])
	}
}

func TestVersionEndpoint_CheckDisabled(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = false
	st := updatecheck.NewState()
	st.Set("9.9.9", "https://github.com/drzero42/nexorious/releases/tag/v9.9.9")
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "0.1.0", "abc1234", st)

	body := getVersion(t, e)
	if body["update_check_enabled"] != false {
		t.Errorf("update_check_enabled = %v, want false", body["update_check_enabled"])
	}
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false when disabled", body["update_available"])
	}
	if body["latest_version"] != "" || body["release_url"] != "" {
		t.Errorf("latest_version/release_url = %v/%v, want empty", body["latest_version"], body["release_url"])
	}
}

func TestVersionEndpoint_NilStateIsSafe(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	cfg := testCfg()
	cfg.UpdateCheckEnabled = true
	e := api.New(testEncrypter, cfg, m, nil, "", nil, nil, nil, "0.1.0", "abc1234", nil)

	body := getVersion(t, e)
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false with nil state", body["update_available"])
	}
}
```

Add imports to router_test.go as needed: `"github.com/drzero42/nexorious/internal/services/updatecheck"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestVersionEndpoint -v`
Expected: FAIL (compile error — `api.New` does not accept the extra argument)

- [ ] **Step 3: Change the signature and handler**

In `internal/api/router.go`:

`New` (line ~48) — add `updateState *updatecheck.State` between `commit string` and the variadic:

```go
func New(encrypter *crypto.Encrypter, cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, version, commit string, updateState *updatecheck.State, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
```

Pass it through to `registerRoutes` (line ~147) and update `registerRoutes`'s signature the same way (line ~152).

Replace the `/api/version` handler (lines 178–184):

```go
	// Version — public, not cached (changes on every deploy)
	e.GET("/api/version", func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		resp := map[string]any{
			"version":              version,
			"commit":               commit,
			"update_check_enabled": cfg.UpdateCheckEnabled,
			"update_available":     false,
			"latest_version":       "",
			"release_url":          "",
		}
		if cfg.UpdateCheckEnabled && updateState != nil {
			if latest, releaseURL := updateState.Latest(); updatecheck.UpdateAvailable(version, latest) {
				resp["update_available"] = true
				resp["latest_version"] = latest
				resp["release_url"] = releaseURL
			}
		}
		return c.JSON(http.StatusOK, resp)
	})
```

Add the import `"github.com/drzero42/nexorious/internal/services/updatecheck"` to router.go.

- [ ] **Step 4: Update all callers**

- `cmd/nexorious/serve.go:351`: `e := api.New(encrypter, cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, version, commit, updateState, riverClient)`
- Every test call site found by `grep -rn "api.New(" internal/`: insert `nil` between `commit` and the river client (or as the new last arg when no river client is passed). Example: `api.New(testEncrypter, cfg, m, db, "", nil, nil, nil, "dev", "unknown")` → `api.New(testEncrypter, cfg, m, db, "", nil, nil, nil, "dev", "unknown", nil)`.

- [ ] **Step 5: Run tests**

Run: `go build ./... && go test ./internal/api/... -run TestVersionEndpoint -v`
Expected: PASS (all 4 new tests)

Run: `go test ./internal/api/... 2>&1 | tail -3`
Expected: package passes (call-site updates verified).

- [ ] **Step 6: Commit**

```bash
git add internal/api/ cmd/nexorious/serve.go
git commit -m "feat: expose update availability in /api/version"
```

---

### Task 9: Frontend — VersionInfo fields + sidebar link

**Files:**
- Modify: `ui/frontend/src/hooks/use-version.ts`
- Create: `ui/frontend/src/components/navigation/version-footer.tsx`
- Modify: `ui/frontend/src/components/navigation/sidebar.tsx`
- Test: `ui/frontend/src/components/navigation/version-footer.test.tsx`

- [ ] **Step 1: Extend the type**

`ui/frontend/src/hooks/use-version.ts` — extend the interface (hook body unchanged):

```ts
export interface VersionInfo {
  version: string;
  commit: string;
  update_check_enabled: boolean;
  update_available: boolean;
  latest_version: string;
  release_url: string;
}
```

- [ ] **Step 2: Write the failing test**

Create `ui/frontend/src/components/navigation/version-footer.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { VersionFooter } from './version-footer';

const mockUseVersion = vi.fn();
vi.mock('@/hooks', () => ({
  useVersion: () => mockUseVersion(),
}));

describe('VersionFooter', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders the update link when an update is available', () => {
    mockUseVersion.mockReturnValue({
      data: {
        version: '0.9.0',
        commit: 'abc1234',
        update_check_enabled: true,
        update_available: true,
        latest_version: '0.10.0',
        release_url: 'https://github.com/drzero42/nexorious/releases/tag/v0.10.0',
      },
    });
    render(<VersionFooter />);
    expect(screen.getByText('Version: 0.9.0')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: /a newer version is available/i });
    expect(link).toHaveAttribute('href', 'https://github.com/drzero42/nexorious/releases/tag/v0.10.0');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('renders no link when no update is available', () => {
    mockUseVersion.mockReturnValue({
      data: {
        version: '0.9.0',
        commit: 'abc1234',
        update_check_enabled: true,
        update_available: false,
        latest_version: '',
        release_url: '',
      },
    });
    render(<VersionFooter />);
    expect(screen.getByText('Version: 0.9.0')).toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  it('renders nothing while version data is missing', () => {
    mockUseVersion.mockReturnValue({ data: undefined });
    const { container } = render(<VersionFooter />);
    expect(container).toBeEmptyDOMElement();
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test version-footer.test.tsx`
Expected: FAIL (cannot resolve `./version-footer`)

- [ ] **Step 4: Implement the component**

Create `ui/frontend/src/components/navigation/version-footer.tsx`:

```tsx
import { useVersion } from '@/hooks';

export function VersionFooter() {
  const { data: versionInfo } = useVersion();

  if (!versionInfo?.version) return null;

  return (
    <div className="px-4 pb-3 text-xs text-muted-foreground">
      <div>Version: {versionInfo.version}</div>
      {versionInfo.update_available && versionInfo.release_url && (
        <a
          href={versionInfo.release_url}
          target="_blank"
          rel="noopener noreferrer"
          className="underline hover:text-foreground"
        >
          A newer version is available
        </a>
      )}
    </div>
  );
}
```

- [ ] **Step 5: Use it in the sidebar**

In `ui/frontend/src/components/navigation/sidebar.tsx`:
- remove `useVersion` from the `@/hooks` import and delete the `const { data: versionInfo } = useVersion();` line;
- add `import { VersionFooter } from './version-footer';`
- replace the version block (lines 75–80):

```tsx
      {/* Version */}
      <VersionFooter />
```

- [ ] **Step 6: Run tests + checks**

Run (from `ui/frontend/`):
```bash
npm run test version-footer.test.tsx
npm run check
npm run knip
```
Expected: test PASS, zero type errors, zero knip findings.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/hooks/use-version.ts ui/frontend/src/components/navigation/
git commit -m "feat: show newer-version link in sidebar footer"
```

---

### Task 10: Document `UPDATE_CHECK_ENABLED`

**Files:**
- Modify: `docs/admin-guide.md` (Full reference → **Application** table)

- [ ] **Step 1: Add the row**

In `docs/admin-guide.md`, in the **Application** env-var table (after the `WORKER_COUNT` row):

```markdown
| `UPDATE_CHECK_ENABLED` | `true` | Periodically check GitHub for a newer release. When one exists, the sidebar shows an update notice and admins receive a one-time notification per release. The check runs server-side (one request to the GitHub API every 6 hours); set `false` to disable it entirely. |
```

- [ ] **Step 2: Commit**

```bash
git add docs/admin-guide.md
git commit -m "docs: document UPDATE_CHECK_ENABLED"
```

---

### Task 11: Nix `vendorHash` bump (go.mod changed)

**Files:**
- Modify: `nix/package.nix`

- [ ] **Step 1: Recompute the hash**

Edit `nix/package.nix`: set `vendorHash = pkgs.lib.fakeHash;` (or `lib.fakeHash`, matching the file's existing idiom), then:

```bash
nix build .#nexorious 2>&1 | grep "got:"
```

Paste the `got:` hash back into `nix/package.nix` → `vendorHash`.

- [ ] **Step 2: Verify the build**

```bash
nix build .#nexorious
```
Expected: builds successfully.

- [ ] **Step 3: Commit**

```bash
git add nix/package.nix
git commit -m "chore: update vendorHash for golang.org/x/mod"
```

---

### Task 12: Final verification + PR

- [ ] **Step 1: Full backend suite**

Run: `go test -timeout 600s ./...`
Expected: all packages PASS.

Run: `golangci-lint run`
Expected: zero findings.

- [ ] **Step 2: Full frontend suite**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all green.

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin 899-update-check
gh pr create --title "feat: notify when a newer version is available" --body "$(cat <<'EOF'
Adds a server-side GitHub release check (every 6h, opt-out via `UPDATE_CHECK_ENABLED`) that:

- shows a small "A newer version is available" link in the sidebar footer under the version number, pointing at the GitHub release page;
- emits an `admin.version.available` notification through the existing notification system, once per release (deduped on version), delivered to subscribed admins via their configured channels;
- extends `GET /api/version` with `update_check_enabled`, `update_available`, `latest_version`, and `release_url` — read from in-process state, no network call in the request path;
- backfills the new default-on subscription for existing admins (new admins are seeded automatically);
- treats `dev`/non-semver builds as never outdated.

Closes #899
EOF
)"
```

Note (per project rules): do **not** merge the PR — wait for the user's explicit instruction.
