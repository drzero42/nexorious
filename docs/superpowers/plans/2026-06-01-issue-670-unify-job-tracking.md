# Unify Import/Export and Sync Job Tracking — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the permanently-empty Recent Activity on the Import/Export page by adding a generic job-type status endpoint and unifying all job-progress tracking (import/export, maintenance, sidebar) onto the same pattern sync already uses, then retiring `useActiveJob`.

**Architecture:** A new `GET /api/jobs/status/:job_type` returns `{is_active, active_job_id, last_completed_job_id, last_completed_at}`. A `useJobTypeStatus` hook polls it (30 s baseline, 3 s active). A shared `useJobCompletionEffect` hook detects the `active_job_id` non-null → null transition and invalidates dependent queries. All three current `useActiveJob` consumers migrate to this; `useActiveJob` and the `/active/:job_type` endpoint are then deleted.

**Tech Stack:** Go + Echo v5 + Bun (backend); React 19 + TanStack Query + Vitest + MSW (frontend).

**Reference spec:** `docs/superpowers/specs/2026-06-01-issue-670-unify-job-tracking-design.md`

---

## File Structure

**Backend**
- Modify `internal/api/jobs.go` — add `HandleJobTypeStatus`, remove `HandleActiveJob`.
- Modify `internal/api/router.go:285` — swap the route registration.
- Modify `internal/api/jobs_test.go` — add status-endpoint tests, remove active-endpoint tests.
- Modify `slumber.yaml` — add `get_status`, remove `active_job`.

**Frontend**
- Modify `ui/frontend/src/types/jobs.ts` — add `JobTypeStatus` interface.
- Modify `ui/frontend/src/api/jobs.ts` — add `getJobTypeStatus`, remove `getActiveJob`.
- Modify `ui/frontend/src/hooks/use-jobs.ts` — add `useJobTypeStatus` + `jobsKeys.typeStatus`, remove `useActiveJob` + `jobsKeys.active`.
- Create `ui/frontend/src/hooks/use-job-completion-effect.ts` + `.test.ts`.
- Modify `ui/frontend/src/hooks/index.ts` — update barrel exports.
- Modify `ui/frontend/src/hooks/use-import-export.ts` — optimistic status writes.
- Modify `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` — use shared hook.
- Modify `ui/frontend/src/routes/_authenticated/import-export.tsx` — rewrite tracking.
- Modify `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` — rewrite tracking.
- Modify `ui/frontend/src/routes/_authenticated.tsx` + `.test.tsx` — rewrite sidebar invalidation.

---

## Task 1: Backend status endpoint

**Files:**
- Modify: `internal/api/jobs.go` (add handler after `HandleActiveJob`, ~line 306)
- Modify: `internal/api/router.go:285`
- Test: `internal/api/jobs_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/jobs_test.go` (after `TestHandleActiveJob_FallbackToCompleted`, before the `TestHandleRecentJobs` divider):

