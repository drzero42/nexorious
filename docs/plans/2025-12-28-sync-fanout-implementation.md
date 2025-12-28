# Sync Fan-out Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the sync system to use per-game tasks for parallel processing with a generic adapter pattern.

**Architecture:** Two-phase fan-out (dispatch creates JobItems, workers process them in parallel). Generic adapter abstraction allows Steam/Epic/GOG to share processing logic. Queue simplification to two priority levels (high/low).

**Tech Stack:** Python 3.13, TaskIQ, NATS JetStream, SQLModel, FastAPI

**Reference:** See `docs/plans/2025-12-28-sync-fanout-design.md` for full design details.

---

## Task 1: Simplify Queue Subjects

**Files:**
- Modify: `backend/app/worker/queues.py`

**Step 1: Update queues.py with simplified subjects**

```python
"""Queue configuration for NATS subject-based routing."""

# Priority subjects for NATS JetStream
# High priority: user-initiated tasks (manual sync, imports, exports)
# Low priority: automated tasks (scheduled syncs, maintenance)

SUBJECT_HIGH = "tasks.high"
SUBJECT_LOW = "tasks.low"

# Legacy compatibility (will be removed after full migration)
QUEUE_HIGH = "high"
QUEUE_LOW = "low"

# Keep old subjects as aliases during migration
SUBJECT_HIGH_IMPORT = SUBJECT_HIGH
SUBJECT_HIGH_SYNC = SUBJECT_HIGH
SUBJECT_HIGH_EXPORT = SUBJECT_HIGH
SUBJECT_LOW_IMPORT = SUBJECT_LOW
SUBJECT_LOW_SYNC = SUBJECT_LOW
SUBJECT_LOW_MAINTENANCE = SUBJECT_LOW
```

**Step 2: Run tests to verify nothing breaks**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pytest app/tests/ -v -k "queue or broker" --tb=short`
Expected: All tests pass (aliases maintain backward compatibility)

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/queues.py
git commit -m "refactor(worker): simplify queue subjects to high/low priority"
```

---

## Task 2: Create Enqueue Helper Function

**Files:**
- Modify: `backend/app/worker/queues.py`

**Step 1: Add enqueue helper to queues.py**

Append to the end of `queues.py`:

```python
from app.models.job import BackgroundJobPriority


async def enqueue_task(task_func, *args, priority: BackgroundJobPriority):
    """Dispatch task to appropriate priority queue.

    Args:
        task_func: The TaskIQ task function to dispatch
        *args: Arguments to pass to the task
        priority: HIGH for user-initiated, LOW for automated tasks
    """
    subject = SUBJECT_HIGH if priority == BackgroundJobPriority.HIGH else SUBJECT_LOW
    await task_func.kicker().with_labels(subject=subject).kiq(*args)
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/queues.py
git commit -m "feat(worker): add generic enqueue_task helper for priority routing"
```

---

## Task 3: Create Source Adapter Base Module

**Files:**
- Create: `backend/app/worker/tasks/sync/adapters/__init__.py`
- Create: `backend/app/worker/tasks/sync/adapters/base.py`

**Step 1: Create adapters directory and __init__.py**

Create `backend/app/worker/tasks/sync/adapters/__init__.py`:

```python
"""Sync source adapters for external game libraries."""

from .base import ExternalGame, SyncSourceAdapter, get_sync_adapter

__all__ = ["ExternalGame", "SyncSourceAdapter", "get_sync_adapter"]
```

**Step 2: Create base.py with ExternalGame and SyncSourceAdapter**

Create `backend/app/worker/tasks/sync/adapters/base.py`:

```python
"""Base classes and protocols for sync source adapters.

This module provides the abstraction layer for fetching games from
external services (Steam, Epic, GOG, etc.) in a uniform format.
"""

from dataclasses import dataclass
from typing import Protocol, Optional, List, Dict, Any

from app.models.user import User
from app.models.job import BackgroundJobSource


@dataclass
class ExternalGame:
    """Standardized representation of a game from an external source.

    Attributes:
        external_id: Unique identifier from the source (e.g., Steam AppID)
        title: Game name from the source
        platform_id: Platform identifier (e.g., "pc-windows")
        storefront_id: Storefront identifier (e.g., "steam")
        metadata: Source-specific data (playtime, achievements, etc.)
    """
    external_id: str
    title: str
    platform_id: str
    storefront_id: str
    metadata: Dict[str, Any]


class SyncSourceAdapter(Protocol):
    """Protocol for sync source adapters.

    Each external service (Steam, Epic, GOG) implements this protocol
    to provide a uniform interface for fetching game libraries.
    """

    source: BackgroundJobSource

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch all games from external source for user.

        Args:
            user: The user whose games to fetch

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If credentials are not configured
            Exception: On API errors
        """
        ...

    def get_credentials(self, user: User) -> Optional[Dict[str, str]]:
        """Extract credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with credential keys, or None if not configured
        """
        ...

    def is_configured(self, user: User) -> bool:
        """Check if user has valid credentials for this source.

        Args:
            user: The user to check

        Returns:
            True if credentials are configured and verified
        """
        ...


def get_sync_adapter(source: str) -> SyncSourceAdapter:
    """Get the appropriate adapter for a sync source.

    Args:
        source: Source identifier ("steam", "epic", "gog")

    Returns:
        Adapter instance for the specified source

    Raises:
        ValueError: If source is not supported
    """
    from .steam import SteamSyncAdapter

    adapters = {
        "steam": SteamSyncAdapter,
    }

    adapter_class = adapters.get(source.lower())
    if not adapter_class:
        raise ValueError(f"Unsupported sync source: {source}")

    return adapter_class()
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors (steam adapter import will fail until next task)

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/sync/adapters/
git commit -m "feat(sync): add ExternalGame dataclass and SyncSourceAdapter protocol"
```

---

## Task 4: Create Steam Sync Adapter

**Files:**
- Create: `backend/app/worker/tasks/sync/adapters/steam.py`

**Step 1: Create steam.py adapter**

Create `backend/app/worker/tasks/sync/adapters/steam.py`:

