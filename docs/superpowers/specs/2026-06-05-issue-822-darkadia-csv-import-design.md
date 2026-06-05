# Darkadia CSV Import — Design

**Issue:** #822
**Format reference (source of truth for the field mapping):** [`docs/darkadia-import.md`](../../darkadia-import.md)
**Date:** 2026-06-05

This spec describes *how* to build the Darkadia CSV importer on top of the existing
`jobs`/`job_items` import framework. The Darkadia format and the field-mapping rules
themselves are owned by `docs/darkadia-import.md`; this document does not restate them
except where an implementation decision depends on one. Where the two disagree, the
format doc wins on *mapping*, this doc wins on *pipeline shape*.

---

## Goal

A one-off, in-app migration of a Darkadia collection export into the Nexorious library:
upload a Darkadia CSV on the Import/Export page, match each game to IGDB (reusing the
existing matching primitives and `pending_review` review experience), and write
`user_games` + `user_game_platforms` with additive, non-destructive merge semantics. No
recurring sync, no persistent connection, no new storefronts, no new staging tables.

---

## Verified groundwork

These facts were confirmed against the codebase before writing this spec; the design
relies on them.

- **Import framework template.** The Nexorious-JSON importer (`POST /api/import/nexorious`,
  `internal/api/import.go`) parses the upload **synchronously in the handler**, creates one
  `job_item` per game (`source_metadata` carries the payload), and enqueues one River
  `import_item` task each. It trusts an embedded `igdb_id` and does **no** matching — so it
  is the template for *upload + finalize*, but Darkadia additionally needs a *match* stage.
- **Matching primitives** are reusable and clean: `igdb.Client.SearchGames(ctx, query, limit, platformIDs)`
  (`internal/services/igdb/igdb.go`), `matching.FuzzyConfidence(query, title) float64`
  and `matching.NormalizeTitle(s)` (`internal/services/matching/`).
- **Auto-resolve decision is welded to sync.** The thresholds (`autoResolveThreshold = 0.85`,
  `tieEpsilon = 0.01`) and the "confident & unambiguous" logic live inline in
  `IGDBMatchWorker` (`internal/worker/tasks/sync.go`) and write to `external_games`. They
  must be **extracted** to be reused without the `external_games` coupling.
- **`igdb.Client.Configured()`** reports IGDB readiness (true only when `IGDB_CLIENT_ID`
  and `IGDB_CLIENT_SECRET` are set) — this is the up-front guard.
- **Resolve/skip are sync-only today.** They exist as `POST /api/sync/external-games/:id/rematch`
  and `POST /api/sync/ignored/:id`, both bound to `external_games`. Generic, import-scoped
  resolve/skip on a `job_item` are **net-new**.
- **`IGDBMatchDialog`** (`ui/frontend/src/components/sync/igdb-match-dialog.tsx`) takes an
  **optional** `externalGameId`, so it is reusable for imports as-is. `JobItemsDetails`
  (`ui/frontend/src/components/jobs/job-items-details.tsx`) currently exposes only retry.
- **Data model is complete.** Every `play_status` value used by the mapping
  (`not_started`, `in_progress`, `completed`, `mastered`, `dominated`, `shelved`, `dropped`),
  `ownership_status = owned`, every platform slug, and every storefront slug in the format
  doc **already exist** in the seed (`internal/db/migrations/20260503000001_initial.up.sql`).
  Nothing new is seeded. The `user_game_platforms` unique index is
  `(user_game_id, platform, storefront) NULLS NOT DISTINCT` — `(platform, NULL)` can exist
  only once per game, matching the doc's de-dup rule.
- **`JobSourceCSV = "csv"` is in use** by the Nexorious **CSV export** (`internal/api/export.go:100`,
  `HandleExportCSV`). It is **not** repurposed for this feature; the Darkadia import gets its
  own `JobSourceDarkadia = "darkadia"`.

---

## Design decisions

Three decisions are not fully determined by the format doc and were confirmed with the
maintainer:

1. **Translate at parse time (Decision A).** All Darkadia→Nexorious translation — platform
   string → slug, `Copy source`/media → storefront, provenance-note assembly, `play_status`
   precedence, rating truncation, `created_at` from `Added` — runs in the parse step. Each
   `job_item`'s `source_metadata` therefore stores the **finished Nexorious-shaped payload**,
   and the finalize stage does no Darkadia-specific work (just IGDB metadata fetch + DB writes).
   This matches the format doc's "provenance lines already assembled".

2. **Unmapped platform string → preserve in the note, never fail (Decision B).** The
   platform-mapping table is complete for the frozen format, so an unmapped string is a
   should-never-happen guard. When it occurs, the consolidator emits **no**
   `user_game_platform` row for that string, appends a provenance line to `personal_notes`
   recording the original string, and imports the game normally with whatever other platforms
   mapped. If *every* platform is unmapped, the game imports platform-less (the no-platform
   case) with the original string(s) preserved in the note. `Parse` never returns an error for
   this; it mirrors how unrecognized storefronts are already preserved in notes.

3. **Keep `JobSourceCSV` (Decision C).** It backs the CSV export, so it stays. Add a new
   `JobSourceDarkadia = "darkadia"` constant for the import.

---

## Pipeline

Four stages, all on the existing `jobs`/`job_items` tables. No `external_games`, no synthetic
storefront, no `UserGameWorker`/sync-rematch reuse.

```
Upload handler (synchronous)        darkadia_match worker        darkadia_finalize worker
────────────────────────────        ─────────────────────        ────────────────────────
guard: IGDBClient.Configured()      SearchGames(title)           ensure games row
guard: header == 29 canonical cols  matching.Decide(title, …)      (FetchFullMetadata +
darkadia.Parse(bytes) → []Game      ├ confident & unambiguous →    cover download — reuse
create job (source=darkadia,        │   resolved_igdb_id set,       import_item helpers)
  dispatch_complete=true,           │   enqueue finalize           write user_game (ADDITIVE)
  total_items=N)                    └ else → store candidates,     write user_game_platforms
per game → job_item                     match_confidence,            (merge + dedup on
  (source_metadata = finished           status = pending_review      (platform, storefront))
   Nexorious payload),                                             record change row
  enqueue darkadia_match                                          mark item completed + result
                                                                  checkJobCompletion()
```

**Manual resolution** (user picks a candidate in the review UI) calls the resolve endpoint,
which sets `resolved_igdb_id` and enqueues **the same finalize worker**. Finalize is the
single write path for both automatic and manual resolution.

Matching is rate-limited (IGDB ~4 req/s); ~1,474 games take several minutes. That is expected
and handled by River concurrency + the IGDB rate limiter, exactly as sync does. Only matching
is slow; parsing is fast and in-memory, which is why it stays in the handler.

---

## Components

### 1. `internal/services/darkadia` — the consolidation core (pure, heavily tested)

`func Parse(raw []byte) ([]Game, error)` — the heart of the feature, with no DB or IGDB
dependency so it is exhaustively unit-testable.

- Uses `encoding/csv` with `FieldsPerRecord = -1` (ragged rows; missing trailing columns =
  empty) and honours quoted multi-line `Notes` (cannot be parsed line-by-line).
- **Header validation** belongs here (or a thin exported helper it shares with the handler):
  parse row 1 and compare its 29 field values to the canonical slice
  (`Name, Added, Loved, …, Platforms, Notes`). Compare *values*, not the raw line — quoting is
  incidental (only space-containing headers are quoted in the real export). Mismatch → a typed
  error the handler turns into "not a Darkadia export".