```go
// ─── TestHandleJobTypeStatus ──────────────────────────────────────────────────

func TestHandleJobTypeStatus_NoJobs(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	_, token := setupTagUser(t, testDB, e, "jobs-status-none")

	rec := getAuth(t, e, "/api/jobs/status/import", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["is_active"].(bool) {
		t.Fatal("expected is_active=false")
	}
	if resp["active_job_id"] != nil {
		t.Fatalf("expected active_job_id=null, got %v", resp["active_job_id"])
	}
	if resp["last_completed_job_id"] != nil {
		t.Fatalf("expected last_completed_job_id=null, got %v", resp["last_completed_job_id"])
	}
}

func TestHandleJobTypeStatus_ActiveJob(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-status-active")

	insertJob(t, testDB, "job-status-active", userID, "import", "nexorious", "processing")

	rec := getAuth(t, e, "/api/jobs/status/import", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp["is_active"].(bool) {
		t.Fatal("expected is_active=true")
	}
	if resp["active_job_id"] != "job-status-active" {
		t.Fatalf("expected active_job_id=job-status-active, got %v", resp["active_job_id"])
	}
}

func TestHandleJobTypeStatus_LastCompleted(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithPool(t, testDB)
	userID, token := setupTagUser(t, testDB, e, "jobs-status-completed")

	// Completed job with an explicit completed_at; no active job of this type.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at, completed_at)
		 VALUES ('job-status-done', ?, 'export', 'nexorious', 'completed', 'high', now(), now())`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert completed job: %v", err)
	}

	rec := getAuth(t, e, "/api/jobs/status/export", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["is_active"].(bool) {
		t.Fatal("expected is_active=false")
	}
	if resp["last_completed_job_id"] != "job-status-done" {
		t.Fatalf("expected last_completed_job_id=job-status-done, got %v", resp["last_completed_job_id"])
	}
	if resp["last_completed_at"] == nil {
		t.Fatal("expected last_completed_at to be set")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/ -run TestHandleJobTypeStatus -v`
Expected: FAIL — route returns 404 / handler does not exist (Echo will match `/:id` and 404 on `status`).

- [ ] **Step 3: Add the handler**

In `internal/api/jobs.go`, insert after `HandleActiveJob` (after line 306, before the `syncChangeItem` type at line 308):

```go
// HandleJobTypeStatus handles GET /api/jobs/status/:job_type.
// Lightweight status for any job type: the current active job (if any) plus the
// most recent terminal job, so the UI can poll continuously and detect
// completion via the active_job_id non-null → null transition.
func (h *JobsHandler) HandleJobTypeStatus(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	jobType := c.Param("job_type")
	ctx := context.Background()

	var activeJobID *string
	var activeID string
	err := h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND job_type = ? AND status IN ('pending', 'processing') ORDER BY created_at DESC LIMIT 1`,
		userID, jobType,
	).Scan(ctx, &activeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job status")
	}
	if err == nil {
		activeJobID = &activeID
	}

	var lastCompletedJobID *string
	var lastCompletedAt *time.Time
	var last struct {
		ID          string     `bun:"id"`
		CompletedAt *time.Time `bun:"completed_at"`
	}
	err = h.db.NewRaw(
		`SELECT id, completed_at FROM jobs WHERE user_id = ? AND job_type = ? AND status IN ('completed', 'failed', 'cancelled') ORDER BY completed_at DESC NULLS LAST, created_at DESC LIMIT 1`,
		userID, jobType,
	).Scan(ctx, &last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get job status")
	}
	if err == nil {
		lastCompletedJobID = &last.ID
		lastCompletedAt = last.CompletedAt
	}

	return c.JSON(http.StatusOK, map[string]any{
		"is_active":             activeJobID != nil,
		"active_job_id":         activeJobID,
		"last_completed_job_id": lastCompletedJobID,
		"last_completed_at":     lastCompletedAt,
	})
}
```

(No new imports needed: `context`, `database/sql`, `errors`, `net/http`, `time` are all already imported in `jobs.go`.)

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, add the new route next to the other static-prefix routes — **before** `GET /:id`. Insert after line 285 (`jobsGroup.GET("/active/:job_type", jh.HandleActiveJob)`):

```go
		jobsGroup.GET("/status/:job_type", jh.HandleJobTypeStatus)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/api/ -run TestHandleJobTypeStatus -v`
Expected: PASS (all three).

- [ ] **Step 6: Add the slumber entry**

In `slumber.yaml`, add after the `active_job` block (after line 369):

```yaml
      get_status:
        name: Get Job Type Status
        method: GET
        url: "{{base_url}}/api/jobs/status/import"
        $ref: "#/.authenticated"
```

Run: `slumber collection`
Expected: collection loads without errors.

- [ ] **Step 7: Commit**

```bash
git add internal/api/jobs.go internal/api/router.go internal/api/jobs_test.go slumber.yaml
git commit -m "feat: add GET /api/jobs/status/:job_type endpoint"
```

---

## Task 2: Frontend status type, api function, and hook

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts`
- Modify: `ui/frontend/src/api/jobs.ts`
- Modify: `ui/frontend/src/hooks/use-jobs.ts`
- Modify: `ui/frontend/src/hooks/index.ts`
- Test: `ui/frontend/src/hooks/use-jobs.test.ts`

- [ ] **Step 1: Add the `JobTypeStatus` type**

In `ui/frontend/src/types/jobs.ts`, add (near the other job interfaces):

```ts
export interface JobTypeStatus {
  isActive: boolean;
  activeJobId: string | null;
  lastCompletedJobId: string | null;
  lastCompletedAt: string | null;
}
```

- [ ] **Step 2: Write the failing hook test**

In `ui/frontend/src/hooks/use-jobs.test.ts`, add `useJobTypeStatus` to the imports from `./use-jobs`, then add this test inside the top-level `describe` (or a new one):

```ts
describe('useJobTypeStatus', () => {
  it('fetches and transforms job type status', async () => {
    server.use(
      http.get(`${API_URL}/jobs/status/import`, () =>
        HttpResponse.json({
          is_active: true,
          active_job_id: 'job-9',
          last_completed_job_id: 'job-8',
          last_completed_at: '2026-01-01T00:00:00Z',
        }),
      ),
    );

    const { result } = renderHook(() => useJobTypeStatus(JobType.IMPORT), {
      wrapper: QueryWrapper,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({
      isActive: true,
      activeJobId: 'job-9',
      lastCompletedJobId: 'job-8',
      lastCompletedAt: '2026-01-01T00:00:00Z',
    });
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run (from `ui/frontend/`): `npm run test -- use-jobs`
Expected: FAIL — `useJobTypeStatus` is not exported.

- [ ] **Step 4: Add the api function**

In `ui/frontend/src/api/jobs.ts`, add `JobTypeStatus` to the type imports from `@/types`, then add an api-response interface near the other `*ApiResponse` interfaces:

```ts
interface JobTypeStatusApiResponse {
  is_active: boolean;
  active_job_id: string | null;
  last_completed_job_id: string | null;
  last_completed_at: string | null;
}
```

and add the function (place it right after `getActiveJob`, which is removed in Task 8 — for now add it after `getJob`):

```ts
/**
 * Get lightweight status for a job type: the active job (if any) and the most
 * recent terminal job. Used for continuous polling + completion detection.
 */
export async function getJobTypeStatus(jobType: JobType): Promise<JobTypeStatus> {
  const response = await api.get<JobTypeStatusApiResponse>(`/jobs/status/${jobType}`);
  return {
    isActive: response.is_active,
    activeJobId: response.active_job_id,
    lastCompletedJobId: response.last_completed_job_id,
    lastCompletedAt: response.last_completed_at,
  };
}
```

- [ ] **Step 5: Add the hook**

In `ui/frontend/src/hooks/use-jobs.ts`:

Add `JobTypeStatus` to the `import type { ... } from '@/types'` block.

Add a query key to the `jobsKeys` object (after the `active` key, line 29):

```ts
  typeStatus: (jobType: JobType) => [...jobsKeys.all, 'typeStatus', jobType] as const,
```

Add the hook (after `useActiveJob`, ~line 131):

```ts
/**
 * Hook to fetch lightweight status for a job type. Polls every 30 s at baseline
 * and every 3 s while a job is active — the baseline poll catches background
 * jobs and reliably detects completion.
 */
export function useJobTypeStatus(jobType: JobType, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.typeStatus(jobType),
    queryFn: () => jobsApi.getJobTypeStatus(jobType),
    enabled: options?.enabled,
    refetchInterval: (query) => {
      const data = query.state.data as JobTypeStatus | undefined;
      return data?.isActive ? 3000 : 30000;
    },
  });
}
```

- [ ] **Step 6: Export from the barrel**

In `ui/frontend/src/hooks/index.ts`, add `useJobTypeStatus,` to the export block from `./use-jobs` (after `useActiveJob,` on line 94).

- [ ] **Step 7: Run the test to verify it passes**

Run (from `ui/frontend/`): `npm run test -- use-jobs`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add ui/frontend/src/types/jobs.ts ui/frontend/src/api/jobs.ts ui/frontend/src/hooks/use-jobs.ts ui/frontend/src/hooks/index.ts ui/frontend/src/hooks/use-jobs.test.ts
git commit -m "feat: add useJobTypeStatus hook"
```

---

## Task 3: Shared `useJobCompletionEffect` hook

**Files:**
- Create: `ui/frontend/src/hooks/use-job-completion-effect.ts`
- Create: `ui/frontend/src/hooks/use-job-completion-effect.test.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/hooks/use-job-completion-effect.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useJobCompletionEffect } from './use-job-completion-effect';

describe('useJobCompletionEffect', () => {
  it('does not fire on mount', () => {
    const onComplete = vi.fn();
    renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: 'job-1' as string | null },
    });
    expect(onComplete).not.toHaveBeenCalled();
  });

  it('does not fire on null -> non-null', () => {
    const onComplete = vi.fn();
    const { rerender } = renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: null as string | null },
    });
    rerender({ id: 'job-1' });
    expect(onComplete).not.toHaveBeenCalled();
  });

  it('fires once on non-null -> null', () => {
    const onComplete = vi.fn();
    const { rerender } = renderHook(({ id }) => useJobCompletionEffect(id, onComplete), {
      initialProps: { id: 'job-1' as string | null },
    });
    rerender({ id: null });
    expect(onComplete).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run (from `ui/frontend/`): `npm run test -- use-job-completion-effect`
Expected: FAIL — module does not exist.

- [ ] **Step 3: Create the hook**

Create `ui/frontend/src/hooks/use-job-completion-effect.ts`:

```ts
import { useEffect, useRef } from 'react';

/**
 * Calls `onComplete` when `activeJobId` transitions from a non-null value to
 * null/undefined — the signal that a tracked job has finished. Does not fire on
 * mount or on the null → non-null transition.
 *
 * Callers should memoise `onComplete` (useCallback) so the effect deps stay
 * stable.
 */
export function useJobCompletionEffect(
  activeJobId: string | null | undefined,
  onComplete: () => void,
) {
  const prevRef = useRef<string | null>(null);
  useEffect(() => {
    if (prevRef.current && !activeJobId) {
      onComplete();
    }
    prevRef.current = activeJobId ?? null;
  }, [activeJobId, onComplete]);
}
```

- [ ] **Step 4: Export from the barrel**

In `ui/frontend/src/hooks/index.ts`, add after the `./use-jobs` export block:

```ts
export { useJobCompletionEffect } from './use-job-completion-effect';
```

- [ ] **Step 5: Run the test to verify it passes**

Run (from `ui/frontend/`): `npm run test -- use-job-completion-effect`
Expected: PASS (all three).

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/hooks/use-job-completion-effect.ts ui/frontend/src/hooks/use-job-completion-effect.test.ts ui/frontend/src/hooks/index.ts
git commit -m "feat: add shared useJobCompletionEffect hook"
```

---

## Task 4: Refactor sync onto the shared hook

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx:1,182-191`

This is a pure refactor — behaviour is unchanged. Verified by the existing sync tests.

- [ ] **Step 1: Update the React import**

Change line 1 of `$storefront.tsx` from:

```ts
import { useEffect, useRef, useState } from 'react';
```

to:

```ts
import { useCallback, useEffect, useRef, useState } from 'react';
```

- [ ] **Step 2: Add `useJobCompletionEffect` to the hooks import**

In the `from '@/hooks'` block (lines 5–19), add `useJobCompletionEffect,` (e.g. after `jobsKeys,`).

- [ ] **Step 3: Replace the inline ref/effect**

Replace lines 182–191 (the `previousActiveJobIdRef` block):

```ts
  const previousActiveJobIdRef = useRef<string | null>(null);
  useEffect(() => {
    const previous = previousActiveJobIdRef.current;
    const current = status?.activeJobId ?? null;
    previousActiveJobIdRef.current = current;
    if (previous && !current) {
      queryClient.invalidateQueries({ queryKey: jobsKeys.recent(storefront) });
      queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(storefront) });
    }
  }, [status?.activeJobId, storefront, queryClient]);
```

with:

```ts
  const handleSyncComplete = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: jobsKeys.recent(storefront) });
    queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(storefront) });
  }, [queryClient, storefront]);
  useJobCompletionEffect(status?.activeJobId, handleSyncComplete);
```

- [ ] **Step 4: Run the sync tests and typecheck**

Run (from `ui/frontend/`): `npm run test -- sync && npm run check`
Expected: PASS, no type errors. (If `useRef` is now unused anywhere in the file, ESLint/`npm run check` will flag it — it is still used by `wasResettingRef`/`connectionOpenInitialized`, so it stays.)

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/sync/\$storefront.tsx
git commit -m "refactor: use shared useJobCompletionEffect in sync route"
```

---

## Task 5: Optimistic status update in import/export mutations

**Files:**
- Modify: `ui/frontend/src/hooks/use-import-export.ts`

- [ ] **Step 1: Rewrite the mutation hooks**

Replace the entire contents of `ui/frontend/src/hooks/use-import-export.ts` with:

```ts
import { useMutation, useQueryClient } from '@tanstack/react-query';
import * as importExportApi from '@/api/import-export';
import { JobType } from '@/types';
import type {
  ImportJobCreatedResponse,
  ExportJobCreatedResponse,
  ExportFormat,
  JobTypeStatus,
} from '@/types';
import { jobsKeys } from './use-jobs';

// ============================================================================
// Query Keys
// ============================================================================

export const importExportKeys = {
  all: ['import-export'] as const,
  jobs: () => [...importExportKeys.all, 'jobs'] as const,
};

// Optimistically mark a job type active so the progress card appears
// immediately, without waiting for the next status poll. Mirrors useTriggerSync.
function markJobTypeActive(
  queryClient: ReturnType<typeof useQueryClient>,
  jobType: JobType,
  jobId: string,
) {
  queryClient.setQueryData<JobTypeStatus>(jobsKeys.typeStatus(jobType), (old) => ({
    isActive: true,
    activeJobId: jobId,
    lastCompletedJobId: old?.lastCompletedJobId ?? null,
    lastCompletedAt: old?.lastCompletedAt ?? null,
  }));
  queryClient.invalidateQueries({ queryKey: jobsKeys.typeStatus(jobType) });
}

// ============================================================================
// Import Mutation Hooks
// ============================================================================

/**
 * Hook to import games from a Nexorious JSON export file.
 * Non-interactive import that trusts IGDB IDs.
 */
export function useImportNexorious() {
  const queryClient = useQueryClient();
  return useMutation<ImportJobCreatedResponse, Error, File>({
    mutationFn: (file) => importExportApi.importNexoriousJson(file),
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.IMPORT, result.job_id);
    },
  });
}

// ============================================================================
// Export Mutation Hooks
// ============================================================================

/**
 * Hook to start an export of all user games.
 * Returns the job ID for tracking progress.
 */
export function useExportCollection() {
  const queryClient = useQueryClient();
  return useMutation<ExportJobCreatedResponse, Error, ExportFormat>({
    mutationFn: (format) => {
      if (format === 'json') {
        return importExportApi.exportCollectionJson();
      }
      return importExportApi.exportCollectionCsv();
    },
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.EXPORT, result.job_id);
    },
  });
}

/**
 * Hook to download a completed export file.
 */
export function useDownloadExport() {
  return useMutation<{ blob: Blob; filename: string }, Error, string>({
    mutationFn: (jobId) => importExportApi.downloadExport(jobId),
    onSuccess: ({ blob, filename }) => {
      importExportApi.triggerBlobDownload(blob, filename);
    },
  });
}
```

- [ ] **Step 2: Run the import-export hook tests and typecheck**

Run (from `ui/frontend/`): `npm run test -- use-import-export && npm run check`
Expected: PASS. If the existing `use-import-export.test.ts` asserts there is no `onSuccess` side effect, update it to wrap the hook in a real `QueryClientProvider` (the `QueryWrapper` from `@/test/test-utils`) — the mutation now reads/writes the query cache.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/hooks/use-import-export.ts ui/frontend/src/hooks/use-import-export.test.ts
git commit -m "feat: optimistically mark job type active on import/export start"
```

---

## Task 6: Rewrite `import-export.tsx` tracking

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/import-export.tsx`

The `getActiveJob()` selection logic, `dismissedJobId`, `excludeJobIds`, and the
download/cancel handlers stay; only the **data source** changes from
`useActiveJob` to `useJobTypeStatus` + `useJob(active ?? lastCompleted)`, plus a
completion effect that invalidates Recent Activity.

- [ ] **Step 1: Update imports**

Replace the `from '@/hooks'` import block (lines 4–10):

```ts
import {
  useImportNexorious,
  useExportCollection,
  useActiveJob,
  useCancelJob,
  useDownloadExport,
} from '@/hooks';
```

with:

```ts
import {
  useImportNexorious,
  useExportCollection,
  useJob,
  useJobTypeStatus,
  useJobCompletionEffect,
  useCancelJob,
  useDownloadExport,
  jobsKeys,
} from '@/hooks';
```

Change the React import on line 1:

```ts
import { useRef, useState } from 'react';
```

to:

```ts
import { useCallback, useRef, useState } from 'react';
```

Add a TanStack Query import (after line 2's react-router import):

```ts
import { useQueryClient } from '@tanstack/react-query';
```

- [ ] **Step 2: Replace the active-job data source**

Inside `ImportExportPage`, add `const queryClient = useQueryClient();` near the other hook calls (e.g. after line 209's `useDownloadExport`), then replace lines 211–213:

```ts
  // Check for active import and export jobs
  const { data: activeImportJob, refetch: refetchImport } = useActiveJob(JobType.IMPORT);
  const { data: activeExportJob, refetch: refetchExport } = useActiveJob(JobType.EXPORT);
```

with:

```ts
  // Track import/export job status (active + most recent completed) and fetch
  // the displayed job by id — falling back to the last completed job so the
  // result card (e.g. the export Download button) survives completion.
  const { data: importStatus } = useJobTypeStatus(JobType.IMPORT);
  const { data: exportStatus } = useJobTypeStatus(JobType.EXPORT);

  const importJobId = importStatus?.activeJobId ?? importStatus?.lastCompletedJobId ?? null;
  const exportJobId = exportStatus?.activeJobId ?? exportStatus?.lastCompletedJobId ?? null;

  const { data: activeImportJob } = useJob(importJobId ?? undefined);
  const { data: activeExportJob } = useJob(exportJobId ?? undefined);

  // Refresh Recent Activity when either job completes.
  const handleJobComplete = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: jobsKeys.lists() });
  }, [queryClient]);
  useJobCompletionEffect(importStatus?.activeJobId, handleJobComplete);
  useJobCompletionEffect(exportStatus?.activeJobId, handleJobComplete);