```python
"""Steam sync adapter for fetching user's Steam library.

Implements SyncSourceAdapter protocol to fetch games from Steam
and convert them to the standardized ExternalGame format.
"""

import logging
from typing import Optional, List, Dict

from app.models.user import User
from app.models.job import BackgroundJobSource
from app.services.steam import SteamService
from .base import ExternalGame

logger = logging.getLogger(__name__)


class SteamSyncAdapter:
    """Adapter for syncing games from Steam.

    Fetches the user's Steam library and converts games to
    ExternalGame format for generic processing.
    """

    source = BackgroundJobSource.STEAM

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch all games from user's Steam library.

        Args:
            user: The user whose Steam library to fetch

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If Steam credentials are not configured
        """
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("Steam credentials not configured for this user")

        steam_service = SteamService(api_key=credentials["api_key"])
        steam_games = await steam_service.get_owned_games(credentials["steam_id"])

        logger.info(f"Fetched {len(steam_games)} games from Steam for user {user.id}")

        return [
            ExternalGame(
                external_id=str(game.appid),
                title=game.name,
                platform_id="pc-windows",
                storefront_id="steam",
                metadata={
                    "playtime_minutes": game.playtime_forever,
                    "playtime_2weeks": getattr(game, "playtime_2weeks", 0),
                    "img_icon_url": getattr(game, "img_icon_url", None),
                },
            )
            for game in steam_games
        ]

    def get_credentials(self, user: User) -> Optional[Dict[str, str]]:
        """Extract Steam credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with api_key and steam_id, or None if not configured
        """
        preferences = user.preferences or {}
        steam_config = preferences.get("steam", {})

        api_key = steam_config.get("web_api_key")
        steam_id = steam_config.get("steam_id")
        is_verified = steam_config.get("is_verified", False)

        if not api_key or not steam_id or not is_verified:
            return None

        return {"api_key": api_key, "steam_id": steam_id}

    def is_configured(self, user: User) -> bool:
        """Check if user has verified Steam credentials.

        Args:
            user: The user to check

        Returns:
            True if Steam credentials are configured and verified
        """
        return self.get_credentials(user) is not None
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/sync/adapters/steam.py
git commit -m "feat(sync): add Steam sync adapter"
```

---

## Task 5: Create Sync Dispatch Task

**Files:**
- Create: `backend/app/worker/tasks/sync/dispatch.py`

**Step 1: Create dispatch.py with fan-out task**

Create `backend/app/worker/tasks/sync/dispatch.py`:

```python
"""Sync dispatch task for fan-out processing.

Creates JobItems for each game from an external source and dispatches
individual processing tasks for parallel execution.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlmodel import Session

from app.worker.broker import broker
from app.worker.queues import SUBJECT_HIGH, SUBJECT_LOW, enqueue_task
from app.core.database import get_session_context
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobPriority,
)
from app.models.user import User
from app.worker.tasks.sync.adapters import get_sync_adapter

logger = logging.getLogger(__name__)


@broker.task(task_name="sync.dispatch")
async def dispatch_sync_items(
    job_id: str,
    user_id: str,
    source: str,
) -> Dict[str, Any]:
    """
    Fan-out task that creates JobItems and dispatches worker tasks.

    This task:
    1. Fetches the user's game library via the source adapter
    2. Creates a JobItem for each game (streaming insert)
    3. Dispatches a process_sync_item task for each JobItem
    4. Returns quickly - actual processing happens in parallel workers

    Args:
        job_id: The Job ID for tracking progress
        user_id: The user to sync
        source: Source identifier ("steam", "epic", "gog")

    Returns:
        Dictionary with dispatch statistics.
    """
    logger.info(f"Starting sync dispatch for user {user_id}, source {source} (job: {job_id})")

    stats = {
        "total_games": 0,
        "dispatched": 0,
        "errors": 0,
    }

    async with get_session_context() as session:
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status to PROCESSING
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Get user
            user = session.get(User, user_id)
            if not user:
                raise ValueError(f"User {user_id} not found")

            # Get adapter and fetch games
            adapter = get_sync_adapter(source)
            games = await adapter.fetch_games(user)
            stats["total_games"] = len(games)

            # Update job total_items
            job.total_items = len(games)
            session.add(job)
            session.commit()

            logger.info(f"Fetched {len(games)} games from {source} for user {user_id}")

            # Determine priority for item tasks
            priority = job.priority

            # Stream create JobItems and dispatch tasks
            for game in games:
                try:
                    job_item = _create_job_item(
                        session=session,
                        job=job,
                        user_id=user_id,
                        game=game,
                    )

                    # Dispatch worker task
                    await _dispatch_process_task(job_item.id, priority)
                    stats["dispatched"] += 1

                except Exception as e:
                    logger.error(f"Error creating/dispatching item for {game.title}: {e}")
                    stats["errors"] += 1

            logger.info(
                f"Sync dispatch completed for job {job_id}: "
                f"{stats['dispatched']} dispatched, {stats['errors']} errors"
            )

            # Note: Job stays in PROCESSING state
            # It will be marked COMPLETED by the last worker task

            return {"status": "dispatched", **stats}

        except Exception as e:
            logger.error(f"Sync dispatch failed for job {job_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}


def _create_job_item(
    session: Session,
    job: Job,
    user_id: str,
    game,  # ExternalGame
) -> JobItem:
    """Create a JobItem for a game.

    Args:
        session: Database session
        job: The parent Job
        user_id: User ID
        game: ExternalGame from adapter

    Returns:
        Created JobItem
    """
    source_metadata = {
        "external_id": game.external_id,
        "platform_id": game.platform_id,
        "storefront_id": game.storefront_id,
        "metadata": game.metadata,
    }

    job_item = JobItem(
        job_id=job.id,
        user_id=user_id,
        item_key=f"{game.storefront_id}_{game.external_id}",
        source_title=game.title,
        source_metadata_json=json.dumps(source_metadata),
        status=JobItemStatus.PENDING,
    )

    session.add(job_item)
    session.commit()
    session.refresh(job_item)

    logger.debug(f"Created JobItem {job_item.id} for {game.title}")

    return job_item


async def _dispatch_process_task(job_item_id: str, priority: BackgroundJobPriority):
    """Dispatch a process_sync_item task for a JobItem.

    Args:
        job_item_id: The JobItem ID to process
        priority: Task priority (HIGH or LOW)
    """
    from app.worker.tasks.sync.process_item import process_sync_item

    await enqueue_task(process_sync_item, job_item_id, priority=priority)
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors (process_item import will fail until next task, that's OK)

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/sync/dispatch.py
git commit -m "feat(sync): add fan-out dispatch task for sync items"
```

---

## Task 6: Create Sync Process Item Task

**Files:**
- Create: `backend/app/worker/tasks/sync/process_item.py`

**Step 1: Create process_item.py with worker task**

Create `backend/app/worker/tasks/sync/process_item.py`:

```python
"""Sync item processor for individual game processing.

Processes individual JobItems created by the dispatch task.
Handles matching, linking, and review workflow.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Dict, Any, Optional

from sqlalchemy import update as sa_update
from sqlmodel import Session, select, func, col

from app.worker.broker import broker
from app.worker.queues import enqueue_task
from app.core.database import get_sync_session
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobPriority,
)
from app.models.user_game import UserGame, UserGamePlatform
from app.models.user_sync_config import UserSyncConfig
from app.models.ignored_external_game import IgnoredExternalGame
from app.services.igdb.service import IGDBService
from app.services.matching.service import MatchingService
from app.services.matching.models import MatchRequest, MatchStatus
from app.services.game_service import GameService

logger = logging.getLogger(__name__)

# Auto-match confidence threshold (85%)
AUTO_MATCH_CONFIDENCE_THRESHOLD = 0.85


@broker.task(task_name="sync.process_item")
async def process_sync_item(job_item_id: str) -> Dict[str, Any]:
    """
    Process a single sync item.

    Implements the processing flows:
    - Flow A (new item): Check synced → check ignored → IGDB match → link/review
    - Flow B (reviewed item): Check synced → use resolved_igdb_id → link

    Args:
        job_item_id: The JobItem ID to process

    Returns:
        Dictionary with processing result details
    """
    # Phase 1: Fetch job item and validate
    session = get_sync_session()
    try:
        job_item = session.get(JobItem, job_item_id)
        if not job_item:
            logger.error(f"JobItem {job_item_id} not found")
            return {"status": "error", "error": "JobItem not found"}

        # Idempotency check
        if job_item.status not in (JobItemStatus.PENDING, JobItemStatus.PROCESSING):
            logger.info(f"JobItem {job_item_id} already processed: {job_item.status}")
            return {"status": "skipped", "reason": "already_processed"}

        # Set status to PROCESSING
        job_item.status = JobItemStatus.PROCESSING
        session.add(job_item)
        session.commit()

        # Extract data
        user_id = job_item.user_id
        job_id = job_item.job_id
        resolved_igdb_id = job_item.resolved_igdb_id
        source_metadata_json = job_item.source_metadata_json
        source_title = job_item.source_title
    finally:
        session.close()

    # Phase 2: Parse metadata
    try:
        metadata = json.loads(source_metadata_json)
    except json.JSONDecodeError as e:
        logger.error(f"Invalid JSON in JobItem {job_item_id}: {e}")
        return await _update_job_item_error(job_item_id, f"Invalid metadata: {e}")

    external_id = metadata.get("external_id")
    platform_id = metadata.get("platform_id")
    storefront_id = metadata.get("storefront_id")

    if not external_id or not storefront_id:
        return await _update_job_item_error(job_item_id, "Missing external_id or storefront_id")

    # Phase 3: Process with fresh session
    session = get_sync_session()
    try:
        # Step 1: Check if already synced
        if _is_already_synced(session, user_id, storefront_id, external_id):
            return await _complete_job_item(
                session, job_item_id, job_id,
                JobItemStatus.COMPLETED, "already_synced"
            )

        # Step 2: Check if ignored
        if _is_ignored(session, user_id, storefront_id, external_id):
            return await _complete_job_item(
                session, job_item_id, job_id,
                JobItemStatus.SKIPPED, "ignored"
            )

        # Step 3: Check if user provided resolved_igdb_id (Flow B)
        if resolved_igdb_id:
            return await _process_with_resolved_id(
                session, job_item_id, job_id, user_id,
                resolved_igdb_id, platform_id, storefront_id, external_id, source_title
            )

        # Step 4: Flow A - Match via IGDB
        return await _process_with_matching(
            session, job_item_id, job_id, user_id,
            source_title, platform_id, storefront_id, external_id, metadata
        )

    except Exception as e:
        logger.error(f"Error processing JobItem {job_item_id}: {e}", exc_info=True)
        session.close()
        return await _update_job_item_error(job_item_id, str(e)[:500])
    finally:
        session.close()


def _is_already_synced(
    session: Session,
    user_id: str,
    storefront_id: str,
    external_id: str
) -> bool:
    """Check if game is already synced (exists in UserGamePlatform)."""
    result = session.exec(
        select(UserGamePlatform)
        .join(UserGame)
        .where(
            UserGame.user_id == user_id,
            UserGamePlatform.storefront_id == storefront_id,
            UserGamePlatform.store_game_id == external_id,
        )
    ).first()
    return result is not None


def _is_ignored(
    session: Session,
    user_id: str,
    storefront_id: str,
    external_id: str
) -> bool:
    """Check if game is in the ignored list."""
    # Map storefront_id to BackgroundJobSource
    source_map = {
        "steam": BackgroundJobSource.STEAM,
        "epic": BackgroundJobSource.EPIC,
        "gog": BackgroundJobSource.GOG,
    }
    source = source_map.get(storefront_id)
    if not source:
        return False

    result = session.exec(
        select(IgnoredExternalGame).where(
            IgnoredExternalGame.user_id == user_id,
            IgnoredExternalGame.source == source,
            IgnoredExternalGame.external_id == external_id,
        )
    ).first()
    return result is not None


async def _process_with_resolved_id(
    session: Session,
    job_item_id: str,
    job_id: str,
    user_id: str,
    igdb_id: int,
    platform_id: str,
    storefront_id: str,
    external_id: str,
    source_title: str,
) -> Dict[str, Any]:
    """Process item with user-provided IGDB ID (Flow B)."""
    logger.info(f"Processing {source_title} with resolved IGDB ID {igdb_id}")

    # Check if game exists in user's collection
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if existing_user_game:
        # Add platform association to existing game
        _add_platform_association(
            session, existing_user_game.id,
            platform_id, storefront_id, external_id
        )
        return await _complete_job_item(
            session, job_item_id, job_id,
            JobItemStatus.COMPLETED, "linked_existing"
        )
    else:
        # Create new UserGame and add platform association
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)

        user_game = await game_service.add_game_to_collection(user_id, igdb_id)
        _add_platform_association(
            session, user_game.id,
            platform_id, storefront_id, external_id
        )
        return await _complete_job_item(
            session, job_item_id, job_id,
            JobItemStatus.COMPLETED, "imported_new"
        )


async def _process_with_matching(
    session: Session,
    job_item_id: str,
    job_id: str,
    user_id: str,
    source_title: str,
    platform_id: str,
    storefront_id: str,
    external_id: str,
    metadata: Dict[str, Any],
) -> Dict[str, Any]:
    """Process item with IGDB matching (Flow A)."""
    logger.debug(f"Matching {source_title} via IGDB")

    igdb_service = IGDBService()
    matching_service = MatchingService(session, igdb_service)

    match_request = MatchRequest(
        source_title=source_title,
        source_platform=storefront_id,
        platform_id=external_id,
        source_metadata=metadata.get("metadata", {}),
    )

    match_result = await matching_service.match_game(match_request)

    if match_result.status == MatchStatus.MATCHED:
        confidence = match_result.confidence_score or 0.0
        igdb_id = match_result.igdb_id

        if not igdb_id:
            logger.warning(f"Match result MATCHED but no IGDB ID for {source_title}")
            return await _set_pending_review(
                session, job_item_id, job_id,
                candidates=[], confidence=0.0
            )

        if confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
            # High confidence - auto-import
            return await _auto_import_game(
                session, job_item_id, job_id, user_id,
                igdb_id, platform_id, storefront_id, external_id,
                match_result, confidence
            )
        else:
            # Low confidence - needs review
            return await _set_pending_review(
                session, job_item_id, job_id,
                candidates=match_result.candidates or [],
                confidence=confidence,
                igdb_id=igdb_id,
                igdb_title=match_result.igdb_title,
            )

    elif match_result.status == MatchStatus.NEEDS_REVIEW:
        # Multiple candidates
        return await _set_pending_review(
            session, job_item_id, job_id,
            candidates=match_result.candidates or [],
            confidence=0.0,
        )

    else:
        # No match found
        return await _set_pending_review(
            session, job_item_id, job_id,
            candidates=[],
            confidence=0.0,
        )


async def _auto_import_game(
    session: Session,
    job_item_id: str,
    job_id: str,
    user_id: str,
    igdb_id: int,
    platform_id: str,
    storefront_id: str,
    external_id: str,
    match_result,
    confidence: float,
) -> Dict[str, Any]:
    """Auto-import a high-confidence match."""
    logger.info(f"Auto-importing {match_result.igdb_title} (confidence: {confidence:.2f})")

    # Check if game exists in user's collection
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if existing_user_game:
        # Add platform association to existing game
        _add_platform_association(
            session, existing_user_game.id,
            platform_id, storefront_id, external_id
        )
        result = "auto_linked"
    else:
        # Create new UserGame
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)

        user_game = await game_service.add_game_to_collection(user_id, igdb_id)
        _add_platform_association(
            session, user_game.id,
            platform_id, storefront_id, external_id
        )
        result = "auto_imported"

    # Update JobItem with match info
    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.resolved_igdb_id = igdb_id
        job_item.match_confidence = confidence
        session.add(job_item)

    return await _complete_job_item(
        session, job_item_id, job_id,
        JobItemStatus.COMPLETED, result
    )


def _add_platform_association(
    session: Session,
    user_game_id: str,
    platform_id: str,
    storefront_id: str,
    external_id: str,
) -> None:
    """Add platform association to a UserGame."""
    # Check if association already exists
    existing = session.exec(
        select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game_id,
            UserGamePlatform.storefront_id == storefront_id,
            UserGamePlatform.store_game_id == external_id,
        )
    ).first()

    if not existing:
        # Build store URL based on storefront
        store_url = None
        if storefront_id == "steam":
            store_url = f"https://store.steampowered.com/app/{external_id}"

        platform = UserGamePlatform(
            user_game_id=user_game_id,
            platform_id=platform_id,
            storefront_id=storefront_id,
            store_game_id=external_id,
            store_url=store_url,
            is_available=True,
        )
        session.add(platform)
        session.commit()
        logger.debug(f"Added platform association for UserGame {user_game_id}")


async def _set_pending_review(
    session: Session,
    job_item_id: str,
    job_id: str,
    candidates: list,
    confidence: float,
    igdb_id: Optional[int] = None,
    igdb_title: Optional[str] = None,
) -> Dict[str, Any]:
    """Set JobItem to PENDING_REVIEW status."""
    # Serialize candidates
    serializable_candidates = []
    for candidate in candidates:
        if hasattr(candidate, "to_dict"):
            serializable_candidates.append(candidate.to_dict())
        elif isinstance(candidate, dict):
            serializable_candidates.append(candidate)
        else:
            try:
                serializable_candidates.append(candidate.__dict__)
            except AttributeError:
                pass

    # Add matched game to candidates if not present
    if igdb_id and igdb_title:
        candidate_ids = {c.get("igdb_id") for c in serializable_candidates}
        if igdb_id not in candidate_ids:
            serializable_candidates.insert(0, {
                "igdb_id": igdb_id,
                "name": igdb_title,
                "similarity_score": confidence,
            })

    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.status = JobItemStatus.PENDING_REVIEW
        job_item.igdb_candidates_json = json.dumps(serializable_candidates)
        job_item.match_confidence = confidence if confidence > 0 else None
        job_item.processed_at = datetime.now(timezone.utc)
        session.add(job_item)
        session.commit()

    # Check job completion (PENDING_REVIEW counts as "processed" for job status)
    _check_and_update_job_completion(session, job_id)

    return {"status": "success", "result": "pending_review", "candidates": len(serializable_candidates)}


async def _complete_job_item(
    session: Session,
    job_item_id: str,
    job_id: str,
    status: JobItemStatus,
    result: str,
) -> Dict[str, Any]:
    """Mark JobItem as complete and check job completion."""
    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.status = status
        job_item.processed_at = datetime.now(timezone.utc)
        session.add(job_item)
        session.commit()

    _check_and_update_job_completion(session, job_id)

    return {"status": "success", "result": result, "job_item_status": status.value}


async def _update_job_item_error(job_item_id: str, error_message: str) -> Dict[str, Any]:
    """Update JobItem with error status."""
    session = get_sync_session()
    try:
        job_item = session.get(JobItem, job_item_id)
        if job_item:
            job_id = job_item.job_id
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = error_message
            job_item.processed_at = datetime.now(timezone.utc)
            session.add(job_item)
            session.commit()

            _check_and_update_job_completion(session, job_id)
    finally:
        session.close()

    return {"status": "error", "error": error_message}


def _check_and_update_job_completion(session: Session, job_id: str) -> bool:
    """Check if all job items are processed and update job status.

    Also updates last_synced_at for SYNC jobs when complete.

    Returns:
        True if job was marked complete, False otherwise
    """
    # Count items still pending or processing
    pending_count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .where(
            JobItem.job_id == job_id,
            col(JobItem.status).in_([JobItemStatus.PENDING, JobItemStatus.PROCESSING])
        )
    ).one()

    if pending_count > 0:
        return False

    # All items processed - update job
    job = session.get(Job, job_id)
    if not job:
        logger.error(f"Job {job_id} not found when checking completion")
        return False

    # Only update if not already terminal
    if job.status in (BackgroundJobStatus.COMPLETED, BackgroundJobStatus.FAILED, BackgroundJobStatus.CANCELLED):
        return False

    # Mark job complete
    job.status = BackgroundJobStatus.COMPLETED
    job.completed_at = datetime.now(timezone.utc)
    session.add(job)

    # Update last_synced_at for SYNC jobs
    if job.job_type == BackgroundJobType.SYNC:
        _update_sync_config_timestamp(session, job.user_id, job.source)

    session.commit()
    logger.info(f"Job {job_id} marked as COMPLETED")

    return True


def _update_sync_config_timestamp(session: Session, user_id: str, source: BackgroundJobSource):
    """Update last_synced_at for the user's sync config."""
    platform_map = {
        BackgroundJobSource.STEAM: "steam",
        BackgroundJobSource.EPIC: "epic",
        BackgroundJobSource.GOG: "gog",
    }
    platform = platform_map.get(source)
    if not platform:
        return

    sync_config = session.exec(
        select(UserSyncConfig).where(
            UserSyncConfig.user_id == user_id,
            UserSyncConfig.platform == platform,
        )
    ).first()

    if sync_config:
        sync_config.last_synced_at = datetime.now(timezone.utc)
        session.add(sync_config)
        logger.info(f"Updated last_synced_at for user {user_id}, platform {platform}")
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/sync/process_item.py
git commit -m "feat(sync): add per-item sync processor task"
```

