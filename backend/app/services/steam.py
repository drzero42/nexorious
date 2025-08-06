"""
Steam Web API service for library import and game metadata.
"""

import logging
from typing import Optional, Dict, Any, List
from datetime import datetime, timedelta, timezone
import asyncio
from dataclasses import dataclass

import httpx
from rapidfuzz import fuzz, process

from app.utils.rate_limiter import (
    RateLimitConfig, 
    RateLimitedClient, 
    TokenBucketRateLimiter,
    RateLimitExceeded
)


logger = logging.getLogger(__name__)


def create_steam_rate_limiter(config: Optional[RateLimitConfig] = None) -> RateLimitedClient:
    """
    Create a rate limiter configured for Steam Web API calls.
    
    Args:
        config: Optional custom configuration, uses Steam defaults if None
        
    Returns:
        RateLimitedClient configured for Steam Web API
    """
    if config is None:
        config = RateLimitConfig(
            requests_per_second=1.0,  # Conservative rate limiting
            burst_capacity=5,         # Allow small bursts
            backoff_factor=1.0,       # 1 second backoff
            max_retries=3             # Up to 3 retries
        )
    
    rate_limiter = TokenBucketRateLimiter(config)
    return RateLimitedClient(rate_limiter)


@dataclass
class SteamUserInfo:
    """Steam user information from Steam Web API."""
    steam_id: str
    persona_name: str
    profile_url: str
    avatar: str
    avatar_medium: str
    avatar_full: str
    persona_state: int
    community_visibility_state: int
    profile_state: Optional[int] = None
    last_logoff: Optional[int] = None
    comment_permission: Optional[int] = None


@dataclass
class SteamGame:
    """Steam game information from Steam Web API."""
    appid: int
    name: str
    img_icon_url: Optional[str] = None


class SteamAPIError(Exception):
    """Steam Web API error."""
    pass


class SteamAuthenticationError(SteamAPIError):
    """Steam Web API authentication error."""
    pass


