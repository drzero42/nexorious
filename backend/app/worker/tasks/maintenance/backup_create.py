"""
Manual backup creation task.

Handles on-demand backup creation triggered via API.
"""

import logging
from typing import Any, Dict

from app.services.backup_service import BackupType, backup_service
from app.worker.broker import broker

logger = logging.getLogger(__name__)


@broker.task(task_name="backup.create")
async def create_backup_task(
    backup_type: str = "manual",
) -> Dict[str, Any]:
    """
    Create a backup asynchronously.

    This task is triggered via the API when a user manually
    requests a backup.

    Args:
        backup_type: Type of backup ("manual", "scheduled", "pre_restore").

    Returns:
        Dictionary with backup result.
    """
    logger.info(f"Starting backup creation (type: {backup_type})")

    try:
        # Convert string to enum
        bt = BackupType(backup_type)

        # Create the backup
        backup_id = backup_service.create_backup(backup_type=bt)

        logger.info(f"Backup created successfully: {backup_id}")

        return {
            "status": "success",
            "backup_id": backup_id,
            "backup_type": backup_type,
        }

    except Exception as e:
        logger.error(f"Backup creation failed: {e}", exc_info=True)
        return {
            "status": "error",
            "error": str(e),
        }