---

## Task 7: Update Sync __init__.py

**Files:**
- Modify: `backend/app/worker/tasks/sync/__init__.py`

**Step 1: Update __init__.py to export new tasks**

Replace contents of `backend/app/worker/tasks/sync/__init__.py`:

```python
"""Sync tasks for external game library synchronization."""

from .dispatch import dispatch_sync_items
from .process_item import process_sync_item
from .check_pending import check_pending_syncs
from .adapters import ExternalGame, SyncSourceAdapter, get_sync_adapter

__all__ = [
    "dispatch_sync_items",
    "process_sync_item",
    "check_pending_syncs",
    "ExternalGame",
    "SyncSourceAdapter",
    "get_sync_adapter",
]
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/sync/__init__.py
git commit -m "refactor(sync): update exports for new task structure"
```

---

## Task 8: Update check_pending_syncs to Use New Dispatch

**Files:**
- Modify: `backend/app/worker/tasks/sync/check_pending.py`

**Step 1: Update check_pending.py to dispatch new fan-out task**

Replace the `_dispatch_sync_for_config` function and update imports:

```python
"""Task to check for pending syncs and dispatch sync tasks.

Runs every 15 minutes to check which user sync configurations
need syncing based on their frequency settings.
"""

import logging
from typing import Dict, Any

from sqlmodel import select, col

from app.worker.broker import broker
from app.worker.queues import SUBJECT_LOW, enqueue_task
from app.core.database import get_session_context
from app.models.user_sync_config import UserSyncConfig, SyncFrequency
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
)

logger = logging.getLogger(__name__)


@broker.task(
    task_name="sync.check_pending_syncs",
    schedule=[{"cron": "*/15 * * * *"}],  # Every 15 minutes
)
async def check_pending_syncs() -> Dict[str, Any]:
    """
    Check for sync configurations that need syncing and dispatch tasks.

    This is the scheduler fan-out task that:
    1. Queries all enabled, non-manual sync configs
    2. Checks if enough time has passed based on frequency
    3. Creates Job records and dispatches sync tasks

    Returns:
        Dictionary with dispatch statistics.
    """
    logger.info("Checking for pending syncs")

    syncs_dispatched = 0
    platforms: Dict[str, int] = {}

    async with get_session_context() as session:
        # Get all sync configs that might need syncing
        stmt = select(UserSyncConfig).where(
            UserSyncConfig.enabled,  # noqa: E712 - SQLAlchemy boolean column
            UserSyncConfig.frequency != SyncFrequency.MANUAL,
        )
        configs = list(session.exec(stmt).all())
        configs_checked = len(configs)

        logger.info(f"Found {configs_checked} active sync configurations to check")

        for config in configs:
            if config.needs_sync:
                try:
                    dispatched = await _dispatch_sync_for_config(session, config)
                    if dispatched:
                        syncs_dispatched += 1
                        platform = config.platform
                        platforms[platform] = platforms.get(platform, 0) + 1
                        logger.info(
                            f"Dispatched {platform} sync for user {config.user_id}"
                        )
                except Exception as e:
                    logger.error(
                        f"Failed to dispatch sync for config {config.id}: {e}"
                    )

    logger.info(
        f"Pending sync check complete: {syncs_dispatched} syncs dispatched "
        f"out of {configs_checked} configs checked"
    )

    return {
        "configs_checked": configs_checked,
        "syncs_dispatched": syncs_dispatched,
        "platforms": platforms,
    }


async def _dispatch_sync_for_config(session, config: UserSyncConfig) -> bool:
    """
    Dispatch a sync task for a given config.

    Creates a Job record and kicks off the dispatch_sync_items task.

    Returns:
        True if sync was dispatched, False otherwise.
    """
    # Determine the source based on platform
    source_map = {
        "steam": BackgroundJobSource.STEAM,
        "epic": BackgroundJobSource.EPIC,
        "gog": BackgroundJobSource.GOG,
    }

    source = source_map.get(config.platform)
    if not source:
        logger.warning(f"Unknown platform: {config.platform}")
        return False

    # Check for existing pending/processing job for this user+platform
    existing_job_stmt = select(Job).where(
        Job.user_id == config.user_id,
        Job.job_type == BackgroundJobType.SYNC,
        Job.source == source,
        col(Job.status).in_([
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
        ]),
    )
    existing_job = session.exec(existing_job_stmt).first()

    if existing_job:
        logger.debug(
            f"Skipping {config.platform} sync for user {config.user_id}: "
            f"existing job {existing_job.id} is {existing_job.status.value}"
        )
        return False

    # Create job record
    job = Job(
        user_id=config.user_id,
        job_type=BackgroundJobType.SYNC,
        source=source,
        status=BackgroundJobStatus.PENDING,
        priority=BackgroundJobPriority.LOW,  # Scheduled syncs are low priority
    )
    session.add(job)
    session.commit()
    session.refresh(job)

    # Dispatch the generic sync dispatch task
    from app.worker.tasks.sync.dispatch import dispatch_sync_items

    await enqueue_task(
        dispatch_sync_items,
        job.id,
        config.user_id,
        config.platform,
        priority=BackgroundJobPriority.LOW,
    )

    return True
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/sync/check_pending.py
git commit -m "refactor(sync): update check_pending to use new dispatch task"
```

