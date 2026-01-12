# ExternalGame Model Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the ExternalGame SQLModel to replace the transient dataclass, enabling persistent IGDB resolution storage, subscription tracking, and improved sync flow.

**Architecture:** ExternalGame becomes a persistent SQLModel that stores sync source state. UserGamePlatform links to ExternalGame via nullable FK. Sync flow updates ExternalGame first, then propagates to UserGamePlatform based on `sync_from_source` flag.

**Tech Stack:** Python 3.13, SQLModel, Alembic, FastAPI, pytest

---

## Task 1: Create ExternalGame Model

**Files:**
- Create: `backend/app/models/external_game.py`
- Modify: `backend/app/models/__init__.py`
- Modify: `backend/app/models/user.py` (add relationship)
- Test: `backend/app/tests/test_external_game_model.py`

**Step 1: Write the failing test**

Create `backend/app/tests/test_external_game_model.py`:

```python
"""Tests for ExternalGame model."""

import pytest
from datetime import datetime, timezone
from sqlmodel import Session, select

from app.models.external_game import ExternalGame
from app.models.user_game import OwnershipStatus


class TestExternalGameModel:
    """Tests for ExternalGame SQLModel."""

    def test_create_external_game(self, db_session: Session, test_user):
        """Test creating an ExternalGame record."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            platform="pc-windows",
        )
        db_session.add(external_game)
        db_session.commit()
        db_session.refresh(external_game)

        assert external_game.id is not None
        assert external_game.user_id == test_user.id
        assert external_game.storefront == "steam"
        assert external_game.external_id == "12345"
        assert external_game.title == "Test Game"
        assert external_game.resolved_igdb_id is None
        assert external_game.is_skipped is False
        assert external_game.is_available is True
        assert external_game.is_subscription is False
        assert external_game.playtime_hours == 0

    def test_unique_constraint(self, db_session: Session, test_user):
        """Test unique constraint on user_id, storefront, external_id."""
        external_game1 = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )
        db_session.add(external_game1)
        db_session.commit()

        external_game2 = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game Duplicate",
        )
        db_session.add(external_game2)

        with pytest.raises(Exception):  # IntegrityError
            db_session.commit()

    def test_store_url_steam(self, db_session: Session, test_user):
        """Test store_url computed property for Steam."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )
        db_session.add(external_game)
        db_session.commit()

        assert external_game.store_url == "https://store.steampowered.com/app/12345"

    def test_store_url_epic(self, db_session: Session, test_user):
        """Test store_url computed property for Epic."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="epic",
            external_id="fortnite",
            title="Fortnite",
        )
        db_session.add(external_game)
        db_session.commit()

        assert external_game.store_url == "https://store.epicgames.com/p/fortnite"

    def test_store_url_psn(self, db_session: Session, test_user):
        """Test store_url computed property for PSN."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="psn",
            external_id="UP0001-CUSA00001_00-TESTGAME00000001",
            title="Test Game",
        )
        db_session.add(external_game)
        db_session.commit()

        assert external_game.store_url == "https://store.playstation.com/product/UP0001-CUSA00001_00-TESTGAME00000001"

    def test_store_url_gog(self, db_session: Session, test_user):
        """Test store_url computed property for GOG."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="gog",
            external_id="1234567890",
            title="Test Game",
        )
        db_session.add(external_game)
        db_session.commit()

        assert external_game.store_url == "https://www.gog.com/game/1234567890"

    def test_with_ownership_status(self, db_session: Session, test_user):
        """Test ExternalGame with ownership status."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="psn",
            external_id="12345",
            title="PS Plus Game",
            ownership_status=OwnershipStatus.SUBSCRIPTION,
            is_subscription=True,
        )
        db_session.add(external_game)
        db_session.commit()
        db_session.refresh(external_game)

        assert external_game.ownership_status == OwnershipStatus.SUBSCRIPTION
        assert external_game.is_subscription is True
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_external_game_model.py -v`

Expected: FAIL with "ModuleNotFoundError: No module named 'app.models.external_game'"

**Step 3: Write the ExternalGame model**

Create `backend/app/models/external_game.py`:

```python
"""
ExternalGame model for persistent sync source tracking.

This model stores the state of games from external sync sources (Steam, Epic, PSN, etc.)
including IGDB resolution, subscription status, and availability.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import Optional, TYPE_CHECKING
from datetime import datetime, timezone
from pydantic import computed_field
import uuid

from .user_game import OwnershipStatus

if TYPE_CHECKING:
    from .user import User
    from .user_game import UserGamePlatform


def build_store_url(storefront: str, external_id: str) -> Optional[str]:
    """Build store URL from storefront and external_id.

    Args:
        storefront: The storefront identifier (steam, epic, psn, gog)
        external_id: The platform-specific game ID

    Returns:
        The store URL or None if storefront is not supported
    """
    url_patterns = {
        "steam": f"https://store.steampowered.com/app/{external_id}",
        "epic": f"https://store.epicgames.com/p/{external_id}",
        "psn": f"https://store.playstation.com/product/{external_id}",
        "gog": f"https://www.gog.com/game/{external_id}",
    }
    return url_patterns.get(storefront.lower())


class ExternalGame(SQLModel, table=True):
    """
    Persistent model for games from external sync sources.

    This model tracks:
    - Source state (what the platform reports)
    - Resolution state (IGDB ID, skip status)
    - Link to UserGamePlatform (when imported to collection)
    """

    __tablename__ = "external_games"  # pyrefly: ignore[bad-override]

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    storefront: str = Field(foreign_key="storefronts.name", index=True)
    external_id: str = Field(max_length=200, index=True)
    title: str = Field(max_length=500)

    # Resolution state
    resolved_igdb_id: Optional[int] = Field(default=None, foreign_key="games.id", index=True)
    is_skipped: bool = Field(default=False, index=True)

    # Source state (always reflects what platform reports)
    is_available: bool = Field(default=True, index=True)
    is_subscription: bool = Field(default=False)
    playtime_hours: int = Field(default=0, ge=0)
    ownership_status: Optional[OwnershipStatus] = Field(default=None)

    # Platform info
    platform: Optional[str] = Field(default=None, foreign_key="platforms.name")

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: "User" = Relationship(back_populates="external_games")
    user_game_platforms: list["UserGamePlatform"] = Relationship(back_populates="external_game")

    __table_args__ = (
        UniqueConstraint("user_id", "storefront", "external_id", name="uq_external_games_user_storefront_external"),
        {"extend_existing": True},
    )

    @computed_field
    @property
    def store_url(self) -> Optional[str]:
        """Compute store URL from storefront and external_id."""
        return build_store_url(self.storefront, self.external_id)
```

**Step 4: Update models/__init__.py**

Modify `backend/app/models/__init__.py` to add import and export:

```python
# Add import after IgnoredExternalGame import
from .external_game import ExternalGame

# Add to __all__ list
"ExternalGame",
```

**Step 5: Update User model**

Modify `backend/app/models/user.py`:

Add to TYPE_CHECKING imports:
```python
from .external_game import ExternalGame
```

Add relationship to User class:
```python
external_games: List["ExternalGame"] = Relationship(back_populates="user")
```

**Step 6: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_external_game_model.py -v`

Expected: PASS

**Step 7: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pyrefly check`

Expected: No errors

**Step 8: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add backend/app/models/external_game.py backend/app/models/__init__.py backend/app/models/user.py backend/app/tests/test_external_game_model.py
git commit -m "feat: Add ExternalGame model for persistent sync tracking

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Modify UserGamePlatform Model

**Files:**
- Modify: `backend/app/models/user_game.py`
- Test: `backend/app/tests/test_user_game_platform_external_link.py`

**Step 1: Write the failing test**

Create `backend/app/tests/test_user_game_platform_external_link.py`:

```python
"""Tests for UserGamePlatform external_game link."""

import pytest
from sqlmodel import Session

from app.models.user_game import UserGame, UserGamePlatform
from app.models.external_game import ExternalGame


class TestUserGamePlatformExternalLink:
    """Tests for UserGamePlatform external_game_id field."""

    def test_user_game_platform_with_external_game(self, db_session: Session, test_user, test_game):
        """Test linking UserGamePlatform to ExternalGame."""
        # Create ExternalGame
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            resolved_igdb_id=test_game.id,
        )
        db_session.add(external_game)
        db_session.commit()
        db_session.refresh(external_game)

        # Create UserGame
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        db_session.add(user_game)
        db_session.commit()
        db_session.refresh(user_game)

        # Create UserGamePlatform linked to ExternalGame
        platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform="pc-windows",
            storefront="steam",
            external_game_id=external_game.id,
        )
        db_session.add(platform)
        db_session.commit()
        db_session.refresh(platform)

        assert platform.external_game_id == external_game.id
        assert platform.sync_from_source is True  # Default

    def test_user_game_platform_without_external_game(self, db_session: Session, test_user, test_game):
        """Test UserGamePlatform without ExternalGame (manual entry)."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        db_session.add(user_game)
        db_session.commit()
        db_session.refresh(user_game)

        platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform="pc-windows",
            storefront="steam",
        )
        db_session.add(platform)
        db_session.commit()
        db_session.refresh(platform)

        assert platform.external_game_id is None
        assert platform.sync_from_source is True

    def test_sync_from_source_flag(self, db_session: Session, test_user, test_game):
        """Test sync_from_source flag can be set to False."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        db_session.add(user_game)
        db_session.commit()

        platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform="pc-windows",
            storefront="steam",
            sync_from_source=False,
        )
        db_session.add(platform)
        db_session.commit()
        db_session.refresh(platform)

        assert platform.sync_from_source is False
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_user_game_platform_external_link.py -v`

