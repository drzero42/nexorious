# PSN Sync Batch Dispatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Switch PSN sync from a single all-at-once library fetch to incremental page-by-page dispatch so job items appear in the progress box after the first batch (~0.5–1 s) rather than after the entire library is fetched.

**Architecture:** Change `PSNLibraryAdapter.GetLibrary` to a callback-based interface; `psn.Client` auths once then loops `GetTrophyTitles` with `limit=batchSize, offset`, calling `onBatch` per page. `DispatchSyncWorker` upserts and dispatches each batch inline before fetching the next. Steps 5 (mark unavailable) and 7 (update `last_synced_at`) run after the loop. Steam path is untouched.

**Tech Stack:** Go 1.25, River (`riverqueue/river`), Bun ORM (`uptrace/bun`), go-psn-api SDK (`sizovilya/go-psn-api`), testcontainers-go.

---

## File Map

| File | Change |
|---|---|
| `internal/worker/tasks/sync.go` | Change `PSNLibraryAdapter` interface, add constant, rewrite `DispatchSyncWorker.Work` |
| `internal/services/psn/client.go` | Rewrite `GetLibrary` to paginate with callback |
| `internal/worker/tasks/sync_test.go` | Add `fakePSNAdapter` + PSN dispatch tests (new import: psn package) |

No migration. No frontend changes. `cmd/nexorious/serve.go` is untouched — `psnsvc.NewClient()` satisfies the new interface once Task 2 is done.

---

## Task 1: Update PSNLibraryAdapter interface + add fakePSNAdapter

**Files:**
- Modify: `internal/worker/tasks/sync.go` (interface + constant)
- Modify: `internal/worker/tasks/sync_test.go` (add import + fakePSNAdapter)

After this task `go build ./...` will fail because `psn.Client.GetLibrary` no longer matches — that is expected and fixed in Task 2.

- [ ] **Step 1: Change the interface and add the constant in sync.go**

In `internal/worker/tasks/sync.go`, replace the existing `PSNLibraryAdapter` interface and add the constant immediately after it:

```go
// PSNLibraryAdapter fetches the PSN game library.
type PSNLibraryAdapter interface {
	GetLibrary(ctx context.Context, npssoToken string, batchSize int, onBatch func([]psnsvc.ExternalLibraryEntry) error) error
}

const psnLibraryBatchSize = 10
```

- [ ] **Step 2: Add psnsvc import + fakePSNAdapter to sync_test.go**

Add `psnsvc "github.com/drzero42/nexorious-go/internal/services/psn"` to the import block at the top of `internal/worker/tasks/sync_test.go`.

Then add these declarations directly below the existing `fakeSteamAdapter` block (around line 35):

