"""
Integration tests for the distributed rate limiter using real NATS container.

These tests use testcontainers to spin up a real NATS instance with JetStream
to verify the distributed rate limiter works correctly in a realistic environment.
"""

import asyncio
import time
import uuid
import pytest
import pytest_asyncio
import nats
from testcontainers.nats import NatsContainer

from app.utils.rate_limiter import (
    RateLimitConfig,
    DistributedTokenBucketRateLimiter,
    create_distributed_igdb_rate_limiter,
)


@pytest.fixture(scope="module")
def nats_container():
    """
    Module-scoped fixture that starts a NATS container with JetStream enabled.

    The container is shared across all tests in this module for efficiency.
    """
    with NatsContainer().with_command("-js") as container:
        yield container


@pytest_asyncio.fixture
async def nats_client(nats_container):
    """
    Function-scoped async fixture that connects to the NATS container.

    A fresh connection is created for each test to ensure test isolation.
    """
    client = await nats.connect(nats_container.nats_uri())
    yield client
    await client.close()


class TestDistributedRateLimiterIntegration:
    """Integration tests for DistributedTokenBucketRateLimiter with real NATS."""

    @pytest.mark.asyncio
    async def test_basic_acquire(self, nats_client):
        """Test basic token acquisition with real NATS."""
        config = RateLimitConfig(
            requests_per_second=10.0,
            burst_capacity=5
        )

        # Use unique resource name to ensure test isolation
        resource_name = f"test_basic_acquire_{uuid.uuid4().hex[:8]}"
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name=resource_name,
            config=config,
            bucket_name="test-rate-limiters"
        )

        # First acquisition should succeed (bucket starts full)
        result = await limiter.acquire(1.0)
        assert result is True

        # Subsequent acquisitions should also succeed until bucket is empty
        for _ in range(4):
            result = await limiter.acquire(1.0)
            assert result is True

        # Bucket should now be empty (5 tokens consumed)
        result = await limiter.acquire(1.0)
        assert result is False

    @pytest.mark.asyncio
    async def test_multi_worker_simulation(self, nats_client):
        """Test multiple concurrent workers sharing rate limit."""
        config = RateLimitConfig(
            requests_per_second=10.0,
            burst_capacity=10
        )

        # Use unique resource name to ensure test isolation
        resource_name = f"test_multi_worker_{uuid.uuid4().hex[:8]}"
        # Create multiple "workers" sharing the same resource
        workers = [
            DistributedTokenBucketRateLimiter(
                nats_client=nats_client,
                resource_name=resource_name,
                config=config,
                bucket_name="test-rate-limiters"
            )
            for _ in range(3)
        ]

        # Track successful acquisitions
        success_count = 0

        async def worker_task(worker, acquisitions):
            nonlocal success_count
            for _ in range(acquisitions):
                if await worker.acquire(1.0):
                    success_count += 1

        # Run workers concurrently, each trying to acquire 5 tokens
        # Total attempts: 15, but bucket only has 10 tokens
        await asyncio.gather(
            worker_task(workers[0], 5),
            worker_task(workers[1], 5),
            worker_task(workers[2], 5)
        )

        # Exactly 10 acquisitions should succeed (burst_capacity)
        assert success_count == 10

    @pytest.mark.asyncio
    async def test_token_refill_over_time(self, nats_client):
        """Test tokens refill correctly over time."""
        config = RateLimitConfig(
            requests_per_second=10.0,  # 10 tokens per second
            burst_capacity=5
        )

        # Use unique resource name to ensure test isolation
        resource_name = f"test_token_refill_{uuid.uuid4().hex[:8]}"
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name=resource_name,
            config=config,
            bucket_name="test-rate-limiters"
        )

        # Drain the bucket completely
        for _ in range(5):
            await limiter.acquire(1.0)

        # Verify bucket is empty
        result = await limiter.acquire(1.0)
        assert result is False

        # Wait for tokens to refill (0.5 seconds at 10 req/s = 5 tokens)
        await asyncio.sleep(0.5)

        # Should have tokens available again
        result = await limiter.acquire(1.0)
        assert result is True

    @pytest.mark.asyncio
    async def test_wait_for_tokens(self, nats_client):
        """Test waiting for tokens to become available."""
        config = RateLimitConfig(
            requests_per_second=10.0,  # 10 tokens per second
            burst_capacity=2
        )

        # Use unique resource name to ensure test isolation
        resource_name = f"test_wait_for_tokens_{uuid.uuid4().hex[:8]}"
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name=resource_name,
            config=config,
            bucket_name="test-rate-limiters"
        )

        # Drain the bucket
        await limiter.acquire(2.0)

        # Verify immediate acquire fails
        result = await limiter.acquire(1.0)
        assert result is False

        # Wait for tokens should succeed within timeout
        start_time = time.monotonic()
        result = await limiter.wait_for_tokens(1.0, timeout=1.0)
        elapsed = time.monotonic() - start_time

        assert result is True
        # Should have waited approximately 0.1 seconds (1 token at 10 req/s)
        assert elapsed >= 0.05  # Allow some tolerance
        assert elapsed < 1.0  # Should not timeout

    @pytest.mark.asyncio
    async def test_wait_for_tokens_timeout(self, nats_client):
        """Test timeout when waiting for tokens."""
        config = RateLimitConfig(
            requests_per_second=1.0,  # Very slow: 1 token per second
            burst_capacity=1
        )

        # Use unique resource name to ensure test isolation
        resource_name = f"test_wait_timeout_{uuid.uuid4().hex[:8]}"
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name=resource_name,
            config=config,
            bucket_name="test-rate-limiters"
        )

        # Drain the bucket
        await limiter.acquire(1.0)

        # Wait should timeout since refill is too slow
        start_time = time.monotonic()
        result = await limiter.wait_for_tokens(1.0, timeout=0.2)
        elapsed = time.monotonic() - start_time

        assert result is False
        assert elapsed >= 0.2  # Should have waited until timeout

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
            bucket_name="test-client-bucket"
        )

        # Track function calls
        call_count = 0

        async def mock_api_call():
            nonlocal call_count
            call_count += 1
            return f"result_{call_count}"

        # Make several rate-limited calls
        results = []
        for _ in range(3):
            result = await client.call(mock_api_call)
            results.append(result)

        assert call_count == 3
        assert results == ["result_1", "result_2", "result_3"]

        # Verify status
        status = await client.get_status()
        assert status["max_tokens"] == 5
        assert status["requests_per_second"] == 10.0
        assert status["tokens_available"] <= 5  # Some tokens consumed

    @pytest.mark.asyncio
    async def test_distributed_client_with_retries(self, nats_client):
        """Test that DistributedRateLimitedClient retries on failures."""
        config = RateLimitConfig(
            requests_per_second=10.0,
            burst_capacity=10,
            max_retries=3,
            backoff_factor=0.01  # Fast backoff for testing
        )

        client = await create_distributed_igdb_rate_limiter(
            nats_client=nats_client,
            config=config,
            bucket_name="test-retry-bucket"
        )

        # Create a function that fails twice then succeeds
        call_count = 0

        async def flaky_api_call():
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise ConnectionError(f"Simulated failure {call_count}")
            return "success"

        result = await client.call(flaky_api_call)

        assert result == "success"
        assert call_count == 3  # Failed twice, succeeded on third

    @pytest.mark.asyncio
    async def test_status_reflects_current_state(self, nats_client):
        """Test that get_status accurately reflects the rate limiter state."""
        config = RateLimitConfig(
            requests_per_second=10.0,
            burst_capacity=10
        )

        # Use unique resource name to ensure test isolation
        resource_name = f"test_status_{uuid.uuid4().hex[:8]}"
        limiter = DistributedTokenBucketRateLimiter(
            nats_client=nats_client,
            resource_name=resource_name,
            config=config,
            bucket_name="test-rate-limiters"
        )

        # Get initial status (bucket should be full)
        status = await limiter.get_status()
        assert status["max_tokens"] == 10
        assert status["requests_per_second"] == 10.0
        # Initial tokens should be close to burst capacity
        assert status["tokens_available"] >= 9.0

        # Consume some tokens
        for _ in range(5):
            await limiter.acquire(1.0)

        # Status should reflect consumed tokens (allow small tolerance for refill)
        status = await limiter.get_status()
        assert status["tokens_available"] <= 5.5  # Allow small tolerance for token refill
        assert status["utilization"] >= 0.4  # At least 40% utilized
