"""Tests for BackupConfig model."""

from app.models.backup_config import BackupConfig, BackupSchedule, RetentionMode


def test_backup_config_defaults():
    """Test BackupConfig has correct defaults."""
    config = BackupConfig()
    assert config.schedule == BackupSchedule.MANUAL
    assert config.schedule_time == "02:00"
    assert config.schedule_day is None
    assert config.retention_mode == RetentionMode.COUNT
    assert config.retention_value == 10


def test_backup_schedule_enum_values():
    """Test BackupSchedule enum has correct values."""
    assert BackupSchedule.MANUAL.value == "manual"
    assert BackupSchedule.DAILY.value == "daily"
    assert BackupSchedule.WEEKLY.value == "weekly"


def test_retention_mode_enum_values():
    """Test RetentionMode enum has correct values."""
    assert RetentionMode.DAYS.value == "days"
    assert RetentionMode.COUNT.value == "count"
