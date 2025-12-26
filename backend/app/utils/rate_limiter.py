"""
Rate limiter implementation using token bucket algorithm.

This module provides a rate limiter that can be used to control the rate of API calls
to external services like IGDB, preventing rate limit violations.
"""

import asyncio
import time
import logging
from typing import Optional, Callable, Any, Awaitable
from dataclasses import dataclass, asdict
import json
import random  # noqa: F401 - used in later tasks for jitter

logger = logging.getLogger(__name__)


@dataclass
class RateLimitConfig:
    """Configuration for rate limiting."""

    requests_per_second: float = 4.0
    burst_capacity: int = 10
    backoff_factor: float = 1.0
    max_retries: int = 3


@dataclass
class TokenBucketState:
    """State stored in NATS KV for distributed rate limiting."""

    tokens: float
    last_refill_at: float

    def to_json(self) -> bytes:
        """Serialize state to JSON bytes."""
        return json.dumps(asdict(self)).encode('utf-8')

    @classmethod
    def from_json(cls, data: bytes) -> "TokenBucketState":
        """Deserialize state from JSON bytes."""
        parsed = json.loads(data.decode('utf-8'))
        return cls(tokens=parsed['tokens'], last_refill_at=parsed['last_refill_at'])


class CASRetriesExhausted(Exception):
    """Exception raised when CAS retries are exhausted."""
    pass


class RateLimitExceeded(Exception):
    """Exception raised when rate limit is exceeded and retries are exhausted."""
    
    def __init__(self, message: str, retry_after: Optional[float] = None):
        super().__init__(message)
        self.retry_after = retry_after


class TokenBucketRateLimiter:
    """
    Token bucket rate limiter for controlling API request rates.
    
    This implementation uses a token bucket algorithm to allow bursts of requests
    up to the bucket capacity while maintaining the average rate over time.
    """
    
    def __init__(self, config: RateLimitConfig):
        """
        Initialize the rate limiter.
        
        Args:
            config: Rate limiting configuration
        """
        self.config = config
        self.tokens = float(config.burst_capacity)
        self.last_refill = time.monotonic()
        self._lock = asyncio.Lock()
        
        logger.info(
            f"Initialized rate limiter: {config.requests_per_second} req/s, "
            f"burst capacity: {config.burst_capacity}"
        )
    
    async def acquire(self, tokens_needed: int = 1) -> bool:
        """
        Acquire tokens from the bucket.
        
        Args:
            tokens_needed: Number of tokens to acquire
            
        Returns:
            True if tokens were acquired, False if not enough tokens available
        """
        async with self._lock:
            now = time.monotonic()
            
            # Refill tokens based on elapsed time
            time_elapsed = now - self.last_refill
            tokens_to_add = time_elapsed * self.config.requests_per_second
            self.tokens = min(self.config.burst_capacity, self.tokens + tokens_to_add)
            self.last_refill = now
            
            # Check if we have enough tokens
            if self.tokens >= tokens_needed:
                self.tokens -= tokens_needed
                logger.debug(f"Acquired {tokens_needed} tokens, {self.tokens:.1f} remaining")
                return True
            else:
                logger.debug(f"Insufficient tokens: need {tokens_needed}, have {self.tokens:.1f}")
                return False
    
    async def wait_for_tokens(self, tokens_needed: int = 1, timeout: Optional[float] = None) -> bool:
        """
        Wait until enough tokens are available.
        
        Args:
            tokens_needed: Number of tokens needed
            timeout: Maximum time to wait (None for no timeout)
            
        Returns:
            True if tokens were acquired, False if timeout occurred
        """
        start_time = time.monotonic()
        
        while True:
            if await self.acquire(tokens_needed):
                return True
            
            # Check timeout
            if timeout is not None:
                elapsed = time.monotonic() - start_time
                if elapsed >= timeout:
                    logger.warning(f"Rate limiter timeout after {elapsed:.1f}s")
                    return False
            
            # Calculate wait time until next token is available
            async with self._lock:
                time_until_token = 1.0 / self.config.requests_per_second
                
            # Wait a fraction of the token refill time
            wait_time = min(0.1, time_until_token / 4)
            await asyncio.sleep(wait_time)
    
    def get_status(self) -> dict:
        """
        Get current rate limiter status.
        
        Returns:
            Dictionary with current status information
        """
        return {
            "tokens_available": self.tokens,
            "max_tokens": self.config.burst_capacity,
            "requests_per_second": self.config.requests_per_second,
            "utilization": 1.0 - (self.tokens / self.config.burst_capacity)
        }


