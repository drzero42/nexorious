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


logger = logging.getLogger(__name__)


@dataclass
class GameMetadata:
    """Structured game metadata from IGDB."""
    
    igdb_id: str
    title: str
    slug: str
    description: Optional[str] = None
    genre: Optional[str] = None
    developer: Optional[str] = None
    publisher: Optional[str] = None
    release_date: Optional[str] = None
    cover_art_url: Optional[str] = None
    rating_average: Optional[float] = None
    rating_count: Optional[int] = None
    estimated_playtime_hours: Optional[int] = None


class TwitchAuthError(Exception):
    """Exception for Twitch authentication errors."""
    pass


class IGDBError(Exception):
    """Exception for IGDB API errors."""
    pass


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
                fields id, name, slug, summary, genres.name, involved_companies.company.name, 
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
            
            # Convert to GameMetadata objects
            games = []
            for game_data in games_data:
                metadata = self._parse_game_data(game_data)
                if metadata:
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
                fields id, name, slug, summary, genres.name, involved_companies.company.name, 
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
            
            return self._parse_game_data(games_data[0])
            
        except Exception as e:
            logger.error(f"Error fetching game by ID {igdb_id}: {e}")
            raise IGDBError(f"Failed to fetch game: {e}")
    
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
                slug=game_data.get('slug', ''),
                description=game_data.get('summary'),
                genre=genre,
                developer=developer,
                publisher=publisher,
                release_date=release_date,
                cover_art_url=cover_art_url,
                rating_average=game_data.get('rating'),
                rating_count=game_data.get('rating_count'),
                estimated_playtime_hours=None  # IGDB doesn't provide this directly
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