# Generic user-mapped CSV import (#1004) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a generic, user-mapped CSV import path (`source = "csv"`): the user uploads a CSV, maps its columns and status values in a single dialog, and the same shared import pipeline (IGDB matching → `pending_review` → additive merge → job history) takes over.

**Architecture:** Two new endpoints (`POST /api/import/csv/inspect`, `POST /api/import/csv`) on the existing `ImportHandler`. The import handler's job-creation tail is extracted into a shared `enqueueImportJob` reused by both the registry sources and the CSV path. A flat request DTO (`csvMapping`) is translated by `buildCSVConfig` into a `csvmap.Config` (simple subset) and run through the already-merged `csvmap.Parse` engine (#1014/PR #1017). The frontend adds a hand-rendered CSV `ImportCard` (like the Nexorious-JSON card) that inspects the file then opens a stacked mapping dialog.

**Tech Stack:** Go (Echo v5, Bun, River), stdlib `encoding/csv`, `internal/services/csvmap`; React 19 + TypeScript, TanStack Query, shadcn/ui (Dialog/Select/Switch/Label), Vitest + testing-library.

**Spec:** `docs/superpowers/specs/2026-06-15-issue-1004-generic-csv-import-design.md`

---

## File Structure

**Backend**
- `internal/api/import.go` — *modify*: extract `enqueueImportJob`; rewrite `handleImportSource` tail to call it.
- `internal/api/import_csv.go` — *create*: `csvMapping` DTO, `buildCSVConfig`, `readUploadFile`, `HandleImportCSVInspect`, `HandleImportCSV`, response types.
- `internal/api/router.go` — *modify*: register the two CSV routes.
- `internal/api/import_csv_internal_test.go` — *create* (`package api`): unit tests for `buildCSVConfig`.
- `internal/api/import_csv_test.go` — *create* (`package api_test`): endpoint tests for inspect + import.

**Frontend**
- `ui/frontend/src/api/client.ts` — *modify*: `apiUploadFile` gains optional `extraFields`.
- `ui/frontend/src/types/import-export.ts` — *modify*: `CsvColumnInfo`, `CsvInspectResponse`, `CsvMapping`.
- `ui/frontend/src/api/import-export.ts` — *modify*: `inspectCsv`, `importCsv`.
- `ui/frontend/src/hooks/use-import-export.ts` — *modify*: `useInspectCsv`, `useImportCsv`.
- `ui/frontend/src/hooks/index.ts` — *modify*: export the two hooks.
- `ui/frontend/src/components/import/csv-mapping.ts` — *create*: pure helpers (`emptyCsvMapping`, `initStatusValueMap`).
- `ui/frontend/src/components/import/csv-mapping.test.ts` — *create*: unit tests for the helpers.
- `ui/frontend/src/components/import/csv-mapping-dialog.tsx` — *create*: the dialog.
- `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx` — *create*: dialog tests.
- `ui/frontend/src/routes/_authenticated/import-export.tsx` — *modify*: CSV card + dialog wiring + pending-review predicate fix.
- `ui/frontend/src/routes/_authenticated/import-export.test.tsx` — *modify*: assert the CSV card renders.

No migration, no schema change (`JobSourceCSV = "csv"` already exists). No new route file → no `routeTree.gen.ts` regeneration (the dialog is a component, not a route).

---

## Task 1: Extract the shared `enqueueImportJob` tail

This is a behaviour-preserving refactor. The existing Darkadia/vglist/nexorious handler tests are the guard — no new test is written; they must stay green.

**Files:**
- Modify: `internal/api/import.go` (add `enqueueImportJob`; rewrite `handleImportSource:271-330`)

- [ ] **Step 1: Add `enqueueImportJob`**

Add this method to `internal/api/import.go` (e.g. directly above `handleImportSource`):

```go
// enqueueImportJob runs the shared post-mapping import tail: it refuses a second
// active import for (user, source), inserts the processing job, creates one
// job_item per game with an ImportMatch task, marks dispatch complete, and
// triggers the completion check. It returns the job id and item count, or an
// *echo.HTTPError the caller can return directly.
func (h *ImportHandler) enqueueImportJob(reqCtx context.Context, userID, source, displayName string, games []importmodel.Game) (string, int, error) {
	ctx := context.Background()

	var existing models.Job
	err := h.db.NewSelect().Model(&existing).
		Where("user_id = ?", userID).
		Where("job_type = ?", models.JobTypeImport).
		Where("source = ?", source).
		Where("status IN (?)", bun.List([]string{models.JobStatusPending, models.JobStatusProcessing})).
		Limit(1).Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to check active import")
	}
	if err == nil {
		return "", 0, echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("an active %s import is already in progress", displayName))
	}

	now := time.Now().UTC()
	job := &models.Job{
		ID:               uuid.NewString(),
		UserID:           userID,
		JobType:          models.JobTypeImport,
		Source:           source,
		Status:           models.JobStatusProcessing,
		Priority:         models.JobPriorityHigh,
		TotalItems:       len(games),
		DispatchComplete: false,
		CreatedAt:        now,
	}
	if _, err := h.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to create import job")
	}

	for i, g := range games {
		meta, err := json.Marshal(g)
		if err != nil {
			return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to marshal game payload")
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
			return "", 0, echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
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

	return job.ID, len(games), nil
}
```

- [ ] **Step 2: Rewrite the `handleImportSource` tail**

In `handleImportSource`, replace everything from `ctx := context.Background()` (currently `import.go:255`) through the end of the `return c.JSON(...)` block (currently `import.go:330`) with:

```go
		jobID, total, err := h.enqueueImportJob(c.Request().Context(), userID, src.Slug, src.DisplayName, games)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{
			"job_id":      jobID,
			"source":      src.Slug,
			"status":      models.JobStatusProcessing,
			"message":     fmt.Sprintf("%s import job created. Matching %d games.", src.DisplayName, total),
			"total_items": total,
		})
	}
}
```

