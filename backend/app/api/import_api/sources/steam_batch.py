"""
Steam batch processing endpoints for the new import framework.

This module provides batch processing endpoints for Steam games under the new
import API structure, integrating with existing batch session infrastructure.
"""

from fastapi import APIRouter, Depends, HTTPException, status
from sqlmodel import Session, select, and_, col
from typing import Annotated, List
import logging

from ....core.database import get_session
from ...dependencies import verify_steam_games_enabled
from ....models.user import User
from ....models.steam_game import SteamGame
from ....models.batch_session import BatchOperationType, BATCH_SIZES, BatchSessionStatus
from ....services.batch_session_manager import get_batch_session_manager
from ....services.import_sources.steam import create_steam_import_service
from ....utils.sqlalchemy_typed import is_, is_not, in_
from ....schemas.batch import (
    BatchSessionStartRequest,
    BatchSessionStartResponse,
    BatchNextRequest, 
    BatchNextResponse,
    BatchStatusResponse,
    BatchCancelResponse
)
from ....schemas.steam import SteamGameResponse

router = APIRouter(prefix="/batch", tags=["Steam Batch Import"])
logger = logging.getLogger(__name__)


def get_steam_service(session: Annotated[Session, Depends(get_session)]):
    """Dependency to get Steam import service."""
    return create_steam_import_service(session)


# Auto-match batch endpoints
@router.post("/auto-match/start", response_model=BatchSessionStartResponse, status_code=status.HTTP_201_CREATED)
async def start_batch_auto_match(
    request: BatchSessionStartRequest,
    db_session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchSessionStartResponse:
    """
    Start a new batch auto-matching session for Steam games.
    
    This endpoint creates a session for processing unmatched Steam games in batches.
    The frontend can then call the next endpoint repeatedly to process games in small chunks.
    """
    try:
        logger.info(f"Starting batch auto-match session for user {current_user.id}")
        
        # Find all unmatched Steam games for this user
        unmatched_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == current_user.id,
                is_(col(SteamGame.igdb_id), None),  # No IGDB match yet
                is_(SteamGame.ignored, False)    # Not ignored by user
            )
        )
        unmatched_games = db_session.exec(unmatched_games_query).all()
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


@router.post("/auto-match/{session_id}/next", response_model=BatchNextResponse, status_code=status.HTTP_200_OK)
async def process_next_auto_match_batch(
    session_id: str,
    request: BatchNextRequest,
    db_session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> BatchNextResponse:
    """
    Process the next batch of unmatched Steam games for auto-matching.
    
    This endpoint processes a small batch of games and returns progress information
    along with the processed games data.
    """
    try:
        logger.info(f"Processing next auto-match batch for session {session_id} by user {current_user.id}")
        
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
                is_(col(SteamGame.igdb_id), None),
                is_(SteamGame.ignored, False),
                ~in_(col(SteamGame.id), batch_session.processed_item_ids)  # Exclude already processed
            )
        ).limit(batch_size)
        
        games_to_process = db_session.exec(unmatched_games_query).all()
        
        if not games_to_process:
            # No more games to process - mark session as complete
            batch_session.status = BatchSessionStatus.COMPLETED if batch_session.status.value != "cancelled" else batch_session.status
            logger.info(f"Batch auto-match session {session_id} completed - no more games to process")
            return _create_batch_response(batch_session, [], "No more games to process")
        
        # Process the batch using the steam import service
        game_ids = [game.id for game in games_to_process]
        
        logger.info(f"Auto-matching {len(game_ids)} games in batch for session {session_id}")
        
        # Process games individually through the import service
        successful_count = 0
        failed_count = 0
        errors = []
        
        for game_id in game_ids:
            try:
                result = await steam_service.auto_match_game(current_user.id, game_id)
                if result.matched:
                    successful_count += 1
                else:
                    failed_count += 1
                    if result.error_message:
                        errors.append(f"Game {game_id}: {result.error_message}")
            except Exception as e:
                failed_count += 1
                errors.append(f"Game {game_id}: {str(e)}")
        
        # Update session progress
        processed_ids = game_ids
        failed_ids = [game_ids[i] for i in range(len(game_ids)) if i >= successful_count]  # Approximate failed IDs
        
        session_manager.update_session_progress(
            session_id=session_id,
            processed_count=len(games_to_process),
            successful_count=successful_count,
            failed_count=failed_count,
            processed_ids=processed_ids,
            failed_ids=failed_ids,
            errors=errors
        )
        
        # Get updated games data for response
        updated_games_query = select(SteamGame).where(
            col(SteamGame.id).in_(game_ids)
        )
        updated_games = db_session.exec(updated_games_query).all()
        
        # Convert to response format using the original Steam schema field names
        current_batch_items = [
            SteamGameResponse.model_validate({
                "id": game.id,
                "steam_appid": game.steam_appid,  # Keep original field name for schema compatibility
                "game_name": game.game_name,  # Keep original field name for schema compatibility
                "igdb_id": game.igdb_id,
                "igdb_title": game.igdb_title,
                "game_id": game.game_id,
                "user_game_id": None,  # Not synced yet
                "ignored": game.ignored,
                "created_at": game.created_at,
                "updated_at": game.updated_at
            })
            for game in updated_games
        ]
        
        message = f"Processed batch of {len(games_to_process)} games: {successful_count} matched, {failed_count} failed"
        
        logger.info(f"Completed auto-match batch processing for session {session_id}: {message}")
        
        return BatchNextResponse(
            session_id=batch_session.id,
            batch_processed=len(current_batch_items),
            batch_successful=successful_count,
            batch_failed=failed_count,
            batch_errors=errors,
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
        
    except HTTPException:
        # Re-raise HTTP exceptions without modification
        raise
    except Exception as e:
        logger.error(f"Error processing auto-match batch for session {session_id}: {str(e)}")
        
        # Mark session as failed
        session_manager = get_batch_session_manager()
        session_manager.fail_session(session_id, str(e))
        
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to process batch"
        )


