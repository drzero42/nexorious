"""
Nexorious API Client

This module provides a wrapper around the Nexorious API for the
Darkadia import script, handling authentication, error handling,
and retry logic.
"""

import asyncio
from typing import Dict, Any, List, Optional, Union
from datetime import datetime
import json

import httpx
from rich.console import Console

console = Console()


class APIException(Exception):
    """Exception raised for API errors."""
    
    def __init__(self, message: str, status_code: Optional[int] = None, response_data: Optional[Dict] = None):
        super().__init__(message)
        self.status_code = status_code
        self.response_data = response_data or {}


class NexoriousAPIClient:
    """Client for interacting with the Nexorious API."""
    
    def __init__(self, base_url: str, timeout: float = 30.0):
        self.base_url = base_url.rstrip('/')
        self.timeout = timeout
        self.auth_token: Optional[str] = None
        self.client = httpx.AsyncClient(timeout=timeout, follow_redirects=True)
        
        # Cache for platforms and storefronts
        self._platforms_cache: Optional[List[Dict[str, Any]]] = None
        self._storefronts_cache: Optional[List[Dict[str, Any]]] = None
    
    async def __aenter__(self):
        return self
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()
    
    async def close(self):
        """Close the HTTP client."""
        if self.client:
            await self.client.aclose()
    
    def set_token(self, token: str):
        """Set the authentication token."""
        self.auth_token = token
        self.client.headers.update({'Authorization': f'Bearer {token}'})
    
    async def authenticate(self, username: str, password: str) -> str:
        """
        Authenticate with the API and set the token.
        
        Args:
            username: Username for authentication
            password: Password for authentication
            
        Returns:
            Access token
            
        Raises:
            APIException: If authentication fails
        """
        
        try:
            response = await self.client.post(
                f"{self.base_url}/api/auth/login",
                json={"username": username, "password": password}
            )
            
            if response.status_code == 200:
                data = response.json()
                token = data.get('access_token')
                if token:
                    self.set_token(token)
                    console.print("✓ Authentication successful")
                    return token
                else:
                    raise APIException("No access token in response")
            else:
                error_msg = f"Authentication failed: {response.status_code}"
                try:
                    error_data = response.json()
                    error_msg += f" - {error_data.get('detail', 'Unknown error')}"
                except:
                    error_msg += f" - {response.text}"
                
                raise APIException(error_msg, response.status_code)
                
        except httpx.RequestError as e:
            raise APIException(f"Network error during authentication: {str(e)}")
    
    async def health_check(self) -> bool:
        """Check if the API is healthy."""
        try:
            response = await self.client.get(f"{self.base_url}/health")
            return response.status_code == 200
        except:
            return False
    
    async def search_games(self, query: str, fuzzy_threshold: float = 0.8) -> List[Dict[str, Any]]:
        """
        Search for games in the user's collection.
        
        Args:
            query: Search query
            fuzzy_threshold: Fuzzy matching threshold
            
        Returns:
            List of matching games
        """
        
        try:
            params = {
                'q': query,
                'fuzzy_threshold': fuzzy_threshold,
                'limit': 50  # Reasonable limit for search
            }
            
            response = await self.client.get(
                f"{self.base_url}/api/user-games",
                params=params
            )
            
            if response.status_code == 200:
                data = response.json()
                return data.get('user_games', [])
            else:
                console.print(f"[yellow]Search failed: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            console.print(f"[yellow]Search error: {str(e)}[/yellow]")
            return []
    
    async def search_igdb_games(self, query: str, limit: int = 10) -> List[Dict[str, Any]]:
        """
        Search for games in IGDB.
        
        Args:
            query: Search query
            limit: Maximum number of results
            
        Returns:
            List of IGDB game candidates
        """
        
        try:
            response = await self.client.post(
                f"{self.base_url}/api/games/search/igdb",
                json={"query": query, "limit": limit}
            )
            
            if response.status_code == 200:
                data = response.json()
                return data.get('games', [])
            else:
                console.print(f"[yellow]IGDB search failed: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            console.print(f"[yellow]IGDB search error: {str(e)}[/yellow]")
            return []
    
    async def create_user_game(self, user_id: str, game_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """
        Create a new user game entry.
        
        Args:
            user_id: User ID
            game_data: Game data in Nexorious format
            
        Returns:
            Created user game data or None if failed
        """
        
        try:
            # Prepare the payload
            payload = {
                'title': game_data['title'],
                'ownership_status': game_data['ownership_status'],
                'play_status': game_data['play_status'],
                'personal_rating': game_data.get('personal_rating'),
                'is_loved': game_data.get('is_loved', False),
                'personal_notes': game_data.get('personal_notes', ''),
                'acquired_date': game_data.get('acquired_date'),
                'hours_played': game_data.get('hours_played', 0),
                'platforms': []
            }
            
            # Add platform associations
            for platform_info in game_data.get('platforms', []):
                platform_data = {
                    'platform_name': platform_info['platform_name'],
                    'storefront_name': platform_info['storefront_name'],
                    'is_available': platform_info.get('is_available', True)
                }
                payload['platforms'].append(platform_data)
            
            response = await self.client.post(
                f"{self.base_url}/api/user-games",
                json=payload
            )
            
            if response.status_code == 201:
                return response.json()
            else:
                error_msg = f"Failed to create user game: {response.status_code}"
                try:
                    error_data = response.json()
                    error_msg += f" - {error_data.get('detail', 'Unknown error')}"
                except:
                    error_msg += f" - {response.text}"
                
                raise APIException(error_msg, response.status_code, response.json() if response.content else {})
                
        except httpx.RequestError as e:
            raise APIException(f"Network error creating user game: {str(e)}")
    
    async def update_user_game(self, user_game_id: str, game_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """
        Update an existing user game entry.
        
        Args:
            user_game_id: User game ID to update
            game_data: Updated game data
            
        Returns:
            Updated user game data or None if failed
        """
        
        try:
            # Prepare update payload (only include fields that should be updated)
            payload = {}
            
            updatable_fields = [
                'ownership_status', 'play_status', 'personal_rating', 'is_loved',
                'personal_notes', 'acquired_date', 'hours_played'
            ]
            
            for field in updatable_fields:
                if field in game_data:
                    payload[field] = game_data[field]
            
            response = await self.client.put(
                f"{self.base_url}/api/user-games/{user_game_id}",
                json=payload
            )
            
            if response.status_code == 200:
                return response.json()
            else:
                error_msg = f"Failed to update user game: {response.status_code}"
                try:
                    error_data = response.json()
                    error_msg += f" - {error_data.get('detail', 'Unknown error')}"
                except:
                    error_msg += f" - {response.text}"
                
                raise APIException(error_msg, response.status_code)
                
        except httpx.RequestError as e:
            raise APIException(f"Network error updating user game: {str(e)}")
    
    async def add_platform_to_user_game(self, user_game_id: str, platform_data: Dict[str, Any]) -> bool:
        """
        Add a platform association to an existing user game.
        
        Args:
            user_game_id: User game ID
            platform_data: Platform association data
            
        Returns:
            True if successful, False otherwise
        """
        
        try:
            payload = {
                'platform_name': platform_data['platform_name'],
                'storefront_name': platform_data['storefront_name'],
                'is_available': platform_data.get('is_available', True)
            }
            
            response = await self.client.post(
                f"{self.base_url}/api/user-games/{user_game_id}/platforms",
                json=payload
            )
            
            return response.status_code in [200, 201]
            
        except httpx.RequestError as e:
            console.print(f"[yellow]Error adding platform: {str(e)}[/yellow]")
            return False
    
    async def get_platforms(self) -> List[Dict[str, Any]]:
        """Get list of available platforms."""
        
        if self._platforms_cache is not None:
            return self._platforms_cache
        
        try:
            response = await self.client.get(f"{self.base_url}/api/platforms")
            
            if response.status_code == 200:
                platforms = response.json()
                # Handle different response formats
                if isinstance(platforms, list):
                    self._platforms_cache = platforms
                elif isinstance(platforms, dict) and 'platforms' in platforms:
                    self._platforms_cache = platforms['platforms']
                else:
                    self._platforms_cache = []
                
                return self._platforms_cache
            else:
                console.print(f"[yellow]Failed to get platforms: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            console.print(f"[yellow]Error getting platforms: {str(e)}[/yellow]")
            return []
    
    async def get_storefronts(self) -> List[Dict[str, Any]]:
        """Get list of available storefronts."""
        
        if self._storefronts_cache is not None:
            return self._storefronts_cache
        
        try:
            response = await self.client.get(f"{self.base_url}/api/storefronts")
            
            if response.status_code == 200:
                storefronts = response.json()
                self._storefronts_cache = storefronts
                return storefronts
            else:
                console.print(f"[yellow]Failed to get storefronts: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            console.print(f"[yellow]Error getting storefronts: {str(e)}[/yellow]")
            return []
    
    async def validate_platform_storefront(self, platform_name: str, storefront_name: str) -> bool:
        """
        Validate that a platform and storefront combination is valid.
        
        Args:
            platform_name: Platform name
            storefront_name: Storefront name
            
        Returns:
            True if valid, False otherwise
        """
        
        platforms = await self.get_platforms()
        storefronts = await self.get_storefronts()
        
        # Check if platform exists
        platform_exists = any(p.get('display_name') == platform_name or p.get('name') == platform_name 
                             for p in platforms)
        
        # Check if storefront exists
        storefront_exists = any(s.get('display_name') == storefront_name or s.get('name') == storefront_name 
                               for s in storefronts)
        
        return platform_exists and storefront_exists
    
    async def retry_request(self, func, max_retries: int = 3, backoff_factor: float = 1.0) -> Any:
        """
        Retry a request with exponential backoff.
        
        Args:
            func: Async function to retry
            max_retries: Maximum number of retries
            backoff_factor: Backoff multiplier
            
        Returns:
            Result of the function call
        """
        
        last_exception = None
        
        for attempt in range(max_retries + 1):
            try:
                return await func()
            except (httpx.RequestError, APIException) as e:
                last_exception = e
                
                if attempt < max_retries:
                    delay = backoff_factor * (2 ** attempt)
                    console.print(f"[yellow]Request failed (attempt {attempt + 1}/{max_retries + 1}), retrying in {delay}s...[/yellow]")
                    await asyncio.sleep(delay)
                else:
                    break
        
        # If we get here, all retries failed
        raise last_exception or APIException("All retries failed")