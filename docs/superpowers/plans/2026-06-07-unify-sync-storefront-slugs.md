# Unify Sync-Source Slugs with `storefronts.name` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a storefront's sync-source slug equal its canonical `storefronts.name` everywhere (`epic`→`epic-games-store`, `psn`→`playstation-store`; `steam`/`gog`/`humble-bundle` unchanged), delete the `StorefrontToCollectionSlug` bridge, and derive all storefront display names from the `storefronts` catalog instead of parallel local maps.

**Architecture:** The `storefronts` catalog table is the single source of truth for a storefront's identity (`name`) and human-readable label (`display_name`). After this change there is exactly one storefront vocabulary: the catalog `name`. The slug appears unchanged in the URL, API paths, the adapter-dispatch switch, and persisted columns — all now equal to `storefronts.name`. A one-shot data migration rewrites the two divergent slugs in the three columns that store the *unresolved* slug (`external_games.storefront`, `user_sync_configs.storefront`, `jobs.source`) plus any in-flight `dispatch_sync` River job args. Display names are read from the catalog via the existing `useStorefront`/`useStorefronts` TanStack Query hooks (Infinity staleTime).

**Tech Stack:** Go 1.26 (Echo v5, Bun ORM, Bun migrate, River), PostgreSQL, React 19 + TypeScript (TanStack Router/Query), Vitest.

**Clean cutover:** No transitional acceptance of the old slugs. After deploy, `epic`/`psn` are invalid everywhere.

---

## Background facts (verified — do not re-derive)

- `user_game_platforms.storefront` already stores the **resolved** catalog name (written after `StorefrontToCollectionSlug`). It does **NOT** need migrating.
- The three columns storing the **unresolved** slug that DO need migrating: `external_games.storefront`, `user_sync_configs.storefront`, `jobs.source`.
- The slug is also persisted in River job args: `river_job.args->>'storefront'` for `kind = 'dispatch_sync'` (inserted at `internal/api/sync.go:564`). Pending jobs at upgrade time must be patched.
- `storefronts` catalog canonical names (seeded in `internal/db/migrations/20260503000001_initial.up.sql:314`): `steam`, `epic-games-store`, `gog`, `playstation-store`, `humble-bundle` (display names `Steam`, `Epic Games Store`, `GOG`, `PlayStation Store`, `Humble Bundle`).
- `platformresolution` package keeps `igdb_ids.go` (`IGDBPlatformIDsForExternalGame`); only `resolution.go` (`StorefrontToCollectionSlug`) + its tests are deleted. The package survives.
- `internal/services/darkadia/darkadia.go:200-204` already maps to canonical names (`epic-games-store`, `playstation-store`) — it is **import mapping, not a sync slug**. Do NOT touch it.
- Most frontend `epic`/`psn` occurrences are the **catalog** names `epic-games-store`/`playstation-store` (platform-selector, platform-reconcile, etc.) and already match `storefronts.name`. Do NOT change those. Only the sync-slug occurrences listed per task change.

## TS enum representation decision

TanStack/TS enum member identifiers cannot contain hyphens. To honor "no second vocabulary" while staying idiomatic (the codebase uses enums for `SyncFrequency`, `JobStatus`, `JobSource`), keep the `enum` but make each member identifier the **canonical slug in SCREAMING_SNAKE_CASE** whose value is the canonical slug:
- `PLAYSTATION_STORE = 'playstation-store'` (was `PSN = 'psn'`)
- `EPIC_GAMES_STORE = 'epic-games-store'` (was `EPIC = 'epic'`)

The identifier is now the same token as the value (not a different word like `PSN`), so it is not a translation. `STEAM`/`GOG`/`HUMBLE` are unchanged (their identifiers were never a different word — `HUMBLE` is a short identifier for the hyphenated `humble-bundle`, not a synonym).

## Deviation note for plan review

During brainstorming we said job-source labels would derive storefront names from the catalog. Implementation detail discovered: `getJobSourceLabel` is a **pure function** used in card/list contexts and cannot call hooks. Task 9 therefore replaces it with a `useJobSourceLabel()` hook that derives storefront-typed labels from the catalog and keeps a static label map only for the genuinely non-storefront origins (`manual`, `darkadia`, `nexorious`, `csv`, `system`) which are not storefronts and not in the catalog. If you'd prefer to leave job-source labels as a static (catalog-consistent) map and treat full job-label derivation as a follow-up, that is a smaller alternative — confirm before executing Task 9.

## File map

