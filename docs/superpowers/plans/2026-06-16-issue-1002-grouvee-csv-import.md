# Grouvee CSV Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class Grouvee CSV import as a `csvmap` preset `Config`, extending the engine with the JSON-handling the real export requires.

**Architecture:** Grouvee's export embeds JSON in three columns (`shelves`/`platforms` objects, `dates` play-log array) and carries an IGDB id per row. We add four generic engine capabilities (JSON-object-keys decode, shelf→status+wishlist, two-column note assembly, a play-log block), one shared-pipeline field (`IsWishlisted`), then express Grouvee entirely as a `Config` and register it. No migration, no new pipeline.

**Tech Stack:** Go 1.26, Bun ORM, River, `encoding/json`, stdlib `testing` + testcontainers. Frontend untouched (the #1003 Format dropdown is registry-driven).

**Spec:** `docs/superpowers/specs/2026-06-16-issue-1002-grouvee-csv-import-design.md`

**Branch:** `feat/1002-grouvee-csv-import` (already created; the spec is committed on it).

---

## File Structure

- `internal/services/importmodel/model.go` — **modify**: add `IsWishlisted bool` to `Game`.
- `internal/services/csvmap/config.go` — **modify**: `ColumnFormat` type + consts, `WishlistStatus` const, `StatusColumn.Format`/`Precedence`, `PlatformSimple.PlatformFormat`, `NotesConfig.TitleColumn`, `PlayLogConfig` + `Config.PlayLog`.
- `internal/services/csvmap/parse.go` — **modify**: `decodeKeys`, `assembleNote`, `statusRank`, rewrite `extractStatus`/`extractPlatforms`, add `extractPlayLog`, rewire `extractGame`.
- `internal/services/csvmap/validate.go` — **modify**: validate the new format fields + `PlayLog` + `Duration`/`PlayLog` exclusivity.
- `internal/services/csvmap/grouvee.go` — **create**: `func Grouvee() Config`.
- `internal/services/csvmap/presets.go` — **modify**: register the `grouvee` preset.
- `internal/worker/tasks/import_pipeline.go` — **modify**: set `IsWishlisted` on the new-`user_game` insert.
- `docs/grouvee-import.md` — **create**: the format spec.
- Tests: `parse_test.go` (engine primitives), `validate_test.go` (new), `grouvee_test.go` (new, fixture), `presets_test.go`, `import_pipeline_test.go`.

All `go test` commands run from the repo root. Single-package runs: `go test ./internal/services/csvmap/... -run <Name> -v`.

---

### Task 1: Add `IsWishlisted` to the canonical import model

**Files:**
- Modify: `internal/services/importmodel/model.go`
- Test: `internal/services/importmodel/model_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/services/importmodel/model_test.go`:

```go
package importmodel

import (
	"encoding/json"
	"strings"
	"testing"
)

// A Game with IsWishlisted=false must serialize WITHOUT the is_wishlisted key,
// so every existing mapper's source_metadata stays byte-identical (omitempty).
func TestGame_IsWishlistedOmittedWhenFalse(t *testing.T) {
	b, err := json.Marshal(Game{Title: "X", PlayStatus: "not_started"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "is_wishlisted") {
		t.Errorf("is_wishlisted must be omitted when false, got %s", b)
	}
}

func TestGame_IsWishlistedPresentWhenTrue(t *testing.T) {
	b, err := json.Marshal(Game{Title: "X", PlayStatus: "not_started", IsWishlisted: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"is_wishlisted":true`) {
		t.Errorf("is_wishlisted:true must serialize, got %s", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/importmodel/... -run TestGame_IsWishlisted -v`
Expected: FAIL — `Game` has no field `IsWishlisted` (compile error).

- [ ] **Step 3: Add the field**

In `internal/services/importmodel/model.go`, add the field to `Game` (after `IsLoved`):

```go
	IsLoved        bool       `json:"is_loved"`
	IsWishlisted   bool       `json:"is_wishlisted,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/services/importmodel/... -run TestGame_IsWishlisted -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/services/importmodel/model.go internal/services/importmodel/model_test.go
git commit -m "feat: add IsWishlisted to canonical import model"
```

---

### Task 2: `ColumnFormat` + `decodeKeys` JSON-object-keys primitive

**Files:**
- Modify: `internal/services/csvmap/config.go`
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/csvmap/parse_test.go`:

```go
func TestDecodeKeys(t *testing.T) {
	tests := []struct {
		name string
		cell string
		f    ColumnFormat
		want []string
	}{
		{"scalar value", "PC", FormatScalar, []string{"PC"}},
		{"scalar blank", "", FormatScalar, nil},
		{"json object keys", `{"Played": {"x": 1}, "Backlog": {}}`, FormatJSONKeys, []string{"Backlog", "Played"}},
		{"json empty object", "{}", FormatJSONKeys, nil},
		{"json blank", "", FormatJSONKeys, nil},
		{"json malformed", `{"Played": `, FormatJSONKeys, nil},
		{"json not-an-object", `["a","b"]`, FormatJSONKeys, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeKeys(tt.cell, tt.f)
			sort.Strings(got)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("decodeKeys(%q,%q) = %v, want %v", tt.cell, tt.f, got, want)
			}
		})
	}
}
```

Ensure the test file imports `reflect` and `sort` (add to its import block if missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run TestDecodeKeys -v`
Expected: FAIL — `FormatScalar`/`decodeKeys` undefined (compile error).

- [ ] **Step 3: Add the type/consts and helper**

In `internal/services/csvmap/config.go`, add near the top (after the package doc / before `Config`):

```go
// ColumnFormat selects how a configured column's cell is read.
type ColumnFormat string

const (
	FormatScalar   ColumnFormat = ""          // default: the trimmed cell is the single value
	FormatJSONKeys ColumnFormat = "json-keys" // cell is a JSON object; its keys are the values
)
```

In `internal/services/csvmap/parse.go`, add `"encoding/json"` to the import block and add the helper:

