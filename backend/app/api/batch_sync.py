"""
Batch sync API endpoints.

This module provides endpoints for batched Steam game sync operations,
allowing the frontend to process matched games in small batches with progress feedback.
"""

from fastapi import APIRouter, Depends, HTTPException, status
from sqlmodel import Session, select, and_
from typing import Annotated, List
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.user_game import UserGame
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

router = APIRouter(prefix="/steam-games/batch/sync", tags=["Steam Games Batch Sync"])
logger = logging.getLogger(__name__)


@router.post("/start", response_model=BatchSessionStartResponse, status_code=status.HTTP_201_CREATED)
async def start_batch_sync(
    request: BatchSessionStartRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchSessionStartResponse:
    """
    Start a new batch sync session.
    
    This endpoint creates a session for syncing matched Steam games to the collection in batches.
    The frontend can then call the next endpoint repeatedly to sync games in small chunks.
    """
    try:
        logger.info(f"Starting batch sync session for user {current_user.id}")
        
        # Find all matched Steam games that haven't been synced yet
        matched_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == current_user.id,
                SteamGame.igdb_id.isnot(None),  # Has IGDB match
                SteamGame.game_id.is_(None),     # Not yet synced
                SteamGame.ignored == False       # Not ignored
            )
        )
        matched_games = session.exec(matched_games_query).all()
        total_items = len(matched_games)
        
        if total_items == 0:
            logger.info(f"No matched Steam games found for user {current_user.id}")
            return BatchSessionStartResponse(
                session_id="",
                total_items=0,
                operation_type=BatchOperationType.SYNC.value,
                status="completed",
                message="No matched Steam games found to sync"
            )
        
        # Create the batch session
        session_manager = get_batch_session_manager()
        batch_session = session_manager.create_session(
            user_id=current_user.id,
            operation_type=BatchOperationType.SYNC,
            total_items=total_items
        )
        
        logger.info(
            f"Created batch sync session {batch_session.id} for user {current_user.id} "
            f"with {total_items} matched games"
        )
        
        return BatchSessionStartResponse(
            session_id=batch_session.id,
            total_items=total_items,
            operation_type=BatchOperationType.SYNC.value,
            status=batch_session.status.value,
            message=f"Batch sync session started for {total_items} matched games"
        )
        
    except Exception as e:
        logger.error(f"Error starting batch sync session for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to start batch sync session"
        )


