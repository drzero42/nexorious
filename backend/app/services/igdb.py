"""
IGDB API service for game metadata retrieval.
"""

import json
import logging
from typing import Optional, Dict, Any, List
from datetime import datetime, timedelta, timezone
import asyncio
import re
from dataclasses import dataclass

import httpx
from igdb.wrapper import IGDBWrapper

from app.core.config import settings
from app.services.storage import storage_service
from app.utils.rate_limiter import (
    RateLimitConfig, 
    create_igdb_rate_limiter,
    RateLimitExceeded
)


logger = logging.getLogger(__name__)

# IGDB Platform ID to internal platform name mapping
# Based on IGDB platform IDs from their API documentation
IGDB_PLATFORM_MAPPING = {
    6: "pc-windows",         # PC (Microsoft Windows)
    3: "pc-windows",         # Linux (map to PC Windows for simplicity) 
    14: "pc-windows",        # Mac (map to PC Windows for simplicity)
    48: "playstation-4",     # PlayStation 4
    167: "playstation-5",    # PlayStation 5
    9: "playstation-3",      # PlayStation 3
    49: "xbox-one",          # Xbox One
    169: "xbox-series",      # Xbox Series X|S
    12: "xbox-360",          # Xbox 360
    130: "nintendo-switch",  # Nintendo Switch
    5: "nintendo-wii",       # Nintendo Wii
    39: "ios",               # iOS
    34: "android",           # Android
}

# Keyword expansions for search query enhancement
KEYWORD_EXPANSIONS = {
    "goty": "Game of the Year",
    "The Telltale Series": "",  # Remove this phrase from queries
    "®": "",  # Remove registered trademark symbol
    "(classic)": "",  # Remove (classic) from queries (case insensitive)
    ":": " ",  # Replace colon with space
    # Pattern-based keywords (special keys that trigger regex patterns)
    "_pattern_year_parentheses": "",  # Remove years in parentheses like (2023)
    "_pattern_standalone_one": "",   # Remove standalone number "1" (avoiding version numbers and episodes)
}


@dataclass
class GameMetadata:
    """Structured game metadata from IGDB."""
    
    igdb_id: int
    title: str
    igdb_slug: Optional[str] = None
    description: Optional[str] = None
    genre: Optional[str] = None
    developer: Optional[str] = None
    publisher: Optional[str] = None
    release_date: Optional[str] = None
    cover_art_url: Optional[str] = None
    rating_average: Optional[float] = None
    rating_count: Optional[int] = None
    estimated_playtime_hours: Optional[int] = None
    # How Long to Beat data from IGDB (hastily, normally, completely) - stored in hours
    hastily: Optional[int] = None
    normally: Optional[int] = None
    completely: Optional[int] = None
    # Platform data from IGDB
    igdb_platform_ids: Optional[List[int]] = None
    platform_names: Optional[List[str]] = None


class TwitchAuthError(Exception):
    """Exception for Twitch authentication errors."""
    pass


class IGDBError(Exception):
    """Exception for IGDB API errors."""
    pass


def map_igdb_time_to_beat_to_db_fields(igdb_time_data: Dict[str, Any]) -> Dict[str, Optional[int]]:
    """Map IGDB time-to-beat fields to our database fields.
    
    Note: This function expects igdb_time_data to already be converted to hours.
    The conversion from IGDB's seconds to hours should happen in _get_time_to_beat_data().
    """
    return {
        "howlongtobeat_main": igdb_time_data.get("hastily"),
        "howlongtobeat_extra": igdb_time_data.get("normally"),
        "howlongtobeat_completionist": igdb_time_data.get("completely")
    }