---

## Task 9: Update API to Dispatch New Task

**Files:**
- Modify: `backend/app/api/sync.py`

**Step 1: Update trigger_manual_sync to dispatch new task**

In `backend/app/api/sync.py`, update the imports at the top:

```python
from ..worker.queues import enqueue_task
from ..models.job import BackgroundJobPriority
```

Then update the `trigger_manual_sync` function (around line 221):

```python
@router.post("/{platform}", response_model=ManualSyncTriggerResponse)
async def trigger_manual_sync(
    platform: SyncPlatform,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ManualSyncTriggerResponse:
    """
    Trigger a manual sync for a specific platform.

    Creates a high-priority sync job that will be processed immediately.
    Returns the job ID for tracking progress.
    """
    logger.info(f"Manual sync triggered for user {current_user.id}, platform {platform}")

    # Check if there's already an active sync for this platform
    active_job_stmt = select(Job).where(
        Job.user_id == current_user.id,
        Job.job_type == BackgroundJobType.SYNC,
        Job.source == _platform_to_job_source(platform),
        Job.status.in_([  # type: ignore[union-attr]
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
        ]),
    )
    active_job = session.exec(active_job_stmt).first()

    if active_job:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"A sync is already in progress for {platform.value}. Job ID: {active_job.id}",
        )

    # Create a new job record
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.SYNC,
        source=_platform_to_job_source(platform),
        status=BackgroundJobStatus.PENDING,
        priority=BackgroundJobPriority.HIGH,
    )
    session.add(job)
    session.commit()
    session.refresh(job)

    logger.info(
        f"Created sync job {job.id} for user {current_user.id}, platform {platform}"
    )

    # Dispatch the sync dispatch task
    from ..worker.tasks.sync.dispatch import dispatch_sync_items

    await enqueue_task(
        dispatch_sync_items,
        job.id,
        current_user.id,
        platform.value,
        priority=BackgroundJobPriority.HIGH,
    )

    return ManualSyncTriggerResponse(
        message=f"Sync job created for {platform.value}",
        job_id=job.id,
        platform=platform.value,
        status="queued",
    )
```

