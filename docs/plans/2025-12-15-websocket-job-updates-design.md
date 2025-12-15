# WebSocket Job Updates Design

## Overview

Real-time WebSocket endpoint for streaming job status updates to the frontend, eliminating the need for polling HTTP endpoints.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Update mechanism | Database polling | Simple, no external dependencies, can optimize later |
| Authentication | JWT in query parameter | Standard pattern, works with all WebSocket clients |
| Subscription model | Automatic per-user | Watches all active jobs, simpler frontend |
| Message format | Full job payload | Frontend can replace store state directly |
| Bidirectionality | Read-only WebSocket | Reuse existing REST endpoints for commands |

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│                 │   WS    │                 │
│    Frontend     │◄────────│   API Server    │
│                 │         │                 │
└─────────────────┘         └────────┬────────┘
                                     │ poll
                                     ▼
                            ┌─────────────────┐
                            │                 │
                            │   PostgreSQL    │
                            │                 │
                            └─────────────────┘
```

**Connection flow:**
1. Client connects to `GET /api/ws/jobs?token=<JWT>`
2. Server validates token, extracts user ID
3. Server starts polling loop (1 second interval)
4. On job changes, server sends event with full job payload
5. Connection closes on client disconnect

## Endpoint

**URL:** `GET /api/ws/jobs?token=<JWT>`

**Authentication:** JWT access token in query parameter, validated using existing `verify_token()` from `security.py`.

## Message Schema

### Base structure

All messages follow this format:

```json
{
  "event": "<event_type>",
  "timestamp": "2025-12-15T10:30:00Z",
  "job": { /* JobResponse or null */ }
}
```

### Event types

| Event | Trigger | Has job payload |
|-------|---------|-----------------|
| `connected` | Successful connection | No |
| `error` | Authentication or server error | No |
| `job_created` | New job appears | Yes |
| `job_progress` | progress_current changes | Yes |
| `job_status_change` | status field changes | Yes |
| `job_completed` | Job reaches completed status | Yes |
| `job_failed` | Job reaches failed status | Yes |
| `review_item_update` | Review item counts change | Yes |

### Examples

**Connection successful:**
```json
{
  "event": "connected",
  "timestamp": "2025-12-15T10:30:00Z",
  "user_id": "abc-123"
}
```

**Job progress update:**
```json
{
  "event": "job_progress",
  "timestamp": "2025-12-15T10:30:01Z",
  "job": {
    "id": "job-456",
    "user_id": "abc-123",
    "job_type": "import",
    "source": "steam",
    "status": "processing",
    "progress_current": 50,
    "progress_total": 100,
    "progress_percent": 50,
    ...
  }
}
```

**Authentication error:**
```json
{
  "event": "error",
  "timestamp": "2025-12-15T10:30:00Z",
  "message": "Invalid or expired token"
}
```

## Implementation

### Files

| File | Purpose |
|------|---------|
| `app/api/websocket.py` | WebSocket endpoint and polling logic |
| `app/schemas/websocket.py` | Pydantic message schemas |
| `app/tests/test_websocket.py` | Unit and integration tests |

### Change detection

Track state per connection:

```python
@dataclass
class JobSnapshot:
    status: str
    progress_current: int
    progress_total: int
    review_item_count: int
    pending_review_count: int
```

Compare snapshots to detect:
- New job ID → `job_created`
- Status → `completed` → `job_completed`
- Status → `failed` → `job_failed`
- Status changed (other) → `job_status_change`
- Progress changed → `job_progress`
- Review counts changed → `review_item_update`

### Polling query

```sql
SELECT * FROM job
WHERE user_id = :user_id
AND (
  is_terminal = false
  OR completed_at > now() - interval '5 seconds'
)
```

Include recently-completed jobs to ensure completion events are sent.

## Error Handling

### WebSocket close codes

| Code | Meaning |
|------|---------|
| 1000 | Normal closure |
| 1001 | Server going away |
| 4001 | Authentication failed |

### Resilience

- Database error during poll → Log, continue polling
- Serialization error → Log, skip message
- Token validated only on connect (not re-validated)
- Frontend should reconnect when JWT refreshes

## Testing

### Unit tests
- Token validation (valid, expired, invalid, missing)
- Change detection logic
- Message serialization

### Integration tests
- WebSocket connection lifecycle
- Receive events when jobs change in database
- Connection rejected with invalid token

Use short poll interval (0.1s) in tests for speed.

## Future Considerations

- **PostgreSQL LISTEN/NOTIFY**: Could replace polling for lower latency
- **Redis pub/sub**: If horizontal scaling needed
- **Heartbeat**: Add ping/pong for connection health monitoring