```

(The `getActiveJob()` body below this — lines 217–241 — is unchanged: it still
references `activeImportJob` / `activeExportJob`, which now resolve to `Job |
undefined` of the same shape.)

- [ ] **Step 3: Update the start handlers (remove stale refetch)**

In `handleImportFile`, replace the comment + `refetchImport();` (lines 261–264):

```ts
      // Reset dismissed job to show the new job
      setDismissedJobId(null);
      // Refetch to get the new job
      refetchImport();
```

with:

```ts
      // Reset dismissed job; the mutation optimistically marks the job active.
      setDismissedJobId(null);
```

In `handleCollectionExport`, replace the analogous lines (281–282 region):

```ts
      // Reset dismissed job to show the new job
      setDismissedJobId(null);
      // Refetch to get the new job
      refetchExport();
```

with:

```ts
      // Reset dismissed job; the mutation optimistically marks the job active.
      setDismissedJobId(null);
```

- [ ] **Step 4: Update the cancel handler**

Replace the `handleCancelJob` body (lines 291–304):

```ts
  const handleCancelJob = async () => {
    if (!activeJob) return;

    cancelJob(activeJob.id, {
      onSuccess: () => {
        toast.success('Job cancelled');
        refetchImport();
        refetchExport();
      },
      onError: (error) => {
        toast.error(error.message || 'Failed to cancel job');
      },
    });
  };
