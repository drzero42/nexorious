"""
Tests for Steam Web API service.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
import httpx

from nexorious.services.steam import (
    SteamService, 
    create_steam_service,
    SteamAPIError,
    SteamAuthenticationError,
    SteamUserInfo,
    SteamGame
)
from nexorious.utils.rate_limiter import RateLimitExceeded


@pytest.fixture
def steam_service():
    """Create a Steam service instance for testing."""
    return SteamService("test_api_key_32chars_long_1234567")


@pytest.fixture
def mock_rate_limiter():
    """Mock rate limiter for testing."""
    with patch('nexorious.services.steam.RateLimitedClient') as mock:
        yield mock


class TestSteamService:
    """Test Steam service functionality."""

    def test_init(self, mock_rate_limiter):
        """Test Steam service initialization."""
        api_key = "test_api_key_32chars_long_1234567"
        service = SteamService(api_key)
        
        assert service.api_key == api_key
        assert service.base_url == "https://api.steampowered.com"
        mock_rate_limiter.assert_called_once()

    @pytest.mark.asyncio
    async def test_make_request_success(self, steam_service, mock_rate_limiter):
        """Test successful API request."""
        # Mock the rate limiter
        mock_rate_limiter_instance = MagicMock()
        mock_rate_limiter.return_value = mock_rate_limiter_instance
        
        # Mock successful response
        expected_response = {"response": {"success": 1}}
        mock_rate_limiter_instance.call = AsyncMock(return_value=expected_response)
        
        # Re-initialize service to use mocked rate limiter
        service = SteamService("test_api_key_32chars_long_1234567")
        
        result = await service._make_request("test/endpoint", {"param": "value"})
        
        assert result == expected_response
        mock_rate_limiter_instance.call.assert_called_once()

    @pytest.mark.asyncio
    async def test_make_request_authentication_error(self, steam_service):
        """Test API request with authentication error."""
        # Mock HTTP 401 error
        http_error = httpx.HTTPStatusError(
            "Unauthorized", 
            request=MagicMock(), 
            response=MagicMock(status_code=401, text="Unauthorized")
        )
        
        # Mock the rate limiter's call method
        with patch.object(steam_service._rate_limiter, 'call') as mock_call:
            async def mock_request():
                raise http_error
            
            async def mock_side_effect(func):
                return await mock_request()
            
            mock_call.side_effect = mock_side_effect
            
            with pytest.raises(SteamAuthenticationError, match="Invalid Steam Web API key"):
                await steam_service._make_request("test/endpoint")

    @pytest.mark.asyncio
    async def test_make_request_forbidden_error(self, steam_service):
        """Test API request with forbidden error."""
        # Mock HTTP 403 error
        http_error = httpx.HTTPStatusError(
            "Forbidden", 
            request=MagicMock(), 
            response=MagicMock(status_code=403, text="Forbidden")
        )
        
        # Mock the rate limiter's call method
        with patch.object(steam_service._rate_limiter, 'call') as mock_call:
            async def mock_request():
                raise http_error
            
            async def mock_side_effect(func):
                return await mock_request()
            
            mock_call.side_effect = mock_side_effect
            
            with pytest.raises(SteamAuthenticationError, match="does not have required permissions"):
                await steam_service._make_request("test/endpoint")

    @pytest.mark.asyncio
    async def test_make_request_rate_limit_exceeded(self, steam_service, mock_rate_limiter):
        """Test API request with rate limit exceeded."""
        mock_rate_limiter_instance = MagicMock()
        mock_rate_limiter.return_value = mock_rate_limiter_instance
        
        mock_rate_limiter_instance.call = AsyncMock(
            side_effect=RateLimitExceeded("Rate limit exceeded")
        )
        
        service = SteamService("test_api_key_32chars_long_1234567")
        
        with pytest.raises(SteamAPIError, match="rate limit exceeded"):
            await service._make_request("test/endpoint")

    @pytest.mark.asyncio
    async def test_verify_api_key_valid(self, steam_service):
        """Test API key verification with valid key."""
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.return_value = {"response": {"players": []}}
            
            result = await steam_service.verify_api_key()
            
            assert result is True
            mock_request.assert_called_once()

    @pytest.mark.asyncio
    async def test_verify_api_key_invalid(self, steam_service):
        """Test API key verification with invalid key."""
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.side_effect = SteamAuthenticationError("Invalid key")
            
            result = await steam_service.verify_api_key()
            
            assert result is False

    @pytest.mark.asyncio
    async def test_get_user_info_success(self, steam_service):
        """Test getting user info successfully."""
        mock_response = {
            "response": {
                "players": [{
                    "steamid": "76561197960435530",
                    "personaname": "Test User",
                    "profileurl": "https://steamcommunity.com/id/testuser/",
                    "avatar": "https://avatar.url/small.jpg",
                    "avatarmedium": "https://avatar.url/medium.jpg",
                    "avatarfull": "https://avatar.url/full.jpg",
                    "personastate": 1,
                    "communityvisibilitystate": 3,
                    "profilestate": 1,
                    "lastlogoff": 1234567890,
                    "commentpermission": 1
                }]
            }
        }
        
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.return_value = mock_response
            
            result = await steam_service.get_user_info("76561197960435530")
            
            assert isinstance(result, SteamUserInfo)
            assert result.steam_id == "76561197960435530"
            assert result.persona_name == "Test User"
            assert result.profile_url == "https://steamcommunity.com/id/testuser/"

    @pytest.mark.asyncio
    async def test_get_user_info_not_found(self, steam_service):
        """Test getting user info for non-existent user."""
        mock_response = {"response": {"players": []}}
        
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.return_value = mock_response
            
            result = await steam_service.get_user_info("76561197960435530")
            
            assert result is None

    @pytest.mark.asyncio
    async def test_get_owned_games_success(self, steam_service):
        """Test getting owned games successfully."""
        mock_response = {
            "response": {
                "games": [
                    {
                        "appid": 730,
                        "name": "Counter-Strike: Global Offensive",
                        "img_icon_url": "icon_url"
                    },
                    {
                        "appid": 440,
                        "name": "Team Fortress 2"
                    }
                ]
            }
        }
        
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.return_value = mock_response
            
            result = await steam_service.get_owned_games("76561197960435530")
            
            assert len(result) == 2
            assert isinstance(result[0], SteamGame)
            assert result[0].appid == 730
            assert result[0].name == "Counter-Strike: Global Offensive"
            assert result[0].img_icon_url == "icon_url"
            assert result[1].appid == 440
            assert result[1].name == "Team Fortress 2"

    @pytest.mark.asyncio
    async def test_get_recently_played_games_success(self, steam_service):
        """Test getting recently played games successfully."""
        mock_response = {
            "response": {
                "games": [
                    {
                        "appid": 730,
                        "name": "Counter-Strike: Global Offensive"
                    }
                ]
            }
        }
        
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.return_value = mock_response
            
            result = await steam_service.get_recently_played_games("76561197960435530", count=5)
            
            assert len(result) == 1
            assert isinstance(result[0], SteamGame)
            assert result[0].appid == 730

    def test_validate_steam_id_valid(self, steam_service):
        """Test validating valid Steam ID."""
        valid_steam_id = "76561197960435530"
        
        result = steam_service.validate_steam_id(valid_steam_id)
        
        assert result is True

    def test_validate_steam_id_invalid_length(self, steam_service):
        """Test validating Steam ID with invalid length."""
        invalid_steam_id = "1234567890"
        
        result = steam_service.validate_steam_id(invalid_steam_id)
        
        assert result is False

    def test_validate_steam_id_invalid_prefix(self, steam_service):
        """Test validating Steam ID with invalid prefix."""
        invalid_steam_id = "12345678901234567"
        
        result = steam_service.validate_steam_id(invalid_steam_id)
        
        assert result is False

    def test_validate_steam_id_non_numeric(self, steam_service):
        """Test validating non-numeric Steam ID."""
        invalid_steam_id = "abcd1197960435530"
        
        result = steam_service.validate_steam_id(invalid_steam_id)
        
        assert result is False

    @pytest.mark.asyncio
    async def test_resolve_vanity_url_success(self, steam_service):
        """Test resolving vanity URL successfully."""
        mock_response = {
            "response": {
                "success": 1,
                "steamid": "76561197960435530"
            }
        }
        
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.return_value = mock_response
            
            result = await steam_service.resolve_vanity_url("testuser")
            
            assert result == "76561197960435530"

    @pytest.mark.asyncio
    async def test_resolve_vanity_url_not_found(self, steam_service):
        """Test resolving non-existent vanity URL."""
        mock_response = {
            "response": {
                "success": 42,  # Steam returns 42 for "No match"
                "message": "No match"
            }
        }
        
        with patch.object(steam_service, '_make_request') as mock_request:
            mock_request.return_value = mock_response
            
            result = await steam_service.resolve_vanity_url("nonexistentuser")
            
            assert result is None

    def test_create_steam_service(self):
        """Test Steam service factory function."""
        api_key = "test_api_key_32chars_long_1234567"
        
        service = create_steam_service(api_key)
        
        assert isinstance(service, SteamService)
        assert service.api_key == api_key


class TestSteamDataClasses:
    """Test Steam data classes."""

    def test_steam_user_info_creation(self):
        """Test SteamUserInfo creation."""
        user_info = SteamUserInfo(
            steam_id="76561197960435530",
            persona_name="Test User",
            profile_url="https://steamcommunity.com/id/testuser/",
            avatar="small.jpg",
            avatar_medium="medium.jpg",
            avatar_full="full.jpg",
            persona_state=1,
            community_visibility_state=3
        )
        
        assert user_info.steam_id == "76561197960435530"
        assert user_info.persona_name == "Test User"
        assert user_info.persona_state == 1

    def test_steam_game_creation(self):
        """Test SteamGame creation."""
        game = SteamGame(
            appid=730,
            name="Counter-Strike: Global Offensive",
            img_icon_url="icon_url"
        )
        
        assert game.appid == 730
        assert game.name == "Counter-Strike: Global Offensive"
        assert game.img_icon_url == "icon_url"


class TestSteamExceptions:
    """Test Steam exception classes."""

    def test_steam_api_error(self):
        """Test SteamAPIError exception."""
        error = SteamAPIError("Test error")
        assert str(error) == "Test error"

    def test_steam_authentication_error(self):
        """Test SteamAuthenticationError exception."""
        error = SteamAuthenticationError("Auth error")
        assert str(error) == "Auth error"
        assert isinstance(error, SteamAPIError)  # Should inherit from SteamAPIError