- Groups rows: a non-empty `Name` starts a game; following empty-`Name` rows attach as copies.
- Produces, per game, the finished Nexorious payload:
  - `play_status` via the precedence table (format doc Part 2).
  - `is_loved` (bool), `personal_rating` (`*int32`, half-stars truncated, empty/`0` → nil),
    `created_at` (from `Added`; empty → caller defaults to now()).
  - `personal_notes`: the verbatim Darkadia `Notes`, followed by assembled provenance lines
    (physical retailer, unrecognized digital store, **and** unmapped platform string per
    Decision B).
  - Consolidated platform list: union of aggregate `Platforms` + every `Copy platform`,
    decorated per copy with `(platform_slug, storefront_slug|nil, acquired_date)`, deduped on
    `(platform, storefront)`. Platform strings map via the static table in the format doc;
    storefront resolves via the ordered rules (recognized digital → slug; `Copy media ==
    "Physical"` → `physical` + retailer-in-note; unrecognized digital → nil + name-in-note;
    empty source → nil + no note; platform-inferred storefront as fallback).

The exported `Game` struct is the JSON shape stored in `source_metadata` (game-level fields +
the resolved platform list + the assembled note). It is independent of which IGDB id matches.

### 2. `matching.Decide` — extracted auto-resolve decision

`func Decide(query string, candidates []igdb.GameMetadata) Decision` in
`internal/services/matching/`, where `Decision` carries the scored+sorted candidates, the best
score, whether it is confident & unambiguous (`best ≥ 0.85 && best-second > 0.01`), and the
resolved IGDB id when confident. The thresholds move here as the single source of truth.
`sync.go`'s `IGDBMatchWorker` is refactored to call it (its existing tests guard the
behaviour). The new `darkadia_match` worker calls the same function.

> If extracting cleanly from `sync.go` proves to entangle `external_games` writes, the
> fallback is to extract only the pure scoring+threshold decision (no DB) and leave sync's
> `external_games` write where it is — the decision function is still shared.

### 3. `internal/worker/tasks` — two new workers

- **`DarkadiaMatchWorker`** (`darkadia_match`): load `job_item`, `SearchGames(source_title)`,
  `matching.Decide(...)`. Confident → set `resolved_igdb_id`/`match_confidence`, enqueue
  finalize. Otherwise → store `igdb_candidates` + `match_confidence`, set `status =
  pending_review`. No `external_games`.
- **`DarkadiaFinalizeWorker`** (`darkadia_finalize`): load `job_item` (has
  `resolved_igdb_id`), ensure the `games` row exists (reuse the import worker's
  `FetchFullMetadata` → `igdbMetadataToGame` → `DownloadCoverArt` path; factor a shared helper
  if needed), then apply the payload:
  - **`user_game`** — additive merge. If the user already has the game, leave
    `play_status`, `personal_rating`, `is_loved`, `personal_notes` untouched. If new, set them
    from the payload and `created_at` from `Added`.
  - **`user_game_platforms`** — merge: insert each `(platform, storefront, acquired_date,
    ownership_status=owned)` not already present; the `NULLS NOT DISTINCT` unique index dedups
    `(platform, storefront)`. (Collisions on the same `(platform, storefront)` with differing
    `acquired_date` keep the existing/earliest row; out-of-scope to reconcile.)
  - Record a `changes` row (`added` / `updated` / `already_in_library`), mark the item
    `completed` with its result, then `checkJobCompletion`.

### 4. Import-scoped resolve/skip endpoints (net-new)

On the existing job-items group (`internal/api/job_items.go`):

- `POST /api/job-items/:id/resolve` `{ "igdb_id": <int> }` → set `resolved_igdb_id`, clear
  candidates, enqueue `darkadia_finalize`.
- `POST /api/job-items/:id/skip` → set `status = skipped`, `processed_at = now()`,
  `checkJobCompletion`.

Both **guard that the item's parent job is an import source** (reject sync jobs with 4xx) so
the `external_games` resolution path is untouched. Job completion treats `pending`,
`processing`, **and `pending_review`** as blocking (per the documented gotcha:
`pending_review` blocks termination until the user resolves/skips every such item). This
mirrors `syncCheckJobCompletion`'s pending-review handling rather than the JSON importer's
simpler "count pending only" check.

### 5. Upload handler (net-new, mirrors `HandleImportNexorious`)

`POST /api/import/darkadia`, multipart `file`. Order:

