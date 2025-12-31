# Epic Games Store Authentication Improvements Design

**Date:** 2026-01-01
**Status:** Approved for Implementation

## Overview

This design implements database-backed credential storage for Epic Games Store authentication, enabling both API and worker containers to share credentials. This eliminates the current limitation where credentials stored in the API container's filesystem are not accessible to the worker container.

## Problem Statement

Currently, Epic credentials are stored per-container in legendary's filesystem config (`/var/lib/nexorious/legendary-configs/{user_id}`). This prevents the worker container from accessing credentials saved by the API container, breaking Epic library sync functionality.

## Solution

Store legendary's `user.json` credentials in the `user_sync_configs.platform_credentials` database field (already exists). Both API and worker containers access credentials via the database, with automatic hydration to the filesystem for legendary's use.

## Design Decisions

### 1. Database-Backed Credential Storage

**Credential Flow:**
1. **After Authentication**: When `complete_epic_auth()` succeeds, read legendary's `user.json` file and store it as JSON string in `platform_credentials`
2. **On Service Initialization**: When `EpicService.__init__()` runs, check if database has credentials for this user's Epic platform config
3. **Hydration**: If database credentials exist, write them to the filesystem location that legendary expects before creating `LegendaryCore`
4. **Token Refresh**: Let legendary handle token refresh automatically via `login()`, then read updated credentials back to database

**Database Schema** (already exists):
- `UserSyncConfig.platform_credentials` - TEXT field storing JSON
- No schema changes needed

**File Locations:**
- Use existing storage configuration (not hardcoded)
- Legendary config: Current default path pattern maintained
- This is where legendary stores auth data via `XDG_CONFIG_HOME`

### 2. OAuth Flow

**Decision:** Keep the existing manual OAuth code entry flow as-is.

**Rationale:**
- Legendary's OAuth implementation uses a hardcoded redirect to Epic's own page (`https://www.epicgames.com/id/api/redirect`)
- Fully automating code capture would require a browser extension or custom Epic OAuth client
- The manual flow works reliably and is consistent with legendary's CLI behavior
- We can focus implementation effort on the more valuable database credential storage feature

**No Changes Required:**
- Keep existing endpoints: `/sync/epic/auth/start`, `/sync/epic/auth/complete`
- No new OAuth callback endpoint needed
- No frontend popup/capture logic needed

## Implementation Details

### EpicService Changes

**File:** [backend/app/services/epic.py](../../backend/app/services/epic.py)

**New methods to add:**

```python
def _get_user_json_path(self) -> str:
    """Get path to legendary's user.json file."""
    return os.path.join(self.config_path, "legendary", "user.json")

def _load_credentials_from_db(self, session: Session) -> None:
    """Load credentials from database and write to filesystem for legendary."""
    # Query UserSyncConfig for this user's Epic credentials
    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == self.user_id,
        UserSyncConfig.platform == "epic"
    )
    config = session.exec(stmt).first()

    if config and config.platform_credentials:
        # Parse JSON credentials from database
        credentials = json.loads(config.platform_credentials)

        # Ensure directory exists
        user_json_path = self._get_user_json_path()
        os.makedirs(os.path.dirname(user_json_path), exist_ok=True)

        # Write credentials to filesystem for legendary to use
        with open(user_json_path, 'w') as f:
            json.dump(credentials, f)

        logger.debug(f"Loaded Epic credentials from database for user {self.user_id}")

def _save_credentials_to_db(self, session: Session) -> None:
    """Read credentials from filesystem and save to database."""
    user_json_path = self._get_user_json_path()

    if not os.path.exists(user_json_path):
        logger.warning(f"No user.json found at {user_json_path}")
        return

    # Read credentials from filesystem
    with open(user_json_path, 'r') as f:
        credentials = json.load(f)

    # Find or create UserSyncConfig
    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == self.user_id,
        UserSyncConfig.platform == "epic"
    )
    config = session.exec(stmt).first()

    if not config:
        config = UserSyncConfig(
            user_id=self.user_id,
            platform="epic"
        )
        session.add(config)

    # Store credentials as JSON string
    config.platform_credentials = json.dumps(credentials)
    config.updated_at = datetime.now(timezone.utc)

    session.commit()
    logger.info(f"Saved Epic credentials to database for user {self.user_id}")
```

**Modified `__init__` method:**

```python
def __init__(self, user_id: str, session: Session | None = None):
    """Initialize Epic service with user-specific config path."""
    self.user_id = user_id
    self.config_path = f"/var/lib/nexorious/legendary-configs/{user_id}"

    # Load credentials from database if available
    if session:
        self._load_credentials_from_db(session)

    # Set environment variable for legendary
    os.environ['XDG_CONFIG_HOME'] = self.config_path

    # Initialize legendary core
    try:
        self.core = LegendaryCore()
        logger.debug(f"EpicService initialized for user {user_id}")
    except Exception as e:
        logger.error(f"Failed to initialize LegendaryCore: {e}")
        raise EpicAPIError(f"Failed to initialize Epic service: {e}")
```

