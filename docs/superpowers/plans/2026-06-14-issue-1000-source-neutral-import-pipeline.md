# Source-neutral import pipeline (#1000) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract a source-neutral import pipeline from the Darkadia importer so a new migration source needs only a mapper + one line of registration, with zero behaviour change for Darkadia.

**Architecture:** A leaf `importmodel` package holds the canonical `Game`/`Platform` and a shared signature sentinel. Mapper packages (`darkadia`) parse a source file into `[]importmodel.Game`. An `importsource` registry maps a slug to `{Mapper, display metadata}`. The match/finalize/completion workers, the upload handler, and the source-routing helpers become source-neutral and registry-keyed; the frontend picker is driven by a new `GET /api/import/sources` endpoint.

**Tech Stack:** Go 1.26, Bun ORM, River job queue, Echo v5, Vite/React 19 + TanStack Query, Vitest.

**Refactor note on TDD:** Tasks 1–2 are pure renames/moves with **zero** behaviour change — the existing test suites are the safety net, so each step's "test" is *run the existing tests and confirm still green*, not write-a-failing-test-first. Tasks 3–6 add new behaviour and use real TDD (failing test first).

---

## File Structure

**Created:**
- `internal/services/importmodel/model.go` — canonical `Game`, `Platform`, `ErrInvalidSignature` (leaf; no deps).
- `internal/services/importsource/registry.go` — `Mapper` interface, `Source` struct, registry table, `Lookup`/`All`/`IsRegistered`.
- `internal/services/importsource/registry_test.go` — registry unit tests.
- `internal/api/import_sources.go` — `GET /api/import/sources` handler.

**Renamed:**
- `internal/worker/tasks/darkadia.go` → `internal/worker/tasks/import_pipeline.go`.
- `internal/worker/tasks/darkadia_test.go` → `internal/worker/tasks/import_pipeline_test.go`.

**Modified (Go):**
- `internal/services/darkadia/darkadia.go` — `Parse` returns `[]importmodel.Game`; `consolidate` builds `importmodel.*`; `ErrInvalidHeader` wraps `importmodel.ErrInvalidSignature`.
- `internal/worker/tasks/import_pipeline.go` — type/kind renames; unmarshal `importmodel.Game`; drop `darkadia` import.
- `internal/worker/tasks/enqueue.go` — `ArgsForJobType`/`FinalizeArgsForSource` return renamed args and key on `importsource.IsRegistered`.
- `internal/api/import.go` — generic `handleImportSource(src)`; remove `HandleImportDarkadia`.
- `internal/api/router.go` — register import routes in a loop over the registry; add the sources route.
- `cmd/nexorious/serve.go` — register the renamed workers (two sites).

**Modified (Frontend):**
- `ui/frontend/src/api/import-export.ts` — `fetchImportSources()` + generic `importFromSource(slug, file)`; remove `importDarkadiaCsv`.
- `ui/frontend/src/types/import-export.ts` — remove `ImportSource` enum + `getImportSourceDisplayInfo`; add `ImportSourceInfo`.
- `ui/frontend/src/hooks/use-import-export.ts` — `useImportSources()` query + generic `useImportSource()`; remove `useImportDarkadia`.
- `ui/frontend/src/hooks/index.ts` — export updates.
- `ui/frontend/src/routes/_authenticated/import-export.tsx` — render cards from the fetched registry; gate review box on registry membership.
- `ui/frontend/src/routes/_authenticated/import-export.test.tsx` — drive from fetched sources.
- `ui/frontend/src/components/navigation/nav-items.tsx` — sum review counts across import sources.

---

## Task 1: Extract the canonical `importmodel` package

**Files:**
- Create: `internal/services/importmodel/model.go`
- Modify: `internal/services/darkadia/darkadia.go`
- Modify: `internal/worker/tasks/darkadia.go` (the finalize payload type + imports)
- Test: existing `internal/services/darkadia/darkadia_test.go`, `internal/worker/tasks/darkadia_test.go` (unchanged — safety net)

- [ ] **Step 1: Create the canonical model package**

Create `internal/services/importmodel/model.go`:

```go
// Package importmodel defines the canonical, source-neutral game shape that
// every import mapper produces and the import pipeline consumes. It is a leaf
// package (no internal dependencies) so mappers and the upload handler can both
// reference it without an import cycle.
package importmodel

import "errors"

// ErrInvalidSignature is the shared sentinel a mapper returns (wrapped) when a
// file is the wrong shape for that source. The generic upload handler turns
// errors.Is(err, ErrInvalidSignature) into a 400 "not a <source> export".
var ErrInvalidSignature = errors.New("file does not match the expected source format")

// Game is the consolidated, Nexorious-shaped payload for one imported game. It
// is marshalled verbatim into job_item.source_metadata.
type Game struct {
	Title          string     `json:"title"`
	PlayStatus     string     `json:"play_status"`
	IsLoved        bool       `json:"is_loved"`
	PersonalRating *int32     `json:"personal_rating,omitempty"`
	PersonalNotes  *string    `json:"personal_notes,omitempty"`
	CreatedAt      string     `json:"created_at,omitempty"` // "2006-01-02" or ""
	Platforms      []Platform `json:"platforms"`
	Tags           []string   `json:"tags,omitempty"`
	HoursPlayed    *float64   `json:"hours_played,omitempty"`
}

// Platform is one consolidated (platform, storefront, acquired_date) ownership entry.
type Platform struct {
	Platform     string  `json:"platform"`                // Nexorious slug
	Storefront   *string `json:"storefront,omitempty"`    // slug or nil
	AcquiredDate string  `json:"acquired_date,omitempty"` // "2006-01-02" or ""
}
```

