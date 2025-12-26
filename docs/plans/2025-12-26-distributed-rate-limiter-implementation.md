# Distributed Rate Limiter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace per-instance token bucket rate limiting with NATS KV-backed distributed rate limiting for IGDB API calls.

**Architecture:** Workers share a single token bucket stored in NATS KV. Token acquisition uses CAS (compare-and-swap) for atomicity. Lazy refill calculates tokens to add on each access. Fail closed when NATS is unavailable.

**Tech Stack:** Python 3.13, nats-py, NATS JetStream KV, pytest, testcontainers

---

## Task 1: Add Configuration Settings

**Files:**
- Modify: `backend/app/core/config.py:89-96`

**Step 1: Add distributed rate limiter settings to config**

Add after the existing IGDB rate limiting settings (line 77):

```python
    # Distributed Rate Limiting
    rate_limiter_nats_bucket: str = Field(
        default="rate-limiters",
        description="NATS KV bucket name for distributed rate limiting"
    )
    rate_limiter_cas_max_retries: int = Field(
        default=10,
        description="Maximum CAS retry attempts for distributed rate limiter"
    )
    rate_limiter_cas_retry_base_ms: int = Field(
        default=5,
        description="Base delay in ms for CAS retry jitter"
    )
    rate_limiter_cas_retry_max_ms: int = Field(
        default=50,
        description="Maximum delay in ms for CAS retry jitter"
    )
```

**Step 2: Verify config loads correctly**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.core.config import settings; print(settings.rate_limiter_nats_bucket)"`
Expected: `rate-limiters`

**Step 3: Commit**

```bash
git add backend/app/core/config.py
git commit -m "feat: add distributed rate limiter configuration settings"
```

---

## Task 2: Create State Model and Exceptions

**Files:**
- Modify: `backend/app/utils/rate_limiter.py:1-33`

**Step 1: Add imports and state model**

Add after the existing imports (line 12):

```python
import json
import random
from dataclasses import asdict
```

Add after `RateLimitConfig` class (line 25):

```python
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
```

**Step 2: Verify imports work**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.utils.rate_limiter import TokenBucketState, CASRetriesExhausted; print('OK')"`
Expected: `OK`

**Step 3: Commit**

```bash
git add backend/app/utils/rate_limiter.py
git commit -m "feat: add TokenBucketState model and CASRetriesExhausted exception"
```

---

## Task 3: Write Failing Tests for DistributedTokenBucketRateLimiter

**Files:**
- Create: `backend/app/tests/test_distributed_rate_limiter.py`

**Step 1: Write the failing tests**

