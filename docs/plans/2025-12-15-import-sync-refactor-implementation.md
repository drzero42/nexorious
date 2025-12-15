# Import/Sync Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor import/sync to cleanly separate one-time imports (files) from recurring syncs (APIs), remove SteamGame staging table, and unify review workflow.

**Architecture:** Remove SteamGame model, add IgnoredExternalGame for tracking ignored games, add match_confidence to ReviewItem. Steam only has sync (no import). Darkadia has two-phase review (platform/storefront resolution, then game matching).

**Tech Stack:** FastAPI, SQLModel, Alembic, Taskiq, Svelte 5, TypeScript

---

## Phase 1: Database Schema Changes

### Task 1.1: Add IgnoredExternalGame Model

**Files:**
- Create: `backend/app/models/ignored_external_game.py`
- Modify: `backend/app/models/__init__.py`
- Modify: `backend/app/models/user.py` (add relationship)
- Modify: `backend/app/core/database.py` (import for Alembic)

**Step 1: Create the model file**

```python
# backend/app/models/ignored_external_game.py
"""
Model for tracking games users have explicitly ignored from external sources.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import TYPE_CHECKING
from datetime import datetime, timezone
import uuid

from .job import BackgroundJobSource

if TYPE_CHECKING:
    from .user import User


class IgnoredExternalGame(SQLModel, table=True):
    """
    Tracks games a user has explicitly ignored from external sync sources.

    When a user ignores a game during sync review, it's recorded here
    so it won't appear in future syncs.
    """

    __tablename__ = "ignored_external_games"
    __table_args__ = (
        UniqueConstraint(
            "user_id", "source", "external_id",
            name="uq_ignored_external_games_user_source_external"
        ),
    )

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    source: BackgroundJobSource = Field(index=True, description="Source platform (STEAM, EPIC, GOG)")
    external_id: str = Field(max_length=100, index=True, description="Platform-specific ID (Steam AppID, etc.)")
    title: str = Field(max_length=500, description="Game title for display purposes")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: "User" = Relationship(back_populates="ignored_external_games")
```

**Step 2: Update models/__init__.py**

Add to imports and __all__:
```python
from .ignored_external_game import IgnoredExternalGame
```

**Step 3: Update models/user.py**

Add relationship to User class:
```python
ignored_external_games: List["IgnoredExternalGame"] = Relationship(back_populates="user")
```

Add to TYPE_CHECKING imports:
```python
from .ignored_external_game import IgnoredExternalGame
```

**Step 4: Update core/database.py**

Add import for Alembic detection:
```python
from app.models.ignored_external_game import IgnoredExternalGame  # noqa: F401
```

**Step 5: Generate migration**

Run: `cd backend && uv run alembic revision --autogenerate -m "add ignored_external_games table"`

**Step 6: Apply migration**

Run: `cd backend && uv run alembic upgrade head`

**Step 7: Commit**

```bash
git add backend/app/models/ignored_external_game.py backend/app/models/__init__.py backend/app/models/user.py backend/app/core/database.py backend/app/alembic/versions/
git commit -m "feat(models): add IgnoredExternalGame model for tracking ignored sync games"
```

---

### Task 1.2: Add match_confidence to ReviewItem

**Files:**
- Modify: `backend/app/models/job.py`

**Step 1: Add field to ReviewItem model**

In `backend/app/models/job.py`, add to ReviewItem class after `resolved_igdb_id`:

```python
    match_confidence: Optional[float] = Field(
        default=None,
        ge=0.0,
        le=1.0,
        description="Confidence score for auto-match (1.0 = exact match or single result)"
    )
```

**Step 2: Generate migration**

Run: `cd backend && uv run alembic revision --autogenerate -m "add match_confidence to review_items"`

**Step 3: Apply migration**

Run: `cd backend && uv run alembic upgrade head`

**Step 4: Commit**

```bash
git add backend/app/models/job.py backend/app/alembic/versions/
git commit -m "feat(models): add match_confidence field to ReviewItem"
```

---

### Task 1.3: Remove SteamGame Model

**Files:**
- Delete: `backend/app/models/steam_game.py`
- Modify: `backend/app/models/__init__.py`
- Modify: `backend/app/models/user.py` (remove relationship)
- Modify: `backend/app/core/database.py` (remove import)

**Step 1: Remove from models/__init__.py**

Remove from imports and __all__:
```python
# Remove: from .steam_game import SteamGame
```

**Step 2: Remove relationship from models/user.py**

Remove from User class:
```python
# Remove: steam_games: List["SteamGame"] = Relationship(back_populates="user")
```

