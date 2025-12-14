"""Task to clean up expired user sessions.

Runs every 30 minutes to delete sessions that have expired.
This keeps the sessions table clean and improves query performance.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlalchemy import text

from app.worker.broker import broker
from app.worker.queues import QUEUE_LOW
from app.core.database import get_engine

logger = logging.getLogger(__name__)


@broker.task(
    task_name="maintenance.cleanup_expired_sessions",
    schedule=[{"cron": "*/30 * * * *"}],  # Every 30 minutes
    queue=QUEUE_LOW,
)
async def cleanup_expired_sessions() -> Dict[str, Any]:
    """
    Delete expired user sessions from the database.

    Sessions with expires_at in the past are removed.
    This task runs on the low priority queue every 30 minutes.

    Returns:
        Dictionary with cleanup statistics.
    """
    logger.info("Starting cleanup of expired sessions")

    now = datetime.now(timezone.utc)
    deleted_count = 0

    try:
        engine = get_engine()

        with engine.connect() as conn:
            # Check if sessions table exists before attempting cleanup
            result = conn.execute(
                text("""
                    SELECT EXISTS (
                        SELECT FROM information_schema.tables
                        WHERE table_name = 'sessions'
                    )
                """)
            )
            table_exists = result.scalar()

            if not table_exists:
                logger.info("Sessions table does not exist, skipping cleanup")
                return {
                    "status": "skipped",
                    "message": "Sessions table does not exist",
                    "deleted_count": 0,
                }

            # Delete expired sessions
            result = conn.execute(
                text("""
                    DELETE FROM sessions
                    WHERE expires_at < :now
                """),
                {"now": now},
            )
            deleted_count = result.rowcount
            conn.commit()

        if deleted_count > 0:
            logger.info(f"Deleted {deleted_count} expired sessions")
        else:
            logger.debug("No expired sessions to cleanup")

        return {
            "status": "success",
            "deleted_count": deleted_count,
            "cleanup_time": now.isoformat(),
        }

    except Exception as e:
        logger.error(f"Failed to cleanup expired sessions: {e}")
        return {
            "status": "error",
            "error": str(e),
            "deleted_count": deleted_count,
        }
