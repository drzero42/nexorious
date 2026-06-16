# Grouvee CSV import as a csvmap Config — #1002

**Issue:** [#1002](https://github.com/drzero42/nexorious/issues/1002) — a CSV-track child of epic [#984](https://github.com/drzero42/nexorious/issues/984) (multi-source game-library import).
**Status:** design approved, ready for an implementation plan.
**Depends on:** [#1014](https://github.com/drzero42/nexorious/issues/1014) — the config-driven `csvmap` engine — [#1004](https://github.com/drzero42/nexorious/issues/1004) — the generic CSV import path — [#1022](https://github.com/drzero42/nexorious/issues/1022) — IGDB-id direct match — and [#1003](https://github.com/drzero42/nexorious/issues/1003) — the manual Format selector. All merged.
**Pairs with:** [#1015](https://github.com/drzero42/nexorious/issues/1015) — auto-detect on `inspect` (out of scope here; this preset registers into the same registry #1015 reuses).

## Problem

A user migrating from **Grouvee** has a native CSV export but cannot import it today. The issue framed Grouvee as a straightforward preset — a "shelf → `play_status` value map" in the simple `csvmap` subset. **A verification spike against a real export proves that framing wrong**: several of Grouvee's user-meaningful columns are **JSON embedded in the CSV** (objects *and* a play-log array), which the simple subset cannot express. This spec records the verified format and delivers Grouvee as a real `Config`, extending the engine where the format genuinely requires it (scope item #3 of the issue).

## Verification spike — the Grouvee export

Pinned against two real exports of the reporter's own account (`/home/abo/Downloads/grouvee/collection.csv`): an initial 5-game export, and a second after deliberately populating play-log data.

- **Dialect:** standard RFC-4180 CSV, **UTF-8/ASCII**, one header row, **20 columns**. No malformed quoting, no Windows-1252 — the existing `csvmap.ReadRecords` parses it unchanged. **No reader work.**
- **IGDB id IS exported.** Every row carries a populated `igdb_id` (e.g. Portal `71`, Red Dead Redemption 2 `25076`). This maps to the existing `ColumnMap.IGDBID` and rides the **#1022 direct-match path** — hydrate from IGDB by id, skip title matching, never touch `pending_review`. `giantbomb_id` is also present but ignored (Nexorious keys on IGDB).
- **`shelves` and `platforms` are JSON objects** whose **keys** carry the data:

  ```
  shelves:   {"Played": {"date_added": "2017-07-18T13:48:26Z", "url": "…"}}
  platforms: {"PC (Microsoft Windows)": {"url": "…"}}        (or {} — empty for most rows)
  ```

  Fed to the scalar engine, `shelves` would lowercase the whole JSON blob and never match a `ValueMap` key; `platforms` would pass the raw JSON through as a bogus slug, and `{}` (non-empty) would fabricate a junk entry. The simple subset genuinely cannot read these.
- **`dates` is a JSON array play-log** (NOT empty, as the first export misleadingly suggested). Each element is a play-session object:

  ```
  dates: [{"date_started": "2010-01-01", "date_finished": "2020-02-02",
           "seconds_played": 36300, "level_of_completion": "100% Completion"}]
  ```

  This carries real, mappable data: **`seconds_played`** (playtime) and **`level_of_completion`** (a HowLongToBeat-style completion tier). `date_started`/`date_finished` (a date or the literal string `"None"`) are **not** used.
- **`statuses` is verified empty.** It is `[]` on every row even after deliberately setting per-game statuses in the Grouvee UI — that data surfaces in `dates`, not `statuses`. The `statuses` column appears unused in the CSV export and is dropped as **verified-empty**, not inferred.
- **Columns (20):** `id, name, shelves, platforms, rating, review_title, review, review_platform, dates, statuses, genres, franchises, series, developers, publishers, release_date, date_added_to_collection, url, giantbomb_id, igdb_id`. Their fate is in the column reference below.

## Decisions taken in brainstorming

1. **Full JSON fidelity.** `shelves`, `platforms`, and the `dates` play-log are all parsed (via two new JSON primitives), preserving play-status, platform-ownership, playtime, and completion data rather than dropping it.
2. **Wish List is first-class.** Grouvee's built-in `Wish List` shelf maps to Nexorious's `is_wishlisted` (#867), not to a `play_status`. This requires a shared-surface change: `IsWishlisted bool` on `importmodel.Game`, wired through `ImportFinalizeWorker`. Chosen over the lossy alternative (importing a wishlisted, unowned game as an owned `not_started` entry).
3. **Ratings round to nearest, scale 5.** Grouvee uses a 1–5 star scale with half-steps; Nexorious stores whole 1–5. `RatingConfig{Scale: 5, Truncate: false}` — matching the Completionator preset's convention (`4.5` → `5`).
4. **Notes combine `review_title` + `review`.** The optional heading is preserved: `**title**\n\nbody` (either alone works; both empty → no note). This needs a focused two-column note-assembly capability.
5. **Playtime + completion tier from `dates`.** `seconds_played` (summed across entries, ÷3600) → `hours_played`. `level_of_completion` → `play_status`, mapping `Main Story`→`completed`, `Main Story + Extras`→`mastered`, `100% Completion`→`dominated`.
6. **A populated tier overrides the shelf.** When `dates` carries a recognized `level_of_completion`, that tier sets `play_status`, overriding the shelf-derived status (the user's explicit choice — the play-log completion is treated as the authoritative status signal). Unrecognized tiers are ignored (fall back to shelf); `date_finished` is not consulted.
7. **Drop the editorial JSON and the unused columns.** `genres`, `franchises`, `series`, `developers`, `publishers`, `release_date` are IGDB-supplied on match; `statuses` is verified-empty; `id`, `url`, `giantbomb_id`, `review_platform`, and the `dates` start/finish dates have no model home. Each is recorded in `docs/grouvee-import.md`, never silent.

## Architecture

Four generic engine extensions, one shared-pipeline field, the Grouvee `Config`, the registry entry, the doc, and tests. No migration, no new pipeline.

### Extension 1 — JSON-object-keys column decode

A configured column may be read as a JSON object instead of a scalar. A new column-format marker plus a decode helper:

```go
// ColumnFormat selects how a configured column's cell is read.
type ColumnFormat string

const (
	FormatScalar   ColumnFormat = ""          // default: the trimmed cell is the single value
	FormatJSONKeys ColumnFormat = "json-keys" // cell is a JSON object; its keys are the values
)

// decodeKeys returns the value list a column yields under format f. For
// FormatScalar: [cell] (or nil if blank). For FormatJSONKeys: the JSON object's
// keys (nil for "", "{}", or unparseable JSON).
func decodeKeys(cell string, f ColumnFormat) []string
```

`decodeKeys` is the new primitive the JSON-aware scalar extractors (status, platform) call. Empty/`{}`/blank → no values (graceful, never a junk entry); malformed JSON → no values (the row still imports via its other columns and its `igdb_id`). JSON object key order from `encoding/json` is non-deterministic, so any consumer that must pick one value uses an explicit precedence (below), never map order.

### Extension 2 — shelf → status + wishlist

Status derivation is unified across scalar and JSON columns. `StatusColumn` gains a format and an explicit precedence; a reserved `ValueMap` target routes a value to the wishlist flag instead of `play_status`:

```go
type StatusColumn struct {
	Column     string
	Format     ColumnFormat      // "" scalar (default) | "json-keys"
	ValueMap   map[string]string // normalized source value -> play_status, or the reserved WishlistStatus
	Precedence []string          // normalized source values, highest priority first; used to pick when several map
	Default    string            // empty/unmapped -> this; "" falls back to "not_started"
}

// WishlistStatus is the reserved ValueMap target that flags is_wishlisted rather
// than setting play_status. play_status is then derived from the remaining values.
const WishlistStatus = "wishlisted"
```

`extractStatus(rec, idx, cfg) (status string, wishlisted bool)`:

1. `values := decodeKeys(cell(...Column), Format)` (normalized).
2. `wishlisted` = any value maps (via `ValueMap`) to `WishlistStatus`.
3. status candidates = values mapping to a non-`WishlistStatus` play_status.
4. pick the candidate whose source value appears **earliest in `Precedence`**; if no candidate is listed in `Precedence` (or there are none), use `Default` (→ `not_started`).

Scalar status columns are the degenerate one-value case — `Precedence` is then a no-op and behaviour is unchanged for Completionator. For Grouvee:

```
ValueMap:   {"playing":"in_progress", "played":"completed", "backlog":"not_started", "wish list": WishlistStatus}
Precedence: ["playing", "played", "backlog"]
```

So a game on `{Playing, Wish List}` → `in_progress` **and** `is_wishlisted=true`; an unrecognized custom shelf → `not_started`. (This shelf-derived status is the baseline; a populated `dates` tier overrides it — Extension 4.)

### Extension 3 — two-column note assembly

`NotesConfig` gains an optional heading column, leaving the parked advanced `Assembly` (Darkadia review/copy-note assembly, #1016) untouched:

```go
type NotesConfig struct {
	Column      string        // SIMPLE: body / verbatim notes column
	TitleColumn string        // NEW: optional heading; when its cell is non-empty, prepended as "**title**\n\n"
	Assembly    *NoteAssembly // ADVANCED #1016 (Parse still rejects)
}
```

Assembly rule (in `extractGame`): let `title = cell(TitleColumn)`, `body = cell(Column)`.
- both empty → no note (`PersonalNotes` nil)
- title empty → note = `body`
- body empty → note = `**title**`
- both present → note = `**title**\n\nbody`

### Extension 4 — `dates` play-log → playtime + completion tier

A new optional config block reads a JSON-array column of play-session objects, summing a seconds field into `hours_played` and mapping a completion-tier field to a `play_status` that **overrides** the shelf:

```go
// PlayLogConfig reads playtime and a completion tier from a JSON-array column
// whose elements are play-session objects (Grouvee's "dates"). Optional; nil =
// ignore. When a recognized CompletionField value is present, it overrides the
// shelf-derived play_status.
type PlayLogConfig struct {
	Column          string            // JSON-array column of session objects
	SecondsField    string            // numeric object field; summed across entries, ÷3600 -> hours_played
	CompletionField string            // object field holding the completion tier
	CompletionMap   map[string]string // normalized tier -> play_status; unrecognized tier ignored
}
```

Added to `Config` as `PlayLog *PlayLogConfig`. `extractPlayLog(rec, idx, cfg) (hours *float64, tierStatus string)`:

1. JSON-decode the column as an array of objects (distinct from `decodeKeys` — this reads object *fields*, not keys). Non-array/blank/malformed → `(nil, "")`.
2. Sum `SecondsField` across entries (numeric only; `0`/non-numeric contribute nothing). `hours = total/3600.0` when `total > 0`, else nil.
3. For each entry, look up the normalized `CompletionField` value in `CompletionMap`. Across entries, keep the **highest-ranked** resulting `play_status` (rank `completed < mastered < dominated`); unrecognized tiers contribute nothing. `tierStatus` = that status, or `""` if none recognized.

`date_started`/`date_finished` are not read. For Grouvee:

```
Column: "dates", SecondsField: "seconds_played", CompletionField: "level_of_completion",
CompletionMap: {"main story":"completed", "main story + extras":"mastered", "100% completion":"dominated"}
```

**Status resolution (in `extractGame`):** `play_status = tierStatus` when `tierStatus != ""`, else the shelf-derived `status` from Extension 2. `is_wishlisted` (from the shelf) and `hours_played` (from the play-log) are set independently of which status wins.

### Validation

`validate.go` gains: `StatusColumn.Format` and `PlatformSimple.PlatformFormat` must be `""` or `"json-keys"`; a non-nil `PlayLog` requires `Column`, `SecondsField`, `CompletionField`; `Duration` and `PlayLog` are mutually exclusive (two sources of `hours_played`). Each violation is a descriptive error, not `ErrInvalidSignature`. The advanced `Status.Flags`, `Platform.Tables`, `Notes.Assembly`, `Grouping.CopyRows` remain rejected via `notImplemented` — this spec implements none of them; it adds *new* simple-subset capabilities, distinct from the parked Darkadia advanced model.

### Extension to the platform extractor

`PlatformSimple` gains a format so the platform column can yield several entries:

```go
type PlatformSimple struct {
	PlatformColumn     string
	PlatformFormat     ColumnFormat      // "" scalar (default, one entry) | "json-keys" (one entry per key)
	StorefrontColumn   string
	AcquiredDateColumn string
	PlatformMap        map[string]string
	StorefrontMap      map[string]string
}
```

`extractPlatforms` decodes the platform column via `decodeKeys`. For each value: map via `PlatformMap` (miss → passthrough), emit one `importmodel.Platform{Platform: slug, Storefront, AcquiredDate}`, deduped by slug within the row. Grouvee's platform JSON values carry only a `url` (no storefront, no acquired date), so `StorefrontColumn`/`AcquiredDateColumn` are unset for Grouvee and those fields are nil/empty. The scalar path (Completionator) is unchanged.

### Shared-pipeline change — `IsWishlisted`

```go
// importmodel.Game
IsWishlisted bool `json:"is_wishlisted,omitempty"`
```

`omitempty` + `false` default means every existing mapper (Darkadia, vglist, generic CSV, Completionator, Nexorious JSON) serializes **byte-identically** — the flag stays inert for sources that never set it. `ImportFinalizeWorker` sets `IsWishlisted: payload.IsWishlisted` on the **new-`user_game` insert only**. No other finalize change is needed:

- `models.UserGame.IsWishlisted` already exists (#867).
- The existing unconditional `usergame.ClearWishlistOnAcquire(ctx, db, ug.ID)` call already guards on `EXISTS (… user_game_platforms …)`: a Wish-List-only game (no platforms) keeps the flag; an owned game (any platform attached) clears it. Correct for both cases with no new logic.
- The **merge path** (game already in library) is additive-only and does not touch `is_wishlisted` — consistent with "never overwrite existing curation."

### The Grouvee `Config`

`func Grouvee() Config`, registered in `presetList`:

| Canonical field | Source column | Handling |
|---|---|---|
| Title (required) | `name` | verbatim |
| IGDB id | `igdb_id` | `ColumnMap.IGDBID`; positive int → #1022 direct match |
| play_status / wishlist | `shelves` | `Format: json-keys`; ValueMap + Precedence + `WishlistStatus` (Extension 2) |
| play_status (override) + hours_played | `dates` | `PlayLog`: tier → status override, `seconds_played` → hours (Extension 4) |
| Platforms | `platforms` | `PlatformFormat: json-keys`; `PlatformMap` IGDB-name → slug, passthrough on miss (Extension 1) |
| personal_rating | `rating` | `RatingConfig{Scale: 5, Truncate: false}` |
| personal_notes | `review_title` + `review` | `NotesConfig{TitleColumn: "review_title", Column: "review"}` (Extension 3) |
| created_at | `date_added_to_collection` | `ColumnMap.CreatedAt`; ISO `2006-01-02` (engine default layout) |
| Grouping | — | one-row (`MergeByTitle: false`); each Grouvee row is a distinct game |

**Platform map.** Targets are the seeded `platforms.name` slugs; Grouvee uses IGDB platform names (`PC (Microsoft Windows)` → `pc-windows` is fixture-verified). The map is built during implementation by matching the common IGDB platform names against the platform seed; an unseen value passes through unchanged and imports as that platform (or is dropped at finalize if it matches no seeded platform — existing `import_item` behaviour). `docs/grouvee-import.md` records the map as fixture-derived and extensible.

**Signature** (all present, matched normalized) — a distinctive Grouvee subset, not all 20, so a minor upstream column addition does not break detection:

```
shelves, date_added_to_collection, review_platform, giantbomb_id, igdb_id
```

### Registration

```go
// presets.go
{Slug: "grouvee", DisplayName: "Grouvee", Config: Grouvee()},
```

Per the #1003 wiring this is the only integration step: `POST /api/import/csv/inspect` returns `{slug:"grouvee", name:"Grouvee"}` in `presets`, the CSV mapping dialog's **Format** dropdown lists it automatically (no frontend change), and `POST /api/import/csv` with `format=grouvee` runs `csvmap.Parse(body, Grouvee())` server-side with the signature enforced against the upload. Auto-detection on the same registry remains #1015.

## Column reference (the spike result, for `docs/grouvee-import.md`)

| Column | Fate |
|---|---|
| `id` | Grouvee's own game id — **dropped** (Nexorious keys on IGDB). |
| `name` | Title (fallback matching for any row missing `igdb_id`). |
| `shelves` | JSON object → `play_status` (baseline) + `is_wishlisted` (Extension 2). |
| `platforms` | JSON object → ownership entries (Extension 1). |
| `rating` | → `personal_rating`, scale 5, round-to-nearest. |
| `review_title` | → note heading (Extension 3). |
| `review` | → note body (Extension 3). |
| `review_platform` | **Dropped** — which platform a review targets has no model home (used in the signature). |
| `dates` | JSON-array play-log → `hours_played` (`seconds_played`) + `play_status` override (`level_of_completion`); start/finish dates **dropped** (Extension 4). |
| `statuses` | **Dropped** — verified empty (`[]`) in every export even after setting statuses in-app. |
| `genres`, `franchises`, `series`, `developers`, `publishers`, `release_date` | Editorial metadata — **dropped**, IGDB resupplies on match. |
| `date_added_to_collection` | → `user_games.created_at`. |
| `url` | Grouvee page URL — **dropped**. |
| `giantbomb_id` | GiantBomb id — **dropped** (used in the signature). |
| `igdb_id` | → direct IGDB-id match (#1022). |

## Data flow into the existing pipeline

No new pipeline, migration, or schema change. `Grouvee()` produces `[]importmodel.Game`, handed to the same `enqueueImportJob` → `ImportMatch` → finalise flow every source uses. Because every Grouvee row carries `igdb_id`, the match stage short-circuits to a direct id match (no `pending_review`); a row that ever lacked the id falls back to title matching, with ambiguous matches landing on the existing `pending_review` surface. **Import is refused unless IGDB is configured** — hydration by id still calls the IGDB API (existing guard).

## Error handling

- **Malformed/empty JSON in `shelves`/`platforms`/`dates`** → the decoder returns no values: the game still imports (status falls to the shelf default or baseline, no platforms, no playtime) via its `igdb_id`. Never a parse failure, never a junk entry.
- **`dates` with `seconds_played: 0` or `"None"` date fields** → no playtime added; the tier (if recognized) still applies.
- **Empty/non-numeric `rating`** → `extractRating` returns nil (unrated), as today.
- **Unknown platform / shelf / tier** → platform passthrough then drop-if-unseeded; unknown shelf → `Default` (`not_started`); unknown tier → ignored (shelf status stands). Game still imports.
- **Wrong-shape file** (signature fails) → 400 wrapping `ErrInvalidSignature`, both on auto-`Parse` and on a manual `format=grouvee` pick against a non-Grouvee file.

## Testing

- **`decodeKeys` (table-driven):** object → keys; `""`/`{}` → nil; malformed JSON → nil; scalar passthrough.
- **`extractStatus`:** single shelf each (`Playing`→in_progress, `Played`→completed, `Backlog`→not_started); multi-shelf precedence (`{Played, Backlog}`→completed); `Wish List` alone → `not_started` + `wishlisted=true`; `{Playing, Wish List}` → `in_progress` + wishlisted; unknown shelf → `not_started`.
- **`extractPlayLog`:** single entry seconds → hours (`36300`→`10.08`); multiple entries summed; `0`/`"None"`/missing → nil hours; tier map (`Main Story`→completed, `Main Story + Extras`→mastered, `100% Completion`→dominated); highest-tier-wins across entries; unrecognized tier → `""`; empty `[]` / malformed → `(nil, "")`.
- **Status resolution:** tier present overrides shelf (`Backlog` + `100% Completion` → `dominated`); tier absent → shelf status; wishlist flag and hours independent of which status wins.
- **Note assembly:** title+body, body-only, title-only, both-empty.
- **Rating:** `5`→5, `4.5`→5, `4.4`→4, empty→nil (scale 5, round).
- **Signature:** matches the Grouvee header; rejects an unrelated header.
- **`Grouvee()` over the trimmed real fixture** (the play-log export): asserts title, `igdb_id`, the tier-overridden play_status (Portal `100% Completion`→`dominated`), `is_wishlisted` (Borderlands 4 Wish List), Portal's rating `5`, `hours_played` `10.08`, `PC (Microsoft Windows)`→`pc-windows`, the empty-`platforms` rows yielding no entries, and `created_at`.
- **Pipeline:** `ImportFinalizeWorker` sets `is_wishlisted` on a new wishlisted (platform-less) game and leaves it set; an owned game clears it via `ClearWishlistOnAcquire`; a pre-existing game is not re-wishlisted (merge path).

Per the project test policy these cover genuinely non-obvious logic (JSON decode, precedence selection, the wishlist sentinel split, tier override, scale normalization) — where a plausible bug would hide. Thin accessors and tautologies are not tested.

## Known gaps (documented, not silent)

- **Platform vocabulary is fixture-derived.** Only `PC (Microsoft Windows)` and `PlayStation 4` are verified from the real export; the map is extensible and unseen values fall back gracefully. Recorded in `docs/grouvee-import.md`.
- **Completion-tier vocabulary is fixture-derived.** Only `Main Story`, `Main Story + Extras`, `100% Completion` are verified; other Grouvee tiers (if any) are ignored for status until added to `CompletionMap`.
- **Custom shelves get `not_started`.** Only Grouvee's four built-in shelves carry semantics; a user's custom shelf maps to the default.
- **A populated tier always overrides the shelf**, even on an in-progress or backlog shelf (the user's explicit choice). In practice a tier is only set on games the user has actually completed.

## Out of scope

- **Auto-detect on `inspect`** — #1015 (this preset registers into the registry it reuses).
- **Recurring/incremental sync** — one-off migration only.
- **The advanced `csvmap` model** (`StatusFlags`, `PlatformTables`, full `NoteAssembly`, `CopyRows`) — #1016. This spec adds new *simple-subset* capabilities, distinct from those.
- **`dates` start/finish dates, `statuses`, provenance notes** for `url`/`giantbomb_id` — dropped, not mapped.
