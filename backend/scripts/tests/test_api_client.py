"""
Tests for Nexorious API Client module.
"""

import pytest
import asyncio
from unittest.mock import AsyncMock, MagicMock, patch
import httpx

from scripts.darkadia.api_client import NexoriousAPIClient, APIException


class TestNexoriousAPIClient:
    """Test cases for NexoriousAPIClient."""
    
    @pytest.fixture
    def mock_httpx_client(self):
        """Create a mock httpx client."""
        return AsyncMock(spec=httpx.AsyncClient)
    
    @pytest.fixture
    def api_client(self, mock_httpx_client):
        """Create an API client with mocked httpx client."""
        client = NexoriousAPIClient('http://localhost:8000')
        client.client = mock_httpx_client
        return client
    
    @pytest.mark.asyncio
    async def test_authenticate_success(self, api_client, mock_httpx_client):
        """Test successful authentication."""
        # Mock successful login response
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {'access_token': 'test_token_123'}
        mock_httpx_client.post.return_value = mock_response
        
        token = await api_client.authenticate('testuser', 'testpass')
        
        assert token == 'test_token_123'
        assert api_client.auth_token == 'test_token_123'
        mock_httpx_client.post.assert_called_once_with(
            'http://localhost:8000/api/auth/login',
            json={'username': 'testuser', 'password': 'testpass'}
        )
    
    @pytest.mark.asyncio
    async def test_authenticate_failure(self, api_client, mock_httpx_client):
        """Test authentication failure."""
        # Mock failed login response
        mock_response = MagicMock()
        mock_response.status_code = 401
        mock_response.json.return_value = {'detail': 'Invalid credentials'}
        mock_httpx_client.post.return_value = mock_response
        
        with pytest.raises(APIException) as exc_info:
            await api_client.authenticate('baduser', 'badpass')
        
        assert exc_info.value.status_code == 401
        assert 'Authentication failed' in str(exc_info.value)
    
    @pytest.mark.asyncio
    async def test_authenticate_network_error(self, api_client, mock_httpx_client):
        """Test authentication with network error."""
        mock_httpx_client.post.side_effect = httpx.RequestError("Connection failed")
        
        with pytest.raises(APIException) as exc_info:
            await api_client.authenticate('user', 'pass')
        
        assert 'Network error during authentication' in str(exc_info.value)
    
    @pytest.mark.asyncio
    async def test_get_current_user_success(self, api_client, mock_httpx_client):
        """Test successful current user retrieval."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            'id': 'user_123',
            'username': 'testuser',
            'is_admin': False,
            'is_active': True
        }
        mock_httpx_client.get.return_value = mock_response
        
        user_data = await api_client.get_current_user()
        
        assert user_data['id'] == 'user_123'
        assert user_data['username'] == 'testuser'
        mock_httpx_client.get.assert_called_once_with('http://localhost:8000/api/auth/me')
    
    @pytest.mark.asyncio
    async def test_get_current_user_failure(self, api_client, mock_httpx_client):
        """Test current user retrieval failure."""
        mock_response = MagicMock()
        mock_response.status_code = 401
        mock_response.json.return_value = {'detail': 'Unauthorized'}
        mock_response.text = 'Unauthorized'
        mock_httpx_client.get.return_value = mock_response
        
        with pytest.raises(APIException) as exc_info:
            await api_client.get_current_user()
        
        assert exc_info.value.status_code == 401
        assert 'Failed to get current user' in str(exc_info.value)
    
    @pytest.mark.asyncio
    async def test_get_current_user_network_error(self, api_client, mock_httpx_client):
        """Test current user retrieval with network error."""
        mock_httpx_client.get.side_effect = httpx.RequestError("Connection failed")
        
        with pytest.raises(APIException) as exc_info:
            await api_client.get_current_user()
        
        assert 'Network error getting current user' in str(exc_info.value)
    
    @pytest.mark.asyncio
    async def test_health_check_success(self, api_client, mock_httpx_client):
        """Test successful health check."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_httpx_client.get.return_value = mock_response
        
        result = await api_client.health_check()
        
        assert result is True
        mock_httpx_client.get.assert_called_once_with('http://localhost:8000/health')
    
    @pytest.mark.asyncio
    async def test_health_check_failure(self, api_client, mock_httpx_client):
        """Test failed health check."""
        mock_response = MagicMock()
        mock_response.status_code = 500
        mock_httpx_client.get.return_value = mock_response
        
        result = await api_client.health_check()
        
        assert result is False
    
    @pytest.mark.asyncio
    async def test_health_check_exception(self, api_client, mock_httpx_client):
        """Test health check with exception."""
        mock_httpx_client.get.side_effect = httpx.RequestError("Connection failed")
        
        result = await api_client.health_check()
        
        assert result is False
    
    @pytest.mark.asyncio
    async def test_search_games_success(self, api_client, mock_httpx_client):
        """Test successful game search."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            'user_games': [
                {'id': '1', 'title': 'Test Game 1'},
                {'id': '2', 'title': 'Test Game 2'}
            ]
        }
        mock_httpx_client.get.return_value = mock_response
        
        results = await api_client.search_games('test query')
        
        assert len(results) == 2
        assert results[0]['title'] == 'Test Game 1'
        mock_httpx_client.get.assert_called_once_with(
            'http://localhost:8000/api/user-games',
            params={'q': 'test query', 'fuzzy_threshold': 0.8, 'limit': 50}
        )
    
    @pytest.mark.asyncio
    async def test_search_games_no_results(self, api_client, mock_httpx_client):
        """Test game search with no results."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {'user_games': []}
        mock_httpx_client.get.return_value = mock_response
        
        results = await api_client.search_games('nonexistent game')
        
        assert len(results) == 0
    
    @pytest.mark.asyncio
    async def test_search_games_error(self, api_client, mock_httpx_client):
        """Test game search with error."""
        mock_response = MagicMock()
        mock_response.status_code = 500
        mock_httpx_client.get.return_value = mock_response
        
        results = await api_client.search_games('test query')
        
        assert len(results) == 0
    
    @pytest.mark.asyncio
    async def test_search_igdb_games_success(self, api_client, mock_httpx_client):
        """Test successful IGDB game search."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            'games': [
                {'id': 1, 'name': 'IGDB Game 1'},
                {'id': 2, 'name': 'IGDB Game 2'}
            ]
        }
        mock_httpx_client.post.return_value = mock_response
        
        results = await api_client.search_igdb_games('test query')
        
        assert len(results) == 2
        assert results[0]['name'] == 'IGDB Game 1'
        mock_httpx_client.post.assert_called_once_with(
            'http://localhost:8000/api/games/search/igdb',
            json={'query': 'test query', 'limit': 10}
        )
    
    @pytest.mark.asyncio
    async def test_create_user_game_success(self, api_client, mock_httpx_client):
        """Test successful user game creation."""
        mock_response = MagicMock()
        mock_response.status_code = 201
        mock_response.json.return_value = {'id': 'game_123', 'title': 'New Game'}
        mock_httpx_client.post.return_value = mock_response
        
        game_data = {
            'title': 'New Game',
            'ownership_status': 'owned',
            'play_status': 'not_started',
            'personal_rating': 4.0,
            'platforms': [
                {'platform_name': 'PC (Windows)', 'storefront_name': 'Steam', 'is_available': True}
            ]
        }
        
        result = await api_client.create_user_game('user_123', game_data)
        
        assert result is not None
        assert result['id'] == 'game_123'
        mock_httpx_client.post.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_create_user_game_failure(self, api_client, mock_httpx_client):
        """Test user game creation failure."""
        mock_response = MagicMock()
        mock_response.status_code = 400
        mock_response.json.return_value = {'detail': 'Invalid data'}
        mock_response.content = b'{"detail": "Invalid data"}'
        mock_httpx_client.post.return_value = mock_response
        
        game_data = {'title': 'Bad Game'}
        
        with pytest.raises(APIException) as exc_info:
            await api_client.create_user_game('user_123', game_data)
        
        assert exc_info.value.status_code == 400
        assert 'Failed to create user game' in str(exc_info.value)
    
    @pytest.mark.asyncio
    async def test_update_user_game_success(self, api_client, mock_httpx_client):
        """Test successful user game update."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {'id': 'game_123', 'title': 'Updated Game'}
        mock_httpx_client.put.return_value = mock_response
        
        game_data = {
            'personal_rating': 5.0,
            'play_status': 'completed'
        }
        
        result = await api_client.update_user_game('game_123', game_data)
        
        assert result is not None
        assert result['title'] == 'Updated Game'
        mock_httpx_client.put.assert_called_once_with(
            'http://localhost:8000/api/user-games/game_123',
            json={'personal_rating': 5.0, 'play_status': 'completed'}
        )
    
    @pytest.mark.asyncio
    async def test_add_platform_to_user_game_success(self, api_client, mock_httpx_client):
        """Test successful platform addition."""
        mock_response = MagicMock()
        mock_response.status_code = 201
        mock_httpx_client.post.return_value = mock_response
        
        platform_data = {
            'platform_name': 'PlayStation 4',
            'storefront_name': 'PlayStation Store',
            'is_available': True
        }
        
        result = await api_client.add_platform_to_user_game('game_123', platform_data)
        
        assert result is True
        mock_httpx_client.post.assert_called_once_with(
            'http://localhost:8000/api/user-games/game_123/platforms',
            json=platform_data
        )
    
    @pytest.mark.asyncio
    async def test_get_platforms_success(self, api_client, mock_httpx_client):
        """Test successful platform retrieval."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = [
            {'id': '1', 'name': 'PC (Windows)'},
            {'id': '2', 'name': 'PlayStation 4'}
        ]
        mock_httpx_client.get.return_value = mock_response
        
        platforms = await api_client.get_platforms()
        
        assert len(platforms) == 2
        assert platforms[0]['name'] == 'PC (Windows)'
        
        # Test caching - second call should not make HTTP request
        platforms2 = await api_client.get_platforms()
        assert len(platforms2) == 2
        mock_httpx_client.get.assert_called_once()  # Only called once due to caching
    
    @pytest.mark.asyncio
    async def test_get_platforms_dict_response(self, api_client, mock_httpx_client):
        """Test platform retrieval with dictionary response."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            'platforms': [
                {'id': '1', 'name': 'PC (Windows)'},
                {'id': '2', 'name': 'PlayStation 4'}
            ]
        }
        mock_httpx_client.get.return_value = mock_response
        
        platforms = await api_client.get_platforms()
        
        assert len(platforms) == 2
        assert platforms[0]['name'] == 'PC (Windows)'
    
    @pytest.mark.asyncio
    async def test_validate_platform_storefront_valid(self, api_client):
        """Test platform/storefront validation with valid combination."""
        # Mock the get_platforms and get_storefronts methods
        api_client._platforms_cache = [
            {'name': 'PC (Windows)', 'display_name': 'PC (Windows)'},
            {'name': 'PlayStation 4', 'display_name': 'PlayStation 4'}
        ]
        api_client._storefronts_cache = [
            {'name': 'Steam', 'display_name': 'Steam'},
            {'name': 'PlayStation Store', 'display_name': 'PlayStation Store'}
        ]
        
        result = await api_client.validate_platform_storefront('PC (Windows)', 'Steam')
        
        assert result is True
    
    @pytest.mark.asyncio
    async def test_validate_platform_storefront_invalid(self, api_client):
        """Test platform/storefront validation with invalid combination."""
        api_client._platforms_cache = [
            {'name': 'PC (Windows)', 'display_name': 'PC (Windows)'}
        ]
        api_client._storefronts_cache = [
            {'name': 'Steam', 'display_name': 'Steam'}
        ]
        
        result = await api_client.validate_platform_storefront('Unknown Platform', 'Steam')
        
        assert result is False
    
    @pytest.mark.asyncio
    async def test_retry_request_success(self, api_client):
        """Test retry mechanism with eventual success."""
        call_count = 0
        
        async def failing_func():
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise httpx.RequestError("Temporary failure")
            return "success"
        
        result = await api_client.retry_request(failing_func, max_retries=3, backoff_factor=0.01)
        
        assert result == "success"
        assert call_count == 3
    
    @pytest.mark.asyncio
    async def test_retry_request_failure(self, api_client):
        """Test retry mechanism with ultimate failure."""
        async def always_failing_func():
            raise httpx.RequestError("Permanent failure")
        
        with pytest.raises(httpx.RequestError):
            await api_client.retry_request(always_failing_func, max_retries=2, backoff_factor=0.01)
    
    @pytest.mark.asyncio
    async def test_context_manager(self, mock_httpx_client):
        """Test using API client as context manager."""
        async with NexoriousAPIClient('http://localhost:8000') as client:
            # Replace with mock for testing
            client.client = mock_httpx_client
            assert client.client is not None
        
        # Should have called aclose on the client
        mock_httpx_client.aclose.assert_called_once()