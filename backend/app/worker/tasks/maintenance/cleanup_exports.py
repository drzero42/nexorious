"""Task to clean up expired export files.

Runs daily at 4:00 AM UTC to delete export files older than 24 hours.
This prevents the storage directory from growing indefinitely.
"""

import logging
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Dict, Any

from app.worker.broker import broker
from app.worker.queues import SUBJECT_LOW_MAINTENANCE
from app.core.config import settings

logger = logging.getLogger(__name__)

# Configuration
EXPORT_RETENTION_HOURS = 24


@broker.task(
    task_name=SUBJECT_LOW_MAINTENANCE,
    schedule=[{"cron": "0 4 * * *"}],  # Daily at 4:00 AM UTC
)
async def cleanup_expired_exports() -> Dict[str, Any]:
    """
    Delete export files older than the retention period.

    Scans the exports directory and removes files older than
    EXPORT_RETENTION_HOURS. This task runs on the low priority queue.

    Returns:
        Dictionary with cleanup statistics.
    """
    logger.info(f"Starting cleanup of export files older than {EXPORT_RETENTION_HOURS} hours")

    cutoff_time = datetime.now(timezone.utc) - timedelta(hours=EXPORT_RETENTION_HOURS)
    deleted_count = 0
    total_bytes_freed = 0
    errors = []

    # Get exports directory from settings or use default
    exports_dir = Path(getattr(settings, "exports_dir", "storage/exports"))

    if not exports_dir.exists():
        logger.info(f"Exports directory does not exist: {exports_dir}")
        return {
            "status": "success",
            "deleted_count": 0,
            "bytes_freed": 0,
            "message": "Exports directory does not exist",
        }

    try:
        # Scan for export files (JSON and CSV exports)
        for file_path in exports_dir.glob("*"):
            if not file_path.is_file():
                continue

            try:
                # Get file modification time
                mtime = datetime.fromtimestamp(
                    file_path.stat().st_mtime, tz=timezone.utc
                )

                if mtime < cutoff_time:
                    file_size = file_path.stat().st_size
                    file_path.unlink()
                    deleted_count += 1
                    total_bytes_freed += file_size
                    logger.debug(f"Deleted expired export: {file_path.name}")

            except OSError as e:
                error_msg = f"Failed to delete {file_path.name}: {e}"
                logger.warning(error_msg)
                errors.append(error_msg)

        logger.info(
            f"Deleted {deleted_count} expired export files, "
            f"freed {total_bytes_freed / 1024:.1f} KB"
        )

        return {
            "status": "success" if not errors else "partial",
            "deleted_count": deleted_count,
            "bytes_freed": total_bytes_freed,
            "cutoff_time": cutoff_time.isoformat(),
            "retention_hours": EXPORT_RETENTION_HOURS,
            "errors": errors if errors else None,
        }

    except Exception as e:
        logger.error(f"Failed to cleanup export files: {e}")
        return {
            "status": "error",
            "error": str(e),
            "deleted_count": deleted_count,
            "bytes_freed": total_bytes_freed,
        }
