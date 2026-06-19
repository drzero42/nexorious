# `nexctl` Phase 5 — `import` + `export` Command Groups Design (Epic #1060)

**Status:** Phase 5 of the `nexctl` CLI epic (#1060). Phases 1 (#1081), 2 (#1083), 3 (#1085), the `game list --pool` follow-up (#1086), and 4 (#1087) are merged. This phase adds the `import` and `export` command groups in **one PR** (user-chosen scope).

**Builds on:** merged `internal/cliui` (`Prompt`, `ReadPassword`, `Confirm`, `EncodeJSON`, `FirstNonEmpty`), `internal/cliclient` (`doBearer`, `SearchIGDB`, `GetJobItems`, `Job`/`JobItem` types), and the render/ref-resolution + interactive IGDB-pick helpers from the sync review walker (`cmd/nexctl/sync.go`).

## Problem

`nexctl` can manage the collection, pools, tags, sync, and jobs but cannot **import** a library (Nexorious JSON round-trip, CSV, or a registry migration source like vglist) or **export** the collection. It also cannot resolve the **import** match-review queue — the generic job-item review (`/api/job-items/:id/resolve|skip`) that Phase 4 explicitly deferred here, "where the producer side has context". This phase makes both importable/exportable from the terminal, including interactive import match review.

## Command surface (this phase)

```
import sources                                            [--json|-q]  # registry sources + CSV presets
import nexorious <file>                                   [--json]     # Nexorious JSON round-trip
import csv <file> --inspect                               [--json]     # headers / suggested mapping / detected preset / presets
import csv <file> --preset <slug>                         [--json]     # preset-based (completionator/grouvee/darkadia/nexorious)
import csv <file> --title-col C [mapping flags…]          [--json]     # generic manual mapping (flag-built)
import run <source> <file>                                [--json]     # any registry source (e.g. vglist), validated at runtime
import review <job-id>                                                 # interactive: walk pending_review items
import resolve <item-id> --igdb-id N                                   # non-interactive resolve
import skip <item-id>                                     [-y]         # non-interactive skip

export [--format json|csv] [--out FILE|-] [--no-wait]     [--json]     # trigger → poll → download
```

Both groups register on the `nexctl` root.

## Scope decisions (settled with the user)

- **One PR for the whole phase** (import + export + import-review together).
- **CSV mapping = presets + flags + `--inspect`** (no interactive wizard). The web UI's column-mapping dialog is replaced by: named presets, an `--inspect` that prints the server's headers/suggested-mapping/detected-preset/presets, and column-mapping **flags** that the command assembles into the server's flat `mapping` JSON. Scriptable, matches the flag-driven conventions of the other groups.
- **Import job-item review lands here** (not sync). `import review`/`resolve`/`skip` drive `/api/job-items/:id/resolve|skip` (generic import pipeline + nexorious round-trip). Sync external-games rematch stays on the Phase 4 `sync review`/`resolve`/`skip` — disjoint flows, the server rejects cross-use (`isImportSource` guard).

## Architecture

Pure REST client over the bearer key (`resolveProfile`). New `internal/cliclient` methods; `cmd/nexctl/{import,export}*.go` orchestrate them. `nexctl` keeps importing only stdlib + cobra + `clicfg`/`cliclient`/`cliui`/`cliauth` (no server/DB packages) — the compile-time REST boundary verified in earlier phases. In particular the client does **not** import `internal/services/{csvmap,importsource,importmodel}`; the CSV mapping/inspect shapes are mirrored as local client structs.

### Multipart uploads (new)

Every import endpoint takes a `multipart/form-data` body with a `file` field (50 MB cap server-side); CSV additionally takes `format` and/or `mapping` text fields. `cliclient` gains one helper:

- `doBearerMultipart(method, path, key, filename string, data []byte, fields map[string]string, out any) error` — builds the multipart body (`file` part + each `fields` entry as a form value), sets the boundary content-type + bearer, 2xx-checks, decodes into `out`. Mirrors `doBearer`'s error handling.

### Registry-driven import sources (not hardcoded)

`import run <source> <file>` validates `<source>` against the live registry from `GET /api/import/sources` (`importsource.All()`), erroring with the valid list on a miss — the same runtime-validation pattern as Phase 4's `resolveStorefront` (a static binary must not bake in a server-defined set). `nexorious` and `csv` are **not** registry sources (dedicated endpoints with distinct request shapes), so they get their own subcommands. `import sources` lists the registry sources from the API for `import run`, and names nexctl's own dedicated importers (`nexorious`, `csv`) in its footer. **CSV presets are not separately listable over REST** (the only endpoint returning them, `/api/import/csv/inspect`, requires a file upload), so preset discovery lives on `import csv <file> --inspect`, not on `import sources` — keeping the binary free of a hardcoded preset set.

### CSV mapping flags → server `mapping` JSON

The generic-CSV path assembles the server's flat `mapping` body (the shape `internal/api/import_csv.go` `csvMapping` binds) from flags, marshalled client-side:

| flag | maps to |
|---|---|
| `--title-col` (required for manual) | `columns.title` |
| `--igdb-id-col` | `columns.igdb_id` |
| `--platform-col`, `--storefront-col`, `--acquired-date-col` | `columns.{platform,storefront,acquired_date}` |
| `--rating-col` + `--rating-scale {5,10,100}` | `columns.rating` + `rating_scale` |
| `--hours-col` | `columns.hours_played` |
| `--notes-col`, `--tags-col`, `--loved-col` | `columns.{notes,tags,loved}` |
| `--status-col` + repeated `--status-map raw=canonical` | `status.column` + `status.value_map` |
| `--merge-by-title` | `merge_by_title` |

`--preset` and the manual flags are mutually exclusive; `--inspect` ignores both and just reports. Off the inspect path, a title column (`--title-col` or a preset) is required — surfaced as a client-side error before upload.

### New `cliclient` methods + types

**Import:**
- `ListImportSources(key) ([]ImportSource, error)` — `GET /api/import/sources`.
- `ImportNexorious(key, filename string, data []byte) (*ImportResult, error)` — `POST /api/import/nexorious`.
- `InspectCSV(key, filename string, data []byte) (*CSVInspect, error)` — `POST /api/import/csv/inspect`.
- `ImportCSV(key, filename string, data []byte, format string, mapping json.RawMessage) (*ImportResult, error)` — `POST /api/import/csv` (`format` or `mapping` field; never both meaningfully — preset wins server-side).
- `ImportSource(key, slug, filename string, data []byte) (*ImportResult, error)` — `POST /api/import/:slug`.

**Export:**
- `TriggerExport(key, format string) (*ExportResult, error)` — `POST /api/export/json|csv`.
- `DownloadExport(key, jobID string, w io.Writer) error` — `GET /api/export/:id/download` streamed to `w` (no JSON decode).

**Import review (job-items):**
- `ResolveJobItem(key, id string, igdbID int) error` — `POST /api/job-items/:id/resolve` (`{igdb_id}`).
- `SkipJobItem(key, id string) error` — `POST /api/job-items/:id/skip`.
- (`GetJobItems` from Phase 4 lists the review queue with `?status=pending_review`.)

**Types:** `ImportSource{Slug,DisplayName,Description string,Accept []string,...}` (subset), `ImportResult{JobID,Source,Status,Message string,TotalItems,SkippedCount int}` (`skipped_count` only set by nexorious), `CSVInspect{Headers []string,RowCount int,Columns []CSVColumn,SuggestedMapping CSVSuggestedMapping,Presets []CSVPreset,Detected *CSVPreset}`, `CSVColumn{Name string,DistinctValues []string,DistinctTruncated bool}`, `CSVPreset{Slug,Name string}`, `CSVSuggestedMapping{Columns map-ish subset,Status{Column,ValueMap},RatingScale int,MergeByTitle bool}` (local mirror of `csvmap.SuggestedMapping`’s JSON), `ExportResult{JobID,Status,Message string,EstimatedItems int}`.

## Command behaviour

- **`import sources`** — table of registry sources (SLUG / NAME / DESCRIPTION) plus a CSV-presets section; `--json` raw; `-q` bare slugs.
- **`import nexorious <file>`** — read file, `ImportNexorious`, print `created import job <id> (N games, M skipped)`. 400 (bad/legacy file, IGDB not configured), 409 (active import) surface verbatim.
- **`import csv <file>`** —
  - `--inspect`: `InspectCSV`; print headers, row count, detected preset, available presets, and the suggested column mapping (so the user can build flags). `--json` raw.
  - `--preset <slug>`: `ImportCSV(format=slug)`.
  - manual flags: build `mapping` JSON (title required), `ImportCSV(mapping=…)`.
  - prints the import-job confirmation; 400 surfaces verbatim.
- **`import run <source> <file>`** — resolve `<source>` against the registry, `ImportSource`, print the confirmation. Unknown source errors with the valid list.
- **`import review <job-id>`** — interactive only (errors off-TTY with a hint to use `resolve`/`skip`). `GetJobItems(?status=pending_review)`; for each: print source title + any IGDB candidates; offer **[s]earch IGDB & pick** (reuse `SearchIGDB` by the source title → numbered candidates → `ResolveJobItem(igdb_id)`), **[k]skip** (`SkipJobItem`), **[n]ext**, **[q]uit**. A failed resolve/skip prints the error and advances (the user re-runs the non-interactive verb).
- **`import resolve <item-id> --igdb-id N`** — non-interactive `ResolveJobItem`. 400 (sync-flow item / unsupported source), 409 (not pending review) surface verbatim.
- **`import skip <item-id>`** — confirm unless `-y` → `SkipJobItem`.
- **`export`** — `TriggerExport(--format, default json)`; with `--no-wait` print the job id and return. Otherwise poll `GetJob(id)` until terminal: on `completed`, `DownloadExport` to `--out` (a path, `-` for stdout, or a default `nexorious_export_<job-id>.<ext>` in cwd) and print the path; on `failed`, surface the job error. `--json` (with `--no-wait`) prints the trigger result.

## Cross-cutting conventions (unchanged)

Human table/detail default; `--json`; `-q` bare ids/slugs. Confirms on destructive ops (`import skip`) unless `-y`/non-TTY. `url.PathEscape` on every id/slug path segment. Void client methods pass `nil` out to `doBearer`. Multipart helper mirrors `doBearer` error handling.

## Out of scope (later phases / API limits)

- **`export --filter`** (from the epic surface) — the export API exports the **whole library** unconditionally (`handleExport` counts/exports all `user_games`); there is no server-side filter param, so the REST boundary precludes it. Noted, not implemented.
- `backup` / `admin` / `config` notify (Phase 6), packaging (7), `mcp` (8, blocked on #518).
- An interactive CSV mapping wizard (presets + flags + `--inspect` cover scripted and one-off use).
- `job-items` retry (`POST /api/job-items/:id/retry`) — job-level `job retry` (Phase 4) already re-enqueues failed items; per-item retry adds no CLI surface this phase.
