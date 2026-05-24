# Shared Sync Code Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all compile errors caused by data model alignment and refactor the sync pipeline to split `ProcessSyncItemWorker` into `IGDBMatchWorker` + `UserGameWorker`, add `sync_changes` writes, fix playtime handling, and simplify job completion logic.

**Architecture:** Three-stage pipeline — Stage 1 `DispatchSyncWorker` fetches library and enqueues `IGDBMatchArgs`; Stage 2 `IGDBMatchWorker` resolves IGDB IDs; Stage 3 `UserGameWorker` writes user_games/user_game_platforms and sync_changes. `ProcessSyncItemWorker` is deleted. River's own retry handles transient IGDB failures.

**Tech Stack:** Go, River (riverqueue/river), Bun ORM, PostgreSQL, testcontainers-go

**Spec:** `docs/superpowers/specs/2026-05-24-shared-sync-code-refactor-design.md`

---

## Task 1: Fix export.go and import_item.go compile errors

**Files:**
- Modify: `internal/worker/tasks/export.go`
- Modify: `internal/worker/tasks/import_item.go`
- Modify: `internal/worker/tasks/export_helpers_test.go`
- Modify: `internal/worker/tasks/export_test.go`

- [ ] **Step 1: Write a failing test for hours-from-platforms computation**

Add to `internal/worker/tasks/export_helpers_test.go` (package `tasks`, internal tests):

```go
func TestBuildJSONDoc_HoursFromPlatforms(t *testing.T) {
	h1 := 10.5
	h2 := 4.5
	platform := "pc-windows"
	ug := models.UserGame{
		ID:        "ug1",
		UserID:    "u1",
		GameID:    42,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, HoursPlayed: &h1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: "ugp2", UserGameID: "ug1", Platform: &platform, HoursPlayed: &h2, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	doc := buildJSONDoc("u1", []models.UserGame{ug})
	if doc.ExportStats.TotalHours != 15.0 {
		t.Errorf("TotalHours: want 15.0, got %v", doc.ExportStats.TotalHours)
	}
	if doc.Games[0].HoursPlayed == nil || *doc.Games[0].HoursPlayed != 15.0 {
		t.Errorf("games[0].HoursPlayed: want 15.0, got %v", doc.Games[0].HoursPlayed)
	}
}
```

- [ ] **Step 2: Verify the test fails to compile**

```bash
go build ./internal/worker/tasks/...
```
Expected: compile errors mentioning `HoursPlayed` field on `models.UserGame`.

- [ ] **Step 3: Fix buildJSONDoc in export.go — compute hours from platforms**

In `internal/worker/tasks/export.go`, replace the `buildJSONDoc` loop body. The old block:
```go
		if ug.HoursPlayed != nil {
			stats.TotalHours += *ug.HoursPlayed
		}
```
Replace with (accumulate inside the platforms loop):
```go
		// platforms loop (keep existing pj construction; add hours accumulation)
		var ugTotalHours float64
		platforms := make([]exportPlatformJSON, 0, len(ug.Platforms))
		for _, p := range ug.Platforms {
			if p.HoursPlayed != nil {
				ugTotalHours += *p.HoursPlayed
			}
			// ... existing pj construction unchanged ...
		}
		stats.TotalHours += ugTotalHours
		var ugHoursPtr *float64
		if ugTotalHours > 0 {
			ugHoursPtr = &ugTotalHours
		}
```

Then in the `games = append(games, exportGameJSON{...})` call, change:
```go
		HoursPlayed:    ug.HoursPlayed,
```
to:
```go
		HoursPlayed:    ugHoursPtr,
```

Full replacement for the entire for-loop in `buildJSONDoc` (lines ~263–349 of export.go):

```go
	for _, ug := range ugs {
		if ug.PlayStatus != nil {
			stats.ByStatus[*ug.PlayStatus]++
		}
		if ug.PersonalRating != nil {
			stats.RatedCount++
		}
		if ug.IsLoved {
			stats.LovedCount++
		}

		var releaseYear *int
		if ug.Game != nil && ug.Game.ReleaseDate != nil {
			y := ug.Game.ReleaseDate.Year()
			releaseYear = &y
		}

		var ugTotalHours float64
		platforms := make([]exportPlatformJSON, 0, len(ug.Platforms))
		for _, p := range ug.Platforms {
			if p.HoursPlayed != nil {
				ugTotalHours += *p.HoursPlayed
			}
			pj := exportPlatformJSON{
				PlatformID:      p.Platform,
				StorefrontID:    p.Storefront,
				StoreGameID:     p.StoreGameID,
				StoreURL:        p.StoreUrl,
				IsAvailable:     p.IsAvailable,
				HoursPlayed:     p.HoursPlayed,
				OwnershipStatus: p.OwnershipStatus,
			}
			if p.OriginalPlatformName != nil {
				pj.PlatformName = p.OriginalPlatformName
			} else {
				pj.PlatformName = p.Platform
			}
			if p.OriginalStorefrontName != nil {
				pj.StorefrontName = p.OriginalStorefrontName
			} else {
				pj.StorefrontName = p.Storefront
			}
			if p.AcquiredDate != nil {
				d := p.AcquiredDate.Format("2006-01-02")
				pj.AcquiredDate = &d
			}
			if p.Platform != nil {
				stats.ByPlatform[*p.Platform]++
			}
			platforms = append(platforms, pj)
		}
		stats.TotalHours += ugTotalHours
		var ugHoursPtr *float64
		if ugTotalHours > 0 {
			ugHoursPtr = &ugTotalHours
		}

		tags := make([]exportTagJSON, 0, len(ug.Tags))
		for _, ugt := range ug.Tags {
			if ugt.Tag == nil {
				continue
			}
			tags = append(tags, exportTagJSON{
				Name:  ugt.Tag.Name,
				Color: ugt.Tag.Color,
			})
		}

		var title string
		if ug.Game != nil {
			title = ug.Game.Title
		}

		games = append(games, exportGameJSON{
			IGDBID:         ug.GameID,
			Title:          title,
			ReleaseYear:    releaseYear,
			PlayStatus:     ug.PlayStatus,
			PersonalRating: ug.PersonalRating,
			IsLoved:        ug.IsLoved,
			HoursPlayed:    ugHoursPtr,
			PersonalNotes:  ug.PersonalNotes,
			Platforms:      platforms,
			Tags:           tags,
			CreatedAt:      ug.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      ug.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
```

- [ ] **Step 4: Fix buildCSVRow in export.go — compute hours from platforms**

Replace the hours block in `buildCSVRow` (around line 425–428):
```go
	hours := ""
	if ug.HoursPlayed != nil {
		hours = strconv.FormatFloat(*ug.HoursPlayed, 'f', -1, 64)
	}
```
With:
```go
	var totalHoursF float64
	for _, p := range ug.Platforms {
		if p.HoursPlayed != nil {
			totalHoursF += *p.HoursPlayed
		}
	}
	hours := ""
	if totalHoursF > 0 {
		hours = strconv.FormatFloat(totalHoursF, 'f', -1, 64)
	}
```

- [ ] **Step 5: Fix import_item.go — remove game-level HoursPlayed, add backward-compat fallback**

In `internal/worker/tasks/import_item.go`, remove `HoursPlayed: gd.HoursPlayed,` from the `models.UserGame` struct literal (around line 217):
```go
		ug = &models.UserGame{
			ID:             uuid.NewString(),
			UserID:         item.UserID,
			GameID:         gd.IGDBID,
			PlayStatus:     gd.PlayStatus,
			PersonalRating: personalRating,
			IsLoved:        gd.IsLoved,
			PersonalNotes:  gd.PersonalNotes,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		}
```

In the platform loop (around line 253, inside `for _, pd := range gd.Platforms`), add a fallback for old exports. Replace the line that assigns `HoursPlayed: pd.HoursPlayed,` in the `ugp` construction:

```go
		// Backward-compat: old exports stored hours at game level only.
		// If this platform has no per-platform hours but the game record has a
		// total, apply it to the first platform row as a best-effort migration.
		hoursPlayed := pd.HoursPlayed
		if hoursPlayed == nil && gd.HoursPlayed != nil && len(existingPlatforms) == 0 {
			hoursPlayed = gd.HoursPlayed
		}

		ugp := &models.UserGamePlatform{
			ID:              uuid.NewString(),
			UserGameID:      ug.ID,
			Platform:        &platformName,
			Storefront:      storefrontPtr,
			StoreGameID:     pd.StoreGameID,
			StoreUrl:        pd.StoreUrl,
			IsAvailable:     pd.IsAvailable,
			HoursPlayed:     hoursPlayed,
			OwnershipStatus: pd.OwnershipStatus,
			AcquiredDate:    parseFlexibleDate(pd.AcquiredDate),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
```

- [ ] **Step 6: Fix test compile errors in export_helpers_test.go**

Remove `HoursPlayed: &hours,` from the `models.UserGame` struct literal in `TestBuildCSVRow_AllFieldsSet` (the field no longer exists).

Also update `TestBuildJSONDoc_WithLovedAndRated` — hours now come from platforms. Replace:
```go
	ug := models.UserGame{
		ID:             "ug1",
		UserID:         "u1",
		GameID:         42,
		IsLoved:        true,
		PersonalRating: &rating,
		PlayStatus:     &status,
		HoursPlayed:    &hours,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
```
With:
```go
	platform := "pc-windows"
	ug := models.UserGame{
		ID:             "ug1",
		UserID:         "u1",
		GameID:         42,
		IsLoved:        true,
		PersonalRating: &rating,
		PlayStatus:     &status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Platforms: []models.UserGamePlatform{
			{ID: "ugp1", UserGameID: "ug1", Platform: &platform, HoursPlayed: &hours, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
```

- [ ] **Step 7: Fix test compile errors in export_test.go**

In `TestExportJSON_Task`, remove `HoursPlayed: &hours,` from the `models.UserGame` insert. Also remove the `hours := float64(55.5)` declaration if it becomes unused. The test still passes because the export JSON check doesn't assert on hours_played.

- [ ] **Step 8: Verify build and tests pass**

```bash
go build ./internal/worker/tasks/...
go test -timeout 600s ./internal/worker/tasks/... -run "TestBuildJSONDoc|TestBuildCSVRow|TestExportJSON|TestExportCSV|TestImport" -v 2>&1 | tail -30
```
Expected: all tests PASS, no compile errors.

- [ ] **Step 9: Commit**

```bash
git add internal/worker/tasks/export.go internal/worker/tasks/import_item.go \
        internal/worker/tasks/export_helpers_test.go internal/worker/tasks/export_test.go
git commit -m "$(cat <<'EOF'
fix(sync): compute export hours from platforms; add import backward-compat fallback

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Fix sync.go compile errors + patch DispatchSyncWorker SQL

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`
- Modify: `internal/api/sync.go`

- [ ] **Step 1: Write a failing test for hours_played stored on external_game_platforms**

Add to `internal/worker/tasks/sync_test.go`:

```go
func TestDispatchSync_Steam_PlaytimeStoredOnPlatform(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	rawCreds := `{"web_api_key":"k","steam_id":"s"}`
	credsCiphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt creds: %v", err)
	}
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, credsCiphertext,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{
			{AppID: 730, Title: "Counter-Strike 2", PlaytimeHours: 42},
		},
		platformsByAppID: map[int]steamsvc.Platforms{
			730: {Windows: true, Linux: true},
		},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Primary platform (pc-windows) must get hours_played=42; secondary (pc-linux) gets 0.
	var windowsHours, linuxHours float64
	_ = testDB.NewRaw(`
		SELECT egp.hours_played FROM external_game_platforms egp
		JOIN external_games eg ON eg.id = egp.external_game_id
		WHERE eg.user_id = ? AND eg.external_id = '730' AND egp.platform = 'pc-windows'`, userID,
	).Scan(ctx, &windowsHours)
	_ = testDB.NewRaw(`
		SELECT egp.hours_played FROM external_game_platforms egp
		JOIN external_games eg ON eg.id = egp.external_game_id
		WHERE eg.user_id = ? AND eg.external_id = '730' AND egp.platform = 'pc-linux'`, userID,
	).Scan(ctx, &linuxHours)

	if windowsHours != 42.0 {
		t.Errorf("pc-windows hours_played: want 42.0, got %v", windowsHours)
	}
	if linuxHours != 0.0 {
		t.Errorf("pc-linux hours_played: want 0.0, got %v", linuxHours)
	}
}
```

Also add a test for job_items.external_game_id set directly:

```go
func TestDispatchSync_Steam_JobItemExternalGameIDSet(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	rawCreds := `{"web_api_key":"k","steam_id":"s"}`
	credsCiphertext, _ := testEncrypter.Encrypt([]byte(rawCreds))
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, credsCiphertext,
	).Exec(ctx)

	adapter := &fakeSteamAdapter{
		games: []steamsvc.OwnedGame{{AppID: 730, Title: "Counter-Strike 2", PlaytimeHours: 0}},
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: adapter, RiverClient: nil}
	_ = w.Work(ctx, &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	})

	var egID *string
	_ = testDB.NewRaw(
		`SELECT external_game_id FROM job_items WHERE job_id = ?`, jobID,
	).Scan(ctx, &egID)
	if egID == nil || *egID == "" {
		t.Error("job_items.external_game_id should be set directly, not nil")
	}
}
```

- [ ] **Step 2: Verify tests fail to compile**

```bash
go build ./...
```
Expected: compile errors in `sync.go` (`eg.PlaytimeHours` undefined), `api/sync.go` (`eg.PlaytimeHours` undefined).

- [ ] **Step 3: Fix DispatchSyncWorker Steam case — remove playtime_hours from external_games, add hours_played to platforms**

In `internal/worker/tasks/sync.go`, find the Steam `external_games` INSERT/UPDATE (around line 242). Replace:
```go
				`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, playtime_hours, ownership_status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, true, false, ?, ?, ?, ?)
				ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
					title = EXCLUDED.title,
					playtime_hours = EXCLUDED.playtime_hours,
					is_subscription = EXCLUDED.is_subscription,
					ownership_status = EXCLUDED.ownership_status,
					is_available = true,
					updated_at = now()
				RETURNING id, is_skipped`,
				uuid.NewString(), p.UserID, p.Storefront, appidStr, og.Title,
				og.PlaytimeHours, &ownership, upsertNow, upsertNow,
```
With:
```go
				`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, true, false, ?, ?, ?)
				ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
					title = EXCLUDED.title,
					is_subscription = EXCLUDED.is_subscription,
					ownership_status = EXCLUDED.ownership_status,
					is_available = true,
					updated_at = now()
				RETURNING id, is_skipped`,
				uuid.NewString(), p.UserID, p.Storefront, appidStr, og.Title,
				&ownership, upsertNow, upsertNow,
```