```

with:

```ts
  const handleCancelJob = async () => {
    if (!activeJob) return;

    cancelJob(activeJob.id, {
      onSuccess: () => {
        toast.success('Job cancelled');
        queryClient.invalidateQueries({ queryKey: jobsKeys.typeStatus(JobType.IMPORT) });
        queryClient.invalidateQueries({ queryKey: jobsKeys.typeStatus(JobType.EXPORT) });
      },
      onError: (error) => {
        toast.error(error.message || 'Failed to cancel job');
      },
    });
  };
```

- [ ] **Step 5: Typecheck and run any page tests**

Run (from `ui/frontend/`): `npm run check`
Expected: no type/lint errors. In particular, `JobType` is still used (in `useJobTypeStatus(JobType.IMPORT)` etc.) so its import on line ~14 stays; `useActiveJob` is no longer referenced.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/import-export.tsx
git commit -m "fix: track import/export jobs via status endpoint; refresh recent activity on completion"
```

---

## Task 7: Rewrite `admin/maintenance.tsx` tracking

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx:5,77-79,98-111`

Same migration for the single `METADATA_REFRESH` type. `queryClient` already
exists (line 46).

- [ ] **Step 1: Update imports**

Change line 5:

```ts
import { useActiveJob, useCancelJob } from '@/hooks';
```

to:

```ts
import { useJob, useJobTypeStatus, useJobCompletionEffect, useCancelJob, jobsKeys } from '@/hooks';
```

Change line 1 to add `useCallback`:

```ts
import { useState, useEffect, useCallback } from 'react';
```

- [ ] **Step 2: Replace the active-job data source**

Replace lines 76–80:

```ts
  // Track active maintenance job
  const { data: activeMaintenanceJob, refetch: refetchMaintenanceJob } = useActiveJob(
    JobType.METADATA_REFRESH,
  );
  const { mutate: cancelJob, isPending: isCancelling } = useCancelJob();