### API Endpoint Changes

**File:** [backend/app/api/sync.py](../../backend/app/api/sync.py)

**Update `complete_epic_auth()` at line 492:**

```python
@router.post("/epic/auth/complete", response_model=EpicAuthCompleteResponse)
async def complete_epic_auth(
    request: EpicAuthCompleteRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> EpicAuthCompleteResponse:
    """Complete Epic Games authentication with authorization code."""
    logger.info(f"Completing Epic authentication for user {current_user.id}")

    try:
        epic_service = EpicService(current_user.id, session=session)

        # Complete authentication with the code
        await epic_service.complete_auth(request.code)

        # Get account information
        account_info = await epic_service.get_account_info()

        # Save credentials to database (NEW)
        epic_service._save_credentials_to_db(session)

        # Update user preferences with Epic credentials
        preferences = current_user.preferences or {}
        preferences["epic"] = {
            "is_verified": True,
            "display_name": account_info.display_name,
            "account_id": account_info.account_id,
        }
        current_user.preferences_json = json.dumps(preferences)
        current_user.updated_at = datetime.now(timezone.utc)

        session.commit()

        logger.info(f"Epic authentication completed for user {current_user.id}")

        return EpicAuthCompleteResponse(
            valid=True,
            display_name=account_info.display_name,
        )
    except EpicAuthenticationError as e:
        # ... existing error handling
```

**Update `disconnect_epic()` at line 583:**

```python
@router.delete("/epic/connection", response_model=SuccessResponse)
async def disconnect_epic(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SuccessResponse:
    """Disconnect Epic Games integration."""
    logger.info(f"Disconnecting Epic for user {current_user.id}")

    try:
        epic_service = EpicService(current_user.id, session=session)
        await epic_service.disconnect()
    except EpicAPIError as e:
        logger.warning(f"Error disconnecting Epic service: {e}")

    # Clear credentials from database (NEW)
    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == current_user.id,
        UserSyncConfig.platform == "epic"
    )
    config = session.exec(stmt).first()
    if config:
        config.platform_credentials = None
        config.updated_at = datetime.now(timezone.utc)

    # Clear Epic credentials from preferences
    preferences = current_user.preferences or {}
    if "epic" in preferences:
        del preferences["epic"]
        current_user.preferences_json = json.dumps(preferences)

    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()

    logger.info(f"Epic disconnected for user {current_user.id}")

    return SuccessResponse(
        success=True,
        message="Epic Games disconnected successfully",
    )
```

### Worker Adapter Changes

**File:** [backend/app/worker/tasks/sync/adapters/epic.py](../../backend/app/worker/tasks/sync/adapters/epic.py)

**Update `fetch_games()` method:**

```python
async def fetch_games(self, user: User, session: Session) -> List[ExternalGame]:
    """Fetch all games from user's Epic library."""
    credentials = self.get_credentials(user)
    if not credentials:
        raise ValueError("Epic credentials not configured for this user")

    epic_service = EpicService(user_id=user.id, session=session)  # Pass session
    epic_games = await epic_service.get_library()

    # ... rest remains the same
```

## Testing Strategy

### Unit Tests

**File:** [backend/app/tests/test_epic_service.py](../../backend/app/tests/test_epic_service.py)

**New tests to add:**

1. **`test_load_credentials_from_db_success`**
   - Mock UserSyncConfig with valid platform_credentials JSON
   - Verify credentials are written to filesystem at correct path
   - Verify user.json file contains expected data

2. **`test_load_credentials_from_db_no_config`**
   - No UserSyncConfig exists for user
   - Verify no filesystem write occurs
   - Verify EpicService still initializes successfully

3. **`test_load_credentials_from_db_empty_credentials`**
   - UserSyncConfig exists but platform_credentials is None
   - Verify no filesystem write occurs

4. **`test_save_credentials_to_db_success`**
   - Create mock user.json file on filesystem
   - Call `_save_credentials_to_db()`
   - Verify UserSyncConfig.platform_credentials contains JSON
   - Verify JSON can be parsed and matches filesystem file

5. **`test_save_credentials_to_db_creates_config`**
   - No existing UserSyncConfig for Epic platform
   - Call `_save_credentials_to_db()`
   - Verify new UserSyncConfig is created
   - Verify credentials are saved

6. **`test_save_credentials_to_db_no_file`**
   - user.json doesn't exist on filesystem
   - Call `_save_credentials_to_db()`
   - Verify graceful handling (log warning, no crash)

### API Integration Tests

**File:** [backend/app/tests/test_sync_api.py](../../backend/app/tests/test_sync_api.py) or new file

1. **`test_complete_epic_auth_saves_to_db`**
   - Mock legendary auth flow
   - POST to `/sync/epic/auth/complete`
   - Verify UserSyncConfig.platform_credentials is populated
   - Verify credentials are valid JSON

