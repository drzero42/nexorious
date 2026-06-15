# Config-driven CSV mapping engine (`csvmap`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `internal/services/csvmap` — a config-driven engine that turns any CSV into canonical `importmodel.Game[]` via a declarative `Config`, implementing the simple subset and declaring (but rejecting) the advanced Darkadia-era features.

**Architecture:** One leaf-ish package. `config.go` holds the frozen `Config` type (full feature shape). `parse.go` holds `Parse(raw, cfg)`, validation, the `MatchesSignature` helper, and per-field extractors. Variants are optional pointer sub-structs; the engine dispatches on which is non-nil and rejects unimplemented advanced slots with a descriptive (non-`ErrInvalidSignature`) error. Pure functions throughout — no DB, no I/O beyond the byte slice.

**Tech Stack:** Go 1.26, stdlib `encoding/csv`, `internal/services/importmodel` for `Game`/`Platform`/`ErrInvalidSignature`. Tests: stdlib `testing` over synthetic `Config`s (no DB, no fixtures).

**Spec:** `docs/superpowers/specs/2026-06-14-issue-1014-csvmap-engine-design.md`

---

## File Structure

- **Create** `internal/services/csvmap/config.go` — the `Config` type and all sub-structs (type declarations only).
- **Create** `internal/services/csvmap/parse.go` — `Parse`, `validate`, `MatchesSignature`, `buildIndex`, `cell`, `normKey`, and per-field extractors.
- **Create** `internal/services/csvmap/parse_test.go` — unit tests over synthetic `Config`s.

`csvmap` imports `importmodel` and stdlib only. It must NOT import `importsource`, `darkadia`, or `vglist`. No other package, the API, the frontend, or the schema is touched.

---

### Task 1: Package + frozen `Config` type

**Files:**
- Create: `internal/services/csvmap/config.go`

- [ ] **Step 1: Write `config.go`**

```go
// Package csvmap is a config-driven engine that maps any CSV export into the
// canonical importmodel.Game shape. A CSV "source" is a Config value (plus an
// optional header Signature), not a hand-written mapper. This package implements
// the simple subset of the Config feature range; the advanced (Darkadia-era)
// sub-structs are declared but rejected by Parse until issue #1016 implements
// their behaviour. See docs/superpowers/specs/2026-06-14-issue-1014-csvmap-engine-design.md.
package csvmap

// Config declaratively describes how to turn one source's CSV into canonical
// importmodel.Game values.
type Config struct {
	Signature    []string        // headers that must all be present; nil = accept any non-empty CSV
	Columns      ColumnMap       // plain scalar field -> source header
	Status       StatusConfig    //
	Platform     PlatformConfig  //
	Notes        NotesConfig     //
	Grouping     GroupingConfig  //
	Rating       *RatingConfig   // nil = ignore ratings
	Duration     *DurationConfig // nil = ignore hours_played
	TruthyValues []string        // Loved truthy set (matched normalized); nil = {"1","true","yes"}
	TagSeparator string          // tag list separator; "" = ","
	DateLayout   string          // Go time layout for date columns; "" = "2006-01-02"
}

// ColumnMap maps each plain scalar canonical field to its source header name.
type ColumnMap struct {
	Title       string // required
	Rating      string
	CreatedAt   string // game "added"/created date
	HoursPlayed string
	Tags        string
	Loved       string
}

// StatusConfig selects how play_status is derived. At most one of Column / Flags.
type StatusConfig struct {
	Column *StatusColumn // SIMPLE (implemented)
	Flags  *StatusFlags  // ADVANCED #1016 (Parse rejects)
}

// StatusColumn derives play_status from a single column via a value map.
type StatusColumn struct {
	Column   string
	ValueMap map[string]string // normalized source value -> play_status
	Default  string            // empty/unmapped -> this; "" falls back to "not_started"
}

// StatusFlags derives play_status from ordered boolean-flag columns (Darkadia).
type StatusFlags struct {
	Rules   []FlagRule // first matching rule (in order) wins
	Default string
}

// FlagRule is one ordered (column is truthy -> status) rule.
type FlagRule struct {
	Column string
	Truthy []string // values meaning "set", e.g. {"1"}
	Status string
}

// PlatformConfig selects how ownership entries are derived. At most one of Simple / Tables.
type PlatformConfig struct {
	Simple *PlatformSimple // SIMPLE (implemented)
	Tables *PlatformTables // ADVANCED #1016 (Parse rejects)
}

// PlatformSimple derives a single (platform, storefront, acquired-date) entry from columns.
type PlatformSimple struct {
	PlatformColumn     string
	StorefrontColumn   string            // optional
	AcquiredDateColumn string            // optional; attaches to the platform entry
	PlatformMap        map[string]string // optional value->slug; nil/miss = passthrough as-is
	StorefrontMap      map[string]string // optional value->slug
}

// PlatformTables is the Darkadia table+precedence model. Behaviour lands in #1016.
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

// PlatformMapping is a platform slug plus an optional inferred storefront fallback.
type PlatformMapping struct {
	Slug               string
	InferredStorefront *string
}

// NotesConfig is a verbatim notes column plus optional advanced assembly.
type NotesConfig struct {
	Column   string        // SIMPLE: verbatim notes column
	Assembly *NoteAssembly // ADVANCED #1016 (Parse rejects)
}

// NoteAssembly describes extra column-sourced note inputs (Darkadia). Behaviour lands in #1016.
type NoteAssembly struct {
	ReviewSubjectColumn string
	ReviewColumn        string
	CopyNoteColumn      string
}

// GroupingConfig selects how multiple CSV rows collapse into one game.
type GroupingConfig struct {
	MergeByTitle bool             // false = one-row; true = merge rows sharing a title
	CopyRows     *CopyRowGrouping // ADVANCED #1016 (Parse rejects)
}

// CopyRowGrouping is Darkadia's blank-name continuation grouping. Behaviour lands in #1016.
type CopyRowGrouping struct {
	ContinuationColumn string // blank here => row continues the previous game as a copy
}

// RatingConfig normalizes a source rating scale to whole 1-5 stars.
type RatingConfig struct {
	Scale    int  // 5, 10, or 100
	Truncate bool // false = round to nearest whole star; true = truncate toward zero
}

// DurationConfig describes the hours_played format.
type DurationConfig struct {
	Format string // "decimal" (SIMPLE) | "h:mm" (ADVANCED #1016, rejected)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/services/csvmap/`
