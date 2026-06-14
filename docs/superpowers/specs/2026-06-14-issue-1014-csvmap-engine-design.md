# Config-driven CSV mapping engine (`csvmap`) — #1014

**Issue:** [#1014](https://github.com/drzero42/nexorious/issues/1014) — foundation of the CSV-import track in epic [#984](https://github.com/drzero42/nexorious/issues/984).
**Status:** design approved, ready for an implementation plan.
**Blocks:** #1004 (generic CSV), #1002 (Grouvee), #1003 (Completionator), #1015 (auto-detect), #1016 (absorb Darkadia). Nothing in the CSV track can start until this lands.

## Problem

#1000 unified the import *pipeline* (matching → `pending_review` → finalise → additive merge → job history) but left each source's **mapping** layer as a bespoke `Parse([]byte)` function. For **CSV** sources that mapping layer can itself be unified: verified against `internal/services/darkadia/darkadia.go`, every CSV mapping operation is either **declarative data** (column→field maps, status value/precedence maps, platform/storefront tables, rating scale, duration/date formats, truthy values, grouping mode) or a **reusable engine capability** parameterised by that data (grouping/consolidation, storefront-resolution precedence, `(platform, storefront)` dedup, provenance-note assembly). None of it is inherently source-specific control flow.

So a CSV "source" should be a **`Config` value** (+ an optional header signature), not a hand-written mapper. This issue introduces that engine.

## Scope

In:

- A new leaf-ish package `internal/services/csvmap` with:
  - A **comprehensive `Config` type** holding the *full* feature range — including the advanced (Darkadia-era) features — so the type is frozen now and #1016 adds engine behaviour and a `darkadia` `Config` *value* with **zero new fields**.
  - `Parse(raw []byte, cfg Config) ([]importmodel.Game, error)` implementing the **simple subset only**.
  - `MatchesSignature(headers []string, cfg Config) bool`, the header-signature check (wraps `importmodel.ErrInvalidSignature`), exposed for #1015 reuse.
- Unit tests over **synthetic** `Config`s (no real source, no DB, no fixtures).

Out (own issues):

- The generic interactive flow / inspect endpoint / mapping dialog (#1004).
- Grouvee / Completionator `Config`s (#1002 / #1003).
- Auto-detect of known formats (#1015).
- Absorbing Darkadia + **implementing** the advanced engine features (#1016). The advanced `Config` *fields* are declared here; their *behaviour* is not.
- Any change to `darkadia`, `vglist`, the registry, the API, the frontend, or the schema. This package lands with **no user-visible effect**.

## Decisions taken in brainstorming

1. **Full shape declared now (option B).** The `Config` *type* is complete on landing: every Darkadia input from `darkadia.go` has a config home. The hard, risky work — reproducing Darkadia byte-for-byte — stays in #1016, behind its existing test contract. This freezes the foundational type without speculatively guessing behaviour: #1016 fills in engine logic + a `Config` value, not new fields.
2. **Variants as optional pointer sub-structs.** A feature's simple-vs-advanced variants are nil-able sub-structs; the engine dispatches on which is non-nil and validates at most one. No mode-enum to keep in sync; presets stay plain declarative data (not code), which keeps "a source is a `Config`" true and a future JSON-driven generic mapping feasible.
3. **Advanced slots are rejected, not ignored.** `Parse` returns a descriptive error (distinct from `ErrInvalidSignature`) if any advanced slot is populated, so a preset author who reaches for an unimplemented feature fails loudly.

## The `Config` type (frozen)

`SIMPLE` fields/sub-structs are implemented in #1014; `ADVANCED #1016` pointers are declared but rejected by `Parse` until #1016.

```go
package csvmap

type Config struct {
    Signature    []string        // headers that must all be present; nil = accept any non-empty CSV
    Columns      ColumnMap       // plain scalar field -> source header
    Status       StatusConfig
    Platform     PlatformConfig
    Notes        NotesConfig
    Grouping     GroupingConfig
    Rating       *RatingConfig   // nil = ignore ratings
    Duration     *DurationConfig // nil = ignore hours_played
    TruthyValues []string        // Loved truthy set (normalized); default {"1","true","yes"}
    TagSeparator string          // default ","
    DateLayout   string          // Go layout for date columns; "" = "2006-01-02"
}

type ColumnMap struct {
    Title       string // required
    Rating      string
    CreatedAt   string // game "added"/created date
    HoursPlayed string
    Tags        string
    Loved       string
}

// Status — at most one of Column / Flags.
type StatusConfig struct {
    Column *StatusColumn // SIMPLE
    Flags  *StatusFlags  // ADVANCED #1016 (Parse rejects)
}
type StatusColumn struct {
    Column   string
    ValueMap map[string]string // normalized source value -> play_status
    Default  string            // empty/unmapped -> this, e.g. "not_started"
}
type StatusFlags struct { // Darkadia: Dominated>Mastered>Finished>Shelved>Playing>Played
    Rules   []FlagRule // first matching rule (in order) wins
    Default string
}
type FlagRule struct {
    Column string
    Truthy []string // values meaning "set", e.g. {"1"}
    Status string
}

// Platform — at most one of Simple / Tables.
type PlatformConfig struct {
    Simple *PlatformSimple // SIMPLE
    Tables *PlatformTables // ADVANCED #1016 (Parse rejects)
}
type PlatformSimple struct {
    PlatformColumn     string
    StorefrontColumn   string            // optional
    AcquiredDateColumn string            // optional; attaches to the platform entry
    PlatformMap        map[string]string // optional value->slug; nil = passthrough as-is (#1004)
    StorefrontMap      map[string]string // optional value->slug
}
type PlatformTables struct {
    AggregateColumn    string // comma-separated owned list ("Platforms")
    PlatformColumn     string // per-copy ("Copy platform")
    SourceColumn       string // digital source ("Copy source")
    SourceOtherColumn  string // free-text when SourceColumn == OtherSentinel
    OtherSentinel      string // e.g. "Other"
    MediaColumn        string // "Copy media"
    MediaPhysicalValue string // value meaning physical, e.g. "Physical"
    PurchaseDateColumn string // per-copy acquired date
    Platforms          map[string]PlatformMapping // source string -> {slug, inferred storefront}
    Storefronts        map[string]string          // recognized source (lowercased) -> storefront slug
}
type PlatformMapping struct {
    Slug               string
    InferredStorefront *string // fallback storefront when no copy supplies one
}

// Notes — verbatim Column plus optional advanced assembly.
type NotesConfig struct {
    Column   string        // SIMPLE: verbatim notes column
    Assembly *NoteAssembly // ADVANCED #1016 (Parse rejects)
}
type NoteAssembly struct {
    ReviewSubjectColumn string
    ReviewColumn        string
    CopyNoteColumn      string
}

// Grouping — simple toggle + optional advanced continuation.
type GroupingConfig struct {
    MergeByTitle bool             // false = one-row; true = merge rows sharing a title
    CopyRows     *CopyRowGrouping // ADVANCED #1016 (Parse rejects)
}
type CopyRowGrouping struct {
    ContinuationColumn string // blank here => row continues the previous game as a copy
}

type RatingConfig struct {
    Scale    int  // 5, 10, or 100
    Truncate bool // false = round to nearest whole star; true = truncate toward zero
}
type DurationConfig struct {
    Format string // "decimal" (SIMPLE) | "h:mm" (ADVANCED #1016, rejected)
}
```

### How each Darkadia input maps onto the frozen type (proof the type is complete)

| Darkadia column / behaviour | Config home |
|---|---|
| `Name` | `Columns.Title` + `CopyRowGrouping.ContinuationColumn` |
| `Added` | `Columns.CreatedAt` |
| `Loved` (`"1"`) | `Columns.Loved` + `TruthyValues` |
| `Rating` (0–5, truncate) | `Columns.Rating` + `RatingConfig{Scale:5, Truncate:true}` |
| `Tags` (comma) | `Columns.Tags` + `TagSeparator` |
| `Time played` (`H:MM`) | `Columns.HoursPlayed` + `DurationConfig{Format:"h:mm"}` |
| `Review` / `Review subject` | `NoteAssembly.ReviewColumn` / `ReviewSubjectColumn` |
| `Copy notes` | `NoteAssembly.CopyNoteColumn` |
| `Notes` (verbatim) | `NotesConfig.Column` |
| `Platforms` (aggregate) | `PlatformTables.AggregateColumn` |
| `Copy platform` | `PlatformTables.PlatformColumn` |
| `Copy source` / `Copy source other` (+`"Other"`) | `PlatformTables.SourceColumn` / `SourceOtherColumn` / `OtherSentinel` |
| `Copy media` (`"Physical"`) | `PlatformTables.MediaColumn` / `MediaPhysicalValue` |
| `Copy purchase date` | `PlatformTables.PurchaseDateColumn` |
| `platformTable` (string→slug, inferred storefront) | `PlatformTables.Platforms` |
| `storefrontTable` (source→slug) | `PlatformTables.Storefronts` |
| flag precedence (`resolvePlayStatus`) | `StatusFlags.Rules` (ordered) + `Default` |
| storefront-resolution precedence, longest-prefix, note assembly, dedup-earliest-date | engine *behaviour* in #1016 (no config fields) |

## The engine (`Parse`, simple subset)

`func Parse(raw []byte, cfg Config) ([]importmodel.Game, error)` — pure function; no DB, no I/O beyond the byte slice.

1. **Validate `cfg`** (see Validation) before reading data.
2. **Read CSV** via stdlib `encoding/csv`, `FieldsPerRecord = -1` (ragged-row tolerance, matching `darkadia`).
3. **Header index**: read the header row → `normalizedName → column index`. Normalization = `TrimSpace` + `ToLower`, applied to both the CSV headers and the names in `Config`. (Darkadia's headers match exactly; case-folding both sides is a safe superset that cannot break #1016.)
4. **Signature check** (`MatchesSignature`) — on a miss, return `fmt.Errorf("csv does not match the expected format: %w", importmodel.ErrInvalidSignature)`.
5. **Resolve column indices** for every configured column; an absent optional column → "not present" → empty value at extraction.
6. **Group rows** per `Grouping`: `one-row` (each data row → one game) or `merge-by-title` (rows sharing a normalized title collapse). `copy-rows` is rejected.
7. **Extract** each group into one `importmodel.Game`.

### Field extraction (simple subset)

| Field | Behaviour |
|---|---|
| Title | required; a row with an empty title is skipped defensively |
| Status | `StatusColumn`: cell looked up in `ValueMap` (case-insensitive); empty/unmapped → `Default` |
| Rating | `stars = value/Scale*5`, then round-half-up *or* truncate toward zero; clamp to 1–5; 0/empty/invalid → `nil` |
| Loved | cell ∈ `TruthyValues` (normalized) |
| Notes | verbatim `NotesConfig.Column`; empty → `nil` |
| CreatedAt | parsed via `DateLayout` (or passthrough `"2006-01-02"`), re-emitted as `"2006-01-02"` |
| HoursPlayed | `Duration.Format=="decimal"`: parse float; ≤0/empty/invalid → `nil` |
| Tags | split on `TagSeparator`, trim, drop empties, order-preserving dedupe |
| Platform | `PlatformSimple`: one `importmodel.Platform` from platform cell (+`PlatformMap` if set, else passthrough), storefront cell (+`StorefrontMap`), acquired-date cell; **empty platform cell → no entry** |

### `merge-by-title` merge rule

The **first** row for a title establishes all scalar fields (status, rating, notes, loved, created, hours); **every** row contributes its platform entry, **union-deduped on `(platform, storefront)`**. So "same game, two storefront rows" → one game with two platform entries; scalars come from the first occurrence. First-wins is order-stable and predictable (chosen over first-non-empty-per-field, which is fuzzier).

### Map-key normalization

`ValueMap` / `PlatformMap` / `StorefrontMap` keys and `TruthyValues` are matched case-insensitively: the engine normalizes the cell (`TrimSpace`+`ToLower`) before lookup, and `Config` map keys are expected lowercased (as `darkadia`/`vglist` tables already are).

## Validation (enforced by `Parse` in #1014)

- `Columns.Title` non-empty.
- **At most one** of `Status.Column` / `Status.Flags`; at most one of `Platform.Simple` / `Platform.Tables`. Both-nil is legal (no status column → every game gets the `not_started` default; no platform → empty `Platforms[]`), which the generic #1004 path needs.
- `Rating != nil` → `Scale ∈ {5,10,100}`.
- **Reject advanced slots** with a descriptive, non-`ErrInvalidSignature` error: `Status.Flags`, `Platform.Tables`, `Notes.Assembly`, `Grouping.CopyRows`, or `Duration.Format=="h:mm"`.

## Package layout

```
internal/services/csvmap/
  config.go      // the Config type and its sub-structs
  parse.go       // Parse, validation, MatchesSignature, extraction helpers
  parse_test.go  // unit tests over synthetic Configs
```

Split validation into `validate.go` only if `parse.go` exceeds ~300 lines. `csvmap` imports `importmodel` (for `Game`/`Platform`/`ErrInvalidSignature`) and stdlib only — it does **not** import `importsource`, `darkadia`, or `vglist`, so it stays independently testable.

## Testing

Pure-function unit tests over **synthetic** `Config`s (no real source, no DB, no fixtures), in `parse_test.go`:

- **Column mapping** — title + scalar fields land; absent optional column → zero value; header case/whitespace insensitivity.
- **Status** — mapped value; unmapped → `Default`; empty → `Default`.
- **Rating** — `Scale` 5/10/100; round vs. truncate at a half-boundary; 0/empty/invalid → `nil`; clamp >5 and <1.
- **Grouping** — same two rows: `one-row` → 2 games; `merge-by-title` → 1 game with 2 union-deduped platform entries, scalars from the first row.
- **Platform (simple)** — passthrough (no map); with `PlatformMap`/`StorefrontMap`; with storefront + acquired-date; empty platform cell → no entry; `(platform, storefront)` dedupe.
- **Tags** — separator split, trim, empty-drop, dedupe. **Loved** — truthy hit/miss. **Duration** — decimal parse; empty/≤0 → `nil`.
- **Signature** — `MatchesSignature` hit/miss; `Parse` on a missing signature column → `errors.Is(err, importmodel.ErrInvalidSignature)`.
- **Validation guardrails** — each advanced slot populated → a descriptive error that is **not** `ErrInvalidSignature`; missing `Columns.Title` → error; both `Status.Column` and `Status.Flags` set → error.

## Acceptance criteria

- [ ] `internal/services/csvmap` exists with the frozen `Config` type (full shape), `Parse` (simple subset), and `MatchesSignature`.
- [ ] `Parse` rejects every advanced slot with a clear, non-signature error.
- [ ] Unit tests over synthetic configs cover: column mapping, single-column status + default, rating scale/round/truncate, merge-by-title vs one-row grouping, tags split, loved, duration, platform-simple, signature, and the validation guardrails.
- [ ] No user-visible change; `darkadia` / `vglist` / registry / API / frontend / schema untouched and green.

## Out of scope

- Generic interactive flow, inspect endpoint, mapping dialog (#1004).
- Grouvee / Completionator `Config`s (#1002 / #1003).
- Auto-detect of known formats (#1015).
- Implementing the advanced engine behaviour and absorbing Darkadia (#1016) — only the advanced *fields* are declared here.
