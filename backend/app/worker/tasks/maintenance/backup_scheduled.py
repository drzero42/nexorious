"""
Scheduled backup task.

Runs according to the backup schedule configuration.
"""

import logging
from datetime import datetime, timezone
from typing import Any, Dict

from sqlmodel import Session

from app.core.database import get_engine
from app.models.backup_config import BackupConfig, BackupSchedule
from app.services.backup_service import BackupType, backup_service
from app.worker.broker import broker
from app.worker.queues import SUBJECT_LOW_MAINTENANCE

logger = logging.getLogger(__name__)


@broker.task(
    task_name=SUBJECT_LOW_MAINTENANCE,
    schedule=[{"cron": "0 * * * *"}],  # Check every hour
)
async def check_and_run_backup() -> Dict[str, Any]:
    """
    Check if a scheduled backup should run and execute it.

    This task runs hourly and checks if the backup schedule
    configuration indicates a backup should run now.

    Returns:
        Dictionary with task status and results.
    """
    logger.info("Checking scheduled backup configuration")

    try:
        engine = get_engine()
        with Session(engine) as session:
            config = session.get(BackupConfig, 1)

            if not config:
                logger.debug("No backup configuration found")
                return {"status": "skipped", "reason": "no_config"}

            if config.schedule == BackupSchedule.MANUAL:
                logger.debug("Backup schedule set to manual, skipping")
                return {"status": "skipped", "reason": "manual_schedule"}

            now = datetime.now(timezone.utc)

            # Check if we should run based on schedule
            should_run = False

            if config.schedule == BackupSchedule.DAILY:
                # Check if current hour matches schedule time
                schedule_hour = int(config.schedule_time.split(":")[0])
                if now.hour == schedule_hour:
                    should_run = True

            elif config.schedule == BackupSchedule.WEEKLY:
                # Check if current day and hour match
                schedule_hour = int(config.schedule_time.split(":")[0])
                if config.schedule_day is not None and now.weekday() == config.schedule_day and now.hour == schedule_hour:
                    should_run = True

            if not should_run:
                logger.debug(f"Not time to run backup (schedule: {config.schedule.value})")
                return {"status": "skipped", "reason": "not_scheduled_time"}

            # Run the backup
            logger.info("Running scheduled backup")
            backup_id = backup_service.create_backup(backup_type=BackupType.SCHEDULED)

            # Run retention cleanup
            deleted = backup_service.run_retention_cleanup(
                retention_mode=config.retention_mode.value,
                retention_value=config.retention_value,
            )

            return {
                "status": "success",
                "backup_id": backup_id,
                "retention_deleted": deleted,
                "timestamp": now.isoformat(),
            }

    except Exception as e:
        logger.error(f"Scheduled backup failed: {e}")
        return {
            "status": "error",
            "error": str(e),
        }
