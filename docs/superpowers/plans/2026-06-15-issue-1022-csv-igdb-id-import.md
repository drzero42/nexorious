# CSV IGDB-ID Import Passthrough Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the generic CSV import map an "IGDB ID" column and, when a row carries a valid IGDB id, hydrate the game directly from it — skipping the title→IGDB matching / `pending_review` step.

**Architecture:** Add an optional `IGDBID *int32` to the canonical `importmodel.Game`. The csvmap engine extracts it from a configurable column; the API DTO/auto-guess/frontend dialog expose it as a mappable field. The shared `ImportMatchWorker` short-circuits: if the item's payload carries a valid id (>0), it sets `resolved_igdb_id` and enqueues finalize directly (the existing confident-match branch). Finalize is unchanged — it already trusts `resolved_igdb_id` and hydrates via `ensureGameRow` (fetch metadata, fall back to a title-only stub). The field is nil for every other mapper, so the short-circuit is inert for them.

**Tech Stack:** Go (Bun, River, testcontainers), React + TypeScript (Vitest), `encoding/csv`.

**Spec:** `docs/superpowers/specs/2026-06-15-issue-1022-csv-igdb-id-import-design.md`

---

## File Structure

- `internal/services/importmodel/model.go` — add `IGDBID *int32` to `Game` (canonical, shared).
- `internal/services/csvmap/config.go` — add `IGDBID string` to `ColumnMap`.
- `internal/services/csvmap/parse.go` — `extractIGDBID` helper; wire into `extractGame`; carry first-wins in `buildMerged`.
- `internal/services/csvmap/parse_test.go` — engine tests.
- `internal/services/csvmap/guess.go` — add IGDB-id alias entry to `fieldAliases` + field to `SuggestedMapping`.
- `internal/services/csvmap/guess_test.go` — guess test.
- `internal/api/import_csv.go` — add `igdb_id` to `csvMapping` DTO; wire into `buildCSVConfig`.
- `internal/api/import_csv_test.go` — `buildCSVConfig` test (new file).
- `internal/worker/tasks/import_pipeline.go` — short-circuit in `ImportMatchWorker.Work`.
- `internal/worker/tasks/import_pipeline_test.go` — short-circuit test.
- `ui/frontend/src/types/import-export.ts` — add `igdb_id` to `CsvMapping.columns`.
- `ui/frontend/src/components/import/csv-mapping.ts` — add `igdb_id` to `emptyCsvMapping`.
- `ui/frontend/src/components/import/csv-mapping-dialog.tsx` — add to `OPTIONAL_FIELDS`.
- `ui/frontend/src/components/import/csv-mapping.test.ts` — assert `emptyCsvMapping` includes the field.
- `docs/import-export-format.md` — correct the stale "CSV is not round-trippable" claim.

---

### Task 1: Add `IGDBID` to the canonical import model

No standalone test — this is a plain optional struct field with no logic; it is exercised by every downstream task (csvmap parse, match worker). The compile + downstream tests are the gate.

**Files:**
- Modify: `internal/services/importmodel/model.go:16-26`

- [ ] **Step 1: Add the field**

In `internal/services/importmodel/model.go`, add `IGDBID` to the `Game` struct (place it right after `Title` so it reads as identity-first):

```go
type Game struct {
	Title          string     `json:"title"`
	IGDBID         *int32     `json:"igdb_id,omitempty"` // when set (>0), import hydrates directly and skips title matching
	PlayStatus     string     `json:"play_status"`
	IsLoved        bool       `json:"is_loved"`
	PersonalRating *int32     `json:"personal_rating,omitempty"`
	PersonalNotes  *string    `json:"personal_notes,omitempty"`
	CreatedAt      string     `json:"created_at,omitempty"` // "2006-01-02" or ""
	Platforms      []Platform `json:"platforms"`
	Tags           []string   `json:"tags,omitempty"`
	HoursPlayed    *float64   `json:"hours_played,omitempty"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/services/importmodel/...`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/services/importmodel/model.go
git commit -m "feat: add optional IGDBID to canonical import model (#1022)"
```

---

### Task 2: csvmap — extract the IGDB id from a configured column