Expected: builds with no errors (no test yet — this file is type declarations only, which the project's testing policy exempts from tests).

- [ ] **Step 3: Commit**

```bash
git add internal/services/csvmap/config.go
git commit -m "feat: csvmap Config type (full feature shape)"
```

---

### Task 2: `Parse` scaffold — CSV read, header index, one-row title extraction

**Files:**
- Create: `internal/services/csvmap/parse.go`
- Create: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing test**

```go
package csvmap

import (
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// titleOnlyConfig is the minimal valid config: one-row grouping, just a title.
func titleOnlyConfig() Config {
	return Config{Columns: ColumnMap{Title: "Name"}}
}

func TestParse_TitleOnly_OneRowPerGame(t *testing.T) {
	csv := "Name,Other\nHalf-Life,x\nPortal,y\n"
	games, err := Parse([]byte(csv), titleOnlyConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games, got %d", len(games))
	}
	if games[0].Title != "Half-Life" || games[1].Title != "Portal" {
		t.Fatalf("unexpected titles: %+v", games)
	}
}

func TestParse_SkipsEmptyTitleRow(t *testing.T) {
	csv := "Name\nHalf-Life\n\nPortal\n"
	games, err := Parse([]byte(csv), titleOnlyConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games (empty-title row skipped), got %d", len(games))
	}
}

func TestParse_HeaderCaseAndWhitespaceInsensitive(t *testing.T) {
	csv := "  NAME \nHalf-Life\n"
	games, err := Parse([]byte(csv), titleOnlyConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 || games[0].Title != "Half-Life" {
		t.Fatalf("header normalization failed: %+v", games)
	}
}

// guard against an unused import until later tasks reference these.
var _ = errors.Is
var _ = importmodel.ErrInvalidSignature
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/services/csvmap/ -run TestParse_TitleOnly -v`
Expected: FAIL — `undefined: Parse`.

- [ ] **Step 3: Write minimal implementation**

```go
package csvmap

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"

	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// normKey lowercases and trims a string for case-insensitive matching.
func normKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// buildIndex maps each normalized header name to its column position (first wins).
func buildIndex(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, name := range header {
		k := normKey(name)
		if _, ok := m[k]; !ok {
			m[k] = i
		}
	}
	return m
}

// cell returns the trimmed value of colName in rec, or "" if the column is
// unconfigured, absent from the header, or past the end of a ragged row.
func cell(rec []string, idx map[string]int, colName string) string {
	if colName == "" {
		return ""
	}
	i, ok := idx[normKey(colName)]
	if !ok || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

// Parse maps a CSV export into canonical games per cfg. On a wrong-shape file
// (failed Signature) it returns an error wrapping importmodel.ErrInvalidSignature.
func Parse(raw []byte, cfg Config) ([]importmodel.Game, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1 // tolerate ragged rows (missing trailing columns)

	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := buildIndex(header)

	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, rec)
	}

	games := make([]importmodel.Game, 0, len(rows))
	for _, rec := range rows {
		g, ok := extractGame(rec, idx, cfg)
		if ok {
			games = append(games, g)
		}
	}
	return games, nil
}

// extractGame builds one Game from a row, or (zero, false) if the title is empty.
func extractGame(rec []string, idx map[string]int, cfg Config) (importmodel.Game, bool) {
	title := cell(rec, idx, cfg.Columns.Title)
	if title == "" {
		return importmodel.Game{}, false
	}
	return importmodel.Game{Title: title}, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run TestParse_ -v`
Expected: PASS (TitleOnly, SkipsEmptyTitleRow, HeaderCaseAndWhitespaceInsensitive).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap Parse scaffold (read, header index, title)"
```

---

### Task 3: Validation guardrails

**Files:**
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `parse_test.go`:

```go
func TestParse_RequiresTitle(t *testing.T) {
	_, err := Parse([]byte("A\nx\n"), Config{})
	if err == nil {
		t.Fatal("want error when Columns.Title is empty")
	}
	if errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Fatal("title error must not be ErrInvalidSignature")
	}
}

