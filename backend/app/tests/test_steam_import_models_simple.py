"""
Simple test for Steam import models creation and validation.
"""

import pytest
from datetime import datetime, timezone

from app.models.steam_import import (
    SteamImportJob,
    SteamImportGame,
    SteamImportJobStatus,
    SteamImportGameStatus
)


def test_steam_import_job_status_enum():
    """Test Steam import job status enum values."""
    assert SteamImportJobStatus.PENDING == "pending"
    assert SteamImportJobStatus.PROCESSING == "processing"
    assert SteamImportJobStatus.AWAITING_REVIEW == "awaiting_review"
    assert SteamImportJobStatus.FINALIZING == "finalizing"
    assert SteamImportJobStatus.COMPLETED == "completed"
    assert SteamImportJobStatus.FAILED == "failed"


def test_steam_import_game_status_enum():
    """Test Steam import game status enum values."""
    assert SteamImportGameStatus.MATCHED == "matched"
    assert SteamImportGameStatus.AWAITING_USER == "awaiting_user"
    assert SteamImportGameStatus.SKIPPED == "skipped"
    assert SteamImportGameStatus.IMPORTED == "imported"
    assert SteamImportGameStatus.PLATFORM_ADDED == "platform_added"
    assert SteamImportGameStatus.ALREADY_OWNED == "already_owned"
    assert SteamImportGameStatus.IMPORT_FAILED == "import_failed"


def test_steam_import_job_model_creation():
    """Test Steam import job model creation with default values."""
    # Test with minimal required fields
    job = SteamImportJob(
        user_id="test-user-id",
        steam_library_data='[{"appid": 123, "name": "Test Game"}]'
    )
    
    # Check default values
    assert job.status == SteamImportJobStatus.PENDING
    assert job.total_games == 0
    assert job.processed_games == 0
    assert job.matched_games == 0
    assert job.awaiting_review_games == 0
    assert job.skipped_games == 0
    assert job.imported_games == 0
    assert job.platform_added_games == 0
    assert job.error_message is None
    assert job.steam_library_data == '[{"appid": 123, "name": "Test Game"}]'
    assert job.completed_at is None


def test_steam_import_job_model_with_values():
    """Test Steam import job model creation with custom values."""
    now = datetime.now(timezone.utc)
    
    job = SteamImportJob(
        user_id="test-user-id",
        status=SteamImportJobStatus.COMPLETED,
        total_games=100,
        processed_games=95,
        matched_games=85,
        awaiting_review_games=5,
        skipped_games=5,
        imported_games=80,
        platform_added_games=5,
        error_message=None,
        steam_library_data='[{"appid": 123, "name": "Test Game"}]',
        completed_at=now
    )
    
    assert job.status == SteamImportJobStatus.COMPLETED
    assert job.total_games == 100
    assert job.processed_games == 95
    assert job.matched_games == 85
    assert job.awaiting_review_games == 5
    assert job.skipped_games == 5
    assert job.imported_games == 80
    assert job.platform_added_games == 5
    assert job.completed_at == now


def test_steam_import_game_model_creation():
    """Test Steam import game model creation with default values."""
    game = SteamImportGame(
        import_job_id="test-job-id",
        steam_appid=123456,
        steam_name="Test Steam Game"
    )
    
    # Check default values
    assert game.status == SteamImportGameStatus.AWAITING_USER
    assert game.matched_game_id is None
    assert game.user_decision is None
    assert game.error_message is None
    assert game.steam_appid == 123456
    assert game.steam_name == "Test Steam Game"


def test_steam_import_game_model_with_values():
    """Test Steam import game model creation with custom values."""
    game = SteamImportGame(
        import_job_id="test-job-id",
        steam_appid=789012,
        steam_name="Another Test Game",
        status=SteamImportGameStatus.MATCHED,
        matched_game_id="game-123",
        user_decision='{"action": "match", "confirmed": true}',
        error_message=None
    )
    
    assert game.status == SteamImportGameStatus.MATCHED
    assert game.matched_game_id == "game-123"
    assert game.user_decision == '{"action": "match", "confirmed": true}'
    assert game.steam_appid == 789012
    assert game.steam_name == "Another Test Game"


def test_steam_import_game_with_error():
    """Test Steam import game model with error status."""
    game = SteamImportGame(
        import_job_id="test-job-id",
        steam_appid=999999,
        steam_name="Failed Game",
        status=SteamImportGameStatus.IMPORT_FAILED,
        error_message="Failed to import due to missing metadata"
    )
    
    assert game.status == SteamImportGameStatus.IMPORT_FAILED
    assert game.error_message == "Failed to import due to missing metadata"


def test_steam_import_job_with_error():
    """Test Steam import job model with error status."""
    job = SteamImportJob(
        user_id="test-user-id",
        status=SteamImportJobStatus.FAILED,
        error_message="Steam API authentication failed",
        steam_library_data="[]"
    )
    
    assert job.status == SteamImportJobStatus.FAILED
    assert job.error_message == "Steam API authentication failed"