```python
"""
Tests for the distributed rate limiter implementation using NATS KV.
"""

import asyncio
import time
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.utils.rate_limiter import (
    RateLimitConfig,
    TokenBucketState,
    DistributedTokenBucketRateLimiter,
    CASRetriesExhausted,
)


class TestTokenBucketState:
    """Test the token bucket state serialization."""

    def test_to_json(self):
        """Test serialization to JSON."""
        state = TokenBucketState(tokens=4.5, last_refill_at=1703520000.123)
        json_bytes = state.to_json()

        assert b'"tokens": 4.5' in json_bytes
        assert b'"last_refill_at": 1703520000.123' in json_bytes

    def test_from_json(self):
        """Test deserialization from JSON."""
        json_bytes = b'{"tokens": 3.0, "last_refill_at": 1703520000.0}'
        state = TokenBucketState.from_json(json_bytes)

        assert state.tokens == 3.0
        assert state.last_refill_at == 1703520000.0

    def test_roundtrip(self):
        """Test serialization roundtrip."""
        original = TokenBucketState(tokens=7.25, last_refill_at=1703520999.456)
        restored = TokenBucketState.from_json(original.to_json())

        assert restored.tokens == original.tokens
        assert restored.last_refill_at == original.last_refill_at


class TestDistributedTokenBucketRateLimiter:
    """Test the distributed token bucket rate limiter."""

    @pytest.fixture
    def mock_nats(self):
        """Create a mock NATS client with KV store."""
        nats_client = AsyncMock()
        kv_store = AsyncMock()
        js = AsyncMock()

        nats_client.jetstream.return_value = js
        js.key_value.return_value = kv_store

        return nats_client, kv_store, js

    @pytest.fixture
    def config(self):
        """Create a test rate limit config."""
        return RateLimitConfig(
            requests_per_second=4.0,
            burst_capacity=8
        )

    @pytest.mark.asyncio
    async def test_acquire_success(self, mock_nats, config):
        """Test successful token acquisition."""
        nats_client, kv_store, js = mock_nats

        # Setup: bucket exists with tokens available
        entry = MagicMock()
        entry.value = TokenBucketState(tokens=8.0, last_refill_at=time.time()).to_json()
        entry.revision = 1
        kv_store.get.return_value = entry
        kv_store.update.return_value = 2

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        result = await limiter.acquire(1.0)

        assert result is True
        kv_store.update.assert_called_once()

    @pytest.mark.asyncio
    async def test_acquire_insufficient_tokens(self, mock_nats, config):
        """Test acquisition fails when insufficient tokens."""
        nats_client, kv_store, js = mock_nats

        # Setup: bucket exists but empty
        entry = MagicMock()
        entry.value = TokenBucketState(tokens=0.0, last_refill_at=time.time()).to_json()
        entry.revision = 1
        kv_store.get.return_value = entry

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        result = await limiter.acquire(1.0)

        assert result is False
        kv_store.update.assert_not_called()

    @pytest.mark.asyncio
    async def test_lazy_initialization(self, mock_nats, config):
        """Test bucket and key are created lazily on first access."""
        nats_client, kv_store, js = mock_nats

        # Setup: bucket doesn't exist, then key doesn't exist
        from nats.js.errors import BucketNotFoundError, KeyNotFoundError

        js.key_value.side_effect = [BucketNotFoundError(), kv_store]
        js.create_key_value.return_value = kv_store
        kv_store.get.side_effect = KeyNotFoundError()
        kv_store.create.return_value = 1

        # After creation, simulate successful get
        entry = MagicMock()
        entry.value = TokenBucketState(tokens=8.0, last_refill_at=time.time()).to_json()
        entry.revision = 1

        async def get_after_create(key):
            kv_store.get.side_effect = None
            kv_store.get.return_value = entry
            return entry

        kv_store.create.side_effect = lambda k, v: get_after_create(k)

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        # This should trigger lazy initialization
        await limiter.acquire(1.0)

        js.create_key_value.assert_called_once()
        kv_store.create.assert_called_once()

    @pytest.mark.asyncio
    async def test_token_refill_calculation(self, mock_nats, config):
        """Test tokens are refilled based on elapsed time."""
        nats_client, kv_store, js = mock_nats

        # Setup: bucket has 0 tokens but last refill was 2 seconds ago
        # At 4 req/s, should have 8 tokens refilled (capped at burst_capacity)
        old_time = time.time() - 2.0
        entry = MagicMock()
        entry.value = TokenBucketState(tokens=0.0, last_refill_at=old_time).to_json()
        entry.revision = 1
        kv_store.get.return_value = entry
        kv_store.update.return_value = 2

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        result = await limiter.acquire(1.0)

        assert result is True
        # Verify the update was called with refilled tokens
        call_args = kv_store.update.call_args
        updated_state = TokenBucketState.from_json(call_args[0][1])
        assert updated_state.tokens >= 6.0  # 8 refilled - 1 acquired, with some tolerance

    @pytest.mark.asyncio
    async def test_cas_retry_on_conflict(self, mock_nats, config):
        """Test CAS conflict triggers retry with jitter."""
        nats_client, kv_store, js = mock_nats

        from nats.js.errors import KeyWrongLastSequenceError

        entry = MagicMock()
        entry.value = TokenBucketState(tokens=8.0, last_refill_at=time.time()).to_json()
        entry.revision = 1
        kv_store.get.return_value = entry

        # First update fails with CAS conflict, second succeeds
        kv_store.update.side_effect = [KeyWrongLastSequenceError(), 2]

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config,
            max_cas_retries=3,
            cas_retry_base_ms=1,
            cas_retry_max_ms=5
        )

        with patch('asyncio.sleep', new_callable=AsyncMock) as mock_sleep:
            result = await limiter.acquire(1.0)

        assert result is True
        assert kv_store.update.call_count == 2
        mock_sleep.assert_called_once()

    @pytest.mark.asyncio
    async def test_cas_retries_exhausted(self, mock_nats, config):
        """Test CASRetriesExhausted raised when all retries fail."""
        nats_client, kv_store, js = mock_nats

        from nats.js.errors import KeyWrongLastSequenceError

        entry = MagicMock()
        entry.value = TokenBucketState(tokens=8.0, last_refill_at=time.time()).to_json()
        entry.revision = 1
        kv_store.get.return_value = entry

        # All updates fail with CAS conflict
        kv_store.update.side_effect = KeyWrongLastSequenceError()

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config,
            max_cas_retries=3,
            cas_retry_base_ms=1,
            cas_retry_max_ms=5
        )

        with patch('asyncio.sleep', new_callable=AsyncMock):
            with pytest.raises(CASRetriesExhausted):
                await limiter.acquire(1.0)

    @pytest.mark.asyncio
    async def test_nats_unavailable_returns_false(self, mock_nats, config):
        """Test NATS unavailability returns False (fail closed)."""
        nats_client, kv_store, js = mock_nats

        # Simulate NATS connection error
        js.key_value.side_effect = Exception("Connection refused")

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        result = await limiter.acquire(1.0)

        assert result is False

    @pytest.mark.asyncio
    async def test_wait_for_tokens_success(self, mock_nats, config):
        """Test waiting for tokens to become available."""
        nats_client, kv_store, js = mock_nats

        call_count = 0

        def get_entry():
            nonlocal call_count
            call_count += 1
            entry = MagicMock()
            # First call: no tokens, subsequent calls: tokens available
            if call_count == 1:
                entry.value = TokenBucketState(tokens=0.0, last_refill_at=time.time()).to_json()
            else:
                entry.value = TokenBucketState(tokens=8.0, last_refill_at=time.time()).to_json()
            entry.revision = call_count
            return entry

        kv_store.get.side_effect = lambda k: get_entry()
        kv_store.update.return_value = 2

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        with patch('asyncio.sleep', new_callable=AsyncMock):
            result = await limiter.wait_for_tokens(1.0, timeout=1.0)

        assert result is True

    @pytest.mark.asyncio
    async def test_wait_for_tokens_timeout(self, mock_nats, config):
        """Test timeout when waiting for tokens."""
        nats_client, kv_store, js = mock_nats

        # Always return empty bucket
        entry = MagicMock()
        entry.value = TokenBucketState(tokens=0.0, last_refill_at=time.time()).to_json()
        entry.revision = 1
        kv_store.get.return_value = entry

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        result = await limiter.wait_for_tokens(1.0, timeout=0.1)

        assert result is False

    @pytest.mark.asyncio
    async def test_get_status(self, mock_nats, config):
        """Test status reporting."""
        nats_client, kv_store, js = mock_nats

        entry = MagicMock()
        entry.value = TokenBucketState(tokens=4.0, last_refill_at=time.time()).to_json()
        entry.revision = 1
        kv_store.get.return_value = entry

        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="igdb",
            config=config
        )

        status = await limiter.get_status()

        assert status['tokens_available'] == 4.0
        assert status['max_tokens'] == 8
        assert status['requests_per_second'] == 4.0
        assert status['utilization'] == 0.5  # 4/8 tokens used
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_distributed_rate_limiter.py -v 2>&1 | head -50`
Expected: Tests fail with `ImportError: cannot import name 'DistributedTokenBucketRateLimiter'`

