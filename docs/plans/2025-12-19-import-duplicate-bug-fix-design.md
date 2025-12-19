# Import Duplicate Bug Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix duplicate game creation during import by adding advisory locks and a database unique constraint.

**Architecture:** Defense in depth with two layers: (1) PostgreSQL advisory locks at task level to prevent duplicate task execution, (2) unique constraint on UserGame(user_id, game_id) as database-level safety net.

**Tech Stack:** PostgreSQL advisory locks, SQLAlchemy/SQLModel, Alembic migrations, pytest

---

## Task 1: Write Tests for Advisory Lock Utilities

**Files:**
- Create: `backend/app/tests/test_worker_locking.py`

**Step 1: Write the failing tests**

```python
"""Tests for worker advisory lock utilities."""

import pytest
from sqlmodel import Session, text

from app.worker.locking import job_id_to_lock_key, acquire_job_lock, release_job_lock


class TestJobIdToLockKey:
    """Test lock key generation from job IDs."""

    def test_returns_positive_integer(self):
        """Lock key is always a positive integer."""
        key = job_id_to_lock_key("test-job-123")
        assert isinstance(key, int)
        assert key >= 0

    def test_same_job_id_same_key(self):
        """Same job ID produces same lock key."""
        key1 = job_id_to_lock_key("job-abc-123")
        key2 = job_id_to_lock_key("job-abc-123")
        assert key1 == key2

    def test_different_job_ids_different_keys(self):
        """Different job IDs produce different lock keys."""
        key1 = job_id_to_lock_key("job-abc-123")
        key2 = job_id_to_lock_key("job-xyz-456")
        assert key1 != key2

    def test_fits_in_bigint(self):
        """Lock key fits in PostgreSQL bigint range."""
        key = job_id_to_lock_key("test-job-with-long-uuid-identifier")
        # PostgreSQL bigint max is 2^63-1
        assert key <= 0x7FFFFFFFFFFFFFFF


class TestAcquireJobLock:
    """Test advisory lock acquisition."""

    def test_acquire_lock_succeeds_when_available(self, session: Session):
        """Lock acquisition returns True when lock is available."""
        result = acquire_job_lock(session, "test-job-001")
        assert result is True
        # Clean up
        release_job_lock(session, "test-job-001")

    def test_acquire_lock_fails_when_held(self, session: Session):
        """Lock acquisition returns False when another session holds the lock."""
        from app.core.database import get_engine

        # First session acquires the lock
        result1 = acquire_job_lock(session, "test-job-002")
        assert result1 is True

        # Second session tries to acquire same lock
        with Session(get_engine()) as session2:
            result2 = acquire_job_lock(session2, "test-job-002")
            assert result2 is False

        # Clean up
        release_job_lock(session, "test-job-002")

    def test_different_jobs_can_lock_simultaneously(self, session: Session):
        """Different jobs can be locked by different sessions."""
        from app.core.database import get_engine

        # First session locks job A
        result1 = acquire_job_lock(session, "job-A")
        assert result1 is True

        # Second session locks job B (should succeed)
        with Session(get_engine()) as session2:
            result2 = acquire_job_lock(session2, "job-B")
            assert result2 is True
            release_job_lock(session2, "job-B")

        # Clean up
        release_job_lock(session, "job-A")


class TestReleaseJobLock:
    """Test advisory lock release."""

    def test_release_allows_reacquisition(self, session: Session):
        """After release, another session can acquire the lock."""
        from app.core.database import get_engine

        # Acquire and release
        acquire_job_lock(session, "test-job-003")
        release_job_lock(session, "test-job-003")

        # Another session can now acquire
        with Session(get_engine()) as session2:
            result = acquire_job_lock(session2, "test-job-003")
            assert result is True
            release_job_lock(session2, "test-job-003")

    def test_release_nonexistent_lock_is_safe(self, session: Session):
        """Releasing a lock that wasn't acquired doesn't raise."""
        # Should not raise
        release_job_lock(session, "never-acquired-job")
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_worker_locking.py -v`
Expected: FAIL with "ModuleNotFoundError: No module named 'app.worker.locking'"