@router.post("/{session_id}/next", response_model=BatchNextResponse, status_code=status.HTTP_200_OK)
async def process_next_sync_batch(
    session_id: str,
    request: BatchNextRequest,
    db_session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchNextResponse:
    """
    Process the next batch of matched Steam games for syncing.
    
    This endpoint processes a small batch of games (default 10) and returns
    progress information along with the processed games data.
    """
    try:
        logger.info(f"Processing next sync batch for session {session_id} by user {current_user.id}")
        
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
        
        # Get next batch of matched games to sync
        batch_size = BATCH_SIZES[BatchOperationType.SYNC]
        matched_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == current_user.id,
                SteamGame.igdb_id.isnot(None),
                SteamGame.game_id.is_(None),
                SteamGame.ignored == False,
                ~SteamGame.id.in_(batch_session.processed_item_ids)  # Exclude already processed
            )
        ).limit(batch_size)
        
        games_to_process = db_session.exec(matched_games_query).all()
        
        if not games_to_process:
            # No more games to process - mark session as complete
            batch_session.status = BatchSessionStatus.COMPLETED if batch_session.status.value != "cancelled" else batch_session.status
            logger.info(f"Batch sync session {session_id} completed - no more games to process")
            return _create_batch_response(batch_session, [], "No more games to process")
        
        # Process the batch using the steam games service
        steam_games_service = create_steam_games_service(db_session)
        
        logger.info(f"Syncing {len(games_to_process)} games in batch for session {session_id}")
        
        # Process each game individually and collect results
        successful_syncs = 0
        failed_syncs = 0
        processed_ids = []
        failed_ids = []
        batch_errors = []
        
        for game in games_to_process:
            try:
                sync_result = await steam_games_service.sync_steam_game_to_collection(
                    steam_game_id=game.id,
                    user_id=current_user.id
                )
                
                if sync_result.action != "failed":
                    successful_syncs += 1
                    processed_ids.append(game.id)
                else:
                    failed_syncs += 1
                    failed_ids.append(game.id)
                    if sync_result.error_message:
                        batch_errors.append(f"{game.game_name}: {sync_result.error_message}")
                        
            except Exception as e:
                failed_syncs += 1
                failed_ids.append(game.id)
                error_msg = f"{game.game_name}: {str(e)}"
                batch_errors.append(error_msg)
                logger.error(f"Error syncing game {game.id} ({game.game_name}): {str(e)}")
        
        # Update session progress
        session_manager.update_session_progress(
            session_id=session_id,
            processed_count=len(games_to_process),
            successful_count=successful_syncs,
            failed_count=failed_syncs,
            processed_ids=processed_ids,
            failed_ids=failed_ids,
            errors=batch_errors
        )
        
        # Get updated games data for response (including user_game_id)
        updated_games_query = select(SteamGame).where(
            SteamGame.id.in_([game.id for game in games_to_process])
        )
        updated_games = db_session.exec(updated_games_query).all()
        
        # Convert to response format with user_game_id lookup
        current_batch_items = []
        for game in updated_games:
            # Get user_game_id if the game was synced
            user_game_id = None
            if game.game_id:
                user_game_result = db_session.exec(
                    select(UserGame.id).where(
                        and_(
                            UserGame.game_id == game.game_id,
                            UserGame.user_id == current_user.id
                        )
                    )
                ).first()
                user_game_id = user_game_result
            
            current_batch_items.append(SteamGameResponse(
                id=game.id,
                steam_appid=game.steam_appid,
                game_name=game.game_name,
                igdb_id=game.igdb_id,
                game_id=game.game_id,
                user_game_id=user_game_id,
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at
            ))
        
        message = f"Processed sync batch of {len(games_to_process)} games: {successful_syncs} synced successfully, {failed_syncs} failed"
        
        logger.info(f"Completed sync batch processing for session {session_id}: {message}")
        
        return _create_batch_response(
            batch_session, 
            current_batch_items, 
            message, 
            successful_syncs, 
            failed_syncs, 
            batch_errors
        )
        
    except HTTPException:
        # Re-raise HTTP exceptions without modification
        raise
    except Exception as e:
        logger.error(f"Error processing sync batch for session {session_id}: {str(e)}")
        
        # Mark session as failed
        session_manager = get_batch_session_manager()
        session_manager.fail_session(session_id, str(e))
        
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to process sync batch"
        )


@router.get("/{session_id}/status", response_model=BatchStatusResponse, status_code=status.HTTP_200_OK)
async def get_batch_sync_status(
    session_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchStatusResponse:
    """
    Get the current status of a batch sync session.
    
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
        logger.error(f"Error getting batch sync status for session {session_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get batch sync status"
        )


@router.delete("/{session_id}", response_model=BatchCancelResponse, status_code=status.HTTP_200_OK)
async def cancel_batch_sync_session(
    session_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchCancelResponse:
    """
    Cancel a batch sync session.
    
    This endpoint cancels an active batch session, preserving any progress made so far.
    """
    try:
        logger.info(f"Cancelling batch sync session {session_id} for user {current_user.id}")
        
        session_manager = get_batch_session_manager()
        batch_session = session_manager.cancel_session(session_id, current_user.id)
        
        if not batch_session:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Batch session not found or access denied"
            )
        
        logger.info(
            f"Cancelled batch sync session {session_id}: "
            f"{batch_session.processed_items} processed, {batch_session.successful_items} successful"
        )
        
        return BatchCancelResponse(
            session_id=batch_session.id,
            status=batch_session.status.value,
            processed_items=batch_session.processed_items,
            successful_items=batch_session.successful_items,
            failed_items=batch_session.failed_items,
            message=f"Batch sync session cancelled. Processed {batch_session.processed_items} games with {batch_session.successful_items} successful syncs."
        )
        
    except HTTPException:
        # Re-raise HTTP exceptions
        raise
    except Exception as e:
        logger.error(f"Error cancelling batch sync session {session_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to cancel batch sync session"
        )


def _create_batch_response(
    batch_session, 
    current_batch_items: List[SteamGameResponse], 
    message: str,
    batch_successful: int = 0,
    batch_failed: int = 0,
    batch_errors: List[str] = None
) -> BatchNextResponse:
    """Helper function to create a consistent batch response."""
    batch_processed = len(current_batch_items)
    
    return BatchNextResponse(
        session_id=batch_session.id,
        batch_processed=batch_processed,
        batch_successful=batch_successful,
        batch_failed=batch_failed,
        batch_errors=batch_errors or [],
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