**Step 3: Commit failing tests**

```bash
git add backend/app/tests/test_distributed_rate_limiter.py
git commit -m "test: add failing tests for DistributedTokenBucketRateLimiter"
```

---

## Task 4: Implement DistributedTokenBucketRateLimiter

**Files:**
- Modify: `backend/app/utils/rate_limiter.py`

**Step 1: Add the DistributedTokenBucketRateLimiter class**

Add after the `TokenBucketRateLimiter` class (after line 132):

```python
class DistributedTokenBucketRateLimiter:
    """
    Distributed token bucket rate limiter using NATS KV.

    This implementation stores token bucket state in NATS KV for coordination
    across multiple workers. Uses CAS (compare-and-swap) for atomic updates.
    """

    def __init__(
        self,
        nats_client,
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
        self._kv = None
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
            except Exception as e:
                if "bucket not found" in str(e).lower():
                    # Create bucket
                    logger.info(f"Creating NATS KV bucket: {self._bucket_name}")
                    self._kv = await js.create_key_value(bucket=self._bucket_name)
                else:
                    raise

            # Try to get existing key, create if not exists
            try:
                await self._kv.get(self._resource_name)
            except Exception as e:
                if "key not found" in str(e).lower() or "no message found" in str(e).lower():
                    # Create initial state with full bucket
                    initial_state = TokenBucketState(
                        tokens=float(self.config.burst_capacity),
                        last_refill_at=time.time()
                    )
                    logger.info(f"Creating initial rate limiter state for: {self._resource_name}")
                    await self._kv.create(self._resource_name, initial_state.to_json())
                else:
                    raise

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
                except Exception as e:
                    if "wrong last sequence" in str(e).lower():
                        # CAS conflict, retry with jitter
                        jitter_ms = random.uniform(self._cas_retry_base_ms, self._cas_retry_max_ms)
                        logger.debug(f"CAS conflict, retrying in {jitter_ms:.1f}ms")
                        await asyncio.sleep(jitter_ms / 1000)
                        continue
                    raise

            except Exception as e:
                if "wrong last sequence" in str(e).lower():
                    # CAS conflict from update, continue retry loop
                    jitter_ms = random.uniform(self._cas_retry_base_ms, self._cas_retry_max_ms)
                    await asyncio.sleep(jitter_ms / 1000)
                    continue
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
```

