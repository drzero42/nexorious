"""Tests for ConnectionMonitor exponential backoff and connection checking."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

import app.worker.connection_monitor as connection_monitor_module
from app.worker.connection_monitor import ConnectionMonitor, get_connection_monitor


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
        mock_client.close = AsyncMock()

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

    @pytest.mark.asyncio
    async def test_check_nats_connected_but_not_ready(self):
        """Test check_nats returns False when connected but not ready."""
        monitor = ConnectionMonitor()
        mock_client = MagicMock()
        mock_client.is_connected = False
        mock_client.close = AsyncMock()

        with patch(
            "app.worker.connection_monitor.nats.connect",
            new_callable=AsyncMock,
            return_value=mock_client,
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
            monitor.check_nats.assert_called_once()  # type: ignore[attr-defined]
            monitor.check_postgres.assert_called_once()  # type: ignore[attr-defined]

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


class TestGetConnectionMonitor:
    """Tests for get_connection_monitor factory function."""

    def test_get_connection_monitor_returns_singleton(self):
        """Test get_connection_monitor returns the same instance."""
        # Reset global state for test
        connection_monitor_module._monitor = None

        monitor1 = get_connection_monitor()
        monitor2 = get_connection_monitor()
        assert monitor1 is monitor2

        # Clean up
        connection_monitor_module._monitor = None
