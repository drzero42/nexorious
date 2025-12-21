# Unified Export Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Consolidate the separate collection and wishlist export endpoints into a single unified export that includes all user games.

**Architecture:** Remove the `ExportScope` concept entirely. A single export endpoint per format (JSON/CSV) exports all games regardless of ownership status. The `ownership_status` field in each game record preserves this information for re-import.

**Tech Stack:** FastAPI (backend), Next.js/React (frontend), pytest, Vitest

---

## Task 1: Update Backend Export Schemas

**Files:**
- Modify: `backend/app/schemas/export.py`

**Step 1: Remove ExportScope enum and update NexoriousExportData**

```python
"""
Pydantic schemas for collection export API.

Provides request/response models for exporting user collections to JSON and CSV formats.
Export jobs are tracked using the unified Job model.
"""

from pydantic import BaseModel, Field, ConfigDict
from typing import Optional, List, Dict, Any
from datetime import datetime, date
from enum import Enum


class ExportFormat(str, Enum):
    """Supported export formats."""

    JSON = "json"
    CSV = "csv"


class ExportJobCreatedResponse(BaseModel):
    """Response when an export job is created."""

    model_config = ConfigDict(from_attributes=True)

    job_id: str = Field(..., description="ID of the created export job")
    status: str = Field(default="pending", description="Initial job status")
    message: str = Field(..., description="Success message")
    estimated_items: int = Field(default=0, description="Estimated number of items to export")


class ExportDownloadResponse(BaseModel):
    """Response for export download metadata."""

    model_config = ConfigDict(from_attributes=True)

    job_id: str
    status: str
    file_path: Optional[str] = None
    download_url: Optional[str] = None
    file_size: Optional[int] = None
    format: ExportFormat
    created_at: datetime
    completed_at: Optional[datetime] = None
    expires_at: Optional[datetime] = None


# Export data schemas (for JSON exports)


class ExportPlatformData(BaseModel):
    """Platform data in export format."""

    platform_id: Optional[str] = None
    platform_name: Optional[str] = None
    storefront_id: Optional[str] = None
    storefront_name: Optional[str] = None
    store_game_id: Optional[str] = None
    store_url: Optional[str] = None
    is_available: bool = True


class ExportTagData(BaseModel):
    """Tag data in export format."""

    name: str
    color: Optional[str] = None


class ExportGameData(BaseModel):
    """Game data in export format (for JSON exports)."""

    # IGDB data
    igdb_id: int = Field(..., description="IGDB game ID for reliable re-import")
    title: str
    release_year: Optional[int] = None

    # User data
    ownership_status: str
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None
    acquired_date: Optional[date] = None

    # Relationships
    platforms: List[ExportPlatformData] = Field(default_factory=list)
    tags: List[ExportTagData] = Field(default_factory=list)

    # Timestamps
    created_at: datetime
    updated_at: datetime


class NexoriousExportData(BaseModel):
    """Complete Nexorious JSON export format."""

    export_version: str = Field(default="1.1", description="Export format version")
    export_date: datetime = Field(..., description="When the export was created")
    user_id: str = Field(..., description="User ID (for reference only)")

    # Statistics
    total_games: int
    export_stats: Dict[str, Any] = Field(
        default_factory=dict, description="Summary statistics about the export"
    )

    # Game data
    games: List[ExportGameData]


# CSV export row schema


class CsvExportRow(BaseModel):
    """Single row for CSV export."""

    igdb_id: int
    title: str
    release_year: Optional[int] = None
    ownership_status: str
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None
    acquired_date: Optional[str] = None
    platforms: str = ""  # Comma-separated platform names
    storefronts: str = ""  # Comma-separated storefront names
    tags: str = ""  # Comma-separated tag names
    created_at: str
    updated_at: str
```

**Step 2: Run type check to verify schema is valid**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/schemas/export.py`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/app/schemas/export.py
git commit -m "refactor: remove ExportScope from export schemas"
```

---

## Task 2: Update Backend Export Task

**Files:**
- Modify: `backend/app/worker/tasks/import_export/export.py`

**Step 1: Simplify export task to remove scope handling**

