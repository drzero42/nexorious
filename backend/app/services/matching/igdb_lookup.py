"""
IGDB lookup module for title-based game matching.

Handles searching IGDB for games by title and scoring matches by confidence.
"""

import logging
from typing import List, Optional

from app.services.igdb import IGDBService, IGDBError
from app.services.igdb.models import GameMetadata
from app.utils.fuzzy_match import calculate_fuzzy_confidence

from .models import IGDBCandidate, MatchResult, MatchStatus, MatchSource

logger = logging.getLogger(__name__)

# Default thresholds
DEFAULT_AUTO_MATCH_THRESHOLD = 0.85  # 85% confidence for auto-match
DEFAULT_SEARCH_LIMIT = 5


async def search_igdb_for_game(
    igdb_service: IGDBService,
    title: str,
    limit: int = DEFAULT_SEARCH_LIMIT,
) -> List[IGDBCandidate]:
    """
    Search IGDB for games matching a title and return scored candidates.

    Args:
        igdb_service: IGDB service instance
        title: Game title to search for
        limit: Maximum number of candidates to return

    Returns:
        List of IGDBCandidate objects sorted by confidence score (highest first)
    """
    if not title or not title.strip():
        logger.debug("Empty title provided for IGDB search")
        return []

    logger.debug(f"Searching IGDB for title: '{title}'")

    try:
        # Use the existing IGDB service search functionality
        # fuzzy_threshold=0.6 is the same as the manual search to get more candidates
        game_results = await igdb_service.search_games(
            query=title.strip(),
            limit=limit,
            fuzzy_threshold=0.6,
        )

        if not game_results:
            logger.debug(f"No IGDB results found for title: '{title}'")
            return []

        # Convert to IGDBCandidate objects with confidence scores
        candidates = []
        for game in game_results:
            confidence = calculate_fuzzy_confidence(title, game.title)
            candidate = _game_metadata_to_candidate(game, confidence)
            candidates.append(candidate)

        # Sort by confidence (highest first)
        candidates.sort(key=lambda c: c.confidence_score or 0, reverse=True)

        logger.debug(
            f"Found {len(candidates)} IGDB candidates for '{title}', "
            f"top match: '{candidates[0].name}' ({candidates[0].confidence_score:.1%})"
        )

        return candidates

    except IGDBError as e:
        logger.error(f"IGDB error searching for '{title}': {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error searching IGDB for '{title}': {e}")
        raise IGDBError(f"Failed to search IGDB: {e}")


def _game_metadata_to_candidate(
    game: GameMetadata, confidence_score: float
) -> IGDBCandidate:
    """Convert GameMetadata to IGDBCandidate."""
    # Format release date
    release_date = None
    if game.release_date:
        release_date = game.release_date

    return IGDBCandidate(
        igdb_id=game.igdb_id,
        name=game.title,
        first_release_date=release_date,
        cover_url=game.cover_art_url,
        summary=game.description[:500] if game.description else None,
        platforms=game.platform_names,
        confidence_score=confidence_score,
    )


async def match_by_title(
    igdb_service: IGDBService,
    title: str,
    auto_match_threshold: float = DEFAULT_AUTO_MATCH_THRESHOLD,
    search_limit: int = DEFAULT_SEARCH_LIMIT,
    source_metadata: Optional[dict] = None,
) -> MatchResult:
    """
    Attempt to match a game by title using IGDB search.

    If the best match has confidence >= auto_match_threshold, returns MATCHED.
    If candidates exist but confidence is low, returns NEEDS_REVIEW with candidates.
    If no candidates found, returns NO_MATCH.

    Args:
        igdb_service: IGDB service instance
        title: Game title to match
        auto_match_threshold: Minimum confidence for automatic matching
        search_limit: Maximum number of candidates to retrieve
        source_metadata: Optional metadata about the source game

    Returns:
        MatchResult with status and candidate information
    """
    logger.info(f"Attempting title-based match for: '{title}'")

    try:
        candidates = await search_igdb_for_game(
            igdb_service, title, limit=search_limit
        )

        if not candidates:
            logger.info(f"No IGDB matches found for: '{title}'")
            return MatchResult(
                source_title=title,
                status=MatchStatus.NO_MATCH,
                source_metadata=source_metadata or {},
            )

        best_match = candidates[0]
        confidence = best_match.confidence_score or 0.0

        if confidence >= auto_match_threshold:
            # High confidence - automatic match
            logger.info(
                f"Auto-matched '{title}' -> '{best_match.name}' "
                f"(IGDB ID: {best_match.igdb_id}, confidence: {confidence:.1%})"
            )
            return MatchResult(
                source_title=title,
                status=MatchStatus.MATCHED,
                match_source=MatchSource.TITLE_SEARCH_AUTO,
                igdb_id=best_match.igdb_id,
                igdb_title=best_match.name,
                confidence_score=confidence,
                candidates=candidates,
                source_metadata=source_metadata or {},
            )
        else:
            # Low confidence - needs user review
            logger.info(
                f"Needs review: '{title}' -> best match '{best_match.name}' "
                f"(confidence: {confidence:.1%}, threshold: {auto_match_threshold:.1%})"
            )
            return MatchResult(
                source_title=title,
                status=MatchStatus.NEEDS_REVIEW,
                match_source=MatchSource.TITLE_SEARCH_REVIEW,
                confidence_score=confidence,
                candidates=candidates,
                source_metadata=source_metadata or {},
            )

    except IGDBError as e:
        logger.error(f"IGDB error matching '{title}': {e}")
        return MatchResult(
            source_title=title,
            status=MatchStatus.ERROR,
            error_message=str(e),
            source_metadata=source_metadata or {},
        )
    except Exception as e:
        logger.error(f"Unexpected error matching '{title}': {e}")
        return MatchResult(
            source_title=title,
            status=MatchStatus.ERROR,
            error_message=f"Unexpected error: {e}",
            source_metadata=source_metadata or {},
        )


async def get_game_by_igdb_id(
    igdb_service: IGDBService, igdb_id: int
) -> Optional[GameMetadata]:
    """
    Get game metadata by IGDB ID.

    Wrapper around IGDBService.get_game_by_id for use in matching flow.

    Args:
        igdb_service: IGDB service instance
        igdb_id: IGDB ID to look up

    Returns:
        GameMetadata if found, None otherwise
    """
    try:
        return await igdb_service.get_game_by_id(igdb_id)
    except IGDBError as e:
        logger.error(f"Failed to fetch IGDB game {igdb_id}: {e}")
        return None
