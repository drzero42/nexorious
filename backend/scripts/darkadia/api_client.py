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
    """Exception raised for API errors with enhanced error details."""
    
    def __init__(self, message: str, status_code: Optional[int] = None, response_data: Optional[Dict] = None):
        super().__init__(message)
        self.status_code = status_code
        self.response_data = response_data or {}
        self.error_type = self._determine_error_type()
        self.validation_errors = self._extract_validation_errors()
        self.conflict_details = self._extract_conflict_details()
    
    def _determine_error_type(self) -> str:
        """Determine the type of error based on status code and response."""
        if not self.status_code:
            return "network"
        
        if self.status_code == 400:
            return "validation"
        elif self.status_code == 401:
            return "authentication"
        elif self.status_code == 403:
            return "authorization"
        elif self.status_code == 404:
            return "not_found"
        elif self.status_code == 409:
            return "conflict"
        elif self.status_code == 422:
            return "validation"
        elif 400 <= self.status_code < 500:
            return "client_error"
        elif 500 <= self.status_code < 600:
            return "server_error"
        else:
            return "unknown"
    
    def _extract_validation_errors(self) -> List[Dict[str, Any]]:
        """Extract detailed validation errors from response."""
        validation_errors = []
        
        if not self.response_data:
            return validation_errors
        
        # Handle FastAPI validation errors (422)
        if self.status_code == 422 and 'detail' in self.response_data:
            detail = self.response_data['detail']
            if isinstance(detail, list):
                for error in detail:
                    if isinstance(error, dict):
                        validation_errors.append({
                            'field': '.'.join(str(loc) for loc in error.get('loc', [])),
                            'message': error.get('msg', 'Validation error'),
                            'type': error.get('type', 'unknown'),
                            'input': error.get('input')
                        })
        
        # Handle general validation errors (400)
        elif self.status_code == 400 and 'detail' in self.response_data:
            detail = self.response_data['detail']
            if isinstance(detail, dict) and 'errors' in detail:
                # Handle structured validation errors
                for field, messages in detail['errors'].items():
                    if isinstance(messages, list):
                        for msg in messages:
                            validation_errors.append({
                                'field': field,
                                'message': msg,
                                'type': 'validation'
                            })
                    else:
                        validation_errors.append({
                            'field': field,
                            'message': str(messages),
                            'type': 'validation'
                        })
        
        return validation_errors
    
    def _extract_conflict_details(self) -> Dict[str, Any]:
        """Extract detailed conflict information from 409 responses."""
        conflict_details = {}
        
        if self.status_code != 409 or not self.response_data:
            return conflict_details
        
        detail = self.response_data.get('detail', '')
        if not detail:
            return conflict_details
        
        # Parse different types of conflicts
        if "Game with title" in detail and "already exists" in detail:
            # Extract the conflicting title
            import re
            title_match = re.search(r"Game with title '([^']+)' already exists", detail)
            if title_match:
                conflict_details.update({
                    'type': 'duplicate_title',
                    'conflicting_title': title_match.group(1),
                    'reason': 'A game with this exact title already exists in the database',
                    'recommendation': 'Check if this is the same game or modify the title to distinguish different versions'
                })
        
        elif "Game already exists in database" in detail:
            # IGDB ID conflict
            igdb_id = self.response_data.get('igdb_id')
            game_title = self.response_data.get('game_title', 'Unknown')
            
            conflict_details.update({
                'type': 'duplicate_igdb_id',
                'conflicting_igdb_id': igdb_id,
                'game_title': game_title,
                'reason': 'A game with this IGDB ID already exists in the database',
                'recommendation': 'This appears to be a duplicate entry - consider skipping this import'
            })
        
        elif "already exists" in detail.lower():
            # Generic conflict
            conflict_details.update({
                'type': 'generic_conflict',
                'reason': detail,
                'recommendation': 'Review the existing data to understand the conflict'
            })
        
        return conflict_details
    
    def get_user_friendly_message(self) -> str:
        """Get a user-friendly error message."""
        if self.validation_errors:
            # Create a summary of validation errors
            field_errors = []
            for error in self.validation_errors:
                field = error.get('field', 'unknown field')
                message = error.get('message', 'invalid value')
                field_errors.append(f"{field}: {message}")
            
            return f"Validation failed: {'; '.join(field_errors)}"
        
        # Handle 409 conflicts with detailed explanations
        if self.conflict_details:
            conflict_type = self.conflict_details.get('type', 'unknown')
            reason = self.conflict_details.get('reason', 'Conflict detected')
            
            if conflict_type == 'duplicate_title':
                title = self.conflict_details.get('conflicting_title', 'Unknown')
                return f"Duplicate game title: '{title}' already exists in your collection"
            elif conflict_type == 'duplicate_igdb_id':
                igdb_id = self.conflict_details.get('conflicting_igdb_id', 'Unknown')
                return f"Duplicate game: IGDB ID {igdb_id} already exists in the database"
            else:
                return reason
        
        # Extract meaningful message from response
        if self.response_data:
            detail = self.response_data.get('detail', '')
            if detail and detail != 'Unknown error':
                return detail
            
            # Try other common error message fields
            for field in ['error', 'message', 'msg']:
                if field in self.response_data:
                    return str(self.response_data[field])
        
        # Fall back to the original message
        return str(self)
    
    def get_troubleshooting_hint(self) -> str:
        """Get a troubleshooting hint based on the error type."""
        # Provide specific hints for 409 conflicts
        if self.conflict_details:
            return self.conflict_details.get('recommendation', 'Review the conflict and take appropriate action.')
        
        hints = {
            "authentication": "Check your username and password. If you're using a token, ensure it's valid and not expired.",
            "authorization": "You don't have permission to perform this operation. Contact an administrator.",
            "validation": "Check the data you're trying to submit. Some required fields may be missing or invalid.",
            "not_found": "The requested resource was not found. It may have been deleted or moved.",
            "conflict": "This operation conflicts with existing data. Check for duplicates or related records.",
            "network": "Check your internet connection and ensure the API server is running.",
            "server_error": "The server encountered an error. Try again later or contact support.",
            "client_error": "There's an issue with the request. Check the data and try again."
        }
        
        return hints.get(self.error_type, "Review the error details and try again.")


