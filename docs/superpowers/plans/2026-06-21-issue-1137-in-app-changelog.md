# In-App Changelog ("What's New") Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface release changelogs in-app and in `nexctl`, with each user seeing the diff of releases newer than the version they last viewed.

**Architecture:** The server embeds the repo-root `CHANGELOG.md` (release-please output) into a new `internal/changelog` package that parses it into structured entries and slices/renders it. A `GET /api/changelog` endpoint owns parsing + slicing + the per-user `last_seen_changelog_version` (stored in `user_settings`); a sibling `GET /api/changelog/unseen` gives the web a cheap "is there anything new?" boolean and lazily captures each user's baseline. The web shows a quiet dot + dialog; `nexctl changelog` mirrors it for the CLI.

**Tech Stack:** Go 1.26, Bun ORM, Echo v5, `golang.org/x/mod/semver` (via `internal/services/updatecheck`), `//go:embed`; React 19 + TanStack Query + shadcn Dialog + ReactMarkdown; cobra (`nexctl`).

## Global Constraints

- **This lands as a single PR** on one feature branch. Commit per task for reviewability.
- **`main` is PR-protected** — branch first, PR at the end; never push to `main`.
- **Migrations are new files** named `YYYYMMDD<nnnnnn>_name.{up,down}.sql`; the latest existing is `20260620000001_baseline`, so the new one is `20260621000001_*`.
- **The `nexorious` server binary embeds the changelog; `nexctl` does NOT** (it fetches over REST). Do not link `internal/changelog` from `cmd/nexctl`.
- **Semver source of truth is `internal/services/updatecheck`** — slicing/validation reuse its helpers; do not introduce a second semver implementation in Go. (The web needs one tiny client-side check; see Task 8.)
- **Graceful degradation is mandatory:** an absent/placeholder embedded changelog, a non-release running version (`dev`, `<branch>-<date>-<commit>`), an unknown `?since=`, or `last_seen` newer than the running version must all yield an empty/"unavailable" result, never an error or a full-history blast.
- **Logging:** request-path code uses `slog.ErrorContext(ctx, …, logging.KeyErr, err, logging.Cat(logging.CategoryDB))`; never log secrets/bodies.
- **errcheck runs with check-blank; gosec is on** — handle or annotate every error; no bare `_ =`.

---

### Task 1: Exported semver `Compare` in `updatecheck`

**Files:**
- Modify: `internal/services/updatecheck/compare.go`
- Test: `internal/services/updatecheck/compare_test.go`

**Interfaces:**
- Consumes: existing unexported `normalize`, `golang.org/x/mod/semver`.
- Produces: `func Compare(a, b string) int` — `-1/0/+1` as `a` <, =, > `b` by semver. Two invalid versions compare equal (`0`); a valid version sorts **after** an invalid one (so `Compare("0.1.0", "") == 1`). Used by `internal/changelog`.

- [ ] **Step 1: Write the failing test**

Add to `internal/services/updatecheck/compare_test.go`:

```go
func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.2.0", -1},
		{"1.0.0", "1.0.0", 0},
		{"v0.2.0", "0.1.0", 1}, // leading v tolerated on either side
		{"0.90.0", "0.17.1", 1},
		{"0.1.0", "", 1},       // valid sorts after invalid/empty
		{"", "0.1.0", -1},
		{"dev", "also-bad", 0}, // two invalid compare equal
	}
	for _, tc := range cases {
		if got := updatecheck.Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
```