**Step 2: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_distributed_rate_limiter.py -v`
Expected: All tests pass

**Step 3: Verify type checking passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/utils/rate_limiter.py`
Expected: No type errors

**Step 4: Commit**

```bash
git add backend/app/utils/rate_limiter.py
git commit -m "feat: implement DistributedTokenBucketRateLimiter with NATS KV"
```

---

## Task 5: Create NATS Client Utility

**Files:**
- Create: `backend/app/utils/nats_client.py`

**Step 1: Write the NATS client singleton**

```python
"""
NATS client singleton for application-wide access.

This module provides a shared NATS client instance that can be used
across the application for distributed coordination (rate limiting, etc.)
"""

import logging
from typing import Optional

import nats
from nats.aio.client import Client as NATSClient

from app.core.config import settings

logger = logging.getLogger(__name__)

_nats_client: Optional[NATSClient] = None


async def get_nats_client() -> NATSClient:
    """
    Get the shared NATS client instance.

    Creates a new connection if one doesn't exist.

    Returns:
        Connected NATS client
    """
    global _nats_client

    if _nats_client is None or not _nats_client.is_connected:
        logger.info(f"Connecting to NATS at {settings.NATS_URL}")
        _nats_client = await nats.connect(settings.NATS_URL)
        logger.info("NATS connection established")

    return _nats_client


async def close_nats_client() -> None:
    """Close the NATS client connection."""
    global _nats_client

    if _nats_client is not None and _nats_client.is_connected:
        logger.info("Closing NATS connection")
        await _nats_client.close()
        _nats_client = None
```

