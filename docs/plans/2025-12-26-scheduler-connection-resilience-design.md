# Scheduler Connection Resilience Design

## Problem

The scheduler crashes or logs noisy errors when NATS or PostgreSQL connections are lost. It should handle these outages gracefully.

## Solution

When NATS or PostgreSQL becomes unavailable, the scheduler pauses, enters a reconnection loop with exponential backoff, and resumes automatically when connections recover.

## Behavior

1. **Detection**: Before dispatching any scheduled task, check NATS and PostgreSQL connectivity
2. **Pause**: On connection failure, stop dispatching tasks and enter reconnection mode
3. **Backoff**: Retry connections with exponential backoff (5s → 10s → 20s → 40s → 60s max)
4. **Logging**: Log warnings on each retry (no full tracebacks), log success on recovery
5. **Resume**: When both connections are healthy, resume normal scheduler operation
6. **Missed tasks**: Skipped during outage - no queuing or replay

## Implementation

### New File: `backend/app/worker/connection_monitor.py`

```python
class ConnectionMonitor:
    """Monitors NATS and PostgreSQL connections with exponential backoff reconnection."""

    def __init__(
        self,
        initial_delay: float = 5.0,
        max_delay: float = 60.0,
        backoff_multiplier: float = 2.0,
    ):
        self.initial_delay = initial_delay
        self.max_delay = max_delay
        self.backoff_multiplier = backoff_multiplier
        self._current_delay = initial_delay

    async def check_nats(self) -> bool:
        """Check if NATS is reachable."""
        ...

    async def check_postgres(self) -> bool:
        """Check if PostgreSQL is reachable."""
        ...

    async def wait_for_connections(self) -> None:
        """Block until both NATS and PostgreSQL are available."""
        # Exponential backoff loop
        ...

    def reset_backoff(self) -> None:
        """Reset backoff delay after successful connection."""
        self._current_delay = self.initial_delay
```

### Modified: `backend/app/worker/schedules.py`

Wrap scheduler startup to use the connection monitor:

```python
from app.worker.connection_monitor import ConnectionMonitor

monitor = ConnectionMonitor()

# Custom scheduler source that checks connections before dispatch
class ResilientScheduleSource(LabelScheduleSource):
    async def get_schedules(self):
        await monitor.wait_for_connections()
        return await super().get_schedules()
```

### Configuration: `backend/app/core/config.py`

Add settings for backoff parameters:

```python
scheduler_reconnect_initial_delay: float = 5.0
scheduler_reconnect_max_delay: float = 60.0
scheduler_reconnect_backoff_multiplier: float = 2.0
```

## Testing

- Unit tests for `ConnectionMonitor` backoff logic
- Integration test simulating NATS/PostgreSQL unavailability
- Manual test: stop NATS container, verify scheduler pauses, restart NATS, verify scheduler resumes

## Files Changed

1. `backend/app/worker/connection_monitor.py` (new)
2. `backend/app/worker/schedules.py` (modified)
3. `backend/app/core/config.py` (modified)
4. `backend/app/worker/tests/test_connection_monitor.py` (new)