**Files:**
- Modify: `internal/services/csvmap/config.go:26-33`
- Modify: `internal/services/csvmap/parse.go` (add `extractIGDBID`; wire into `extractGame`; carry in `buildMerged`)
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/services/csvmap/parse_test.go`:

```go
func TestParse_IGDBIDExtracted(t *testing.T) {
	raw := []byte("title,igdb_id\nAnodyne,42\n")
	cfg := Config{
		Columns: ColumnMap{Title: "title", IGDBID: "igdb_id"},
	}
	games, err := Parse(raw, cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1", len(games))
	}
	if games[0].IGDBID == nil || *games[0].IGDBID != 42 {
		t.Fatalf("IGDBID = %v, want 42", games[0].IGDBID)
	}
}

func TestParse_IGDBIDInvalidLeftNil(t *testing.T) {
	// blank, non-numeric, zero, and negative all yield a nil IGDBID -> the row
	// falls back to title matching downstream.
	cases := []string{"", "abc", "0", "-3"}
	for _, v := range cases {
		raw := []byte("title,igdb_id\nAnodyne," + v + "\n")
		cfg := Config{Columns: ColumnMap{Title: "title", IGDBID: "igdb_id"}}
		games, err := Parse(raw, cfg)
		if err != nil {
			t.Fatalf("Parse(%q): %v", v, err)
		}
		if len(games) != 1 {
			t.Fatalf("Parse(%q): got %d games, want 1", v, len(games))
		}
		if games[0].IGDBID != nil {
			t.Errorf("Parse(%q): IGDBID = %v, want nil", v, *games[0].IGDBID)
		}
	}
}

