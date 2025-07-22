"""
IGDB API service for game metadata retrieval.
"""

import json
import logging
from typing import Optional, Dict, Any, List
from datetime import datetime, timedelta, timezone
import asyncio
from dataclasses import dataclass

import httpx
from igdb.wrapper import IGDBWrapper
from rapidfuzz import fuzz, process

from nexorious.core.config import settings
from nexorious.services.storage import storage_service


logger = logging.getLogger(__name__)


@dataclass
class GameMetadata:
    """Structured game metadata from IGDB."""
    
    igdb_id: str
    title: str
    description: Optional[str] = None
    genre: Optional[str] = None
    developer: Optional[str] = None
    publisher: Optional[str] = None
    release_date: Optional[str] = None
    cover_art_url: Optional[str] = None
    rating_average: Optional[float] = None
    rating_count: Optional[int] = None
    estimated_playtime_hours: Optional[int] = None
    # How Long to Beat data from IGDB (hastily, normally, completely)
    hastily: Optional[int] = None
    normally: Optional[int] = None
    completely: Optional[int] = None


class TwitchAuthError(Exception):
    """Exception for Twitch authentication errors."""
    pass


class IGDBError(Exception):
    """Exception for IGDB API errors."""
    pass


def map_igdb_time_to_beat_to_db_fields(igdb_time_data: Dict[str, Any]) -> Dict[str, Optional[int]]:
    """Map IGDB time-to-beat fields to our database fields."""
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
        
    async def __aenter__(self):
        return self
        
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self._http_client.aclose()
    
    async def _get_access_token(self) -> str:
        """Get or refresh Twitch access token using client credentials flow."""
        if not self.client_id or not self.client_secret:
            raise TwitchAuthError("IGDB client ID and secret must be configured")
        
        # Check if current token is still valid
        if (self._access_token and 
            self._token_expires_at and 
            datetime.now(timezone.utc) < self._token_expires_at - timedelta(minutes=5)):
            return self._access_token
        
        logger.info("Requesting new Twitch access token")
        
        try:
            response = await self._http_client.post(
                "https://id.twitch.tv/oauth2/token",
                data={
                    "client_id": self.client_id,
                    "client_secret": self.client_secret,
                    "grant_type": "client_credentials"
                }
            )
            response.raise_for_status()
            
            token_data = response.json()
            self._access_token = token_data["access_token"]
            expires_in = token_data.get("expires_in", 3600)  # Default to 1 hour
            self._token_expires_at = datetime.now(timezone.utc) + timedelta(seconds=expires_in)
            
            logger.info(f"Successfully obtained Twitch access token, expires at {self._token_expires_at}")
            return self._access_token
            
        except httpx.HTTPError as e:
            logger.error(f"Failed to get Twitch access token: {e}")
            raise TwitchAuthError(f"Failed to authenticate with Twitch: {e}")
    
    async def _get_wrapper(self) -> IGDBWrapper:
        """Get initialized IGDB wrapper with valid access token."""
        if not self._wrapper:
            if not self.client_id:
                raise TwitchAuthError("IGDB client ID is required")
            
            access_token = await self._get_access_token()
            if not access_token:
                raise TwitchAuthError("Failed to obtain valid access token")
            
            self._wrapper = IGDBWrapper(self.client_id, access_token)
        return self._wrapper
    
    async def search_games(self, query: str, limit: int = 10, fuzzy_threshold: float = 0.6) -> List[GameMetadata]:
        """Search for games by title with fuzzy matching."""
        if not query.strip():
            return []
        
        try:
            wrapper = await self._get_wrapper()
            
            # First, try exact/close search with IGDB's built-in search
            igdb_query = f'''
                fields id, name, summary, genres.name, involved_companies.company.name, 
                       involved_companies.developer, involved_companies.publisher, 
                       first_release_date, cover.image_id, rating, rating_count;
                search "{query.strip()}";
                limit {limit * 2};
            '''
            
            # Execute search in thread pool to avoid blocking
            loop = asyncio.get_event_loop()
            response = await loop.run_in_executor(
                None, 
                lambda: wrapper.api_request('games', igdb_query)
            )
            
            # Parse JSON response
            games_data = json.loads(response.decode('utf-8'))
            
            # Convert to GameMetadata objects and fetch time-to-beat data
            games = []
            for game_data in games_data:
                metadata = self._parse_game_data(game_data)
                if metadata:
                    # Fetch time-to-beat data for each game
                    time_to_beat_data = await self._get_time_to_beat_data(metadata.igdb_id)
                    if time_to_beat_data:
                        metadata.hastily = time_to_beat_data.get("hastily")
                        metadata.normally = time_to_beat_data.get("normally")
                        metadata.completely = time_to_beat_data.get("completely")
                    games.append(metadata)
            
            # Apply fuzzy matching and ranking
            if games:
                games = self._rank_games_by_fuzzy_match(games, query, fuzzy_threshold)
                games = games[:limit]  # Limit results after ranking
            
            logger.info(f"Found {len(games)} games for query: {query}")
            return games
            
        except Exception as e:
            logger.error(f"Error searching games: {e}")
            raise IGDBError(f"Failed to search games: {e}")
    
    def _rank_games_by_fuzzy_match(self, games: List[GameMetadata], query: str, threshold: float = 0.6) -> List[GameMetadata]:
        """Rank games by fuzzy matching similarity to query."""
        if not games or not query.strip():
            return games
        
        query_lower = query.lower().strip()
        
        # Calculate similarity scores for each game
        scored_games = []
        for game in games:
            # Calculate multiple similarity scores
            title_lower = game.title.lower()
            
            # Different matching strategies
            exact_score = 1.0 if query_lower == title_lower else 0.0
            ratio_score = fuzz.ratio(query_lower, title_lower) / 100.0
            partial_score = fuzz.partial_ratio(query_lower, title_lower) / 100.0
            token_sort_score = fuzz.token_sort_ratio(query_lower, title_lower) / 100.0
            token_set_score = fuzz.token_set_ratio(query_lower, title_lower) / 100.0
            
            # Calculate weighted final score
            final_score = max(
                exact_score * 1.0,  # Exact match gets highest priority
                ratio_score * 0.9,  # Overall similarity
                partial_score * 0.8,  # Partial match
                token_sort_score * 0.7,  # Token order similarity
                token_set_score * 0.6  # Token set similarity
            )
            
            # Only include games above threshold
            if final_score >= threshold:
                scored_games.append((game, final_score))
        
        # Sort by score (descending) and return games
        scored_games.sort(key=lambda x: x[1], reverse=True)
        
        logger.debug(f"Fuzzy matching results for '{query}': {[(g.title, s) for g, s in scored_games[:5]]}")
        
        return [game for game, score in scored_games]
    
    async def get_game_by_id(self, igdb_id: str) -> Optional[GameMetadata]:
        """Get game metadata by IGDB ID."""
        try:
            wrapper = await self._get_wrapper()
            
            igdb_query = f'''
                fields id, name, summary, genres.name, involved_companies.company.name, 
                       involved_companies.developer, involved_companies.publisher, 
                       first_release_date, cover.image_id, rating, rating_count;
                where id = {igdb_id};
            '''
            
            loop = asyncio.get_event_loop()
            response = await loop.run_in_executor(
                None, 
                lambda: wrapper.api_request('games', igdb_query)
            )
            
            games_data = json.loads(response.decode('utf-8'))
            
            if not games_data:
                return None
            
            # Get basic game data
            game_metadata = self._parse_game_data(games_data[0])
            
            # Fetch time-to-beat data if available
            if game_metadata:
                time_to_beat_data = await self._get_time_to_beat_data(igdb_id)
                if time_to_beat_data:
                    game_metadata.hastily = time_to_beat_data.get("hastily")
                    game_metadata.normally = time_to_beat_data.get("normally")
                    game_metadata.completely = time_to_beat_data.get("completely")
            
            return game_metadata
            
        except Exception as e:
            logger.error(f"Error fetching game by ID {igdb_id}: {e}")
            raise IGDBError(f"Failed to fetch game: {e}")
    
    async def _get_time_to_beat_data(self, igdb_id: str) -> Optional[Dict[str, Any]]:
        """Get time-to-beat data for a game from IGDB."""
        try:
            wrapper = await self._get_wrapper()
            
            time_query = f'''
                fields hastily, normally, completely;
                where game_id = {igdb_id};
            '''
            
            loop = asyncio.get_event_loop()
            response = await loop.run_in_executor(
                None, 
                lambda: wrapper.api_request('game_time_to_beats', time_query)
            )
            
            time_data = json.loads(response.decode('utf-8'))
            
            if time_data:
                return time_data[0]
            return None
            
        except Exception as e:
            logger.error(f"Error fetching time-to-beat data for game {igdb_id}: {e}")
            return None
    
    def _parse_game_data(self, game_data: Dict[str, Any]) -> Optional[GameMetadata]:
        """Parse IGDB game data into GameMetadata object."""
        try:
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
                release_timestamp = game_data['first_release_date']
                release_date = datetime.fromtimestamp(release_timestamp).strftime('%Y-%m-%d')
            
            # Extract cover art URL
            cover_art_url = None
            if 'cover' in game_data and 'image_id' in game_data['cover']:
                image_id = game_data['cover']['image_id']
                cover_art_url = f"https://images.igdb.com/igdb/image/upload/t_cover_big/{image_id}.jpg"
            
            return GameMetadata(
                igdb_id=str(game_data['id']),
                title=game_data.get('name', ''),
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
                completely=None
            )
            
        except Exception as e:
            logger.error(f"Error parsing game data: {e}")
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
    
    async def download_and_store_cover_art(self, igdb_id: str, cover_url: str) -> Optional[str]:
        """Download and store cover art locally. Returns local URL on success."""
        if not cover_url or not igdb_id:
            return None
            
        try:
            local_url = await storage_service.download_and_store_cover_art(igdb_id, cover_url)
            if local_url:
                logger.info(f"Successfully stored cover art for IGDB ID {igdb_id}")
                return local_url
            return None
            
        except Exception as e:
            logger.error(f"Failed to store cover art for IGDB ID {igdb_id}: {e}")
            return None
    
    async def refresh_game_metadata(self, igdb_id: str) -> Optional[GameMetadata]:
        """Refresh game metadata from IGDB by ID."""
        if not igdb_id:
            return None
        
        try:
            return await self.get_game_by_id(igdb_id)
        except Exception as e:
            logger.error(f"Failed to refresh metadata for IGDB ID {igdb_id}: {e}")
            return None
    
    async def populate_missing_metadata(self, current_metadata: GameMetadata, igdb_id: str) -> Optional[GameMetadata]:
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
            'title', 'description', 'genre', 'developer', 'publisher',
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
        essential_fields = ['title', 'description', 'genre', 'developer', 'publisher', 'release_date']
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