Then in the Steam platform insertion loop (around line 315), replace:
```go
			if _, err := w.DB.NewRaw(`
				INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
				VALUES (?, ?, ?, now())
				ON CONFLICT (external_game_id, platform) DO NOTHING`,
				uuid.NewString(), egID, platform,
			).Exec(ctx); err != nil {
```
With (assign hours_played to the primary platform only — first in `resolvedPlatforms`):
```go
			platformHours := 0.0
			if i == 0 {
				platformHours = float64(og.PlaytimeHours)
			}
			if _, err := w.DB.NewRaw(`
				INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
				VALUES (?, ?, ?, ?, now())
				ON CONFLICT (external_game_id, platform) DO UPDATE SET
					hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
				uuid.NewString(), egID, platform, platformHours,
			).Exec(ctx); err != nil {
```

Note: the loop `for _, platform := range resolvedPlatforms` needs to change to `for i, platform := range resolvedPlatforms`.

- [ ] **Step 4: Fix DispatchSyncWorker PSN case — remove playtime_hours from external_games, add hours_played to platform**

Find the PSN `external_games` INSERT (around line 424). Replace:
```go
						`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, playtime_hours, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							playtime_hours = EXCLUDED.playtime_hours,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						e.IsSubscription, e.PlaytimeHours, &ownership, upsertNow, upsertNow,
```
With:
```go
						`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_available, is_subscription, ownership_status, created_at, updated_at)
						VALUES (?, ?, ?, ?, ?, true, ?, ?, ?, ?)
						ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET
							title = EXCLUDED.title,
							is_subscription = EXCLUDED.is_subscription,
							ownership_status = EXCLUDED.ownership_status,
							is_available = true,
							updated_at = now()
						RETURNING id`,
						uuid.NewString(), p.UserID, p.Storefront, e.ExternalID, e.Title,
						e.IsSubscription, &ownership, upsertNow, upsertNow,
```

