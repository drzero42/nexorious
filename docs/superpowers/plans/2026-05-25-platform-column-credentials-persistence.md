# Platform Column and Credentials Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add platform slugs to the external games API response and surface them in the Matched UI column; persist `credentials_error` to the database so the flag survives page navigation, and fix Steam and Epic adapters to correctly detect auth failures.

**Architecture:** Eight sequential tasks split across two independent parts. Part A (Tasks 1–2) adds the platform column to the backend response and frontend UI. Part B (Tasks 3–8) adds a `credentials_error` boolean column to `user_sync_configs`, writes it in the sync worker, reads it in the four connection-status endpoints, clears it in the four credential-save handlers, and adds `ErrCredentials`-wrapping to the Steam and Epic adapters.

**Tech Stack:** Go 1.25, Echo v5, Bun ORM, PostgreSQL; TypeScript, React 19, TanStack Query, shadcn/ui, Vitest

**Spec:** `docs/superpowers/specs/2026-05-25-platform-column-credentials-persistence-design.md`

---

## Task 1: Part A backend — add `platforms` to external games API response

**Files:**
- Modify: `internal/api/sync.go`
- Modify: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/sync_test.go`:

```go
func TestListExternalGames_ReturnsPlatforms(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-plat")

	insertExternalGame(t, testDB, "eg-p1", userID, "steam", "730", "CS2")

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES ('egp-1', 'eg-p1', 'pc-windows', 0, now()),
		        ('egp-2', 'eg-p1', 'pc-linux', 0, now())`)
	if err != nil {
		t.Fatalf("insert platforms: %v", err)
	}

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (8888, 'CS2', now(), now()) ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE external_games SET resolved_igdb_id = 8888 WHERE id = 'eg-p1'`)

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 game, got %d", len(resp))
	}
	plRaw, ok := resp[0]["platforms"]
	if !ok {
		t.Fatal("expected 'platforms' field in response")
	}
	platforms, _ := plRaw.([]any)
	if len(platforms) != 2 {
		t.Errorf("expected 2 platforms, got %v", plRaw)
	}
	platformStrs := make(map[string]bool)
	for _, p := range platforms {
		platformStrs[p.(string)] = true
	}
	if !platformStrs["pc-windows"] || !platformStrs["pc-linux"] {
		t.Errorf("expected pc-windows and pc-linux, got %v", platforms)
	}
}

