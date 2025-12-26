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
import random

# NATS KV error types for proper exception handling
from nats.js.errors import (
    BucketNotFoundError,
    KeyNotFoundError,
    KeyWrongLastSequenceError,
)

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


class DistributedTokenBucketRateLimiter:
    """
    Distributed token bucket rate limiter using NATS KV.

    This implementation stores token bucket state in NATS KV for coordination
    across multiple workers. Uses CAS (compare-and-swap) for atomic updates.
    """

    def __init__(
        self,
        nats_client: Any,
        resource_name: str,
        config: RateLimitConfig,
        bucket_name: str = "rate-limiters",
        max_cas_retries: int = 10,
        cas_retry_base_ms: int = 5,
        cas_retry_max_ms: int = 50,
    ):
        """
        Initialize the distributed rate limiter.

        Args:
            nats_client: Connected NATS client
            resource_name: Name of the resource being rate limited (e.g., "igdb")
            config: Rate limiting configuration
            bucket_name: NATS KV bucket name
            max_cas_retries: Maximum CAS retry attempts
            cas_retry_base_ms: Minimum jitter delay in milliseconds
            cas_retry_max_ms: Maximum jitter delay in milliseconds
        """
        self._nats = nats_client
        self._resource_name = resource_name
        self.config = config
        self._bucket_name = bucket_name
        self._max_cas_retries = max_cas_retries
        self._cas_retry_base_ms = cas_retry_base_ms
        self._cas_retry_max_ms = cas_retry_max_ms
        self._kv: Any = None
        self._initialized = False

        logger.info(
            f"Initialized distributed rate limiter for '{resource_name}': "
            f"{config.requests_per_second} req/s, burst: {config.burst_capacity}"
        )

    async def _ensure_initialized(self) -> bool:
        """
        Ensure KV bucket and key exist. Returns False if NATS unavailable.
        """
        if self._initialized and self._kv is not None:
            return True

        try:
            js = self._nats.jetstream()

            # Try to get existing bucket
            try:
                self._kv = await js.key_value(self._bucket_name)
            except BucketNotFoundError:
                # Create bucket
                logger.info(f"Creating NATS KV bucket: {self._bucket_name}")
                self._kv = await js.create_key_value(bucket=self._bucket_name)

            # Try to get existing key, create if not exists
            try:
                await self._kv.get(self._resource_name)
            except KeyNotFoundError:
                # Create initial state with full bucket
                initial_state = TokenBucketState(
                    tokens=float(self.config.burst_capacity),
                    last_refill_at=time.time()
                )
                logger.info(f"Creating initial rate limiter state for: {self._resource_name}")
                await self._kv.create(self._resource_name, initial_state.to_json())

            self._initialized = True
            return True

        except Exception as e:
            logger.warning(f"Failed to initialize distributed rate limiter: {e}")
            return False

    def _calculate_refill(self, state: TokenBucketState) -> TokenBucketState:
        """Calculate new state with refilled tokens."""
        now = time.time()
        elapsed = now - state.last_refill_at
        tokens_to_add = elapsed * self.config.requests_per_second
        new_tokens = min(state.tokens + tokens_to_add, float(self.config.burst_capacity))

        return TokenBucketState(tokens=new_tokens, last_refill_at=now)

    async def acquire(self, tokens_needed: float = 1.0) -> bool:
        """
        Acquire tokens from the distributed bucket.

        Args:
            tokens_needed: Number of tokens to acquire

        Returns:
            True if tokens were acquired, False if unavailable or NATS error
        """
        if not await self._ensure_initialized():
            logger.warning("Rate limiter not initialized, failing closed")
            return False

        for attempt in range(self._max_cas_retries):
            try:
                # Read current state
                entry = await self._kv.get(self._resource_name)
                state = TokenBucketState.from_json(entry.value)
                revision = entry.revision

                # Calculate refill
                state = self._calculate_refill(state)

                # Check if we have enough tokens
                if state.tokens < tokens_needed:
                    logger.debug(
                        f"Insufficient tokens: need {tokens_needed}, have {state.tokens:.1f}"
                    )
                    return False

                # Deduct tokens and update
                state.tokens -= tokens_needed

                try:
                    await self._kv.update(self._resource_name, state.to_json(), revision)
                    logger.debug(f"Acquired {tokens_needed} tokens, {state.tokens:.1f} remaining")
                    return True
                except KeyWrongLastSequenceError:
                    # CAS conflict, retry with jitter
                    jitter_ms = random.uniform(self._cas_retry_base_ms, self._cas_retry_max_ms)
                    logger.debug(f"CAS conflict, retrying in {jitter_ms:.1f}ms")
                    await asyncio.sleep(jitter_ms / 1000)
                    continue

            except KeyWrongLastSequenceError:
                # CAS conflict from update, continue retry loop
                jitter_ms = random.uniform(self._cas_retry_base_ms, self._cas_retry_max_ms)
                await asyncio.sleep(jitter_ms / 1000)
                continue
            except Exception as e:
                logger.warning(f"Error acquiring tokens: {e}")
                return False

        raise CASRetriesExhausted(
            f"Failed to acquire tokens after {self._max_cas_retries} CAS retries"
        )

    async def wait_for_tokens(
        self, tokens_needed: float = 1.0, timeout: Optional[float] = None
    ) -> bool:
        """
        Wait until enough tokens are available.

        Args:
            tokens_needed: Number of tokens needed
            timeout: Maximum time to wait (None for no timeout)

        Returns:
            True if tokens were acquired, False if timeout or NATS error
        """
        start_time = time.monotonic()

        while True:
            try:
                if await self.acquire(tokens_needed):
                    return True
            except CASRetriesExhausted:
                pass  # Continue waiting

            # Check timeout
            if timeout is not None:
                elapsed = time.monotonic() - start_time
                if elapsed >= timeout:
                    logger.warning(f"Rate limiter timeout after {elapsed:.1f}s")
                    return False

            # Wait before retry
            wait_time = min(0.1, 1.0 / self.config.requests_per_second / 4)
            await asyncio.sleep(wait_time)

    async def get_status(self) -> dict:
        """
        Get current rate limiter status.

        Returns:
            Dictionary with current status information
        """
        if not await self._ensure_initialized():
            return {
                "tokens_available": 0,
                "max_tokens": self.config.burst_capacity,
                "requests_per_second": self.config.requests_per_second,
                "utilization": 1.0,
                "error": "NATS unavailable"
            }

        try:
            entry = await self._kv.get(self._resource_name)
            state = TokenBucketState.from_json(entry.value)
            state = self._calculate_refill(state)

            return {
                "tokens_available": state.tokens,
                "max_tokens": self.config.burst_capacity,
                "requests_per_second": self.config.requests_per_second,
                "utilization": 1.0 - (state.tokens / self.config.burst_capacity)
            }
        except Exception as e:
            logger.warning(f"Error getting rate limiter status: {e}")
            return {
                "tokens_available": 0,
                "max_tokens": self.config.burst_capacity,
                "requests_per_second": self.config.requests_per_second,
                "utilization": 1.0,
                "error": str(e)
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


class DistributedRateLimitedClient:
    """
    A wrapper that adds distributed rate limiting to async function calls.

    This class wraps DistributedTokenBucketRateLimiter with automatic retries
    and exponential backoff, providing the same interface as RateLimitedClient.
    """

    def __init__(self, rate_limiter: DistributedTokenBucketRateLimiter):
        """
        Initialize the distributed rate limited client.

        Args:
            rate_limiter: The distributed rate limiter to use
        """
        self.rate_limiter = rate_limiter
        self.config = rate_limiter.config

    async def call(
        self,
        func: Callable[[], Awaitable[Any]],
        timeout: Optional[float] = 30.0,
        tokens_needed: float = 1.0
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
                logger.debug(f"Making distributed rate-limited call (attempt {attempt + 1})")
                result = await func()

                if attempt > 0:
                    logger.info(f"Distributed rate-limited call succeeded on attempt {attempt + 1}")

                return result

            except RateLimitExceeded:
                # Don't retry on rate limit exceeded
                raise
            except CASRetriesExhausted:
                # Don't retry on CAS exhaustion
                raise RateLimitExceeded("Rate limiter CAS retries exhausted")
            except Exception as e:
                last_exception = e

                if attempt < self.config.max_retries:
                    # Calculate backoff delay
                    delay = self.config.backoff_factor * (2 ** attempt)
                    logger.warning(
                        f"Distributed rate-limited call failed (attempt {attempt + 1}/{self.config.max_retries + 1}): {str(e)}, "
                        f"retrying in {delay:.1f}s"
                    )
                    await asyncio.sleep(delay)
                else:
                    logger.error(
                        f"Distributed rate-limited call failed after {self.config.max_retries + 1} attempts: {str(e)}"
                    )
                    break

        # If we get here, all retries failed
        raise last_exception or Exception("All retries failed")

    async def get_status(self) -> dict:
        """Get current status of the rate limiter."""
        return await self.rate_limiter.get_status()


async def create_distributed_igdb_rate_limiter(
    nats_client: Any,
    config: Optional[RateLimitConfig] = None,
    bucket_name: str = "rate-limiters",
    max_cas_retries: int = 10,
    cas_retry_base_ms: int = 5,
    cas_retry_max_ms: int = 50,
) -> DistributedRateLimitedClient:
    """
    Create a distributed rate limiter configured for IGDB API calls.

    Args:
        nats_client: Connected NATS client
        config: Optional custom configuration, uses IGDB defaults if None
        bucket_name: NATS KV bucket name
        max_cas_retries: Maximum CAS retry attempts
        cas_retry_base_ms: Minimum jitter delay in milliseconds
        cas_retry_max_ms: Maximum jitter delay in milliseconds

    Returns:
        DistributedRateLimitedClient configured for IGDB
    """
    if config is None:
        config = IGDB_RATE_LIMIT_CONFIG

    rate_limiter = DistributedTokenBucketRateLimiter(
        nats_client=nats_client,
        resource_name="igdb",
        config=config,
        bucket_name=bucket_name,
        max_cas_retries=max_cas_retries,
        cas_retry_base_ms=cas_retry_base_ms,
        cas_retry_max_ms=cas_retry_max_ms,
    )
    return DistributedRateLimitedClient(rate_limiter)