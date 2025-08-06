"""
Tests for WebSocket connection manager service.
"""

import pytest
import json
from datetime import datetime, timezone, timedelta
from unittest.mock import Mock, AsyncMock, patch
from fastapi import WebSocket, status

from app.services.websocket_manager import (
    WebSocketConnectionManager,
    WebSocketConnection,
    WebSocketEventType,
    WebSocketMessage,
    get_websocket_manager
)
from app.models.steam_import import SteamImportJob, SteamImportJobStatus
from app.models.user import User, UserSession


class TestWebSocketConnection:
    """Test WebSocket connection functionality."""
    
    def test_websocket_connection_creation(self):
        """Test WebSocket connection object creation."""
        mock_websocket = Mock(spec=WebSocket)
        user_id = "user123"
        job_id = "job456"
        
        connection = WebSocketConnection(mock_websocket, user_id, job_id)
        
        assert connection.websocket == mock_websocket
        assert connection.user_id == user_id
        assert connection.job_id == job_id
        assert isinstance(connection.connected_at, datetime)
        assert isinstance(connection.last_heartbeat, datetime)
    
    @pytest.mark.asyncio
    async def test_send_message_success(self):
        """Test successful message sending."""
        mock_websocket = AsyncMock(spec=WebSocket)
        connection = WebSocketConnection(mock_websocket, "user123", "job456")
        
        message = WebSocketMessage(
            event_type=WebSocketEventType.IMPORT_PROGRESS,
            job_id="job456",
            timestamp=datetime.now(timezone.utc),
            data={"progress": 50}
        )
        
        result = await connection.send_message(message)
        
        assert result is True
        mock_websocket.send_text.assert_called_once()
        sent_data = mock_websocket.send_text.call_args[0][0]
        parsed_data = json.loads(sent_data)
        assert parsed_data["event_type"] == "import_progress"
        assert parsed_data["job_id"] == "job456"
        assert parsed_data["data"]["progress"] == 50
    
    @pytest.mark.asyncio
    async def test_send_message_failure(self):
        """Test message sending failure."""
        mock_websocket = AsyncMock(spec=WebSocket)
        mock_websocket.send_text.side_effect = Exception("Connection error")
        connection = WebSocketConnection(mock_websocket, "user123", "job456")
        
        message = WebSocketMessage(
            event_type=WebSocketEventType.IMPORT_ERROR,
            job_id="job456",
            timestamp=datetime.now(timezone.utc),
            data={"error": "test error"}
        )
        
        result = await connection.send_message(message)
        
        assert result is False
    
    @pytest.mark.asyncio
    async def test_send_heartbeat(self):
        """Test heartbeat message sending."""
        mock_websocket = AsyncMock(spec=WebSocket)
        connection = WebSocketConnection(mock_websocket, "user123", "job456")
        original_heartbeat = connection.last_heartbeat
        
        result = await connection.send_heartbeat()
        
        assert result is True
        assert connection.last_heartbeat > original_heartbeat
        mock_websocket.send_text.assert_called_once()
        sent_data = mock_websocket.send_text.call_args[0][0]
        parsed_data = json.loads(sent_data)
        assert parsed_data["event_type"] == "heartbeat"
        assert parsed_data["data"]["status"] == "ping"


