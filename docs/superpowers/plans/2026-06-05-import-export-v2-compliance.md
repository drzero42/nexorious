# v2.0 Import/Export Compliance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the native Nexorious JSON export and import code into compliance with the v2.0 library interchange spec (`docs/import-export-format.md`).

**Architecture:** Export emits the minimal `2.0` envelope carrying only user-owned data. Import requires IGDB, accepts only `version "2.0"`, re-hydrates each game from `igdb_id` (reusing the existing `ensureGameRow` helper), and leniently coerces invalid enum/rating values rather than failing. CSV stays a separate, human-oriented, export-only convenience format.

**Tech Stack:** Go 1.26, Bun ORM, River queue, Echo v5, stdlib `testing` + testcontainers.

**Spec:** `docs/superpowers/specs/2026-06-05-import-export-v2-compliance-design.md`

---

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `internal/worker/tasks/export.go` | JSON + CSV export | Rewrite JSON structs/`buildJSONDoc`; drop CSV `release_year` |
| `internal/worker/tasks/export_helpers_test.go` | Unit tests for export helpers | Rewrite to `2.0` shape |
| `internal/api/import.go` | Import HTTP handler | Version gate + hard IGDB requirement |
| `internal/api/import_test.go` | Handler tests | New version/IGDB expectations |
| `internal/worker/tasks/import_item.go` | Per-item import worker | New structs, `ensureGameRow` reuse, validation |
| `internal/worker/tasks/import_item_test.go` | Worker tests | New field names, validation expectations |
| `internal/worker/tasks/import_roundtrip_test.go` | Round-trip test | **Create** |
| `docs/import-export-format.md` | Format spec | Status note + CSV paragraph |

Notes that apply throughout:
- The hooks auto-run `gofmt`/`golangci-lint` on edited files and `go build` at turn end. Run targeted tests by hand as each task says.
- JSON unmarshalling ignores unknown fields, so leaving a stray legacy key in a test fixture is harmless — but keys the parser now reads (`platform`, `storefront`) **must** be renamed or the data is dropped.

---

## Task 1: Rewrite JSON export to the v2.0 envelope

**Files:**
- Modify: `internal/worker/tasks/export.go` (structs at lines 185–235; `buildJSONDoc` at 267–369)
- Test: `internal/worker/tasks/export_helpers_test.go` (the `buildJSONDoc` tests, lines 149–264)

- [ ] **Step 1: Replace the export JSON struct block**

In `internal/worker/tasks/export.go`, replace the entire struct block (current lines 185–235: `exportGameJSON`, `exportPlatformJSON`, `exportTagJSON`, `exportStatsJSON`, `exportDocJSON`) with:

```go
type exportGameJSON struct {
	IGDBID         int32                `json:"igdb_id"`
	Title          string               `json:"title"`
	PlayStatus     *string              `json:"play_status"`
	PersonalRating *int32               `json:"personal_rating"`
	IsLoved        bool                 `json:"is_loved"`
	PersonalNotes  *string              `json:"personal_notes"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
	Platforms      []exportPlatformJSON `json:"platforms"`
	Tags           []exportTagJSON      `json:"tags"`
}

type exportPlatformJSON struct {
	Platform        *string  `json:"platform"`
	Storefront      *string  `json:"storefront"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"`
	HoursPlayed     *float64 `json:"hours_played"`
}

type exportTagJSON struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

