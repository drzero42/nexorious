# Darkadia CSV import — tolerate extended (feature-toggle) exports

## Problem

The Darkadia CSV importer rejects valid exports. A user's `darkadia_export_csv_20251213.csv`
(29 columns) imports fine, but `darkadia_export_csv_20251215.csv` (40 columns) is rejected
as "not a Darkadia export".

### Root cause

`internal/services/darkadia/darkadia.go` validates the file with `headerMatches`, an exact,
position-by-position comparison against a hardcoded 29-column `header`, and then reads every
field by **fixed integer index** (`colPlatforms = 27`, `colNotes = 28`, …).

The 40-column export is the **same tool with more Darkadia features enabled**. It inserts 11
columns *in the middle* of the row (not appended at the end):

| New column | Inserted after |
|---|---|
| `Date completed` | Finished |
| `Date mastered` | Mastered |
| `Date dominated` | Dominated |
| `Time played`, `Time to complete`, `Time to master`, `Time to dominate` | Shelved |
| `Review subject`, `Review` | Rating |
| `Copy notes` | Copy complete notes |
| `Tags` | Platforms |

The `len() != 29` check fails on the first line, so the file is rejected before any row is read.

This is **not** just a too-strict validator: because the new columns are inserted mid-row, every
fixed index after `Finished` is shifted. Merely loosening the header check would make the importer
silently read the wrong columns and produce corrupt data. The fix must address columns **by header
name**, which `docs/darkadia-import.md:90` already states the parser *must* do but the implementation
never did.

### Secondary finding

`docs/darkadia-import.md` asserts Darkadia provides "No playtime. No tags." The 40-column export
disproves both: it carries a `Tags` column (e.g. `Co-op`, `Dansk`, `VR`) and `Time played`
(e.g. `148:00`). Those earlier exports simply had the features disabled. Nexorious has first-class
homes for both (`user_game_tags`, `user_game_platforms.hours_played`), so this fix also maps them.

## Approach

**Name-indexed parsing.** Parse the header row into a `map[columnName]int`. Validate by checking a
set of *required signature columns* is present (not exact-match). Read every field by name through a
ragged-safe accessor.

Rejected alternatives:
- **Known-header whitelist** (accept a set of exact headers): Darkadia's extra columns are
  independent feature toggles (time-tracking, reviews, tags, completion-dates — each on/off), so the
  valid-header set is combinatorial. Every new toggle combination breaks again.
- **Loosened prefix match**: still index-coupled; breaks because new columns are inserted mid-row.

## Design

### 1. Header model & validation

In `internal/services/darkadia/darkadia.go`:

- Replace the `header []string` value and the positional `col*` constants with a parsed lookup
  `cols map[string]int` built from the header row (exact Darkadia header names → position).
- **Validation:** require the canonical 29 column names to all be present. A non-Darkadia CSV will
  not carry the full signature (`Copy purchase date`, `Dominated`, `Copy source other`, …). Missing
  any required name → `ErrInvalidHeader`. Extra columns are allowed and ignored unless mapped below.
- Introduce a ragged-safe accessor: `field(row, "Copy platform")` returns `row[cols[name]]` when the
  column exists and the row is long enough, else `""`. This preserves today's ragged-row tolerance
  (`FieldsPerRecord = -1`, short trailing rows treated as empty).
- Grouping logic (named row starts a game; empty-`Name` rows attach as copies) is unchanged, keyed
  off the `Name` column resolved by name.

### 2. New field mappings

Extend the `darkadia.Game` payload. Each new column is read **only if present** in `cols`:

| Darkadia column | Level | → Nexorious | Rule |
|---|---|---|---|
| `Tags` | game | `user_game_tags` | comma-split, trim, drop empties; find-or-create per tag; no color |
| `Time played` (`H:MM`) | game | `hours_played` | parse `HHH:MM` → float hours; lands on the **first** consolidated platform entry only |
| `Review subject` + `Review` | game | appended to `personal_notes` | if `Review` non-empty: append a block; `Review subject` (if any) as a heading line, then the body |
| `Copy notes` | copy | provenance line in `personal_notes` | folded like existing provenance lines, deduped against existing note lines |

Payload struct additions:
- `Game.Tags []string` — consolidated, de-duplicated tag names.
- `Game.HoursPlayed *float64` — game-level total playtime in hours (nil when absent/unparseable).
- Review and Copy-notes text are folded into `Game.PersonalNotes` during `consolidate`, so they need
  no new payload fields.

`Time played` parsing: split on `:`, `hours + minutes/60`. Empty or unparseable → nil (no playtime).

### 3. Still dropped (documented, no Nexorious home)

- `Date completed` / `Date mastered` / `Date dominated` — no completion-date field on `user_games`.
- `Time to complete` / `Time to master` / `Time to dominate` — `games.howlongtobeat_*` is IGDB
  community metadata, not per-user data; there is no per-user milestone-time field.

These are added to the accepted-loss ledger.

### 4. Finalize-worker changes

In `internal/worker/tasks/darkadia.go` (`DarkadiaFinalizeWorker.Work`):

- **Playtime:** stamp `hours_played` on the **first** consolidated platform entry. For a fresh game
  the first entry is created carrying the value; in the merge case, set it only if that first entry is
  newly inserted (additive — never overwrite an existing entry's `hours_played`). Game-level total on
  one entry keeps any library-wide sum exactly right (no double-counting across platforms).
- **Tags:** mirror the JSON importer's tag block (`internal/worker/tasks/import_item.go:282-317`):
  build `existingTagIDs` for the merge case, call the existing `findOrCreateTag(ctx, db, userID, name, nil)`
  (same `tasks` package — direct reuse), insert `user_game_tag` links for tags not already present,
  and count new tags toward the `updated` vs `already_in_library` change-type decision.

Merge semantics stay consistent with the existing design: game-level scalar fields (play_status,
rating, loved, notes) remain additive-only; platforms, tags, and the first-entry playtime are merged
in without overwriting existing curation.

### 5. Documentation correction

Update `docs/darkadia-import.md`:
- Column reference: document the optional feature-toggle columns and that parsing is by header name.
- "What Darkadia does not provide": remove the false "No playtime / No tags" claims; note they are
  feature-gated in the export and now mapped when present.
- CSV dialect / validation: replace "29-column exact header" with the signature-based rule.
- Accepted-loss ledger: tags and playtime now mapped; add completion-dates and milestone-times as the
  new documented drops.

## Testing

`internal/services/darkadia/darkadia_test.go`:
- A 40-column header parses; both the 29-col and 40-col headers produce identical **core** output
  (title, play_status, rating, loved, notes, platforms, storefronts, dates) for the same game.
- `Time played` `148:00` → `148.0` on the first platform entry; multi-platform game puts it on the
  first entry only.
- `Tags` `"Co-op, VR"` → `["Co-op","VR"]` in the payload.
- `Copy notes` value → appears as a provenance line in `personal_notes`; deduped.
- `Review` non-empty → appended to notes (subject as heading); empty → no change.
- A non-Darkadia header (missing required signature columns) still returns `ErrInvalidHeader`.
- Ragged short rows still tolerated (missing trailing optional columns treated as empty).

`internal/worker/tasks/darkadia_test.go`:
- Tags created and attached on fresh import; re-import merges without duplicate links.
- `hours_played` set on the first platform entry only; existing entry's hours not overwritten on merge.
- Review / Copy-notes text present in finalized `personal_notes`.

## Out of scope

- No new completion-date or milestone-time fields on `user_games` (would be a schema change beyond
  this fix).
- No migration: this is parser/finalizer behaviour only.
