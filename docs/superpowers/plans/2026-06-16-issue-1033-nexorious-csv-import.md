# Nexorious-CSV Import Format Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Author a `NexoriousCSV()` `csvmap.Config`, register it as a preset, and extend the engine with a delimited-platform-column capability so a Nexorious CSV export is a recognised, faithfully round-tripping import format.

**Architecture:** One generic engine extension (`PlatformSeparator` on `PlatformSimple`, split in `extractPlatforms`, guarded in `validate`), then the `NexoriousCSV()` Config + registry entry that reuses it, then a reference doc. No migration, no schema change, no new pipeline — the Config feeds the existing `enqueueImportJob` → `ImportMatch` → finalise flow, and the preset is surfaced automatically by the #1003 import wiring.

**Tech Stack:** Go, the `internal/services/csvmap` config-driven engine, stdlib `testing`. Run Go tests from the repo root.

**Spec:** `docs/superpowers/specs/2026-06-16-issue-1033-nexorious-csv-import-design.md`

---

## File Structure

- `internal/services/csvmap/config.go` — add `PlatformSeparator string` to `PlatformSimple` (modify).
- `internal/services/csvmap/parse.go` — `extractPlatforms` chooses split-vs-decodeKeys (modify).
- `internal/services/csvmap/validate.go` — reject `PlatformSeparator` + `json-keys` together (modify).
- `internal/services/csvmap/parse_test.go` — `extractPlatforms` split table test (modify/add).
- `internal/services/csvmap/validate_test.go` — `PlatformSeparator` validation tests (modify/add).
- `internal/services/csvmap/nexorious.go` — `func NexoriousCSV() Config` (create).
- `internal/services/csvmap/nexorious_test.go` — fixture round-trip + signature reject (create).
- `internal/services/csvmap/presets.go` — registry entry (modify).
- `internal/services/csvmap/presets_test.go` — registry assertion for `nexorious` (modify/add).
- `docs/nexorious-csv-import.md` — reference doc (create).

---

## Task 1: Engine extension — delimited platform column

**Files:**
- Modify: `internal/services/csvmap/config.go` (the `PlatformSimple` struct, ~line 84)
- Modify: `internal/services/csvmap/parse.go` (`extractPlatforms`, ~line 392)
- Modify: `internal/services/csvmap/validate.go` (the `cfg.Platform.Simple != nil` block, ~line 54)
- Test: `internal/services/csvmap/parse_test.go`, `internal/services/csvmap/validate_test.go`

- [ ] **Step 1: Write the failing split test**

Add to `internal/services/csvmap/parse_test.go`:

```go
func TestExtractPlatforms_SeparatorSplit(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "title"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn:    "platforms",
			PlatformSeparator: ";",
		}},
	}
	header := []string{"title", "platforms"}
	idx := buildIndex(header)

	tests := []struct {
		name string
		cell string
		want []string // expected platform slugs, in order
	}{
		{"two slugs", "pc-windows;playstation-5", []string{"pc-windows", "playstation-5"}},
		{"single slug", "pc-windows", []string{"pc-windows"}},
		{"empty", "", nil},
		{"trailing separator", "pc-windows;", []string{"pc-windows"}},
		{"empty middle piece", "pc-windows;;mac", []string{"pc-windows", "mac"}},
		{"duplicate deduped", "mac;mac", []string{"mac"}},
		{"whitespace trimmed", "pc-windows; mac ", []string{"pc-windows", "mac"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPlatforms([]string{"X", tc.cell}, idx, cfg)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d entries %+v, want %v", len(got), got, tc.want)
			}
			for i := range tc.want {
				if got[i].Platform != tc.want[i] {
					t.Errorf("entry %d = %q, want %q", i, got[i].Platform, tc.want[i])
				}
				if got[i].Storefront != nil {
					t.Errorf("entry %d storefront = %v, want nil", i, got[i].Storefront)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestExtractPlatforms_SeparatorSplit -v`
Expected: FAIL — `PlatformSeparator` is an unknown field of `PlatformSimple` (compile error).

- [ ] **Step 3: Add the `PlatformSeparator` field**