type exportDocJSON struct {
	Format     string           `json:"format"`
	Version    string           `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Games      []exportGameJSON `json:"games"`
}
```

- [ ] **Step 2: Rewrite `buildJSONDoc`**

Replace the whole `buildJSONDoc` function (current lines 267–369) with:

```go
func buildJSONDoc(_ string, ugs []models.UserGame) exportDocJSON {
	games := make([]exportGameJSON, 0, len(ugs))
	for _, ug := range ugs {
		platforms := make([]exportPlatformJSON, 0, len(ug.Platforms))
		for _, p := range ug.Platforms {
			pj := exportPlatformJSON{
				Platform:        p.Platform,
				Storefront:      p.Storefront,
				OwnershipStatus: p.OwnershipStatus,
				HoursPlayed:     p.HoursPlayed,
			}
			if p.AcquiredDate != nil {
				d := p.AcquiredDate.Format("2006-01-02")
				pj.AcquiredDate = &d
			}
			platforms = append(platforms, pj)
		}

		tags := make([]exportTagJSON, 0, len(ug.Tags))
		for _, ugt := range ug.Tags {
			if ugt.Tag == nil {
				continue
			}
			tags = append(tags, exportTagJSON{Name: ugt.Tag.Name, Color: ugt.Tag.Color})
		}

		var title string
		if ug.Game != nil {
			title = ug.Game.Title
		}

		games = append(games, exportGameJSON{
			IGDBID:         ug.GameID,
			Title:          title,
			PlayStatus:     ug.PlayStatus,
			PersonalRating: ug.PersonalRating,
			IsLoved:        ug.IsLoved,
			PersonalNotes:  ug.PersonalNotes,
			CreatedAt:      ug.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      ug.UpdatedAt.UTC().Format(time.RFC3339),
			Platforms:      platforms,
			Tags:           tags,
		})
	}

	return exportDocJSON{
		Format:     "nexorious-library",
		Version:    "2.0",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Games:      games,
	}
}
```

The `userID` parameter is now unused; `writeJSONExport` still calls `buildJSONDoc(userID, ugs)` — leave the call site as-is (the `_` parameter name keeps the signature stable so the call compiles).

- [ ] **Step 3: Rewrite the `buildJSONDoc` unit tests**

In `internal/worker/tasks/export_helpers_test.go`, replace the four `buildJSONDoc` tests (lines 149–264: `TestBuildJSONDoc_EmptyGames`, `TestBuildJSONDoc_NilTag`, `TestBuildJSONDoc_PlatformDisplayNamesFromSlug`, `TestBuildJSONDoc_WithLovedAndRated`, `TestBuildJSONDoc_HoursFromPlatforms`) with:

```go
func TestBuildJSONDoc_EmptyGames(t *testing.T) {
	doc := buildJSONDoc("u1", nil)
	if doc.Format != "nexorious-library" {
		t.Errorf("format = %q, want nexorious-library", doc.Format)
	}
	if doc.Version != "2.0" {
		t.Errorf("version = %q, want 2.0", doc.Version)
	}
	if len(doc.Games) != 0 {
		t.Errorf("expected 0 games, got %d", len(doc.Games))
	}
}

func TestBuildJSONDoc_NilTag(t *testing.T) {
	ug := models.UserGame{
		ID: "ug1", UserID: "u1", GameID: 42,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Tags: []models.UserGameTag{
			{ID: "t1", UserGameID: "ug1", TagID: "tag1", Tag: nil},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	if len(doc.Games) != 1 {
		t.Fatalf("expected 1 game entry")
	}
	if len(doc.Games[0].Tags) != 0 {
		t.Errorf("expected 0 tags (nil tag skipped), got %d", len(doc.Games[0].Tags))
	}
}

func TestBuildJSONDoc_PlatformCanonicalKeys(t *testing.T) {
	platform := "pc-windows"
	storefront := "steam"
	ug := models.UserGame{
		ID: "ug1", UserID: "u1", GameID: 42,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, Storefront: &storefront, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	if len(doc.Games[0].Platforms) != 1 {
		t.Fatalf("expected 1 platform")
	}
	pj := doc.Games[0].Platforms[0]
	if pj.Platform == nil || *pj.Platform != platform {
		t.Errorf("platform = %v, want %q", pj.Platform, platform)
	}
	if pj.Storefront == nil || *pj.Storefront != storefront {
		t.Errorf("storefront = %v, want %q", pj.Storefront, storefront)
	}
}

func TestBuildJSONDoc_UserFieldsAndPerPlatformHours(t *testing.T) {
	rating := int32(4)
	h := 10.5
	status := "completed"
	platform := "pc-windows"
	ug := models.UserGame{
		ID: "ug1", UserID: "u1", GameID: 42,
		IsLoved: true, PersonalRating: &rating, PlayStatus: &status,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, HoursPlayed: &h, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	g := doc.Games[0]
	if g.PersonalRating == nil || *g.PersonalRating != 4 {
		t.Errorf("personal_rating = %v, want 4", g.PersonalRating)
	}
	if !g.IsLoved {
		t.Errorf("is_loved = false, want true")
	}
	if g.PlayStatus == nil || *g.PlayStatus != "completed" {
		t.Errorf("play_status = %v, want completed", g.PlayStatus)
	}
	if g.Platforms[0].HoursPlayed == nil || *g.Platforms[0].HoursPlayed != 10.5 {
		t.Errorf("platform hours = %v, want 10.5", g.Platforms[0].HoursPlayed)
	}
}
```

- [ ] **Step 4: Run the export helper tests**

Run: `go test ./internal/worker/tasks/ -run 'TestBuildJSONDoc' -v`
Expected: PASS (4 tests). The CSV helper tests will still be red until Task 2 — that's fine; scope this run to `TestBuildJSONDoc`.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/export_helpers_test.go
git commit -m "feat: emit v2.0 JSON export envelope (#828)"
```

---

## Task 2: Drop `release_year` from CSV export

**Files:**
- Modify: `internal/worker/tasks/export.go` (`csvHeaders` line 373–377; `buildCSVRow` line 417–484)
- Test: `internal/worker/tasks/export_helpers_test.go` (CSV tests, lines 16–143)

- [ ] **Step 1: Remove `release_year` from `csvHeaders`**

Replace the `csvHeaders` var (lines 373–377) with:

```go
var csvHeaders = []string{
	"title", "igdb_id", "play_status", "personal_rating", "is_loved",
	"hours_played", "personal_notes", "platforms", "tags",
	"created_at", "updated_at",
}
```

- [ ] **Step 2: Remove `release_year` from `buildCSVRow`**

In `buildCSVRow` delete the `releaseYear` block (current lines 465–468):

```go
	releaseYear := ""
	if ug.Game != nil && ug.Game.ReleaseDate != nil {
		releaseYear = strconv.Itoa(ug.Game.ReleaseDate.Year())
	}
```

and remove `releaseYear,` from the returned slice (it sits between `strings.Join(tagNames, ";")` and `ug.CreatedAt...`). The final return becomes:

```go
	return []string{
		title,
		strconv.Itoa(int(ug.GameID)),
		playStatus,
		rating,
		strconv.FormatBool(ug.IsLoved),
		hours,
		notes,
		strings.Join(platformSlugs, ";"),
		strings.Join(tagNames, ";"),
		ug.CreatedAt.UTC().Format(time.RFC3339),
		ug.UpdatedAt.UTC().Format(time.RFC3339),
	}
```

- [ ] **Step 3: Fix the CSV unit test that asserts the release_year column**

In `export_helpers_test.go`, `TestBuildCSVRow_AllFieldsSet` (lines 49–103) asserts `row[9] == "2020"` (the old `release_year` column). The column is gone; `row[9]` is now `created_at`. Delete this assertion block (lines 100–102):

```go
	if row[9] != "2020" {
		t.Errorf("release_year: expected '2020', got %q", row[9])
	}
```

The `releaseDate := time.Date(2020, ...)` local and the `Game.ReleaseDate` field in the fixture can stay (harmless) or be removed; leaving them is fine. The other CSV tests reference `row[0]`, `row[2]`, `row[3]`, `row[7]`, `row[8]` — all unchanged by dropping column index 9, so they pass as-is.

- [ ] **Step 4: Run the CSV helper tests**

Run: `go test ./internal/worker/tasks/ -run 'TestBuildCSVRow' -v`
Expected: PASS (all 5 `TestBuildCSVRow_*` tests).

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/export_helpers_test.go
git commit -m "feat: drop release_year from CSV export (#828)"
```

---

## Task 3: Import handler — version gate + hard IGDB requirement

**Files:**
- Modify: `internal/api/import.go` (`nexoriousExport` lines 41–45; `HandleImportNexorious` lines 47–82)
- Test: `internal/api/import_test.go`

- [ ] **Step 1: Update the test fixtures to v2.0 first (red)**

In `internal/api/import_test.go`:

1. Rewrite `validExportJSON` (lines 58–86) so the envelope is v2.0:

```go
func validExportJSON(t *testing.T, n int) []byte {
	t.Helper()

	type gameEntry struct {
		IgdbID *int   `json:"igdb_id,omitempty"`
		Title  string `json:"title"`
	}

	games := make([]gameEntry, n)
	for i := range games {
		id := i + 1
		games[i] = gameEntry{IgdbID: &id, Title: fmt.Sprintf("Game %d", i+1)}
	}

	export := map[string]any{
		"format":  "nexorious-library",
		"version": "2.0",
		"games":   games,
	}

	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	return data
}
```

2. In `TestImportNexorious_EmptyGames` (lines 200–204) change the inline body to v2.0:

```go
	emptyGames := map[string]any{
		"format":  "nexorious-library",
		"version": "2.0",
		"games":   []map[string]any{},
	}
```

3. Switch every nexorious test that expects to get *past* the IGDB guard to a configured-IGDB Echo. Replace `e := newTestEchoPool(t, testDB, cfg)` with `e := newTestEchoConfiguredIGDB(t, testDB, cfg, darkadiaTestIGDB(true))` in: `TestImportNexorious_Success`, `TestImportNexorious_Conflict`, `TestImportNexorious_MalformedRecord`, `TestImportNexorious_EmptyGames`, `TestImportNexorious_InvalidJSON`, `TestImportNexorious_WrongVersion`. (Leave `TestImportNexorious_NoFile` on `newTestEchoPool` — see step 3 below, it becomes the not-configured case is separate; NoFile with configured IGDB also returns 400 on missing file, so either works — switch it too for consistency.)

4. Rewrite `TestImportNexorious_WrongVersion` (lines 164–191) to send a legacy `1.x` file and assert the new message:

```go
func TestImportNexorious_WrongVersion(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoConfiguredIGDB(t, testDB, cfg, darkadiaTestIGDB(true))

	_, token := setupTagUser(t, testDB, e, "imp-wrongver")

	legacy := map[string]any{
		"export_version": "1.2",
		"games":          []map[string]any{{"title": "Game 1"}},
	}
	data, _ := json.Marshal(legacy)

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(resp["message"], "2.0") {
		t.Fatalf("error message = %q, want it to mention version 2.0", resp["message"])
	}
}
```

5. Add a new test mirroring the Darkadia IGDB guard (place it next to `TestImportDarkadia_RefusesWhenIGDBNotConfigured`):

```go
func TestImportNexorious_RefusesWhenIGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	// newTestEchoPool wires a nil IGDB client, so the handler's guard fires.
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "imp-noigdb")

	data := validExportJSON(t, 1)
	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run the handler tests to verify they fail**

Run: `go test ./internal/api/ -run 'TestImportNexorious' -v`
Expected: FAIL — `TestImportNexorious_WrongVersion` (old code still emits the 1.2 message / accepts 1.2), `TestImportNexorious_RefusesWhenIGDBNotConfigured` (no guard yet), and the success-path tests (old code rejects `version`-only files because it checks `export_version == "1.2"`).

- [ ] **Step 3: Add the IGDB guard and version gate in the handler**

In `internal/api/import.go`, change `nexoriousExport` (lines 41–45) to:

```go
// nexoriousExport is the expected structure of a nexorious export file.
type nexoriousExport struct {
	Version       string            `json:"version"`
	ExportVersion string            `json:"export_version"` // legacy 1.x key, used only for error messages
	Games         []json.RawMessage `json:"games"`
}
```

In `HandleImportNexorious`, immediately after the `userID == ""` check (after line 52), add the IGDB guard:

```go
	// Prerequisite: IGDB must be configured. Each game is re-hydrated from its
	// igdb_id; with no client an import cannot construct usable games.
	if h.igdbClient == nil || !h.igdbClient.Configured() {
		return echo.NewHTTPError(http.StatusBadRequest, "IGDB must be configured to import a Nexorious library")
	}
```

Replace the version check (current lines 80–82):

```go
	if export.ExportVersion != "1.2" {
		return echo.NewHTTPError(http.StatusBadRequest, "Unsupported export version. Only version 1.2 is supported.")
	}
```

with:

```go
	if export.Version != "2.0" {
		msg := "Unsupported import file. Only Nexorious library format version 2.0 is supported."
		if export.ExportVersion != "" {
			msg = fmt.Sprintf("Unsupported legacy export (version %s). Only Nexorious library format version 2.0 is supported.", export.ExportVersion)
		}
		return echo.NewHTTPError(http.StatusBadRequest, msg)
	}
```

`fmt` is already imported in `import.go`.

- [ ] **Step 4: Run the handler tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestImportNexorious|TestImportDarkadia' -v`
Expected: PASS (all nexorious and darkadia handler tests).

- [ ] **Step 5: Commit**

```bash
git add internal/api/import.go internal/api/import_test.go
git commit -m "feat: gate nexorious import on v2.0 + require IGDB (#828)"
```

---

## Task 4: Import worker — v2.0 structs, IGDB re-hydration reuse, validation

**Files:**
- Modify: `internal/worker/tasks/import_item.go` (structs 40–71; game-upsert block 122–174; rating 207–211; platform insert 297–316)
- Test: `internal/worker/tasks/import_item_test.go`

- [ ] **Step 1: Update worker tests to the v2.0 shape first (red)**

In `internal/worker/tasks/import_item_test.go`:

1. `TestImportItem_BasicGame` (lines 31–37, 65–68): change the fixture so the rating is a valid integer and drop the game-level hours; fix the assertion.

Replace the `gameData` map (lines 31–37) with:

```go
	gameData := map[string]any{
		"igdb_id":         int32(12345),
		"title":           "Test Game",
		"play_status":     "completed",
		"personal_rating": 5,
	}
```

Replace the rating assertion (lines 65–68) with:

```go
	if ug.PersonalRating == nil || *ug.PersonalRating != 5 {
		t.Errorf("personal_rating = %v, want 5", ug.PersonalRating)
	}
```

2. Rename platform keys in every platform fixture. In `TestImportItem_WithPlatformsAndTags` (≈210–214), `TestImportItem_PlatformsSkippedWithoutSeed` (≈292), `TestImportItem_ReimportMergesPlatforms` (≈338), and `TestImportItem_StorefrontNotFound` (≈716): change `"platform_name"` → `"platform"` and `"storefront_name"` → `"storefront"`. The `"is_available"` keys can stay (now ignored) or be deleted.

3. Delete `TestImportItem_WithReleaseDate` (lines 604–646 — the function and its leading comment). It exercises the removed `release_date` JSON-fallback path, which no longer exists (release info is IGDB-sourced; an unconfigured-IGDB import creates a minimal `id`+`title` row with NULL release_date).

4. Add two validation tests (place after `TestImportItem_BasicGame`):

```go
func TestImportItem_InvalidPlayStatusCoercedToNull(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	gameData := map[string]any{
		"igdb_id":         int32(22001),
		"title":           "Bad Status",
		"play_status":     "not_a_real_status",
		"personal_rating": 9, // out of range -> null
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("user_id = ? AND game_id = ?", userID, int32(22001)).Scan(ctx); err != nil {
		t.Fatalf("user_game not found: %v", err)
	}
	if ug.PlayStatus != nil {
		t.Errorf("play_status = %v, want nil (invalid coerced to unset)", ug.PlayStatus)
	}
	if ug.PersonalRating != nil {
		t.Errorf("personal_rating = %v, want nil (out-of-range coerced to unrated)", ug.PersonalRating)
	}

	var item models.JobItem
	if err := testDB.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		t.Fatalf("job_item not found: %v", err)
	}
	if item.Status != models.JobItemStatusCompleted {
		t.Errorf("status = %q, want completed (lenient coercion never fails the item)", item.Status)
	}
}

func TestImportItem_InvalidOwnershipCoercedToOwned(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	insertTestJob(t, testDB, jobID, userID, 1)

	// Seed a platform so the platform row is accepted.
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT (name) DO NOTHING`,
	); err != nil {
		t.Fatalf("seed platform: %v", err)
	}

	gameData := map[string]any{
		"igdb_id": int32(22002),
		"title":   "Bad Ownership",
		"platforms": []map[string]any{
			{"platform": "pc-windows", "ownership_status": "bogus"},
		},
	}
	itemID := insertTestJobItem(t, testDB, jobID, userID, gameData)

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	var ugp models.UserGamePlatform
	if err := testDB.NewSelect().Model(&ugp).
		Join("JOIN user_games ug ON ug.id = user_game_platform.user_game_id").
		Where("ug.user_id = ? AND ug.game_id = ?", userID, int32(22002)).Scan(ctx); err != nil {
		t.Fatalf("user_game_platform not found: %v", err)
	}
	if ugp.OwnershipStatus == nil || *ugp.OwnershipStatus != "owned" {
		t.Errorf("ownership_status = %v, want 'owned'", ugp.OwnershipStatus)
	}
	if !ugp.IsAvailable {
		t.Errorf("is_available = false, want true (imported rows default available)")
	}
}
```

> Check the exact `platforms` table column names against the seed migration if the `INSERT INTO platforms` above fails — adjust the column list to the real NOT NULL columns. Use `\d platforms` via the dev psql connection from CLAUDE.md if needed.

- [ ] **Step 2: Run the worker tests to verify they fail**

Run: `go test ./internal/worker/tasks/ -run 'TestImportItem' -v`
Expected: FAIL — `TestImportItem_BasicGame` (old code truncates 9.5→9, but new fixture sends int 5 and old code still parses float; actually old parser handles it, so the new validation tests are the clear failures), `TestImportItem_InvalidPlayStatusCoercedToNull` (old code stores the invalid status verbatim), `TestImportItem_InvalidOwnershipCoercedToOwned` (old code stores "bogus"). Compilation also fails if `release_date`-only helpers were removed — that's expected pre-implementation.

- [ ] **Step 3: Add the enum import**

In `internal/worker/tasks/import_item.go`, add to the import block:

```go
	"github.com/drzero42/nexorious/internal/enum"
