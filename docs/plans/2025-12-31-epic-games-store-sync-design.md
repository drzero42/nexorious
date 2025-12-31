# Epic Games Store Sync Integration Design

**Date**: 2025-12-31
**Status**: Design Phase
**Related PRD Section**: 6.1 Enhanced Storefront Integration

## Executive Summary

Add Epic Games Store as a sync source to automatically import user's Epic library into Nexorious collection. Implementation uses the legendary CLI tool (GPL3 licensed) as an external subprocess to maintain MIT license compatibility, following the existing Steam sync adapter pattern.

## Goals

1. Enable users to sync their Epic Games Store library
2. Maintain MIT license compatibility (legendary as external tool only)
3. Follow existing Steam sync architecture patterns
4. Handle Epic OAuth authentication flow
5. Support multi-user isolation on server
6. Handle authentication expiration gracefully

## Non-Goals

- Installing legendary on user's local machine (server-side only)
- Syncing game installation status (requires local legendary)
- Syncing playtime data (Epic API doesn't provide this)
- Supporting platforms other than PC (Epic is PC-only currently)

## Architecture Overview

### Core Approach

Server-side sync using legendary CLI as an external subprocess, following the existing Steam sync pattern with adapter-based architecture.

### Key Components

1. **Epic Sync Adapter** (`app/worker/tasks/sync/adapters/epic.py`) - Implements `SyncSourceAdapter` protocol
2. **Epic Service** (`app/services/epic.py`) - Wraps legendary CLI subprocess calls
3. **Authentication Flow** - Manual OAuth flow via legendary
4. **Credential Storage** - Epic auth verification status in user preferences

### Licensing Strategy

- legendary remains external system dependency (like PostgreSQL)
- Called via subprocess only, no Python imports from legendary code
- Maintains MIT license compatibility for Nexorious
- Document legendary as required system dependency in deployment docs

### Platform/Storefront Mapping

- Platform: `pc-windows` (Epic is PC-only currently)
- Storefront: `epic` (already exists in seed data per PRD)

### Data Flow

```
User clicks "Connect Epic"
  → Backend calls `legendary auth`
  → Auth URL displayed to user
  → User authenticates via Epic OAuth in browser
  → User copies authorization code from Epic response
  → User pastes code into Nexorious UI
  → Backend calls `legendary auth --code <code>`
  → legendary stores auth in isolated config directory
  → Backend verifies auth with `legendary status --json`
  → Backend stores verification in user preferences
  → User triggers sync
  → EpicSyncAdapter.fetch_games()
  → EpicService calls `legendary list --json`
  → Parse JSON output → ExternalGame objects
  → Existing sync pipeline (dispatch → IGDB match → process)
```

## Authentication Implementation

### Epic Service Methods (`app/services/epic.py`)

```python
class EpicService:
    def __init__(self, user_id: str):
        # Set isolated legendary config path per user
        self.config_path = f"/var/lib/nexorious/legendary-configs/{user_id}"
        self.user_id = user_id

    async def start_device_auth() -> str:
        # Run: legendary auth
        # Capture and return authentication URL from stdout
        # legendary will output URL for user to visit

    async def complete_auth(code: str) -> bool:
        # Run: legendary auth --code <code>
        # Complete authentication with user's code
        # Return success/failure

    async def verify_auth() -> bool:
        # Run: legendary status --json
        # Check if authenticated
        # Return True if valid auth exists

    async def get_account_info() -> EpicAccountInfo:
        # Run: legendary status --json
        # Parse account details (display name, account ID)
        # Return structured account info

    async def get_library() -> List[EpicGame]:
        # Run: legendary list --json
        # Parse game list with metadata
        # Return structured game data

    async def disconnect():
        # Run: legendary auth --delete
        # Cleanup config directory
        # Return success
```

### Legendary Command Reference

Based on legendary documentation:

- **Auth start**: `legendary auth`
- **Auth with code**: `legendary auth --code <authorization_code>`
- **Auth delete**: `legendary auth --delete`
- **Status check**: `legendary status --json --offline`
- **List games**: `legendary list --json`
- **Force refresh**: `legendary list --json --force-refresh`

### User Preferences Storage

```json
{
  "epic": {
    "account_id": "...",
    "display_name": "...",
    "is_verified": true,
    "auth_expired_at": null
  }
}
```

### Config Isolation Strategy

Multiple users on server require isolated legendary configurations:

- Use `XDG_CONFIG_HOME` environment variable per subprocess call
- Set to user-specific path: `/var/lib/nexorious/legendary-configs/<user_id>/`
- This makes legendary store its config at `$XDG_CONFIG_HOME/legendary/`
- Each user gets isolated legendary authentication

```python
async def _run_legendary_command(self, user_id: str, args: List[str]):
    env = os.environ.copy()
    env['XDG_CONFIG_HOME'] = f'/var/lib/nexorious/legendary-configs/{user_id}'

    process = await asyncio.create_subprocess_exec(
        'legendary', *args,
        env=env,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE
    )
    # ... handle process execution
```

## API Endpoints

### New Epic Auth Endpoints (`app/api/sync.py`)

```python
POST /sync/epic/auth/start
→ EpicAuthStartResponse
  - auth_url: str
  - instructions: str

POST /sync/epic/auth/complete
← EpicAuthCompleteRequest
  - code: str
→ EpicAuthCompleteResponse
  - valid: bool
  - display_name: Optional[str]
  - error: Optional[str]

GET /sync/epic/auth/check
→ EpicAuthCheckResponse
  - is_authenticated: bool
  - display_name: Optional[str]

DELETE /sync/epic/connection
→ SuccessResponse
  - success: bool
  - message: str
```

### Existing Endpoints (No Changes Needed)

- `POST /sync/epic` - Trigger sync (already generic)
- `GET /sync/epic/status` - Check sync status (already generic)
- `GET /sync/config` - Get all sync configs including Epic
- `PUT /sync/config/epic` - Update Epic sync settings

## Sync Adapter Implementation

### EpicSyncAdapter (`app/worker/tasks/sync/adapters/epic.py`)

```python
class EpicSyncAdapter:
    source = BackgroundJobSource.EPIC

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch games from Epic via legendary CLI."""
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("Epic not configured")

        epic_service = EpicService(user.id)

        try:
            epic_games = await epic_service.get_library()
        except EpicAuthExpiredError:
            # Mark auth as expired in preferences
            self._mark_auth_expired(user)
            raise

        return [
            ExternalGame(
                external_id=game.app_name,
                title=game.title,
                platform="pc-windows",
                storefront="epic",
                metadata={"app_name": game.app_name},
                playtime_hours=0  # Epic doesn't provide playtime
            )
            for game in epic_games
        ]

    def get_credentials(self, user: User) -> Optional[Dict[str, str]]:
        """Check if Epic is authenticated."""
        preferences = user.preferences or {}
        epic_config = preferences.get("epic", {})
        return epic_config if epic_config.get("is_verified") else None

    def is_configured(self, user: User) -> bool:
        """Check if Epic credentials verified."""
        return self.get_credentials(user) is not None
```

### Integration Points

1. **Register adapter** in `get_sync_adapter()` function (`base.py`):
   ```python
   adapters = {
       "steam": SteamSyncAdapter,
       "epic": EpicSyncAdapter,  # ADD THIS
   }
   ```

2. **Enum already exists**: `SyncPlatform.EPIC` in `app/schemas/sync.py`
3. **Job source exists**: `BackgroundJobSource.EPIC` in `app/models/job.py`
4. **No changes needed** to dispatch/process_item tasks - they're generic

### Data Parsing

legendary's `list --json` output structure (example):
```json
[
  {
    "app_name": "Fortnite",
    "app_title": "Fortnite",
    "app_version": "...",
    "metadata": {...}
  }
]
```

Parse `app_name` (unique Epic identifier) and `app_title` (display name), map to `ExternalGame` format.

## Authentication Expiration Handling

### Problem

Epic OAuth tokens expire (typically 8 hours for access tokens, refresh tokens last longer but can also expire). legendary will fail when tokens expire.

### Detection Strategy

**legendary Behavior on Expired Auth**:
- Commands return non-zero exit code
- Stderr contains authentication error messages
- `legendary status` will show not authenticated

**EpicService Error Detection**:
```python
async def _run_legendary_command(self, args: List[str]) -> Dict:
    """Run legendary and detect auth expiration."""
    result = await subprocess_run(...)

    if result.returncode != 0:
        stderr = result.stderr.decode()
        if "not authenticated" in stderr.lower() or \
           "login" in stderr.lower() or \
           "expired" in stderr.lower():
            raise EpicAuthExpiredError("Epic authentication expired")
        raise EpicAPIError(f"legendary command failed: {stderr}")
```

### Job Handling

```python
# In process_item.py or dispatch.py
try:
    games = await adapter.fetch_games(user)
except EpicAuthExpiredError:
    # Mark job as failed with specific error
    job.status = BackgroundJobStatus.FAILED
    job.error_message = "epic_auth_expired"
    job.metadata = {"requires_reauth": True}
    session.commit()
    # Don't retry automatically - requires user action
```

### User Experience

1. User triggers sync
2. Backend detects expired auth during sync
3. Job fails with `epic_auth_expired` error
4. Frontend polls job status, sees auth error
5. Frontend shows "Re-authenticate with Epic" button
6. User goes through auth flow again
7. User can retry sync

### Preferences Update

```python
# When auth expires, mark as unverified
preferences["epic"]["is_verified"] = False
preferences["epic"]["auth_expired_at"] = datetime.now(timezone.utc).isoformat()
```

### API Enhancement

```python
class SyncStatusResponse(BaseModel):
    platform: str
    is_syncing: bool
    last_synced_at: Optional[datetime]
    active_job_id: Optional[str]
    requires_reauth: bool = False  # NEW
    auth_expired: bool = False      # NEW
```

## Error Handling

### Custom Exceptions

```python
class LegendaryNotFoundError(Exception):
    """legendary CLI not found on system."""
    pass

class EpicAuthenticationError(Exception):
    """Epic authentication failed or invalid."""
    pass

class EpicAuthExpiredError(Exception):
    """Epic authentication token expired."""
    pass

class EpicAPIError(Exception):
    """Epic API error or legendary command failed."""
    pass
```

### Error Handling Strategy

1. **legendary not installed**: Fail gracefully, return clear error to user
2. **Auth failed**: Return validation error with instructions
3. **Auth expired**: Mark as expired, prompt user to re-authenticate
4. **Network errors**: Retry with exponential backoff
5. **legendary command timeout**: Kill process after 60 seconds for list, 30s for auth

## Deployment Requirements

### legendary Installation

**Package**: `legendary-gl` (PyPI package name)

**Dockerfile**:
```dockerfile
RUN pip install legendary-gl>=0.20.34
```

**Nix flake** (for development):
```nix
packages = [
  pkgs.legendary-gl
];
```

**Version pinning**: Use latest stable `legendary-gl>=0.20.34`

### Storage Requirements

**Volume mount**: `/var/lib/nexorious/legendary-configs/`
- Permissions: Backend process needs read/write access
- Persistence: legendary configs must survive container restarts
- Cleanup: Consider retention policy for disconnected accounts

**Docker Compose Update**:
```yaml
services:
  api:
    volumes:
      - legendary-configs:/var/lib/nexorious/legendary-configs
volumes:
  legendary-configs:
```

### Environment Variables

No new environment variables needed. Use `XDG_CONFIG_HOME` per subprocess to isolate configs.

### Documentation Updates

- Add legendary to system requirements in README.md
- Document as external dependency like PostgreSQL
- Add troubleshooting section for legendary issues
- Update deployment guides

## Testing Strategy

### Unit Tests

**`app/tests/test_epic_service.py`**:
- Mock subprocess calls to legendary
- Test auth flow: start → complete → verify
- Test library fetching and parsing
- Test error handling (legendary not found, auth failures)
- Test auth expiration detection
- Test config isolation

**`app/tests/test_sync_adapters.py`**:
- Add Epic adapter tests following Steam pattern
- Mock legendary JSON responses
- Verify ExternalGame conversion
- Test credential validation
- Test auth expiration handling

### Integration Tests

**`app/tests/test_sync_api.py`**:
- Epic auth endpoints (start, complete, disconnect, check)
- Epic sync trigger endpoint
- Error cases (not configured, invalid codes, expired auth)
- Multi-user isolation verification

### Test Coverage Goals

- EpicService: >80% coverage
- EpicSyncAdapter: >80% coverage
- API endpoints: 100% coverage (all paths tested)
- Overall backend: Maintain >80% coverage

### Manual Testing Checklist

- [ ] legendary installed and accessible in container
- [ ] Auth flow works end-to-end
- [ ] Library sync pulls Epic games
- [ ] IGDB matching works for Epic titles
- [ ] Games appear in collection with Epic storefront
- [ ] Disconnect cleans up legendary config
- [ ] Multi-user isolation works (different configs)
- [ ] Auth expiration detected and handled gracefully
- [ ] Re-authentication flow works after expiration

## Implementation Plan

### Phase 1: Foundation (Backend Service)

1. Create `EpicService` class with legendary subprocess wrapper
2. Implement auth methods (start, complete, verify, disconnect)
3. Implement library fetching with JSON parsing
4. Add error handling and auth expiration detection
5. Write unit tests for EpicService

### Phase 2: Sync Integration

1. Create `EpicSyncAdapter` implementing `SyncSourceAdapter`
2. Register Epic adapter in `get_sync_adapter()`
3. Add auth expiration handling to sync pipeline
4. Write adapter unit tests
5. Test end-to-end sync with mocked legendary responses

### Phase 3: API Endpoints

1. Add Epic auth endpoints (start, complete, check, disconnect)
2. Update sync status response schema for auth expiration
3. Add request/response schemas for Epic endpoints
4. Write API integration tests
5. Test error cases and edge cases

### Phase 4: Deployment

1. Update Dockerfile with legendary installation
2. Add legendary-configs volume to docker-compose
3. Update Nix flake for development environment
4. Document legendary installation in README
5. Add troubleshooting guide

### Phase 5: Testing & Documentation

1. End-to-end testing with real Epic account (optional)
2. Load testing with multiple users
3. Update PRD with implementation status
4. Write user-facing documentation for Epic sync
5. Create admin guide for legendary maintenance

## Security Considerations

### Credential Isolation

- Each user has separate legendary config directory
- Use `XDG_CONFIG_HOME` to enforce isolation
- legendary stores tokens in its own encrypted format
- Never expose legendary auth tokens in API responses
- Clear legendary config on user deletion

### Process Security

- Run legendary commands with timeout limits
- Validate all input before passing to legendary
- Sanitize legendary output before returning to user
- Log security-relevant events (auth, disconnect)

### API Security

- All endpoints require authentication
- Rate limit auth endpoints to prevent abuse
- Validate authorization codes before passing to legendary
- HTTPS only in production

## Risks & Mitigations

### Risk: legendary CLI Changes

**Impact**: Commands or output format changes break integration
**Mitigation**:
- Pin legendary version in dependencies
- Monitor legendary releases for breaking changes
- Add integration tests that verify legendary output format
- Document legendary version requirements

### Risk: Epic API Changes

**Impact**: legendary stops working if Epic changes their API
**Mitigation**:
- legendary maintainers typically update quickly
- Our adapter is isolated, can be updated independently
- Graceful degradation if sync fails
- Clear error messages to users

### Risk: Authentication Complexity

**Impact**: Users find manual code copy/paste confusing
**Mitigation**:
- Clear step-by-step instructions in UI
- Screenshots in documentation
- Helpful error messages
- Consider future automation if Epic provides better OAuth

### Risk: Multi-User Config Corruption

**Impact**: legendary configs get mixed up between users
**Mitigation**:
- Strict XDG_CONFIG_HOME isolation
- Integration tests for multi-user scenarios
- File system permissions enforcement
- Regular config validation

### Risk: GPL3 License Contamination

**Impact**: Accidentally importing legendary code violates MIT
**Mitigation**:
- Document subprocess-only usage clearly
- Code review requirements for Epic integration
- Never import legendary Python modules
- Maintain legendary as system dependency only

## Future Enhancements

### Automatic Token Refresh

legendary may support automatic refresh in future. Monitor for this capability to improve UX.

### Playtime Sync

If Epic exposes playtime data in future, add to `ExternalGame.playtime_hours`.

### Platform Support

If Epic adds support for additional platforms (e.g., Mac), extend platform mapping.

### Parallel Fetching

For users with large libraries, consider parallel IGDB matching to speed up sync.

## Success Criteria

- [ ] Users can authenticate with Epic Games Store
- [ ] Epic library syncs automatically on trigger
- [ ] Games appear in collection with Epic storefront
- [ ] Auth expiration handled gracefully
- [ ] Multi-user isolation works correctly
- [ ] All tests pass with >80% coverage
- [ ] legendary remains external dependency (no GPL3 contamination)
- [ ] Documentation complete and accurate
- [ ] Zero breaking changes to existing sync functionality

## References

- [legendary GitHub Repository](https://github.com/derrod/legendary)
- [legendary PyPI Package](https://pypi.org/project/legendary-gl/)
- [Epic Games Store](https://www.epicgames.com/store/)
- Existing Steam sync implementation: `app/worker/tasks/sync/adapters/steam.py`
- Sync adapter protocol: `app/worker/tasks/sync/adapters/base.py`