**Step 3: Commit failing tests**

```bash
git add backend/app/tests/test_worker_locking.py
git commit -m "test: add failing tests for advisory lock utilities"
```

---

## Task 2: Implement Advisory Lock Utilities

**Files:**
- Create: `backend/app/worker/locking.py`

**Step 1: Write minimal implementation**

```python
"""Advisory lock utilities for preventing duplicate task execution.

PostgreSQL advisory locks are used to ensure only one worker processes
a given job, even when the taskiq-pg broker broadcasts to all workers.
"""

from sqlmodel import Session, text


def job_id_to_lock_key(job_id: str) -> int:
    """Convert a job ID string to a PostgreSQL advisory lock key.

    Advisory locks use bigint keys. We hash the job_id and mask to ensure
    it fits in the positive bigint range (0 to 2^63-1).

    Args:
        job_id: The job ID string (typically a UUID)

    Returns:
        A positive integer suitable for pg_advisory_lock
    """
    return hash(job_id) & 0x7FFFFFFFFFFFFFFF


def acquire_job_lock(session: Session, job_id: str) -> bool:
    """Attempt to acquire an advisory lock for a job.

    Uses pg_try_advisory_lock which returns immediately (non-blocking).
    The lock is held until explicitly released or the session ends.

    Args:
        session: Database session
        job_id: The job ID to lock

    Returns:
        True if lock acquired, False if another session holds it
    """
    lock_key = job_id_to_lock_key(job_id)
    result = session.exec(text(f"SELECT pg_try_advisory_lock({lock_key})"))
    return result.scalar() is True


def release_job_lock(session: Session, job_id: str) -> None:
    """Release an advisory lock for a job.

    Safe to call even if the lock wasn't acquired (returns False but no error).

    Args:
        session: Database session
        job_id: The job ID to unlock
    """
    lock_key = job_id_to_lock_key(job_id)
    session.exec(text(f"SELECT pg_advisory_unlock({lock_key})"))
```

**Step 2: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_worker_locking.py -v`
Expected: PASS (all tests green)

**Step 3: Commit implementation**

```bash
git add backend/app/worker/locking.py
git commit -m "feat: add advisory lock utilities for worker task deduplication"
```

---

## Task 3: Write Tests for UserGame Unique Constraint

**Files:**
- Create: `backend/app/tests/test_user_game_unique_constraint.py`

**Step 1: Write the failing tests**

```python
"""Tests for UserGame unique constraint on (user_id, game_id)."""

import pytest
from sqlalchemy.exc import IntegrityError
from sqlmodel import Session

from app.models.user_game import UserGame
from app.models.game import Game


