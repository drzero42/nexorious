"""
Steam import API endpoints for managing background Steam library import jobs.
"""

import json
import logging
from typing import Annotated, Dict, Any
from fastapi import APIRouter, Depends, HTTPException, status, BackgroundTasks
from sqlmodel import Session, select, and_
from datetime import datetime, timezone, timedelta

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.steam_import import SteamImportJob, SteamImportGame, SteamImportJobStatus, SteamImportGameStatus
from ..services.steam_import import SteamImportService, SteamImportProcessingError, create_steam_import_service
from ..services.igdb import IGDBService
# Removed WebSocket imports - now using simple polling
from ..api.dependencies import get_igdb_service_dependency
from ..api.schemas.steam import (
    SteamImportJobCreateRequest,
    SteamImportJobResponse,
    SteamImportJobStatusResponse,
    SteamImportGameResponse,
    SteamImportUserDecisionRequest,
    SteamImportConfirmRequest
)
from ..api.schemas.common import SuccessResponse

router = APIRouter(prefix="/steam/import", tags=["Steam Import"])
logger = logging.getLogger(__name__)


def _get_user_steam_config(user: User) -> dict:
    """Get user's Steam configuration from preferences."""
    try:
        preferences = user.preferences
        return preferences.get("steam", {})
    except (json.JSONDecodeError, TypeError):
        return {}


def _validate_steam_config(steam_config: dict) -> tuple[str, str]:
    """
    Validate Steam configuration and return API key and Steam ID.
    
    Returns:
        Tuple of (api_key, steam_id)
        
    Raises:
        HTTPException if configuration is invalid
    """
    if not steam_config.get("web_api_key"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Steam Web API key not configured. Please configure Steam settings first."
        )
    
    if not steam_config.get("steam_id"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Steam ID not configured. Please configure Steam settings first."
        )
    
    if not steam_config.get("is_verified", False):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Steam configuration is not verified. Please verify Steam settings first."
        )
    
    return steam_config["web_api_key"], steam_config["steam_id"]


def _cleanup_stale_jobs(session: Session, user_id: str, stale_hours: int = 2) -> int:
    """
    Clean up stale Steam import jobs that have been stuck in active states.
    
    Jobs are considered stale if they've been in PENDING, PROCESSING, AWAITING_REVIEW,
    or FINALIZING status for more than the specified number of hours.
    
    Args:
        session: Database session
        user_id: User ID to clean up jobs for
        stale_hours: Number of hours after which jobs are considered stale
        
    Returns:
        Number of jobs cleaned up
    """
    stale_threshold = datetime.now(timezone.utc) - timedelta(hours=stale_hours)
    logger.debug(f"Looking for jobs older than {stale_threshold} for user {user_id}")
    
    # Find stale jobs for this user
    stale_jobs = session.exec(
        select(SteamImportJob).where(
            and_(
                SteamImportJob.user_id == user_id,
                SteamImportJob.status.in_([
                    SteamImportJobStatus.PENDING,
                    SteamImportJobStatus.PROCESSING,
                    SteamImportJobStatus.AWAITING_REVIEW,
                    SteamImportJobStatus.FINALIZING
                ]),
                SteamImportJob.updated_at < stale_threshold
            )
        )
    ).all()
    
    cleaned_count = 0
    for job in stale_jobs:
        logger.debug(f"Cleaning up stale job {job.id} (status: {job.status}, updated: {job.updated_at})")
        job.status = SteamImportJobStatus.FAILED
        job.error_message = f"Job timed out after {stale_hours} hours in {job.status} status"
        job.updated_at = datetime.now(timezone.utc)
        session.add(job)
        cleaned_count += 1
    
    if cleaned_count > 0:
        session.commit()
        logger.info(f"Cleaned up {cleaned_count} stale Steam import jobs for user {user_id}")
    else:
        logger.debug(f"No stale jobs found requiring cleanup for user {user_id}")
    
    return cleaned_count


