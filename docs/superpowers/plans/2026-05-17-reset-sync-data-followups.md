# Reset Sync Data — Follow-up Items

These were flagged as out-of-scope during the implementation of the reset sync data feature (`feat/reset-sync-data`). Address before or after merge as appropriate.

---

## 1. `useResetSyncData` doesn't invalidate `syncKeys.configs()` (plural)

**File:** `ui/frontend/src/hooks/use-sync.ts`

The hook invalidates `syncKeys.config(platform)` and `syncKeys.status(platform)` on success, but not `syncKeys.configs()` — the list-all query that backs the card list. After a reset, `last_synced_at` shown on the sync card may remain stale until the next poll interval.

**Fix:** Add `queryClient.invalidateQueries({ queryKey: syncKeys.configs() })` to `useResetSyncData.onSuccess`.

---

## 2. `TestResetSyncData_CancelsActiveJob` doesn't verify `river_job` rows are cancelled

**File:** `internal/api/sync_test.go`

The test only asserts that the `jobs` table row has `status='cancelled'`. It does not verify that the corresponding `river_job` rows had their `state` updated to `'cancelled'`. Accepted as matching the existing pattern in `TestCancelJob` (same gap), but both tests could be strengthened.

---

## 3. AlertDialog gives no in-flight feedback

**File:** `ui/frontend/src/components/sync/sync-service-card.tsx`

The confirmation dialog closes synchronously when the user clicks "Reset". The `isResetting` spinner appears on the card, not inside the dialog, so the dialog disappears before any progress indication is shown.

**Fix (if desired):** Keep dialog open while `isResetting` is true and show a loading state inside the dialog; close it only after the mutation settles.

---

## 4. No component tests for the reset dialog

**File:** `ui/frontend/src/components/sync/sync-service-card.tsx` (no test file exists)

No frontend tests cover:
- Reset button is absent when `onReset` prop is omitted
- Reset button is absent when `!config.isConfigured`
- Clicking the button opens the confirmation dialog
- Clicking "Reset" in the dialog calls `onReset`

---

## 5. `time.Now().UTC()` vs `NOW()` inconsistency in the reset handler

**File:** `internal/api/sync.go` — `HandleResetSyncData`

Job cancellation writes `completed_at` using Go-side `time.Now().UTC()`, while the transaction uses Postgres `now()` for `updated_at`. Functionally identical but inconsistent.

**Fix:** Use `now()` in both places (or `time.Now().UTC()` in both via a bound parameter).