- [ ] **Step 2: Point `darkadia` at the canonical type**

In `internal/services/darkadia/darkadia.go`:

1. Add the import: in the existing `import (...)` block add `"github.com/drzero42/nexorious/internal/services/importmodel"`.
2. **Delete** the local `type Game struct {...}` and `type Platform struct {...}` blocks (lines 64–83) — they now live in `importmodel`.
3. Update the type references (the `rawGame` struct is unaffected; only the canonical types move):
   - `func Parse(raw []byte) ([]Game, error)` → `([]importmodel.Game, error)`
   - `games := make([]Game, 0, len(raws))` → `make([]importmodel.Game, 0, len(raws))`
   - `func consolidate(rg rawGame) Game {` → `) importmodel.Game {`
   - `g := Game{` → `g := importmodel.Game{`
   - `g.Platforms = append(g.Platforms, Platform{...})` → `importmodel.Platform{...}`

Exact scoped replacement:

```bash
cd internal/services/darkadia
# delete the two moved type blocks by hand (lines ~64-83), then:
sed -i \
  -e 's/(\[\]Game, error)/([]importmodel.Game, error)/' \
  -e 's/make(\[\]Game,/make([]importmodel.Game,/' \
  -e 's/rg rawGame) Game {/rg rawGame) importmodel.Game {/' \
  -e 's/g := Game{/g := importmodel.Game{/' \
  -e 's/append(g.Platforms, Platform{/append(g.Platforms, importmodel.Platform{/' \
  darkadia.go
cd -
```

- [ ] **Step 3: Point the finalize worker at the canonical type**

In `internal/worker/tasks/darkadia.go`:
- Replace import `"github.com/drzero42/nexorious/internal/services/darkadia"` with `"github.com/drzero42/nexorious/internal/services/importmodel"` (the worker only used `darkadia` for the `Game` type).
- Change `var payload darkadia.Game` (line ~144) to `var payload importmodel.Game`.

- [ ] **Step 4: Build and run the affected suites — expect green**

Run:
```bash
go build ./...
go test ./internal/services/darkadia/... ./internal/worker/tasks/... -count=1
```
Expected: build succeeds; all tests PASS. (`darkadia_test.go` never names the moved type — it only does field access on `Parse` results — so it compiles unchanged. The worker test still references `DarkadiaFinalizeArgs` etc., which are untouched in this task.)

- [ ] **Step 5: Commit**

```bash
git add internal/services/importmodel/model.go internal/services/darkadia/darkadia.go internal/worker/tasks/darkadia.go
git commit -m "refactor: extract canonical importmodel.Game from darkadia (#1000)"
```

---

## Task 2: Rename the pipeline to source-neutral identifiers

This is a scoped, mechanical rename. The tokens below are unique CamelCase/underscore identifiers — they will **not** collide with the `darkadia` package name, the `JobSourceDarkadia` constant, or the SQL literal `'darkadia'` (which has no `_match`/`_finalize` suffix).

**Files:**
- Rename: `internal/worker/tasks/darkadia.go` → `import_pipeline.go`; `internal/worker/tasks/darkadia_test.go` → `import_pipeline_test.go`
- Modify: `internal/worker/tasks/enqueue.go`, `internal/api/import.go`, `cmd/nexorious/serve.go`

- [ ] **Step 1: Rename the worker source + test files**

```bash
git mv internal/worker/tasks/darkadia.go internal/worker/tasks/import_pipeline.go
git mv internal/worker/tasks/darkadia_test.go internal/worker/tasks/import_pipeline_test.go
```

- [ ] **Step 2: Apply the scoped identifier rename across the package + callers**

```bash
cd /home/abo/workspace/home/nexorious
FILES="internal/worker/tasks/import_pipeline.go internal/worker/tasks/import_pipeline_test.go internal/worker/tasks/enqueue.go internal/api/import.go cmd/nexorious/serve.go"
sed -i \
  -e 's/DarkadiaMatchArgs/ImportMatchArgs/g' \
  -e 's/DarkadiaMatchWorker/ImportMatchWorker/g' \
  -e 's/DarkadiaFinalizeArgs/ImportFinalizeArgs/g' \
  -e 's/DarkadiaFinalizeWorker/ImportFinalizeWorker/g' \
  -e 's/DarkadiaCheckJobCompletion/ImportCheckJobCompletion/g' \
  -e 's/darkadiaMarkPendingReview/importMarkPendingReview/g' \
  -e 's/darkadia_match/import_match/g' \
  -e 's/darkadia_finalize/import_finalize/g' \
  -e 's/darkadiaMatchWorker/importMatchWorker/g' \
  -e 's/newDarkadiaMatch/newImportMatch/g' \
  -e 's/TestDarkadiaMatch/TestImportMatch/g' \
  -e 's/TestDarkadiaFinalize/TestImportFinalize/g' \
  -e 's/TestDarkadiaCheckJobCompletion/TestImportCheckJobCompletion/g' \
  -e 's/insertDarkadiaItem/insertImportItem/g' \
  $FILES
```

