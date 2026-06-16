# First-class Nexorious-CSV import format (own-export round-trip) — #1033

**Issue:** [#1033](https://github.com/drzero42/nexorious/issues/1033) — a CSV-track child of epic [#984](https://github.com/drzero42/nexorious/issues/984) (multi-source game-library import).
**Status:** design approved, ready for an implementation plan.
**Depends on:** [#1014](https://github.com/drzero42/nexorious/issues/1014) — the config-driven `csvmap` engine — [#1004](https://github.com/drzero42/nexorious/issues/1004) — the generic CSV import path — [#1022](https://github.com/drzero42/nexorious/issues/1022) — IGDB-id direct match — [#1021](https://github.com/drzero42/nexorious/issues/1021) — auto-guess mapping — and [#1003](https://github.com/drzero42/nexorious/issues/1003) — the manual Format selector. All merged.
**Pairs with:** [#1015](https://github.com/drzero42/nexorious/issues/1015) — auto-detect on `inspect` (out of scope here; this preset registers into the same registry #1015 reuses).

## Problem

Nexorious can already *export* a CSV (`csvHeaders` in `internal/worker/tasks/export.go`), and re-importing it *mostly* works today through the generic user-mapped path (#1004) plus auto-guess (#1021) and the IGDB-id short-circuit (#1022). But there is **no registered preset/signature** for our own format, so a Nexorious CSV is not a recognised import format in the Format dropdown and is invisible to auto-detect (#1015). And one column — `platforms` — does **not** survive the generic path: it is semicolon-joined slugs, a shape the current engine cannot read.

This spec closes the "export to CSV → re-import" loop that JSON export/import already has but CSV currently lacks: it authors a `NexoriousCSV()` `csvmap.Config` (header signature + column mapping), registers it as a preset, and extends the engine by exactly the one capability the format genuinely requires (a delimited platform column).

## Verification of the export format (current)

Confirmed against `internal/worker/tasks/export.go` (`csvHeaders`, line 306; `buildCSVRow`, line 350).

Header (11 columns):

```
title, igdb_id, play_status, personal_rating, is_loved, hours_played, personal_notes, platforms, tags, created_at, updated_at
```

Cell encodings:

- `title` — the game title (`Game.Title`).
- `igdb_id` — `strconv.Itoa(int(ug.GameID))`; `games.id` is the IGDB-keyed id, so this rides the **#1022 direct-match path** (hydrate by id, skip title matching, never touch `pending_review`).
- `play_status` — written **verbatim** as a canonical value. The eight canonical values (`internal/enum/enum.go`): `not_started`, `in_progress`, `completed`, `mastered`, `dominated`, `shelved`, `dropped`, `replay`. Empty cell when `PlayStatus` is nil.
- `personal_rating` — whole `1`–`5` (`strconv.Itoa`); empty when unrated.
- `is_loved` — `true`/`false` (`strconv.FormatBool`).
- `hours_played` — decimal, **summed across all platforms** (`totalHoursF`); empty when the sum is 0.
- `personal_notes` — verbatim.
- `platforms` — **semicolon-joined platform slugs** (e.g. `pc-windows;playstation-5`); already canonical `platforms.name` slugs, no display-name mapping.
- `tags` — **semicolon-joined** tag names.
- `created_at` / `updated_at` — **RFC3339** (`time.RFC3339`, UTC).

## Why the generic path is not enough

1. **No signature/preset.** Without an entry in `presetList`, a Nexorious export is not a selectable Format and is invisible to #1015's auto-detect.
2. **`play_status` falls to the default.** `extractStatus` maps each cell value through `ValueMap`; an unmapped value is skipped and the row falls to `Default` (`not_started`). Verbatim canonical values therefore need an **explicit identity `ValueMap`** over the eight values — otherwise every imported game becomes `not_started`.
3. **`platforms` cannot be read.** `PlatformSimple` supports only scalar (one entry) or `json-keys` (one per object key). A semicolon-joined slug list (`pc-windows;playstation-5`) is neither: scalar would emit one bogus entry named after the whole joined string; `json-keys` would fail to parse. The engine needs a delimited-split capability.

## Decision taken in brainstorming

**The `platforms` column is round-tripped via a small engine extension (issue option a), not accepted as a gap (option b).** The issue's goal is a *faithfully round-tripping* format, and the extension is small and mirrors the established pattern (the Grouvee `json-keys` platform extension). A documented gap would silently drop the user's curated platform ownership on every re-import.

## Architecture

One generic engine extension, the `NexoriousCSV()` `Config`, the registry entry, the doc, and tests. No migration, no schema change, no new pipeline.

### Engine extension — delimited platform column

`PlatformSimple` gains an optional separator. When set, the platform cell is split on it (rather than read as a scalar or JSON object), yielding one ownership entry per slug:

```go
type PlatformSimple struct {
	PlatformColumn     string
	PlatformFormat     ColumnFormat      // "" scalar | "json-keys"
	PlatformSeparator  string            // NEW: when non-empty, split PlatformColumn on this; one entry per piece
	StorefrontColumn   string
	AcquiredDateColumn string
	PlatformMap        map[string]string
	StorefrontMap      map[string]string
}
```

`extractPlatforms` chooses its value source: when `PlatformSeparator != ""`, it splits `cell(PlatformColumn)` on the separator, trims, and drops empties (mirroring `extractTags`); otherwise it calls `decodeKeys(cell, PlatformFormat)` as today. The rest of the function is unchanged — `PlatformMap` (miss → passthrough), per-row dedupe by slug, and the optional storefront/acquired-date scalar columns all apply identically. For the Nexorious preset the slugs are already canonical, so `PlatformMap` is unset; `StorefrontColumn`/`AcquiredDateColumn` are unset (the column carries slugs only).

**Validation** (`validate.go`): `PlatformSeparator` is only meaningful with the scalar format. Setting both `PlatformSeparator != ""` and `PlatformFormat == json-keys` is a descriptive config error (NOT `ErrInvalidSignature`) — the two value sources are mutually exclusive. `decodeKeys` is untouched.

This mirrors the existing `TagSeparator` idiom (a Config separator + a split in the extractor) and the Grouvee precedent of extending the simple subset by exactly what a real export requires.

### The `NexoriousCSV()` `Config`

`func NexoriousCSV() Config`, registered in `presetList`:

| Canonical field | Source column | Handling |
|---|---|---|
| Title (required) | `title` | `ColumnMap.Title`; verbatim |
| IGDB id | `igdb_id` | `ColumnMap.IGDBID`; positive int → #1022 direct match |
| play_status | `play_status` | `Status.Column`, scalar, **identity `ValueMap`** over the 8 canonical values; `Default: "not_started"` |
| personal_rating | `personal_rating` | `RatingConfig{Scale: 5}` (whole 1–5 in, whole 1–5 out) |
| is_loved | `is_loved` | `ColumnMap.Loved`; default truthy set (`1`/`true`/`yes`) matches the exported `true`/`false` |
| hours_played | `hours_played` | `ColumnMap.HoursPlayed` + `DurationConfig{Format: "decimal"}` |
| personal_notes | `personal_notes` | `NotesConfig{Column: "personal_notes"}` (verbatim, no heading) |
| Platforms | `platforms` | `PlatformSimple{PlatformColumn: "platforms", PlatformSeparator: ";"}` (the new extension); slugs passthrough |
| Tags | `tags` | `ColumnMap.Tags` + `TagSeparator: ";"` |
| created_at | `created_at` | `ColumnMap.CreatedAt` + `DateLayout: time.RFC3339` |
| `updated_at` | — | **dropped** (no model home; documented) |
| Grouping | — | one-row (`MergeByTitle: false`); each row is a distinct game |

The `play_status` identity map is the eight canonical values each mapping to themselves:

```
ValueMap: {"not_started":"not_started", "in_progress":"in_progress",
           "completed":"completed", "mastered":"mastered",
           "dominated":"dominated", "shelved":"shelved",
           "dropped":"dropped", "replay":"replay"}
Default:  "not_started"
```

(No `Precedence` — the scalar column yields at most one value, so the single mapped candidate is chosen.)

**Signature** (all present, matched normalized) — a distinctive Nexorious subset, not all 11, so a future minor column addition does not break detection and the subset does not collide with the Grouvee/Completionator signatures or a generic CSV:

```
play_status, personal_rating, is_loved, hours_played, personal_notes
```

### Registration

```go
// presets.go
{Slug: "nexorious", DisplayName: "Nexorious CSV", Config: NexoriousCSV()},
```

Per the #1003 wiring this is the only integration step: `POST /api/import/csv/inspect` returns `{slug:"nexorious", name:"Nexorious CSV"}` in `presets`, the CSV mapping dialog's **Format** dropdown lists it automatically (no frontend change), and `POST /api/import/csv` with `format=nexorious` runs `csvmap.Parse(body, NexoriousCSV())` server-side with the signature enforced against the upload. Auto-detection on the same registry remains #1015.

## Data flow into the existing pipeline

No new pipeline, migration, or schema change. `NexoriousCSV()` produces `[]importmodel.Game`, handed to the same `enqueueImportJob` → `ImportMatch` → finalise flow every source uses. Because every Nexorious row carries `igdb_id`, the match stage short-circuits to a direct id match (no `pending_review`). **Import is refused unless IGDB is configured** — hydration by id still calls the IGDB API (existing guard).

## Error handling

- **Empty/invalid `play_status`** → falls to `Default` (`not_started`), as for any unmapped value.
- **Empty/non-numeric `personal_rating`** → `extractRating` returns nil (unrated).
- **Empty/0 `hours_played`** → nil hours.
- **Empty `platforms`** → no ownership entries; a blank or whitespace-only piece between separators is dropped (no junk entry).
- **Unparseable `created_at`** → `extractDate` returns "" (no created-at override; finalise uses its default).
- **Wrong-shape file** (signature fails) → 400 wrapping `ErrInvalidSignature`, both on auto-`Parse` and on a manual `format=nexorious` pick against a non-Nexorious file.

## Testing

Per the project test policy, tests target the genuinely non-obvious logic (the new split, the identity-map status path, the signature) over a real exported fixture — where a plausible bug would hide. Thin accessors and tautologies are not tested.

- **`PlatformSeparator` split (`extractPlatforms`, table-driven):** `"pc-windows;playstation-5"` → two entries; single slug → one; `""` → none; trailing/empty pieces (`"pc-windows;"`, `"pc-windows;;mac"`) trimmed/dropped; duplicate slug deduped; scalar/json-keys paths unchanged when `PlatformSeparator` is unset.
- **Validation:** `PlatformSeparator` + `json-keys` together → descriptive error (not `ErrInvalidSignature`); `PlatformSeparator` + scalar → accepted.
- **`NexoriousCSV()` over a real exported fixture:** asserts every row matched by `igdb_id` (id set, nothing routed to title matching / `pending_review`); `play_status` round-trips for several canonical values incl. one of the rarer ones (`shelved`/`replay`); empty `play_status` → `not_started`; `personal_rating` whole-number round-trip; `is_loved` `true`/`false`; `hours_played` decimal; `personal_notes` verbatim; `tags` split on `;`; **`platforms` split on `;` → multiple ownership entries**; `created_at` parsed from RFC3339.
- **Signature:** matches the exported header; rejects an unrelated header (e.g. the Grouvee or Completionator header).

## Documented round-trip caveats (faithful, not silent — recorded in the import docs)

- **`updated_at` is dropped** — no model home on import.
- **`hours_played` is the export-summed total** across all platforms; on re-import it attaches as the game-level hours via the existing generic path — the per-platform split is not recovered.
- **Platforms come back without storefront, acquired-date, or per-platform hours** — the export column carries slugs only, so re-imported ownership entries have those fields empty.

## Out of scope

- **Auto-detect on `inspect`** — #1015 (this preset registers into the registry it reuses).
- **A richer export** (per-platform hours/storefront/acquired-date columns) that would make the round-trip lossless — not in this issue; the caveats above are accepted.
- **The advanced `csvmap` model** (`StatusFlags`, `PlatformTables`, full `NoteAssembly`, `CopyRows`) — #1016. This spec adds one new *simple-subset* capability, distinct from those.