Remove from TYPE_CHECKING imports if present.

**Step 3: Remove from core/database.py**

Remove import:
```python
# Remove: from app.models.steam_game import SteamGame
```

**Step 4: Delete the model file**

```bash
rm backend/app/models/steam_game.py
```

**Step 5: Generate migration**

Run: `cd backend && uv run alembic revision --autogenerate -m "drop steam_games table"`

**Step 6: Apply migration**

Run: `cd backend && uv run alembic upgrade head`

**Step 7: Commit**

```bash
git add -u backend/app/models/ backend/app/core/database.py backend/app/alembic/versions/
git commit -m "refactor(models): remove SteamGame model (replaced by ReviewItem flow)"
```

---

### Task 1.4: Remove ImportJob Model (if still used)

**Files:**
- Delete: `backend/app/models/import_job.py` (if exists and unused)
- Modify: `backend/app/models/__init__.py`
- Modify: `backend/app/core/database.py`

**Step 1: Check usage**

Run: `grep -r "ImportJob" backend/app --include="*.py" | grep -v "test_" | grep -v "__pycache__"`

If only imported in models/__init__.py and database.py, proceed with removal.

**Step 2: Remove from models/__init__.py**

Remove imports and __all__ entries for ImportJob, ImportType, ImportStatus, JobType.

**Step 3: Remove from core/database.py**

Remove import if present.

**Step 4: Delete the model file**

```bash
rm backend/app/models/import_job.py
```

**Step 5: Generate and apply migration**

Run: `cd backend && uv run alembic revision --autogenerate -m "drop import_jobs table"`
Run: `cd backend && uv run alembic upgrade head`

**Step 6: Commit**

```bash
git add -u backend/app/models/ backend/app/core/database.py backend/app/alembic/versions/
git commit -m "refactor(models): remove legacy ImportJob model (replaced by unified Job)"
```

---

## Phase 2: Backend API Changes

### Task 2.1: Add Ignored Games Endpoints to Sync API

**Files:**
- Modify: `backend/app/api/sync.py`
- Create: `backend/app/schemas/ignored_game.py`
- Modify: `backend/app/schemas/__init__.py`

**Step 1: Create schema file**

```python
# backend/app/schemas/ignored_game.py
"""Schemas for ignored external games."""

from pydantic import BaseModel, Field
from datetime import datetime
from typing import Optional

from app.models.job import BackgroundJobSource


class IgnoredGameResponse(BaseModel):
    """Response schema for an ignored game."""

    id: str
    source: BackgroundJobSource
    external_id: str
    title: str
    created_at: datetime

    model_config = {"from_attributes": True}


class IgnoredGameListResponse(BaseModel):
    """Response schema for list of ignored games."""

    items: list[IgnoredGameResponse]
    total: int


class IgnoredGameCreateRequest(BaseModel):
    """Request schema for ignoring a game."""

    source: BackgroundJobSource
    external_id: str
    title: str
```

**Step 2: Update schemas/__init__.py**

Add imports for new schemas.

**Step 3: Add endpoints to sync.py**

Add after existing endpoints:

```python
from app.models.ignored_external_game import IgnoredExternalGame
from app.schemas.ignored_game import (
    IgnoredGameResponse,
    IgnoredGameListResponse,
)


@router.get("/ignored", response_model=IgnoredGameListResponse)
async def list_ignored_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    source: Optional[BackgroundJobSource] = None,
    skip: int = 0,
    limit: int = 100,
) -> IgnoredGameListResponse:
    """List all ignored external games for the current user."""
    query = select(IgnoredExternalGame).where(
        IgnoredExternalGame.user_id == current_user.id
    )

    if source:
        query = query.where(IgnoredExternalGame.source == source)

    # Get total count
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()

    # Get paginated results
    query = query.offset(skip).limit(limit).order_by(IgnoredExternalGame.created_at.desc())
    items = session.exec(query).all()

    return IgnoredGameListResponse(
        items=[IgnoredGameResponse.model_validate(item) for item in items],
        total=total,
    )


@router.delete("/ignored/{ignored_id}")
async def unignore_game(
    ignored_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> dict:
    """Remove a game from the ignored list (will appear in next sync)."""
    ignored = session.exec(
        select(IgnoredExternalGame).where(
            IgnoredExternalGame.id == ignored_id,
            IgnoredExternalGame.user_id == current_user.id,
        )
    ).first()

    if not ignored:
        raise HTTPException(status_code=404, detail="Ignored game not found")

    session.delete(ignored)
    session.commit()

    return {"message": "Game removed from ignored list"}
```

**Step 4: Write tests**