```

- [ ] **Step 4: Replace the parsed structs**

Replace `importGameData` (lines 40–60) and `importPlatformData` (lines 62–71) with:

```go
// importGameData is the parsed shape inside JobItem.SourceMetadata.data (v2.0).
type importGameData struct {
	IGDBID         int32                `json:"igdb_id"`
	Title          string               `json:"title"`
	PlayStatus     *string              `json:"play_status"`
	PersonalRating *int                 `json:"personal_rating"`
	IsLoved        bool                 `json:"is_loved"`
	PersonalNotes  *string              `json:"personal_notes"`
	CreatedAt      *string              `json:"created_at"` // RFC3339
	UpdatedAt      *string              `json:"updated_at"` // RFC3339
	Platforms      []importPlatformData `json:"platforms"`
	Tags           []importTagData      `json:"tags"`
}

type importPlatformData struct {
	Platform        string   `json:"platform"`
	Storefront      string   `json:"storefront"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"` // date-only or RFC3339
	HoursPlayed     *float64 `json:"hours_played"`
}
```

(Leave `parseFlexibleDate` and `importTagData` as they are.)

- [ ] **Step 5: Replace the game-upsert block with `ensureGameRow`**

Replace the whole block from `// Upsert Game — fetch from IGDB if not already in DB` through the game-insert error handling (current lines 122–174 — the `existingGame`/`gameExists` lookup, the IGDB fetch, the `if game == nil` JSON fallback, and the `if !gameExists { ... insert ... }` block) with:

```go
	// Re-hydrate the game from IGDB by id (cover art, metadata). On any per-item
	// IGDB failure, ensureGameRow inserts a minimal id+title row so user data is
	// preserved; a later metadata refresh fills in the rest.
	if err := ensureGameRow(ctx, w.DB, w.IGDBClient, w.StoragePath, gd.IGDBID, gd.Title); err != nil {
		markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("ensure game row: %v", err), "import_item: markItemFailed")
		checkJobCompletion(w.DB, item.JobID)
		return nil
	}
```

After this change, the `igdb` package is still used (the `*igdb.Client` worker field), so keep the import. Verify no remaining reference to the removed `existingGame`, `gameExists`, or `game` locals (the user-game block below uses `gd.IGDBID` directly).

- [ ] **Step 6: Add lenient rating + play_status validation on the new-user-game path**

In the `else` branch that builds a new `UserGame` (the block currently at lines ~193–223), replace the rating conversion (lines 207–211):

```go
		var personalRating *int32
		if gd.PersonalRating != nil {
			r := int32(*gd.PersonalRating)
			personalRating = &r
		}
```

with:

```go
		var personalRating *int32
		if gd.PersonalRating != nil && *gd.PersonalRating >= 1 && *gd.PersonalRating <= 5 {
			r := int32(*gd.PersonalRating) //nolint:gosec // bounded to 1..5 above
			personalRating = &r
		} else if gd.PersonalRating != nil {
			slog.Warn("import_item: personal_rating out of range, treating as unrated", "value", *gd.PersonalRating)
		}

		playStatus := gd.PlayStatus
		if playStatus != nil && !enum.PlayStatus(*playStatus).Valid() {
			slog.Warn("import_item: invalid play_status, treating as unset", "value", *playStatus)
			playStatus = nil
		}
```

