# Fix Recent Activity Stale/Empty Data (Issue #535)

## Summary

Three targeted bug fixes for Recent Activity sections on the Sync, Import/Export, and Maintenance pages showing empty or stale data after background jobs complete. No new components, no API shape changes, no migrations.

---

## Bug 1 — Sync page: Recent Activity never refreshes after sync completes

### Root cause

`useRecentJobs` (called inside `<RecentActivity platform={platform} />`) has no `refetchInterval` and nothing invalidates `jobsKeys.recent(platform)` after a sync completes. The data from the initial mount remains stale forever.

The active job is tracked via `useJob(status?.activeJobId)`, which polls while the job is in progress and stops automatically when it becomes terminal. There is no downstream effect wired to that terminal transition.

### Fix

**File:** `ui/frontend/src/routes/_authenticated/sync/$platform.tsx`

Add a `useEffect` after the `activeJob` declaration. A `useRef` tracks the last job ID that has already been invalidated so the effect fires exactly once per job — safe against re-renders and cached terminal state on mount:

```ts
const invalidatedJobRef = useRef<string | undefined>(undefined);
useEffect(() => {
  if (activeJob?.isTerminal && activeJob.id !== invalidatedJobRef.current) {
    invalidatedJobRef.current = activeJob.id;
    queryClient.invalidateQueries({ queryKey: jobsKeys.recent(platform) });
  }
}, [activeJob?.isTerminal, activeJob?.id, platform, queryClient]);
```

`queryClient` is already used on this page (for other invalidations); `useRef` is already imported from React.

---

## Bug 2 — Import/Export page: Dismissed job never appears in Recent Activity

### Root cause

`<RecentActivity>` on the import/export page is rendered with:

```tsx
excludeJobIds={[activeImportJob?.id, activeExportJob?.id].filter((id): id is string => !!id)}
```

`activeImportJob` and `activeExportJob` come directly from `useActiveJob`, which caches the last polled value even after polling stops. After dismissal, both IDs remain populated, so the just-dismissed job is permanently excluded.

The maintenance page uses the correct pattern: `excludeJobIds={activeJob ? [activeJob.id] : []}`. `activeJob` there is computed from the dismissed-job state — it is `null` after dismissal, so the exclusion list becomes empty.

### Fix

**File:** `ui/frontend/src/routes/_authenticated/import-export.tsx` line 440

```tsx
// before
excludeJobIds={[activeImportJob?.id, activeExportJob?.id].filter((id): id is string => !!id)}

// after
excludeJobIds={activeJob ? [activeJob.id] : []}
```

`activeJob` is already computed earlier in the component (line 233) via the `getActiveJob()` function, which returns `null` when the active job has been dismissed.

---

## Bug 3 — Import/Export and Maintenance pages: Progress counts always show zero

### Root cause

`HandleListJobs` builds all job response DTOs using a hardcoded `emptyProgress` map (all zeros) instead of fetching real item counts:

```go
emptyProgress := map[string]any{
    "pending": 0, "processing": 0, "completed": 0, ...
}
for i := range jobs {
    jobDTOs = append(jobDTOs, toJobResponse(&jobs[i], emptyProgress))
}
```

`components/jobs/recent-activity.tsx` calls `useJobs` → `HandleListJobs` and renders `job.progress.completed` / `job.progress.failed` from these zero values.

`HandleGetJob` and `HandleActiveJob` both call `h.jobItemCounts(ctx, job.ID)` correctly — `HandleListJobs` was the only endpoint that skipped it.

### Fix

**File:** `internal/api/jobs.go`

Replace the `emptyProgress` loop with per-job `jobItemCounts` calls (N+1 is acceptable at the default page size of 20):

```go
jobDTOs := make([]map[string]any, 0, len(jobs))
for i := range jobs {
    progress := h.jobItemCounts(context.Background(), jobs[i].ID)
    jobDTOs = append(jobDTOs, toJobResponse(&jobs[i], progress))
}
```

The existing `jobItemCounts` helper is unchanged.

---

## Testing

Bug 3 requires a backend test: verify that `HandleListJobs` returns non-zero progress counts for a job that has completed items. The existing test pattern in `internal/api/` uses a shared PostgreSQL container via `TestMain`.

Bugs 1 and 2 are trivial one-line wiring fixes matching existing patterns — no new tests.

---

## Pages affected

| Page | Bug | Fix location |
|------|-----|--------------|
| Sync (`/sync/:platform`) | Recent Activity stale after sync | `sync/$platform.tsx` — add `useEffect` |
| Import/Export | Dismissed job excluded from Recent Activity | `import-export.tsx` line 440 — fix `excludeJobIds` |
| Import/Export | Progress counts show zero | `jobs.go` `HandleListJobs` — call `jobItemCounts` |
| Maintenance | Progress counts show zero | `jobs.go` `HandleListJobs` — call `jobItemCounts` |