func TestParse_RejectsAdvancedSlots(t *testing.T) {
	base := func() Config { return Config{Columns: ColumnMap{Title: "Name"}} }
	cases := map[string]Config{
		"status flags": func() Config { c := base(); c.Status.Flags = &StatusFlags{}; return c }(),
		"platform tables": func() Config {
			c := base()
			c.Platform.Tables = &PlatformTables{}
			return c
		}(),
		"notes assembly": func() Config { c := base(); c.Notes.Assembly = &NoteAssembly{}; return c }(),
		"copy rows": func() Config {
			c := base()
			c.Grouping.CopyRows = &CopyRowGrouping{}
			return c
		}(),
		"h:mm duration": func() Config {
			c := base()
			c.Duration = &DurationConfig{Format: "h:mm"}
			return c
		}(),
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte("Name\nx\n"), cfg)
			if err == nil {
				t.Fatalf("want error for advanced slot %q", name)
			}
			if errors.Is(err, importmodel.ErrInvalidSignature) {
				t.Fatalf("advanced-slot error must not be ErrInvalidSignature (%q)", name)
			}
		})
	}
}

func TestParse_RejectsConflictingStatusVariants(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Status:  StatusConfig{Column: &StatusColumn{Column: "S"}, Flags: &StatusFlags{}},
	}
	_, err := Parse([]byte("Name\nx\n"), cfg)
	if err == nil {
		t.Fatal("want error when both Status.Column and Status.Flags are set")
	}
}