In `internal/services/csvmap/config.go`, modify the `PlatformSimple` struct to add the field after `PlatformFormat`:

```go
// PlatformSimple derives a single (platform, storefront, acquired-date) entry from columns.
type PlatformSimple struct {
	PlatformColumn     string
	PlatformFormat     ColumnFormat      // "" scalar (default, one entry) | "json-keys" (one entry per key)
	PlatformSeparator  string            // when non-empty, split PlatformColumn on this (one entry per piece); mutually exclusive with json-keys
	StorefrontColumn   string            // optional
	AcquiredDateColumn string            // optional; attaches to the platform entry
	PlatformMap        map[string]string // optional value (normalized) -> slug; nil/miss = passthrough as-is
	StorefrontMap      map[string]string // optional value (normalized) -> slug
}
```

- [ ] **Step 4: Implement the split in `extractPlatforms`**

In `internal/services/csvmap/parse.go`, replace the value-source line in `extractPlatforms`. The current code is:

```go
	ps := cfg.Platform.Simple
	if ps == nil {
		return nil
	}
	values := decodeKeys(cell(rec, idx, ps.PlatformColumn), ps.PlatformFormat)
	if len(values) == 0 {
		return nil
	}
```

Replace the `values := ...` line with a branch:

```go
	ps := cfg.Platform.Simple
	if ps == nil {
		return nil
	}
	var values []string
	if ps.PlatformSeparator != "" {
		values = splitTrim(cell(rec, idx, ps.PlatformColumn), ps.PlatformSeparator)
	} else {
		values = decodeKeys(cell(rec, idx, ps.PlatformColumn), ps.PlatformFormat)
	}
	if len(values) == 0 {
		return nil
	}
```

Then add a shared `splitTrim` helper near `extractTags` (so `extractTags` can reuse it too — see Step 5). Add it just above `extractTags` in `parse.go`:

```go
// splitTrim splits raw on sep, trims each piece, and drops empties. Order
// preserved; duplicates are NOT removed (callers dedupe as their semantics need).
func splitTrim(raw, sep string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, sep) {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
```

(Note: per-row slug dedupe already happens in `extractPlatforms` via its existing `seen` map, so the `"mac;mac"` case yields one entry without `splitTrim` deduping.)

- [ ] **Step 5: Refactor `extractTags` onto `splitTrim` (DRY, no behaviour change)**

In `internal/services/csvmap/parse.go`, the current `extractTags` body splits inline. Replace its split loop to call `splitTrim`, keeping the order-preserving dedupe:

```go
// extractTags splits, trims, drops empties, and order-preserving dedupes the tag cell.
func extractTags(rec []string, idx map[string]int, cfg Config) []string {
	sep := cfg.TagSeparator
	if sep == "" {
		sep = ","
	}
	var out []string
	seen := map[string]bool{}
	for _, tag := range splitTrim(cell(rec, idx, cfg.Columns.Tags), sep) {
		if seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}
```

- [ ] **Step 6: Run the split test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestExtractPlatforms_SeparatorSplit -v`
Expected: PASS (all 7 sub-cases).

- [ ] **Step 7: Write the failing validation tests**

Add to `internal/services/csvmap/validate_test.go`:

```go
func TestValidate_RejectsSeparatorWithJSONKeys(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "title"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn:    "platforms",
			PlatformFormat:    FormatJSONKeys,
			PlatformSeparator: ";",
		}},
	}
	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "PlatformSeparator") {
		t.Fatalf("want a PlatformSeparator error, got %v", err)
	}
	if errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Error("config error must not be ErrInvalidSignature")
	}
}

func TestValidate_AcceptsSeparatorWithScalar(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "title"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn:    "platforms",
			PlatformSeparator: ";",
		}},
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("scalar + separator should validate, got %v", err)
	}
}
```

Add the imports `"errors"` and `"github.com/drzero42/nexorious/internal/services/importmodel"` to `validate_test.go` if not already present (the file currently imports only `strings` and `testing`).

- [ ] **Step 8: Run the validation tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run TestValidate_.*Separator -v`
Expected: FAIL — `TestValidate_RejectsSeparatorWithJSONKeys` fails because no such guard exists yet (err is nil).

