# Issue #643 — Unify pending-review count source

## Problem

`HandlePendingReviewCount` (`internal/api/jobs.go`) filters `pending_review` job items to only those whose parent job is `pending` or `processing`:

```sql
AND j.status IN ('pending', 'processing')
```

`HandleListExternalGames` (`internal/api/sync.go`) derives `sync_status = 'needs_review'` from the `pending_review` state of the job item alone, with no condition on the parent job's status.

The two sources diverge whenever `pending_review` items exist under a terminal job (e.g. after a user cancels a job, or as a result of the #642 bug). The detail page shows "Needs Review (N)" while the nav badge and service-card badge show zero.

## Root cause

The job-status guard in `HandlePendingReviewCount` is the wrong model. `pending_review` is a state on a job item that signals the user must make a decision about that game. Whether the parent job is still running is irrelevant to whether the game needs attention. `docs/sync.md` already describes the count as "aggregate count of `pending_review` items" with no job-status qualifier — the implementation was wrong relative to the spec.

## Fix

Remove the `AND j.status IN ('pending', 'processing')` join condition from `HandlePendingReviewCount`. The query already groups by `j.source` (still needed for the per-storefront breakdown) and counts `DISTINCT ji.source_title` (which naturally groups PSN siblings with the same title). Only the job-status filter is removed.

```sql
-- Before
SELECT j.source, COUNT(DISTINCT ji.source_title) AS count
FROM job_items ji
JOIN jobs j ON ji.job_id = j.id
WHERE ji.user_id = ? AND ji.status = 'pending_review'
  AND j.status IN ('pending', 'processing')
GROUP BY j.source

-- After
SELECT j.source, COUNT(DISTINCT ji.source_title) AS count
FROM job_items ji
JOIN jobs j ON ji.job_id = j.id
WHERE ji.user_id = ? AND ji.status = 'pending_review'
GROUP BY j.source
```

## Scope

This is the complete fix. No frontend changes, no skip-cascade changes, no grouping changes. Those concerns are addressed properly by the sibling refactor (see follow-up issue filed alongside this spec).

## Follow-on work

The broader sibling problem — two `external_games` rows for the same PSN game requiring separate review actions — is tracked separately and will be resolved by establishing the sibling relationship explicitly at Stage 1 via a `parent_id` field on `external_games`. That work will also update `docs/sync.md` to reflect the new sibling model.
