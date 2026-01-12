"""
Sync configuration endpoints for managing platform sync settings.

Provides endpoints for:
- Getting all sync configurations for a user
- Updating sync settings for a specific platform
- Triggering manual syncs
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, func
from typing import Annotated, Optional
from datetime import datetime, timezone
import json
import logging
import re
import uuid

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.user_sync_config import UserSyncConfig, SyncFrequency as ModelSyncFrequency
from ..models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus, BackgroundJobPriority
from ..models.external_game import ExternalGame
from ..schemas.sync import (
    SyncConfigResponse,
    SyncConfigListResponse,
    SyncConfigUpdateRequest,
    ManualSyncTriggerResponse,
    SyncStatusResponse,
    SyncFrequency,
    SyncPlatform,
    SteamVerifyRequest,
    SteamVerifyResponse,
    EpicAuthStartResponse,
    EpicAuthCompleteRequest,
    EpicAuthCompleteResponse,
    PSNConfigureRequest,
    PSNConfigureResponse,
    PSNStatusResponse,
    EpicAuthCheckResponse,
)
from ..services.steam import SteamService, SteamAPIError, SteamAuthenticationError
from ..services.epic import EpicService, EpicAuthenticationError, EpicAPIError
from ..schemas.ignored_game import (
    IgnoredGameResponse,
    IgnoredGameListResponse,
)
from ..schemas.common import SuccessResponse
from ..worker.queues import enqueue_task

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


def _is_platform_configured(user: User, platform: str) -> bool:
    """Check if a platform has verified credentials configured."""
    preferences = user.preferences or {}

    if platform == "steam":
        steam_config = preferences.get("steam", {})
        return bool(
            steam_config.get("web_api_key")
            and steam_config.get("steam_id")
            and steam_config.get("is_verified", False)
        )
    elif platform == "epic":
        epic_config = preferences.get("epic", {})
        return bool(
            epic_config.get("is_verified", False)
            and epic_config.get("account_id")
        )
    elif platform == "psn":
        psn_config = preferences.get("psn", {})
        return bool(
            psn_config.get("npsso_token")
            and psn_config.get("is_verified", False)
        )
    # Other platforms not yet supported
    return False


def _config_to_response(config: UserSyncConfig, user: User) -> SyncConfigResponse:
    """Convert UserSyncConfig model to response schema."""
    return SyncConfigResponse(
        id=config.id,
        user_id=config.user_id,
        platform=config.platform,
        frequency=FREQUENCY_MODEL_TO_SCHEMA[config.frequency],
        auto_add=config.auto_add,
        last_synced_at=config.last_synced_at,
        created_at=config.created_at,
        updated_at=config.updated_at,
        is_configured=_is_platform_configured(user, config.platform),
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
            configs_response.append(_config_to_response(config_map[platform.value], current_user))
        else:
            # Create a default config for this platform (not persisted yet)
            default_config = UserSyncConfig(
                id=str(uuid.uuid4()),
                user_id=current_user.id,
                platform=platform.value,
                frequency=ModelSyncFrequency.MANUAL,
                auto_add=False,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            )
            configs_response.append(_config_to_response(default_config, current_user))

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
        return _config_to_response(config, current_user)

    # Return default config (not persisted)
    default_config = UserSyncConfig(
        id=str(uuid.uuid4()),
        user_id=current_user.id,
        platform=platform.value,
        frequency=ModelSyncFrequency.MANUAL,
        auto_add=False,
        created_at=datetime.now(timezone.utc),
        updated_at=datetime.now(timezone.utc),
    )
    return _config_to_response(default_config, current_user)


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

    config.updated_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(config)

    logger.info(
        f"Updated sync config for user {current_user.id}, platform {platform}: "
        f"frequency={config.frequency}, auto_add={config.auto_add}"
    )

    return _config_to_response(config, current_user)


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
        priority=BackgroundJobPriority.HIGH,
    )
    session.add(job)
    session.commit()
    session.refresh(job)

    logger.info(
        f"Created sync job {job.id} for user {current_user.id}, platform {platform}"
    )

    # Dispatch the sync dispatch task
    from ..worker.tasks.sync.dispatch import dispatch_sync_items

    await enqueue_task(
        dispatch_sync_items,
        job.id,
        current_user.id,
        platform.value,
        priority=BackgroundJobPriority.HIGH,
    )

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
        SyncPlatform.PSN: BackgroundJobSource.PSN,
    }
    return mapping[platform]


@router.post("/steam/verify", response_model=SteamVerifyResponse)
async def verify_steam_credentials(
    request: SteamVerifyRequest,
    current_user: Annotated[User, Depends(get_current_user)],
) -> SteamVerifyResponse:
    """
    Verify Steam credentials before saving them.

    Validates format and tests the credentials against Steam Web API.
    Returns the Steam username on success for user confirmation.
    """
    logger.info(f"Verifying Steam credentials for user {current_user.id}")

    # Validate Steam ID format (17 digits starting with 7656119)
    if not re.match(r"^7656119\d{10}$", request.steam_id):
        return SteamVerifyResponse(
            valid=False,
            error="invalid_steam_id"
        )

    # Validate API key format (32 alphanumeric characters)
    if not re.match(r"^[A-Fa-f0-9]{32}$", request.web_api_key):
        return SteamVerifyResponse(
            valid=False,
            error="invalid_api_key"
        )

    # Test credentials against Steam API
    try:
        steam_service = SteamService(request.web_api_key)

        # Verify API key is valid
        if not await steam_service.verify_api_key():
            return SteamVerifyResponse(
                valid=False,
                error="invalid_api_key"
            )

        # Get user info to verify Steam ID and get username
        user_info = await steam_service.get_user_info(request.steam_id)

        if not user_info:
            return SteamVerifyResponse(
                valid=False,
                error="invalid_steam_id"
            )

        # Check if profile is public (communityvisibilitystate 3 = public)
        if user_info.community_visibility_state != 3:
            return SteamVerifyResponse(
                valid=False,
                error="private_profile"
            )

        logger.info(
            f"Steam credentials verified for user {current_user.id}: "
            f"Steam username '{user_info.persona_name}'"
        )

        return SteamVerifyResponse(
            valid=True,
            steam_username=user_info.persona_name
        )

    except SteamAuthenticationError:
        return SteamVerifyResponse(
            valid=False,
            error="invalid_api_key"
        )
    except SteamAPIError as e:
        logger.error(f"Steam API error during verification: {str(e)}")
        if "rate limit" in str(e).lower():
            return SteamVerifyResponse(
                valid=False,
                error="rate_limited"
            )
        return SteamVerifyResponse(
            valid=False,
            error="network_error"
        )
    except Exception as e:
        logger.error(f"Unexpected error during Steam verification: {str(e)}")
        return SteamVerifyResponse(
            valid=False,
            error="network_error"
        )


@router.delete("/steam/connection", response_model=SuccessResponse)
async def disconnect_steam(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SuccessResponse:
    """
    Disconnect Steam integration.

    Clears Steam credentials from user preferences and disables sync.
    """
    logger.info(f"Disconnecting Steam for user {current_user.id}")

    # Clear Steam credentials from preferences
    preferences = current_user.preferences or {}
    if "steam" in preferences:
        del preferences["steam"]
        current_user.preferences_json = json.dumps(preferences)

    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()

    logger.info(f"Steam disconnected for user {current_user.id}")

    return SuccessResponse(
        success=True,
        message="Steam disconnected successfully"
    )


@router.post("/epic/auth/start", response_model=EpicAuthStartResponse)
async def start_epic_auth(
    current_user: Annotated[User, Depends(get_current_user)],
) -> EpicAuthStartResponse:
    """
    Start Epic Games authentication flow.

    Returns a URL that the user should visit to authenticate with Epic Games.
    """
    logger.info(f"Starting Epic authentication for user {current_user.id}")

    try:
        epic_service = EpicService(current_user.id)
        auth_url = await epic_service.start_device_auth()

        return EpicAuthStartResponse(
            auth_url=auth_url,
            instructions="Visit the URL above and enter the authorization code to link your Epic Games account.",
        )
    except EpicAPIError as e:
        logger.error(f"Failed to start Epic authentication: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to start Epic authentication: {str(e)}",
        )


@router.post("/epic/auth/complete", response_model=EpicAuthCompleteResponse)
async def complete_epic_auth(
    request: EpicAuthCompleteRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> EpicAuthCompleteResponse:
    """
    Complete Epic Games authentication with authorization code.

    Validates the code with Epic Games and stores the authentication credentials.
    """
    logger.info(f"Completing Epic authentication for user {current_user.id}")

    try:
        epic_service = EpicService(current_user.id, session=session)

        # Complete authentication with the code
        await epic_service.complete_auth(request.code)

        # Get account information
        account_info = await epic_service.get_account_info()

        # Save credentials to database
        epic_service._save_credentials_to_db(session)

        # Update user preferences with Epic credentials
        preferences = current_user.preferences or {}
        preferences["epic"] = {
            "is_verified": True,
            "display_name": account_info.display_name,
            "account_id": account_info.account_id,
        }
        current_user.preferences_json = json.dumps(preferences)
        current_user.updated_at = datetime.now(timezone.utc)

        session.commit()

        logger.info(
            f"Epic authentication completed for user {current_user.id}: "
            f"Epic username '{account_info.display_name}'"
        )

        return EpicAuthCompleteResponse(
            valid=True,
            display_name=account_info.display_name,
        )

    except EpicAuthenticationError as e:
        logger.warning(f"Epic authentication failed for user {current_user.id}: {e}")
        return EpicAuthCompleteResponse(
            valid=False,
            error="invalid_code",
        )
    except EpicAPIError as e:
        logger.error(f"Epic API error during authentication: {e}")
        return EpicAuthCompleteResponse(
            valid=False,
            error="network_error",
        )


@router.get("/epic/auth/check", response_model=EpicAuthCheckResponse)
async def check_epic_auth(
    current_user: Annotated[User, Depends(get_current_user)],
) -> EpicAuthCheckResponse:
    """
    Check Epic Games authentication status.

    Returns whether the user is currently authenticated with Epic Games.
    """
    logger.debug(f"Checking Epic authentication for user {current_user.id}")

    try:
        epic_service = EpicService(current_user.id)
        is_authenticated = await epic_service.verify_auth()

        if is_authenticated:
            account_info = await epic_service.get_account_info()
            return EpicAuthCheckResponse(
                is_authenticated=True,
                display_name=account_info.display_name,
            )
        else:
            return EpicAuthCheckResponse(
                is_authenticated=False,
            )

    except Exception as e:
        logger.warning(f"Error checking Epic authentication: {e}")
        return EpicAuthCheckResponse(
            is_authenticated=False,
        )


@router.delete("/epic/connection", response_model=SuccessResponse)
async def disconnect_epic(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SuccessResponse:
    """
    Disconnect Epic Games integration.

    Clears Epic Games credentials from user preferences and disables sync.
    """
    logger.info(f"Disconnecting Epic for user {current_user.id}")

    try:
        epic_service = EpicService(current_user.id, session=session)
        await epic_service.disconnect()
    except EpicAPIError as e:
        logger.warning(f"Error disconnecting Epic service: {e}")

    # Clear credentials from database
    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == current_user.id,
        UserSyncConfig.platform == "epic"
    )
    config = session.exec(stmt).first()
    if config:
        config.platform_credentials = None
        config.updated_at = datetime.now(timezone.utc)

    # Clear Epic credentials from preferences
    preferences = current_user.preferences or {}
    if "epic" in preferences:
        del preferences["epic"]
        current_user.preferences_json = json.dumps(preferences)

    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()

    logger.info(f"Epic disconnected for user {current_user.id}")

    return SuccessResponse(
        success=True,
        message="Epic Games disconnected successfully",
    )


@router.get("/ignored", response_model=IgnoredGameListResponse)
async def list_ignored_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    source: Optional[BackgroundJobSource] = Query(default=None, description="Filter by source platform"),
    limit: int = Query(default=50, ge=1, le=100, description="Number of items to return"),
    offset: int = Query(default=0, ge=0, description="Number of items to skip"),
) -> IgnoredGameListResponse:
    """
    List all ignored games for the current user.

    Returns games that have been explicitly skipped during sync operations,
    optionally filtered by source platform with pagination support.
    """
    logger.debug(
        f"Listing ignored games for user {current_user.id}, source={source}, "
        f"limit={limit}, offset={offset}"
    )

    # Build query - filter for skipped ExternalGames
    stmt = select(ExternalGame).where(
        ExternalGame.user_id == current_user.id,
        ExternalGame.is_skipped == True,  # noqa: E712
    )

    # Apply source filter if provided (storefront matches source value)
    if source:
        stmt = stmt.where(ExternalGame.storefront == source.value)

    # Apply ordering (newest first)
    # pyrefly: ignore[missing-attribute] - SQLAlchemy column has desc() method
    stmt = stmt.order_by(ExternalGame.created_at.desc())

    # Get total count
    count_stmt = select(func.count()).select_from(ExternalGame).where(
        ExternalGame.user_id == current_user.id,
        ExternalGame.is_skipped == True,  # noqa: E712
    )
    if source:
        count_stmt = count_stmt.where(ExternalGame.storefront == source.value)

    total = session.exec(count_stmt).one()

    # Apply pagination
    stmt = stmt.limit(limit).offset(offset)

    # Execute query
    skipped_games = session.exec(stmt).all()

    # Convert to response models
    items = [
        IgnoredGameResponse(
            id=game.id,
            source=BackgroundJobSource(game.storefront),
            external_id=game.external_id,
            title=game.title,
            created_at=game.created_at,
        )
        for game in skipped_games
    ]

    logger.debug(f"Found {len(items)} ignored games (total: {total})")

    return IgnoredGameListResponse(items=items, total=total)


@router.delete("/ignored/{ignored_id}", response_model=SuccessResponse)
async def unignore_game(
    ignored_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SuccessResponse:
    """
    Remove a game from the ignored list.

    This clears the is_skipped flag, allowing the game to appear in future sync operations.
    The ignored_id must belong to the current user and be a skipped game.
    """
    logger.info(f"Unignoring game {ignored_id} for user {current_user.id}")

    # Find the skipped external game
    stmt = select(ExternalGame).where(
        ExternalGame.id == ignored_id,
        ExternalGame.user_id == current_user.id,
        ExternalGame.is_skipped == True,  # noqa: E712
    )
    skipped_game = session.exec(stmt).first()

    if not skipped_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"Ignored game not found: {ignored_id}",
        )

    # Clear the is_skipped flag instead of deleting
    skipped_game.is_skipped = False
    session.commit()

    logger.info(
        f"Successfully unignored game {ignored_id} ({skipped_game.title}) "
        f"from {skipped_game.storefront} for user {current_user.id}"
    )

    return SuccessResponse(
        success=True,
        message=f"Game '{skipped_game.title}' removed from ignored list",
    )


# ===== PSN Sync Endpoints =====

@router.post("/psn/configure", response_model=PSNConfigureResponse)
async def configure_psn(
    request: PSNConfigureRequest,
    current_user: User = Depends(get_current_user),
    session: Session = Depends(get_session)
):
    """Configure PSN sync by verifying and storing NPSSO token."""
    from app.services.psn import PSNService, PSNAuthenticationError
    from app.schemas.sync import PSNConfigureResponse

    try:
        psn_service = PSNService(npsso_token=request.npsso_token)
        account_info = await psn_service.get_account_info()

        preferences = current_user.preferences or {}
        preferences["psn"] = {
            "npsso_token": request.npsso_token,
            "online_id": account_info.online_id,
            "account_id": account_info.account_id,
            "region": account_info.region,
            "is_verified": True
        }
        current_user.preferences_json = json.dumps(preferences)
        current_user.updated_at = datetime.now(timezone.utc)
        session.commit()

        logger.info(f"PSN configured successfully for user {current_user.id}")

        return PSNConfigureResponse(
            success=True,
            online_id=account_info.online_id,
            account_id=account_info.account_id,
            region=account_info.region,
            message="PSN configured successfully"
        )

    except PSNAuthenticationError as e:
        logger.error(f"PSN authentication failed: {e}")
        raise HTTPException(
            status_code=400,
            detail=f"Invalid NPSSO token: {str(e)}"
        )
    except Exception as e:
        logger.error(f"Error configuring PSN: {e}")
        raise HTTPException(
            status_code=500,
            detail="Failed to configure PSN"
        )


@router.get("/psn/status", response_model=PSNStatusResponse)
async def get_psn_status(
    current_user: User = Depends(get_current_user)
):
    """Get PSN connection status and account information."""
    from app.schemas.sync import PSNStatusResponse

    preferences = current_user.preferences or {}
    psn_config = preferences.get("psn", {})

    return PSNStatusResponse(
        is_configured=psn_config.get("is_verified", False),
        online_id=psn_config.get("online_id"),
        account_id=psn_config.get("account_id"),
        region=psn_config.get("region"),
        token_expired=not psn_config.get("is_verified", False) and "token_expired_at" in psn_config
    )


@router.delete("/psn/disconnect")
async def disconnect_psn(
    current_user: User = Depends(get_current_user),
    session: Session = Depends(get_session)
):
    """Disconnect PSN account by removing stored credentials."""
    preferences = current_user.preferences or {}
    if "psn" in preferences:
        del preferences["psn"]
        current_user.preferences_json = json.dumps(preferences)
        current_user.updated_at = datetime.now(timezone.utc)
        session.commit()
        logger.info(f"PSN disconnected for user {current_user.id}")

    return {"success": True, "message": "PSN disconnected successfully"}
