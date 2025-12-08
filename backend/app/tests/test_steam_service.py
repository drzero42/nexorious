"""
Tests for Steam Web API service.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

import httpx

from app.services.steam import (
    SteamService,
    create_steam_service,
    SteamAPIError,
    SteamAuthenticationError,
    SteamUserInfo,
    SteamGame
)
from app.utils.rate_limiter import RateLimitExceeded


def create_mock_http_response(status_code: int, text: str = ""):
    """Create a mock httpx response that raises HTTPStatusError on raise_for_status()."""
    mock_request = MagicMock(spec=httpx.Request)
    mock_request.url = "https://api.steampowered.com/test"

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = status_code
    mock_response.text = text
    mock_response.request = mock_request

    def raise_for_status():
        if status_code >= 400:
            raise httpx.HTTPStatusError(
                f"HTTP {status_code}",
                request=mock_request,
                response=mock_response
            )

    mock_response.raise_for_status = raise_for_status
    return mock_response


@pytest.fixture
def steam_service():
    """Create a Steam service instance for testing."""
    return SteamService("test_api_key_32chars_long_1234567")


@pytest.fixture
def mock_rate_limiter():
    """Mock rate limiter for testing."""
    with patch('app.services.steam.RateLimitedClient') as mock:
        yield mock


@pytest.fixture
def steam_test_responses():
    """Predefined Steam API responses for common scenarios."""
    return {
        "user_info_success": {
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
        },
        "owned_games_success": {
            "response": {
                "games": [
                    {"appid": 730, "name": "Counter-Strike: Global Offensive", "img_icon_url": "icon_url"},
                    {"appid": 440, "name": "Team Fortress 2"}
                ]
            }
        },
        "recently_played_success": {
            "response": {
                "games": [
                    {"appid": 730, "name": "Counter-Strike: Global Offensive"}
                ]
            }
        },
        "vanity_url_success": {
            "response": {
                "success": 1,
                "steamid": "76561197960435530"
            }
        },
        "vanity_url_not_found": {
            "response": {
                "success": 42,  # Steam returns 42 for "No match"
                "message": "No match"
            }
        },
        "empty_games": {"response": {"games": []}},
        "user_not_found": {"response": {"players": []}},
        "api_key_test": {"response": {"players": []}}
    }


@pytest.fixture 
def steam_error_scenarios():
    """Common Steam API error scenarios."""
    return {
        "auth_401": SteamAuthenticationError("Invalid Steam Web API key"),
        "auth_403": SteamAuthenticationError("does not have required permissions"),
        "rate_limit": SteamAPIError("rate limit exceeded"),
        "general_api": SteamAPIError("API temporarily unavailable")
    }


@pytest.fixture
def steam_service_with_responses():
    """Create Steam service with configurable response mocking."""
    def create_service(responses=None, errors=None):
        service = SteamService("test_api_key_32chars_long_1234567")

        # Replace the complex rate limiter mocking with direct method mocking

        async def mock_make_request(endpoint, params=None):
            if errors and endpoint in errors:
                raise errors[endpoint]
            if responses and endpoint in responses:
                return responses[endpoint]
            return {"response": {"success": 1}}

        service._make_request = mock_make_request
        return service

    return create_service


@pytest.fixture
def mock_rate_limiter_passthrough():
    """Mock rate limiter that passes through to the actual function."""
    with patch('app.services.steam.RateLimitedClient') as mock_class:
        mock_instance = MagicMock()
        mock_class.return_value = mock_instance

        async def passthrough(func, **kwargs):
            return await func()
        mock_instance.call = passthrough

        yield mock_class


@pytest.fixture
def mock_httpx_client():
    """Mock httpx.AsyncClient for HTTP-level testing."""
    with patch('httpx.AsyncClient') as mock_class:
        mock_client = AsyncMock()
        mock_class.return_value.__aenter__.return_value = mock_client
        yield mock_client


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
    async def test_make_request_authentication_error(self, mock_rate_limiter_passthrough, mock_httpx_client):
        """Test that 401 HTTP response is converted to SteamAuthenticationError."""
        mock_httpx_client.get.return_value = create_mock_http_response(401)

        service = SteamService("test_api_key_32chars_long_1234567")

        with pytest.raises(SteamAuthenticationError, match="Invalid Steam Web API key"):
            await service._make_request("test/endpoint")

    @pytest.mark.asyncio
    async def test_make_request_forbidden_error(self, mock_rate_limiter_passthrough, mock_httpx_client):
        """Test that 403 HTTP response is converted to SteamAuthenticationError."""
        mock_httpx_client.get.return_value = create_mock_http_response(403)

        service = SteamService("test_api_key_32chars_long_1234567")

        with pytest.raises(SteamAuthenticationError, match="does not have required permissions"):
            await service._make_request("test/endpoint")

    @pytest.mark.asyncio
    async def test_make_request_rate_limit_exceeded(self):
        """Test that RateLimitExceeded from rate limiter is converted to SteamAPIError."""
        with patch('app.services.steam.RateLimitedClient') as mock_class:
            mock_instance = MagicMock()
            mock_class.return_value = mock_instance
            mock_instance.call = AsyncMock(
                side_effect=RateLimitExceeded("Rate limit exceeded", retry_after=1.0)
            )

            service = SteamService("test_api_key_32chars_long_1234567")

            with pytest.raises(SteamAPIError, match="rate limit exceeded"):
                await service._make_request("test/endpoint")

    @pytest.mark.asyncio
    async def test_verify_api_key_valid(self, steam_service_with_responses, steam_test_responses):
        """Test API key verification with valid key."""
        service = steam_service_with_responses(
            responses={"ISteamUser/GetPlayerSummaries/v0002/": steam_test_responses["api_key_test"]}
        )
        
        result = await service.verify_api_key()
        
        assert result is True

    @pytest.mark.asyncio
    async def test_verify_api_key_invalid(self, steam_service_with_responses, steam_error_scenarios):
        """Test API key verification with invalid key."""
        service = steam_service_with_responses(
            errors={"ISteamUser/GetPlayerSummaries/v0002/": steam_error_scenarios["auth_401"]}
        )
        
        result = await service.verify_api_key()
        
        assert result is False

    @pytest.mark.asyncio
    async def test_get_user_info_success(self, steam_service_with_responses, steam_test_responses):
        """Test getting user info successfully."""
        service = steam_service_with_responses(
            responses={"ISteamUser/GetPlayerSummaries/v0002/": steam_test_responses["user_info_success"]}
        )
        
        result = await service.get_user_info("76561197960435530")
        
        assert isinstance(result, SteamUserInfo)
        assert result.steam_id == "76561197960435530"
        assert result.persona_name == "Test User"
        assert result.profile_url == "https://steamcommunity.com/id/testuser/"

    @pytest.mark.asyncio
    async def test_get_user_info_not_found(self, steam_service_with_responses, steam_test_responses):
        """Test getting user info for non-existent user."""
        service = steam_service_with_responses(
            responses={"ISteamUser/GetPlayerSummaries/v0002/": steam_test_responses["user_not_found"]}
        )
        
        result = await service.get_user_info("76561197960435530")
        
        assert result is None

    @pytest.mark.asyncio
    async def test_get_owned_games_success(self, steam_service_with_responses, steam_test_responses):
        """Test getting owned games successfully."""
        service = steam_service_with_responses(
            responses={"IPlayerService/GetOwnedGames/v0001/": steam_test_responses["owned_games_success"]}
        )
        
        result = await service.get_owned_games("76561197960435530")
        
        assert len(result) == 2
        assert isinstance(result[0], SteamGame)
        assert result[0].appid == 730
        assert result[0].name == "Counter-Strike: Global Offensive"
        assert result[1].appid == 440
        assert result[1].name == "Team Fortress 2"

    @pytest.mark.asyncio
    async def test_get_recently_played_games_success(self, steam_service_with_responses, steam_test_responses):
        """Test getting recently played games successfully."""
        service = steam_service_with_responses(
            responses={"IPlayerService/GetRecentlyPlayedGames/v0001/": steam_test_responses["recently_played_success"]}
        )
        
        result = await service.get_recently_played_games("76561197960435530", count=5)
        
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
    async def test_resolve_vanity_url_success(self, steam_service_with_responses, steam_test_responses):
        """Test resolving vanity URL successfully."""
        service = steam_service_with_responses(
            responses={"ISteamUser/ResolveVanityURL/v0001/": steam_test_responses["vanity_url_success"]}
        )
        
        result = await service.resolve_vanity_url("testuser")
        
        assert result == "76561197960435530"

    @pytest.mark.asyncio
    async def test_resolve_vanity_url_not_found(self, steam_service_with_responses, steam_test_responses):
        """Test resolving non-existent vanity URL."""
        service = steam_service_with_responses(
            responses={"ISteamUser/ResolveVanityURL/v0001/": steam_test_responses["vanity_url_not_found"]}
        )
        
        result = await service.resolve_vanity_url("nonexistentuser")
        
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
            name="Counter-Strike: Global Offensive"
        )
        
        assert game.appid == 730
        assert game.name == "Counter-Strike: Global Offensive"


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