```go
// decodeKeys returns the values a column yields under format f. Scalar: the cell
// as a single value (nil if blank). JSON-keys: the keys of a JSON object (nil for
// "", "{}", a non-object, or unparseable JSON). Object key order is undefined;
// callers that must pick one value apply an explicit precedence.
func decodeKeys(cell string, f ColumnFormat) []string {
	if cell == "" {
		return nil
	}
	if f != FormatJSONKeys {
		return []string{cell}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cell), &obj); err != nil {
		return nil
	}
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/services/csvmap/... -run TestDecodeKeys -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/config.go internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: add JSON-object-keys column decode to csvmap engine"
```

---

### Task 3: shelf → status + wishlist (`extractStatus` rewrite)

**Files:**
- Modify: `internal/services/csvmap/config.go`
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/csvmap/parse_test.go`:

```go
func TestExtractStatus_JSONShelvesAndWishlist(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		Status: StatusConfig{Column: &StatusColumn{
			Column: "shelves", Format: FormatJSONKeys,
			ValueMap: map[string]string{
				"playing": "in_progress", "played": "completed",
				"backlog": "not_started", "wish list": WishlistStatus,
			},
			Precedence: []string{"playing", "played", "backlog"},
			Default:    "not_started",
		}},
	}
	idx := map[string]int{"shelves": 0}
	cases := []struct {
		shelves   string
		wantStat  string
		wantWish  bool
	}{
		{`{"Playing": {}}`, "in_progress", false},
		{`{"Played": {}}`, "completed", false},
		{`{"Backlog": {}}`, "not_started", false},
		{`{"Played": {}, "Backlog": {}}`, "completed", false}, // precedence: played > backlog
		{`{"Wish List": {}}`, "not_started", true},            // wishlist flag, status defaults
		{`{"Playing": {}, "Wish List": {}}`, "in_progress", true},
		{`{"Some Custom Shelf": {}}`, "not_started", false}, // unrecognized -> default
		{`{}`, "not_started", false},
	}
	for _, c := range cases {
		st, wish := extractStatus([]string{c.shelves}, idx, cfg)
		if st != c.wantStat || wish != c.wantWish {
			t.Errorf("shelves %s -> (%q,%v), want (%q,%v)", c.shelves, st, wish, c.wantStat, c.wantWish)
		}
	}
}

