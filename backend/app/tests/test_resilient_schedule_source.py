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
