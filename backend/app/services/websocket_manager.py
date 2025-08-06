"""
WebSocket connection manager service for real-time Steam import communication.
"""

import json
import logging
from datetime import datetime, timezone
from enum import Enum
from typing import Dict, List, Optional, Set, Any
from fastapi import WebSocket, WebSocketDisconnect, HTTPException, status
from sqlmodel import Session
from pydantic import BaseModel, ConfigDict, field_serializer

from ..core.security import verify_token, hash_token
from ..core.database import get_session
from ..models.user import User, UserSession
from ..models.steam_import import SteamImportJob
from sqlmodel import select

logger = logging.getLogger(__name__)


class WebSocketEventType(str, Enum):
    """WebSocket event types for Steam import system."""
    IMPORT_STATUS_CHANGE = "import_status_change"
    IMPORT_PROGRESS = "import_progress" 
    GAME_MATCHED = "game_matched"
    GAME_NEEDS_REVIEW = "game_needs_review"
    GAME_IMPORTED = "game_imported"
    PLATFORM_ADDED = "platform_added"
    GAME_SKIPPED = "game_skipped"
    IMPORT_COMPLETE = "import_complete"
    IMPORT_ERROR = "import_error"
    CONNECTION_STATUS = "connection_status"
    HEARTBEAT = "heartbeat"
    PONG = "pong"


class WebSocketMessage(BaseModel):
    """WebSocket message structure."""
    event_type: WebSocketEventType
    job_id: str
    timestamp: datetime
    data: Dict[str, Any]
    
    model_config = ConfigDict()
    
    @field_serializer('timestamp')
    def serialize_timestamp(self, dt: datetime) -> str:
        return dt.isoformat()


class WebSocketConnection:
    """Represents an active WebSocket connection."""
    
    def __init__(self, websocket: WebSocket, user_id: str, job_id: str):
        self.websocket = websocket
        self.user_id = user_id
        self.job_id = job_id
        self.connected_at = datetime.now(timezone.utc)
        self.last_heartbeat = datetime.now(timezone.utc)
    
    async def send_message(self, message: WebSocketMessage) -> bool:
        """Send a message to this connection."""
        try:
            await self.websocket.send_text(message.model_dump_json())
            logger.debug(f"Message sent to connection {self.user_id}/{self.job_id}: {message.event_type}")
            return True
        except Exception as e:
            logger.error(f"Error sending message to connection {self.user_id}/{self.job_id}: {str(e)}")
            return False
    
    async def send_heartbeat(self) -> bool:
        """Send a heartbeat message to keep connection alive."""
        heartbeat_message = WebSocketMessage(
            event_type=WebSocketEventType.HEARTBEAT,
            job_id=self.job_id,
            timestamp=datetime.now(timezone.utc),
            data={"status": "ping"}
        )
        success = await self.send_message(heartbeat_message)
        if success:
            self.last_heartbeat = datetime.now(timezone.utc)
        return success


