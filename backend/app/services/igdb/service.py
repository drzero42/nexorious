"""
IGDB service module.

Main facade for all IGDB API interactions.
"""

import json
import logging
import asyncio
from typing import Optional, Dict, Any, List, Union

import httpx

from app.core.config import settings
from app.services.storage import storage_service
from app.utils.rate_limiter import (
    RateLimitConfig,
    RateLimitedClient,
    DistributedRateLimitedClient,
    create_igdb_rate_limiter,
    create_distributed_igdb_rate_limiter,
    RateLimitExceeded
)
from app.utils.nats_client import get_nats_client

from .models import GameMetadata, IGDBError
from .auth import IGDBAuthManager
from .parser import parse_game_data
from .search import (
    detect_keywords,
    generate_expanded_queries,
    rank_games_by_fuzzy_match,
    merge_and_deduplicate_results
)


logger = logging.getLogger(__name__)


class IGDBService:
    """Service for interacting with IGDB API."""

    _rate_limiter: Union[RateLimitedClient, DistributedRateLimitedClient, None]

    def __init__(
        self,
        rate_limiter: Optional[Union[RateLimitedClient, DistributedRateLimitedClient]] = None
    ):
        self._http_client = httpx.AsyncClient()
        self._auth_manager = IGDBAuthManager(self._http_client)
        self._rate_limiter = rate_limiter
        self._rate_limiter_initialized = rate_limiter is not None

        # Backwards compatibility: expose wrapper reference
        self._wrapper: Any = None  # Lazily set by _auth_manager.get_wrapper()

    async def _ensure_rate_limiter(self):
        """Ensure rate limiter is initialized."""
        if self._rate_limiter_initialized:
            return

        # Create distributed rate limiter
        rate_config = RateLimitConfig(
            requests_per_second=settings.igdb_requests_per_second,
            burst_capacity=settings.igdb_burst_capacity,
            backoff_factor=settings.igdb_backoff_factor,
            max_retries=settings.igdb_max_retries
        )

        try:
            nats_client = await get_nats_client()
            self._rate_limiter = await create_distributed_igdb_rate_limiter(
                nats_client=nats_client,
                config=rate_config,
                bucket_name=settings.rate_limiter_nats_bucket,
                max_cas_retries=settings.rate_limiter_cas_max_retries,
                cas_retry_base_ms=settings.rate_limiter_cas_retry_base_ms,
                cas_retry_max_ms=settings.rate_limiter_cas_retry_max_ms,
            )
            logger.info(
                f"IGDB service initialized with distributed rate limiting: "
                f"{rate_config.requests_per_second} req/s, burst: {rate_config.burst_capacity}"
            )
        except Exception as e:
            logger.warning(f"Failed to create distributed rate limiter, falling back to local: {e}")
            self._rate_limiter = create_igdb_rate_limiter(rate_config)
            logger.info(
                f"IGDB service initialized with local rate limiting: "
                f"{rate_config.requests_per_second} req/s, burst: {rate_config.burst_capacity}"
            )

        self._rate_limiter_initialized = True

    @property
    def client_id(self):
        """Backwards compatibility: expose client_id from auth manager."""
        return self._auth_manager.client_id

    async def _get_wrapper(self):
        """Backwards compatibility: expose wrapper getter from auth manager."""
        wrapper = await self._auth_manager.get_wrapper()
        self._wrapper = wrapper  # Update internal reference
        return wrapper

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self._http_client.aclose()

    async def _rate_limited_api_request(self, endpoint: str, query: str) -> bytes:
        """
        Make a rate-limited IGDB API request.

        Args:
            endpoint: IGDB API endpoint (e.g., 'games', 'game_time_to_beats')
            query: IGDB query string

        Returns:
            Raw response bytes from IGDB API

        Raises:
            IGDBError: If API request fails
            RateLimitExceeded: If rate limit cannot be satisfied
        """
        await self._ensure_rate_limiter()

        try:
            wrapper = await self._get_wrapper()

            # Create the API request function
            async def make_request() -> bytes:
                loop = asyncio.get_event_loop()

                def _sync_request() -> bytes:
                    return wrapper.api_request(endpoint, query)  # type: ignore[return-value]

                return await loop.run_in_executor(None, _sync_request)  # type: ignore[arg-type]

            # Execute with rate limiting
            logger.debug(f"Making rate-limited IGDB API request to {endpoint}")
            response = await self._rate_limiter.call(make_request)

            logger.debug(f"IGDB API request successful: {len(response)} bytes received")
            return response

        except RateLimitExceeded as e:
            logger.error(f"IGDB rate limit exceeded for {endpoint}: {str(e)}")
            raise IGDBError(f"Rate limit exceeded for IGDB API: {str(e)}")
        except Exception as e:
            logger.error(f"IGDB API request failed for {endpoint}: {str(e)}")
            raise IGDBError(f"IGDB API request failed: {str(e)}")

    async def search_games(self, query: str, limit: int = 10, fuzzy_threshold: float = 0.6) -> List[GameMetadata]:
        """Search for games by title with fuzzy matching and keyword expansion."""
        if not query.strip():
            logger.debug("Empty search query provided")
            return []

        logger.info(f"Starting IGDB search for query: '{query}' (limit: {limit}, threshold: {fuzzy_threshold})")

        # Check for keywords that need expansion
        detected_keywords = detect_keywords(query)

        try:
            # Normalize the query before IGDB lookup to improve matching
            # e.g., "The Witcher® 3: Wild Hunt - GOTY Edition" -> "the witcher 3 wild hunt game of the year edition"
            from app.utils.normalize_title import normalize_title
            normalized_query = normalize_title(query)

            # Run fuzzy search and exact-name search in parallel
            # Exact-name search ensures we catch exact matches that IGDB's fuzzy search might rank lower
            fuzzy_task = self._perform_single_search(normalized_query, limit * 2)
            exact_task = self._search_by_exact_name(normalized_query, limit)

            original_results, exact_results = await asyncio.gather(fuzzy_task, exact_task)

            # Merge exact-name results into original results (exact matches first, deduplicated)
            if exact_results:
                logger.debug(f"Exact-name search found {len(exact_results)} results for '{query}'")
                seen_ids = set()
                merged_with_exact = []

                # Add exact matches first
                for game in exact_results:
                    if game.igdb_id not in seen_ids:
                        merged_with_exact.append(game)
                        seen_ids.add(game.igdb_id)

                # Add remaining fuzzy results
                for game in original_results:
                    if game.igdb_id not in seen_ids:
                        merged_with_exact.append(game)
                        seen_ids.add(game.igdb_id)

                original_results = merged_with_exact
                logger.debug(f"After merging exact-name results: {len(original_results)} unique games")

            # If keywords detected, perform expanded searches
            expanded_results = []
            if detected_keywords:
                logger.info(f"Keywords detected in query '{query}': {list(detected_keywords.keys())}")
                expanded_queries = generate_expanded_queries(query, detected_keywords)

                # Execute expanded searches concurrently
                logger.debug(f"Executing {len(expanded_queries)} expanded searches concurrently")
                expanded_search_tasks = [
                    self._perform_single_search(exp_query, limit * 2)
                    for exp_query in expanded_queries
                ]

                expanded_search_results = await asyncio.gather(*expanded_search_tasks, return_exceptions=True)

                # Filter out failed searches and collect successful results
                for i, result in enumerate(expanded_search_results):
                    if isinstance(result, Exception):
                        logger.warning(f"Expanded search failed for query '{expanded_queries[i]}': {result}")
                    elif isinstance(result, list):
                        expanded_results.append(result)
                        logger.debug(f"Expanded search for '{expanded_queries[i]}' returned {len(result)} results")

            # Merge and deduplicate results
            if expanded_results:
                merged_games = merge_and_deduplicate_results(original_results, expanded_results, limit * 2)
                logger.info(f"Merged {len(original_results)} original + {sum(len(r) for r in expanded_results)} expanded results into {len(merged_games)} unique games")
            else:
                merged_games = original_results
                logger.debug(f"No keyword expansion performed, using {len(merged_games)} original results")

            # Apply fuzzy matching and ranking
            if merged_games:
                logger.debug("Applying fuzzy matching and ranking to merged results")
                merged_games = self._rank_games_by_fuzzy_match(merged_games, query, fuzzy_threshold)
                merged_games = merged_games[:limit]  # Limit results after ranking
                logger.debug(f"After fuzzy matching and limiting: {len(merged_games)} games")

            logger.info(f"Successfully found {len(merged_games)} games for query: '{query}'" +
                       (f" (with {len(detected_keywords)} keyword expansions)" if detected_keywords else ""))
            return merged_games

        except IGDBError:
            # Re-raise IGDB errors as-is
            raise
        except Exception as e:
            logger.error(f"Unexpected error during IGDB search for query '{query}': {e}", exc_info=True)
            raise IGDBError(f"Failed to search games: {e}")

    async def _perform_single_search(self, query: str, limit: int) -> List[GameMetadata]:
        """Perform a single IGDB search and return GameMetadata objects."""
        # Build IGDB query
        igdb_query = f'''
            fields id, name, slug, summary, genres.name, involved_companies.company.name,
                   involved_companies.developer, involved_companies.publisher,
                   first_release_date, cover.image_id, rating, rating_count, platforms.id, platforms.name,
                   game_modes.name, themes.name, player_perspectives.name;
            search "{query}";
            limit {limit};
        '''

        logger.debug(f"IGDB query: {igdb_query.strip()}")

        # Execute rate-limited search
        logger.debug("Executing rate-limited IGDB API request")
        response = await self._rate_limited_api_request('games', igdb_query)

        logger.debug(f"IGDB API response received: {len(response)} bytes")

        # Parse JSON response
        try:
            games_data = json.loads(response.decode('utf-8'))
            logger.debug(f"Parsed {len(games_data)} games from IGDB response")
        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse IGDB response as JSON: {e}")
            logger.debug(f"Raw response (first 500 chars): {response[:500]}")
            raise IGDBError(f"Invalid JSON response from IGDB: {e}")

        # Convert to GameMetadata objects (without time-to-beat data for performance)
        games = []
        for i, game_data in enumerate(games_data):
            logger.debug(f"Processing game {i+1}/{len(games_data)}: {game_data.get('name', 'Unknown')}")

            metadata = parse_game_data(game_data)
            if metadata:
                # Note: Time-to-beat data is not fetched during search for performance reasons
                # It will be fetched later during actual game import via get_game_by_id()
                games.append(metadata)
            else:
                logger.debug(f"Failed to parse game data for item {i+1}")

        logger.debug(f"Successfully parsed {len(games)} valid games")
        return games

    async def _search_by_exact_name(self, query: str, limit: int) -> List[GameMetadata]:
        """Search IGDB for games with exact name match (case-insensitive prefix).

        Uses IGDB's where clause with ~ operator for case-insensitive prefix matching.
        This finds games that start with the exact query, helping catch exact matches
        that IGDB's fuzzy search might rank lower.

        Args:
            query: The exact game title to search for
            limit: Maximum number of results to return

        Returns:
            List of GameMetadata objects for games matching the exact name
        """
        # Escape double quotes in query to prevent IGDB query injection
        escaped_query = query.replace('"', '\\"')

        # Use ~ operator for case-insensitive match, * for prefix matching
        # This finds "The Dig" and "The Digital Asset Inquisition" for query "The Dig"
        igdb_query = f'''
            fields id, name, slug, summary, genres.name, involved_companies.company.name,
                   involved_companies.developer, involved_companies.publisher,
                   first_release_date, cover.image_id, rating, rating_count, platforms.id, platforms.name,
                   game_modes.name, themes.name, player_perspectives.name;
            where name ~ "{escaped_query}"*;
            limit {limit};
        '''

        logger.debug(f"IGDB exact-name query: {igdb_query.strip()}")

        try:
            response = await self._rate_limited_api_request('games', igdb_query)
            games_data = json.loads(response.decode('utf-8'))
            logger.debug(f"Exact-name search returned {len(games_data)} results for '{query}'")

            games = []
            for game_data in games_data:
                metadata = parse_game_data(game_data)
                if metadata:
                    games.append(metadata)

            return games

        except IGDBError as e:
            # Log but don't fail - exact-name search is supplementary
            logger.warning(f"Exact-name search failed for '{query}': {e}")
            return []
        except json.JSONDecodeError as e:
            logger.warning(f"Failed to parse exact-name search response: {e}")
            return []

    async def get_game_by_id(self, igdb_id: int) -> Optional[GameMetadata]:
        """Get game metadata by IGDB ID."""
        logger.debug(f"Fetching game metadata from IGDB for ID {igdb_id}")

        try:
            igdb_query = f'''
                fields id, name, slug, summary, genres.name, involved_companies.company.name,
                       involved_companies.developer, involved_companies.publisher,
                       first_release_date, cover.image_id, rating, rating_count, platforms.id, platforms.name,
                       game_modes.name, themes.name, player_perspectives.name;
                where id = {igdb_id};
            '''

            logger.debug(f"IGDB query for game {igdb_id}: {igdb_query.strip()}")

            response = await self._rate_limited_api_request('games', igdb_query)
            logger.debug(f"IGDB API response size for game {igdb_id}: {len(response)} bytes")

            games_data = json.loads(response.decode('utf-8'))
            logger.debug(f"IGDB returned {len(games_data)} game(s) for ID {igdb_id}")

            if not games_data:
                logger.warning(f"No game data returned from IGDB for ID {igdb_id}")
                return None

            # Get basic game data
            logger.debug(f"Parsing game data for IGDB ID {igdb_id}")
            game_metadata = parse_game_data(games_data[0])

            if game_metadata:
                logger.debug(f"Successfully parsed basic metadata for IGDB ID {igdb_id}: "
                           f"title='{game_metadata.title}', genre='{game_metadata.genre}', "
                           f"developer='{game_metadata.developer}', publisher='{game_metadata.publisher}'")

                # Fetch time-to-beat data if available
                logger.debug(f"Fetching time-to-beat data for IGDB ID {igdb_id}")
                time_to_beat_data = await self._get_time_to_beat_data(igdb_id)
                if time_to_beat_data:
                    game_metadata.hastily = time_to_beat_data.get("hastily")
                    game_metadata.normally = time_to_beat_data.get("normally")
                    game_metadata.completely = time_to_beat_data.get("completely")
                    logger.debug(f"Added time-to-beat data for IGDB ID {igdb_id}: "
                               f"hastily={game_metadata.hastily}h, normally={game_metadata.normally}h, "
                               f"completely={game_metadata.completely}h")
                else:
                    logger.debug(f"No time-to-beat data available for IGDB ID {igdb_id}")
            else:
                logger.error(f"Failed to parse game data for IGDB ID {igdb_id}")

            return game_metadata

        except Exception as e:
            logger.error(f"Error fetching game by ID {igdb_id}: {e}", exc_info=True)
            raise IGDBError(f"Failed to fetch game: {e}")

    async def _get_time_to_beat_data(self, igdb_id: int) -> Optional[Dict[str, Any]]:
        """Get time-to-beat data for a game from IGDB.

        IGDB returns time-to-beat data in seconds, but we store and display it in hours.
        This method converts the seconds to hours before returning.
        """
        logger.debug(f"Fetching time-to-beat data from IGDB for game ID {igdb_id}")

        try:
            time_query = f'''
                fields hastily, normally, completely;
                where game_id = {igdb_id};
            '''

            logger.debug(f"IGDB time-to-beat query for game {igdb_id}: {time_query.strip()}")

            response = await self._rate_limited_api_request('game_time_to_beats', time_query)
            logger.debug(f"IGDB time-to-beat response size for game {igdb_id}: {len(response)} bytes")

            time_data = json.loads(response.decode('utf-8'))
            logger.debug(f"IGDB returned {len(time_data)} time-to-beat record(s) for game ID {igdb_id}")

            if time_data:
                raw_data = time_data[0]
                logger.debug(f"Raw time-to-beat data for game {igdb_id}: {raw_data}")

                # Convert from seconds to hours (IGDB returns seconds, we store hours)
                converted_data = {}
                for field in ['hastily', 'normally', 'completely']:
                    if field in raw_data and raw_data[field] is not None:
                        # Convert seconds to hours, round to nearest integer
                        hours = round(raw_data[field] / 3600)
                        converted_data[field] = hours
                        logger.debug(f"Converted {field} for game {igdb_id}: {raw_data[field]}s -> {hours}h")
                    else:
                        converted_data[field] = None
                        logger.debug(f"No {field} data for game {igdb_id}")

                logger.debug(f"Final time-to-beat data for game {igdb_id}: {converted_data}")
                return converted_data
            else:
                logger.debug(f"No time-to-beat data available for game {igdb_id}")
                return None

        except Exception as e:
            logger.error(f"Error fetching time-to-beat data for game {igdb_id}: {e}", exc_info=True)
            return None

    async def download_cover_art(self, cover_url: str) -> Optional[bytes]:
        """Download cover art image."""
        if not cover_url:
            return None

        try:
            response = await self._http_client.get(cover_url)
            response.raise_for_status()
            return response.content

        except httpx.HTTPError as e:
            logger.error(f"Failed to download cover art from {cover_url}: {e}")
            return None

    async def download_and_store_cover_art(self, igdb_id: int, cover_url: str) -> Optional[str]:
        """Download and store cover art locally. Returns local URL on success."""
        if not cover_url or not igdb_id:
            return None

        try:
            local_url = await storage_service.download_and_store_cover_art(str(igdb_id), cover_url)
            if local_url:
                logger.info(f"Successfully stored cover art for IGDB ID {igdb_id}")
                return local_url
            return None

        except Exception as e:
            logger.error(f"Failed to store cover art for IGDB ID {igdb_id}: {e}")
            return None

    async def refresh_game_metadata(self, igdb_id: int) -> Optional[GameMetadata]:
        """Refresh game metadata from IGDB by ID."""
        if not igdb_id:
            logger.warning("Attempted to refresh metadata with empty IGDB ID")
            return None

        logger.debug(f"Starting metadata refresh for IGDB ID {igdb_id}")

        try:
            metadata = await self.get_game_by_id(igdb_id)
            if metadata:
                logger.debug(f"Successfully refreshed metadata for IGDB ID {igdb_id}: "
                           f"title='{metadata.title}', developer='{metadata.developer}', "
                           f"publisher='{metadata.publisher}', genre='{metadata.genre}'")
            else:
                logger.warning(f"No metadata returned for IGDB ID {igdb_id}")
            return metadata
        except Exception as e:
            logger.error(f"Failed to refresh metadata for IGDB ID {igdb_id}: {e}", exc_info=True)
            return None

    async def populate_missing_metadata(self, current_metadata: GameMetadata, igdb_id: int) -> Optional[GameMetadata]:
        """Populate missing fields in existing metadata with IGDB data."""
        if not igdb_id:
            return None

        try:
            # Get fresh metadata from IGDB
            fresh_metadata = await self.get_game_by_id(igdb_id)
            if not fresh_metadata:
                return None

            # Create updated metadata by filling in missing fields
            updated_metadata = GameMetadata(
                igdb_id=current_metadata.igdb_id,
                title=current_metadata.title or fresh_metadata.title,
                igdb_slug=current_metadata.igdb_slug or fresh_metadata.igdb_slug,
                description=current_metadata.description or fresh_metadata.description,
                genre=current_metadata.genre or fresh_metadata.genre,
                developer=current_metadata.developer or fresh_metadata.developer,
                publisher=current_metadata.publisher or fresh_metadata.publisher,
                release_date=current_metadata.release_date or fresh_metadata.release_date,
                cover_art_url=current_metadata.cover_art_url or fresh_metadata.cover_art_url,
                rating_average=current_metadata.rating_average or fresh_metadata.rating_average,
                rating_count=current_metadata.rating_count or fresh_metadata.rating_count,
                estimated_playtime_hours=current_metadata.estimated_playtime_hours or fresh_metadata.estimated_playtime_hours,
                hastily=current_metadata.hastily or fresh_metadata.hastily,
                normally=current_metadata.normally or fresh_metadata.normally,
                completely=current_metadata.completely or fresh_metadata.completely
            )

            return updated_metadata

        except Exception as e:
            logger.error(f"Failed to populate missing metadata for IGDB ID {igdb_id}: {e}")
            return None

    def compare_metadata(self, current: GameMetadata, fresh: GameMetadata) -> dict:
        """Compare current metadata with fresh IGDB data and return differences."""
        differences = {}

        fields_to_compare = [
            'title', 'igdb_slug', 'description', 'genre', 'developer', 'publisher',
            'release_date', 'cover_art_url', 'rating_average', 'rating_count',
            'estimated_playtime_hours', 'hastily', 'normally', 'completely'
        ]

        for field in fields_to_compare:
            current_value = getattr(current, field, None)
            fresh_value = getattr(fresh, field, None)

            if current_value != fresh_value:
                differences[field] = {
                    'current': current_value,
                    'fresh': fresh_value
                }

        return differences

    async def get_metadata_completeness(self, metadata: GameMetadata) -> dict:
        """Analyze metadata completeness and return missing fields."""
        essential_fields = ['title', 'igdb_slug', 'description', 'genre', 'developer', 'publisher', 'release_date']
        optional_fields = ['cover_art_url', 'rating_average', 'rating_count', 'estimated_playtime_hours', 'hastily', 'normally', 'completely']

        missing_essential = []
        missing_optional = []

        for field in essential_fields:
            value = getattr(metadata, field, None)
            if not value:
                missing_essential.append(field)

        for field in optional_fields:
            value = getattr(metadata, field, None)
            if not value:
                missing_optional.append(field)

        total_fields = len(essential_fields) + len(optional_fields)
        filled_fields = total_fields - len(missing_essential) - len(missing_optional)
        completeness_percentage = (filled_fields / total_fields) * 100

        return {
            'completeness_percentage': round(completeness_percentage, 1),
            'missing_essential': missing_essential,
            'missing_optional': missing_optional,
            'total_fields': total_fields,
            'filled_fields': filled_fields
        }

    async def get_rate_limiter_status(self) -> dict:
        """
        Get current rate limiter status for monitoring.

        Returns:
            Dictionary with rate limiter status information
        """
        await self._ensure_rate_limiter()

        if hasattr(self._rate_limiter, 'get_status'):
            result = self._rate_limiter.get_status()
            # Handle both sync and async get_status
            if asyncio.iscoroutine(result):
                return await result
            return result
        return {}

    # Backwards compatibility wrapper methods for internal functions
    def _detect_keywords(self, query: str):
        """Backwards compatibility wrapper for detect_keywords."""
        return detect_keywords(query)

    def _generate_expanded_queries(self, original_query: str, detected_keywords):
        """Backwards compatibility wrapper for generate_expanded_queries."""
        return generate_expanded_queries(original_query, detected_keywords)

    def _rank_games_by_fuzzy_match(self, games: List[GameMetadata], query: str, threshold: float = 0.6) -> List[GameMetadata]:
        """Backwards compatibility wrapper for rank_games_by_fuzzy_match."""
        return rank_games_by_fuzzy_match(games, query, threshold)

    def _merge_and_deduplicate_results(
        self,
        original_results: List[GameMetadata],
        expanded_results: List[List[GameMetadata]],
        limit: int
    ) -> List[GameMetadata]:
        """Backwards compatibility wrapper for merge_and_deduplicate_results."""
        return merge_and_deduplicate_results(original_results, expanded_results, limit)
