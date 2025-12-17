# Backend Architecture Refactoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve backend architecture by fixing layering violations, breaking down monolithic files, and evaluating type checker alternatives.

**Architecture:** Phase 1 addresses schema location and service layer violations. Phase 2 breaks down large API files into focused modules. Phase 3 is research-only for type checker evaluation.

**Tech Stack:** Python 3.13, FastAPI, SQLModel, Pydantic v2, Pyrefly/Pyright

**Related Issues:** nexorious-9mu, nexorious-8wy, nexorious-rx0

---

## Task 1: Research Type Checker Alternatives (nexorious-rx0)

**Files:**
- Create: `docs/adr/2025-12-17-type-checker-evaluation.md`

**Note:** This is a research task, not implementation. The goal is to evaluate Pyrefly vs Pyright and document findings.

**Step 1: Document current Pyrefly setup**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check 2>&1 | head -50`

Note the number of errors and common patterns.

**Step 2: Install Pyright temporarily**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv add --dev pyright`

**Step 3: Run Pyright and compare**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyright 2>&1 | head -100`

**Step 4: Create ADR document**

Create `docs/adr/2025-12-17-type-checker-evaluation.md`:

```markdown
# ADR: Type Checker Evaluation - Pyrefly vs Pyright

**Date:** 2025-12-17
**Status:** [Pending/Accepted/Rejected]
**Deciders:** [Team]

## Context

We currently use Pyrefly for type checking. This ADR evaluates whether to switch to Pyright.

## Comparison

| Aspect | Pyrefly | Pyright |
|--------|---------|---------|
| Speed | 14x faster | Baseline |
| Maturity | Beta | Stable (Microsoft) |
| SQLAlchemy support | Plugin needed | Native 2.0+ support |
| Community | Smaller | Large, industry standard |
| IDE integration | Limited | Excellent (Pylance) |
| False positives | More common | Fewer |

## Current State

- Pyrefly errors: [X]
- Pyright errors: [Y]
- Common error types: [list]

## Evaluation Criteria

1. **Error reduction:** Does switching reduce meaningful type errors?
2. **Migration effort:** How much config/code change needed?
3. **CI/CD impact:** Build time changes?
4. **Developer experience:** IDE integration improvements?

## Findings

[To be filled after evaluation]

## Decision

[To be filled after evaluation]

## Consequences

[To be filled after evaluation]
```

**Step 5: Remove Pyright if not adopting**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv remove pyright` (if not adopting)

**Step 6: Update beads issue with findings**

Run: `bd comments nexorious-rx0 "Completed evaluation. See docs/adr/2025-12-17-type-checker-evaluation.md for findings."`

**Step 7: Close or update issue based on decision**

If adopting Pyright, create follow-up tasks.
If keeping Pyrefly, close the issue:
```bash
bd close nexorious-rx0 --reason="Evaluated Pyright. Decision: [keep Pyrefly / switch to Pyright]. See ADR for details."
```

---

## Task 2: Fix Layering Violations - Move Schemas (nexorious-9mu Phase 1)

**Files:**
- Create: `backend/app/schemas/` directory structure
- Move: `backend/app/api/schemas/` → `backend/app/schemas/`
- Modify: 14+ files with import updates

**Step 1: Create schemas directory**

Run: `mkdir -p /home/abo/workspace/home/nexorious/backend/app/schemas`

**Step 2: Copy schema files to new location**

Run:
```bash
cp -r /home/abo/workspace/home/nexorious/backend/app/api/schemas/* /home/abo/workspace/home/nexorious/backend/app/schemas/
```

**Step 3: Update imports in API files**

For each file, change `from ..api.schemas` or `from .schemas` to `from ..schemas`:

Files to update:
- `backend/app/api/games.py`
- `backend/app/api/platforms.py`
- `backend/app/api/user_games.py`
- `backend/app/api/auth.py`
- `backend/app/api/tags.py`
- `backend/app/api/review.py`
- `backend/app/api/jobs.py`

Example change in `backend/app/api/games.py`:

Change:
```python
from .schemas.game import GameResponse, GameCreateRequest
```

To:
```python
from ..schemas.game import GameResponse, GameCreateRequest
```

**Step 4: Update imports in import_api files**

Files to update:
- `backend/app/api/import_api/core.py`
- `backend/app/api/import_api/sources/*.py`

Change patterns like:
```python
from ...api.schemas.platform import ...
```

To:
```python
from ....schemas.platform import ...
```

**Step 5: Update imports in service files**

Files to update:
- `backend/app/services/platform_resolution.py`
- `backend/app/services/tag_service.py`
- `backend/app/services/steam_games.py`

Change:
```python
from ..api.schemas.platform import PlatformSuggestion
```

To:
```python
from ..schemas.platform import PlatformSuggestion
```

**Step 6: Update imports in test files**

Files to update:
- `backend/app/tests/test_tag_service.py`
- `backend/app/tests/test_storefront_resolution.py`
- `backend/app/tests/test_platform_resolution_service.py`

Change:
```python
from app.api.schemas.platform import PlatformSuggestion
```

