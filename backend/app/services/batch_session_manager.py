"""
In-memory batch session management service.

This module provides thread-safe management of batch processing sessions,
including automatic cleanup of expired sessions and session lifecycle management.
"""

import logging
import asyncio
from typing import Dict, Optional, List
from datetime import datetime, timezone, timedelta
from threading import Lock
from contextlib import contextmanager

from ..models.batch_session import (
    BatchSession, 
    BatchOperationType, 
    BatchSessionStatus,
    SESSION_TIMEOUT_MINUTES
)

logger = logging.getLogger(__name__)


class BatchSessionManager:
    """
    Thread-safe in-memory manager for batch processing sessions.
    
    Provides session storage, retrieval, cleanup, and lifecycle management
    for batch operations with automatic expiry handling.
    """
    
    def __init__(self):
        self._sessions: Dict[str, BatchSession] = {}
        self._lock = Lock()
        self._cleanup_task: Optional[asyncio.Task] = None
    
    def start_cleanup_task(self) -> None:
        """Start the background cleanup task for expired sessions."""
        if self._cleanup_task is None or self._cleanup_task.done():
            self._cleanup_task = asyncio.create_task(self._cleanup_loop())
            logger.info("Batch session cleanup task started")
    
    async def stop_cleanup_task(self) -> None:
        """Stop the background cleanup task."""
        if self._cleanup_task and not self._cleanup_task.done():
            self._cleanup_task.cancel()
            try:
                await self._cleanup_task
            except asyncio.CancelledError:
                pass
            logger.info("Batch session cleanup task stopped")
    
    async def _cleanup_loop(self) -> None:
        """Background task to clean up expired sessions."""
        while True:
            try:
                await asyncio.sleep(300)  # Check every 5 minutes
                self._cleanup_expired_sessions()
            except asyncio.CancelledError:
                logger.info("Batch session cleanup loop cancelled")
                break
            except Exception as e:
                logger.error(f"Error in batch session cleanup loop: {str(e)}")
                # Continue the loop despite errors
    
    def _cleanup_expired_sessions(self) -> None:
        """Clean up sessions that have expired or been completed for too long."""
        cutoff_time = datetime.now(timezone.utc) - timedelta(minutes=SESSION_TIMEOUT_MINUTES)
        
        with self._lock:
            expired_sessions = []
            for session_id, session in self._sessions.items():
                # Remove sessions that are expired or completed and old
                if (session.updated_at < cutoff_time or 
                    (session.is_complete and session.updated_at < cutoff_time)):
                    expired_sessions.append(session_id)
            
            for session_id in expired_sessions:
                session = self._sessions.pop(session_id)
                logger.info(
                    f"Cleaned up expired batch session {session_id} "
                    f"(type: {session.operation_type}, status: {session.status})"
                )
    
    @contextmanager
    def _session_lock(self, session_id: str):
        """Context manager for thread-safe session access."""
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                yield session
            else:
                yield None
    
    def create_session(
        self, 
        user_id: str, 
        operation_type: BatchOperationType, 
        total_items: int
    ) -> BatchSession:
        """
        Create a new batch session.
        
        Args:
            user_id: ID of the user starting the batch operation
            operation_type: Type of batch operation (auto_match or sync)
            total_items: Total number of items to process
            
        Returns:
            BatchSession: The created session
        """
        session = BatchSession.create(user_id, operation_type, total_items)
        
        with self._lock:
            self._sessions[session.id] = session
        
        logger.info(
            f"Created batch session {session.id} for user {user_id} "
            f"(type: {operation_type}, items: {total_items})"
        )
        
        return session
    
    def get_session(self, session_id: str) -> Optional[BatchSession]:
        """
        Get a batch session by ID.
        
        Args:
            session_id: ID of the session to retrieve
            
        Returns:
            BatchSession or None if not found
        """
        with self._lock:
            return self._sessions.get(session_id)
    
    def get_user_sessions(self, user_id: str) -> List[BatchSession]:
        """
        Get all active sessions for a user.
        
        Args:
            user_id: ID of the user
            
        Returns:
            List of BatchSession objects for the user
        """
        with self._lock:
            return [
                session for session in self._sessions.values() 
                if session.user_id == user_id
            ]
    
    def update_session_progress(
        self,
        session_id: str,
        processed_count: int,
        successful_count: int,
        failed_count: int,
        processed_ids: List[str],
        failed_ids: List[str],
        errors: List[str]
    ) -> Optional[BatchSession]:
        """
        Update progress for a batch session.
        
        Args:
            session_id: ID of the session to update
            processed_count: Number of items processed in this batch
            successful_count: Number of items successfully processed
            failed_count: Number of items that failed
            processed_ids: List of IDs that were processed
            failed_ids: List of IDs that failed
            errors: List of error messages
            
        Returns:
            Updated BatchSession or None if session not found
        """
        with self._session_lock(session_id) as session:
            if session and session.is_active:
                session.update_progress(
                    processed_count=processed_count,
                    successful_count=successful_count,
                    failed_count=failed_count,
                    processed_ids=processed_ids,
                    failed_ids=failed_ids,
                    errors=errors
                )
                logger.debug(
                    f"Updated session {session_id} progress: "
                    f"{session.processed_items}/{session.total_items} processed, "
                    f"{session.successful_items} successful, {session.failed_items} failed"
                )
                return session
        return None
    
    def cancel_session(self, session_id: str, user_id: str) -> Optional[BatchSession]:
        """
        Cancel a batch session.
        
        Args:
            session_id: ID of the session to cancel
            user_id: ID of the user (for authorization)
            
        Returns:
            Cancelled BatchSession or None if not found/unauthorized
        """
        with self._session_lock(session_id) as session:
            if session and session.user_id == user_id and session.is_active:
                session.cancel()
                logger.info(f"Cancelled batch session {session_id} for user {user_id}")
                return session
        return None
    
    def fail_session(self, session_id: str, error_message: str) -> Optional[BatchSession]:
        """
        Mark a batch session as failed.
        
        Args:
            session_id: ID of the session to fail
            error_message: Error message describing the failure
            
        Returns:
            Failed BatchSession or None if not found
        """
        with self._session_lock(session_id) as session:
            if session and session.is_active:
                session.fail(error_message)
                logger.error(f"Failed batch session {session_id}: {error_message}")
                return session
        return None
    
    def delete_session(self, session_id: str, user_id: str) -> bool:
        """
        Delete a batch session.
        
        Args:
            session_id: ID of the session to delete
            user_id: ID of the user (for authorization)
            
        Returns:
            True if session was deleted, False otherwise
        """
        with self._lock:
            session = self._sessions.get(session_id)
            if session and session.user_id == user_id:
                del self._sessions[session_id]
                logger.info(f"Deleted batch session {session_id} for user {user_id}")
                return True
        return False
    
    def get_session_count(self) -> int:
        """Get the total number of active sessions."""
        with self._lock:
            return len(self._sessions)
    
    def get_stats(self) -> Dict[str, int]:
        """Get statistics about current sessions."""
        with self._lock:
            stats = {
                "total_sessions": len(self._sessions),
                "active_sessions": 0,
                "completed_sessions": 0,
                "cancelled_sessions": 0,
                "failed_sessions": 0,
                "auto_match_sessions": 0,
                "sync_sessions": 0
            }
            
            for session in self._sessions.values():
                if session.status == BatchSessionStatus.ACTIVE:
                    stats["active_sessions"] += 1
                elif session.status == BatchSessionStatus.COMPLETED:
                    stats["completed_sessions"] += 1
                elif session.status == BatchSessionStatus.CANCELLED:
                    stats["cancelled_sessions"] += 1
                elif session.status == BatchSessionStatus.FAILED:
                    stats["failed_sessions"] += 1
                
                if session.operation_type == BatchOperationType.AUTO_MATCH:
                    stats["auto_match_sessions"] += 1
                elif session.operation_type == BatchOperationType.SYNC:
                    stats["sync_sessions"] += 1
            
            return stats


# Global session manager instance
_session_manager: Optional[BatchSessionManager] = None


def get_batch_session_manager() -> BatchSessionManager:
    """Get the global batch session manager instance."""
    global _session_manager
    if _session_manager is None:
        _session_manager = BatchSessionManager()
    return _session_manager


async def startup_batch_session_manager():
    """Initialize the batch session manager on application startup."""
    manager = get_batch_session_manager()
    manager.start_cleanup_task()
    logger.info("Batch session manager initialized and cleanup task started")


async def shutdown_batch_session_manager():
    """Cleanup the batch session manager on application shutdown."""
    global _session_manager
    if _session_manager:
        await _session_manager.stop_cleanup_task()
        logger.info("Batch session manager shutdown completed")