Create `backend/app/tests/test_ignored_games_api.py`:

```python
"""Tests for ignored games API endpoints."""

import pytest
from fastapi.testclient import TestClient


class TestIgnoredGamesAPI:
    """Test ignored games endpoints."""

    def test_list_ignored_games_empty(self, client: TestClient, auth_headers: dict):
        """List returns empty when no games ignored."""
        response = client.get("/api/sync/ignored", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert data["items"] == []
        assert data["total"] == 0

    def test_unignore_game_not_found(self, client: TestClient, auth_headers: dict):
        """Unignore returns 404 for non-existent game."""
        response = client.delete(
            "/api/sync/ignored/nonexistent-id",
            headers=auth_headers,
        )
        assert response.status_code == 404
```

**Step 5: Run tests**

Run: `cd backend && uv run pytest app/tests/test_ignored_games_api.py -v`

**Step 6: Commit**

```bash
git add backend/app/api/sync.py backend/app/schemas/ignored_game.py backend/app/schemas/__init__.py backend/app/tests/test_ignored_games_api.py
git commit -m "feat(api): add ignored games endpoints to sync API"
```

---

### Task 2.2: Remove Steam Import Endpoint

**Files:**
- Modify: `backend/app/api/import_endpoints.py`

**Step 1: Remove steam import endpoint**

In `backend/app/api/import_endpoints.py`, remove the entire `POST /import/steam` endpoint function.

**Step 2: Update any imports**

Remove unused imports related to Steam import.

**Step 3: Update tests**

Remove or update tests in `backend/app/tests/test_import_tasks.py` that test Steam import endpoint.

**Step 4: Run tests**

Run: `cd backend && uv run pytest app/tests/test_import_tasks.py -v`

**Step 5: Commit**

```bash
git add backend/app/api/import_endpoints.py backend/app/tests/
git commit -m "refactor(api): remove Steam import endpoint (Steam only has sync)"
```

---

### Task 2.3: Update ReviewItem Schema with match_confidence

**Files:**
- Modify: `backend/app/schemas/review.py`

**Step 1: Add match_confidence to response schemas**

In ReviewItemResponse and ReviewItemDetailResponse, add:

```python
    match_confidence: Optional[float] = Field(
        default=None,
        description="Confidence score (1.0 = exact match, >= 0.85 = auto-match, < 0.85 = needs review)"
    )
```

**Step 2: Run tests**

Run: `cd backend && uv run pytest app/tests/test_review_api.py -v`

**Step 3: Commit**

```bash
git add backend/app/schemas/review.py
git commit -m "feat(schemas): add match_confidence to ReviewItem response schemas"
```

---

### Task 2.4: Add Platform/Storefront Resolution Endpoints for Darkadia

**Files:**
- Modify: `backend/app/api/jobs.py`
- Create: `backend/app/schemas/mapping_resolution.py`

**Step 1: Create schema file**

```python
# backend/app/schemas/mapping_resolution.py
"""Schemas for platform/storefront mapping resolution."""

from pydantic import BaseModel, Field
from typing import Optional


class UnresolvedMapping(BaseModel):
    """An unresolved platform or storefront name from import."""

    original_name: str = Field(description="Name from import source")
    suggested_id: Optional[str] = Field(default=None, description="Suggested match ID")
    suggested_name: Optional[str] = Field(default=None, description="Suggested match name")
    confidence: float = Field(default=0.0, description="Match confidence 0.0-1.0")
    type: str = Field(description="'platform' or 'storefront'")


class UnresolvedMappingsResponse(BaseModel):
    """Response with all unresolved mappings for a job."""

    platforms: list[UnresolvedMapping]
    storefronts: list[UnresolvedMapping]
    all_resolved: bool = Field(description="True if no user input needed")


class MappingResolution(BaseModel):
    """User's resolution for a single mapping."""

    original_name: str
    resolved_id: str
    type: str = Field(description="'platform' or 'storefront'")


class ResolveMappingsRequest(BaseModel):
    """Request to resolve multiple mappings."""

    resolutions: list[MappingResolution]
```

**Step 2: Add endpoints to jobs.py**

