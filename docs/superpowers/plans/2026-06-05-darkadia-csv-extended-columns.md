# Darkadia CSV Extended Columns Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Darkadia CSV importer accept exports with extra feature-toggle columns (time-tracking, reviews, tags, completion dates) and map the newly-available Tags and Time-played data into Nexorious.

**Architecture:** Replace exact-header / fixed-index parsing with header-name resolution. `Parse` builds a `columnName → position` map, validates by required-column signature, and **normalizes** every data row into a fixed canonical 34-column layout (the original 29 + 5 appended optional columns). `consolidate` stays positional and unchanged in shape, reading the new columns via new constants. The finalize worker stamps game-level playtime onto the first platform entry and attaches tags by reusing the existing `findOrCreateTag`.

**Tech Stack:** Go, `encoding/csv`, Bun ORM, River workers, stdlib `testing` + testcontainers.

---

## File Structure

- `internal/services/darkadia/darkadia.go` — parser. Header model, validation, row normalization, `consolidate` field mapping, two new helpers (`parseDuration`, `appendUnique`).
- `internal/services/darkadia/darkadia_test.go` — parser unit tests.
- `internal/worker/tasks/darkadia.go` — finalize worker: playtime on first platform, tag attach.
- `internal/worker/tasks/darkadia_test.go` — finalize integration test.
- `docs/darkadia-import.md` — correct the "no tags / no playtime" claims and the validation rule.

The canonical internal layout (positions 0–33):

| Idx | Name | Idx | Name |
|---|---|---|---|
| 0 | Name | 17 | Copy source other |
| 1 | Added | 18 | Copy purchase date |
| 2 | Loved | 19 | Copy box |
| 3 | Owned | 20 | Copy box condition |
| 4 | Played | 21 | Copy box notes |
| 5 | Playing | 22 | Copy manual |
| 6 | Finished | 23 | Copy manual condition |
| 7 | Mastered | 24 | Copy manual notes |
| 8 | Dominated | 25 | Copy complete |
| 9 | Shelved | 26 | Copy complete notes |
| 10 | Rating | 27 | Platforms |
| 11 | Copy label | 28 | Notes |
| 12 | Copy Release | **29** | **Tags** (optional) |
| 13 | Copy platform | **30** | **Time played** (optional) |
| 14 | Copy media | **31** | **Review subject** (optional) |
| 15 | Copy media other | **32** | **Review** (optional) |
| 16 | Copy source | **33** | **Copy notes** (optional) |

Positions 0–28 are the **required signature** (must all be present by name to accept the file). 29–33 are read only if present. The appended order is our internal convention; normalization maps by name, so the real file's interleaved positions don't matter.

---

## Task 1: Name-based header parsing, validation, and row normalization

**Files:**
- Modify: `internal/services/darkadia/darkadia.go`
- Test: `internal/services/darkadia/darkadia_test.go`

- [ ] **Step 1: Add the extended-header test (failing)**

Add to `internal/services/darkadia/darkadia_test.go`:

```go
// extendedHeaderLine is the real 40-column header from an export with
// time-tracking, reviews, completion dates, copy notes, and tags enabled.
const extendedHeaderLine = `Name,Added,Loved,Owned,Played,Playing,Finished,"Date completed",Mastered,"Date mastered",Dominated,"Date dominated",Shelved,"Time played","Time to complete","Time to master","Time to dominate",Rating,"Review subject",Review,"Copy label","Copy Release","Copy platform","Copy media","Copy media other","Copy source","Copy source other","Copy purchase date","Copy box","Copy box condition","Copy box notes","Copy manual","Copy manual condition","Copy manual notes","Copy complete","Copy complete notes","Copy notes",Platforms,Tags,Notes`

func TestParse_AcceptsExtendedHeader(t *testing.T) {
	// One 40-field game row: PC copy bought on Steam, played 148h, tagged, with a copy note.
	row := `"Game X",2013-06-05,0,1,1,0,0,,0,,0,,0,148:00,,,,,,,,,PC,Digital,,Steam,,2013-06-05,,,,,,,,,PS Plus,PC,"Co-op, VR",my note`
	games, err := Parse([]byte(extendedHeaderLine + "\n" + row + "\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	g := games[0]
	if g.Title != "Game X" {
		t.Errorf("title = %q", g.Title)
	}
	if len(g.Platforms) == 0 || g.Platforms[0].Platform != "pc-windows" {
		t.Errorf("platforms = %+v", g.Platforms)
	}
	if g.PersonalNotes == nil || !strings.Contains(*g.PersonalNotes, "my note") {
		t.Errorf("notes = %v", g.PersonalNotes)
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./internal/services/darkadia/ -run TestParse_AcceptsExtendedHeader -v`
Expected: FAIL — `Parse` returns `ErrInvalidHeader` (header length 40 ≠ 29).

