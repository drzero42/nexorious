"""
Integration tests for IGDBService with rate limiting.
"""

import pytest
import asyncio
import time
from unittest.mock import AsyncMock, patch, MagicMock
from typing import List

from nexorious.services.igdb import IGDBService, IGDBError
from nexorious.utils.rate_limiter import RateLimitConfig, RateLimitExceeded


class TestRateLimitedIGDBService:
    """Test IGDBService with rate limiting integration."""
    
    @pytest.fixture
    def mock_settings(self):
        """Mock settings for testing."""
        with patch('nexorious.services.igdb.settings') as mock_settings:
            mock_settings.igdb_client_id = "test_client_id"
            mock_settings.igdb_client_secret = "test_client_secret"
            mock_settings.igdb_access_token = "test_token"
            # Rate limiting settings
            mock_settings.igdb_requests_per_second = 4.0
            mock_settings.igdb_burst_capacity = 8
            mock_settings.igdb_backoff_factor = 1.0
            mock_settings.igdb_max_retries = 3
            yield mock_settings
    
    @pytest.fixture
    def mock_wrapper(self):
        """Mock IGDB wrapper."""
        wrapper = MagicMock()
        wrapper.api_request.side_effect = lambda endpoint, query: b'[{"id": 1, "name": "Test Game"}]'
        return wrapper
    
    @pytest.fixture
    def igdb_service(self, mock_settings):
        """Create IGDBService instance for testing."""
        return IGDBService()
    
    def test_rate_limiter_initialization(self, mock_settings):
        """Test that rate limiter is properly initialized."""
        service = IGDBService()
        
        # Rate limiter should be initialized with config from settings
        status = service.get_rate_limiter_status()
        assert status['requests_per_second'] == 4.0
        assert status['max_tokens'] == 8
        assert status['tokens_available'] == 8.0  # Should start with full bucket
    
    @pytest.mark.asyncio
    async def test_rate_limited_api_request_success(self, igdb_service, mock_wrapper):
        """Test successful rate-limited API request."""
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            response = await igdb_service._rate_limited_api_request('games', 'fields id, name;')
            
            assert response == b'[{"id": 1, "name": "Test Game"}]'
            mock_wrapper.api_request.assert_called_once_with('games', 'fields id, name;')
    
    @pytest.mark.asyncio
    async def test_rate_limited_api_request_error_handling(self, igdb_service, mock_wrapper):
        """Test error handling in rate-limited API request."""
        mock_wrapper.api_request.side_effect = Exception("API Error")
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            with pytest.raises(IGDBError, match="IGDB API request failed"):
                await igdb_service._rate_limited_api_request('games', 'fields id, name;')
    
    @pytest.mark.asyncio
    async def test_search_games_with_rate_limiting(self, igdb_service, mock_wrapper):
        """Test search_games method uses rate limiting."""
        # Mock successful API responses
        games_response = b'''[{
            "id": 1,
            "name": "Test Game",
            "slug": "test-game",
            "summary": "A test game",
            "platforms": [{"id": 6, "name": "PC"}]
        }]'''
        
        time_response = b'''[{
            "hastily": 3600,
            "normally": 7200,
            "completely": 10800
        }]'''
        
        mock_wrapper.api_request.side_effect = [games_response, time_response]
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            results = await igdb_service.search_games("test", limit=1)
        
        assert len(results) == 1
        assert results[0].title == "Test Game"
        assert results[0].hastily == 1  # 3600 seconds = 1 hour
        
        # Should have made 2 API calls (games + time-to-beat)
        assert mock_wrapper.api_request.call_count == 2
    
    @pytest.mark.asyncio
    async def test_get_game_by_id_with_rate_limiting(self, igdb_service, mock_wrapper):
        """Test get_game_by_id method uses rate limiting."""
        games_response = b'''[{
            "id": 123,
            "name": "Test Game",
            "slug": "test-game",
            "summary": "A test game"
        }]'''
        
        time_response = b'''[{
            "hastily": 1800,
            "normally": 3600,
            "completely": 5400
        }]'''
        
        mock_wrapper.api_request.side_effect = [games_response, time_response]
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            result = await igdb_service.get_game_by_id("123")
        
        assert result is not None
        assert result.title == "Test Game"
        assert result.hastily == 0  # 1800 seconds = 0.5 hours, rounds to 0
        
        # Should have made 2 API calls
        assert mock_wrapper.api_request.call_count == 2
    
    @pytest.mark.asyncio
    async def test_concurrent_requests_respect_rate_limit(self, igdb_service, mock_wrapper):
        """Test that concurrent requests respect rate limits."""
        # Configure a slower rate limit for testing
        with patch.object(igdb_service._rate_limiter.rate_limiter.config, 'requests_per_second', 5.0):
            call_count = 0
            
            def mock_api_request(endpoint, query):
                nonlocal call_count
                call_count += 1
                return f'[{{"id": {call_count}, "name": "Game {call_count}"}}]'.encode()
            
            mock_wrapper.api_request.side_effect = mock_api_request
            
            with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
                # Launch 10 concurrent search requests (each makes 2 API calls)
                start_time = time.monotonic()
                
                tasks = [
                    igdb_service.search_games(f"game{i}", limit=1) 
                    for i in range(5)  # 5 searches = 10 API calls total
                ]
                
                results = await asyncio.gather(*tasks)
                end_time = time.monotonic()
                
                # All searches should succeed
                assert len(results) == 5
                assert all(len(result) == 1 for result in results)
                
                # Should have made at least 10 API calls (games + time-to-beat for each)
                assert call_count >= 10
                
                # With burst capacity of 8 and rate of 5 req/s, the remaining 2 calls
                # should take at least 0.4 seconds (2 calls / 5 req/s)
                # Allow some tolerance for test timing
                assert end_time - start_time >= 0.2
    
    @pytest.mark.asyncio
    async def test_rate_limiter_status_monitoring(self, igdb_service, mock_wrapper):
        """Test rate limiter status monitoring."""
        initial_status = igdb_service.get_rate_limiter_status()
        assert initial_status['tokens_available'] == 8.0
        assert initial_status['utilization'] == 0.0
        
        mock_wrapper.api_request.return_value = b'[{"id": 1, "name": "Test"}]'
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            # Make a few API calls
            await igdb_service._rate_limited_api_request('games', 'fields id;')
            await igdb_service._rate_limited_api_request('games', 'fields id;')
            
            # Status should show reduced tokens (allow for small floating-point differences)
            status = igdb_service.get_rate_limiter_status()
            assert 5.5 <= status['tokens_available'] <= 6.5  # Should be around 6, allow for refill
            assert 0.20 <= status['utilization'] <= 0.30  # Should be around 0.25
    
    @pytest.mark.asyncio
    async def test_rate_limit_with_retries(self, igdb_service, mock_wrapper):
        """Test that retries work with rate limiting."""
        call_count = 0
        
        def mock_api_request(endpoint, query):
            nonlocal call_count
            call_count += 1
            if call_count < 2:
                raise Exception("Temporary error")
            return b'[{"id": 1, "name": "Success"}]'
        
        mock_wrapper.api_request.side_effect = mock_api_request
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            with patch('asyncio.sleep', new_callable=AsyncMock):  # Speed up retries
                response = await igdb_service._rate_limited_api_request('games', 'fields id;')
        
        assert response == b'[{"id": 1, "name": "Success"}]'
        assert call_count == 2  # Should have retried once
    
    @pytest.mark.asyncio
    async def test_rate_limit_exhaustion_handling(self, igdb_service):
        """Test handling when rate limit is exhausted."""
        # Set up a very restrictive rate limiter
        restrictive_config = RateLimitConfig(
            requests_per_second=1.0,
            burst_capacity=1,
            max_retries=1
        )
        
        # Replace the rate limiter with a more restrictive one
        from nexorious.utils.rate_limiter import create_igdb_rate_limiter
        igdb_service._rate_limiter = create_igdb_rate_limiter(restrictive_config)
        
        mock_wrapper = MagicMock()
        mock_wrapper.api_request.return_value = b'[{"id": 1, "name": "Test"}]'
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            # First call should succeed (uses the one available token)
            await igdb_service._rate_limited_api_request('games', 'fields id;')
            
            # Second call should fail due to rate limit (patch to use very short timeout)
            with patch.object(igdb_service._rate_limiter, 'call') as mock_call:
                mock_call.side_effect = RateLimitExceeded("Rate limit exceeded", retry_after=1.0)
                with pytest.raises(IGDBError, match="Rate limit exceeded"):
                    await igdb_service._rate_limited_api_request('games', 'fields id;')
    
    @pytest.mark.asyncio
    async def test_search_games_error_propagation(self, igdb_service, mock_wrapper):
        """Test that rate limit errors are properly propagated in search_games."""
        # Configure restrictive rate limiter
        restrictive_config = RateLimitConfig(
            requests_per_second=0.5,  # Very slow
            burst_capacity=1,
            max_retries=0
        )
        
        from nexorious.utils.rate_limiter import create_igdb_rate_limiter
        igdb_service._rate_limiter = create_igdb_rate_limiter(restrictive_config)
        
        mock_wrapper.api_request.return_value = b'[{"id": 1, "name": "Test"}]'
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            # First call should succeed
            result1 = await igdb_service.search_games("test1", limit=1)
            assert len(result1) == 1
            
            # Second call should fail due to rate limiting (patch to simulate failure)
            with patch.object(igdb_service._rate_limiter, 'call') as mock_call:
                mock_call.side_effect = RateLimitExceeded("Rate limit exceeded")
                with pytest.raises(IGDBError):
                    await igdb_service.search_games("test2", limit=1)
    
    @pytest.mark.asyncio
    async def test_multiple_service_instances_independent_rate_limiting(self, mock_settings):
        """Test that multiple service instances have independent rate limiters."""
        service1 = IGDBService()
        service2 = IGDBService()
        
        try:
            # Each service should have its own rate limiter
            status1 = service1.get_rate_limiter_status()
            status2 = service2.get_rate_limiter_status()
            
            assert status1['tokens_available'] == 8.0
            assert status2['tokens_available'] == 8.0
            
            # Using tokens in one service shouldn't affect the other
            mock_wrapper = MagicMock()
            mock_wrapper.api_request.return_value = b'[{"id": 1}]'
            
            with patch.object(service1, '_get_wrapper', return_value=mock_wrapper):
                await service1._rate_limited_api_request('games', 'fields id;')
            
            # Service1 should have fewer tokens, service2 should be unchanged
            status1_after = service1.get_rate_limiter_status()
            status2_after = service2.get_rate_limiter_status()
            
            assert status1_after['tokens_available'] == 7.0
            assert status2_after['tokens_available'] == 8.0
            
        finally:
            await service1.__aexit__(None, None, None)
            await service2.__aexit__(None, None, None)