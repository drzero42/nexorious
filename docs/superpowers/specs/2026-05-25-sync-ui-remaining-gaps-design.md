# Sync UI Remaining Gaps Design

**Date:** 2026-05-25
**Branch:** issue-608-normalise-external-games

## Overview

After completing G-F1 through G-F5 (see `2026-05-25-sync-ui-spec-compliance-design.md`) and
backend gaps G1–G3 (see `2026-05-25-sync-remaining-gaps-design.md`), four further divergences
between the implementation and `docs/sync.md` were identified. All four are code bugs — the spec
is correct and clear.

---

## Gap Map

| ID | Area | Backend change? | Frontend change? |
|----|------|-----------------|-----------------|
| G-F6 | Hub card missing external game count | Yes | Yes |
| G-F7 | JobItemsDetails shown during sync (not in spec) | No | Yes |
| G-F8 | ExternalGamesSection does not poll during active sync | No | Yes |
| G-F9 | Connection section open/close behaviour wrong | No | Yes |

---

## G-F6 — Hub card missing external game count

### Problem

The spec says each hub card shows the count of synced games, e.g. "482 games". This count is the
number of `external_games` rows where `is_available = true` for the user + storefront. Currently
nothing in the status endpoint, type system, or hub card UI surfaces this count.

### Fix

**Backend — `GET /api/sync/{storefront}/status`:**

In `internal/api/sync.go`, add `ExternalGameCount int` to `syncStatusResponse`:

```go
type syncStatusResponse struct {
    // ... existing fields ...
    ExternalGameCount int `json:"external_game_count"`
}
```

Query the count once in `HandleGetSyncStatus`, before building the response:

```go
var count int
err = h.db.NewSelect().
    TableExpr("external_games").
    ColumnExpr("COUNT(*)").
    Where("user_id = ? AND storefront = ? AND is_available = true", userID, storefront).
    Scan(ctx, &count)
if err != nil {
    count = 0  // non-fatal; hub card shows 0 on error
}
```

**Frontend — `types/sync.ts`:**

Add `externalGameCount: number` to the `SyncStatus` interface.

**Frontend — `api/sync.ts`:**

Add `external_game_count: number` to `SyncStatusApiResponse`. Map it in `transformSyncStatus`:

```ts
externalGameCount: apiStatus.external_game_count ?? 0,
```

**Frontend — `components/sync/sync-service-card.tsx`:**

Add an `externalGameCount?: number` prop. When non-zero, render it below the storefront name:

```tsx
{externalGameCount > 0 && (
  <p className="text-sm text-muted-foreground">{externalGameCount} games</p>
)}
```

**Frontend — `routes/_authenticated/sync/index.tsx`:**

In `SyncServiceCardWithStatus`, pass `externalGameCount={status?.externalGameCount ?? 0}` to
`<SyncServiceCard>`.

**Test — `internal/api/sync_test.go`:**

Update `TestGetSyncStatus` to assert `external_game_count` is present in the response.

**Files:** `internal/api/sync.go`, `internal/api/sync_test.go`, `ui/frontend/src/types/sync.ts`,
`ui/frontend/src/api/sync.ts`, `ui/frontend/src/components/sync/sync-service-card.tsx`,
`ui/frontend/src/routes/_authenticated/sync/index.tsx`

---

## G-F7 — JobItemsDetails shown during active sync

### Problem

`$storefront.tsx` renders a `<JobItemsDetails>` panel whenever `isSyncing && activeJob`. This is a
per-item drill-down (tabs: Completed / Needs Review / Failed / Skipped) rendered alongside
`<JobProgressCard>`. The spec does not include this panel; the spec's stable-state view is
`<ExternalGamesSection>`. Its presence is confusing because it duplicates data in a different
format.

Additionally, `ExternalGamesSection` already returns `null` when `games.length === 0`, so it
appears as soon as the first game settles into a stable state — there is no gap during a first
sync.

### Fix

Remove the `<JobItemsDetails>` block and its import from `$storefront.tsx`. The active-sync UI
becomes: `<SyncJobStatus>` (status badge + cancel button) + `<JobProgressCard>` (live counts) +
`<ExternalGamesSection>` (games that have settled).

**Files:** `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`

---

## G-F8 — ExternalGamesSection does not poll during active sync

### Problem

`useExternalGames` in `hooks/use-sync.ts` has no `refetchInterval`. During an active sync, games
settle progressively into stable states (matched, needs_review, skipped, failed). Without polling,
`ExternalGamesSection` remains stale and won't reflect newly-settled games until the page
reloads.

Additionally, when the job becomes terminal the external games query is not invalidated, so there
is a window where the section shows stale data after the sync completes.

### Fix

**`components/sync/external-games-section.tsx`:**

Add an optional `isSyncing?: boolean` prop. Pass it through to `useExternalGames`:

```tsx
const { data: games, isLoading } = useExternalGames(storefront, {
  refetchInterval: isSyncing ? 5000 : undefined,
});
```

**`hooks/use-sync.ts`:**

Update `useExternalGames` signature to accept a `UseQueryOptions`-compatible `options` param (or
a simpler `{ refetchInterval?: number }` overrides object) so the interval can be passed from the
caller.

**`routes/_authenticated/sync/$storefront.tsx`:**

1. Pass `isSyncing` to `<ExternalGamesSection>`.
2. In the `useEffect` that fires when `activeJob` becomes terminal (or `isSyncing` flips to
   false), add:
   ```ts
   queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(storefront) });
   ```

**Files:** `ui/frontend/src/components/sync/external-games-section.tsx`,
`ui/frontend/src/hooks/use-sync.ts`,
`ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`

---

## G-F9 — Connection section open/close behaviour wrong

### Problem

Two observed symptoms, one root cause:

1. After successfully connecting a storefront, the Connection & Settings collapsible does not
   automatically collapse.
2. When navigating to the detail page after the storefront is already connected, the collapsible
   is open by default.

**Root cause:** The collapsible's open state is initialised with:

```tsx
const [connectionSectionOpen, setConnectionSectionOpen] = useState(
  () => !config?.isConfigured || credentialsError
);
```

The lazy initialiser runs synchronously on first render, before `config` has loaded from the
server. At that point `config` is `undefined`, so `!undefined` is `true` — the section always
opens on first render regardless of connection state. Once opened, nothing closes it when config
arrives.

The spec says: collapsed when connected and no credentials error; expanded when not configured or
credentials error.

### Fix

Replace the lazy initialiser with a two-effect pattern:

```tsx
const [connectionSectionOpen, setConnectionSectionOpen] = useState(false);
const connectionOpenInitialized = useRef(false);

// Set initial state once config arrives
useEffect(() => {
  if (!connectionOpenInitialized.current && config !== undefined) {
    connectionOpenInitialized.current = true;
    setConnectionSectionOpen(!config.isConfigured || credentialsError);
  }
}, [config, credentialsError]);

// Auto-collapse when the user successfully connects
useEffect(() => {
  if (connectionOpenInitialized.current && config?.isConfigured && !credentialsError) {
    setConnectionSectionOpen(false);
  }
}, [config?.isConfigured, credentialsError]);
```

The first effect fires once (guarded by `connectionOpenInitialized`) to set the correct initial
state after the async config load. The second effect fires whenever `isConfigured` or
`credentialsError` changes, collapsing the section when both conditions are false (i.e. the
storefront is now connected and healthy).

**Files:** `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`

---

## Out of Scope

- IGDB candidate data in the needs-review dialog (deferred to a later issue).
- Duplicate external-game rows for multi-platform Steam titles (pre-existing, separate issue).
