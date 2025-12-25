"""Task to clean up old task results from the database.

Runs daily at 3:00 AM UTC to delete taskiq results older than 7 days.
This prevents the taskiq_results table from growing indefinitely.
"""

import logging
from datetime import datetime, timedelta, timezone
from typing import Dict, Any

from sqlalchemy import text

from app.worker.broker import broker
from app.worker.queues import SUBJECT_LOW_MAINTENANCE
from app.core.database import get_engine

logger = logging.getLogger(__name__)

# Configuration
RESULT_RETENTION_DAYS = 7


@broker.task(
    task_name=SUBJECT_LOW_MAINTENANCE,
    schedule=[{"cron": "0 3 * * *"}],  # Daily at 3:00 AM UTC
)
async def cleanup_task_results() -> Dict[str, Any]:
    """
    Delete task results older than the retention period.

    Cleans up the taskiq_results table to prevent unbounded growth.
    Results older than RESULT_RETENTION_DAYS are removed.

    Returns:
        Dictionary with cleanup statistics.
    """
    logger.info(f"Starting cleanup of task results older than {RESULT_RETENTION_DAYS} days")

    cutoff_date = datetime.now(timezone.utc) - timedelta(days=RESULT_RETENTION_DAYS)
    deleted_count = 0

    try:
        engine = get_engine()

        # Delete old results from taskiq_results table
        # Note: taskiq-pg uses a table named 'taskiq_results' by default
        with engine.connect() as conn:
            result = conn.execute(
                text("""
                    DELETE FROM taskiq_results
                    WHERE created_at < :cutoff_date
                """),
                {"cutoff_date": cutoff_date},
            )
            deleted_count = result.rowcount
            conn.commit()

        logger.info(f"Deleted {deleted_count} old task results")

        return {
            "status": "success",
            "deleted_count": deleted_count,
            "cutoff_date": cutoff_date.isoformat(),
            "retention_days": RESULT_RETENTION_DAYS,
        }

    except Exception as e:
        logger.error(f"Failed to cleanup task results: {e}")
        return {
            "status": "error",
            "error": str(e),
            "deleted_count": deleted_count,
        }