@router.post("/", response_model=SteamImportJobResponse, status_code=status.HTTP_201_CREATED)
async def create_import_job(
    request: SteamImportJobCreateRequest,
    background_tasks: BackgroundTasks,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """
    Create a new Steam import job and start background processing.
    
    This endpoint creates a new import job and immediately starts background processing
    of the user's Steam library. The job will go through multiple phases:
    1. Retrieve Steam library
    2. Two-phase automatic matching
    3. Await user review for unmatched games
    4. Final import execution
    """
    logger.info(f"Creating Steam import job for user {current_user.username}")
    
    try:
        # Validate Steam configuration
        steam_config = _get_user_steam_config(current_user)
        api_key, steam_id = _validate_steam_config(steam_config)
        
        # Clean up any stale jobs before checking for active ones
        logger.debug(f"Checking for stale jobs for user {current_user.username}")
        cleaned_jobs = _cleanup_stale_jobs(session, current_user.id)
        if cleaned_jobs > 0:
            logger.debug(f"Cleaned up {cleaned_jobs} stale jobs for user {current_user.username}")
        else:
            logger.debug(f"No stale jobs found for user {current_user.username}")
        
        # Check for existing active import jobs
        active_statuses = [
            SteamImportJobStatus.PENDING,
            SteamImportJobStatus.PROCESSING,
            SteamImportJobStatus.AWAITING_REVIEW,
            SteamImportJobStatus.FINALIZING
        ]
        logger.debug(f"Checking for existing active jobs for user {current_user.id} with statuses: {[s.value for s in active_statuses]}")
        
        existing_job = session.exec(
            select(SteamImportJob).where(
                and_(
                    SteamImportJob.user_id == current_user.id,
                    SteamImportJob.status.in_(active_statuses)
                )
            )
        ).first()
        
        if existing_job:
            logger.debug(f"Found existing active job blocking new import:")
            logger.debug(f"  Job ID: {existing_job.id}")
            logger.debug(f"  Status: {existing_job.status}")
            logger.debug(f"  Created: {existing_job.created_at}")
            logger.debug(f"  Updated: {existing_job.updated_at}")
            logger.debug(f"  User ID: {existing_job.user_id}")
            logger.error(f"Returning 409 error - Active import job {existing_job.id} (status: {existing_job.status}) exists for user {current_user.username}")
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail=f"Active import job already exists: {existing_job.id}"
            )
        
        logger.debug(f"No existing active jobs found for user {current_user.username} - proceeding with new job creation")
        
        # Create new import job
        import_job = SteamImportJob(
            user_id=current_user.id,
            status=SteamImportJobStatus.PENDING
        )
        
        session.add(import_job)
        session.commit()
        session.refresh(import_job)
        
        logger.info(f"Created Steam import job {import_job.id} for user {current_user.username}")
        
        # Start background processing
        steam_import_service = create_steam_import_service(session, igdb_service)
        logger.debug(f"Adding background task for Steam import job {import_job.id}")
        background_tasks.add_task(
            steam_import_service.start_import_job,
            import_job.id,
            api_key,
            steam_id
        )
        
        logger.info(f"Started background processing for import job {import_job.id}")
        logger.debug(f"Background task queued successfully for job {import_job.id}")
        
        return SteamImportJobResponse(
            id=import_job.id,
            status=import_job.status,
            total_games=import_job.total_games,
            processed_games=import_job.processed_games,
            matched_games=import_job.matched_games,
            awaiting_review_games=import_job.awaiting_review_games,
            skipped_games=import_job.skipped_games,
            imported_games=import_job.imported_games,
            platform_added_games=import_job.platform_added_games,
            error_message=import_job.error_message,
            created_at=import_job.created_at,
            updated_at=import_job.updated_at,
            completed_at=import_job.completed_at
        )
        
    except HTTPException:
        # Re-raise HTTP exceptions as-is
        raise
    except Exception as e:
        logger.error(f"Error creating Steam import job: {str(e)}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to create import job: {str(e)}"
        )