Update the imports to remove ExportScope:

```python
from app.schemas.export import (
    ExportFormat,
    ExportGameData,
    ExportPlatformData,
    ExportTagData,
    NexoriousExportData,
    CsvExportRow,
)
```

**Step 2: Simplify `_build_user_games_query` function**

Replace the function (around lines 45-62):

```python
def _build_user_games_query(
    session: Session,
    user_id: str,
) -> List[UserGame]:
    """Build and execute query for all user games."""
    query = (
        select(UserGame)
        .where(UserGame.user_id == user_id)
        .order_by(UserGame.created_at)  # pyrefly: ignore[bad-argument-type]
    )

    return list(session.exec(query).all())
```

**Step 3: Update task signature and body**

Update the `export_collection` task (starting around line 192):

```python
@broker.task(
    task_name="export.collection",
    queue=QUEUE_HIGH,
)
async def export_collection(
    job_id: str,
    export_format: str,
) -> Dict[str, Any]:
    """
    Export user's complete game collection.

    Creates a JSON or CSV file containing all of the user's games with
    associated metadata. The file is stored in the exports directory
    and the job's file_path is updated to point to it.

    Args:
        job_id: The Job ID for tracking progress
        export_format: "json" or "csv"

    Returns:
        Dictionary with export statistics.
    """
    logger.info(
        f"Starting export (job: {job_id}, format: {export_format})"
    )

    stats: Dict[str, Any] = {
        "exported_games": 0,
        "file_size_bytes": 0,
        "format": export_format,
    }

    async with get_session_context() as session:
        # Try to acquire advisory lock - prevents duplicate execution
        if not acquire_job_lock(session, job_id):
            logger.info(f"Job {job_id} already being processed by another worker")
            return {"status": "skipped", "reason": "duplicate_execution"}

        # Get job first (outside try so exception handler can access it)
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            release_job_lock(session, job_id)
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status

            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            # Query user games
            user_games = _build_user_games_query(session, job.user_id)
            total_games = len(user_games)

            job.progress_total = total_games
            session.add(job)
            session.commit()

            # Generate filename
            exports_dir = _get_exports_dir()
            timestamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%S")
            extension = "csv" if export_format == ExportFormat.CSV.value else "json"
            filename = f"{job.user_id}_{timestamp}.{extension}"
            file_path = exports_dir / filename

            if export_format == ExportFormat.CSV.value:
                # CSV export
                csv_rows: List[CsvExportRow] = []
                for i, user_game in enumerate(user_games):
                    csv_row = _user_game_to_csv_row(session, user_game)
                    csv_rows.append(csv_row)

                    # Update progress
                    job.progress_current = i + 1
                    session.add(job)
                    if i % 50 == 0:  # Commit every 50 items
                        session.commit()

                session.commit()
                file_size = _write_csv_export(csv_rows, file_path)

            else:
                # JSON export
                games_data: List[ExportGameData] = []
                for i, user_game in enumerate(user_games):
                    game_data = _user_game_to_export_data(session, user_game)
                    games_data.append(game_data)

                    # Update progress
                    job.progress_current = i + 1
                    session.add(job)
                    if i % 50 == 0:  # Commit every 50 items
                        session.commit()

                session.commit()

                # Calculate stats for export
                export_stats = _calculate_export_stats(games_data)

                # Build full export data
                export_data = NexoriousExportData(
                    export_version=EXPORT_VERSION,
                    export_date=datetime.now(timezone.utc),
                    user_id=job.user_id,
                    total_games=total_games,
                    export_stats=export_stats,
                    games=games_data,
                )

                file_size = _write_json_export(export_data, file_path)

            # Update job with results
            stats["exported_games"] = total_games
            stats["file_size_bytes"] = file_size

            result_summary = job.get_result_summary()
            result_summary.update(stats)
            job.set_result_summary(result_summary)

            job.file_path = str(file_path)
            job.status = BackgroundJobStatus.COMPLETED
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            logger.info(
                f"Export completed for job {job_id}: "
                f"{total_games} games, {file_size} bytes"
            )

            return stats

        except Exception as e:
            logger.error(f"Export failed for job {job_id}: {e}", exc_info=True)

            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            return {
                "status": "error",
                "error": str(e),
                **stats,
            }
        finally:
            release_job_lock(session, job_id)
```