```

with:

```ts
  // Track the metadata-refresh job status, fetching the displayed job by id
  // (active job, falling back to the last completed one so the result card
  // survives completion).
  const { data: refreshStatus } = useJobTypeStatus(JobType.METADATA_REFRESH);
  const refreshJobId = refreshStatus?.activeJobId ?? refreshStatus?.lastCompletedJobId ?? null;
  const { data: activeMaintenanceJob } = useJob(refreshJobId ?? undefined);
  const { mutate: cancelJob, isPending: isCancelling } = useCancelJob();

  const handleRefreshComplete = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: jobsKeys.lists() });
  }, [queryClient]);
  useJobCompletionEffect(refreshStatus?.activeJobId, handleRefreshComplete);
```

- [ ] **Step 3: Update the start handler**

Replace `handleStartMetadataRefresh` (lines 98–111):

```ts
  const handleStartMetadataRefresh = async () => {
    try {
      setIsRefreshLoading(true);
      setDismissedJobId(null);
      await adminApi.startMetadataRefreshJob();
      toast.success('Metadata refresh job started');
      refetchMaintenanceJob();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start metadata refresh';
      toast.error(message);
    } finally {
      setIsRefreshLoading(false);
    }
  };
```

with:

```ts
  const handleStartMetadataRefresh = async () => {
    try {
      setIsRefreshLoading(true);
      setDismissedJobId(null);
      await adminApi.startMetadataRefreshJob();
      toast.success('Metadata refresh job started');
      queryClient.invalidateQueries({ queryKey: jobsKeys.typeStatus(JobType.METADATA_REFRESH) });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start metadata refresh';
      toast.error(message);
    } finally {
      setIsRefreshLoading(false);
    }
  };