1. `IGDBClient.Configured()` is false → `400` with "IGDB must be configured to import a
   Darkadia collection." (the up-front prerequisite).
2. Read the file (reuse the existing ~50 MB cap), validate the 29-column header → `400`
   "not a Darkadia export" on mismatch.
3. Reject a duplicate in-progress Darkadia import for the user (mirror the JSON guard).
4. `darkadia.Parse(bytes)`; create the `job` (`source = darkadia`, `dispatch_complete = true`,
   `total_items = len(games)`); per game create a `job_item` (`status = pending`,
   `source_metadata = payload`) and enqueue `darkadia_match`.

### 6. Frontend (extend, no new components)

- `internal/db/models/jobs.go`: add `JobSourceDarkadia = "darkadia"` (keep `JobSourceCSV`).
- `ui/frontend/src/types/import-export.ts`: add `ImportSource.DARKADIA = 'darkadia'` and a
  display entry (title "Darkadia CSV", description, `.csv` accept, icon/colour). `import-export.ts`
  API: `importDarkadiaCSV(file)` → `POST /api/import/darkadia`. Add the card to the
  Import/Export page source list.
- `JobItemsDetails`: for `pending_review` items, render **Find Match** (opens `IGDBMatchDialog`
  with `initialQuery = source_title`, no `externalGameId`) and **Skip**. `onSelect` →
  `POST /api/job-items/:id/resolve`; skip → `POST /api/job-items/:id/skip`. These actions live
  only in `JobItemsDetails` (the import surface); sync keeps using `ExternalGamesSection`, so
  the change is inherently import-scoped on the frontend as well.
- Optional nicety: disable/annotate the Darkadia card when IGDB is not configured (the server
  guard is the hard requirement).

---

## Testing strategy

- **Unit — `darkadia.Parse` (the bulk).** Row grouping (single/multi-copy, no-copy,
  no-platform, empty-`Name`-before-first-named rejected); `play_status` precedence (each tier +
  orthogonal Shelved/Playing); rating truncation (`4.5→4`, empty/`0`→nil); `created_at` from
  `Added`; notes verbatim + provenance assembly; platform union + per-copy decoration + dedup;
  storefront resolution (recognized digital, physical-by-`Copy media`, unrecognized-digital→note,
  empty-source→no note, `Other`→`Copy source other`, Epic/Uplay spelling variants, recognized
  source with extra free text → slug + annotation dropped); **unmapped platform → note, game
  still imports** (Decision B). Dialect: ragged rows, embedded-newline `Notes`, partial quoting,
  header accept/reject (value comparison, not raw string).
- **Unit — `matching.Decide`.** Confident/unambiguous, low-confidence, tie-gap. Sync's existing
  tests must stay green after the extraction.
- **Workers/endpoints (shared test DB, `truncateAllTables` per test).** Match auto-resolve vs
  `pending_review`; finalize additive merge (existing `play_status`/rating/loved/notes untouched;
  platforms merged + deduped; `ownership_status = owned`); resolve enqueues finalize; skip →
  `skipped`; **resolve/skip reject a sync `job_item`** (import-scoping); completion blocked while
  any `pending_review` remains.
- **End-to-end (manual, after implementation).** Import the maintainer's real export at
  `/storage/filebase/download/darkadia_export_csv_20251213.csv` into a fresh dev DB; confirm
  ~1,740 rows → ~1,474 games, spot-check multi-copy/no-copy/no-platform games, provenance notes,
  and that re-importing does not overwrite curation. The file is read-only and never committed.

---

## Out of scope

One-off only (no incremental re-import, saved mapping, or persistent Darkadia state); no
wishlist/tags/playtime; no new storefronts (long tail preserved in notes); half-stars truncated;
no sync-pipeline reuse (`external_games`, synthetic storefront, `UserGameWorker`, sync rematch);
no new staging tables; no use of `platforms.default_storefront` for storefront inference; never
overwrite existing user curation; the existing Nexorious JSON/CSV import-export round-trip is not
touched.