class IGDBService:
    """Service for interacting with IGDB API."""
    
    def __init__(self):
        self.client_id = settings.igdb_client_id
        self.client_secret = settings.igdb_client_secret
        self._access_token = settings.igdb_access_token
        self._token_expires_at: Optional[datetime] = None
        self._wrapper: Optional[IGDBWrapper] = None
        self._http_client = httpx.AsyncClient()
        
        # Initialize rate limiter with settings
        rate_config = RateLimitConfig(
            requests_per_second=settings.igdb_requests_per_second,
            burst_capacity=settings.igdb_burst_capacity,
            backoff_factor=settings.igdb_backoff_factor,
            max_retries=settings.igdb_max_retries
        )
        self._rate_limiter = create_igdb_rate_limiter(rate_config)
        
        logger.info(
            f"IGDB service initialized with rate limiting: {rate_config.requests_per_second} req/s, "
            f"burst: {rate_config.burst_capacity}, retries: {rate_config.max_retries}"
        )
        
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
        try:
            wrapper = await self._get_wrapper()
            
            # Create the API request function
            async def make_request():
                loop = asyncio.get_event_loop()
                return await loop.run_in_executor(
                    None,
                    lambda: wrapper.api_request(endpoint, query)
                )
            
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
    
    async def _get_access_token(self) -> str:
        """Get or refresh Twitch access token using client credentials flow."""
        if not self.client_id or not self.client_secret:
            logger.error("IGDB client ID and secret not configured")
            raise TwitchAuthError("IGDB client ID and secret must be configured")
        
        # Check if current token is still valid
        if (self._access_token and 
            self._token_expires_at and 
            datetime.now(timezone.utc) < self._token_expires_at - timedelta(minutes=5)):
            logger.debug(f"Using existing access token (expires at {self._token_expires_at})")
            return self._access_token
        
        logger.info("Requesting new Twitch access token")
        logger.debug(f"Using client ID: {self.client_id[:8]}...")
        
        try:
            response = await self._http_client.post(
                "https://id.twitch.tv/oauth2/token",
                data={
                    "client_id": self.client_id,
                    "client_secret": self.client_secret,
                    "grant_type": "client_credentials"
                }
            )
            
            logger.debug(f"Twitch auth response status: {response.status_code}")
            response.raise_for_status()
            
            token_data = response.json()
            self._access_token = token_data["access_token"]
            expires_in = token_data.get("expires_in", 3600)  # Default to 1 hour
            self._token_expires_at = datetime.now(timezone.utc) + timedelta(seconds=expires_in)
            
            logger.info(f"Successfully obtained Twitch access token, expires at {self._token_expires_at}")
            logger.debug(f"Token preview: {self._access_token[:10]}...")
            return self._access_token
            
        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting Twitch access token: {e}")
            logger.debug(f"Response body: {e.response.text}")
            raise TwitchAuthError(f"Failed to authenticate with Twitch: {e}")
        except httpx.HTTPError as e:
            logger.error(f"HTTP error getting Twitch access token: {e}")
            raise TwitchAuthError(f"Failed to authenticate with Twitch: {e}")
        except Exception as e:
            logger.error(f"Unexpected error getting access token: {e}", exc_info=True)
            raise TwitchAuthError(f"Failed to authenticate with Twitch: {e}")
    
    async def _get_wrapper(self) -> IGDBWrapper:
        """Get initialized IGDB wrapper with valid access token."""
        if not self._wrapper:
            if not self.client_id:
                logger.error("IGDB client ID is required for wrapper initialization")
                raise TwitchAuthError("IGDB client ID is required")
            
            logger.debug("Initializing IGDB wrapper")
            access_token = await self._get_access_token()
            if not access_token:
                logger.error("Failed to obtain valid access token for IGDB wrapper")
                raise TwitchAuthError("Failed to obtain valid access token")
            
            logger.debug("Creating IGDBWrapper instance")
            self._wrapper = IGDBWrapper(self.client_id, access_token)
            
        return self._wrapper
    
    async def search_games(self, query: str, limit: int = 10, fuzzy_threshold: float = 0.6) -> List[GameMetadata]:
        """Search for games by title with fuzzy matching and keyword expansion."""
        if not query.strip():
            logger.debug("Empty search query provided")
            return []
        
        logger.info(f"Starting IGDB search for query: '{query}' (limit: {limit}, threshold: {fuzzy_threshold})")
        
        # Check for keywords that need expansion
        detected_keywords = self._detect_keywords(query)
        
        try:
            # Always perform original search
            original_results = await self._perform_single_search(query.strip(), limit * 2)
            
            # If keywords detected, perform expanded searches
            expanded_results = []
            if detected_keywords:
                logger.info(f"Keywords detected in query '{query}': {list(detected_keywords.keys())}")
                expanded_queries = self._generate_expanded_queries(query, detected_keywords)
                
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
                merged_games = self._merge_and_deduplicate_results(original_results, expanded_results, limit * 2)
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
                   first_release_date, cover.image_id, rating, rating_count, platforms.id, platforms.name;
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
            
            metadata = self._parse_game_data(game_data)
            if metadata:
                # Note: Time-to-beat data is not fetched during search for performance reasons
                # It will be fetched later during actual game import via get_game_by_id()
                games.append(metadata)
            else:
                logger.debug(f"Failed to parse game data for item {i+1}")
        
        logger.debug(f"Successfully parsed {len(games)} valid games")
        return games
    
    def _rank_games_by_fuzzy_match(self, games: List[GameMetadata], query: str, threshold: float = 0.6) -> List[GameMetadata]:
        """Rank games by fuzzy matching similarity to query."""
        if not games or not query.strip():
            return games
        
        query.lower().strip()
        
        # Calculate similarity scores for each game using shared fuzzy matching logic
        from app.utils.fuzzy_match import calculate_fuzzy_confidence
        scored_games = []
        for game in games:
            # Calculate confidence score using sophisticated multi-metric fuzzy matching
            final_score = calculate_fuzzy_confidence(query, game.title)
            
            # Only include games above threshold
            if final_score >= threshold:
                scored_games.append((game, final_score))
        
        # Sort by score (descending) and return games
        scored_games.sort(key=lambda x: x[1], reverse=True)
        
        logger.debug(f"Fuzzy matching results for '{query}': {[(g.title, s) for g, s in scored_games[:5]]}")
        
        return [game for game, score in scored_games]
    
    async def get_game_by_id(self, igdb_id: int) -> Optional[GameMetadata]:
        """Get game metadata by IGDB ID."""
        logger.debug(f"Fetching game metadata from IGDB for ID {igdb_id}")
        
        try:
            igdb_query = f'''
                fields id, name, slug, summary, genres.name, involved_companies.company.name, 
                       involved_companies.developer, involved_companies.publisher, 
                       first_release_date, cover.image_id, rating, rating_count, platforms.id, platforms.name;
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
            game_metadata = self._parse_game_data(games_data[0])
            
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
    
    def _parse_game_data(self, game_data: Dict[str, Any]) -> Optional[GameMetadata]:
        """Parse IGDB game data into GameMetadata object."""
        try:
            game_id = game_data.get('id')
            game_name = game_data.get('name', 'Unknown')
            
            if not game_id:
                logger.warning(f"Game data missing required 'id' field for {game_name}")
                return None
                
            logger.debug(f"Parsing game data for {game_name} (ID: {game_id})")
            
            # Extract genres
            genres = game_data.get('genres', [])
            genre_names = [g.get('name') for g in genres if g.get('name')]
            genre = ', '.join(genre_names) if genre_names else None
            
            # Extract developer and publisher
            companies = game_data.get('involved_companies', [])
            developer = None
            publisher = None
            
            for company in companies:
                company_name = company.get('company', {}).get('name')
                if company_name:
                    if company.get('developer'):
                        developer = company_name
                    if company.get('publisher'):
                        publisher = company_name
            
            # Extract release date
            release_date = None
            if 'first_release_date' in game_data:
                try:
                    release_timestamp = game_data['first_release_date']
                    release_date = datetime.fromtimestamp(release_timestamp).strftime('%Y-%m-%d')
                    logger.debug(f"Parsed release date for {game_name}: {release_date}")
                except (ValueError, OSError) as e:
                    logger.warning(f"Invalid release date timestamp for {game_name}: {e}")
            
            # Extract cover art URL
            cover_art_url = None
            if 'cover' in game_data and 'image_id' in game_data['cover']:
                image_id = game_data['cover']['image_id']
                cover_art_url = f"https://images.igdb.com/igdb/image/upload/t_cover_big/{image_id}.jpg"
                logger.debug(f"Generated cover art URL for {game_name}: {cover_art_url}")
            
            # Extract platform data
            igdb_platform_ids = []
            platform_names = []
            platforms = game_data.get('platforms', [])
            for platform in platforms:
                platform_id = platform.get('id')
                if platform_id and platform_id in IGDB_PLATFORM_MAPPING:
                    igdb_platform_ids.append(platform_id)
                    platform_names.append(IGDB_PLATFORM_MAPPING[platform_id])
                elif platform_id:
                    logger.debug(f"Unknown platform ID {platform_id} for game {game_name}")
            
            # Only set platform data if we found any mapped platforms
            igdb_platform_ids = igdb_platform_ids if igdb_platform_ids else None
            platform_names = platform_names if platform_names else None
            
            logger.debug(f"Successfully parsed {game_name}: genres={genre}, platforms={len(platform_names) if platform_names else 0}")
            
            return GameMetadata(
                igdb_id=game_id,
                title=game_name,
                igdb_slug=game_data.get('slug'),
                description=game_data.get('summary'),
                genre=genre,
                developer=developer,
                publisher=publisher,
                release_date=release_date,
                cover_art_url=cover_art_url,
                rating_average=game_data.get('rating'),
                rating_count=game_data.get('rating_count'),
                estimated_playtime_hours=None,  # IGDB doesn't provide this directly
                # Time-to-beat data will be populated separately
                hastily=None,
                normally=None,
                completely=None,
                # Platform data from IGDB
                igdb_platform_ids=igdb_platform_ids,
                platform_names=platform_names
            )
            
        except KeyError as e:
            logger.error(f"Missing required field in game data: {e}")
            logger.debug(f"Game data keys available: {list(game_data.keys())}")
            return None
        except Exception as e:
            logger.error(f"Unexpected error parsing game data: {e}", exc_info=True)
            logger.debug(f"Problematic game data: {game_data}")
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
    
    def get_rate_limiter_status(self) -> dict:
        """
        Get current rate limiter status for monitoring.
        
        Returns:
            Dictionary with rate limiter status information
        """
        return self._rate_limiter.get_status()
    
    def _detect_pattern_keyword(self, pattern_key: str, expansion: str, query: str) -> Dict[str, str]:
        """
        Detect pattern-based keywords in search query.
        
        Args:
            pattern_key: Pattern key from KEYWORD_EXPANSIONS (e.g., "_pattern_year_parentheses")
            expansion: Expansion string for the pattern
            query: Search query string
            
        Returns:
            Dictionary mapping found pattern instances to their expansions
        """
        detected = {}
        
        if pattern_key == "_pattern_year_parentheses":
            # Detect years in parentheses (4 digits)
            year_pattern = r'\(\d{4}\)'
            matches = re.findall(year_pattern, query)
            for match in matches:
                detected[match] = expansion
                logger.debug(f"Detected year pattern '{match}' in query '{query}'")
                
        elif pattern_key == "_pattern_standalone_one":
            # Detect standalone number "1" but avoid Episode/Chapter/Part/Version references
            # Improved pattern to avoid false positives like "Episode 1", "Chapter 1", etc.
            # Look for "1" that is:
            # - Word boundary at start
            # - Not followed by a dot or dash (version numbers)
            # - Not preceded by Episode/Chapter/Part/Season/Vol/Volume (case insensitive)
            if re.search(r'\b1\b', query) and not re.search(r'\b1[\.\-]', query):
                # Check if it's NOT preceded by episode/chapter type words
                episode_pattern = r'\b(?:episode|chapter|part|season|vol|volume|series)\s+1\b'
                if not re.search(episode_pattern, query, re.IGNORECASE):
                    # Find the actual match for replacement (include trailing space if present)
                    match = re.search(r'\b1\s*(?=:|$)|\b1\s+', query)
                    if match:
                        matched_text = match.group()
                        detected[matched_text] = expansion
                        logger.debug(f"Detected standalone '1' pattern '{matched_text}' in query '{query}'")
        
        return detected
    
    def _detect_keywords(self, query: str) -> Dict[str, str]:
        """
        Detect keywords in search query that need expansion.
        
        Args:
            query: Search query string
            
        Returns:
            Dictionary mapping found keywords/patterns to their expansions
        """
        detected = {}
        query_lower = query.lower()
        
        for keyword, expansion in KEYWORD_EXPANSIONS.items():
            # Check if this is a pattern-based keyword
            if keyword.startswith('_pattern_'):
                detected.update(self._detect_pattern_keyword(keyword, expansion, query))
            # Special case for (classic) - make it case insensitive
            elif keyword == "(classic)":
                pattern = re.escape(keyword)
                if re.search(pattern, query, re.IGNORECASE):
                    # Find the actual match in the original query to preserve case for replacement
                    match = re.search(pattern, query, re.IGNORECASE)
                    if match:
                        actual_match = match.group()
                        detected[actual_match] = expansion
                        logger.debug(f"Detected case-insensitive '{keyword}' as '{actual_match}' in query '{query}'")
            # Check if keyword is a symbol (no alphanumeric characters)
            elif re.match(r'^[^\w\s]+$', keyword):
                # For symbols, use simple pattern without word boundaries
                pattern = re.escape(keyword)
                if re.search(pattern, query):  # Use original query for case-sensitive symbols
                    detected[keyword] = expansion
                    logger.debug(f"Detected symbol '{keyword}' in query '{query}'")
            else:
                # Use word boundaries to match whole words only for text keywords
                pattern = r'\b' + re.escape(keyword.lower()) + r'\b'
                if re.search(pattern, query_lower):
                    detected[keyword] = expansion
                    logger.debug(f"Detected keyword '{keyword}' in query '{query}'")
        
        return detected
    
    def _generate_expanded_queries(self, original_query: str, detected_keywords: Dict[str, str]) -> List[str]:
        """
        Generate expanded queries by replacing keywords with their expansions.
        
        Args:
            original_query: Original search query
            detected_keywords: Dictionary of detected keywords and their expansions
            
        Returns:
            List of expanded query strings
        """
        expanded_queries = []
        
        # Generate individual transformations (one per keyword)
        for keyword, expansion in detected_keywords.items():
            expanded_query = self._apply_single_transformation(original_query, keyword, expansion)
            expanded_queries.append(expanded_query)
            action = "Removed" if expansion == "" else "Expanded"
            logger.debug(f"Generated {action.lower()} query: '{expanded_query}' from keyword '{keyword}'")
        
        # Generate fully transformed query (all keywords applied together) if multiple keywords
        if len(detected_keywords) > 1:
            fully_transformed = original_query
            for keyword, expansion in detected_keywords.items():
                fully_transformed = self._apply_single_transformation(fully_transformed, keyword, expansion)
            
            # Only add if it's different from individual transformations (avoid duplicates)
            if fully_transformed not in expanded_queries and fully_transformed != original_query:
                expanded_queries.append(fully_transformed)
                logger.debug(f"Generated fully transformed query: '{fully_transformed}'")
        
        return expanded_queries
    
    def _apply_single_transformation(self, query: str, keyword: str, expansion: str) -> str:
        """
        Apply a single keyword transformation to a query.
        
        Args:
            query: Query to transform
            keyword: Keyword to replace
            expansion: Replacement text
            
        Returns:
            Transformed query string
        """
        # Check if keyword is a year in parentheses pattern
        if keyword.startswith('(') and keyword.endswith(')') and re.match(r'\(\d{4}\)', keyword):
            # For year patterns like (2023), use exact pattern matching
            pattern = re.escape(keyword)
            expanded_query = re.sub(pattern, expansion, query)
        # Check if keyword contains trailing space (from standalone "1" detection)
        elif keyword.endswith(' ') or keyword.endswith('\t'):
            # For patterns that include whitespace, use exact replacement
            expanded_query = query.replace(keyword, expansion)
        # Check if keyword is (classic) variants - case-insensitive matches need exact replacement
        elif keyword.lower() == "(classic)":
            # For case-insensitive (classic) matches, use exact replacement of the found match
            expanded_query = query.replace(keyword, expansion)
        # Check if keyword is a symbol (no alphanumeric characters)
        elif re.match(r'^[^\w\s]+$', keyword):
            # For symbols, use simple pattern without word boundaries
            pattern = re.escape(keyword)
            expanded_query = re.sub(pattern, expansion, query)  # Case-sensitive for symbols
        else:
            # Replace keyword with expansion (case-insensitive) for text keywords
            pattern = r'\b' + re.escape(keyword) + r'\b'
            expanded_query = re.sub(pattern, expansion, query, flags=re.IGNORECASE)
        
        # Clean up extra whitespace for all expansions
        expanded_query = re.sub(r'\s+', ' ', expanded_query)  # Multiple spaces -> single space
        expanded_query = expanded_query.strip()  # Remove leading/trailing whitespace
        
        # Additional cleanup for removals (empty expansions)
        if expansion == "":
            expanded_query = re.sub(r':\s+:', ':', expanded_query)  # ": :" -> ":"
            expanded_query = re.sub(r'\s+:', ':', expanded_query)  # "word :" -> "word:"
            expanded_query = re.sub(r':\s*$', '', expanded_query)  # Remove trailing ":"
        
        return expanded_query
    
    def _merge_and_deduplicate_results(
        self, 
        original_results: List[GameMetadata], 
        expanded_results: List[List[GameMetadata]], 
        limit: int
    ) -> List[GameMetadata]:
        """
        Merge and deduplicate search results, prioritizing original query results.
        
        Args:
            original_results: Results from original query
            expanded_results: List of results from expanded queries
            limit: Maximum number of results to return
            
        Returns:
            Merged and deduplicated results
        """
        seen_ids = set()
        merged = []
        
        # Add original results first (highest priority)
        for game in original_results:
            if game.igdb_id not in seen_ids:
                merged.append(game)
                seen_ids.add(game.igdb_id)
                if len(merged) >= limit:
                    break
        
        # Add expanded results for unseen games
        for expanded_list in expanded_results:
            for game in expanded_list:
                if game.igdb_id not in seen_ids and len(merged) < limit:
                    merged.append(game)
                    seen_ids.add(game.igdb_id)
                    if len(merged) >= limit:
                        break
            if len(merged) >= limit:
                break
        
        logger.debug(f"Merged results: {len(merged)} games from {len(original_results)} original + "
                    f"{sum(len(r) for r in expanded_results)} expanded results")
        
        return merged[:limit]