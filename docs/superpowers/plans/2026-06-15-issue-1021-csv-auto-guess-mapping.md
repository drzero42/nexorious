# CSV import auto-guess column mapping (#1021) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Seed the generic CSV import dialog with a best-effort guess — which header maps to each canonical field, the status column with per-value play-status guesses, and an inferred rating scale — computed on the backend and returned from `/api/import/csv/inspect`, so the user confirms rather than maps from scratch; and hide already-claimed columns from the other dropdowns.

**Architecture:** A pure heuristic in `internal/services/csvmap` (`GuessColumns`, `GuessRatingScale`, `GuessStatusValueMap`) produces a `SuggestedMapping` (JSON-shaped to match the frontend `CsvMapping`). `HandleImportCSVInspect` runs the header guess up front, tracks the guessed rating column's numeric max during its existing single streaming pass, then attaches `suggested_mapping` to the response. The dialog seeds its form state from that suggestion (falling back to empty), and each column `<Select>` shows only headers not claimed by another field, recomputed purely from current mapping state.

**Tech Stack:** Go (Echo v5, stdlib `encoding/csv`, `strconv`, `unicode`), `internal/services/csvmap`; React 19 + TypeScript, shadcn/ui Select, Vitest + testing-library.

**Spec:** `docs/superpowers/specs/2026-06-15-issue-1021-csv-auto-guess-mapping-design.md`

---

## File Structure

**Backend**
- `internal/services/csvmap/guess.go` — *create*: `SuggestedMapping` type, `normalizeHeader`, `GuessColumns`, `GuessRatingScale`, `GuessStatusValueMap`, the alias tables, and the status-synonym table.
- `internal/services/csvmap/guess_test.go` — *create*: table-driven unit tests for the three guess functions.
- `internal/api/import_csv.go` — *modify*: add `SuggestedMapping` to `csvInspectResponse`; rewrite `HandleImportCSVInspect` to compute and attach the suggestion (`strconv` import added).
- `internal/api/import_csv_test.go` — *modify*: add `TestImportCSVInspect_SuggestsMapping`.

**Frontend** (all paths under `ui/frontend/`)
- `src/types/import-export.ts` — *modify*: `CsvInspectResponse` gains optional `suggested_mapping?: CsvMapping`.
- `src/components/import/csv-mapping.ts` — *modify*: add `usedColumns` + `availableHeaders` pure helpers.
- `src/components/import/csv-mapping.test.ts` — *modify*: tests for the two helpers.
- `src/components/import/csv-mapping-dialog.tsx` — *modify*: seed state from `inspect.suggested_mapping`; filter each select's options via `availableHeaders`.
- `src/components/import/csv-mapping-dialog.test.tsx` — *modify*: assert the dialog pre-fills from a suggestion.

No migration, no schema change, no new route → no `routeTree.gen.ts` regeneration.

---

## Task 1: `csvmap` guess functions (pure)

**Files:**
- Create: `internal/services/csvmap/guess.go`
- Create: `internal/services/csvmap/guess_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/services/csvmap/guess_test.go`:

```go
package csvmap

import (
	"reflect"
	"testing"
)

func TestGuessColumns_ExactAndContains(t *testing.T) {
	header := []string{"Game Name", "System", "Play Status", "Score", "Date Bought", "Hours Played", "Genres", "Favorite", "Store", "Comments"}
	m := GuessColumns(header)

	if m.Columns.Title != "Game Name" {
		t.Errorf("title = %q, want Game Name", m.Columns.Title)
	}
	if m.Columns.Platform != "System" {
		t.Errorf("platform = %q, want System", m.Columns.Platform)
	}
	if m.Status.Column != "Play Status" {
		t.Errorf("status column = %q, want Play Status", m.Status.Column)
	}
	if m.Columns.Rating != "Score" {
		t.Errorf("rating = %q, want Score", m.Columns.Rating)
	}
	if m.Columns.AcquiredDate != "Date Bought" {
		t.Errorf("acquired_date = %q, want Date Bought", m.Columns.AcquiredDate)
	}
	if m.Columns.HoursPlayed != "Hours Played" {
		t.Errorf("hours_played = %q, want Hours Played", m.Columns.HoursPlayed)
	}
	if m.Columns.Tags != "Genres" {
		t.Errorf("tags = %q, want Genres", m.Columns.Tags)
	}
	if m.Columns.Loved != "Favorite" {
		t.Errorf("loved = %q, want Favorite", m.Columns.Loved)
	}
	if m.Columns.Storefront != "Store" {
		t.Errorf("storefront = %q, want Store", m.Columns.Storefront)
	}
	if m.Columns.Notes != "Comments" {
		t.Errorf("notes = %q, want Comments", m.Columns.Notes)
	}
	// Defaults.
	if !m.MergeByTitle {
		t.Errorf("merge_by_title should default true")
	}
	if m.RatingScale != 5 {
		t.Errorf("rating_scale default = %d, want 5", m.RatingScale)
	}
}

func TestGuessColumns_FirstWinsAndDedup(t *testing.T) {
	// Two title-ish headers: the first (by file order, exact-normalized) wins,
	// and the claimed header is not reused.
	header := []string{"Title", "Name"}
	m := GuessColumns(header)
	if m.Columns.Title != "Title" {
		t.Errorf("title = %q, want Title (first match wins)", m.Columns.Title)
	}
	// "Name" must not also be claimed by another field.
	if m.Columns.Platform == "Name" || m.Columns.Notes == "Name" {
		t.Errorf("Name should remain unclaimed, got platform=%q notes=%q", m.Columns.Platform, m.Columns.Notes)
	}
}

func TestGuessColumns_NoMatchLeavesBlank(t *testing.T) {
	m := GuessColumns([]string{"col_a", "col_b"})
	if m.Columns.Title != "" || m.Status.Column != "" || m.Columns.Rating != "" {
		t.Errorf("expected all blank, got %+v / status=%q", m.Columns, m.Status.Column)
	}
}

func TestGuessRatingScale(t *testing.T) {
	cases := []struct {
		max  float64
		want int
	}{
		{0, 5}, {3, 5}, {5, 5}, {6, 10}, {10, 10}, {42, 100}, {100, 100},
	}
	for _, c := range cases {
		if got := GuessRatingScale(c.max); got != c.want {
			t.Errorf("GuessRatingScale(%v) = %d, want %d", c.max, got, c.want)
		}
	}
}

func TestGuessStatusValueMap(t *testing.T) {
	got := GuessStatusValueMap([]string{"Beaten", "Playing", "Backlog", "Dropped", "Weird Value"})
	want := map[string]string{
		"Beaten":      "completed",
		"Playing":     "in_progress",
		"Backlog":     "not_started",
		"Dropped":     "dropped",
		"Weird Value": "not_started", // unmatched -> default
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GuessStatusValueMap = %v, want %v", got, want)
	}
}

func TestGuessStatusValueMap_Empty(t *testing.T) {
	if got := GuessStatusValueMap(nil); len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/ -run 'TestGuess' -v`
Expected: FAIL — `undefined: GuessColumns` / `undefined: GuessRatingScale` / `undefined: GuessStatusValueMap` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/services/csvmap/guess.go`:

```go
package csvmap

import (
	"strings"
	"unicode"
)

