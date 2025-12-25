"""Nexorious item import task (stub for Task 10).

This is a placeholder stub that will be implemented in Task 10.
"""

import logging
from typing import Dict, Any

from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH

logger = logging.getLogger(__name__)


@broker.task(
    task_name="import.nexorious_item",
    queue=QUEUE_HIGH,
)
async def import_nexorious_item(job_id: str) -> Dict[str, Any]:
    """
    Process a single Nexorious import item (game or wishlist).

    This is a stub that will be implemented in Task 10.
    """
    logger.warning(f"import_nexorious_item called but not yet implemented (job: {job_id})")
    return {"status": "not_implemented", "job_id": job_id}