class RateLimitedClient:
    """
    A wrapper that adds rate limiting to async function calls.
    
    This class can be used to wrap any async function with rate limiting,
    providing automatic retries with exponential backoff.
    """
    
    def __init__(self, rate_limiter: TokenBucketRateLimiter):
        """
        Initialize the rate limited client.
        
        Args:
            rate_limiter: The rate limiter to use
        """
        self.rate_limiter = rate_limiter
        self.config = rate_limiter.config
    
    async def call(
        self,
        func: Callable[[], Awaitable[Any]],
        timeout: Optional[float] = 30.0,
        tokens_needed: int = 1
    ) -> Any:
        """
        Make a rate-limited function call with retries.
        
        Args:
            func: Async function to call
            timeout: Timeout for acquiring rate limit tokens
            tokens_needed: Number of tokens needed for this call
            
        Returns:
            Result of the function call
            
        Raises:
            RateLimitExceeded: If rate limit cannot be satisfied
            Any exception raised by the function
        """
        last_exception = None
        
        for attempt in range(self.config.max_retries + 1):
            try:
                # Wait for rate limit tokens
                if not await self.rate_limiter.wait_for_tokens(tokens_needed, timeout):
                    retry_after = 1.0 / self.config.requests_per_second
                    raise RateLimitExceeded(
                        f"Rate limit exceeded, could not acquire {tokens_needed} tokens within {timeout}s",
                        retry_after=retry_after
                    )
                
                # Make the actual call
                logger.debug(f"Making rate-limited call (attempt {attempt + 1})")
                result = await func()
                
                if attempt > 0:
                    logger.info(f"Rate-limited call succeeded on attempt {attempt + 1}")
                
                return result
                
            except RateLimitExceeded:
                # Don't retry on rate limit exceeded
                raise
            except Exception as e:
                last_exception = e
                
                if attempt < self.config.max_retries:
                    # Calculate backoff delay
                    delay = self.config.backoff_factor * (2 ** attempt)
                    logger.warning(
                        f"Rate-limited call failed (attempt {attempt + 1}/{self.config.max_retries + 1}): {str(e)}, "
                        f"retrying in {delay:.1f}s"
                    )
                    await asyncio.sleep(delay)
                else:
                    logger.error(
                        f"Rate-limited call failed after {self.config.max_retries + 1} attempts: {str(e)}"
                    )
                    break
        
        # If we get here, all retries failed
        raise last_exception or Exception("All retries failed")
    
    def get_status(self) -> dict:
        """Get current status of the rate limiter."""
        return self.rate_limiter.get_status()


# Default rate limiter configuration for IGDB
IGDB_RATE_LIMIT_CONFIG = RateLimitConfig(
    requests_per_second=4.0,  # IGDB limit is 4 requests per second
    burst_capacity=8,         # Allow brief bursts up to 8 requests
    backoff_factor=1.0,       # 1 second base backoff
    max_retries=3             # Retry up to 3 times
)


def create_igdb_rate_limiter(config: Optional[RateLimitConfig] = None) -> RateLimitedClient:
    """
    Create a rate limiter configured for IGDB API calls.
    
    Args:
        config: Optional custom configuration, uses IGDB defaults if None
        
    Returns:
        RateLimitedClient configured for IGDB
    """
    if config is None:
        config = IGDB_RATE_LIMIT_CONFIG
    
    rate_limiter = TokenBucketRateLimiter(config)
    return RateLimitedClient(rate_limiter)