(Use the same package declaration as the existing test file — check whether it is `package updatecheck` or `updatecheck_test` and match it; if it is the external `_test` package, the call is `updatecheck.Compare`, as written above.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/updatecheck/ -run TestCompare -v`
Expected: FAIL — `undefined: updatecheck.Compare`.

- [ ] **Step 3: Add the implementation**

Append to `internal/services/updatecheck/compare.go`:

```go
// Compare returns -1, 0, or +1 as a is less than, equal to, or greater than b,
// comparing as semver (with or without a leading "v"). Invalid versions (e.g.
// "dev") sort before valid ones; two invalid versions compare equal. Callers
// slice the changelog with this so non-release builds degrade gracefully.
func Compare(a, b string) int {
	na, nb := normalize(a), normalize(b)
	va, vb := semver.IsValid(na), semver.IsValid(nb)
	switch {
	case va && vb:
		return semver.Compare(na, nb)
	case va:
		return 1
	case vb:
		return -1
	default:
		return 0
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/services/updatecheck/ -run TestCompare -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/updatecheck/compare.go internal/services/updatecheck/compare_test.go
git commit -m "feat(updatecheck): add exported semver Compare helper"
```

---

### Task 2: `internal/changelog` package — parse, slice, render, embed

**Files:**
- Create: `internal/changelog/changelog.go`
- Create: `internal/changelog/data/.gitkeep` (committed placeholder; empty file)
- Create: `internal/changelog/changelog_test.go`
- Create: `internal/changelog/testdata/sample.md` (test fixture)

**Interfaces:**
- Consumes: `updatecheck.Compare`, `updatecheck.IsValidVersion` (Task 1 + existing).
- Produces:
  - `type Entry struct { Version string; Date string; Groups []Group }` (JSON tags `version`, `date`, `groups`).
  - `type Group struct { Title string; Items []string }` (JSON tags `title`, `items`).
  - `func Parse(md string) []Entry`
  - `func Newer(entries []Entry, sinceExclusive string) []Entry`
  - `func Render(entries []Entry) string` (transformed markdown for the web)
  - `func All() ([]Entry, bool)` — parses the embedded file; `ok=false` when absent/placeholder.

- [ ] **Step 1: Write the test fixture**

Create `internal/changelog/testdata/sample.md` (mirrors real release-please output, including issue/commit refs and a `closes` clause):

```markdown
# Changelog

## [0.90.0](https://github.com/drzero42/nexorious/compare/v0.17.1...v0.90.0) (2026-06-20)


### Features

* **db:** squash migrations into a single baseline ([#1122](https://github.com/drzero42/nexorious/issues/1122)) ([29e809b](https://github.com/drzero42/nexorious/commit/29e809b))
* add nexctl setup command ([#1128](https://github.com/drzero42/nexorious/issues/1128)) ([869bd48](https://github.com/drzero42/nexorious/commit/869bd48)), closes [#1123](https://github.com/drzero42/nexorious/issues/1123)


### Bug Fixes

* **ui:** keep the library tab title visible ([#1131](https://github.com/drzero42/nexorious/issues/1131)) ([36a8540](https://github.com/drzero42/nexorious/commit/36a8540))

## [0.17.1](https://github.com/drzero42/nexorious/compare/v0.17.0...v0.17.1) (2026-06-19)


### Bug Fixes

* **ci:** give the Helm attest step credentials ([#1115](https://github.com/drzero42/nexorious/issues/1115)) ([6941beb](https://github.com/drzero42/nexorious/commit/6941beb))
```

- [ ] **Step 2: Write the failing tests**

Create `internal/changelog/changelog_test.go`:

```go
package changelog

import (
	"os"
	"strings"
	"testing"
)

func loadSample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/sample.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func TestParse(t *testing.T) {
	entries := Parse(loadSample(t))
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Version != "0.90.0" || entries[0].Date != "2026-06-20" {
		t.Fatalf("entry0 = %q/%q", entries[0].Version, entries[0].Date)
	}
	if len(entries[0].Groups) != 2 {
		t.Fatalf("entry0 groups = %d, want 2 (Features, Bug Fixes)", len(entries[0].Groups))
	}
	if entries[0].Groups[0].Title != "Features" {
		t.Fatalf("group0 title = %q", entries[0].Groups[0].Title)
	}
	// dev noise stripped: no commit hash, no (#NNNN), no "closes", no bold markers
	item := entries[0].Groups[0].Items[1]
	if item != "add nexctl setup command" {
		t.Fatalf("item not cleaned: %q", item)
	}
	if strings.Contains(item, "#") || strings.Contains(item, "closes") || strings.Contains(item, "*") {
		t.Fatalf("residual noise in %q", item)
	}
	// scope bold stripped but text kept
	if got := entries[0].Groups[0].Items[0]; got != "db: squash migrations into a single baseline" {
		t.Fatalf("scoped item = %q", got)
	}
}

func TestNewer(t *testing.T) {
	entries := Parse(loadSample(t))
	// since 0.17.1 -> only 0.90.0
	got := Newer(entries, "0.17.1")
	if len(got) != 1 || got[0].Version != "0.90.0" {
		t.Fatalf("Newer(0.17.1) = %+v", got)
	}
	// since current/newest -> empty
	if got := Newer(entries, "0.90.0"); len(got) != 0 {
		t.Fatalf("Newer(0.90.0) should be empty, got %d", len(got))
	}
	// since older than all -> all
	if got := Newer(entries, "0.1.0"); len(got) != 2 {
		t.Fatalf("Newer(0.1.0) = %d, want 2", len(got))
	}
	// since newer than newest (downgrade case) -> empty
	if got := Newer(entries, "9.9.9"); len(got) != 0 {
		t.Fatalf("Newer(9.9.9) should be empty, got %d", len(got))
	}
}

func TestRender(t *testing.T) {
	entries := Parse(loadSample(t))
	md := Render(entries[:1])
	if !strings.Contains(md, "## 0.90.0 — 2026-06-20") {
		t.Fatalf("render missing version header:\n%s", md)
	}
	if !strings.Contains(md, "### Features") || !strings.Contains(md, "- add nexctl setup command") {
		t.Fatalf("render missing grouped item:\n%s", md)
	}
	if strings.Contains(md, "](http") {
		t.Fatalf("render still contains links:\n%s", md)
	}
}

func TestAll_PlaceholderUnavailable(t *testing.T) {
	// With only the committed .gitkeep placeholder (no real CHANGELOG.md copied
	// in), the embedded changelog is unavailable.
	if _, ok := All(); ok {
		t.Skip("a real CHANGELOG.md is present in data/; skipping placeholder check")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/changelog/ -v`
Expected: FAIL — package/`Parse`/`Newer`/`Render`/`All` undefined.

- [ ] **Step 4: Create the placeholder embed file**

```bash
mkdir -p internal/changelog/data
: > internal/changelog/data/.gitkeep
```

- [ ] **Step 5: Write the implementation**

Create `internal/changelog/changelog.go`:

```go
// Package changelog parses the release-please CHANGELOG.md (embedded into the
// server binary) into structured per-version entries and slices it by semver.
// The repo-root CHANGELOG.md is copied into data/ only by asset-populating
// builds (make, release-artifacts.yaml, nix); test/lint builds see just the
// committed .gitkeep placeholder, so All reports unavailable and callers
// degrade gracefully.
package changelog

import (
	"embed"
	"regexp"
	"strings"

	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

//go:embed all:data
var dataFS embed.FS

// Entry is one released version's notes.
type Entry struct {
	Version string  `json:"version"`
	Date    string  `json:"date"`
	Groups  []Group `json:"groups"`
}

// Group is a titled list of human-readable change descriptions.
type Group struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

var (
	// "## [0.90.0](compare-url) (2026-06-20)" — link and date are optional.
	reHeader = regexp.MustCompile(`^## \[?([0-9]+\.[0-9]+\.[0-9]+)\]?(?:\([^)]*\))?(?:\s+\(([0-9]{4}-[0-9]{2}-[0-9]{2})\))?`)
	reGroup  = regexp.MustCompile(`^### (.+)$`)
	reItem   = regexp.MustCompile(`^\*\s+(.+)$`)

	reCloses    = regexp.MustCompile(`(?i),?\s*closes \[#\d+\]\([^)]*\)`)
	reCommitRef = regexp.MustCompile(`\s*\(\[[0-9a-f]{7,40}\]\([^)]*\)\)`)
	reIssueRef  = regexp.MustCompile(`\s*\(\[#\d+\]\([^)]*\)\)`)
	reBold      = regexp.MustCompile(`\*\*`)
)

// cleanItem strips release-please dev noise: trailing "closes #N", commit-hash
// and issue-number link refs, and bold scope markers — keeping the human text.
func cleanItem(s string) string {
	s = reCloses.ReplaceAllString(s, "")
	s = reCommitRef.ReplaceAllString(s, "")
	s = reIssueRef.ReplaceAllString(s, "")
	s = reBold.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// Parse turns a release-please CHANGELOG.md into ordered entries (newest first,
// matching the file order). Lines before the first version header are ignored.
func Parse(md string) []Entry {
	var entries []Entry
	hasGroup := false
	for _, line := range strings.Split(md, "\n") {
		switch {
		case reHeader.MatchString(line):
			m := reHeader.FindStringSubmatch(line)
			entries = append(entries, Entry{Version: m[1], Date: m[2]})
			hasGroup = false
		case len(entries) == 0:
			// preamble before the first "## [x.y.z]" header
		case reGroup.MatchString(line):
			m := reGroup.FindStringSubmatch(line)
			i := len(entries) - 1
			entries[i].Groups = append(entries[i].Groups, Group{Title: strings.TrimSpace(m[1])})
			hasGroup = true
		case reItem.MatchString(line):
			m := reItem.FindStringSubmatch(line)
			item := cleanItem(m[1])
			if item == "" {
				continue
			}
			i := len(entries) - 1
			if !hasGroup {
				entries[i].Groups = append(entries[i].Groups, Group{Title: "Changes"})
				hasGroup = true
			}
			g := len(entries[i].Groups) - 1
			entries[i].Groups[g].Items = append(entries[i].Groups[g].Items, item)
		}
	}
	return entries
}

// Newer returns the entries strictly newer than sinceExclusive (semver). An
// empty or invalid sinceExclusive returns nothing, so callers never blast the
// full history through the since-last path without a captured baseline.
func Newer(entries []Entry, sinceExclusive string) []Entry {
	if !updatecheck.IsValidVersion(sinceExclusive) {
		return nil
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if updatecheck.Compare(e.Version, sinceExclusive) > 0 {
			out = append(out, e)
		}
	}
	return out
}

// Render produces a clean markdown slice for the web view: "## <version> —
// <date>", regrouped bullet lists, all dev-noise links removed.
func Render(entries []Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString("## ")
		b.WriteString(e.Version)
		if e.Date != "" {
			b.WriteString(" — ")
			b.WriteString(e.Date)
		}
		b.WriteString("\n\n")
		for _, g := range e.Groups {
			b.WriteString("### ")
			b.WriteString(g.Title)
			b.WriteString("\n\n")
			for _, it := range g.Items {
				b.WriteString("- ")
				b.WriteString(it)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// All parses the embedded changelog. ok is false when only the .gitkeep
// placeholder is present (test/lint builds) or the file is empty.
func All() (entries []Entry, ok bool) {
	b, err := dataFS.ReadFile("data/CHANGELOG.md")
	if err != nil {
		return nil, false
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, false
	}
	return Parse(string(b)), true
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/changelog/ -v`
Expected: PASS (`TestAll_PlaceholderUnavailable` passes because `All()` returns `ok=false` with only `.gitkeep`).

- [ ] **Step 7: Commit**

```bash
git add internal/changelog/
git commit -m "feat(changelog): add CHANGELOG parser, slicer, renderer, and embed"
```

---

### Task 3: Wire the CHANGELOG copy into asset-populating builds

**Files:**
- Modify: `Makefile` (the `build` target)
- Modify: `.github/workflows/release-artifacts.yaml` (after "Build frontend")
- Modify: `nix/package.nix` (`preBuild`)
- Modify: `.gitignore`

**Interfaces:** none (build wiring only). After this task, `make build` produces a `nexorious` binary whose `changelog.All()` returns `ok=true`.

- [ ] **Step 1: Ignore the copied file, keep the placeholder**

Add to `.gitignore` (next to the existing `ui/frontend/dist/*` block):

```
internal/changelog/data/*
!internal/changelog/data/.gitkeep
```

- [ ] **Step 2: Copy CHANGELOG.md in the Makefile `build` target**

In `Makefile`, change the `build` target from:

```makefile
build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious
	go build $(LDFLAGS) -o nexctl ./cmd/nexctl
```

to:

```makefile
build:
	cp CHANGELOG.md internal/changelog/data/CHANGELOG.md
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious
	go build $(LDFLAGS) -o nexctl ./cmd/nexctl
```

- [ ] **Step 3: Copy CHANGELOG.md in release-artifacts.yaml**

In `.github/workflows/release-artifacts.yaml`, immediately after the `Build frontend` step, add:

```yaml
      - name: Embed changelog
        run: cp CHANGELOG.md internal/changelog/data/CHANGELOG.md
```

- [ ] **Step 4: Copy CHANGELOG.md in nix/package.nix preBuild**

In `nix/package.nix`, change `preBuild` from:

```nix
  preBuild = ''
    export CGO_ENABLED=0
    # Populate the embed directory with built frontend assets.
    # go:embed in ui/ui.go expects ui/frontend/dist/ to contain real files.
    cp -r ${nexorious-frontend}/. ui/frontend/dist/
  '';
```

to:

```nix
  preBuild = ''
    export CGO_ENABLED=0
    # Populate the embed directory with built frontend assets.
    # go:embed in ui/ui.go expects ui/frontend/dist/ to contain real files.
    cp -r ${nexorious-frontend}/. ui/frontend/dist/
    # Populate the embedded changelog (go:embed all:data in internal/changelog).
    cp CHANGELOG.md internal/changelog/data/CHANGELOG.md
  '';
```

- [ ] **Step 5: Verify the copy + build path works locally**

Run:
```bash
cp CHANGELOG.md internal/changelog/data/CHANGELOG.md
go test ./internal/changelog/ -run TestAll -v   # now All() returns ok=true
go build ./cmd/nexorious                          # embeds the real file
git status --porcelain internal/changelog/data/   # expect NO tracked change (gitignored)
```
Expected: build succeeds; `git status` shows the copied `CHANGELOG.md` is ignored (only `.gitkeep` is tracked).

- [ ] **Step 6: Commit**

```bash
git add Makefile .github/workflows/release-artifacts.yaml nix/package.nix .gitignore
git commit -m "build(changelog): copy CHANGELOG.md into the embed dir for asset builds"
```

---

### Task 4: Migration + `UserSettings.LastSeenChangelogVersion`

**Files:**
- Create: `internal/db/migrations/20260621000001_user_settings_last_seen_changelog.up.sql`
- Create: `internal/db/migrations/20260621000001_user_settings_last_seen_changelog.down.sql`
- Modify: `internal/db/models/models.go` (the `UserSettings` struct)

**Interfaces:**
- Produces: `models.UserSettings.LastSeenChangelogVersion *string` with bun tag `last_seen_changelog_version` (nullable). Consumed by Task 5.

- [ ] **Step 1: Write the up migration**

Create `internal/db/migrations/20260621000001_user_settings_last_seen_changelog.up.sql`:

```sql
ALTER TABLE user_settings ADD COLUMN last_seen_changelog_version TEXT;
```

- [ ] **Step 2: Write the down migration**

Create `internal/db/migrations/20260621000001_user_settings_last_seen_changelog.down.sql`:

```sql
ALTER TABLE user_settings DROP COLUMN last_seen_changelog_version;
```

- [ ] **Step 3: Add the model field**

In `internal/db/models/models.go`, change the `UserSettings` struct from:

```go
type UserSettings struct {
	bun.BaseModel `bun:"table:user_settings"`

	UserID     string    `bun:"user_id,pk"          json:"user_id"`
	DealRegion string    `bun:"deal_region,notnull" json:"deal_region"`
	CreatedAt  time.Time `bun:"created_at,notnull"  json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,notnull"  json:"updated_at"`
}
```

to:

```go
type UserSettings struct {
	bun.BaseModel `bun:"table:user_settings"`

	UserID                   string    `bun:"user_id,pk"                     json:"user_id"`
	DealRegion               string    `bun:"deal_region,notnull"            json:"deal_region"`
	LastSeenChangelogVersion *string   `bun:"last_seen_changelog_version"    json:"last_seen_changelog_version,omitempty"`
	CreatedAt                time.Time `bun:"created_at,notnull"             json:"created_at"`
	UpdatedAt                time.Time `bun:"updated_at,notnull"             json:"updated_at"`
}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./internal/db/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/20260621000001_user_settings_last_seen_changelog.up.sql \
        internal/db/migrations/20260621000001_user_settings_last_seen_changelog.down.sql \
        internal/db/models/models.go
git commit -m "feat(db): add last_seen_changelog_version to user_settings"
```

---

### Task 5: `GET /api/changelog` + `GET /api/changelog/unseen` handlers

**Files:**
- Create: `internal/api/changelog.go`
- Create: `internal/api/changelog_test.go`
- Modify: `internal/api/router.go` (register the routes; ~after the version group near line 309)

**Interfaces:**
- Consumes: `changelog.All/Newer/Render` (Task 2), `updatecheck.IsValidVersion/Compare`, `models.UserSettings.LastSeenChangelogVersion` (Task 4), `auth.UserIDFromContext`, `defaultDealRegion` (from `internal/api/settings.go`).
- Produces HTTP:
  - `GET /api/changelog` → `changelogResponse`. No params → since-last (auto-marks seen). `?range=all` → full (auto-marks seen). `?since=X.Y.Z` → newer-than-given, **pure read** (no mark). Unknown/invalid `since` → `available:true, entries:[]`.
  - `GET /api/changelog/unseen` → `{ "has_unseen": bool }`. Captures the baseline (`last_seen = current`) when null; never advances past a real new release.
  - `changelogResponse`: `{ available bool, current string, last_seen string(omitempty), markdown string, entries []changelog.Entry }`.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/changelog_test.go`. Follow the existing `internal/api` test conventions (shared `testDB`, `truncateAllTables(t)` at the top, the package's helper for building an authenticated request — inspect a sibling test such as `settings` or `games` to reuse the exact request/auth helper and user-seeding helper names). The behavioural assertions to cover:

```go
// Pseudocode-level intent — wire to the package's real test helpers.
//
// setup: truncateAllTables(t); seed a user U; build handler with version "0.90.0";
// embed is unavailable in tests, so to exercise slicing inject a parsed changelog
// via a small test seam OR assert the unavailable path. Prefer asserting the
// real wiring: since All() is unavailable under test, test the HANDLER's
// unavailable + mark-seen-skip behaviour here, and cover slicing logic purely in
// internal/changelog (Task 2). Concretely:

func TestChangelog_UnavailableEmbed(t *testing.T) {
	// version "0.90.0", embed unavailable (placeholder) ->
	// GET /api/changelog returns 200 {available:false, current:"0.90.0", entries:[]}
	// and does NOT write user_settings.last_seen.
}

func TestChangelogUnseen_BaselineCapturedWhenNull(t *testing.T) {
	// last_seen null + valid current -> response has_unseen:false AND
	// user_settings.last_seen_changelog_version == current after the call.
}

func TestChangelogUnseen_DevVersionNoCapture(t *testing.T) {
	// handler built with version "dev" -> has_unseen:false, last_seen stays null.
}

func TestChangelogUnseen_HasUnseenWhenCurrentNewer(t *testing.T) {
	// pre-seed last_seen="0.17.1", current "0.90.0" -> has_unseen:true,
	// last_seen unchanged (unseen never advances).
}

func TestChangelog_SinceParamIsPureRead(t *testing.T) {
	// pre-seed last_seen="0.17.1"; GET /api/changelog?since=0.10.0 ->
	// last_seen still "0.17.1" afterwards (pure read).
}
```

Write these as real tests using the package helpers. Because `changelog.All()` is unavailable in test/lint builds (only `.gitkeep` is embedded), the **slicing/rendering** correctness is owned by Task 2's tests; these API tests verify **last-seen state transitions and the unavailable/dev degradation**, which is where the handler logic lives.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run TestChangelog -v`
Expected: FAIL — handler/routes undefined.

- [ ] **Step 3: Write the handler**

Create `internal/api/changelog.go`:

```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/changelog"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// ChangelogHandler serves the embedded release changelog, sliced per user, and
// owns the user's last_seen_changelog_version marker.
type ChangelogHandler struct {
	db      *bun.DB
	version string
}

// NewChangelogHandler constructs a ChangelogHandler. version is the running
// binary's version (the build-time ldflag), used as the "current" baseline.
func NewChangelogHandler(db *bun.DB, version string) *ChangelogHandler {
	return &ChangelogHandler{db: db, version: version}
}

type changelogResponse struct {
	Available bool              `json:"available"`
	Current   string            `json:"current"`
	LastSeen  string            `json:"last_seen,omitempty"`
	Markdown  string            `json:"markdown"`
	Entries   []changelog.Entry `json:"entries"`
}

type unseenResponse struct {
	HasUnseen bool `json:"has_unseen"`
}

// HandleGet serves GET /api/changelog. Modes: default (since-last, auto-marks
// seen), ?range=all (full, auto-marks seen), ?since=X.Y.Z (pure read).
func (h *ChangelogHandler) HandleGet(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()

	entries, ok := changelog.All()
	if !ok {
		return c.JSON(http.StatusOK, changelogResponse{Available: false, Current: h.version})
	}

	lastSeen, err := h.readLastSeen(ctx, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	var slice []changelog.Entry
	switch {
	case c.QueryParam("since") != "":
		// Pure read against an arbitrary version; invalid/unknown -> empty.
		slice = changelog.Newer(entries, c.QueryParam("since"))
	case c.QueryParam("range") == "all":
		slice = entries
		h.advanceSeen(ctx, userID, lastSeen)
	default:
		// since-last: empty (not full history) when no baseline captured yet.
		slice = changelog.Newer(entries, lastSeen)
		h.advanceSeen(ctx, userID, lastSeen)
	}

	return c.JSON(http.StatusOK, changelogResponse{
		Available: true,
		Current:   h.version,
		LastSeen:  lastSeen,
		Markdown:  changelog.Render(slice),
		Entries:   slice,
	})
}

// HandleUnseen serves GET /api/changelog/unseen — the cheap "is there anything
// new?" signal for the web dot. It captures the baseline (last_seen = current)
// the first time it sees a null marker, and never advances past a real new
// release (only HandleGet marks releases seen).
func (h *ChangelogHandler) HandleUnseen(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()

	if !updatecheck.IsValidVersion(h.version) {
		return c.JSON(http.StatusOK, unseenResponse{HasUnseen: false})
	}
	if _, ok := changelog.All(); !ok {
		return c.JSON(http.StatusOK, unseenResponse{HasUnseen: false})
	}

	lastSeen, err := h.readLastSeen(ctx, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if lastSeen == "" {
		// First authenticated encounter: capture the baseline so the user only
		// sees the indicator on the NEXT release, never a full-history blast.
		if err := h.setSeen(ctx, userID, h.version); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		return c.JSON(http.StatusOK, unseenResponse{HasUnseen: false})
	}
	return c.JSON(http.StatusOK, unseenResponse{HasUnseen: updatecheck.Compare(h.version, lastSeen) > 0})
}

func (h *ChangelogHandler) readLastSeen(ctx context.Context, userID string) (string, error) {
	var s models.UserSettings
	err := h.db.NewSelect().Model(&s).Where("user_id = ?", userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if s.LastSeenChangelogVersion == nil {
		return "", nil
	}
	return *s.LastSeenChangelogVersion, nil
}

// advanceSeen marks the running version as seen, but only moves the marker
// forward and only for valid release versions (dev builds never mark).
func (h *ChangelogHandler) advanceSeen(ctx context.Context, userID, lastSeen string) {
	if !updatecheck.IsValidVersion(h.version) {
		return
	}
	if lastSeen != "" && updatecheck.Compare(h.version, lastSeen) <= 0 {
		return
	}
	if err := h.setSeen(ctx, userID, h.version); err != nil {
		slog.ErrorContext(ctx, "advance changelog last-seen", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
}

// setSeen upserts the caller's last_seen_changelog_version, preserving any
// existing deal_region (a new row gets the deal_region default).
func (h *ChangelogHandler) setSeen(ctx context.Context, userID, version string) error {
	now := time.Now().UTC()
	s := models.UserSettings{
		UserID:                   userID,
		DealRegion:               defaultDealRegion,
		LastSeenChangelogVersion: &version,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	_, err := h.db.NewInsert().Model(&s).
		On("CONFLICT (user_id) DO UPDATE").
		Set("last_seen_changelog_version = EXCLUDED.last_seen_changelog_version").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}
```

- [ ] **Step 4: Register the routes**

In `internal/api/router.go`, right after the `versionGroup` block (the `GET ""` handler that ends ~line 309), add:

```go
		// Changelog — auth-protected. Owns parsing/slicing the embedded
		// CHANGELOG.md and the per-user last_seen marker (issue #1137).
		clh := NewChangelogHandler(db, version)
		changelogGroup := e.Group("/api/changelog", auth.AuthMiddleware(db))
		changelogGroup.GET("/unseen", clh.HandleUnseen) // static route before ""
		changelogGroup.GET("", clh.HandleGet)
```

(Register `/unseen` before `""` per the Echo v5 route-order gotcha.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/ -run TestChangelog -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/changelog.go internal/api/changelog_test.go internal/api/router.go
git commit -m "feat(api): add GET /api/changelog and /api/changelog/unseen"
```

---

### Task 6: `nexctl changelog` command + cliclient method

**Files:**
- Create: `cmd/nexctl/changelog.go`
- Modify: `cmd/nexctl/main.go` (register `newChangelogCmd()`)
- Create: `internal/cliclient/changelog.go`
- Create: `internal/cliclient/changelog_test.go`

**Interfaces:**
- Consumes: `cliclient.Client.doBearer`, `resolveProfile`, cobra patterns (mirror `cmd/nexctl/game_filters.go`).
- Produces:
  - `cliclient`: `type ChangelogEntry/ChangelogGroup` (mirror the API JSON) and `type ChangelogResult struct { Available bool; Current string; LastSeen string; Markdown string; Entries []ChangelogEntry }`; `func (c *Client) GetChangelog(key, rangeAll bool, since string) (*ChangelogResult, error)`.
  - `nexctl`: `changelog` command — default since-last; `--all` full; `--since X.Y.Z` pure read.

- [ ] **Step 1: Write the failing cliclient test**

Create `internal/cliclient/changelog_test.go` (mirror an existing client test such as `filters_test.go` for the `httptest` stub + `New(...)` setup):

```go
package cliclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetChangelog_QueryModes(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"available":true,"current":"0.90.0","markdown":"## 0.90.0\n","entries":[{"version":"0.90.0","date":"2026-06-20","groups":[{"title":"Features","items":["x"]}]}]}`))
	}))
	defer srv.Close()
	c := New(srv.URL)

	res, err := c.GetChangelog("k", false, "")
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if !res.Available || res.Current != "0.90.0" || len(res.Entries) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if gotQuery != "" {
		t.Fatalf("default mode should send no query, got %q", gotQuery)
	}

	if _, err := c.GetChangelog("k", true, ""); err != nil || gotQuery != "range=all" {
		t.Fatalf("--all query = %q err=%v", gotQuery, err)
	}
	if _, err := c.GetChangelog("k", false, "0.17.1"); err != nil || gotQuery != "since=0.17.1" {
		t.Fatalf("--since query = %q err=%v", gotQuery, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cliclient/ -run TestGetChangelog -v`
Expected: FAIL — `GetChangelog` undefined.

- [ ] **Step 3: Write the cliclient method**

Create `internal/cliclient/changelog.go`:

```go
package cliclient

import (
	"net/http"
	"net/url"
)

// ChangelogGroup is a titled list of change descriptions.
type ChangelogGroup struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

// ChangelogEntry is one released version's notes.
type ChangelogEntry struct {
	Version string           `json:"version"`
	Date    string           `json:"date"`
	Groups  []ChangelogGroup `json:"groups"`
}

// ChangelogResult is the GET /api/changelog response.
type ChangelogResult struct {
	Available bool             `json:"available"`
	Current   string           `json:"current"`
	LastSeen  string           `json:"last_seen"`
	Markdown  string           `json:"markdown"`
	Entries   []ChangelogEntry `json:"entries"`
}

// GetChangelog fetches the changelog. With rangeAll it requests the full
// history (auto-marks seen server-side); with a non-empty since it requests a
// pure read of entries newer than that version; otherwise it requests the
// since-last diff (auto-marks seen). rangeAll and since are mutually exclusive;
// since takes precedence if both are set.
func (c *Client) GetChangelog(key string, rangeAll bool, since string) (*ChangelogResult, error) {
	q := url.Values{}
	switch {
	case since != "":
		q.Set("since", since)
	case rangeAll:
		q.Set("range", "all")
	}
	path := "/api/changelog"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out ChangelogResult
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
```

- [ ] **Step 4: Run the cliclient test**

Run: `go test ./internal/cliclient/ -run TestGetChangelog -v`
Expected: PASS.

- [ ] **Step 5: Write the cobra command**

Create `cmd/nexctl/changelog.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func newChangelogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changelog",
		Short: "Show release changelog entries",
		Long: "Show the changelog. With no flags, shows releases newer than the\n" +
			"version you last viewed (and marks them seen). --all shows the full\n" +
			"history (also marks seen). --since X.Y.Z shows entries newer than the\n" +
			"given version as a pure read (does not mark anything seen).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			all := flagBool(cmd, "all")
			since, _ := cmd.Flags().GetString("since")
			if all && since != "" {
				return fmt.Errorf("--all and --since are mutually exclusive")
			}

			c := cliclient.New(p.URL)
			res, err := c.GetChangelog(p.Key, all, since)
			if err != nil {
				return fmt.Errorf("get changelog failed: %w", err)
			}
			if !res.Available {
				fmt.Fprintln(out, "Changelog unavailable for this build.")
				return nil
			}
			if len(res.Entries) == 0 {
				fmt.Fprintln(out, "No new changelog entries.")
				return nil
			}
			for _, e := range res.Entries {
				if e.Date != "" {
					fmt.Fprintf(out, "## %s — %s\n\n", e.Version, e.Date)
				} else {
					fmt.Fprintf(out, "## %s\n\n", e.Version)
				}
				for _, g := range e.Groups {
					fmt.Fprintf(out, "### %s\n", g.Title)
					for _, it := range g.Items {
						fmt.Fprintf(out, "  - %s\n", it)
					}
					fmt.Fprintln(out)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "Show the full changelog history (marks seen)")
	cmd.Flags().String("since", "", "Show entries newer than this version (pure read; e.g. 0.17.1)")
	return cmd
}
```

(Verify `flagBool` exists in `cmd/nexctl`; it is used by `game_filters.go`. If the helper signature differs, use `cmd.Flags().GetBool("all")` directly.)

- [ ] **Step 6: Register the command**

In `cmd/nexctl/main.go`, alongside the other `root.AddCommand(...)` calls, add:

```go
	root.AddCommand(newChangelogCmd())
```

- [ ] **Step 7: Verify it builds and the command is wired**

Run:
```bash
go build ./cmd/nexctl
./nexctl changelog --help
```
Expected: build succeeds; help text lists `--all` and `--since`.

- [ ] **Step 8: Commit**

```bash
git add cmd/nexctl/changelog.go cmd/nexctl/main.go internal/cliclient/changelog.go internal/cliclient/changelog_test.go
git commit -m "feat(nexctl): add changelog command"
```

---

### Task 7: Web — `useChangelog` hooks + API client

**Files:**
- Create: `ui/frontend/src/api/changelog.ts`
- Create: `ui/frontend/src/hooks/use-changelog.ts`
- Modify: `ui/frontend/src/hooks/index.ts` (re-export)

**Interfaces:**
- Consumes: `apiCall` (`@/api/client`), TanStack Query.
- Produces:
  - `changelogApi.unseen(): Promise<{ has_unseen: boolean }>`
  - `changelogApi.get(params?: { all?: boolean; since?: string }): Promise<ChangelogResult>`
  - `useChangelogUnseen()` query (`['changelog','unseen']`).
  - `useChangelogContent(params)` query (enabled-gated; used by the dialog).
  - Type `ChangelogResult` (mirrors the API).

- [ ] **Step 1: Write the API client**

Create `ui/frontend/src/api/changelog.ts`:

```ts
import { apiCall } from './client';

export interface ChangelogGroup {
  title: string;
  items: string[];
}

export interface ChangelogEntry {
  version: string;
  date: string;
  groups: ChangelogGroup[];
}

export interface ChangelogResult {
  available: boolean;
  current: string;
  last_seen?: string;
  markdown: string;
  entries: ChangelogEntry[];
}

// Paths are relative to config.apiUrl (which already includes "/api"); do NOT
// prepend "/api" here.
export const changelogApi = {
  unseen: (): Promise<{ has_unseen: boolean }> =>
    apiCall('/changelog/unseen').then((r) => r.json()),
  get: (params?: { all?: boolean; since?: string }): Promise<ChangelogResult> => {
    const qs = new URLSearchParams();
    if (params?.since) qs.set('since', params.since);
    else if (params?.all) qs.set('range', 'all');
    const suffix = qs.toString() ? `?${qs.toString()}` : '';
    return apiCall(`/changelog${suffix}`).then((r) => r.json());
  },
};
```

(Confirm `apiCall` returns a `Response`; `api/docs.ts` calls `.then((r) => r.text())`, so `.json()` is the correct sibling usage.)

- [ ] **Step 2: Write the hooks**

Create `ui/frontend/src/hooks/use-changelog.ts`:

```ts
import { useQuery } from '@tanstack/react-query';
import { changelogApi } from '@/api/changelog';

export const changelogKeys = {
  all: ['changelog'] as const,
  unseen: () => [...changelogKeys.all, 'unseen'] as const,
  content: (mode: string) => [...changelogKeys.all, 'content', mode] as const,
};

// Cheap "is there anything new?" signal for the sidebar dot. Captures the
// per-user baseline server-side on first call.
export function useChangelogUnseen() {
  return useQuery({
    queryKey: changelogKeys.unseen(),
    queryFn: changelogApi.unseen,
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
}

// Changelog content for the dialog. Fetching the default/all modes marks the
// releases seen server-side, so this is gated behind `enabled` (only runs when
// the dialog opens).
export function useChangelogContent(
  params: { all?: boolean; since?: string },
  enabled: boolean,
) {
  const mode = params.since ? `since:${params.since}` : params.all ? 'all' : 'since-last';
  return useQuery({
    queryKey: changelogKeys.content(mode),
    queryFn: () => changelogApi.get(params),
    enabled,
    staleTime: 0,
    retry: false,
  });
}
```

- [ ] **Step 3: Re-export from the hooks barrel**

In `ui/frontend/src/hooks/index.ts`, next to the other exports, add:

```ts
export { useChangelogUnseen, useChangelogContent, changelogKeys } from './use-changelog';
```

- [ ] **Step 4: Verify types**

Run: `cd ui/frontend && npm run check`
Expected: no TypeScript errors. (knip may flag `useChangelogContent`/`changelogKeys` as unused until Task 8 consumes them — defer the knip run to the end of Task 8.)

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/api/changelog.ts ui/frontend/src/hooks/use-changelog.ts ui/frontend/src/hooks/index.ts
git commit -m "feat(ui): add changelog API client and query hooks"
```

---

### Task 8: Web — "What's new" indicator + dialog

**Files:**
- Create: `ui/frontend/src/components/navigation/whats-new.tsx`
- Modify: `ui/frontend/src/components/navigation/version-footer.tsx` (render the indicator)
- Create: `ui/frontend/src/lib/version-compare.ts` (tiny client-side semver gt)
- Create: `ui/frontend/src/lib/version-compare.test.ts`
- Create: `ui/frontend/src/components/navigation/whats-new.test.tsx`

**Interfaces:**
- Consumes: `useChangelogUnseen`, `useChangelogContent`, `changelogKeys` (Task 7); shadcn `Dialog` (`@/components/ui/dialog`); lazy `MarkdownDoc`; `useQueryClient`.
- Produces: `WhatsNew` component (the dot + dialog), rendered by `VersionFooter`.

- [ ] **Step 1: Write the failing version-compare test**

Create `ui/frontend/src/lib/version-compare.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { isValidRelease, isNewer } from './version-compare';

describe('version-compare', () => {
  it('validates release versions', () => {
    expect(isValidRelease('0.90.0')).toBe(true);
    expect(isValidRelease('v0.90.0')).toBe(true);
    expect(isValidRelease('dev')).toBe(false);
    expect(isValidRelease('main-20260621-abc1234')).toBe(false);
    expect(isValidRelease('')).toBe(false);
  });

  it('compares newer-than', () => {
    expect(isNewer('0.90.0', '0.17.1')).toBe(true);
    expect(isNewer('0.17.1', '0.90.0')).toBe(false);
    expect(isNewer('0.90.0', '0.90.0')).toBe(false);
    expect(isNewer('0.90.0', 'dev')).toBe(false); // invalid baseline -> not newer
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd ui/frontend && npm run test version-compare`
Expected: FAIL — module not found.

- [ ] **Step 3: Write version-compare**

Create `ui/frontend/src/lib/version-compare.ts`:

```ts
// Minimal X.Y.Z semver helpers for the "What's new" indicator. The server is
// the source of truth for slicing; this only gates the sidebar dot client-side.
const RELEASE_RE = /^v?(\d+)\.(\d+)\.(\d+)$/;

function parse(v: string): [number, number, number] | null {
  const m = RELEASE_RE.exec(v.trim());
  if (!m) return null;
  return [Number(m[1]), Number(m[2]), Number(m[3])];
}

export function isValidRelease(v: string | undefined | null): boolean {
  return !!v && parse(v) !== null;
}

// isNewer reports whether `current` is a strictly newer release than `other`.
// Returns false if either side is not a valid X.Y.Z release.
export function isNewer(current: string, other: string): boolean {
  const a = parse(current);
  const b = parse(other);
  if (!a || !b) return false;
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] > b[i];
  }
  return false;
}
```

- [ ] **Step 4: Run version-compare test to verify it passes**

Run: `cd ui/frontend && npm run test version-compare`
Expected: PASS.

- [ ] **Step 5: Write the WhatsNew component**

Create `ui/frontend/src/components/navigation/whats-new.tsx`:

```tsx
import { lazy, Suspense, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Sparkles } from 'lucide-react';

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { changelogKeys, useChangelogContent, useChangelogUnseen } from '@/hooks';

const MarkdownDoc = lazy(() =>
  import('@/components/docs/markdown-doc').then((m) => ({ default: m.MarkdownDoc })),
);

export function WhatsNew() {
  const queryClient = useQueryClient();
  const { data: unseen } = useChangelogUnseen();
  const [open, setOpen] = useState(false);
  const [showAll, setShowAll] = useState(false);

  // Content fetch is gated on the dialog being open; fetching marks releases
  // seen server-side, so we clear the dot after a successful open.
  const { data, isLoading } = useChangelogContent({ all: showAll }, open);

  const hasUnseen = unseen?.has_unseen === true;

  function openDialog() {
    setShowAll(false);
    setOpen(true);
    // After the since-last fetch marks things seen, refresh the dot.
    void queryClient.invalidateQueries({ queryKey: changelogKeys.unseen() });
  }

  return (
    <>
      <button
        type="button"
        onClick={openDialog}
        className="inline-flex items-center gap-1 underline hover:text-foreground"
      >
        What&apos;s new
        {hasUnseen && (
          <span
            aria-label="new changelog entries"
            className="ml-0.5 inline-block h-2 w-2 rounded-full bg-primary"
          />
        )}
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Sparkles className="h-4 w-4" />
              {showAll ? 'Full changelog' : "What's new"}
            </DialogTitle>
          </DialogHeader>

          {isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-5 w-48" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-5/6" />
            </div>
          ) : !data?.available ? (
            <p className="text-sm text-muted-foreground">
              The changelog is unavailable for this build.
            </p>
          ) : data.entries.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              You&apos;re up to date — nothing new since you last looked.
            </p>
          ) : (
            <Suspense fallback={<Skeleton className="h-24 w-full" />}>
              <MarkdownDoc slug="changelog" markdown={data.markdown} />
            </Suspense>
          )}

          {!showAll && (
            <div className="pt-2">
              <Button variant="ghost" size="sm" onClick={() => setShowAll(true)}>
                View full changelog
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
```

(Verify `@/components/ui/button` and `@/components/ui/dialog` exports `Dialog`, `DialogContent`, `DialogHeader`, `DialogTitle`, `Button`. `dialog.tsx` exists per scouting; if `Button` is absent, add it via `npx shadcn@latest add button`.)

- [ ] **Step 6: Render WhatsNew in the footer**

In `ui/frontend/src/components/navigation/version-footer.tsx`, change the body to include the indicator beside the GitHub link:

```tsx
import { ExternalLink } from 'lucide-react';

import { useVersion } from '@/hooks';
import { GITHUB_REPO_URL } from '@/lib/repo';
import { isValidRelease } from '@/lib/version-compare';
import { WhatsNew } from './whats-new';

export function VersionFooter() {
  const { data: versionInfo } = useVersion();

  if (!versionInfo?.version) return null;

  return (
    <div className="px-4 pb-3 text-xs text-muted-foreground">
      <div>Version: {versionInfo.version}</div>
      <div className="flex items-center gap-3">
        <a
          href={GITHUB_REPO_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 underline hover:text-foreground"
        >
          GitHub
          <ExternalLink className="h-3 w-3" />
        </a>
        {isValidRelease(versionInfo.version) && <WhatsNew />}
      </div>
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

- [ ] **Step 7: Write a component test for the dot**

Create `ui/frontend/src/components/navigation/whats-new.test.tsx`. Mirror an existing component test in the repo for the render harness (QueryClientProvider wrapper). Assert:
- when `useChangelogUnseen` resolves `{ has_unseen: true }`, the dot (`aria-label="new changelog entries"`) is present;
- when `{ has_unseen: false }`, the dot is absent.

Mock the hooks module (`vi.mock('@/hooks', ...)`) so the test is deterministic and does not hit the network. Follow the mocking style used by a sibling `*.test.tsx` in `src/components` or `src/hooks`.

- [ ] **Step 8: Run the frontend gates**

Run:
```bash
cd ui/frontend
npm run test version-compare
npm run test whats-new
npm run check
npm run knip
npm run build   # regenerates routeTree.gen.ts if needed (no new routes here, but safe)
```
Expected: all pass; knip reports no unused exports (Task 7's hooks are now consumed). Commit `routeTree.gen.ts` only if it changed.

- [ ] **Step 9: Commit**

```bash
git add ui/frontend/src/components/navigation/whats-new.tsx \
        ui/frontend/src/components/navigation/whats-new.test.tsx \
        ui/frontend/src/components/navigation/version-footer.tsx \
        ui/frontend/src/lib/version-compare.ts \
        ui/frontend/src/lib/version-compare.test.ts
git commit -m "feat(ui): add 'What's new' changelog indicator and dialog"
```

---

### Task 9: Full-suite verification, deadcode, and PR

**Files:** none (verification + integration).

- [ ] **Step 1: Backend build + targeted tests**

Run:
```bash
go build ./...
go test ./internal/services/updatecheck/ ./internal/changelog/ ./internal/api/ -run 'Compare|Changelog|Parse|Newer|Render' -v
go test ./internal/cliclient/ -run TestGetChangelog -v
```
Expected: all PASS.

- [ ] **Step 2: Dead-code check**

This change adds an exported `updatecheck.Compare` and several exported `changelog`/`cliclient` symbols, all consumed in-repo. Run:
```bash
make deadcode
```
Reconcile any *new* entries against the diff. Reflection/embed-only references aside, expect no new orphans (everything added is called by a handler, command, or test).

- [ ] **Step 3: Frontend gates**

Run:
```bash
cd ui/frontend && npm run check && npm run knip && npm run test
```
Expected: all PASS.

- [ ] **Step 4: Manual smoke (optional but recommended)**

With a built binary (`make` so the changelog is embedded) and a dev DB:
```bash
make && ./nexorious serve   # in one shell
# In another: confirm the running version is a release in CHANGELOG.md, or the
# dot stays hidden (expected for a dev/branch version).
./nexctl changelog --all     # prints the full embedded history
./nexctl changelog --since 0.1.0
```
Confirm: `nexctl changelog` (default) prints "No new changelog entries" right after `--all` marked everything seen; the web "What's new" link opens the dialog.

- [ ] **Step 5: Push and open the PR**

```bash
git push -u origin <branch>
gh pr create --title "feat: in-app changelog (\"What's new\") with per-user since-last-seen diff" \
  --body "$(cat <<'EOF'
Implements the in-app changelog feature.

- Embeds CHANGELOG.md into the server binary via a committed-placeholder dir in `internal/changelog`; the real copy is wired into `make`, `release-artifacts.yaml`, and `nix/package.nix` only. Absent content degrades gracefully.
- `GET /api/changelog` (since-last default, `?range=all`, `?since=X.Y.Z`) + `GET /api/changelog/unseen`; slicing reuses `updatecheck` semver. Default/all auto-advance `last_seen`; `?since=` is a pure read.
- `last_seen_changelog_version` added to `user_settings` (migration + struct + handlers). Baseline captured at runtime on first authenticated encounter (covers new signups), so nobody is blasted with full history.
- Web: quiet "What's new" indicator + dialog rendering the diff with dev-noise stripped, plus "view full changelog".
- `nexctl changelog` (default / `--all` / `--since X.Y.Z`).

Closes #1137
EOF
)"
```

---

## Self-Review

**Spec coverage** (acceptance criteria → task):
- Embedded via committed-placeholder dir; copy wired into make/release/nix only; graceful degradation → **Tasks 2, 3**.
- `GET /api/changelog` with all three modes; structured entries; `updatecheck` slicing → **Tasks 2, 5**.
- since-last & range=all auto-advance; `?since=` pure read; unseen check doesn't mark → **Task 5** (HandleGet modes + HandleUnseen).
- `last_seen_changelog_version` column (migration + struct + handlers) → **Tasks 4, 5**.
- Baseline at runtime (first encounter when null; covers signup) → **Task 5** (`HandleUnseen` capture + `advanceSeen`). *Deviation from the issue:* the issue lists "first authenticated encounter **and** at signup" as two hooks; a new user's first `unseen` call captures their baseline, so the lazy path subsumes signup — no separate signup hook is added. Net effect (existing + new users baseline at current, indicator first appears on the next release) is identical to the spec.
- Web indicator + panel with dev-noise stripped + "view full changelog" → **Tasks 7, 8**.
- `nexctl changelog` default/`--all`/`--since` → **Task 6**.
- Non-release/unknown versions and `last_seen` > current degrade gracefully → **Tasks 2** (`Newer` empty on invalid since; `Compare` invalid handling), **5** (`advanceSeen`/`HandleUnseen` dev-version guards), **8** (`isValidRelease` gates the indicator).
- Parsing/slicing edge-case tests → **Task 2** (`TestParse/TestNewer/TestRender`) + **Task 5** (state-transition + dev/unavailable tests).

**Out of scope (honored):** no curated notes, no runtime GitHub fetch, single last-seen marker only. MCP tool surface is unchanged (the issue's MCP mirror is game/pool/tag/sync; changelog is intentionally not added).

**Type consistency:** `changelog.Entry/Group` (Go) ↔ `cliclient.ChangelogEntry/Group` ↔ `ChangelogEntry/ChangelogGroup` (TS) all use `version/date/groups` and `title/items`. `changelogResponse` fields (`available/current/last_seen/markdown/entries`) match `cliclient.ChangelogResult` and the TS `ChangelogResult`. `updatecheck.Compare` signature `(a,b string) int` is used consistently by `changelog.Newer` and the API handler.

**Placeholder scan:** Task 5 Step 1 and Task 8 Step 7 describe tests by intent and point at sibling tests for the exact harness/helpers (the repo's `internal/api` auth/seed helpers and frontend QueryClient wrapper are not quoted verbatim because their names must be read from the codebase at execution time); every code-producing step ships complete code. No TBD/TODO remain.