```python
from app.schemas.mapping_resolution import (
    UnresolvedMappingsResponse,
    ResolveMappingsRequest,
)


@router.get("/{job_id}/unresolved-mappings", response_model=UnresolvedMappingsResponse)
async def get_unresolved_mappings(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> UnresolvedMappingsResponse:
    """Get unresolved platform/storefront mappings for a Darkadia import job."""
    job = session.get(Job, job_id)
    if not job or job.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job not found")

    if job.source != BackgroundJobSource.DARKADIA:
        raise HTTPException(status_code=400, detail="Only Darkadia imports have mapping resolution")

    result_summary = job.get_result_summary()
    unresolved = result_summary.get("unresolved_mappings", {"platforms": [], "storefronts": []})

    return UnresolvedMappingsResponse(
        platforms=unresolved.get("platforms", []),
        storefronts=unresolved.get("storefronts", []),
        all_resolved=len(unresolved.get("platforms", [])) == 0 and len(unresolved.get("storefronts", [])) == 0,
    )


@router.put("/{job_id}/resolve-mappings")
async def resolve_mappings(
    job_id: str,
    request: ResolveMappingsRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> dict:
    """Submit user's platform/storefront mapping resolutions."""
    job = session.get(Job, job_id)
    if not job or job.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job not found")

    if job.source != BackgroundJobSource.DARKADIA:
        raise HTTPException(status_code=400, detail="Only Darkadia imports have mapping resolution")

    result_summary = job.get_result_summary()
    resolved_mappings = result_summary.get("resolved_mappings", {"platforms": {}, "storefronts": {}})

    for resolution in request.resolutions:
        if resolution.type == "platform":
            resolved_mappings["platforms"][resolution.original_name] = resolution.resolved_id
        elif resolution.type == "storefront":
            resolved_mappings["storefronts"][resolution.original_name] = resolution.resolved_id

    result_summary["resolved_mappings"] = resolved_mappings
    result_summary["unresolved_mappings"] = {"platforms": [], "storefronts": []}  # Clear unresolved
    job.set_result_summary(result_summary)
    session.add(job)
    session.commit()

    return {"message": "Mappings resolved", "resolved_count": len(request.resolutions)}
```

**Step 3: Commit**

```bash
git add backend/app/api/jobs.py backend/app/schemas/mapping_resolution.py
git commit -m "feat(api): add platform/storefront resolution endpoints for Darkadia imports"
```

---

## Phase 3: Refactor Steam Sync Task

### Task 3.1: Rewrite Steam Sync Worker Task

**Files:**
- Modify: `backend/app/worker/tasks/sync/steam.py`

**Step 1: Rewrite the sync task**

The new sync task should:
1. Fetch Steam library
2. Filter already synced (check UserGamePlatform for steam storefront + store_game_id)
3. Filter ignored (check IgnoredExternalGame)
4. Match remaining to IGDB
5. Auto-link high confidence matches to existing collection games
6. Create ReviewItems for others