**Step 4: Update EXPORT_VERSION constant**

Change line 35 from `"1.0"` to `"1.1"`:

```python
EXPORT_VERSION = "1.1"
```

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/worker/tasks/import_export/export.py`
Expected: No errors

**Step 6: Commit**

```bash
git add backend/app/worker/tasks/import_export/export.py
git commit -m "refactor: simplify export task to export all games"
```

---

## Task 3: Update Backend Export Endpoints

**Files:**
- Modify: `backend/app/api/export_endpoints.py`

**Step 1: Replace the 4 endpoints with 2 unified endpoints**

```python
"""
Export API endpoints for collection data export.

Provides endpoints for exporting user collections to JSON and CSV formats.
Exports are tracked using the unified Job model and files are stored
temporarily for download.
"""

from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.responses import FileResponse
from sqlmodel import Session, select, func
from typing import Annotated
from datetime import datetime, timezone, timedelta
from pathlib import Path
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..core.config import settings
from ..models.user import User
from ..models.user_game import UserGame
from ..models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus
from ..schemas.export import ExportFormat, ExportJobCreatedResponse
from ..worker.tasks.import_export.export import export_collection as export_collection_task

router = APIRouter(prefix="/export", tags=["Export"])
logger = logging.getLogger(__name__)

# Export file retention (24 hours)
EXPORT_RETENTION_HOURS = 24


def _get_exports_dir() -> Path:
    """Get exports directory from settings, creating it if needed."""
    exports_dir = Path(getattr(settings, "storage_path", "storage")) / "exports"
    exports_dir.mkdir(parents=True, exist_ok=True)
    return exports_dir


@router.post("/json", response_model=ExportJobCreatedResponse)
async def export_json(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ExportJobCreatedResponse:
    """
    Start a JSON export of all user games.

    Creates a background job that exports all games to a JSON file
    with full metadata. The export includes:
    - IGDB IDs for reliable re-import
    - All user data (ratings, notes, play status, ownership status, etc.)
    - Platform and storefront associations
    - Tags

    The exported file can be downloaded once the job completes.
    Files are retained for 24 hours before automatic deletion.
    """
    logger.info(f"User {current_user.id} requesting JSON export")

    # Count games to estimate export size
    game_count = session.exec(
        select(func.count()).select_from(UserGame).where(UserGame.user_id == current_user.id)
    ).one()

    if game_count == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No games to export.",
        )

    # Create export job
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.EXPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
        progress_total=game_count,
    )
    job.set_result_summary({
        "format": ExportFormat.JSON.value,
        "estimated_items": game_count,
    })

    session.add(job)
    session.commit()
    session.refresh(job)

    # Schedule the export task
    await export_collection_task.kiq(
        job_id=job.id,
        export_format=ExportFormat.JSON.value,
    )

    logger.info(f"Created JSON export job {job.id} for user {current_user.id}")

    return ExportJobCreatedResponse(
        job_id=job.id,
        status=job.status.value,
        message="Export job created. Check job status for progress.",
        estimated_items=game_count,
    )


@router.post("/csv", response_model=ExportJobCreatedResponse)
async def export_csv(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ExportJobCreatedResponse:
    """
    Start a CSV export of all user games.

    Creates a background job that exports all games to a CSV file.
    The CSV format is useful for spreadsheet applications but has
    some limitations:
    - Platform/storefront data is comma-separated in columns
    - Personal notes may be truncated if very long
    - Not recommended for re-import (use JSON instead)

    The exported file can be downloaded once the job completes.
    Files are retained for 24 hours before automatic deletion.
    """
    logger.info(f"User {current_user.id} requesting CSV export")

    # Count games to estimate export size
    game_count = session.exec(
        select(func.count()).select_from(UserGame).where(UserGame.user_id == current_user.id)
    ).one()

    if game_count == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No games to export.",
        )

    # Create export job
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.EXPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
        progress_total=game_count,
    )
    job.set_result_summary({
        "format": ExportFormat.CSV.value,
        "estimated_items": game_count,
    })

    session.add(job)
    session.commit()
    session.refresh(job)

    # Schedule the export task
    await export_collection_task.kiq(
        job_id=job.id,
        export_format=ExportFormat.CSV.value,
    )

    logger.info(f"Created CSV export job {job.id} for user {current_user.id}")

    return ExportJobCreatedResponse(
        job_id=job.id,
        status=job.status.value,
        message="Export job created. Check job status for progress.",
        estimated_items=game_count,
    )