Replace the PSN platform insert (around line 444):
```go
					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, created_at)
						VALUES (?, ?, ?, now())
						ON CONFLICT (external_game_id, platform) DO NOTHING`,
						uuid.NewString(), egID, platform,
					).Exec(ctx); err != nil {
```
With:
```go
					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
						VALUES (?, ?, ?, ?, now())
						ON CONFLICT (external_game_id, platform) DO UPDATE SET
							hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
						uuid.NewString(), egID, platform, e.PlaytimeHours,
					).Exec(ctx); err != nil {
```

Fix the PSN job_item INSERT (around line 469). Replace:
```go
					for _, eg := range toProcess {
						metaJSON, _ := json.Marshal(map[string]any{
							"external_game_id": eg.ID,
							"playtime_hours":   eg.PlaytimeHours,
						})
						itemID := uuid.NewString()
						if _, err := w.DB.NewRaw(`
							INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
							VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
							ON CONFLICT (job_id, item_key) DO NOTHING`,
							itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
						).Exec(ctx); err != nil {
```
With:
```go
					for _, eg := range toProcess {
						itemID := uuid.NewString()
						if _, err := w.DB.NewRaw(`
							INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
							VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
							ON CONFLICT (job_id, item_key) DO NOTHING`,
							itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, eg.ID,
						).Exec(ctx); err != nil {
```

- [ ] **Step 5: Fix DispatchSyncWorker Epic case — same pattern**

Epic external_games INSERT (around line 537): remove `playtime_hours` column and value.

Epic platform insert (around line 561): add `hours_played = 0` (Epic has no playtime data):
```go
					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
						VALUES (?, ?, 'pc-windows', 0, now())
						ON CONFLICT (external_game_id, platform) DO UPDATE SET
							hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
						uuid.NewString(), egID,
					).Exec(ctx); err != nil {
```

Epic job_item INSERT (around line 587): same pattern as PSN — use `external_game_id` column, drop `source_metadata` payload:
```go
					for _, eg := range toProcess {
						itemID := uuid.NewString()
						if _, err := w.DB.NewRaw(`
							INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
							VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
							ON CONFLICT (job_id, item_key) DO NOTHING`,
							itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, eg.ID,
						).Exec(ctx); err != nil {
```

- [ ] **Step 6: Fix DispatchSyncWorker GOG case — same pattern**

GOG external_games INSERT (around line 650): remove `playtime_hours` column.

GOG platform insert (around line 700): add `hours_played = 0`:
```go
					if _, err := w.DB.NewRaw(`
						INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
						VALUES (?, ?, ?, 0, now())
						ON CONFLICT (external_game_id, platform) DO UPDATE SET
							hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)`,
						uuid.NewString(), egID, platform,
					).Exec(ctx); err != nil {
```

GOG job_item INSERT (around line 731): use `external_game_id` column:
```go
					for _, eg := range toProcess {
						itemID := uuid.NewString()
						if _, err := w.DB.NewRaw(`
							INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
							VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
							ON CONFLICT (job_id, item_key) DO NOTHING`,
							itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, eg.ID,
						).Exec(ctx); err != nil {
```

Also fix the Steam job_item INSERT (around line 348). The Steam case uses a slightly different structure (deferred enqueue). Replace:
```go
			metaJSON, _ := json.Marshal(map[string]any{
				"external_game_id": egID,
				"playtime_hours":   og.PlaytimeHours,
			})
			itemID := uuid.NewString()
			if _, err := w.DB.NewRaw(`
				INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
				ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, appidStr, og.Title, string(metaJSON),
			).Exec(ctx); err != nil {
```
With:
```go
			itemID := uuid.NewString()
			if _, err := w.DB.NewRaw(`
				INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
				VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())
				ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, appidStr, og.Title, egID,
			).Exec(ctx); err != nil {
```

- [ ] **Step 7: Fix ProcessSyncItemWorker compile errors in sync.go**

Remove the `source_metadata` parsing block (around lines 866–874). Replace:
```go
	var meta struct {
		ExternalGameID string `json:"external_game_id"`
		PlaytimeHours  int    `json:"playtime_hours"`
	}
	if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", meta.ExternalGameID).Scan(ctx); err != nil {
```
With:
```go
	if item.ExternalGameID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
```

Fix user_games INSERT (around line 1032) — remove `hours_played` column (the column no longer exists on user_games):
```go
		ugID = uuid.NewString()
		now := time.Now().UTC()
		if _, err := w.DB.NewRaw(
			`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO NOTHING`,
			ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
		).Exec(ctx); err != nil {
```

Remove the playtime update block entirely (around lines 1050–1057):
```go
	if meta.PlaytimeHours > 0 {
		if _, err := w.DB.NewRaw(
			`UPDATE user_games SET hours_played = ?, updated_at = now() WHERE id = ? AND (hours_played IS NULL OR hours_played < ?)`,
			float64(meta.PlaytimeHours), ugID, float64(meta.PlaytimeHours),
		).Exec(ctx); err != nil {
			...
		}
	}
```
Delete this entire block.

Replace `hoursPlayed := float64(eg.PlaytimeHours)` (around line 1068) with inline `egp.HoursPlayed` in the loop. Find:
```go
	hoursPlayed := float64(eg.PlaytimeHours)

	for _, egp := range egPlatforms {
```
Replace with:
```go
	for _, egp := range egPlatforms {
		hoursPlayed := egp.HoursPlayed
```
(The `hoursPlayed` variable is now declared inside the loop, once per platform.)

- [ ] **Step 8: Remove syncMarkItemIGDBFailed and simplify syncCheckJobCompletion**

Delete the entire `syncMarkItemIGDBFailed` function (lines ~1136–1149).

Replace `syncCheckJobCompletion` with the simplified version (remove the `igdb_failed` branch entirely):

```go
func syncCheckJobCompletion(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx], jobID string) {
	var activeRemaining int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'processing')`,
		jobID,
	).Scan(ctx, &activeRemaining); err != nil {
		slog.Error("syncCheckJobCompletion: count active", "job_id", jobID, "err", err)
		return
	}
	if activeRemaining > 0 {
		return
	}

	var pendingReview int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'pending_review'`,
		jobID,
	).Scan(ctx, &pendingReview); err != nil {
		slog.Error("syncCheckJobCompletion: count pending_review", "job_id", jobID, "err", err)
		return
	}
	if pendingReview > 0 {
		return
	}

	var failedCount int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'failed'`,
		jobID,
	).Scan(ctx, &failedCount); err != nil {
		slog.Error("syncCheckJobCompletion: count failed", "job_id", jobID, "err", err)
		return
	}

	now := time.Now().UTC()
	if failedCount > 0 {
		if _, err := db.NewRaw(
			`UPDATE jobs SET status = 'completed_with_errors', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
			now, jobID,
		).Exec(ctx); err != nil {
			slog.Error("syncCheckJobCompletion: mark completed_with_errors", "job_id", jobID, "err", err)
		}
		return
	}

	if _, err := db.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ? AND status IN ('pending', 'processing')`,
		now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("syncCheckJobCompletion: mark completed", "job_id", jobID, "err", err)
	}
}
```

- [ ] **Step 9: Fix api/sync.go compile errors**

In `internal/api/sync.go`, find `HandleUnskipGame` (around line 920). Replace:
```go
		meta, _ := json.Marshal(map[string]any{
			"external_game_id": eg.ID,
			"playtime_hours":   eg.PlaytimeHours,
		})
		itemID := uuid.NewString()
		if _, err := h.db.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
			itemID, jobID, userID, eg.ExternalID, eg.Title, string(meta),
		).Exec(ctx); err != nil {
```
With:
```go
		itemID := uuid.NewString()
		if _, err := h.db.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())`,
			itemID, jobID, userID, eg.ExternalID, eg.Title, eg.ID,
		).Exec(ctx); err != nil {
```

In `HandleResolveItem` (around line 1154), replace:
```go
	meta, _ := json.Marshal(map[string]any{
		"external_game_id": eg.ID,
		"playtime_hours":   eg.PlaytimeHours,
	})
	itemID := uuid.NewString()
	_, err = h.db.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, eg.ExternalID, eg.Title, string(meta),
	).Exec(ctx)
```
With:
```go
	itemID := uuid.NewString()
	_, err = h.db.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, eg.ExternalID, eg.Title, eg.ID,
	).Exec(ctx)
```

- [ ] **Step 10: Update sync_test.go — fix insertTestExternalGame helper**

Replace `insertTestExternalGame` (around line 2398):
```go
func insertTestExternalGame(t *testing.T, userID, storefront, externalID, title, platform string) string {
	t.Helper()
	egID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, ?, ?, ?, false, true, false)`,
		egID, userID, storefront, externalID, title,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestExternalGame: %v", err)
	}
	_, err = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (?, ?, ?, 0, now())`,
		uuid.NewString(), egID, platform,
	).Exec(context.Background())
	if err != nil {
		t.Fatalf("insertTestExternalGamePlatform: %v", err)
	}
	return egID
}
```

- [ ] **Step 11: Update sync_test.go — fix all inline external_games SQL and metaJSON**

Search for every occurrence of `playtime_hours` in `sync_test.go` and update:

For `external_games` INSERT statements: remove `, playtime_hours` from the column list and remove the corresponding `0` or value from VALUES.

Example — change:
```go
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'steam', '730', 'CS2', true, true, false, 0)`,
		egID, userID,
	)
```
To:
```go
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '730', 'CS2', true, true, false)`,
		egID, userID,
	)
```

For `external_games` inserts with `resolved_igdb_id` — same: drop `playtime_hours` column and its value.

For `job_items` INSERTs that use `source_metadata` with `external_game_id`: switch to `external_game_id` column. Change:
```go
	metaJSON, _ := json.Marshal(map[string]any{"external_game_id": egID, "playtime_hours": 0})
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'CS2', ?, 'pending', '{}', '[]')`,
		itemID, jobID, userID, string(metaJSON),
	)
```
To:
```go
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'CS2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)
```

Apply this pattern to **every** `job_items` INSERT in `sync_test.go` that previously used `source_metadata` to carry `external_game_id`. The affected tests are: `TestProcessSyncItem_SkippedExternalGame`, `TestProcessSyncItem_NoIGDBID_PendingReview`, `TestProcessSyncItem_WithResolvedIGDBID_Completed`, `TestProcessSyncItem_NoPlatforms_Failed`, `TestProcessSyncItem_WithIGDBAutoResolve`, `TestProcessSyncItem_LowConfidenceIGDB_StoresMatchConfidence`, `TestProcessSyncItem_IGDBPrefixTitle_AutoResolves`, `TestProcessSyncItem_ManualResolution_DoesNotRevertToPendingReview`, `TestProcessSyncItem_CrossSKU_InheritsResolutionFromSibling`, `TestProcessSyncItem_PlaytimeHoursWrittenToUserGame`, `TestProcessSyncItem_CancelledJobNotOverwritten`, and all igdb_failed tests.

- [ ] **Step 12: Delete igdb_failed and outdated playtime tests from sync_test.go**

Delete these test functions entirely (they test removed behavior):
- `TestProcessSyncItem_IGDBError_MarksItemIGDBFailed` (tests `igdb_failed` status)
- `TestProcessSyncItem_IGDBError_ThenAutoRetry_CompletesWithErrors` (tests auto-retry)
- `TestProcessSyncItem_IGDBFailed_WithPendingReview_StaysProcessing` (tests auto-retry with pending_review)
- `TestProcessSyncItem_AutoRetry_NilRiverClient_MarksItemFailed` (tests auto-retry edge case)
- `TestProcessSyncItem_PlaytimeHoursWrittenToUserGame` (tests `user_games.hours_played` which no longer exists; the concept is now tested via `UserGameWorker` in Task 5)

- [ ] **Step 13: Verify build and tests pass**

```bash
go build ./...
go test -timeout 600s ./internal/worker/tasks/... -v 2>&1 | tail -40
go test -timeout 600s ./internal/api/... -v 2>&1 | tail -20
```
Expected: build succeeds, all tests pass.

- [ ] **Step 14: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go internal/api/sync.go
git commit -m "$(cat <<'EOF'
fix(sync): fix compile errors; move playtime to external_game_platforms; external_game_id direct column on job_items; simplify syncCheckJobCompletion

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add new DispatchSyncWorker behaviors

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write failing test for failSyncJob cancelling pending items**

Add to `internal/worker/tasks/sync_test.go`:

```go
func TestFailSyncJob_CancelsPendingItems(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 2)`,
		jobID, userID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription) VALUES (?, ?, 'steam', '1', 'Game', false, true, false)`,
		egID, userID,
	)
	// Two pending items, one completed item.
	item1 := uuid.NewString()
	item2 := uuid.NewString()
	item3 := uuid.NewString()
	for _, id := range []string{item1, item2} {
		_, _ = testDB.ExecContext(ctx,
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates) VALUES (?, ?, ?, ?, 'Game', ?, '{}', 'pending', '{}', '[]')`,
			id, jobID, userID, id, egID,
		)
	}
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates) VALUES (?, ?, ?, ?, 'Game', ?, '{}', 'completed', '{}', '[]')`,
		item3, jobID, userID, item3, egID,
	)

	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: &fakeSteamAdapter{ownedErr: fmt.Errorf("network error")}, RiverClient: nil}
	_ = w.Work(ctx, &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	})

	var cancelledCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status = 'cancelled'`, jobID).Scan(ctx, &cancelledCount)
	if cancelledCount != 2 {
		t.Errorf("expected 2 cancelled items, got %d", cancelledCount)
	}
	var completedStatus string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, item3).Scan(ctx, &completedStatus)
	if completedStatus != "completed" {
		t.Errorf("completed item should not be cancelled, got %q", completedStatus)
	}
}
```

- [ ] **Step 2: Write failing test for sync_changes on removed games**

```go
func TestDispatchSync_RemovedGames_WritesSyncChange(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	rawCreds := `{"web_api_key":"k","steam_id":"s"}`
	credsCiphertext, _ := testEncrypter.Encrypt([]byte(rawCreds))
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
		uuid.NewString(), userID, credsCiphertext,
	).Exec(ctx)

	// Pre-seed a game that is NOT returned by the sync (should be marked removed).
	insertTestExternalGame(t, userID, "steam", "999", "Old Game", "pc-windows")

	// Sync returns an empty library — game 999 is gone.
	adapter := &fakeSteamAdapter{games: []steamsvc.OwnedGame{}}
	w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: adapter, RiverClient: nil}
	_ = w.Work(ctx, &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	})

	var changeCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'removed'`, jobID,
	).Scan(ctx, &changeCount)
	if changeCount != 1 {
		t.Errorf("expected 1 removed sync_change, got %d", changeCount)
	}
	var changeTitle string
	_ = testDB.NewRaw(
		`SELECT title FROM sync_changes WHERE job_id = ? AND change_type = 'removed'`, jobID,
	).Scan(ctx, &changeTitle)
	if changeTitle != "Old Game" {
		t.Errorf("sync_change title: want 'Old Game', got %q", changeTitle)
	}
}
```