# Sync batch endpoints
@router.post("/sync/start", response_model=BatchSessionStartResponse, status_code=status.HTTP_201_CREATED)
async def start_batch_sync(
    request: BatchSessionStartRequest,
    db_session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchSessionStartResponse:
    """
    Start a new batch sync session for Steam games.
    
    This endpoint creates a session for syncing matched Steam games to the main collection in batches.
    """
    try:
        logger.info(f"Starting batch sync session for user {current_user.id}")
        
        # Find all matched but not synced Steam games for this user
        matched_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == current_user.id,
                is_not(col(SteamGame.igdb_id), None),  # Has IGDB match
                is_(col(SteamGame.game_id), None),     # Not yet synced to collection
                is_(SteamGame.ignored, False)       # Not ignored by user
            )
        )
        matched_games = db_session.exec(matched_games_query).all()
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


@router.post("/sync/{session_id}/next", response_model=BatchNextResponse, status_code=status.HTTP_200_OK)
async def process_next_sync_batch(
    session_id: str,
    request: BatchNextRequest,
    db_session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> BatchNextResponse:
    """
    Process the next batch of matched Steam games for syncing to collection.
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
                is_not(col(SteamGame.igdb_id), None),
                is_(col(SteamGame.game_id), None),
                is_(SteamGame.ignored, False),
                ~in_(col(SteamGame.id), batch_session.processed_item_ids)  # Exclude already processed
            )
        ).limit(batch_size)
        
        games_to_process = db_session.exec(matched_games_query).all()
        
        if not games_to_process:
            # No more games to process - mark session as complete
            batch_session.status = BatchSessionStatus.COMPLETED if batch_session.status.value != "cancelled" else batch_session.status
            logger.info(f"Batch sync session {session_id} completed - no more games to process")
            return _create_batch_response(batch_session, [], "No more games to process")
        
        # Process the batch using the steam import service
        game_ids = [game.id for game in games_to_process]
        
        logger.info(f"Syncing {len(game_ids)} games in batch for session {session_id}")
        
        # Process games individually through the import service and capture sync results
        successful_count = 0
        failed_count = 0
        errors = []
        sync_results = {}  # Map game_id to sync result for response formatting
        
        for game_id in game_ids:
            try:
                result = await steam_service.sync_game(current_user.id, game_id)
                if result.action in ["created", "updated"]:
                    successful_count += 1
                    # Store the sync result for response formatting
                    sync_results[game_id] = {
                        "user_game_id": result.user_game_id,
                        "action": result.action
                    }
                else:
                    failed_count += 1
                    if result.error_message:
                        errors.append(f"Game {game_id}: {result.error_message}")
            except Exception as e:
                failed_count += 1
                errors.append(f"Game {game_id}: {str(e)}")
        
        # Update session progress
        processed_ids = game_ids
        failed_ids = [game_ids[i] for i in range(len(game_ids)) if i >= successful_count]  # Approximate failed IDs
        
        session_manager.update_session_progress(
            session_id=session_id,
            processed_count=len(games_to_process),
            successful_count=successful_count,
            failed_count=failed_count,
            processed_ids=processed_ids,
            failed_ids=failed_ids,
            errors=errors
        )
        
        # Get updated games data for response
        updated_games_query = select(SteamGame).where(
            col(SteamGame.id).in_(game_ids)
        )
        updated_games = db_session.exec(updated_games_query).all()
        
        # Convert to response format using the original Steam schema field names
        current_batch_items = [
            SteamGameResponse.model_validate({
                "id": game.id,
                "steam_appid": game.steam_appid,  # Keep original field name for schema compatibility
                "game_name": game.game_name,  # Keep original field name for schema compatibility  
                "igdb_id": game.igdb_id,
                "igdb_title": game.igdb_title,
                "game_id": game.game_id,
                "user_game_id": sync_results.get(game.id, {}).get("user_game_id"),
                "ignored": game.ignored,
                "created_at": game.created_at,
                "updated_at": game.updated_at
            })
            for game in updated_games
        ]
        
        message = f"Processed batch of {len(games_to_process)} games: {successful_count} synced, {failed_count} failed"
        
        logger.info(f"Completed sync batch processing for session {session_id}: {message}")
        
        return BatchNextResponse(
            session_id=batch_session.id,
            batch_processed=len(current_batch_items),
            batch_successful=successful_count,
            batch_failed=failed_count,
            batch_errors=errors,
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
            detail="Failed to process batch"
        )


# Common batch session endpoints
@router.get("/{session_id}/status", response_model=BatchStatusResponse, status_code=status.HTTP_200_OK)
async def get_batch_status(
    session_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)]
) -> BatchStatusResponse:
    """
    Get the current status of a batch session.
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
    Cancel a batch session.
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
            message=f"Batch session cancelled. Processed {batch_session.processed_items} games with {batch_session.successful_items} successful operations."
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
    message: str
) -> BatchNextResponse:
    """Helper function to create a consistent batch response."""
    return BatchNextResponse(
        session_id=batch_session.id,
        batch_processed=len(current_batch_items),
        batch_successful=0,
        batch_failed=0,
        batch_errors=[],
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