```python
# backend/app/worker/tasks/sync/steam.py
"""Steam library sync task."""

from datetime import datetime, timezone
from typing import Any

from sqlmodel import Session, select, and_

from app.core.database import get_session_context
from app.models.job import (
    Job,
    BackgroundJobStatus,
    BackgroundJobSource,
    ReviewItem,
    ReviewItemStatus,
)
from app.models.ignored_external_game import IgnoredExternalGame
from app.models.user_game import UserGame, UserGamePlatform
from app.models.platform import Platform, Storefront
from app.services.steam import SteamService
from app.services.matching.service import MatchingService
from app.services.matching.models import MatchRequest, MatchStatus
from app.services.game_service import GameService
from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH

# Constants
STEAM_STOREFRONT_ID = "steam"
PC_WINDOWS_PLATFORM_ID = "pc-windows"
AUTO_MATCH_CONFIDENCE_THRESHOLD = 0.85


@broker.task(task_name="sync.steam_library", queue=QUEUE_HIGH)
async def sync_steam_library(job_id: str) -> dict[str, Any]:
    """
    Sync user's Steam library.

    1. Fetch Steam library
    2. Filter already synced games (have Steam storefront + AppID in UserGamePlatform)
    3. Filter ignored games (in IgnoredExternalGame)
    4. Match remaining to IGDB
    5. Auto-link high confidence matches to existing collection games
    6. Create ReviewItems for others
    """
    stats = {
        "total_games": 0,
        "already_synced": 0,
        "ignored": 0,
        "auto_linked": 0,
        "auto_matched": 0,
        "needs_review": 0,
        "no_match": 0,
        "errors": 0,
    }

    async with get_session_context() as session:
        # Get job and update status
        job = session.get(Job, job_id)
        if not job:
            raise ValueError(f"Job {job_id} not found")

        job.status = BackgroundJobStatus.PROCESSING
        job.started_at = datetime.now(timezone.utc)
        session.add(job)
        session.commit()

        try:
            result_summary = job.get_result_summary()
            user_id = job.user_id
            steam_api_key = result_summary.get("steam_api_key")
            steam_id = result_summary.get("steam_id")

            # Initialize services
            steam_service = SteamService(api_key=steam_api_key)
            matching_service = MatchingService(session)
            game_service = GameService(session)

            # Fetch Steam library
            steam_games = await steam_service.get_owned_games(steam_id)
            stats["total_games"] = len(steam_games)
            job.progress_total = len(steam_games)
            session.add(job)
            session.commit()

            # Get already synced AppIDs
            synced_appids = _get_synced_steam_appids(session, user_id)

            # Get ignored AppIDs
            ignored_appids = _get_ignored_steam_appids(session, user_id)

            for i, steam_game in enumerate(steam_games):
                try:
                    appid = str(steam_game.appid)

                    # Update progress
                    job.progress_current = i + 1
                    session.add(job)
                    session.commit()

                    # Skip already synced
                    if appid in synced_appids:
                        stats["already_synced"] += 1
                        continue

                    # Skip ignored
                    if appid in ignored_appids:
                        stats["ignored"] += 1
                        continue

                    # Match to IGDB
                    match_request = MatchRequest(
                        source_title=steam_game.name,
                        source_platform="steam",
                        source_metadata={"steam_appid": appid},
                    )
                    match_result = await matching_service.match(match_request)

                    if match_result.status == MatchStatus.MATCHED:
                        confidence = match_result.confidence_score or 0.0
                        igdb_id = match_result.igdb_id

                        # Check if game already in collection
                        existing_user_game = session.exec(
                            select(UserGame).where(
                                UserGame.user_id == user_id,
                                UserGame.game_id == igdb_id,
                            )
                        ).first()

                        if existing_user_game and confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
                            # Auto-link: add Steam platform association
                            _add_steam_platform(session, existing_user_game.id, appid)
                            stats["auto_linked"] += 1
                        elif confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
                            # High confidence, new game - create ReviewItem for approval
                            _create_review_item(
                                session, job, user_id, steam_game.name, appid,
                                igdb_id, match_result.igdb_title, confidence,
                                match_result.candidates or [],
                            )
                            stats["auto_matched"] += 1
                        else:
                            # Low confidence - needs review
                            _create_review_item(
                                session, job, user_id, steam_game.name, appid,
                                igdb_id, match_result.igdb_title, confidence,
                                match_result.candidates or [],
                            )
                            stats["needs_review"] += 1

                    elif match_result.status == MatchStatus.NEEDS_REVIEW:
                        # Multiple candidates
                        _create_review_item(
                            session, job, user_id, steam_game.name, appid,
                            None, None, 0.0,
                            match_result.candidates or [],
                        )
                        stats["needs_review"] += 1

                    else:
                        # No match found
                        _create_review_item(
                            session, job, user_id, steam_game.name, appid,
                            None, None, 0.0, [],
                        )
                        stats["no_match"] += 1

                except Exception as e:
                    stats["errors"] += 1
                    # Log error but continue with other games

            # Determine final job status
            pending_count = session.exec(
                select(ReviewItem).where(
                    ReviewItem.job_id == job.id,
                    ReviewItem.status == ReviewItemStatus.PENDING,
                )
            ).all()

            if len(pending_count) > 0:
                job.status = BackgroundJobStatus.AWAITING_REVIEW
            else:
                job.status = BackgroundJobStatus.COMPLETED

            job.completed_at = datetime.now(timezone.utc)

            # Clear sensitive data, keep stats
            result_summary.pop("steam_api_key", None)
            result_summary["stats"] = stats
            job.set_result_summary(result_summary)
            session.add(job)
            session.commit()

            return stats

        except Exception as e:
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            raise


def _get_synced_steam_appids(session: Session, user_id: str) -> set[str]:
    """Get all Steam AppIDs already synced for this user."""
    results = session.exec(
        select(UserGamePlatform.store_game_id)
        .join(UserGame)
        .where(
            UserGame.user_id == user_id,
            UserGamePlatform.storefront_id == STEAM_STOREFRONT_ID,
            UserGamePlatform.store_game_id.isnot(None),
        )
    ).all()
    return {appid for appid in results if appid}


def _get_ignored_steam_appids(session: Session, user_id: str) -> set[str]:
    """Get all ignored Steam AppIDs for this user."""
    results = session.exec(
        select(IgnoredExternalGame.external_id).where(
            IgnoredExternalGame.user_id == user_id,
            IgnoredExternalGame.source == BackgroundJobSource.STEAM,
        )
    ).all()
    return set(results)


def _add_steam_platform(session: Session, user_game_id: str, steam_appid: str) -> None:
    """Add Steam platform association to an existing UserGame."""
    # Check if association already exists
    existing = session.exec(
        select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game_id,
            UserGamePlatform.storefront_id == STEAM_STOREFRONT_ID,
            UserGamePlatform.store_game_id == steam_appid,
        )
    ).first()

    if not existing:
        platform = UserGamePlatform(
            user_game_id=user_game_id,
            platform_id=PC_WINDOWS_PLATFORM_ID,
            storefront_id=STEAM_STOREFRONT_ID,
            store_game_id=steam_appid,
            store_url=f"https://store.steampowered.com/app/{steam_appid}",
            is_available=True,
        )
        session.add(platform)
        session.commit()


def _create_review_item(
    session: Session,
    job: Job,
    user_id: str,
    source_title: str,
    steam_appid: str,
    igdb_id: int | None,
    igdb_title: str | None,
    confidence: float,
    candidates: list,
) -> ReviewItem:
    """Create a ReviewItem for user review."""
    import json

    review_item = ReviewItem(
        job_id=job.id,
        user_id=user_id,
        status=ReviewItemStatus.PENDING,
        source_title=source_title,
        source_metadata_json=json.dumps({
            "steam_appid": steam_appid,
            "source": "steam",
        }),
        igdb_candidates_json=json.dumps([
            {"igdb_id": igdb_id, "name": igdb_title, "confidence": confidence}
        ] if igdb_id else [c.__dict__ if hasattr(c, '__dict__') else c for c in candidates]),
        resolved_igdb_id=igdb_id if confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD else None,
        match_confidence=confidence if confidence > 0 else None,
    )
    session.add(review_item)
    session.commit()
    return review_item
```

