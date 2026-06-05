# Bring JSON import/export into compliance with the v2.0 interchange spec

**Issue:** #828
**Format spec (source of truth):** [`docs/import-export-format.md`](../../import-export-format.md) (version `2.0`)
**Status:** design approved

## Problem

The native Nexorious JSON import/export code still emits and parses the legacy
`1.2` format, which diverges from the now-authoritative `2.0` spec in
`docs/import-export-format.md`. The audit captured in the spec and issue #828
found:

- A **round-trip bug**: export writes `release_year` (int) but import only reads
  `release_date` (RFC 3339), so release info is silently dropped on re-import.
- **Dead/derived envelope data**: `user_id`, `total_games`, `total_wishlist`,
  `export_stats`, and an always-empty `wishlist`.
- **IGDB metadata fields** the import accepts as a fallback but export never
  produces (`description`, `genre`, `developer`, `publisher`, `cover_art_url`,
  `rating_average`).
- **Redundant fields**: `platform_id`==`platform_name`,
  `storefront_id`==`storefront_name`; game-level `hours_played` duplicating
  per-platform hours.
- **Type drift**: `personal_rating` exported as int, imported as float.
- **Instance-local / denormalized fields** that must not be serialized:
  `external_game_id`, `sync_from_source`, `store_game_id`, `store_url`,
  `is_available`.

This design brings export and import into compliance with `2.0`. It is purely an
implementation change to match the already-approved format spec; it does not
re-decide the format.

## Guiding principle (from the format spec)

A game's identity in the file is its **IGDB ID plus a human-readable title**.
Everything else about the game is IGDB-owned metadata, re-hydrated from `igdb_id`
on import. The file carries only the IGDB ID, the title, and the data the *user*
added (play status, rating, loved, notes, per-platform ownership facts, tags).

Import is **non-destructive / additive (merge)**: existing user fields are never
overwritten; platforms merge by `(platform, storefront)`; tags are
found-or-created per user, case-insensitively.

## Scope

In scope (per issue #828):

- `internal/worker/tasks/export.go` — JSON export envelope + game/platform/tag
  shape; CSV column reconciliation.
- `internal/api/import.go` — version gate; hard IGDB requirement.
- `internal/worker/tasks/import_item.go` — parsed structs; game re-hydration via
  reuse; enum/rating validation; platform field set.
- Tests and a docs touch-up.

Out of scope:

- **#827** (drop `store_game_id` / `store_url` columns from
  `user_game_platforms`). Those columns still exist in the schema and on the
  model; this work simply stops serializing and reading them. The model fields
  remain until #827 lands.

## Design

### 1. JSON export — `internal/worker/tasks/export.go`

Replace the document/game/platform structs with the `2.0` shape and remove all
stats accumulation from `buildJSONDoc`.

```go
type exportDocJSON struct {
    Format     string           `json:"format"`      // constant "nexorious-library"
    Version    string           `json:"version"`     // constant "2.0"
    ExportedAt string           `json:"exported_at"` // RFC3339 UTC
    Games      []exportGameJSON `json:"games"`
}

type exportGameJSON struct {
    IGDBID         int32                `json:"igdb_id"`
    Title          string               `json:"title"`
    PlayStatus     *string              `json:"play_status"`
    PersonalRating *int32               `json:"personal_rating"`
    IsLoved        bool                 `json:"is_loved"`
    PersonalNotes  *string              `json:"personal_notes"`
    CreatedAt      string               `json:"created_at"`
    UpdatedAt      string               `json:"updated_at"`
    Platforms      []exportPlatformJSON `json:"platforms"`
    Tags           []exportTagJSON      `json:"tags"`
}

type exportPlatformJSON struct {
    Platform        *string  `json:"platform"`
    Storefront      *string  `json:"storefront"`
    OwnershipStatus *string  `json:"ownership_status"`
    AcquiredDate    *string  `json:"acquired_date"` // "YYYY-MM-DD"
    HoursPlayed     *float64 `json:"hours_played"`
}

// exportTagJSON is unchanged: { name string, color *string }
```