func TestListExternalGames_NoPlatforms_ReturnsEmptyArray(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-noplat")

	insertExternalGame(t, testDB, "eg-np1", userID, "steam", "999", "Orphan Game")

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	var resp []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Fatalf("expected 1 game, got %d", len(resp))
	}
	plRaw := resp[0]["platforms"]
	platforms, _ := plRaw.([]any)
	if len(platforms) != 0 {
		t.Errorf("expected empty platforms array, got %v", plRaw)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd /home/abo/workspace/home/nexorious && go test ./internal/api/... -run "TestListExternalGames_ReturnsPlatforms|TestListExternalGames_NoPlatforms" -v 2>&1 | tail -20
```

Expected: FAIL with `'platforms' field` not found in response.

- [ ] **Step 3: Add `PlatformsCSV` and `Platforms` to `externalGameResponse`**

In `internal/api/sync.go`, add `"strings"` to the import block:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	// ... rest unchanged
)
```

Change `externalGameResponse` (lines 132–147) to add two fields:

```go
type externalGameResponse struct {
	ID                         string   `bun:"id"                             json:"id"`
	Storefront                 string   `bun:"storefront"                     json:"storefront"`
	ExternalID                 string   `bun:"external_id"                    json:"external_id"`
	Title                      string   `bun:"title"                          json:"title"`
	ResolvedIGDBID             *int32   `bun:"resolved_igdb_id"               json:"resolved_igdb_id"`
	IsSkipped                  bool     `bun:"is_skipped"                     json:"is_skipped"`
	IsAvailable                bool     `bun:"is_available"                   json:"is_available"`
	IsSubscription             bool     `bun:"is_subscription"                json:"is_subscription"`
	HasUserGame                bool     `bun:"has_user_game"                  json:"has_user_game"`
	UserGameID                 *string  `bun:"user_game_id"                   json:"user_game_id"`
	IGDBTitle                  *string  `bun:"igdb_title"                     json:"igdb_title"`
	UserGameOtherPlatformCount int      `bun:"user_game_other_platform_count" json:"user_game_other_platform_count"`
	SyncStatus                 string   `bun:"sync_status"                    json:"sync_status"`
	FailedJobItemID            *string  `bun:"failed_job_item_id"             json:"failed_job_item_id"`
	PlatformsCSV               string   `bun:"platforms_csv"                  json:"-"`
	Platforms                  []string `bun:"-"                              json:"platforms"`
}
```

- [ ] **Step 4: Add the `platforms_csv` correlated subquery to the SQL**

In `HandleListExternalGames` (around line 963), add to the SELECT list before the `FROM` clause. The subquery goes right after the `failed_job_item_id` subquery:

```go
err := h.db.NewRaw(`
	SELECT
		eg.id,
		eg.storefront,
		eg.external_id,
		eg.title,
		eg.resolved_igdb_id,
		eg.is_skipped,
		eg.is_available,
		eg.is_subscription,
		(ugp.user_game_id IS NOT NULL) AS has_user_game,
		ugp.user_game_id,
		g.title AS igdb_title,
		COALESCE(
			(SELECT COUNT(*) FROM user_game_platforms o
			 WHERE o.user_game_id = ugp.user_game_id AND o.id != ugp.id),
			0
		) AS user_game_other_platform_count,
		CASE
			WHEN EXISTS (
				SELECT 1 FROM job_items ji
				WHERE ji.external_game_id = eg.id AND ji.status = 'pending_review'
			) THEN 'needs_review'
			WHEN EXISTS (
				SELECT 1 FROM job_items ji
				WHERE ji.external_game_id = eg.id AND ji.status = 'failed'
			) THEN 'failed'
			WHEN eg.is_skipped THEN 'skipped'
			WHEN eg.resolved_igdb_id IS NOT NULL THEN 'matched'
			ELSE 'unmatched'
		END AS sync_status,
		(
			SELECT ji.id FROM job_items ji
			WHERE ji.external_game_id = eg.id AND ji.status = 'failed'
			ORDER BY ji.created_at DESC
			LIMIT 1
		) AS failed_job_item_id,
		COALESCE(
			(SELECT string_agg(egp.platform, ',' ORDER BY egp.platform)
			 FROM external_game_platforms egp
			 WHERE egp.external_game_id = eg.id),
			''
		) AS platforms_csv
	FROM external_games eg
	LEFT JOIN user_game_platforms ugp ON ugp.external_game_id = eg.id
	LEFT JOIN games g ON g.id = eg.resolved_igdb_id
	WHERE eg.user_id = ? AND eg.storefront = ?
	  AND NOT EXISTS (
	      SELECT 1 FROM job_items ji
	      WHERE ji.external_game_id = eg.id
	        AND ji.status IN ('pending', 'processing')
	  )
	ORDER BY eg.title ASC`,
	userID, sf,
).Scan(ctx, &games)
```

- [ ] **Step 5: Add post-scan CSV split after the Scan call**

Replace the block after `Scan` (lines 1012–1016) with:

```go
if err != nil {
	return echo.NewHTTPError(http.StatusInternalServerError, "failed to list external games")
}
if games == nil {
	games = []externalGameResponse{}
}
for i := range games {
	if games[i].PlatformsCSV != "" {
		games[i].Platforms = strings.Split(games[i].PlatformsCSV, ",")
	} else {
		games[i].Platforms = []string{}
	}
}
return c.JSON(http.StatusOK, games)
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
go test ./internal/api/... -run "TestListExternalGames" -v 2>&1 | tail -30
```

Expected: all `TestListExternalGames_*` tests PASS.

- [ ] **Step 7: Run the full API test suite**

```bash
go test ./internal/api/... -timeout 300s 2>&1 | tail -10
```

Expected: all tests pass, no failures.

- [ ] **Step 8: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): include platform slugs in external games API response"
```

---

## Task 2: Part A frontend — ExternalGame type and Platform column UI

**Files:**
- Modify: `ui/frontend/src/types/sync.ts`
- Modify: `ui/frontend/src/components/sync/external-games-section.tsx`

- [ ] **Step 1: Update `ExternalGame` in `types/sync.ts`**

Open `ui/frontend/src/types/sync.ts`. Find the `ExternalGame` interface. Remove `playtime_hours: number` and add `platforms: string[]`:

```ts
export interface ExternalGame {
  id: string;
  storefront: string;
  external_id: string;
  title: string;
  resolved_igdb_id: number | null;
  is_skipped: boolean;
  is_available: boolean;
  is_subscription: boolean;
  has_user_game: boolean;
  user_game_id: string | null;
  igdb_title: string | null;
  user_game_other_platform_count: number;
  sync_status: string;
  failed_job_item_id: string | null;
  platforms: string[];
}
```

(Remove `playtime_hours: number` if present; it was never returned by the backend.)

- [ ] **Step 2: Add Platform column to the Matched table in `external-games-section.tsx`**

In `ui/frontend/src/components/sync/external-games-section.tsx`, find the Matched `<Collapsible>` block. It currently has `<TableHeader>` with three `<TableHead>` cells. Replace it with four:

```tsx
<TableHeader>
  <TableRow>
    <TableHead>Storefront Title</TableHead>
    <TableHead>IGDB Title</TableHead>
    <TableHead>Platform</TableHead>
    <TableHead />
  </TableRow>