To:
```python
from app.schemas.platform import PlatformSuggestion
```

**Step 7: Run tests to verify imports work**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 8: Remove old schemas directory**

Run: `rm -rf /home/abo/workspace/home/nexorious/backend/app/api/schemas`

**Step 9: Run tests again after removal**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 10: Commit**

```bash
git add backend/app/schemas backend/app/api backend/app/services backend/app/tests
git commit -m "refactor(backend): move schemas to shared location

Move schemas from app/api/schemas/ to app/schemas/ to fix layering
violations where services were importing from the API layer.

Part of: nexorious-9mu"
```

---

## Task 3: Fix Layering Violations - Extract GameService (nexorious-9mu Phase 2)

**Files:**
- Create: `backend/app/services/game_service.py`
- Modify: `backend/app/api/games.py`
- Modify: `backend/app/services/steam_games.py`
- Test: `backend/app/tests/test_game_service.py`

**Step 1: Write failing test for GameService**

Create `backend/app/tests/test_game_service.py`:

```python
"""Tests for GameService."""
import pytest
from unittest.mock import AsyncMock, MagicMock

from app.services.game_service import GameService


class TestGameService:
    """Tests for GameService.create_or_update_game_from_igdb."""

    @pytest.fixture
    def mock_session(self):
        """Create mock database session."""
        session = MagicMock()
        session.get = MagicMock(return_value=None)
        session.add = MagicMock()
        session.commit = MagicMock()
        session.refresh = MagicMock()
        return session

    @pytest.fixture
    def mock_igdb_service(self):
        """Create mock IGDB service."""
        service = AsyncMock()
        service.get_game_by_id = AsyncMock(return_value={
            "id": 12345,
            "name": "Test Game",
            "slug": "test-game",
            "cover": {"url": "//images.igdb.com/test.jpg"},
        })
        return service

    @pytest.mark.asyncio
    async def test_create_game_from_igdb(self, mock_session, mock_igdb_service):
        """Test creating a new game from IGDB data."""
        service = GameService(mock_session, mock_igdb_service)

        game = await service.create_or_update_game_from_igdb(12345)

        assert game is not None
        mock_igdb_service.get_game_by_id.assert_called_once_with(12345)
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_game_service.py -v`

Expected: FAIL with "cannot import name 'GameService'"

**Step 3: Create GameService class**

Create `backend/app/services/game_service.py`:

```python
"""
Game service for managing game data and IGDB integration.

This service handles game creation, updates, and IGDB data fetching,
providing a clean interface for both API endpoints and other services.
"""
import logging
from typing import Optional

from sqlmodel import Session

from ..models.game import Game
from ..services.igdb.service import IGDBService
from ..services.storage import StorageService

logger = logging.getLogger(__name__)


class GameService:
    """
    Service for game management operations.

    Provides methods for creating and updating games from IGDB data,
    separated from the API layer to allow reuse across services.
    """

    def __init__(self, session: Session, igdb_service: Optional[IGDBService] = None):
        """
        Initialize GameService.

        Args:
            session: Database session
            igdb_service: Optional IGDB service instance (created if not provided)
        """
        self.session = session
        self.igdb_service = igdb_service or IGDBService()
        self.storage_service = StorageService()

    async def create_or_update_game_from_igdb(
        self,
        igdb_id: int,
        download_cover_art: bool = True,
    ) -> Optional[Game]:
        """
        Create or update a game from IGDB data.

        Args:
            igdb_id: IGDB game ID
            download_cover_art: Whether to download cover art

        Returns:
            Game model instance, or None if IGDB lookup failed
        """
        # Check if game already exists
        existing_game = self.session.get(Game, igdb_id)
        if existing_game:
            logger.debug(f"Game {igdb_id} already exists in database")
            return existing_game

        # Fetch from IGDB
        igdb_data = await self.igdb_service.get_game_by_id(igdb_id)
        if not igdb_data:
            logger.warning(f"Could not fetch IGDB data for game {igdb_id}")
            return None

        # Create game from IGDB data
        game = self._create_game_from_igdb_data(igdb_data)

        # Download cover art if requested
        if download_cover_art and igdb_data.get("cover"):
            cover_url = igdb_data["cover"].get("url")
            if cover_url:
                local_cover_url = await self.storage_service.download_cover_art(
                    str(igdb_id),
                    self._normalize_cover_url(cover_url)
                )
                if local_cover_url:
                    game.cover_art_url = local_cover_url

        # Save to database
        self.session.add(game)
        self.session.commit()
        self.session.refresh(game)

        logger.info(f"Created game {game.id}: {game.name}")
        return game

    def _create_game_from_igdb_data(self, igdb_data: dict) -> Game:
        """Create Game model from IGDB API response."""
        return Game(
            id=igdb_data["id"],
            name=igdb_data.get("name", "Unknown"),
            slug=igdb_data.get("slug"),
            summary=igdb_data.get("summary"),
            storyline=igdb_data.get("storyline"),
            first_release_date=igdb_data.get("first_release_date"),
            rating=igdb_data.get("rating"),
            rating_count=igdb_data.get("rating_count"),
            aggregated_rating=igdb_data.get("aggregated_rating"),
            aggregated_rating_count=igdb_data.get("aggregated_rating_count"),
            total_rating=igdb_data.get("total_rating"),
            total_rating_count=igdb_data.get("total_rating_count"),
            hypes=igdb_data.get("hypes"),
            follows=igdb_data.get("follows"),
        )

    @staticmethod
    def _normalize_cover_url(url: str) -> str:
        """Normalize IGDB cover URL to full HTTPS URL."""
        if url.startswith("//"):
            return f"https:{url}"
        return url
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_game_service.py -v`