Also remove the old import `from ..worker.queues import QUEUE_HIGH` if it exists.

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/api/sync.py
git commit -m "feat(api): dispatch new fan-out task from sync endpoint"
```

---

## Task 10: Remove Old Steam Task

**Files:**
- Delete: `backend/app/worker/tasks/sync/steam.py`

**Step 1: Delete the old monolithic steam.py task**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
rm app/worker/tasks/sync/steam.py
```

**Step 2: Run tests to ensure nothing depends on it**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pytest app/tests/ -v --tb=short -x`
Expected: All tests pass (or show which tests need updating)

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add -A
git commit -m "refactor(sync): remove old monolithic steam sync task"
```

---

## Task 11: Update Import Tasks for New Queue Structure

**Files:**
- Modify: `backend/app/worker/tasks/import_export/process_import_item.py`

**Step 1: Update process_import_item.py to use single task function**

The current file has two task functions (`process_import_item_high` and `process_import_item_low`). Update to use a single task:

Replace the task decorators and `enqueue_import_task` function:

```python
# Remove the two separate task functions and replace with one:

@broker.task(task_name="import.process_item")
async def process_import_item(job_item_id: str) -> dict:
    """Process import item.

    Args:
        job_item_id: The JobItem ID to process

    Returns:
        Dictionary with processing result details
    """
    return await _process_import_item(job_item_id)


async def enqueue_import_task(job_item_id: str, priority: BackgroundJobPriority):
    """Enqueue import task with appropriate priority.

    Routes the task to the appropriate priority queue based on the priority parameter.

    Args:
        job_item_id: The JobItem ID to process
        priority: Priority level (HIGH or LOW)
    """
    from app.worker.queues import enqueue_task
    await enqueue_task(process_import_item, job_item_id, priority=priority)
```

Also update the imports at the top to remove `SUBJECT_HIGH_IMPORT` and `SUBJECT_LOW_IMPORT`.

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/import_export/process_import_item.py
git commit -m "refactor(import): use single task function with priority routing"
```

---

## Task 12: Update Export Task for New Queue Structure

**Files:**
- Modify: `backend/app/worker/tasks/import_export/export.py`

**Step 1: Update export.py task registration**

Update the task decorator to use generic task name:

```python
@broker.task(task_name="export.collection")
async def export_collection(
    job_id: str,
    export_format: str,
) -> Dict[str, Any]:
```

Also update the import to remove `SUBJECT_HIGH_EXPORT`.

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/worker/tasks/import_export/export.py
git commit -m "refactor(export): use generic task name without subject binding"
```

---

## Task 13: Write Tests for Sync Adapters

**Files:**
- Create: `backend/app/tests/test_sync_adapters.py`

**Step 1: Create test file for adapters**

Create `backend/app/tests/test_sync_adapters.py`:

```python
"""Tests for sync source adapters."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.worker.tasks.sync.adapters import ExternalGame, get_sync_adapter
from app.worker.tasks.sync.adapters.steam import SteamSyncAdapter
from app.models.job import BackgroundJobSource


class TestExternalGame:
    """Tests for ExternalGame dataclass."""

    def test_external_game_creation(self):
        """Test creating an ExternalGame."""
        game = ExternalGame(
            external_id="12345",
            title="Test Game",
            platform_id="pc-windows",
            storefront_id="steam",
            metadata={"playtime_minutes": 100},
        )

        assert game.external_id == "12345"
        assert game.title == "Test Game"
        assert game.platform_id == "pc-windows"
        assert game.storefront_id == "steam"
        assert game.metadata == {"playtime_minutes": 100}


class TestGetSyncAdapter:
    """Tests for get_sync_adapter factory."""

    def test_get_steam_adapter(self):
        """Test getting Steam adapter."""
        adapter = get_sync_adapter("steam")
        assert isinstance(adapter, SteamSyncAdapter)
        assert adapter.source == BackgroundJobSource.STEAM

    def test_get_steam_adapter_case_insensitive(self):
        """Test adapter lookup is case insensitive."""
        adapter = get_sync_adapter("STEAM")
        assert isinstance(adapter, SteamSyncAdapter)

    def test_get_unsupported_adapter_raises(self):
        """Test that unsupported source raises ValueError."""
        with pytest.raises(ValueError, match="Unsupported sync source"):
            get_sync_adapter("unsupported")


class TestSteamSyncAdapter:
    """Tests for SteamSyncAdapter."""

    def test_source_is_steam(self):
        """Test adapter has correct source."""
        adapter = SteamSyncAdapter()
        assert adapter.source == BackgroundJobSource.STEAM

    def test_get_credentials_returns_none_when_not_configured(self):
        """Test credentials return None when not configured."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {}

        assert adapter.get_credentials(user) is None

    def test_get_credentials_returns_none_when_not_verified(self):
        """Test credentials return None when not verified."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": False,
            }
        }

        assert adapter.get_credentials(user) is None

    def test_get_credentials_returns_credentials_when_valid(self):
        """Test credentials return dict when valid."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        creds = adapter.get_credentials(user)
        assert creds == {"api_key": "test_key", "steam_id": "12345678901234567"}

    def test_is_configured_true_when_credentials_valid(self):
        """Test is_configured returns True when credentials are valid."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        assert adapter.is_configured(user) is True

    def test_is_configured_false_when_not_configured(self):
        """Test is_configured returns False when not configured."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {}

        assert adapter.is_configured(user) is False

    @pytest.mark.asyncio
    async def test_fetch_games_raises_when_not_configured(self):
        """Test fetch_games raises when credentials not configured."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {}

        with pytest.raises(ValueError, match="Steam credentials not configured"):
            await adapter.fetch_games(user)

    @pytest.mark.asyncio
    async def test_fetch_games_returns_external_games(self):
        """Test fetch_games returns list of ExternalGame objects."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.id = "user123"
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        # Mock Steam game response
        mock_steam_game = MagicMock()
        mock_steam_game.appid = 12345
        mock_steam_game.name = "Test Game"
        mock_steam_game.playtime_forever = 100

        with patch("app.worker.tasks.sync.adapters.steam.SteamService") as mock_service:
            mock_instance = AsyncMock()
            mock_instance.get_owned_games = AsyncMock(return_value=[mock_steam_game])
            mock_service.return_value = mock_instance

            games = await adapter.fetch_games(user)

        assert len(games) == 1
        assert games[0].external_id == "12345"
        assert games[0].title == "Test Game"
        assert games[0].platform_id == "pc-windows"
        assert games[0].storefront_id == "steam"
        assert games[0].metadata["playtime_minutes"] == 100
```

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pytest app/tests/test_sync_adapters.py -v`
Expected: All tests pass

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/tests/test_sync_adapters.py
git commit -m "test(sync): add tests for sync source adapters"
```