</TableHeader>
```

In the same block, find the `<TableRow>` for each matched game. Add the Platform cell between IGDB Title and the action button:

```tsx
{matched.map((game) => (
  <TableRow key={game.id}>
    <TableCell>{game.title}</TableCell>
    <TableCell className="text-muted-foreground">{game.igdb_title}</TableCell>
    <TableCell className="text-muted-foreground">
      {game.platforms.join(', ')}
    </TableCell>
    <TableCell className="text-right">
      <Button
        size="sm"
        variant="outline"
        onClick={() => setMatchingGame(game)}
        disabled={isRematching}
      >
        Change Match
      </Button>
    </TableCell>
  </TableRow>
))}
```

- [ ] **Step 3: Run type check and tests**

```bash
cd /home/abo/workspace/home/nexorious/ui/frontend && npm run check 2>&1 | tail -20
```

Expected: no TypeScript errors.

```bash
npm run test -- --run 2>&1 | tail -20
```

Expected: all frontend tests pass.

- [ ] **Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add ui/frontend/src/types/sync.ts ui/frontend/src/components/sync/external-games-section.tsx
git commit -m "feat(sync): add Platform column to Matched external games table"
```

---

## Task 3: Part B schema — add `credentials_error` column

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql`
- Modify: `internal/db/models/models.go`

Note: The down migration (`20260503000001_initial.down.sql`) drops the entire table — no down migration change is needed.

- [ ] **Step 1: Add the column to the migration**

In `internal/db/migrations/20260503000001_initial.up.sql`, find the `user_sync_configs` CREATE TABLE block (around line 205). Add `credentials_error` after `last_synced_at`:

```sql
CREATE TABLE user_sync_configs (
    id                     TEXT PRIMARY KEY,
    user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront             TEXT NOT NULL,
    frequency              TEXT NOT NULL DEFAULT 'manual',
    storefront_credentials TEXT,
    last_synced_at         TIMESTAMPTZ,
    credentials_error      BOOLEAN NOT NULL DEFAULT false,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront)
);
```

- [ ] **Step 2: Add `CredentialsError` to `UserSyncConfig` model**

In `internal/db/models/models.go`, find `UserSyncConfig` (line 187). Add `CredentialsError` after `LastSyncedAt`:

```go
type UserSyncConfig struct {
	bun.BaseModel `bun:"table:user_sync_configs"`

	ID                    string     `bun:"id,pk"                  json:"id"`
	UserID                string     `bun:"user_id,notnull"         json:"user_id"`
	Storefront            string     `bun:"storefront,notnull"      json:"storefront"`
	Frequency             string     `bun:"frequency,notnull"       json:"frequency"`
	StorefrontCredentials *string    `bun:"storefront_credentials"  json:"-"`
	LastSyncedAt          *time.Time `bun:"last_synced_at"          json:"last_synced_at"`
	CredentialsError      bool       `bun:"credentials_error"       json:"-"`
	CreatedAt             time.Time  `bun:"created_at,notnull"      json:"created_at"`
	UpdatedAt             time.Time  `bun:"updated_at,notnull"      json:"updated_at"`
}
```

- [ ] **Step 3: Verify the build compiles**

```bash
cd /home/abo/workspace/home/nexorious && go build ./... 2>&1
```

Expected: no errors.

- [ ] **Step 4: Run all Go tests to confirm the schema is picked up**

```bash
go test ./internal/api/... -timeout 300s 2>&1 | tail -10
```

Expected: all tests pass (the migration runs automatically via `TestMain`).

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/20260503000001_initial.up.sql internal/db/models/models.go
git commit -m "feat(sync): add credentials_error column to user_sync_configs"
```

---

## Task 4: Part B worker — set/clear `credentials_error` in `DispatchSyncWorker`

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write the failing tests**

Add three tests to `internal/worker/tasks/sync_test.go`:

```go
func TestDispatchSync_FactoryCredentialsError_SetsFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: credErrFactory(), RiverClient: nil}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(ctx, &credsErr)
	if !credsErr {
		t.Error("expected credentials_error=true after factory ErrCredentials")
	}
}

func TestDispatchSync_FetchCredentialsError_SetsFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'steam', 'pending', 'low')`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'steam', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx)

	w := &tasks.DispatchSyncWorker{
		DB:          testDB,
		Adapter:     fetchErrFactory(tasks.ErrCredentials),
		RiverClient: nil,
	}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(ctx, &credsErr)
	if !credsErr {
		t.Error("expected credentials_error=true after fetch ErrCredentials")
	}
}

