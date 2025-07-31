"""
Tests for the rate limiter implementation.
"""

import asyncio
import pytest
import time
from unittest.mock import AsyncMock, patch

from nexorious.utils.rate_limiter import (
    RateLimitConfig,
    TokenBucketRateLimiter,
    RateLimitedClient,
    RateLimitExceeded,
    create_igdb_rate_limiter,
    IGDB_RATE_LIMIT_CONFIG
)


class TestRateLimitConfig:
    """Test the rate limit configuration."""
    
    def test_default_config(self):
        """Test default configuration values."""
        config = RateLimitConfig()
        assert config.requests_per_second == 4.0
        assert config.burst_capacity == 10
        assert config.backoff_factor == 1.0
        assert config.max_retries == 3
    
    def test_custom_config(self):
        """Test custom configuration values."""
        config = RateLimitConfig(
            requests_per_second=2.0,
            burst_capacity=5,
            backoff_factor=0.5,
            max_retries=2
        )
        assert config.requests_per_second == 2.0
        assert config.burst_capacity == 5
        assert config.backoff_factor == 0.5
        assert config.max_retries == 2


class TestTokenBucketRateLimiter:
    """Test the token bucket rate limiter."""
    
    def test_initialization(self):
        """Test rate limiter initialization."""
        config = RateLimitConfig(requests_per_second=2.0, burst_capacity=5)
        limiter = TokenBucketRateLimiter(config)
        
        assert limiter.config == config
        assert limiter.tokens == 5.0  # Should start with full bucket
    
    @pytest.mark.asyncio
    async def test_acquire_tokens_success(self):
        """Test successful token acquisition."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5)
        limiter = TokenBucketRateLimiter(config)
        
        # Should be able to acquire tokens up to burst capacity
        assert await limiter.acquire(1) is True
        assert await limiter.acquire(2) is True
        assert await limiter.acquire(2) is True
        # Now we should have 0 tokens left
        assert await limiter.acquire(1) is False
    
    @pytest.mark.asyncio
    async def test_acquire_tokens_insufficient(self):
        """Test token acquisition when insufficient tokens available."""
        config = RateLimitConfig(requests_per_second=1.0, burst_capacity=3)
        limiter = TokenBucketRateLimiter(config)
        
        # Exhaust the bucket
        assert await limiter.acquire(3) is True
        # Should fail to acquire more
        assert await limiter.acquire(1) is False
    
    @pytest.mark.asyncio
    async def test_token_refill(self):
        """Test that tokens are refilled over time."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5)
        limiter = TokenBucketRateLimiter(config)
        
        # Exhaust the bucket
        assert await limiter.acquire(5) is True
        assert await limiter.acquire(1) is False
        
        # Wait for tokens to refill (0.1 seconds should add 1 token at 10 req/s)
        await asyncio.sleep(0.15)
        assert await limiter.acquire(1) is True
    
    @pytest.mark.asyncio
    async def test_wait_for_tokens_success(self):
        """Test waiting for tokens to become available."""
        config = RateLimitConfig(requests_per_second=20.0, burst_capacity=2)
        limiter = TokenBucketRateLimiter(config)
        
        # Exhaust the bucket
        assert await limiter.acquire(2) is True
        
        # Should wait and eventually get tokens
        start_time = time.monotonic()
        result = await limiter.wait_for_tokens(1, timeout=1.0)
        end_time = time.monotonic()
        
        assert result is True
        assert end_time - start_time > 0.0  # Should have waited some time
        assert end_time - start_time < 0.5  # But not too long at 20 req/s
    
    @pytest.mark.asyncio
    async def test_wait_for_tokens_timeout(self):
        """Test timeout when waiting for tokens."""
        config = RateLimitConfig(requests_per_second=1.0, burst_capacity=1)
        limiter = TokenBucketRateLimiter(config)
        
        # Exhaust the bucket
        assert await limiter.acquire(1) is True
        
        # Should timeout quickly
        start_time = time.monotonic()
        result = await limiter.wait_for_tokens(1, timeout=0.1)
        end_time = time.monotonic()
        
        assert result is False
        assert end_time - start_time >= 0.1
        assert end_time - start_time < 0.2  # Should respect timeout
    
    def test_get_status(self):
        """Test rate limiter status reporting."""
        config = RateLimitConfig(requests_per_second=4.0, burst_capacity=8)
        limiter = TokenBucketRateLimiter(config)
        
        status = limiter.get_status()
        
        assert status['tokens_available'] == 8.0
        assert status['max_tokens'] == 8
        assert status['requests_per_second'] == 4.0
        assert status['utilization'] == 0.0  # Full bucket = 0% utilization


