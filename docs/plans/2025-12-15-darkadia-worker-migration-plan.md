# Darkadia Worker Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate Darkadia CSV import from legacy staging tables to worker-based Job/ReviewItem system.

**Architecture:** CSV upload → API validates & stores parsed rows in job → Worker processes & creates ReviewItems → User reviews with platform mapping → Finalization creates UserGame records.

**Tech Stack:** FastAPI, Taskiq worker, SQLModel, SvelteKit, Svelte 5 runes

**Design Document:** `docs/plans/2025-12-15-darkadia-worker-migration-design.md`

---

## Phase 1: Backend - Worker Task Improvements

### Task 1.1: Add Platform String Parsing to Worker

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_darkadia.py`
- Test: `backend/app/tests/test_import_tasks.py`

**Step 1: Write failing test for platform parsing**

Add to `backend/app/tests/test_import_tasks.py`:

```python
import pytest
from app.worker.tasks.import_export.import_darkadia import parse_darkadia_platform


class TestParseDarkadiaPlatform:
    """Tests for Darkadia platform string parsing."""

    def test_parse_full_platform_string(self):
        """Parse platform with all components."""
        result = parse_darkadia_platform("PC|Steam|Digital")
        assert result == {
            "platform": "PC",
            "storefront": "Steam",
            "media_type": "Digital",
        }

    def test_parse_platform_only(self):
        """Parse platform with no storefront or media type."""
        result = parse_darkadia_platform("PlayStation 4")
        assert result == {
            "platform": "PlayStation 4",
            "storefront": None,
            "media_type": None,
        }

    def test_parse_platform_and_storefront(self):
        """Parse platform with storefront but no media type."""
        result = parse_darkadia_platform("PC|GOG")
        assert result == {
            "platform": "PC",
            "storefront": "GOG",
            "media_type": None,
        }

    def test_parse_empty_string(self):
        """Handle empty string."""
        result = parse_darkadia_platform("")
        assert result == {
            "platform": None,
            "storefront": None,
            "media_type": None,
        }

    def test_parse_whitespace_trimming(self):
        """Trim whitespace from components."""
        result = parse_darkadia_platform(" PC | Steam | Digital ")
        assert result == {
            "platform": "PC",
            "storefront": "Steam",
            "media_type": "Digital",
        }
```

**Step 2: Run test to verify it fails**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_import_tasks.py::TestParseDarkadiaPlatform -v
```

Expected: FAIL with "cannot import name 'parse_darkadia_platform'"

**Step 3: Implement platform parsing function**

Add to `backend/app/worker/tasks/import_export/import_darkadia.py` after the COLUMN_MAPPINGS dict (~line 63):

```python
def parse_darkadia_platform(platform_str: str) -> Dict[str, Optional[str]]:
    """
    Parse Darkadia platform string into components.

    Darkadia format: "Platform|Storefront|MediaType"
    Examples:
        "PC|Steam|Digital" -> {"platform": "PC", "storefront": "Steam", "media_type": "Digital"}
        "PlayStation 4" -> {"platform": "PlayStation 4", "storefront": None, "media_type": None}

    Args:
        platform_str: Raw platform string from CSV

    Returns:
        Dict with platform, storefront, and media_type keys
    """
    if not platform_str or not platform_str.strip():
        return {"platform": None, "storefront": None, "media_type": None}

    parts = [p.strip() for p in platform_str.split("|")]

    return {
        "platform": parts[0] if len(parts) > 0 and parts[0] else None,
        "storefront": parts[1] if len(parts) > 1 and parts[1] else None,
        "media_type": parts[2] if len(parts) > 2 and parts[2] else None,
    }
```

**Step 4: Run test to verify it passes**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_import_tasks.py::TestParseDarkadiaPlatform -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/worker/tasks/import_export/import_darkadia.py backend/app/tests/test_import_tasks.py
git commit -m "feat(worker): add Darkadia platform string parsing"
```

---

### Task 1.2: Update Worker to Store Platform/Storefront Arrays

**Files:**
- Modify: `backend/app/worker/tasks/import_export/import_darkadia.py`
- Test: `backend/app/tests/test_import_tasks.py`

**Step 1: Write failing test for source_metadata structure**

Add to `backend/app/tests/test_import_tasks.py`:

```python
def test_process_darkadia_row_stores_platform_arrays():
    """Verify source_metadata contains platforms and storefronts arrays."""
    # This test requires mocking - add after integration test setup exists
    # For now, we'll verify the structure in _process_darkadia_row
    pass
