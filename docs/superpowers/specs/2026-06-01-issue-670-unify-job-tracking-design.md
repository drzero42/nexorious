# Issue #670 — Unify import/export and sync job tracking

## Problem

After starting an import or export, the **Recent Activity** section on the
Import / Export page never shows the completed job — it stays permanently empty
(«No recent activity»), even though the job ran and completed.

### Root cause

Recent Activity is backed by `useJobs` (`GET /api/jobs/`), which only polls
while its **own cached result** already contains an in-progress job. The page
loads before any job exists, so the cache is empty, polling never starts, and
nothing invalidates it when a job later completes. The active-job card uses a
separate `useActiveJob` query, so the two never coordinate.

### Architectural divergence

Sync solves the same problem (track an active job, detect completion, refresh
dependent data) with a robust pattern that import/export and maintenance do
not share:

- A dedicated status endpoint (`GET /api/sync/:storefront/status`) polled
  continuously (30 s baseline, 5 s while active), catching background jobs.
- The active job is fetched by **id** from the status response, not by type.
- A `useRef` + `useEffect` watches `active_job_id` transition non-null → null
  as the explicit completion signal, invalidating dependent queries at that
  moment.
- The trigger mutation optimistically writes `is_syncing` + `active_job_id`
  into the status cache for immediate UI feedback.

`import-export.tsx`, `admin/maintenance.tsx`, and the `_authenticated.tsx`
sidebar all instead use `useActiveJob(jobType)` (`GET /api/jobs/active/:job_type`),
which stops polling once the job is terminal and has no completion signal.

## Goal

Unify all job-progress tracking around the sync pattern by introducing a
generic job-type status endpoint and shared frontend hooks, then retire
`useActiveJob` entirely.

## Backend

### New endpoint `GET /api/jobs/status/:job_type`

A new `HandleJobTypeStatus` handler on `JobsHandler`, modelled on
`HandleGetSyncStatus` but keyed on `job_type` only (no storefront/source).
Scoped to the authenticated user. Response:

```json
{
  "is_active": true,
  "active_job_id": "abc123",
  "last_completed_job_id": "xyz789",
  "last_completed_at": "2026-05-30T12:00:00Z"
}
```

- `is_active` / `active_job_id`: the most recent job of this type with status
  `pending` or `processing` (`active_job_id` null when none).
- `last_completed_job_id` / `last_completed_at`: id and `completed_at` of the
  most recent terminal job of this type (both null when none).

`last_completed_job_id` is an addition beyond the issue's sketch. It lets the
frontend keep showing the just-finished job in the prominent result card
(e.g. the export **Download** button) after completion and on page load.
Because `active_job_id` and `last_completed_job_id` arrive in the same
response, the card's source id (`active_job_id ?? last_completed_job_id`)
transitions atomically with no flicker.

Route registration: `GET /status/:job_type` must be registered **before**
`GET /:id` (Echo v5 does not auto-sort); placed next to the other
static-prefix routes in `router.go`.

Tests in `jobs_test.go`: no-job, active-job, and completed-job cases (mirrors
`TestSyncStatus_ReflectsActiveJob`).

Add a `jobs/get_status` request to `slumber.yaml` (bearer auth).

### Remove the dead `GET /api/jobs/active/:job_type` endpoint

Once `useActiveJob` is retired the endpoint has no callers. Delete
`HandleActiveJob`, its route in `router.go`, its three tests
(`TestHandleActiveJob_*`), and its `slumber.yaml` entry.

## Frontend

### New `useJobTypeStatus(jobType)` hook

In `use-jobs.ts`, mirrors `useSyncStatus`: calls the new endpoint, polls every
30 s at baseline and every 3 s while `is_active`. Backed by a new
`getJobTypeStatus` in `api/jobs.ts` and a `JobTypeStatus` type with a
snake→camel transform (`isActive`, `activeJobId`, `lastCompletedJobId`,
`lastCompletedAt`). New query key `jobsKeys.typeStatus(jobType)`.

