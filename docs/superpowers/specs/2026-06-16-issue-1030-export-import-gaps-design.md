# Design — Close JSON export/import round-trip gaps (#1030)

**Issue:** [#1030](https://github.com/drzero42/nexorious/issues/1030) — *JSON export format omits wishlist flag, Play Planning pools, and platform availability*
**Epic:** [#984](https://github.com/drzero42/nexorious/issues/984) — Multi-source game-library import
**Date:** 2026-06-16

## Problem

The Nexorious JSON export (`format: nexorious-library`, `version: 2.0`) and its JSON
importer are a symmetric pair, so any user-owned field missing from one is missing from
both — a backup → restore round-trip silently drops it. Three genuine gaps were verified
against the code:

1. **`is_wishlisted`** (`user_games.is_wishlisted`) — absent from `exportGameJSON`
   (`export.go`) and `importGameData` (`import_item.go`). A wishlisted game exports as a
   plain `user_games` row and re-imports as an *owned* library entry, converting the whole
   wishlist into owned games.
2. **Play Planning pools & queue** (`pools`, `pool_games`, #955) — entirely absent from the
   export. Pool definitions, saved filters, and Candidate/Up-Next ordering are user-created
   data not re-derivable from anything else.
3. **`is_available`** (`user_game_platforms.is_available`) — absent from `exportPlatformJSON`;
   import hard-codes `IsAvailable: true`. Per-platform availability (e.g. delisted / pulled
   from a subscription) is lost on round-trip.

### Which import path is in scope

Nexorious's own data restores via **`HandleImportNexorious`** (`POST /api/import/nexorious`),
which enqueues the **legacy** `ImportItemWorker` path (`importGameData` in `import_item.go`).
That is the symmetric pair of `buildJSONDoc`/`exportGameJSON`. The CSV/`csvmap` pipeline
(`importmodel.Game`, `import_pipeline.go`) is a *different* path and already carries
`IsWishlisted`; it is **out of scope** here. This issue is specifically the legacy JSON pair.

## Decisions (locked with maintainer)

- **All three gaps in one PR.**
- **Bump the format version to `2.1`.** Import accepts both `2.0` and `2.1` and keys behaviour
  off **field presence, never the version string**, so older and hand-edited files still import.
- **Pools merge semantics:** find-or-create each pool by `(user_id, name)`, merge members
  additively, never overwrite an existing pool's curation (color/filter/position).
- **Pools applied at the job-completion transition** via a synthetic, pre-completed `job_item`
  (Approach A below). No migration — `job_items.source_metadata` is already `jsonb` and every
  model field already exists.

## Format v2.1 — document shape

```jsonc
{
  "format": "nexorious-library",
  "version": "2.1",
  "exported_at": "...",
  "games": [
    {
      "igdb_id": 7777,
      "title": "...",
      "play_status": "completed",
      "personal_rating": 4,
      "is_loved": true,
      "is_wishlisted": false,        // NEW — absent ⇒ false
      "personal_notes": "...",
      "created_at": "...",
      "updated_at": "...",
      "platforms": [
        {
          "platform": "pc-windows",
          "storefront": "steam",
          "ownership_status": "owned",
          "acquired_date": "2024-12-25",
          "hours_played": 12.5,
          "is_available": true       // NEW — absent ⇒ true
        }
      ],
      "tags": [ { "name": "...", "color": "..." } ]
    }
  ],
  "pools": [                          // NEW top-level section — absent/empty ⇒ no pools created
    {
      "name": "Backlog",
      "color": "#abcdef",
      "position": 0,
      "filter": { /* raw jsonb, copied verbatim */ },
      "games": [
        { "igdb_id": 7777, "position": null },   // position null = Candidate
        { "igdb_id": 8888, "position": 0 }       // position set  = Up-Next order
      ]
    }
  ]
}
```

`igdb_id` is the stable cross-instance key for a pool member: the export translates each
membership's opaque `user_game_id` into the game's `igdb_id`, and the import resolves it back.

## Export side (`internal/worker/tasks/export.go`)

- `exportGameJSON` gains `IsWishlisted bool \`json:"is_wishlisted"\`` ← `ug.IsWishlisted`.
- `exportPlatformJSON` gains `IsAvailable bool \`json:"is_available"\`` ← `p.IsAvailable`.
- `exportDocJSON` gains `Pools []exportPoolJSON \`json:"pools"\``:
  - `exportPoolJSON { Name string; Color *string; Position int; Filter json.RawMessage; Games []exportPoolGameJSON }`
  - `exportPoolGameJSON { IGDBID int32 \`json:"igdb_id"\`; Position *int \`json:"position"\` }`
- New helper `loadPoolsForExport(ctx, db, userID) ([]exportPoolJSON, error)`:
  - load the user's pools ordered by `position`;
  - per pool, `SELECT pg.position, ug.game_id FROM pool_games pg JOIN user_games ug ON ug.id = pg.user_game_id WHERE pg.pool_id = ?` ordered by `position NULLS LAST` then a stable key, mapping `game_id → igdb_id`.
- `ExportJSONWorker.Work` loads pools and passes them into `writeJSONExport` → `buildJSONDoc`.
  `buildJSONDoc` gains a `pools []exportPoolJSON` parameter and writes `version: "2.1"`.
- **CSV export is untouched** — pools/availability/wishlist remain JSON-only. (`is_wishlisted`
  is intentionally not added to the flat CSV; CSV is a lossy interchange format, JSON is the
  backup format.)

## Import side

### Version gate (`internal/api/import.go` — `HandleImportNexorious`)

Replace the strict `export.Version != "2.0"` check with a supported-set check accepting
`"2.0"` and `"2.1"`. Keep the existing legacy-`export_version` and unknown-version rejection
messages. No behaviour is gated on the version string beyond admission.

### Wishlist (`importGameData`, `ImportItemWorker`)

- `importGameData` gains `IsWishlisted bool \`json:"is_wishlisted"\``.
- The new-`UserGame` insert sets `IsWishlisted: gd.IsWishlisted`. Absent ⇒ `false`.
- `ClearWishlistOnAcquire` is unchanged: it only clears when the game has ≥1 platform, so a
  true (platform-less) wishlist entry keeps its flag while an owned entry is correctly
  de-wishlisted — preserving the invariant "wishlisted ⇒ not owned".

### Platform availability (`importPlatformData`, `ImportItemWorker`)

- `importPlatformData` gains `IsAvailable *bool \`json:"is_available"\`` (pointer to distinguish
  absent from explicit `false`).
- The platform insert uses `IsAvailable: pd.IsAvailable == nil || *pd.IsAvailable`
  (absent ⇒ `true`, replacing the current hard-coded `true`).

### Pools — Approach A (apply at completion transition)

- **`HandleImportNexorious`** parses a top-level `pools` array from the body. If non-empty, it
  inserts **one synthetic `job_item`**: `ItemKey = "__pools__"`, `Status = completed`,
  `SourceMetadata = {"item_type":"pools","data": <pools raw>}`. It is **not** counted in
  `total_items` and gets **no** River task — inert for the status-filtered completion counts.
- **`checkJobCompletion`** (`import_item.go`) captures the `bool` returned by
  `finalizeJobCompleted`. On the single `pending/processing → completed` transition (the bool is
  `true` exactly once) it calls `applyImportedPools(ctx, db, jobID, userID)`:
  - load the synthetic `__pools__` item for the job; if absent, return;
  - parse the pools payload;
  - per pool: `INSERT INTO pools … ON CONFLICT (user_id, name) DO NOTHING` then `SELECT id`.
    New pools take the export's color/filter and `COALESCE(MAX(position)+1, 0)`; existing pools
    keep their color/filter/position untouched;
  - per member: resolve `igdb_id → user_games.id` for the user; insert into `pool_games`
    `ON CONFLICT (pool_id, user_game_id) DO NOTHING`. Members whose game failed to import are
    skipped + logged;
  - best-effort: per-row errors log and continue; pools never fail the import job.
- **`HandleGetJobItems`** (`jobs.go:529` — list + count) gains `item_key <> '__pools__'` so the
  synthetic row never appears in the UI or inflates the item count. (Completion counts already
  filter by `status` and are unaffected.)

## Backward compatibility

| Missing field        | Default on import |
|----------------------|-------------------|
| `is_wishlisted`      | `false`           |
| `is_available`       | `true`            |
| `pools`              | none created      |

Existing `2.0` files import byte-for-byte identically. Hand-edited files with any field omitted
import with the defaults above.

## Constraints relied upon (verified)

- `pools` has `UNIQUE(user_id, name)` → find-or-create by name.
- `pool_games` has `UNIQUE(pool_id, user_game_id)` → `ON CONFLICT DO NOTHING` dedup.
- `finalizeJobCompleted` flips `pending/processing → completed` via a guarded `UPDATE` and
  returns `RowsAffected > 0`, so the completion transition fires exactly once → pools applied once.
- No migration required: all model fields exist; `job_items.source_metadata` is `jsonb`.

## Testing

- Extend `TestImport_RoundTripPreservesUserData` (`import_roundtrip_test.go`): source user with
  (a) a platform-less wishlisted game, (b) a platform with `is_available=false`, and (c) a pool
  with a Candidate member and a queued member. Export → import into a fresh dest user → assert
  `is_wishlisted`, `is_available`, and the pool + membership + positions are all preserved.
- **Backward-compat:** a literal `2.0` document (no new fields) imports with the documented
  defaults (wishlist=false, available=true, no pools).
- **Version gate:** `2.0` accepted, `2.1` accepted, legacy `1.x` rejected, unknown rejected.
- **Pool merge:** re-importing a pool whose name already exists merges members with no duplicate
  rows and no overwrite of the existing pool's color/filter/position.
- **`applyImportedPools` unit:** resolves members by `igdb_id`, skips members whose game is absent.

## Out of scope

- The CSV export format and the `csvmap`/`importmodel.Game` pipeline (already handles wishlist).
- IGDB-derived game metadata, `user_settings`, `sync_from_source`, `external_game_id`, encrypted
  sync credentials, sessions, API keys, sync cache, and the activity log (intentional exclusions
  per the issue).
- User-facing docs for the format change (tracked separately under the epic's docs capstone
  #1026; this PR may add a one-line note if convenient but does not own that work).