- [ ] **Step 3: Verify tests fail**

```bash
go test -timeout 600s ./internal/worker/tasks/... -run "TestFailSyncJob_CancelsPendingItems|TestDispatchSync_RemovedGames_WritesSyncChange" -v 2>&1 | tail -20
```
Expected: FAIL (behaviors not yet implemented).

- [ ] **Step 4: Update failSyncJob to cancel pending items**

In `internal/worker/tasks/sync.go`, replace `failSyncJob`:

```go
func failSyncJob(ctx context.Context, db *bun.DB, jobID, msg string) {
	now := time.Now().UTC()
	if _, err := db.NewRaw(
		`UPDATE jobs SET status = 'failed', error_message = ?, completed_at = ? WHERE id = ?`,
		msg, now, jobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: fail job update failed", "err", err, "job_id", jobID)
	}
	if _, err := db.NewRaw(
		`UPDATE job_items SET status = 'cancelled' WHERE job_id = ? AND status = 'pending'`,
		jobID,
	).Exec(ctx); err != nil {
		slog.Error("dispatch_sync: cancel pending items failed", "err", err, "job_id", jobID)
	}
}
```

- [ ] **Step 5: Add sync_changes writes in availability sweep**

In `internal/worker/tasks/sync.go`, find the availability sweep (around line 781). Replace the loop body:

```go
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; !found {
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
				eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: mark game unavailable failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
			}
			if _, err := w.DB.NewRaw(
				`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, ?, 'removed', ?, now())`,
				uuid.NewString(), p.JobID, p.UserID, eg.ID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("dispatch_sync: insert sync_change (removed) failed", "err", err, "job_id", p.JobID, "external_game_id", eg.ID)
			}
		}
	}
```

- [ ] **Step 6: Run tests**

```bash
go test -timeout 600s ./internal/worker/tasks/... -run "TestFailSyncJob_CancelsPendingItems|TestDispatchSync_RemovedGames_WritesSyncChange" -v 2>&1 | tail -20
```
Expected: both tests PASS.

- [ ] **Step 7: Run full test suite**

```bash
go test -timeout 600s ./internal/worker/tasks/... 2>&1 | tail -20
```
Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "$(cat <<'EOF'
feat(sync): cancel pending items on job failure; write sync_changes for removed games

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add IGDBMatchWorker

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

The `IGDBMatchWorker` implements Stage 2: given a `job_item`, it resolves the `external_game.resolved_igdb_id` using sibling inheritance or IGDB search, then enqueues `UserGameArgs`. Transient IGDB errors are returned so River retries; on the final attempt, it falls through to `pending_review`.

- [ ] **Step 1: Write a failing test for IGDBMatchWorker — sibling resolution**

Add to `internal/worker/tasks/sync_test.go`:

```go
func TestIGDBMatchWorker_SiblingResolution(t *testing.T) {
	// A sibling external_game row already has resolved_igdb_id set.
	// IGDBMatchWorker must inherit it and enqueue UserGameArgs without calling IGDB.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'processing', 'normal', 1)`,
		jobID, userID,
	)
	const igdbID = int32(7777)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Sibling Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	// Sibling: same user/storefront/title, different external_id, already resolved.
	siblingID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'psn', 'CUSA001', 'Sibling Game', false, true, false, ?)`,
		siblingID, userID, igdbID,
	)
	// Target: same title, unresolved.
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'psn', 'PPSA001', 'Sibling Game', false, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-5', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	rc := newTestRiverClient(t)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'PPSA001', 'Sibling Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: rc}
	job := &river.Job[tasks.IGDBMatchArgs]{
		Args:        tasks.IGDBMatchArgs{JobItemID: itemID},
		MaxAttempts: 5,
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// external_game must have inherited resolved_igdb_id.
	var resolvedID *int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != igdbID {
		t.Errorf("resolved_igdb_id: want %d, got %v", igdbID, resolvedID)
	}
	// Item must still be pending (UserGameWorker handles completion).
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending" {
		t.Errorf("item status after sibling resolution: want 'pending', got %q", status)
	}
}
```

- [ ] **Step 2: Write a failing test for IGDBMatchWorker — pending_review on no IGDB client**

```go
func TestIGDBMatchWorker_NoIGDBClient_PendingReview(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '111', 'Unknown Game', false, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '111', 'Unknown Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: nil, RiverClient: nil}
	job := &river.Job[tasks.IGDBMatchArgs]{
		Args:        tasks.IGDBMatchArgs{JobItemID: itemID},
		MaxAttempts: 5,
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending_review" {
		t.Errorf("expected pending_review, got %q", status)
	}
}
```

- [ ] **Step 3: Write a failing test for IGDBMatchWorker — auto-resolve above threshold**

