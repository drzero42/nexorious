"""Queue configuration for NATS subject-based routing."""

# Priority subjects for NATS JetStream
# High priority: user-initiated tasks (manual sync, imports, exports)
# Low priority: automated tasks (scheduled syncs, maintenance)

SUBJECT_HIGH = "tasks.high"
SUBJECT_LOW = "tasks.low"

# Legacy compatibility (will be removed after full migration)
QUEUE_HIGH = "high"
QUEUE_LOW = "low"

# Alias for maintenance tasks (scheduled via cron)
SUBJECT_LOW_MAINTENANCE = SUBJECT_LOW


from app.models.job import BackgroundJobPriority


async def enqueue_task(task_func, *args, priority: BackgroundJobPriority):
    """Dispatch task to appropriate priority queue.

    Args:
        task_func: The TaskIQ task function to dispatch
        *args: Arguments to pass to the task
        priority: HIGH for user-initiated, LOW for automated tasks
    """
    subject = SUBJECT_HIGH if priority == BackgroundJobPriority.HIGH else SUBJECT_LOW
    await task_func.kicker().with_labels(subject=subject).kiq(*args)
