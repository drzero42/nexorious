"""Tests for priority queue configuration."""

import pytest

from app.worker.queues import (
    QUEUE_HIGH,
    QUEUE_LOW,
    QUEUE_DEFAULT,
    get_queue_for_user_initiated,
    get_queue_for_scheduled,
)


class TestQueueConstants:
    """Test queue constant definitions."""

    def test_queue_high_value(self):
        """QUEUE_HIGH should be 'high'."""
        assert QUEUE_HIGH == "high"

    def test_queue_low_value(self):
        """QUEUE_LOW should be 'low'."""
        assert QUEUE_LOW == "low"

    def test_queue_default_is_high(self):
        """Default queue should be high priority for user experience."""
        assert QUEUE_DEFAULT == QUEUE_HIGH

    def test_queues_are_distinct(self):
        """High and low queues must have different values."""
        assert QUEUE_HIGH != QUEUE_LOW


class TestQueueHelpers:
    """Test queue helper functions."""

    def test_get_queue_for_user_initiated_returns_high(self):
        """User-initiated tasks should use high priority queue."""
        assert get_queue_for_user_initiated() == QUEUE_HIGH

    def test_get_queue_for_scheduled_returns_low(self):
        """Scheduled tasks should use low priority queue."""
        assert get_queue_for_scheduled() == QUEUE_LOW

    def test_user_initiated_not_equal_to_scheduled(self):
        """User-initiated and scheduled queues should be different."""
        assert get_queue_for_user_initiated() != get_queue_for_scheduled()


class TestWorkerModuleExports:
    """Test that queues are properly exported from worker module."""

    def test_imports_from_worker_module(self):
        """Queue constants and helpers should be importable from app.worker."""
        from app.worker import (
            QUEUE_HIGH,
            QUEUE_LOW,
            QUEUE_DEFAULT,
            get_queue_for_user_initiated,
            get_queue_for_scheduled,
        )

        assert QUEUE_HIGH == "high"
        assert QUEUE_LOW == "low"
        assert QUEUE_DEFAULT == "high"
        assert callable(get_queue_for_user_initiated)
        assert callable(get_queue_for_scheduled)
