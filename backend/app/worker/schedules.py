"""Taskiq scheduler configuration for scheduled background tasks.

Uses LabelScheduleSource to define cron-based schedules for:
- Maintenance tasks (cleanup old results, expired sessions)
- Sync scheduler (check for pending syncs)

The scheduler runs as a separate process and triggers tasks at specified intervals.

Usage:
    taskiq scheduler app.worker.schedules:scheduler app.worker.tasks
"""

from taskiq import TaskiqScheduler
from taskiq.schedule_sources import LabelScheduleSource

from app.worker.broker import broker

# Initialize scheduler with LabelScheduleSource
# LabelScheduleSource reads 'schedule' labels from task decorators
scheduler = TaskiqScheduler(
    broker=broker,
    sources=[LabelScheduleSource(broker)],
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