class TestWebSocketConnectionManager:
    """Test WebSocket connection manager functionality."""
    
    def setup_method(self):
        """Set up test fixtures."""
        self.manager = WebSocketConnectionManager()
    
    @pytest.mark.asyncio
    async def test_authenticate_and_connect_success(self):
        """Test successful authentication and connection."""
        mock_websocket = AsyncMock(spec=WebSocket)
        mock_session = Mock()
        
        # Mock user and session data
        mock_user = Mock(spec=User)
        mock_user.id = "user123"
        mock_user.username = "testuser"
        mock_user.is_active = True
        
        mock_user_session = Mock(spec=UserSession)
        mock_user_session.user_id = "user123"
        
        mock_import_job = Mock(spec=SteamImportJob)
        mock_import_job.id = "job456"
        mock_import_job.user_id = "user123"
        
        # Mock database operations
        mock_session.exec.return_value.first.return_value = mock_user_session
        mock_session.get.side_effect = [mock_user, mock_import_job]
        
        with patch('app.services.websocket_manager.verify_token') as mock_verify_token, \
             patch('app.services.websocket_manager.hash_token') as mock_hash_token:
            
            mock_verify_token.return_value = {"sub": "user123"}
            mock_hash_token.return_value = "hashed_token"
            
            connection = await self.manager.authenticate_and_connect(
                websocket=mock_websocket,
                job_id="job456",
                token="valid_token",
                session=mock_session
            )
        
        assert connection is not None
        assert connection.user_id == "user123"
        assert connection.job_id == "job456"
        assert "job456" in self.manager.connections
        assert "user123" in self.manager.user_connections
        mock_websocket.accept.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_authenticate_and_connect_invalid_token(self):
        """Test authentication failure with invalid token."""
        mock_websocket = AsyncMock(spec=WebSocket)
        mock_session = Mock()
        
        with patch('app.services.websocket_manager.verify_token') as mock_verify_token:
            mock_verify_token.side_effect = Exception("Invalid token")
            
            connection = await self.manager.authenticate_and_connect(
                websocket=mock_websocket,
                job_id="job456",
                token="invalid_token",
                session=mock_session
            )
        
        assert connection is None
        mock_websocket.close.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_authenticate_and_connect_job_access_denied(self):
        """Test authentication failure when user doesn't own the job."""
        mock_websocket = AsyncMock(spec=WebSocket)
        mock_session = Mock()
        
        # Mock user and session data
        mock_user = Mock(spec=User)
        mock_user.id = "user123"
        mock_user.is_active = True
        
        mock_user_session = Mock(spec=UserSession)
        mock_user_session.user_id = "user123"
        
        mock_import_job = Mock(spec=SteamImportJob)
        mock_import_job.id = "job456"
        mock_import_job.user_id = "different_user"  # Different user owns the job
        
        # Mock database operations
        mock_session.exec.return_value.first.return_value = mock_user_session
        mock_session.get.side_effect = [mock_user, mock_import_job]
        
        with patch('app.services.websocket_manager.verify_token') as mock_verify_token, \
             patch('app.services.websocket_manager.hash_token') as mock_hash_token:
            
            mock_verify_token.return_value = {"sub": "user123"}
            mock_hash_token.return_value = "hashed_token"
            
            connection = await self.manager.authenticate_and_connect(
                websocket=mock_websocket,
                job_id="job456",
                token="valid_token",
                session=mock_session
            )
        
        assert connection is None
        mock_websocket.close.assert_called_once_with(
            code=status.WS_1008_POLICY_VIOLATION,
            reason="Access denied to import job"
        )
    
    @pytest.mark.asyncio
    async def test_disconnect(self):
        """Test connection disconnection and cleanup."""
        mock_websocket = AsyncMock(spec=WebSocket)
        connection = WebSocketConnection(mock_websocket, "user123", "job456")
        
        # Add connection to manager
        self.manager.connections["job456"] = [connection]
        self.manager.user_connections["user123"] = [connection]
        
        await self.manager.disconnect(connection)
        
        assert "job456" not in self.manager.connections
        assert "user123" not in self.manager.user_connections
    
    @pytest.mark.asyncio
    async def test_send_to_job(self):
        """Test sending message to all connections for a job."""
        # Create mock connections
        mock_websocket1 = AsyncMock(spec=WebSocket)
        mock_websocket2 = AsyncMock(spec=WebSocket)
        connection1 = WebSocketConnection(mock_websocket1, "user123", "job456")
        connection2 = WebSocketConnection(mock_websocket2, "user456", "job456")
        
        # Mock successful message sending
        connection1.send_message = AsyncMock(return_value=True)
        connection2.send_message = AsyncMock(return_value=True)
        
        # Add connections to manager
        self.manager.connections["job456"] = [connection1, connection2]
        
        await self.manager.send_to_job(
            job_id="job456",
            event_type=WebSocketEventType.IMPORT_PROGRESS,
            data={"progress": 75}
        )
        
        connection1.send_message.assert_called_once()
        connection2.send_message.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_send_to_job_with_failed_connection(self):
        """Test sending message with one failed connection."""
        # Create mock connections
        mock_websocket1 = AsyncMock(spec=WebSocket)
        mock_websocket2 = AsyncMock(spec=WebSocket)
        connection1 = WebSocketConnection(mock_websocket1, "user123", "job456")
        connection2 = WebSocketConnection(mock_websocket2, "user456", "job456")
        
        # Mock message sending - one success, one failure
        connection1.send_message = AsyncMock(return_value=True)
        connection2.send_message = AsyncMock(return_value=False)
        
        # Add connections to manager
        self.manager.connections["job456"] = [connection1, connection2]
        self.manager.user_connections["user123"] = [connection1]
        self.manager.user_connections["user456"] = [connection2]
        
        await self.manager.send_to_job(
            job_id="job456",
            event_type=WebSocketEventType.IMPORT_PROGRESS,
            data={"progress": 75}
        )
        
        # Failed connection should be removed
        assert len(self.manager.connections["job456"]) == 1
        assert connection1 in self.manager.connections["job456"]
        assert connection2 not in self.manager.connections["job456"]
        assert "user456" not in self.manager.user_connections
    
    @pytest.mark.asyncio
    async def test_send_to_user(self):
        """Test sending message to all connections for a user."""
        # Create mock connections for the same user
        mock_websocket1 = AsyncMock(spec=WebSocket)
        mock_websocket2 = AsyncMock(spec=WebSocket)
        connection1 = WebSocketConnection(mock_websocket1, "user123", "job456")
        connection2 = WebSocketConnection(mock_websocket2, "user123", "job789")
        
        # Mock successful message sending
        connection1.send_message = AsyncMock(return_value=True)
        connection2.send_message = AsyncMock(return_value=True)
        
        # Add connections to manager
        self.manager.user_connections["user123"] = [connection1, connection2]
        
        await self.manager.send_to_user(
            user_id="user123",
            event_type=WebSocketEventType.IMPORT_COMPLETE,
            job_id="job456",
            data={"status": "completed"}
        )
        
        connection1.send_message.assert_called_once()
        connection2.send_message.assert_called_once()
    
    def test_get_connection_stats(self):
        """Test connection statistics."""
        # Create mock connections
        mock_websocket1 = AsyncMock(spec=WebSocket)
        mock_websocket2 = AsyncMock(spec=WebSocket)
        mock_websocket3 = AsyncMock(spec=WebSocket)
        
        connection1 = WebSocketConnection(mock_websocket1, "user123", "job456")
        connection2 = WebSocketConnection(mock_websocket2, "user456", "job456")
        connection3 = WebSocketConnection(mock_websocket3, "user123", "job789")
        
        # Add connections to manager
        self.manager.connections["job456"] = [connection1, connection2]
        self.manager.connections["job789"] = [connection3]
        self.manager.user_connections["user123"] = [connection1, connection3]
        self.manager.user_connections["user456"] = [connection2]
        
        stats = self.manager.get_connection_stats()
        
        assert stats["total_connections"] == 3
        assert stats["active_jobs"] == 2
        assert stats["active_users"] == 2
        assert stats["connections_by_job"]["job456"] == 2
        assert stats["connections_by_job"]["job789"] == 1
    
    @pytest.mark.asyncio
    async def test_cleanup_stale_connections(self):
        """Test cleanup of stale connections."""
        # Create mock connections with old heartbeat
        mock_websocket1 = AsyncMock(spec=WebSocket)
        mock_websocket2 = AsyncMock(spec=WebSocket)
        
        connection1 = WebSocketConnection(mock_websocket1, "user123", "job456")
        connection2 = WebSocketConnection(mock_websocket2, "user456", "job456")
        
        # Make connection1 stale
        connection1.last_heartbeat = datetime.now(timezone.utc) - timedelta(hours=1)
        
        # Add connections to manager
        self.manager.connections["job456"] = [connection1, connection2]
        self.manager.user_connections["user123"] = [connection1]
        self.manager.user_connections["user456"] = [connection2]
        
        await self.manager.cleanup_stale_connections(max_idle_minutes=30)
        
        # Stale connection should be removed and closed
        assert len(self.manager.connections["job456"]) == 1
        assert connection2 in self.manager.connections["job456"]
        assert connection1 not in self.manager.connections["job456"]
        assert "user123" not in self.manager.user_connections
        mock_websocket1.close.assert_called_once()


def test_get_websocket_manager():
    """Test getting the global WebSocket manager instance."""
    manager1 = get_websocket_manager()
    manager2 = get_websocket_manager()
    
    # Should return the same instance
    assert manager1 is manager2
    assert isinstance(manager1, WebSocketConnectionManager)


class TestWebSocketMessage:
    """Test WebSocket message model."""
    
    def test_websocket_message_creation(self):
        """Test WebSocket message creation and serialization."""
        timestamp = datetime.now(timezone.utc)
        message = WebSocketMessage(
            event_type=WebSocketEventType.GAME_MATCHED,
            job_id="job123",
            timestamp=timestamp,
            data={"steam_appid": 123456, "steam_name": "Test Game"}
        )
        
        assert message.event_type == WebSocketEventType.GAME_MATCHED
        assert message.job_id == "job123"
        assert message.timestamp == timestamp
        assert message.data["steam_appid"] == 123456
        
        # Test JSON serialization
        json_data = message.model_dump_json()
        parsed_data = json.loads(json_data)
        
        assert parsed_data["event_type"] == "game_matched"
        assert parsed_data["job_id"] == "job123"
        assert "timestamp" in parsed_data
        assert parsed_data["data"]["steam_appid"] == 123456