**Step 2: Write tests**

Create comprehensive tests for the new sync task.

**Step 3: Run tests**

Run: `cd backend && uv run pytest app/tests/test_steam_sync_task.py -v`

**Step 4: Commit**

```bash
git add backend/app/worker/tasks/sync/steam.py backend/app/tests/
git commit -m "refactor(worker): rewrite Steam sync task with new review flow"
```

---

### Task 3.2: Update Review API for Sync Finalization

**Files:**
- Modify: `backend/app/api/review.py`

**Step 1: Update match endpoint to handle sync finalization**

When a ReviewItem is matched, if it's from a sync:
1. Add game to collection if not present (via IGDB procedure)
2. Add platform/storefront association with store_game_id

```python
@router.post("/{item_id}/match")
async def match_review_item(
    item_id: str,
    igdb_id: int,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ReviewItemResponse:
    """Match a review item to an IGDB game and sync it."""
    review_item = session.exec(
        select(ReviewItem).where(
            ReviewItem.id == item_id,
            ReviewItem.user_id == current_user.id,
        )
    ).first()

    if not review_item:
        raise HTTPException(status_code=404, detail="Review item not found")

    # Get source metadata
    source_metadata = review_item.get_source_metadata()
    source = source_metadata.get("source")

    # Add game to collection if not present
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == current_user.id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if not existing_user_game:
        # Create game from IGDB
        game_service = GameService(session)
        await game_service.create_or_update_game_from_igdb(igdb_id)

        # Create UserGame
        user_game = UserGame(
            user_id=current_user.id,
            game_id=igdb_id,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)
        existing_user_game = user_game

    # Add platform association based on source
    if source == "steam":
        steam_appid = source_metadata.get("steam_appid")
        if steam_appid:
            _add_steam_platform_association(
                session, existing_user_game.id, steam_appid
            )

    # Update review item
    review_item.status = ReviewItemStatus.MATCHED
    review_item.resolved_igdb_id = igdb_id
    review_item.resolved_at = datetime.now(timezone.utc)
    session.add(review_item)
    session.commit()

    return ReviewItemResponse.model_validate(review_item)
```

**Step 2: Update skip endpoint to create IgnoredExternalGame**

```python
@router.post("/{item_id}/skip")
async def skip_review_item(
    item_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ReviewItemResponse:
    """Skip a review item and add to ignored list."""
    review_item = session.exec(
        select(ReviewItem).where(
            ReviewItem.id == item_id,
            ReviewItem.user_id == current_user.id,
        )
    ).first()

    if not review_item:
        raise HTTPException(status_code=404, detail="Review item not found")

    # Get source metadata
    source_metadata = review_item.get_source_metadata()
    source = source_metadata.get("source")

    # Create IgnoredExternalGame for sync sources
    if source in ["steam", "epic", "gog"]:
        external_id = source_metadata.get(f"{source}_appid") or source_metadata.get("external_id")
        if external_id:
            ignored = IgnoredExternalGame(
                user_id=current_user.id,
                source=BackgroundJobSource(source.upper()),
                external_id=str(external_id),
                title=review_item.source_title,
            )
            session.add(ignored)

    # Update review item
    review_item.status = ReviewItemStatus.SKIPPED
    review_item.resolved_at = datetime.now(timezone.utc)
    session.add(review_item)
    session.commit()

    return ReviewItemResponse.model_validate(review_item)
```