@router.get("/active", response_model=SteamImportJobResponse | None)
async def get_active_import_job(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """
    Get the current user's active Steam import job.
    
    This endpoint checks if the user has any active import jobs currently running.
    Active jobs are those in PENDING, PROCESSING, AWAITING_REVIEW, or FINALIZING status.
    
    Returns:
        Active import job details if one exists, null otherwise
    """
    logger.debug(f"Getting active import job for user {current_user.username}")
    
    try:
        # Check for existing active import jobs using the same logic as create endpoint
        active_statuses = [
            SteamImportJobStatus.PENDING,
            SteamImportJobStatus.PROCESSING,
            SteamImportJobStatus.AWAITING_REVIEW,
            SteamImportJobStatus.FINALIZING
        ]
        
        active_job = session.exec(
            select(SteamImportJob).where(
                and_(
                    SteamImportJob.user_id == current_user.id,
                    SteamImportJob.status.in_(active_statuses)
                )
            )
        ).first()
        
        if active_job:
            logger.debug(f"Found active import job {active_job.id} with status {active_job.status} for user {current_user.username}")
            return SteamImportJobResponse(
                id=active_job.id,
                status=active_job.status,
                total_games=active_job.total_games,
                processed_games=active_job.processed_games,
                matched_games=active_job.matched_games,
                awaiting_review_games=active_job.awaiting_review_games,
                skipped_games=active_job.skipped_games,
                imported_games=active_job.imported_games,
                platform_added_games=active_job.platform_added_games,
                error_message=active_job.error_message,
                created_at=active_job.created_at,
                updated_at=active_job.updated_at,
                completed_at=active_job.completed_at
            )
        else:
            logger.debug(f"No active import job found for user {current_user.username}")
            return None
            
    except Exception as e:
        logger.error(f"Error getting active import job for user {current_user.username}: {str(e)}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to get active import job: {str(e)}"
        )


@router.get("/{job_id}", response_model=SteamImportJobStatusResponse)
async def get_import_job_status(
    job_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """
    Get the status and progress of a Steam import job.
    
    This endpoint provides real-time status updates for import jobs including:
    - Current job status and phase
    - Progress statistics (total/processed/matched games)
    - Error information if applicable
    - Individual game statuses for review phase
    """
    logger.debug(f"Getting status for import job {job_id}")
    
    try:
        # Get the import job
        job = session.get(SteamImportJob, job_id)
        if not job:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Import job {job_id} not found"
            )
        
        # Verify ownership
        if job.user_id != current_user.id:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Access denied to this import job"
            )
        
        # Get individual game statuses
        games = session.exec(
            select(SteamImportGame).where(SteamImportGame.import_job_id == job_id)
        ).all()
        
        game_responses = []
        for game in games:
            user_decision = None
            if game.user_decision:
                try:
                    user_decision = json.loads(game.user_decision)
                except json.JSONDecodeError:
                    pass
            
            game_responses.append(SteamImportGameResponse(
                id=game.id,
                steam_appid=game.steam_appid,
                steam_name=game.steam_name,
                status=game.status,
                matched_game_id=game.matched_game_id,
                user_decision=user_decision,
                error_message=game.error_message,
                created_at=game.created_at,
                updated_at=game.updated_at
            ))
        
        return SteamImportJobStatusResponse(
            id=job.id,
            status=job.status,
            total_games=job.total_games,
            processed_games=job.processed_games,
            matched_games=job.matched_games,
            awaiting_review_games=job.awaiting_review_games,
            skipped_games=job.skipped_games,
            imported_games=job.imported_games,
            platform_added_games=job.platform_added_games,
            error_message=job.error_message,
            created_at=job.created_at,
            updated_at=job.updated_at,
            completed_at=job.completed_at,
            games=game_responses
        )
        
    except HTTPException:
        # Re-raise HTTP exceptions as-is
        raise
    except Exception as e:
        logger.error(f"Error getting job status for {job_id}: {str(e)}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to get job status: {str(e)}"
        )


