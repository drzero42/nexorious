"""Priority queue configuration for background task processing.

Defines two priority levels for task routing:
- HIGH: User-initiated tasks (manual sync, imports, manual exports)
- LOW: Automated tasks (scheduled syncs, scheduled exports, maintenance)

taskiq-pg uses labels for priority routing. Workers can be configured
to process only tasks with specific labels, enabling priority-based
task distribution.

Usage:
    from app.worker.queues import QUEUE_HIGH, QUEUE_LOW

    # User-initiated task (high priority)
    @broker.task(queue=QUEUE_HIGH)
    async def manual_sync_task(user_id: str) -> dict:
        ...

    # Scheduled task (low priority)
    @broker.task(queue=QUEUE_LOW)
    async def scheduled_sync_task(user_id: str) -> dict:
        ...

    # Dynamic priority based on trigger
    await sync_task.kicker().with_labels(queue=QUEUE_HIGH).kiq(user_id=user_id)

Worker configuration:
    # Process all tasks (default)
    taskiq worker app.worker.broker:broker app.worker.tasks

    # Process only high-priority tasks
    taskiq worker app.worker.broker:broker app.worker.tasks --labels queue=high

    # Process only low-priority tasks
    taskiq worker app.worker.broker:broker app.worker.tasks --labels queue=low
"""

# Queue name constants
QUEUE_HIGH = "high"
QUEUE_LOW = "low"

# Default queue for tasks without explicit priority
QUEUE_DEFAULT = QUEUE_HIGH


def get_queue_for_user_initiated() -> str:
    """Get the queue name for user-initiated tasks.

    User-initiated tasks include:
    - Manual sync triggers ("Sync Now" button)
    - File imports (Darkadia CSV, Nexorious JSON)
    - Manual exports
    - Steam initial import

    Returns:
        Queue name for high-priority processing.
    """
    return QUEUE_HIGH


def get_queue_for_scheduled() -> str:
    """Get the queue name for scheduled/automated tasks.

    Scheduled tasks include:
    - Automatic sync based on user frequency settings
    - Scheduled backup exports (future)
    - Maintenance tasks (cleanup old results, expired exports)
    - Sync scheduler fan-out task

    Returns:
        Queue name for low-priority processing.
    """
    return QUEUE_LOW