```go
func TestIGDBMatchWorker_AutoResolve(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer tokenSrv.Close()

	const igdbID = int32(500)
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": igdbID, "name": "Counter-Strike 2", "slug": "counter-strike-2"},
		})
	}))
	defer igdbSrv.Close()

	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false)`,
		egID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	rc := newTestRiverClient(t)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	igdbClient := newIGDBClientForTests(t, tokenSrv.URL, igdbSrv.URL)
	w := &tasks.IGDBMatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc}
	job := &river.Job[tasks.IGDBMatchArgs]{
		Args:        tasks.IGDBMatchArgs{JobItemID: itemID},
		MaxAttempts: 5,
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resolvedID *int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = ?`, egID).Scan(ctx, &resolvedID)
	if resolvedID == nil || *resolvedID != igdbID {
		t.Errorf("resolved_igdb_id: want %d, got %v", igdbID, resolvedID)
	}
	// games row must exist.
	var gameTitle string
	_ = testDB.NewRaw(`SELECT title FROM games WHERE id = ?`, igdbID).Scan(ctx, &gameTitle)
	if gameTitle == "" {
		t.Error("games row should have been inserted by IGDBMatchWorker")
	}
	// Item stays pending (UserGameWorker handles the transition).
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "pending" {
		t.Errorf("item status: want 'pending', got %q (UserGameWorker transitions it)", status)
	}
}
```

- [ ] **Step 4: Verify tests fail**

```bash
go test -timeout 600s ./internal/worker/tasks/... -run "TestIGDBMatchWorker" -v 2>&1 | tail -10
```
Expected: FAIL — `tasks.IGDBMatchWorker` and `tasks.IGDBMatchArgs` undefined.

- [ ] **Step 5: Implement IGDBMatchWorker in sync.go**

Add after the `ownershipRank` function (and before `ProcessSyncItemWorker`):

```go
// ── IGDBMatchWorker — Stage 2 ─────────────────────────────────────────────────

// IGDBMatchArgs is the River job args type for "igdb_match".
type IGDBMatchArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (IGDBMatchArgs) Kind() string { return "igdb_match" }

func (IGDBMatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

// IGDBMatchWorker resolves a single sync item's IGDB ID, then enqueues UserGameArgs.
// Transient IGDB errors are returned so River retries with exponential backoff.
// On the final attempt, the item falls through to pending_review instead of failing.
type IGDBMatchWorker struct {
	river.WorkerDefaults[IGDBMatchArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
}

func (w *IGDBMatchWorker) Work(ctx context.Context, job *river.Job[IGDBMatchArgs]) error {
	p := job.Args

	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", p.JobItemID).Scan(ctx); err != nil {
		slog.Error("igdb_match: load job_item", "id", p.JobItemID, "err", err)
		return err
	}

	if item.ExternalGameID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// Fast-path: skipped games go straight to UserGameWorker.
	if eg.IsSkipped {
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Fast-path: already resolved (manual or prior run).
	if eg.ResolvedIGDBID != nil {
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// Sibling check: same user/storefront/title already resolved by another SKU.
	var sibling models.ExternalGame
	if err := w.DB.NewSelect().Model(&sibling).
		Where("user_id = ? AND storefront = ? AND title = ? AND id != ? AND resolved_igdb_id IS NOT NULL",
			eg.UserID, eg.Storefront, eg.Title, eg.ID).
		Limit(1).
		Scan(ctx); err == nil && sibling.ResolvedIGDBID != nil {
		igdbID := *sibling.ResolvedIGDBID
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("igdb_match: insert game row (sibling)", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("igdb_match: apply sibling resolution", "err", err, "external_game_id", eg.ID)
		}
		return w.enqueueUserGame(ctx, item.ID, item.JobID)
	}

	// IGDB search.
	if w.IGDBClient != nil && w.IGDBClient.Configured() {
		candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10)
		if err != nil {
			// On final attempt, fall through to pending_review instead of erroring.
			if job.Attempt >= job.MaxAttempts {
				slog.Warn("igdb_match: IGDB failed on final attempt, marking pending_review",
					"item_id", p.JobItemID, "err", err)
				syncMarkItemPendingReview(ctx, w.DB, &item)
				syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
				return nil
			}
			return fmt.Errorf("igdb_match: search failed (will retry): %w", err)
		}

		normalizedQuery := matching.NormalizeTitle(eg.Title)
		var bestScore, secondBestScore float64
		var bestID int32
		for _, c := range candidates {
			score := matching.FuzzyConfidence(normalizedQuery, matching.NormalizeTitle(c.Title))
			if score > bestScore {
				secondBestScore = bestScore
				bestScore = score
				bestID = int32(c.IgdbID)
			} else if score > secondBestScore {
				secondBestScore = score
			}
		}

		const autoResolveThreshold = 0.85
		const tieEpsilon = 0.01
		if bestScore >= autoResolveThreshold && (bestScore-secondBestScore) > tieEpsilon {
			if _, err := w.DB.NewRaw(
				`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
				bestID, eg.Title,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: insert game row (auto-resolve)", "err", err, "igdb_id", bestID)
			}
			if _, err := w.DB.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				bestID, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("igdb_match: auto-resolve external_game", "err", err, "external_game_id", eg.ID)
			}
			return w.enqueueUserGame(ctx, item.ID, item.JobID)
		}

		// Low confidence — store candidates, mark pending_review.
		candidatesJSON, _ := json.Marshal(candidates)
		item.IGDBCandidates = candidatesJSON
		item.MatchConfidence = &bestScore
		syncMarkItemPendingReview(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// No IGDB client configured — mark pending_review.
	syncMarkItemPendingReview(ctx, w.DB, &item)
	syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
	return nil
}

func (w *IGDBMatchWorker) enqueueUserGame(ctx context.Context, jobItemID, jobID string) error {
	if err := EnqueueOrFail(ctx, w.DB, w.RiverClient, jobItemID, UserGameArgs{JobItemID: jobItemID}); err != nil {
		slog.Error("igdb_match: enqueue user_game_write failed", "item_id", jobItemID, "err", err)
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, jobID)
	}
	return nil
}
```

Also add the `UserGameArgs` stub (needed by IGDBMatchWorker above, full implementation in Task 5):

```go
// ── UserGameWorker — Stage 3 ──────────────────────────────────────────────────

// UserGameArgs is the River job args type for "user_game_write".
type UserGameArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (UserGameArgs) Kind() string { return "user_game_write" }

func (UserGameArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}
```

- [ ] **Step 6: Run the new tests**

```bash
go test -timeout 600s ./internal/worker/tasks/... -run "TestIGDBMatchWorker" -v 2>&1 | tail -20
```
Expected: all three IGDBMatchWorker tests PASS.

- [ ] **Step 7: Run full test suite**

```bash
go test -timeout 600s ./internal/worker/tasks/... 2>&1 | tail -10
```
Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "$(cat <<'EOF'
feat(sync): add IGDBMatchWorker (Stage 2) with sibling resolution, auto-resolve, and River retry

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add UserGameWorker

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

The `UserGameWorker` implements Stage 3: upserts `user_games` and `user_game_platforms` with ownership rank guard, writes `sync_changes` (added, status_changed), and marks the item completed.

- [ ] **Step 1: Write a failing test for UserGameWorker — creates user_game and writes sync_change**

Add to `internal/worker/tasks/sync_test.go`:

```go
func TestUserGameWorker_CreatesUserGameAndSyncChange(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(730)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Counter-Strike 2', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	egID := uuid.NewString()
	igdbIDVal := igdbID
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '730', 'Counter-Strike 2', false, true, false, ?)`,
		egID, userID, igdbIDVal,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 42.5, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '730', 'Counter-Strike 2', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	job := &river.Job[tasks.UserGameArgs]{
		Args: tasks.UserGameArgs{JobItemID: itemID},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// user_games row must exist.
	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ? AND game_id = ?`, userID, igdbID).Scan(ctx, &ugCount)
	if ugCount != 1 {
		t.Errorf("expected 1 user_game, got %d", ugCount)
	}
	// user_game_platforms row with hours_played=42.5.
	var ugpHours *float64
	_ = testDB.NewRaw(
		`SELECT ugp.hours_played FROM user_game_platforms ugp
		 JOIN user_games ug ON ug.id = ugp.user_game_id
		 WHERE ug.user_id = ? AND ug.game_id = ? AND ugp.platform = 'pc-windows' AND ugp.storefront = 'steam'`,
		userID, igdbID,
	).Scan(ctx, &ugpHours)
	if ugpHours == nil || *ugpHours != 42.5 {
		t.Errorf("hours_played: want 42.5, got %v", ugpHours)
	}
	// sync_changes: added.
	var changeCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'added'`, jobID,
	).Scan(ctx, &changeCount)
	if changeCount != 1 {
		t.Errorf("expected 1 added sync_change, got %d", changeCount)
	}
	// item status: completed.
	var status string
	_ = testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &status)
	if status != "completed" {
		t.Errorf("item status: want 'completed', got %q", status)
	}
}
```

- [ ] **Step 2: Write a failing test for UserGameWorker — ownership rank guard**

