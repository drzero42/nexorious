"""
Tests for WebSocket integration in Steam import service.
"""

import pytest
import json
from datetime import datetime, timezone
from unittest.mock import Mock, AsyncMock, patch

from app.services.steam_import import SteamImportService
from app.services.websocket_manager import WebSocketEventType
from app.models.steam_import import SteamImportJob, SteamImportGame, SteamImportJobStatus, SteamImportGameStatus
from app.services.steam import SteamGame


class TestSteamImportServiceWebSocketIntegration:
    """Test WebSocket integration in Steam import service."""
    
    def setup_method(self):
        """Set up test fixtures."""
        self.mock_session = Mock()
        self.mock_igdb_service = Mock()
        self.mock_ws_manager = AsyncMock()
        
        # Create service instance
        self.service = SteamImportService(self.mock_session, self.mock_igdb_service)
        self.service.ws_manager = self.mock_ws_manager
    
    @pytest.mark.asyncio
    async def test_emit_status_change(self):
        """Test status change event emission."""
        job = SteamImportJob(
            id="job123",
            user_id="user456",
            status=SteamImportJobStatus.PROCESSING,
            total_games=100,
            processed_games=50,
            matched_games=30,
            awaiting_review_games=15,
            skipped_games=5,
            imported_games=0,
            platform_added_games=0
        )
        
        await self.service._emit_status_change(job)
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.IMPORT_STATUS_CHANGE
        assert call_args[1]["data"]["status"] == "processing"
        assert call_args[1]["data"]["total_games"] == 100
        assert call_args[1]["data"]["processed_games"] == 50
        assert call_args[1]["data"]["matched_games"] == 30
    
    @pytest.mark.asyncio
    async def test_emit_progress_update(self):
        """Test progress update event emission."""
        job = SteamImportJob(
            id="job123",
            user_id="user456",
            status=SteamImportJobStatus.PROCESSING,
            total_games=200,
            processed_games=150,
            matched_games=120,
            awaiting_review_games=25,
            skipped_games=5,
            imported_games=0,
            platform_added_games=0
        )
        
        await self.service._emit_progress_update(job)
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.IMPORT_PROGRESS
        assert call_args[1]["data"]["total_games"] == 200
        assert call_args[1]["data"]["processed_games"] == 150
        assert call_args[1]["data"]["progress_percentage"] == 75.0  # 150/200 * 100
    
    @pytest.mark.asyncio
    async def test_emit_game_matched(self):
        """Test game matched event emission."""
        job = SteamImportJob(id="job123", user_id="user456")
        steam_game = SteamGame(
            appid=123456,
            name="Test Game",
            img_icon_url="test_icon.jpg"
        )
        
        await self.service._emit_game_matched(job, steam_game, "game789", "database")
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.GAME_MATCHED
        assert call_args[1]["data"]["steam_appid"] == 123456
        assert call_args[1]["data"]["steam_name"] == "Test Game"
        assert call_args[1]["data"]["matched_game_id"] == "game789"
        assert call_args[1]["data"]["match_type"] == "database"
        assert call_args[1]["data"]["img_icon_url"] == "test_icon.jpg"
    
    @pytest.mark.asyncio
    async def test_emit_game_needs_review(self):
        """Test game needs review event emission."""
        job = SteamImportJob(id="job123", user_id="user456")
        steam_game = SteamGame(
            appid=789123,
            name="Unmatched Game",
            img_icon_url="unmatched_icon.jpg"
        )
        
        await self.service._emit_game_needs_review(job, steam_game)
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.GAME_NEEDS_REVIEW
        assert call_args[1]["data"]["steam_appid"] == 789123
        assert call_args[1]["data"]["steam_name"] == "Unmatched Game"
        assert call_args[1]["data"]["img_icon_url"] == "unmatched_icon.jpg"
    
    @pytest.mark.asyncio
    async def test_emit_game_imported(self):
        """Test game imported event emission."""
        job = SteamImportJob(id="job123", user_id="user456")
        import_game = SteamImportGame(
            id="import123",
            import_job_id="job123",
            steam_appid=456789,
            steam_name="New Game",
            status=SteamImportGameStatus.IMPORTED,
            matched_game_id="game456"
        )
        
        await self.service._emit_game_imported(job, import_game)
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.GAME_IMPORTED
        assert call_args[1]["data"]["steam_appid"] == 456789
        assert call_args[1]["data"]["steam_name"] == "New Game"
        assert call_args[1]["data"]["matched_game_id"] == "game456"
    
    @pytest.mark.asyncio
    async def test_emit_platform_added(self):
        """Test platform added event emission."""
        job = SteamImportJob(id="job123", user_id="user456")
        import_game = SteamImportGame(
            id="import123",
            import_job_id="job123",
            steam_appid=987654,
            steam_name="Existing Game",
            status=SteamImportGameStatus.PLATFORM_ADDED,
            matched_game_id="game789"
        )
        
        await self.service._emit_platform_added(job, import_game)
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.PLATFORM_ADDED
        assert call_args[1]["data"]["steam_appid"] == 987654
        assert call_args[1]["data"]["steam_name"] == "Existing Game"
        assert call_args[1]["data"]["matched_game_id"] == "game789"
    
    @pytest.mark.asyncio
    async def test_emit_game_skipped(self):
        """Test game skipped event emission."""
        job = SteamImportJob(id="job123", user_id="user456")
        import_game = SteamImportGame(
            id="import123",
            import_job_id="job123",
            steam_appid=111222,
            steam_name="Skipped Game",
            status=SteamImportGameStatus.SKIPPED
        )
        
        await self.service._emit_game_skipped(job, import_game)
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.GAME_SKIPPED
        assert call_args[1]["data"]["steam_appid"] == 111222
        assert call_args[1]["data"]["steam_name"] == "Skipped Game"
    
    @pytest.mark.asyncio
    async def test_emit_import_complete(self):
        """Test import complete event emission."""
        completed_at = datetime.now(timezone.utc)
        job = SteamImportJob(
            id="job123",
            user_id="user456",
            status=SteamImportJobStatus.COMPLETED,
            total_games=100,
            processed_games=100,
            matched_games=80,
            skipped_games=10,
            imported_games=60,
            platform_added_games=20,
            completed_at=completed_at
        )
        
        await self.service._emit_import_complete(job)
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.IMPORT_COMPLETE
        assert call_args[1]["data"]["total_games"] == 100
        assert call_args[1]["data"]["processed_games"] == 100
        assert call_args[1]["data"]["matched_games"] == 80
        assert call_args[1]["data"]["skipped_games"] == 10
        assert call_args[1]["data"]["imported_games"] == 60
        assert call_args[1]["data"]["platform_added_games"] == 20
        assert call_args[1]["data"]["completed_at"] == completed_at.isoformat()
    
    @pytest.mark.asyncio
    async def test_emit_import_error(self):
        """Test import error event emission."""
        job = SteamImportJob(
            id="job123",
            user_id="user456",
            status=SteamImportJobStatus.FAILED,
            total_games=50,
            processed_games=25,
            error_message="Steam API error"
        )
        
        await self.service._emit_import_error(job, "Steam API error")
        
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.IMPORT_ERROR
        assert call_args[1]["data"]["error_message"] == "Steam API error"
        assert call_args[1]["data"]["status"] == "failed"
        assert call_args[1]["data"]["total_games"] == 50
        assert call_args[1]["data"]["processed_games"] == 25
    
    @pytest.mark.asyncio
    async def test_websocket_event_emission_error_handling(self):
        """Test error handling in WebSocket event emission."""
        job = SteamImportJob(id="job123", user_id="user456")
        
        # Mock WebSocket manager to raise an exception
        self.mock_ws_manager.send_to_job.side_effect = Exception("WebSocket error")
        
        # Should not raise exception - just log error
        await self.service._emit_status_change(job)
        
        # Verify the WebSocket manager was called despite the error
        self.mock_ws_manager.send_to_job.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_start_import_job_with_websocket_events(self):
        """Test that start_import_job emits appropriate WebSocket events."""
        job_id = "job123"
        steam_api_key = "test_key"
        steam_id = "test_steam_id"
        
        # Mock job
        mock_job = Mock(spec=SteamImportJob)
        mock_job.id = job_id
        mock_job.status = SteamImportJobStatus.PENDING
        mock_job.total_games = 0
        mock_job.processed_games = 0
        mock_job.matched_games = 0
        mock_job.awaiting_review_games = 0
        
        self.mock_session.get.return_value = mock_job
        
        # Mock Steam service and games
        mock_steam_service = AsyncMock()
        mock_steam_games = [
            SteamGame(appid=123, name="Game 1", img_icon_url="icon1.jpg"),
            SteamGame(appid=456, name="Game 2", img_icon_url="icon2.jpg")
        ]
        
        with patch('app.services.steam_import.SteamService') as mock_steam_service_class:
            mock_steam_service_class.return_value = mock_steam_service
            mock_steam_service.get_owned_games.return_value = mock_steam_games
            
            # Mock the service methods that would be called
            self.service._save_job_changes = AsyncMock()
            self.service._process_steam_games = AsyncMock()
            self.service._determine_next_job_status = AsyncMock()
            
            await self.service.start_import_job(job_id, steam_api_key, steam_id)
        
        # Verify WebSocket events were emitted
        # Should emit status change to PROCESSING and progress updates
        assert self.mock_ws_manager.send_to_job.call_count >= 2
        
        # Verify the first call was for status change to PROCESSING
        first_call = self.mock_ws_manager.send_to_job.call_args_list[0]
        assert first_call[1]["event_type"] == WebSocketEventType.IMPORT_STATUS_CHANGE
        
        # Verify a progress update was sent
        progress_calls = [
            call for call in self.mock_ws_manager.send_to_job.call_args_list
            if call[1]["event_type"] == WebSocketEventType.IMPORT_PROGRESS
        ]
        assert len(progress_calls) >= 1
    
    @pytest.mark.asyncio
    async def test_fail_job_emits_error_event(self):
        """Test that _fail_job emits error event."""
        job = SteamImportJob(
            id="job123",
            user_id="user456",
            status=SteamImportJobStatus.PROCESSING
        )
        
        self.service._save_job_changes = AsyncMock()
        
        await self.service._fail_job(job, "Test error message")
        
        # Verify error event was emitted
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        
        assert call_args[1]["job_id"] == "job123"
        assert call_args[1]["event_type"] == WebSocketEventType.IMPORT_ERROR
        assert call_args[1]["data"]["error_message"] == "Test error message"
        assert call_args[1]["data"]["status"] == "failed"
    
    @pytest.mark.asyncio 
    async def test_status_transitions_emit_events(self):
        """Test that status transitions emit appropriate events."""
        job = SteamImportJob(
            id="job123",
            user_id="user456",
            awaiting_review_games=5,
            matched_games=10
        )
        
        self.service._save_job_changes = AsyncMock()
        
        # Test transition to AWAITING_REVIEW
        await self.service._determine_next_job_status(job)
        
        # Should emit status change event
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        assert call_args[1]["event_type"] == WebSocketEventType.IMPORT_STATUS_CHANGE
        assert job.status == SteamImportJobStatus.AWAITING_REVIEW
        
        # Reset mock
        self.mock_ws_manager.reset_mock()
        
        # Test transition to FINALIZING
        job.awaiting_review_games = 0
        await self.service._determine_next_job_status(job)
        
        # Should emit status change event
        self.mock_ws_manager.send_to_job.assert_called_once()
        call_args = self.mock_ws_manager.send_to_job.call_args
        assert call_args[1]["event_type"] == WebSocketEventType.IMPORT_STATUS_CHANGE
        assert job.status == SteamImportJobStatus.FINALIZING