Then change the `UserGame` literal's `PlayStatus: gd.PlayStatus,` to `PlayStatus: playStatus,`.

- [ ] **Step 7: Update the platform loop (field names, ownership coercion, default-available)**

In the platform loop, replace the empty-check and the lookup that reference `pd.PlatformID`/`pd.StorefrontID` with `pd.Platform`/`pd.Storefront`:

- Line 259: `if pd.PlatformID == "" {` → `if pd.Platform == "" {`
- Line 266: `"SELECT name FROM platforms WHERE name = ?", pd.PlatformID,` → `... , pd.Platform,`
- Line 268 log arg: `"platform", pd.PlatformID` → `"platform", pd.Platform`
- Line 274: `if pd.StorefrontID != "" {` → `if pd.Storefront != "" {`
- Line 277: `"SELECT name FROM storefronts WHERE name = ?", pd.StorefrontID,` → `... , pd.Storefront,`
- Line 281 log arg: `"storefront", pd.StorefrontID` → `"storefront", pd.Storefront`

Delete the game-level-hours backfill block (lines 294–301):

```go
		// Backward-compat: old exports stored hours at game level only.
		...
		hoursPlayed := pd.HoursPlayed
		if hoursPlayed == nil && gd.HoursPlayed != nil && !gameHoursApplied {
			hoursPlayed = gd.HoursPlayed
			gameHoursApplied = true
		}
```

