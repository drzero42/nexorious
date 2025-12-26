# Scheduler Connection Resilience Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the scheduler pause during NATS/PostgreSQL outages and automatically resume when connections recover.

**Architecture:** Create a `ConnectionMonitor` class that checks NATS and PostgreSQL connectivity with exponential backoff. Wrap the scheduler's schedule source to call the monitor before dispatching tasks. On connection failure, the monitor blocks until both services are available.

**Tech Stack:** Python 3.13, taskiq, taskiq-nats, nats-py, SQLModel/SQLAlchemy, pytest

---

### Task 1: Add Configuration Settings

**Files:**
- Modify: `backend/app/core/config.py:107-108`

**Step 1: Add scheduler reconnection settings**

Add after line 95 (after `rate_limiter_cas_retry_max_ms`):

```python
    # Scheduler Connection Resilience
    scheduler_reconnect_initial_delay: float = Field(
        default=5.0,
        description="Initial delay in seconds before reconnection attempt"
    )
    scheduler_reconnect_max_delay: float = Field(
        default=60.0,
        description="Maximum delay in seconds between reconnection attempts"
    )
    scheduler_reconnect_backoff_multiplier: float = Field(
        default=2.0,
        description="Multiplier for exponential backoff between reconnection attempts"
    )
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/app/core/config.py
git commit -m "feat(scheduler): add connection resilience configuration settings"
```

---

### Task 2: Create ConnectionMonitor Class with Tests

**Files:**
- Create: `backend/app/worker/connection_monitor.py`
- Create: `backend/app/tests/test_connection_monitor.py`

**Step 1: Write the failing tests**

Create `backend/app/tests/test_connection_monitor.py`:

```python
"""Tests for ConnectionMonitor exponential backoff and connection checking."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.worker.connection_monitor import ConnectionMonitor


class TestConnectionMonitor:
    """Tests for ConnectionMonitor class."""

    def test_init_default_values(self):
        """Test default initialization values."""
        monitor = ConnectionMonitor()
        assert monitor.initial_delay == 5.0
        assert monitor.max_delay == 60.0
        assert monitor.backoff_multiplier == 2.0
        assert monitor._current_delay == 5.0

    def test_init_custom_values(self):
        """Test custom initialization values."""
        monitor = ConnectionMonitor(
            initial_delay=1.0,
            max_delay=30.0,
            backoff_multiplier=3.0,
        )
        assert monitor.initial_delay == 1.0
        assert monitor.max_delay == 30.0
        assert monitor.backoff_multiplier == 3.0

    def test_reset_backoff(self):
        """Test reset_backoff resets delay to initial value."""
        monitor = ConnectionMonitor(initial_delay=5.0)
        monitor._current_delay = 60.0
        monitor.reset_backoff()
        assert monitor._current_delay == 5.0

    def test_get_next_delay_exponential_increase(self):
        """Test exponential backoff increases delay."""
        monitor = ConnectionMonitor(
            initial_delay=5.0,
            max_delay=60.0,
            backoff_multiplier=2.0,
        )
        # First call returns current delay and increases it
        delay1 = monitor._get_next_delay()
        assert delay1 == 5.0
        assert monitor._current_delay == 10.0

        delay2 = monitor._get_next_delay()
        assert delay2 == 10.0
        assert monitor._current_delay == 20.0

        delay3 = monitor._get_next_delay()
        assert delay3 == 20.0
        assert monitor._current_delay == 40.0

    def test_get_next_delay_caps_at_max(self):
        """Test delay is capped at max_delay."""
        monitor = ConnectionMonitor(
            initial_delay=30.0,
            max_delay=60.0,
            backoff_multiplier=2.0,
        )
        delay1 = monitor._get_next_delay()
        assert delay1 == 30.0
        assert monitor._current_delay == 60.0

        # Should cap at max
        delay2 = monitor._get_next_delay()
        assert delay2 == 60.0
        assert monitor._current_delay == 60.0


class TestCheckNats:
    """Tests for NATS connection checking."""

    @pytest.mark.asyncio
    async def test_check_nats_success(self):
        """Test check_nats returns True when connected."""
        monitor = ConnectionMonitor()
        mock_client = MagicMock()
        mock_client.is_connected = True

        with patch(
            "app.worker.connection_monitor.nats.connect",
            new_callable=AsyncMock,
            return_value=mock_client,
        ):
            result = await monitor.check_nats()
            assert result is True

    @pytest.mark.asyncio
    async def test_check_nats_failure(self):
        """Test check_nats returns False when connection fails."""
        monitor = ConnectionMonitor()

        with patch(
            "app.worker.connection_monitor.nats.connect",
            new_callable=AsyncMock,
            side_effect=Exception("Connection refused"),
        ):
            result = await monitor.check_nats()
            assert result is False


class TestCheckPostgres:
    """Tests for PostgreSQL connection checking."""

    @pytest.mark.asyncio
    async def test_check_postgres_success(self):
        """Test check_postgres returns True when connected."""
        monitor = ConnectionMonitor()
        mock_engine = MagicMock()
        mock_connection = MagicMock()
        mock_connection.__enter__ = MagicMock(return_value=mock_connection)
        mock_connection.__exit__ = MagicMock(return_value=None)
        mock_engine.connect.return_value = mock_connection

        with patch(
            "app.worker.connection_monitor.get_engine",
            return_value=mock_engine,
        ):
            result = await monitor.check_postgres()
            assert result is True
            mock_connection.execute.assert_called_once()

    @pytest.mark.asyncio
    async def test_check_postgres_failure(self):
        """Test check_postgres returns False when connection fails."""
        monitor = ConnectionMonitor()

        with patch(
            "app.worker.connection_monitor.get_engine",
            side_effect=Exception("Connection refused"),
        ):
            result = await monitor.check_postgres()
            assert result is False


class TestWaitForConnections:
    """Tests for wait_for_connections blocking behavior."""

    @pytest.mark.asyncio
    async def test_wait_for_connections_immediate_success(self):
        """Test wait_for_connections returns immediately when both connected."""
        monitor = ConnectionMonitor()

        with patch.object(
            monitor, "check_nats", new_callable=AsyncMock, return_value=True
        ), patch.object(
            monitor, "check_postgres", new_callable=AsyncMock, return_value=True
        ):
            await monitor.wait_for_connections()
            # Should complete without blocking
            monitor.check_nats.assert_called_once()
            monitor.check_postgres.assert_called_once()

    @pytest.mark.asyncio
    async def test_wait_for_connections_retries_on_nats_failure(self):
        """Test wait_for_connections retries when NATS fails."""
        monitor = ConnectionMonitor(initial_delay=0.01)  # Fast for testing
        call_count = 0

        async def nats_side_effect():
            nonlocal call_count
            call_count += 1
            return call_count >= 2  # Fail first, succeed second

        with patch.object(
            monitor, "check_nats", new_callable=AsyncMock, side_effect=nats_side_effect
        ), patch.object(
            monitor, "check_postgres", new_callable=AsyncMock, return_value=True
        ), patch("app.worker.connection_monitor.asyncio.sleep", new_callable=AsyncMock):
            await monitor.wait_for_connections()
            assert call_count == 2

    @pytest.mark.asyncio
    async def test_wait_for_connections_retries_on_postgres_failure(self):
        """Test wait_for_connections retries when PostgreSQL fails."""
        monitor = ConnectionMonitor(initial_delay=0.01)
        call_count = 0

        async def postgres_side_effect():
            nonlocal call_count
            call_count += 1
            return call_count >= 2

        with patch.object(
            monitor, "check_nats", new_callable=AsyncMock, return_value=True
        ), patch.object(
            monitor,
            "check_postgres",
            new_callable=AsyncMock,
            side_effect=postgres_side_effect,
        ), patch("app.worker.connection_monitor.asyncio.sleep", new_callable=AsyncMock):
            await monitor.wait_for_connections()
            assert call_count == 2

    @pytest.mark.asyncio
    async def test_wait_for_connections_resets_backoff_on_success(self):
        """Test backoff is reset after successful connection."""
        monitor = ConnectionMonitor(initial_delay=0.01)
        monitor._current_delay = 60.0  # Simulate previous backoff

        with patch.object(
            monitor, "check_nats", new_callable=AsyncMock, return_value=True
        ), patch.object(
            monitor, "check_postgres", new_callable=AsyncMock, return_value=True
        ):
            await monitor.wait_for_connections()
            assert monitor._current_delay == 0.01  # Reset to initial
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_connection_monitor.py -v`
Expected: FAIL with "ModuleNotFoundError: No module named 'app.worker.connection_monitor'"

**Step 3: Write the implementation**

Create `backend/app/worker/connection_monitor.py`:

```python
"""Connection monitor for scheduler resilience.

Monitors NATS and PostgreSQL connectivity with exponential backoff.
Used by the scheduler to pause during outages and resume on recovery.
"""

import asyncio
import logging

import nats
from sqlalchemy import text

from app.core.config import settings
from app.core.database import get_engine

logger = logging.getLogger(__name__)


class ConnectionMonitor:
    """Monitors NATS and PostgreSQL connections with exponential backoff reconnection.

    When connections are lost, wait_for_connections() blocks until both services
    are available again, using exponential backoff between retry attempts.
    """

    def __init__(
        self,
        initial_delay: float | None = None,
        max_delay: float | None = None,
        backoff_multiplier: float | None = None,
    ):
        """Initialize the connection monitor.

        Args:
            initial_delay: Initial delay in seconds before first retry (default from settings)
            max_delay: Maximum delay in seconds between retries (default from settings)
            backoff_multiplier: Multiplier for exponential backoff (default from settings)
        """
        self.initial_delay = (
            initial_delay
            if initial_delay is not None
            else settings.scheduler_reconnect_initial_delay
        )
        self.max_delay = (
            max_delay
            if max_delay is not None
            else settings.scheduler_reconnect_max_delay
        )
        self.backoff_multiplier = (
            backoff_multiplier
            if backoff_multiplier is not None
            else settings.scheduler_reconnect_backoff_multiplier
        )
        self._current_delay = self.initial_delay

    def reset_backoff(self) -> None:
        """Reset backoff delay to initial value after successful connection."""
        self._current_delay = self.initial_delay

    def _get_next_delay(self) -> float:
        """Get the next delay and increase for exponential backoff.

        Returns:
            The current delay value before increasing.
        """
        delay = self._current_delay
        self._current_delay = min(
            self._current_delay * self.backoff_multiplier,
            self.max_delay,
        )
        return delay

    async def check_nats(self) -> bool:
        """Check if NATS is reachable.

        Returns:
            True if NATS connection succeeds, False otherwise.
        """
        try:
            client = await nats.connect(settings.NATS_URL)
            is_connected = client.is_connected
            await client.close()
            return is_connected
        except Exception:
            return False

    async def check_postgres(self) -> bool:
        """Check if PostgreSQL is reachable.

        Returns:
            True if PostgreSQL connection succeeds, False otherwise.
        """
        try:
            engine = get_engine()
            with engine.connect() as conn:
                conn.execute(text("SELECT 1"))
            return True
        except Exception:
            return False

    async def wait_for_connections(self) -> None:
        """Block until both NATS and PostgreSQL are available.

        Uses exponential backoff between retry attempts. Logs warnings
        during reconnection and info on successful recovery.
        """
        while True:
            nats_ok = await self.check_nats()
            postgres_ok = await self.check_postgres()

            if nats_ok and postgres_ok:
                self.reset_backoff()
                return

            # Build status message
            failed_services = []
            if not nats_ok:
                failed_services.append("NATS")
            if not postgres_ok:
                failed_services.append("PostgreSQL")

            delay = self._get_next_delay()
            logger.warning(
                f"Connection check failed for {', '.join(failed_services)}. "
                f"Retrying in {delay:.1f}s..."
            )
            await asyncio.sleep(delay)


# Global monitor instance for scheduler use
_monitor: ConnectionMonitor | None = None


def get_connection_monitor() -> ConnectionMonitor:
    """Get or create the global connection monitor instance."""
    global _monitor
    if _monitor is None:
        _monitor = ConnectionMonitor()
    return _monitor
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_connection_monitor.py -v`
Expected: All tests PASS

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No errors

**Step 6: Commit**

```bash
git add backend/app/worker/connection_monitor.py backend/app/tests/test_connection_monitor.py
git commit -m "feat(scheduler): add ConnectionMonitor with exponential backoff"
```

---

### Task 3: Create Resilient Schedule Source

**Files:**
- Modify: `backend/app/worker/schedules.py`
- Create: `backend/app/tests/test_resilient_schedule_source.py`

**Step 1: Write the failing tests**

Create `backend/app/tests/test_resilient_schedule_source.py`:

```python
"""Tests for ResilientScheduleSource."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch


class TestResilientScheduleSource:
    """Tests for ResilientScheduleSource."""

    @pytest.mark.asyncio
    async def test_get_schedules_waits_for_connections(self):
        """Test get_schedules calls wait_for_connections before returning schedules."""
        from app.worker.schedules import ResilientScheduleSource

        mock_broker = MagicMock()
        mock_monitor = MagicMock()
        mock_monitor.wait_for_connections = AsyncMock()

        source = ResilientScheduleSource(mock_broker, mock_monitor)

        # Mock the parent class method
        with patch(
            "taskiq.schedule_sources.LabelScheduleSource.get_schedules",
            new_callable=AsyncMock,
            return_value=[{"task": "test"}],
        ):
            schedules = await source.get_schedules()

            mock_monitor.wait_for_connections.assert_called_once()
            assert schedules == [{"task": "test"}]

    @pytest.mark.asyncio
    async def test_get_schedules_blocks_until_connections_ready(self):
        """Test get_schedules blocks when connections are unavailable."""
        from app.worker.schedules import ResilientScheduleSource

        mock_broker = MagicMock()
        mock_monitor = MagicMock()

        # Simulate blocking wait
        call_order = []

        async def wait_side_effect():
            call_order.append("wait")

        mock_monitor.wait_for_connections = AsyncMock(side_effect=wait_side_effect)

        source = ResilientScheduleSource(mock_broker, mock_monitor)

        with patch(
            "taskiq.schedule_sources.LabelScheduleSource.get_schedules",
            new_callable=AsyncMock,
            return_value=[],
        ) as mock_get:

            async def get_side_effect():
                call_order.append("get")
                return []

            mock_get.side_effect = get_side_effect

            await source.get_schedules()

            # wait_for_connections should be called before get_schedules
            assert call_order == ["wait", "get"]
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_resilient_schedule_source.py -v`
Expected: FAIL with "cannot import name 'ResilientScheduleSource' from 'app.worker.schedules'"

