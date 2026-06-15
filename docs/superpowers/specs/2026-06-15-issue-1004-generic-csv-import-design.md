# Generic user-mapped CSV import (`csv`) — #1004

**Issue:** [#1004](https://github.com/drzero42/nexorious/issues/1004) — second step of the CSV-import track in epic [#984](https://github.com/drzero42/nexorious/issues/984).
**Status:** design approved, ready for an implementation plan.
**Depends on:** [#1014](https://github.com/drzero42/nexorious/issues/1014) — the config-driven `csvmap` engine (merged as PR #1017).

## Problem

A user migrating from a tracker Nexorious has no preset for needs a way to bring a CSV library in. #1014 landed the `csvmap` engine — `Parse(raw []byte, cfg Config) ([]importmodel.Game, error)` — which turns a CSV into canonical games given a `Config`. What's missing is the path that lets a user **build that `Config` interactively**: upload a CSV, map its columns and status values in one screen, and hand the result to the same shared import pipeline every other source uses.

The mapping the user fills in is just a `csvmap.Config` (the simple subset) — the same shape a future preset (#1002/#1003) would supply as baked-in data. The registered import-source slug is **`csv`**.

## Scope

In:

- Two endpoints on a new `internal/api/import_csv.go`:
  - `POST /api/import/csv/inspect` — parse the uploaded CSV and return headers, row count, and per-column distinct values (capped) to drive the dialog.
  - `POST /api/import/csv` — accept the file plus a `mapping` JSON, build a `csvmap.Config`, run `csvmap.Parse`, and hand off to the shared job-creation tail.
- A refactor extracting that job-creation tail out of `handleImportSource` so both registered sources and the CSV path share it.
- A `csvMapping` request DTO and its `toConfig()` translation to `csvmap.Config`.
- Frontend: a hand-rendered **CSV** `ImportCard` (beside the Nexorious-JSON card) that, on file select, inspects then opens a single-screen **mapping dialog** (Layout A, stacked); hooks; types; a small `apiUploadFile` extension to send `file` + `mapping` together.
- Tests: `toConfig` translation, the inspect endpoint, the import endpoint (happy path + guards), and the dialog component.

Out (own issues / explicitly deferred):

- Auto-detecting a known CSV format to skip the dialog (#1015).
- Grouvee / Completionator preset `Config`s (#1002 / #1003).
- Absorbing Darkadia into `csvmap` and the advanced engine features (#1016).
- Canonical platform/storefront fuzzy-matching — v1 stores those values **as-is** (passthrough), preserved as provenance.
- Any recurring/incremental sync (all imports are one-off migrations).
- A separate `created/added` game-date dialog field — v1 exposes only the platform **acquired-date** (the engine's `ColumnMap.CreatedAt` stays unused here; it's a Darkadia concern for #1016).

## Decisions taken in brainstorming

1. **`csv` is not a registry `Mapper`.** The `importsource.Mapper` interface is `Parse(raw)` with no config; the CSV path needs a user-built `Config`. So `csv` is a separate, hand-rendered `ImportCard` wired to two dedicated endpoints — the same pattern the Nexorious-JSON card already uses. It is deliberately absent from the `/import/sources` registry.
2. **Shared `source = "csv"` slug, distinguished by `job_type`.** `JobSourceCSV = "csv"` already exists and is already the CSV **export** source (`export.go`). Reusing it for the CSV import as `{job_type: import, source: csv}` mirrors how `JobSourceNexorious` already backs both JSON import and export. Every query that filters on `source = csv` is `job_type`-scoped (including the active-import conflict check), so import and export never conflate. No rename to `csv-import`/`csv-export`.
3. **Dialog layout: stacked single-screen form (Layout A).** "Map columns" section on top, merge toggle, then a "Map status values" section that appears once a Status column is chosen.
4. **Status-value rows default to `not_started`.** Each distinct value gets a pre-filled row defaulting to "Not started"; the user adjusts. No fuzzy name-guessing in v1 (matches the engine's `unmapped → Default` rule and keeps the logic test-free of heuristics).
5. **Flat DTO → `Config` (not a raw `Config` over the wire).** The request carries a flat, frontend-shaped `csvMapping`; the handler translates it to a `csvmap.Config` expressing only the simple subset, so a frontend bug cannot reach an advanced/rejected engine slot.
6. **Rating default `/5`, round-to-nearest.** The scale select defaults to out-of-5 (Nexorious's own scale); `Truncate` is left `false` (truncate is a Darkadia-specific advanced behaviour, not generic).
7. **IGDB guard on `inspect` too.** Refuse up front so the user is not made to fill out the whole dialog before being told IGDB is unconfigured.

## Architecture & data flow

```
CSV card (hand-rendered, beside "Nexorious JSON")
   │  user selects a .csv file
   ▼
POST /api/import/csv/inspect  ──► { headers, row_count, columns[{name, distinct_values, distinct_truncated}] }
   │
   ▼  open CsvMappingDialog (Layout A, stacked)
   │  user maps columns + rating scale + merge toggle + status-value→play_status rows
   ▼
POST /api/import/csv   (multipart: file + mapping JSON)
   │  handler: mapping → csvmap.Config → csvmap.Parse(raw, cfg) → []importmodel.Game
   ▼
   enqueueImportJob(...)   ← shared tail extracted from handleImportSource
   ▼
   identical pipeline: ImportMatch → pending_review → finalise → additive merge → job/Recent-Activity history
```

No migration, no schema change: `JobSourceCSV` already exists; the canonical `importmodel.Game` and the whole post-mapping pipeline are unchanged.

## Endpoints & wire contract

### `POST /api/import/csv/inspect`

Multipart `file`. Guards: auth; IGDB configured (else 400, same message style as the other imports); 50 MB limit. Parses with stdlib `encoding/csv`, `FieldsPerRecord = -1` (ragged-row tolerant, matching the engine). Response:

```json
{
  "headers": ["Game Name", "System", "Status", "Score"],
  "row_count": 142,
  "columns": [
    { "name": "Game Name", "distinct_values": ["Celeste", "Hades", "..."], "distinct_truncated": true },
    { "name": "Status", "distinct_values": ["Beaten", "Playing", "Backlog"], "distinct_truncated": false }
  ]
}
```

- `row_count` = data rows (excludes the header).
- Per column, `distinct_values` collects distinct **non-empty** trimmed cell values, capped at **50** (first-seen order); `distinct_truncated` is `true` when more than 50 were seen.
- Errors: unreadable / empty / header-less CSV → 400 "could not read CSV header"; file too large → 413.

### `POST /api/import/csv`

Multipart `file` + `mapping` (a JSON string form field). Guards: auth; IGDB configured; 50 MB limit. The `mapping` DTO:

```json
{
  "columns": {
    "title": "Game Name", "platform": "System", "storefront": "",
    "rating": "Score", "notes": "", "acquired_date": "",
    "hours_played": "", "tags": "", "loved": ""
  },
  "status": { "column": "Status", "value_map": { "beaten": "completed", "playing": "in_progress" } },
  "rating_scale": 10,
  "merge_by_title": true
}
```

Go shape:

```go
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
    RatingScale  int  `json:"rating_scale"`  // 5/10/100; ignored if no rating column
    MergeByTitle bool `json:"merge_by_title"`
}
```

Flow: decode `mapping` → `toConfig()` → `csvmap.Parse(body, cfg)` → on `importmodel.ErrInvalidSignature` (cannot happen for the generic `Config`, which has no `Signature`, but handled for symmetry) or other parse error → 400; empty result → 400 "no games found in file"; else `enqueueImportJob(...)`.

### `toConfig()` — the one piece of non-trivial backend logic

Maps the flat DTO onto a `csvmap.Config` (simple subset only):

| DTO field | `csvmap.Config` slot |
|---|---|
| `columns.title` | `Columns.Title` |
| `columns.rating` | `Columns.Rating` |
| `columns.hours_played` | `Columns.HoursPlayed` |
| `columns.tags` | `Columns.Tags` |
| `columns.loved` | `Columns.Loved` |
| `columns.notes` | `Notes.Column` |
| `columns.platform` / `storefront` / `acquired_date` | `Platform.Simple = &PlatformSimple{PlatformColumn, StorefrontColumn, AcquiredDateColumn}` — `PlatformMap`/`StorefrontMap` nil (passthrough). Set **only if** `platform` is non-empty. |
| `status.column` + `status.value_map` | `Status.Column = &StatusColumn{Column, ValueMap, Default: "not_started"}` — set **only if** `status.column` is non-empty; **value-map keys lowercased** (the engine looks up `normKey(cell)`). |
| `rating_scale` (+ rating column) | `Rating = &RatingConfig{Scale: rating_scale, Truncate: false}` — set **only if** `columns.rating` non-empty **and** scale ∈ {5,10,100}; bad scale → 400. |
| `columns.hours_played` | `Duration = &DurationConfig{Format: "decimal"}` — set **only if** `hours_played` non-empty (the engine returns no hours when `Duration == nil`). |
| `merge_by_title` | `Grouping.MergeByTitle` |

`Columns.Title` empty → 400 (the engine also rejects it; the handler validates for a clean message). No advanced slots are ever populated, so the engine never returns its advanced-slot rejection error.

## Backend: shared-tail refactor

Extract the post-mapping body of `handleImportSource` (`import.go:255–330`) into:

```go
func (h *ImportHandler) enqueueImportJob(
    reqCtx context.Context, userID, source, displayName string, games []importmodel.Game,
) (jobID string, total int, err error)
```

It performs: the active-import conflict check (`job_type = import AND source = ? AND status IN (pending, processing)` → `*echo.HTTPError` 409), insert the `processing` job, per-game `job_item` + `ImportMatchArgs` enqueue, flip `dispatch_complete`, and call `tasks.ImportCheckJobCompletion`. Returns an `*echo.HTTPError` for the conflict/internal cases so the handler can `return err` directly. `handleImportSource` becomes "parse via mapper → `enqueueImportJob`"; the CSV handler is "parse via `csvmap` → `enqueueImportJob`". This is a behaviour-preserving refactor for the existing sources, guarded by their current tests. Routes are registered in `router.go` beside the others: `importGroup.POST("/csv/inspect", imh.HandleImportCSVInspect)` and `importGroup.POST("/csv", imh.HandleImportCSV)`.

## Frontend

- **`apiUploadFile`** gains an optional `extraFields?: Record<string, string>` appended to the `FormData` (back-compat; existing single-file callers unaffected). Inspect uses the plain single-file form; import sends `file` + `mapping`.
- **api/types**: `inspectCsv(file)` and `importCsv(file, mapping)` in `api/import-export.ts`; `CsvInspectResponse`, `CsvColumnInfo`, `CsvMapping` in `types/import-export.ts`.
- **hooks**: `useInspectCsv()`, `useImportCsv()` (TanStack mutations).
- **`CsvMappingDialog`** (`components/`): shadcn `Dialog`/`Select`/`Switch`/`Label` (all already present). Layout A:
  - *Map columns*: one labelled `Select` per canonical field (Title required; others list headers + a "— none —" option). Rating row reveals a scale `Select` (`/5` default, `/10`, `/100`) when a rating column is chosen. Merge toggle (`Switch`) default **on**.
  - *Map status values*: rendered only once a Status column is chosen; one row per `distinct_values` entry of that column, each a `play_status` `Select` defaulting to **Not started**. If that column was `distinct_truncated`, show a note that values beyond the cap import as Not started.
  - Footer: Cancel / Import. **Import disabled until Title is mapped.** On Import, assemble the `CsvMapping` (value-map keyed by the raw distinct value; backend lowercases) and POST.
- **`import-export.tsx`**: add the CSV `ImportCard` beside the Nexorious-JSON card; its `onFileSelect` runs inspect (loading state) then opens the dialog. **Pending-review wiring:** the predicate that shows `JobItemsDetails` is `importSourceSlugs.has(activeJob.source)`, and `csv` is deliberately not in the registry-backed `importSources`; include `'csv'` in that predicate (e.g. `importSourceSlugs.has(source) || source === 'csv'`) so CSV imports surface the review box like every other matching source.

## Error handling & edge cases

- IGDB unconfigured → 400 on **both** endpoints.
- Empty / header-less CSV → 400; zero data rows → import 400 "no games found".
- Missing Title mapping → 400 (and the dialog disables Import).
- `rating_scale` ∉ {5,10,100} when a rating column is set → 400.
- Status column with >50 distinct values → dialog shows the capped rows + a note; values beyond the cap import as `not_started` (engine default).
- Duplicate header names → engine's `buildIndex` is first-wins; acceptable.
- Active CSV import already running → 409 (export running on `source=csv` does not trigger it).

## Testing

- **Backend, `toConfig`** (riskiest logic; table-driven): each field lands in the correct slot; value-map keys lowercased; `Rating` present only with column + valid scale, absent otherwise; `Duration` present only with an hours column; platform-simple passthrough with nil maps; `acquired_date` → `PlatformSimple.AcquiredDateColumn`, `notes` → `Notes.Column`; bad scale and empty title → error.
- **Backend, inspect**: headers + row_count; distinct values capped at 50 with `distinct_truncated`; empty cells excluded from distinct; IGDB-unconfigured → 400; header-less → 400.
- **Backend, import**: happy path creates the job + one `job_item` per game and enqueues match (assert rows, mirroring the existing source tests); 409 on a second active CSV import; 400 on missing title / bad scale / empty file. The shared-tail refactor is additionally covered by the existing Darkadia/vglist handler tests staying green.
- **Frontend, `CsvMappingDialog`** (Vitest + testing-library): status rows appear only after a Status column is chosen and default to Not started; Import is disabled until Title is mapped; the assembled `CsvMapping` payload matches the selections (including rating scale and merge toggle).

## Acceptance criteria

- [ ] `POST /api/import/csv/inspect` returns headers + per-column distinct values (capped, with `distinct_truncated`) + row count; refuses when IGDB is unconfigured.
- [ ] A single-screen stacked mapping dialog maps columns, rating scale, status values, and the merge toggle, with status rows defaulting to Not started and Import gated on Title.
- [ ] `POST /api/import/csv` builds a `csvmap.Config` from the mapping and flows through the identical shared pipeline (`enqueueImportJob`) as the other sources.
- [ ] Ambiguous matches flow to the existing `pending_review` surface (`JobItemsDetails` shown for `source = csv`); import refused if IGDB unconfigured.
- [ ] Tests cover the `toConfig` translation, the inspect endpoint, the import endpoint (happy path + guards), and the dialog component; existing Darkadia/vglist import tests stay green after the tail refactor.