Expected: FAIL with AttributeError (external_game_id doesn't exist)

**Step 3: Update UserGamePlatform model**

Modify `backend/app/models/user_game.py`:

Add to TYPE_CHECKING imports:
```python
from .external_game import ExternalGame
```

Add new fields to UserGamePlatform class (after `original_storefront_name`):
```python
    # Sync link fields
    external_game_id: Optional[str] = Field(default=None, foreign_key="external_games.id", index=True)
    sync_from_source: bool = Field(default=True, description="If True, sync updates this entry from ExternalGame")
```

Add relationship to UserGamePlatform class:
```python
    external_game: Optional["ExternalGame"] = Relationship(back_populates="user_game_platforms")
```

Remove these fields (they will be on ExternalGame):
```python
    store_game_id: Optional[str] = Field(default=None, max_length=200)  # REMOVE
    store_url: Optional[str] = Field(default=None, max_length=500)  # REMOVE
```

**WAIT:** Do NOT remove store_game_id and store_url yet - that's in Task 4 (migration). For now, keep them and just add the new fields.

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_user_game_platform_external_link.py -v`

Expected: PASS

**Step 5: Run all existing tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest -v`

Expected: All tests pass (model changes are additive at this point)

**Step 6: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pyrefly check`

Expected: No errors

**Step 7: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add backend/app/models/user_game.py backend/app/tests/test_user_game_platform_external_link.py
git commit -m "feat: Add external_game_id and sync_from_source to UserGamePlatform

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Create Database Migration

**Files:**
- Generate: `backend/app/alembic/versions/XXXX_add_external_games_table.py` (via autogenerate)

**Step 1: Generate migration**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run alembic revision --autogenerate -m "add external_games table and user_game_platform fields"`

Expected: Creates new migration file

**Step 2: Review migration**

Read the generated migration file and verify it:
- Creates `external_games` table with all columns
- Adds `external_game_id` column to `user_game_platforms`
- Adds `sync_from_source` column to `user_game_platforms`
- Has proper foreign key constraints
- Has unique constraint on (user_id, storefront, external_id)

**Step 3: Apply migration**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run alembic upgrade head`

Expected: Migration applies successfully

**Step 4: Run tests to verify migration works**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_external_game_model.py app/tests/test_user_game_platform_external_link.py -v`

Expected: All tests pass

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add backend/app/alembic/versions/
git commit -m "migration: Add external_games table and link fields

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Migrate IgnoredExternalGame Data

**Files:**
- Generate: `backend/app/alembic/versions/XXXX_migrate_ignored_to_external_games.py` (via autogenerate after model changes)
- Modify: `backend/app/models/__init__.py` (remove IgnoredExternalGame export)
- Modify: `backend/app/models/user.py` (remove relationship)

**Step 1: Create data migration**

This migration will:
1. Copy data from `ignored_external_games` to `external_games` with `is_skipped=True`
2. Drop the `ignored_external_games` table

Create migration manually (since it's data migration):

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run alembic revision -m "migrate ignored_external_games to external_games"`

Then edit the generated file:

```python
"""migrate ignored_external_games to external_games

Revision ID: <generated>
Revises: <previous>
Create Date: <generated>
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa
from datetime import datetime, timezone


# revision identifiers, used by Alembic.
revision: str = '<generated>'
down_revision: Union[str, None] = '<previous>'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Map BackgroundJobSource to storefront names
    source_to_storefront = {
        'STEAM': 'steam',
        'EPIC': 'epic',
        'GOG': 'gog',
        'PSN': 'psn',
    }

    conn = op.get_bind()

    # Get all ignored external games
    ignored_games = conn.execute(
        sa.text("SELECT id, user_id, source, external_id, title, created_at FROM ignored_external_games")
    ).fetchall()

    # Insert into external_games
    for game in ignored_games:
        storefront = source_to_storefront.get(game.source, game.source.lower())
        conn.execute(
            sa.text("""
                INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, playtime_hours, created_at, updated_at)
                VALUES (:id, :user_id, :storefront, :external_id, :title, true, true, false, 0, :created_at, :updated_at)
                ON CONFLICT (user_id, storefront, external_id) DO UPDATE SET is_skipped = true
            """),
            {
                "id": game.id,
                "user_id": game.user_id,
                "storefront": storefront,
                "external_id": game.external_id,
                "title": game.title,
                "created_at": game.created_at,
                "updated_at": datetime.now(timezone.utc),
            }
        )

    # Drop ignored_external_games table
    op.drop_table('ignored_external_games')


def downgrade() -> None:
    # Recreate ignored_external_games table
    op.create_table(
        'ignored_external_games',
        sa.Column('id', sa.String(), nullable=False),
        sa.Column('user_id', sa.String(), nullable=False),
        sa.Column('source', sa.String(), nullable=False),
        sa.Column('external_id', sa.String(100), nullable=False),
        sa.Column('title', sa.String(500), nullable=False),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.ForeignKeyConstraint(['user_id'], ['users.id']),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('user_id', 'source', 'external_id', name='uq_ignored_external_games_user_source_external'),
    )

    # Migrate data back (skipped external_games -> ignored_external_games)
    storefront_to_source = {
        'steam': 'STEAM',
        'epic': 'EPIC',
        'gog': 'GOG',
        'psn': 'PSN',
    }

    conn = op.get_bind()
    skipped_games = conn.execute(
        sa.text("SELECT id, user_id, storefront, external_id, title, created_at FROM external_games WHERE is_skipped = true")
    ).fetchall()

    for game in skipped_games:
        source = storefront_to_source.get(game.storefront, game.storefront.upper())
        conn.execute(
            sa.text("""
                INSERT INTO ignored_external_games (id, user_id, source, external_id, title, created_at)
                VALUES (:id, :user_id, :source, :external_id, :title, :created_at)
            """),
            {
                "id": game.id,
                "user_id": game.user_id,
                "source": source,
                "external_id": game.external_id,
                "title": game.title,
                "created_at": game.created_at,
            }
        )
```

**Step 2: Apply migration**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run alembic upgrade head`

Expected: Migration applies, data migrated, table dropped

**Step 3: Remove IgnoredExternalGame from codebase**

Update `backend/app/models/__init__.py`:
- Remove: `from .ignored_external_game import IgnoredExternalGame`
- Remove: `"IgnoredExternalGame",` from `__all__`

Update `backend/app/models/user.py`:
- Remove from TYPE_CHECKING: `from .ignored_external_game import IgnoredExternalGame`
- Remove relationship: `ignored_external_games: List["IgnoredExternalGame"] = Relationship(back_populates="user")`

**Step 4: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pyrefly check`

Expected: May show errors in files still referencing IgnoredExternalGame (will fix in later tasks)

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add backend/app/alembic/versions/ backend/app/models/__init__.py backend/app/models/user.py
git commit -m "migration: Migrate IgnoredExternalGame data to ExternalGame

- Copy ignored games to external_games with is_skipped=true
- Drop ignored_external_games table
- Remove IgnoredExternalGame model

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Create ExternalGame Service

**Files:**
- Create: `backend/app/services/external_game_service.py`
- Test: `backend/app/tests/test_external_game_service.py`

**Step 1: Write the failing test**

Create `backend/app/tests/test_external_game_service.py`:

```python
"""Tests for ExternalGameService."""

import pytest
from sqlmodel import Session, select
from unittest.mock import MagicMock

from app.services.external_game_service import ExternalGameService
from app.models.external_game import ExternalGame
from app.models.user_game import UserGame, UserGamePlatform, OwnershipStatus


class TestExternalGameService:
    """Tests for ExternalGameService."""

    def test_create_or_update_creates_new(self, db_session: Session, test_user):
        """Test creating a new ExternalGame."""
        service = ExternalGameService(db_session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            platform="pc-windows",
            playtime_hours=10,
        )

        assert external_game.id is not None
        assert external_game.title == "Test Game"
        assert external_game.playtime_hours == 10
        assert external_game.is_available is True

    def test_create_or_update_updates_existing(self, db_session: Session, test_user):
        """Test updating an existing ExternalGame."""
        service = ExternalGameService(db_session)

        # Create initial
        external_game1 = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            playtime_hours=10,
        )

        # Update
        external_game2 = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game Updated",
            playtime_hours=20,
        )

        assert external_game1.id == external_game2.id
        assert external_game2.title == "Test Game Updated"
        assert external_game2.playtime_hours == 20

    def test_mark_unavailable(self, db_session: Session, test_user):
        """Test marking games as unavailable."""
        service = ExternalGameService(db_session)

        # Create two games
        service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="111",
            title="Game 1",
        )
        service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="222",
            title="Game 2",
        )

        # Mark games not in the set as unavailable
        service.mark_unavailable_except(
            user_id=test_user.id,
            storefront="steam",
            available_external_ids={"111"},  # Only 111 is available
        )

        games = db_session.exec(
            select(ExternalGame).where(ExternalGame.user_id == test_user.id)
        ).all()

        game1 = next(g for g in games if g.external_id == "111")
        game2 = next(g for g in games if g.external_id == "222")

        assert game1.is_available is True
        assert game2.is_available is False

    def test_get_unresolved(self, db_session: Session, test_user):
        """Test getting unresolved games."""
        service = ExternalGameService(db_session)

        # Create resolved game
        resolved = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="111",
            title="Resolved",
        )
        resolved.resolved_igdb_id = 12345
        db_session.add(resolved)
        db_session.commit()

        # Create unresolved game
        service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="222",
            title="Unresolved",
        )

        # Create skipped game
        skipped = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="333",
            title="Skipped",
        )
        skipped.is_skipped = True
        db_session.add(skipped)
        db_session.commit()

        unresolved = service.get_unresolved(test_user.id, "steam")

        assert len(unresolved) == 1
        assert unresolved[0].external_id == "222"

    def test_resolve_igdb_id(self, db_session: Session, test_user):
        """Test resolving IGDB ID."""
        service = ExternalGameService(db_session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )

        service.resolve_igdb_id(external_game.id, 99999)

        db_session.refresh(external_game)
        assert external_game.resolved_igdb_id == 99999

    def test_skip_game(self, db_session: Session, test_user):
        """Test skipping a game."""
        service = ExternalGameService(db_session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )

        service.skip(external_game.id)

        db_session.refresh(external_game)
        assert external_game.is_skipped is True

    def test_unskip_game(self, db_session: Session, test_user):
        """Test unskipping a game."""
        service = ExternalGameService(db_session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )
        external_game.is_skipped = True
        db_session.add(external_game)
        db_session.commit()

        service.unskip(external_game.id)

        db_session.refresh(external_game)
        assert external_game.is_skipped is False
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_external_game_service.py -v`

Expected: FAIL with "ModuleNotFoundError: No module named 'app.services.external_game_service'"

**Step 3: Write the service**

Create `backend/app/services/external_game_service.py`:

```python
"""Service for managing ExternalGame records."""

from datetime import datetime, timezone
from typing import Optional, Set, List

from sqlmodel import Session, select

from app.models.external_game import ExternalGame
from app.models.user_game import OwnershipStatus


class ExternalGameService:
    """Service for ExternalGame CRUD operations."""

    def __init__(self, session: Session):
        self.session = session

    def create_or_update(
        self,
        user_id: str,
        storefront: str,
        external_id: str,
        title: str,
        platform: Optional[str] = None,
        playtime_hours: int = 0,
        ownership_status: Optional[OwnershipStatus] = None,
        is_subscription: bool = False,
    ) -> ExternalGame:
        """Create or update an ExternalGame record.

        Args:
            user_id: The user's ID
            storefront: Storefront name (steam, epic, psn, gog)
            external_id: Platform-specific game ID
            title: Game title from source
            platform: Optional platform identifier
            playtime_hours: Playtime from source
            ownership_status: Ownership status from source
            is_subscription: Whether game is from subscription

        Returns:
            The created or updated ExternalGame
        """
        external_game = self.session.exec(
            select(ExternalGame).where(
                ExternalGame.user_id == user_id,
                ExternalGame.storefront == storefront,
                ExternalGame.external_id == external_id,
            )
        ).first()

        if external_game:
            # Update existing
            external_game.title = title
            external_game.playtime_hours = playtime_hours
            external_game.is_available = True
            if platform:
                external_game.platform = platform
            if ownership_status:
                external_game.ownership_status = ownership_status
            external_game.is_subscription = is_subscription
            external_game.updated_at = datetime.now(timezone.utc)
        else:
            # Create new
            external_game = ExternalGame(
                user_id=user_id,
                storefront=storefront,
                external_id=external_id,
                title=title,
                platform=platform,
                playtime_hours=playtime_hours,
                ownership_status=ownership_status,
                is_subscription=is_subscription,
            )

        self.session.add(external_game)
        self.session.commit()
        self.session.refresh(external_game)
        return external_game

    def mark_unavailable_except(
        self,
        user_id: str,
        storefront: str,
        available_external_ids: Set[str],
    ) -> int:
        """Mark all ExternalGames as unavailable except those in the set.

        Args:
            user_id: The user's ID
            storefront: Storefront name
            available_external_ids: Set of external_ids that are still available

        Returns:
            Number of games marked unavailable
        """
        games = self.session.exec(
            select(ExternalGame).where(
                ExternalGame.user_id == user_id,
                ExternalGame.storefront == storefront,
                ExternalGame.is_available == True,
            )
        ).all()

        count = 0
        for game in games:
            if game.external_id not in available_external_ids:
                game.is_available = False
                game.updated_at = datetime.now(timezone.utc)
                self.session.add(game)
                count += 1

        self.session.commit()
        return count

    def get_unresolved(
        self,
        user_id: str,
        storefront: Optional[str] = None,
    ) -> List[ExternalGame]:
        """Get unresolved (and not skipped) ExternalGames.

        Args:
            user_id: The user's ID
            storefront: Optional storefront filter

        Returns:
            List of unresolved ExternalGames
        """
        query = select(ExternalGame).where(
            ExternalGame.user_id == user_id,
            ExternalGame.resolved_igdb_id == None,
            ExternalGame.is_skipped == False,
        )

        if storefront:
            query = query.where(ExternalGame.storefront == storefront)

        return list(self.session.exec(query).all())

    def resolve_igdb_id(self, external_game_id: str, igdb_id: int) -> None:
        """Set the resolved IGDB ID for an ExternalGame.

        Args:
            external_game_id: The ExternalGame ID
            igdb_id: The IGDB game ID
        """
        external_game = self.session.get(ExternalGame, external_game_id)
        if external_game:
            external_game.resolved_igdb_id = igdb_id
            external_game.updated_at = datetime.now(timezone.utc)
            self.session.add(external_game)
            self.session.commit()

    def skip(self, external_game_id: str) -> None:
        """Mark an ExternalGame as skipped.

        Args:
            external_game_id: The ExternalGame ID
        """
        external_game = self.session.get(ExternalGame, external_game_id)
        if external_game:
            external_game.is_skipped = True
            external_game.updated_at = datetime.now(timezone.utc)
            self.session.add(external_game)
            self.session.commit()

    def unskip(self, external_game_id: str) -> None:
        """Remove skip status from an ExternalGame.

        Args:
            external_game_id: The ExternalGame ID
        """
        external_game = self.session.get(ExternalGame, external_game_id)
        if external_game:
            external_game.is_skipped = False
            external_game.updated_at = datetime.now(timezone.utc)
            self.session.add(external_game)
            self.session.commit()

    def get_by_id(self, external_game_id: str) -> Optional[ExternalGame]:
        """Get an ExternalGame by ID.

        Args:
            external_game_id: The ExternalGame ID

        Returns:
            The ExternalGame or None
        """
        return self.session.get(ExternalGame, external_game_id)

    def get_for_sync(
        self,
        user_id: str,
        storefront: str,
    ) -> List[ExternalGame]:
        """Get all resolved, non-skipped ExternalGames ready to sync.

        Args:
            user_id: The user's ID
            storefront: Storefront name

        Returns:
            List of ExternalGames ready to sync
        """
        return list(self.session.exec(
            select(ExternalGame).where(
                ExternalGame.user_id == user_id,
                ExternalGame.storefront == storefront,
                ExternalGame.resolved_igdb_id != None,
                ExternalGame.is_skipped == False,
            )
        ).all())
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_external_game_service.py -v`

Expected: PASS

**Step 5: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pyrefly check`

Expected: No errors

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add backend/app/services/external_game_service.py backend/app/tests/test_external_game_service.py
git commit -m "feat: Add ExternalGameService for CRUD operations

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 6: Update Sync Dispatch to Create ExternalGames

**Files:**
- Modify: `backend/app/worker/tasks/sync/dispatch.py`
- Test: `backend/app/tests/test_sync_dispatch.py` (update existing tests)

**Step 1: Write the failing test**

Add to `backend/app/tests/test_sync_dispatch.py`:

```python
class TestDispatchCreatesExternalGames:
    """Tests for ExternalGame creation during dispatch."""

    @pytest.mark.asyncio
    async def test_creates_external_games(self):
        """Test that dispatch creates ExternalGame records."""
        # Setup mocks
        with patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_context, \
             patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter, \
             patch("app.worker.tasks.sync.dispatch.enqueue_task"):

            mock_session = MagicMock()
            mock_context.return_value.__aenter__.return_value = mock_session

            mock_job = MagicMock()
            mock_job.id = "job123"
            mock_job.priority = BackgroundJobPriority.LOW
            mock_session.get.side_effect = [mock_job, MagicMock()]  # Job, User

            mock_adapter_instance = MagicMock()
            mock_adapter_instance.fetch_games.return_value = [
                ExternalGame(
                    external_id="12345",
                    title="Test Game",
                    platform="pc-windows",
                    storefront="steam",
                    metadata={},
                    playtime_hours=10,
                )
            ]
            mock_adapter.return_value = mock_adapter_instance

            # Mock ExternalGameService
            with patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_service_class:
                mock_service = MagicMock()
                mock_service.create_or_update.return_value = MagicMock(id="eg123")
                mock_service_class.return_value = mock_service

                result = await dispatch_sync_items("job123", "user123", "steam")

                # Verify ExternalGameService.create_or_update was called
                mock_service.create_or_update.assert_called_once_with(
                    user_id="user123",
                    storefront="steam",
                    external_id="12345",
                    title="Test Game",
                    platform="pc-windows",
                    playtime_hours=10,
                    ownership_status=None,
                    is_subscription=False,
                )
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_sync_dispatch.py::TestDispatchCreatesExternalGames -v`

Expected: FAIL

**Step 3: Update dispatch.py**

Modify `backend/app/worker/tasks/sync/dispatch.py`:

Add import:
```python
from app.services.external_game_service import ExternalGameService
```

Update `dispatch_sync_items` function to create ExternalGames before JobItems:

```python
@broker.task(task_name="sync.dispatch")
async def dispatch_sync_items(
    job_id: str,
    user_id: str,
    source: str,
) -> Dict[str, Any]:
    """
    Fan-out task that creates ExternalGames, JobItems, and dispatches worker tasks.

    This task:
    1. Fetches the user's game library via the source adapter
    2. Creates/updates ExternalGame for each game
    3. Marks games not in source as unavailable
    4. Creates a JobItem for each game (streaming insert)
    5. Dispatches a process_sync_item task for each JobItem
    6. Returns quickly - actual processing happens in parallel workers
    """
    logger.info(f"Starting sync dispatch for user {user_id}, source {source} (job: {job_id})")

    stats: Dict[str, int] = {
        "total_games": 0,
        "external_games_created": 0,
        "external_games_updated": 0,
        "marked_unavailable": 0,
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
            games = await adapter.fetch_games(user, session)
            stats["total_games"] = len(games)

            # Update job total_items
            job.total_items = len(games)
            session.add(job)
            session.commit()

            logger.info(f"Fetched {len(games)} games from {source} for user {user_id}")

            # Create ExternalGameService
            external_game_service = ExternalGameService(session)

            # Track external_ids for marking unavailable
            available_external_ids = set()

            # Determine priority for item tasks
            priority = job.priority

            # Process each game: create ExternalGame, then JobItem
            for game in games:
                try:
                    available_external_ids.add(game.external_id)

                    # Create or update ExternalGame
                    external_game = external_game_service.create_or_update(
                        user_id=user_id,
                        storefront=game.storefront,
                        external_id=game.external_id,
                        title=game.title,
                        platform=game.platform,
                        playtime_hours=game.playtime_hours,
                        ownership_status=game.ownership_status,
                        is_subscription=game.ownership_status == OwnershipStatus.SUBSCRIPTION if game.ownership_status else False,
                    )

                    # Create JobItem
                    job_item = _create_job_item(
                        session=session,
                        job=job,
                        user_id=user_id,
                        game=game,
                        external_game_id=external_game.id,
                    )

                    # Only dispatch if we created a new item (not a duplicate)
                    if job_item:
                        await _dispatch_process_task(job_item.id, priority)
                        stats["dispatched"] += 1

                except Exception as e:
                    logger.error(f"Error creating/dispatching item for {game.title}: {e}")
                    stats["errors"] += 1
                    session.rollback()

            # Mark games not in source as unavailable
            stats["marked_unavailable"] = external_game_service.mark_unavailable_except(
                user_id=user_id,
                storefront=source,
                available_external_ids=available_external_ids,
            )

            logger.info(
                f"Sync dispatch completed for job {job_id}: "
                f"{stats['dispatched']} dispatched, {stats['marked_unavailable']} marked unavailable, {stats['errors']} errors"
            )

            return {"status": "dispatched", **stats}

        except Exception as e:
            logger.error(f"Sync dispatch failed for job {job_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}
```

Also update `_create_job_item` to accept `external_game_id`:

```python
def _create_job_item(
    session: Session,
    job: Job,
    user_id: str,
    game: ExternalGame,
    external_game_id: str,
) -> JobItem | None:
    """Create a JobItem for a game."""
    # ... existing code ...

    source_metadata = {
        "external_id": game.external_id,
        "platform": game.platform,
        "storefront": game.storefront,
        "metadata": game.metadata,
        "playtime_hours": game.playtime_hours,
        "ownership_status": game.ownership_status.value if game.ownership_status else None,
        "external_game_id": external_game_id,  # Add this
    }

    # ... rest of function ...
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_sync_dispatch.py -v`

Expected: PASS

**Step 5: Run all tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest -v`

Expected: All tests pass

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add backend/app/worker/tasks/sync/dispatch.py backend/app/tests/test_sync_dispatch.py
git commit -m "feat: Update sync dispatch to create ExternalGames

- Create/update ExternalGame for each game from source
- Mark games not in source as unavailable
- Pass external_game_id to JobItem metadata

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 7: Update Process Item to Use ExternalGame

**Files:**
- Modify: `backend/app/worker/tasks/sync/process_item.py`
- Modify: `backend/app/tests/test_sync_process_item.py`

**Step 1: Update _is_ignored to check ExternalGame.is_skipped**

The `_is_ignored` function should now check `ExternalGame.is_skipped` instead of `IgnoredExternalGame` table.

**Step 2: Update _add_platform_association to link ExternalGame**

The function should now also set `external_game_id` on the created `UserGamePlatform`.

**Step 3: Update tests**

Update the tests in `test_sync_process_item.py` to reflect the new behavior.

**Step 4: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest app/tests/test_sync_process_item.py -v`

**Step 5: Run all tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest -v`

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add backend/app/worker/tasks/sync/process_item.py backend/app/tests/test_sync_process_item.py
git commit -m "feat: Update process_item to use ExternalGame for skip check and linking

- Check ExternalGame.is_skipped instead of IgnoredExternalGame
- Link UserGamePlatform to ExternalGame via external_game_id
- Update ExternalGame.resolved_igdb_id after matching

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 8: Remove store_game_id and store_url from UserGamePlatform

**Files:**
- Modify: `backend/app/models/user_game.py`
- Generate: Migration via alembic autogenerate
- Update: Tests that reference store_game_id or store_url

**Step 1: Remove fields from model**

Remove from `UserGamePlatform`:
```python
store_game_id: Optional[str] = Field(default=None, max_length=200)
store_url: Optional[str] = Field(default=None, max_length=500)
```

**Step 2: Generate migration**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run alembic revision --autogenerate -m "remove store_game_id and store_url from user_game_platforms"`

**Step 3: Apply migration**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run alembic upgrade head`

**Step 4: Update code that references removed fields**

Search for and update any code referencing `store_game_id` or `store_url` on `UserGamePlatform`. These should now come from the linked `ExternalGame`.

**Step 5: Update tests**

Fix any tests that reference the removed fields.

**Step 6: Run all tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest -v`

**Step 7: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add -A
git commit -m "refactor: Remove store_game_id and store_url from UserGamePlatform

These fields now live on ExternalGame.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 9: Delete IgnoredExternalGame Model File

**Files:**
- Delete: `backend/app/models/ignored_external_game.py`
- Update: Any remaining references

**Step 1: Delete the file**

```bash
rm backend/app/models/ignored_external_game.py
```

**Step 2: Search for remaining references**

```bash
grep -r "IgnoredExternalGame" backend/app/
grep -r "ignored_external_game" backend/app/
```

**Step 3: Update any remaining references**

Replace usage of `IgnoredExternalGame` with `ExternalGame.is_skipped`.

**Step 4: Run all tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest -v`

**Step 5: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pyrefly check`

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model
git add -A
git commit -m "refactor: Remove IgnoredExternalGame model file

All functionality moved to ExternalGame.is_skipped.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

---

## Task 10: Run Full Test Suite and Final Verification

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pytest --cov=app --cov-report=term-missing`

Expected: All tests pass with >80% coverage

**Step 2: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run pyrefly check`

Expected: No errors

**Step 3: Run linter**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run ruff check .`

Expected: No errors

**Step 4: Verify migration is complete**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/external-game-model/backend && uv run alembic current`

Expected: Shows latest migration

---

## Summary of Changes

1. **New Model:** `ExternalGame` - persistent sync source tracking
2. **Modified Model:** `UserGamePlatform` - added `external_game_id` FK and `sync_from_source` flag
3. **Removed Model:** `IgnoredExternalGame` - replaced by `ExternalGame.is_skipped`
4. **Removed Fields:** `store_game_id`, `store_url` from `UserGamePlatform`
5. **New Service:** `ExternalGameService` - CRUD operations for ExternalGame
6. **Modified Sync:** Dispatch creates ExternalGames, process_item links to them
7. **Migrations:** 3 migrations for schema changes and data migration