| File | Change |
|---|---|
| `internal/db/migrations/20260605000003_unify_sync_storefront_slugs.up.sql` | Create — data + river_job rewrite |
| `internal/db/migrations/20260605000003_unify_sync_storefront_slugs.down.sql` | Create — inverse |
| `internal/db/models/jobs.go` | Modify — `JobSourceEpic`/`JobSourcePSN` constant values + names |
| `internal/api/sync.go` | Modify — `supportedStorefronts`, delete `storefrontDisplayName`, route literals, per-handler slug literals |
| `cmd/nexorious/serve.go` | Modify — adapter-dispatch switch `case` labels |
| `internal/services/platformresolution/resolution.go` | Delete |
| `internal/services/platformresolution/resolution_test.go` | Delete |
| `internal/worker/tasks/sync.go` | Modify — drop `StorefrontToCollectionSlug` call at `:717`, use `eg.Storefront` directly |
| `internal/worker/tasks/sync_test.go` | Modify — old→new slug fixtures |
| `internal/api/sync_test.go`, `internal/api/jobs_test.go`, `internal/api/job_items_test.go` | Modify — old→new slug fixtures |
| `ui/frontend/src/types/sync.ts` | Modify — enum member rename, delete `getStorefrontDisplayInfo` |
| `ui/frontend/src/types/jobs.ts` | Modify — `JobSource` member rename, delete `getJobSourceLabel` |
| `ui/frontend/src/api/sync.ts` | Modify — `/sync/epic/` `/sync/psn/` path literals |
| `ui/frontend/src/hooks/use-sync.ts` | Modify — `SyncStorefront.EPIC/PSN` refs |
| `ui/frontend/src/hooks/use-job-source-label.ts` | Create — catalog-aware label hook |
| `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` | Modify — head map → catalog, `getStorefrontDisplayInfo` → `useStorefront`, enum refs |
| `ui/frontend/src/routes/_authenticated/sync/index.tsx` | Modify — enum refs |
| `ui/frontend/src/components/sync/sync-service-card.tsx` | Modify — `getStorefrontDisplayInfo` → `useStorefront`, neutral swatch |
| `ui/frontend/src/components/jobs/recent-activity.tsx`, `job-card.tsx`, `job-progress-card.tsx` | Modify — `getJobSourceLabel` → `useJobSourceLabel` |
| `ui/frontend/src/types/sync.test.ts` | Modify/trim — drop `getStorefrontDisplayInfo` tests, update enum value asserts |
| `ui/frontend/src/routeTree.gen.ts` | Regenerate if any route path string changes (the `$storefront` param path does not change — verify) |

---

## Task 1: Data migration (up + down)

**Files:**
- Create: `internal/db/migrations/20260605000003_unify_sync_storefront_slugs.up.sql`
- Create: `internal/db/migrations/20260605000003_unify_sync_storefront_slugs.down.sql`

Bun discovers these automatically via `Migrations.Discover(FS)` — no registration needed.

- [ ] **Step 1: Write the up migration**

Create `internal/db/migrations/20260605000003_unify_sync_storefront_slugs.up.sql`:

```sql
-- Unify sync-source slugs with storefronts.name (issue #850).
-- Renames the two divergent sync slugs (epic -> epic-games-store, psn ->
-- playstation-store) in the three columns that store the UNRESOLVED slug, plus
-- any in-flight dispatch_sync River job args. user_game_platforms.storefront is
-- NOT touched: it already stores the resolved catalog name.
--
-- Safe against the unique constraints on user_sync_configs (user_id, storefront)
-- and external_games (user_id, storefront, external_id): the target values
-- 'playstation-store'/'epic-games-store' were never valid sync slugs before, so
-- no colliding row can pre-exist.

UPDATE external_games   SET storefront = 'playstation-store' WHERE storefront = 'psn';
UPDATE external_games   SET storefront = 'epic-games-store'  WHERE storefront = 'epic';

UPDATE user_sync_configs SET storefront = 'playstation-store' WHERE storefront = 'psn';
UPDATE user_sync_configs SET storefront = 'epic-games-store'  WHERE storefront = 'epic';

UPDATE jobs SET source = 'playstation-store' WHERE source = 'psn';
UPDATE jobs SET source = 'epic-games-store'  WHERE source = 'epic';

-- River manages its own schema and may not exist yet on a fresh install when
-- this migration runs; guard with to_regclass so fresh installs don't fail.
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"playstation-store"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'psn';
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"epic-games-store"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'epic';
  END IF;
END $$;
```

- [ ] **Step 2: Write the down migration**

Create `internal/db/migrations/20260605000003_unify_sync_storefront_slugs.down.sql`:

```sql
-- Reverse of 20260605000003 up. Safe: post-up, the only 'playstation-store' /
-- 'epic-games-store' values in these three columns originated from the sync
-- slug, so reverting them to 'psn'/'epic' is the exact inverse.

UPDATE external_games   SET storefront = 'psn'  WHERE storefront = 'playstation-store';
UPDATE external_games   SET storefront = 'epic' WHERE storefront = 'epic-games-store';

UPDATE user_sync_configs SET storefront = 'psn'  WHERE storefront = 'playstation-store';
UPDATE user_sync_configs SET storefront = 'epic' WHERE storefront = 'epic-games-store';

UPDATE jobs SET source = 'psn'  WHERE source = 'playstation-store';
UPDATE jobs SET source = 'epic' WHERE source = 'epic-games-store';

DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"psn"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'playstation-store';
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"epic"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'epic-games-store';
  END IF;
END $$;
```