- [ ] **Step 9: Add the validation guard**

In `internal/services/csvmap/validate.go`, inside the existing `if cfg.Platform.Simple != nil {` block, after the `validateColumnFormat("Platform.Simple", ...)` check, add:

```go
	if cfg.Platform.Simple != nil {
		if err := validateColumnFormat("Platform.Simple", cfg.Platform.Simple.PlatformFormat); err != nil {
			return err
		}
		if cfg.Platform.Simple.PlatformSeparator != "" && cfg.Platform.Simple.PlatformFormat == FormatJSONKeys {
			return errors.New("csvmap: Platform.Simple PlatformSeparator and json-keys format are mutually exclusive")
		}
	}
```

(`errors` is already imported in `validate.go`.)

- [ ] **Step 10: Run the validation tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run TestValidate_.*Separator -v`
Expected: PASS (both).

- [ ] **Step 11: Run the whole csvmap package to confirm no regression**

Run: `go test ./internal/services/csvmap/`
Expected: PASS (the `extractTags` refactor must not have changed tag behaviour).

- [ ] **Step 12: Commit**

```bash
git add internal/services/csvmap/config.go internal/services/csvmap/parse.go internal/services/csvmap/validate.go internal/services/csvmap/parse_test.go internal/services/csvmap/validate_test.go
git commit -m "feat: csvmap delimited platform column (PlatformSeparator)"
```

---

## Task 2: The `NexoriousCSV()` Config + registry entry

**Files:**
- Create: `internal/services/csvmap/nexorious.go`
- Create: `internal/services/csvmap/nexorious_test.go`
- Modify: `internal/services/csvmap/presets.go` (the `presetList` var, ~line 13)
- Test: `internal/services/csvmap/presets_test.go`

- [ ] **Step 1: Write the failing fixture round-trip test**

Create `internal/services/csvmap/nexorious_test.go`. The fixture is a real Nexorious CSV export header (`title,igdb_id,play_status,personal_rating,is_loved,hours_played,personal_notes,platforms,tags,created_at,updated_at`) with three trimmed rows exercising: multi-platform split, a non-default `play_status`, an empty `play_status`, tags split on `;`, and RFC3339 `created_at`.

```go
package csvmap

import (
	"math"
	"testing"
)

// nexoriousFixture: a Nexorious CSV export header + three rows.
// Portal: two platforms, completed, loved, rated, two tags, hours.
// RDR2: single platform, shelved, not loved, no rating, one tag.
// Blank: empty play_status (-> not_started), no platforms, no tags, no hours.
const nexoriousFixture = `title,igdb_id,play_status,personal_rating,is_loved,hours_played,personal_notes,platforms,tags,created_at,updated_at
Portal,71,completed,5,true,10.5,Loved it,pc-windows;playstation-5,puzzle;favorite,2017-07-18T13:48:26Z,2020-02-02T00:00:00Z
Red Dead Redemption 2,25076,shelved,,false,,,playstation-4,western,2026-06-15T14:38:06Z,2026-06-15T14:38:06Z
Untitled Game,314246,,,false,,,,,2026-06-15T21:04:27Z,2026-06-15T21:04:27Z
`