func TestParse_IGDBIDUnconfiguredLeftNil(t *testing.T) {
	raw := []byte("title,igdb_id\nAnodyne,42\n")
	cfg := Config{Columns: ColumnMap{Title: "title"}} // IGDBID column not mapped
	games, err := Parse(raw, cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if games[0].IGDBID != nil {
		t.Errorf("IGDBID = %v, want nil", *games[0].IGDBID)
	}
}

func TestParse_IGDBIDMergeFirstWins(t *testing.T) {
	// Two rows, same title, different ids; merge keeps the first row's id.
	raw := []byte("title,igdb_id\nAnodyne,42\nAnodyne,99\n")
	cfg := Config{
		Columns:  ColumnMap{Title: "title", IGDBID: "igdb_id"},
		Grouping: GroupingConfig{MergeByTitle: true},
	}
	games, err := Parse(raw, cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1 (merged)", len(games))
	}
	if games[0].IGDBID == nil || *games[0].IGDBID != 42 {
		t.Fatalf("IGDBID = %v, want 42 (first wins)", games[0].IGDBID)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run TestParse_IGDBID -v`
Expected: FAIL — `ColumnMap` has no field `IGDBID` (compile error), or `IGDBID` is nil.

- [ ] **Step 3: Add the config field**

In `internal/services/csvmap/config.go`, add `IGDBID` to `ColumnMap`:

```go
// ColumnMap maps each plain scalar canonical field to its source header name.
type ColumnMap struct {
	Title       string // required
	IGDBID      string // optional; when a row's value parses to a positive int, import hydrates directly and skips title matching
	Rating      string
	CreatedAt   string // game "added"/created date
	HoursPlayed string
	Tags        string
	Loved       string
}
```

- [ ] **Step 4: Add the extractor and wire it in**

In `internal/services/csvmap/parse.go`, add an `extractIGDBID` helper (place it near `extractHours`):

```go
// extractIGDBID parses the configured IGDB-id cell into a positive int32. A
// missing/unconfigured column, blank, non-numeric, zero, or negative value
// yields nil — such a row falls back to title matching downstream.
func extractIGDBID(rec []string, idx map[string]int, cfg Config) *int32 {
	raw := cell(rec, idx, cfg.Columns.IGDBID)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return nil
	}
	v := int32(n)
	return &v
}
```

Then set it in `extractGame` (right after the `Title`/`PlayStatus` assignment, before the rating block):

```go
	g := importmodel.Game{
		Title:      title,
		PlayStatus: extractStatus(rec, idx, cfg),
	}
	g.IGDBID = extractIGDBID(rec, idx, cfg)
	if r := extractRating(cell(rec, idx, cfg.Columns.Rating), cfg); r != nil {
		g.PersonalRating = r
	}
```

Merge-by-title needs no change: `buildMerged` keeps the first row's whole `Game` (including `IGDBID`) as the scalar baseline and only unions `Platforms`, so first-wins already holds. The `TestParse_IGDBIDMergeFirstWins` test guards this.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run TestParse_IGDBID -v`
Expected: PASS (all four).

- [ ] **Step 6: Run the full csvmap package to catch regressions**

Run: `go test ./internal/services/csvmap/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/services/csvmap/config.go internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap extracts IGDB id from a mapped column (#1022)"
```

---

### Task 3: csvmap — auto-guess the IGDB-id column

**Files:**
- Modify: `internal/services/csvmap/guess.go:12-30` (`SuggestedMapping`), `:48-62` (`fieldAliases`)
- Test: `internal/services/csvmap/guess_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/services/csvmap/guess_test.go`:

```go
func TestGuessColumns_IGDBID(t *testing.T) {
	// The Nexorious CSV export header is literally "igdb_id".
	m := GuessColumns([]string{"title", "igdb_id", "play_status"})
	if m.Columns.IGDBID != "igdb_id" {
		t.Errorf("IGDBID guess = %q, want %q", m.Columns.IGDBID, "igdb_id")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestGuessColumns_IGDBID -v`
Expected: FAIL — `SuggestedMapping.Columns` has no field `IGDBID` (compile error).

- [ ] **Step 3: Add the field to `SuggestedMapping`**

In `internal/services/csvmap/guess.go`, add `IGDBID` to the `SuggestedMapping.Columns` struct (mirror the placement and JSON tag used by the DTO — `igdb_id`, after `title`):

```go
	Columns struct {
		Title        string `json:"title"`
		IGDBID       string `json:"igdb_id"`
		Platform     string `json:"platform"`
		Storefront   string `json:"storefront"`
		Rating       string `json:"rating"`
		Notes        string `json:"notes"`
		AcquiredDate string `json:"acquired_date"`
		HoursPlayed  string `json:"hours_played"`
		Tags         string `json:"tags"`
		Loved        string `json:"loved"`
	} `json:"columns"`
```

- [ ] **Step 4: Add the alias entry**

In `fieldAliases`, add an entry. Put it right after the `Title` entry so the IGDB id is claimed before the broad `contains` pass touches other fields:

```go
	{func(m *SuggestedMapping, v string) { m.Columns.Title = v }, []string{"name", "title", "game", "gamename", "gametitle"}},
	{func(m *SuggestedMapping, v string) { m.Columns.IGDBID = v }, []string{"igdbid", "igdb"}},
	{func(m *SuggestedMapping, v string) { m.Status.Column = v }, []string{"status", "playstatus", "state", "progress", "completionstatus"}},
```

Note: `normalizeHeader` strips the underscore, so `"igdb_id"` normalizes to `"igdbid"` — matched by the exact-normalized first pass. `"igdb"` covers a bare `igdb` header via the contains pass.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestGuessColumns_IGDBID -v`
Expected: PASS.

- [ ] **Step 6: Run the full csvmap package**

Run: `go test ./internal/services/csvmap/`
Expected: PASS (no existing guess test regressions).

- [ ] **Step 7: Commit**

```bash
git add internal/services/csvmap/guess.go internal/services/csvmap/guess_test.go
git commit -m "feat: auto-guess the IGDB-id CSV column (#1022)"
```

---

### Task 4: API — accept and wire the `igdb_id` mapping field

**Files:**
- Modify: `internal/api/import_csv.go:23-41` (`csvMapping` DTO), `:46-98` (`buildCSVConfig`)
- Test: `internal/api/import_csv_test.go` (new file)

- [ ] **Step 1: Write the failing test**

Create `internal/api/import_csv_test.go`:

```go
package api

import "testing"

func TestBuildCSVConfig_IGDBIDColumn(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "title"
	m.Columns.IGDBID = "igdb_id"

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Columns.IGDBID != "igdb_id" {
		t.Errorf("cfg.Columns.IGDBID = %q, want %q", cfg.Columns.IGDBID, "igdb_id")
	}
}

func TestBuildCSVConfig_IGDBIDOmittedWhenUnmapped(t *testing.T) {
	var m csvMapping
	m.Columns.Title = "title"

	cfg, err := buildCSVConfig(m)
	if err != nil {
		t.Fatalf("buildCSVConfig: %v", err)
	}
	if cfg.Columns.IGDBID != "" {
		t.Errorf("cfg.Columns.IGDBID = %q, want empty", cfg.Columns.IGDBID)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/api/ -run TestBuildCSVConfig_IGDBID -v`
Expected: FAIL — `m.Columns.IGDBID` undefined (compile error).

- [ ] **Step 3: Add the DTO field**

In `internal/api/import_csv.go`, add `IGDBID` to the `csvMapping.Columns` struct (after `Title`, JSON tag `igdb_id`):

```go
	Columns struct {
		Title        string `json:"title"`
		IGDBID       string `json:"igdb_id"`
		Platform     string `json:"platform"`
		Storefront   string `json:"storefront"`
		Rating       string `json:"rating"`
		Notes        string `json:"notes"`
		AcquiredDate string `json:"acquired_date"`
		HoursPlayed  string `json:"hours_played"`
		Tags         string `json:"tags"`
		Loved        string `json:"loved"`
	} `json:"columns"`
```

- [ ] **Step 4: Wire it into `buildCSVConfig`**

In `buildCSVConfig`, add `IGDBID` to the initial `ColumnMap` literal:

```go
	cfg := csvmap.Config{
		Columns: csvmap.ColumnMap{
			Title:  m.Columns.Title,
			IGDBID: m.Columns.IGDBID,
			Tags:   m.Columns.Tags,
			Loved:  m.Columns.Loved,
		},
		Grouping: csvmap.GroupingConfig{MergeByTitle: m.MergeByTitle},
	}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/api/ -run TestBuildCSVConfig_IGDBID -v`
Expected: PASS (both).

- [ ] **Step 6: Commit**

```bash
git add internal/api/import_csv.go internal/api/import_csv_test.go
git commit -m "feat: accept igdb_id in the CSV import mapping (#1022)"
```

---

### Task 5: Pipeline — short-circuit matching when a valid IGDB id is present

**Files:**
- Modify: `internal/worker/tasks/import_pipeline.go:44-57` (`ImportMatchWorker.Work`, before the IGDB-client guard)
- Test: `internal/worker/tasks/import_pipeline_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/worker/tasks/import_pipeline_test.go`. With `IGDBClient: nil` the pre-existing behaviour is `pending_review`; a payload carrying a valid id must instead set `resolved_igdb_id` (proving the search path was skipped before the nil-client guard). `RiverClient: nil` means the finalize enqueue fails afterwards, but `resolved_igdb_id` is written first, so we assert on that.

```go
func TestImportMatch_IGDBIDShortCircuits(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-match-igdbid"
	insertTestUser(t, testDB, userID)
	payload := map[string]any{
		"title":       "Anodyne",
		"igdb_id":     42,
		"play_status": "not_started",
		"platforms":   []map[string]any{},
	}
	_, itemID := insertImportItem(t, userID, payload)

	// Nil IGDB client: without the short-circuit this item would go
	// pending_review. With it, the id is trusted and resolved directly.
	w := &tasks.ImportMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.ImportMatchArgs]{Args: tasks.ImportMatchArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("match: %v", err)
	}

	var resolved sql.NullInt64
	if err := testDB.NewRaw(`SELECT resolved_igdb_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &resolved); err != nil {
		t.Fatal(err)
	}
	if !resolved.Valid || resolved.Int64 != 42 {
		t.Fatalf("resolved_igdb_id = %v, want 42", resolved)
	}

	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status == "pending_review" {
		t.Errorf("status = pending_review; short-circuit should have skipped matching")
	}
}

func TestImportMatch_NoIGDBIDStillPendingReview(t *testing.T) {
	// A payload with no igdb_id and a nil client keeps the existing behaviour.
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-match-noid"
	insertTestUser(t, testDB, userID)
	payload := map[string]any{"title": "Whatever", "play_status": "not_started", "platforms": []map[string]any{}}
	_, itemID := insertImportItem(t, userID, payload)

	w := &tasks.ImportMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.ImportMatchArgs]{Args: tasks.ImportMatchArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("match: %v", err)
	}
	var status string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status); err != nil {
		t.Fatal(err)
	}
	if status != "pending_review" {
		t.Errorf("status = %q, want pending_review", status)
	}
}
```

- [ ] **Step 2: Run to verify the new test fails**

Run: `go test ./internal/worker/tasks/ -run TestImportMatch_IGDBIDShortCircuits -v`
Expected: FAIL — `resolved_igdb_id` is NULL (item went pending_review; no short-circuit yet).

- [ ] **Step 3: Add the short-circuit**

In `internal/worker/tasks/import_pipeline.go`, in `ImportMatchWorker.Work`, insert the short-circuit immediately after the `ctx = logging.WithJobID(...)` line and **before** the `if w.IGDBClient == nil` guard:

```go
	// Correlate every line below to the parent import job.
	ctx = logging.WithJobID(ctx, item.JobID)

	// Short-circuit: a row that already carries a real IGDB id (e.g. a
	// re-imported Nexorious CSV export) needs no title matching. Trust it and
	// hand straight to finalize, which hydrates via ensureGameRow (falling back
	// to a title-only stub if IGDB doesn't recognize the id). Inert for sources
	// that never set IGDBID. A nil/<=0 id falls through to title matching below.
	var payload importmodel.Game
	if err := json.Unmarshal(item.SourceMetadata, &payload); err == nil &&
		payload.IGDBID != nil && *payload.IGDBID > 0 {
		if _, err := w.DB.NewRaw(
			`UPDATE job_items SET resolved_igdb_id = ?, match_confidence = 1 WHERE id = ?`,
			*payload.IGDBID, item.ID,
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "import_match: set resolved id from payload", logging.KeyErr, err, "item_id", item.ID, logging.Cat(logging.CategoryDB))
		}
		if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, item.ID, ImportFinalizeArgs{JobItemID: item.ID}); err != nil {
			slog.WarnContext(ctx, "import_match: enqueue finalize (igdb-id short-circuit)", logging.KeyErr, err, "item_id", item.ID, logging.Cat(logging.CategoryDB))
			ImportCheckJobCompletion(w.DB, item.JobID)
		}
		return nil
	}

	if w.IGDBClient == nil || !w.IGDBClient.Configured() {
```

`importmodel` and `encoding/json` are already imported in this file — no new imports.

- [ ] **Step 4: Run to verify the new test passes**

Run: `go test ./internal/worker/tasks/ -run TestImportMatch_IGDBIDShortCircuits -v`
Expected: PASS.

- [ ] **Step 5: Run the no-id regression test and the full match suite**

Run: `go test ./internal/worker/tasks/ -run TestImportMatch -v`
Expected: PASS (`TestImportMatch_IGDBIDShortCircuits`, `TestImportMatch_NoIGDBIDStillPendingReview`, `TestImportMatch_NoIGDBClientMarksPendingReview`).

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/import_pipeline.go internal/worker/tasks/import_pipeline_test.go
git commit -m "feat: skip title matching when an import row carries an IGDB id (#1022)"
```

---

### Task 6: Frontend — expose the IGDB-id mappable column

**Files:**
- Modify: `ui/frontend/src/types/import-export.ts:46-64` (`CsvMapping`)
- Modify: `ui/frontend/src/components/import/csv-mapping.ts:5-22` (`emptyCsvMapping`)
- Modify: `ui/frontend/src/components/import/csv-mapping-dialog.tsx:29-38` (`OPTIONAL_FIELDS`)
- Test: `ui/frontend/src/components/import/csv-mapping.test.ts`

- [ ] **Step 1: Write the failing test**

Add to `ui/frontend/src/components/import/csv-mapping.test.ts`:

```ts
import { emptyCsvMapping } from './csv-mapping';

it('emptyCsvMapping includes an igdb_id column slot', () => {
  expect(emptyCsvMapping().columns.igdb_id).toBe('');
});
```

(Use the existing import/`describe` style in the file — if the file already imports `emptyCsvMapping`, do not duplicate the import.)

- [ ] **Step 2: Run to verify it fails**

Run (from `ui/frontend/`): `npm run test csv-mapping.test.ts`
Expected: FAIL — `columns.igdb_id` is missing (type error / undefined).

- [ ] **Step 3: Add the field to the `CsvMapping` type**

In `ui/frontend/src/types/import-export.ts`, add `igdb_id` to `CsvMapping.columns` (after `title`):

```ts
export interface CsvMapping {
  columns: {
    title: string;
    igdb_id: string;
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

- [ ] **Step 4: Add the field to `emptyCsvMapping`**

In `ui/frontend/src/components/import/csv-mapping.ts`, add `igdb_id: ''` to the `columns` object (after `title`):

```ts
    columns: {
      title: '',
      igdb_id: '',
      platform: '',
      storefront: '',
      rating: '',
      notes: '',
      acquired_date: '',
      hours_played: '',
      tags: '',
      loved: '',
    },
```

- [ ] **Step 5: Add the dialog row**

In `ui/frontend/src/components/import/csv-mapping-dialog.tsx`, add an entry to `OPTIONAL_FIELDS` (first, so it renders directly under Title/Play status):

```ts
const OPTIONAL_FIELDS = [
  { key: 'igdb_id', label: 'IGDB ID' },
  { key: 'platform', label: 'Platform' },
  { key: 'storefront', label: 'Storefront' },
  { key: 'rating', label: 'Rating' },
  { key: 'notes', label: 'Notes' },
  { key: 'acquired_date', label: 'Acquired date' },
  { key: 'hours_played', label: 'Hours played' },
  { key: 'tags', label: 'Tags' },
  { key: 'loved', label: 'Loved' },
] as const;
```

`OptionalKey` is derived from `OPTIONAL_FIELDS`, and the row renderer already maps `mapping.columns[f.key]` over every entry, so no other dialog code changes. The rating-scale special case keys off `f.key === 'rating'` and is unaffected.

- [ ] **Step 6: Run to verify the test passes**

Run (from `ui/frontend/`): `npm run test csv-mapping.test.ts`
Expected: PASS.

- [ ] **Step 7: Type-check, dead-code check, and dialog tests**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test csv-mapping-dialog.test.tsx`
Expected: no type errors, no knip findings, dialog tests PASS.

- [ ] **Step 8: Commit**

```bash
git add ui/frontend/src/types/import-export.ts ui/frontend/src/components/import/csv-mapping.ts ui/frontend/src/components/import/csv-mapping-dialog.tsx ui/frontend/src/components/import/csv-mapping.test.ts
git commit -m "feat: add IGDB ID column to the CSV import mapping dialog (#1022)"
```

---

### Task 7: Docs — correct the stale "CSV is not round-trippable" claim

**Files:**
- Modify: `docs/import-export-format.md:250-260`

This doc is a reference doc (not embedded/served), so there is no embed or in-app link concern.

- [ ] **Step 1: Update the CSV export section**

Replace the paragraph at `docs/import-export-format.md:250-260` ("CSV export (separate convenience format)") so it reflects that a generic CSV import now exists (#1004) and that the IGDB id round-trips (#1022). Use this text:

```markdown
## CSV export and import

Alongside the JSON interchange format, Nexorious can export a **CSV** file and
import a user-mapped CSV. CSV is a **human-oriented convenience format**: it is
*not* governed by this specification and flattens per-platform data into
semicolon-joined cells with a game-level `hours_played` total for readability.
Its columns are `title`, `igdb_id`, `play_status`, `personal_rating`,
`is_loved`, `hours_played`, `personal_notes`, `platforms`, `tags`, `created_at`,
and `updated_at` (the `release_year` column was removed — release info is
IGDB-sourced).

CSV re-import is **partial, not lossless**: per-platform detail and storefront
links are flattened on export and cannot be fully reconstructed. The one piece
that does round-trip cleanly is game identity — mapping the exported `igdb_id`
column on import hydrates each game directly from IGDB and skips title matching.
For moving a full library between instances, use the JSON format, which
round-trips losslessly.
```

- [ ] **Step 2: Verify the rendered Markdown reads cleanly**

Run: `git diff docs/import-export-format.md`
Expected: the old "no import counterpart … not round-trippable" sentence is gone; the new two-paragraph section is present. (No automated docs test; visual check only.)

- [ ] **Step 3: Commit**

```bash
git add docs/import-export-format.md
git commit -m "docs: CSV import now round-trips IGDB id (#1022)"
```

---

### Task 8: Full verification

- [ ] **Step 1: Run the Go suites for touched packages**

Run: `go test ./internal/services/csvmap/ ./internal/api/ ./internal/worker/tasks/ ./internal/services/importmodel/`
Expected: PASS.

- [ ] **Step 2: Run the frontend gates**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: no type errors, no knip findings, all tests PASS.

- [ ] **Step 3: Build the backend**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 4: Push and open the PR**

The pre-push hook runs the full suites. Open a PR titled `feat: carry IGDB id through CSV import to skip matching (#1022)` with body containing `Closes #1022`.

---

## Notes for the implementer

- **Why the short-circuit precedes the IGDB-client guard:** a trusted id needs no IGDB *search*; finalize's `ensureGameRow` still attempts a metadata fetch and degrades to a title-only stub if IGDB is unreachable — identical to the Nexorious-JSON path. In production, `handleImportSource`/`csvIGDBGuard` already require IGDB to be configured for any CSV import, so the stub fallback is an edge case, not the norm.
- **Inert for other sources:** Darkadia, vglist, and any future mapper never set `IGDBID`, so `payload.IGDBID` unmarshals to nil and the short-circuit is skipped. This is the "design carefully — affects all mapper-based sources" point from the issue: safety comes from the field being an opt-in nil pointer.
- **Not a foreign id:** this uses a real IGDB id, which is explicitly distinct from the epic's v1 "Wikidata/GiantBomb ids are not a match signal" constraint.
```
