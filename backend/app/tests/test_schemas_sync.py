"""Tests for sync schemas."""

import pytest


def test_sync_platform_psn_exists():
    """Test PSN platform exists in SyncPlatform enum."""
    from app.schemas.sync import SyncPlatform

    assert hasattr(SyncPlatform, 'PSN')
    assert SyncPlatform.PSN.value == "psn"