---

## Task 14: Write Tests for Sync Dispatch Task

**Files:**
- Create: `backend/app/tests/test_sync_dispatch.py`

**Step 1: Create test file for dispatch task**

Create `backend/app/tests/test_sync_dispatch.py`:

```python
"""Tests for sync dispatch task."""

import pytest
import json
from unittest.mock import AsyncMock, MagicMock, patch
from datetime import datetime, timezone

from app.worker.tasks.sync.dispatch import dispatch_sync_items, _create_job_item
from app.worker.tasks.sync.adapters import ExternalGame
from app.models.job import Job, JobItem, BackgroundJobStatus, BackgroundJobType, BackgroundJobSource, BackgroundJobPriority


class TestCreateJobItem:
    """Tests for _create_job_item helper."""

    def test_creates_job_item_with_correct_fields(self):
        """Test JobItem is created with correct fields."""
        session = MagicMock()
        job = MagicMock()
        job.id = "job123"

        game = ExternalGame(
            external_id="12345",
            title="Test Game",
            platform_id="pc-windows",
            storefront_id="steam",
            metadata={"playtime_minutes": 100},
        )

        # Mock session behavior
        def mock_refresh(item):
            item.id = "item123"
        session.refresh = mock_refresh

        job_item = _create_job_item(session, job, "user123", game)

        assert job_item.job_id == "job123"
        assert job_item.user_id == "user123"
        assert job_item.item_key == "steam_12345"
        assert job_item.source_title == "Test Game"

        metadata = json.loads(job_item.source_metadata_json)
        assert metadata["external_id"] == "12345"
        assert metadata["platform_id"] == "pc-windows"
        assert metadata["storefront_id"] == "steam"
        assert metadata["metadata"]["playtime_minutes"] == 100


class TestDispatchSyncItems:
    """Tests for dispatch_sync_items task."""

    @pytest.mark.asyncio
    async def test_returns_error_when_job_not_found(self):
        """Test returns error when job doesn't exist."""
        with patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx:
            mock_session = MagicMock()
            mock_session.get.return_value = None
            mock_ctx.return_value.__aenter__ = AsyncMock(return_value=mock_session)
            mock_ctx.return_value.__aexit__ = AsyncMock(return_value=None)

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "error"
            assert "Job not found" in result["error"]

    @pytest.mark.asyncio
    async def test_returns_error_when_user_not_found(self):
        """Test returns error when user doesn't exist."""
        with patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx:
            mock_session = MagicMock()
            mock_job = MagicMock()
            mock_session.get.side_effect = lambda model, id: mock_job if model == Job else None
            mock_ctx.return_value.__aenter__ = AsyncMock(return_value=mock_session)
            mock_ctx.return_value.__aexit__ = AsyncMock(return_value=None)

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "error"
            assert "User" in result["error"]

    @pytest.mark.asyncio
    async def test_dispatches_items_for_each_game(self):
        """Test creates JobItems and dispatches tasks for each game."""
        mock_games = [
            ExternalGame("1", "Game 1", "pc-windows", "steam", {}),
            ExternalGame("2", "Game 2", "pc-windows", "steam", {}),
        ]

        with patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx, \
             patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter, \
             patch("app.worker.tasks.sync.dispatch._dispatch_process_task") as mock_dispatch:

            mock_session = MagicMock()
            mock_job = MagicMock()
            mock_job.id = "job123"
            mock_job.priority = BackgroundJobPriority.HIGH
            mock_user = MagicMock()
            mock_user.id = "user123"

            def get_side_effect(model, id):
                if model == Job:
                    return mock_job
                return mock_user

            mock_session.get.side_effect = get_side_effect

            # Mock refresh to set ID
            def mock_refresh(item):
                if hasattr(item, 'id') and item.id is None:
                    item.id = f"item_{item.item_key}"
            mock_session.refresh = mock_refresh

            mock_ctx.return_value.__aenter__ = AsyncMock(return_value=mock_session)
            mock_ctx.return_value.__aexit__ = AsyncMock(return_value=None)

            mock_adapter_instance = MagicMock()
            mock_adapter_instance.fetch_games = AsyncMock(return_value=mock_games)
            mock_adapter.return_value = mock_adapter_instance

            mock_dispatch.return_value = None

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "dispatched"
            assert result["total_games"] == 2
            assert result["dispatched"] == 2
            assert mock_dispatch.call_count == 2
```

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pytest app/tests/test_sync_dispatch.py -v`
Expected: All tests pass

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/tests/test_sync_dispatch.py
git commit -m "test(sync): add tests for sync dispatch task"
```

---

## Task 15: Write Tests for Sync Process Item Task

**Files:**
- Create: `backend/app/tests/test_sync_process_item.py`

**Step 1: Create test file for process item task**

Create `backend/app/tests/test_sync_process_item.py`:

```python
"""Tests for sync process item task."""

import pytest
import json
from unittest.mock import MagicMock, patch, AsyncMock
from datetime import datetime, timezone

from app.worker.tasks.sync.process_item import (
    process_sync_item,
    _is_already_synced,
    _is_ignored,
    _add_platform_association,
)
from app.models.job import JobItem, JobItemStatus, BackgroundJobSource


class TestIsAlreadySynced:
    """Tests for _is_already_synced helper."""

    def test_returns_true_when_synced(self):
        """Test returns True when platform association exists."""
        session = MagicMock()
        session.exec.return_value.first.return_value = MagicMock()  # Found

        result = _is_already_synced(session, "user123", "steam", "12345")
        assert result is True

    def test_returns_false_when_not_synced(self):
        """Test returns False when no platform association."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        result = _is_already_synced(session, "user123", "steam", "12345")
        assert result is False


class TestIsIgnored:
    """Tests for _is_ignored helper."""

    def test_returns_true_when_ignored(self):
        """Test returns True when game is ignored."""
        session = MagicMock()
        session.exec.return_value.first.return_value = MagicMock()  # Found

        result = _is_ignored(session, "user123", "steam", "12345")
        assert result is True

    def test_returns_false_when_not_ignored(self):
        """Test returns False when game is not ignored."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        result = _is_ignored(session, "user123", "steam", "12345")
        assert result is False

    def test_returns_false_for_unknown_storefront(self):
        """Test returns False for unknown storefront."""
        session = MagicMock()

        result = _is_ignored(session, "user123", "unknown", "12345")
        assert result is False


class TestAddPlatformAssociation:
    """Tests for _add_platform_association helper."""

    def test_creates_association_when_not_exists(self):
        """Test creates new association."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None  # Not found

        _add_platform_association(session, "ug123", "pc-windows", "steam", "12345")

        session.add.assert_called_once()
        session.commit.assert_called_once()

    def test_skips_when_association_exists(self):
        """Test skips if association already exists."""
        session = MagicMock()
        session.exec.return_value.first.return_value = MagicMock()  # Found

        _add_platform_association(session, "ug123", "pc-windows", "steam", "12345")

        session.add.assert_not_called()

    def test_sets_steam_store_url(self):
        """Test sets correct store URL for Steam."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        _add_platform_association(session, "ug123", "pc-windows", "steam", "12345")

        # Check the added platform has correct URL
        call_args = session.add.call_args
        platform = call_args[0][0]
        assert platform.store_url == "https://store.steampowered.com/app/12345"


class TestProcessSyncItem:
    """Tests for process_sync_item task."""

    @pytest.mark.asyncio
    async def test_returns_error_when_job_item_not_found(self):
        """Test returns error when job item doesn't exist."""
        with patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_session:
            mock_session.return_value.get.return_value = None

            result = await process_sync_item("item123")

            assert result["status"] == "error"
            assert "JobItem not found" in result["error"]

    @pytest.mark.asyncio
    async def test_skips_already_processed_items(self):
        """Test skips items not in PENDING/PROCESSING status."""
        with patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_session:
            mock_item = MagicMock()
            mock_item.status = JobItemStatus.COMPLETED
            mock_session.return_value.get.return_value = mock_item

            result = await process_sync_item("item123")

            assert result["status"] == "skipped"
            assert result["reason"] == "already_processed"

    @pytest.mark.asyncio
    async def test_marks_already_synced_as_completed(self):
        """Test marks already synced items as COMPLETED."""
        with patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session, \
             patch("app.worker.tasks.sync.process_item._is_already_synced") as mock_synced, \
             patch("app.worker.tasks.sync.process_item._complete_job_item") as mock_complete:

            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = json.dumps({
                "external_id": "12345",
                "storefront_id": "steam",
                "platform_id": "pc-windows",
            })

            mock_session.get.return_value = mock_item
            mock_synced.return_value = True
            mock_complete.return_value = {"status": "success", "result": "already_synced"}

            result = await process_sync_item("item123")

            mock_complete.assert_called_once()
            assert result["result"] == "already_synced"

    @pytest.mark.asyncio
    async def test_marks_ignored_as_skipped(self):
        """Test marks ignored items as SKIPPED."""
        with patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session, \
             patch("app.worker.tasks.sync.process_item._is_already_synced") as mock_synced, \
             patch("app.worker.tasks.sync.process_item._is_ignored") as mock_ignored, \
             patch("app.worker.tasks.sync.process_item._complete_job_item") as mock_complete:

            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = json.dumps({
                "external_id": "12345",
                "storefront_id": "steam",
                "platform_id": "pc-windows",
            })

            mock_session.get.return_value = mock_item
            mock_synced.return_value = False
            mock_ignored.return_value = True
            mock_complete.return_value = {"status": "success", "result": "ignored"}

            result = await process_sync_item("item123")

            mock_complete.assert_called_once()
            assert result["result"] == "ignored"
```

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pytest app/tests/test_sync_process_item.py -v`
Expected: All tests pass

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add app/tests/test_sync_process_item.py
git commit -m "test(sync): add tests for sync process item task"
```

---

## Task 16: Run Full Test Suite

**Files:** None (verification only)

**Step 1: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pyrefly check`
Expected: No errors

**Step 2: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pytest --tb=short -q`
Expected: All tests pass

**Step 3: Run frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/frontend && npm run test`
Expected: All tests pass

**Step 4: If tests fail, fix issues and commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add -A
git commit -m "fix: resolve test failures from sync refactor"
```

---

## Task 17: Clean Up Old Queue References

**Files:**
- Modify: `backend/app/worker/queues.py`

**Step 1: Remove legacy aliases from queues.py**

Update `backend/app/worker/queues.py` to remove the compatibility aliases:

```python
"""Queue configuration for NATS subject-based routing."""

from app.models.job import BackgroundJobPriority

# Priority subjects for NATS JetStream
# High priority: user-initiated tasks (manual sync, imports, exports)
# Low priority: automated tasks (scheduled syncs, maintenance)

SUBJECT_HIGH = "tasks.high"
SUBJECT_LOW = "tasks.low"


async def enqueue_task(task_func, *args, priority: BackgroundJobPriority):
    """Dispatch task to appropriate priority queue.

    Args:
        task_func: The TaskIQ task function to dispatch
        *args: Arguments to pass to the task
        priority: HIGH for user-initiated, LOW for automated tasks
    """
    subject = SUBJECT_HIGH if priority == BackgroundJobPriority.HIGH else SUBJECT_LOW
    await task_func.kicker().with_labels(subject=subject).kiq(*args)
```

**Step 2: Search for any remaining old queue references**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && grep -r "SUBJECT_HIGH_\|SUBJECT_LOW_\|QUEUE_HIGH\|QUEUE_LOW" app/`

Fix any remaining references found.

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend && uv run pytest --tb=short -q`
Expected: All tests pass

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/sync-fanout/backend
git add -A
git commit -m "refactor(worker): remove legacy queue aliases"
```

---

## Summary

This plan implements the sync fan-out design in 17 tasks:

1. **Tasks 1-2**: Queue simplification and enqueue helper
2. **Tasks 3-4**: Source adapter abstraction (base + Steam)
3. **Tasks 5-6**: New sync tasks (dispatch + process_item)
4. **Tasks 7-9**: Integration (exports, check_pending, API)
5. **Task 10**: Remove old monolithic Steam task
6. **Tasks 11-12**: Update import/export for new queues
7. **Tasks 13-15**: Tests for new components
8. **Tasks 16-17**: Verification and cleanup

Total estimated implementation: ~2-3 hours for experienced developer.