func TestDispatchSync_Success_ClearsCredentialsFlag(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	jobID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	_, _ = testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'steam', 'pending', 'low', 0)`,
		jobID, userID,
	)
	_, _ = testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, credentials_error)
		 VALUES (?, ?, 'steam', 'daily', true)`,
		uuid.NewString(), userID,
	).Exec(ctx)

	fakeAdapter := &fakeStorefrontAdapter{batches: [][]tasks.ExternalGameEntry{}}
	rc := newTestRiverClient(t)
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(fakeAdapter), RiverClient: rc}
	job := &river.Job[tasks.DispatchSyncArgs]{
		Args: tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "steam"},
	}
	if err := w.Work(ctx, job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(ctx, &credsErr)
	if credsErr {
		t.Error("expected credentials_error=false after successful sync")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/worker/tasks/... -run "TestDispatchSync_FactoryCredentialsError_SetsFlag|TestDispatchSync_FetchCredentialsError_SetsFlag|TestDispatchSync_Success_ClearsCredentialsFlag" -v 2>&1 | tail -20
```

Expected: FAIL — the column read returns `false` in all three cases.

- [ ] **Step 3: Update the factory-ErrCredentials site in `sync.go` (line ~162)**

In `internal/worker/tasks/sync.go`, find the first `ErrCredentials` check (after `w.Adapter(...)`):

```go
if errors.Is(err, ErrCredentials) {
	failSyncJob(ctx, w.DB, p.JobID, "credentials error")
	_, _ = w.DB.NewRaw(
		`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		p.UserID, p.Storefront,
	).Exec(ctx)
	return nil
}
```

- [ ] **Step 4: Update the fetch-ErrCredentials site (line ~199)**

Find the second `ErrCredentials` check (inside the `if err := adapter.GetLibrary(...); err != nil` block):

```go
if errors.Is(err, ErrCredentials) {
	failSyncJob(ctx, w.DB, p.JobID, "credentials error")
	_, _ = w.DB.NewRaw(
		`UPDATE user_sync_configs SET credentials_error = true, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		p.UserID, p.Storefront,
	).Exec(ctx)
	return nil
}
```

- [ ] **Step 5: Update the `last_synced_at` UPDATE to also clear the flag**

Find step 8 (`// 8. Update last_synced_at.`) and replace the raw SQL:

```go
syncedNow := time.Now().UTC()
if _, err := w.DB.NewRaw(
	`UPDATE user_sync_configs SET last_synced_at = ?, credentials_error = false, updated_at = now() WHERE user_id = ? AND storefront = ?`,
	syncedNow, p.UserID, p.Storefront,
).Exec(context.Background()); err != nil {
	slog.Error("dispatch_sync: update last_synced_at failed", "err", err, "job_id", p.JobID)
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
go test ./internal/worker/tasks/... -run "TestDispatchSync_FactoryCredentialsError_SetsFlag|TestDispatchSync_FetchCredentialsError_SetsFlag|TestDispatchSync_Success_ClearsCredentialsFlag" -v 2>&1 | tail -20
```

Expected: all three tests PASS.

- [ ] **Step 7: Run the full worker test suite**

```bash
go test ./internal/worker/tasks/... -timeout 300s 2>&1 | tail -10
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "feat(sync): persist credentials_error flag in DispatchSyncWorker"
```

---

## Task 5: Part B status endpoints — read `credentials_error` from DB column

**Files:**
- Modify: `internal/api/sync.go`
- Modify: `internal/api/sync_test.go`

The four handlers to update are `HandleGetSteamConnection`, `HandleGetPSNStatus`, `HandleGetGOGConnection`, and `HandleGetEpicConnection`. In each, the final successful-decrypt response path must include `row.CredentialsError`.

- [ ] **Step 1: Write failing tests**

Add to `internal/api/sync_test.go`:

```go
func TestGetSteamConnection_DBCredentialsErrorFlag(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "sc-db-cred-err")

	rawCreds := `{"steam_id":"76561198000000001","web_api_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","display_name":"TestUser"}`
	ciphertext, err := testEncrypter.Encrypt([]byte(rawCreds))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, credentials_error, created_at, updated_at)
		 VALUES (?, ?, 'steam', 'manual', ?, true, now(), now())`,
		uuid.NewString(), userID, ciphertext,
	)

	rec := getAuth(t, e, "/api/sync/steam/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["credentials_error"] != true {
		t.Errorf("expected credentials_error=true from DB flag, got %v", resp["credentials_error"])
	}
}

func TestGetPSNStatus_DBCredentialsErrorFlag(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "psn-db-cred-err")

	rawCreds := `{"npsso_token":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","online_id":"MyPSN","account_id":"123","region":"GB","is_verified":true,"token_expired_at":null}`
	ciphertext, _ := testEncrypter.Encrypt([]byte(rawCreds))
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, credentials_error, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'manual', ?, true, now(), now())`,
		uuid.NewString(), userID, ciphertext,
	)

	rec := getAuth(t, e, "/api/sync/psn/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["credentials_error"] != true {
		t.Errorf("expected credentials_error=true from DB flag, got %v", resp["credentials_error"])
	}
}

func TestGetGOGConnection_DBCredentialsErrorFlag(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "gog-db-cred-err")

	rawCreds := `{"access_token":"aaa","refresh_token":"bbb","user_id":"u1","username":"GogUser"}`
	ciphertext, _ := testEncrypter.Encrypt([]byte(rawCreds))
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, credentials_error, created_at, updated_at)
		 VALUES (?, ?, 'gog', 'manual', ?, true, now(), now())`,
		uuid.NewString(), userID, ciphertext,
	)

	rec := getAuth(t, e, "/api/sync/gog/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["credentials_error"] != true {
		t.Errorf("expected credentials_error=true from DB flag, got %v", resp["credentials_error"])
	}
}

