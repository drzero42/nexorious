# Nexorious Import Parallelization Design

## Context
Current Nexorious JSON imports run as a single TaskIQ job, blocking parallel processing and tying progress/cancellation to one worker. We need fan-out per item so multiple workers can process in parallel while keeping a single parent job for tracking.

## Goals
- Fan-out Nexorious import into per-item jobs (games + wishlist) on `QUEUE_HIGH`.
- Keep the existing parent job as the primary progress/status tracker.
- Support cancellation: cancelling parent stops new work and causes in-flight children to exit early (best effort).
- Support deletion: deleting parent deletes all child jobs and prevents further writes.
- Maintain existing counters/progress fields and result_summary shape.

## Non-Goals
- No schema changes to import payloads.
- No change to API surface or auth.
- No batching of other import types in this iteration.

## Architecture
- Parent job (existing Job row): created by API, validates payload, sets `progress_total = games + wishlist`, `status = PROCESSING`, stores `_import_data` only until enqueue completes.
- Child jobs (new Job rows): one per game and wishlist item. Fields: `parent_job_id` (FK to jobs, ON DELETE CASCADE), `job_type=import`, `source=nexorious`, `priority=high`, `status=pending`. Each child runs a TaskIQ task that processes exactly one item.
- Task fan-out: parent task enqueues N children (games) + M children (wishlist). Parent does not process items directly.
- Completion: each child calls aggregation helper to update parent counters and `progress_current`; when `progress_current == progress_total`, parent is marked COMPLETED and `completed_at` set. Optional periodic safety checker can re-finalize idempotently.

## Workflow
1) API creates parent job and enqueues parent task (as today). Parent task loads `_import_data`, validates export version, sets progress totals.
2) Parent enqueues child tasks with payload `{parent_job_id, child_job_id, user_id, item_type, index, payload}`; creates corresponding child Job rows.
3) Parent clears `_import_data` after successful enqueue to reduce payload size.
4) Children run per item using refactored helpers (current `_process_nexorious_game` / `_process_wishlist_item`).
5) Child writes its own Job status/result, then calls aggregation helper to bump parent progress/counters and optionally append to `error_log`. Aggregation checks for duplicate processing to avoid double increments on retries.
6) When totals match, parent is marked COMPLETED. Parent remains PROCESSING otherwise.

## Cancellation
- Cancelling parent sets `status=CANCELLED`, attempts TaskIQ revocation of queued children (if available), and prevents further enqueue.
- Child start: reload parent; if missing or not active (cancelled/deleted), mark child CANCELLED, increment parent `progress_current` and optional `cancelled_children`, then exit without user/game writes.
- During long work: child re-checks parent before final DB writes (post-IGDB fetch) to honor mid-flight cancellations.

## Deletion
- Parent deletion flow: mark parent CANCELLED, attempt to revoke queued children, then delete parent. DB FK ON DELETE CASCADE removes child Job rows. Any running child that wakes up without a parent exits early and performs no writes.

## Data Contracts
- Child result_summary: `{status: imported|already_in_collection|skipped_no_igdb_id|skipped_invalid|error|cancelled, igdb_id, title, item_type, error?}`.
- Parent counters: reuse existing (`imported`, `already_in_collection`, `skipped_invalid`, `skipped_no_igdb_id`, `errors`, `wishlist_imported`, `wishlist_already_exists`, `wishlist_errors`); add `cancelled_children` if needed.
- Aggregation helper: transactional update of parent progress/counters and optional `error_log` entry `{child_job_id, index, title, item_type, error}`; guard against double-count via processed IDs list.

## Error Handling & Retries
- Child catches exceptions, logs, sets own status FAILED, increments parent `errors` and `progress_current` via aggregation.
- IntegrityError on duplicates remains handled as today in item helpers.
- Retried child should not double-increment parent; use processed ID guard.

## Testing & Rollout
- Fan-out: enqueuing creates N+M child Job rows/tasks.
- Aggregation: children increment parent progress/counters; parent completes when totals match.
- Idempotency: retrying a child doesnt double-count.
- Cancellation: parent CANCELLED causes new/in-flight children to exit early; parent remains CANCELLED with progress advanced.
- Deletion: deleting parent deletes children and prevents writes from late children.
- Error path: child exception increments `errors` and progress; parent still completes when all children finish.
- Wishlist parity: wishlist items follow same child flow.

## Migration Notes
- Add `parent_job_id` FK (ON DELETE CASCADE) to jobs table.
- Ensure parent enqueue clears `_import_data` after fan-out.
- Wire TaskIQ revocation if supported; otherwise rely on early-exit checks.
