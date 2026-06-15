# Issue #1022 — CSV: carry IGDB ID through import to skip matching

Follow-up to #1004, part of epic #984 (multi-source library import).

## Problem

The generic CSV import always matches games by title (+platform) → IGDB, routing
unconfident matches to `pending_review`. When a CSV already carries a real IGDB
id — most notably when re-importing our own CSV export — that matching round-trip
is wasteful and lossy: the id we exported is discarded on the way back in.

## Verified starting state

- **Export already carries the id.** `internal/worker/tasks/export.go:306` lists
  `igdb_id` as the second CSV column, populated from `ug.GameID`
  (`export.go:400`), which is the IGDB-keyed `games.id` primary key. The issue's
  Part 1 (export) is therefore already done — this work is **import only**.
- **`importmodel.Game`** (`internal/services/importmodel/model.go`) has no IGDB-id
  field. It is the canonical shape every mapper (Darkadia, vglist, generic CSV,
  Nexorious JSON) feeds into the pipeline.
- **`ImportMatchWorker`** (`internal/worker/tasks/import_pipeline.go:44`) matches
  purely by title: `IGDBClient.SearchGames(item.SourceTitle, …)` →
  `matching.Decide`. A confident decision sets `resolved_igdb_id` and enqueues
  `ImportFinalizeArgs`; otherwise the item is marked `pending_review`. There is no
  short-circuit for a pre-supplied id.
- **`ImportFinalizeWorker`** (`import_pipeline.go:128`) reads `resolved_igdb_id`
  and hydrates the game via `ensureGameRow(...)` (`import_pipeline.go:311`), which
  fetches full IGDB metadata and falls back to a title-only stub row if the fetch
  fails. This is the exact path the Nexorious-JSON importer (`ImportItemWorker`)
  already uses to trust an incoming IGDB id.
- The match worker already has everything it needs: each `job_item` carries both
  `SourceTitle` and `SourceMetadata` (the marshaled `importmodel.Game`).
- **csvmap** (`internal/services/csvmap/`), the API DTO (`internal/api/import_csv.go`),
  the auto-guess aliases (`csvmap/guess.go`, #1021), and the frontend
  `CsvMapping` type + `CsvMappingDialog` all lack an IGDB-id field.

## Design

### 1. Canonical model

Add one field to `importmodel.Game`:

```go
IGDBID *int32 `json:"igdb_id,omitempty"`
```

Pointer + `omitempty` so every existing mapper serializes byte-identically — the
field is simply absent when unset, and the short-circuit below stays inert for
sources that never populate it. This is the single shared-surface change the
issue flags ("affects all mapper-based sources, so design carefully"); making it
opt-in via a nil pointer is what keeps it safe.

### 2. csvmap engine

- `ColumnMap` (`config.go`) gains `IGDBID string` — the source column name.
- `extractGame` (`parse.go`) parses that cell with `strconv.Atoi`. Per the agreed
  fallback behaviour: **missing, blank, non-numeric, or ≤0 → leave `IGDBID` nil.**
  Such a row then behaves exactly like any title-only row (normal title→IGDB
  match, possibly `pending_review`). A mixed CSV with the id filled on only some
  rows just works.
- Merge-by-title consolidation carries `IGDBID` first-wins, consistent with how
  the engine already merges other scalar fields.

### 3. API + frontend

- `csvMapping` DTO (`import_csv.go`) gains `columns.igdb_id`; `buildCSVConfig`
  wires it into `ColumnMap.IGDBID`.
- `GuessColumns` (`csvmap/guess.go`) gains an alias entry for the IGDB-id field:
  `["igdb_id", "igdbid", "igdb"]`. Because the export header is literally
  `igdb_id`, re-importing a Nexorious CSV auto-maps the column via #1021's
  auto-guess — closing the round-trip loop without manual mapping.
- Frontend `CsvMapping` type (`ui/frontend/src/types/import-export.ts`) gains
  `columns.igdb_id`; `CsvMappingDialog` renders one more mappable-column row
  labelled "IGDB ID".

### 4. The short-circuit (shared pipeline)

In `ImportMatchWorker.Work`, **before** the IGDB search:

1. Unmarshal `item.SourceMetadata` into `importmodel.Game`.
2. If `IGDBID != nil && *IGDBID > 0`: set `resolved_igdb_id` (and a
   `match_confidence` sentinel, e.g. 1.0) on the job item, enqueue
   `ImportFinalizeArgs`, and return — reusing the same enqueue/return branch the
   confident-match path already uses.
3. Otherwise: fall through to today's title-search logic, unchanged.

`ImportFinalizeWorker` needs no change: it already trusts `resolved_igdb_id` and
hydrates via `ensureGameRow` (fetch metadata, fall back to title-only stub) —
which is the agreed "trust + hydrate" behaviour and identical to the
Nexorious-JSON path.

The short-circuit is inert for Darkadia/vglist and any future mapper, since they
leave `IGDBID` nil. `handleImportSource` already refuses any mapper import when
IGDB is unconfigured (`import.go:299`), so the short-circuit always runs in a
context where finalize can attempt a metadata fetch.

This deliberately uses a real IGDB id, which is distinct from the epic's v1
constraint that *foreign* ids (Wikidata/GiantBomb) are not a match signal — a
real IGDB id is not a foreign id.

### 5. Documentation

After import consumes the id, audit `docs/import-export-format.md` (and any
related guide) for a now-stale "CSV re-import is lossy" claim and tighten it to
reflect that IGDB ids now round-trip through CSV.

## Testing

- **csvmap parse:** id column parsed into `IGDBID`; blank / non-numeric / ≤0 →
  nil; merge-by-title carries the id first-wins.
- **guess:** an `igdb_id` header maps to the IGDB-id field.
- **match worker:** a job item whose `SourceMetadata` carries a valid id skips the
  IGDB search and enqueues finalize with `resolved_igdb_id` set; a nil id leaves
  the existing title-search path unchanged.
- **frontend:** the dialog exposes the IGDB-ID mappable row (light test).

## Out of scope

- Export changes (already done).
- Verifying the supplied id against IGDB before trusting it (explicitly rejected
  in favour of the existing trust + hydrate pattern; an unrecognised id degrades
  to a title-only stub, same as the Nexorious-JSON path).
- Foreign-id (Wikidata/GiantBomb) matching — out by the epic's v1 decision.