- [ ] **Step 3: Verify migrations apply cleanly on a fresh DB**

Run: `go test ./internal/migrate/... -run TestRunMigrations -v`
Expected: PASS. `TestRunMigrations_AllTablesExist` / `TestRunMigrations_TransitionsToReady` run the full set including the new migration; this catches SQL syntax errors and confirms the `to_regclass` guard does not fail when `river_job` is absent.

- [ ] **Step 4: Manual data-transform verification (documented, not automated)**

The repo has no per-migration data-transform test harness (each test package runs the full migration set once at startup), and the transform is simple deterministic `UPDATE`s. Verify by hand against a scratch DB before merging:

```bash
# In a psql connected to a scratch DB migrated to 20260605000002 (the prior migration):
#   INSERT a user, then rows with OLD slugs:
#   INSERT INTO external_games (...) VALUES (..., 'psn', ...), (..., 'epic', ...);
#   INSERT INTO user_sync_configs (...) VALUES (..., 'psn'), (..., 'epic');
#   INSERT INTO jobs (...) VALUES (..., source='psn'), (..., source='epic');
# Then run the up migration and assert:
#   SELECT DISTINCT storefront FROM external_games;     -- no 'psn'/'epic'
#   SELECT DISTINCT storefront FROM user_sync_configs;  -- no 'psn'/'epic'
#   SELECT DISTINCT source FROM jobs WHERE source IN ('psn','epic');  -- 0 rows
```

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/20260605000003_unify_sync_storefront_slugs.up.sql \
        internal/db/migrations/20260605000003_unify_sync_storefront_slugs.down.sql
git commit -m "feat: migrate sync-source slugs to canonical storefronts.name"
```

---

## Task 2: Backend job-source constants

**Files:**
- Modify: `internal/db/models/jobs.go:22-24`

- [ ] **Step 1: Rename the two diverging constants**

In `internal/db/models/jobs.go`, change:

```go
	JobSourceEpic         = "epic"
	JobSourcePSN          = "psn"
```

to:

```go
	JobSourceEpicGamesStore  = "epic-games-store"
	JobSourcePlaystationStore = "playstation-store"
```

(Keep `JobSourceSteam`, `JobSourceGOG`, `JobSourceHumbleBundle`, `JobSourceManual`, etc. unchanged. Re-align the `=` columns with `gofmt`, which the edit hook runs automatically.)

- [ ] **Step 2: Update every reference to the renamed constants**

Run: `grep -rn "JobSourceEpic\b\|JobSourcePSN\b" --include=*.go internal/ cmd/`
Expected after Step 1: only stale references remain. Replace each `JobSourceEpic` → `JobSourceEpicGamesStore` and `JobSourcePSN` → `JobSourcePlaystationStore`. (At authoring time the only occurrences were the definitions themselves; confirm none were added.)

- [ ] **Step 3: Verify build**

Run: `go build ./internal/db/...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/db/models/jobs.go
git commit -m "refactor: rename epic/psn job-source constants to canonical slugs"
```

---

## Task 3: Backend sync API — allow-list, routes, handler literals, delete display-name map

**Files:**
- Modify: `internal/api/sync.go` (lines `93`, `103-120`, `240-245`, `324`, `727`, `741`, `760`, `840`, `859`, `887`)
- Test: `internal/api/sync_test.go`

- [ ] **Step 1: Write a failing test asserting the allow-list accepts new slugs and rejects old ones**

Add to `internal/api/sync_test.go`, mirroring the existing `TestSyncTrigger_EpicCreatesJob` pattern exactly (`newSyncTestApp` + `setupTagUser` + `postJSONAuth`, shared `testDB`, no new container):

```go
func TestSyncTrigger_RejectsOldSlugs(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "trig-oldslug-1")

	// Clean cutover: the pre-rename slugs are no longer valid storefronts.
	for _, sf := range []string{"psn", "epic"} {
		rec := postJSONAuth(t, e, "/api/sync/"+sf, nil, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("old slug %q: got %d, want 400", sf, rec.Code)
		}
	}
}