and remove the now-unused `gameHoursApplied := false` declaration near line 257.

Add ownership coercion just before building `ugp`:

```go
		ownership := pd.OwnershipStatus
		if ownership == nil || !enum.OwnershipStatus(*ownership).Valid() {
			if ownership != nil {
				slog.Warn("import_item: invalid ownership_status, defaulting to owned", "value", *ownership)
			}
			owned := string(enum.OwnershipOwned)
			ownership = &owned
		}
```

Replace the `ugp` literal (lines 303–316) with:

```go
		ugp := &models.UserGamePlatform{
			ID:              uuid.NewString(),
			UserGameID:      ug.ID,
			Platform:        &platformName,
			Storefront:      storefrontPtr,
			IsAvailable:     true, // imported rows default available; sync re-derives
			HoursPlayed:     pd.HoursPlayed,
			OwnershipStatus: ownership,
			AcquiredDate:    parseFlexibleDate(pd.AcquiredDate),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
```

(`StoreGameID`, `StoreUrl`, `ExternalGameID`, `SyncFromSource` are omitted → nil/false defaults.)

- [ ] **Step 8: Run the worker tests**

Run: `go test ./internal/worker/tasks/ -run 'TestImportItem' -v`
Expected: PASS (all `TestImportItem_*`, including the two new validation tests).

- [ ] **Step 9: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/import_item_test.go
git commit -m "feat: parse v2.0 import shape, reuse ensureGameRow, validate enums (#828)"
```

---

## Task 5: Round-trip test (export → import preserves user data)

**Files:**
- Create: `internal/worker/tasks/import_roundtrip_test.go`

- [ ] **Step 1: Write the round-trip test**

This test seeds a user game with a platform and a tag, runs `buildJSONDoc` to produce the v2.0 doc, then feeds each game entry through `ImportItemWorker` into a *fresh* user, and asserts the user-owned data reproduces. With an unconfigured IGDB client, `ensureGameRow` takes the minimal-row fallback (preserving title), which is sufficient for asserting user-owned fields.

```go
package tasks_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

