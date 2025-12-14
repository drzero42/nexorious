"""
Main matching service orchestrator.

Provides the unified interface for matching games from various sources
to IGDB IDs using multiple strategies.
"""

import logging
from typing import List, Optional

from sqlmodel import Session

from app.services.igdb import IGDBService

from .models import (
    MatchRequest,
    MatchResult,
    MatchStatus,
    MatchSource,
    BatchMatchResult,
)
from .igdb_lookup import match_by_title, get_game_by_igdb_id
from .platform_lookup import lookup_by_steam_appid, lookup_by_igdb_id

logger = logging.getLogger(__name__)

# Configuration defaults
DEFAULT_AUTO_MATCH_THRESHOLD = 0.85  # 85% confidence for auto-match
DEFAULT_BATCH_SIZE = 10


class MatchingService:
    """
    Unified service for matching games from external sources to IGDB.

    Implements a multi-strategy matching approach:
    1. IGDB ID if present - Use directly (trusted source like Nexorious export)
    2. Platform ID lookup - Check DB for existing Steam AppID / Epic ID matches
    3. Title-based search - Search IGDB, auto-match if high confidence

    Usage:
        async with MatchingService(session, igdb_service) as matcher:
            result = await matcher.match_game(request)

        # Or for batch operations:
        results = await matcher.match_batch(requests)
    """

    def __init__(
        self,
        session: Session,
        igdb_service: IGDBService,
        auto_match_threshold: float = DEFAULT_AUTO_MATCH_THRESHOLD,
    ):
        """
        Initialize the matching service.

        Args:
            session: Database session for platform lookups
            igdb_service: IGDB service for title-based searching
            auto_match_threshold: Minimum confidence for automatic title matching
        """
        self._session = session
        self._igdb_service = igdb_service
        self._auto_match_threshold = auto_match_threshold

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        pass

    @property
    def auto_match_threshold(self) -> float:
        """Get the current auto-match threshold."""
        return self._auto_match_threshold

    @auto_match_threshold.setter
    def auto_match_threshold(self, value: float):
        """Set the auto-match threshold (0.0 to 1.0)."""
        if not 0.0 <= value <= 1.0:
            raise ValueError("Auto-match threshold must be between 0.0 and 1.0")
        self._auto_match_threshold = value

    async def match_game(self, request: MatchRequest) -> MatchResult:
        """
        Attempt to match a single game using all available strategies.

        The matching flow is:
        1. If IGDB ID provided -> Use directly (verify it exists)
        2. If platform ID provided -> Lookup by platform ID
        3. Fall back to title-based IGDB search

        Args:
            request: MatchRequest with game information

        Returns:
            MatchResult with matching status and details
        """
        logger.info(
            f"Matching game: '{request.source_title}' "
            f"(platform: {request.source_platform})"
        )

        # Strategy 1: IGDB ID provided (trusted source)
        if request.igdb_id:
            result = await self._match_by_igdb_id(request)
            if result.is_matched:
                return result
            # If IGDB ID lookup failed, continue to other strategies
            logger.warning(
                f"Provided IGDB ID {request.igdb_id} not found, "
                "falling back to other strategies"
            )

        # Strategy 2: Platform-specific ID lookup
        if request.platform_id:
            result = await self._match_by_platform_id(request)
            if result:
                return result

        # Strategy 3: Title-based IGDB search
        return await self._match_by_title(request)

    async def match_batch(
        self,
        requests: List[MatchRequest],
        batch_size: int = DEFAULT_BATCH_SIZE,
    ) -> BatchMatchResult:
        """
        Match a batch of games.

        Processes games in order, accumulating results. Does not use
        concurrent execution to respect IGDB rate limits.

        Args:
            requests: List of MatchRequest objects
            batch_size: Not used currently, reserved for future optimization

        Returns:
            BatchMatchResult with aggregated statistics and individual results
        """
        logger.info(f"Starting batch match for {len(requests)} games")

        batch_result = BatchMatchResult()

        for i, request in enumerate(requests):
            logger.debug(
                f"Processing game {i + 1}/{len(requests)}: '{request.source_title}'"
            )

            try:
                result = await self.match_game(request)
                batch_result.add_result(result)
            except Exception as e:
                logger.error(
                    f"Unexpected error matching '{request.source_title}': {e}"
                )
                batch_result.add_result(
                    MatchResult(
                        source_title=request.source_title,
                        status=MatchStatus.ERROR,
                        error_message=str(e),
                        source_metadata=request.source_metadata or {},
                    )
                )

        logger.info(
            f"Batch match complete: {batch_result.matched} matched, "
            f"{batch_result.needs_review} need review, "
            f"{batch_result.no_match} no match, "
            f"{batch_result.errors} errors "
            f"(success rate: {batch_result.success_rate:.1%})"
        )

        return batch_result

    async def _match_by_igdb_id(self, request: MatchRequest) -> MatchResult:
        """Match by provided IGDB ID (Strategy 1)."""
        igdb_id = request.igdb_id
        if not igdb_id:
            return MatchResult(
                source_title=request.source_title,
                status=MatchStatus.NO_MATCH,
                source_metadata=request.source_metadata or {},
            )

        logger.debug(f"Attempting match by IGDB ID: {igdb_id}")

        # First check local DB
        local_result = await lookup_by_igdb_id(self._session, igdb_id)
        if local_result:
            logger.info(
                f"Matched '{request.source_title}' via local IGDB ID lookup: "
                f"'{local_result.igdb_title}'"
            )
            # Update source_title with the request's title
            local_result.source_title = request.source_title
            local_result.source_metadata = request.source_metadata or {}
            return local_result

        # Not in local DB, verify with IGDB API
        game_data = await get_game_by_igdb_id(self._igdb_service, igdb_id)
        if game_data:
            logger.info(
                f"Matched '{request.source_title}' via IGDB API: "
                f"'{game_data.title}' (IGDB ID: {igdb_id})"
            )
            return MatchResult(
                source_title=request.source_title,
                status=MatchStatus.MATCHED,
                match_source=MatchSource.IGDB_ID_PROVIDED,
                igdb_id=igdb_id,
                igdb_title=game_data.title,
                confidence_score=1.0,
                source_metadata=request.source_metadata or {},
            )

        # IGDB ID not found even in API
        logger.warning(f"IGDB ID {igdb_id} not found in API")
        return MatchResult(
            source_title=request.source_title,
            status=MatchStatus.NO_MATCH,
            error_message=f"IGDB ID {igdb_id} not found",
            source_metadata=request.source_metadata or {},
        )

    async def _match_by_platform_id(
        self, request: MatchRequest
    ) -> Optional[MatchResult]:
        """Match by platform-specific ID (Strategy 2)."""
        platform_id = request.platform_id
        if not platform_id:
            return None

        logger.debug(
            f"Attempting match by platform ID: {platform_id} "
            f"(platform: {request.source_platform})"
        )

        result = None

        # Dispatch to appropriate platform lookup
        if request.source_platform == "steam":
            try:
                steam_appid = int(platform_id)
                result = await lookup_by_steam_appid(self._session, steam_appid)
            except ValueError:
                logger.warning(f"Invalid Steam AppID: {platform_id}")
                return None

        # TODO: Add Epic, GOG, etc. lookups here in future

        if result:
            # Update with request info
            result.source_title = request.source_title
            result.source_metadata = {
                **(request.source_metadata or {}),
                "platform_id": platform_id,
                "platform": request.source_platform,
            }
            logger.info(
                f"Matched '{request.source_title}' via platform lookup: "
                f"'{result.igdb_title}' (IGDB ID: {result.igdb_id})"
            )
            return result

        logger.debug(
            f"No existing match found for {request.source_platform} ID: {platform_id}"
        )
        return None

    async def _match_by_title(self, request: MatchRequest) -> MatchResult:
        """Match by title search (Strategy 3)."""
        logger.debug(f"Attempting match by title search: '{request.source_title}'")

        result = await match_by_title(
            self._igdb_service,
            request.source_title,
            auto_match_threshold=self._auto_match_threshold,
            source_metadata={
                **(request.source_metadata or {}),
                "platform": request.source_platform,
                "platform_id": request.platform_id,
            },
        )

        return result


# Convenience function for simple use cases
async def match_single_game(
    session: Session,
    igdb_service: IGDBService,
    title: str,
    platform: str = "unknown",
    igdb_id: Optional[int] = None,
    platform_id: Optional[str] = None,
    auto_match_threshold: float = DEFAULT_AUTO_MATCH_THRESHOLD,
) -> MatchResult:
    """
    Convenience function to match a single game without instantiating the service.

    Args:
        session: Database session
        igdb_service: IGDB service instance
        title: Game title
        platform: Source platform (steam, epic, darkadia, etc.)
        igdb_id: Optional IGDB ID if already known
        platform_id: Optional platform-specific ID (Steam AppID, etc.)
        auto_match_threshold: Confidence threshold for auto-matching

    Returns:
        MatchResult with matching status and details
    """
    service = MatchingService(
        session, igdb_service, auto_match_threshold=auto_match_threshold
    )

    request = MatchRequest(
        source_title=title,
        source_platform=platform,
        igdb_id=igdb_id,
        platform_id=platform_id,
    )

    return await service.match_game(request)