```

- [ ] **Step 4: Update the cancel handler**

Replace the `refetchMaintenanceJob();` call in `handleCancelJob` (line 119) with:

```ts
        queryClient.invalidateQueries({ queryKey: jobsKeys.typeStatus(JobType.METADATA_REFRESH) });
```

- [ ] **Step 5: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: no type/lint errors. `JobType` import stays (used in `JobType.METADATA_REFRESH`); `refetchMaintenanceJob` is fully removed.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/admin/maintenance.tsx
git commit -m "refactor: track maintenance job via status endpoint"
```

---

## Task 8: Rewrite `_authenticated.tsx` sidebar invalidation

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated.tsx:1,7,15-30`
- Modify: `ui/frontend/src/routes/_authenticated.test.tsx:24-26`

- [ ] **Step 1: Update the test mock**

In `ui/frontend/src/routes/_authenticated.test.tsx`, replace the `@/hooks` mock (lines 24–26):

```ts
vi.mock('@/hooks', () => ({
  useActiveJob: () => ({ data: undefined }),
}));
```

with:

```ts
vi.mock('@/hooks', () => ({
  useJobTypeStatus: () => ({ data: undefined }),
  useJobCompletionEffect: () => {},
}));
```

- [ ] **Step 2: Run the test to verify it fails**

Run (from `ui/frontend/`): `npm run test -- _authenticated`
Expected: FAIL — `_authenticated.tsx` still imports/calls `useActiveJob`, which the mock no longer provides (TypeError / undefined).

- [ ] **Step 3: Rewrite the hook**

In `ui/frontend/src/routes/_authenticated.tsx`:

Change line 1:

```ts
import { useEffect, useRef } from 'react';
```

to:

```ts
import { useCallback } from 'react';
```

Change line 7:

```ts
import { useActiveJob } from '@/hooks';
```

to:

```ts
import { useJobTypeStatus, useJobCompletionEffect } from '@/hooks';
```

Replace `useInvalidateGamesOnImportComplete` (lines 15–30):

```ts
function useInvalidateGamesOnImportComplete() {
  const queryClient = useQueryClient();
  const { data: activeImportJob } = useActiveJob(JobType.IMPORT);
  const wasTerminalRef = useRef<boolean | undefined>(undefined);

  useEffect(() => {
    const isTerminal = activeImportJob?.isTerminal;
    if (isTerminal && wasTerminalRef.current === false) {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
    }
    if (isTerminal !== undefined) {
      wasTerminalRef.current = isTerminal;
    }
  }, [activeImportJob?.isTerminal, queryClient]);
}
```

with:

```ts
function useInvalidateGamesOnImportComplete() {
  const queryClient = useQueryClient();
  const { data: importStatus } = useJobTypeStatus(JobType.IMPORT);

  const onImportComplete = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
  }, [queryClient]);
  useJobCompletionEffect(importStatus?.activeJobId, onImportComplete);
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run (from `ui/frontend/`): `npm run test -- _authenticated && npm run check`
Expected: PASS, no type errors. (`useEffect`/`useRef` imports removed; `useQueryClient`, `gameKeys`, `JobType` all still used.)

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/routes/_authenticated.tsx ui/frontend/src/routes/_authenticated.test.tsx
git commit -m "refactor: invalidate games on import completion via status endpoint"
```

---

## Task 9: Retire `useActiveJob` (frontend)

All consumers are migrated. Remove the dead hook and api function.

**Files:**
- Modify: `ui/frontend/src/hooks/use-jobs.ts`
- Modify: `ui/frontend/src/hooks/index.ts`
- Modify: `ui/frontend/src/api/jobs.ts`
- Modify: `ui/frontend/src/hooks/use-jobs.test.ts` (if it tests `useActiveJob`)

- [ ] **Step 1: Remove the hook and its query key**

In `ui/frontend/src/hooks/use-jobs.ts`:
- Delete the `useActiveJob` function (lines ~115–131, the whole block including its doc comment).
- Delete the `active:` entry from `jobsKeys` (line 29).
- If `JobType` is now only used by `typeStatus`/`useJobTypeStatus`, leave the import (still used). If `getActiveJob` was the only thing referencing some import, the typecheck in Step 4 will catch it.

- [ ] **Step 2: Remove the barrel export**

In `ui/frontend/src/hooks/index.ts`, delete the `useActiveJob,` line (line 94).

- [ ] **Step 3: Remove the api function**

In `ui/frontend/src/api/jobs.ts`, delete the `getActiveJob` function (lines ~362–369, including its doc comment).

- [ ] **Step 4: Remove/adjust any `useActiveJob` test**

In `ui/frontend/src/hooks/use-jobs.test.ts`, remove any `useActiveJob` import and its `describe`/`it` blocks. Search first:

Run (from `ui/frontend/`): `grep -rn "useActiveJob\|getActiveJob" src`
Expected after edits: no matches.

- [ ] **Step 5: Typecheck, dead-code check, and full test run**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: PASS, zero knip findings (confirms `getActiveJob`/`useActiveJob` are fully gone and nothing else is now orphaned).

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/hooks/use-jobs.ts ui/frontend/src/hooks/index.ts ui/frontend/src/api/jobs.ts ui/frontend/src/hooks/use-jobs.test.ts
git commit -m "refactor: remove unused useActiveJob hook and getActiveJob api"
```

