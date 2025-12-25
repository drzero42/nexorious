"""Tests for NATS subject-based routing configuration."""


from app.worker.queues import (
    QUEUE_HIGH,
    QUEUE_LOW,
    SUBJECT_HIGH_IMPORT,
    SUBJECT_HIGH_SYNC,
    SUBJECT_HIGH_EXPORT,
    SUBJECT_LOW_IMPORT,
    SUBJECT_LOW_SYNC,
    SUBJECT_LOW_MAINTENANCE,
)


class TestQueueConstants:
    """Test queue constant definitions."""

    def test_queue_high_value(self):
        """QUEUE_HIGH should be 'high'."""
        assert QUEUE_HIGH == "high"

    def test_queue_low_value(self):
        """QUEUE_LOW should be 'low'."""
        assert QUEUE_LOW == "low"

    def test_queues_are_distinct(self):
        """High and low queues must have different values."""
        assert QUEUE_HIGH != QUEUE_LOW


class TestSubjectConstants:
    """Test NATS subject routing constants."""

    def test_high_priority_subjects_start_with_tasks_high(self):
        """All high priority subjects should start with 'tasks.high.'."""
        assert SUBJECT_HIGH_IMPORT.startswith("tasks.high.")
        assert SUBJECT_HIGH_SYNC.startswith("tasks.high.")
        assert SUBJECT_HIGH_EXPORT.startswith("tasks.high.")

    def test_low_priority_subjects_start_with_tasks_low(self):
        """All low priority subjects should start with 'tasks.low.'."""
        assert SUBJECT_LOW_IMPORT.startswith("tasks.low.")
        assert SUBJECT_LOW_SYNC.startswith("tasks.low.")
        assert SUBJECT_LOW_MAINTENANCE.startswith("tasks.low.")

    def test_subject_values(self):
        """Subject constants should have expected values."""
        assert SUBJECT_HIGH_IMPORT == "tasks.high.import"
        assert SUBJECT_HIGH_SYNC == "tasks.high.sync"
        assert SUBJECT_HIGH_EXPORT == "tasks.high.export"
        assert SUBJECT_LOW_IMPORT == "tasks.low.import"
        assert SUBJECT_LOW_SYNC == "tasks.low.sync"
        assert SUBJECT_LOW_MAINTENANCE == "tasks.low.maintenance"


class TestWorkerModuleExports:
    """Test that subjects are properly exported from worker module."""

    def test_imports_from_worker_module(self):
        """Subject constants should be importable from app.worker."""
        from app.worker import (
            QUEUE_HIGH,
            QUEUE_LOW,
            SUBJECT_HIGH_IMPORT,
            SUBJECT_HIGH_SYNC,
            SUBJECT_HIGH_EXPORT,
            SUBJECT_LOW_IMPORT,
            SUBJECT_LOW_SYNC,
            SUBJECT_LOW_MAINTENANCE,
        )

        assert QUEUE_HIGH == "high"
        assert QUEUE_LOW == "low"
        assert SUBJECT_HIGH_IMPORT == "tasks.high.import"
        assert SUBJECT_LOW_MAINTENANCE == "tasks.low.maintenance"