```

**Step 2: Update _process_darkadia_row to parse platforms**

In `backend/app/worker/tasks/import_export/import_darkadia.py`, modify the `_process_darkadia_row` function.

Find this section (~line 259-274):

```python
    # Get optional fields for matching hints
    platform = _get_row_value(row, column_map, "platform")
    release_year = _get_row_value(row, column_map, "release_year")

    # Build source metadata
    source_metadata = {
        "platform": platform,
        "release_year": release_year,
```

Replace with:

```python
    # Get optional fields for matching hints
    platform_raw = _get_row_value(row, column_map, "platform")
    release_year = _get_row_value(row, column_map, "release_year")

    # Parse platform string into components
    platforms: List[str] = []
    storefronts: List[str] = []
    if platform_raw:
        parsed = parse_darkadia_platform(platform_raw)
        if parsed["platform"]:
            platforms.append(parsed["platform"])
        if parsed["storefront"]:
            storefronts.append(parsed["storefront"])

    # Build source metadata
    source_metadata = {
        "source": "darkadia",
        "platforms": platforms,
        "storefronts": storefronts,
        "platform_raw": platform_raw,  # Keep original for reference
        "release_year": release_year,
```

Also update the imports at the top of the file to include `List`:

```python
from typing import Dict, Any, List, Optional
```

**Step 3: Run full test suite for import tasks**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_import_tasks.py -v
```

Expected: All tests PASS

**Step 4: Commit**

```bash
git add backend/app/worker/tasks/import_export/import_darkadia.py
git commit -m "feat(worker): store platform/storefront arrays in ReviewItem metadata"
```

---

## Phase 2: Backend - New API Endpoints

### Task 2.1: Add Platform Summary Endpoint

**Files:**
- Modify: `backend/app/api/review.py`
- Modify: `backend/app/schemas/review.py`
- Test: `backend/app/tests/test_review_api.py`

**Step 1: Add schema for platform summary response**

Add to `backend/app/schemas/review.py` (after ReviewCountsByType class):

```python
class PlatformMappingSuggestion(BaseModel):
    """A platform or storefront string that needs mapping."""

    original: str = Field(..., description="Original string from CSV")
    count: int = Field(..., description="Number of games with this value")
    suggested_id: Optional[str] = Field(None, description="Suggested Platform/Storefront ID")
    suggested_name: Optional[str] = Field(None, description="Suggested Platform/Storefront name")


class PlatformSummaryResponse(BaseModel):
    """Summary of platform/storefront strings needing mapping for a job."""

    platforms: List[PlatformMappingSuggestion] = Field(default_factory=list)
    storefronts: List[PlatformMappingSuggestion] = Field(default_factory=list)
    all_resolved: bool = Field(..., description="True if all strings have suggestions")
```

**Step 2: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

Expected: PASS

**Step 3: Write failing test for platform summary endpoint**

Add to `backend/app/tests/test_review_api.py`:

```python
@pytest.mark.asyncio
async def test_get_platform_summary_empty_job(
    async_client: AsyncClient,
    auth_headers: dict,
    test_user: User,
    session: Session,
):
    """Test platform summary returns empty for job with no platform data."""
    # Create a job
    job = Job(
        user_id=test_user.id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        status=BackgroundJobStatus.AWAITING_REVIEW,
    )
    session.add(job)
    session.commit()

    response = await async_client.get(
        f"/api/review/platform-summary?job_id={job.id}",
        headers=auth_headers,
    )

    assert response.status_code == 200
    data = response.json()
    assert data["platforms"] == []
    assert data["storefronts"] == []
    assert data["all_resolved"] is True
```

**Step 4: Run test to verify it fails**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_review_api.py::test_get_platform_summary_empty_job -v
```

Expected: FAIL with 404 (endpoint doesn't exist)

**Step 5: Implement platform summary endpoint**

Add to `backend/app/api/review.py` (before the `get_review_item` endpoint):

```python
@router.get("/platform-summary", response_model=PlatformSummaryResponse)
async def get_platform_summary(
    job_id: str = Query(..., description="Job ID to get platform summary for"),
    session: Annotated[Session, Depends(get_session)] = None,
    current_user: Annotated[User, Depends(get_current_user)] = None,
):
    """
    Get summary of platform/storefront strings needing mapping for a job.

    Extracts unique platform and storefront strings from all ReviewItems
    for the given job, counts occurrences, and suggests matches to
    existing Platform/Storefront entities.
    """
    from collections import Counter
    from ..models.platform import Platform, Storefront

    # Verify job exists and belongs to user
    job = session.get(Job, job_id)
    if not job:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )
    if job.user_id != current_user.id:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Get all review items for this job
    items = session.exec(
        select(ReviewItem).where(ReviewItem.job_id == job_id)
    ).all()

    # Extract and count platform/storefront strings
    platform_counts: Counter = Counter()
    storefront_counts: Counter = Counter()

    for item in items:
        metadata = item.get_source_metadata()
        for p in metadata.get("platforms", []):
            if p:
                platform_counts[p] += 1
        for s in metadata.get("storefronts", []):
            if s:
                storefront_counts[s] += 1

    # Get all platforms and storefronts for matching
    all_platforms = session.exec(select(Platform).where(Platform.is_active == True)).all()
    all_storefronts = session.exec(select(Storefront).where(Storefront.is_active == True)).all()

    # Build platform suggestions
    platform_suggestions = []
    for original, count in platform_counts.items():
        suggested = _find_best_match(original, [(p.id, p.name, p.display_name) for p in all_platforms])
        platform_suggestions.append(PlatformMappingSuggestion(
            original=original,
            count=count,
            suggested_id=suggested[0] if suggested else None,
            suggested_name=suggested[1] if suggested else None,
        ))

    # Build storefront suggestions
    storefront_suggestions = []
    for original, count in storefront_counts.items():
        suggested = _find_best_match(original, [(s.id, s.name, s.display_name) for s in all_storefronts])
        storefront_suggestions.append(PlatformMappingSuggestion(
            original=original,
            count=count,
            suggested_id=suggested[0] if suggested else None,
            suggested_name=suggested[1] if suggested else None,
        ))

    # Sort by count descending
    platform_suggestions.sort(key=lambda x: x.count, reverse=True)
    storefront_suggestions.sort(key=lambda x: x.count, reverse=True)

    # Check if all resolved
    all_resolved = (
        all(p.suggested_id for p in platform_suggestions) and
        all(s.suggested_id for s in storefront_suggestions)
    )

    return PlatformSummaryResponse(
        platforms=platform_suggestions,
        storefronts=storefront_suggestions,
        all_resolved=all_resolved,
    )


def _find_best_match(
    original: str,
    candidates: list[tuple[str, str, str]],  # (id, name, display_name)
) -> Optional[tuple[str, str]]:
    """
    Find best matching platform/storefront for a string.

    Uses case-insensitive exact matching on name or display_name.
    Returns (id, display_name) or None.
    """
    original_lower = original.lower().strip()

    for id_, name, display_name in candidates:
        if name.lower() == original_lower or display_name.lower() == original_lower:
            return (id_, display_name)

    # Try partial matching for common abbreviations
    abbrev_map = {
        "ps4": "playstation 4",
        "ps5": "playstation 5",
        "ps3": "playstation 3",
        "xb1": "xbox one",
        "xsx": "xbox series x",
        "nsw": "nintendo switch",
        "psn": "playstation store",
        "xbl": "xbox live",
    }

    if original_lower in abbrev_map:
        expanded = abbrev_map[original_lower]
        for id_, name, display_name in candidates:
            if name.lower() == expanded or display_name.lower() == expanded:
                return (id_, display_name)

    return None
```

Also add the import for the schema at the top of the file:

```python
from ..schemas.review import (
    ReviewItemResponse,
    ReviewItemDetailResponse,
    ReviewListResponse,
    MatchRequest,
    MatchResponse,
    ReviewSummary,
    ReviewCountsByType,
    ReviewItemStatus,
    ReviewSource,
    IGDBCandidate,
    PlatformMappingSuggestion,
    PlatformSummaryResponse,
)
```

**Step 6: Run test to verify it passes**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_review_api.py::test_get_platform_summary_empty_job -v
```

Expected: PASS

**Step 7: Commit**

```bash
git add backend/app/api/review.py backend/app/schemas/review.py backend/app/tests/test_review_api.py
git commit -m "feat(api): add platform summary endpoint for Darkadia import"
```

---

### Task 2.2: Add Finalization Endpoint

**Files:**
- Modify: `backend/app/api/review.py`
- Modify: `backend/app/schemas/review.py`
- Test: `backend/app/tests/test_review_api.py`

**Step 1: Add schemas for finalization**

Add to `backend/app/schemas/review.py`:

```python
class FinalizeImportRequest(BaseModel):
    """Request to finalize an import job."""

    job_id: str = Field(..., description="Job ID to finalize")
    platform_mappings: Dict[str, str] = Field(
        default_factory=dict,
        description="Map of original platform strings to Platform IDs"
    )
    storefront_mappings: Dict[str, str] = Field(
        default_factory=dict,
        description="Map of original storefront strings to Storefront IDs"
    )


class FinalizeImportResponse(BaseModel):
    """Response from finalizing an import job."""

    success: bool
    message: str
    games_imported: int = 0
    games_skipped: int = 0
    errors: List[str] = Field(default_factory=list)
```

**Step 2: Write failing test for finalization**

Add to `backend/app/tests/test_review_api.py`:

```python
@pytest.mark.asyncio
async def test_finalize_import_empty_job(
    async_client: AsyncClient,
    auth_headers: dict,
    test_user: User,
    session: Session,
):
    """Test finalize returns success with zero imports for job with no matched items."""
    # Create a job in AWAITING_REVIEW status
    job = Job(
        user_id=test_user.id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        status=BackgroundJobStatus.AWAITING_REVIEW,
    )
    session.add(job)
    session.commit()

    response = await async_client.post(
        "/api/review/finalize",
        headers=auth_headers,
        json={
            "job_id": job.id,
            "platform_mappings": {},
            "storefront_mappings": {},
        },
    )

    assert response.status_code == 200
    data = response.json()
    assert data["success"] is True
    assert data["games_imported"] == 0
    assert data["games_skipped"] == 0
```

**Step 3: Run test to verify it fails**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_review_api.py::test_finalize_import_empty_job -v
```

Expected: FAIL with 404 or 405 (endpoint doesn't exist)

**Step 4: Implement finalization endpoint**

Add to `backend/app/api/review.py` (at the end of the file, before the helper functions):

```python
@router.post("/finalize", response_model=FinalizeImportResponse)
async def finalize_import(
    request: FinalizeImportRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Finalize an import job by creating UserGame records for matched items.

    Processes all ReviewItems with MATCHED status:
    1. Creates Game records from IGDB if needed
    2. Creates UserGame records
    3. Creates UserGamePlatform records using provided mappings
    4. Marks job as COMPLETED
    """
    from ..models.platform import Platform, Storefront

    logger.info(f"User {current_user.id} finalizing import job {request.job_id}")

    # Verify job exists and belongs to user
    job = session.get(Job, request.job_id)
    if not job:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )
    if job.user_id != current_user.id:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Verify job is in correct status
    if job.status not in [BackgroundJobStatus.AWAITING_REVIEW, BackgroundJobStatus.READY]:
        raise HTTPException(
            status_code=http_status.HTTP_400_BAD_REQUEST,
            detail=f"Job cannot be finalized in status: {job.status.value}",
        )

    # Validate platform mappings reference real platforms
    for platform_id in request.platform_mappings.values():
        if platform_id and not session.get(Platform, platform_id):
            raise HTTPException(
                status_code=http_status.HTTP_400_BAD_REQUEST,
                detail=f"Invalid platform ID: {platform_id}",
            )

    # Validate storefront mappings reference real storefronts
    for storefront_id in request.storefront_mappings.values():
        if storefront_id and not session.get(Storefront, storefront_id):
            raise HTTPException(
                status_code=http_status.HTTP_400_BAD_REQUEST,
                detail=f"Invalid storefront ID: {storefront_id}",
            )

    # Get all matched review items for this job
    matched_items = session.exec(
        select(ReviewItem).where(
            ReviewItem.job_id == request.job_id,
            ReviewItem.status == ModelReviewItemStatus.MATCHED,
        )
    ).all()

    # Count skipped items
    skipped_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.job_id == request.job_id,
            ReviewItem.status == ModelReviewItemStatus.SKIPPED,
        )
    ).one()

    games_imported = 0
    errors = []

    # Create services
    igdb_service = IGDBService()
    game_service = GameService(session, igdb_service)

    for item in matched_items:
        if not item.resolved_igdb_id:
            errors.append(f"Item {item.id} ({item.source_title}) has no resolved IGDB ID")
            continue

        try:
            # Ensure game exists in our database
            game = session.get(Game, item.resolved_igdb_id)
            if not game:
                game = await game_service.create_or_update_game_from_igdb(
                    item.resolved_igdb_id, download_cover_art=True
                )

            # Check if user already has this game
            existing_user_game = session.exec(
                select(UserGame).where(
                    UserGame.user_id == current_user.id,
                    UserGame.game_id == item.resolved_igdb_id,
                )
            ).first()

            if existing_user_game:
                user_game = existing_user_game
                logger.debug(f"User already has game {item.resolved_igdb_id}, adding platforms")
            else:
                # Create UserGame
                user_game = UserGame(
                    user_id=current_user.id,
                    game_id=item.resolved_igdb_id,
                )
                session.add(user_game)
                session.commit()
                session.refresh(user_game)

            # Create UserGamePlatform entries
            metadata = item.get_source_metadata()
            platforms = metadata.get("platforms", [])
            storefronts = metadata.get("storefronts", [])

            for i, platform_str in enumerate(platforms):
                platform_id = request.platform_mappings.get(platform_str)
                storefront_str = storefronts[i] if i < len(storefronts) else None
                storefront_id = request.storefront_mappings.get(storefront_str) if storefront_str else None

                if platform_id:
                    # Check if association already exists
                    existing_platform = session.exec(
                        select(UserGamePlatform).where(
                            UserGamePlatform.user_game_id == user_game.id,
                            UserGamePlatform.platform_id == platform_id,
                            UserGamePlatform.storefront_id == storefront_id if storefront_id else UserGamePlatform.storefront_id.is_(None),
                        )
                    ).first()

                    if not existing_platform:
                        platform_entry = UserGamePlatform(
                            user_game_id=user_game.id,
                            platform_id=platform_id,
                            storefront_id=storefront_id,
                            is_available=True,
                        )
                        session.add(platform_entry)

            games_imported += 1

        except Exception as e:
            logger.error(f"Error importing game {item.source_title}: {e}", exc_info=True)
            errors.append(f"Failed to import {item.source_title}: {str(e)}")

    # Mark job as completed
    job.status = BackgroundJobStatus.COMPLETED
    job.completed_at = datetime.now(timezone.utc)
    session.add(job)
    session.commit()

    logger.info(
        f"Finalized import job {request.job_id}: "
        f"{games_imported} imported, {skipped_count} skipped, {len(errors)} errors"
    )

    return FinalizeImportResponse(
        success=True,
        message=f"Import finalized: {games_imported} games imported",
        games_imported=games_imported,
        games_skipped=skipped_count,
        errors=errors,
    )
```

Add the import for the new schemas:

```python
from ..schemas.review import (
    # ... existing imports ...
    FinalizeImportRequest,
    FinalizeImportResponse,
)
```

**Step 5: Run test to verify it passes**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_review_api.py::test_finalize_import_empty_job -v
```

Expected: PASS

**Step 6: Run full review API tests**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_review_api.py -v
```

Expected: All tests PASS

**Step 7: Commit**

```bash
git add backend/app/api/review.py backend/app/schemas/review.py backend/app/tests/test_review_api.py
git commit -m "feat(api): add finalization endpoint for Darkadia import"
```

---

## Phase 3: Frontend - Upload Page Rewrite

### Task 3.1: Rewrite Darkadia Upload Page

**Files:**
- Modify: `frontend/src/routes/import/darkadia/+page.svelte`

**Step 1: Backup existing page (for reference during deletion)**

```bash
cp frontend/src/routes/import/darkadia/+page.svelte frontend/src/routes/import/darkadia/+page.svelte.legacy
```

**Step 2: Rewrite the page**

Replace `frontend/src/routes/import/darkadia/+page.svelte` with:

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import { config } from '$lib/env';
	import { api } from '$lib/services/api';
	import { RouteGuard } from '$lib/components';

	let fileInput = $state<HTMLInputElement | null>(null);
	let selectedFile = $state<File | null>(null);
	let uploading = $state(false);
	let error = $state<string | null>(null);
	let dragOver = $state(false);

	function handleFileSelect(event: Event) {
		const input = event.target as HTMLInputElement;
		if (input.files && input.files[0]) {
			selectFile(input.files[0]);
		}
	}

	function handleDrop(event: DragEvent) {
		event.preventDefault();
		dragOver = false;
		if (event.dataTransfer?.files && event.dataTransfer.files[0]) {
			selectFile(event.dataTransfer.files[0]);
		}
	}

	function handleDragOver(event: DragEvent) {
		event.preventDefault();
		dragOver = true;
	}

	function handleDragLeave() {
		dragOver = false;
	}

	function selectFile(file: File) {
		error = null;
		if (!file.name.toLowerCase().endsWith('.csv')) {
			error = 'Please select a CSV file';
			return;
		}
		if (file.size > 10 * 1024 * 1024) {
			error = 'File is too large. Maximum size is 10MB.';
			return;
		}
		selectedFile = file;
	}

	function clearFile() {
		selectedFile = null;
		error = null;
		if (fileInput) {
			fileInput.value = '';
		}
	}

	async function handleUpload() {
		if (!selectedFile) return;

		uploading = true;
		error = null;

		try {
			const formData = new FormData();
			formData.append('file', selectedFile);

			const response = await api.post(`${config.apiUrl}/import/darkadia`, formData, {
				isFormData: true
			});

			if (!response.ok) {
				const data = await response.json();
				throw new Error(data.detail || 'Upload failed');
			}

			const data = await response.json();

			// Redirect to review page with job_id
			goto(`/review?job_id=${data.job_id}&source=import`);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Upload failed';
			uploading = false;
		}
	}
</script>

<svelte:head>
	<title>Import from Darkadia - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-2xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
		<div class="mb-8">
			<h1 class="text-2xl font-bold text-gray-900 dark:text-white">Import from Darkadia</h1>
			<p class="mt-2 text-gray-600 dark:text-gray-400">
				Upload your Darkadia CSV export to import your game collection.
			</p>
		</div>

		<!-- Upload Area -->
		<div
			class="border-2 border-dashed rounded-lg p-8 text-center transition-colors {dragOver
				? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/20'
				: 'border-gray-300 dark:border-gray-600 hover:border-gray-400 dark:hover:border-gray-500'}"
			ondrop={handleDrop}
			ondragover={handleDragOver}
			ondragleave={handleDragLeave}
			role="button"
			tabindex="0"
			onkeydown={(e) => e.key === 'Enter' && fileInput?.click()}
		>
			{#if selectedFile}
				<div class="space-y-4">
					<svg
						class="mx-auto h-12 w-12 text-green-500"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
						/>
					</svg>
					<div>
						<p class="text-lg font-medium text-gray-900 dark:text-white">
							{selectedFile.name}
						</p>
						<p class="text-sm text-gray-500 dark:text-gray-400">
							{(selectedFile.size / 1024).toFixed(1)} KB
						</p>
					</div>
					<button
						type="button"
						class="text-sm text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300"
						onclick={clearFile}
					>
						Remove file
					</button>
				</div>
			{:else}
				<svg
					class="mx-auto h-12 w-12 text-gray-400"
					fill="none"
					viewBox="0 0 24 24"
					stroke="currentColor"
				>
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"
					/>
				</svg>
				<div class="mt-4">
					<button
						type="button"
						class="text-indigo-600 dark:text-indigo-400 font-medium hover:text-indigo-800 dark:hover:text-indigo-300"
						onclick={() => fileInput?.click()}
					>
						Select a file
					</button>
					<span class="text-gray-500 dark:text-gray-400"> or drag and drop</span>
				</div>
				<p class="mt-2 text-sm text-gray-500 dark:text-gray-400">CSV file up to 10MB</p>
			{/if}
		</div>

		<input
			bind:this={fileInput}
			type="file"
			accept=".csv"
			class="hidden"
			onchange={handleFileSelect}
		/>

		<!-- Error Message -->
		{#if error}
			<div class="mt-4 p-4 bg-red-50 dark:bg-red-900/20 rounded-lg">
				<p class="text-sm text-red-700 dark:text-red-300">{error}</p>
			</div>
		{/if}

		<!-- Upload Button -->
		<div class="mt-6">
			<button
				type="button"
				class="w-full flex justify-center py-3 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
				disabled={!selectedFile || uploading}
				onclick={handleUpload}
			>
				{#if uploading}
					<svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" fill="none" viewBox="0 0 24 24">
						<circle
							class="opacity-25"
							cx="12"
							cy="12"
							r="10"
							stroke="currentColor"
							stroke-width="4"
						></circle>
						<path
							class="opacity-75"
							fill="currentColor"
							d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
						></path>
					</svg>
					Processing...
				{:else}
					Upload & Process
				{/if}
			</button>
		</div>

		<!-- Instructions -->
		<div class="mt-8 p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
			<h3 class="text-sm font-medium text-gray-900 dark:text-white">How to export from Darkadia</h3>
			<ol class="mt-2 text-sm text-gray-600 dark:text-gray-400 list-decimal list-inside space-y-1">
				<li>Open Darkadia and go to your collection</li>
				<li>Click on "Export" in the menu</li>
				<li>Select CSV format and download the file</li>
				<li>Upload the CSV file here</li>
			</ol>
		</div>
	</div>
</RouteGuard>
```

**Step 3: Run frontend type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/routes/import/darkadia/+page.svelte frontend/src/routes/import/darkadia/+page.svelte.legacy
git commit -m "feat(frontend): rewrite Darkadia upload page for worker flow"
```

---

## Phase 4: Frontend - Platform Mapping Component

### Task 4.1: Create Platform Mapping Section Component

**Files:**
- Create: `frontend/src/lib/components/PlatformMappingSection.svelte`

**Step 1: Create the component**

Create `frontend/src/lib/components/PlatformMappingSection.svelte`:

```svelte
<script lang="ts">
	import { platforms as platformsStore } from '$lib/stores';

	interface MappingSuggestion {
		original: string;
		count: number;
		suggested_id: string | null;
		suggested_name: string | null;
	}

	interface Props {
		platformSuggestions: MappingSuggestion[];
		storefrontSuggestions: MappingSuggestion[];
		platformMappings: Record<string, string>;
		storefrontMappings: Record<string, string>;
		onPlatformMappingChange: (original: string, platformId: string) => void;
		onStorefrontMappingChange: (original: string, storefrontId: string) => void;
	}

	let {
		platformSuggestions,
		storefrontSuggestions,
		platformMappings,
		storefrontMappings,
		onPlatformMappingChange,
		onStorefrontMappingChange
	}: Props = $props();

	const platforms = $derived(platformsStore.value.platforms);
	const storefronts = $derived(platformsStore.value.storefronts);

	const unresolvedPlatformCount = $derived(
		platformSuggestions.filter((p) => !platformMappings[p.original]).length
	);
	const unresolvedStorefrontCount = $derived(
		storefrontSuggestions.filter((s) => !storefrontMappings[s.original]).length
	);
	const hasUnresolved = $derived(unresolvedPlatformCount > 0 || unresolvedStorefrontCount > 0);
</script>

{#if platformSuggestions.length > 0 || storefrontSuggestions.length > 0}
	<div class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6 mb-6">
		<div class="flex items-center justify-between mb-4">
			<h2 class="text-lg font-medium text-gray-900 dark:text-white">
				Platform & Storefront Mappings
			</h2>
			{#if hasUnresolved}
				<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300">
					{unresolvedPlatformCount + unresolvedStorefrontCount} need mapping
				</span>
			{:else}
				<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300">
					All mapped
				</span>
			{/if}
		</div>

		{#if platformSuggestions.length > 0}
			<div class="mb-6">
				<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Platforms</h3>
				<div class="space-y-3">
					{#each platformSuggestions as suggestion}
						<div class="flex items-center gap-4">
							<div class="flex-1 min-w-0">
								<span class="text-sm font-medium text-gray-900 dark:text-white">
									"{suggestion.original}"
								</span>
								<span class="text-sm text-gray-500 dark:text-gray-400 ml-2">
									({suggestion.count} game{suggestion.count !== 1 ? 's' : ''})
								</span>
							</div>
							<div class="flex items-center gap-2">
								<span class="text-gray-400">→</span>
								<select
									class="block w-48 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm"
									value={platformMappings[suggestion.original] || suggestion.suggested_id || ''}
									onchange={(e) => onPlatformMappingChange(suggestion.original, e.currentTarget.value)}
								>
									<option value="">-- Skip --</option>
									{#each platforms as platform}
										<option value={platform.id}>{platform.display_name}</option>
									{/each}
								</select>
								{#if platformMappings[suggestion.original] || suggestion.suggested_id}
									<svg class="w-5 h-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		{#if storefrontSuggestions.length > 0}
			<div>
				<h3 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">Storefronts</h3>
				<div class="space-y-3">
					{#each storefrontSuggestions as suggestion}
						<div class="flex items-center gap-4">
							<div class="flex-1 min-w-0">
								<span class="text-sm font-medium text-gray-900 dark:text-white">
									"{suggestion.original}"
								</span>
								<span class="text-sm text-gray-500 dark:text-gray-400 ml-2">
									({suggestion.count} game{suggestion.count !== 1 ? 's' : ''})
								</span>
							</div>
							<div class="flex items-center gap-2">
								<span class="text-gray-400">→</span>
								<select
									class="block w-48 rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm"
									value={storefrontMappings[suggestion.original] || suggestion.suggested_id || ''}
									onchange={(e) => onStorefrontMappingChange(suggestion.original, e.currentTarget.value)}
								>
									<option value="">-- Skip --</option>
									{#each storefronts as storefront}
										<option value={storefront.id}>{storefront.display_name}</option>
									{/each}
								</select>
								{#if storefrontMappings[suggestion.original] || suggestion.suggested_id}
									<svg class="w-5 h-5 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
									</svg>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
{/if}
```

**Step 2: Export from components index**

Add to `frontend/src/lib/components/index.ts`:

```typescript
export { default as PlatformMappingSection } from './PlatformMappingSection.svelte';
```

**Step 3: Run frontend type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/lib/components/PlatformMappingSection.svelte frontend/src/lib/components/index.ts
git commit -m "feat(frontend): add PlatformMappingSection component"
```

---

## Phase 5: Frontend - Review Store Updates

### Task 5.1: Add Platform Summary and Finalize Methods to Review Store

**Files:**
- Modify: `frontend/src/lib/stores/review.svelte.ts`
- Modify: `frontend/src/lib/types/jobs.ts`

**Step 1: Add types for platform summary and finalization**

Add to `frontend/src/lib/types/jobs.ts`:

```typescript
export interface PlatformMappingSuggestion {
  original: string;
  count: number;
  suggested_id: string | null;
  suggested_name: string | null;
}

export interface PlatformSummaryResponse {
  platforms: PlatformMappingSuggestion[];
  storefronts: PlatformMappingSuggestion[];
  all_resolved: boolean;
}

export interface FinalizeImportRequest {
  job_id: string;
  platform_mappings: Record<string, string>;
  storefront_mappings: Record<string, string>;
}

export interface FinalizeImportResponse {
  success: boolean;
  message: string;
  games_imported: number;
  games_skipped: number;
  errors: string[];
}
```

**Step 2: Add methods to review store**

Add to `frontend/src/lib/stores/review.svelte.ts` (inside the store object, after `reset`):

```typescript
    /**
     * Load platform summary for a job.
     */
    loadPlatformSummary: async (jobId: string): Promise<PlatformSummaryResponse> => {
      try {
        const response = await api.get(`${config.apiUrl}/review/platform-summary?job_id=${jobId}`);
        const data: PlatformSummaryResponse = await response.json();
        return data;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to load platform summary';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Finalize an import job.
     */
    finalizeImport: async (
      jobId: string,
      platformMappings: Record<string, string>,
      storefrontMappings: Record<string, string>
    ): Promise<FinalizeImportResponse> => {
      try {
        const response = await api.post(`${config.apiUrl}/review/finalize`, {
          job_id: jobId,
          platform_mappings: platformMappings,
          storefront_mappings: storefrontMappings
        });
        const data: FinalizeImportResponse = await response.json();
        return data;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to finalize import';
        state.error = errorMessage;
        throw error;
      }
    }
```

Also add the imports at the top of the file:

```typescript
import type {
  ReviewItem,
  ReviewItemDetail,
  ReviewListResponse,
  ReviewSummary,
  ReviewCountsByType,
  MatchResponse,
  ReviewFilters,
  PlatformSummaryResponse,
  FinalizeImportResponse
} from '$lib/types/jobs';
```

**Step 3: Run frontend type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/lib/stores/review.svelte.ts frontend/src/lib/types/jobs.ts
git commit -m "feat(frontend): add platform summary and finalize methods to review store"
```

---

## Phase 6: Frontend - Review Page Integration

### Task 6.1: Update Review Page with Platform Mapping and Finalize

**Files:**
- Modify: `frontend/src/routes/review/+page.svelte`

**Step 1: Update the review page**

This is a significant modification. The key changes:
1. Load platform summary when viewing a Darkadia import job
2. Show PlatformMappingSection at top
3. Add Finalize button
4. Initialize mappings from suggestions

Add these state variables after the existing ones (~line 20):

```svelte
	// Platform mapping state (for Darkadia imports)
	let platformSummary = $state<{
		platforms: Array<{original: string; count: number; suggested_id: string | null; suggested_name: string | null}>;
		storefronts: Array<{original: string; count: number; suggested_id: string | null; suggested_name: string | null}>;
		all_resolved: boolean;
	} | null>(null);
	let platformMappings = $state<Record<string, string>>({});
	let storefrontMappings = $state<Record<string, string>>({});
	let isFinalizingImport = $state(false);
```

Add this derived state after the existing ones:

```svelte
	// Check if this is a Darkadia import job
	const isDarkadiaImport = $derived(
		jobIdFromUrl && items.some(item => item.job_source === 'darkadia')
	);
```

Add platform summary loading in onMount (after `review.loadSummary();`):

```svelte
		// Load platform summary if filtering by job
		if (jobIdFromUrl) {
			loadPlatformSummary(jobIdFromUrl);
		}
```

Add the function to load platform summary:

```svelte
	async function loadPlatformSummary(jobId: string) {
		try {
			platformSummary = await review.loadPlatformSummary(jobId);

			// Initialize mappings from suggestions
			for (const p of platformSummary.platforms) {
				if (p.suggested_id && !platformMappings[p.original]) {
					platformMappings[p.original] = p.suggested_id;
				}
			}
			for (const s of platformSummary.storefronts) {
				if (s.suggested_id && !storefrontMappings[s.original]) {
					storefrontMappings[s.original] = s.suggested_id;
				}
			}
		} catch (e) {
			console.error('Failed to load platform summary:', e);
		}
	}
```

Add handlers for platform mapping changes:

```svelte
	function handlePlatformMappingChange(original: string, platformId: string) {
		if (platformId) {
			platformMappings[original] = platformId;
		} else {
			delete platformMappings[original];
		}
	}

	function handleStorefrontMappingChange(original: string, storefrontId: string) {
		if (storefrontId) {
			storefrontMappings[original] = storefrontId;
		} else {
			delete storefrontMappings[original];
		}
	}
```

Add finalize handler:

```svelte
	async function handleFinalizeImport() {
		if (!jobIdFromUrl) return;

		isFinalizingImport = true;
		try {
			const result = await review.finalizeImport(
				jobIdFromUrl,
				platformMappings,
				storefrontMappings
			);

			if (result.success) {
				// Show success and redirect to collection
				goto('/games?notification=import_complete&count=' + result.games_imported);
			}
		} catch (e) {
			console.error('Failed to finalize import:', e);
		} finally {
			isFinalizingImport = false;
		}
	}
```

Add derived for finalize button state:

```svelte
	const canFinalize = $derived(
		isDarkadiaImport &&
		items.some(item => item.status === 'matched') &&
		!isFinalizingImport
	);
```

In the template, add the PlatformMappingSection and Finalize button after the filters section (before the error state):

```svelte
		<!-- Platform Mapping Section (for Darkadia imports) -->
		{#if isDarkadiaImport && platformSummary}
			<PlatformMappingSection
				platformSuggestions={platformSummary.platforms}
				storefrontSuggestions={platformSummary.storefronts}
				{platformMappings}
				{storefrontMappings}
				onPlatformMappingChange={handlePlatformMappingChange}
				onStorefrontMappingChange={handleStorefrontMappingChange}
			/>

			<!-- Finalize Button -->
			<div class="mb-6 flex justify-end">
				<button
					type="button"
					class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 disabled:opacity-50 disabled:cursor-not-allowed"
					disabled={!canFinalize}
					onclick={handleFinalizeImport}
				>
					{#if isFinalizingImport}
						<svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
							<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
							<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
						</svg>
						Finalizing...
					{:else}
						Finalize Import
					{/if}
				</button>
			</div>
		{/if}
```

Add the import at the top:

```svelte
	import { PlatformMappingSection } from '$lib/components';
```

**Step 2: Run frontend checks**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/routes/review/+page.svelte
git commit -m "feat(frontend): integrate platform mapping and finalize into review page"
```

---

## Phase 7: Legacy Code Removal

### Task 7.1: Remove Legacy Backend Files

**Files:**
- Delete: `backend/app/api/import_api/sources/darkadia.py`
- Delete: `backend/app/api/import_api/sources/darkadia_batch.py`
- Delete: `backend/app/services/import_sources/darkadia.py`
- Delete: `backend/app/models/darkadia_game.py`
- Delete: `backend/app/models/darkadia_import.py`
- Modify: `backend/app/api/import_api/sources/__init__.py`
- Modify: `backend/app/models/__init__.py`

**Step 1: Remove legacy API files**

```bash
rm backend/app/api/import_api/sources/darkadia.py
rm backend/app/api/import_api/sources/darkadia_batch.py
```

**Step 2: Update import_api sources __init__.py**

Replace `backend/app/api/import_api/sources/__init__.py` with:

```python
"""
Import sources API package for managing source-specific import operations.
"""

from fastapi import APIRouter

# Future imports:
# from .epic import router as epic_router
# from .gog import router as gog_router

router = APIRouter(tags=["Import Sources"])

# Future source routers:
# router.include_router(epic_router, prefix="/epic", tags=["Import - Epic"])
# router.include_router(gog_router, prefix="/gog", tags=["Import - GOG"])
```

**Step 3: Remove legacy service**

```bash
rm backend/app/services/import_sources/darkadia.py
```

**Step 4: Remove legacy models**

```bash
rm backend/app/models/darkadia_game.py
rm backend/app/models/darkadia_import.py
```

**Step 5: Update models __init__.py**

Edit `backend/app/models/__init__.py` to remove the darkadia imports:

Remove these lines:
```python
from .darkadia_game import DarkadiaGame
from .darkadia_import import DarkadiaImport
```

And remove from `__all__`:
```python
    "DarkadiaGame",
    "DarkadiaImport",
```

**Step 6: Run backend tests**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest -v
```

Expected: Tests pass (some tests referencing legacy code may need deletion)

**Step 7: Commit**

```bash
git add -A backend/
git commit -m "refactor(backend): remove legacy Darkadia import system"
```

---

### Task 7.2: Remove Legacy Frontend Files

**Files:**
- Delete: `frontend/src/lib/stores/darkadia.svelte.ts`
- Delete: `frontend/src/lib/components/DarkadiaFileUpload.svelte`
- Delete: `frontend/src/lib/components/DarkadiaGameCard.svelte`
- Delete: `frontend/src/lib/components/DarkadiaGamesTable.svelte`
- Delete: `frontend/src/lib/components/DarkadiaManualMatchModal.svelte`
- Delete: `frontend/src/lib/components/DarkadiaFileUpload.test.ts`
- Delete: `frontend/src/routes/import/darkadia/+page.svelte.legacy` (backup)
- Modify: `frontend/src/lib/stores/index.ts`
- Modify: `frontend/src/lib/components/index.ts`

**Step 1: Remove legacy store**

```bash
rm frontend/src/lib/stores/darkadia.svelte.ts
```

**Step 2: Update stores index**

Edit `frontend/src/lib/stores/index.ts` to remove darkadia exports.

Remove these lines:
```typescript
// Darkadia store
export { darkadia } from './darkadia.svelte';
export type {
  DarkadiaState,
  DarkadiaGameStatusFilter
} from './darkadia.svelte';
```

**Step 3: Remove legacy components**

```bash
rm frontend/src/lib/components/DarkadiaFileUpload.svelte
rm frontend/src/lib/components/DarkadiaFileUpload.test.ts
rm frontend/src/lib/components/DarkadiaGameCard.svelte
rm frontend/src/lib/components/DarkadiaGamesTable.svelte
rm frontend/src/lib/components/DarkadiaManualMatchModal.svelte
rm frontend/src/routes/import/darkadia/+page.svelte.legacy
```

**Step 4: Update components index if needed**

Check if any Darkadia components are exported from `frontend/src/lib/components/index.ts` and remove them.

**Step 5: Run frontend checks**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: PASS (may have import errors to fix)

**Step 6: Run frontend tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

Expected: Tests pass (tests referencing deleted components will be gone)

**Step 7: Commit**

```bash
git add -A frontend/
git commit -m "refactor(frontend): remove legacy Darkadia import system"
```

---

### Task 7.3: Create Database Migration for Legacy Table Removal

**Files:**
- Create: Alembic migration

**Step 1: Create migration**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run alembic revision --autogenerate -m "drop legacy darkadia tables"
```

**Step 2: Verify migration content**

Check the generated migration file in `backend/app/migrations/versions/` and ensure it includes:

```python
def upgrade() -> None:
    op.drop_table('darkadia_imports')
    op.drop_table('darkadia_games')

def downgrade() -> None:
    # Recreation SQL would go here (optional)
    pass
```

**Step 3: Run migration**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run alembic upgrade head
```

**Step 4: Commit**

```bash
git add backend/app/migrations/
git commit -m "migration: drop legacy Darkadia tables"
```

---

## Phase 8: Integration Testing

### Task 8.1: End-to-End Test

**Manual testing checklist:**

1. Start services: `podman-compose up --build`
2. Upload a Darkadia CSV at `/import/darkadia`
3. Verify redirect to `/review?job_id=...&source=import`
4. Check platform mapping section appears with suggestions
5. Verify games appear in review list
6. Match a few games
7. Skip a few games
8. Click "Finalize Import"
9. Verify redirect to collection with games present
10. Check worker logs show processing happened in worker container

**Step 1: Run manual test**

Follow the checklist above.

**Step 2: Update beads issues**

```bash
bd close nexorious-2dc --reason="Frontend wired to worker-based import"
bd close nexorious-otu --reason="Legacy system removed"
bd sync
```

**Step 3: Final commit**

```bash
git add -A
git commit -m "feat: complete Darkadia worker migration

- Rewrote upload page to use /api/import/darkadia endpoint
- Added platform/storefront mapping to review UI
- Added finalization endpoint for creating UserGame records
- Removed ~5000 lines of legacy backend code
- Removed ~2500 lines of legacy frontend code
- Processing now happens in worker container

Closes: nexorious-2dc, nexorious-otu"
```

---

## Summary

| Phase | Tasks | Estimated Changes |
|-------|-------|-------------------|
| 1 | Worker improvements | ~50 lines added |
| 2 | New API endpoints | ~250 lines added |
| 3 | Upload page rewrite | ~200 lines (replace ~1300) |
| 4 | Platform mapping component | ~150 lines added |
| 5 | Review store updates | ~50 lines added |
| 6 | Review page integration | ~100 lines added |
| 7 | Legacy removal | ~5000 lines deleted |
| 8 | Integration testing | Manual + commits |

**Net change:** ~4200 lines removed