func TestImport_RoundTripPreservesUserData(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Seed platform + storefront referenced by the source game.
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed platform: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed storefront: %v", err)
	}

	// Source user with one fully-populated game.
	srcUser := uuid.NewString()
	insertTestUser(t, testDB, srcUser)

	game := &models.Game{ID: 7777, Title: "Round Trip", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(game).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}

	status := "completed"
	rating := int32(4)
	notes := "loved it"
	ug := &models.UserGame{
		ID: uuid.NewString(), UserID: srcUser, GameID: 7777,
		PlayStatus: &status, PersonalRating: &rating, IsLoved: true, PersonalNotes: &notes,
		CreatedAt: time.Now().UTC().Truncate(time.Second), UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if _, err := testDB.NewInsert().Model(ug).Exec(ctx); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	plat := "pc-windows"
	store := "steam"
	own := "owned"
	hours := 12.5
	acq := time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC)
	ugp := &models.UserGamePlatform{
		ID: uuid.NewString(), UserGameID: ug.ID, Platform: &plat, Storefront: &store,
		OwnershipStatus: &own, HoursPlayed: &hours, AcquiredDate: &acq, IsAvailable: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := testDB.NewInsert().Model(ugp).Exec(ctx); err != nil {
		t.Fatalf("insert ugp: %v", err)
	}

	color := "#7C3AED"
	tag := &models.Tag{ID: uuid.NewString(), UserID: srcUser, Name: "metroidvania", Color: &color, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(tag).Exec(ctx); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	if _, err := testDB.NewInsert().Model(&models.UserGameTag{ID: uuid.NewString(), UserGameID: ug.ID, TagID: tag.ID, CreatedAt: time.Now()}).Exec(ctx); err != nil {
		t.Fatalf("insert user_game_tag: %v", err)
	}

	// Export.
	ugs, err := tasks.LoadUserGamesWithRelationsForTest(ctx, testDB, srcUser)
	if err != nil {
		t.Fatalf("load source games: %v", err)
	}
	doc := tasks.BuildJSONDocForTest(srcUser, ugs)
	if len(doc.Games) != 1 {
		t.Fatalf("expected 1 exported game, got %d", len(doc.Games))
	}

	// Import into a fresh user.
	dstUser := uuid.NewString()
	insertTestUser(t, testDB, dstUser)
	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, dstUser, len(doc.Games))

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	for _, g := range doc.Games {
		raw, err := json.Marshal(g)
		if err != nil {
			t.Fatalf("marshal exported game: %v", err)
		}
		var asMap map[string]any
		if err := json.Unmarshal(raw, &asMap); err != nil {
			t.Fatalf("game to map: %v", err)
		}
		itemID := insertTestJobItem(t, testDB, jobID, dstUser, asMap)
		if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
			t.Fatalf("import work: %v", err)
		}
	}

	// Assert destination user-owned data.
	var got models.UserGame
	if err := testDB.NewSelect().Model(&got).Where("user_id = ? AND game_id = ?", dstUser, int32(7777)).Scan(ctx); err != nil {
		t.Fatalf("dst user_game not found: %v", err)
	}
	if got.PlayStatus == nil || *got.PlayStatus != "completed" {
		t.Errorf("play_status = %v, want completed", got.PlayStatus)
	}
	if got.PersonalRating == nil || *got.PersonalRating != 4 {
		t.Errorf("personal_rating = %v, want 4", got.PersonalRating)
	}
	if !got.IsLoved {
		t.Errorf("is_loved = false, want true")
	}
	if got.PersonalNotes == nil || *got.PersonalNotes != "loved it" {
		t.Errorf("personal_notes = %v, want 'loved it'", got.PersonalNotes)
	}

	var gotP models.UserGamePlatform
	if err := testDB.NewSelect().Model(&gotP).Where("user_game_id = ?", got.ID).Scan(ctx); err != nil {
		t.Fatalf("dst platform not found: %v", err)
	}
	if gotP.Platform == nil || *gotP.Platform != "pc-windows" {
		t.Errorf("platform = %v, want pc-windows", gotP.Platform)
	}
	if gotP.Storefront == nil || *gotP.Storefront != "steam" {
		t.Errorf("storefront = %v, want steam", gotP.Storefront)
	}
	if gotP.OwnershipStatus == nil || *gotP.OwnershipStatus != "owned" {
		t.Errorf("ownership = %v, want owned", gotP.OwnershipStatus)
	}
	if gotP.HoursPlayed == nil || *gotP.HoursPlayed != 12.5 {
		t.Errorf("hours = %v, want 12.5", gotP.HoursPlayed)
	}
	if gotP.AcquiredDate == nil || gotP.AcquiredDate.Format("2006-01-02") != "2024-12-25" {
		t.Errorf("acquired_date = %v, want 2024-12-25", gotP.AcquiredDate)
	}

	var tagCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM user_game_tags ugt JOIN tags tg ON tg.id = ugt.tag_id
		 WHERE ugt.user_game_id = ? AND LOWER(tg.name) = 'metroidvania'`, got.ID).Scan(ctx, &tagCount); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if tagCount != 1 {
		t.Errorf("tag count = %d, want 1", tagCount)
	}
}
```

- [ ] **Step 2: Add test-only exported wrappers for the unexported export helpers**

