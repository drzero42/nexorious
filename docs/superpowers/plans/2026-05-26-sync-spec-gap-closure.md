# Sync Spec Gap Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three remaining `docs/sync.md` spec gaps identified by the 2026-05-26 audit on branch `issue-608-normalise-external-games`: (G1) Stage 3 `external_game_id` backfill on `user_game_platforms`, (G2) Epic adapter library chunking, (G3) PSN inter-page rate limiting.

**Architecture:** Three independent, file-local changes — no shared modules, no migrations. Each task adds one new test against the spec invariant, makes it pass with the minimal code change, and commits separately so each gap closure is independently revertible. Existing test patterns are reused (`truncateAllTables` + `testDB` for the Stage 3 test; the existing `fakeEpicClient` struct for the Epic test; the existing `httptest.NewServer` + `SetX` pattern for the PSN test).

**Tech Stack:** Go 1.25, Bun ORM (raw SQL), River queue, `golang.org/x/time/rate`, `testcontainers-go` (Postgres), stdlib `testing`.

**Spec reference:** [docs/superpowers/specs/2026-05-26-sync-spec-gap-closure-design.md](../specs/2026-05-26-sync-spec-gap-closure-design.md).

---

## Task 1: G1 — Stage 3 backfills `external_game_id` on every conflict path

**Files:**
- Modify: `internal/worker/tasks/sync.go` (lines 625–689 — the per-platform loop in `UserGameWorker.Work`)
- Test: `internal/worker/tasks/sync_test.go` (add a new test function alongside existing `TestUserGameWorker_*` tests)

### Background

Spec § "Manually added games" requires `external_game_id` to be always set on `user_game_platforms`. The current code at `sync.go:625-689` only writes it on the INSERT branch; all three UPDATE sub-cases (ownership upgrade, hours-only increase, no-op) leave `external_game_id` untouched. The most acute case: a manually-added row with `external_game_id = NULL` whose first sync brings same rank + equal-or-lower playtime → no UPDATE runs at all → `external_game_id` stays NULL permanently.

The fix collapses the three UPDATE branches into one unconditional UPDATE that runs once per platform iteration, computing `ownership_status` and `hours_played` in Go and always setting `external_game_id = eg.ID`.

### Steps

- [ ] **Step 1.1: Write the failing test**

Append this test at the bottom of `internal/worker/tasks/sync_test.go` (after the existing `TestUserGameWorker_*` tests):