@router.get("/{job_id}/download")
async def download_export(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> FileResponse:
    """
    Download a completed export file.

    Returns the exported file for download. Only available for completed
    export jobs that belong to the current user. Files are available
    for 24 hours after creation.
    """
    logger.debug(f"User {current_user.id} downloading export {job_id}")

    job = session.get(Job, job_id)

    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Export job not found.",
        )

    # Authorization check
    if job.user_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Export job not found.",
        )

    # Check job type
    if job.job_type != BackgroundJobType.EXPORT:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Job is not an export job.",
        )

    # Check job status
    if job.status != BackgroundJobStatus.COMPLETED:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Export not ready. Current status: {job.status.value}",
        )

    # Check file path
    if not job.file_path:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Export file path not set.",
        )

    file_path = Path(job.file_path)
    if not file_path.exists():
        raise HTTPException(
            status_code=status.HTTP_410_GONE,
            detail="Export file has expired or been deleted.",
        )

    # Check expiration (24 hours from completion)
    if job.completed_at:
        # Ensure completed_at is timezone-aware for comparison
        completed_at = job.completed_at
        if completed_at.tzinfo is None:
            completed_at = completed_at.replace(tzinfo=timezone.utc)
        expiration = completed_at + timedelta(hours=EXPORT_RETENTION_HOURS)
        if datetime.now(timezone.utc) > expiration:
            # Delete the file
            file_path.unlink(missing_ok=True)
            raise HTTPException(
                status_code=status.HTTP_410_GONE,
                detail="Export file has expired.",
            )

    # Determine content type and filename
    result_summary = job.get_result_summary()
    export_format = result_summary.get("format", "json")

    if export_format == "csv":
        media_type = "text/csv"
        filename = f"nexorious_export_{job.created_at.strftime('%Y%m%d_%H%M%S')}.csv"
    else:
        media_type = "application/json"
        filename = f"nexorious_export_{job.created_at.strftime('%Y%m%d_%H%M%S')}.json"

    logger.info(f"User {current_user.id} downloading export {job_id}: {filename}")

    return FileResponse(
        path=str(file_path),
        media_type=media_type,
        filename=filename,
    )
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/api/export_endpoints.py`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/app/api/export_endpoints.py
git commit -m "refactor: consolidate export endpoints to /export/json and /export/csv"
```

---

## Task 4: Update Backend Export Tests

**Files:**
- Modify: `backend/app/tests/test_export_endpoints.py`
- Modify: `backend/app/tests/test_export_tasks.py`

**Step 1: Read current test files to understand structure**

Run: `cat backend/app/tests/test_export_endpoints.py`
Run: `cat backend/app/tests/test_export_tasks.py`

**Step 2: Update test_export_endpoints.py**

Update tests to use new endpoints `/export/json` and `/export/csv` instead of `/export/collection/json`, `/export/collection/csv`, `/export/wishlist/json`, `/export/wishlist/csv`.

Remove any tests specific to wishlist exports. Update assertions that check for `scope` in result summaries.

**Step 3: Update test_export_tasks.py**

Update task invocations to remove the `export_scope` parameter. Update assertions that check for scope in export data.

**Step 4: Run the tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_export_endpoints.py app/tests/test_export_tasks.py -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add backend/app/tests/test_export_endpoints.py backend/app/tests/test_export_tasks.py
git commit -m "test: update export tests for unified export endpoints"
```

---

## Task 5: Update Darkadia Import Script

**Files:**
- Modify: `backend/scripts/darkadia_to_nexorious.py`

**Step 1: Update generate_nexorious_json function**

Find the `generate_nexorious_json` function (around line 700) and update the return statement:

```python
    return {
        "export_version": "1.1",
        "export_date": now.isoformat(),
        "user_id": user_id,
        "total_games": len(exported_games),
        "export_stats": stats,
        "games": exported_games,
    }