func TestParse_RejectsBadRatingScale(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name"}, Rating: &RatingConfig{Scale: 7}}
	_, err := Parse([]byte("Name\nx\n"), cfg)
	if err == nil {
		t.Fatal("want error for Rating.Scale not in {5,10,100}")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Requires|TestParse_Rejects' -v`
Expected: FAIL — validation not yet implemented (Parse currently accepts these).

- [ ] **Step 3: Implement validation**

Add to `parse.go` (new imports `errors`, `fmt`):

```go
// validate checks the config before any data is read. Advanced (Darkadia-era)
// slots are rejected with a descriptive error that is NOT ErrInvalidSignature.
func validate(cfg Config) error {
	if strings.TrimSpace(cfg.Columns.Title) == "" {
		return errors.New("csvmap: Columns.Title is required")
	}
	if cfg.Status.Column != nil && cfg.Status.Flags != nil {
		return errors.New("csvmap: Status.Column and Status.Flags are mutually exclusive")
	}
	if cfg.Platform.Simple != nil && cfg.Platform.Tables != nil {
		return errors.New("csvmap: Platform.Simple and Platform.Tables are mutually exclusive")
	}
	if cfg.Rating != nil {
		switch cfg.Rating.Scale {
		case 5, 10, 100:
		default:
			return fmt.Errorf("csvmap: Rating.Scale must be 5, 10, or 100, got %d", cfg.Rating.Scale)
		}
	}
	if cfg.Duration != nil {
		switch normKey(cfg.Duration.Format) {
		case "decimal":
		case "h:mm":
			return notImplemented(`Duration.Format "h:mm"`)
		default:
			return fmt.Errorf("csvmap: Duration.Format must be %q or %q, got %q", "decimal", "h:mm", cfg.Duration.Format)
		}
	}
	if cfg.Status.Flags != nil {
		return notImplemented("Status.Flags")
	}
	if cfg.Platform.Tables != nil {
		return notImplemented("Platform.Tables")
	}
	if cfg.Notes.Assembly != nil {
		return notImplemented("Notes.Assembly")
	}
	if cfg.Grouping.CopyRows != nil {
		return notImplemented("Grouping.CopyRows")
	}
	return nil
}

// notImplemented is returned for an advanced Config slot whose behaviour lands in #1016.
func notImplemented(feature string) error {
	return fmt.Errorf("csvmap: %s is not implemented yet (advanced feature, see #1016)", feature)
}
```

Wire it as the first line of `Parse` (before constructing the reader):

```go
func Parse(raw []byte, cfg Config) ([]importmodel.Game, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(raw))
	// ... unchanged ...
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Requires|TestParse_Rejects' -v`
Expected: PASS. Also run the whole package: `go test ./internal/services/csvmap/ -v` — all green.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap config validation and advanced-slot rejection"
```

---

### Task 4: Signature check (`MatchesSignature`)

**Files:**
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `parse_test.go`:

```go
func TestMatchesSignature(t *testing.T) {
	headers := []string{"Name", "Status", "Rating"}
	if !MatchesSignature(headers, Config{Signature: []string{"name", "rating"}}) {
		t.Fatal("expected match (case-insensitive, subset)")
	}
	if MatchesSignature(headers, Config{Signature: []string{"Name", "Missing"}}) {
		t.Fatal("expected no match when a signature column is absent")
	}
	if !MatchesSignature(headers, Config{}) {
		t.Fatal("nil signature must always match")
	}
}

func TestParse_SignatureMismatchIsInvalidSignature(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name"}, Signature: []string{"Name", "Shelves"}}
	_, err := Parse([]byte("Name,Rating\nHalf-Life,5\n"), cfg)
	if err == nil {
		t.Fatal("want error on signature mismatch")
	}
	if !errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run 'Signature' -v`
Expected: FAIL — `undefined: MatchesSignature`.

- [ ] **Step 3: Implement**

Add to `parse.go`:

```go
// MatchesSignature reports whether every name in cfg.Signature is present in
// headers (compared normalized). A nil/empty Signature always matches — the
// generic mapping path accepts any non-empty CSV.
func MatchesSignature(headers []string, cfg Config) bool {
	if len(cfg.Signature) == 0 {
		return true
	}
	present := make(map[string]bool, len(headers))
	for _, h := range headers {
		present[normKey(h)] = true
	}
	for _, name := range cfg.Signature {
		if !present[normKey(name)] {
			return false
		}
	}
	return true
}
```

In `Parse`, after `header, err := r.Read()` and before `idx := buildIndex(header)`:

```go
	if !MatchesSignature(header, cfg) {
		return nil, fmt.Errorf("csv does not match the expected format: %w", importmodel.ErrInvalidSignature)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run 'Signature' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap header signature check"
```

---

### Task 5: Status extraction (single column + value map + default)

**Files:**
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `parse_test.go`:

```go
func TestParse_Status(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Status: StatusConfig{Column: &StatusColumn{
			Column:   "Shelf",
			ValueMap: map[string]string{"playing": "in_progress", "beaten": "completed"},
			Default:  "not_started",
		}},
	}
	csv := "Name,Shelf\nA,Playing\nB,Beaten\nC,Wishlist\nD,\n"
	games, err := Parse([]byte(csv), cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"in_progress", "completed", "not_started", "not_started"}
	for i, w := range want {
		if games[i].PlayStatus != w {
			t.Fatalf("game %d: want status %q, got %q", i, w, games[i].PlayStatus)
		}
	}
}

func TestParse_StatusAbsentDefaultsNotStarted(t *testing.T) {
	games, err := Parse([]byte("Name\nA\n"), Config{Columns: ColumnMap{Title: "Name"}})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if games[0].PlayStatus != "not_started" {
		t.Fatalf("want not_started when no status column, got %q", games[0].PlayStatus)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Status' -v`
Expected: FAIL — `PlayStatus` is empty (not extracted yet).

- [ ] **Step 3: Implement**

Add to `parse.go`:

```go
// extractStatus resolves play_status from the simple status column, or
// "not_started" when no status column is configured.
func extractStatus(rec []string, idx map[string]int, cfg Config) string {
	if cfg.Status.Column == nil {
		return "not_started"
	}
	sc := cfg.Status.Column
	def := sc.Default
	if def == "" {
		def = "not_started"
	}
	v := normKey(cell(rec, idx, sc.Column))
	if v == "" {
		return def
	}
	if status, ok := sc.ValueMap[v]; ok {
		return status
	}
	return def
}
```

In `extractGame`, set the status field:

```go
	g := importmodel.Game{
		Title:      title,
		PlayStatus: extractStatus(rec, idx, cfg),
	}
	return g, true
```

(Replace the previous `return importmodel.Game{Title: title}, true`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Status' -v`
Expected: PASS. Confirm `TestParse_TitleOnly` still passes (it does not set a status column → `not_started`, but it only asserts titles).

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap status extraction (column + value map)"
```

---

### Task 6: Rating extraction (scale, round/truncate, clamp)

**Files:**
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `parse_test.go`:

```go
func ratingOf(t *testing.T, cfg Config, cell string) *int32 {
	t.Helper()
	games, err := Parse([]byte("Name,R\nA,"+cell+"\n"), cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return games[0].PersonalRating
}

func TestParse_Rating(t *testing.T) {
	round5 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 5}}
	trunc5 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 5, Truncate: true}}
	scale10 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 10}}
	scale100 := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}, Rating: &RatingConfig{Scale: 100}}

	if got := ratingOf(t, trunc5, "4.5"); got == nil || *got != 4 {
		t.Fatalf("truncate 4.5/5 want 4, got %v", got)
	}
	if got := ratingOf(t, round5, "4.5"); got == nil || *got != 5 {
		t.Fatalf("round 4.5/5 want 5, got %v", got)
	}
	if got := ratingOf(t, scale10, "7"); got == nil || *got != 4 {
		t.Fatalf("round 7/10 want 4 (3.5 -> 4), got %v", got)
	}
	if got := ratingOf(t, scale100, "100"); got == nil || *got != 5 {
		t.Fatalf("100/100 want 5, got %v", got)
	}
	if got := ratingOf(t, round5, "0"); got != nil {
		t.Fatalf("0 want nil, got %v", got)
	}
	if got := ratingOf(t, round5, ""); got != nil {
		t.Fatalf("empty want nil, got %v", got)
	}
	if got := ratingOf(t, round5, "garbage"); got != nil {
		t.Fatalf("invalid want nil, got %v", got)
	}
}