```go
// TestUserGameWorker_BackfillsExternalGameID covers the spec invariant from
// docs/sync.md § "Manually added games": Stage 3 must always set
// external_game_id on user_game_platforms, even when the row pre-existed
// (e.g. manually added by the user) and the incoming sync brings neither an
// ownership rank upgrade nor a higher playtime. This is the no-op sub-case
// of the conflict branch — the one most prone to silently leaving
// external_game_id NULL forever.
func TestUserGameWorker_BackfillsExternalGameID(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	_, _ = testDB.ExecContext(ctx, `INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx, `INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1)`,
		jobID, userID,
	)
	const igdbID = int32(1100)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Hades', now(), now()) ON CONFLICT (id) DO NOTHING`, igdbID,
	)
	// Pre-create a "manually added" user_game + user_game_platforms row:
	// external_game_id = NULL, ownership = 'owned', hours_played = 50.
	ugID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		ugID, userID, igdbID,
	)
	ugpID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, sync_from_source, created_at, updated_at)
		 VALUES (?, ?, 'pc-windows', 'steam', true, 50.0, 'owned', false, now(), now())`,
		ugpID, ugID,
	)

	// Incoming sync: same ownership rank ('owned'), lower hours (30 < 50) — the no-op sub-case.
	egID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, ownership_status, resolved_igdb_id)
		 VALUES (?, ?, 'steam', '1145360', 'Hades', false, true, false, 'owned', ?)`,
		egID, userID, igdbID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at) VALUES (?, ?, 'pc-windows', 30.0, now())`,
		uuid.NewString(), egID,
	).Exec(ctx)
	itemID := uuid.NewString()
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, '1145360', 'Hades', ?, '{}', 'pending', '{}', '[]')`,
		itemID, jobID, userID, egID,
	)

	w := &tasks.UserGameWorker{DB: testDB, RiverClient: nil}
	if err := w.Work(ctx, &river.Job[tasks.UserGameArgs]{Args: tasks.UserGameArgs{JobItemID: itemID}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// external_game_id must now point at the synced external_games row.
	var gotEGID *string
	_ = testDB.NewRaw(
		`SELECT external_game_id FROM user_game_platforms WHERE id = ?`, ugpID,
	).Scan(ctx, &gotEGID)
	if gotEGID == nil || *gotEGID != egID {
		t.Errorf("external_game_id: want %q, got %v", egID, gotEGID)
	}

	// ownership_status unchanged (no upgrade).
	var gotOwnership string
	_ = testDB.NewRaw(
		`SELECT ownership_status FROM user_game_platforms WHERE id = ?`, ugpID,
	).Scan(ctx, &gotOwnership)
	if gotOwnership != "owned" {
		t.Errorf("ownership_status: want 'owned', got %q", gotOwnership)
	}

	// hours_played unchanged (incoming was lower).
	var gotHours float64
	_ = testDB.NewRaw(
		`SELECT hours_played FROM user_game_platforms WHERE id = ?`, ugpID,
	).Scan(ctx, &gotHours)
	if gotHours != 50.0 {
		t.Errorf("hours_played: want 50.0 (preserved), got %v", gotHours)
	}
}
```

- [ ] **Step 1.2: Run the new test, confirm it fails**

Run:
```
go test -timeout 600s ./internal/worker/tasks/ -run TestUserGameWorker_BackfillsExternalGameID -v
```

Expected: FAIL — the assertion `external_game_id: want "<egID>", got <nil>` will print, because the current `default:` branch in `sync.go` takes the `else` path (no ownership upgrade) and then takes the inner `else` path (hours not higher), so no UPDATE runs and `external_game_id` stays NULL.

- [ ] **Step 1.3: Replace the `default:` branch in `sync.go` with a single unconditional UPDATE**

In `internal/worker/tasks/sync.go`, replace lines 651–688 of the existing `switch { ... }` block with the snippet below. That is: the `default:` label, its body (starting `existingRank := 0`), and the trailing `}` that closes the switch statement. The for-loop's closing `}` on line 689 is **not** part of the replacement and must remain in place.

```go
		default:
			// Resolve final ownership and hours in Go, then write a single
			// unconditional UPDATE. This collapses the previous three branches
			// (ownership upgrade / hours-only / no-op) and guarantees that
			// external_game_id is always backfilled — see docs/sync.md
			// § "Manually added games".
			existingRank := 0
			if existingOwnership != nil {
				existingRank = ownershipRank(*existingOwnership)
			}
			newRank := ownershipRank(ownership)

			finalOwnership := ownership
			if existingOwnership != nil {
				finalOwnership = *existingOwnership
			}
			if newRank > existingRank {
				// Insert the status_changed sync_change BEFORE the UPDATE so
				// that old_status reflects the pre-UPDATE value.
				if _, err := w.DB.NewRaw(
					`INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, old_status, new_status, created_at)
					 VALUES (?, ?, ?, ?, 'status_changed', ?, ?, ?, now())`,
					uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title, existingOwnership, &ownership,
				).Exec(ctx); err != nil {
					slog.Error("user_game_write: insert sync_change (status_changed)", "err", err)
				}
				finalOwnership = ownership
			}

			finalHours := egp.HoursPlayed
			if existingHours != nil && *existingHours > finalHours {
				finalHours = *existingHours
			}

			if _, err := w.DB.NewRaw(
				`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, external_game_id = ?, updated_at = now() WHERE id = ?`,
				finalOwnership, finalHours, eg.ID, existingID,
			).Exec(ctx); err != nil {
				slog.Error("user_game_write: update ugp", "err", err, "item_id", p.JobItemID)
			}
		}