This also rewrites the log-message prefixes `"darkadia_match:"`/`"darkadia_finalize:"` → `"import_match:"`/`"import_finalize:"` (they contain the renamed kind tokens). For the bare `"darkadia: ..."` count-helper log prefixes inside `ImportCheckJobCompletion`, rename them by hand to `"import: ..."` in `import_pipeline.go` (search for `"darkadia: count`, `"darkadia: finalize`).

- [ ] **Step 3: Verify no stale identifiers remain and the SQL literal is untouched**

```bash
grep -rn "Darkadia\(Match\|Finalize\|CheckJobCompletion\)\|darkadia_match\|darkadia_finalize" internal/ cmd/ --include=*.go
# Expected: no matches.
grep -rn "source, status" internal/worker/tasks/import_pipeline_test.go | grep "'darkadia'"
# Expected: still present — the job-source SQL literal 'darkadia' must NOT have been renamed.
```

- [ ] **Step 4: Build and run — expect green**

Run:
```bash
go build ./...
go test ./internal/worker/tasks/... ./internal/api/... -count=1
```
Expected: build succeeds; tests PASS. (`enqueue.go` still routes on the `JobSourceDarkadia` literal — only the returned arg *types* were renamed. Registry-driven routing comes in Task 4. `TestFinalizeArgsForSource` still asserts `ImportFinalizeArgs` for the darkadia source and compiles via the rename.)

- [ ] **Step 5: Commit**

```bash
git add -A internal/worker/tasks/ internal/api/import.go internal/worker/tasks/enqueue.go cmd/nexorious/serve.go
git commit -m "refactor: rename darkadia import workers to source-neutral import_* (#1000)"
```

---

## Task 3: Add the `importsource` registry

**Files:**
- Create: `internal/services/importsource/registry.go`
- Create: `internal/services/importsource/registry_test.go`
- Modify: `internal/services/darkadia/darkadia.go` (sentinel wrap)

- [ ] **Step 1: Make the Darkadia signature error wrap the shared sentinel**

In `internal/services/darkadia/darkadia.go`, replace the `ErrInvalidHeader` definition:

```go
// ErrInvalidHeader signals the file is not a Darkadia export. It wraps the
// shared importmodel.ErrInvalidSignature so the generic upload handler can
// detect a wrong-file upload without knowing the per-source sentinel.
var ErrInvalidHeader = fmt.Errorf("not a Darkadia export (header mismatch): %w", importmodel.ErrInvalidSignature)
```

(`fmt` and `importmodel` are already imported.) The existing `TestParse_RejectsNonDarkadiaHeader` (`errors.Is(err, darkadia.ErrInvalidHeader)`) still passes because `errors.Is` matches the sentinel itself.

- [ ] **Step 2: Write the failing registry test**

Create `internal/services/importsource/registry_test.go`:

```go
package importsource_test

import (
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/importmodel"
	"github.com/drzero42/nexorious/internal/services/importsource"
)

func TestLookup_Darkadia(t *testing.T) {
	src, ok := importsource.Lookup(models.JobSourceDarkadia)
	if !ok {
		t.Fatal("darkadia not registered")
	}
	if src.DisplayName != "Darkadia" {
		t.Errorf("DisplayName = %q, want Darkadia", src.DisplayName)
	}
	if src.Mapper == nil {
		t.Error("Mapper is nil")
	}
}

func TestLookup_Unknown(t *testing.T) {
	if _, ok := importsource.Lookup("nope"); ok {
		t.Error("unknown slug reported as registered")
	}
}

func TestIsRegistered(t *testing.T) {
	if !importsource.IsRegistered(models.JobSourceDarkadia) {
		t.Error("darkadia should be registered")
	}
	if importsource.IsRegistered(models.JobSourceNexorious) {
		t.Error("nexorious is not a mapper-based migration source")
	}
}

func TestAll_IncludesDarkadia(t *testing.T) {
	found := false
	for _, s := range importsource.All() {
		if s.Slug == models.JobSourceDarkadia {
			found = true
		}
	}
	if !found {
		t.Error("All() omits darkadia")
	}
}

func TestDarkadiaMapper_RejectsWrongFile(t *testing.T) {
	src, _ := importsource.Lookup(models.JobSourceDarkadia)
	_, err := src.Mapper.Parse([]byte("not,a,darkadia,file\n1,2,3,4\n"))
	if !errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Errorf("err = %v, want wrapping ErrInvalidSignature", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/services/importsource/... -count=1`
Expected: FAIL — package `importsource` does not exist yet.