func TestNexoriousCSV_MapsRealFixture(t *testing.T) {
	games, err := Parse([]byte(nexoriousFixture), NexoriousCSV())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("want 3 games, got %d", len(games))
	}

	// Portal: id match, completed, rating 5, loved, 10.5h, two platforms, two tags.
	p := games[0]
	if p.Title != "Portal" || p.IGDBID == nil || *p.IGDBID != 71 {
		t.Fatalf("portal title/igdb = %q/%v", p.Title, p.IGDBID)
	}
	if p.PlayStatus != "completed" {
		t.Errorf("portal status = %q, want completed", p.PlayStatus)
	}
	if p.PersonalRating == nil || *p.PersonalRating != 5 {
		t.Errorf("portal rating = %v, want 5", p.PersonalRating)
	}
	if !p.IsLoved {
		t.Errorf("portal should be loved")
	}
	if p.HoursPlayed == nil || math.Abs(*p.HoursPlayed-10.5) > 1e-9 {
		t.Errorf("portal hours = %v, want 10.5", p.HoursPlayed)
	}
	if len(p.Platforms) != 2 || p.Platforms[0].Platform != "pc-windows" || p.Platforms[1].Platform != "playstation-5" {
		t.Errorf("portal platforms = %+v, want [pc-windows playstation-5]", p.Platforms)
	}
	if len(p.Tags) != 2 || p.Tags[0] != "puzzle" || p.Tags[1] != "favorite" {
		t.Errorf("portal tags = %v, want [puzzle favorite]", p.Tags)
	}
	if p.PersonalNotes == nil || *p.PersonalNotes != "Loved it" {
		t.Errorf("portal notes = %v, want \"Loved it\"", p.PersonalNotes)
	}
	if p.CreatedAt != "2017-07-18" {
		t.Errorf("portal created = %q, want 2017-07-18", p.CreatedAt)
	}
	if p.IsWishlisted {
		t.Errorf("portal should not be wishlisted")
	}

	// RDR2: shelved (non-default canonical value round-trips), one platform, no rating/hours.
	r := games[1]
	if r.PlayStatus != "shelved" {
		t.Errorf("rdr2 status = %q, want shelved", r.PlayStatus)
	}
	if r.PersonalRating != nil {
		t.Errorf("rdr2 rating = %v, want nil", r.PersonalRating)
	}
	if r.HoursPlayed != nil {
		t.Errorf("rdr2 hours = %v, want nil", r.HoursPlayed)
	}
	if len(r.Platforms) != 1 || r.Platforms[0].Platform != "playstation-4" {
		t.Errorf("rdr2 platforms = %+v, want [playstation-4]", r.Platforms)
	}
	if r.IsLoved {
		t.Errorf("rdr2 should not be loved")
	}

	// Blank play_status -> not_started; no platforms/tags.
	b := games[2]
	if b.PlayStatus != "not_started" {
		t.Errorf("blank status = %q, want not_started", b.PlayStatus)
	}
	if len(b.Platforms) != 0 {
		t.Errorf("blank platforms = %+v, want none", b.Platforms)
	}
	if len(b.Tags) != 0 {
		t.Errorf("blank tags = %v, want none", b.Tags)
	}
}

