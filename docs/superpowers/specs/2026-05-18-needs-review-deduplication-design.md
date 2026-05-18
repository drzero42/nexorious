# Needs Review Deduplication Design

**Date:** 2026-05-18
**Status:** Approved

## Problem

PSN assigns distinct `external_id` values per platform — PS4 titles get a `CUSA*` ID, PS5 titles get a `PPSA*` ID. When a game exists on both platforms, the sync creates two separate `external_games` rows and two `job_items` with the same `source_title`. If neither auto-matches to IGDB, both land in `pending_review` and the Needs Review list shows the same title twice.

The sibling resolution mechanism in `HandleResolveItem` already propagates a match to all `external_games` sharing the same `user_id + storefront + title`, so resolving one item automatically handles the other. The problem is purely in display: the user sees a duplicate they do not need to act on.

## Goal

De-duplicate the Needs Review list and badge count so each game title appears exactly once, regardless of how many platform SKUs are in `pending_review` for it. No schema changes. No dispatch changes. No frontend changes.

## Design

### 1. List query — `HandleGetJobItems` (`internal/api/jobs.go`)

For `status=pending_review`, replace the Bun query builder path with raw SQL using `DISTINCT ON`:

**Count:**
```sql
SELECT COUNT(DISTINCT source_title)
FROM job_items
WHERE job_id = ? AND status = 'pending_review'
```

**List:**
```sql
SELECT DISTINCT ON (source_title) *
FROM job_items
WHERE job_id = ? AND status = 'pending_review'
ORDER BY source_title ASC
LIMIT ? OFFSET ?
```

`DISTINCT ON (source_title)` returns one row per unique title. `ORDER BY source_title ASC` satisfies PostgreSQL's requirement that the DISTINCT ON expression leads the ORDER BY, and gives the correct alphabetical sort. Which representative row is picked is intentionally unspecified — resolving any sibling propagates to all others.

All other statuses keep the existing Bun query builder path unchanged.

### 2. Badge count — `HandlePendingReviewCount` (`internal/api/jobs.go`)

Change `COUNT(*)` to `COUNT(DISTINCT ji.source_title)` in the existing raw query:

```sql
SELECT j.source, COUNT(DISTINCT ji.source_title) AS count
FROM job_items ji
JOIN jobs j ON ji.job_id = j.id
WHERE ji.user_id = ? AND ji.status = 'pending_review'
  AND j.status IN ('pending', 'processing')
GROUP BY j.source
```

This reports the number of unique titles needing review per source, matching what the user sees in the list.

### 3. Tests (`internal/api/jobs_test.go`)

Two new tests:

- `TestHandleGetJobItems_DeduplicatesPendingReview` — insert two `job_items` for the same job with the same `source_title` but different `item_key` (simulating PS4 + PS5 SKUs), both `pending_review`. Assert the response contains exactly one item and `total = 1`.
- `TestPendingReviewCount_Deduplicates` — insert two `pending_review` items with the same `source_title` for a PSN job. Assert `pending_review_count = 1` and `counts_by_source["psn"] = 1`.

## Out of scope

- Deduplication at dispatch time — both `external_games` rows and both `job_items` must still be created; the sibling resolution logic depends on them existing.
- Frontend changes — the list and pagination already work correctly once the backend returns deduplicated results.
- Steam sync — Steam uses a single `external_id` per title; duplicates cannot arise.
- The `HandleSkipItem` endpoint — skipping the representative item leaves the sibling's `job_item` in `pending_review`; on the next fetch it becomes the new representative. This is acceptable behaviour.