```go
func TestUserGameWorker_OwnershipRankGuard(t *testing.T) {
	// existing UGP has ownership=owned (rank 4). New sync says subscription (rank 2).
	// The existing ownership must NOT be downgraded.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(800)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'PSN Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	ownership := "owned"
	existingHours := 10.0
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'playstation-4', 'psn', true, ?, ?, true, now(), now())`,
		uuid.NewString(), ugID, existingHours, ownership,
	)
	egID := uuid.NewString()
	isSubTrue := true
	subOwnership := "subscription"
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'psn', 'CUSA800', 'PSN Game', false, true, true, ?, ?)`,
		egID, userID, subOwnership, igdbID,
	)
	_ = isSubTrue
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'playstation-4', 20.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'CUSA800', 'PSN Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ownership must not be downgraded from 'owned' to 'subscription'.
	var resultOwnership string
	_ = testDB.NewRaw(
		`SELECT ownership_status FROM user_game_platforms WHERE user_game_id = ? AND platform = 'playstation-4' AND storefront = 'psn'`,
		ugID,
	).Scan(ctx, &resultOwnership)
	if resultOwnership != "owned" {
		t.Errorf("ownership should not be downgraded: want 'owned', got %q", resultOwnership)
	}
	// hours_played should be updated to the higher value (20.0 > 10.0).
	var resultHours float64
	_ = testDB.NewRaw(
		`SELECT hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = 'playstation-4' AND storefront = 'psn'`,
		ugID,
	).Scan(ctx, &resultHours)
	if resultHours != 20.0 {
		t.Errorf("hours_played should update to higher value: want 20.0, got %v", resultHours)
	}
	// No sync_change: game already existed (not a new addition).
	var changeCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM sync_changes WHERE job_id = ? AND change_type = 'added'`, jobID).Scan(ctx, &changeCount)
	if changeCount != 0 {
		t.Errorf("expected 0 added sync_changes (game pre-existed), got %d", changeCount)
	}
}
```

- [ ] **Step 3: Write a failing test for UserGameWorker — status_changed sync_change**

```go
func TestUserGameWorker_StatusChangedSyncChange(t *testing.T) {
	// Existing UGP has subscription; new sync says owned (upgrade).
	// Expect a status_changed sync_change.
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(900)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Status Change Game', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', 'steam', true, 0, 'subscription', true, now(), now())`,
		uuid.NewString(), ugID,
	)
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '900', 'Status Change Game', false, true, false, 'owned', ?)`,
		egID, userID, igdbID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 5.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '900', 'Status Change Game', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sc struct {
		ChangeType string  `bun:"change_type"`
		OldStatus  *string `bun:"old_status"`
		NewStatus  *string `bun:"new_status"`
	}
	_ = testDB.NewRaw(
		`SELECT change_type, old_status, new_status FROM sync_changes WHERE job_id = ?`, jobID,
	).Scan(ctx, &sc)
	if sc.ChangeType != "status_changed" {
		t.Errorf("change_type: want 'status_changed', got %q", sc.ChangeType)
	}
	if sc.OldStatus == nil || *sc.OldStatus != "subscription" {
		t.Errorf("old_status: want 'subscription', got %v", sc.OldStatus)
	}
	if sc.NewStatus == nil || *sc.NewStatus != "owned" {
		t.Errorf("new_status: want 'owned', got %v", sc.NewStatus)
	}
}
```

- [ ] **Step 4: Verify tests fail**

```bash
go test -timeout 600s ./internal/worker/tasks/... -run "TestUserGameWorker" -v 2>&1 | tail -10
```
Expected: FAIL — `tasks.UserGameWorker` undefined (only stub args defined so far).

- [ ] **Step 5: Implement UserGameWorker in sync.go**

Replace the `UserGameArgs` stub (added in Task 4) with the full implementation. The struct declaration and InsertOpts remain; add the `UserGameWorker` struct and `Work` method:

```go
// UserGameWorker writes the user_game and user_game_platform rows for a resolved sync item.
type UserGameWorker struct {
	river.WorkerDefaults[UserGameArgs]
	DB          *bun.DB
	RiverClient *river.Client[pgx.Tx]
}