2. **`test_disconnect_epic_clears_db_credentials`**
   - Set up user with Epic credentials in database
   - DELETE `/sync/epic/connection`
   - Verify UserSyncConfig.platform_credentials is None

3. **`test_epic_service_uses_db_credentials`**
   - Store credentials in database
   - Initialize EpicService
   - Verify filesystem has credentials written
   - Mock legendary `verify_auth()` call succeeds

### Integration Testing

**Cross-Container Credential Sharing:**

Manual testing scenario:
1. **Setup**: Run both API and worker containers
2. **Authenticate in API**: Use frontend to connect Epic account
3. **Verify Database**: Check `user_sync_configs.platform_credentials` has data
4. **Trigger Sync**: Start Epic library sync job
5. **Verify Worker**: Check worker logs show successful Epic authentication
6. **Expected Result**: Worker successfully fetches Epic library using credentials from database

### Testing Edge Cases

1. **Token Refresh Scenario:**
   - Store expired credentials in database
   - Initialize EpicService
   - Legendary should auto-refresh token via `login()`
   - Verify refreshed token is still in filesystem (legendary handles this)
   - Future enhancement: Update database after refresh

2. **Concurrent Access:**
   - API container authenticates while worker is syncing
   - Verify no race conditions or file lock issues
   - Each container should have its own filesystem path

3. **Migration Scenario:**
   - User has existing filesystem credentials (old setup)
   - No database credentials
   - On next auth verification, credentials should stay functional
   - Future: Add migration script to move existing filesystem creds to DB

### Coverage Goals

- **EpicService credential methods**: 100% coverage
- **API endpoints (complete_auth, disconnect)**: 100% coverage
- **Overall backend**: Maintain >80% coverage requirement

## Deployment & Migration

### Deployment Steps

**No database migration required** - `platform_credentials` field already exists in `user_sync_configs` table.

**Code deployment order:**
1. Deploy backend changes (API + worker simultaneously is fine)
2. No frontend changes required
3. No configuration changes required
4. Restart containers to pick up new code

### Backward Compatibility

**Existing users with filesystem credentials:**
- Will continue to work normally
- Filesystem credentials take precedence if they exist
- On next authentication, credentials will be saved to database
- No data loss or disruption

**New users:**
- Will have credentials stored in database immediately after first auth
- Credentials hydrated to filesystem automatically

### Migration Path (Optional Future Enhancement)

For existing users, could add a one-time migration to move filesystem credentials to database:

```python
# Optional: Add to a management command or startup routine
def migrate_filesystem_credentials_to_db():
    """One-time migration of existing filesystem credentials to database."""
    # Find users with Epic preferences but no platform_credentials
    # Read their filesystem user.json
    # Save to database
    # This is not required for functionality, just for consistency
```

**Decision**: Skip this migration initially. Users will naturally migrate as they re-authenticate or sync runs.

### Monitoring & Observability

**Logging to add:**
- `INFO`: "Loaded Epic credentials from database for user {user_id}"
- `INFO`: "Saved Epic credentials to database for user {user_id}"
- `WARNING`: "No user.json found at {path}" (when saving fails)
- `DEBUG`: "No database credentials found for user {user_id}, proceeding with filesystem only"

**Metrics to consider (future):**
- Count of users with database credentials vs filesystem-only
- Auth success/failure rates
- Token refresh frequency

### Rollback Plan

If issues arise:
1. Revert code deployment
2. Users with filesystem credentials continue working normally
3. No database rollback needed (platform_credentials field can remain populated)

### Security Considerations

**Credential storage:**
- Database credentials stored as JSON TEXT
- Same security posture as user preferences
- Database should be encrypted at rest (infrastructure level)
- Consider encrypting platform_credentials field in future enhancement

**Access control:**
- Only user's own credentials accessible via API endpoints
- Worker jobs already have user_id context for proper isolation

### Known Limitations

1. **Token refresh not persisted back to database** - When legendary refreshes an expired token, the new token stays in filesystem only. Next authentication will update database. This is acceptable for MVP.

2. **No encryption** - Credentials stored as plaintext JSON in database. Same as user preferences. Future enhancement could add field-level encryption.

3. **No credential expiry handling** - Rely on legendary's built-in token refresh. Database may contain stale tokens but legendary handles this transparently.

## Success Criteria

**Feature is successful when:**
- ✅ API container can authenticate Epic account
- ✅ Worker container can fetch Epic library using same credentials
- ✅ No manual credential copying between containers needed
- ✅ All tests pass with >80% coverage
- ✅ No regression in existing Epic sync functionality

## References

- [Epic Games Store Sync Implementation Plan](2025-12-31-epic-games-store-sync-implementation.md)
- [Ideas Document](../IDEAS.md#epic-games-store-authentication-improvements)
- [legendary-gl GitHub Repository](https://github.com/derrod/legendary)
