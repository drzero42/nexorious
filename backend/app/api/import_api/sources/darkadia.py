"""
Darkadia CSV import endpoints using the new import framework.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query, UploadFile, File, BackgroundTasks
from sqlmodel import Session, select, and_, func
from typing import Annotated, Optional, Dict, Any
import logging
import json
from pathlib import Path
from datetime import datetime, timezone

from ....utils.sqlalchemy_typed import is_, is_not, ilike, desc, label, in_, asc

from ....core.database import get_session
from ....core.security import get_current_user
from ....models.user import User
from ....models.import_job import ImportJob, ImportStatus, ImportType, JobType
from ....models.darkadia_game import DarkadiaGame
from ....models.darkadia_import import DarkadiaImport
from ....models.platform import Platform, Storefront
from ....services.import_sources.darkadia import create_darkadia_import_service
from ....services.platform_resolution import create_platform_resolution_service
from ....schemas.import_schemas import (
    VerificationResponse,
    LibraryPreviewResponse,
    ImportGameResponse,
    ImportStartResponse,
    ImportJobResponse,
    ImportJobsListResponse,
    ImportJobCancelResponse,
    GameMatchRequest,
    GameMatchResponse,
    GameSyncResponse,
    GameIgnoreResponse,
    BulkOperationResponse
)
from ....schemas.darkadia import (
    DarkadiaConfigRequest,
    DarkadiaConfigResponse,
    DarkadiaVerificationRequest,
    DarkadiaUploadResponse,
    DarkadiaResolutionSummary,
    DarkadiaResetResponse,
    DarkadiaResolutionSummaryResponse,
    DarkadiaUpdateMappingsRequest,
    DarkadiaUpdateMappingsResponse,
    DarkadiaGameResponse,
    DarkadiaGamesListResponse,
    DarkadiaPlatformInfo
)
from ....schemas.platform import (
    PendingResolutionsListResponse,
    BulkPlatformResolutionRequest,
    BulkPlatformResolutionResponse
)

router = APIRouter()
logger = logging.getLogger(__name__)


async def _cleanup_temp_file(user_id: str, session: Session) -> None:
    """Clean up temporary CSV file for the user after successful import."""
    try:
        from ....models.user import User
        
        # Get user and their Darkadia configuration
        user = session.get(User, user_id)
        if not user:
            logger.warning(f"User {user_id} not found during cleanup")
            return
        
        preferences = user.preferences or {}
        darkadia_config = preferences.get("darkadia", {})
        csv_file_path = darkadia_config.get("csv_file_path")
        
        if csv_file_path and Path(csv_file_path).exists():
            # Delete the temporary file
            Path(csv_file_path).unlink()
            logger.info(f"Deleted temporary CSV file: {csv_file_path}")
            
            # Clear the file path from user configuration
            darkadia_config.pop("csv_file_path", None)
            darkadia_config.pop("file_hash", None)
            darkadia_config.pop("file_exists", None)
            
            # Update user preferences
            preferences["darkadia"] = darkadia_config
            user.preferences_json = json.dumps(preferences) if preferences else "{}"
            user.updated_at = datetime.now(timezone.utc)
            
            session.add(user)
            session.commit()
            
            logger.info(f"Cleared Darkadia configuration for user {user_id}")
        else:
            logger.debug(f"No temporary file to cleanup for user {user_id}")
            
    except Exception as e:
        logger.error(f"Error during file cleanup for user {user_id}: {str(e)}")
        session.rollback()
        raise


async def process_darkadia_import_background(job_id: str, user_id: str, session: Session):
    """Background task to process Darkadia CSV import with job tracking."""
    darkadia_service = create_darkadia_import_service(session)
    
    try:
        # Get the job
        job = session.get(ImportJob, job_id)
        if not job:
            logger.error(f"Import job {job_id} not found")
            return
        
        # Update job status to running
        job.status = ImportStatus.PROCESSING
        job.started_at = datetime.now(timezone.utc)
        job.progress = 0
        session.add(job)
        session.commit()
        
        logger.info(f"Starting background import for job {job_id}, user {user_id}")
        
        # Create progress callback to update job
        def update_progress(progress_percent: int):
            """Update job progress in database."""
            try:
                job_refresh = session.get(ImportJob, job_id)
                if job_refresh:
                    job_refresh.progress = progress_percent
                    session.add(job_refresh)
                    session.commit()
            except Exception as e:
                logger.error(f"Error updating progress for job {job_id}: {e}")
        
        # Process the import with progress tracking
        result = await darkadia_service.import_library(user_id, progress_callback=update_progress)
        
        # Update job with results
        job.status = ImportStatus.COMPLETED
        job.progress = 100
        job.total_items = result.total_games
        job.processed_items = result.imported_count + result.skipped_count
        job.successful_items = result.imported_count
        job.failed_items = len(result.errors)
        job.set_errors(result.errors)
        job.completed_at = datetime.now(timezone.utc)
        
        session.add(job)
        session.commit()
        
        # Clean up uploaded CSV file after successful import
        try:
            await _cleanup_temp_file(user_id, session)
            logger.info(f"Cleaned up temporary CSV file for user {user_id}")
        except Exception as cleanup_error:
            logger.warning(f"Failed to cleanup temporary file for user {user_id}: {cleanup_error}")
            # Don't fail the job if cleanup fails
        
        logger.info(f"Completed import job {job_id}: {result.imported_count} imported, {result.skipped_count} skipped")
        
    except Exception as e:
        logger.error(f"Import job {job_id} failed: {str(e)}")
        
        # Update job with error
        job = session.get(ImportJob, job_id)
        if job:
            job.status = ImportStatus.FAILED
            job.error_message = str(e)
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()


def get_darkadia_service(session: Annotated[Session, Depends(get_session)]):
    """Dependency to get Darkadia import service."""
    return create_darkadia_import_service(session)


@router.get("/availability")
async def get_darkadia_availability(
    current_user: Annotated[User, Depends(get_current_user)]
) -> Dict[str, Any]:
    """
    Check if Darkadia import feature is available for the current user.
    
    Returns simple boolean response indicating availability.
    """
    try:
        logger.info(f"Checking Darkadia availability for user {current_user.id}")
        
        # Darkadia CSV import is always available (no special requirements)
        logger.info(f"Darkadia is available for user {current_user.id}")
        return {
            "available": True,
            "reason": None
        }
    except Exception as e:
        logger.error(f"Unexpected error checking Darkadia availability for user {current_user.id}: {str(e)}")
        return {
            "available": False,
            "reason": "Internal error checking Darkadia availability"
        }


@router.get("/status")
async def get_darkadia_status(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> Dict[str, Any]:
    """Get Darkadia import source status."""
    try:
        config = await darkadia_service.get_config(current_user.id)
        return {
            "available": True,
            "configured": config.is_configured,
            "verified": config.is_verified,
            "last_configured": config.configured_at,
            "last_import": config.last_import
        }
    except Exception as e:
        logger.error(f"Error getting Darkadia status for user {current_user.id}: {str(e)}")
        return {
            "available": True,
            "configured": False,
            "verified": False,
            "last_configured": None,
            "last_import": None
        }


# Configuration endpoints
@router.get("/config", response_model=DarkadiaConfigResponse)
async def get_darkadia_config(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> DarkadiaConfigResponse:
    """Get current Darkadia configuration."""
    try:
        config = await darkadia_service.get_config(current_user.id)
        
        return DarkadiaConfigResponse(
            has_csv_file=config.config_data.get("has_csv_file", False),
            csv_file_path=config.config_data.get("csv_file_path"),
            file_exists=config.config_data.get("file_exists", False),
            file_hash=config.config_data.get("file_hash"),
            configured_at=config.configured_at
        )
    except Exception as e:
        logger.error(f"Error getting Darkadia config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve Darkadia configuration"
        )


@router.post("/upload", response_model=DarkadiaUploadResponse)
async def upload_darkadia_csv(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service),
    file: UploadFile = File(...)
) -> DarkadiaUploadResponse:
    """Upload and validate Darkadia CSV file."""
    try:
        # Validate file
        if not file.filename or not file.filename.endswith('.csv'):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="File must be a CSV file with .csv extension"
            )
        
        # Read file content
        content = await file.read()
        
        # Create a temporary file to store the upload
        import hashlib
        import time
        from ....core.config import settings
        
        file_hash = hashlib.sha256(content).hexdigest()[:16]
        timestamp = int(time.time())
        safe_filename = f"darkadia_{current_user.id}_{timestamp}_{file_hash}.csv"
        
        # Create temp directory if it doesn't exist (using configurable path)
        temp_dir = Path(settings.temp_storage_dir)
        temp_dir.mkdir(parents=True, exist_ok=True)
        temp_file = temp_dir / safe_filename
        
        # Write file
        temp_file.write_bytes(content)
        temp_file.chmod(0o600)  # Read/write for owner only
        
        # Set configuration with temp file path
        await darkadia_service.set_config(current_user.id, {
            "csv_file_path": str(temp_file)
        })
        
        # Get preview
        preview = await darkadia_service.get_library_preview(current_user.id)
        
        return DarkadiaUploadResponse(
            message="CSV file uploaded and validated successfully",
            file_id=file_hash,
            total_games=preview.get("total_games_estimate", 0),
            file_path=str(temp_file),
            file_size=len(content),
            preview_games=preview.get("preview_games", [])
        )
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error uploading CSV file for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to upload CSV file: {str(e)}"
        )


@router.put("/config", response_model=DarkadiaConfigResponse)
async def update_darkadia_config(
    request: DarkadiaConfigRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> DarkadiaConfigResponse:
    """Update Darkadia configuration."""
    try:
        config = await darkadia_service.set_config(current_user.id, {
            "csv_file_path": request.csv_file_path
        })
        
        return DarkadiaConfigResponse(
            has_csv_file=config.config_data.get("has_csv_file", False),
            csv_file_path=config.config_data.get("csv_file_path"),
            file_exists=config.config_data.get("file_exists", False),
            file_hash=config.config_data.get("file_hash"),
            configured_at=config.configured_at
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error updating Darkadia config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to update Darkadia configuration"
        )


@router.delete("/config")
async def delete_darkadia_config(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> Dict[str, str]:
    """Delete Darkadia configuration."""
    try:
        await darkadia_service.delete_config(current_user.id)
        return {"message": "Darkadia configuration deleted successfully"}
    except Exception as e:
        logger.error(f"Error deleting Darkadia config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to delete Darkadia configuration"
        )


@router.post("/verify", response_model=VerificationResponse)
async def verify_darkadia_config(
    request: DarkadiaVerificationRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> VerificationResponse:
    """Verify CSV file configuration."""
    try:
        is_valid, error_message, verification_data = await darkadia_service.verify_config({
            "csv_file_path": request.csv_file_path
        })
        
        return VerificationResponse(
            is_valid=is_valid,
            error_message=error_message,
            additional_data=verification_data or {}
        )
    except Exception as e:
        logger.error(f"Error verifying Darkadia config for user {current_user.id}: {str(e)}")
        return VerificationResponse(
            is_valid=False,
            error_message=f"Verification failed: {str(e)}",
            additional_data={}
        )


@router.get("/preview", response_model=LibraryPreviewResponse)
async def get_darkadia_library_preview(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> LibraryPreviewResponse:
    """Get preview of CSV data."""
    try:
        preview = await darkadia_service.get_library_preview(current_user.id)
        
        return LibraryPreviewResponse(
            total_games=preview.get("total_games_estimate", 0),
            preview_games=preview.get("preview_games", []),
            source_info=preview.get("file_info", {})
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error getting library preview for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get library preview"
        )


@router.post("/import", response_model=ImportStartResponse)
async def trigger_darkadia_import(
    background_tasks: BackgroundTasks,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    darkadia_service = Depends(get_darkadia_service)
) -> ImportStartResponse:
    """Trigger Darkadia CSV import to staging table with background processing."""
    try:
        # Validate configuration before creating job
        config = await darkadia_service.get_config(current_user.id)
        if not config.is_configured or not config.is_verified:
            raise ValueError("Darkadia CSV file not configured or verified")
        
        # Create import job
        job = ImportJob(
            user_id=current_user.id,
            import_type=ImportType.DARKADIA,
            job_type=JobType.LIBRARY_IMPORT,
            source="darkadia",
            status=ImportStatus.PENDING,
            total_items=0,  # Will be updated when processing starts
            progress=0
        )
        
        session.add(job)
        session.commit()
        session.refresh(job)
        
        # Start background processing
        background_tasks.add_task(
            process_darkadia_import_background,
            job.id,
            current_user.id,
            session
        )
        
        logger.info(f"Started Darkadia import job {job.id} for user {current_user.id}")
        
        return ImportStartResponse(
            message="Import started successfully. Check job status for progress.",
            job_id=job.id,
            started=True
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error starting import for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to start import"
        )


@router.get("/games", response_model=DarkadiaGamesListResponse)
async def list_darkadia_games(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    offset: int = Query(default=0, ge=0),
    limit: int = Query(default=100, ge=1),
    status_filter: Optional[str] = Query(default=None),
    search: Optional[str] = Query(default=None)
) -> DarkadiaGamesListResponse:
    """List imported games with multi-platform support and filtering."""
    try:
        # Build base query for DarkadiaGame
        base_query = select(DarkadiaGame).where(DarkadiaGame.user_id == current_user.id)
        
        # Apply status filtering
        if status_filter:
            if status_filter == "unmatched":
                base_query = base_query.where(
                    and_(is_(DarkadiaGame.igdb_id, None), not DarkadiaGame.ignored)
                )
            elif status_filter == "matched":
                base_query = base_query.where(
                    and_(
                        is_not(DarkadiaGame.igdb_id, None),
                        is_(DarkadiaGame.game_id, None),
                        not DarkadiaGame.ignored
                    )
                )
            elif status_filter == "synced":
                base_query = base_query.where(is_not(DarkadiaGame.game_id, None))
            elif status_filter == "ignored":
                base_query = base_query.where(DarkadiaGame.ignored)

        # Apply search filtering
        if search and search.strip():
            search_term = f"%{search.strip()}%"
            base_query = base_query.where(
                ilike(DarkadiaGame.game_name, search_term)
            )
        
        # Get total count for pagination
        # Use direct count query to avoid Cartesian product from subquery
        count_query = select(func.count('*')).where(DarkadiaGame.user_id == current_user.id)
        
        # Apply the same filtering as base_query
        if status_filter:
            if status_filter == "unmatched":
                count_query = count_query.where(
                    and_(is_(DarkadiaGame.igdb_id, None), not DarkadiaGame.ignored)
                )
            elif status_filter == "matched":
                count_query = count_query.where(
                    and_(
                        is_not(DarkadiaGame.igdb_id, None),
                        is_(DarkadiaGame.game_id, None),
                        not DarkadiaGame.ignored
                    )
                )
            elif status_filter == "synced":
                count_query = count_query.where(is_not(DarkadiaGame.game_id, None))
            elif status_filter == "ignored":
                count_query = count_query.where(DarkadiaGame.ignored)

        if search and search.strip():
            search_term = f"%{search.strip()}%"
            count_query = count_query.where(
                ilike(DarkadiaGame.game_name, search_term)
            )
        
        total_count = session.exec(count_query).first() or 0
        logger.debug(f"Total count query for user {current_user.id}: {total_count} games")
        
        # Apply pagination and get games
        games_query = base_query.order_by(desc(DarkadiaGame.created_at)).offset(offset).limit(limit)
        games = session.exec(games_query).all()
        
        # For each game, get associated platform/storefront data from DarkadiaImport
        games_response = []
        for game in games:
            # Query for all imports associated with this game
            imports_query = select(DarkadiaImport).where(
                and_(
                    DarkadiaImport.user_id == current_user.id,
                    DarkadiaImport.game_name == game.game_name  # Games are linked by name
                )
            )
            imports = session.exec(imports_query).all()
            
            # Aggregate platform information
            platforms: list[DarkadiaPlatformInfo] = []
            primary_platform: Optional[DarkadiaPlatformInfo] = None
            
            for imp in imports:
                # Determine resolution status
                platform_status = "pending"
                storefront_status = None
                
                if imp.resolved_platform_id:
                    platform_status = "resolved"
                elif imp.platform_resolved:
                    platform_status = "resolved" 
                    
                if imp.resolved_storefront_id:
                    storefront_status = "resolved"
                elif imp.storefront_resolved:
                    storefront_status = "resolved"
                elif imp.original_storefront_name:
                    storefront_status = "pending"
                
                platform_info = DarkadiaPlatformInfo(
                    original_platform_name=imp.original_platform_name or imp.fallback_platform_name,
                    original_storefront_name=imp.original_storefront_name,
                    resolved_platform_name=None,  # We'll populate this with actual platform names
                    resolved_storefront_name=None,  # We'll populate this with actual storefront names
                    platform_resolution_status=platform_status,
                    storefront_resolution_status=storefront_status,
                    copy_identifier=imp.copy_identifier
                )
                
                # Get actual resolved names if available
                if imp.resolved_platform_id:
                    platform = session.get(Platform, imp.resolved_platform_id)
                    if platform:
                        platform_info.resolved_platform_name = platform.display_name
                        
                if imp.resolved_storefront_id:
                    storefront = session.get(Storefront, imp.resolved_storefront_id)
                    if storefront:
                        platform_info.resolved_storefront_name = storefront.display_name
                
                platforms.append(platform_info)
                
                # Use first platform as primary for backward compatibility
                if primary_platform is None:
                    primary_platform = platform_info
            
            # If no imports found, create basic entry from game data
            if not platforms:
                platforms = [DarkadiaPlatformInfo(
                    platform_resolution_status="pending",
                    storefront_resolution_status=None
                )]
                primary_platform = platforms[0]
            
            # Create game response
            game_response = DarkadiaGameResponse(
                id=game.id,
                external_id=game.external_id,
                name=game.game_name,
                igdb_id=game.igdb_id,
                igdb_title=game.igdb_title,
                game_id=game.game_id,
                user_game_id=None,  # This would need to be populated if we had the relationship
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at,
                platforms=platforms,
                # Legacy single platform fields for backward compatibility
                platform_resolved=primary_platform.platform_resolution_status == "resolved" if primary_platform else None,
                original_platform_name=primary_platform.original_platform_name if primary_platform else None,
                platform_resolution_status=primary_platform.platform_resolution_status if primary_platform else None,
                platform_name=primary_platform.resolved_platform_name if primary_platform else None,
                original_storefront_name=primary_platform.original_storefront_name if primary_platform else None,
                storefront_resolution_status=primary_platform.storefront_resolution_status if primary_platform else None,
                storefront_name=primary_platform.resolved_storefront_name if primary_platform else None
            )
            
            games_response.append(game_response)
        
        return DarkadiaGamesListResponse(
            total=total_count,
            games=games_response
        )
        
    except Exception as e:
        logger.error(f"Error listing games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to list games"
        )


@router.post("/games/{game_id}/match", response_model=GameMatchResponse)
async def match_darkadia_game(
    game_id: str,
    request: GameMatchRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> GameMatchResponse:
    """Manually match game to IGDB."""
    try:
        game = await darkadia_service.match_game(current_user.id, game_id, request.igdb_id)
        
        return GameMatchResponse(
            message="Game matched successfully" if request.igdb_id else "Game match cleared",
            game=ImportGameResponse(
                id=game.id,
                external_id=game.external_id,
                name=game.name,
                igdb_id=game.igdb_id,
                igdb_title=game.igdb_title,
                game_id=game.game_id,
                user_game_id=game.user_game_id,
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at,
                platform_resolved=game.platform_resolved,
                original_platform_name=game.original_platform_name,
                platform_resolution_status=game.platform_resolution_status,
                platform_name=game.platform_name,
                original_storefront_name=game.original_storefront_name,
                storefront_resolution_status=game.storefront_resolution_status,
                storefront_name=game.storefront_name
            )
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error matching Darkadia game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to match game"
        )


@router.post("/games/{game_id}/auto-match", response_model=GameMatchResponse)
async def auto_match_darkadia_game(
    game_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> GameMatchResponse:
    """Automatically match game to IGDB."""
    try:
        result = await darkadia_service.auto_match_game(current_user.id, game_id)
        
        if result.matched:
            # Get updated game info
            games, _ = await darkadia_service.list_games(
                user_id=current_user.id,
                offset=0,
                limit=1,
                status_filter=None,
                search=None
            )
            game = next((g for g in games if g.id == game_id), None)
            
            if game:
                return GameMatchResponse(
                    message=f"Game auto-matched successfully with confidence {result.confidence_score:.2f}",
                    game=ImportGameResponse(
                        id=game.id,
                        external_id=game.external_id,
                        name=game.name,
                        igdb_id=game.igdb_id,
                        igdb_title=game.igdb_title,
                        game_id=game.game_id,
                        user_game_id=game.user_game_id,
                        ignored=game.ignored,
                        created_at=game.created_at,
                        updated_at=game.updated_at,
                        platform_resolved=game.platform_resolved,
                        original_platform_name=game.original_platform_name,
                        platform_resolution_status=game.platform_resolution_status,
                        platform_name=game.platform_name,
                        original_storefront_name=game.original_storefront_name,
                        storefront_resolution_status=game.storefront_resolution_status,
                        storefront_name=game.storefront_name
                    )
                )
        
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=result.error_message or "Auto-matching failed"
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error auto-matching Darkadia game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to auto-match game"
        )


@router.post("/games/auto-match-all", response_model=BulkOperationResponse)
async def auto_match_all_darkadia_games(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> BulkOperationResponse:
    """Auto-match all unmatched games."""
    try:
        result = await darkadia_service.auto_match_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Auto-matched {result.successful_operations}/{result.total_processed} games",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            errors=result.errors
        )
    except Exception as e:
        logger.error(f"Error auto-matching all games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to auto-match games"
        )


@router.post("/games/{game_id}/sync", response_model=GameSyncResponse)
async def sync_darkadia_game(
    game_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> GameSyncResponse:
    """Sync game to main collection."""
    try:
        result = await darkadia_service.sync_game(current_user.id, game_id)
        
        if result.action == "failed":
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=result.error_message or "Failed to sync game"
            )
        
        # Get updated game info
        games, _ = await darkadia_service.list_games(
            user_id=current_user.id,
            offset=0,
            limit=1,
            status_filter=None,
            search=None
        )
        game = next((g for g in games if g.id == game_id), None)
        
        if not game:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Game not found after sync"
            )
        
        return GameSyncResponse(
            message=f"Game {result.action.replace('_', ' ')} successfully",
            game=ImportGameResponse(
                id=game.id,
                external_id=game.external_id,
                name=game.name,
                igdb_id=game.igdb_id,
                igdb_title=game.igdb_title,
                game_id=game.game_id,
                user_game_id=game.user_game_id,
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at,
                platform_resolved=game.platform_resolved,
                original_platform_name=game.original_platform_name,
                platform_resolution_status=game.platform_resolution_status,
                platform_name=game.platform_name,
                original_storefront_name=game.original_storefront_name,
                storefront_resolution_status=game.storefront_resolution_status,
                storefront_name=game.storefront_name
            ),
            user_game_id=result.user_game_id,
            action=result.action
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error syncing Darkadia game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to sync game"
        )


@router.post("/games/sync-all", response_model=BulkOperationResponse)
async def sync_all_darkadia_games(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> BulkOperationResponse:
    """Sync all matched games to collection."""
    try:
        result = await darkadia_service.sync_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Synced {result.successful_operations}/{result.total_processed} games",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            errors=result.errors
        )
    except Exception as e:
        logger.error(f"Error syncing all games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to sync games"
        )


@router.post("/games/{game_id}/ignore", response_model=GameIgnoreResponse)
async def ignore_darkadia_game(
    game_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> GameIgnoreResponse:
    """Toggle ignore status of game."""
    try:
        game = await darkadia_service.ignore_game(current_user.id, game_id)
        
        return GameIgnoreResponse(
            message=f"Game {'ignored' if game.ignored else 'unignored'} successfully",
            game=ImportGameResponse(
                id=game.id,
                external_id=game.external_id,
                name=game.name,
                igdb_id=game.igdb_id,
                igdb_title=game.igdb_title,
                game_id=game.game_id,
                user_game_id=game.user_game_id,
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at,
                platform_resolved=game.platform_resolved,
                original_platform_name=game.original_platform_name,
                platform_resolution_status=game.platform_resolution_status,
                platform_name=game.platform_name,
                original_storefront_name=game.original_storefront_name,
                storefront_resolution_status=game.storefront_resolution_status,
                storefront_name=game.storefront_name
            ),
            ignored=game.ignored
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error ignoring Darkadia game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to ignore game"
        )


@router.post("/games/unignore-all", response_model=BulkOperationResponse)
async def unignore_all_darkadia_games(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> BulkOperationResponse:
    """Unignore all ignored games."""
    try:
        result = await darkadia_service.unignore_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Unignored {result.successful_operations} games",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            errors=result.errors
        )
    except Exception as e:
        logger.error(f"Error unignoring all games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to unignore games"
        )


@router.post("/games/unmatch-all", response_model=BulkOperationResponse)
async def unmatch_all_darkadia_games(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> BulkOperationResponse:
    """Remove IGDB matches from all games."""
    try:
        result = await darkadia_service.unmatch_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Unmatched {result.successful_operations} games",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            errors=result.errors
        )
    except Exception as e:
        logger.error(f"Error unmatching all games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to unmatch games"
        )


# Individual game platform management endpoints
@router.get("/games/{game_id}/platforms")
async def get_game_platform_options(
    game_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> Dict[str, Any]:
    """Get available platform/storefront options for a specific game."""
    try:
        options = await darkadia_service.get_game_platform_options(current_user.id, game_id)
        return options
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error getting platform options for game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get platform options"
        )


@router.post("/games/{game_id}/platforms")
async def update_game_platform(
    game_id: str,
    request: Dict[str, Any],
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> Dict[str, Any]:
    """Update platform/storefront for a specific game copy."""
    try:
        # Extract required fields from request
        copy_identifier = request.get("copy_identifier")
        if not copy_identifier:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="copy_identifier is required"
            )
            
        platform_id = request.get("platform_id")
        storefront_id = request.get("storefront_id")
        
        result = await darkadia_service.update_game_platform(
            user_id=current_user.id,
            game_id=game_id,
            copy_identifier=copy_identifier,
            platform_id=platform_id,
            storefront_id=storefront_id
        )
        
        return result
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error updating platform for game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to update game platform"
        )


# Job status and tracking endpoints
@router.get("/jobs", response_model=ImportJobsListResponse)
async def list_darkadia_jobs(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    offset: int = Query(default=0, ge=0),
    limit: int = Query(default=100, ge=1),
    job_status: Optional[str] = Query(default=None, alias="status")
) -> ImportJobsListResponse:
    """List Darkadia import jobs with filtering and pagination."""
    try:
        # Build query
        from sqlmodel import select, desc
        query = select(ImportJob).where(
            ImportJob.user_id == current_user.id,
            ImportJob.source == "darkadia"
        )
        
        # Apply status filter
        if job_status:
            query = query.where(ImportJob.status == job_status)
        
        # Get total count
        count_query = select(ImportJob).where(
            ImportJob.user_id == current_user.id,
            ImportJob.source == "darkadia"
        )
        if job_status:
            count_query = count_query.where(ImportJob.status == job_status)
        
        total = len(session.exec(count_query).all())
        
        # Apply pagination and ordering
        query = query.order_by(desc(ImportJob.created_at)).offset(offset).limit(limit)
        jobs = session.exec(query).all()
        
        return ImportJobsListResponse(
            jobs=[
                ImportJobResponse(
                    id=job.id,
                    source=job.source or "darkadia",
                    job_type=job.job_type.value if job.job_type else "library_import",
                    status=job.status.value,
                    progress=job.progress,
                    total_items=job.total_items,
                    processed_items=job.processed_items,
                    successful_items=job.successful_items,
                    failed_items=job.failed_items,
                    created_at=job.created_at,
                    started_at=job.started_at,
                    completed_at=job.completed_at,
                    error_message=job.error_message,
                    metadata=job.get_metadata()
                )
                for job in jobs
            ],
            total=total,
            offset=offset,
            limit=limit
        )
    except Exception as e:
        logger.error(f"Error listing jobs for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to list jobs"
        )


@router.get("/jobs/{job_id}", response_model=ImportJobResponse)
async def get_darkadia_job(
    job_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
) -> ImportJobResponse:
    """Get specific Darkadia import job status."""
    try:
        from sqlmodel import select
        
        job = session.exec(
            select(ImportJob).where(
                ImportJob.id == job_id,
                ImportJob.user_id == current_user.id,
                ImportJob.source == "darkadia"
            )
        ).first()
        
        if not job:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Import job not found"
            )
        
        return ImportJobResponse(
            id=job.id,
            source=job.source or "darkadia",
            job_type=job.job_type.value if job.job_type else "library_import",
            status=job.status.value,
            progress=job.progress,
            total_items=job.total_items,
            processed_items=job.processed_items,
            successful_items=job.successful_items,
            failed_items=job.failed_items,
            created_at=job.created_at,
            started_at=job.started_at,
            completed_at=job.completed_at,
            error_message=job.error_message,
            metadata=job.get_metadata()
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting job {job_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get job status"
        )


@router.post("/jobs/{job_id}/cancel", response_model=ImportJobCancelResponse)
async def cancel_darkadia_job(
    job_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
) -> ImportJobCancelResponse:
    """Cancel a running Darkadia import job."""
    try:
        from sqlmodel import select
        
        job = session.exec(
            select(ImportJob).where(
                ImportJob.id == job_id,
                ImportJob.user_id == current_user.id,
                ImportJob.source == "darkadia"
            )
        ).first()
        
        if not job:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Import job not found"
            )
        
        if job.status not in [ImportStatus.PENDING, ImportStatus.PROCESSING]:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=f"Cannot cancel job in status: {job.status}"
            )
        
        # Update job status to cancelled
        job.status = ImportStatus.CANCELLED
        job.completed_at = datetime.now(timezone.utc)
        session.add(job)
        session.commit()
        
        logger.info(f"Cancelled import job {job_id} for user {current_user.id}")
        
        return ImportJobCancelResponse(
            message="Import job cancelled successfully",
            job_id=job.id,
            cancelled=True
        )
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error cancelling job {job_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to cancel job"
        )


# Platform Resolution Endpoints for Darkadia

@router.get("/platform-resolution/summary", response_model=DarkadiaResolutionSummary)
async def get_darkadia_resolution_summary(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
) -> DarkadiaResolutionSummary:
    """
    Get a summary of platform resolution status for user's Darkadia imports.
    
    Provides an overview of pending platform resolutions, affected games,
    and resolution suggestions available for the user.
    """
    try:
        resolution_service = create_platform_resolution_service(session)
        
        # Get pending resolutions
        pending_resolutions, total_pending = await resolution_service.get_pending_resolutions(
            user_id=current_user.id,
            page=1,
            per_page=100  # Get all for summary
        )
        
        # Calculate summary statistics
        total_affected_games = sum(pr.affected_games_count for pr in pending_resolutions)
        
        # Get most common unresolved platforms
        platform_counts = {}
        suggested_count = 0
        
        for pr in pending_resolutions:
            platform_name = pr.original_platform_name
            if platform_name not in platform_counts:
                platform_counts[platform_name] = {
                    "name": platform_name,
                    "affected_games": pr.affected_games_count,
                    "has_suggestions": len(pr.resolution_data.suggestions) > 0
                }
            
            if pr.resolution_data.suggestions:
                suggested_count += 1
        
        # Sort by affected games count
        most_common = sorted(
            platform_counts.values(),
            key=lambda x: x["affected_games"],
            reverse=True
        )[:5]
        
        return DarkadiaResolutionSummary(
            total_pending_resolutions=total_pending,
            total_affected_games=total_affected_games,
            most_common_unresolved=most_common,
            suggested_resolutions_available=suggested_count,
            recent_resolutions=[]  # TODO: Add recent resolutions tracking
        )
        
    except Exception as e:
        logger.error(f"Error getting resolution summary for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get platform resolution summary"
        )


@router.get("/platform-resolution/pending", response_model=PendingResolutionsListResponse)
async def get_pending_darkadia_resolutions(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page")
) -> PendingResolutionsListResponse:
    """
    Get pending platform resolutions for Darkadia imports.
    
    Returns a paginated list of unresolved platforms from the user's
    Darkadia CSV imports that require manual resolution.
    """
    try:
        resolution_service = create_platform_resolution_service(session)
        
        pending_resolutions, total = await resolution_service.get_pending_resolutions(
            user_id=current_user.id,
            page=page,
            per_page=per_page
        )
        
        # Calculate pages
        pages = (total + per_page - 1) // per_page
        
        return PendingResolutionsListResponse(
            pending_resolutions=pending_resolutions,
            total=total,
            page=page,
            per_page=per_page,
            pages=pages
        )
        
    except Exception as e:
        logger.error(f"Error getting pending resolutions for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get pending platform resolutions"
        )


@router.post("/platform-resolution/bulk-resolve", response_model=BulkPlatformResolutionResponse)
async def bulk_resolve_darkadia_platforms(
    request: BulkPlatformResolutionRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
) -> BulkPlatformResolutionResponse:
    """
    Bulk resolve multiple platform mappings for Darkadia imports.
    
    Efficiently handles multiple platform resolutions at once,
    useful when users have many unknown platforms to resolve
    from their CSV imports.
    """
    try:
        resolution_service = create_platform_resolution_service(session)
        
        # Convert request to dict format expected by service
        resolutions_data = []
        for resolution in request.resolutions:
            resolutions_data.append({
                "import_id": resolution.import_id,
                "resolved_platform_id": resolution.resolved_platform_id,
                "resolved_storefront_id": resolution.resolved_storefront_id,
                "user_notes": resolution.user_notes
            })
        
        result = await resolution_service.bulk_resolve_platforms(
            resolutions=resolutions_data,
            user_id=current_user.id
        )
        
        logger.info(f"Bulk resolved {result.successful_resolutions}/{result.total_processed} platforms for user {current_user.id}")
        
        return result
        
    except Exception as e:
        logger.error(f"Error bulk resolving platforms for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to bulk resolve platforms"
        )


@router.delete("/reset", response_model=DarkadiaResetResponse)
async def reset_darkadia_import(
    current_user: Annotated[User, Depends(get_current_user)],
    darkadia_service = Depends(get_darkadia_service)
) -> DarkadiaResetResponse:
    """
    Complete reset of Darkadia import data.
    
    This will:
    1. Remove all synced games from the main collection
    2. Delete all Darkadia staging games
    3. Delete all import tracking records
    4. Clear user configuration
    5. Delete uploaded CSV file
    
    WARNING: This action cannot be undone. All Darkadia import data will be permanently deleted.
    """
    try:
        logger.info(f"🔄 Starting Darkadia import reset for user {current_user.id}")
        
        # Perform the reset
        result = await darkadia_service.reset_import(current_user.id)
        
        logger.info(f"🔄 Darkadia import reset completed for user {current_user.id}: {result}")
        
        return DarkadiaResetResponse(
            message=result["message"],
            deleted_games=result["deleted_games"],
            unsynced_games=result["unsynced_games"],
            deleted_imports=result["deleted_imports"],
            config_cleared=result["config_cleared"],
            file_deleted=result["file_deleted"]
        )
        
    except ValueError as e:
        logger.warning(f"🔄 Reset validation error for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"🔄 Error resetting Darkadia import for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to reset Darkadia import"
        )


# Platform/Storefront Resolution Review Endpoints

@router.get("/resolution-summary", response_model=DarkadiaResolutionSummaryResponse)
async def get_resolution_summary(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
) -> DarkadiaResolutionSummaryResponse:
    """
    Get summary of platform and storefront mappings for user's Darkadia imports.
    
    Returns all original → mapped name pairs along with count of affected games,
    allowing users to see what was auto-matched and make corrections.
    """
    try:
        logger.info(f"🔍 [DEBUG] Starting resolution summary for user {current_user.id}")
        logger.info("🔍 [DEBUG] Attempting imports...")
        
        try:
            from sqlmodel import select, func
            logger.info("🔍 [DEBUG] ✅ SQLModel imports successful")
        except Exception as import_err:
            logger.error(f"🔍 [DEBUG] ❌ SQLModel import failed: {import_err}")
            raise
            
        try:
            from ....models.darkadia_import import DarkadiaImport
            logger.info("🔍 [DEBUG] ✅ DarkadiaImport import successful")
        except Exception as import_err:
            logger.error(f"🔍 [DEBUG] ❌ DarkadiaImport import failed: {import_err}")
            raise
            
        try:
            from ....models.platform import Platform, Storefront
            logger.info("🔍 [DEBUG] ✅ Platform/Storefront imports successful")
        except Exception as import_err:
            logger.error(f"🔍 [DEBUG] ❌ Platform/Storefront import failed: {import_err}")
            raise
            
        
        logger.info("🔍 [DEBUG] All imports successful, proceeding with queries")
        
        # First, let's check what DarkadiaImport records exist
        try:
            logger.info("🔍 [DEBUG] Executing total imports count query...")
            total_imports = session.exec(
                select(func.count('*'))
                .where(DarkadiaImport.user_id == current_user.id)
            ).one()
            logger.info(f"🔍 [DEBUG] ✅ Total DarkadiaImport records for user: {total_imports}")
        except Exception as query_err:
            logger.error(f"🔍 [DEBUG] ❌ Total imports query failed: {query_err}")
            raise
        
        try:
            logger.info("🔍 [DEBUG] Executing imports with platform names query...")
            imports_with_platforms = session.exec(
                select(func.count('*'))
                .where(
                    DarkadiaImport.user_id == current_user.id,
                    is_not(DarkadiaImport.original_platform_name, None)
                )
            ).one()
            logger.info(f"🔍 [DEBUG] ✅ Imports with platform names: {imports_with_platforms}")
        except Exception as query_err:
            logger.error(f"🔍 [DEBUG] ❌ Imports with platform names query failed: {query_err}")
            raise
        
        try:
            logger.info("🔍 [DEBUG] Executing imports with user_game_platform_id query...")
            imports_with_platform_ids = session.exec(
                select(func.count('*'))
                .where(
                    DarkadiaImport.user_id == current_user.id,
                    is_not(DarkadiaImport.user_game_platform_id, None)
                )
            ).one()
            logger.info(f"🔍 [DEBUG] ✅ Imports with user_game_platform_id set: {imports_with_platform_ids}")
        except Exception as query_err:
            logger.error(f"🔍 [DEBUG] ❌ Imports with user_game_platform_id query failed: {query_err}")
            raise
        
        try:
            logger.info("🔍 [DEBUG] Executing imports with resolved_platform_id query...")
            logger.info("🔍 [DEBUG] Checking if DarkadiaImport.resolved_platform_id exists...")
            
            # Check if resolved_platform_id exists on the model
            if not hasattr(DarkadiaImport, 'resolved_platform_id'):
                logger.error("🔍 [DEBUG] ❌ DarkadiaImport.resolved_platform_id field does not exist!")
                logger.info(f"🔍 [DEBUG] Available DarkadiaImport fields: {dir(DarkadiaImport)}")
                raise AttributeError("resolved_platform_id field does not exist on DarkadiaImport model")
            
            imports_with_resolved_platform_ids = session.exec(
                select(func.count('*'))
                .where(
                    DarkadiaImport.user_id == current_user.id,
                    is_not(DarkadiaImport.resolved_platform_id, None)
                )
            ).one()
            logger.info(f"🔍 [DEBUG] ✅ Imports with resolved_platform_id set: {imports_with_resolved_platform_ids}")
        except Exception as query_err:
            logger.error(f"🔍 [DEBUG] ❌ Imports with resolved_platform_id query failed: {query_err}")
            raise
        
        try:
            logger.info("🔍 [DEBUG] Executing imports with resolved_storefront_id query...")
            
            # Check if resolved_storefront_id exists on the model
            if not hasattr(DarkadiaImport, 'resolved_storefront_id'):
                logger.error("🔍 [DEBUG] ❌ DarkadiaImport.resolved_storefront_id field does not exist!")
                raise AttributeError("resolved_storefront_id field does not exist on DarkadiaImport model")
            
            imports_with_resolved_storefront_ids = session.exec(
                select(func.count('*'))
                .where(
                    DarkadiaImport.user_id == current_user.id,
                    is_not(DarkadiaImport.resolved_storefront_id, None)
                )
            ).one()
            logger.info(f"🔍 [DEBUG] ✅ Imports with resolved_storefront_id set: {imports_with_resolved_storefront_ids}")
        except Exception as query_err:
            logger.error(f"🔍 [DEBUG] ❌ Imports with resolved_storefront_id query failed: {query_err}")
            raise
        
        # Get platform mappings - use resolved_platform_id for import-phase records
        # Only include platforms that are successfully resolved (not requiring resolution)
        try:
            logger.info("🔍 [DEBUG] Building platform query...")
            platform_query = (
                select(
                    DarkadiaImport.original_platform_name,
                    label(Platform.name, 'platform_name'),
                    label(func.count('*'), 'game_count')
                )
                .outerjoin(Platform, DarkadiaImport.resolved_platform_id == Platform.id)  # type: ignore[arg-type]
                .where(
                    DarkadiaImport.user_id == current_user.id,
                    is_not(DarkadiaImport.original_platform_name, None),
                    is_not(DarkadiaImport.resolved_platform_id, None)  # Only include resolved platforms
                )
                .group_by(DarkadiaImport.original_platform_name, Platform.name)
            )
            logger.info("🔍 [DEBUG] ✅ Platform query built successfully")
        except Exception as query_err:
            logger.error(f"🔍 [DEBUG] ❌ Platform query building failed: {query_err}")
            raise
        
        try:
            logger.info(f"🔍 [DEBUG] Platform query SQL: {platform_query}")
            logger.info("🔍 [DEBUG] Executing platform query...")
            platform_results = session.exec(platform_query).all()
            logger.info(f"🔍 [DEBUG] ✅ Platform query returned {len(platform_results)} results")
        except Exception as query_err:
            logger.error(f"🔍 [DEBUG] ❌ Platform query execution failed: {query_err}")
            logger.error(f"🔍 [DEBUG] Query was: {platform_query}")
            raise
        
        for i, result in enumerate(platform_results):
            # Row results: [0]=original_platform_name, [1]=platform_name, [2]=game_count
            logger.info(f"🔍 [DEBUG] Platform result {i}: original='{result[0]}', mapped='{result[1]}', count={result[2]}")
        
        # Get storefront mappings
        # Only include storefronts that are successfully resolved (not requiring resolution)
        storefront_query = (
            select(
                DarkadiaImport.original_storefront_name,
                label(Storefront.name, 'storefront_name'),
                label(func.count('*'), 'game_count')
            )
            .outerjoin(Storefront, DarkadiaImport.resolved_storefront_id == Storefront.id)  # type: ignore[arg-type]
            .where(
                DarkadiaImport.user_id == current_user.id,
                is_not(DarkadiaImport.original_storefront_name, None),
                is_(DarkadiaImport.requires_storefront_resolution, False),  # Exclude storefronts requiring resolution
                is_not(DarkadiaImport.resolved_storefront_id, None)  # Only include actually resolved storefronts
            )
            .group_by(DarkadiaImport.original_storefront_name, Storefront.name)
        )
        
        logger.info(f"🔍 [DEBUG] Storefront query SQL: {storefront_query}")
        storefront_results = session.exec(storefront_query).all()
        logger.info(f"🔍 [DEBUG] Storefront query returned {len(storefront_results)} results")
        
        for i, result in enumerate(storefront_results):
            # Row results: [0]=original_storefront_name, [1]=storefront_name, [2]=game_count
            logger.info(f"🔍 [DEBUG] Storefront result {i}: original='{result[0]}', mapped='{result[1]}', count={result[2]}")
        
        # Process platform mappings - only successfully resolved platforms
        platforms = []
        for result in platform_results:
            # Row results: [0]=original_platform_name, [1]=platform_name, [2]=game_count
            platform_entry = {
                "original": result[0],
                "mapped": result[1],  # Will always have a value since we filtered for resolved platforms
                "game_count": result[2]
            }
            platforms.append(platform_entry)
            logger.info(f"🔍 [DEBUG] Processed platform: {platform_entry}")
        
        # Process storefront mappings - only successfully resolved storefronts
        storefronts = []
        for result in storefront_results:
            # Row results: [0]=original_storefront_name, [1]=storefront_name, [2]=game_count
            storefront_entry = {
                "original": result[0],
                "mapped": result[1],  # Will always have a value since we filtered out unresolved storefronts
                "game_count": result[2]
            }
            storefronts.append(storefront_entry)
            logger.info(f"🔍 [DEBUG] Processed storefront: {storefront_entry}")
        
        logger.info(f"🔍 [DEBUG] Final response: {len(platforms)} platforms, {len(storefronts)} storefronts")
        logger.info(f"Retrieved resolution summary for user {current_user.id}: {len(platforms)} platforms, {len(storefronts)} storefronts")
        
        response = DarkadiaResolutionSummaryResponse(
            platforms=platforms,
            storefronts=storefronts
        )
        logger.info(f"🔍 [DEBUG] Response object created: {response}")
        return response
        
    except Exception as e:
        logger.error(f"Error getting resolution summary for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get resolution summary"
        )


@router.post("/update-mappings", response_model=DarkadiaUpdateMappingsResponse)
async def update_mappings(
    request: DarkadiaUpdateMappingsRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
) -> DarkadiaUpdateMappingsResponse:
    """
    Update platform and storefront mappings for user's Darkadia imports.
    
    Allows users to change what platforms/storefronts their original CSV names
    are mapped to, affecting all games that use those original names.
    """
    try:
        from sqlmodel import select
        from ....models.darkadia_import import DarkadiaImport
        from ....models.platform import Platform, Storefront
        from ....models.user_game import UserGamePlatform
        
        updated_mappings = 0
        affected_games = 0
        errors = []
        
        for mapping in request.mappings:
            try:
                if mapping.mapping_type == "platform":
                    # Find the target platform
                    platform = session.exec(
                        select(Platform).where(Platform.name == mapping.new_mapped_name)
                    ).first()
                    
                    if not platform:
                        errors.append(f"Platform '{mapping.new_mapped_name}' not found")
                        continue
                    
                    # Update DarkadiaImport records
                    imports = session.exec(
                        select(DarkadiaImport).where(
                            DarkadiaImport.user_id == current_user.id,
                            DarkadiaImport.original_platform_name == mapping.original_name
                        )
                    ).all()
                    
                    games_updated = 0
                    for import_record in imports:
                        # Update platform resolution data if we have a user_game_platform
                        if import_record.user_game_platform_id:
                            user_platform = session.get(UserGamePlatform, import_record.user_game_platform_id)
                            if user_platform:
                                user_platform.platform_id = platform.id
                                session.add(user_platform)
                                games_updated += 1
                    
                    affected_games += games_updated
                    
                elif mapping.mapping_type == "storefront":
                    # Find the target storefront
                    storefront = session.exec(
                        select(Storefront).where(Storefront.name == mapping.new_mapped_name)
                    ).first()
                    
                    if not storefront:
                        errors.append(f"Storefront '{mapping.new_mapped_name}' not found")
                        continue
                    
                    # Update DarkadiaImport records
                    imports = session.exec(
                        select(DarkadiaImport).where(
                            DarkadiaImport.user_id == current_user.id,
                            DarkadiaImport.original_storefront_name == mapping.original_name
                        )
                    ).all()
                    
                    games_updated = 0
                    for import_record in imports:
                        import_record.resolved_storefront_id = storefront.id
                        session.add(import_record)
                        
                        # Also update user_game_platform storefront if exists
                        if import_record.user_game_platform_id:
                            user_platform = session.get(UserGamePlatform, import_record.user_game_platform_id)
                            if user_platform:
                                user_platform.storefront_id = storefront.id
                                session.add(user_platform)
                        games_updated += 1
                    
                    affected_games += games_updated
                else:
                    errors.append(f"Unknown mapping type: {mapping.mapping_type}")
                    continue
                
                updated_mappings += 1
                
            except Exception as mapping_error:
                logger.error(f"Error updating mapping {mapping.original_name} -> {mapping.new_mapped_name}: {str(mapping_error)}")
                errors.append(f"Failed to update {mapping.original_name}: {str(mapping_error)}")
        
        # Commit all changes
        session.commit()
        
        logger.info(f"Updated {updated_mappings} mappings for user {current_user.id}, affecting {affected_games} games")
        
        return DarkadiaUpdateMappingsResponse(
            message=f"Updated {updated_mappings} mappings, affecting {affected_games} games",
            updated_mappings=updated_mappings,
            affected_games=affected_games,
            errors=errors
        )
        
    except Exception as e:
        logger.error(f"Error updating mappings for user {current_user.id}: {str(e)}")
        session.rollback()
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to update mappings"
        )