- [ ] **Step 4: Write the registry**

Create `internal/services/importsource/registry.go`:

```go
// Package importsource is the registry of mapper-based migration import sources.
// Each entry maps a job-source slug to its file mapper plus the display metadata
// the upload handler and the frontend source picker are driven by. Adding a
// source means adding one entry here.
package importsource

import (
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/darkadia"
	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// Mapper parses a source export file into canonical games. On a wrong-shape
// file it returns an error wrapping importmodel.ErrInvalidSignature.
type Mapper interface {
	Parse(raw []byte) ([]importmodel.Game, error)
}

// mapperFunc adapts a plain parse function to the Mapper interface, so a mapper
// package need not import importsource to be registered.
type mapperFunc func(raw []byte) ([]importmodel.Game, error)

func (f mapperFunc) Parse(raw []byte) ([]importmodel.Game, error) { return f(raw) }

// Source is one registered import source.
type Source struct {
	Slug        string   `json:"slug"`         // JobSource* value, e.g. "darkadia"
	DisplayName string   `json:"display_name"` // "Darkadia"
	Description string   `json:"description"`  // picker blurb
	Features    []string `json:"features"`     // picker bullet list
	Accept      []string `json:"accept"`       // file-input accept hints
	Mapper      Mapper   `json:"-"`
}

// registry is the ordered list of sources (stable order drives the picker).
var registry = []Source{
	{
		Slug:        models.JobSourceDarkadia,
		DisplayName: "Darkadia",
		Description: "Migrate a Darkadia collection export. Games are matched to IGDB; ambiguous matches go to review. Requires IGDB to be configured.",
		Features: []string{
			"Preserves ratings, notes & added date",
			"Matches games to IGDB",
			"Interactive review",
		},
		Accept: []string{".csv", "text/csv"},
		Mapper: mapperFunc(darkadia.Parse),
	},
}

// Lookup returns the source for a slug.
func Lookup(slug string) (Source, bool) {
	for _, s := range registry {
		if s.Slug == slug {
			return s, true
		}
	}
	return Source{}, false
}

// All returns every registered source in stable order.
func All() []Source {
	out := make([]Source, len(registry))
	copy(out, registry)
	return out
}

// IsRegistered reports whether slug is a mapper-based import source.
func IsRegistered(slug string) bool {
	_, ok := Lookup(slug)
	return ok
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/services/importsource/... ./internal/services/darkadia/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/services/importsource/ internal/services/darkadia/darkadia.go
git commit -m "feat: add importsource registry with darkadia entry (#1000)"
```

---

## Task 4: Registry-driven routing and a generic upload handler

**Files:**
- Modify: `internal/worker/tasks/enqueue.go` (route on `importsource.IsRegistered`)
- Modify: `internal/worker/tasks/import_pipeline_test.go` (`TestFinalizeArgsForSource`)
- Modify: `internal/api/import.go` (generic handler)
- Modify: `internal/api/router.go` (loop registration)
- Test: `internal/api/import_test.go` (new cases)

- [ ] **Step 1: Generalise the source-routing helpers**

In `internal/worker/tasks/enqueue.go`:
- Add import `"github.com/drzero42/nexorious/internal/services/importsource"`.
- In `ArgsForJobType`, replace `if source == models.JobSourceDarkadia {` with `if importsource.IsRegistered(source) {` (keep the body returning `ImportMatchArgs{JobItemID: jobItemID}`).
- Replace the body of `FinalizeArgsForSource`:

```go
func FinalizeArgsForSource(source, jobItemID string) (river.JobArgs, error) {
	if importsource.IsRegistered(source) {
		return ImportFinalizeArgs{JobItemID: jobItemID}, nil
	}
	return nil, fmt.Errorf("source %q has no interactive finalize stage", source)
}
```

No cycle: `importsource` does not import `tasks`.

- [ ] **Step 2: Update `TestFinalizeArgsForSource` for registry routing**