Removed: `exportStatsJSON`; envelope fields `export_version`, `export_date`,
`user_id`, `total_games`, `total_wishlist`, `export_stats`, `wishlist`;
game-level `release_year` and `hours_played`; platform `platform_id`,
`storefront_id`, `store_game_id`, `store_url`, `is_available`.

`buildJSONDoc` becomes a straight projection: for each `UserGame`, map user
fields and project platforms/tags. No stats maps, no release-year derivation, no
game-level hours summation. The envelope is `{format:"nexorious-library",
version:"2.0", exported_at:<now RFC3339 UTC>, games:[...]}`.

Field ordering in the struct follows the spec example
(`igdb_id, title, play_status, personal_rating, is_loved, personal_notes,
created_at, updated_at, platforms, tags`).

### 2. CSV export — same file

CSV remains a **separate, human-oriented, export-only convenience format**. It is
**not round-trippable** — there is no CSV import counterpart, and it is not
governed by the interchange spec.

Only change: drop the `release_year` column from `csvHeaders` and
`buildCSVRow`. The game-level total `hours_played` and the `;`-joined platform /
tag columns stay, because CSV is for human consumption, not re-import.

A short paragraph documenting this is added to `docs/import-export-format.md`.

### 3. Import handler — `internal/api/import.go`

- **Hard IGDB requirement**, mirroring `HandleImportDarkadia`: at the top of
  `HandleImportNexorious`,
  `if h.igdbClient == nil || !h.igdbClient.Configured()` → `400` with
  `"IGDB must be configured to import a Nexorious library"`. The game record is
  re-hydrated from IGDB by ID; with no client an import cannot construct usable
  games.
- **Version gate**: change `nexoriousExport` to read `Version string
  json:"version"`. Also read the legacy `ExportVersion string
  json:"export_version"` field *only* to enrich the error message. Accept only
  `"2.0"`; reject everything else. A legacy `1.x` file has no `version` key, so
  it is rejected. Error message: `"Unsupported import file. Only Nexorious
  library format version 2.0 is supported."`; when `export_version` is present,
  append the detected legacy version for clarity.
- The rest of the handler (active-job conflict check, job + per-item creation,
  River enqueue, `skipCount` accounting) is unchanged.

### 4. Import worker — `internal/worker/tasks/import_item.go`

**Parsed structs** trimmed to the `2.0` shape:

```go
type importGameData struct {
    IGDBID         int32                `json:"igdb_id"`
    Title          string               `json:"title"`
    PlayStatus     *string              `json:"play_status"`
    PersonalRating *int                 `json:"personal_rating"` // integer now
    IsLoved        bool                 `json:"is_loved"`
    PersonalNotes  *string              `json:"personal_notes"`
    CreatedAt      *string              `json:"created_at"` // RFC3339
    UpdatedAt      *string              `json:"updated_at"` // RFC3339
    Platforms      []importPlatformData `json:"platforms"`
    Tags           []importTagData      `json:"tags"`
}

type importPlatformData struct {
    Platform        string   `json:"platform"`
    Storefront      string   `json:"storefront"`
    OwnershipStatus *string  `json:"ownership_status"`
    AcquiredDate    *string  `json:"acquired_date"` // date-only or RFC3339
    HoursPlayed     *float64 `json:"hours_played"`
}

// importTagData unchanged: { name string, color *string }
```

Deleted from `importGameData`: `description`, `genre`, `developer`, `publisher`,
`release_date`, `cover_art_url`, `rating_average`, game-level `hours_played`.
Deleted from `importPlatformData`: the `_id` keys, `store_game_id`, `store_url`,
`is_available`.

**Game re-hydration via reuse.** Replace the inline IGDB-fetch-or-JSON-fallback
block (and the dead `models.Game{...}` fallback constructor) with a call to the
existing helper `ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath,
gd.IGDBID, gd.Title)` from `darkadia.go`. It:

1. Returns early if the game already exists.
2. If IGDB is configured and the fetch succeeds, inserts the full metadata row
   and downloads cover art.
3. **On any per-item IGDB failure** (transient error, or an ID no longer in
   IGDB), inserts a **minimal `id`+`title` row** (`ON CONFLICT (id) DO
   NOTHING`). User data is preserved; a later metadata refresh can fill in the
   rest.

This is the approved per-item fallback behavior and removes a duplicated upsert
path.

**Validation — lenient coercion (never lose a game over a bad field):**

- `personal_rating`: keep only if it is an integer in the inclusive range
  `1..5`; otherwise store `nil` (unrated) and log a warning.
- `play_status`: keep only if `enum.PlayStatus(v).Valid()`; otherwise store
  `nil` (unset) and log a warning.
- `ownership_status`: if absent (`nil`) or not
  `enum.OwnershipStatus(v).Valid()`, coerce to `"owned"` (the documented
  default); log a warning on an invalid value.

**Platform insert** (`UserGamePlatform`):

- `IsAvailable: true` — imported rows default to available; sync re-derives.
- `StoreGameID`, `StoreUrl`, `ExternalGameID`: left `nil`.
- `SyncFromSource: false`.
- Drop the game-level-hours backfill (the field no longer exists); per-platform
  `hours_played` is the only source.
- Keep: the platform-exists check (unknown platform → skip), nullable-storefront
  handling (unknown/blank storefront → null), and the `(platform, storefront)`
  merge dedup against existing rows.

**Unchanged:** non-destructive merge (existing `user_games` user fields left
untouched when the game is already owned), `created_at`/`updated_at` preservation
for new games, `findOrCreateTag` (case-insensitive find-or-create, color applied
only on create), the per-item `changes` row, and `checkJobCompletion`.

## Error handling

- Handler: missing/unconfigured IGDB → `400`; wrong/absent `version` → `400`;
  empty `games` → `400` (unchanged). All other handler paths unchanged.
- Worker: per-item parse/insert failures continue to mark the individual
  `JobItem` failed (never returns an error from `Work`), exactly as today.
  Lenient field coercion means malformed enum/rating values degrade gracefully
  rather than failing the item.

## Testing

- **Round-trip test** (`internal/worker/tasks`): seed a user game with
  platforms and tags, run `buildJSONDoc`, then feed each game entry through
  `ImportItemWorker` into a fresh user. With an unconfigured IGDB client
  (`igdb.NewClient(&config.Config{})`, the pattern existing tests use)
  `ensureGameRow` takes the minimal-row fallback and preserves `title`. Assert
  the user-owned fields round-trip: `play_status`, `personal_rating`,
  `is_loved`, `personal_notes`, `created_at`/`updated_at`, each platform's
  `(platform, storefront, ownership_status, acquired_date, hours_played)`, and
  the tag set.
- **Version-gate test** (`internal/api`): a `1.x` file
  (`export_version:"1.2"`, no `version`) → handler returns `400` with the clear
  message.
- **IGDB-required test** (`internal/api`): POST with an unconfigured IGDB client
  → `400`.
- **Update existing** `import_item_test.go` cases that asserted the now-removed
  JSON-fallback metadata fields (`description`, `genre`, etc.).

## Docs

- `docs/import-export-format.md`: flip the "current code does not yet implement
  this format" status note (now implemented), and add the short CSV
  convenience-format paragraph (separate, human-oriented, export-only,
  non-round-trippable).

## Risks / notes

- The `models.UserGamePlatform` struct retains `StoreGameID`, `StoreUrl`,
  `IsAvailable`, `ExternalGameID`, `SyncFromSource` because the columns still
  exist (and sync uses them). We simply stop populating store-linkage from
  import. #827 removes the columns later.
- `parseFlexibleDate` and `igdbMetadataToGame` remain (used by the platform
  acquired-date parse and by `ensureGameRow` respectively).
