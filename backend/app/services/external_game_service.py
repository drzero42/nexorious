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
        """Create or update an ExternalGame record."""
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
        """Mark all ExternalGames as unavailable except those in the set."""
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
        """Get unresolved (and not skipped) ExternalGames."""
        query = select(ExternalGame).where(
            ExternalGame.user_id == user_id,
            ExternalGame.resolved_igdb_id == None,
            ExternalGame.is_skipped == False,
        )

        if storefront:
            query = query.where(ExternalGame.storefront == storefront)

        return list(self.session.exec(query).all())

    def resolve_igdb_id(self, external_game_id: str, igdb_id: int) -> None:
        """Set the resolved IGDB ID for an ExternalGame."""
        external_game = self.session.get(ExternalGame, external_game_id)
        if external_game:
            external_game.resolved_igdb_id = igdb_id
            external_game.updated_at = datetime.now(timezone.utc)
            self.session.add(external_game)
            self.session.commit()

    def skip(self, external_game_id: str) -> None:
        """Mark an ExternalGame as skipped."""
        external_game = self.session.get(ExternalGame, external_game_id)
        if external_game:
            external_game.is_skipped = True
            external_game.updated_at = datetime.now(timezone.utc)
            self.session.add(external_game)
            self.session.commit()

    def unskip(self, external_game_id: str) -> None:
        """Remove skip status from an ExternalGame."""
        external_game = self.session.get(ExternalGame, external_game_id)
        if external_game:
            external_game.is_skipped = False
            external_game.updated_at = datetime.now(timezone.utc)
            self.session.add(external_game)
            self.session.commit()

    def get_by_id(self, external_game_id: str) -> Optional[ExternalGame]:
        """Get an ExternalGame by ID."""
        return self.session.get(ExternalGame, external_game_id)

    def get_for_sync(
        self,
        user_id: str,
        storefront: str,
    ) -> List[ExternalGame]:
        """Get all resolved, non-skipped ExternalGames ready to sync."""
        return list(self.session.exec(
            select(ExternalGame).where(
                ExternalGame.user_id == user_id,
                ExternalGame.storefront == storefront,
                ExternalGame.resolved_igdb_id != None,
                ExternalGame.is_skipped == False,
            )
        ).all())