### New shared `useJobCompletionEffect(activeJobId, onComplete)` hook

Extracted to its own file (`hooks/use-job-completion-effect.ts`). Holds the
`useRef` + `useEffect` transition-detection pattern currently inline in
`$storefront.tsx`:

```ts
export function useJobCompletionEffect(
  activeJobId: string | null | undefined,
  onComplete: () => void,
) {
  const prevRef = useRef<string | null>(null);
  useEffect(() => {
    if (prevRef.current && !activeJobId) onComplete();
    prevRef.current = activeJobId ?? null;
  }, [activeJobId, onComplete]);
}
```

Callers must memoise `onComplete` (`useCallback`) so the effect deps are
stable.

### Refactor `$storefront.tsx` (sync)

Replace the inline ref/effect block (currently lines ~182–191) with a call to
`useJobCompletionEffect(status?.activeJobId, …)` whose callback invalidates
`jobsKeys.recent(storefront)` and `syncKeys.externalGames(storefront)`.
Behaviour is identical.

### Rewrite `import-export.tsx`

- Replace the two `useActiveJob(IMPORT/EXPORT)` calls with two
  `useJobTypeStatus` calls.
- For each type, derive the displayed job id as
  `status?.activeJobId ?? status?.lastCompletedJobId` and fetch it with
  `useJob(displayedId)` — preserving the on-load and post-completion result
  card (including the export Download button).
- Keep the existing dismiss (`dismissedJobId`) and import-vs-export selection
  logic, adapted to the new job sources.
- Call `useJobCompletionEffect` for **each** type with a callback that
  invalidates `jobsKeys.lists()` → fixes the empty Recent Activity.

### Rewrite `admin/maintenance.tsx`

Same pattern for the single `METADATA_REFRESH` type: `useJobTypeStatus` +
`useJob(active ?? lastCompleted)` + completion effect. The result card and
dismiss behaviour are preserved via `last_completed_job_id`.

### Rewrite `_authenticated.tsx` sidebar (`useInvalidateGamesOnImportComplete`)

No result card — simplest case. `useJobTypeStatus(IMPORT)` +
`useJobCompletionEffect` whose callback invalidates `gameKeys.lists()` and
`gameKeys.stats()`. As a bonus this fixes a latent bug: today the games list
fails to refresh when an import starts after the layout has already cached
"no active job", because `useActiveJob` never resumes polling. The 30 s
baseline poll of the new status hook makes completion detection reliable.

### Optimistic update on start

Move (or add) optimistic status-cache writes into the import/export mutation
success handlers (`useImportNexorious` / `useExportCollection` in
`use-import-export.ts`), mirroring `useTriggerSync`: write `isActive: true` +
`activeJobId` into `jobsKeys.typeStatus(jobType)` so the active-job card
appears immediately without waiting for the next poll.

### Retire `useActiveJob`

Remove the `useActiveJob` hook from `use-jobs.ts` and its re-export in
`hooks/index.ts`, and remove the now-unused `getActiveJob` function from
`api/jobs.ts` (knip would otherwise flag it). Update
`routes/_authenticated.test.tsx` which mocks `useActiveJob`.

## Testing

- Backend: new `HandleJobTypeStatus` tests (no-job / active / completed);
  remove `HandleActiveJob` tests.
- Frontend: unit test for `useJobCompletionEffect` (fires `onComplete` only on
  non-null → null transition, not on mount or null → non-null). Update existing
  `use-jobs` / route tests affected by the hook changes.

## Out of scope

A per-outcome summary for imports in Recent Activity (Added / Removed / etc.)
as sync shows from `sync_changes`. Import per-item results live in `job_items`
and are already rendered by `JobItemsDetails` when expanded; a grouped summary
is a separate follow-up.

## PR

Title prefixed `feat:` (architectural change introducing a new endpoint and
unified tracking).
