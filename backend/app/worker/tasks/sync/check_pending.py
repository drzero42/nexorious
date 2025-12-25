"""Task to check for pending syncs and dispatch sync tasks.

Runs every 15 minutes to check which user sync configurations
need syncing based on their frequency settings.
"""

import logging
from typing import Dict, Any

from sqlmodel import select, col

from app.worker.broker import broker
from app.worker.queues import QUEUE_LOW
from app.core.database import get_session_context
from app.models.user_sync_config import UserSyncConfig, SyncFrequency
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
)

logger = logging.getLogger(__name__)


@broker.task(
    task_name="sync.check_pending_syncs",
    schedule=[{"cron": "*/15 * * * *"}],  # Every 15 minutes
    queue=QUEUE_LOW,
)
async def check_pending_syncs() -> Dict[str, Any]:
    """
    Check for sync configurations that need syncing and dispatch tasks.

    This is the scheduler fan-out task that:
    1. Queries all enabled, non-manual sync configs
    2. Checks if enough time has passed based on frequency
    3. Creates Job records and dispatches sync tasks

    Returns:
        Dictionary with dispatch statistics.
    """
    logger.info("Checking for pending syncs")

    syncs_dispatched = 0
    platforms: Dict[str, int] = {}

    async with get_session_context() as session:
        # Get all sync configs that might need syncing
        stmt = select(UserSyncConfig).where(
            UserSyncConfig.enabled,  # noqa: E712 - SQLAlchemy boolean column
            UserSyncConfig.frequency != SyncFrequency.MANUAL,
        )
        configs = list(session.exec(stmt).all())
        configs_checked = len(configs)

        logger.info(f"Found {configs_checked} active sync configurations to check")

        for config in configs:
            if config.needs_sync:
                try:
                    dispatched = await _dispatch_sync_for_config(session, config)
                    if dispatched:
                        syncs_dispatched += 1
                        platform = config.platform
                        platforms[platform] = platforms.get(platform, 0) + 1
                        logger.info(
                            f"Dispatched {platform} sync for user {config.user_id}"
                        )
                except Exception as e:
                    logger.error(
                        f"Failed to dispatch sync for config {config.id}: {e}"
                    )

    logger.info(
        f"Pending sync check complete: {syncs_dispatched} syncs dispatched "
        f"out of {configs_checked} configs checked"
    )

    return {
        "configs_checked": configs_checked,
        "syncs_dispatched": syncs_dispatched,
        "platforms": platforms,
    }


async def _dispatch_sync_for_config(session, config: UserSyncConfig) -> bool:
    """
    Dispatch a sync task for a given config.

    Creates a Job record and kicks off the appropriate sync task.

    Returns:
        True if sync was dispatched, False otherwise.
    """
    # Determine the source based on platform
    source_map = {
        "steam": BackgroundJobSource.STEAM,
        "epic": BackgroundJobSource.EPIC,
        "gog": BackgroundJobSource.GOG,
    }

    source = source_map.get(config.platform)
    if not source:
        logger.warning(f"Unknown platform: {config.platform}")
        return False

    # Check for existing pending/processing job for this user+platform
    existing_job_stmt = select(Job).where(
        Job.user_id == config.user_id,
        Job.job_type == BackgroundJobType.SYNC,
        Job.source == source,
        col(Job.status).in_([
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
        ]),
    )
    existing_job = session.exec(existing_job_stmt).first()

    if existing_job:
        logger.debug(
            f"Skipping {config.platform} sync for user {config.user_id}: "
            f"existing job {existing_job.id} is {existing_job.status.value}"
        )
        return False

    # Create job record
    job = Job(
        user_id=config.user_id,
        job_type=BackgroundJobType.SYNC,
        source=source,
        status=BackgroundJobStatus.PENDING,
        priority=BackgroundJobPriority.LOW,  # Scheduled syncs are low priority
    )
    session.add(job)
    session.commit()
    session.refresh(job)

    # Dispatch the appropriate sync task
    if config.platform == "steam":
        from app.worker.tasks.sync.steam import sync_steam_library

        # Kick with low priority for scheduled syncs
        await sync_steam_library.kicker().with_labels(
            queue=QUEUE_LOW
        ).kiq(
            job_id=job.id,
            user_id=config.user_id,
            is_scheduled=True,
        )

        return True

    # Add other platforms here as they're implemented
    # elif config.platform == "epic":
    #     ...

    logger.warning(f"No sync task implemented for platform: {config.platform}")
    return False
