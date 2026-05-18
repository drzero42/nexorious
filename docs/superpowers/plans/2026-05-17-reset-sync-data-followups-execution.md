# Reset Sync Data Followups — Execution Plan

Branch: `fix/sync-test-and-followups`

Resolves the 5 items in [2026-05-17-reset-sync-data-followups.md](./2026-05-17-reset-sync-data-followups.md) plus the 11 frontend test failures that surfaced alongside them.

## Context

Frontend tests are failing:

| File | Failures | Root cause |
|---|---|---|
| `ui/frontend/src/api/sync.test.ts` | 2 | reset-sync-data PR (`fb2df6f`) renamed `SyncConfigApiResponse.platform` → `storefront` to match the backend, but test mocks still send `platform: 'steam'`. Transformer reads `apiConfig.storefront` → `undefined`. |
| `ui/frontend/src/hooks/use-sync.test.ts` | 2 | Same root cause as above. |
| `ui/frontend/src/api/tags.test.ts` | 7 | Unrelated to reset-sync-data. Orphaned from commit `c86386b` (tags refactor). MSW handlers register `/tags/` (now `/tags`) and return paginated envelopes (backend now returns a plain array). |

The reset-sync-data PR fixed only `SyncConfigApiResponse`. The same `platform`/`storefront` mismatch is still present in `SyncStatusApiResponse` and `ManualSyncApiResponse` (`sync.ts:39-51`) — backend returns `storefront`, transformers read `.platform`. This is a latent production bug: `transformSyncStatus` produces `platform: undefined`. Tests miss it because the mocks also use `platform`.

## Steps

### 1. Fix latent storefront/platform bug in sync.ts

`ui/frontend/src/api/sync.ts`:

- `SyncStatusApiResponse.platform: string` → `storefront: string`
- `transformSyncStatus`: read `apiStatus.storefront` (cast to `SyncPlatform`).
- `ManualSyncApiResponse.platform: string` → `storefront: string`
- `transformManualSyncResponse`: read `apiResponse.storefront`; the `ManualSyncResponse.platform` field stays — the storefront value flows into it.

### 2. Update sync test mocks

`ui/frontend/src/api/sync.test.ts`:
- `getSyncConfigs` mock: `platform: 'steam'` → `storefront: 'steam'` in the config item.
- `getSyncConfig` mock: same swap.
- `updateSyncConfig` mock: same swap.
- `triggerSync` mock: `platform: 'steam'` → `storefront: 'steam'`.
- `getSyncStatus` mock: `platform: 'steam'` → `storefront: 'steam'`.

`ui/frontend/src/hooks/use-sync.test.ts`:
- `mockSyncConfigApi`: rename `platform: 'steam'` → `storefront: 'steam'`.
- `mockSyncStatusApi`: rename `platform: 'steam'` → `storefront: 'steam'`.

### 3. Update tags test mocks

`ui/frontend/src/api/tags.test.ts`:
- All MSW paths: `${API_URL}/tags/` → `${API_URL}/tags` (no trailing slash). `POST /tags/` → `POST /tags`.
- `getTags` test: mock returns a plain array `[mockTagApi, mockTag2Api]`. Assertions adjusted — `result.tags.length`, `total = 2`, `page = 1`, `perPage = array.length`, `totalPages = 1`.
- `getTags > passes custom parameters`: query params still asserted, response a plain array.
- `getAllTags > paginates through all pages`: replace with a single call returning `[mockTagApi, mockTag2Api]`, assert `callCount === 1` (backend doesn't paginate).
- `getAllTags > empty array`: mock returns `[]`.
- `createTag` handlers: `POST /tags/` → `POST /tags`; response shape already correct.

### 4. F1 — invalidate `syncKeys.configs()` on reset success

`ui/frontend/src/hooks/use-sync.ts`, `useResetSyncData.onSuccess`: add `queryClient.invalidateQueries({ queryKey: syncKeys.configs() })` alongside the existing per-platform invalidations.

### 5. F3 — keep reset dialog open during mutation

`ui/frontend/src/components/sync/sync-service-card.tsx`:
- Control `AlertDialog` `open` via state.
- Mark the Reset action button `disabled` while `isResetting` and render a small spinner inside it.
- Don't auto-close on click — close only after the mutation settles (success and error) via `onSuccess`/`onError` callbacks on the mutate invocation (or via an effect watching `isResetting` transitions).
- Confirm a similar pattern is used (or skip the change) for the per-platform pages if they have their own dialog; check `routes/sync/$platform.tsx` (if it uses the same component, this is free).

### 6. F4 — component tests for the reset dialog

New file `ui/frontend/src/components/sync/sync-service-card.test.tsx`. Cases:
- Reset button is not rendered when `onReset` is omitted.
- Reset button is not rendered when `config.isConfigured === false`.
- Clicking the Reset button opens the confirmation dialog.
- Clicking "Reset" in the dialog calls `onReset` once.

Use `@testing-library/react` + `user-event`. No MSW needed — `onReset` is a prop.

### 7. F5 — SQL `now()` in `HandleResetSyncData`

`internal/api/sync.go`, `HandleResetSyncData`: rewrite the job cancellation UPDATE so `completed_at` is set via `now()` rather than `time.Now().UTC()`. Both columns then use the same time source.

### 8. F2 — assert `river_job` state in tests

`internal/api/sync_test.go`:
- `TestResetSyncData_CancelsActiveJob`: after the reset, query `river_job` for the row(s) belonging to the cancelled job and assert `state = 'cancelled'`.
- `TestCancelJob`: same assertion.

Use `testDB.NewSelect()` on the appropriate model or a raw query — pick what matches existing test style in the file.

### 9. Verification

- `cd ui/frontend && npm run check` — type check
- `cd ui/frontend && npm run test` — all frontend tests pass
- `go test -timeout 600s ./...` — backend tests pass
- `golangci-lint run` — no lint errors

### 10. Commit & PR

Logical commits on the branch, single PR to main. Suggested commit boundaries:

1. plan doc
2. `fix(sync): align SyncStatus/ManualSync types with backend storefront field`
3. `test(sync): update mocks to match storefront response field`
4. `test(tags): align mocks with plain-array response and /tags path`
5. `fix(sync): invalidate syncKeys.configs() after reset` (F1)
6. `feat(sync): keep reset dialog open during reset mutation` (F3)
7. `test(sync): add component tests for sync-service-card reset dialog` (F4)
8. `refactor(sync): use SQL now() for completed_at in reset cancellation` (F5)
9. `test(api): assert river_job state in cancel/reset tests` (F2)