func TestSyncTrigger_PlaystationStoreCreatesJob(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "trig-pss-1")

	rec := postJSONAuth(t, e, "/api/sync/playstation-store", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for playstation-store trigger, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["storefront"] != "playstation-store" {
		t.Fatalf("expected storefront=playstation-store, got %v", resp["storefront"])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run 'TestSyncTrigger_RejectsOldSlugs|TestSyncTrigger_PlaystationStoreCreatesJob' -v`
Expected: FAIL (old slugs currently accepted; `playstation-store` currently rejected by the old allow-list).

- [ ] **Step 3: Update the allow-list**

In `internal/api/sync.go:93` change:

```go
var supportedStorefronts = []string{"steam", "psn", "epic", "gog", "humble-bundle"}
```

to:

```go
var supportedStorefronts = []string{"steam", "playstation-store", "epic-games-store", "gog", "humble-bundle"}
```

- [ ] **Step 4: Delete `storefrontDisplayName` and inline the slug in its one caller**

Delete the entire `storefrontDisplayName` function (`internal/api/sync.go:103-120`, including its doc comment). Then at `internal/api/sync.go:324` change:

```go
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect "+storefrontDisplayName(sf))
```

to:

```go
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect "+sf)
```

(The user-facing toast is catalog-driven on the frontend; this is a fallback error string. Doing a catalog DB lookup here is fragile — the DB just failed an UPDATE.)

- [ ] **Step 5: Rename the hardcoded connection route literals**

In `RegisterRoutes` (`internal/api/sync.go:240-245`) change the six `psn`/`epic` route literals:

```go
	g.PUT("/playstation-store/connection", h.HandlePSNConnect)
	g.GET("/playstation-store/connection", h.HandleGetPSNConnection)
	g.DELETE("/playstation-store/connection", h.HandlePSNDisconnect)
	g.PUT("/epic-games-store/connection", h.HandleEpicConnect)
	g.DELETE("/epic-games-store/connection", h.HandleEpicDisconnect)
	g.GET("/epic-games-store/connection", h.HandleGetEpicConnection)
```

(Leave the Go handler method names `HandlePSNConnect`/`HandleEpicConnect` etc. as-is — renaming Go method identifiers is gratuitous churn and out of scope. Only the URL path string and stored slug change.)

- [ ] **Step 6: Rename the per-handler slug literals**

Change the storefront slug literal in each PSN/Epic handler call:
- `internal/api/sync.go:727` `"psn"` → `"playstation-store"` (persistStorefrontCredentials)
- `internal/api/sync.go:741` `"psn"` → `"playstation-store"` (serveConnectionStatus)
- `internal/api/sync.go:760` `"psn"` → `"playstation-store"` (disconnectStorefront)
- `internal/api/sync.go:840` `"epic"` → `"epic-games-store"` (persistStorefrontCredentials)
- `internal/api/sync.go:859` `"epic"` → `"epic-games-store"` (clearStorefrontCredentials)
- `internal/api/sync.go:887` `"epic"` → `"epic-games-store"` (serveConnectionStatus)

Re-run `grep -n '"psn"\|"epic"' internal/api/sync.go` and confirm zero remaining sync-slug literals.

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/api/... -run 'TestSyncTrigger_RejectsOldSlugs|TestSyncTrigger_PlaystationStoreCreatesJob' -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "refactor: use canonical storefront slugs in sync API routes and validation"
```

---

## Task 4: Backend adapter-dispatch switch

**Files:**
- Modify: `cmd/nexorious/serve.go:458,509`

- [ ] **Step 1: Rename the two switch case labels**

In `buildAdapterFactory` (`cmd/nexorious/serve.go`), change `case "psn":` (line 458) → `case "playstation-store":` and `case "epic":` (line 509) → `case "epic-games-store":`. Leave `steam`, `gog`, `humble-bundle` cases unchanged.

- [ ] **Step 2: Verify build**

Run: `go build ./cmd/...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "refactor: dispatch sync adapters by canonical storefront slug"
```

---

## Task 5: Delete the `StorefrontToCollectionSlug` bridge

**Files:**
- Delete: `internal/services/platformresolution/resolution.go`
- Delete: `internal/services/platformresolution/resolution_test.go`
- Modify: `internal/worker/tasks/sync.go:717-722`

- [ ] **Step 1: Update the one caller to use the slug directly**

At `internal/worker/tasks/sync.go:717`, the slug now equals the catalog name, so the resolve-and-fail branch is dead. Replace:

```go
	storefrontSlug, ok := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
	if !ok {
		markItemFailed(ctx, w.DB, &item, fmt.Sprintf("unresolved storefront=%s", eg.Storefront), "process_sync_item: markItemFailed")
		SyncCheckJobCompletion(ctx, w.DB, item.JobID)
		return nil
	}
```

with:

```go
	storefrontSlug := eg.Storefront
```

Leave all later uses of `storefrontSlug` (the raw `SELECT`/`INSERT` into `user_game_platforms`) unchanged — they continue to write the canonical name, exactly as before.

- [ ] **Step 2: Delete the bridge files**

```bash
git rm internal/services/platformresolution/resolution.go \
       internal/services/platformresolution/resolution_test.go
```

- [ ] **Step 3: Verify the package still builds (igdb_ids.go remains)**

Run: `go build ./internal/services/platformresolution/... ./internal/worker/...`
Expected: no errors. If `platformresolution` is now an unused import in `sync.go`, confirm it is still used (it is — `IGDBPlatformIDsForExternalGame` at `sync.go:502`); the import stays.

- [ ] **Step 4: Run worker tests for the changed path**

Run: `go test ./internal/worker/tasks/... -run TestProcessSyncItem -v` (or the nearest existing test covering `UserGameWorker`; list with `grep -n "func Test" internal/worker/tasks/sync_test.go`)
Expected: compile + run. Failures here are likely stale `"psn"`/`"epic"` fixtures — fix them in Task 6.

- [ ] **Step 5: Commit**

```bash
git add -A internal/services/platformresolution/ internal/worker/tasks/sync.go
git commit -m "refactor: remove StorefrontToCollectionSlug bridge (slug now equals catalog name)"
```

---

## Task 6: Update backend test fixtures (old → new slugs)

**Files:**
- Modify: `internal/worker/tasks/sync_test.go`, `internal/api/jobs_test.go`, `internal/api/job_items_test.go` (and any other `_test.go` still referencing the old sync slug)

- [ ] **Step 1: Find every remaining old-slug reference in test files**

Run: `grep -rn '"psn"\|"epic"' --include=*_test.go internal/`
Expected: a list of fixture literals. Inspect each: only change those that represent the **sync-source slug** (an `external_games.storefront`, `user_sync_configs.storefront`, `jobs.source`, a `/api/sync/<slug>` path, or a `DispatchSyncArgs.Storefront`). Leave any that legitimately mean the catalog name (already `epic-games-store`/`playstation-store`) or an unrelated platform string.

- [ ] **Step 2: Replace sync-slug fixtures**

For each confirmed sync-slug occurrence: `"psn"` → `"playstation-store"`, `"epic"` → `"epic-games-store"`. Specific known cases:
- `internal/api/sync_test.go` `TestSyncTrigger_EpicCreatesJob` posts to `/api/sync/epic` and asserts `resp["storefront"] == "epic"` — change the path to `/api/sync/epic-games-store` and the assertion to `"epic-games-store"` (otherwise it will 400 after the rename).
- `internal/worker/tasks/sync_test.go:1936` has a comment referencing `StorefrontToCollectionSlug` — update or remove it to reflect the bridge's deletion (Task 5).
- Inspect each `external_games`/`user_sync_configs`/`jobs` insert helper in these test files for hardcoded `psn`/`epic` and update.

- [ ] **Step 3: Run the affected suites**

Run: `go test ./internal/worker/tasks/... ./internal/api/... -run 'Sync|Job' -v`
Expected: PASS. (The pre-push hook runs the full `go test ./...`; you do not need to run it by hand here.)

- [ ] **Step 4: Commit**

```bash
git add internal/worker/tasks/sync_test.go internal/api/jobs_test.go internal/api/job_items_test.go
git commit -m "test: update sync-slug fixtures to canonical storefront names"
```

---

## Task 7: Frontend sync types — enum rename, delete `getStorefrontDisplayInfo`

**Files:**
- Modify: `ui/frontend/src/types/sync.ts` (lines `5-19`, `72-114`)
- Modify: `ui/frontend/src/types/sync.test.ts`

- [ ] **Step 1: Rename the two diverging enum members + the support list**

In `ui/frontend/src/types/sync.ts`, change the enum and the list:

```typescript
export enum SyncStorefront {
  STEAM = 'steam',
  EPIC_GAMES_STORE = 'epic-games-store',
  GOG = 'gog',
  PLAYSTATION_STORE = 'playstation-store',
  HUMBLE = 'humble-bundle',
}

export const SUPPORTED_SYNC_STOREFRONTS: SyncStorefront[] = [
  SyncStorefront.STEAM,
  SyncStorefront.EPIC_GAMES_STORE,
  SyncStorefront.GOG,
  SyncStorefront.PLAYSTATION_STORE,
  SyncStorefront.HUMBLE,
];
```

- [ ] **Step 2: Delete `getStorefrontDisplayInfo`**

Delete the entire `getStorefrontDisplayInfo` function (`ui/frontend/src/types/sync.ts:72-114`). Display names + icons now come from the catalog (Tasks 8 & 10). Leave `getSyncFrequencyLabel` and all interfaces intact.

- [ ] **Step 3: Update sync.test.ts**

In `ui/frontend/src/types/sync.test.ts`, delete any test of `getStorefrontDisplayInfo`. Update any enum-value assertions to the new values (e.g. expect `SyncStorefront.PLAYSTATION_STORE === 'playstation-store'`). Do not add new tests for trivial enum values beyond what already existed.

- [ ] **Step 4: Type-check (will surface all downstream enum-member references)**

Run: `cd ui/frontend && npm run check`
Expected: FAIL with errors at every `SyncStorefront.EPIC` / `SyncStorefront.PSN` / `getStorefrontDisplayInfo` reference. These are fixed in Tasks 8–10. (Do not commit yet — leave the tree red until Task 10 makes it green, OR commit per-task and accept transient red between frontend tasks. Recommended: complete Tasks 7→10 then run check once before committing the frontend as one unit. The Stop hook's typecheck is a nudge, not a gate.)

---

## Task 8: Frontend sync-service-card — catalog-driven name/icon, neutral swatch

**Files:**
- Modify: `ui/frontend/src/components/sync/sync-service-card.tsx`

- [ ] **Step 1: Replace `getStorefrontDisplayInfo` with `useStorefront`**

In `sync-service-card.tsx`:
- Remove the import `import { getStorefrontDisplayInfo } from '@/types';`.
- Add `import { useStorefront } from '@/hooks';`.
- Replace line 30 `const platformInfo = getStorefrontDisplayInfo(config.storefront);` with:

```typescript
  const { data: storefront } = useStorefront(config.storefront);
  const displayName = storefront?.display_name ?? config.storefront;
```

- [ ] **Step 2: Use the neutral swatch + catalog icon + display name**

- Line 39: replace `${platformInfo.bgColor}` with `bg-muted` (drop the brand tint — the logo carries the brand):

```tsx
            <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-muted">
```

- Lines 42-43: source from the catalog `icon_url` (relative path; still prefixed with `envConfig.staticUrl`). Guard null icon:

```tsx
              {storefront?.icon_url && (
                <img
                  src={`${envConfig.staticUrl}${storefront.icon_url}`}
                  alt={`${displayName} icon`}
                  width={28}
                  height={28}
                  className="h-7 w-7"
                  loading="lazy"
                />
              )}
```

- Line 57: replace `{platformInfo.name}` with `{displayName}`.

- [ ] **Step 2 verification (visual reasoning):** the card now shows the catalog `display_name` ("PlayStation Store"/"Epic Games Store") and the catalog icon, in a neutral `bg-muted` rounded square. No behavior change beyond the swatch color and the corrected names.

---

## Task 9: Frontend job-source label hook (catalog-aware)

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts` (lines `17-28`, `219-236`)
- Create: `ui/frontend/src/hooks/use-job-source-label.ts`
- Modify: `ui/frontend/src/hooks/index.ts` (export the hook)
- Modify: `ui/frontend/src/components/jobs/recent-activity.tsx`, `job-card.tsx`, `job-progress-card.tsx`

> See the "Deviation note for plan review" above — confirm this approach (hook) vs. the smaller static-map alternative before executing.

- [ ] **Step 1: Rename the diverging `JobSource` enum members**

In `ui/frontend/src/types/jobs.ts:17-28`, change `EPIC = 'epic'` → `EPIC_GAMES_STORE = 'epic-games-store'` and `PSN = 'psn'` → `PLAYSTATION_STORE = 'playstation-store'`. Leave `STEAM`, `GOG`, `HUMBLE_BUNDLE`, `MANUAL`, `DARKADIA`, `NEXORIOUS`, `CSV`, `SYSTEM` unchanged.

- [ ] **Step 2: Replace `getJobSourceLabel` with a non-storefront-only static map + delete the storefront entries**

In `ui/frontend/src/types/jobs.ts`, replace `getJobSourceLabel` (lines ~219-236) with a static map covering ONLY the non-storefront origins, exported for the hook to fall back to:

```typescript
/**
 * Static labels for non-storefront job sources. Storefront-typed sources derive
 * their label from the storefronts catalog via useJobSourceLabel().
 */
export const NON_STOREFRONT_JOB_SOURCE_LABELS: Partial<Record<JobSource, string>> = {
  [JobSource.MANUAL]: 'Manual',
  [JobSource.DARKADIA]: 'Darkadia',
  [JobSource.NEXORIOUS]: 'Nexorious',
  [JobSource.CSV]: 'CSV',
  [JobSource.SYSTEM]: 'System',
};
```

(Delete the old `getJobSourceLabel` function entirely. If knip flags it as still-imported, Step 4 removes the importers.)

- [ ] **Step 3: Create the catalog-aware hook**

Create `ui/frontend/src/hooks/use-job-source-label.ts`:

```typescript
import { useStorefronts } from '@/hooks';
import { JobSource, NON_STOREFRONT_JOB_SOURCE_LABELS } from '@/types/jobs';

/**
 * Returns a labeller for job sources. Storefront-typed sources (steam,
 * epic-games-store, gog, playstation-store, humble-bundle) resolve to the
 * catalog display_name — the single source of truth. Non-storefront origins
 * (manual, darkadia, csv, ...) use a static label map.
 */
export function useJobSourceLabel(): (source: JobSource | string) => string {
  const { data } = useStorefronts();
  const byName = new Map((data?.storefronts ?? []).map((s) => [s.name, s.display_name]));
  return (source) =>
    byName.get(source) ??
    NON_STOREFRONT_JOB_SOURCE_LABELS[source as JobSource] ??
    source;
}
```

Confirm the `useStorefronts()` return shape exposes `data.storefronts` with `name`/`display_name` (it does per `ui/frontend/src/hooks/use-platforms.ts` + `transformStorefront`); adjust the destructuring if the wrapper differs.

- [ ] **Step 4: Export the hook and swap the three call sites**

- Add `export { useJobSourceLabel } from './use-job-source-label';` to `ui/frontend/src/hooks/index.ts`.
- In each of `recent-activity.tsx`, `job-card.tsx`, `job-progress-card.tsx`: remove the `getJobSourceLabel` import, add `import { useJobSourceLabel } from '@/hooks';`, call `const jobSourceLabel = useJobSourceLabel();` at the top of the component, and replace `getJobSourceLabel(x)` with `jobSourceLabel(x)`. (These are all components, so a hook call is valid.)

- [ ] **Step 5: Update jobs label tests**

If `ui/frontend/src/types/jobs.test.ts` (or similar) tested `getJobSourceLabel`, replace with a small test of `NON_STOREFRONT_JOB_SOURCE_LABELS` mapping and/or a render test of `useJobSourceLabel` via `@testing-library/react` with a mocked `useStorefronts`. Keep it minimal — assert a storefront source resolves to catalog name and a non-storefront source resolves to its static label.

---

## Task 10: Frontend sync detail route + API paths + hooks (enum refs, catalog title/heading)

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/sync/index.tsx`
- Modify: `ui/frontend/src/api/sync.ts`
- Modify: `ui/frontend/src/hooks/use-sync.ts`

- [ ] **Step 1: API path literals**

In `ui/frontend/src/api/sync.ts`, change the six hardcoded paths: `/sync/epic/connection` (lines 240, 253, 267) → `/sync/epic-games-store/connection`; `/sync/psn/connection` (lines 345, 361, 441) → `/sync/playstation-store/connection`.

- [ ] **Step 2: Hook enum refs**

In `ui/frontend/src/hooks/use-sync.ts`, replace `SyncStorefront.EPIC` (lines 192, 208) → `SyncStorefront.EPIC_GAMES_STORE` and `SyncStorefront.PSN` (lines 271, 304) → `SyncStorefront.PLAYSTATION_STORE`.

- [ ] **Step 3: `index.tsx` enum refs**

In `ui/frontend/src/routes/_authenticated/sync/index.tsx`, replace `SyncStorefront.PSN` (lines 64, 80) → `SyncStorefront.PLAYSTATION_STORE` and `SyncStorefront.EPIC` (lines 67, 81) → `SyncStorefront.EPIC_GAMES_STORE`. (`countsBySource[config.storefront]` at line 78 needs no change — both keys are now canonical slugs.)

- [ ] **Step 4: `$storefront.tsx` — replace the head title map with a catalog-driven document title**

Remove the `getStorefrontDisplayInfo` import (line 35). Replace the route `head` block (the inline `names` map) with a static fallback title only:

```typescript
export const Route = createFileRoute('/_authenticated/sync/$storefront')({
  head: () => ({ meta: [{ title: 'Sync | Nexorious' }] }),
  component: SyncDetailPage,
});
```

Then in the `SyncDetailPage` component, fetch the catalog storefront and set the document title + heading from it. Replace `const platformInfo = getStorefrontDisplayInfo(storefront);` (line 182) with:

```typescript
  const { data: storefrontInfo } = useStorefront(storefront);
  const displayName = storefrontInfo?.display_name ?? storefront;

  useEffect(() => {
    document.title = `${displayName} Sync | Nexorious`;
  }, [displayName]);
```

Add `useStorefront` to the `@/hooks` import. `useEffect` is already imported (line 1). Title and heading now share one source (`storefrontInfo.display_name`), so they cannot disagree, and the missing-`humble-bundle` fallback bug is gone.

- [ ] **Step 5: `$storefront.tsx` — swap remaining `platformInfo`/enum usages**

- Replace `platformInfo.name` with `displayName` at every occurrence (lines ~233, 246, 299, 319, 340, 392).
- Header swatch (line ~381): replace `${platformInfo.bgColor}` with `bg-muted`.
- Header icon (lines ~384-385): source from catalog with a null guard:

```tsx
              {storefrontInfo?.icon_url && (
                <img
                  src={`${envConfig.staticUrl}${storefrontInfo.icon_url}`}
                  alt={`${displayName} icon`}
                  loading="lazy"
                  className="h-10 w-10"
                />
              )}
```

- Credentials-error enum refs (lines 192-193): `SyncStorefront.PSN` → `SyncStorefront.PLAYSTATION_STORE`, `SyncStorefront.EPIC` → `SyncStorefront.EPIC_GAMES_STORE`. Same for the connection-card render guards (lines 480, 500). `SyncStorefront.STEAM/GOG/HUMBLE` references are unchanged.

- [ ] **Step 6: Type-check, knip, lint, tests**

Run: `cd ui/frontend && npm run check && npm run knip`
Expected: PASS, zero knip findings. (knip will flag `getStorefrontDisplayInfo`/`getJobSourceLabel` if any importer was missed — fix by removing the stale import.)

Run: `cd ui/frontend && npm run test`
Expected: PASS.

- [ ] **Step 7: Regenerate route tree if needed**

Run: `cd ui/frontend && npm run build`
The `$storefront` param route path string did not change, so `routeTree.gen.ts` should be unchanged. Run `git status ui/frontend/src/routeTree.gen.ts` — if it changed, stage it; if not, nothing to commit there.

- [ ] **Step 8: Commit the frontend as one unit**

```bash
git add ui/frontend/src/types/sync.ts ui/frontend/src/types/sync.test.ts \
        ui/frontend/src/types/jobs.ts ui/frontend/src/hooks/use-job-source-label.ts \
        ui/frontend/src/hooks/index.ts ui/frontend/src/api/sync.ts \
        ui/frontend/src/hooks/use-sync.ts \
        ui/frontend/src/components/sync/sync-service-card.tsx \
        ui/frontend/src/components/jobs/recent-activity.tsx \
        ui/frontend/src/components/jobs/job-card.tsx \
        ui/frontend/src/components/jobs/job-progress-card.tsx \
        ui/frontend/src/routes/_authenticated/sync/index.tsx \
        ui/frontend/src/routes/_authenticated/sync/\$storefront.tsx
# include routeTree.gen.ts only if it changed
git commit -m "refactor: derive sync/job storefront display from catalog, canonical slugs in UI"
```

---

## Task 11: Full-stack verification

- [ ] **Step 1: Backend build + targeted tests**

Run: `make build && go test ./internal/api/... ./internal/worker/... ./internal/migrate/... ./internal/db/...`
Expected: build succeeds, tests PASS.

- [ ] **Step 2: Frontend gates**

Run: `cd ui/frontend && npm run check && npm run knip && npm run test`
Expected: all PASS, zero knip findings.

- [ ] **Step 3: Grep for any stray old sync-slug literal**

Run from repo root:
```bash
grep -rn '"psn"\|"epic"' --include=*.go internal/ cmd/ | grep -v darkadia
grep -rn "'psn'\|'epic'\|/sync/psn/\|/sync/epic/\|SyncStorefront\.\(PSN\|EPIC\)\b\|JobSource\.\(PSN\|EPIC\)\b" ui/frontend/src/ | grep -v "epic-games-store\|playstation-store"
```
Expected: no results (darkadia import-mapping in Go is the only legitimate `epic`/`psn` remaining, already excluded).

- [ ] **Step 4: Manual smoke (documented)**

With a dev server + a migrated DB that had pre-existing `psn`/`epic` sync data: open `/sync`, confirm the PlayStation Store and Epic Games Store cards render with catalog names + icons; open each detail page, confirm the browser tab title matches the page heading; confirm a previously-synced library still lists its games (FK to `user_game_platforms` unchanged) and job history shows "PlayStation Store"/"Epic Games Store" labels.

- [ ] **Step 5: Final commit (if anything was left staged) and push**

```bash
git status   # expect clean working tree
git push -u origin refactor/unify-sync-storefront-slugs   # pre-push runs full go test + frontend gates
```

---

## Acceptance criteria mapping (self-review)

- *One identifier across URL, API, persisted data = `storefronts.name`* → Tasks 1 (data), 3 (routes/validation/handlers), 4 (dispatch), 2 (constants), 7/9/10 (frontend enums + API paths).
- *`StorefrontToCollectionSlug` and its bridging role gone* → Task 5.
- *Sync titles/headings consistent from a single source* → Task 10 Step 4 (catalog-driven title + heading).
- *Existing users' sync configs, jobs, external games resolve after migration* → Task 1 (+ river_job patch); Task 11 Step 4 smoke.
- *Job-source enum aligned in the same change* (open question → decided yes, forced by the `jobs.source` migration) → Tasks 2, 9.
- *Reversible down-migration* (open question → decided yes) → Task 1 Step 2.
- *Clean cutover, no transitional shim* (open question → decided) → Task 3 Step 1 test asserts old slugs 400.
- *Display name standardized on catalog `display_name`* (open question → decided) → Tasks 8, 9, 10.