func TestGetEpicConnection_DBCredentialsErrorFlag(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "epic-db-cred-err")

	rawCreds := `{"user.json":"{\"displayName\":\"EpicUser\",\"account_id\":\"abc123\"}"}`
	ciphertext, _ := testEncrypter.Encrypt([]byte(rawCreds))
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, credentials_error, created_at, updated_at)
		 VALUES (?, ?, 'epic', 'manual', ?, true, now(), now())`,
		uuid.NewString(), userID, ciphertext,
	)

	rec := getAuth(t, e, "/api/sync/epic/connection", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["credentials_error"] != true {
		t.Errorf("expected credentials_error=true from DB flag, got %v", resp["credentials_error"])
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/api/... -run "TestGetSteamConnection_DBCredentialsErrorFlag|TestGetPSNStatus_DBCredentialsErrorFlag|TestGetGOGConnection_DBCredentialsErrorFlag|TestGetEpicConnection_DBCredentialsErrorFlag" -v 2>&1 | tail -20
```

Expected: FAIL — `credentials_error` is nil/false in all four responses.

- [ ] **Step 3: Update `HandleGetSteamConnection`**

Find the final return in `HandleGetSteamConnection` (line ~576). Change:

```go
return c.JSON(http.StatusOK, steamConnectionResponse{
	Connected: true,
	SteamID:   creds.SteamID,
	Username:  creds.DisplayName,
})
```

To:

```go
return c.JSON(http.StatusOK, steamConnectionResponse{
	Connected:        true,
	SteamID:          creds.SteamID,
	Username:         creds.DisplayName,
	CredentialsError: row.CredentialsError,
})
```

- [ ] **Step 4: Update `HandleGetPSNStatus`**

Find the final return in `HandleGetPSNStatus` (line ~676). Change:

```go
return c.JSON(http.StatusOK, psnStatusResponse{
	IsConfigured:     true,
	CredentialsError: !creds.IsVerified,
	OnlineID:         creds.OnlineID,
	AccountID:        creds.AccountID,
	Region:           creds.Region,
})
```

To:

```go
return c.JSON(http.StatusOK, psnStatusResponse{
	IsConfigured:     true,
	CredentialsError: row.CredentialsError,
	OnlineID:         creds.OnlineID,
	AccountID:        creds.AccountID,
	Region:           creds.Region,
})
```

- [ ] **Step 5: Update `HandleGetGOGConnection`**

Find the final return in `HandleGetGOGConnection` (line ~1418). Change:

```go
return c.JSON(http.StatusOK, map[string]any{
	"connected": true,
	"username":  creds.Username,
	"user_id":   creds.UserID,
	"auth_url":  authURL,
})
```

To:

```go
return c.JSON(http.StatusOK, map[string]any{
	"connected":        true,
	"username":         creds.Username,
	"user_id":          creds.UserID,
	"auth_url":         authURL,
	"credentials_error": row.CredentialsError,
})
```

- [ ] **Step 6: Update `HandleGetEpicConnection`**

Find the final return in `HandleGetEpicConnection` (line ~821). The handler unmarshals snapshot JSON into a struct with `DisplayName` and `AccountID`:

```go
var creds struct {
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
}
if err := json.Unmarshal(plainCreds, &creds); err != nil {
```

Wait — Epic stores the legendary **snapshot** (a `map[string]string` of config files), not a struct with `display_name`. The current unmarshal target is wrong and will silently produce empty strings. Looking at the code, it tries to unmarshal the snapshot map as a `display_name`/`account_id` struct. This won't error (JSON ignores unknown keys) but it won't populate the fields either. This is a pre-existing bug — the `display_name` returned is always empty. Do not fix this pre-existing bug in this task. Just add `credentials_error` to the final response:

```go
return c.JSON(http.StatusOK, map[string]any{
	"connected":         true,
	"disabled":          false,
	"display_name":      creds.DisplayName,
	"account_id":        creds.AccountID,
	"credentials_error": row.CredentialsError,
})
```

- [ ] **Step 7: Run the tests to verify they pass**

```bash
go test ./internal/api/... -run "TestGetSteamConnection_DBCredentialsErrorFlag|TestGetPSNStatus_DBCredentialsErrorFlag|TestGetGOGConnection_DBCredentialsErrorFlag|TestGetEpicConnection_DBCredentialsErrorFlag" -v 2>&1 | tail -20
```

Expected: all four tests PASS.

- [ ] **Step 8: Run the full API test suite**

```bash
go test ./internal/api/... -timeout 300s 2>&1 | tail -10
```