class SteamService:
    """Steam Web API service for user library import."""
    
    def __init__(self, api_key: str):
        """Initialize Steam service with user's API key."""
        self.api_key = api_key
        self.base_url = "https://api.steampowered.com"
        
        # Create rate limiter for Steam Web API
        # Steam Web API has a rate limit of 100,000 calls per day per API key
        # That's roughly 1.15 calls per second, but we'll be conservative
        self._rate_limiter = create_steam_rate_limiter()
        logger.info("Steam service initialized with rate limiting (1.0 req/s, burst: 5)")
    
    async def _make_request(self, endpoint: str, params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Make a rate-limited request to Steam Web API."""
        if params is None:
            params = {}
        
        # Add API key to all requests
        params["key"] = self.api_key
        params["format"] = "json"
        
        url = f"{self.base_url}/{endpoint}"
        
        try:
            async def request_func():
                async with httpx.AsyncClient() as client:
                    response = await client.get(url, params=params, timeout=30.0)
                    response.raise_for_status()
                    return response.json()
            
            result = await self._rate_limiter.call(request_func)
            return result
            
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 401:
                raise SteamAuthenticationError("Invalid Steam Web API key")
            elif e.response.status_code == 403:
                raise SteamAuthenticationError("Steam Web API key does not have required permissions")
            else:
                raise SteamAPIError(f"Steam API error: {e.response.status_code} - {e.response.text}")
        except httpx.RequestError as e:
            raise SteamAPIError(f"Steam API request failed: {str(e)}")
        except RateLimitExceeded as e:
            logger.warning(f"Steam API rate limit exceeded: {str(e)}")
            raise SteamAPIError("Steam API rate limit exceeded. Please try again later.")
        except Exception as e:
            logger.error(f"Unexpected Steam API error: {str(e)}")
            raise SteamAPIError(f"Unexpected Steam API error: {str(e)}")
    
    async def verify_api_key(self) -> bool:
        """Verify that the Steam Web API key is valid."""
        try:
            # Try to get the user's Steam ID using GetPlayerSummaries with a test Steam ID
            # This will fail with authentication error if the API key is invalid
            await self._make_request(
                "ISteamUser/GetPlayerSummaries/v0002/",
                {"steamids": "76561197960435530"}  # Test with a known Steam ID
            )
            return True
        except SteamAuthenticationError:
            return False
        except Exception as e:
            logger.error(f"Error verifying Steam API key: {str(e)}")
            return False
    
    async def get_user_info(self, steam_id: str) -> Optional[SteamUserInfo]:
        """Get Steam user information by Steam ID."""
        try:
            result = await self._make_request(
                "ISteamUser/GetPlayerSummaries/v0002/",
                {"steamids": steam_id}
            )
            
            players = result.get("response", {}).get("players", [])
            if not players:
                return None
            
            player_data = players[0]
            return SteamUserInfo(
                steam_id=player_data["steamid"],
                persona_name=player_data["personaname"],
                profile_url=player_data["profileurl"],
                avatar=player_data["avatar"],
                avatar_medium=player_data["avatarmedium"],
                avatar_full=player_data["avatarfull"],
                persona_state=player_data["personastate"],
                community_visibility_state=player_data["communityvisibilitystate"],
                profile_state=player_data.get("profilestate"),
                last_logoff=player_data.get("lastlogoff"),
                comment_permission=player_data.get("commentpermission")
            )
            
        except Exception as e:
            logger.error(f"Error getting Steam user info for {steam_id}: {str(e)}")
            raise
    
    async def get_owned_games(self, steam_id: str, include_appinfo: bool = True, include_played_free_games: bool = True) -> List[SteamGame]:
        """Get list of games owned by a Steam user."""
        try:
            params = {
                "steamid": steam_id,
                "include_appinfo": 1 if include_appinfo else 0,
                "include_played_free_games": 1 if include_played_free_games else 0
            }
            
            result = await self._make_request(
                "IPlayerService/GetOwnedGames/v0001/",
                params
            )
            
            games_data = result.get("response", {}).get("games", [])
            games = []
            
            for game_data in games_data:
                game = SteamGame(
                    appid=game_data["appid"],
                    name=game_data.get("name", ""),
                    img_icon_url=game_data.get("img_icon_url")
                )
                games.append(game)
            
            logger.info(f"Retrieved {len(games)} games for Steam user {steam_id}")
            return games
            
        except Exception as e:
            logger.error(f"Error getting owned games for Steam user {steam_id}: {str(e)}")
            raise
    
    async def get_recently_played_games(self, steam_id: str, count: int = 10) -> List[SteamGame]:
        """Get list of recently played games for a Steam user."""
        try:
            result = await self._make_request(
                "IPlayerService/GetRecentlyPlayedGames/v0001/",
                {"steamid": steam_id, "count": count}
            )
            
            games_data = result.get("response", {}).get("games", [])
            games = []
            
            for game_data in games_data:
                game = SteamGame(
                    appid=game_data["appid"],
                    name=game_data.get("name", ""),
                    img_icon_url=game_data.get("img_icon_url")
                )
                games.append(game)
            
            logger.info(f"Retrieved {len(games)} recently played games for Steam user {steam_id}")
            return games
            
        except Exception as e:
            logger.error(f"Error getting recently played games for Steam user {steam_id}: {str(e)}")
            raise
    
    def validate_steam_id(self, steam_id: str) -> bool:
        """Validate Steam ID format (64-bit Steam ID)."""
        try:
            # Steam ID should be a 17-digit number starting with 7656119
            steam_id_int = int(steam_id)
            return len(steam_id) == 17 and steam_id.startswith("7656119")
        except ValueError:
            return False
    
    async def resolve_vanity_url(self, vanity_url: str) -> Optional[str]:
        """Resolve a Steam vanity URL to a Steam ID."""
        try:
            result = await self._make_request(
                "ISteamUser/ResolveVanityURL/v0001/",
                {"vanityurl": vanity_url}
            )
            
            response = result.get("response", {})
            if response.get("success") == 1:
                return response.get("steamid")
            else:
                return None
                
        except Exception as e:
            logger.error(f"Error resolving vanity URL {vanity_url}: {str(e)}")
            return None


def create_steam_service(api_key: str) -> SteamService:
    """Factory function to create a Steam service instance."""
    return SteamService(api_key)