```

Leave the `errors.Is(err, sql.ErrNoRows)` (INSERT) branch and the `err != nil` (log-and-skip) branch unchanged.

- [ ] **Step 1.4: Run the full UserGameWorker test set, confirm all pass**

Run:
```
go test -timeout 600s ./internal/worker/tasks/ -run TestUserGameWorker -v
```

Expected: PASS for all `TestUserGameWorker_*` tests including the new `TestUserGameWorker_BackfillsExternalGameID`, the existing `TestUserGameWorker_CreatesUserGameAndSyncChange`, `TestUserGameWorker_OwnershipRankGuard`, `TestUserGameWorker_StatusChangedSyncChange`, and any other UserGameWorker tests in the file. The existing tests cover the INSERT and ownership-upgrade paths; the new test covers the no-op path. If `TestUserGameWorker_OwnershipRankGuard` fails because hours_played is now being touched on every iteration (it should still be 20.0 in that test), that's a real assertion to investigate — but the existing test expects `resultHours = 20.0` which is the max(20 incoming, 10 existing), which is what the new code computes.

- [ ] **Step 1.5: Commit**

```
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "$(cat <<'EOF'
fix(sync): backfill external_game_id on user_game_platforms updates

Stage 3 previously only set external_game_id on the INSERT branch of
the user_game_platforms conflict handling. The three UPDATE sub-cases
(ownership upgrade, hours-only increase, no-op) all left it untouched,
which meant manually-added rows whose first sync brought same rank and
equal-or-lower hours never got linked to their ExternalGame.

Collapse the three UPDATE branches into a single unconditional UPDATE
that always sets external_game_id = eg.ID. Closes G1 of the
2026-05-26 sync-spec gap closure.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: G2 — Epic adapter chunks library into ≤10-game batches

**Files:**
- Modify: `internal/services/epic/adapter.go` (lines 50–88 — the `GetLibrary` method, currently ignores `batchSize`)
- Test: `internal/services/epic/adapter_test.go` (add a new test alongside the existing `TestEpicAdapter_*` tests)

### Background

Spec § Epic line 354: *"the adapter chunks the output into batches of ≤10."* The Legendary CLI's `legendary list --json` returns the entire library as one JSON document with no native pagination, so the client at `epic/client.go:131` calls `onBatch(entries)` once with the full slice. The adapter at `epic/adapter.go:50` declares the `batchSize` parameter as `_ int` and passes the batch through unchanged.

Consequence: the dispatcher's Progress Box `count grows as games are fetched` UX (spec § "Progress Box") doesn't grow at all — it jumps from 0 to N at end-of-fetch. All Stage 2 enqueues happen in one tight loop after Legendary returns.

The fix lives in `adapter.go` (where the spec says chunking belongs). The client is unchanged.

### Steps

- [ ] **Step 2.1: Write the failing test**

Append this test at the bottom of `internal/services/epic/adapter_test.go`:

```go
// TestEpicAdapter_ChunksLibraryIntoBatches covers the spec invariant from
// docs/sync.md § Epic: the adapter must re-chunk the client's single big
// batch into chunks of ≤10 before invoking the outer onBatch.
func TestEpicAdapter_ChunksLibraryIntoBatches(t *testing.T) {
	// Build one client-side batch of 25 entries.
	big := make([]ExternalGameEntry, 25)
	for i := range big {
		big[i] = ExternalGameEntry{
			ExternalID:      fmt.Sprintf("game-%02d", i),
			Title:           fmt.Sprintf("Game %02d", i),
			OwnershipStatus: "owned",
		}
	}
	fake := &fakeEpicClient{
		configured:      true,
		libraryBatches:  [][]ExternalGameEntry{big},
		captureSnapshot: map[string]string{},
	}
	a := NewAdapter(fake, "user1", map[string]string{}, nil)

	var receivedSizes []int
	if err := a.GetLibrary(context.Background(), 10, func(batch []storefrontadapter.ExternalGameEntry) error {
		receivedSizes = append(receivedSizes, len(batch))
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSizes := []int{10, 10, 5}
	if len(receivedSizes) != len(wantSizes) {
		t.Fatalf("expected %d outer onBatch calls, got %d (sizes=%v)", len(wantSizes), len(receivedSizes), receivedSizes)
	}
	for i, got := range receivedSizes {
		if got != wantSizes[i] {
			t.Errorf("batch %d size: want %d, got %d", i, wantSizes[i], got)
		}
	}
}
```

Then add `"fmt"` to the imports of `adapter_test.go` if it's not already present. Check the existing import block at the top of the file — if `fmt` is missing, add it.

- [ ] **Step 2.2: Run the new test, confirm it fails**

Run:
```
go test ./internal/services/epic/ -run TestEpicAdapter_ChunksLibraryIntoBatches -v
```

Expected: FAIL — `expected 3 outer onBatch calls, got 1 (sizes=[25])`. The current adapter passes the 25-entry batch straight through.

- [ ] **Step 2.3: Update `GetLibrary` to chunk the batch**

In `internal/services/epic/adapter.go`, replace the `GetLibrary` method (currently lines 49–88) with this:

```go
// GetLibrary implements storefrontadapter.Adapter.
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if !a.client.Configured() {
		return fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}

	if err := a.client.RestoreSnapshot(a.userID, a.snapshot); err != nil {
		return fmt.Errorf("epic: restore snapshot: %w", err)
	}

	// Per docs/sync.md § Epic, the adapter chunks the client's single big
	// batch into batches of ≤10 before invoking the outer onBatch.
	chunkSize := batchSize
	if chunkSize <= 0 || chunkSize > 10 {
		chunkSize = 10
	}

	fetchErr := a.client.GetLibrary(ctx, a.userID, func(batch []ExternalGameEntry) error {
		for start := 0; start < len(batch); start += chunkSize {
			end := start + chunkSize
			if end > len(batch) {
				end = len(batch)
			}
			mapped := make([]storefrontadapter.ExternalGameEntry, 0, end-start)
			for _, e := range batch[start:end] {
				mapped = append(mapped, storefrontadapter.ExternalGameEntry{
					ExternalID:      e.ExternalID,
					Title:           e.Title,
					PlaytimeHours:   0,
					Platforms:       []string{"pc-windows"},
					OwnershipStatus: e.OwnershipStatus,
					IsSubscription:  false,
				})
			}
			if err := onBatch(mapped); err != nil {
				return err
			}
		}
		return nil
	})

	// Capture updated snapshot regardless of fetch error.
	newSnapshot, captureErr := a.client.CaptureSnapshot(a.userID)
	if captureErr != nil {
		slog.Error("epic: capture snapshot failed", "user_id", a.userID, "err", captureErr)
	} else if len(newSnapshot) > 0 && a.onSnapshot != nil {
		if err := a.onSnapshot(newSnapshot); err != nil {
			slog.Error("epic: persist updated snapshot failed", "user_id", a.userID, "err", err)
		}
	}

	if errors.Is(fetchErr, ErrAuthFailed) {
		return fmt.Errorf("%w: epic legendary auth failure", storefrontadapter.ErrCredentials)
	}
	return fetchErr
}
```

**Important — argument order for `a.client.GetLibrary`:** double-check that the existing call in the file is `a.client.GetLibrary(ctx, a.userID, callback)` (ctx then userID) by reading the current line — my snippet above mirrors the existing order. If the new code differs only in the callback body, leave the `ctx, a.userID` order matching the original. The `clientInterface` declaration at `adapter.go:14-19` is the source of truth for the signature.