class TestUserGameUniqueConstraint:
    """Test unique constraint prevents duplicate user-game combinations."""

    @pytest.fixture
    def second_game(self, session: Session) -> Game:
        """Create a second test game."""
        game = Game(
            id=99999,
            title="Second Test Game",
            igdb_id=99999,
        )
        session.add(game)
        session.commit()
        session.refresh(game)
        return game

    def test_create_user_game_succeeds(self, session: Session, test_user, test_game):
        """Creating a UserGame succeeds normally."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        assert user_game.id is not None
        assert user_game.user_id == test_user.id
        assert user_game.game_id == test_game.id

    def test_duplicate_user_game_raises_integrity_error(
        self, session: Session, test_user, test_game
    ):
        """Creating duplicate (user_id, game_id) raises IntegrityError."""
        # Create first user_game
        user_game1 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game1)
        session.commit()

        # Try to create duplicate
        user_game2 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game2)

        with pytest.raises(IntegrityError):
            session.commit()

        session.rollback()

    def test_same_game_different_users_allowed(
        self, session: Session, test_user, admin_user, test_game
    ):
        """Same game can be owned by different users."""
        user_game1 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        user_game2 = UserGame(
            user_id=admin_user.id,
            game_id=test_game.id,
        )
        session.add(user_game1)
        session.add(user_game2)
        session.commit()

        # Both should exist
        session.refresh(user_game1)
        session.refresh(user_game2)
        assert user_game1.id != user_game2.id

    def test_same_user_different_games_allowed(
        self, session: Session, test_user, test_game, second_game
    ):
        """Same user can own different games."""
        user_game1 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        user_game2 = UserGame(
            user_id=test_user.id,
            game_id=second_game.id,
        )
        session.add(user_game1)
        session.add(user_game2)
        session.commit()

        # Both should exist
        session.refresh(user_game1)
        session.refresh(user_game2)
        assert user_game1.id != user_game2.id
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_user_game_unique_constraint.py::TestUserGameUniqueConstraint::test_duplicate_user_game_raises_integrity_error -v`
Expected: FAIL - the duplicate insert should succeed (no constraint yet)

**Step 3: Commit failing tests**

```bash
git add backend/app/tests/test_user_game_unique_constraint.py
git commit -m "test: add failing tests for UserGame unique constraint"
```

---

## Task 4: Add Unique Constraint to UserGame Model

**Files:**
- Modify: `backend/app/models/user_game.py:70-73`

**Step 1: Update the model**

Change lines 70-73 from:
```python
    # Unique constraint
    __table_args__ = (
        {"extend_existing": True},
    )
```

To:
```python
    # Unique constraint: each user can only have one entry per game
    __table_args__ = (
        UniqueConstraint("user_id", "game_id", name="uq_user_games_user_game"),
        {"extend_existing": True},
    )
```

**Step 2: Create Alembic migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic revision --autogenerate -m "add unique constraint to user_games user_id game_id"`

**Step 3: Run the migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic upgrade head`

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_user_game_unique_constraint.py -v`
Expected: PASS (all tests green)

**Step 5: Commit model and migration**

```bash
git add backend/app/models/user_game.py backend/app/alembic/versions/*.py
git commit -m "feat: add unique constraint on UserGame(user_id, game_id)"
```

---

## Task 5: Write Tests for Import Task Lock Integration

**Files:**
- Modify: `backend/app/tests/test_import_tasks.py` (add new test class)

**Step 1: Add tests for lock behavior**

Add to the end of `test_import_tasks.py`:

```python
class TestNexoriousImportLocking:
    """Test advisory lock behavior in Nexorious import task."""

    @pytest.mark.asyncio
    async def test_import_skips_when_lock_held(self, session, test_user):
        """Import returns skipped status when another worker holds the lock."""
        from app.worker.locking import acquire_job_lock, release_job_lock
        from app.core.database import get_engine

        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )
        job.set_result_summary({
            "_import_data": {
                "export_version": "1.0",
                "games": [{"title": "Test", "igdb_id": 123}],
            }
        })
        session.add(job)
        session.commit()
        session.refresh(job)

        # Simulate another worker holding the lock
        with Session(get_engine()) as other_session:
            acquired = acquire_job_lock(other_session, job.id)
            assert acquired is True

            # Now run the import task - should skip
            result = await import_nexorious_json(job.id)

            assert result["status"] == "skipped"
            assert result["reason"] == "duplicate_execution"

            # Release the lock
            release_job_lock(other_session, job.id)


class TestNexoriousImportIntegrityError:
    """Test IntegrityError handling in Nexorious import task."""

    @pytest.mark.asyncio
    async def test_import_handles_integrity_error_gracefully(
        self, session, test_user, test_game
    ):
        """Import counts as already_in_collection when IntegrityError occurs."""
        # Pre-create the UserGame to trigger IntegrityError on import
        existing = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(existing)
        session.commit()

        # Create import job for the same game
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )
        job.set_result_summary({
            "_import_data": {
                "export_version": "1.0",
                "games": [
                    {
                        "title": test_game.title,
                        "igdb_id": test_game.id,
                        "play_status": "completed",
                    }
                ],
            }
        })
        session.add(job)
        session.commit()
        session.refresh(job)

        # Mock services
        with patch(
            "app.worker.tasks.import_export.import_nexorious.IGDBService"
        ), patch(
            "app.worker.tasks.import_export.import_nexorious.GameService"
        ):
            result = await import_nexorious_json(job.id)

        # Should complete with already_in_collection count
        assert result["status"] == "success"
        assert result["already_in_collection"] == 1
        assert result["imported"] == 0
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_import_tasks.py::TestNexoriousImportLocking -v`
Expected: FAIL - import doesn't have lock logic yet

**Step 3: Commit failing tests**

```bash
git add backend/app/tests/test_import_tasks.py
git commit -m "test: add failing tests for import task locking and IntegrityError handling"
```

---

## Task 6: Add Locking to Nexorious Import Task

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_nexorious.py`

**Step 1: Add imports at top of file (after line 13)**

After `from sqlmodel import Session, select` add:

```python
from sqlalchemy.exc import IntegrityError

from app.worker.locking import acquire_job_lock, release_job_lock
```

**Step 2: Add lock acquisition at start of task (after line 65)**

Replace the task function body structure. After `async with get_session_context() as session:` (line 65), add lock acquisition before the job lookup:

```python
    async with get_session_context() as session:
        # Try to acquire advisory lock - prevents duplicate execution
        if not acquire_job_lock(session, job_id):
            logger.info(f"Job {job_id} already being processed by another worker")
            return {"status": "skipped", "reason": "duplicate_execution"}

        try:
            # Get job and update status
            job = session.get(Job, job_id)
            # ... rest of existing code ...
        finally:
            release_job_lock(session, job_id)
```

**Step 3: Handle IntegrityError in _process_nexorious_game**

In the `_process_nexorious_game` function, wrap the UserGame creation (lines 218-232) with IntegrityError handling:

Change:
```python
    # Create UserGame with user data from export
    user_game = UserGame(
        user_id=user_id,
        game_id=igdb_id,
        play_status=_map_play_status(game_data.get("play_status")),
        ownership_status=_map_ownership_status(game_data.get("ownership_status")),
        personal_rating=_parse_rating(game_data.get("personal_rating")),
        is_loved=game_data.get("is_loved", False),
        hours_played=game_data.get("hours_played", 0),
        personal_notes=game_data.get("personal_notes"),
        acquired_date=_parse_date(game_data.get("acquired_date")),
    )
    session.add(user_game)
    session.commit()
    session.refresh(user_game)
```

To:
```python
    # Create UserGame with user data from export
    user_game = UserGame(
        user_id=user_id,
        game_id=igdb_id,
        play_status=_map_play_status(game_data.get("play_status")),
        ownership_status=_map_ownership_status(game_data.get("ownership_status")),
        personal_rating=_parse_rating(game_data.get("personal_rating")),
        is_loved=game_data.get("is_loved", False),
        hours_played=game_data.get("hours_played", 0),
        personal_notes=game_data.get("personal_notes"),
        acquired_date=_parse_date(game_data.get("acquired_date")),
    )
    session.add(user_game)
    try:
        session.commit()
    except IntegrityError:
        session.rollback()
        logger.info(f"Game '{title}' already in collection (caught by constraint)")
        return "already_in_collection"
    session.refresh(user_game)
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_import_tasks.py -v`
Expected: PASS (all tests green)

**Step 5: Commit implementation**

```bash
git add backend/app/worker/tasks/import_export/import_nexorious.py
git commit -m "feat: add advisory lock and IntegrityError handling to Nexorious import"
```

---

## Task 7: Add Locking to Darkadia Import Task

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_darkadia.py`

**Step 1: Add imports**

Add after existing imports:
```python
from app.worker.locking import acquire_job_lock, release_job_lock
```

**Step 2: Add lock acquisition**

At the start of `import_darkadia_csv` function, after `async with get_session_context() as session:`, add:

```python
        # Try to acquire advisory lock - prevents duplicate execution
        if not acquire_job_lock(session, job_id):
            logger.info(f"Job {job_id} already being processed by another worker")
            return {"status": "skipped", "reason": "duplicate_execution"}

        try:
            # ... existing code ...
        finally:
            release_job_lock(session, job_id)
```

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_import_tasks.py -v`
Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/worker/tasks/import_export/import_darkadia.py
git commit -m "feat: add advisory lock to Darkadia import task"
```

---

## Task 8: Add Locking to Export Task

**Files:**
- Modify: `backend/app/worker/tasks/import_export/export.py`

**Step 1: Add imports**

Add after existing imports:
```python
from app.worker.locking import acquire_job_lock, release_job_lock
```

**Step 2: Add lock acquisition**

At the start of `export_collection` function, after `async with get_session_context() as session:`, add the same lock pattern.

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_export_tasks.py -v`
Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/worker/tasks/import_export/export.py
git commit -m "feat: add advisory lock to export task"
```

---

## Task 9: Add Locking to Steam Sync Task

**Files:**
- Modify: `backend/app/worker/tasks/sync/steam.py`

**Step 1: Add imports**

Add after existing imports:
```python
from app.worker.locking import acquire_job_lock, release_job_lock
```

**Step 2: Add lock acquisition**

At the start of `sync_steam_library` function, after `async with get_session_context() as session:`, add the same lock pattern.

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/ -v -k "steam or sync"`
Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/worker/tasks/sync/steam.py
git commit -m "feat: add advisory lock to Steam sync task"
```

---

## Task 10: Run Full Test Suite and Type Check

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass, coverage >80%

**Step 2: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No type errors

**Step 3: Run linting**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run ruff check .`
Expected: No linting errors

---

## Task 11: Create Pull Request

**Step 1: Push branch**

```bash
git push -u origin fix/import-duplicate-bug
```

**Step 2: Create PR**

```bash
gh pr create --title "fix: prevent duplicate game creation during import" --body "$(cat <<'EOF'
## Summary
- Add advisory locks to prevent duplicate task execution when multiple workers receive the same job
- Add unique constraint on UserGame(user_id, game_id) as database-level safety net
- Handle IntegrityError gracefully in import tasks

## Root Cause
The taskiq-pg broker uses PostgreSQL LISTEN/NOTIFY which broadcasts to all workers. Without locking, multiple workers could process the same import job simultaneously, creating duplicate games.

## Changes
- New `backend/app/worker/locking.py` with advisory lock utilities
- New migration adding unique constraint to user_games table
- Updated import/export/sync tasks to acquire locks before processing
- IntegrityError handling in Nexorious import for race condition safety

## Test plan
- [x] Unit tests for lock utilities
- [x] Integration tests for unique constraint
- [x] Tests for lock behavior in import tasks
- [x] All existing tests pass
- [x] Type checking passes
- [x] Linting passes

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Summary of Files Changed

### New Files
- `backend/app/worker/locking.py` - Advisory lock utilities
- `backend/app/tests/test_worker_locking.py` - Lock utility tests
- `backend/app/tests/test_user_game_unique_constraint.py` - Constraint tests
- `backend/app/alembic/versions/*_add_unique_constraint_*.py` - Migration

### Modified Files
- `backend/app/models/user_game.py` - Add UniqueConstraint
- `backend/app/worker/tasks/import_export/import_nexorious.py` - Lock + IntegrityError
- `backend/app/worker/tasks/import_export/import_darkadia.py` - Lock
- `backend/app/worker/tasks/import_export/export.py` - Lock
- `backend/app/worker/tasks/sync/steam.py` - Lock
- `backend/app/tests/test_import_tasks.py` - Additional tests
