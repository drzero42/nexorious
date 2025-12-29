"""
Manual backup creation task.

Handles on-demand backup creation triggered via API.
Calls back to the API container to perform the actual backup.
"""

import logging
from typing import Any, Dict

import httpx

from app.core.config import settings
from app.worker.broker import broker

logger = logging.getLogger(__name__)


@broker.task(task_name="backup.create")
async def create_backup_task(
    backup_type: str = "manual",
) -> Dict[str, Any]:
    """
    Create a backup asynchronously by calling the API.

    This task is triggered via the API when a user manually
    requests a backup. It calls back to the API's internal
    endpoint to perform the actual backup creation.

    Args:
        backup_type: Type of backup ("manual", "scheduled", "pre_restore").

    Returns:
        Dictionary with backup result.
    """
    logger.info(f"Starting backup creation task (type: {backup_type})")

    try:
        # Call the API's internal backup endpoint
        api_url = f"{settings.internal_api_url}/api/admin/backups/internal/create"

        async with httpx.AsyncClient(timeout=300.0) as client:
            response = await client.post(
                api_url,
                json={"backup_type": backup_type},
                headers={"X-Internal-API-Key": settings.internal_api_key},
            )

            if response.status_code != 200:
                error_msg = f"API returned status {response.status_code}: {response.text}"
                logger.error(error_msg)
                return {
                    "status": "error",
                    "error": error_msg,
                }

            result = response.json()

            if result.get("success"):
                backup_id = result.get("backup_id")
                logger.info(f"Backup created successfully: {backup_id}")
                return {
                    "status": "success",
                    "backup_id": backup_id,
                    "backup_type": backup_type,
                }
            else:
                error_msg = result.get("error", "Unknown error")
                logger.error(f"Backup creation failed: {error_msg}")
                return {
                    "status": "error",
                    "error": error_msg,
                }

    except httpx.TimeoutException:
        logger.error("Backup creation timed out")
        return {
            "status": "error",
            "error": "Backup creation timed out (>300s)",
        }
    except Exception as e:
        logger.error(f"Backup creation failed: {e}", exc_info=True)
        return {
            "status": "error",
            "error": str(e),
        }