**Step 3: Commit**

```bash
git add backend/app/api/review.py
git commit -m "feat(api): update review endpoints to handle sync finalization and ignored games"
```

---

## Phase 4: Remove Steam-Specific Code

### Task 4.1: Remove Steam Import Worker Task

**Files:**
- Delete: `backend/app/worker/tasks/import_export/import_steam.py`
- Modify: `backend/app/worker/tasks/import_export/__init__.py`

**Step 1: Remove import from __init__.py**

**Step 2: Delete the file**

```bash
rm backend/app/worker/tasks/import_export/import_steam.py
```

**Step 3: Commit**

```bash
git add -u backend/app/worker/tasks/import_export/
git commit -m "refactor(worker): remove Steam import task (Steam only has sync)"
```

---

### Task 4.2: Remove steam_games Service Directory

**Files:**
- Delete: `backend/app/services/steam_games/` (entire directory)

**Step 1: Check for any remaining usages**

Run: `grep -r "steam_games" backend/app --include="*.py" | grep -v test_ | grep -v __pycache__`

**Step 2: Update any remaining imports**

**Step 3: Delete the directory**

```bash
rm -rf backend/app/services/steam_games/
```

**Step 4: Commit**

```bash
git add -u backend/app/services/
git commit -m "refactor(services): remove steam_games service (replaced by sync task)"
```

---

### Task 4.3: Remove Steam Batch Import API

**Files:**
- Delete: `backend/app/api/import_api/sources/steam_batch.py`

**Step 1: Check router registration and remove**

**Step 2: Delete the file**

```bash
rm backend/app/api/import_api/sources/steam_batch.py
```

**Step 3: Commit**

```bash
git add -u backend/app/api/
git commit -m "refactor(api): remove Steam batch import API"
```

---

## Phase 5: Frontend Changes

### Task 5.1: Remove Steam Import Route

**Files:**
- Delete: `frontend/src/routes/import/steam/` (entire directory)

**Step 1: Delete the directory**

```bash
rm -rf frontend/src/routes/import/steam/
```

**Step 2: Update import landing page to remove Steam import option**

In `frontend/src/routes/import/+page.svelte`, remove Steam import card/link.

**Step 3: Commit**

```bash
git add -u frontend/src/routes/import/
git commit -m "refactor(frontend): remove Steam import route (Steam only has sync)"
```

---

### Task 5.2: Remove Steam-Specific Components

**Files:**
- Delete: `frontend/src/lib/components/SteamGameCard.svelte`
- Delete: `frontend/src/lib/components/SteamGameCard.test.ts`
- Delete: `frontend/src/lib/components/SteamGamesTable.svelte`
- Modify: `frontend/src/lib/components/index.ts`

**Step 1: Remove from index.ts exports**

**Step 2: Delete the files**

```bash
rm frontend/src/lib/components/SteamGameCard.svelte
rm frontend/src/lib/components/SteamGameCard.test.ts
rm frontend/src/lib/components/SteamGamesTable.svelte
```

**Step 3: Commit**

```bash
git add -u frontend/src/lib/components/
git commit -m "refactor(frontend): remove Steam-specific components"
```

---

### Task 5.3: Remove steam-games Store

**Files:**
- Delete: `frontend/src/lib/stores/steam-games.svelte.ts`

**Step 1: Check for usages**

Run: `grep -r "steam-games" frontend/src --include="*.ts" --include="*.svelte"`

**Step 2: Delete the file**

```bash
rm frontend/src/lib/stores/steam-games.svelte.ts
```

**Step 3: Commit**

```bash
git add -u frontend/src/lib/stores/
git commit -m "refactor(frontend): remove steam-games store"
```

---

### Task 5.4: Remove SteamImportServiceAdapter

**Files:**
- Delete: `frontend/src/lib/adapters/SteamImportServiceAdapter.ts`

**Step 1: Delete the file**

```bash
rm frontend/src/lib/adapters/SteamImportServiceAdapter.ts
```

**Step 2: Commit**

```bash
git add -u frontend/src/lib/adapters/
git commit -m "refactor(frontend): remove SteamImportServiceAdapter"
```

---

### Task 5.5: Update Review Store with match_confidence

**Files:**
- Modify: `frontend/src/lib/stores/review.svelte.ts`