func TestParse_RatingIgnoredWhenConfigNil(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Rating: "R"}}
	if got := ratingOf(t, cfg, "5"); got != nil {
		t.Fatalf("rating must be nil when Config.Rating is nil, got %v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Rating' -v`
Expected: FAIL — rating not extracted.

- [ ] **Step 3: Implement**

Add to `parse.go` (new import `math`, `strconv`):

```go
// extractRating normalizes a raw rating to whole 1-5 stars per cfg.Rating.
// Returns nil when ratings are disabled, the cell is empty/invalid, or the
// result is <= 0.
func extractRating(raw string, cfg Config) *int32 {
	if cfg.Rating == nil || raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	stars := f / float64(cfg.Rating.Scale) * 5.0
	var v int32
	if cfg.Rating.Truncate {
		v = int32(math.Trunc(stars))
	} else {
		v = int32(math.Round(stars))
	}
	if v <= 0 {
		return nil
	}
	if v > 5 {
		v = 5
	}
	return &v
}
```

In `extractGame`, after building `g`:

```go
	if r := extractRating(cell(rec, idx, cfg.Columns.Rating), cfg); r != nil {
		g.PersonalRating = r
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Rating' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap rating normalization (scale/round/truncate)"
```

---

### Task 7: Loved, Tags, CreatedAt (date), Duration (decimal)

**Files:**
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `parse_test.go`:

```go
func TestParse_Loved(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Loved: "Fav"}} // default truthy {"1","true","yes"}
	games, _ := Parse([]byte("Name,Fav\nA,Yes\nB,0\nC,\n"), cfg)
	if !games[0].IsLoved {
		t.Fatal("A: Yes should be loved")
	}
	if games[1].IsLoved || games[2].IsLoved {
		t.Fatal("B/C should not be loved")
	}
}

func TestParse_LovedCustomTruthy(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Loved: "Fav"}, TruthyValues: []string{"★"}}
	games, _ := Parse([]byte("Name,Fav\nA,★\nB,yes\n"), cfg)
	if !games[0].IsLoved || games[1].IsLoved {
		t.Fatalf("custom truthy failed: %+v", games)
	}
}

func TestParse_Tags(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", Tags: "T"}}
	games, _ := Parse([]byte("Name,T\nA,\"rpg, indie , rpg,\"\n"), cfg)
	want := []string{"rpg", "indie"}
	if len(games[0].Tags) != 2 || games[0].Tags[0] != want[0] || games[0].Tags[1] != want[1] {
		t.Fatalf("tags split/trim/dedupe failed: %#v", games[0].Tags)
	}
}

func TestParse_CreatedAtPassthrough(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", CreatedAt: "Added"}}
	games, _ := Parse([]byte("Name,Added\nA,2021-05-03\nB,not-a-date\n"), cfg)
	if games[0].CreatedAt != "2021-05-03" {
		t.Fatalf("want passthrough date, got %q", games[0].CreatedAt)
	}
	if games[1].CreatedAt != "" {
		t.Fatalf("invalid date should yield empty, got %q", games[1].CreatedAt)
	}
}

func TestParse_CreatedAtCustomLayout(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", CreatedAt: "Added"}, DateLayout: "01/02/2006"}
	games, _ := Parse([]byte("Name,Added\nA,05/03/2021\n"), cfg)
	if games[0].CreatedAt != "2021-05-03" {
		t.Fatalf("want reformatted date 2021-05-03, got %q", games[0].CreatedAt)
	}
}

func TestParse_DurationDecimal(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", HoursPlayed: "H"}, Duration: &DurationConfig{Format: "decimal"}}
	games, _ := Parse([]byte("Name,H\nA,12.5\nB,0\nC,\n"), cfg)
	if games[0].HoursPlayed == nil || *games[0].HoursPlayed != 12.5 {
		t.Fatalf("A: want 12.5, got %v", games[0].HoursPlayed)
	}
	if games[1].HoursPlayed != nil || games[2].HoursPlayed != nil {
		t.Fatal("B/C: want nil hours")
	}
}

func TestParse_DurationIgnoredWhenConfigNil(t *testing.T) {
	cfg := Config{Columns: ColumnMap{Title: "Name", HoursPlayed: "H"}}
	games, _ := Parse([]byte("Name,H\nA,12.5\n"), cfg)
	if games[0].HoursPlayed != nil {
		t.Fatalf("hours must be nil when Config.Duration is nil, got %v", games[0].HoursPlayed)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Loved|TestParse_Tags|TestParse_CreatedAt|TestParse_Duration' -v`
Expected: FAIL — fields not extracted.

- [ ] **Step 3: Implement**

Add to `parse.go` (new import `time`):

```go
var defaultTruthy = []string{"1", "true", "yes"}

// extractLoved reports whether the loved cell matches a truthy value.
func extractLoved(rec []string, idx map[string]int, cfg Config) bool {
	if cfg.Columns.Loved == "" {
		return false
	}
	v := normKey(cell(rec, idx, cfg.Columns.Loved))
	if v == "" {
		return false
	}
	truthy := cfg.TruthyValues
	if truthy == nil {
		truthy = defaultTruthy
	}
	for _, t := range truthy {
		if normKey(t) == v {
			return true
		}
	}
	return false
}

// extractTags splits, trims, drops empties, and order-preserving dedupes the tag cell.
func extractTags(rec []string, idx map[string]int, cfg Config) []string {
	raw := cell(rec, idx, cfg.Columns.Tags)
	if raw == "" {
		return nil
	}
	sep := cfg.TagSeparator
	if sep == "" {
		sep = ","
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(raw, sep) {
		tag := strings.TrimSpace(p)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

// extractDate normalizes a date cell to "2006-01-02". With no DateLayout (or the
// ISO layout) it accepts already-ISO input and rejects anything else. Invalid
// input yields "".
func extractDate(raw string, cfg Config) string {
	if raw == "" {
		return ""
	}
	layout := cfg.DateLayout
	if layout == "" {
		layout = "2006-01-02"
	}
	tm, err := time.Parse(layout, raw)
	if err != nil {
		return ""
	}
	return tm.Format("2006-01-02")
}

// extractHours parses decimal hours. h:mm is rejected at validation, so only the
// decimal branch is needed here.
func extractHours(rec []string, idx map[string]int, cfg Config) *float64 {
	if cfg.Duration == nil {
		return nil
	}
	raw := cell(rec, idx, cfg.Columns.HoursPlayed)
	if raw == "" {
		return nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 {
		return nil
	}
	return &f
}
```

In `extractGame`, set the remaining scalar fields:

```go
	g.IsLoved = extractLoved(rec, idx, cfg)
	g.CreatedAt = extractDate(cell(rec, idx, cfg.Columns.CreatedAt), cfg)
	g.Tags = extractTags(rec, idx, cfg)
	if h := extractHours(rec, idx, cfg); h != nil {
		g.HoursPlayed = h
	}
	if n := cell(rec, idx, cfg.Notes.Column); n != "" {
		g.PersonalNotes = &n
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Loved|TestParse_Tags|TestParse_CreatedAt|TestParse_Duration' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap loved/tags/date/duration/notes extraction"
```

---

### Task 8: Platform (simple) extraction

**Files:**
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `parse_test.go`:

```go
func TestParse_PlatformPassthrough(t *testing.T) {
	cfg := Config{
		Columns:  ColumnMap{Title: "Name"},
		Platform: PlatformConfig{Simple: &PlatformSimple{PlatformColumn: "Plat"}},
	}
	games, _ := Parse([]byte("Name,Plat\nA,SomePlatform\nB,\n"), cfg)
	if len(games[0].Platforms) != 1 || games[0].Platforms[0].Platform != "SomePlatform" {
		t.Fatalf("A: want passthrough platform, got %+v", games[0].Platforms)
	}
	if games[0].Platforms[0].Storefront != nil {
		t.Fatal("A: storefront should be nil")
	}
	if len(games[1].Platforms) != 0 {
		t.Fatalf("B: empty platform cell -> no entry, got %+v", games[1].Platforms)
	}
}

func TestParse_PlatformWithMapsStorefrontAndDate(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn:     "Plat",
			StorefrontColumn:   "Store",
			AcquiredDateColumn: "Bought",
			PlatformMap:        map[string]string{"pc": "pc-windows"},
			StorefrontMap:      map[string]string{"steam": "steam"},
		}},
	}
	games, _ := Parse([]byte("Name,Plat,Store,Bought\nA,PC,Steam,2020-01-02\n"), cfg)
	p := games[0].Platforms[0]
	if p.Platform != "pc-windows" {
		t.Fatalf("want mapped slug pc-windows, got %q", p.Platform)
	}
	if p.Storefront == nil || *p.Storefront != "steam" {
		t.Fatalf("want mapped storefront steam, got %v", p.Storefront)
	}
	if p.AcquiredDate != "2020-01-02" {
		t.Fatalf("want acquired date 2020-01-02, got %q", p.AcquiredDate)
	}
}

func TestParse_PlatformUnmappedPassesThrough(t *testing.T) {
	cfg := Config{
		Columns: ColumnMap{Title: "Name"},
		Platform: PlatformConfig{Simple: &PlatformSimple{
			PlatformColumn: "Plat",
			PlatformMap:    map[string]string{"pc": "pc-windows"},
		}},
	}
	games, _ := Parse([]byte("Name,Plat\nA,Dreamcast\n"), cfg)
	if games[0].Platforms[0].Platform != "Dreamcast" {
		t.Fatalf("unmapped value should pass through, got %q", games[0].Platforms[0].Platform)
	}
}

func TestParse_NoPlatformConfigYieldsNoEntries(t *testing.T) {
	games, _ := Parse([]byte("Name\nA\n"), Config{Columns: ColumnMap{Title: "Name"}})
	if len(games[0].Platforms) != 0 {
		t.Fatalf("want no platform entries, got %+v", games[0].Platforms)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Platform|TestParse_NoPlatform' -v`
Expected: FAIL — platforms not extracted.

- [ ] **Step 3: Implement**

Add to `parse.go`:

```go
// extractPlatforms builds the simple-variant ownership entry (at most one) from
// the configured platform/storefront/acquired-date columns. An empty platform
// cell or no PlatformSimple config yields no entries. Map miss = passthrough.
func extractPlatforms(rec []string, idx map[string]int, cfg Config) []importmodel.Platform {
	ps := cfg.Platform.Simple
	if ps == nil {
		return nil
	}
	pv := cell(rec, idx, ps.PlatformColumn)
	if pv == "" {
		return nil
	}
	slug := pv
	if ps.PlatformMap != nil {
		if mapped, ok := ps.PlatformMap[normKey(pv)]; ok {
			slug = mapped
		}
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
	return []importmodel.Platform{{Platform: slug, Storefront: sf, AcquiredDate: date}}
}
```

In `extractGame`, before `return g, true`:

```go
	g.Platforms = extractPlatforms(rec, idx, cfg)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_Platform|TestParse_NoPlatform' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap simple platform extraction"
```

---

### Task 9: `merge-by-title` grouping

**Files:**
- Modify: `internal/services/csvmap/parse.go`
- Test: `internal/services/csvmap/parse_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `parse_test.go`:

```go
func mergeCfg() Config {
	return Config{
		Columns:  ColumnMap{Title: "Name", Rating: "R"},
		Rating:   &RatingConfig{Scale: 5},
		Status:   StatusConfig{Column: &StatusColumn{Column: "S", ValueMap: map[string]string{"playing": "in_progress", "done": "completed"}, Default: "not_started"}},
		Platform: PlatformConfig{Simple: &PlatformSimple{PlatformColumn: "Plat", StorefrontColumn: "Store"}},
		Grouping: GroupingConfig{MergeByTitle: true},
	}
}

func TestParse_MergeByTitle_UnionsPlatformsFirstWinsScalars(t *testing.T) {
	csv := "Name,S,R,Plat,Store\n" +
		"Hades,playing,5,pc-windows,steam\n" +
		"Hades,done,3,nintendo-switch,nintendo-eshop\n"
	games, err := Parse([]byte(csv), mergeCfg())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("want 1 merged game, got %d", len(games))
	}
	g := games[0]
	if g.PlayStatus != "in_progress" {
		t.Fatalf("first-wins status want in_progress, got %q", g.PlayStatus)
	}
	if g.PersonalRating == nil || *g.PersonalRating != 5 {
		t.Fatalf("first-wins rating want 5, got %v", g.PersonalRating)
	}
	if len(g.Platforms) != 2 {
		t.Fatalf("want 2 union platforms, got %+v", g.Platforms)
	}
}

func TestParse_MergeByTitle_DedupesIdenticalPlatform(t *testing.T) {
	csv := "Name,S,R,Plat,Store\n" +
		"Hades,playing,5,pc-windows,steam\n" +
		"Hades,playing,5,pc-windows,steam\n"
	games, _ := Parse([]byte(csv), mergeCfg())
	if len(games) != 1 || len(games[0].Platforms) != 1 {
		t.Fatalf("want 1 game with 1 deduped platform, got %d games / %+v", len(games), games[0].Platforms)
	}
}

func TestParse_OneRow_DoesNotMerge(t *testing.T) {
	cfg := mergeCfg()
	cfg.Grouping.MergeByTitle = false
	csv := "Name,S,R,Plat,Store\n" +
		"Hades,playing,5,pc-windows,steam\n" +
		"Hades,done,3,nintendo-switch,nintendo-eshop\n"
	games, _ := Parse([]byte(csv), cfg)
	if len(games) != 2 {
		t.Fatalf("one-row grouping want 2 games, got %d", len(games))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_MergeByTitle|TestParse_OneRow' -v`
Expected: FAIL — `MergeByTitle` rows currently produce 2 games (`TestParse_MergeByTitle_*` fail); `TestParse_OneRow_DoesNotMerge` passes already.

- [ ] **Step 3: Implement**

Refactor the game-building loop in `Parse`. Replace:

```go
	games := make([]importmodel.Game, 0, len(rows))
	for _, rec := range rows {
		g, ok := extractGame(rec, idx, cfg)
		if ok {
			games = append(games, g)
		}
	}
	return games, nil
```

with:

```go
	return buildGames(rows, idx, cfg), nil
}

// buildGames dispatches between one-row and merge-by-title grouping.
func buildGames(rows [][]string, idx map[string]int, cfg Config) []importmodel.Game {
	if cfg.Grouping.MergeByTitle {
		return buildMerged(rows, idx, cfg)
	}
	games := make([]importmodel.Game, 0, len(rows))
	for _, rec := range rows {
		if g, ok := extractGame(rec, idx, cfg); ok {
			games = append(games, g)
		}
	}
	return games
}

// platKey is the (platform, storefront) dedupe key for an ownership entry.
func platKey(p importmodel.Platform) string {
	sf := ""
	if p.Storefront != nil {
		sf = *p.Storefront
	}
	return p.Platform + "\x00" + sf
}

// buildMerged collapses rows sharing a normalized title into one game: the first
// row establishes all scalar fields; every row contributes platform entries,
// union-deduped on (platform, storefront). Output order is first-seen order.
func buildMerged(rows [][]string, idx map[string]int, cfg Config) []importmodel.Game {
	type entry struct {
		game importmodel.Game
		seen map[string]bool
	}
	var order []string
	byTitle := map[string]*entry{}
	for _, rec := range rows {
		g, ok := extractGame(rec, idx, cfg)
		if !ok {
			continue
		}
		key := normKey(g.Title)
		e, exists := byTitle[key]
		if !exists {
			e = &entry{game: g, seen: map[string]bool{}}
			for _, p := range g.Platforms {
				e.seen[platKey(p)] = true
			}
			byTitle[key] = e
			order = append(order, key)
			continue
		}
		for _, p := range g.Platforms {
			k := platKey(p)
			if e.seen[k] {
				continue
			}
			e.seen[k] = true
			e.game.Platforms = append(e.game.Platforms, p)
		}
	}
	out := make([]importmodel.Game, 0, len(order))
	for _, key := range order {
		out = append(out, byTitle[key].game)
	}
	return out
}
```

Note: the closing `}` shown after `return buildGames(...)` ends `Parse`; the helpers follow it. Ensure `Parse` no longer has its own trailing `}` duplicated.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/csvmap/ -run 'TestParse_MergeByTitle|TestParse_OneRow' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/csvmap/parse.go internal/services/csvmap/parse_test.go
git commit -m "feat: csvmap merge-by-title grouping"
```

---

### Task 10: Full-package verification & lint

**Files:** none (verification only)

- [ ] **Step 1: Run the full package test suite**

Run: `go test ./internal/services/csvmap/ -v`
Expected: all tests PASS.

- [ ] **Step 2: Build the whole module**

Run: `go build ./...`
Expected: no errors (no other package references `csvmap` yet — confirms it is self-contained).

- [ ] **Step 3: Lint the package**

Run: `golangci-lint run ./internal/services/csvmap/...`
Expected: no findings. If `errcheck` flags a discard, handle the error (do not blank it); if `gosec` flags something, fix or add a per-site `//nolint:gosec // <reason>` only for a confirmed false positive.

- [ ] **Step 4: Confirm untouched packages still pass**

Run: `go test ./internal/services/darkadia/... ./internal/services/vglist/... ./internal/services/importsource/...`
Expected: PASS — this task changed nothing in them (sanity check of the "no user-visible change" acceptance criterion).

- [ ] **Step 5: Commit (only if lint required a fix)**

```bash
git add -A
git commit -m "chore: csvmap lint cleanup"
```

If nothing changed, skip this commit.

---

## Self-Review Notes

**Spec coverage check:**
- Frozen `Config` type (full shape) → Task 1. ✓
- `Parse` simple subset (read/header/group/extract) → Tasks 2, 5–9. ✓
- Validation incl. advanced-slot rejection → Task 3. ✓
- `MatchesSignature` + signature wrap → Task 4. ✓
- Field extraction table (title/status/rating/loved/notes/created/hours/tags/platform) → Tasks 2,5,6,7,8. ✓
- `merge-by-title` first-wins + union dedupe → Task 9. ✓
- Map-key normalization → covered by `normKey` use in status/loved/platform extractors. ✓
- Testing list → Tasks 2–9 mirror each spec test bullet; Task 10 = full run + lint + untouched-package sanity. ✓
- "No user-visible change; darkadia/vglist/registry/API/frontend/schema untouched" → no task edits them; Task 10 step 4 verifies. ✓

**Type consistency:** `Parse`, `validate`, `MatchesSignature`, `buildIndex`, `cell`, `normKey`, `extractGame`, `extractStatus`, `extractRating`, `extractLoved`, `extractTags`, `extractDate`, `extractHours`, `extractPlatforms`, `buildGames`, `buildMerged`, `platKey`, `notImplemented`, `defaultTruthy` — names are used identically across tasks. `extractGame` is extended in Tasks 2/5/6/7/8 (additive, no signature change).

**No DB / no migration / no frontend** — pure Go package, consistent with the spec.