Expected: all tests pass. If `TestPSNStatus_WithCredentials` fails because it now gets `credentials_error=false` when `is_verified=true`, that is correct behaviour — update the test to remove the `!is_verified` assertion if it exists, since that field is no longer the source of truth.

- [ ] **Step 9: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): status endpoints read credentials_error from DB column"
```

---

## Task 6: Part B save handlers — clear `credentials_error` on new credentials

**Files:**
- Modify: `internal/api/sync.go`
- Modify: `internal/api/sync_test.go`

All four credential-save handlers use `ON CONFLICT ... DO UPDATE`. We extend the conflict clause to also set `credentials_error = false`, so reconnecting always clears a stale error.

- [ ] **Step 1: Write failing tests**

Add to `internal/api/sync_test.go`:

```go
func TestSteamVerify_ClearsCredentialsErrorFlag(t *testing.T) {
	truncateAllTables(t)
	stub := &stubSteamClient{
		summary: &api.SteamPlayerSummary{PersonaName: "TestUser", CommunityVisibilityState: 3},
	}
	e := newSyncTestApp(t, testDB, stub, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "sv-clear-cred")

	// Seed a pre-existing row with credentials_error=true.
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, credentials_error, created_at, updated_at)
		 VALUES (?, ?, 'steam', 'manual', true, now(), now())`,
		uuid.NewString(), userID,
	)

	body := `{"steam_id":"76561198000000001","web_api_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`
	rec := postAuth(t, e, "/api/sync/steam/verify", token, strings.NewReader(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Scan(context.Background(), &credsErr)
	if credsErr {
		t.Error("expected credentials_error=false after successful Steam verify, got true")
	}
}

func TestPSNConfigure_ClearsCredentialsErrorFlag(t *testing.T) {
	truncateAllTables(t)
	stub := &stubPSNClient{
		info: &api.PSNAccountInfo{OnlineID: "MyPSN", AccountID: "123", Region: "GB"},
	}
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, stub)
	userID, token := setupTagUser(t, testDB, e, "psn-clear-cred")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, credentials_error, created_at, updated_at)
		 VALUES (?, ?, 'psn', 'manual', true, now(), now())`,
		uuid.NewString(), userID,
	)

	body := `{"npsso_token":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}`
	rec := postAuth(t, e, "/api/sync/psn/configure", token, strings.NewReader(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var credsErr bool
	_ = testDB.NewRaw(
		`SELECT credentials_error FROM user_sync_configs WHERE user_id = ? AND storefront = 'psn'`,
		userID,
	).Scan(context.Background(), &credsErr)
	if credsErr {
		t.Error("expected credentials_error=false after PSN configure, got true")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/api/... -run "TestSteamVerify_ClearsCredentialsErrorFlag|TestPSNConfigure_ClearsCredentialsErrorFlag" -v 2>&1 | tail -20
```

Expected: FAIL — the flag is not cleared.

- [ ] **Step 3: Update `HandleSteamVerify` ON CONFLICT clause**

In `HandleSteamVerify` (around line 516), update the `.On(...)` string:

```go
if _, err := h.db.NewInsert().Model(row).
	On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, credentials_error = false, updated_at = EXCLUDED.updated_at").
	Exec(context.Background()); err != nil {
```

- [ ] **Step 4: Update `HandlePSNConfigure` ON CONFLICT clause**

In `HandlePSNConfigure` (around line 626), update the `.On(...)` string:

```go
if _, err := h.db.NewInsert().Model(row).
	On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, credentials_error = false, updated_at = EXCLUDED.updated_at").
	Exec(context.Background()); err != nil {
```

- [ ] **Step 5: Update `HandleGOGConnect` ON CONFLICT clause**

In `HandleGOGConnect` (around line 1351), update the `.On(...)` string:

```go
if _, err := h.db.NewInsert().Model(row).
	On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, credentials_error = false, updated_at = EXCLUDED.updated_at").
	Exec(context.Background()); err != nil {
```

- [ ] **Step 6: Update `HandleEpicConnect` raw SQL ON CONFLICT clause**

In `HandleEpicConnect` (around line 736), update the raw SQL:

```go
if _, err := h.db.NewRaw(
	`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
	 VALUES (?, ?, 'epic', 'manual', ?, ?, ?)
	 ON CONFLICT (user_id, storefront) DO UPDATE SET
	     storefront_credentials = EXCLUDED.storefront_credentials,
	     credentials_error = false,
	     updated_at = EXCLUDED.updated_at`,
	uuid.NewString(), userID, stateCiphertext, now, now,
).Exec(context.Background()); err != nil {
```

- [ ] **Step 7: Run the tests to verify they pass**

```bash
go test ./internal/api/... -run "TestSteamVerify_ClearsCredentialsErrorFlag|TestPSNConfigure_ClearsCredentialsErrorFlag" -v 2>&1 | tail -20
```

Expected: both PASS.

- [ ] **Step 8: Run the full API test suite**

```bash
go test ./internal/api/... -timeout 300s 2>&1 | tail -10
```

Expected: all tests pass.

- [ ] **Step 9: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): credential-save handlers clear credentials_error flag"
```

---

## Task 7: Part B Steam — `ErrAPIKeyRejected` sentinel

**Files:**
- Modify: `internal/services/steam/client.go`
- Modify: `internal/services/steam/adapter.go`
- Create: `internal/services/steam/adapter_test.go`
- Modify: `internal/services/steam/client_test.go`

- [ ] **Step 1: Write the failing client test**

Add to `internal/services/steam/client_test.go`:

```go
func TestGetOwnedGames_403_ReturnsErrAPIKeyRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetOwnedGames(context.Background(), "badkey", "steamid")
	if !errors.Is(err, steam.ErrAPIKeyRejected) {
		t.Errorf("expected ErrAPIKeyRejected on 403, got %v", err)
	}
}