**Step 2: Verify module loads**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.utils.nats_client import get_nats_client; print('OK')"`
Expected: `OK`

**Step 3: Commit**

```bash
git add backend/app/utils/nats_client.py
git commit -m "feat: add NATS client singleton utility"
```

---

## Task 6: Create Distributed Rate Limiter Factory

**Files:**
- Modify: `backend/app/utils/rate_limiter.py`

**Step 1: Add factory function for distributed rate limiter**

Add at the end of the file:

```python
async def create_distributed_igdb_rate_limiter(
    nats_client,
    config: Optional[RateLimitConfig] = None,
    bucket_name: str = "rate-limiters",
    max_cas_retries: int = 10,
    cas_retry_base_ms: int = 5,
    cas_retry_max_ms: int = 50,
) -> "DistributedRateLimitedClient":
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
```

**Step 2: Update exports in rate_limiter module**

Verify imports work:

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.utils.rate_limiter import create_distributed_igdb_rate_limiter, DistributedRateLimitedClient; print('OK')"`
Expected: `OK`

**Step 3: Commit**

```bash
git add backend/app/utils/rate_limiter.py
git commit -m "feat: add DistributedRateLimitedClient and factory function"
```

---

## Task 7: Update IGDBService to Use Distributed Rate Limiter

**Files:**
- Modify: `backend/app/services/igdb/service.py`

**Step 1: Update imports**

Replace lines 16-20:

```python
from app.utils.rate_limiter import (
    RateLimitConfig,
    create_igdb_rate_limiter,
    create_distributed_igdb_rate_limiter,
    RateLimitExceeded
)
from app.utils.nats_client import get_nats_client
from app.core.config import settings
```

**Step 2: Update IGDBService.__init__ to accept optional rate limiter**

Replace the `__init__` method (lines 39-58):

```python
    def __init__(self, rate_limiter=None):
        self._http_client = httpx.AsyncClient()
        self._auth_manager = IGDBAuthManager(self._http_client)
        self._rate_limiter = rate_limiter
        self._rate_limiter_initialized = rate_limiter is not None

        # Backwards compatibility: expose wrapper reference
        self._wrapper: Any = None  # Lazily set by _auth_manager.get_wrapper()

    async def _ensure_rate_limiter(self):
        """Ensure rate limiter is initialized."""
        if self._rate_limiter_initialized:
            return

        # Create distributed rate limiter
        rate_config = RateLimitConfig(
            requests_per_second=settings.igdb_requests_per_second,
            burst_capacity=settings.igdb_burst_capacity,
            backoff_factor=settings.igdb_backoff_factor,
            max_retries=settings.igdb_max_retries
        )

        try:
            nats_client = await get_nats_client()
            self._rate_limiter = await create_distributed_igdb_rate_limiter(
                nats_client=nats_client,
                config=rate_config,
                bucket_name=settings.rate_limiter_nats_bucket,
                max_cas_retries=settings.rate_limiter_cas_max_retries,
                cas_retry_base_ms=settings.rate_limiter_cas_retry_base_ms,
                cas_retry_max_ms=settings.rate_limiter_cas_retry_max_ms,
            )
            logger.info(
                f"IGDB service initialized with distributed rate limiting: "
                f"{rate_config.requests_per_second} req/s, burst: {rate_config.burst_capacity}"
            )
        except Exception as e:
            logger.warning(f"Failed to create distributed rate limiter, falling back to local: {e}")
            self._rate_limiter = create_igdb_rate_limiter(rate_config)
            logger.info(
                f"IGDB service initialized with local rate limiting: "
                f"{rate_config.requests_per_second} req/s, burst: {rate_config.burst_capacity}"
            )

        self._rate_limiter_initialized = True
```

**Step 3: Update _rate_limited_api_request to ensure rate limiter**