- [ ] **Step 3: Extend the canonical layout and add the new column constants**

In `internal/services/darkadia/darkadia.go`, replace the `header` var (currently 29 entries) and the `const` index block with:

```go
// header is the canonical internal column layout: the 29-column required
// signature (0–28) followed by 5 optional feature-toggle columns (29–33) that
// some exports add. Rows are normalized into this layout by header name, so the
// real file's column order and any extra columns it carries do not matter.
var header = []string{
	"Name", "Added", "Loved", "Owned", "Played", "Playing", "Finished",
	"Mastered", "Dominated", "Shelved", "Rating", "Copy label", "Copy Release",
	"Copy platform", "Copy media", "Copy media other", "Copy source",
	"Copy source other", "Copy purchase date", "Copy box", "Copy box condition",
	"Copy box notes", "Copy manual", "Copy manual condition", "Copy manual notes",
	"Copy complete", "Copy complete notes", "Platforms", "Notes",
	// Optional (feature-toggle) columns — read only when present.
	"Tags", "Time played", "Review subject", "Review", "Copy notes",
}

// requiredColumnCount is the number of leading canonical columns that must be
// present (by name) for a file to be accepted as a Darkadia export.
const requiredColumnCount = 29

// Column indices (only the ones the importer reads).
const (
	colName            = 0
	colAdded           = 1
	colLoved           = 2
	colOwned           = 3
	colPlayed          = 4
	colPlaying         = 5
	colFinished        = 6
	colMastered        = 7
	colDominated       = 8
	colShelved         = 9
	colRating          = 10
	colCopyPlatform    = 13
	colCopyMedia       = 14
	colCopySource      = 16
	colCopySourceOther = 17
	colCopyPurchase    = 18
	colPlatforms       = 27
	colNotes           = 28
	colTags            = 29
	colTimePlayed      = 30
	colReviewSubject   = 31
	colReview          = 32
	colCopyNotes       = 33
)
```

- [ ] **Step 4: Replace `headerMatches`/`pad` with name-based parsing helpers**

In `internal/services/darkadia/darkadia.go`, delete `func headerMatches(...)` and `func pad(...)`, and add:

```go
// buildColumnIndex maps each header name to its position. First occurrence wins.
func buildColumnIndex(hdr []string) map[string]int {
	m := make(map[string]int, len(hdr))
	for i, name := range hdr {
		if _, ok := m[name]; !ok {
			m[name] = i
		}
	}
	return m
}

// hasRequiredColumns reports whether every required signature column is present.
func hasRequiredColumns(cols map[string]int) bool {
	for _, name := range header[:requiredColumnCount] {
		if _, ok := cols[name]; !ok {
			return false
		}
	}
	return true
}

// normalize maps a raw record into the canonical layout by header name. Absent
// columns and ragged short rows yield empty strings (ragged-row tolerance).
func normalize(rec []string, cols map[string]int) []string {
	out := make([]string, len(header))
	for canon, name := range header {
		if src, ok := cols[name]; ok && src < len(rec) {
			out[canon] = rec[src]
		}
	}
	return out
}
```

- [ ] **Step 5: Update `Parse` to use the new helpers**