Keep the lines above it intact (auth check, IGDB guard, multipart parse, `src.Mapper.Parse(body)`, the `len(games) == 0` guard). After this edit, `handleImportSource` no longer references `ctx`, `now`, `job`, `existing`, or the per-item loop directly.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: builds clean (no unused-import/variable errors).

- [ ] **Step 4: Run the existing import handler tests (the refactor guard)**

Run: `go test ./internal/api/... -run 'TestImportDarkadia|TestImportNexorious|TestImportVglist|TestImportSource' -v`
Expected: PASS (behaviour unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/api/import.go
git commit -m "refactor: extract shared enqueueImportJob from import handler"
```

---

## Task 2: `csvMapping` DTO + `buildCSVConfig` translation

**Files:**
- Create: `internal/api/import_csv.go`
- Create: `internal/api/import_csv_internal_test.go` (`package api`)

- [ ] **Step 1: Write the failing test**

Create `internal/api/import_csv_internal_test.go`:

```go
package api

import "testing"

func TestBuildCSVConfig_FullMapping(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "Game Name"
	m.Columns.Platform = "System"
	m.Columns.Storefront = "Store"
	m.Columns.AcquiredDate = "Bought"
	m.Columns.Rating = "Score"
	m.Columns.Notes = "Comment"
	m.Columns.HoursPlayed = "Hours"
	m.Columns.Tags = "Labels"
	m.Columns.Loved = "Fav"
	m.Status.Column = "State"
	m.Status.ValueMap = map[string]string{"Beaten": "completed"}
	m.RatingScale = 10
	m.MergeByTitle = true

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Columns.Title != "Game Name" || cfg.Columns.Rating != "Score" || cfg.Columns.HoursPlayed != "Hours" || cfg.Columns.Tags != "Labels" || cfg.Columns.Loved != "Fav" {
		t.Errorf("scalar columns not mapped: %+v", cfg.Columns)
	}
	if cfg.Notes.Column != "Comment" {
		t.Errorf("notes column = %q, want Comment", cfg.Notes.Column)
	}
	if cfg.Platform.Simple == nil || cfg.Platform.Simple.PlatformColumn != "System" ||
		cfg.Platform.Simple.StorefrontColumn != "Store" || cfg.Platform.Simple.AcquiredDateColumn != "Bought" {
		t.Errorf("platform-simple not mapped: %+v", cfg.Platform.Simple)
	}
	if cfg.Platform.Simple.PlatformMap != nil || cfg.Platform.Simple.StorefrontMap != nil {
		t.Errorf("platform/storefront maps should be nil (passthrough)")
	}
	if cfg.Status.Column == nil || cfg.Status.Column.Column != "State" || cfg.Status.Column.Default != "not_started" {
		t.Fatalf("status column not mapped: %+v", cfg.Status.Column)
	}
	if got, ok := cfg.Status.Column.ValueMap["beaten"]; !ok || got != "completed" {
		t.Errorf("value-map key not lowercased / wrong: %+v", cfg.Status.Column.ValueMap)
	}
	if cfg.Rating == nil || cfg.Rating.Scale != 10 || cfg.Rating.Truncate {
		t.Errorf("rating not mapped: %+v", cfg.Rating)
	}
	if cfg.Duration == nil || cfg.Duration.Format != "decimal" {
		t.Errorf("duration not mapped: %+v", cfg.Duration)
	}
	if !cfg.Grouping.MergeByTitle {
		t.Errorf("merge-by-title not set")
	}
}

func TestBuildCSVConfig_OptionalsOmitted(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "Title"

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Platform.Simple != nil {
		t.Errorf("platform should be nil when no platform column")
	}
	if cfg.Status.Column != nil {
		t.Errorf("status should be nil when no status column")
	}
	if cfg.Rating != nil {
		t.Errorf("rating should be nil when no rating column")
	}
	if cfg.Duration != nil {
		t.Errorf("duration should be nil when no hours column")
	}
	if cfg.Notes.Column != "" {
		t.Errorf("notes should be empty when no notes column")
	}
}

func TestBuildCSVConfig_EmptyTitle(t *testing.T) {
	if _, err := buildCSVConfig(csvMapping{}); err == nil {
		t.Fatal("expected an error for empty title")
	}
}

func TestBuildCSVConfig_BadRatingScale(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "Title"
	m.Columns.Rating = "Score"
	m.RatingScale = 7
	if _, err := buildCSVConfig(m); err == nil {
		t.Fatal("expected an error for an invalid rating scale")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestBuildCSVConfig -v`
Expected: FAIL — `undefined: csvMapping` / `undefined: buildCSVConfig` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/api/import_csv.go`:

```go
package api

import (
	"fmt"
	"strings"

	"github.com/drzero42/nexorious/internal/services/csvmap"
)

// csvMapping is the flat, frontend-shaped request body the mapping dialog POSTs
// as the "mapping" form field. It is translated to a csvmap.Config (simple
// subset only) by buildCSVConfig.
type csvMapping struct {
	Columns struct {
		Title        string `json:"title"`
		Platform     string `json:"platform"`
		Storefront   string `json:"storefront"`
		Rating       string `json:"rating"`
		Notes        string `json:"notes"`
		AcquiredDate string `json:"acquired_date"`
		HoursPlayed  string `json:"hours_played"`
		Tags         string `json:"tags"`
		Loved        string `json:"loved"`
	} `json:"columns"`
	Status struct {
		Column   string            `json:"column"`
		ValueMap map[string]string `json:"value_map"`
	} `json:"status"`
	RatingScale  int  `json:"rating_scale"`
	MergeByTitle bool `json:"merge_by_title"`
}

// buildCSVConfig translates the dialog mapping into a csvmap.Config expressing
// only the simple subset. It errors on an empty title or an invalid rating
// scale; advanced engine slots are never populated.
func buildCSVConfig(m csvMapping) (csvmap.Config, error) {
	if strings.TrimSpace(m.Columns.Title) == "" {
		return csvmap.Config{}, fmt.Errorf("a title column is required")
	}

	cfg := csvmap.Config{
		Columns: csvmap.ColumnMap{
			Title:       m.Columns.Title,
			Rating:      m.Columns.Rating,
			HoursPlayed: m.Columns.HoursPlayed,
			Tags:        m.Columns.Tags,
			Loved:       m.Columns.Loved,
		},
		Grouping: csvmap.GroupingConfig{MergeByTitle: m.MergeByTitle},
	}

	if m.Columns.Notes != "" {
		cfg.Notes.Column = m.Columns.Notes
	}

	if m.Status.Column != "" {
		vm := make(map[string]string, len(m.Status.ValueMap))
		for k, v := range m.Status.ValueMap {
			vm[strings.ToLower(strings.TrimSpace(k))] = v
		}
		cfg.Status.Column = &csvmap.StatusColumn{
			Column:   m.Status.Column,
			ValueMap: vm,
			Default:  "not_started",
		}
	}

	if m.Columns.Platform != "" {
		cfg.Platform.Simple = &csvmap.PlatformSimple{
			PlatformColumn:     m.Columns.Platform,
			StorefrontColumn:   m.Columns.Storefront,
			AcquiredDateColumn: m.Columns.AcquiredDate,
		}
	}

	if m.Columns.Rating != "" {
		if m.RatingScale != 5 && m.RatingScale != 10 && m.RatingScale != 100 {
			return csvmap.Config{}, fmt.Errorf("rating scale must be 5, 10, or 100")
		}
		cfg.Rating = &csvmap.RatingConfig{Scale: m.RatingScale, Truncate: false}
	}

	if m.Columns.HoursPlayed != "" {
		cfg.Duration = &csvmap.DurationConfig{Format: "decimal"}
	}

	return cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api/... -run TestBuildCSVConfig -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_internal_test.go
git commit -m "feat: csvMapping DTO and buildCSVConfig translation for CSV import"
```

---

## Task 3: `POST /api/import/csv/inspect` endpoint

**Files:**
- Modify: `internal/api/import_csv.go` (add `readUploadFile`, response types, `HandleImportCSVInspect`)
- Modify: `internal/api/router.go`
- Create: `internal/api/import_csv_test.go` (`package api_test`)

- [ ] **Step 1: Write the failing test**

Create `internal/api/import_csv_test.go`:

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postCSVImport posts a multipart request with a "file" field and, when mapping
// is non-empty, a "mapping" form field. Used by the /api/import/csv tests.
func postCSVImport(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path, filename string, fileContent []byte, mapping, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if fileContent != nil {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("createFormFile: %v", err)
		}
		if _, err := fw.Write(fileContent); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if mapping != "" {
		if err := mw.WriteField("mapping", mapping); err != nil {
			t.Fatalf("write mapping: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestImportCSVInspect_ReturnsHeadersDistinctAndCount(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-inspect")

	csvData := []byte("Name,Status\nCeleste,Beaten\nHades,Playing\nTunic,Beaten\nDanger, \n")
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", csvData, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Headers  []string `json:"headers"`
		RowCount int      `json:"row_count"`
		Columns  []struct {
			Name              string   `json:"name"`
			DistinctValues    []string `json:"distinct_values"`
			DistinctTruncated bool     `json:"distinct_truncated"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Headers) != 2 || resp.Headers[0] != "Name" || resp.Headers[1] != "Status" {
		t.Fatalf("headers = %v", resp.Headers)
	}
	if resp.RowCount != 4 {
		t.Errorf("row_count = %d, want 4", resp.RowCount)
	}
	// "Status" column distinct = Beaten, Playing (dedup; empty cell excluded).
	var statusCol []string
	for _, c := range resp.Columns {
		if c.Name == "Status" {
			statusCol = c.DistinctValues
		}
	}
	if len(statusCol) != 2 {
		t.Errorf("Status distinct = %v, want 2 (Beaten, Playing)", statusCol)
	}
}

func TestImportCSVInspect_CapsDistinctValues(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-cap")

	var b strings.Builder
	b.WriteString("Name,Tag\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "Game %d,tag-%d\n", i, i)
	}
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", []byte(b.String()), token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp struct {
		Columns []struct {
			Name              string   `json:"name"`
			DistinctValues    []string `json:"distinct_values"`
			DistinctTruncated bool     `json:"distinct_truncated"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, c := range resp.Columns {
		if c.Name == "Tag" {
			if len(c.DistinctValues) != 50 || !c.DistinctTruncated {
				t.Errorf("Tag: len=%d truncated=%v, want 50 / true", len(c.DistinctValues), c.DistinctTruncated)
			}
		}
	}
}

func TestImportCSVInspect_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(false))
	_, token := setupTagUser(t, testDB, e, "csv-noigdb")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", []byte("Name\nCeleste\n"), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImportCSVInspect_HeaderlessEmpty(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-empty")

	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", []byte(""), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

var _ = context.Background // keep the context import if unused above
```

> Note: remove the `var _ = context.Background` line if `context` ends up used by Task 4's tests in the same file (it will be). It's a harmless placeholder so this file compiles on its own at this step.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestImportCSVInspect -v`
Expected: FAIL — route returns 404/405 (handler/route not yet wired).

- [ ] **Step 3: Add the handler + helpers to `import_csv.go`**

Add the following imports to the existing `import.go`-style block in `internal/api/import_csv.go` — the file's import block becomes:

```go
import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/csvmap"
)
```

Append to `internal/api/import_csv.go`:

```go
const csvDistinctCap = 50

// csvColumnInfo is one column's name plus a capped set of its distinct non-empty
// values, used to drive the mapping dialog's status-value rows.
type csvColumnInfo struct {
	Name              string   `json:"name"`
	DistinctValues    []string `json:"distinct_values"`
	DistinctTruncated bool     `json:"distinct_truncated"`
}

type csvInspectResponse struct {
	Headers  []string        `json:"headers"`
	RowCount int             `json:"row_count"`
	Columns  []csvColumnInfo `json:"columns"`
}

// readUploadFile parses the multipart form and reads the "file" field, enforcing
// the 50 MB limit. Returns the bytes or an *echo.HTTPError.
func (h *ImportHandler) readUploadFile(c *echo.Context) ([]byte, error) {
	if err := c.Request().ParseMultipartForm(maxImportBodyBytes); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "failed to parse multipart form")
	}
	file, _, err := c.Request().FormFile("file")
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "missing file field")
	}
	defer func() { _ = file.Close() }()
	lr := io.LimitReader(file, maxImportBodyBytes+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	if len(body) > maxImportBodyBytes {
		return nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 50 MB limit")
	}
	return body, nil
}

// csvIGDBGuard returns a 400 *echo.HTTPError when IGDB is not configured.
func (h *ImportHandler) csvIGDBGuard() error {
	if h.igdbClient == nil || !h.igdbClient.Configured() {
		return echo.NewHTTPError(http.StatusBadRequest, "IGDB must be configured to import a CSV collection")
	}
	return nil
}

// HandleImportCSVInspect handles POST /api/import/csv/inspect. It parses the
// uploaded CSV and returns headers, data-row count, and per-column distinct
// values (capped) to drive the mapping dialog.
func (h *ImportHandler) HandleImportCSVInspect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.csvIGDBGuard(); err != nil {
		return err
	}
	body, herr := h.readUploadFile(c)
	if herr != nil {
		return herr
	}

	r := csv.NewReader(bytes.NewReader(body))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read CSV header")
	}

	cols := make([]csvColumnInfo, len(header))
	seen := make([]map[string]bool, len(header))
	for i, name := range header {
		cols[i] = csvColumnInfo{Name: name, DistinctValues: []string{}}
		seen[i] = map[string]bool{}
	}

	rowCount := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
		}
		rowCount++
		for i := range header {
			if i >= len(rec) {
				continue
			}
			v := strings.TrimSpace(rec[i])
			if v == "" || seen[i][v] {
				continue
			}
			seen[i][v] = true
			if len(cols[i].DistinctValues) < csvDistinctCap {
				cols[i].DistinctValues = append(cols[i].DistinctValues, v)
			} else {
				cols[i].DistinctTruncated = true
			}
		}
	}

	return c.JSON(http.StatusOK, csvInspectResponse{Headers: header, RowCount: rowCount, Columns: cols})
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, immediately after the `importGroup.POST("/nexorious", imh.HandleImportNexorious)` line (currently line 409), add:

```go
		importGroup.POST("/csv/inspect", imh.HandleImportCSVInspect)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/api/... -run TestImportCSVInspect -v`
Expected: PASS (all four).

- [ ] **Step 6: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go internal/api/router.go
git commit -m "feat: POST /api/import/csv/inspect endpoint"
```

---

## Task 4: `POST /api/import/csv` endpoint

**Files:**
- Modify: `internal/api/import_csv.go` (add `HandleImportCSV`)
- Modify: `internal/api/router.go`
- Modify: `internal/api/import_csv_test.go` (add import tests; drop the placeholder line)

- [ ] **Step 1: Write the failing test**

Remove the `var _ = context.Background ...` placeholder line at the bottom of `import_csv_test.go`, then append:

```go
// validMapping returns a mapping JSON wiring Name→title and Status→status with
// the given value map.
func validMapping(t *testing.T, valueMap map[string]string) string {
	t.Helper()
	m := map[string]any{
		"columns": map[string]string{
			"title": "Name", "platform": "", "storefront": "", "rating": "",
			"notes": "", "acquired_date": "", "hours_played": "", "tags": "", "loved": "",
		},
		"status":        map[string]any{"column": "Status", "value_map": valueMap},
		"rating_scale":  5,
		"merge_by_title": true,
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal mapping: %v", err)
	}
	return string(b)
}

func TestImportCSV_CreatesJobAndItems(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-success")

	csvData := []byte("Name,Status\nCeleste,Beaten\nHades,Playing\n")
	mapping := validMapping(t, map[string]string{"Beaten": "completed", "Playing": "in_progress"})
	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", csvData, mapping, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	jobID, _ := resp["job_id"].(string)
	if jobID == "" {
		t.Fatalf("empty job_id")
	}
	if resp["source"] != "csv" {
		t.Errorf("source = %v, want csv", resp["source"])
	}
	if tot, _ := resp["total_items"].(float64); int(tot) != 2 {
		t.Errorf("total_items = %v, want 2", resp["total_items"])
	}

	ctx := context.Background()
	var itemCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount); err != nil {
		t.Fatalf("count job_items: %v", err)
	}
	if itemCount != 2 {
		t.Errorf("job_items = %d, want 2", itemCount)
	}
}

func TestImportCSV_Conflict(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-conflict")

	csvData := []byte("Name,Status\nCeleste,Beaten\n")
	mapping := validMapping(t, map[string]string{"Beaten": "completed"})
	if rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", csvData, mapping, token); rec.Code != http.StatusOK {
		t.Fatalf("first import status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", csvData, mapping, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second import status = %d, want 409", rec.Code)
	}
}

func TestImportCSV_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(false))
	_, token := setupTagUser(t, testDB, e, "csv-imp-noigdb")

	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", []byte("Name\nCeleste\n"), validMapping(t, nil), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImportCSV_MissingTitleMapping(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-notitle")

	mapping := `{"columns":{"title":""},"status":{"column":"","value_map":{}},"rating_scale":5,"merge_by_title":true}`
	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", []byte("Name\nCeleste\n"), mapping, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImportCSV_NoDataRows(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-norows")

	rec := postCSVImport(t, e, "/api/import/csv", "lib.csv", []byte("Name,Status\n"), validMapping(t, nil), token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no games)", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestImportCSV_ -v`
Expected: FAIL — `/api/import/csv` not routed (404/405).

- [ ] **Step 3: Add `HandleImportCSV`**

Append to `internal/api/import_csv.go`:

```go
// HandleImportCSV handles POST /api/import/csv. It parses the uploaded CSV with
// a csvmap.Config built from the "mapping" form field, then hands off to the
// shared import pipeline (enqueueImportJob).
func (h *ImportHandler) HandleImportCSV(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.csvIGDBGuard(); err != nil {
		return err
	}
	body, herr := h.readUploadFile(c)
	if herr != nil {
		return herr
	}

	mappingJSON := c.Request().FormValue("mapping")
	if mappingJSON == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing mapping field")
	}
	var mapping csvMapping
	if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid mapping JSON")
	}
	cfg, err := buildCSVConfig(mapping)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	games, err := csvmap.Parse(body, cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
	}
	if len(games) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no games found in file")
	}

	jobID, total, err := h.enqueueImportJob(c.Request().Context(), userID, models.JobSourceCSV, "CSV", games)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"job_id":      jobID,
		"source":      models.JobSourceCSV,
		"status":      models.JobStatusProcessing,
		"message":     fmt.Sprintf("CSV import job created. Matching %d games.", total),
		"total_items": total,
	})
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, immediately after the `importGroup.POST("/csv/inspect", ...)` line added in Task 3, add:

```go
		importGroup.POST("/csv", imh.HandleImportCSV)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/... -run 'TestImportCSV' -v`
Expected: PASS (inspect + import tests).

- [ ] **Step 6: Full backend build + vet the package**

Run: `go build ./... && go test ./internal/api/... -run 'TestImportCSV|TestBuildCSVConfig|TestImportDarkadia|TestImportNexorious' -v`
Expected: builds clean; all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go internal/api/router.go
git commit -m "feat: POST /api/import/csv endpoint on the shared import pipeline"
```

---

## Task 5: Frontend API client, types, and functions

**Files:**
- Modify: `ui/frontend/src/api/client.ts`
- Modify: `ui/frontend/src/types/import-export.ts`
- Modify: `ui/frontend/src/api/import-export.ts`

All commands in this task run from `ui/frontend/`.

- [ ] **Step 1: Extend `apiUploadFile` with `extraFields`**

In `ui/frontend/src/api/client.ts`, change the `apiUploadFile` signature and body (currently lines 161-167):

```ts
export async function apiUploadFile<T>(
  path: string,
  file: File,
  fieldName: string = 'file',
  extraFields?: Record<string, string>,
): Promise<T> {
  const formData = new FormData();
  formData.append(fieldName, file);
  if (extraFields) {
    for (const [key, value] of Object.entries(extraFields)) {
      formData.append(key, value);
    }
  }
```

Leave the rest of the function (the `fetch`, error handling, `return response.json()`) unchanged.

- [ ] **Step 2: Add types**

Append to `ui/frontend/src/types/import-export.ts`:

```ts
export interface CsvColumnInfo {
  name: string;
  distinct_values: string[];
  distinct_truncated: boolean;
}

export interface CsvInspectResponse {
  headers: string[];
  row_count: number;
  columns: CsvColumnInfo[];
}

export interface CsvMapping {
  columns: {
    title: string;
    platform: string;
    storefront: string;
    rating: string;
    notes: string;
    acquired_date: string;
    hours_played: string;
    tags: string;
    loved: string;
  };
  status: {
    column: string;
    value_map: Record<string, string>;
  };
  rating_scale: number;
  merge_by_title: boolean;
}
```

- [ ] **Step 3: Add API functions**

In `ui/frontend/src/api/import-export.ts`, add `CsvInspectResponse, CsvMapping` to the type import from `@/types`, then append the functions (after `importFromSource`):

```ts
/** Inspect a CSV file: headers, row count, and per-column distinct values. */
export async function inspectCsv(file: File): Promise<CsvInspectResponse> {
  return apiUploadFile<CsvInspectResponse>('/import/csv/inspect', file);
}

/** Import a CSV with a user-built column/status mapping. */
export async function importCsv(file: File, mapping: CsvMapping): Promise<ImportJobCreatedResponse> {
  return apiUploadFile<ImportJobCreatedResponse>('/import/csv', file, 'file', {
    mapping: JSON.stringify(mapping),
  });
}
```

- [ ] **Step 4: Type-check**

Run: `npm run check`
Expected: no TypeScript/lint errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/api/client.ts ui/frontend/src/types/import-export.ts ui/frontend/src/api/import-export.ts
git commit -m "feat: CSV inspect/import API client functions and types"
```

---

## Task 6: Frontend hooks

**Files:**
- Modify: `ui/frontend/src/hooks/use-import-export.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

- [ ] **Step 1: Add the hooks**

In `ui/frontend/src/hooks/use-import-export.ts`, add `CsvInspectResponse, CsvMapping` to the `import type { ... } from '@/types'` block, then add after `useImportSource`:

```ts
/** Inspect a CSV file to drive the mapping dialog. */
export function useInspectCsv() {
  return useMutation<CsvInspectResponse, Error, File>({
    mutationFn: (file) => importExportApi.inspectCsv(file),
  });
}

/** Import a CSV with a user-built mapping. */
export function useImportCsv() {
  const queryClient = useQueryClient();
  return useMutation<ImportJobCreatedResponse, Error, { file: File; mapping: CsvMapping }>({
    mutationFn: ({ file, mapping }) => importExportApi.importCsv(file, mapping),
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.IMPORT, result.job_id);
    },
  });
}
```

- [ ] **Step 2: Export from the barrel**

In `ui/frontend/src/hooks/index.ts`, add `useInspectCsv` and `useImportCsv` to the existing export block alongside `useImportSource` (line ~86):

```ts
  useImportNexorious,
  useImportSources,
  useImportSource,
  useInspectCsv,
  useImportCsv,
```

- [ ] **Step 3: Type-check**

Run: `npm run check`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/hooks/use-import-export.ts ui/frontend/src/hooks/index.ts
git commit -m "feat: useInspectCsv and useImportCsv hooks"
```

---

## Task 7: Mapping helpers + the `CsvMappingDialog` component

**Files:**
- Create: `ui/frontend/src/components/import/csv-mapping.ts`
- Create: `ui/frontend/src/components/import/csv-mapping.test.ts`
- Create: `ui/frontend/src/components/import/csv-mapping-dialog.tsx`
- Create: `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx`

- [ ] **Step 1: Write the failing helper test**

Create `ui/frontend/src/components/import/csv-mapping.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { emptyCsvMapping, initStatusValueMap } from './csv-mapping';
import { PlayStatus } from '@/types';

describe('csv-mapping helpers', () => {
  it('emptyCsvMapping defaults merge on, scale 5, all columns blank', () => {
    const m = emptyCsvMapping();
    expect(m.columns.title).toBe('');
    expect(m.rating_scale).toBe(5);
    expect(m.merge_by_title).toBe(true);
    expect(m.status.column).toBe('');
    expect(m.status.value_map).toEqual({});
  });

  it('initStatusValueMap maps every distinct value to Not Started', () => {
    expect(initStatusValueMap(['Beaten', 'Playing'])).toEqual({
      Beaten: PlayStatus.NOT_STARTED,
      Playing: PlayStatus.NOT_STARTED,
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test csv-mapping.test.ts`
Expected: FAIL — cannot resolve `./csv-mapping`.

- [ ] **Step 3: Write the helpers**

Create `ui/frontend/src/components/import/csv-mapping.ts`:

```ts
import { PlayStatus } from '@/types';
import type { CsvMapping } from '@/types';

/** A blank mapping: no columns chosen, merge-by-title on, rating scale 5. */
export function emptyCsvMapping(): CsvMapping {
  return {
    columns: {
      title: '',
      platform: '',
      storefront: '',
      rating: '',
      notes: '',
      acquired_date: '',
      hours_played: '',
      tags: '',
      loved: '',
    },
    status: { column: '', value_map: {} },
    rating_scale: 5,
    merge_by_title: true,
  };
}

/** Map every distinct source value to the Not Started default. */
export function initStatusValueMap(distinct: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const v of distinct) {
    out[v] = PlayStatus.NOT_STARTED;
  }
  return out;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test csv-mapping.test.ts`
Expected: PASS.

- [ ] **Step 5: Write the dialog component**

Create `ui/frontend/src/components/import/csv-mapping-dialog.tsx`:

```tsx
import { useMemo, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { Loader2, Upload } from 'lucide-react';
import { PlayStatus } from '@/types';
import type { CsvInspectResponse, CsvMapping } from '@/types';
import { statusLabels } from '@/lib/play-status';
import { emptyCsvMapping, initStatusValueMap } from './csv-mapping';

// shadcn Select cannot hold an empty-string value, so "no column" uses a sentinel.
const NONE = '__none__';

const OPTIONAL_FIELDS = [
  { key: 'platform', label: 'Platform' },
  { key: 'storefront', label: 'Storefront' },
  { key: 'rating', label: 'Rating' },
  { key: 'notes', label: 'Notes' },
  { key: 'acquired_date', label: 'Acquired date' },
  { key: 'hours_played', label: 'Hours played' },
  { key: 'tags', label: 'Tags' },
  { key: 'loved', label: 'Loved' },
] as const;

type OptionalKey = (typeof OPTIONAL_FIELDS)[number]['key'];

interface CsvMappingDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  inspect: CsvInspectResponse;
  isImporting: boolean;
  onImport: (mapping: CsvMapping) => void;
}

export function CsvMappingDialog({
  open,
  onOpenChange,
  inspect,
  isImporting,
  onImport,
}: CsvMappingDialogProps) {
  const [mapping, setMapping] = useState<CsvMapping>(emptyCsvMapping);

  const setColumn = (key: 'title' | OptionalKey, raw: string) => {
    const value = raw === NONE ? '' : raw;
    setMapping((m) => ({ ...m, columns: { ...m.columns, [key]: value } }));
  };

  const handleStatusColumn = (raw: string) => {
    const column = raw === NONE ? '' : raw;
    const distinct = column
      ? (inspect.columns.find((c) => c.name === column)?.distinct_values ?? [])
      : [];
    setMapping((m) => ({ ...m, status: { column, value_map: initStatusValueMap(distinct) } }));
  };

  const statusColumnInfo = useMemo(
    () => inspect.columns.find((c) => c.name === mapping.status.column),
    [inspect.columns, mapping.status.column],
  );

  const columnSelect = (
    id: string,
    label: string,
    value: string,
    onChange: (raw: string) => void,
  ) => (
    <Select value={value || NONE} onValueChange={onChange}>
      <SelectTrigger id={id} aria-label={`${label} column`}>
        <SelectValue placeholder="— none —" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NONE}>— none —</SelectItem>
        {inspect.headers.map((h) => (
          <SelectItem key={h} value={h}>
            {h}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Import CSV — map your columns</DialogTitle>
          <DialogDescription>
            {inspect.row_count} rows detected. Map your CSV columns to Nexorious fields.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <section className="space-y-3">
            <h4 className="text-sm font-semibold">1 · Map columns</h4>

            <div className="grid grid-cols-2 items-center gap-2">
              <Label htmlFor="csv-title">
                Title <span className="text-red-500">*</span>
              </Label>
              {columnSelect('csv-title', 'Title', mapping.columns.title, (raw) =>
                setColumn('title', raw),
              )}
            </div>

            <div className="grid grid-cols-2 items-center gap-2">
              <Label htmlFor="csv-status">Status</Label>
              <Select value={mapping.status.column || NONE} onValueChange={handleStatusColumn}>
                <SelectTrigger id="csv-status" aria-label="Status column">
                  <SelectValue placeholder="— none —" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>— none —</SelectItem>
                  {inspect.headers.map((h) => (
                    <SelectItem key={h} value={h}>
                      {h}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {OPTIONAL_FIELDS.map((f) => (
              <div key={f.key} className="grid grid-cols-2 items-center gap-2">
                <Label htmlFor={`csv-${f.key}`}>{f.label}</Label>
                <div className="flex items-center gap-2">
                  {columnSelect(`csv-${f.key}`, f.label, mapping.columns[f.key], (raw) =>
                    setColumn(f.key, raw),
                  )}
                  {f.key === 'rating' && mapping.columns.rating && (
                    <Select
                      value={String(mapping.rating_scale)}
                      onValueChange={(v) =>
                        setMapping((m) => ({ ...m, rating_scale: Number(v) }))
                      }
                    >
                      <SelectTrigger aria-label="Rating scale" className="w-28">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="5">out of 5</SelectItem>
                        <SelectItem value="10">out of 10</SelectItem>
                        <SelectItem value="100">out of 100</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                </div>
              </div>
            ))}

            <div className="flex items-center gap-2 pt-1">
              <Switch
                id="csv-merge"
                checked={mapping.merge_by_title}
                onCheckedChange={(checked) =>
                  setMapping((m) => ({ ...m, merge_by_title: checked }))
                }
              />
              <Label htmlFor="csv-merge">Merge rows with the same title</Label>
            </div>
          </section>

          {mapping.status.column && (statusColumnInfo?.distinct_values.length ?? 0) > 0 && (
            <section className="space-y-3">
              <h4 className="text-sm font-semibold">2 · Map status values</h4>
              {statusColumnInfo?.distinct_truncated && (
                <p className="text-xs text-muted-foreground">
                  Showing the first {statusColumnInfo.distinct_values.length} values; any others
                  import as Not Started.
                </p>
              )}
              {statusColumnInfo?.distinct_values.map((value) => (
                <div key={value} className="grid grid-cols-2 items-center gap-2">
                  <Label htmlFor={`csv-sv-${value}`} className="truncate">
                    “{value}”
                  </Label>
                  <Select
                    value={mapping.status.value_map[value] ?? PlayStatus.NOT_STARTED}
                    onValueChange={(v) =>
                      setMapping((m) => ({
                        ...m,
                        status: { ...m.status, value_map: { ...m.status.value_map, [value]: v } },
                      }))
                    }
                  >
                    <SelectTrigger id={`csv-sv-${value}`} aria-label={`Status for ${value}`}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {Object.values(PlayStatus).map((ps) => (
                        <SelectItem key={ps} value={ps}>
                          {statusLabels[ps]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ))}
            </section>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isImporting}>
            Cancel
          </Button>
          <Button onClick={() => onImport(mapping)} disabled={!mapping.columns.title || isImporting}>
            {isImporting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Importing...
              </>
            ) : (
              <>
                <Upload className="mr-2 h-4 w-4" />
                Import {inspect.row_count} games
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 6: Write the dialog test**

Create `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CsvMappingDialog } from './csv-mapping-dialog';
import type { CsvInspectResponse } from '@/types';

const inspect: CsvInspectResponse = {
  headers: ['Name', 'Status'],
  row_count: 3,
  columns: [
    { name: 'Name', distinct_values: ['Celeste', 'Hades', 'Tunic'], distinct_truncated: false },
    { name: 'Status', distinct_values: ['Beaten', 'Playing'], distinct_truncated: false },
  ],
};

function renderDialog(onImport = vi.fn()) {
  render(
    <CsvMappingDialog
      open
      onOpenChange={vi.fn()}
      inspect={inspect}
      isImporting={false}
      onImport={onImport}
    />,
  );
  return onImport;
}

describe('CsvMappingDialog', () => {
  it('disables Import until a title column is chosen', () => {
    renderDialog();
    expect(screen.getByRole('button', { name: /import 3 games/i })).toBeDisabled();
  });

  it('shows status-value rows only after a status column is chosen', async () => {
    const user = userEvent.setup();
    renderDialog();

    expect(screen.queryByText('2 · Map status values')).not.toBeInTheDocument();

    await user.click(screen.getByRole('combobox', { name: 'Status column' }));
    await user.click(screen.getByRole('option', { name: 'Status' }));

    await waitFor(() => {
      expect(screen.getByText('2 · Map status values')).toBeInTheDocument();
    });
    // One row per distinct status value.
    expect(screen.getByRole('combobox', { name: 'Status for Beaten' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Status for Playing' })).toBeInTheDocument();
  });

  it('imports with the assembled mapping (status values default to not_started)', async () => {
    const user = userEvent.setup();
    const onImport = renderDialog();

    await user.click(screen.getByRole('combobox', { name: 'Title column' }));
    await user.click(screen.getByRole('option', { name: 'Name' }));

    await user.click(screen.getByRole('combobox', { name: 'Status column' }));
    await user.click(screen.getByRole('option', { name: 'Status' }));
    await waitFor(() => screen.getByRole('combobox', { name: 'Status for Beaten' }));

    const importBtn = screen.getByRole('button', { name: /import 3 games/i });
    expect(importBtn).toBeEnabled();
    await user.click(importBtn);

    expect(onImport).toHaveBeenCalledTimes(1);
    expect(onImport).toHaveBeenCalledWith({
      columns: {
        title: 'Name',
        platform: '',
        storefront: '',
        rating: '',
        notes: '',
        acquired_date: '',
        hours_played: '',
        tags: '',
        loved: '',
      },
      status: { column: 'Status', value_map: { Beaten: 'not_started', Playing: 'not_started' } },
      rating_scale: 5,
      merge_by_title: true,
    });
  });
});
```

- [ ] **Step 7: Run the dialog tests**

Run: `npm run test csv-mapping-dialog.test.tsx`
Expected: PASS (3 tests). If a Select interaction flakes on pointer events, confirm `src/test/setup.ts` mocks `hasPointerCapture`/`scrollIntoView` (it does) — no test change needed.

- [ ] **Step 8: Type-check**

Run: `npm run check`
Expected: no TypeScript/lint errors. **Do not run `knip` yet** — `CsvMappingDialog` has no consumer until Task 8 wires it into the import page, so knip would (correctly) flag it as unused here. Knip is run in Task 8 once the component is consumed.

- [ ] **Step 9: Commit**

```bash
git add ui/frontend/src/components/import/
git commit -m "feat: CsvMappingDialog and mapping helpers"
```

---

## Task 8: Wire the CSV card + dialog into the Import page

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/import-export.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/import-export.test.tsx`

- [ ] **Step 1: Add imports + hooks + state**

In `import-export.tsx`, add to the `@/hooks` import block: `useInspectCsv`, `useImportCsv`. Add to the type import: `CsvInspectResponse, CsvMapping`. Add the component import:

```ts
import { CsvMappingDialog } from '@/components/import/csv-mapping-dialog';
```

Inside `ImportExportPage`, near the other `useState`/hook calls, add:

```ts
  const [csvFile, setCsvFile] = useState<File | null>(null);
  const [csvInspect, setCsvInspect] = useState<CsvInspectResponse | null>(null);
  const [csvDialogOpen, setCsvDialogOpen] = useState(false);
  const { mutateAsync: inspectCsv, isPending: isInspecting } = useInspectCsv();
  const { mutateAsync: importCsv, isPending: isCsvImporting } = useImportCsv();
```

- [ ] **Step 2: Add the CSV handlers**

Add alongside `handleImportFile`:

```ts
  const handleCsvSelect = async (file: File) => {
    try {
      const result = await inspectCsv(file);
      setCsvFile(file);
      setCsvInspect(result);
      setCsvDialogOpen(true);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to read CSV');
    }
  };

  const handleCsvImport = async (mapping: CsvMapping) => {
    if (!csvFile) return;
    try {
      const result = await importCsv({ file: csvFile, mapping });
      toast.success(`Import started: ${result.message}`);
      setCsvDialogOpen(false);
      setDismissedJobId(null);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Import failed');
    }
  };
```

- [ ] **Step 3: Include `csv` in the pending-review source set**

Change the `importSourceSlugs` construction (currently a single `new Set(...)` line) to also include `csv`, so the `JobItemsDetails` review box shows for CSV imports:

```ts
  const importSourceSlugs = new Set<string>(importSources.map((s: ImportSourceInfo) => s.slug));
  importSourceSlugs.add('csv');
```

- [ ] **Step 4: Render the CSV card**

In the Import Section grid, add a CSV card immediately after the Nexorious JSON `<ImportCard ... />` and before `{importSources.map(...)}`:

```tsx
            <ImportCard
              title="CSV"
              description="Import a library from any tracker's CSV export. You'll map its columns to Nexorious fields in the next step. Requires IGDB to be configured."
              features={['Map any CSV layout', 'Matches games to IGDB', 'Interactive review']}
              accept=".csv,text/csv"
              isUploading={isInspecting}
              disabled={hasActiveJob}
              onFileSelect={handleCsvSelect}
            />
```

- [ ] **Step 5: Render the dialog**

Just before the final closing `</div>` of the component's returned JSX (after the Recent Activity `</section>`), add:

```tsx
      {csvInspect && (
        <CsvMappingDialog
          open={csvDialogOpen}
          onOpenChange={setCsvDialogOpen}
          inspect={csvInspect}
          isImporting={isCsvImporting}
          onImport={handleCsvImport}
        />
      )}
```

- [ ] **Step 6: Add a render assertion to the page test**

In `ui/frontend/src/routes/_authenticated/import-export.test.tsx`, add a test that the CSV card renders. Match the file's existing render helper/imports (reuse whatever `renderPage`/wrapper the file already defines):

```tsx
  it('renders the CSV import card', async () => {
    renderImportExportPage(); // use the file's existing render helper
    expect(await screen.findByRole('heading', { name: 'CSV' })).toBeInTheDocument();
  });
```

If the existing tests use a different render helper name, call that instead; do not introduce a new wrapper.

- [ ] **Step 7: Run the page tests + checks**

Run: `npm run test import-export.test.tsx && npm run check && npm run knip`
Expected: PASS; no type errors; no knip findings (CsvMappingDialog now consumed).

- [ ] **Step 8: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/import-export.tsx ui/frontend/src/routes/_authenticated/import-export.test.tsx
git commit -m "feat: wire CSV import card and mapping dialog into the import page"
```

---

## Task 9: Full verification

- [ ] **Step 1: Backend build + targeted tests**

Run: `go build ./... && go test ./internal/api/... -run 'TestImportCSV|TestBuildCSVConfig|TestImportDarkadia|TestImportNexorious|TestImportSource' -v`
Expected: builds clean; all PASS.

- [ ] **Step 2: Frontend full gate**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: no type/lint errors, no knip findings, all tests PASS.

- [ ] **Step 3: Manual smoke (optional but recommended)**

With a running dev server and IGDB configured: open `/import-export`, pick a small `.csv`, confirm the dialog opens pre-populated, map Title + Status, click Import, and confirm a job appears and (for ambiguous titles) the review box shows.

- [ ] **Step 4: Push and open the PR**

```bash
git push -u origin feat/1004-generic-csv-import
gh pr create --title "feat: generic user-mapped CSV import (#1004)" --body "$(cat <<'EOF'
Implements #1004 — a generic, user-mapped CSV import path on the shared import pipeline.

- `POST /api/import/csv/inspect` — headers, row count, per-column distinct values (capped at 50).
- `POST /api/import/csv` — builds a `csvmap.Config` from a dialog mapping and runs the shared pipeline.
- Extracts `enqueueImportJob` shared by all import sources.
- Single-screen stacked mapping dialog (columns, rating scale, merge toggle, status-value rows defaulting to Not Started).
- `source = "csv"` reused (distinguished by `job_type`); CSV imports surface the `pending_review` review box.

Closes #1004
EOF
)"
```

---

## Self-Review notes (for the executor)

- **Spec coverage:** inspect endpoint (Task 3), mapping dialog + status prefill (Task 7), `csvmap.Config` build + shared pipeline (Tasks 2/4/1), `pending_review` wiring + IGDB guard (Tasks 8/3/4), tests across translation/inspect/import/dialog (Tasks 2/3/4/7) — all map to acceptance criteria.
- **`csvMapping`/`buildCSVConfig`/`enqueueImportJob`/`readUploadFile`/`HandleImportCSV(Inspect)` names** are used consistently across tasks.
- **No new route file**, so `routeTree.gen.ts` is untouched.
- **Play-status value** is `in_progress` (not `playing`) — used in Task 4's value map and the dialog options come from `PlayStatus`/`statusLabels`.