In `internal/worker/tasks/import_pipeline_test.go`, the test already asserts `ImportFinalizeArgs` for `models.JobSourceDarkadia` (via Task 2's rename) and an error for `nexorious`/`steam`. Confirm it reads:

```go
func TestFinalizeArgsForSource(t *testing.T) {
	args, err := tasks.FinalizeArgsForSource(models.JobSourceDarkadia, "item-1")
	if err != nil {
		t.Fatalf("darkadia: unexpected error %v", err)
	}
	if _, ok := args.(tasks.ImportFinalizeArgs); !ok {
		t.Fatalf("darkadia: got %T, want ImportFinalizeArgs", args)
	}
	if _, err := tasks.FinalizeArgsForSource(models.JobSourceNexorious, "item-1"); err == nil {
		t.Error("nexorious: expected error (no interactive finalize stage)")
	}
	if _, err := tasks.FinalizeArgsForSource("steam", "item-1"); err == nil {
		t.Error("steam: expected error")
	}
}
```

Run: `go test ./internal/worker/tasks/... -run TestFinalizeArgsForSource -count=1` — Expected: PASS.

- [ ] **Step 3: Replace `HandleImportDarkadia` with a generic handler**

In `internal/api/import.go`:
- Replace import `"github.com/drzero42/nexorious/internal/services/darkadia"` with `"github.com/drzero42/nexorious/internal/services/importmodel"` and `"github.com/drzero42/nexorious/internal/services/importsource"`.
- Delete `func (h *ImportHandler) HandleImportDarkadia(...)` and replace with:

```go
// handleImportSource returns an Echo handler for a registered import source.
// It validates IGDB is configured, parses the upload via the source's mapper,
// creates the job + one job_item per game, and enqueues the match stage. One
// active import per (user, source) is allowed at a time.
func (h *ImportHandler) handleImportSource(src importsource.Source) echo.HandlerFunc {
	return func(c *echo.Context) error {
		userID := auth.UserIDFromContext(c)
		if userID == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		if h.igdbClient == nil || !h.igdbClient.Configured() {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("IGDB must be configured to import a %s collection", src.DisplayName))
		}

		if err := c.Request().ParseMultipartForm(maxImportBodyBytes); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "failed to parse multipart form")
		}
		file, _, err := c.Request().FormFile("file")
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "missing file field")
		}
		defer func() { _ = file.Close() }()

		lr := io.LimitReader(file, maxImportBodyBytes+1)
		body, err := io.ReadAll(lr)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
		}
		if len(body) > maxImportBodyBytes {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 50 MB limit")
		}

		games, err := src.Mapper.Parse(body)
		if err != nil {
			if errors.Is(err, importmodel.ErrInvalidSignature) {
				return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("not a %s export", src.DisplayName))
			}
			return echo.NewHTTPError(http.StatusBadRequest, "failed to parse file: "+err.Error())
		}
		if len(games) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no games found in file")
		}

		ctx := context.Background()

		var existing models.Job
		err = h.db.NewSelect().Model(&existing).
			Where("user_id = ?", userID).
			Where("job_type = ?", models.JobTypeImport).
			Where("source = ?", src.Slug).
			Where("status IN (?)", bun.List([]string{models.JobStatusPending, models.JobStatusProcessing})).
			Limit(1).Scan(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to check active import")
		}
		if err == nil {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("an active %s import is already in progress", src.DisplayName))
		}

		now := time.Now().UTC()
		job := &models.Job{
			ID:               uuid.NewString(),
			UserID:           userID,
			JobType:          models.JobTypeImport,
			Source:           src.Slug,
			Status:           models.JobStatusProcessing,
			Priority:         models.JobPriorityHigh,
			TotalItems:       len(games),
			DispatchComplete: false,
			CreatedAt:        now,
		}
		if _, err := h.db.NewInsert().Model(job).Exec(ctx); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
		}

		reqCtx := c.Request().Context()
		for i, g := range games {
			meta, err := json.Marshal(g)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to marshal game payload")
			}
			item := &models.JobItem{
				ID:             uuid.NewString(),
				JobID:          job.ID,
				UserID:         userID,
				ItemKey:        fmt.Sprintf("game_%d", i),
				SourceTitle:    g.Title,
				SourceMetadata: meta,
				Status:         models.JobItemStatusPending,
				Result:         json.RawMessage(`{}`),
				IGDBCandidates: json.RawMessage(`[]`),
			}
			if _, err := h.db.NewInsert().Model(item).Exec(ctx); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
			}
			if h.riverClient != nil {
				if _, err := h.riverClient.Insert(ctx, tasks.ImportMatchArgs{JobItemID: item.ID}, nil); err != nil {
					slog.ErrorContext(reqCtx, "import: submit import_match", "item_id", item.ID, logging.KeyErr, err)
				}
			}
		}

		if _, err := h.db.NewRaw(`UPDATE jobs SET dispatch_complete = true WHERE id = ?`, job.ID).Exec(ctx); err != nil {
			slog.ErrorContext(reqCtx, "import: mark dispatch complete", logging.KeyJobID, job.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		}
		tasks.ImportCheckJobCompletion(h.db, job.ID)

		return c.JSON(http.StatusOK, map[string]any{
			"job_id":      job.ID,
			"source":      job.Source,
			"status":      job.Status,
			"message":     fmt.Sprintf("%s import job created. Matching %d games.", src.DisplayName, len(games)),
			"total_items": len(games),
		})
	}
}
```

- [ ] **Step 4: Register import routes from the registry**

In `internal/api/router.go`, replace the two hardcoded import POST lines (`importGroup.POST("/nexorious", ...)` and `importGroup.POST("/darkadia", ...)`) with:

```go
importGroup.POST("/nexorious", imh.HandleImportNexorious)
for _, src := range importsource.All() {
	importGroup.POST("/"+src.Slug, imh.handleImportSource(src))
}
```

Add import `"github.com/drzero42/nexorious/internal/services/importsource"` to `router.go`.

- [ ] **Step 5: Write the handler tests**

Add to `internal/api/import_test.go` (create if absent; match the package + test-DB helpers used by sibling API tests — `truncateAllTables(t)`, an authed request helper). The two cheap, high-value cases:

```go
func TestHandleImportSource_UnknownSourceNotRouted(t *testing.T) {
	// /api/import/<unregistered> has no route → 404. Only registry slugs are wired.
	truncateAllTables(t)
	rec := doAuthedMultipartUpload(t, "/api/import/grouvee", "file", "x.csv", []byte("a,b\n1,2\n"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unregistered source", rec.Code)
	}
}

func TestHandleImportSource_WrongFileRejected(t *testing.T) {
	// A registered source given the wrong file shape → 400 "not a Darkadia export".
	truncateAllTables(t)
	seedIGDBConfigured(t) // ensure h.igdbClient.Configured() is true in this harness
	rec := doAuthedMultipartUpload(t, "/api/import/darkadia", "file", "x.csv", []byte("wrong,header\n1,2\n"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not a Darkadia export") {
		t.Errorf("body = %q, want 'not a Darkadia export'", rec.Body.String())
	}
}
```

If the API test harness has no existing authed-multipart helper, reuse the pattern from the nearest existing import/upload test in `internal/api/` (search: `ParseMultipartForm` / `CreateFormFile` in `*_test.go`) rather than inventing one. If IGDB cannot be made "configured" in the API harness, drop `TestHandleImportSource_WrongFileRejected` and keep the unknown-source 404 test (which needs no IGDB); the wrong-file path is already covered by `TestDarkadiaMapper_RejectsWrongFile` in Task 3.

- [ ] **Step 6: Build and run — expect green**

Run:
```bash
go build ./...
go test ./internal/api/... ./internal/worker/tasks/... -count=1
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/enqueue.go internal/worker/tasks/import_pipeline_test.go internal/api/import.go internal/api/router.go internal/api/import_test.go
git commit -m "feat: registry-driven import routing and generic upload handler (#1000)"
```

---

## Task 5: `GET /api/import/sources` endpoint

**Files:**
- Create: `internal/api/import_sources.go`
- Modify: `internal/api/router.go` (add the route)
- Test: `internal/api/import_sources_test.go`

- [ ] **Step 1: Write the failing endpoint test**

Create `internal/api/import_sources_test.go`:

```go
func TestHandleListImportSources(t *testing.T) {
	truncateAllTables(t)
	rec := doAuthedGet(t, "/api/import/sources")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, s := range got {
		if s["slug"] == "darkadia" {
			found = true
			if s["display_name"] != "Darkadia" {
				t.Errorf("display_name = %v, want Darkadia", s["display_name"])
			}
			if _, ok := s["accept"]; !ok {
				t.Error("missing accept field")
			}
		}
	}
	if !found {
		t.Error("darkadia missing from /import/sources")
	}
}
```

Use the same authed-GET helper sibling API tests use (search `internal/api/*_test.go` for an existing `GET` helper). Run: `go test ./internal/api/... -run TestHandleListImportSources -count=1` — Expected: FAIL (route 404).

- [ ] **Step 2: Write the handler**

Create `internal/api/import_sources.go`:

```go
package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/services/importsource"
)

// HandleListImportSources returns the registered mapper-based import sources so
// the frontend picker is data-driven from the registry. It does not require
// IGDB to be configured — that guard stays on the upload itself.
func (h *ImportHandler) HandleListImportSources(c *echo.Context) error {
	return c.JSON(http.StatusOK, importsource.All())
}
```

(`Source.Mapper` has a `json:"-"` tag, so only the display fields are serialised.)

- [ ] **Step 3: Register the route**

In `internal/api/router.go`, in the import group add (before the loop, a static route — Echo v5 needs static before any param route, though there is none here, keep it adjacent and explicit):

```go
importGroup.GET("/sources", imh.HandleListImportSources)
```

- [ ] **Step 4: Run the test — expect pass**

Run: `go test ./internal/api/... -run TestHandleListImportSources -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/import_sources.go internal/api/import_sources_test.go internal/api/router.go
git commit -m "feat: add GET /api/import/sources endpoint (#1000)"
```

---

## Task 6: Data-driven frontend source picker

**Files:**
- Modify: `ui/frontend/src/api/import-export.ts`, `ui/frontend/src/types/import-export.ts`, `ui/frontend/src/hooks/use-import-export.ts`, `ui/frontend/src/hooks/index.ts`, `ui/frontend/src/routes/_authenticated/import-export.tsx`, `ui/frontend/src/components/navigation/nav-items.tsx`
- Test: `ui/frontend/src/routes/_authenticated/import-export.test.tsx`

All frontend commands run from `ui/frontend/`.

- [ ] **Step 1: Add the API functions and types**

In `ui/frontend/src/types/import-export.ts`: **remove** the `ImportSource` enum and `getImportSourceDisplayInfo`. Add:

```ts
export interface ImportSourceInfo {
  slug: string;
  display_name: string;
  description: string;
  features: string[];
  accept: string[];
}
```

(Keep `ExportFormat`, `ImportJobCreatedResponse`, `ExportJobCreatedResponse`, `getExportFormatDisplayInfo` untouched.)

In `ui/frontend/src/api/import-export.ts`: **remove** `importDarkadiaCsv`. Add:

```ts
import type { ImportSourceInfo } from '@/types';

/** List the registered mapper-based import sources (drives the picker). */
export async function fetchImportSources(): Promise<ImportSourceInfo[]> {
  return api.get<ImportSourceInfo[]>('/import/sources');
}

/** Upload a file to a registered import source by slug. */
export async function importFromSource(
  slug: string,
  file: File,
): Promise<ImportJobCreatedResponse> {
  return apiUploadFile<ImportJobCreatedResponse>(`/import/${slug}`, file);
}
```

(Keep `importNexoriousJson` as-is — Nexorious is a separate, non-registry path. Add the `ImportSourceInfo` import to the existing type import line.)

- [ ] **Step 2: Add the hooks**

In `ui/frontend/src/hooks/use-import-export.ts`: **remove** `useImportDarkadia`. Add:

```ts
import { useQuery } from '@tanstack/react-query';
import type { ImportSourceInfo } from '@/types';

/** Fetch the registry of mapper-based import sources. */
export function useImportSources() {
  return useQuery<ImportSourceInfo[]>({
    queryKey: [...importExportKeys.all, 'sources'],
    queryFn: () => importExportApi.fetchImportSources(),
    staleTime: Infinity, // registry is static within a session
  });
}

/** Generic import upload for a registered source slug. */
export function useImportSource() {
  const queryClient = useQueryClient();
  return useMutation<ImportJobCreatedResponse, Error, { slug: string; file: File }>({
    mutationFn: ({ slug, file }) => importExportApi.importFromSource(slug, file),
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.IMPORT, result.job_id);
    },
  });
}
```

In `ui/frontend/src/hooks/index.ts`: replace the `useImportDarkadia` export with `useImportSources` and `useImportSource`.

- [ ] **Step 3: Drive the picker from the registry**

In `ui/frontend/src/routes/_authenticated/import-export.tsx`:
- Replace the `useImportDarkadia` usage with `useImportSource` + `useImportSources`.
- Replace the two hardcoded `<ImportCard source=...>` instances. Keep one explicit Nexorious card (its own `useImportNexorious` + `.json` accept), then map the fetched sources:

```tsx
const { mutateAsync: importFromSource } = useImportSource();
const { data: importSources = [] } = useImportSources();
// ...
{/* Nexorious JSON — separate, non-registry path */}
<ImportCard
  title="Nexorious JSON"
  description="Restore a previous Nexorious export with all metadata, ratings, play status, and notes intact."
  features={['Full metadata restoration', 'Preserves ratings and notes', 'Non-interactive import']}
  accept=".json,application/json"
  isUploading={uploadingSlug === 'nexorious'}
  disabled={hasActiveJob}
  onFileSelect={(file) => handleImportFile('nexorious', file)}
/>
{importSources.map((src) => (
  <ImportCard
    key={src.slug}
    title={src.display_name}
    description={src.description}
    features={src.features}
    accept={src.accept.join(',')}
    isUploading={uploadingSlug === src.slug}
    disabled={hasActiveJob}
    onFileSelect={(file) => handleImportFile(src.slug, file)}
  />
))}
```

- Change `ImportCardProps` to take `title`/`description`/`features`/`accept` strings directly instead of `source: ImportSource` + `getImportSourceDisplayInfo`. Drop the icon/color lookup (use a single neutral card style, or keep `color` as an optional prop defaulting to `purple`). Replace the `uploadingSource` state (`ImportSource | null`) with `uploadingSlug` (`string | null`).
- Rewrite `handleImportFile`:

```tsx
const handleImportFile = async (slug: string, file: File) => {
  setUploadingSlug(slug);
  try {
    const result =
      slug === 'nexorious'
        ? await importNexorious(file)
        : await importFromSource({ slug, file });
    toast.success(`Import started: ${result.message}`);
    setDismissedJobId(null);
  } catch (error) {
    toast.error(error instanceof Error ? error.message : 'Import failed');
  } finally {
    setUploadingSlug(null);
  }
};
```

- Replace the manual-match gating `activeJob.source === JobSource.DARKADIA` with a registry-membership check:

```tsx
const importSourceSlugs = new Set(importSources.map((s) => s.slug));
// ...
{!activeJob.isTerminal &&
  activeJob.jobType === JobType.IMPORT &&
  importSourceSlugs.has(activeJob.source) && (
    <JobItemsDetails ... />
)}
```

- [ ] **Step 4: Generalise the nav review-count badge**

In `ui/frontend/src/components/navigation/nav-items.tsx`, replace the hardcoded `reviewData?.countsBySource?.[JobSource.DARKADIA] ?? 0` with a sum across registered import-source slugs:

```tsx
const { data: importSources } = useImportSources();
const importReviewCount = (importSources ?? []).reduce(
  (sum, s) => sum + (reviewData?.countsBySource?.[s.slug] ?? 0),
  0,
);
```

Add the `useImportSources` import. (With only Darkadia registered this is behaviour-identical; it stops being a hardcoded source check.)

- [ ] **Step 5: Update the page test**

In `ui/frontend/src/routes/_authenticated/import-export.test.tsx`:
- Replace the `useImportDarkadia: () => ({ mutateAsync: vi.fn() })` mock with `useImportSource` + a `useImportSources` mock returning `[{ slug: 'darkadia', display_name: 'Darkadia', description: '...', features: [], accept: ['.csv'] }]`.
- Anywhere the test asserts the Darkadia card renders, assert it from the mocked sources list. Keep the existing `makeJob(JobSource.DARKADIA)` review-box assertion — with the mocked sources containing `darkadia`, the membership gate still shows `JobItemsDetails`.

- [ ] **Step 6: Run the frontend gates — expect green**

Run (from `ui/frontend/`):
```bash
npm run check
npm run knip
npm run test
```
Expected: typecheck clean, **zero knip findings** (confirms `ImportSource`/`getImportSourceDisplayInfo`/`useImportDarkadia`/`importDarkadiaCsv` are fully removed, not orphaned), tests PASS.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src
git commit -m "feat: data-driven import source picker from /import/sources (#1000)"
```

---

## Task 7: Full-suite verification and PR

- [ ] **Step 1: Run the complete backend + frontend suites**

```bash
go test -timeout 600s ./...
( cd ui/frontend && npm run check && npm run knip && npm run test )
golangci-lint run
```
Expected: all green. Confirms Darkadia behaviour is unchanged (every original assertion still holds under the renamed tests) and the registry/endpoint/handler are wired.

- [ ] **Step 2: Manual smoke check of the rename invariants**

```bash
grep -rn "darkadia_match\|darkadia_finalize\|DarkadiaMatchWorker\|DarkadiaFinalizeWorker\|DarkadiaCheckJobCompletion" internal/ cmd/ --include=*.go
# Expected: no matches (River kinds + workers fully renamed).
grep -rn "JobSourceDarkadia\|'darkadia'" internal/ --include=*.go | head
# Expected: still present — the source slug "darkadia" is intentionally unchanged.
```

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin issue-1000-source-neutral-import-pipeline
gh pr create --title "refactor: extract a source-neutral import pipeline from the Darkadia importer" --body "$(cat <<'EOF'
Extracts a source-neutral import pipeline from the Darkadia importer so new migration sources need only a mapper + registration. Blocks the rest of the multi-source import epic.

- Canonical `internal/services/importmodel.Game` (moved from `darkadia`).
- `internal/services/importsource` registry (`Mapper` interface + Darkadia entry).
- Source-neutral workers `ImportMatch`/`ImportFinalize`/`ImportCheckJobCompletion` (River kinds `import_match`/`import_finalize`); registry-keyed routing.
- Generic registry-driven upload handler + `GET /api/import/sources`.
- Data-driven frontend picker; nav review badge sums across import sources.

Darkadia import behaviour is unchanged — existing assertions preserved, reorganised under generic test names where the code became generic.

Note: this interprets #1000's "Darkadia tests pass unchanged" as behavioural coverage preserved, tests reorganised to match the now-generic code (see the design doc's "Deliberate deviation" section), rather than byte-identical files.

In-flight `darkadia_match`/`darkadia_finalize` River jobs queued across a deploy are orphaned by the kind rename; acceptable per #1000 (transient one-off migration jobs).

Closes #1000
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- Canonical `importmodel.Game` + `ErrInvalidSignature` → Task 1 + Task 3 Step 1. ✓
- `Mapper` interface + registry, Darkadia registered → Task 3. ✓
- Source-neutral workers + River kind rename + both `serve.go` sites → Task 2. ✓
- Source-routing helpers via registry membership → Task 4 Steps 1–2. ✓
- Parameterised handler + loop route registration → Task 4 Steps 3–4. ✓
- `GET /api/import/sources` → Task 5. ✓
- Data-driven picker + membership-gated review box + nav badge → Task 6. ✓
- Multi-line consolidation stays in the mapper → untouched (Task 1 moves only the return type; `consolidate` logic is unchanged). ✓
- Tests reorganised, Darkadia behaviour unchanged → Tasks 2/4 (renamed worker tests), Task 7 (full suite). ✓
- Nexorious importer untouched → Task 4/6 keep it on its own path. ✓

**Placeholder scan:** No TBD/TODO; all code blocks concrete. The two conditional fallbacks (API IGDB-config helper in Task 4 Step 5; authed-request helpers in Tasks 4–5) are explicit instructions to reuse the nearest existing harness pattern, not placeholders — the assertion logic is fully specified.

**Type consistency:** `ImportMatchArgs`/`ImportFinalizeArgs`/`ImportMatchWorker`/`ImportFinalizeWorker`/`ImportCheckJobCompletion` used consistently across Tasks 2/4. `importsource.Source{Slug, DisplayName, Description, Features, Accept, Mapper}` matches the handler (`src.DisplayName`, `src.Slug`, `src.Mapper`) and the endpoint JSON (`slug`, `display_name`, `description`, `features`, `accept`) which the frontend `ImportSourceInfo` mirrors. `importmodel.ErrInvalidSignature` used by both the mapper (Task 3) and the handler (Task 4).