class NexoriousAPIClient:
    """Client for interacting with the Nexorious API."""
    
    def __init__(self, base_url: str, timeout: float = 30.0, progress_console: Optional[Console] = None):
        self.base_url = base_url.rstrip('/')
        self.timeout = timeout
        self.auth_token: Optional[str] = None
        self.client = httpx.AsyncClient(timeout=timeout, follow_redirects=True)
        
        # Use progress console if provided, otherwise fall back to global console
        self.console = progress_console or console
        
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
    
    def set_progress_console(self, progress_console: Optional[Console]):
        """Set the progress console for messages during progress tracking."""
        self.console = progress_console or console
    
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
                    self.console.print("✓ Authentication successful")
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
    
    async def get_current_user(self) -> Dict[str, Any]:
        """
        Get current user's profile information.
        
        Returns:
            User profile data including id, username, etc.
            
        Raises:
            APIException: If request fails or user is not authenticated
        """
        
        try:
            response = await self.client.get(
                f"{self.base_url}/api/auth/me"
            )
            
            if response.status_code == 200:
                user_data = response.json()
                self.console.print(f"✓ Retrieved user profile: {user_data.get('username')}")
                return user_data
            else:
                error_msg = f"Failed to get current user: {response.status_code}"
                try:
                    error_data = response.json()
                    error_msg += f" - {error_data.get('detail', 'Unknown error')}"
                except:
                    error_msg += f" - {response.text}"
                
                raise APIException(error_msg, response.status_code)
                
        except httpx.RequestError as e:
            raise APIException(f"Network error getting current user: {str(e)}")
    
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
                self.console.print(f"[yellow]Search failed: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            self.console.print(f"[yellow]Search error: {str(e)}[/yellow]")
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
                self.console.print(f"[yellow]IGDB search failed: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            self.console.print(f"[yellow]IGDB search error: {str(e)}[/yellow]")
            return []
    
    async def create_user_game(self, user_id: str, game_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """
        Create a new user game entry with proper workflow.
        
        Args:
            user_id: User ID
            game_data: Game data in Nexorious format
            
        Returns:
            Created user game data or None if failed
        """
        
        try:
            # Step 1: Find or create the game record
            game_record = await self.find_or_create_game(game_data['title'])
            if not game_record:
                raise APIException(f"Failed to find or create game: {game_data['title']}")
            
            # Step 2: Resolve platform and storefront names to IDs
            platform_associations = []
            for platform_info in game_data.get('platforms', []):
                platform_id = await self.get_platform_id(platform_info['platform_name'])
                if not platform_id:
                    self.console.print(f"[yellow]Platform not found: {platform_info['platform_name']}[/yellow]")
                    continue
                
                storefront_id = None
                if platform_info.get('storefront_name'):
                    storefront_id = await self.get_storefront_id(platform_info['storefront_name'])
                    if not storefront_id:
                        self.console.print(f"[yellow]Storefront not found: {platform_info['storefront_name']}[/yellow]")
                        continue
                
                platform_associations.append({
                    'platform_id': platform_id,
                    'storefront_id': storefront_id,
                    'is_available': platform_info.get('is_available', True)
                })
            
            # Step 3: Prepare the payload with proper format
            payload = {
                'game_id': game_record['id'],  # Required field
                'ownership_status': game_data['ownership_status'],
                'play_status': game_data['play_status'],
                'personal_rating': game_data.get('personal_rating'),
                'is_loved': game_data.get('is_loved', False),
                'personal_notes': game_data.get('personal_notes', ''),
                'acquired_date': game_data.get('acquired_date'),
                'hours_played': game_data.get('hours_played', 0),
                'platforms': platform_associations  # Use resolved IDs
            }
            
            # Step 4: Create the user game
            response = await self.client.post(
                f"{self.base_url}/api/user-games",
                json=payload
            )
            
            if response.status_code == 201:
                created_game = response.json()
                self.console.print(f"\n✓ Created user game: {game_record['title']}")
                return created_game
            else:
                # Parse response data for detailed error information
                try:
                    error_data = response.json()
                except:
                    error_data = {"detail": response.text}
                
                # Add context to error data
                error_data['game_title'] = game_record.get('title', 'Unknown')
                error_data['operation'] = 'create_user_game'
                
                error_msg = f"Failed to create user game"
                raise APIException(error_msg, response.status_code, error_data)
                
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
                # Parse response data for detailed error information
                try:
                    error_data = response.json()
                except:
                    error_data = {"detail": response.text}
                
                # Add context to error data
                error_data['user_game_id'] = user_game_id
                error_data['operation'] = 'update_user_game'
                error_data['payload_fields'] = list(payload.keys())
                
                error_msg = f"Failed to update user game"
                raise APIException(error_msg, response.status_code, error_data)
                
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
            
            # Handle the case where platform already exists (409 Conflict or similar)
            if response.status_code in [200, 201]:
                return True
            elif response.status_code == 409:
                # Platform already exists - this is OK for idempotency
                return True
            else:
                return False
            
        except httpx.RequestError as e:
            self.console.print(f"[yellow]Error adding platform: {str(e)}[/yellow]")
            return False
    
    async def get_user_game_details(self, user_game_id: str) -> Optional[Dict[str, Any]]:
        """
        Get detailed information about a user game including platforms.
        
        Args:
            user_game_id: User game ID
            
        Returns:
            User game details with platforms or None if not found
        """
        
        try:
            response = await self.client.get(f"{self.base_url}/api/user-games/{user_game_id}")
            
            if response.status_code == 200:
                return response.json()
            else:
                return None
                
        except httpx.RequestError as e:
            self.console.print(f"[yellow]Error getting user game details: {str(e)}[/yellow]")
            return None
    
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
                self.console.print(f"[yellow]Failed to get platforms: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            self.console.print(f"[yellow]Error getting platforms: {str(e)}[/yellow]")
            return []
    
    async def get_storefronts(self) -> List[Dict[str, Any]]:
        """Get list of available storefronts."""
        
        if self._storefronts_cache is not None:
            return self._storefronts_cache
        
        try:
            response = await self.client.get(f"{self.base_url}/api/platforms/storefronts/")
            
            if response.status_code == 200:
                data = response.json()
                # Handle different response formats
                if isinstance(data, list):
                    self._storefronts_cache = data
                elif isinstance(data, dict) and 'storefronts' in data:
                    self._storefronts_cache = data['storefronts']
                else:
                    self._storefronts_cache = []
                
                return self._storefronts_cache
            else:
                self.console.print(f"[yellow]Failed to get storefronts: {response.status_code}[/yellow]")
                return []
                
        except httpx.RequestError as e:
            self.console.print(f"[yellow]Error getting storefronts: {str(e)}[/yellow]")
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
    
    async def get_platform_id(self, platform_name: str) -> Optional[str]:
        """
        Get platform ID from platform name.
        
        Args:
            platform_name: Platform display name or name
            
        Returns:
            Platform ID if found, None otherwise
        """
        
        platforms = await self.get_platforms()
        
        for platform in platforms:
            if (platform.get('display_name') == platform_name or 
                platform.get('name') == platform_name):
                return platform.get('id')
        
        return None
    
    async def get_storefront_id(self, storefront_name: str) -> Optional[str]:
        """
        Get storefront ID from storefront name.
        
        Args:
            storefront_name: Storefront display name or name
            
        Returns:
            Storefront ID if found, None otherwise
        """
        
        storefronts = await self.get_storefronts()
        
        for storefront in storefronts:
            if (storefront.get('display_name') == storefront_name or 
                storefront.get('name') == storefront_name):
                return storefront.get('id')
        
        return None
    
    async def find_or_create_game(self, game_title: str) -> Optional[Dict[str, Any]]:
        """
        Find an existing game or create a new one from IGDB data.
        
        Args:
            game_title: Game title to search for
            
        Returns:
            Game data with id field, or None if failed
        """
        
        try:
            # First, search IGDB for the game
            igdb_candidates = await self.search_igdb_games(game_title, limit=5)
            
            if not igdb_candidates:
                self.console.print(f"[yellow]No IGDB results found for: {game_title}[/yellow]")
                return None
            
            # Take the first (best) match
            best_match = igdb_candidates[0]
            
            # Check if game already exists in our database by IGDB ID
            try:
                response = await self.client.get(
                    f"{self.base_url}/api/games",
                    params={"q": game_title, "limit": 50}
                )
                
                if response.status_code == 200:
                    existing_games = response.json().get('games', [])
                    
                    # Look for existing game with same IGDB ID
                    for game in existing_games:
                        if game.get('igdb_id') == best_match.get('igdb_id'):
                            self.console.print(f"✓ Found existing game: {game['title']} (ID: {game['id']})")
                            return game
            except Exception as e:
                self.console.print(f"[yellow]Error searching existing games: {str(e)}[/yellow]")
            
            # Game doesn't exist, create it from IGDB data
            try:
                import_payload = {
                    "igdb_id": best_match['igdb_id'],
                    "custom_overrides": {}
                }
                
                response = await self.client.post(
                    f"{self.base_url}/api/games/igdb-import",
                    json=import_payload
                )
                
                if response.status_code == 201:
                    new_game = response.json()
                    self.console.print(f"\n✓ Created new game: {new_game['title']} (ID: {new_game['id']})")
                    return new_game
                else:
                    # Create detailed APIException with response data
                    try:
                        error_data = response.json()
                    except:
                        error_data = {"detail": response.text}
                    
                    # Add context to error data
                    error_data['game_title'] = game_title
                    if best_match and best_match.get('igdb_id'):
                        error_data['igdb_id'] = best_match['igdb_id']
                    
                    error_msg = f"Failed to import '{game_title}' from IGDB"
                    api_exception = APIException(error_msg, response.status_code, error_data)
                    
                    self.console.print(f"\n[red]Error creating game: {api_exception.get_user_friendly_message()}[/red]")
                    if best_match and best_match.get('igdb_id'):
                        self.console.print(f"[red]IGDB ID: {best_match['igdb_id']}[/red]")
                    
                    # Don't raise the exception, just return None for backward compatibility
                    return None
                    
            except httpx.RequestError as e:
                self.console.print(f"\n[red]Network error creating game '{game_title}': {str(e)}[/red]")
                return None
                
        except Exception as e:
            self.console.print(f"\n[red]Error finding/creating game '{game_title}': {str(e)}[/red]")
            return None
    
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
                    self.console.print(f"[yellow]Request failed (attempt {attempt + 1}/{max_retries + 1}), retrying in {delay}s...[/yellow]")
                    await asyncio.sleep(delay)
                else:
                    break
        
        # If we get here, all retries failed
        raise last_exception or APIException("All retries failed")