func (w *UserGameWorker) Work(ctx context.Context, job *river.Job[UserGameArgs]) error {
	p := job.Args

	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", p.JobItemID).Scan(ctx); err != nil {
		slog.Error("user_game_write: load job_item", "id", p.JobItemID, "err", err)
		return err
	}

	if item.ExternalGameID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external_game_id not set on job_item")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	var eg models.ExternalGame
	if err := w.DB.NewSelect().Model(&eg).Where("id = ?", *item.ExternalGameID).Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, "external game not found")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// Skipped games: update updated_at, mark skipped, check completion.
	if eg.IsSkipped {
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET updated_at = now() WHERE id = ?`, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: update external_game updated_at (skipped)", "err", err)
		}
		syncMarkItemSkipped(ctx, w.DB, &item)
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// Manual resolution propagation: job_item has resolved_igdb_id but external_game doesn't.
	if eg.ResolvedIGDBID == nil && item.ResolvedIGDBID != nil {
		igdbID := int32(*item.ResolvedIGDBID)
		eg.ResolvedIGDBID = &igdbID
		if _, err := w.DB.NewRaw(
			`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
			igdbID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: insert game row (manual resolve)", "err", err, "igdb_id", igdbID)
		}
		if _, err := w.DB.NewRaw(
			`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
			igdbID, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: apply manual resolution", "err", err, "external_game_id", eg.ID)
		}
	}

	if eg.ResolvedIGDBID == nil {
		syncMarkItemFailed(ctx, w.DB, &item, "no resolved_igdb_id on external_game")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	// Ensure games row exists.
	if _, err := w.DB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		*eg.ResolvedIGDBID, eg.Title,
	).Exec(ctx); err != nil {
		slog.Error("user_game_write: ensure game row", "err", err)
	}

	// Upsert user_games.
	ugID := uuid.NewString()
	now := time.Now().UTC()
	var isNewRow struct {
		ID    string `bun:"id"`
		IsNew bool   `bun:"is_new"`
	}
	if err := w.DB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now()
		 RETURNING id, (xmax = 0) AS is_new`,
		ugID, item.UserID, *eg.ResolvedIGDBID, now, now,
	).Scan(ctx, &isNewRow); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("upsert user_game: %v", err))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}
	ugID = isNewRow.ID
	if isNewRow.IsNew {
		if _, err := w.DB.NewRaw(
			`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
			 VALUES (?, ?, ?, ?, 'added', ?, now())`,
			uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
		).Exec(ctx); err != nil {
			slog.Error("user_game_write: insert sync_change (added)", "err", err)
		}
	}

	// Load platform rows.
	var egPlatforms []models.ExternalGamePlatform
	if err := w.DB.NewSelect().Model(&egPlatforms).
		Where("external_game_id = ?", eg.ID).
		Scan(ctx); err != nil {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("load platforms: %v", err))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}
	if len(egPlatforms) == 0 {
		syncMarkItemFailed(ctx, w.DB, &item, "external game has no platform rows")
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	storefrontSlug, ok := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !ok {
		syncMarkItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved storefront=%s", eg.Storefront))
		syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
		return nil
	}

	ownership := "owned"
	if eg.OwnershipStatus != nil {
		ownership = *eg.OwnershipStatus
	} else if eg.IsSubscription {
		ownership = "subscription"
	}

	for _, egp := range egPlatforms {
		var existingID string
		var existingOwnership *string
		var existingHours *float64
		err := w.DB.NewRaw(
			`SELECT id, ownership_status, hours_played FROM user_game_platforms WHERE user_game_id = ? AND platform = ? AND storefront = ?`,
			ugID, egp.Platform, storefrontSlug,
		).Scan(ctx, &existingID, &existingOwnership, &existingHours)

		if errors.Is(err, sql.ErrNoRows) || err != nil {
			ugpID := uuid.NewString()
			if _, err := w.DB.NewRaw(`
				INSERT INTO user_game_platforms
				(id, user_game_id, platform, storefront, is_available, hours_played, ownership_status,
				 original_platform_name, original_storefront_name, external_game_id, sync_from_source, created_at, updated_at)
				VALUES (?, ?, ?, ?, true, ?, ?, ?, ?, ?, true, now(), now())
				ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
				ugpID, ugID, egp.Platform, storefrontSlug, egp.HoursPlayed, ownership,
				egp.Platform, eg.Storefront, eg.ID,
			).Exec(ctx); err != nil {
				slog.Error("user_game_write: insert user_game_platform", "err", err, "item_id", p.JobItemID)
			}
		} else {
			existingRank := 0
			if existingOwnership != nil {
				existingRank = ownershipRank(*existingOwnership)
			}
			newRank := ownershipRank(ownership)

			if newRank > existingRank {
				// Ownership upgrade — write status_changed sync_change.
				if _, err := w.DB.NewRaw(
					`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, old_status, new_status, created_at)
					 VALUES (?, ?, ?, ?, 'status_changed', ?, ?, ?, now())`,
					uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title, existingOwnership, &ownership,
				).Exec(ctx); err != nil {
					slog.Error("user_game_write: insert sync_change (status_changed)", "err", err)
				}
				newHours := egp.HoursPlayed
				if existingHours != nil && *existingHours > egp.HoursPlayed {
					newHours = *existingHours
				}
				if _, err := w.DB.NewRaw(
					`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, updated_at = now() WHERE id = ?`,
					ownership, newHours, existingID,
				).Exec(ctx); err != nil {
					slog.Error("user_game_write: update ugp ownership", "err", err, "item_id", p.JobItemID)
				}
			} else {
				// No ownership change — only update hours if higher.
				if egp.HoursPlayed > 0 && (existingHours == nil || egp.HoursPlayed > *existingHours) {
					if _, err := w.DB.NewRaw(
						`UPDATE user_game_platforms SET hours_played = ?, updated_at = now() WHERE id = ?`,
						egp.HoursPlayed, existingID,
					).Exec(ctx); err != nil {
						slog.Error("user_game_write: update ugp hours", "err", err, "item_id", p.JobItemID)
					}
				}
			}
		}
	}

	if _, err := w.DB.NewRaw(
		`UPDATE external_games SET updated_at = now() WHERE id = ?`, eg.ID,
	).Exec(ctx); err != nil {
		slog.Error("user_game_write: update external_game updated_at", "err", err)
	}

	syncMarkItemCompleted(ctx, w.DB, &item)
	syncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
	return nil
}
```

- [ ] **Step 6: Run the new tests**

```bash
go test -timeout 600s ./internal/worker/tasks/... -run "TestUserGameWorker" -v 2>&1 | tail -20
```
Expected: all three UserGameWorker tests PASS.

- [ ] **Step 7: Run full test suite**

```bash
go test -timeout 600s ./internal/worker/tasks/... 2>&1 | tail -10
```
Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "$(cat <<'EOF'
feat(sync): add UserGameWorker (Stage 3) with ownership rank guard and sync_changes writes

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Replace ProcessSyncItemWorker and update all call sites

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`
- Modify: `internal/worker/tasks/enqueue.go`
- Modify: `internal/api/sync.go`
- Modify: `internal/scheduler/orphaned_items.go`
- Modify: `cmd/nexorious/serve.go`

- [ ] **Step 1: Delete ProcessSyncItemWorker from sync.go**

Remove the entire `ProcessSyncItemArgs` type, `ProcessSyncItemWorker` struct, and `ProcessSyncItemWorker.Work` method from `internal/worker/tasks/sync.go`.

Also update the DispatchSyncWorker to enqueue `IGDBMatchArgs` instead of `ProcessSyncItemArgs` in all four places:
1. Steam deferred enqueue loop (~line 372): change `ProcessSyncItemArgs{JobItemID: item.ID}` → `IGDBMatchArgs{JobItemID: item.ID}`
2. PSN batch enqueue (~line 478): `ProcessSyncItemArgs{JobItemID: itemID}` → `IGDBMatchArgs{JobItemID: itemID}`
3. Epic batch enqueue (~line 595): same
4. GOG batch enqueue (~line 745): same

Also remove any remaining `json` import if it's now unused (check; it's likely still used elsewhere).

- [ ] **Step 2: Delete ProcessSyncItem tests from sync_test.go**

Delete all `TestProcessSyncItem_*` test functions from `sync_test.go`. These are now fully replaced by `TestIGDBMatchWorker_*` and `TestUserGameWorker_*` from Tasks 4 and 5.

The test functions to delete:
- `TestProcessSyncItem_ItemNotFound`
- `TestProcessSyncItem_SkippedExternalGame`
- `TestProcessSyncItem_NoIGDBID_PendingReview`
- `TestProcessSyncItem_WithResolvedIGDBID_Completed`
- `TestProcessSyncItem_NoPlatforms_Failed`
- `TestProcessSyncItem_WithIGDBAutoResolve`
- `TestProcessSyncItem_LowConfidenceIGDB_StoresMatchConfidence`
- `TestProcessSyncItem_IGDBPrefixTitle_AutoResolves`
- `TestProcessSyncItem_ManualResolution_DoesNotRevertToPendingReview`
- `TestProcessSyncItem_CrossSKU_InheritsResolutionFromSibling`
- `TestProcessSyncItem_CancelledJobNotOverwritten`

(The igdb_failed and playtime tests were already deleted in Task 2.)

- [ ] **Step 3: Update enqueue.go ArgsForJobType**

In `internal/worker/tasks/enqueue.go`, change:
```go
	case models.JobTypeSync:
		return ProcessSyncItemArgs{JobItemID: jobItemID}, nil
```
To:
```go
	case models.JobTypeSync:
		return IGDBMatchArgs{JobItemID: jobItemID}, nil
```

- [ ] **Step 4: Update api/sync.go HandleUnskipGame**

In `internal/api/sync.go`, find `HandleUnskipGame` (around line 936). Change:
```go
		if _, err := h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
```
To:
```go
		if _, err := h.riverClient.Insert(ctx, tasks.IGDBMatchArgs{JobItemID: itemID}, nil); err != nil {
```

- [ ] **Step 5: Update api/sync.go HandleResolveItem**

In `HandleResolveItem` (around line 1169), change:
```go
		if _, err = h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil); err != nil {
```
To (UserGameArgs — the external_game already has resolved_igdb_id set by this handler):
```go
		if _, err = h.riverClient.Insert(ctx, tasks.UserGameArgs{JobItemID: itemID}, nil); err != nil {
```

- [ ] **Step 6: Update orphaned_items.go**

In `internal/scheduler/orphaned_items.go`, change:
```go
		case "sync":
			args = tasks.ProcessSyncItemArgs{JobItemID: o.ID}
```
To:
```go
		case "sync":
			args = tasks.IGDBMatchArgs{JobItemID: o.ID}
```

- [ ] **Step 7: Update serve.go — initial setup block**

In `cmd/nexorious/serve.go`, find the initial worker setup (around line 186). Replace:
```go
	processSyncItemWorker := &tasks.ProcessSyncItemWorker{DB: db, IGDBClient: igdbClient}
```
With:
```go
	igdbMatchWorker := &tasks.IGDBMatchWorker{DB: db, IGDBClient: igdbClient}
	userGameWorker := &tasks.UserGameWorker{DB: db}
```

Replace the `AddWorker` call:
```go
	river.AddWorker(workers, processSyncItemWorker)
```
With:
```go
	river.AddWorker(workers, igdbMatchWorker)
	river.AddWorker(workers, userGameWorker)
```

Replace the RiverClient wiring:
```go
	processSyncItemWorker.RiverClient = riverClient
```
With:
```go
	igdbMatchWorker.RiverClient = riverClient
	userGameWorker.RiverClient = riverClient
```

- [ ] **Step 8: Update serve.go — hot-reload (RebuildServices) block**

In `cmd/nexorious/serve.go`, find the `RebuildServices` closure (around line 264). Replace:
```go
			newProcessSyncItem := &tasks.ProcessSyncItemWorker{DB: newDB, IGDBClient: igdbClient}
```
With:
```go
			newIGDBMatch := &tasks.IGDBMatchWorker{DB: newDB, IGDBClient: igdbClient}
			newUserGame := &tasks.UserGameWorker{DB: newDB}
```

Replace:
```go
			river.AddWorker(newWorkers, newProcessSyncItem)
```
With:
```go
			river.AddWorker(newWorkers, newIGDBMatch)
			river.AddWorker(newWorkers, newUserGame)
```

Replace:
```go
			newProcessSyncItem.RiverClient = newClient
```
With:
```go
			newIGDBMatch.RiverClient = newClient
			newUserGame.RiverClient = newClient
```

- [ ] **Step 9: Build and run full test suite**

```bash
go build ./...
go test -timeout 600s ./... 2>&1 | tail -20
```
Expected: build succeeds, all tests pass.

- [ ] **Step 10: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go \
        internal/worker/tasks/enqueue.go internal/api/sync.go \
        internal/scheduler/orphaned_items.go cmd/nexorious/serve.go
git commit -m "$(cat <<'EOF'
refactor(sync): replace ProcessSyncItemWorker with IGDBMatchWorker + UserGameWorker; update all call sites

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```