class TestRateLimitedClient:
    """Test the rate limited client wrapper."""
    
    @pytest.mark.asyncio
    async def test_successful_call(self):
        """Test successful rate-limited function call."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5)
        limiter = TokenBucketRateLimiter(config)
        client = RateLimitedClient(limiter)
        
        # Mock function that returns a value
        async def mock_func():
            return "success"
        
        result = await client.call(mock_func)
        assert result == "success"
    
    @pytest.mark.asyncio
    async def test_call_with_retries(self):
        """Test function call with retries on failure."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5, max_retries=2)
        limiter = TokenBucketRateLimiter(config)
        client = RateLimitedClient(limiter)
        
        call_count = 0
        
        async def mock_func():
            nonlocal call_count
            call_count += 1
            if call_count < 2:
                raise ValueError("Temporary error")
            return "success"
        
        # Should retry and eventually succeed
        with patch('asyncio.sleep', new_callable=AsyncMock):  # Speed up test
            result = await client.call(mock_func)
        
        assert result == "success"
        assert call_count == 2
    
    @pytest.mark.asyncio
    async def test_call_rate_limit_exceeded(self):
        """Test rate limit exceeded scenario."""
        config = RateLimitConfig(requests_per_second=1.0, burst_capacity=1)
        limiter = TokenBucketRateLimiter(config)
        client = RateLimitedClient(limiter)
        
        # Exhaust the bucket first
        await limiter.acquire(1)
        
        async def mock_func():
            return "should not reach here"
        
        # Should raise RateLimitExceeded
        with pytest.raises(RateLimitExceeded):
            await client.call(mock_func, timeout=0.01)  # Very short timeout
    
    @pytest.mark.asyncio
    async def test_call_exhausted_retries(self):
        """Test function call that exhausts all retries."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5, max_retries=1)
        limiter = TokenBucketRateLimiter(config)
        client = RateLimitedClient(limiter)
        
        async def mock_func():
            raise ValueError("Persistent error")
        
        # Should exhaust retries and raise the original exception
        with patch('asyncio.sleep', new_callable=AsyncMock):  # Speed up test
            with pytest.raises(ValueError, match="Persistent error"):
                await client.call(mock_func)
    
    @pytest.mark.asyncio
    async def test_call_multiple_tokens(self):
        """Test function call requiring multiple tokens."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5)
        limiter = TokenBucketRateLimiter(config)
        client = RateLimitedClient(limiter)
        
        async def mock_func():
            return "success"
        
        # Should successfully acquire 3 tokens
        result = await client.call(mock_func, tokens_needed=3)
        assert result == "success"
        
        # Should have 2 tokens remaining
        status = client.get_status()
        assert status['tokens_available'] == 2.0
    
    def test_get_status(self):
        """Test client status reporting."""
        config = RateLimitConfig(requests_per_second=4.0, burst_capacity=8)
        limiter = TokenBucketRateLimiter(config)
        client = RateLimitedClient(limiter)
        
        status = client.get_status()
        assert status['requests_per_second'] == 4.0
        assert status['tokens_available'] == 8.0


class TestIGDBRateLimiter:
    """Test IGDB-specific rate limiter creation."""
    
    def test_igdb_default_config(self):
        """Test IGDB default configuration."""
        assert IGDB_RATE_LIMIT_CONFIG.requests_per_second == 4.0
        assert IGDB_RATE_LIMIT_CONFIG.burst_capacity == 8
        assert IGDB_RATE_LIMIT_CONFIG.backoff_factor == 1.0
        assert IGDB_RATE_LIMIT_CONFIG.max_retries == 3
    
    def test_create_igdb_rate_limiter_default(self):
        """Test creating IGDB rate limiter with default config."""
        client = create_igdb_rate_limiter()
        
        status = client.get_status()
        assert status['requests_per_second'] == 4.0
        assert status['max_tokens'] == 8
    
    def test_create_igdb_rate_limiter_custom(self):
        """Test creating IGDB rate limiter with custom config."""
        custom_config = RateLimitConfig(
            requests_per_second=2.0,
            burst_capacity=4
        )
        client = create_igdb_rate_limiter(custom_config)
        
        status = client.get_status()
        assert status['requests_per_second'] == 2.0
        assert status['max_tokens'] == 4


class TestRateLimitExceeded:
    """Test the RateLimitExceeded exception."""
    
    def test_exception_creation(self):
        """Test creating RateLimitExceeded exception."""
        exc = RateLimitExceeded("Rate limit exceeded", retry_after=1.5)
        
        assert str(exc) == "Rate limit exceeded"
        assert exc.retry_after == 1.5
    
    def test_exception_without_retry_after(self):
        """Test creating exception without retry_after."""
        exc = RateLimitExceeded("Rate limit exceeded")
        
        assert str(exc) == "Rate limit exceeded"
        assert exc.retry_after is None


@pytest.mark.asyncio
async def test_concurrent_rate_limiting():
    """Test rate limiting with concurrent requests."""
    config = RateLimitConfig(requests_per_second=5.0, burst_capacity=3)
    limiter = TokenBucketRateLimiter(config)
    client = RateLimitedClient(limiter)
    
    call_count = 0
    
    async def mock_func():
        nonlocal call_count
        call_count += 1
        await asyncio.sleep(0.01)  # Simulate some work
        return f"result_{call_count}"
    
    # Launch 5 concurrent requests
    tasks = [client.call(mock_func) for _ in range(5)]
    
    # Some should succeed immediately (burst capacity), others should wait
    results = await asyncio.gather(*tasks)
    
    assert len(results) == 5
    assert call_count == 5
    assert all(result.startswith("result_") for result in results)


@pytest.mark.asyncio 
async def test_rate_limiter_under_load():
    """Test rate limiter behavior under sustained load."""
    config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5)
    limiter = TokenBucketRateLimiter(config)
    client = RateLimitedClient(limiter)
    
    successful_calls = 0
    
    async def mock_func():
        nonlocal successful_calls
        successful_calls += 1
        return "success"
    
    # Make 20 calls - should respect rate limit
    start_time = time.monotonic()
    tasks = [client.call(mock_func, timeout=2.0) for _ in range(20)]
    
    results = await asyncio.gather(*tasks, return_exceptions=True)
    end_time = time.monotonic()
    
    # All calls should succeed
    assert len(results) == 20
    assert successful_calls == 20
    
    # Should take at least 1.5 seconds for 20 calls at 10 req/s (minus initial burst)
    # 5 calls from burst capacity + 15 calls at 10 req/s = 1.5 seconds minimum
    assert end_time - start_time >= 1.0  # Allow some tolerance for test timing