`buildJSONDoc` and `loadUserGamesWithRelations` are unexported in package `tasks`, but the round-trip test is in `package tasks_test`. Add a small export-for-test shim. Create `internal/worker/tasks/export_export_test.go` is **not** allowed (a `_test.go` file's exported symbols aren't visible to other packages' tests). Instead add a non-test file `internal/worker/tasks/testexports.go`:

```go
package tasks

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// BuildJSONDocForTest exposes buildJSONDoc for cross-package tests.
func BuildJSONDocForTest(userID string, ugs []models.UserGame) exportDocJSON {
	return buildJSONDoc(userID, ugs)
}

// LoadUserGamesWithRelationsForTest exposes loadUserGamesWithRelations for tests.
func LoadUserGamesWithRelationsForTest(ctx context.Context, db *bun.DB, userID string) ([]models.UserGame, error) {
	return loadUserGamesWithRelations(ctx, db, userID)
}
```

> `exportDocJSON` is unexported, so `BuildJSONDocForTest` returns an unexported type from an exported func. That compiles and the test (same module) can use `doc.Games` because the test references it via the returned value, not by naming the type. If `golangci-lint` flags the unexported-return (`revive`), instead change the round-trip test to live in `package tasks` (internal test) and call `buildJSONDoc`/`loadUserGamesWithRelations` directly — then delete `testexports.go`. Prefer the internal-test approach if lint complains; it is simpler. (Decide at implementation time based on the linter result.)

**Simpler default:** make `import_roundtrip_test.go` an **internal** test (`package tasks`, not `tasks_test`) and call `buildJSONDoc` / `loadUserGamesWithRelations` directly, dropping `testexports.go` and the `tasks.` qualifiers on those two calls and on `ImportItemWorker`/`ImportItemArgs` (use them unqualified). Use this unless you have a reason to keep it external.

- [ ] **Step 3: Run the round-trip test**

Run: `go test ./internal/worker/tasks/ -run 'TestImport_RoundTrip' -v`
Expected: PASS.

- [ ] **Step 4: Run the full tasks + api packages**

Run: `go test ./internal/worker/tasks/ ./internal/api/ -count=1`
Expected: PASS (no regressions).

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/import_roundtrip_test.go internal/worker/tasks/testexports.go
git commit -m "test: round-trip export->import preserves user data (#828)"
```

(Drop `testexports.go` from the `git add` if you used the internal-test approach.)

---

## Task 6: Update the format spec doc

**Files:**
- Modify: `docs/import-export-format.md`

- [ ] **Step 1: Flip the "not yet implemented" status note**

Replace the status paragraph at the top (lines 3–5):

```
**Status:** specification (source of truth). The current code does **not** yet
implement this format — see the tracking issue referenced at the bottom. This
document defines what export must produce and import must accept.
```

with:

```
**Status:** specification (source of truth), implemented as of #828. This
document defines what export produces and import accepts.
```

- [ ] **Step 2: Add the CSV convenience-format note**

Add a new section just before `## Versioning`:

```markdown
## CSV export (separate convenience format)

Alongside the JSON interchange format, Nexorious can export a **CSV** file. CSV
is a **human-oriented, export-only convenience format**: it is *not* governed by
this specification, has **no import counterpart**, and is therefore **not
round-trippable**. Its columns (`title`, `igdb_id`, `play_status`,
`personal_rating`, `is_loved`, `hours_played`, `personal_notes`, `platforms`,
`tags`, `created_at`, `updated_at`) flatten per-platform data into
semicolon-joined cells and a game-level `hours_played` total for readability. The
`release_year` column was removed (release info is IGDB-sourced). Use the JSON
format for moving a library between instances.
```

- [ ] **Step 3: Commit**

```bash
git add docs/import-export-format.md
git commit -m "docs: mark v2.0 format implemented; document CSV convenience export (#828)"
```

---

## Final verification

- [ ] **Run the full affected suites**

Run: `go test ./internal/worker/tasks/ ./internal/api/ -count=1`
Expected: PASS.

- [ ] **Build everything**

Run: `go build ./...`
Expected: no errors.

- [ ] **Confirm git state**

Run: `git log --oneline origin/main..HEAD`
Expected: the design-doc commit plus the six task commits, all on `feat/import-export-v2-compliance`.

---

## Self-Review

**Spec coverage** (each spec section → task):
- Export envelope + game/platform/tag shape → Task 1 ✓
- CSV reconciliation (drop `release_year`, keep human-oriented) → Task 2 ✓
- Import version gate (`2.0` only, reject `1.x`) → Task 3 ✓
- Import hard IGDB requirement → Task 3 ✓
- v2.0 parsed structs, single canonical `platform`/`storefront` keys → Task 4 ✓
- IGDB re-hydration with minimal-row fallback (reuse `ensureGameRow`) → Task 4 ✓
- `personal_rating` integer 1–5; invalid → null → Task 4 ✓
- Enum validation; invalid play_status → null, invalid/absent ownership → owned → Task 4 ✓
- Imported rows default available; store-linkage fields not serialized → Task 4 ✓
- Non-destructive merge (unchanged) → covered by existing tests, asserted in Task 5 ✓
- Round-trip test → Task 5 ✓
- Version-gate test → Task 3 ✓
- IGDB-required test → Task 3 ✓
- Docs (status + CSV) → Task 6 ✓

**Placeholder scan:** No TBD/TODO. The two implementation-time judgement calls (exact `platforms` seed columns; internal-vs-external test package) are flagged with concrete fallbacks, not left open.

**Type consistency:** `exportDocJSON{Format,Version,ExportedAt,Games}`, `exportGameJSON`, `exportPlatformJSON{Platform,Storefront,OwnershipStatus,AcquiredDate,HoursPlayed}` consistent across Tasks 1/5. `importGameData`/`importPlatformData` field names (`Platform`, `Storefront`, `PersonalRating *int`) consistent across Task 4 edits and the Task 5 fixtures. `ensureGameRow(ctx, db, client, storagePath, igdbID, title)` signature matches `darkadia.go:265`. `enum.PlayStatus(...).Valid()` / `enum.OwnershipStatus(...).Valid()` / `enum.OwnershipOwned` match `internal/enum/enum.go`.