```

Remove the `"export_scope": "collection",` line.

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check scripts/darkadia_to_nexorious.py`
Expected: No errors (or only pre-existing issues)

**Step 3: Commit**

```bash
git add backend/scripts/darkadia_to_nexorious.py
git commit -m "refactor: update darkadia script to export v1.1 format"
```

---

## Task 6: Update Frontend Types

**Files:**
- Modify: `frontend/src/types/import-export.ts`

**Step 1: Read current types file**

Run: `cat frontend/src/types/import-export.ts`

**Step 2: Remove ExportScope if present, keep ExportFormat**

The types should only have `ExportFormat` (JSON/CSV), no scope-related types.

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Pass (or errors in files we haven't updated yet)

**Step 4: Commit**

```bash
git add frontend/src/types/import-export.ts
git commit -m "refactor: remove ExportScope from frontend types"
```

---

## Task 7: Update Frontend API

**Files:**
- Modify: `frontend/src/api/import-export.ts`

**Step 1: Read current API file**

Run: `cat frontend/src/api/import-export.ts`

**Step 2: Update export functions**

- Change `exportCollection` to call `/export/json` or `/export/csv` based on format
- Remove `exportWishlist` functions entirely
- Rename function to `exportAll` or keep as `exportCollection` (simpler)

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Errors in hooks and page that still reference old functions

**Step 4: Commit**

```bash
git add frontend/src/api/import-export.ts
git commit -m "refactor: update export API to use unified endpoints"
```

---

## Task 8: Update Frontend Hooks

**Files:**
- Modify: `frontend/src/hooks/use-import-export.ts`
- Modify: `frontend/src/hooks/index.ts`

**Step 1: Read current hooks file**

Run: `cat frontend/src/hooks/use-import-export.ts`

**Step 2: Remove useExportWishlist hook**

Keep only `useExportCollection` (or rename to `useExport`).

**Step 3: Update index.ts exports**

Remove `useExportWishlist` from exports.

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Errors in page that still references old hook

**Step 5: Commit**

```bash
git add frontend/src/hooks/use-import-export.ts frontend/src/hooks/index.ts
git commit -m "refactor: remove useExportWishlist hook"
```

---

## Task 9: Update Frontend Import/Export Page

**Files:**
- Modify: `frontend/src/app/(main)/import-export/page.tsx`

**Step 1: Simplify export UI**

- Remove the separate "Export Collection" and "Export Wishlist" sections
- Create a single "Export" section with JSON and CSV buttons
- Remove `exportingWishlistFormat` state
- Remove `handleWishlistExport` function
- Update imports to remove `useExportWishlist`

**Step 2: Run type check and build**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Pass

**Step 3: Run frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: Pass (may need to update tests if any exist for this page)

**Step 4: Commit**

```bash
git add frontend/src/app/\(main\)/import-export/page.tsx
git commit -m "refactor: consolidate export UI into single section"
```

---

## Task 10: Update Frontend Hook Tests

**Files:**
- Modify: `frontend/src/hooks/use-import-export.test.ts`
- Modify: `frontend/src/api/import-export.test.ts`

**Step 1: Read current test files**

Run: `cat frontend/src/hooks/use-import-export.test.ts`
Run: `cat frontend/src/api/import-export.test.ts`

**Step 2: Remove tests for wishlist export**

Update tests to only test the unified export functionality.

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/hooks/use-import-export.test.ts frontend/src/api/import-export.test.ts
git commit -m "test: update export tests for unified export"
```

---

## Task 11: Final Verification

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass, >80% coverage

**Step 2: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: All tests pass

**Step 3: Run type checks**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: No type errors

**Step 4: Manual smoke test (optional)**

Start the app and verify:
1. Export JSON works and produces a file without `export_scope`
2. Export CSV works
3. Import of old v1.0 files still works
4. Import of new v1.1 files works

**Step 5: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: address any issues found during verification"
```