In `internal/services/darkadia/darkadia.go`, replace the body of `Parse` from the header read through the row loop:

```go
	first, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	cols := buildColumnIndex(first)
	if !hasRequiredColumns(cols) {
		return nil, ErrInvalidHeader
	}

	var raws []rawGame
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		row := normalize(rec, cols)
		if row[colName] != "" {
			raws = append(raws, rawGame{named: row, copies: [][]string{row}})
			continue
		}
		if len(raws) == 0 {
			// Continuation row before any named row — malformed; skip defensively.
			continue
		}
		last := &raws[len(raws)-1]
		last.copies = append(last.copies, row)
	}
```

- [ ] **Step 6: Run the new test and the full parser suite**

Run: `go test ./internal/services/darkadia/ -v`
Expected: PASS — `TestParse_AcceptsExtendedHeader` passes; all existing tests (`TestParse_RejectsNonDarkadiaHeader`, `TestParse_GroupsRowsIntoGamesAndCopies`, `TestParse_ToleratesRaggedRowsAndEmbeddedNewline`, the `TestConsolidate_*`, `TestGame_JSONRoundTrip`) still pass — they use `len(header)` and `colName…colNotes`, which are unchanged in value (header only grew at the tail).

- [ ] **Step 7: Commit**

```bash
git add internal/services/darkadia/darkadia.go internal/services/darkadia/darkadia_test.go
git commit -m "fix: accept Darkadia exports with extra feature-toggle columns"
```

---

## Task 2: Map Tags, Time played, Review, and Copy notes in `consolidate`

**Files:**
- Modify: `internal/services/darkadia/darkadia.go` (the `Game` struct, `consolidate`, two helpers)
- Test: `internal/services/darkadia/darkadia_test.go`

- [ ] **Step 1: Add the consolidate mapping test (failing)**

Add to `internal/services/darkadia/darkadia_test.go`:

```go
func TestConsolidate_TagsPlaytimeReviewCopyNotes(t *testing.T) {
	named := mkRow("G", map[int]string{
		colOwned:         "1",
		colTags:          "Co-op, VR",
		colTimePlayed:    "10:30",
		colReviewSubject: "Loved it",
		colReview:        "Best game ever",
		colNotes:         "my note",
		colCopyPlatform:  "PC",
		colCopyNotes:     "PS Plus",
	})
	g := consolidate(rawGame{named: named, copies: [][]string{named}})

	if len(g.Tags) != 2 || g.Tags[0] != "Co-op" || g.Tags[1] != "VR" {
		t.Fatalf("tags = %v, want [Co-op VR]", g.Tags)
	}
	if g.HoursPlayed == nil || *g.HoursPlayed != 10.5 {
		t.Errorf("hours = %v, want 10.5", g.HoursPlayed)
	}
	if g.PersonalNotes == nil {
		t.Fatal("notes nil")
	}
	for _, want := range []string{"my note", "Loved it", "Best game ever", "PS Plus"} {
		if !strings.Contains(*g.PersonalNotes, want) {
			t.Errorf("notes missing %q in: %s", want, *g.PersonalNotes)
		}
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./internal/services/darkadia/ -run TestConsolidate_TagsPlaytimeReviewCopyNotes -v`
Expected: FAIL — `g.Tags` / `g.HoursPlayed` do not exist (compile error), or are empty.

- [ ] **Step 3: Add the two payload fields to `Game`**

In `internal/services/darkadia/darkadia.go`, add to the `Game` struct (after `Platforms []Platform`):

```go
	Tags        []string `json:"tags,omitempty"`
	HoursPlayed *float64 `json:"hours_played,omitempty"`
```

- [ ] **Step 4: Add the `parseDuration` and `appendUnique` helpers**

In `internal/services/darkadia/darkadia.go`, add:

```go
// parseDuration parses Darkadia "H:MM" playtime into hours. "148:00" → 148.0,
// "10:30" → 10.5. Empty, malformed, or non-positive → nil (no playtime).
func parseDuration(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return nil
	}
	v := float64(h) + float64(m)/60.0
	if v <= 0 {
		return nil
	}
	return &v
}

// appendUnique appends s to xs unless already present (order-preserving).
func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}
```

- [ ] **Step 5: Read Tags and Time played in `consolidate`**

In `internal/services/darkadia/darkadia.go`, in `consolidate`, immediately after the rating block:

```go
	if r := parseRating(n[colRating]); r != nil {
		g.PersonalRating = r
	}
```

add:

```go
	for _, t := range splitAggregate(n[colTags]) {
		g.Tags = appendUnique(g.Tags, t)
	}
	if h := parseDuration(n[colTimePlayed]); h != nil {
		g.HoursPlayed = h
	}
```

- [ ] **Step 6: Fold Review into the note lines**

In `internal/services/darkadia/darkadia.go`, in `consolidate`, immediately after the `addNote := func(line string) { ... }` closure definition, add:

```go
	if rev := strings.TrimSpace(n[colReview]); rev != "" {
		if subj := strings.TrimSpace(n[colReviewSubject]); subj != "" {
			addNote("Review — " + subj + "\n" + rev)
		} else {
			addNote("Review: " + rev)
		}
	}
```

- [ ] **Step 7: Fold per-copy Copy notes into the note lines**

In `internal/services/darkadia/darkadia.go`, in `consolidate`, after the existing copy/storefront loop (the `for _, row := range rg.copies { ... slugHasCopy ... }` block that ends with `add(m.slug, sf, ...)`), add a dedicated loop:

```go
	for _, row := range rg.copies {
		if cn := strings.TrimSpace(row[colCopyNotes]); cn != "" {
			addNote("Copy note: " + cn)
		}
	}
```

- [ ] **Step 8: Run the parser suite**

Run: `go test ./internal/services/darkadia/ -v`
Expected: PASS — `TestConsolidate_TagsPlaytimeReviewCopyNotes` passes; all existing tests still pass.

- [ ] **Step 9: Commit**

```bash
git add internal/services/darkadia/darkadia.go internal/services/darkadia/darkadia_test.go
git commit -m "feat: map Darkadia tags, playtime, review, and copy notes"
```

---

## Task 3: Finalize worker — playtime on first platform + tag attach

**Files:**
- Modify: `internal/worker/tasks/darkadia.go` (`DarkadiaFinalizeWorker.Work`)
- Test: `internal/worker/tasks/darkadia_test.go`

- [ ] **Step 1: Add the finalize integration test (failing)**

Add to `internal/worker/tasks/darkadia_test.go`. Add `"database/sql"` to that file's imports.