- [ ] **Step 2.4: Run the full Epic adapter test set, confirm all pass**

Run:
```
go test ./internal/services/epic/ -v
```

Expected: PASS for all `TestEpicAdapter_*` tests. The new chunking test passes; the existing tests (NotConfigured, RestoresSnapshotBeforeFetch, PersistsNewSnapshotAfterSuccess, PersistsSnapshotEvenOnFetchError, SkipsPersistWhenSnapshotEmpty, LegendaryAuthFailure_ReturnsErrCredentials, MapsEntriesToStorefrontFormat) all use small batches (0 or 1 entries) so they remain in a single chunk and behave identically.

- [ ] **Step 2.5: Commit**

```
git add internal/services/epic/adapter.go internal/services/epic/adapter_test.go
git commit -m "$(cat <<'EOF'
fix(sync): chunk Epic library into <=10-game batches

The Epic adapter previously passed the Legendary CLI's single big
batch straight through, ignoring the batchSize parameter. This meant
the dispatcher's Progress Box stayed at 0 for the entire fetch and
all Stage 2 enqueues ran in one synchronous loop at end-of-fetch.

Slice the client's batch into chunks of <=10 inside the adapter, per
docs/sync.md § Epic. The client is unchanged — Legendary itself is
not paginated. Closes G2 of the 2026-05-26 sync-spec gap closure.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: G3 — PSN client throttles paginated fetches

**Files:**
- Modify: `internal/services/psn/client.go` (add `limiter` field; initialize in `NewClient`; call `limiter.Wait` in `fetchPlayHistory` and `fetchPurchasedGames`)
- Modify: `internal/services/psn/export_test.go` (add `SetLimiter` helper)
- Modify: `internal/services/psn/library_test.go` (existing pagination tests inject an unlimited limiter so they stay fast)
- Test: `internal/services/psn/library_test.go` (add a new test asserting the limiter is consulted between pages)

### Background

Spec § PSN line 337: *"the adapter applies a conservative request delay between pages."* The current client makes pages as fast as the network allows. The fix adds a `*rate.Limiter` to the client, defaulting to `rate.NewLimiter(rate.Every(200*time.Millisecond), 1)` — 5 requests/second, matching the Steam client's cadence at `internal/services/steam/client.go:29`. The package's existing test-hook pattern (`SetX` methods in `export_test.go`) is reused — no new constructor.

Both `fetchPlayHistory` and `fetchPurchasedGames` are paginated; both must call `limiter.Wait(ctx)` at the top of each loop iteration before issuing the HTTP request.

### Steps

- [ ] **Step 3.1: Write the failing timing test**

Append this test at the bottom of `internal/services/psn/library_test.go`:

```go
// TestFetchPurchasedGames_RateLimiterWaitsBetweenPages covers the spec
// invariant from docs/sync.md § PSN: the adapter applies a conservative
// request delay between pages. With a 50ms-per-token limiter and 3 page
// fetches, the second and third calls each wait one token => total
// elapsed time must be >= 100ms.
func TestFetchPurchasedGames_RateLimiterWaitsBetweenPages(t *testing.T) {
	// Server emits 2 games per call for the first two calls and 1 game for
	// the third (so the < size condition breaks the loop after the third).
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		var body string
		if callCount < 3 {
			body = `{"data":{"purchasedTitlesRetrieve":{"games":[
				{"titleId":"CUSA00001_00","name":"Game A","platform":"PS5","subscriptionService":"NONE"},
				{"titleId":"CUSA00002_00","name":"Game B","platform":"PS5","subscriptionService":"NONE"}
			]}}}`
		} else {
			body = `{"data":{"purchasedTitlesRetrieve":{"games":[
				{"titleId":"CUSA00003_00","name":"Game C","platform":"PS5","subscriptionService":"NONE"}
			]}}}`
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGraphQLURL(srv.URL)
	c.SetGraphQLPageSize(2)
	c.SetLimiter(rate.NewLimiter(rate.Every(50*time.Millisecond), 1))

	start := time.Now()
	if _, err := c.fetchPurchasedGames(context.Background(), "test-token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	if callCount != 3 {
		t.Fatalf("expected 3 HTTP calls, got %d", callCount)
	}
	// First call passes through immediately (bucket starts full); the next
	// two each wait ~50ms => >= 100ms total.
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected elapsed >= 100ms, got %v (limiter not consulted between pages?)", elapsed)
	}
}
```

Then add these imports to the top of `library_test.go` if they are not already present: `"time"`, `"golang.org/x/time/rate"`. Check the existing import block first.

- [ ] **Step 3.2: Run the new test, confirm it fails**

Run:
```
go test ./internal/services/psn/ -run TestFetchPurchasedGames_RateLimiterWaitsBetweenPages -v
```

Expected: FAIL — the test will fail at the `c.SetLimiter(...)` line with `c.SetLimiter undefined`. That is the compile-time failure that proves the limiter wiring is not in place yet. Implement Steps 3.3 and 3.4 together (the test will not compile until SetLimiter exists), then re-run.

- [ ] **Step 3.3: Add the limiter field, default, and test-hook setter**

In `internal/services/psn/client.go`:

1. Add `"time"` and `"golang.org/x/time/rate"` to the import block at the top of the file (check whether they're already imported; the file already imports many packages, so add only what's missing).

2. Add a `limiter` field to the `Client` struct (currently around lines 33–41). The struct should become:

```go
type Client struct {
	httpClient      *http.Client
	gamelistURL     string
	graphqlURL      string
	graphqlPageSize int
	limiter         *rate.Limiter
	// authFn overrides psnsdk authentication; used in tests only.
	authFn func(ctx context.Context, npssoToken string) (string, error)
}
```

3. Initialize the limiter in `NewClient` (currently lines 43–51):

```go
func NewClient() *Client {
	return &Client{
		httpClient:      http.DefaultClient,
		gamelistURL:     "https://m.np.playstation.com",
		graphqlURL:      "https://web.np.playstation.com",
		graphqlPageSize: 200,
		// 5 req/sec, matching internal/services/steam/client.go.
		// docs/sync.md § PSN requires a conservative request delay
		// between pages; PSN has no published rate ceiling.
		limiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
	}
}
```

4. In `internal/services/psn/export_test.go`, add a `SetLimiter` helper following the existing one-liner pattern. The file currently ends with `SetAuthFn`. Add this line after it:

```go
func (c *Client) SetLimiter(l *rate.Limiter)                                            { c.limiter = l }
```

And add `"golang.org/x/time/rate"` to the imports of `export_test.go` (currently imports only `context` and `net/http`).

- [ ] **Step 3.4: Call `limiter.Wait` at the top of each paginated fetch loop**

In `internal/services/psn/client.go`, modify both paginated fetch functions to call `c.limiter.Wait(ctx)` before issuing the HTTP request.

**`fetchPlayHistory` (currently lines 155–222):** in the `for offset := 0; ; offset += limit {` loop, insert these three lines as the very first statements inside the loop body, immediately above the existing `u := fmt.Sprintf(...)` line:

```go
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("psn: rate limiter wait: %w", err)
		}
