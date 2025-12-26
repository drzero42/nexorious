"""
Tests for the distributed rate limiter implementation using NATS KV.
"""

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