```go
// fakePSNAdapter implements PSNLibraryAdapter for testing.
type fakePSNAdapter struct {
	pages [][]psnsvc.ExternalLibraryEntry // each inner slice is one batch/page
	err   error                            // if non-nil, returned by GetLibrary
}

func (f *fakePSNAdapter) GetLibrary(_ context.Context, _ string, _ int, onBatch func([]psnsvc.ExternalLibraryEntry) error) error {
	if f.err != nil {
		return f.err
	}
	for _, page := range f.pages {
		if err := onBatch(page); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 3: Verify expected compile failure**

```bash
go build ./...
```

Expected: error mentioning `psn.Client` does not implement `PSNLibraryAdapter` (because `GetLibrary` signature changed). This is expected.

---

## Task 2: Rewrite psn.Client.GetLibrary with pagination

**Files:**
- Modify: `internal/services/psn/client.go`

- [ ] **Step 1: Replace the GetLibrary method body**

In `internal/services/psn/client.go`, replace the entire `GetLibrary` method (lines 133–178) with:

```go
// GetLibrary fetches the PSN trophy title list as a proxy for the user's game library.
// Auth is performed once; pages of batchSize titles are fetched in a loop.
// onBatch is called for each page and may return an error to abort the loop.
// GetTrophyTitles failures after auth succeed return nil (same behaviour as before).
func (c *Client) GetLibrary(ctx context.Context, npssoToken string, batchSize int, onBatch func([]ExternalLibraryEntry) error) error {
	psnClient, err := psnsdk.NewClient(&psnsdk.Options{
		Lang:   "en",
		Region: "us",
		Npsso:  npssoToken,
	})
	if err != nil {
		return fmt.Errorf("psn: failed to create client: %w", err)
	}

	if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
		return ErrInvalidNPSSOToken
	}

	platformMap := map[string]string{
		"PS3":    "playstation-3",
		"PS4":    "playstation-4",
		"PS5":    "playstation-5",
		"PSVITA": "ps-vita",
	}

	for offset := 0; ; offset += batchSize {
		resp, err := psnClient.GetTrophyTitles(ctx, "me", batchSize, offset)
		if err != nil {
			// Trophy titles may not be accessible; surface what we have rather than blocking.
			return nil
		}
		if len(resp.TrophyTitles) == 0 {
			break
		}

		entries := make([]ExternalLibraryEntry, 0, len(resp.TrophyTitles))
		for _, t := range resp.TrophyTitles {
			rawPlatform := platformMap[t.TrophyTitlePlatfrom]
			if rawPlatform == "" {
				rawPlatform = "playstation-4"
			}
			entries = append(entries, ExternalLibraryEntry{
				ExternalID:      t.NpCommunicationID,
				Title:           t.TrophyTitleName,
				RawPlatform:     rawPlatform,
				PlaytimeHours:   0,
				OwnershipStatus: "owned",
				IsSubscription:  false,
			})
		}

		if err := onBatch(entries); err != nil {
			return err
		}

		if len(resp.TrophyTitles) < batchSize {
			break
		}
	}

	return nil
}
```

- [ ] **Step 2: Verify compilation is restored**

```bash
go build ./...
```

Expected: success (no errors).

- [ ] **Step 3: Run existing tests to confirm nothing regressed**

```bash
go test ./internal/worker/tasks/... -timeout 300s -v 2>&1 | head -60
```

Expected: all existing Steam dispatch tests and ProcessSyncItem tests pass. PSN-specific tests don't exist yet.

- [ ] **Step 4: Commit**

```bash
git add internal/services/psn/client.go internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "$(cat <<'EOF'
feat(psn): change PSNLibraryAdapter to callback-based paginated interface

Replaces the single-call GetLibrary return-slice signature with an
onBatch callback so DispatchSyncWorker can dispatch job items per page
rather than after fetching the full library.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Rewrite DispatchSyncWorker.Work

**Files:**
- Modify: `internal/worker/tasks/sync.go` (the `Work` method only; helpers unchanged)

The refactor moves steps 4+6 (upsert + dispatch) into the switch cases, declares `fetchedIDs` before the switch so both branches contribute to it, and keeps steps 5+7 (mark unavailable + `last_synced_at`) after the switch as shared code.

- [ ] **Step 1: Replace the entire Work method**

In `internal/worker/tasks/sync.go`, replace `func (w *DispatchSyncWorker) Work(...)` with the following. The helper functions below `Work` (`failSyncJob`, `ownershipRank`, etc.) are untouched.

