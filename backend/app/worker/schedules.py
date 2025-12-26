"""Taskiq scheduler configuration for scheduled background tasks.

Uses ResilientScheduleSource to define cron-based schedules with connection
resilience - the scheduler pauses during NATS/PostgreSQL outages and
automatically resumes when connections recover.

The scheduler runs as a separate process and triggers tasks at specified intervals.

Usage:
    taskiq scheduler app.worker.schedules:scheduler app.worker.tasks
"""

from taskiq import TaskiqScheduler
from taskiq.schedule_sources import LabelScheduleSource

from app.worker.broker import broker
from app.worker.connection_monitor import ConnectionMonitor, get_connection_monitor


class ResilientScheduleSource(LabelScheduleSource):
    """Schedule source that checks connections before dispatching tasks.

    Wraps LabelScheduleSource to add connection resilience. Before returning
    schedules, it waits for both NATS and PostgreSQL to be available.
    """

    def __init__(self, broker, monitor: ConnectionMonitor | None = None):
        """Initialize the resilient schedule source.

        Args:
            broker: The taskiq broker instance.
            monitor: Optional ConnectionMonitor instance (uses global if not provided).
        """
        super().__init__(broker)
        self._monitor = monitor if monitor is not None else get_connection_monitor()

    async def get_schedules(self):
        """Get schedules after ensuring connections are available.

        Blocks until both NATS and PostgreSQL connections are healthy,
        then returns the schedules from the parent class.
        """
        await self._monitor.wait_for_connections()
        return await super().get_schedules()


# Initialize scheduler with ResilientScheduleSource for connection resilience
scheduler = TaskiqScheduler(
    broker=broker,
    sources=[ResilientScheduleSource(broker)],
)

# Schedule definitions are applied via task decorators using the 'schedule' label.
# See individual task modules for schedule configurations:
#
# Maintenance tasks (app/worker/tasks/maintenance/):
#   - cleanup_task_results: Daily at 3:00 AM UTC
#   - cleanup_expired_sessions: Every 30 minutes
#
# Sync tasks (app/worker/tasks/sync/):
#   - check_pending_syncs: Every 15 minutes
#
# Example task with schedule:
#   @broker.task(
#       schedule=[{"cron": "0 3 * * *"}],  # Daily at 3 AM
#       queue=QUEUE_LOW,
#   )
#   async def cleanup_task_results():
#       ...