// The scalar status path (Completionator) is unchanged: one value, no precedence.
func TestExtractStatus_ScalarUnchanged(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		Status: StatusConfig{Column: &StatusColumn{
			Column:   "Progress Status",
			ValueMap: map[string]string{"finished": "completed", "incomplete": "not_started"},
			Default:  "not_started",
		}},
	}
	idx := map[string]int{"progress status": 0}
	if st, wish := extractStatus([]string{"Finished"}, idx, cfg); st != "completed" || wish {
		t.Errorf("Finished -> (%q,%v), want (completed,false)", st, wish)
	}
	if st, _ := extractStatus([]string{""}, idx, cfg); st != "not_started" {
		t.Errorf("empty -> %q, want not_started", st)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run TestExtractStatus -v`
Expected: FAIL — `StatusColumn.Format`/`Precedence`, `WishlistStatus` undefined, and `extractStatus` returns one value (compile error).

- [ ] **Step 3: Extend the config and rewrite `extractStatus`**

In `internal/services/csvmap/config.go`, replace the `StatusColumn` struct and add the const:

```go
// StatusColumn derives play_status from a single column (scalar or JSON-keys).
type StatusColumn struct {
	Column     string
	Format     ColumnFormat      // "" scalar (default) | "json-keys"
	ValueMap   map[string]string // normalized source value -> play_status, or WishlistStatus
	Precedence []string          // normalized source values, highest priority first
	Default    string            // empty/unmapped -> this; "" falls back to "not_started"
}

// WishlistStatus is the reserved ValueMap target that flags is_wishlisted rather
// than setting play_status. play_status is then derived from the remaining values.
const WishlistStatus = "wishlisted"
```

In `internal/services/csvmap/parse.go`, replace the whole `extractStatus` function:

```go
// extractStatus resolves the shelf-derived play_status and the wishlist flag from
// the status column (scalar or JSON-keys). A value mapped to WishlistStatus sets
// wishlisted; the remaining mapped values are play_status candidates, resolved by
// Precedence (first listed present wins), else the single candidate (scalar
// case), else Default.
func extractStatus(rec []string, idx map[string]int, cfg Config) (status string, wishlisted bool) {
	if cfg.Status.Column == nil {
		return "not_started", false
	}
	sc := cfg.Status.Column
	def := sc.Default
	if def == "" {
		def = "not_started"
	}
	present := map[string]string{} // normalized source value -> mapped play_status
	for _, v := range decodeKeys(cell(rec, idx, sc.Column), sc.Format) {
		nv := normKey(v)
		mapped, ok := sc.ValueMap[nv]
		if !ok {
			continue
		}
		if mapped == WishlistStatus {
			wishlisted = true
			continue
		}
		present[nv] = mapped
	}
	for _, p := range sc.Precedence {
		if s, ok := present[normKey(p)]; ok {
			return s, wishlisted
		}
	}
	if len(sc.Precedence) == 0 {
		for _, s := range present { // scalar case: at most one entry
			return s, wishlisted
		}
	}
	return def, wishlisted
}
```

Then update the single call site in `extractGame` (it currently does `PlayStatus: extractStatus(rec, idx, cfg)`). For now make it compile by consuming both return values — the full rewire happens in Task 6, but the build must stay green:

```go
	status, _ := extractStatus(rec, idx, cfg)
	g := importmodel.Game{
		Title:      title,
		PlayStatus: status,
	}
```

(Replace the existing `g := importmodel.Game{Title: title, PlayStatus: extractStatus(rec, idx, cfg)}` literal accordingly.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/... -run TestExtractStatus -v`
Expected: PASS (both). Also run the existing suite to confirm Completionator is unaffected: `go test ./internal/services/csvmap/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/config.go internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: derive play_status and wishlist from shelf column in csvmap"
```

---

### Task 4: multi-platform from a JSON-keys platform column

**Files:**
- Modify: `internal/services/csvmap/config.go`
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/csvmap/parse_test.go`:

```go
func TestExtractPlatforms_JSONKeys(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn: "platforms", PlatformFormat: FormatJSONKeys,
			PlatformMap: map[string]string{
				"pc (microsoft windows)": "pc-windows", "playstation 4": "playstation-4",
			},
		}},
	}
	idx := map[string]int{"platforms": 0}

	one := extractPlatforms([]string{`{"PC (Microsoft Windows)": {"url": "x"}}`}, idx, cfg)
	if len(one) != 1 || one[0].Platform != "pc-windows" || one[0].Storefront != nil {
		t.Fatalf("single platform = %+v", one)
	}
	empty := extractPlatforms([]string{`{}`}, idx, cfg)
	if empty != nil {
		t.Errorf("empty platforms = %+v, want nil", empty)
	}
	two := extractPlatforms([]string{`{"PC (Microsoft Windows)": {}, "PlayStation 4": {}}`}, idx, cfg)
	got := map[string]bool{}
	for _, p := range two {
		got[p.Platform] = true
	}
	if len(two) != 2 || !got["pc-windows"] || !got["playstation-4"] {
		t.Errorf("two platforms = %+v", two)
	}
	miss := extractPlatforms([]string{`{"Sega Saturn": {}}`}, idx, cfg)
	if len(miss) != 1 || miss[0].Platform != "Sega Saturn" {
		t.Errorf("unmapped platform should passthrough, got %+v", miss)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run TestExtractPlatforms_JSONKeys -v`
Expected: FAIL — `PlatformSimple.PlatformFormat` undefined.

- [ ] **Step 3: Add the field and rewrite `extractPlatforms`**

In `internal/services/csvmap/config.go`, add the field to `PlatformSimple` (after `PlatformColumn`):

```go
	PlatformColumn     string
	PlatformFormat     ColumnFormat      // "" scalar (default, one entry) | "json-keys" (one entry per key)
	StorefrontColumn   string            // optional
```

In `internal/services/csvmap/parse.go`, replace `extractPlatforms`:

```go
// extractPlatforms builds ownership entries from the platform column. Scalar
// yields at most one; json-keys yields one per key (deduped by slug within the
// row). The optional storefront/acquired-date scalar columns apply to every
// entry. Map miss = passthrough. No PlatformSimple config / empty column = none.
func extractPlatforms(rec []string, idx map[string]int, cfg Config) []importmodel.Platform {
	ps := cfg.Platform.Simple
	if ps == nil {
		return nil
	}
	values := decodeKeys(cell(rec, idx, ps.PlatformColumn), ps.PlatformFormat)
	if len(values) == 0 {
		return nil
	}
	var sf *string
	if sv := cell(rec, idx, ps.StorefrontColumn); sv != "" {
		s := sv
		if ps.StorefrontMap != nil {
			if mapped, ok := ps.StorefrontMap[normKey(sv)]; ok {
				s = mapped
			}
		}
		sf = &s
	}
	date := extractDate(cell(rec, idx, ps.AcquiredDateColumn), cfg)
	var out []importmodel.Platform
	seen := map[string]bool{}
	for _, pv := range values {
		slug := pv
		if ps.PlatformMap != nil {
			if mapped, ok := ps.PlatformMap[normKey(pv)]; ok {
				slug = mapped
			}
		}
		if seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, importmodel.Platform{Platform: slug, Storefront: sf, AcquiredDate: date})
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/... -run TestExtractPlatforms_JSONKeys -v` → PASS
Run: `go test ./internal/services/csvmap/...` → PASS (Completionator's scalar platform path is the one-value case, unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/config.go internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: support multi-platform from JSON-keys column in csvmap"
```

---

### Task 5: two-column note assembly

**Files:**
- Modify: `internal/services/csvmap/config.go`
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/csvmap/parse_test.go`:

```go
func TestAssembleNote(t *testing.T) {
	cases := []struct{ title, body, want string }{
		{"", "", ""},
		{"", "just body", "just body"},
		{"Heading", "", "**Heading**"},
		{"Heading", "body text", "**Heading**\n\nbody text"},
	}
	for _, c := range cases {
		if got := assembleNote(c.title, c.body); got != c.want {
			t.Errorf("assembleNote(%q,%q) = %q, want %q", c.title, c.body, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run TestAssembleNote -v`
Expected: FAIL — `assembleNote` undefined.

- [ ] **Step 3: Add the field and helper, and rewire the notes block**

In `internal/services/csvmap/config.go`, add the field to `NotesConfig` (before `Assembly`):

```go
type NotesConfig struct {
	Column      string        // SIMPLE: verbatim notes / body column
	TitleColumn string        // optional heading; non-empty -> prepended as "**title**\n\n"
	Assembly    *NoteAssembly // ADVANCED #1016 (Parse rejects)
}
```

In `internal/services/csvmap/parse.go`, add the helper:

```go
// assembleNote combines an optional bold heading with a body. Either may be empty.
func assembleNote(title, body string) string {
	switch {
	case title == "" && body == "":
		return ""
	case title == "":
		return body
	case body == "":
		return "**" + title + "**"
	default:
		return "**" + title + "**\n\n" + body
	}
}
```

In `extractGame`, replace the existing notes block:

```go
	if n := cell(rec, idx, cfg.Notes.Column); n != "" {
		g.PersonalNotes = &n
	}
```

with:

```go
	if note := assembleNote(cell(rec, idx, cfg.Notes.TitleColumn), cell(rec, idx, cfg.Notes.Column)); note != "" {
		g.PersonalNotes = &note
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/... -run TestAssembleNote -v` → PASS
Run: `go test ./internal/services/csvmap/...` → PASS (Completionator has no notes columns, so `assembleNote("","")` → "" → no note, unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/config.go internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: add two-column note assembly to csvmap engine"
```

---

### Task 6: `dates` play-log → playtime + completion-tier override

**Files:**
- Modify: `internal/services/csvmap/config.go`
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/csvmap/parse_test.go`:

```go
func TestExtractPlayLog(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		PlayLog: &PlayLogConfig{
			Column: "dates", SecondsField: "seconds_played", CompletionField: "level_of_completion",
			CompletionMap: map[string]string{
				"main story": "completed", "main story + extras": "mastered", "100% completion": "dominated",
			},
		},
	}
	idx := map[string]int{"dates": 0}

	// Real finish: seconds -> hours, tier -> dominated.
	hrs, tier := extractPlayLog([]string{`[{"seconds_played": 36300, "level_of_completion": "100% Completion"}]`}, idx, cfg)
	if hrs == nil || math.Abs(*hrs-36300.0/3600.0) > 1e-9 {
		t.Errorf("hours = %v, want %v", hrs, 36300.0/3600.0)
	}
	if tier != "dominated" {
		t.Errorf("tier = %q, want dominated", tier)
	}

	// Zero seconds -> no hours; tier still recognized.
	hrs, tier = extractPlayLog([]string{`[{"seconds_played": 0, "level_of_completion": "Main Story"}]`}, idx, cfg)
	if hrs != nil {
		t.Errorf("hours = %v, want nil", hrs)
	}
	if tier != "completed" {
		t.Errorf("tier = %q, want completed", tier)
	}

	// Multiple entries: seconds summed, highest tier wins.
	hrs, tier = extractPlayLog([]string{`[{"seconds_played": 1800, "level_of_completion": "Main Story"},{"seconds_played": 1800, "level_of_completion": "Main Story + Extras"}]`}, idx, cfg)
	if hrs == nil || math.Abs(*hrs-1.0) > 1e-9 {
		t.Errorf("hours = %v, want 1.0", hrs)
	}
	if tier != "mastered" {
		t.Errorf("tier = %q, want mastered (highest of the two)", tier)
	}

	// Empty / unrecognized / no config.
	if h, ts := extractPlayLog([]string{`[]`}, idx, cfg); h != nil || ts != "" {
		t.Errorf("empty array = (%v,%q), want (nil,\"\")", h, ts)
	}
	if h, ts := extractPlayLog([]string{`[{"seconds_played": 60, "level_of_completion": "Unknown"}]`}, idx, cfg); ts != "" {
		t.Errorf("unrecognized tier ts = %q, want \"\" (h=%v)", ts, h)
	}
	if h, ts := extractPlayLog([]string{`garbage`}, idx, cfg); h != nil || ts != "" {
		t.Errorf("malformed = (%v,%q), want (nil,\"\")", h, ts)
	}
}
```

Ensure the test file imports `math`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run TestExtractPlayLog -v`
Expected: FAIL — `PlayLogConfig`/`Config.PlayLog`/`extractPlayLog` undefined.

- [ ] **Step 3: Add config, helper, and rewire `extractGame`**

In `internal/services/csvmap/config.go`, add the field to `Config` (after `Duration`):

```go
	Duration     *DurationConfig // nil = ignore hours_played
	PlayLog      *PlayLogConfig  // nil = ignore; JSON-array play-log (seconds + completion tier)
```

and add the type (near `DurationConfig`):

```go
// PlayLogConfig reads playtime and a completion tier from a JSON-array column of
// play-session objects (Grouvee's "dates"). A recognized CompletionField value
// overrides the shelf-derived play_status. Mutually exclusive with Duration.
type PlayLogConfig struct {
	Column          string            // JSON-array column of session objects
	SecondsField    string            // numeric field; summed across entries, ÷3600 -> hours_played
	CompletionField string            // completion-tier field
	CompletionMap   map[string]string // normalized tier -> play_status; unrecognized ignored
}
```

In `internal/services/csvmap/parse.go`, add the rank table and helper:

```go
// statusRank orders completion tiers so the highest across play-log entries wins.
var statusRank = map[string]int{"completed": 1, "mastered": 2, "dominated": 3}

// extractPlayLog sums the seconds field into hours_played and maps the highest
// recognized completion tier to a play_status. Non-array/blank/malformed input,
// zero seconds, and unrecognized tiers yield nil hours / "" tier respectively.
func extractPlayLog(rec []string, idx map[string]int, cfg Config) (hours *float64, tierStatus string) {
	pl := cfg.PlayLog
	if pl == nil {
		return nil, ""
	}
	raw := cell(rec, idx, pl.Column)
	if raw == "" {
		return nil, ""
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, ""
	}
	var totalSeconds float64
	bestRank := 0
	for _, e := range entries {
		if rs, ok := e[pl.SecondsField]; ok {
			var sec float64
			if json.Unmarshal(rs, &sec) == nil && sec > 0 {
				totalSeconds += sec
			}
		}
		if rt, ok := e[pl.CompletionField]; ok {
			var tier string
			if json.Unmarshal(rt, &tier) == nil {
				if st, ok := pl.CompletionMap[normKey(tier)]; ok && statusRank[st] > bestRank {
					bestRank = statusRank[st]
					tierStatus = st
				}
			}
		}
	}
	if totalSeconds > 0 {
		h := totalSeconds / 3600.0
		hours = &h
	}
	return hours, tierStatus
}
```

Now rewire `extractGame` to consume the play-log: a populated tier overrides the shelf status, and play-log hours set `HoursPlayed`. Replace the top of `extractGame` (the title/status/`g :=` block from Task 3) with:

```go
func extractGame(rec []string, idx map[string]int, cfg Config) (importmodel.Game, bool) {
	title := cell(rec, idx, cfg.Columns.Title)
	if title == "" {
		return importmodel.Game{}, false
	}
	status, wishlisted := extractStatus(rec, idx, cfg)
	hours, tierStatus := extractPlayLog(rec, idx, cfg)
	if tierStatus != "" {
		status = tierStatus
	}
	g := importmodel.Game{
		Title:        title,
		PlayStatus:   status,
		IsWishlisted: wishlisted,
	}
```

and, lower in `extractGame`, after the existing scalar `extractHours` block, apply the play-log hours (play-log wins; the two sources are mutually exclusive by validation):

```go
	if h := extractHours(rec, idx, cfg); h != nil {
		g.HoursPlayed = h
	}
	if hours != nil {
		g.HoursPlayed = hours
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/... -run TestExtractPlayLog -v` → PASS
Run: `go test ./internal/services/csvmap/...` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/config.go internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: map play-log to hours and completion-tier status override in csvmap"
```

---

### Task 7: validation for the new config slots

**Files:**
- Modify: `internal/services/csvmap/validate.go`
- Test: `internal/services/csvmap/validate_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/services/csvmap/validate_test.go`:

```go
package csvmap

import (
	"strings"
	"testing"
)

func TestValidate_RejectsBadColumnFormat(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		Status:  StatusConfig{Column: &StatusColumn{Column: "shelves", Format: "bogus"}},
	}
	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("want a format error, got %v", err)
	}
}

func TestValidate_PlayLogRequiresFields(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "name"},
		PlayLog: &PlayLogConfig{Column: "dates"}, // missing SecondsField/CompletionField
	}
	if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "PlayLog") {
		t.Fatalf("want a PlayLog error, got %v", err)
	}
}

func TestValidate_PlayLogAndDurationExclusive(t *testing.T) {
	cfg := Config{
		Columns:  ColumnMap{Title: "name"},
		Duration: &DurationConfig{Format: "decimal"},
		PlayLog:  &PlayLogConfig{Column: "dates", SecondsField: "s", CompletionField: "c"},
	}
	if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("want a mutual-exclusion error, got %v", err)
	}
}