---

## Task 10: Remove the dead `/api/jobs/active/:job_type` endpoint

**Files:**
- Modify: `internal/api/jobs.go:263-306`
- Modify: `internal/api/router.go:285`
- Modify: `internal/api/jobs_test.go:550-602`
- Modify: `slumber.yaml:365-369`

- [ ] **Step 1: Remove the handler**

In `internal/api/jobs.go`, delete `HandleActiveJob` (lines 263–306, the whole function including its `// HandleActiveJob handles ...` comment).

- [ ] **Step 2: Remove the route**

In `internal/api/router.go`, delete line 285:

```go
		jobsGroup.GET("/active/:job_type", jh.HandleActiveJob)
```

- [ ] **Step 3: Remove the tests**

In `internal/api/jobs_test.go`, delete the three `TestHandleActiveJob_*` functions and the `// ─── TestHandleActiveJob ───` divider (lines 550–602).

- [ ] **Step 4: Remove the slumber entry**

In `slumber.yaml`, delete the `active_job:` block (lines 365–369):

```yaml
      active_job:
        name: Active Job
        method: GET
        url: "{{base_url}}/api/jobs/active/sync"
        $ref: "#/.authenticated"
```

- [ ] **Step 5: Build, test, and verify slumber**

Run:
```bash
go build ./... && go test ./internal/api/ -run 'TestHandleJobTypeStatus|TestHandleActiveJob' -v
slumber collection
```
Expected: build succeeds; `TestHandleJobTypeStatus*` PASS; no `TestHandleActiveJob*` tests found (removed); slumber loads without errors.

