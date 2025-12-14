"""
Sync configuration endpoints for managing platform sync settings.

Provides endpoints for:
- Getting all sync configurations for a user
- Updating sync settings for a specific platform
- Triggering manual syncs
"""

from fastapi import APIRouter, Depends, HTTPException, status
from sqlmodel import Session, select
from typing import Annotated
from datetime import datetime, timezone
import logging
import uuid

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.user_sync_config import UserSyncConfig, SyncFrequency as ModelSyncFrequency
from ..models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus
from ..schemas.sync import (
    SyncConfigResponse,
    SyncConfigListResponse,
    SyncConfigUpdateRequest,
    SyncConfigCreateRequest,
    ManualSyncTriggerResponse,
    SyncStatusResponse,
    SyncFrequency,
    SyncPlatform,
)
from ..worker.queues import QUEUE_HIGH

router = APIRouter(prefix="/sync", tags=["Sync"])
logger = logging.getLogger(__name__)

# Mapping between schema enum and model enum
FREQUENCY_SCHEMA_TO_MODEL = {
    SyncFrequency.MANUAL: ModelSyncFrequency.MANUAL,
    SyncFrequency.HOURLY: ModelSyncFrequency.HOURLY,
    SyncFrequency.DAILY: ModelSyncFrequency.DAILY,
    SyncFrequency.WEEKLY: ModelSyncFrequency.WEEKLY,
}

FREQUENCY_MODEL_TO_SCHEMA = {v: k for k, v in FREQUENCY_SCHEMA_TO_MODEL.items()}


def _config_to_response(config: UserSyncConfig) -> SyncConfigResponse:
    """Convert UserSyncConfig model to response schema."""
    return SyncConfigResponse(
        id=config.id,
        user_id=config.user_id,
        platform=config.platform,
        frequency=FREQUENCY_MODEL_TO_SCHEMA[config.frequency],
        auto_add=config.auto_add,
        enabled=config.enabled,
        last_synced_at=config.last_synced_at,
        created_at=config.created_at,
        updated_at=config.updated_at,
    )