Update the `_rate_limited_api_request` method, add at the start:

```python
    async def _rate_limited_api_request(self, endpoint: str, query: str) -> bytes:
        """
        Make a rate-limited IGDB API request.

        Args:
            endpoint: IGDB API endpoint (e.g., 'games', 'game_time_to_beats')
            query: IGDB query string

        Returns:
            Raw response bytes from IGDB API

        Raises:
            IGDBError: If API request fails
            RateLimitExceeded: If rate limit cannot be satisfied
        """
        await self._ensure_rate_limiter()

        try:
            # ... rest of the method unchanged
```

**Step 4: Update get_rate_limiter_status to be async**

Replace the `get_rate_limiter_status` method:

```python
    async def get_rate_limiter_status(self) -> dict:
        """
        Get current rate limiter status for monitoring.

        Returns:
            Dictionary with rate limiter status information
        """
        await self._ensure_rate_limiter()

        if hasattr(self._rate_limiter, 'get_status'):
            result = self._rate_limiter.get_status()
            # Handle both sync and async get_status
            if asyncio.iscoroutine(result):
                return await result
            return result
        return {}
```

**Step 5: Verify type checking passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/services/igdb/service.py`
Expected: No errors (or only pre-existing ones)

**Step 6: Commit**

```bash
git add backend/app/services/igdb/service.py
git commit -m "feat: update IGDBService to use distributed rate limiter"
```

---

## Task 8: Update Docker Compose

**Files:**
- Modify: `docker-compose.yml`

**Step 1: Remove per-worker rate limit overrides**

Remove lines 126-129 from the worker service environment:

```yaml
      # Lower IGDB rate limits for workers to prevent violations from concurrent tasks
      # API uses full 4 req/s; workers use 1 req/s each to share the limit safely
      IGDB_REQUESTS_PER_SECOND: "1.0"
      IGDB_BURST_CAPACITY: "2"
```

**Step 2: Verify docker-compose config is valid**

Run: `cd /home/abo/workspace/home/nexorious && podman-compose config > /dev/null && echo "Config valid"`
Expected: `Config valid`

**Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "chore: remove per-worker IGDB rate limits (now distributed)"
```

---

## Task 9: Update Existing Rate Limiter Tests

**Files:**
- Modify: `backend/app/tests/test_rate_limited_igdb_service.py`

**Step 1: Update tests to work with async rate limiter**

Read the existing test file and update any tests that call `get_rate_limiter_status()` to use `await`. Also ensure tests inject a local rate limiter to avoid needing NATS.

**Step 2: Run existing tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_rate_limited_igdb_service.py -v`
Expected: All tests pass

**Step 3: Commit if changes were needed**

```bash
git add backend/app/tests/test_rate_limited_igdb_service.py
git commit -m "test: update IGDB service tests for async rate limiter"
```

---

## Task 10: Write Integration Test with Real NATS

**Files:**
- Create: `backend/app/tests/test_distributed_rate_limiter_integration.py`

**Step 1: Write integration test using testcontainers**

```python
"""
Integration tests for distributed rate limiter with real NATS.
"""

import asyncio
import time
import pytest

import nats
from testcontainers.nats import NatsContainer

from app.utils.rate_limiter import (
    RateLimitConfig,
    DistributedTokenBucketRateLimiter,
    create_distributed_igdb_rate_limiter,
)


@pytest.fixture(scope="module")
def nats_container():
    """Start a NATS container for testing."""
    with NatsContainer() as container:
        yield container


@pytest.fixture
async def nats_client(nats_container):
    """Create a NATS client connected to the test container."""
    url = nats_container.nats_uri()
    client = await nats.connect(url)
    yield client
    await client.close()