func TestValidate_AcceptsGrouvee(t *testing.T) {
	if err := validate(Grouvee()); err != nil {
		t.Fatalf("Grouvee() must validate, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run TestValidate -v`
Expected: FAIL — `TestValidate_AcceptsGrouvee` won't compile yet (`Grouvee` undefined) AND the format/PlayLog checks aren't enforced. (Grouvee lands in Task 8; this test is written now and goes green after Task 8. To keep Task 7 self-contained and green, temporarily omit `TestValidate_AcceptsGrouvee` from this file and add it in Task 8 — see Task 8 Step 1.)

> **Note:** Write only the first three test functions in this task. Add `TestValidate_AcceptsGrouvee` in Task 8 once `Grouvee()` exists.

- [ ] **Step 3: Implement the validation**

In `internal/services/csvmap/validate.go`, add a helper and wire the checks into `validate` (before the final `return nil`):

```go
func validateColumnFormat(name string, f ColumnFormat) error {
	switch f {
	case FormatScalar, FormatJSONKeys:
		return nil
	}
	return fmt.Errorf("csvmap: %s format %q must be %q or %q", name, f, FormatScalar, FormatJSONKeys)
}
```

```go
	if cfg.Status.Column != nil {
		if err := validateColumnFormat("Status.Column", cfg.Status.Column.Format); err != nil {
			return err
		}
	}
	if cfg.Platform.Simple != nil {
		if err := validateColumnFormat("Platform.Simple", cfg.Platform.Simple.PlatformFormat); err != nil {
			return err
		}
	}
	if cfg.PlayLog != nil {
		if cfg.Duration != nil {
			return errors.New("csvmap: Duration and PlayLog are mutually exclusive")
		}
		if strings.TrimSpace(cfg.PlayLog.Column) == "" ||
			strings.TrimSpace(cfg.PlayLog.SecondsField) == "" ||
			strings.TrimSpace(cfg.PlayLog.CompletionField) == "" {
			return errors.New("csvmap: PlayLog requires Column, SecondsField, and CompletionField")
		}
	}
```

(`fmt`, `errors`, and `strings` are already imported in `validate.go`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/... -run TestValidate -v`
Expected: PASS (the three written tests).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/validate.go internal/services/csvmap/validate_test.go
git commit -m "feat: validate JSON-keys formats and PlayLog config in csvmap"
```

---

### Task 8: the Grouvee `Config` + fixture test

**Files:**
- Create: `internal/services/csvmap/grouvee.go`
- Create: `internal/services/csvmap/grouvee_test.go`
- Modify: `internal/services/csvmap/validate_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/services/csvmap/grouvee_test.go`. The fixture is a trimmed real export — the full 20-column header plus three rows (Portal, RDR2, Borderlands 4):

```go
package csvmap

import (
	"math"
	"testing"
)

// grouveeFixture: real 20-column Grouvee header + three trimmed rows. Portal is a
// finished 100%-completion row (tier overrides Played -> dominated, with hours
// and a PC platform); RDR2 is a Backlog row whose Main Story tier overrides to
// completed; Borderlands 4 is a Wish List row (wishlisted, no platforms).
const grouveeFixture = `id,name,shelves,platforms,rating,review_title,review,review_platform,dates,statuses,genres,franchises,series,developers,publishers,release_date,date_added_to_collection,url,giantbomb_id,igdb_id
107168,Portal,"{""Played"": {""date_added"": ""2017-07-18T13:48:26Z""}}","{""PC (Microsoft Windows)"": {""url"": ""x""}}",5,Great,Loved it,,"[{""date_started"": ""2010-01-01"", ""date_finished"": ""2020-02-02"", ""seconds_played"": 36300, ""level_of_completion"": ""100% Completion""}]",[],{},{},{},{},{},2007-10-10,2017-07-18,x,,71
117835,Red Dead Redemption 2,"{""Backlog"": {""date_added"": ""2026-06-15T14:38:06Z"", ""order"": 1}}","{""PlayStation 4"": {""url"": ""x""}}",4,,,,"[{""date_started"": ""None"", ""date_finished"": ""None"", ""seconds_played"": 0, ""level_of_completion"": ""Main Story""}]",[],{},{},{},{},{},2018-10-26,2026-06-15,x,,25076
196974,Borderlands 4,"{""Wish List"": {""date_added"": ""2026-06-15T21:04:27Z"", ""order"": 1}}",{},,,,,[],[],{},{},{},{},{},2025-09-11,2026-06-15,x,,314246
`

func TestGrouvee_MapsRealFixture(t *testing.T) {
	games, err := Parse([]byte(grouveeFixture), Grouvee())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("want 3 games, got %d", len(games))
	}

	// Portal: tier 100% Completion overrides Played -> dominated; rating 5;
	// hours 36300/3600; pc-windows; igdb 71; review note assembled.
	p := games[0]
	if p.Title != "Portal" || p.IGDBID == nil || *p.IGDBID != 71 {
		t.Fatalf("portal title/igdb = %q/%v", p.Title, p.IGDBID)
	}
	if p.PlayStatus != "dominated" {
		t.Errorf("portal status = %q, want dominated (tier override)", p.PlayStatus)
	}
	if p.PersonalRating == nil || *p.PersonalRating != 5 {
		t.Errorf("portal rating = %v, want 5", p.PersonalRating)
	}
	if p.HoursPlayed == nil || math.Abs(*p.HoursPlayed-36300.0/3600.0) > 1e-9 {
		t.Errorf("portal hours = %v, want %v", p.HoursPlayed, 36300.0/3600.0)
	}
	if len(p.Platforms) != 1 || p.Platforms[0].Platform != "pc-windows" {
		t.Errorf("portal platforms = %+v", p.Platforms)
	}
	if p.PersonalNotes == nil || *p.PersonalNotes != "**Great**\n\nLoved it" {
		t.Errorf("portal notes = %v", p.PersonalNotes)
	}
	if p.CreatedAt != "2017-07-18" {
		t.Errorf("portal created = %q", p.CreatedAt)
	}
	if p.IsWishlisted {
		t.Errorf("portal should not be wishlisted")
	}

	// RDR2: Backlog shelf, but Main Story tier overrides -> completed; ps4; no hours.
	r := games[1]
	if r.PlayStatus != "completed" {
		t.Errorf("rdr2 status = %q, want completed (tier override of Backlog)", r.PlayStatus)
	}
	if r.HoursPlayed != nil {
		t.Errorf("rdr2 hours = %v, want nil (0 seconds)", r.HoursPlayed)
	}
	if len(r.Platforms) != 1 || r.Platforms[0].Platform != "playstation-4" {
		t.Errorf("rdr2 platforms = %+v", r.Platforms)
	}

	// Borderlands 4: Wish List -> wishlisted, default status, no platforms, no rating.
	b := games[2]
	if !b.IsWishlisted {
		t.Errorf("borderlands should be wishlisted")
	}
	if b.PlayStatus != "not_started" {
		t.Errorf("borderlands status = %q, want not_started", b.PlayStatus)
	}
	if len(b.Platforms) != 0 {
		t.Errorf("borderlands platforms = %+v, want none", b.Platforms)
	}
	if b.PersonalRating != nil {
		t.Errorf("borderlands rating = %v, want nil", b.PersonalRating)
	}
}

func TestGrouvee_SignatureRejectsUnrelated(t *testing.T) {
	_, err := Parse([]byte("name,foo,bar\nX,1,2\n"), Grouvee())
	if err == nil {
		t.Fatal("want signature rejection for a non-Grouvee header")
	}
}
```

Also add `TestValidate_AcceptsGrouvee` to `internal/services/csvmap/validate_test.go` (deferred from Task 7):

```go
func TestValidate_AcceptsGrouvee(t *testing.T) {
	if err := validate(Grouvee()); err != nil {
		t.Fatalf("Grouvee() must validate, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run "TestGrouvee|TestValidate_AcceptsGrouvee" -v`
Expected: FAIL — `Grouvee` undefined.

- [ ] **Step 3: Write `Grouvee()`**

Create `internal/services/csvmap/grouvee.go`:

```go
package csvmap

// Grouvee returns the preset Config for a Grouvee CSV export. Grouvee exports an
// IGDB id per row (direct match, #1022). Its shelves and platforms columns are
// JSON objects whose keys carry the data, and its dates column is a JSON play-log
// (seconds_played -> hours_played, level_of_completion -> play_status override).
// See docs/grouvee-import.md.
func Grouvee() Config {
	return Config{
		Signature: []string{
			"shelves", "date_added_to_collection", "review_platform", "giantbomb_id", "igdb_id",
		},
		Columns: ColumnMap{
			Title:     "name",
			IGDBID:    "igdb_id",
			Rating:    "rating",
			CreatedAt: "date_added_to_collection",
		},
		Status: StatusConfig{
			Column: &StatusColumn{
				Column: "shelves",
				Format: FormatJSONKeys,
				ValueMap: map[string]string{
					"playing":   "in_progress",
					"played":    "completed",
					"backlog":   "not_started",
					"wish list": WishlistStatus,
				},
				Precedence: []string{"playing", "played", "backlog"},
				Default:    "not_started",
			},
		},
		PlayLog: &PlayLogConfig{
			Column:          "dates",
			SecondsField:    "seconds_played",
			CompletionField: "level_of_completion",
			CompletionMap: map[string]string{
				"main story":          "completed",
				"main story + extras": "mastered",
				"100% completion":     "dominated",
			},
		},
		Platform: PlatformConfig{
			Simple: &PlatformSimple{
				PlatformColumn: "platforms",
				PlatformFormat: FormatJSONKeys,
				PlatformMap: map[string]string{
					"pc (microsoft windows)": "pc-windows",
					"playstation 5":          "playstation-5",
					"playstation 4":          "playstation-4",
					"playstation 3":          "playstation-3",
					"playstation 2":          "playstation-2",
					"playstation":            "playstation",
					"playstation vita":       "playstation-vita",
					"xbox series x|s":        "xbox-series",
					"xbox one":               "xbox-one",
					"xbox 360":               "xbox-360",
					"xbox":                   "xbox",
					"nintendo switch":        "nintendo-switch",
					"nintendo switch 2":      "nintendo-switch-2",
					"wii":                    "nintendo-wii",
					"wii u":                  "nintendo-wii-u",
					"nintendo 64":            "nintendo-64",
					"nintendo gamecube":      "nintendo-gamecube",
					"nintendo ds":            "nintendo-ds",
					"nintendo 3ds":           "nintendo-3ds",
					"mac":                    "mac",
					"linux":                  "pc-linux",
					"ios":                    "ios",
					"android":                "android",
				},
			},
		},
		Notes: NotesConfig{
			TitleColumn: "review_title",
			Column:      "review",
		},
		Rating:   &RatingConfig{Scale: 5, Truncate: false},
		Grouping: GroupingConfig{MergeByTitle: false},
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/... -run "TestGrouvee|TestValidate_AcceptsGrouvee" -v` → PASS
Run: `go test ./internal/services/csvmap/...` → PASS (whole package).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/grouvee.go internal/services/csvmap/grouvee_test.go internal/services/csvmap/validate_test.go
git commit -m "feat: add Grouvee csvmap preset Config"
```

---

### Task 9: register the Grouvee preset

**Files:**
- Modify: `internal/services/csvmap/presets.go`
- Test: `internal/services/csvmap/presets_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/services/csvmap/presets_test.go`:

```go
func TestPresets_IncludesGrouvee(t *testing.T) {
	cfg, ok := PresetBySlug("grouvee")
	if !ok {
		t.Fatal("expected a 'grouvee' preset in the registry")
	}
	if cfg.Columns.Title != "name" {
		t.Errorf("grouvee preset not wired to Grouvee() (title col = %q)", cfg.Columns.Title)
	}
	var found bool
	for _, p := range Presets() {
		if p.Slug == "grouvee" && p.DisplayName == "Grouvee" {
			found = true
		}
	}
	if !found {
		t.Error("Presets() must list grouvee with DisplayName Grouvee")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/... -run TestPresets_IncludesGrouvee -v`
Expected: FAIL — `PresetBySlug("grouvee")` returns `ok=false`.

- [ ] **Step 3: Register the preset**

In `internal/services/csvmap/presets.go`, add the entry to `presetList`:

```go
var presetList = []Preset{
	{Slug: "completionator", DisplayName: "Completionator", Config: Completionator()},
	{Slug: "grouvee", DisplayName: "Grouvee", Config: Grouvee()},
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/... -run TestPresets -v` → PASS
Run: `go test ./internal/services/csvmap/...` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/presets.go internal/services/csvmap/presets_test.go
git commit -m "feat: register Grouvee as a selectable CSV import preset"
```

---

### Task 10: wire `IsWishlisted` through `ImportFinalizeWorker`

**Files:**
- Modify: `internal/worker/tasks/import_pipeline.go`
- Test: `internal/worker/tasks/import_pipeline_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/worker/tasks/import_pipeline_test.go`:

```go
// A wishlisted, platform-less game keeps is_wishlisted=true after finalize
// (ClearWishlistOnAcquire only clears when a platform exists). A wishlisted game
// that also has a platform is cleared (acquisition wins).
func TestImportFinalize_WishlistFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-gv-wish"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (314246, 'Borderlands 4', now(), now()), (71, 'Portal', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	// Wishlist-only (no platforms) -> stays wishlisted.
	wishPayload := map[string]any{
		"title": "Borderlands 4", "play_status": "not_started",
		"is_wishlisted": true, "platforms": []map[string]any{},
	}
	_, wishItem := insertImportItem(t, userID, wishPayload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 314246 WHERE id = ?`, wishItem).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	// Wishlisted + owned (has a platform) -> cleared on acquire.
	ownedPayload := map[string]any{
		"title": "Portal", "play_status": "completed",
		"is_wishlisted": true,
		"platforms":     []map[string]any{{"platform": "pc-windows"}},
	}
	_, ownedItem := insertImportItem(t, userID, ownedPayload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 71 WHERE id = ?`, ownedItem).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.ImportFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	for _, id := range []string{wishItem, ownedItem} {
		if err := w.Work(ctx, &river.Job[tasks.ImportFinalizeArgs]{Args: tasks.ImportFinalizeArgs{JobItemID: id}}); err != nil {
			t.Fatalf("finalize %s: %v", id, err)
		}
	}

	var wish models.UserGame
	if err := testDB.NewSelect().Model(&wish).Where("user_id = ? AND game_id = 314246", userID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if !wish.IsWishlisted {
		t.Errorf("platform-less wishlist game: is_wishlisted = false, want true")
	}
	var owned models.UserGame
	if err := testDB.NewSelect().Model(&owned).Where("user_id = ? AND game_id = 71", userID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if owned.IsWishlisted {
		t.Errorf("owned game: is_wishlisted = true, want false (cleared on acquire)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/worker/tasks/... -run TestImportFinalize_WishlistFlag -v`
Expected: FAIL — the new `user_game` is inserted without `IsWishlisted`, so the platform-less game's flag is `false`.

- [ ] **Step 3: Set the flag on insert**

In `internal/worker/tasks/import_pipeline.go`, in `ImportFinalizeWorker.Work`, find the `!alreadyExists` branch's `ug = models.UserGame{...}` literal and add `IsWishlisted`:

```go
		ug = models.UserGame{
			ID: uuid.NewString(), UserID: item.UserID, GameID: igdbID,
			PlayStatus: ps, PersonalRating: payload.PersonalRating, IsLoved: payload.IsLoved,
			IsWishlisted:  payload.IsWishlisted,
			PersonalNotes: payload.PersonalNotes, CreatedAt: created, UpdatedAt: now,
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/worker/tasks/... -run TestImportFinalize_WishlistFlag -v` → PASS
Run: `go test ./internal/worker/tasks/... -run TestImportFinalize -v` → PASS (no regression in the other finalize tests).

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/import_pipeline.go internal/worker/tasks/import_pipeline_test.go
git commit -m "feat: persist IsWishlisted from import payload on finalize"
```

---

### Task 11: `docs/grouvee-import.md`

**Files:**
- Create: `docs/grouvee-import.md`

This is a reference doc (not embedded/served — like `docs/darkadia-import.md`). No test.

- [ ] **Step 1: Write the doc**

Create `docs/grouvee-import.md`:

```markdown
# Grouvee CSV Import

This document is the source of truth for how Nexorious imports a game collection
from a **Grouvee CSV export**. Grouvee is a web-based game tracker built around a
"shelves" model; users export their collection to CSV from account settings
(delivered by email). This importer is a **one-off migration path** — no
persistent connection, no incremental re-import.

Grouvee is delivered as a `csvmap` preset `Config` (see
`internal/services/csvmap/grouvee.go`), not a bespoke mapper. A Grouvee file is
selected via the **Format** dropdown in the CSV import dialog (or, once #1015
lands, auto-detected by its header signature).

## Identity and matching: IGDB id is exported

Every Grouvee row carries a populated **`igdb_id`** column. Nexorious uses it for
a **direct id match** — the game is hydrated from IGDB by id, skipping title
matching and the `pending_review` surface entirely (the #1022 path). A row that
somehow lacks the id falls back to title matching. As with every import, **IGDB
must be configured** (hydration by id still calls the IGDB API), or the import is
refused up front.

## CSV format

Standard RFC-4180 CSV, UTF-8, one header row, 20 columns. Three columns embed
JSON:

- **`shelves`** — a JSON object keyed by shelf name, e.g.
  `{"Played": {"date_added": "...", "url": "..."}}`. The **keys** are the shelves.
- **`platforms`** — a JSON object keyed by IGDB platform name, e.g.
  `{"PC (Microsoft Windows)": {"url": "..."}}` (often `{}`).
- **`dates`** — a JSON array of play-session objects, e.g.
  `[{"seconds_played": 36300, "level_of_completion": "100% Completion", ...}]`.

## Column reference

| Column | Fate |
|---|---|
| `id` | Grouvee's own game id — dropped (Nexorious keys on IGDB). |
| `name` | Title (fallback matching for any row missing `igdb_id`). |
| `shelves` | JSON object → `play_status` (baseline) + `is_wishlisted`. |
| `platforms` | JSON object → owned-platform entries. |
| `rating` | → `personal_rating`, 1–5 scale, **rounded to nearest** (`4.5` → `5`). |
| `review_title` | → note heading (`**title**`). |
| `review` | → note body. |
| `review_platform` | Dropped (no model home; part of the signature). |
| `dates` | JSON play-log → `hours_played` (`seconds_played`) and a completion-tier `play_status` override (`level_of_completion`). Start/finish dates are not read. |
| `statuses` | Dropped — **verified empty** in real exports even after setting statuses in-app. |
| `genres`, `franchises`, `series`, `developers`, `publishers`, `release_date` | Dropped — editorial metadata IGDB resupplies on match. |
| `date_added_to_collection` | → `user_games.created_at`. |
| `url` | Grouvee page URL — dropped. |
| `giantbomb_id` | Dropped (part of the signature). |
| `igdb_id` | → direct IGDB-id match. |

## Shelves → play_status and wishlist

The shelf keys map as follows. When a game sits on several shelves, precedence is
`Playing > Played > Backlog`. An unrecognized/custom shelf falls back to
`not_started`.

| Shelf | Result |
|---|---|
| `Playing` | `play_status = in_progress` |
| `Played` | `play_status = completed` |
| `Backlog` | `play_status = not_started` |
| `Wish List` | `is_wishlisted = true` (orthogonal to play_status) |

A `Wish List` game with no other shelf imports as a wishlisted, **unowned** entry
(`is_wishlisted = true`, no platforms). If it is also owned (has a platform), the
wishlist flag is cleared on acquire, consistent with the rest of Nexorious.

## Play-log → playtime and completion tier

From the `dates` array:

- **`seconds_played`** is summed across entries and divided by 3600 →
  `hours_played` (0 / unset → no playtime). It lands on the game's first platform
  entry (no platform → playtime is dropped, as elsewhere in import).
- **`level_of_completion`** maps to a completion tier that **overrides** the
  shelf-derived `play_status` whenever present:

  | `level_of_completion` | `play_status` |
  |---|---|
  | `Main Story` | `completed` |
  | `Main Story + Extras` | `mastered` |
  | `100% Completion` | `dominated` |

  Across multiple entries the **highest** tier wins. Unrecognized tiers are
  ignored (the shelf status stands). `date_started` / `date_finished` are not
  used.

## Platform mapping

Grouvee platform names are IGDB platform names. They map to Nexorious platform
slugs (`PC (Microsoft Windows)` → `pc-windows`, `PlayStation 4` →
`playstation-4`, …). The map is **fixture-derived and extensible**; an unrecognized
platform name passes through unchanged. Grouvee's platform objects carry no
storefront or purchase date, so imported platforms have neither.

## Merge semantics

The import merges into the existing library; it never clears it. Game-level
fields are additive-only — an already-present game is not re-wishlisted and its
curation is not overwritten. New `(platform, storefront)` entries are added;
existing ones are not duplicated.

## Known limitations

- **One-off only** — no incremental re-import or saved connection.
- **Half-star ratings** round to the nearest whole star.
- **A populated completion tier always overrides the shelf** (even on a Playing or
  Backlog shelf). In practice a tier is only set on games actually completed.
- **Custom shelves** map to `not_started`.
- **Play-session dates and the `statuses` column are not imported** — Nexorious
  has no model for them.
- **Platform vocabulary is fixture-derived** — unseen IGDB platform names pass
  through unmapped.
```

- [ ] **Step 2: Commit**

```bash
git add docs/grouvee-import.md
git commit -m "docs: add Grouvee CSV import format reference"
```

---

## Final verification

- [ ] **Run the full backend suite for the touched packages**

Run: `go test ./internal/services/csvmap/... ./internal/services/importmodel/... ./internal/worker/tasks/...`
Expected: PASS.

- [ ] **Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Lint**

Run: `golangci-lint run ./internal/services/csvmap/... ./internal/worker/tasks/... ./internal/services/importmodel/...`
Expected: no findings. (Watch for `errcheck` on any new `_ =` and `gosec` — none expected here.)

---

## Self-Review notes (coverage map)

- Spec Extension 1 (JSON-object keys) → Task 2. Extension 2 (shelf→status+wishlist) → Task 3. Extension 3 (note assembly) → Task 5. Extension 4 (play-log) → Task 6. Platform extractor change → Task 4. Validation → Task 7. `IsWishlisted` model field → Task 1; pipeline wiring → Task 10. Grouvee `Config` + signature → Task 8. Registration → Task 9. Doc → Task 11.
- Frontend: no change required — the #1003 Format dropdown is registry-driven (`inspect.presets`), so registering the preset (Task 9) surfaces "Grouvee" automatically.
- No migration: `models.UserGame.IsWishlisted` already exists (#867); `ClearWishlistOnAcquire` already guards on platform existence.
```