Expected: PASS

**Step 5: Update steam_games.py to use GameService**

Modify `backend/app/services/steam_games.py`:

Remove:
```python
from ..api.games import import_from_igdb
```

Add:
```python
from .game_service import GameService
```

Change usage from:
```python
game_response = await import_from_igdb(import_request, self.session, user, self.igdb_service)
```

To:
```python
game_service = GameService(self.session, self.igdb_service)
game = await game_service.create_or_update_game_from_igdb(igdb_id)
```

**Step 6: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 7: Commit**

```bash
git add backend/app/services/game_service.py backend/app/services/steam_games.py backend/app/tests/test_game_service.py
git commit -m "refactor(services): extract GameService from API layer

Create GameService with create_or_update_game_from_igdb method,
removing the layering violation where steam_games.py imported
from the API layer.

Part of: nexorious-9mu"
```

---

## Task 4: Break Down Monolithic API Files (nexorious-8wy)

**Files:**
- Refactor: `backend/app/api/platforms.py` (1747 LOC)
- Refactor: `backend/app/api/user_games.py` (1178 LOC)
- Refactor: `backend/app/api/games.py` (1112 LOC)

**Note:** This is a large refactoring task. Each file should be broken into focused modules.

### Sub-task 4a: Break down platforms.py

**Step 1: Analyze platforms.py structure**

Run: `cd /home/abo/workspace/home/nexorious/backend && grep -n "^@router\|^async def\|^def " app/api/platforms.py | head -40`

Identify logical groupings:
- Platform CRUD operations
- Storefront CRUD operations
- Platform-Storefront associations
- Resolution/suggestion endpoints

**Step 2: Create platforms API package**

```bash
mkdir -p /home/abo/workspace/home/nexorious/backend/app/api/platforms_api
touch /home/abo/workspace/home/nexorious/backend/app/api/platforms_api/__init__.py
```

**Step 3: Extract platform CRUD to separate module**

Create `backend/app/api/platforms_api/platforms.py` with platform CRUD endpoints.

**Step 4: Extract storefront CRUD to separate module**

Create `backend/app/api/platforms_api/storefronts.py` with storefront CRUD endpoints.

**Step 5: Extract resolution endpoints to separate module**

Create `backend/app/api/platforms_api/resolution.py` with suggestion/resolution endpoints.

**Step 6: Update __init__.py to combine routers**

```python
from fastapi import APIRouter
from .platforms import router as platforms_router
from .storefronts import router as storefronts_router
from .resolution import router as resolution_router

router = APIRouter()
router.include_router(platforms_router)
router.include_router(storefronts_router)
router.include_router(resolution_router)
```

**Step 7: Update main router registration**

**Step 8: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -k "platform" -v`

**Step 9: Commit**

```bash
git add backend/app/api/platforms_api backend/app/api/platforms.py
git commit -m "refactor(api): break down platforms.py into focused modules

Split 1747 LOC platforms.py into:
- platforms.py: Platform CRUD
- storefronts.py: Storefront CRUD
- resolution.py: Platform/storefront resolution

Part of: nexorious-8wy"
```

### Sub-task 4b: Break down user_games.py

Follow similar pattern to 4a, splitting into:
- `user_games_api/crud.py` - Basic CRUD
- `user_games_api/bulk.py` - Bulk operations
- `user_games_api/platforms.py` - Platform associations

### Sub-task 4c: Break down games.py

Follow similar pattern, splitting into:
- `games_api/crud.py` - Basic CRUD
- `games_api/igdb.py` - IGDB integration
- `games_api/search.py` - Search functionality

---

## Task 5: Close Issues and Sync

**Step 1: Close nexorious-9mu**

Run: `bd close nexorious-9mu --reason="Schemas moved to shared location, GameService extracted from API layer"`

**Step 2: Close nexorious-8wy**

Run: `bd close nexorious-8wy --reason="Monolithic API files broken into focused modules"`

**Step 3: Sync beads**

Run: `bd sync`

---

## Summary

| Task | Issue | Complexity | Priority |
|------|-------|------------|----------|
| Type checker evaluation | nexorious-rx0 | Low (research) | P2 |
| Move schemas | nexorious-9mu | Medium | P2 |
| Extract GameService | nexorious-9mu | Medium | P2 |
| Break down API files | nexorious-8wy | High | P2 |

**Recommended execution order:**
1. Task 1 (research, non-blocking)
2. Task 2 (enables Task 3)
3. Task 3 (completes nexorious-9mu)
4. Task 4 (can be done incrementally)
