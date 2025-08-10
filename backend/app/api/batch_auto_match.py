"""
Batch auto-matching API endpoints.

This module provides endpoints for batched Steam game auto-matching operations,
allowing the frontend to process games in small batches with progress feedback.
"""

from fastapi import APIRouter, Depends, HTTPException, status
from sqlmodel import Session, select, and_
from typing import Annotated, List
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.batch_session import BatchOperationType, BATCH_SIZES, BatchSessionStatus
from ..services.batch_session_manager import get_batch_session_manager
from ..services.steam_games import create_steam_games_service
from ..api.dependencies import verify_steam_games_enabled
from .schemas.batch import (
    BatchSessionStartRequest,
    BatchSessionStartResponse,
    BatchNextRequest, 
    BatchNextResponse,
    BatchStatusResponse,
    BatchCancelResponse
)
from .schemas.steam import SteamGameResponse

router = APIRouter(prefix="/steam-games/batch/auto-match", tags=["Steam Games Batch Auto-Match"])
logger = logging.getLogger(__name__)


@router.post("/start", response_model=BatchSessionStartResponse, status_code=status.HTTP_201_CREATED)
async def start_batch_auto_match(
    request: BatchSessionStartRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchSessionStartResponse:
    """
    Start a new batch auto-matching session.
    
    This endpoint creates a session for processing unmatched Steam games in batches.
    The frontend can then call the next endpoint repeatedly to process games in small chunks.
    """
    try:
        logger.info(f"Starting batch auto-match session for user {current_user.id}")
        
        # Find all unmatched Steam games for this user
        unmatched_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == current_user.id,
                SteamGame.igdb_id.is_(None),  # No IGDB match yet
                SteamGame.ignored == False    # Not ignored by user
            )
        )
        unmatched_games = session.exec(unmatched_games_query).all()
        total_items = len(unmatched_games)
        
        if total_items == 0:
            logger.info(f"No unmatched Steam games found for user {current_user.id}")
            return BatchSessionStartResponse(
                session_id="",
                total_items=0,
                operation_type=BatchOperationType.AUTO_MATCH.value,
                status="completed",
                message="No unmatched Steam games found to process"
            )
        
        # Create the batch session
        session_manager = get_batch_session_manager()
        batch_session = session_manager.create_session(
            user_id=current_user.id,
            operation_type=BatchOperationType.AUTO_MATCH,
            total_items=total_items
        )
        
        logger.info(
            f"Created batch auto-match session {batch_session.id} for user {current_user.id} "
            f"with {total_items} unmatched games"
        )
        
        return BatchSessionStartResponse(
            session_id=batch_session.id,
            total_items=total_items,
            operation_type=BatchOperationType.AUTO_MATCH.value,
            status=batch_session.status.value,
            message=f"Batch auto-match session started for {total_items} unmatched games"
        )
        
    except Exception as e:
        logger.error(f"Error starting batch auto-match session for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to start batch auto-match session"
        )


@router.post("/{session_id}/next", response_model=BatchNextResponse, status_code=status.HTTP_200_OK)
async def process_next_batch(
    session_id: str,
    request: BatchNextRequest,
    db_session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchNextResponse:
    """
    Process the next batch of unmatched Steam games for auto-matching.
    
    This endpoint processes a small batch of games (default 7) and returns
    progress information along with the processed games data.
    """
    try:
        logger.info(f"Processing next batch for session {session_id} by user {current_user.id}")
        
        # Get the batch session
        session_manager = get_batch_session_manager()
        batch_session = session_manager.get_session(session_id)
        
        if not batch_session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Batch session not found"
            )
        
        if batch_session.user_id != current_user.id:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Access denied to this batch session"
            )
        
        if not batch_session.is_active:
            logger.warning(f"Attempt to process inactive session {session_id} (status: {batch_session.status})")
            return _create_batch_response(batch_session, [], "Session is not active")
        
        # Get next batch of unmatched games
        batch_size = BATCH_SIZES[BatchOperationType.AUTO_MATCH]
        unmatched_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == current_user.id,
                SteamGame.igdb_id.is_(None),
                SteamGame.ignored == False,
                ~SteamGame.id.in_(batch_session.processed_item_ids)  # Exclude already processed
            )
        ).limit(batch_size)
        
        games_to_process = db_session.exec(unmatched_games_query).all()
        
        if not games_to_process:
            # No more games to process - mark session as complete
            batch_session.status = BatchSessionStatus.COMPLETED if batch_session.status.value != "cancelled" else batch_session.status
            logger.info(f"Batch auto-match session {session_id} completed - no more games to process")
            return _create_batch_response(batch_session, [], "No more games to process")
        
        # Process the batch using the steam games service
        steam_games_service = create_steam_games_service(db_session)
        game_ids = [game.id for game in games_to_process]
        
        logger.info(f"Auto-matching {len(game_ids)} games in batch for session {session_id}")
        
        # Process the auto-matching batch
        match_results = await steam_games_service._auto_match_steam_games(game_ids)
        
        # Update session progress
        processed_ids = [result.steam_game_id for result in match_results.results]
        failed_ids = [result.steam_game_id for result in match_results.results if not result.matched and result.error_message]
        
        session_manager.update_session_progress(
            session_id=session_id,
            processed_count=len(games_to_process),
            successful_count=match_results.successful_matches,
            failed_count=match_results.failed_matches + match_results.skipped_games,  # Count skipped games as failed for UI purposes
            processed_ids=processed_ids,
            failed_ids=failed_ids,
            errors=match_results.errors
        )
        
        # Get updated games data for response
        updated_games_query = select(SteamGame).where(
            SteamGame.id.in_(game_ids)
        )
        updated_games = db_session.exec(updated_games_query).all()
        
        # Convert to response format
        current_batch_items = [
            SteamGameResponse(
                id=game.id,
                steam_appid=game.steam_appid,
                game_name=game.game_name,
                igdb_id=game.igdb_id,
                game_id=game.game_id,
                user_game_id=None,  # Not synced yet
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at
            )
            for game in updated_games
        ]
        
        message = f"Processed batch of {len(games_to_process)} games: {match_results.successful_matches} matched, {match_results.failed_matches} failed, {match_results.skipped_games} skipped"
        
        logger.info(f"Completed batch processing for session {session_id}: {message}")
        
        return _create_batch_response(batch_session, current_batch_items, message, match_results)
        
    except HTTPException:
        # Re-raise HTTP exceptions without modification
        raise
    except Exception as e:
        logger.error(f"Error processing batch for session {session_id}: {str(e)}")
        
        # Mark session as failed
        session_manager = get_batch_session_manager()
        session_manager.fail_session(session_id, str(e))
        
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to process batch"
        )