**Step 3: Update schedules.py with ResilientScheduleSource**

Replace contents of `backend/app/worker/schedules.py`:

```python
"""Taskiq scheduler configuration for scheduled background tasks.

Uses ResilientScheduleSource to define cron-based schedules with connection
resilience - the scheduler pauses during NATS/PostgreSQL outages and
automatically resumes when connections recover.

The scheduler runs as a separate process and triggers tasks at specified intervals.

Usage:
    taskiq scheduler app.worker.schedules:scheduler app.worker.tasks
"""

from taskiq import TaskiqScheduler
from taskiq.schedule_sources import LabelScheduleSource

from app.worker.broker import broker
from app.worker.connection_monitor import ConnectionMonitor, get_connection_monitor


class ResilientScheduleSource(LabelScheduleSource):
    """Schedule source that checks connections before dispatching tasks.

    Wraps LabelScheduleSource to add connection resilience. Before returning
    schedules, it waits for both NATS and PostgreSQL to be available.
    """

    def __init__(self, broker, monitor: ConnectionMonitor | None = None):
        """Initialize the resilient schedule source.

        Args:
            broker: The taskiq broker instance.
            monitor: Optional ConnectionMonitor instance (uses global if not provided).
        """
        super().__init__(broker)
        self._monitor = monitor if monitor is not None else get_connection_monitor()

    async def get_schedules(self):
        """Get schedules after ensuring connections are available.

        Blocks until both NATS and PostgreSQL connections are healthy,
        then returns the schedules from the parent class.
        """
        await self._monitor.wait_for_connections()
        return await super().get_schedules()


# Initialize scheduler with ResilientScheduleSource for connection resilience
scheduler = TaskiqScheduler(
    broker=broker,
    sources=[ResilientScheduleSource(broker)],
)

# Schedule definitions are applied via task decorators using the 'schedule' label.
# See individual task modules for schedule configurations:
#
# Maintenance tasks (app/worker/tasks/maintenance/):
#   - cleanup_task_results: Daily at 3:00 AM UTC
#   - cleanup_expired_sessions: Every 30 minutes
#
# Sync tasks (app/worker/tasks/sync/):
#   - check_pending_syncs: Every 15 minutes
#
# Example task with schedule:
#   @broker.task(
#       schedule=[{"cron": "0 3 * * *"}],  # Daily at 3 AM
#       queue=QUEUE_LOW,
#   )
#   async def cleanup_task_results():
#       ...
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_resilient_schedule_source.py -v`
Expected: All tests PASS

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No errors

**Step 6: Commit**

```bash
git add backend/app/worker/schedules.py backend/app/tests/test_resilient_schedule_source.py
git commit -m "feat(scheduler): add ResilientScheduleSource with connection checks"
```

---

### Task 4: Run Full Test Suite

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests PASS with >80% coverage

**Step 2: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit any fixes if needed**

---

### Task 5: Manual Integration Test

**Step 1: Start the stack**

Run: `podman-compose up --build -d`

**Step 2: Verify scheduler starts normally**

Run: `podman-compose logs scheduler`
Expected: "Starting scheduler" and "Startup completed" messages

**Step 3: Stop NATS to simulate outage**

Run: `podman-compose stop nats`

**Step 4: Check scheduler logs for pause behavior**

Run: `podman-compose logs --tail=20 scheduler`
Expected: Warning messages like "Connection check failed for NATS. Retrying in 5.0s..."

**Step 5: Restart NATS**

Run: `podman-compose start nats`

**Step 6: Verify scheduler resumes**

Run: `podman-compose logs --tail=20 scheduler`
Expected: Scheduler resumes normal operation, tasks being sent

**Step 7: Document results**

If all steps pass, the implementation is complete.

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Add configuration settings | `config.py` |
| 2 | Create ConnectionMonitor with tests | `connection_monitor.py`, `test_connection_monitor.py` |
| 3 | Create ResilientScheduleSource | `schedules.py`, `test_resilient_schedule_source.py` |
| 4 | Run full test suite | - |
| 5 | Manual integration test | - |