@router.put("/{job_id}/decision", response_model=SuccessResponse)
async def submit_user_decisions(
    job_id: str,
    request: SteamImportUserDecisionRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """
    Submit user decisions for games awaiting manual review.
    
    This endpoint allows users to specify how to handle games that couldn't be
    automatically matched during the import process. Users can:
    - Select a specific IGDB game to match
    - Choose to skip the game
    - Provide additional matching information
    """
    logger.info(f"Submitting user decisions for import job {job_id}")
    
    try:
        # Get the import job
        job = session.get(SteamImportJob, job_id)
        if not job:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Import job {job_id} not found"
            )
        
        # Verify ownership
        if job.user_id != current_user.id:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Access denied to this import job"
            )
        
        # Verify job status - with workaround for stuck jobs
        if job.status == SteamImportJobStatus.AWAITING_REVIEW:
            pass  # Status is correct
        elif job.status == SteamImportJobStatus.PROCESSING and job.awaiting_review_games > 0:
            # WORKAROUND: Job is stuck in PROCESSING but has games awaiting review
            logger.warning(f"WORKAROUND - Job {job_id} is stuck in PROCESSING status but has {job.awaiting_review_games} games awaiting review")
            logger.warning(f"Applying workaround - transitioning job to AWAITING_REVIEW")
            
            # Update the job status to AWAITING_REVIEW
            job.status = SteamImportJobStatus.AWAITING_REVIEW
            job.updated_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            session.refresh(job)
            
            logger.info(f"Workaround applied - job {job_id} transitioned from PROCESSING to AWAITING_REVIEW")
        else:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=f"Job {job_id} is not awaiting review (current status: {job.status})"
            )
        
        # Submit decisions using the import service
        steam_import_service = create_steam_import_service(session, igdb_service)
        await steam_import_service.submit_user_decisions(job_id, request.decisions)
        
        logger.info(f"User decisions submitted successfully for job {job_id}")
        
        return SuccessResponse(message="User decisions submitted successfully")
        
    except HTTPException:
        # Re-raise HTTP exceptions as-is
        raise
    except SteamImportProcessingError as e:
        logger.error(f"Steam import processing error: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error submitting user decisions for job {job_id}: {str(e)}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to submit decisions: {str(e)}"
        )


@router.post("/{job_id}/confirm", response_model=SuccessResponse)
async def confirm_final_import(
    job_id: str,
    request: SteamImportConfirmRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """
    Confirm and execute final import of matched games.
    
    This endpoint triggers the final phase of the import process where:
    - New games are imported from IGDB
    - Steam platform is added to existing games
    - Final statistics are calculated
    - Job is marked as completed
    """
    logger.info(f"Confirming final import for job {job_id}")
    
    try:
        # Get the import job
        job = session.get(SteamImportJob, job_id)
        if not job:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Import job {job_id} not found"
            )
        
        # Verify ownership
        if job.user_id != current_user.id:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Access denied to this import job"
            )
        
        # Verify job status
        if job.status != SteamImportJobStatus.FINALIZING:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=f"Job {job_id} is not ready for final import (current status: {job.status})"
            )
        
        # Execute final import using the import service
        steam_import_service = create_steam_import_service(session, igdb_service)
        await steam_import_service.confirm_final_import(job_id)
        
        logger.info(f"Final import confirmed and executed for job {job_id}")
        
        return SuccessResponse(message="Final import executed successfully")
        
    except HTTPException:
        # Re-raise HTTP exceptions as-is
        raise
    except SteamImportProcessingError as e:
        logger.error(f"Steam import processing error: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error confirming final import for job {job_id}: {str(e)}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to confirm final import: {str(e)}"
        )


@router.delete("/{job_id}", response_model=SuccessResponse)
async def cancel_import_job(
    job_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """
    Cancel an active Steam import job.
    
    This endpoint allows users to cancel import jobs that are in progress.
    Cancellation is only allowed for jobs that are not yet completed or failed.
    """
    logger.info(f"Cancelling import job {job_id}")
    
    try:
        # Get the import job
        job = session.get(SteamImportJob, job_id)
        if not job:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Import job {job_id} not found"
            )
        
        # Verify ownership
        if job.user_id != current_user.id:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail="Access denied to this import job"
            )
        
        # Cancel the job using the import service
        steam_import_service = create_steam_import_service(session, igdb_service)
        await steam_import_service.cancel_import_job(job_id)
        
        logger.info(f"Import job {job_id} cancelled successfully")
        
        return SuccessResponse(message="Import job cancelled successfully")
        
    except HTTPException:
        # Re-raise HTTP exceptions as-is
        raise
    except SteamImportProcessingError as e:
        logger.error(f"Steam import processing error: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error cancelling job {job_id}: {str(e)}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to cancel job: {str(e)}"
        )


# WebSocket endpoint removed - now using simple HTTP polling via existing GET /steam/import/{job_id} endpoint