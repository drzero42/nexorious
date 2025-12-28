"""Tests for NATS subject-based routing configuration."""


from app.worker.queues import (
    QUEUE_HIGH,
    QUEUE_LOW,
    SUBJECT_HIGH,
    SUBJECT_LOW,
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

    def test_subject_high_value(self):
        """SUBJECT_HIGH should be 'tasks.high'."""
        assert SUBJECT_HIGH == "tasks.high"

    def test_subject_low_value(self):
        """SUBJECT_LOW should be 'tasks.low'."""
        assert SUBJECT_LOW == "tasks.low"

    def test_subjects_are_distinct(self):
        """High and low subjects must have different values."""
        assert SUBJECT_HIGH != SUBJECT_LOW

    def test_high_priority_aliases_point_to_subject_high(self):
        """All high priority subject aliases should point to SUBJECT_HIGH."""
        assert SUBJECT_HIGH_IMPORT == SUBJECT_HIGH
        assert SUBJECT_HIGH_SYNC == SUBJECT_HIGH
        assert SUBJECT_HIGH_EXPORT == SUBJECT_HIGH

    def test_low_priority_aliases_point_to_subject_low(self):
        """All low priority subject aliases should point to SUBJECT_LOW."""
        assert SUBJECT_LOW_IMPORT == SUBJECT_LOW
        assert SUBJECT_LOW_SYNC == SUBJECT_LOW
        assert SUBJECT_LOW_MAINTENANCE == SUBJECT_LOW


class TestWorkerModuleExports:
    """Test that subjects are properly exported from worker module."""

    def test_imports_from_worker_module(self):
        """Subject constants should be importable from app.worker."""
        from app.worker import (
            QUEUE_HIGH,
            QUEUE_LOW,
            SUBJECT_HIGH,
            SUBJECT_LOW,
            SUBJECT_HIGH_IMPORT,
            SUBJECT_HIGH_SYNC,
            SUBJECT_HIGH_EXPORT,
            SUBJECT_LOW_IMPORT,
            SUBJECT_LOW_SYNC,
            SUBJECT_LOW_MAINTENANCE,
        )

        assert QUEUE_HIGH == "high"
        assert QUEUE_LOW == "low"
        assert SUBJECT_HIGH == "tasks.high"
        assert SUBJECT_LOW == "tasks.low"
        # Aliases now point to simplified subjects
        assert SUBJECT_HIGH_IMPORT == SUBJECT_HIGH
        assert SUBJECT_LOW_MAINTENANCE == SUBJECT_LOW