```

**`fetchPurchasedGames` (currently lines 239–318):** in the `for start := 0; ; start += size {` loop, insert the same three lines as the very first statements inside the loop body, immediately above the existing `variables := fmt.Sprintf(...)` line:

```go
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("psn: rate limiter wait: %w", err)
		}
```

- [ ] **Step 3.5: Inject an unlimited limiter into existing pagination tests**

The existing `TestFetchPlayHistory_Pagination` test (currently in `library_test.go` around line 100) makes 2 HTTP calls; with the default 200ms limiter it would now wait ~200ms (one inter-page wait). Keep the test fast by setting an unlimited limiter at the top:

In `library_test.go`, find `TestFetchPlayHistory_Pagination`. After the existing two lines:
```go
	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetGamelistURL(srv.URL)
```
add a third line:
```go
	c.SetLimiter(rate.NewLimiter(rate.Inf, 1))
```

If any other test in this file makes more than one paginated call (search the file for `fetchPlayHistory` and `fetchPurchasedGames` invocations), apply the same `SetLimiter(rate.NewLimiter(rate.Inf, 1))` line to keep them fast. Single-page tests don't need it (they hit the bucket exactly once and burst=1 lets it through).

- [ ] **Step 3.6: Run the full PSN test set, confirm all pass**

Run:
```
go test ./internal/services/psn/ -v
```

Expected: PASS for all `TestFetchPlayHistory_*`, `TestFetchPurchasedGames_*`, and `TestParse*` tests, including the new `TestFetchPurchasedGames_RateLimiterWaitsBetweenPages`. The whole package should finish in well under 1 second.

- [ ] **Step 3.7: Commit**

```
git add internal/services/psn/client.go internal/services/psn/export_test.go internal/services/psn/library_test.go
git commit -m "$(cat <<'EOF'
fix(sync): rate-limit PSN paginated fetches

