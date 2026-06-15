# CSV import: auto-guess column mapping from headers — #1021

**Issue:** [#1021](https://github.com/drzero42/nexorious/issues/1021) — follow-up to [#1004](https://github.com/drzero42/nexorious/issues/1004) in the CSV-import track of epic [#984](https://github.com/drzero42/nexorious/issues/984).
**Status:** design approved, ready for an implementation plan.
**Depends on:** #1004 (the generic CSV import dialog + `csvmap.Config` translation, merged as PR #1018) and #1014 (the `csvmap` engine, PR #1017).

## Problem

The generic CSV import dialog (#1004) opens with **every column unmapped**: the form state is seeded by `emptyCsvMapping()`, so the user must pick each field by hand from scratch — even when their CSV's headers are unambiguous (`Name`, `Platform`, `Status`, `Hours`, …). For a typical export this is a dozen manual selections before they can import.

We want to **pre-populate** the dialog with a best-effort guess derived from the CSV's headers (and, where cheap, its values), leaving the user to confirm or adjust. The guess must never change *what is imported* — it only seeds the form; the submitted mapping is still the single source of truth for the import.

## Scope

In:

- A **backend** heuristic in `internal/services/csvmap` that, given the parsed CSV, produces a `SuggestedMapping` (a value shaped to fill the frontend `CsvMapping` DTO): which header maps to each canonical field, the status column, a per-status-value guess, and an inferred rating scale.
- Extending the `POST /api/import/csv/inspect` response with a `suggested_mapping` field, computed during the handler's existing single streaming pass.
- **Frontend:** the dialog seeds its initial state from `inspect.suggested_mapping` (falling back to empty); and each column dropdown hides headers already claimed by *other* fields, recomputed purely from current mapping state so changing/clearing a field instantly frees its column again.
- Tests: Go table-driven tests for the three guess functions; an inspect-endpoint test asserting `suggested_mapping`; a frontend test for the available-columns helper and for the dialog seeding from a suggestion.

Out (explicitly deferred / not in this issue):

- Auto-detecting a *known* CSV format to skip the dialog entirely — that is #1015 (format signature matching), a different mechanism.
- Canonical platform/storefront fuzzy-matching of *values* — v1 still stores platform/storefront values as-is (passthrough). The guess only matches *headers* to fields and *status values* to play statuses.
- Any change to the import pipeline, job model, or `csvmap.Parse` behaviour.

## Why the backend (placement decision)

The guess could live entirely in the frontend (it only seeds form state). It is placed on the **backend** instead, for three reasons:

1. **Cohesion.** `/api/import/csv/inspect` exists precisely to help the client build a mapping. "Here is the CSV, and here is what we think it maps to" is one responsibility; `suggested_mapping` sits naturally beside `headers`/`columns`.
2. **One source of truth for the field vocabulary.** The header→field heuristic is domain knowledge (a column named `system` means Platform). That knowledge belongs next to the canonical field set and the `csvmap` engine, not duplicated in a second TypeScript copy. A future non-web consumer (CLI import) reuses it for free.
3. **Uncapped data for the rating-scale guess.** The inspect response caps each column's distinct values at 50; the backend has the *full* column, so it can scan the true numeric max to infer the scale. The frontend never sees that.

No import cycle: `internal/api` already imports `csvmap`, so `csvmap` defines `SuggestedMapping` (JSON-tagged to match the frontend `CsvMapping`) and the handler just serializes it.

## Decisions taken in brainstorming

1. **Matching strategy: normalized + alias, confident matches only.** Normalize each header (lowercase, strip all non-alphanumerics) and match against a per-field alias list. Try **exact-normalized** equality across all headers first; then a **whole-word "contains"** pass for the still-unmatched fields. No fuzzy/edit-distance matching — it produces surprising wrong guesses the user must hunt down and undo, which is worse than leaving a field blank.
2. **First column wins; each header is claimed at most once.** Headers are scanned in file order; the first header that matches a field claims it, and a claimed header is not reused for another field. This avoids one header seeding two fields (e.g. a stray `name` column landing in both Title and elsewhere). Title takes precedence when contended.
3. **Pre-guess the rating scale from values.** When a rating column is matched, scan its full (uncapped) values, parse the numeric ones, take the max: `max ≤ 5 → 5`, `≤ 10 → 10`, else `100`. Unparseable / no numeric values → fall back to `5` (the existing default). The user can still override in the dialog.
4. **Pre-guess per-value status mappings.** When a status column is matched, map each distinct value (normalized) against a play-status synonym table; unmatched values keep the `not_started` default (the engine's `unmapped → Default` rule). Guessing is bounded to the same distinct values the dialog renders (the 50-cap set), so an enormous status column cannot blow up the suggestion.
5. **Hide already-mapped columns, derived purely from current state.** Each dropdown's option list = `[— none —, the field's own current value, every header not claimed by another field]`. It is recomputed from the current `CsvMapping` on every render — no stored exclusion set — so clearing or reassigning a field immediately returns its column to the other dropdowns. (Constraint, not just a nicety: a stored set would desync on change.)
6. **The guess only seeds; it never overrides the user.** The dialog still submits exactly the mapping shown. A wrong guess is at worst a few corrections; the import semantics are unchanged from #1004.

## Field alias vocabulary (initial)

Normalized aliases per field (exact-normalized, then whole-word contains). Indicative, finalized in the plan/tests:

| Field | Aliases (normalized) |
|---|---|
| `title` | `name`, `title`, `game`, `gamename`, `gametitle` |
| `platform` | `platform`, `system`, `console`, `device` |
| `storefront` | `storefront`, `store`, `source`, `launcher`, `service` |
| `status` (column) | `status`, `playstatus`, `state`, `progress`, `completion`, `completionstatus` |
| `rating` | `rating`, `score`, `stars`, `rate` |
| `notes` | `notes`, `note`, `review`, `comment`, `comments` |
| `acquired_date` | `acquired`, `acquireddate`, `dateacquired`, `dateadded`, `added`, `purchased`, `purchasedate`, `bought` |
| `hours_played` | `hours`, `hoursplayed`, `playtime`, `timeplayed`, `hrs`, `playtimehours` |
| `tags` | `tags`, `tag`, `labels`, `label`, `categories`, `genres` |
| `loved` | `loved`, `favorite`, `favourite`, `fav`, `liked`, `starred` |

Status-value synonym table (normalized → `PlayStatus`), unmatched → `not_started`:

| play_status | source synonyms (normalized) |
|---|---|
| `completed` | `completed`, `complete`, `beaten`, `finished`, `done`, `100`, `100percent` |
| `in_progress` | `inprogress`, `playing`, `started`, `current`, `ongoing` |
| `not_started` | `notstarted`, `backlog`, `unplayed`, `neverplayed`, `tobeplayed`, `tbp`, `wishlist` |
| `dropped` | `dropped`, `abandoned`, `quit`, `gaveup` |
| `shelved` | `shelved`, `onhold`, `hold`, `paused`, `suspended` |
| `mastered` | `mastered`, `platinum`, `perfected` |
| `dominated` | `dominated` |
| `replay` | `replay`, `replaying`, `revisiting` |

## Architecture & data flow

```
POST /api/import/csv/inspect   (multipart: file)
   │  read header
   │  guess header→field map from headers alone (cheap, pure)
   │  single streaming pass over rows:
   │     • per-column distinct values (capped 50) — existing
   │     • numeric max of the guessed rating column (uncapped) — new
   │  assemble SuggestedMapping = header guess
   │                            + GuessRatingScale(max)
   │                            + GuessStatusValueMap(status column's distinct)
   ▼
{ headers, row_count, columns[…], suggested_mapping }
   │
   ▼  CsvMappingDialog seeds state from suggested_mapping (else emptyCsvMapping())
      each dropdown hides columns claimed by other fields (derived from state)
   │  user confirms / adjusts
   ▼
POST /api/import/csv   (unchanged — submitted mapping is authoritative)
```

The header guess runs *before* the streaming pass so the handler knows the rating column index (to track its max) and the status column index up front. This reuses the existing single pass and avoids retaining all rows in memory (consistent with the handler's current care to keep `seen` bounded to the 50-cap).

## Wire contract change

`csvInspectResponse` (and the frontend `CsvInspectResponse`) gains:

```json
"suggested_mapping": {
  "columns": {
    "title": "Name", "platform": "System", "storefront": "",
    "rating": "Score", "notes": "", "acquired_date": "",
    "hours_played": "Hours", "tags": "", "loved": ""
  },
  "status": { "column": "Status", "value_map": { "Beaten": "completed", "Playing": "in_progress" } },
  "rating_scale": 10,
  "merge_by_title": true
}
```

`merge_by_title` defaults to `true` (unchanged from `emptyCsvMapping`). Fields with no confident match are `""`. The shape is byte-for-byte the existing `CsvMapping`, so the dialog drops it straight into `useState`.

## Backend surface

New `internal/services/csvmap/guess.go`:

- `SuggestedMapping` struct — JSON-tagged mirror of the frontend `CsvMapping`.
- `GuessColumns(header []string) ColumnGuess` — pure header→field matcher (returns field→header, status column, rating column name/index).
- `GuessRatingScale(max float64) int` — `5` / `10` / `100`.
- `GuessStatusValueMap(distinct []string) map[string]string` — synonym table.
- A small `normalize(string) string` helper (lowercase + strip non-alphanumerics), shared with whatever the engine already uses if applicable.

`HandleImportCSVInspect` orchestrates: guess columns from the header, track the rating column's max during the existing loop, build and attach `SuggestedMapping`.

## Frontend surface

- `CsvInspectResponse` gains `suggested_mapping: CsvMapping`.
- `CsvMappingForm` seeds `useState<CsvMapping>(() => inspect.suggested_mapping ?? emptyCsvMapping())`.
- New pure helper (in `csv-mapping.ts`) e.g. `availableHeaders(mapping, allHeaders, currentValue) → string[]` returning the headers not claimed by any *other* field, plus `currentValue`. Used by every column select (title, status, and the optional fields). Recomputed each render from `mapping`, so a change frees columns immediately.

## Testing strategy

- **Go (`guess_test.go`)**: table-driven — exact-alias matches, whole-word-contains matches, no-match leaves blank, first-wins/dedup (a header claimed once), rating-scale inference (5/10/100 + unparseable→5), status synonym mapping (+ unmatched→not_started). These are the bulk; the heuristic is the non-obvious logic worth locking down.
- **Go (endpoint)**: extend an inspect test to assert `suggested_mapping` is present and correct for a representative CSV (`Name,Platform,Status,Score`).
- **Frontend**: unit-test `availableHeaders` (other-field exclusion + own-value retention + frees-on-change); a dialog test asserting the title select is pre-filled from `suggested_mapping`.

## Risks / notes

- **Over-eager guesses.** Mitigated by confident-match-only (no fuzzy) and the user always confirming. A wrong guess costs a correction, never a bad import.
- **Genres → tags.** Mapping `genres`→Tags is opinionated; kept because Nexorious has no dedicated genre field on the user side and tags are the closest home. Flagged for review.
- **`source`→storefront vs other meanings.** `source` is a plausible storefront header but also generic; left in the storefront alias list as whole-word only, accepted as low-risk since the user confirms.