@router.get("/config", response_model=SyncConfigListResponse)
async def get_sync_configs(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SyncConfigListResponse:
    """
    Get all sync configurations for the current user.

    Returns configurations for all platforms the user has configured,
    plus default configurations for platforms not yet configured.
    """
    logger.debug(f"Getting sync configs for user {current_user.id}")

    # Get existing configs
    stmt = select(UserSyncConfig).where(UserSyncConfig.user_id == current_user.id)
    existing_configs = session.exec(stmt).all()

    # Create a map of existing configs by platform
    config_map = {config.platform: config for config in existing_configs}

    # Ensure all supported platforms have a config
    configs_response = []
    for platform in SyncPlatform:
        if platform.value in config_map:
            configs_response.append(_config_to_response(config_map[platform.value]))
        else:
            # Create a default config for this platform (not persisted yet)
            default_config = UserSyncConfig(
                id=str(uuid.uuid4()),
                user_id=current_user.id,
                platform=platform.value,
                frequency=ModelSyncFrequency.MANUAL,
                auto_add=False,
                enabled=True,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )
            configs_response.append(_config_to_response(default_config))

    return SyncConfigListResponse(configs=configs_response, total=len(configs_response))


@router.get("/config/{platform}", response_model=SyncConfigResponse)
async def get_sync_config(
    platform: SyncPlatform,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SyncConfigResponse:
    """
    Get sync configuration for a specific platform.

    Returns the configuration if it exists, or a default configuration
    if the user hasn't configured this platform yet.
    """
    logger.debug(f"Getting sync config for user {current_user.id}, platform {platform}")

    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == current_user.id,
        UserSyncConfig.platform == platform.value,
    )
    config = session.exec(stmt).first()

    if config:
        return _config_to_response(config)

    # Return default config (not persisted)
    default_config = UserSyncConfig(
        id=str(uuid.uuid4()),
        user_id=current_user.id,
        platform=platform.value,
        frequency=ModelSyncFrequency.MANUAL,
        auto_add=False,
        enabled=True,
        created_at=datetime.now(timezone.utc),
        updated_at=datetime.now(timezone.utc),
    )
    return _config_to_response(default_config)


@router.put("/config/{platform}", response_model=SyncConfigResponse)
async def update_sync_config(
    platform: SyncPlatform,
    request: SyncConfigUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SyncConfigResponse:
    """
    Update sync configuration for a specific platform.

    Creates the configuration if it doesn't exist yet.
    Only updates fields that are provided in the request.
    """
    logger.info(
        f"Updating sync config for user {current_user.id}, platform {platform}: {request}"
    )

    # Find or create config
    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == current_user.id,
        UserSyncConfig.platform == platform.value,
    )
    config = session.exec(stmt).first()

    if not config:
        # Create new config
        config = UserSyncConfig(
            user_id=current_user.id,
            platform=platform.value,
        )
        session.add(config)
        logger.info(f"Created new sync config for user {current_user.id}, platform {platform}")

    # Update provided fields
    if request.frequency is not None:
        config.frequency = FREQUENCY_SCHEMA_TO_MODEL[request.frequency]
    if request.auto_add is not None:
        config.auto_add = request.auto_add
    if request.enabled is not None:
        config.enabled = request.enabled

    config.updated_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(config)

    logger.info(
        f"Updated sync config for user {current_user.id}, platform {platform}: "
        f"frequency={config.frequency}, auto_add={config.auto_add}, enabled={config.enabled}"
    )

    return _config_to_response(config)


@router.post("/{platform}", response_model=ManualSyncTriggerResponse)
async def trigger_manual_sync(
    platform: SyncPlatform,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ManualSyncTriggerResponse:
    """
    Trigger a manual sync for a specific platform.

    Creates a high-priority sync job that will be processed immediately.
    Returns the job ID for tracking progress.

    Note: The actual sync task execution is handled by the taskiq worker.
    This endpoint only creates the job record.
    """
    logger.info(f"Manual sync triggered for user {current_user.id}, platform {platform}")

    # Check if there's already an active sync for this platform
    active_job_stmt = select(Job).where(
        Job.user_id == current_user.id,
        Job.job_type == BackgroundJobType.SYNC,
        Job.source == _platform_to_job_source(platform),
        Job.status.in_([  # type: ignore[union-attr]
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.AWAITING_REVIEW,
        ]),
    )
    active_job = session.exec(active_job_stmt).first()

    if active_job:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"A sync is already in progress for {platform.value}. Job ID: {active_job.id}",
        )

    # Create a new job record
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.SYNC,
        source=_platform_to_job_source(platform),
        status=BackgroundJobStatus.PENDING,
        priority=QUEUE_HIGH,
    )
    session.add(job)
    session.commit()
    session.refresh(job)

    logger.info(
        f"Created sync job {job.id} for user {current_user.id}, platform {platform}"
    )

    # TODO: In the future, dispatch the actual taskiq task here
    # For now, we just create the job record
    # await sync_task.kicker().with_labels(queue=QUEUE_HIGH).kiq(job_id=job.id)

    return ManualSyncTriggerResponse(
        message=f"Sync job created for {platform.value}",
        job_id=job.id,
        platform=platform.value,
        status="queued",
    )


@router.get("/{platform}/status", response_model=SyncStatusResponse)
async def get_sync_status(
    platform: SyncPlatform,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SyncStatusResponse:
    """
    Get the current sync status for a platform.

    Returns whether a sync is in progress and the last sync timestamp.
    """
    logger.debug(f"Getting sync status for user {current_user.id}, platform {platform}")

    # Check for active sync job
    active_job_stmt = select(Job).where(
        Job.user_id == current_user.id,
        Job.job_type == BackgroundJobType.SYNC,
        Job.source == _platform_to_job_source(platform),
        Job.status.in_([  # type: ignore[union-attr]
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.AWAITING_REVIEW,
        ]),
    )
    active_job = session.exec(active_job_stmt).first()

    # Get sync config for last_synced_at
    config_stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == current_user.id,
        UserSyncConfig.platform == platform.value,
    )
    config = session.exec(config_stmt).first()

    return SyncStatusResponse(
        platform=platform.value,
        is_syncing=active_job is not None,
        last_synced_at=config.last_synced_at if config else None,
        active_job_id=active_job.id if active_job else None,
    )


def _platform_to_job_source(platform: SyncPlatform) -> BackgroundJobSource:
    """Convert SyncPlatform enum to BackgroundJobSource enum."""
    mapping = {
        SyncPlatform.STEAM: BackgroundJobSource.STEAM,
        SyncPlatform.EPIC: BackgroundJobSource.EPIC,
        SyncPlatform.GOG: BackgroundJobSource.GOG,
    }
    return mapping[platform]