```go
func (w *DispatchSyncWorker) Work(ctx context.Context, job *river.Job[DispatchSyncArgs]) error {
	p := job.Args

	// ── 1. Mark job as processing ─────────────────────────────────────────
	now := time.Now().UTC()
	_, _ = w.DB.NewRaw(
		`UPDATE jobs SET status = 'processing', started_at = ? WHERE id = ?`,
		now, p.JobID,
	).Exec(ctx)

	// ── 2. Read sync credentials ──────────────────────────────────────────
	var cfg models.UserSyncConfig
	if err := w.DB.NewSelect().Model(&cfg).
		Where("user_id = ? AND storefront = ?", p.UserID, p.Storefront).
		Scan(ctx); err != nil {
		failSyncJob(ctx, w.DB, p.JobID, "no sync config found")
		return nil
	}
	if cfg.StorefrontCredentials == nil {
		failSyncJob(ctx, w.DB, p.JobID, "credentials not configured")
		return nil
	}

	// fetchedIDs accumulates all external IDs seen in the fetch; used by step 5.
	fetchedIDs := make(map[string]struct{})

	// ── 3+4+6. Fetch library, upsert external_games, dispatch items ───────
	switch p.Storefront {
	case "steam":
		var creds struct {
			WebAPIKey string `json:"web_api_key"`
			SteamID   string `json:"steam_id"`
		}
		if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &creds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid steam credentials")
			return nil
		}
		raw, err := w.Steam.GetOwnedGames(ctx, creds.WebAPIKey, creds.SteamID)
		if err != nil {
			failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("fetch steam library: %v", err))
			return nil
		}

		rawPlatformByExtID := make(map[string]string, len(raw))
		for _, e := range raw {
			fetchedIDs[e.ExternalID] = struct{}{}
			rawPlatformByExtID[e.ExternalID] = e.RawPlatform
			ownership := e.OwnershipStatus
			upsertNow := time.Now().UTC()
			row := &models.ExternalGame{
				ID:              uuid.NewString(),
				UserID:          p.UserID,
				Storefront:      p.Storefront,
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				IsAvailable:     true,
				IsSubscription:  e.IsSubscription,
				PlaytimeHours:   e.PlaytimeHours,
				OwnershipStatus: &ownership,
				RawPlatform:     e.RawPlatform,
				CreatedAt:       upsertNow,
				UpdatedAt:       upsertNow,
			}
			_, _ = w.DB.NewInsert().Model(row).
				On("CONFLICT (user_id, storefront, external_id) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, raw_platform = EXCLUDED.raw_platform, is_available = true, updated_at = now()").
				Exec(ctx)
		}

		var toProcess []models.ExternalGame
		_ = w.DB.NewSelect().Model(&toProcess).
			Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false", p.UserID, p.Storefront).
			Scan(ctx)
		for _, eg := range toProcess {
			meta := map[string]string{
				"external_game_id": eg.ID,
				"raw_platform":     rawPlatformByExtID[eg.ExternalID],
			}
			metaJSON, _ := json.Marshal(meta)
			itemID := uuid.NewString()
			_, _ = w.DB.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
				 ON CONFLICT (job_id, item_key) DO NOTHING`,
				itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
			).Exec(ctx)
			if w.RiverClient != nil {
				_, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
			}
		}

	case "psn":
		var psnCreds struct {
			NpssoToken string `json:"npsso_token"`
			IsVerified bool   `json:"is_verified"`
		}
		if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &psnCreds); err != nil {
			failSyncJob(ctx, w.DB, p.JobID, "invalid psn credentials")
			return nil
		}
		if !psnCreds.IsVerified {
			failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			return nil
		}

		if err := w.PSN.GetLibrary(ctx, psnCreds.NpssoToken, psnLibraryBatchSize,
			func(batch []psnsvc.ExternalLibraryEntry) error {
				rawPlatformByExtID := make(map[string]string, len(batch))
				batchExtIDs := make([]string, 0, len(batch))
				for _, e := range batch {
					fetchedIDs[e.ExternalID] = struct{}{}
					batchExtIDs = append(batchExtIDs, e.ExternalID)
					rawPlatformByExtID[e.ExternalID] = e.RawPlatform
					ownership := e.OwnershipStatus
					upsertNow := time.Now().UTC()
					row := &models.ExternalGame{
						ID:              uuid.NewString(),
						UserID:          p.UserID,
						Storefront:      p.Storefront,
						ExternalID:      e.ExternalID,
						Title:           e.Title,
						IsAvailable:     true,
						IsSubscription:  e.IsSubscription,
						PlaytimeHours:   e.PlaytimeHours,
						OwnershipStatus: &ownership,
						RawPlatform:     e.RawPlatform,
						CreatedAt:       upsertNow,
						UpdatedAt:       upsertNow,
					}
					_, _ = w.DB.NewInsert().Model(row).
						On("CONFLICT (user_id, storefront, external_id) DO UPDATE SET title = EXCLUDED.title, playtime_hours = EXCLUDED.playtime_hours, is_subscription = EXCLUDED.is_subscription, ownership_status = EXCLUDED.ownership_status, raw_platform = EXCLUDED.raw_platform, is_available = true, updated_at = now()").
						Exec(ctx)
				}

				// Re-query only this batch to get DB state (is_skipped, id).
				// is_skipped is preserved by the ON CONFLICT clause above.
				var toProcess []models.ExternalGame
				_ = w.DB.NewSelect().Model(&toProcess).
					Where("user_id = ? AND storefront = ? AND is_available = true AND is_skipped = false AND external_id IN (?)",
						p.UserID, p.Storefront, bun.In(batchExtIDs)).
					Scan(ctx)

				for _, eg := range toProcess {
					meta := map[string]string{
						"external_game_id": eg.ID,
						"raw_platform":     rawPlatformByExtID[eg.ExternalID],
					}
					metaJSON, _ := json.Marshal(meta)
					itemID := uuid.NewString()
					_, _ = w.DB.NewRaw(
						`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
						 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())
						 ON CONFLICT (job_id, item_key) DO NOTHING`,
						itemID, p.JobID, p.UserID, eg.ExternalID, eg.Title, string(metaJSON),
					).Exec(ctx)
					if w.RiverClient != nil {
						_, _ = w.RiverClient.Insert(ctx, ProcessSyncItemArgs{JobItemID: itemID}, nil)
					}
				}
				return nil
			},
		); err != nil {
			expiredAt := time.Now().UTC()
			newCreds := map[string]any{
				"npsso_token":      psnCreds.NpssoToken,
				"is_verified":      false,
				"token_expired_at": expiredAt,
			}
			if b, merr := json.Marshal(newCreds); merr == nil {
				s := string(b)
				_, _ = w.DB.NewRaw(
					`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
					s, p.UserID,
				).Exec(context.Background())
			}
			failSyncJob(ctx, w.DB, p.JobID, "psn_token_expired")
			return nil
		}

	default:
		failSyncJob(ctx, w.DB, p.JobID, fmt.Sprintf("unknown storefront: %s", p.Storefront))
		return nil
	}

	// ── 5. Mark removed games as unavailable ──────────────────────────────
	var available []models.ExternalGame
	_ = w.DB.NewSelect().Model(&available).
		Where("user_id = ? AND storefront = ? AND is_available = true", p.UserID, p.Storefront).
		Scan(ctx)
	for _, eg := range available {
		if _, found := fetchedIDs[eg.ExternalID]; !found {
			_, _ = w.DB.NewRaw(
				`UPDATE external_games SET is_available = false, updated_at = now() WHERE id = ?`,
				eg.ID,
			).Exec(ctx)
		}
	}

	// ── 7. Update last_synced_at ──────────────────────────────────────────
	syncedNow := time.Now().UTC()
	_, _ = w.DB.NewRaw(
		`UPDATE user_sync_configs SET last_synced_at = ?, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		syncedNow, p.UserID, p.Storefront,
	).Exec(context.Background())

	return nil
}
```

- [ ] **Step 2: Build and run existing dispatch tests**

```bash
go build ./... && go test ./internal/worker/tasks/... -run TestDispatchSync -timeout 300s -v
```

Expected: all 7 existing `TestDispatchSync_*` tests pass. The PSN tests don't exist yet.

---

## Task 4: Add PSN dispatch tests

**Files:**
- Modify: `internal/worker/tasks/sync_test.go` (append new test functions)

- [ ] **Step 1: Write TestDispatchSync_PSNInvalidCredentials**

Append to `internal/worker/tasks/sync_test.go`:

```go
func TestDispatchSync_PSNInvalidCredentials(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	)
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, "not-valid-json",
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: &fakePSNAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (invalid psn credentials), got %q", status)
	}
}
```

- [ ] **Step 2: Write TestDispatchSync_PSNTokenNotVerified**

```go
func TestDispatchSync_PSNTokenNotVerified(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	)
	creds := `{"npsso_token":"abc123","is_verified":false}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: &fakePSNAdapter{}, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (token not verified), got %q", status)
	}
}
```