class TestDistributedRateLimiterIntegration:
    """Integration tests with real NATS."""

    @pytest.mark.asyncio
    async def test_basic_acquire(self, nats_client):
        """Test basic token acquisition with real NATS."""
        config = RateLimitConfig(requests_per_second=4.0, burst_capacity=8)
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="test_basic",
            config=config
        )

        # Should successfully acquire tokens
        assert await limiter.acquire(1.0) is True
        assert await limiter.acquire(1.0) is True

        # Check status
        status = await limiter.get_status()
        assert status['tokens_available'] < 8.0

    @pytest.mark.asyncio
    async def test_multi_worker_simulation(self, nats_client):
        """Test multiple concurrent workers sharing rate limit."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5)

        # Create multiple "workers" (same limiter, simulating distributed workers)
        workers = [
            DistributedTokenBucketRateLimiter(
                nats_client=nats_client,
                resource_name="test_multi_worker",
                config=config
            )
            for _ in range(3)
        ]

        acquired_count = 0

        async def worker_task(limiter, num_requests):
            nonlocal acquired_count
            for _ in range(num_requests):
                if await limiter.acquire(1.0):
                    acquired_count += 1
                await asyncio.sleep(0.01)

        # Each worker tries to acquire 5 tokens
        tasks = [worker_task(w, 5) for w in workers]
        await asyncio.gather(*tasks)

        # Total acquired should respect the burst capacity initially
        # then refill over time
        assert acquired_count >= 5  # At least burst capacity
        assert acquired_count <= 15  # At most all requested

    @pytest.mark.asyncio
    async def test_token_refill_over_time(self, nats_client):
        """Test that tokens refill correctly over time."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=5)
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="test_refill",
            config=config
        )

        # Exhaust the bucket
        for _ in range(5):
            await limiter.acquire(1.0)

        # Should fail immediately
        assert await limiter.acquire(1.0) is False

        # Wait for refill (0.2s should add 2 tokens at 10 req/s)
        await asyncio.sleep(0.25)

        # Should succeed now
        assert await limiter.acquire(1.0) is True

    @pytest.mark.asyncio
    async def test_wait_for_tokens(self, nats_client):
        """Test waiting for tokens to become available."""
        config = RateLimitConfig(requests_per_second=10.0, burst_capacity=2)
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name="test_wait",
            config=config
        )

        # Exhaust bucket
        await limiter.acquire(2.0)

        # Wait for tokens
        start = time.monotonic()
        result = await limiter.wait_for_tokens(1.0, timeout=1.0)
        elapsed = time.monotonic() - start

        assert result is True
        assert elapsed > 0.05  # Should have waited some time
        assert elapsed < 0.5  # But not too long

    @pytest.mark.asyncio
    async def test_distributed_rate_limited_client(self, nats_client):
        """Test the full DistributedRateLimitedClient wrapper."""
        config = RateLimitConfig(
            requests_per_second=10.0,
            burst_capacity=5,
            max_retries=2
        )

        client = await create_distributed_igdb_rate_limiter(
            nats_client=nats_client,
            config=config,
            bucket_name="test-rate-limiters"
        )

        call_count = 0

        async def mock_api_call():
            nonlocal call_count
            call_count += 1
            return f"result_{call_count}"

        # Make several calls
        results = []
        for _ in range(5):
            result = await client.call(mock_api_call)
            results.append(result)

        assert len(results) == 5
        assert call_count == 5
```

**Step 2: Run integration tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_distributed_rate_limiter_integration.py -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add backend/app/tests/test_distributed_rate_limiter_integration.py
git commit -m "test: add integration tests for distributed rate limiter with real NATS"
```

---

## Task 11: Run Full Test Suite

**Files:** None (verification only)

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass with >80% coverage

**Step 2: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No type errors

**Step 3: Run linting**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run ruff check .`
Expected: No errors

---

## Task 12: Update Documentation

**Files:**
- Modify: `backend/README.md`

**Step 1: Update rate limiting documentation**

Find the IGDB rate limiting section and update to mention distributed rate limiting via NATS KV.

**Step 2: Commit**

```bash
git add backend/README.md
git commit -m "docs: update README with distributed rate limiting info"
```