func TestNexoriousCSV_SignatureRejectsUnrelated(t *testing.T) {
	_, err := Parse([]byte("name,foo,bar\nX,1,2\n"), NexoriousCSV())
	if err == nil {
		t.Fatal("want signature rejection for a non-Nexorious header")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestNexoriousCSV -v`
Expected: FAIL — `NexoriousCSV` is undefined (compile error).

- [ ] **Step 3: Implement `NexoriousCSV()`**

Create `internal/services/csvmap/nexorious.go`:

```go
package csvmap

import "time"

// NexoriousCSV returns the preset Config for Nexorious's own CSV export
// (internal/worker/tasks/export.go). Every row carries an igdb_id (the
// IGDB-keyed games.id), so matching is a direct id match (#1022) — no title
// matching, nothing in pending_review. play_status is exported verbatim as one
// of the eight canonical values, so the status ValueMap is an identity map over
// them. platforms is a semicolon-joined list of canonical platform slugs, read
// via the PlatformSeparator extension. The updated_at column has no model home
// and is dropped. See docs/nexorious-csv-import.md.
func NexoriousCSV() Config {
	return Config{
		Signature: []string{
			"play_status", "personal_rating", "is_loved", "hours_played", "personal_notes",
		},
		Columns: ColumnMap{
			Title:       "title",
			IGDBID:      "igdb_id",
			Rating:      "personal_rating",
			HoursPlayed: "hours_played",
			Tags:        "tags",
			Loved:       "is_loved",
			CreatedAt:   "created_at",
		},
		Status: StatusConfig{
			Column: &StatusColumn{
				Column: "play_status",
				ValueMap: map[string]string{
					"not_started": "not_started",
					"in_progress": "in_progress",
					"completed":   "completed",
					"mastered":    "mastered",
					"dominated":   "dominated",
					"shelved":     "shelved",
					"dropped":     "dropped",
					"replay":      "replay",
				},
				Default: "not_started",
			},
		},
		Platform: PlatformConfig{
			Simple: &PlatformSimple{
				PlatformColumn:    "platforms",
				PlatformSeparator: ";",
			},
		},
		Notes:      NotesConfig{Column: "personal_notes"},
		Rating:     &RatingConfig{Scale: 5, Truncate: false},
		Duration:   &DurationConfig{Format: "decimal"},
		TagSeparator: ";",
		DateLayout: time.RFC3339,
		Grouping:   GroupingConfig{MergeByTitle: false},
	}
}
```

- [ ] **Step 4: Run the fixture test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestNexoriousCSV -v`
Expected: PASS (both `_MapsRealFixture` and `_SignatureRejectsUnrelated`).

- [ ] **Step 5: Write the failing registry test**

Add to `internal/services/csvmap/presets_test.go`:

```go
func TestPresets_IncludesNexorious(t *testing.T) {
	cfg, ok := PresetBySlug("nexorious")
	if !ok {
		t.Fatal("expected a 'nexorious' preset in the registry")
	}
	if cfg.Columns.Title != "title" {
		t.Errorf("nexorious preset not wired to NexoriousCSV() (title col = %q)", cfg.Columns.Title)
	}
	var found bool
	for _, p := range Presets() {
		if p.Slug == "nexorious" && p.DisplayName == "Nexorious CSV" {
			found = true
		}
	}
	if !found {
		t.Error("Presets() must list nexorious with DisplayName Nexorious CSV")
	}
}
```

- [ ] **Step 6: Run the registry test to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestPresets_IncludesNexorious -v`
Expected: FAIL — `PresetBySlug("nexorious")` ok = false (not registered yet).

- [ ] **Step 7: Register the preset**

In `internal/services/csvmap/presets.go`, add the entry to `presetList`:

```go
var presetList = []Preset{
	{Slug: "completionator", DisplayName: "Completionator", Config: Completionator()},
	{Slug: "grouvee", DisplayName: "Grouvee", Config: Grouvee()},
	{Slug: "nexorious", DisplayName: "Nexorious CSV", Config: NexoriousCSV()},
}
```

- [ ] **Step 8: Run the registry test to verify it passes**

Run: `go test ./internal/services/csvmap/ -run TestPresets_IncludesNexorious -v`
Expected: PASS.

- [ ] **Step 9: Run the whole csvmap package**

Run: `go test ./internal/services/csvmap/`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/services/csvmap/nexorious.go internal/services/csvmap/nexorious_test.go internal/services/csvmap/presets.go internal/services/csvmap/presets_test.go
git commit -m "feat: Nexorious-CSV import preset (own-export round-trip)"
```

---

## Task 3: Reference documentation

**Files:**
- Create: `docs/nexorious-csv-import.md`

This mirrors `docs/grouvee-import.md` / `docs/completionator-import.md` (a reference doc; NOT embedded — per CLAUDE.md, only `user-guide.md`/`admin-guide.md` are served in-app).

- [ ] **Step 1: Write the reference doc**

Create `docs/nexorious-csv-import.md`:

```markdown
# Nexorious CSV Import

This document is the source of truth for how Nexorious re-imports a collection
from its **own CSV export** (`internal/worker/tasks/export.go`). It closes the
"export to CSV → re-import" loop that JSON export/import already has. The
importer is delivered as a `csvmap` preset `Config`
(`internal/services/csvmap/nexorious.go`), selected via the **Format** dropdown
in the CSV import dialog (or auto-detected by its header signature once #1015
lands).

## Identity and matching: IGDB id is exported

Every row carries the **`igdb_id`** column — Nexorious's IGDB-keyed `games.id`.
Re-import uses it for a **direct id match**: the game is hydrated from IGDB by
id, skipping title matching and the `pending_review` surface entirely (the #1022
path). As with every import, **IGDB must be configured** (hydration by id still
calls the IGDB API) or the import is refused up front.

## CSV format

Standard RFC-4180 CSV, UTF-8, one header row, 11 columns:

```
title, igdb_id, play_status, personal_rating, is_loved, hours_played, personal_notes, platforms, tags, created_at, updated_at
```

- **`title`** → game title (fallback only; matching is by id).
- **`igdb_id`** → direct IGDB-id match (#1022).
- **`play_status`** → one of the eight canonical values
  (`not_started`, `in_progress`, `completed`, `mastered`, `dominated`,
  `shelved`, `dropped`, `replay`), round-tripped verbatim via an identity value
  map. An empty or unrecognized value imports as `not_started`.
- **`personal_rating`** → whole `1`–`5` (scale 5).
- **`is_loved`** → `true`/`false`.
- **`hours_played`** → decimal; **the export-summed total across all platforms**.
- **`personal_notes`** → verbatim.
- **`platforms`** → **semicolon-joined platform slugs** (e.g.
  `pc-windows;playstation-5`); split into one ownership entry per slug. Slugs are
  already canonical, so there is no name→slug mapping.
- **`tags`** → semicolon-joined tag names.
- **`created_at`** → RFC3339; stored to date granularity.

## Round-trip caveats

The re-import is faithful for curated scalar fields but not byte-lossless:

- **`updated_at` is dropped** — it has no import-side model home.
- **`hours_played` is the summed total** across platforms; on re-import it
  attaches as the game-level hours (the existing generic behaviour) — the
  per-platform split is not recovered.
- **Platforms come back without storefront, acquired-date, or per-platform
  hours** — the export column carries slugs only, so re-imported ownership
  entries have those fields empty.
- **`created_at`** round-trips to date granularity (the engine normalizes dates
  to `YYYY-MM-DD`); the time component is dropped.
```

- [ ] **Step 2: Commit**

```bash
git add docs/nexorious-csv-import.md
git commit -m "docs: Nexorious CSV import reference"
```

---

## Task 4: Full-suite verification

**Files:** none (verification only).

- [ ] **Step 1: Build the backend**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 2: Lint the changed package**

Run: `golangci-lint run ./internal/services/csvmap/...`
Expected: no findings. (Watch for `gofmt` alignment on the new struct field and the `NexoriousCSV()` literal — the post-edit hook normally fixes this; re-run if it reports a diff.)

- [ ] **Step 3: Run the full csvmap test suite verbose**

Run: `go test ./internal/services/csvmap/ -v`
Expected: PASS — all existing tests plus `TestExtractPlatforms_SeparatorSplit`, `TestValidate_RejectsSeparatorWithJSONKeys`, `TestValidate_AcceptsSeparatorWithScalar`, `TestNexoriousCSV_MapsRealFixture`, `TestNexoriousCSV_SignatureRejectsUnrelated`, `TestPresets_IncludesNexorious`.

- [ ] **Step 4: Run the import API package (preset wiring is exercised there)**

Run: `go test ./internal/api/ -run TestImport -v`
Expected: PASS — confirms the new preset doesn't break the inspect/import handlers (which enumerate `csvmap.Presets()`).

---

## Acceptance criteria mapping (from the issue)

- [ ] *A Nexorious CSV export is a registered, signature-matched import format* — Task 2 (preset + signature + registry test).
- [ ] *Re-importing matches every row by `igdb_id` — no title matching, nothing in `pending_review`* — `ColumnMap.IGDBID` (Task 2); asserted by the fixture test's id checks.
- [ ] *Curated scalar fields round-trip (`play_status`, `personal_rating`, `is_loved`, `hours_played`, `personal_notes`, `tags`, `created_at`)* — Task 2 Config + fixture test.
- [ ] *The `platforms` decision (a) is implemented and documented* — Task 1 (engine) + Task 2 (Config uses it) + Task 3 (doc).
- [ ] *Tests cover the Config over a real exported fixture and the signature* — Task 2 (`nexorious_test.go`).