```go
func TestDarkadiaFinalize_TagsAndPlaytimeOnFirstPlatform(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := "u-dk-tags"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(`INSERT INTO games (id, title, last_updated, created_at) VALUES (77, 'Tagged', now(), now())`).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"title": "Tagged", "play_status": "completed",
		"tags":         []string{"Co-op", "VR"},
		"hours_played": 148.0,
		"platforms": []map[string]any{
			{"platform": "pc-windows", "storefront": "steam"},
			{"platform": "mac"},
		},
	}
	_, itemID := insertDarkadiaItem(t, userID, payload)
	if _, err := testDB.NewRaw(`UPDATE job_items SET resolved_igdb_id = 77 WHERE id = ?`, itemID).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	w := &tasks.DarkadiaFinalizeWorker{DB: testDB, IGDBClient: nil, StoragePath: t.TempDir()}
	if err := w.Work(ctx, &river.Job[tasks.DarkadiaFinalizeArgs]{Args: tasks.DarkadiaFinalizeArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = 77", userID).Scan(ctx); err != nil {
		t.Fatal(err)
	}

	var tagCount int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_game_tags WHERE user_game_id = ?`, ug.ID).Scan(ctx, &tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 2 {
		t.Errorf("tags = %d, want 2", tagCount)
	}

	var pcHours, macHours sql.NullFloat64
	if err := testDB.NewRaw(`SELECT hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = 'pc-windows'`, ug.ID).Scan(ctx, &pcHours); err != nil {
		t.Fatal(err)
	}
	if err := testDB.NewRaw(`SELECT hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = 'mac'`, ug.ID).Scan(ctx, &macHours); err != nil {
		t.Fatal(err)
	}
	if !pcHours.Valid || pcHours.Float64 != 148 {
		t.Errorf("pc hours = %+v, want 148", pcHours)
	}
	if macHours.Valid {
		t.Errorf("mac hours = %+v, want NULL", macHours)
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./internal/worker/tasks/ -run TestDarkadiaFinalize_TagsAndPlaytimeOnFirstPlatform -v`
Expected: FAIL — `tags = 0, want 2` and `pc hours = {0 false}, want 148` (finalize ignores tags/playtime).

- [ ] **Step 3: Stamp playtime on the first platform entry**

In `internal/worker/tasks/darkadia.go`, in the platform-insert loop, change the loop variable to include the index and set hours on index 0. Replace:

```go
	owned := "owned"
	newPlatforms := 0
	for _, pl := range payload.Platforms {
		sf := pl.Storefront
		if existing[[2]string{pl.Platform, deref(sf)}] {
			continue
		}
		platform := pl.Platform
		ugp := models.UserGamePlatform{
			ID: uuid.NewString(), UserGameID: ug.ID, Platform: &platform, Storefront: sf,
			OwnershipStatus: &owned, AcquiredDate: parseDateOnly(pl.AcquiredDate),
			CreatedAt: now, UpdatedAt: now,
		}
```

with:

```go
	owned := "owned"
	newPlatforms := 0
	for i, pl := range payload.Platforms {
		sf := pl.Storefront
		if existing[[2]string{pl.Platform, deref(sf)}] {
			continue
		}
		platform := pl.Platform
		ugp := models.UserGamePlatform{
			ID: uuid.NewString(), UserGameID: ug.ID, Platform: &platform, Storefront: sf,
			OwnershipStatus: &owned, AcquiredDate: parseDateOnly(pl.AcquiredDate),
			CreatedAt: now, UpdatedAt: now,
		}
		// Game-level total playtime lands on the first consolidated entry only,
		// and only when that entry is newly inserted (additive merge).
		if i == 0 {
			ugp.HoursPlayed = payload.HoursPlayed
		}
```

- [ ] **Step 4: Attach tags after the platform loop**

In `internal/worker/tasks/darkadia.go`, immediately after the platform loop closes (before the `changeType := "added"` line), add:

```go
	existingTagIDs := map[string]bool{}
	if alreadyExists {
		var existingUGTs []models.UserGameTag
		if err := w.DB.NewSelect().Model(&existingUGTs).Where("user_game_id = ?", ug.ID).Scan(ctx); err == nil {
			for _, ugt := range existingUGTs {
				existingTagIDs[ugt.TagID] = true
			}
		}
	}
	newTags := 0
	for _, name := range payload.Tags {
		tagID, terr := findOrCreateTag(ctx, w.DB, item.UserID, name, nil)
		if terr != nil {
			slog.Error("darkadia_finalize: find/create tag", "err", terr, "name", name)
			continue
		}
		if existingTagIDs[tagID] {
			continue
		}
		ugt := &models.UserGameTag{ID: uuid.NewString(), UserGameID: ug.ID, TagID: tagID, CreatedAt: now}
		if _, ierr := w.DB.NewInsert().Model(ugt).Exec(ctx); ierr != nil {
			slog.Error("darkadia_finalize: insert user_game_tag", "err", ierr)
		} else {
			newTags++
		}
	}
```

- [ ] **Step 5: Count new tags toward the change type**

In `internal/worker/tasks/darkadia.go`, change:

```go
		if newPlatforms > 0 {
			changeType = "updated"
		} else {
			changeType = "already_in_library"
		}
```

to:

```go
		if newPlatforms > 0 || newTags > 0 {
			changeType = "updated"
		} else {
			changeType = "already_in_library"
		}
```

- [ ] **Step 6: Run the finalize test and the package suite**

Run: `go test ./internal/worker/tasks/ -run TestDarkadiaFinalize -v`
Expected: PASS — the new test passes; existing `TestDarkadiaFinalize_*` tests still pass.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/darkadia.go internal/worker/tasks/darkadia_test.go
git commit -m "feat: finalize Darkadia tags and per-game playtime into the library"
```

---

## Task 4: Correct the Darkadia import documentation

**Files:**
- Modify: `docs/darkadia-import.md`

- [ ] **Step 1: Fix the "What Darkadia does not provide" section**

In `docs/darkadia-import.md`, replace the bullets under **### What Darkadia does not provide**:

```markdown
- **No playtime.** There is no hours-played data.
- **No tags.** Darkadia has no tag concept.
- **No game metadata.** Descriptions, cover art, genres, release dates, etc. are not in the export; Nexorious obtains these from IGDB after matching.
```

with:

```markdown
- **Playtime and tags are feature-gated.** Exports made with Darkadia's time-tracking and tags features enabled carry a `Time played` column and a `Tags` column; exports without those features omit them. When present, `Time played` → `hours_played` (on the game's first platform entry) and `Tags` → `user_game_tags`. When absent, nothing is lost.
- **No game metadata.** Descriptions, cover art, genres, release dates, etc. are not in the export; Nexorious obtains these from IGDB after matching.
```

- [ ] **Step 2: Fix the column-count / validation claims**

In `docs/darkadia-import.md`, under **### Column reference**, change the sentence "The export has **29 columns, in this exact order**." to:

```markdown
The export has a **core of 29 columns**, plus optional columns added when Darkadia features (time-tracking, reviews, completion dates, copy notes, tags) are enabled. Columns are addressed **by header name**, never by fixed position, so the optional columns — which appear interleaved among the core columns — do not shift the import.
```

In the **### CSV dialect and parsing** section, replace the **Header validation** bullet:

```markdown
- **Header validation.** The importer validates the file by matching the 29-column header row (the exact string formed by the columns in "Column reference"); a mismatch rejects the file as non-Darkadia **before** any rows are processed.
```

with:

```markdown
- **Header validation.** The importer parses the header into a name→position map and requires the 29 core column names to all be present (a strong Darkadia signature). Extra columns are tolerated; a file missing any required column is rejected as non-Darkadia **before** any rows are processed. Reordered or interleaved columns are handled because every field is read by name.
```

- [ ] **Step 3: Update the accepted-loss ledger**

In `docs/darkadia-import.md`, under **### Deliberate field drops (accepted-loss ledger)**, add these bullets:

```markdown
- **Completion dates** — `Date completed`, `Date mastered`, `Date dominated`: Nexorious has no per-status completion-date field.
- **Milestone times** — `Time to complete`, `Time to master`, `Time to dominate`: `games.howlongtobeat_*` is IGDB community data, not per-user; there is no per-user milestone-time field. (The user's actual `Time played` *is* mapped, to `hours_played`.)
```

- [ ] **Step 4: Commit**

```bash
git add docs/darkadia-import.md
git commit -m "docs: correct Darkadia format notes for feature-toggle columns"
```

---

## Final verification

- [ ] **Run both affected packages**

Run: `go test ./internal/services/darkadia/ ./internal/worker/tasks/ -v`
Expected: PASS across the board.

- [ ] **Manual end-to-end check against the real files**

Confirm the previously-rejected file now parses to the same game count as the canonical one (both are the same collection, ~1,474 games). A quick way without the server is a throwaway test that calls `darkadia.Parse` on each file and prints `len(games)`; expect both to succeed and land in the same ballpark. Remove the snippet afterward.

- [ ] **Push (triggers the full pre-push gate)**

```bash
git push -u origin fix/darkadia-csv-extended-columns
```
Expected: pre-push hook runs `go test ./...` and passes.