- [ ] **Step 6: Commit**

```bash
git add internal/api/jobs.go internal/api/router.go internal/api/jobs_test.go slumber.yaml
git commit -m "refactor: remove unused GET /api/jobs/active/:job_type endpoint"
```

---

## Task 11: Full verification & PR

- [ ] **Step 1: Backend suite**

Run: `go test -timeout 600s ./...`
Expected: PASS.

- [ ] **Step 2: Frontend suite + gates**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: PASS, zero knip findings.

- [ ] **Step 3: Manual smoke (optional but recommended)**

Build and run the app (`make && ./nexorious`), start an import, confirm:
- the active-job card appears immediately (optimistic update),
- on completion the card shows the result (Download for exports), and
- the **Recent Activity** section now lists the completed job (the bug fix).

- [ ] **Step 4: Push and open the PR**

```bash
git push -u origin fix/670-import-export-recent-activity
gh pr create --title "feat: unify import/export and sync job tracking (#670)" --body "Closes #670. Adds GET /api/jobs/status/:job_type, a useJobTypeStatus hook, and a shared useJobCompletionEffect; migrates import/export, maintenance, and the sidebar onto it; retires useActiveJob and the /api/jobs/active/:job_type endpoint. Fixes the permanently-empty Recent Activity on the Import/Export page."
```

**PR title MUST start with `feat:`** (new endpoint + architectural change).

---

## Self-Review Notes

- **Spec coverage:** status endpoint (T1) ✓, `last_completed_job_id` rationale (T1, T6, T7) ✓, `useJobTypeStatus` (T2) ✓, `useJobCompletionEffect` + sync refactor (T3, T4) ✓, optimistic update (T5) ✓, import-export rewrite (T6) ✓, maintenance rewrite (T7) ✓, sidebar rewrite + latent-bug fix (T8) ✓, retire `useActiveJob` (T9) ✓, remove dead endpoint (T10) ✓, slumber add/remove (T1/T10) ✓, PR `feat:` prefix (T11) ✓.
- **Type consistency:** `JobTypeStatus` fields (`isActive`, `activeJobId`, `lastCompletedJobId`, `lastCompletedAt`) are used identically in T2/T5/T6/T7/T8. `jobsKeys.typeStatus(jobType)` defined in T2, used in T5/T6/T7/T8. Backend JSON keys (`is_active`, `active_job_id`, `last_completed_job_id`, `last_completed_at`) match the api transform in T2.
- **Ordering:** `useActiveJob` is only deleted in T9, after every consumer (T6/T7/T8) is migrated, so the build stays green at each commit. The backend `/active` endpoint is removed in T10 after the frontend no longer calls it.
- **Out of scope:** per-outcome import summary in Recent Activity (follow-up).
