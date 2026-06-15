# Completionator CSV import as a csvmap Config — #1003

**Issue:** [#1003](https://github.com/drzero42/nexorious/issues/1003) — a CSV-track child of epic [#984](https://github.com/drzero42/nexorious/issues/984) (multi-source game-library import).
**Status:** design approved, ready for an implementation plan.
**Depends on:** [#1014](https://github.com/drzero42/nexorious/issues/1014) — the config-driven `csvmap` engine (merged) — and [#1004](https://github.com/drzero42/nexorious/issues/1004) — the generic CSV import path (merged).
**Pairs with:** [#1015](https://github.com/drzero42/nexorious/issues/1015) — auto-detect, which registers and selects this preset (out of scope here; see boundary below).

## Problem

A user migrating from **Completionator** has a native CSV export but cannot import it today. Two concrete defects block it, both surfaced by running a real export through the existing generic CSV path (`POST /api/import/csv/inspect`):

1. **Malformed quoting.** Completionator quote-wraps every field but does **not** RFC-4180-escape embedded quotes in titles. The exact bytes on one row:

   ```
   "The Walking Dead: The Final Season - Episode 1: "Done Running"",""...
   ```

   The inner quotes around *Done Running* are raw, and the field closes with `""` directly before the separator — there is no valid RFC-4180 reading. Strict `encoding/csv` (current behaviour) correctly errors at the first such row: of 524 data rows only 492 parse before it stops. `r.LazyQuotes = true` is **not** an acceptable fix — verified to silently corrupt (the field never closes, swallows later columns, and the row comes out with 22 columns instead of 24, misaligned). A hard error is safer than silent corruption.

2. **Non-UTF-8 encoding.** A real export is **Windows-1252**, not UTF-8 (e.g. byte `0xf4` = `ô`). Go's `encoding/csv` and the later JSON encoding assume UTF-8, so accented titles would corrupt to `�`.

Beyond parsing, Completionator needs a column **mapping** — the same `csvmap.Config` shape every CSV source uses. This issue delivers that mapping as a reusable preset, plus the shared parsing fix both defects demand.

## Scope

**In:**

- A shared tolerant CSV reader in `internal/services/csvmap` (transcode + strict-first + guarded fallback), replacing the two `csv.NewReader(... FieldsPerRecord = -1)` call sites that both defects flow through.
- A Completionator `csvmap.Config` (+ header signature), exported for #1015 to register.
- `docs/completionator-import.md` — the format spec, modelled on `docs/darkadia-import.md`.
- Tests: the reader (strict, quote-fallback, transcode, guard-misfire) and the Config over a trimmed real fixture.

**Out (deferred / not applicable):**

- **The preset registry, auto-detect on `inspect`, and any confirm/select UX — these are #1015.** After this issue a Completionator file parses correctly and is importable through the **existing manual mapping dialog** (#1004); #1015 makes it automatic by recognising the signature.
- **Advanced `csvmap` engine features (`StatusFlags`, `PlatformTables`, etc.)** — Completionator is fully expressible in the simple subset; nothing from #1016 is pulled forward.
- **Recurring/incremental sync** — all imports are one-off migrations.
- **Canonical platform/storefront fuzzy-matching** — the preset maps the *known* (fixture-verified) values to canonical slugs; anything unseen falls back gracefully (see Known gaps).

## Decisions taken in brainstorming

1. **Play-status from `Progress Status` alone (simple subset).** Completionator spreads play-state across `Progress Status` (Incomplete/Finished) plus boolean flags `Now Playing` and `Backlogged`. True multi-flag precedence is the engine's advanced `StatusFlags`, rejected by `Parse` until #1016. We use the single `Progress Status` column — `Finished` → `completed`, `Incomplete` → `not_started` — and accept that the handful of "Now Playing = Yes" rows import as `not_started` rather than `in_progress`. This keeps #1003 in the simple subset and out of the #1016 capstone.
2. **Transcode non-UTF-8 input in this issue (not deferred).** Decoding accented titles correctly is part of "make Completionator parse correctly," is low-effort, and lives in the shared reader.
3. **Map the `Tags` column, not `Genre`.** `Tags` is the user's own field (empty in the sample, but semantically correct). `Genre` is editorial metadata IGDB re-supplies on match, so it is left unmapped.
4. **Map `Rating` with a 1–10 scale.** A rated export shows integer values `1, 3, 9, 10` → Completionator uses a 1–10 scale. Mapped with `Scale: 10`, round-to-nearest. (This was an open gap until a real rated export verified the scale; we do **not** guess.)
5. **Drop the provenance columns.** `Acquisition Type` (how acquired: Purchase/Gift/…), `Acquisition Source` (free-text retailer/person), `Edition`, `Region`, the money columns (`Est. Value`, `Amt. Paid`), the physical-condition columns (`Box/Case`, `Cart/Disc`, `Manual`, `Extras`), and the release-date columns have no canonical home in our model and are unmapped.

## Architecture

### The tolerant reader (the one engine extension)

New exported function in `csvmap`:

```go
// ReadRecords parses CSV bytes tolerantly and returns every record (header
// included). It (1) transcodes Windows-1252 input to UTF-8 when the bytes are
// not already valid UTF-8, (2) parses strictly with encoding/csv, and (3) only
// when strict parsing fails on a quote error AND the file is uniformly
// quote-wrapped, falls back to a de-quote split. Otherwise it returns the
// strict error rather than risk silent corruption.
func ReadRecords(raw []byte) ([][]string, error)
```

Algorithm:

1. **Transcode.** If `!utf8.Valid(raw)`, decode with `golang.org/x/text/encoding/charmap.Windows1252` → UTF-8. Valid UTF-8 (including pure ASCII) passes through unchanged. (`x/text` is already in `go.mod`; no new dependency, no `go.sum`/vendorHash churn.)
2. **Strict first.** `csv.NewReader` with `FieldsPerRecord = -1` (preserves the engine's existing ragged-row tolerance). On success, return all records. Normal, well-formed CSVs are completely unaffected.
3. **Guarded fallback.** Engage **only** when strict parsing returns a `*csv.ParseError` whose `Err` is a quote error (`csv.ErrQuote`/`csv.ErrBareQuote`). Then split the (transcoded) text into lines and accept the fallback **only if**:
   - every non-empty line is fully quote-wrapped (`^"…"$`), **and**
   - splitting each line on the literal three-byte sequence `","` yields the **same field count** on every line.

   If both hold, return those de-quoted records. If either fails, return the original strict error.

The uniform-field-count guard is what makes the fallback safe against the issue's named edge cases: a partially-quoted file, a multi-line quoted field (embedded newline), or a literal `","` inside a field all break the guard, so the reader errors instead of mis-aligning. This honours "a hard error is safer than silent corruption."

### Shared by both call sites

```
raw bytes
   │
   ▼
csvmap.ReadRecords(raw) ──► [][]string  (header = records[0], data = records[1:])
   │                                   │
   ▼                                   ▼
csvmap.Parse (parse.go)        HandleImportCSVInspect (import_csv.go)
   signature + extract            headers + row_count + distinct values
```

- **`parse.go`:** `Parse` replaces its inline `csv.NewReader` loop with `ReadRecords`, then takes `records[0]` as the header (signature check) and `records[1:]` as data rows. `Parse`'s exported signature is unchanged.
- **`import_csv.go`:** `HandleImportCSVInspect` replaces its inline `csv.NewReader` streaming loop with a `ReadRecords` call, then iterates the returned slice for its existing header / row-count / capped distinct-value logic. No wire-contract change.

Both currently error on a Completionator file; after this change both read it correctly. The manual-mapping path (`HandleImportCSV` → `buildCSVConfig` → `csvmap.Parse`) is unblocked for Completionator with no further change.

### The Completionator Config

Exported as `func Completionator() csvmap.Config` (a constructor, matching how presets carry the full `Config` as data). The 24 export columns map as:

| Canonical field | Source column | Handling |
|---|---|---|
| Title (required) | `Name` | verbatim |
| play_status | `Progress Status` | `ValueMap`: `finished`→`completed`, `incomplete`→`not_started`; `Default`: `not_started` |
| Platform | `Platform` | `PlatformMap`: `pc / windows`→`pc-windows`, `playstation 5`→`playstation-5`; miss → passthrough |
| Storefront | `Format` | `StorefrontMap`: `digital (steam)`→`steam`, `digital (gog)`→`gog`, `physical (unassigned)`/`physical (new)`/`physical (used)`→`physical`; miss → no storefront |
| Acquired date | `Acquisition Date` | on the platform entry; layout `1/2/2006` |
| Created date | `Added On` | `ColumnMap.CreatedAt`; layout `1/2/2006` |
| personal_rating | `Rating` | `RatingConfig{Scale: 10, Truncate: false}` |
| Tags | `Tags` | default `,` separator |
| Grouping | — | `MergeByTitle: true` (union platform entries across same-title rows) |

Map keys are normalized (lowercased/trimmed) to match the engine's `normKey` lookups. Slug targets are the seeded `platforms.name` / `storefronts.name` values (`pc-windows`, `playstation-5`, `steam`, `gog`, `physical`).

**Signature** (all must be present, matched normalized) — a distinctive subset of Completionator-specific headers rather than all 24, so a minor column addition upstream does not break detection:

```
Now Playing, Backlogged, Ownership Status, Acquisition Source, Added On
```

### Data flow into the existing pipeline

No new pipeline, no migration, no schema change. The Config produces `[]importmodel.Game`, handed to the same `enqueueImportJob` → `ImportMatch` → `pending_review` → finalise → additive-merge flow every source uses. Platform/storefront slugs that match a seeded `name` attach; unmatched platform → game imports without that platform (warned by `import_item`); unmatched storefront → platform recorded without a storefront. Import is refused unless IGDB is configured (existing guard).

## Error handling

- **Unrecoverably malformed CSV** (strict fails, guard does not hold) → `ReadRecords` returns the original strict `encoding/csv` error; callers surface it as the existing 400 ("failed to parse CSV: …"). No silent corruption.
- **Empty / header-less file** → strict read of the header fails; existing 400 paths unchanged.
- **Unknown platform / storefront value** → graceful: skip the platform / drop the storefront, game still imports (existing `import_item` behaviour).
- **Empty or non-numeric `Rating`** → `extractRating` already returns nil (no rating); a `0` or out-of-range value → nil.

## Testing

- **`ReadRecords` (table-driven):**
  - well-formed UTF-8 CSV → strict path, records intact (no fallback).
  - Completionator-style malformed quotes, uniformly wrapped → fallback recovers; the line-494 title parses as `The Walking Dead: The Final Season - Episode 1: "Done Running"` with all 24 fields aligned.
  - Windows-1252 bytes (accented title) → transcoded; the rune is correct, not `�`.
  - guard-misfire cases → original error, **not** a corrupt record: (a) partially-quoted file (some lines not `^"…"$`), (b) a quoted field containing an embedded newline, (c) ragged de-quoted line counts.
- **`Completionator()` over a trimmed real fixture** (a handful of rows incl. the line-494 title, the lone `PlayStation 5` row, and the four rated rows `1/3/9/10`): asserts title, play_status (`Finished`→completed, `Incomplete`→not_started), platform/storefront slugs, created/acquired dates, and rating stars (`1`→1, `3`→2, `9`→5, `10`→5).
- **Signature:** matches the Completionator header; rejects an unrelated header.

Per the project test policy these cover security-irrelevant but genuinely non-obvious logic (the fallback heuristic, scale normalization, slug mapping) — exactly where a plausible bug would hide.

## Known gaps (documented, not silent)

- **Platform / `Format` vocabulary is fixture-derived.** Only the values present in a real export are verified; the maps are extensible, and any unseen value falls back gracefully (platform dropped with a warning, or storefront omitted). `docs/completionator-import.md` records this.
- **`Now Playing` / `Backlogged` flags are not honoured** for play-status (see decision 1) — a deliberate simple-subset trade-off, revisitable if/when `StatusFlags` lands in #1016.

## Addendum (2026-06-15): manual format selection rolled in

The original scope left the preset reachable only in theory — nothing referenced `Completionator()`, so a user had no way to say "this is a Completionator CSV." The manual column-mapping dialog (#1004) parses the file post-fix but **cannot apply the preset's platform/storefront value→slug maps** (the flat `csvMapping` DTO has no value-map field), so platforms import unmapped. We therefore roll a **manual format selector** into this PR. (Auto-detection by signature remains #1015; this is the manual escape hatch and the registry #1015 builds on.)

**UX:** a **Format** dropdown at the top of the existing CSV mapping dialog — `Generic CSV` (default) plus each registered preset (`Completionator`). Choosing `Generic CSV` shows today's mapping form unchanged. Choosing a preset **collapses the mapping form** (there is nothing to map) and shows a short note; Import then sends the preset slug.

**Crux:** a preset import uses the **server-side `Config`**, never a client-reconstructed mapping — otherwise the slug maps are lost. So the request carries `format=<slug>` instead of `mapping`, and the handler runs `csvmap.Parse(body, preset)`.

**Components:**
- `csvmap` preset registry: `Preset{Slug, DisplayName, Config}`, `Presets() []Preset`, `PresetBySlug(slug) (Config, bool)`. Completionator registered. (#1015 reuses this for auto-detect.)
- `POST /api/import/csv`: optional `format` field. Known slug → registry `Config` (ignore `mapping`); empty/`generic` → existing `buildCSVConfig(mapping)`. Unknown slug → 400. A preset whose header **signature** fails → 400 "this file does not match the selected format" (the manual choice is validated against the file, preventing silent mis-mapping).
- `POST /api/import/csv/inspect`: response gains `presets: [{slug, name}]` (from the registry) so the dropdown is server-driven.
- Frontend: `Format` select in `csv-mapping-dialog.tsx`; conditional rendering; `CsvInspectResponse.presets`; `importCsv(file, format, mapping)` + hook; route `handleCsvImport` wiring.

**Decision — signature is enforced for a manual pick.** Picking "Completionator" then uploading a non-Completionator file is rejected rather than mis-mapped. Reliable for Completionator's 5-column signature; revisit if a real export legitimately fails it.
