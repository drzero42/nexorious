# Design: Remove WebSocket Support

## Summary

Remove unused WebSocket infrastructure from the backend. The frontend uses HTTP polling via TanStack Query for job status updates, making the WebSocket implementation dead code.

## Files to Delete

- `backend/app/api/websocket.py` - WebSocket endpoint (`/api/ws/jobs`)
- `backend/app/schemas/websocket.py` - Message type schemas
- `backend/app/tests/test_websocket.py` - WebSocket tests

## Files to Modify

- `backend/app/main.py` - Remove websocket router import and registration

## Implementation Steps

1. Remove websocket router import from `main.py`
2. Remove websocket router registration from `main.py`
3. Delete the three websocket files
4. Run tests to verify nothing breaks

## Rationale

- Frontend uses polling (3-10s intervals depending on context)
- Polling is appropriate for this single-user app with infrequent jobs
- Removes ~600 lines of unused code and tests
- Simplifies codebase maintenance