**Step 1: Add match_confidence to interfaces**

```typescript
export interface ReviewItem {
  id: string;
  job_id: string;
  status: ReviewItemStatus;
  source_title: string;
  source_metadata: Record<string, unknown>;
  igdb_candidates: IGDBCandidate[];
  resolved_igdb_id: number | null;
  match_confidence: number | null;  // Add this
  created_at: string;
  resolved_at: string | null;
}
```

**Step 2: Add helper for determining review state**

```typescript
export function getReviewState(item: ReviewItem): 'auto_matched' | 'needs_input' | 'no_results' {
  if (!item.igdb_candidates || item.igdb_candidates.length === 0) {
    return 'no_results';
  }
  if (item.match_confidence !== null && item.match_confidence >= 0.85) {
    return 'auto_matched';
  }
  return 'needs_input';
}
```

**Step 3: Commit**

```bash
git add frontend/src/lib/stores/review.svelte.ts
git commit -m "feat(frontend): add match_confidence support to review store"
```

---

### Task 5.6: Add Ignored Games to Sync Store

**Files:**
- Modify: `frontend/src/lib/stores/sync.svelte.ts`

**Step 1: Add ignored games state and methods**

```typescript
export interface IgnoredGame {
  id: string;
  source: SyncPlatform;
  external_id: string;
  title: string;
  created_at: string;
}

// In store state
let ignoredGames = $state<IgnoredGame[]>([]);

// Add methods
const loadIgnoredGames = async (source?: SyncPlatform) => {
  const params = source ? `?source=${source}` : '';
  const response = await apiCall<{ items: IgnoredGame[]; total: number }>(
    `/sync/ignored${params}`
  );
  ignoredGames = response.items;
};

const unignoreGame = async (id: string) => {
  await apiCall(`/sync/ignored/${id}`, { method: 'DELETE' });
  ignoredGames = ignoredGames.filter(g => g.id !== id);
};
```

**Step 2: Commit**

```bash
git add frontend/src/lib/stores/sync.svelte.ts
git commit -m "feat(frontend): add ignored games support to sync store"
```

---

## Phase 6: Update Tests

### Task 6.1: Remove Obsolete Backend Tests

**Files:**
- Delete/update: `backend/app/tests/test_steam_games_*.py`
- Delete/update: `backend/app/tests/test_steam_import_*.py`

**Step 1: Identify obsolete test files**

```bash
ls backend/app/tests/test_steam_*.py
```

**Step 2: Delete obsolete tests**

**Step 3: Run all tests to verify**

Run: `cd backend && uv run pytest -v`

**Step 4: Commit**

```bash
git add -u backend/app/tests/
git commit -m "test(backend): remove obsolete Steam import tests"
```

---

### Task 6.2: Update Frontend Tests

**Files:**
- Update: `frontend/src/routes/import/page.test.ts`
- Delete: Steam-specific test files

**Step 1: Update import page tests to not expect Steam option**

**Step 2: Run all tests**

Run: `cd frontend && npm run test`

**Step 3: Commit**

```bash
git add frontend/src/
git commit -m "test(frontend): update tests for removed Steam import"
```

---

## Phase 7: Final Verification

### Task 7.1: Run Full Test Suite

**Step 1: Backend tests**

Run: `cd backend && uv run pytest --cov=app --cov-report=term-missing`

Expected: All tests pass, >80% coverage

**Step 2: Frontend tests**

Run: `cd frontend && npm run check && npm run test`

Expected: All checks pass, all tests pass

**Step 3: Type checking**

Run: `cd backend && uv run pyrefly check`

Expected: No type errors

---

### Task 7.2: Manual Verification

**Step 1: Start services**

```bash
podman-compose up --build
```

**Step 2: Verify Steam sync works**

1. Go to sync settings
2. Configure Steam credentials
3. Trigger sync
4. Verify ReviewItems are created
5. Approve a game
6. Verify game added to collection with Steam platform

**Step 3: Verify Darkadia import works**

1. Go to import
2. Upload Darkadia CSV
3. Verify platform/storefront resolution step (if needed)
4. Review matched games
5. Approve and verify games added

**Step 4: Verify ignored games**

1. Skip a game during review
2. Verify it appears in ignored list
3. Unignore it
4. Trigger sync again
5. Verify game reappears in review

---

### Task 7.3: Final Commit and Cleanup

**Step 1: Run linting**

```bash
cd backend && uv run ruff check . --fix
cd frontend && npm run check
```

**Step 2: Final commit if any fixes**

```bash
git add -A
git commit -m "chore: final cleanup after import/sync refactor"
```