- [ ] **Step 3: Write TestDispatchSync_PSNFetchError**

```go
func TestDispatchSync_PSNFetchError(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'psn', 'pending', 'low')`,
		jobID, userID,
	)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	adapter := &fakePSNAdapter{err: errType("psn auth failed")}
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var status string
	_ = testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status)
	if status != "failed" {
		t.Errorf("expected job status=failed (psn fetch error), got %q", status)
	}

	// Token must be marked as expired in user_sync_configs.
	var rawCreds string
	_ = testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &rawCreds)
	var parsedCreds struct {
		IsVerified bool `json:"is_verified"`
	}
	_ = json.Unmarshal([]byte(rawCreds), &parsedCreds)
	if parsedCreds.IsVerified {
		t.Error("expected is_verified=false after fetch error, token still marked verified")
	}
}
```

- [ ] **Step 4: Write TestDispatchSync_PSNSuccess_ItemsDispatchedPerBatch**

```go
func TestDispatchSync_PSNSuccess_ItemsDispatchedPerBatch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	// Two pages of games — verifies that both pages are processed.
	page1 := []psnsvc.ExternalLibraryEntry{
		{ExternalID: "NPWR00001_00", Title: "God of War", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
		{ExternalID: "NPWR00002_00", Title: "Spider-Man", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
	}
	page2 := []psnsvc.ExternalLibraryEntry{
		{ExternalID: "NPWR00003_00", Title: "Horizon", RawPlatform: "playstation-5", OwnershipStatus: "owned"},
	}
	adapter := &fakePSNAdapter{pages: [][]psnsvc.ExternalLibraryEntry{page1, page2}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 3 games upserted as external_games.
	var egCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM external_games WHERE user_id = ? AND storefront = 'psn'`, userID).Scan(ctx, &egCount)
	if egCount != 3 {
		t.Errorf("expected 3 external_games, got %d", egCount)
	}

	// 3 job_items created (none pre-skipped).
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 3 {
		t.Errorf("expected 3 job_items, got %d", itemCount)
	}

	// last_synced_at updated.
	var lastSynced *time.Time
	_ = testDB.NewRaw(`SELECT last_synced_at FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &lastSynced)
	if lastSynced == nil {
		t.Error("expected last_synced_at to be set after successful psn sync")
	}
}
```

- [ ] **Step 5: Write TestDispatchSync_PSNSuccess_SkippedGameExcluded**

```go
func TestDispatchSync_PSNSuccess_SkippedGameExcluded(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'psn', 'pending', 'low', 0)`,
		jobID, userID,
	)
	creds := `{"npsso_token":"validtoken","is_verified":true}`
	configID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'psn', 'daily', ?)`,
		configID, userID, creds,
	).Exec(ctx)

	// Pre-insert God of War as skipped. The ON CONFLICT upsert does not touch
	// is_skipped, so it remains true even when the batch includes this game.
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours)
		 VALUES (?, ?, 'psn', 'NPWR00001_00', 'God of War', true, true, false, 0)`,
		uuid.NewString(), userID,
	)

	page1 := []psnsvc.ExternalLibraryEntry{
		{ExternalID: "NPWR00001_00", Title: "God of War", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
		{ExternalID: "NPWR00002_00", Title: "Spider-Man", RawPlatform: "playstation-4", OwnershipStatus: "owned"},
	}
	adapter := &fakePSNAdapter{pages: [][]psnsvc.ExternalLibraryEntry{page1}}
	w := &tasks.DispatchSyncWorker{DB: testDB, PSN: adapter, RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "psn"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 job_item (Spider-Man); God of War is skipped.
	var itemCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID).Scan(ctx, &itemCount)
	if itemCount != 1 {
		t.Errorf("expected 1 job_item (skipped game excluded), got %d", itemCount)
	}

	// Confirm no job_item for God of War.
	var gow int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND item_key = 'NPWR00001_00'`, jobID).Scan(ctx, &gow)
	if gow != 0 {
		t.Error("expected no job_item for skipped God of War")
	}
}
```

- [ ] **Step 6: Run PSN-specific tests**

```bash
go test ./internal/worker/tasks/... -run TestDispatchSync_PSN -timeout 300s -v
```

Expected: all 5 new `TestDispatchSync_PSN*` tests pass.

---

## Task 5: Full verification and commit

- [ ] **Step 1: Run the full test suite**

```bash
go test ./... -timeout 600s
```

Expected: all tests pass.

- [ ] **Step 2: Run linter**

```bash
golangci-lint run
```

Expected: zero errors.

- [ ] **Step 3: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go internal/services/psn/client.go
git commit -m "$(cat <<'EOF'
feat(psn): batch-dispatch PSN sync items per page for faster progress

Switches DispatchSyncWorker from fetching the full PSN library before
creating any job_items to dispatching in pages of 10. Job items now
appear in the progress box after the first batch rather than after the
entire library is fetched, fixing the "stuck" appearance. Also removes
the silent 100-game cap that affected large libraries.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```