PSN had no inter-page delay; the spec (docs/sync.md § PSN) requires a
conservative request delay between pages. Add a *rate.Limiter to the
PSN client at 5 req/sec (matching Steam's cadence in
internal/services/steam/client.go) and call Wait() at the top of each
paginated fetch loop in fetchPlayHistory and fetchPurchasedGames.

Existing pagination tests inject an unlimited limiter via SetLimiter
so they remain instant. Closes G3 of the 2026-05-26 sync-spec gap
closure.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Verification

**Files:** none — quality gates only.

### Steps

- [ ] **Step 4.1: Run the full Go test suite**

Run:
```
go test -timeout 600s ./...
```

Expected: PASS for every package. No backend functionality outside the three touched packages was changed; the only risk is a regression in `internal/worker/tasks/sync_test.go` from the G1 refactor — already covered by Step 1.4, but the full-suite run is the broader safety net.

- [ ] **Step 4.2: Run golangci-lint**

Run:
```
golangci-lint run
```

Expected: zero findings. Any issues likely live in the new code blocks added in Tasks 1–3 — fix them in place and re-run before continuing. Do NOT use any `--no-verify` or skip-hook flag.

- [ ] **Step 4.3: Re-audit the three spec sections against the touched code**

Open `docs/sync.md` and re-read these three excerpts:
1. § "Manually added games" — *"`external_game_id` is always set (or updated)..."*
2. § "Storefront Adapters → Epic Games Store" — *"the adapter chunks the output into batches of ≤10"*
3. § "Storefront Adapters → PSN" — *"the adapter applies a conservative request delay between pages"*

For each, open the corresponding file and confirm the new code MATCHES the requirement:
- G1 → `internal/worker/tasks/sync.go` — the single UPDATE in the `default:` branch always sets `external_game_id = eg.ID`
- G2 → `internal/services/epic/adapter.go` — the `GetLibrary` callback iterates the input slice in `chunkSize`-sized windows
- G3 → `internal/services/psn/client.go` — both paginated fetch loops begin with `c.limiter.Wait(ctx)`

No action if all three match; if any deviates, return to the relevant task.

- [ ] **Step 4.4: Optional — push and open PR**

The user will decide when to push. Do not push without explicit instruction. When the user asks, push to `origin/issue-608-normalise-external-games` and open a PR titled `fix(sync): close remaining docs/sync.md spec gaps` with the body summarising the three closures and linking to the spec file at `docs/superpowers/specs/2026-05-26-sync-spec-gap-closure-design.md`.
