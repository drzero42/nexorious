# Nexorious Import Parallelization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Parallelize Nexorious JSON import by fanning out per-item child jobs under a parent job, with cancellation/deletion propagation and correct progress aggregation.

**Architecture:** Parent TaskIQ job enqueues per-game/per-wishlist child jobs (each a Job row with parent FK). Children process items using existing helpers, then aggregate counters/progress back to the parent. Cancellation/deletion stops or revokes children, and DB cascade removes child Job rows.

**Tech Stack:** FastAPI, SQLModel, TaskIQ (AsyncpgBroker), PostgreSQL, pytest.

---

### Task 1: Add parent_job_id to jobs table

**Files:**
- Create: `backend/app/migrations/versions/XXXX_parent_job_fk.py`
- Modify: `backend/app/models/job.py`
- Test: `backend/app/tests` (reuse existing to ensure import)

**Step 1: Write migration to add nullable `parent_job_id` FK to jobs (ON DELETE CASCADE).**

**Step 2: Update Job model to include `parent_job_id: Optional[str] = Field(foreign_key="jobs.id", default=None, index=True)` and relationship to children (if desired for querying).**

**Step 3: Run (or describe) migration command:**
`uv run alembic revision --autogenerate -m "add parent job fk"`
`uv run alembic upgrade head`

**Step 4: Verify model imports/build (run linters/tests later).**

### Task 2: Refactor per-item logic to reusable helpers

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_nexorious.py`
- Modify tests: `backend/app/tests/test_import_tasks.py`

**Step 1: Extract per-game and per-wishlist processing into standalone async functions usable by child tasks (reuse existing `_process_nexorious_game`, `_process_wishlist_item` with minimal changes for injectability and cancellation check hook).**

**Step 2: Add cancellation check hook (callable) invoked before DB writes.**

**Step 3: Adjust tests/mocks if needed to call the refactored helpers.**

### Task 3: Create child TaskIQ tasks and Job rows for items

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_nexorious.py`
- Modify: `backend/app/worker/tasks/__init__.py`
- Modify tests: `backend/app/tests/test_import_tasks.py`

**Step 1: Define child tasks `process_nexorious_game_item` and `process_nexorious_wishlist_item` that accept `{parent_job_id, child_job_id, user_id, index, payload}`.**

**Step 2: In the parent task, after validation, create child Job rows (one per item) with `parent_job_id`, `status=pending`, `job_type=import`, `source=nexorious`, `priority=high`, and enqueue the matching TaskIQ tasks.**

**Step 3: Clear `_import_data` from parent result_summary after successful enqueue.**

**Step 4: Ensure child tasks set their own status/progress and return per-item result_summary.**

### Task 4: Aggregation helper for parent progress/counters

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_nexorious.py`
- Modify: `backend/app/models/job.py` (if helper lives there) or keep in task module
- Modify tests: `backend/app/tests/test_import_tasks.py`

**Step 1: Implement `record_child_result(parent_job_id, child_job_id, result)` to reload parent, check active status, increment counters, increment `progress_current`, optionally append to `error_log`, and mark parent COMPLETED when totals match.**

**Step 2: Guard against double-count (e.g., track processed child IDs in parent `processed_item_ids_json`).**

**Step 3: Call this helper from child tasks after processing.**

### Task 5: Cancellation and deletion handling

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_nexorious.py`
- Modify: `backend/app/api/import_endpoints.py` (if deletion/cancel endpoints exist) or relevant job cancellation handler
- Modify tests: `backend/app/tests/test_import_tasks.py`, `backend/app/tests/test_worker_locking.py` (if needed)

**Step 1: In child tasks, check parent status at start; if CANCELLED or missing, mark child CANCELLED, bump parent progress (and optional cancelled counter), exit before writes. Add mid-processing cancellation check before final DB writes.**

**Step 2: In parent task, short-circuit enqueue if parent already cancelled. Attempt TaskIQ revocation of queued children on parent cancellation if supported; otherwise rely on start-time checks.**

**Step 3: Parent deletion flow: ensure DB FK cascade handles child rows; if API supports deletion, mark parent CANCELLED, attempt revocation, then delete parent.**

**Step 4: Add tests for cancellation and deletion behavior (children exit, no writes, parent progress updated, children deleted with parent).**

### Task 6: Tests and verification

**Files:**
- Modify: `backend/app/tests/test_import_tasks.py`
- Possibly add: `backend/app/tests/test_job_relations.py` (optional)

**Step 1: Add tests for fan-out count, aggregation to parent, idempotent child retry, cancellation early-exit, deletion cascade, error path aggregation, wishlist parity.**

**Step 2: Run targeted tests:**
`cd backend && uv run pytest app/tests/test_import_tasks.py -v`

**Step 3: Run full test suite (as required):**
`cd backend && uv run pytest --cov=app --cov-report=term-missing`

**Step 4: Run lint (ruff) and type check if needed:**
`cd backend && uv run ruff check .`

---

Plan complete and saved to `docs/plans/2025-12-22-nexorious-import-parallelization-plan.md`. Two execution options:

1. Subagent-Driven (this session) - I dispatch fresh subagent per task with reviews.
2. Parallel Session (separate) - Open new session and use superpowers:executing-plans to run it task-by-task.

Which approach?