func TestGetOwnedGames_401_ReturnsErrAPIKeyRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	_, err := c.GetOwnedGames(context.Background(), "badkey", "steamid")
	if !errors.Is(err, steam.ErrAPIKeyRejected) {
		t.Errorf("expected ErrAPIKeyRejected on 401, got %v", err)
	}
}
```

- [ ] **Step 2: Write the failing adapter test**

Create `internal/services/steam/adapter_test.go`:

```go
package steam_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"

	"github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

func TestSteamAdapter_APIKeyRejected_ReturnsErrCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := steam.NewClientForTests(srv.Client(), rate.NewLimiter(rate.Inf, 1), srv.URL, srv.URL)
	a := steam.NewAdapter(c, "badkey", "76561198000000001")

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials, got %v", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
go test ./internal/services/steam/... -v 2>&1 | tail -20
```

Expected: compilation error (`steam.ErrAPIKeyRejected` undefined).

- [ ] **Step 4: Add `ErrAPIKeyRejected` sentinel to `client.go`**

In `internal/services/steam/client.go`, alongside `ErrRateLimited` (line ~156):

```go
// ErrAPIKeyRejected is returned by GetOwnedGames when the Steam API responds
// with HTTP 401 or 403, indicating the API key is invalid or revoked.
var ErrAPIKeyRejected = errors.New("steam: API key rejected")
```

- [ ] **Step 5: Return the sentinel in `GetOwnedGames`**

In `GetOwnedGames`, replace:

```go
if resp.StatusCode != http.StatusOK {
	return nil, fmt.Errorf("steam HTTP %d", resp.StatusCode)
}
```

With:

```go
if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
	return nil, ErrAPIKeyRejected
}
if resp.StatusCode != http.StatusOK {
	return nil, fmt.Errorf("steam HTTP %d", resp.StatusCode)
}
```

- [ ] **Step 6: Wrap the sentinel in `adapter.go`**

In `internal/services/steam/adapter.go`, find `GetLibrary`. After `GetOwnedGames`:

```go
owned, err := a.client.GetOwnedGames(ctx, a.apiKey, a.steamID)
if errors.Is(err, ErrAPIKeyRejected) {
	return fmt.Errorf("%w: steam API key rejected", storefrontadapter.ErrCredentials)
}
if err != nil {
	return fmt.Errorf("steam: fetch owned games: %w", err)
}
```

(Remove the existing `if err != nil { return fmt.Errorf("steam: fetch owned games: %w", err) }` and replace with the three-line block above.)

- [ ] **Step 7: Run the tests to verify they pass**

```bash
go test ./internal/services/steam/... -v 2>&1 | tail -20
```

Expected: all tests pass, including the two new client tests and the adapter test.

- [ ] **Step 8: Run the full test suite**

```bash
go test ./... -timeout 300s 2>&1 | tail -10
```

Expected: all tests pass.

- [ ] **Step 9: Commit**

```bash
git add internal/services/steam/client.go internal/services/steam/adapter.go \
        internal/services/steam/client_test.go internal/services/steam/adapter_test.go
git commit -m "feat(sync): Steam adapter returns ErrCredentials on API key rejection"
```

---

## Task 8: Part B Epic — `ErrAuthFailed` sentinel

**Files:**
- Modify: `internal/services/epic/client.go`
- Modify: `internal/services/epic/adapter.go`
- Modify: `internal/services/epic/adapter_test.go`

- [ ] **Step 1: Write the failing adapter test**

Add to `internal/services/epic/adapter_test.go`:

```go
func TestEpicAdapter_LegendaryAuthFailure_ReturnsErrCredentials(t *testing.T) {
	authErr := fmt.Errorf("epic: legendary list --json: ERROR: Not logged in. Please login first.")
	fake := &fakeEpicClient{
		configured:    true,
		getLibraryErr: epic.ErrAuthFailed,
		captureSnapshot: map[string]string{},
	}
	a := NewAdapter(fake, "user1", map[string]string{}, nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials, got %v", err)
	}
	_ = authErr
}
```

Note: the test sets `getLibraryErr: epic.ErrAuthFailed` on the fake client — `ErrAuthFailed` must be exported from the `epic` package and reachable from `adapter_test.go` (which is in `package epic` — the internal package test).

Update the import block in `adapter_test.go` to add `"fmt"` if the compiler requires it:

```go
import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/services/epic/... -run "TestEpicAdapter_LegendaryAuthFailure_ReturnsErrCredentials" -v 2>&1 | tail -20
```

Expected: compilation error (`epic.ErrAuthFailed` undefined).

- [ ] **Step 3: Add `ErrAuthFailed` and `isAuthError` to `client.go`**

In `internal/services/epic/client.go`, add before `runLegendary`:

```go
// ErrAuthFailed is returned by GetLibrary when the legendary CLI exits with
// an authentication-related error (session expired, not logged in, token refresh failed).
var ErrAuthFailed = errors.New("epic: legendary authentication failed")