class WebSocketConnectionManager:
    """Manages WebSocket connections for Steam import system."""
    
    def __init__(self):
        # Active connections grouped by job_id
        self.connections: Dict[str, List[WebSocketConnection]] = {}
        # Quick lookup by user_id for targeted messaging
        self.user_connections: Dict[str, List[WebSocketConnection]] = {}
        logger.info("WebSocket connection manager initialized")
    
    async def authenticate_and_connect(
        self, 
        websocket: WebSocket, 
        job_id: str, 
        token: str,
        session: Session
    ) -> Optional[WebSocketConnection]:
        """
        Authenticate user and establish WebSocket connection.
        
        Args:
            websocket: WebSocket connection
            job_id: Steam import job ID
            token: JWT access token
            session: Database session
            
        Returns:
            WebSocketConnection if successful, None if authentication failed
        """
        try:
            # Verify JWT token
            payload = verify_token(token, "access")
            user_id = payload.get("sub")
            if not user_id:
                await websocket.close(code=status.WS_1008_POLICY_VIOLATION, reason="Invalid token")
                return None
            
            # Verify session exists
            token_hash = hash_token(token)
            session_record = session.exec(
                select(UserSession).where(
                    (UserSession.user_id == user_id) & 
                    (UserSession.token_hash == token_hash)
                )
            ).first()
            
            if not session_record:
                await websocket.close(code=status.WS_1008_POLICY_VIOLATION, reason="Session not found")
                return None
            
            # Get user and verify active status
            user = session.get(User, user_id)
            if not user or not user.is_active:
                await websocket.close(code=status.WS_1008_POLICY_VIOLATION, reason="User not found or inactive")
                return None
            
            # Verify user owns the import job
            import_job = session.get(SteamImportJob, job_id)
            if not import_job:
                await websocket.close(code=status.WS_1003_UNSUPPORTED_DATA, reason="Import job not found")
                return None
            
            if import_job.user_id != user_id:
                await websocket.close(code=status.WS_1008_POLICY_VIOLATION, reason="Access denied to import job")
                return None
            
            # Accept WebSocket connection
            await websocket.accept()
            logger.debug(f"WebSocket connection accepted for job {job_id}, user {user_id}")
            
            # Create connection object
            connection = WebSocketConnection(websocket, user_id, job_id)
            
            # Add to connection tracking
            if job_id not in self.connections:
                self.connections[job_id] = []
            self.connections[job_id].append(connection)
            
            if user_id not in self.user_connections:
                self.user_connections[user_id] = []
            self.user_connections[user_id].append(connection)
            
            logger.info(f"WebSocket connection established: user={user.username}, job={job_id}")
            
            # Send connection confirmation
            await self._send_connection_status(connection, "connected")
            
            return connection
            
        except Exception as e:
            logger.error(f"WebSocket authentication error: {str(e)}")
            try:
                await websocket.close(code=status.WS_1011_INTERNAL_ERROR, reason="Authentication error")
            except Exception:
                pass  # Connection might already be closed
            return None
    
    async def disconnect(self, connection: WebSocketConnection):
        """Remove a connection from tracking."""
        try:
            # Remove from job connections
            if connection.job_id in self.connections:
                if connection in self.connections[connection.job_id]:
                    self.connections[connection.job_id].remove(connection)
                if not self.connections[connection.job_id]:
                    del self.connections[connection.job_id]
            
            # Remove from user connections
            if connection.user_id in self.user_connections:
                if connection in self.user_connections[connection.user_id]:
                    self.user_connections[connection.user_id].remove(connection)
                if not self.user_connections[connection.user_id]:
                    del self.user_connections[connection.user_id]
            
            logger.info(f"WebSocket connection removed: user={connection.user_id}, job={connection.job_id}")
            
        except Exception as e:
            logger.error(f"Error removing WebSocket connection: {str(e)}")
    
    async def send_to_job(self, job_id: str, event_type: WebSocketEventType, data: Dict[str, Any]):
        """Send a message to all connections for a specific job."""
        if job_id not in self.connections:
            logger.debug(f"No active connections for job {job_id}")
            return
        
        logger.debug(f"Sending {event_type} event to {len(self.connections[job_id])} connections for job {job_id}")
        
        message = WebSocketMessage(
            event_type=event_type,
            job_id=job_id,
            timestamp=datetime.now(timezone.utc),
            data=data
        )
        
        # Send to all connections for this job
        disconnected_connections = []
        for connection in self.connections[job_id][:]:  # Create a copy to iterate safely
            success = await connection.send_message(message)
            if not success:
                disconnected_connections.append(connection)
        
        # Clean up failed connections
        for connection in disconnected_connections:
            await self.disconnect(connection)
        
        success_count = len(self.connections.get(job_id, []))
        if disconnected_connections:
            logger.debug(f"Sent {event_type} event to {success_count} connections for job {job_id} ({len(disconnected_connections)} failed)")
        else:
            logger.debug(f"Sent {event_type} event to {success_count} connections for job {job_id}")
    
    async def send_to_user(self, user_id: str, event_type: WebSocketEventType, job_id: str, data: Dict[str, Any]):
        """Send a message to all connections for a specific user."""
        if user_id not in self.user_connections:
            logger.debug(f"No active connections for user {user_id}")
            return
        
        message = WebSocketMessage(
            event_type=event_type,
            job_id=job_id,
            timestamp=datetime.now(timezone.utc),
            data=data
        )
        
        # Send to all connections for this user
        disconnected_connections = []
        for connection in self.user_connections[user_id][:]:  # Create a copy to iterate safely
            success = await connection.send_message(message)
            if not success:
                disconnected_connections.append(connection)
        
        # Clean up failed connections
        for connection in disconnected_connections:
            await self.disconnect(connection)
        
        logger.debug(f"Sent {event_type} event to {len(self.user_connections.get(user_id, []))} connections for user {user_id}")
    
    async def broadcast_to_all(self, event_type: WebSocketEventType, job_id: str, data: Dict[str, Any]):
        """Broadcast a message to all active connections."""
        message = WebSocketMessage(
            event_type=event_type,
            job_id=job_id,
            timestamp=datetime.now(timezone.utc),
            data=data
        )
        
        total_sent = 0
        disconnected_connections = []
        
        for job_connections in self.connections.values():
            for connection in job_connections[:]:  # Create a copy to iterate safely
                success = await connection.send_message(message)
                if success:
                    total_sent += 1
                else:
                    disconnected_connections.append(connection)
        
        # Clean up failed connections
        for connection in disconnected_connections:
            await self.disconnect(connection)
        
        logger.debug(f"Broadcast {event_type} event to {total_sent} connections")
    
    async def _send_connection_status(self, connection: WebSocketConnection, status: str):
        """Send connection status message to a specific connection."""
        status_message = WebSocketMessage(
            event_type=WebSocketEventType.CONNECTION_STATUS,
            job_id=connection.job_id,
            timestamp=datetime.now(timezone.utc),
            data={
                "status": status,
                "connected_at": connection.connected_at.isoformat(),
                "user_id": connection.user_id
            }
        )
        await connection.send_message(status_message)
    
    def get_connection_stats(self) -> Dict[str, Any]:
        """Get statistics about active connections."""
        total_connections = sum(len(connections) for connections in self.connections.values())
        active_jobs = len(self.connections)
        active_users = len(self.user_connections)
        
        return {
            "total_connections": total_connections,
            "active_jobs": active_jobs,
            "active_users": active_users,
            "connections_by_job": {job_id: len(connections) for job_id, connections in self.connections.items()}
        }
    
    async def cleanup_stale_connections(self, max_idle_minutes: int = 30):
        """Clean up stale connections that haven't sent heartbeats."""
        stale_threshold = datetime.now(timezone.utc).timestamp() - (max_idle_minutes * 60)
        stale_connections = []
        
        for job_connections in self.connections.values():
            for connection in job_connections:
                if connection.last_heartbeat.timestamp() < stale_threshold:
                    stale_connections.append(connection)
        
        # Clean up stale connections
        for connection in stale_connections:
            try:
                await connection.websocket.close(code=status.WS_1001_GOING_AWAY, reason="Connection timeout")
            except Exception:
                pass  # Connection might already be closed
            await self.disconnect(connection)
        
        if stale_connections:
            logger.info(f"Cleaned up {len(stale_connections)} stale WebSocket connections")


# Global connection manager instance
websocket_manager = WebSocketConnectionManager()


def get_websocket_manager() -> WebSocketConnectionManager:
    """Get the global WebSocket connection manager instance."""
    return websocket_manager