// SuggestedMapping is a best-effort, frontend-shaped guess of how a CSV's
// headers map to canonical fields. Its JSON shape is byte-for-byte the
// frontend CsvMapping, so the import dialog can drop it straight into its form
// state. It only seeds the dialog; the submitted mapping remains authoritative.
type SuggestedMapping struct {
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

// normalizeHeader lowercases and strips every non-alphanumeric rune, so
// "Date Bought" -> "datebought" and "Play-Status" -> "playstatus".
func normalizeHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fieldAliases lists, per canonical field, the normalized header aliases that
// identify it. Order is priority order for contended headers (title first).
// The matcher does an exact-normalized pass over all fields, then a
// substring-contains pass for the still-unassigned ones.
var fieldAliases = []struct {
	set     func(*SuggestedMapping, string)
	aliases []string
}{
	{func(m *SuggestedMapping, v string) { m.Columns.Title = v }, []string{"name", "title", "game", "gamename", "gametitle"}},
	{func(m *SuggestedMapping, v string) { m.Status.Column = v }, []string{"status", "playstatus", "state", "progress", "completion", "completionstatus"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Platform = v }, []string{"platform", "system", "console", "device"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Storefront = v }, []string{"storefront", "store", "source", "launcher", "service"}},
	{func(m *SuggestedMapping, v string) { m.Columns.HoursPlayed = v }, []string{"hoursplayed", "playtimehours", "playtime", "timeplayed", "hours", "hrs"}},
	{func(m *SuggestedMapping, v string) { m.Columns.AcquiredDate = v }, []string{"acquireddate", "dateacquired", "dateadded", "purchasedate", "acquired", "purchased", "bought", "added"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Rating = v }, []string{"rating", "score", "stars"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Tags = v }, []string{"tags", "tag", "labels", "label", "categories", "genres"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Notes = v }, []string{"notes", "note", "review", "comment", "comments"}},
	{func(m *SuggestedMapping, v string) { m.Columns.Loved = v }, []string{"loved", "favorite", "favourite", "fav", "liked", "starred"}},
}

func matchesAlias(norm string, aliases []string, contains bool) bool {
	for _, a := range aliases {
		if norm == a {
			return true
		}
		if contains && strings.Contains(norm, a) {
			return true
		}
	}
	return false
}

// GuessColumns matches each canonical field to at most one header, header by
// header in file order. Exact-normalized matches are taken first (all fields),
// then a substring-contains pass fills the rest. A header claimed by one field
// is never reused. RatingScale defaults to 5 and MergeByTitle to true; the
// caller refines RatingScale and the status ValueMap from the data.
func GuessColumns(header []string) SuggestedMapping {
	var m SuggestedMapping
	m.MergeByTitle = true
	m.RatingScale = 5
	m.Status.ValueMap = map[string]string{}

	norms := make([]string, len(header))
	for i, h := range header {
		norms[i] = normalizeHeader(h)
	}
	claimed := make([]bool, len(header))
	assigned := make([]bool, len(fieldAliases))

	pass := func(contains bool) {
		for fi := range fieldAliases {
			if assigned[fi] {
				continue
			}
			for hi := range header {
				if claimed[hi] || norms[hi] == "" {
					continue
				}
				if matchesAlias(norms[hi], fieldAliases[fi].aliases, contains) {
					fieldAliases[fi].set(&m, header[hi])
					claimed[hi] = true
					assigned[fi] = true
					break
				}
			}
		}
	}
	pass(false) // exact-normalized
	pass(true)  // substring-contains

	return m
}

// GuessRatingScale maps an observed maximum rating value to a supported scale.
// A non-positive max (no numeric values seen) falls back to 5.
func GuessRatingScale(max float64) int {
	switch {
	case max <= 5:
		return 5
	case max <= 10:
		return 10
	default:
		return 100
	}
}

// statusSynonyms maps a normalized source status value to a play_status. Values
// not present here fall back to "not_started".
var statusSynonyms = map[string]string{
	"completed": "completed", "complete": "completed", "beaten": "completed",
	"finished": "completed", "done": "completed", "100": "completed", "100percent": "completed",
	"inprogress": "in_progress", "playing": "in_progress", "started": "in_progress",
	"current": "in_progress", "ongoing": "in_progress",
	"notstarted": "not_started", "backlog": "not_started", "unplayed": "not_started",
	"neverplayed": "not_started", "tobeplayed": "not_started", "tbp": "not_started", "wishlist": "not_started",
	"dropped": "dropped", "abandoned": "dropped", "quit": "dropped", "gaveup": "dropped",
	"shelved": "shelved", "onhold": "shelved", "hold": "shelved", "paused": "shelved", "suspended": "shelved",
	"mastered": "mastered", "platinum": "mastered", "perfected": "mastered",
	"dominated": "dominated",
	"replay":    "replay", "replaying": "replay", "revisiting": "replay",
}

// GuessStatusValueMap maps each distinct source value to a guessed play_status,
// keyed by the raw source value (matching the dialog's value-row keys). Values
// with no synonym map to "not_started".
func GuessStatusValueMap(distinct []string) map[string]string {
	out := make(map[string]string, len(distinct))
	for _, v := range distinct {
		if ps, ok := statusSynonyms[normalizeHeader(v)]; ok {
			out[v] = ps
		} else {
			out[v] = "not_started"
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run 'TestGuess' -v`
Expected: PASS (all six).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/guess.go internal/services/csvmap/guess_test.go
git commit -m "feat: csvmap header->field guess heuristic for CSV import"
```

---

## Task 2: Attach `suggested_mapping` to the inspect response

**Files:**
- Modify: `internal/api/import_csv.go` (add `strconv` import; add field to `csvInspectResponse`; rewrite `HandleImportCSVInspect`)
- Modify: `internal/api/import_csv_test.go` (add `TestImportCSVInspect_SuggestsMapping`)

- [ ] **Step 1: Write the failing test**

Append to `internal/api/import_csv_test.go`:

```go
func TestImportCSVInspect_SuggestsMapping(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, testIGDBClient(true))
	_, token := setupTagUser(t, testDB, e, "csv-suggest")

	csvData := []byte("Name,Platform,Status,Score\nCeleste,Switch,Beaten,9\nHades,PC,Playing,8\n")
	rec := postMultipartFile(t, e, "/api/import/csv/inspect", "lib.csv", csvData, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp struct {
		SuggestedMapping struct {
			Columns struct {
				Title    string `json:"title"`
				Platform string `json:"platform"`
				Rating   string `json:"rating"`
			} `json:"columns"`
			Status struct {
				Column   string            `json:"column"`
				ValueMap map[string]string `json:"value_map"`
			} `json:"status"`
			RatingScale  int  `json:"rating_scale"`
			MergeByTitle bool `json:"merge_by_title"`
		} `json:"suggested_mapping"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sm := resp.SuggestedMapping
	if sm.Columns.Title != "Name" || sm.Columns.Platform != "Platform" || sm.Columns.Rating != "Score" {
		t.Errorf("columns guess wrong: %+v", sm.Columns)
	}
	if sm.Status.Column != "Status" {
		t.Errorf("status column = %q, want Status", sm.Status.Column)
	}
	if sm.Status.ValueMap["Beaten"] != "completed" || sm.Status.ValueMap["Playing"] != "in_progress" {
		t.Errorf("status value_map wrong: %+v", sm.Status.ValueMap)
	}
	if sm.RatingScale != 10 { // max score 9 -> out of 10
		t.Errorf("rating_scale = %d, want 10", sm.RatingScale)
	}
	if !sm.MergeByTitle {
		t.Errorf("merge_by_title should default true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -run TestImportCSVInspect_SuggestsMapping -v`
Expected: FAIL — `suggested_mapping` is absent/zero (title empty, status column empty).

- [ ] **Step 3: Add `strconv` to the import block**

In `internal/api/import_csv.go`, add `"strconv"` to the stdlib import group (alongside `"strings"`):

```go
import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/csvmap"
)
```

- [ ] **Step 4: Add `SuggestedMapping` to the response type**

In `internal/api/import_csv.go`, replace the `csvInspectResponse` struct with:

```go
type csvInspectResponse struct {
	Headers          []string                `json:"headers"`
	RowCount         int                     `json:"row_count"`
	Columns          []csvColumnInfo         `json:"columns"`
	SuggestedMapping csvmap.SuggestedMapping `json:"suggested_mapping"`
}
```

- [ ] **Step 5: Rewrite `HandleImportCSVInspect` to compute the suggestion**

Replace the body of `HandleImportCSVInspect` (from `r := csv.NewReader(...)` through the final `return c.JSON(...)`) with:

```go
	r := csv.NewReader(bytes.NewReader(body))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read CSV header")
	}

	// Guess the column->field mapping from the headers alone so we know which
	// column is the rating column (to track its numeric max below).
	suggested := csvmap.GuessColumns(header)
	ratingIdx := -1
	for i, name := range header {
		if name == suggested.Columns.Rating {
			ratingIdx = i
			break
		}
	}

	cols := make([]csvColumnInfo, len(header))
	seen := make([]map[string]bool, len(header))
	for i, name := range header {
		cols[i] = csvColumnInfo{Name: name, DistinctValues: []string{}}
		seen[i] = map[string]bool{}
	}

	rowCount := 0
	var ratingMax float64
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
			if i == ratingIdx && v != "" {
				if f, perr := strconv.ParseFloat(v, 64); perr == nil && f > ratingMax {
					ratingMax = f
				}
			}
			if v == "" || cols[i].DistinctTruncated || seen[i][v] {
				continue
			}
			if len(cols[i].DistinctValues) < csvDistinctCap {
				seen[i][v] = true
				cols[i].DistinctValues = append(cols[i].DistinctValues, v)
			} else {
				// A distinct value beyond the cap: flag truncation and stop
				// tracking this column so `seen` stays bounded to the cap.
				cols[i].DistinctTruncated = true
			}
		}
	}

	// Refine the suggestion from the data: rating scale from the observed max,
	// and per-value status guesses from the status column's distinct values.
	if suggested.Columns.Rating != "" {
		suggested.RatingScale = csvmap.GuessRatingScale(ratingMax)
	}
	if suggested.Status.Column != "" {
		for _, col := range cols {
			if col.Name == suggested.Status.Column {
				suggested.Status.ValueMap = csvmap.GuessStatusValueMap(col.DistinctValues)
				break
			}
		}
	}

	return c.JSON(http.StatusOK, csvInspectResponse{
		Headers:          header,
		RowCount:         rowCount,
		Columns:          cols,
		SuggestedMapping: suggested,
	})
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/api/... -run 'TestImportCSVInspect' -v`
Expected: PASS (the new test plus the existing inspect tests stay green).

- [ ] **Step 7: Build the backend**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 8: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go
git commit -m "feat: return suggested_mapping from CSV inspect endpoint"
```

---

## Task 3: Frontend type — `suggested_mapping`

**Files:**
- Modify: `ui/frontend/src/types/import-export.ts`

- [ ] **Step 1: Add the optional field**

In `ui/frontend/src/types/import-export.ts`, change `CsvInspectResponse` to:

```ts
export interface CsvInspectResponse {
  headers: string[];
  row_count: number;
  columns: CsvColumnInfo[];
  suggested_mapping?: CsvMapping;
}
```

It is optional so existing fixtures/tests that build a `CsvInspectResponse` without a suggestion still type-check, and the dialog's `?? emptyCsvMapping()` fallback stays meaningful. The backend always populates it.

- [ ] **Step 2: Type-check**

Run (from `ui/frontend/`): `npm run check`
Expected: no TypeScript/lint errors.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/types/import-export.ts
git commit -m "feat: add suggested_mapping to CsvInspectResponse type"
```

---

## Task 4: Frontend `availableHeaders` / `usedColumns` helpers

**Files:**
- Modify: `ui/frontend/src/components/import/csv-mapping.ts`
- Modify: `ui/frontend/src/components/import/csv-mapping.test.ts`

- [ ] **Step 1: Write the failing test**

Append to `ui/frontend/src/components/import/csv-mapping.test.ts` (inside the existing `describe` block, before its closing `});`):

```ts
  it('usedColumns collects every non-empty column + status column', () => {
    const m = emptyCsvMapping();
    m.columns.title = 'Name';
    m.columns.platform = 'System';
    m.status.column = 'Status';
    expect(usedColumns(m)).toEqual(new Set(['Name', 'System', 'Status']));
  });

  it('availableHeaders hides headers used by other fields but keeps own value', () => {
    const headers = ['Name', 'System', 'Status'];
    const m = emptyCsvMapping();
    m.columns.title = 'Name';
    m.columns.platform = 'System';
    // For the title field (own value 'Name'): 'System' is used elsewhere and hidden.
    expect(availableHeaders(headers, m, 'Name')).toEqual(['Name', 'Status']);
    // For an unset field: both used headers hidden.
    expect(availableHeaders(headers, m, '')).toEqual(['Status']);
  });

  it('availableHeaders frees a column once its field is cleared', () => {
    const headers = ['Name', 'System'];
    const m = emptyCsvMapping();
    m.columns.title = 'Name';
    m.columns.platform = 'System';
    // Platform still set -> hidden for the unset rating field.
    expect(availableHeaders(headers, m, '')).toEqual([]);
    // Clear platform -> 'System' returns to the pool immediately.
    m.columns.platform = '';
    expect(availableHeaders(headers, m, '')).toEqual(['System']);
  });
```

Add `usedColumns, availableHeaders` to the existing import at the top of the file:

```ts
import { emptyCsvMapping, initStatusValueMap, usedColumns, availableHeaders } from './csv-mapping';
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test csv-mapping.test.ts`
Expected: FAIL — `usedColumns` / `availableHeaders` are not exported.

- [ ] **Step 3: Write the helpers**

Append to `ui/frontend/src/components/import/csv-mapping.ts`:

```ts
/** The set of headers currently claimed by any field (columns + status). */
export function usedColumns(mapping: CsvMapping): Set<string> {
  const used = new Set<string>();
  for (const v of Object.values(mapping.columns)) {
    if (v) used.add(v);
  }
  if (mapping.status.column) used.add(mapping.status.column);
  return used;
}

/**
 * Headers selectable for one field: every header not claimed by another field,
 * plus this field's own current value. Derived purely from the current mapping,
 * so clearing or reassigning any field immediately frees its column for others.
 */
export function availableHeaders(
  allHeaders: string[],
  mapping: CsvMapping,
  currentValue: string,
): string[] {
  const used = usedColumns(mapping);
  return allHeaders.filter((h) => h === currentValue || !used.has(h));
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (from `ui/frontend/`): `npm run test csv-mapping.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/import/csv-mapping.ts ui/frontend/src/components/import/csv-mapping.test.ts
git commit -m "feat: availableHeaders helper to hide already-mapped CSV columns"
```

---

## Task 5: Wire the dialog — seed from suggestion + hide used columns

**Files:**
- Modify: `ui/frontend/src/components/import/csv-mapping-dialog.tsx`
- Modify: `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx`

- [ ] **Step 1: Write the failing test**

Append to `ui/frontend/src/components/import/csv-mapping-dialog.test.tsx` (inside the existing `describe('CsvMappingDialog', ...)` block, before its closing `});`):

```ts
  it('pre-fills the form from inspect.suggested_mapping', () => {
    const seeded: CsvInspectResponse = {
      ...inspect,
      suggested_mapping: {
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
        status: { column: 'Status', value_map: { Beaten: 'completed', Playing: 'in_progress' } },
        rating_scale: 5,
        merge_by_title: true,
      },
    };
    render(
      <CsvMappingDialog
        open
        onOpenChange={vi.fn()}
        inspect={seeded}
        isImporting={false}
        onImport={vi.fn()}
      />,
    );
    // Title pre-filled -> Import button is enabled.
    expect(screen.getByRole('button', { name: /import 3 games/i })).toBeEnabled();
    // Title select shows the guessed header.
    expect(screen.getByLabelText('Title column')).toHaveTextContent('Name');
    // Status column guessed -> the status-value section is already shown.
    expect(screen.getByText('2 · Map status values')).toBeInTheDocument();
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test csv-mapping-dialog.test.tsx`
Expected: FAIL — the form still seeds from `emptyCsvMapping()`, so Title is empty and the Import button is disabled.

- [ ] **Step 3: Seed the form from the suggestion**

In `ui/frontend/src/components/import/csv-mapping-dialog.tsx`, update the import from `./csv-mapping` to include `availableHeaders`:

```ts
import { emptyCsvMapping, initStatusValueMap, availableHeaders } from './csv-mapping';
```

Then change the `useState` initializer in `CsvMappingForm` (currently `useState<CsvMapping>(emptyCsvMapping)`) to seed from the suggestion, falling back to empty:

```ts
  const [mapping, setMapping] = useState<CsvMapping>(
    () => inspect.suggested_mapping ?? emptyCsvMapping(),
  );
```

- [ ] **Step 4: Hide already-mapped columns in every select**

In the `columnSelect` helper inside `CsvMappingForm`, replace the `inspect.headers.map(...)` options with the filtered list. Change:

```tsx
        {inspect.headers.map((h) => (
          <SelectItem key={h} value={h}>
            {h}
          </SelectItem>
        ))}
```

to:

```tsx
        {availableHeaders(inspect.headers, mapping, value).map((h) => (
          <SelectItem key={h} value={h}>
            {h}
          </SelectItem>
        ))}
```

Because every field's `<Select>` (title, status, and the optional fields) is rendered through `columnSelect`, this single change applies the hide-used-columns behaviour everywhere, and `availableHeaders` recomputes from `mapping` on each render so a change frees columns immediately.

- [ ] **Step 5: Run the dialog tests to verify they pass**

Run (from `ui/frontend/`): `npm run test csv-mapping-dialog.test.tsx`
Expected: PASS — the new pre-fill test plus the existing dialog tests (which build `inspect` without a `suggested_mapping`, so they still fall back to empty and behave as before).

- [ ] **Step 6: Full frontend gates**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: no type/lint errors, no knip findings, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/components/import/csv-mapping-dialog.tsx ui/frontend/src/components/import/csv-mapping-dialog.test.tsx
git commit -m "feat: seed CSV mapping dialog from suggestion and hide mapped columns"
```

---

## Task 6: Final verification

- [ ] **Step 1: Backend build + targeted tests**

Run: `go build ./... && go test ./internal/services/csvmap/ ./internal/api/... -run 'TestGuess|TestImportCSV' -v`
Expected: builds clean; all PASS.

- [ ] **Step 2: Frontend gates**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: clean.

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin issue-1021-csv-auto-guess-mapping
gh pr create --title "feat: auto-guess CSV import column mapping from headers" \
  --body "$(cat <<'EOF'
Pre-populates the generic CSV import dialog (#1004) with a best-effort guess instead of an all-blank form.

- Backend heuristic in `internal/services/csvmap` (`GuessColumns`, `GuessRatingScale`, `GuessStatusValueMap`) returns a `SuggestedMapping`; `/api/import/csv/inspect` now includes `suggested_mapping`. The guess only seeds the dialog — the submitted mapping is still authoritative.
- Header matching is normalized exact-alias first, then substring-contains; first column wins and a claimed header is not reused. Rating scale is inferred from the column's (uncapped) max; status values are mapped via a synonym table, unmatched → not started.
- The dialog seeds its form from the suggestion and hides columns already claimed by other fields (recomputed from current state, so clearing/reassigning a field frees its column).

Closes #1021
EOF
)"
```

> Note: the `feat:` PR title is the squash commit release-please parses (minor bump). The pre-push hook runs the full Go + frontend suites.

---

## Self-Review

**Spec coverage:**
- Backend placement / `suggested_mapping` on inspect → Tasks 1–2. ✓
- Normalized+alias, exact-then-contains, first-wins/dedup → Task 1 (`GuessColumns` + tests). ✓
- Rating-scale inference from uncapped max → Task 1 (`GuessRatingScale`) + Task 2 (handler tracks `ratingMax`). ✓
- Status-value synonym guessing → Task 1 (`GuessStatusValueMap`) + Task 2 (handler fills from distinct). ✓
- Dialog seeds from suggestion → Task 5. ✓
- Hide already-mapped columns, derived from state, frees on change → Task 4 (`availableHeaders` + frees-on-clear test) + Task 5 (applied in `columnSelect`). ✓
- Tests: Go guess functions, inspect endpoint, frontend helper + dialog → Tasks 1, 2, 4, 5. ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows full code. ✓

**Type consistency:** `SuggestedMapping` (Go) JSON tags match `CsvMapping` (TS). `GuessColumns`/`GuessRatingScale`/`GuessStatusValueMap` names consistent across Tasks 1–2. `usedColumns`/`availableHeaders` consistent across Tasks 4–5. Play-status literals (`completed`, `in_progress`, `not_started`, …) match `PlayStatus` enum values and the engine's status strings. ✓

**Note carried from spec Risks:** `genres`→Tags and `source`→storefront are intentional, low-risk (user confirms); revisit if reports show false positives.