// isAuthError reports whether the error message from a legendary subprocess
// indicates an authentication failure. This is a best-effort heuristic based
// on known legendary error patterns.
func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not logged in") ||
		strings.Contains(msg, "login session") ||
		strings.Contains(msg, "token failed") ||
		strings.Contains(msg, "please login")
}
```

- [ ] **Step 4: Return `ErrAuthFailed` in `GetLibrary` on auth errors**

In `GetLibrary` (in `client.go`), replace:

```go
out, err := c.runLegendary(ctx, userID, "list", "--json")
if err != nil {
	return err
}
```

With:

```go
out, err := c.runLegendary(ctx, userID, "list", "--json")
if err != nil {
	if isAuthError(err) {
		return ErrAuthFailed
	}
	return err
}
```

- [ ] **Step 5: Wrap `ErrAuthFailed` as `ErrCredentials` in `adapter.go`**

In `internal/services/epic/adapter.go`, find the `GetLibrary` method. After the `fetchErr := a.client.GetLibrary(...)` call, and before the snapshot capture, add a check. The snapshot capture block is:

```go
fetchErr := a.client.GetLibrary(ctx, a.userID, func(batch []ExternalGameEntry) error {
	// ... mapping ...
})

// Capture updated snapshot regardless of fetch error.
newSnapshot, captureErr := a.client.CaptureSnapshot(a.userID)
// ...
return fetchErr
```

Replace `return fetchErr` with:

```go
if errors.Is(fetchErr, ErrAuthFailed) {
	return fmt.Errorf("%w: epic legendary auth failure", storefrontadapter.ErrCredentials)
}
return fetchErr
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
go test ./internal/services/epic/... -v 2>&1 | tail -20
```

Expected: all tests pass, including `TestEpicAdapter_LegendaryAuthFailure_ReturnsErrCredentials` and all existing adapter tests.

Also verify `TestEpicAdapter_PersistsSnapshotEvenOnFetchError` still passes — the fake client's `getLibraryErr` is a plain `errors.New("library fetch failed")` which does NOT satisfy `errors.Is(err, ErrAuthFailed)`, so `fetchErr` is returned as-is and the snapshot is still captured. ✓

- [ ] **Step 7: Run the full test suite**

```bash
go test ./... -timeout 300s 2>&1 | tail -10
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/services/epic/client.go internal/services/epic/adapter.go \
        internal/services/epic/adapter_test.go
git commit -m "feat(sync): Epic adapter returns ErrCredentials on legendary auth failure"
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Task |
|---|---|
| Part A — Platform column in API response | Task 1 |
| Part A — `ExternalGame.platforms` frontend type | Task 2 |
| Part A — Platform column in Matched UI table | Task 2 |
| Part B — `credentials_error` schema column | Task 3 |
| Part B — Worker sets flag on ErrCredentials (both sites) | Task 4 |
| Part B — Worker clears flag on success | Task 4 |
| Part B — Status endpoints read from DB column | Task 5 |
| Part B — Save handlers clear flag | Task 6 |
| Part B — Steam `ErrAPIKeyRejected` | Task 7 |
| Part B — Epic `ErrAuthFailed` | Task 8 |

All spec requirements covered. ✓

**Consistency check:**
- `row.CredentialsError` references `UserSyncConfig.CredentialsError bool` added in Task 3 — used in Tasks 5 and 6.
- `ErrAPIKeyRejected` defined in Task 7 (`client.go`) and referenced in `adapter.go` same task — both in `package steam`. ✓
- `ErrAuthFailed` defined in Task 8 (`client.go`) and referenced in `adapter.go` same task — both in `package epic`. ✓
- `storefrontadapter.ErrCredentials` is the existing sentinel from `internal/services/storefrontadapter/storefrontadapter.go` — used in Tasks 7 and 8. ✓
- `strings.Split` in Task 1 requires `"strings"` import added in the same step. ✓
- `isAuthError` in Task 8 calls `strings.ToLower` and `strings.Contains` — `strings` is already imported in `client.go`. Verify with `grep '"strings"' internal/services/epic/client.go` before the task; add the import if absent. ✓