@router.get("/{session_id}/status", response_model=BatchStatusResponse, status_code=status.HTTP_200_OK)
async def get_batch_status(
    session_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchStatusResponse:
    """
    Get the current status of a batch auto-matching session.
    
    This endpoint returns detailed progress information for a batch session.
    """
    try:
        session_manager = get_batch_session_manager()
        batch_session = session_manager.get_session(session_id)
        
        if not batch_session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Batch session not found"
            )
        
        if batch_session.user_id != current_user.id:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Access denied to this batch session"
            )
        
        return BatchStatusResponse(
            session_id=batch_session.id,
            operation_type=batch_session.operation_type.value,
            total_items=batch_session.total_items,
            processed_items=batch_session.processed_items,
            successful_items=batch_session.successful_items,
            failed_items=batch_session.failed_items,
            remaining_items=batch_session.remaining_items,
            progress_percentage=batch_session.progress_percentage,
            status=batch_session.status.value,
            is_complete=batch_session.is_complete,
            created_at=batch_session.created_at,
            updated_at=batch_session.updated_at,
            errors=batch_session.errors
        )
        
    except HTTPException:
        # Re-raise HTTP exceptions
        raise
    except Exception as e:
        logger.error(f"Error getting batch status for session {session_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get batch status"
        )


@router.delete("/{session_id}", response_model=BatchCancelResponse, status_code=status.HTTP_200_OK)
async def cancel_batch_session(
    session_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchCancelResponse:
    """
    Cancel a batch auto-matching session.
    
    This endpoint cancels an active batch session, preserving any progress made so far.
    """
    try:
        logger.info(f"Cancelling batch session {session_id} for user {current_user.id}")
        
        session_manager = get_batch_session_manager()
        batch_session = session_manager.cancel_session(session_id, current_user.id)
        
        if not batch_session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Batch session not found or access denied"
            )
        
        logger.info(
            f"Cancelled batch session {session_id}: "
            f"{batch_session.processed_items} processed, {batch_session.successful_items} successful"
        )
        
        return BatchCancelResponse(
            session_id=batch_session.id,
            status=batch_session.status.value,
            processed_items=batch_session.processed_items,
            successful_items=batch_session.successful_items,
            failed_items=batch_session.failed_items,
            message=f"Batch session cancelled. Processed {batch_session.processed_items} games with {batch_session.successful_items} successful matches."
        )
        
    except HTTPException:
        # Re-raise HTTP exceptions
        raise
    except Exception as e:
        logger.error(f"Error cancelling batch session {session_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to cancel batch session"
        )


def _create_batch_response(
    batch_session, 
    current_batch_items: List[SteamGameResponse], 
    message: str,
    match_results=None
) -> BatchNextResponse:
    """Helper function to create a consistent batch response."""
    batch_processed = len(current_batch_items) if match_results else 0
    batch_successful = match_results.successful_matches if match_results else 0
    batch_failed = match_results.failed_matches if match_results else 0
    batch_errors = match_results.errors if match_results else []
    
    return BatchNextResponse(
        session_id=batch_session.id,
        batch_processed=batch_processed,
        batch_successful=batch_successful,
        batch_failed=batch_failed,
        batch_errors=batch_errors,
        current_batch_items=current_batch_items,
        total_items=batch_session.total_items,
        processed_items=batch_session.processed_items,
        successful_items=batch_session.successful_items,
        failed_items=batch_session.failed_items,
        remaining_items=batch_session.remaining_items,
        progress_percentage=batch_session.progress_percentage,
        status=batch_session.status.value,
        is_complete=batch_session.is_complete,
        message=message
    )