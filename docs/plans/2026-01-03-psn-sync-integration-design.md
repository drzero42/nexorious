# PlayStation Network Sync Integration Design

**Date**: 2026-01-03
**Status**: Design Phase
**Related PRD Section**: 6.1 Enhanced Storefront Integration

## Executive Summary

Add PlayStation Network as a sync source to automatically import user's PSN library into Nexorious collection. Implementation uses the PSNAWP Python library with NPSSO token authentication, following the existing Steam sync adapter pattern with simple credential storage in user preferences.

## Goals

1. Enable users to sync their PlayStation Network library
2. Follow existing Steam sync architecture pattern (simplest approach)
3. Handle NPSSO token authentication (manual entry like Steam API key)
4. Support smart platform detection (PS4/PS5) with fallback
5. Handle token expiration gracefully (~2 month lifespan)

## Non-Goals

- Automated token extraction (manual entry only)
- Syncing trophy data (future enhancement)
- Syncing playtime data (if PSN doesn't provide it)
- Supporting non-PlayStation platforms
- PS3 game sync (API limitation - getPurchasedGames only returns PS4/PS5)

## Architecture Overview

### Core Approach

Follow Steam's adapter-based architecture with single-storage credentials in user preferences. PSNAWP is a Python library that can be directly imported and used (unlike Epic's legendary CLI which requires filesystem storage).

### Key Components

1. **PSN Sync Adapter** (`app/worker/tasks/sync/adapters/psn.py`) - Implements `SyncSourceAdapter` protocol
2. **PSN Service** (`app/services/psn.py`) - Wraps PSNAWP library
3. **Authentication Flow** - Manual NPSSO token entry (like Steam API key)
4. **Credential Storage** - Simple storage in `user.preferences["psn"]` (like Steam)

### Platform/Storefront Mapping

- Platforms: `playstation-4`, `playstation-5` (smart detection from entitlements)
- Storefront: `playstation-store` (already exists in seed data per PRD)
- PS3: Not included in sync (API limitation), users can add manually

### Data Flow

```
User enters NPSSO token in settings
  → Backend verifies token with PSNAWP
  → PSNAWP fetches account info
  → Backend stores token + account info in user.preferences["psn"]
  → User triggers sync
  → PSNSyncAdapter.fetch_games()
  → PSNService uses PSNAWP to fetch purchased games
  → Smart platform detection (PS4/PS5 entitlements)
  → Convert to ExternalGame objects
  → Existing sync pipeline (dispatch → IGDB match → process)
```

## Authentication Implementation

### NPSSO Token

**What is NPSSO?**: Network Platform Single Sign-On cookie obtained after signing into PlayStation Network. Required for PSN API access.

**How to Obtain**:
1. Sign in to PlayStation.com with PSN account
2. Navigate to: https://ca.account.sony.com/api/v1/ssocookie
3. Copy the 64-character NPSSO token
4. Paste into Nexorious settings

**Token Lifespan**: ~2 months, then requires refresh

**Security**: Treat like a password - never share or paste on untrusted websites

### PSNAWP Initialization

```python
from psnawp_api import PSNAWP

# Initialize with NPSSO token (no filesystem needed)
psnawp = PSNAWP('<64 character npsso code>')
client = psnawp.me()

# Get account info
online_id = client.online_id
account_id = client.account_id
region = client.get_region()

# Get purchased games (PS4/PS5 only)
purchased_games = client.purchased_games()
```

### User Preferences Storage

```json
{
  "psn": {
    "npsso_token": "64-character-token-here",
    "online_id": "user_psn_name",
    "account_id": "account-id-here",
    "region": "us",
    "is_verified": true
  }
}
```

When token expires:
```json
{
  "psn": {
    "npsso_token": "...",
    "online_id": "...",
    "account_id": "...",
    "region": "...",
    "is_verified": false,
    "token_expired_at": "2026-03-03T12:00:00Z"
  }
}
```

## Platform Detection Strategy

### Data Source

Use PSNAWP's **purchased games** endpoint because:
- Returns actual **owned/purchased** games (matches Steam/Epic behavior)
- Includes PS4 and PS5 games with entitlement information
- PS3 games excluded (API limitation - users add manually if needed)

### Detection Logic

```python
# Get purchased games for user
purchased_games = client.purchased_games()

for game in purchased_games:
    platforms = []

    # Check which platforms the user has entitlement for
    # If game is available on both PS4 and PS5, add both
    if game.has_ps5_entitlement:
        platforms.append("playstation-5")
    if game.has_ps4_entitlement:
        platforms.append("playstation-4")

    # Fallback: if no platform info detected, default to PS5
    if not platforms:
        platforms = ["playstation-5"]

    # Create ExternalGame for EACH platform user has entitlement for
    for platform in platforms:
        external_game = ExternalGame(
            external_id=game.product_id,
            title=game.name,
            platform=platform,
            storefront="playstation-store",
            metadata={"product_id": game.product_id, ...}
        )
```

### Multi-Platform Handling

- If user purchased a game that gives them **both PS4 and PS5 versions** (common with cross-gen purchases), we create **separate ExternalGame entries for each platform**
- During IGDB matching and sync, the game will be added to the user's collection with **both PlayStation 4 and PlayStation 5 platform associations**
- Matches Nexorious's existing multi-platform ownership model perfectly

### PS3 Games

- Not included in automatic sync (PSN API limitation)
- Users can manually add PS3 games using standard "Add Game" flow
- Documentation should note this limitation

## Service Implementation

### PSN Service (`app/services/psn.py`)

```python
from psnawp_api import PSNAWP
from dataclasses import dataclass
from typing import List, Dict, Any
import logging

logger = logging.getLogger(__name__)


@dataclass
class PSNAccountInfo:
    """PSN account information."""
    online_id: str        # PSN username
    account_id: str       # Unique account identifier
    region: str           # Account region


@dataclass
class PSNGame:
    """PSN game information from purchased library."""
    product_id: str       # Unique game identifier
    name: str             # Game title
    platforms: List[str]  # ["playstation-4", "playstation-5"]
    metadata: Dict[str, Any]  # Additional game metadata


class PSNAPIError(Exception):
    """PSN API error."""
    pass


class PSNAuthenticationError(PSNAPIError):
    """PSN authentication failed or invalid NPSSO token."""
    pass


class PSNTokenExpiredError(PSNAPIError):
    """PSN NPSSO token expired (~2 months)."""
    pass


class PSNService:
    """Service for interacting with PlayStation Network via PSNAWP library.

    Args:
        npsso_token: User's 64-character NPSSO token from PlayStation.com
    """

    def __init__(self, npsso_token: str):
        """Initialize PSN service with user's NPSSO token."""
        self.npsso_token = npsso_token
        try:
            self.psnawp = PSNAWP(npsso_token)
        except Exception as e:
            logger.error(f"Failed to initialize PSNAWP: {e}")
            raise PSNAuthenticationError(f"Failed to initialize PSN service: {e}")

    async def verify_token(self) -> bool:
        """Verify that the NPSSO token is valid.

        Returns:
            True if token is valid, False otherwise
        """
        try:
            client = self.psnawp.me()
            # Try to access basic account info
            _ = client.online_id
            return True
        except Exception as e:
            logger.warning(f"Token verification failed: {e}")
            return False

    async def get_account_info(self) -> PSNAccountInfo:
        """Get PSN account information.

        Returns:
            PSN account information

        Raises:
            PSNAuthenticationError: If token is invalid
            PSNTokenExpiredError: If token has expired
        """
        try:
            client = self.psnawp.me()

            return PSNAccountInfo(
                online_id=client.online_id,
                account_id=client.account_id,
                region=client.get_region()
            )
        except Exception as e:
            # Check if error indicates expired token
            error_str = str(e).lower()
            if "expired" in error_str or "unauthorized" in error_str:
                raise PSNTokenExpiredError("NPSSO token has expired")
            raise PSNAuthenticationError(f"Failed to get account info: {e}")

    async def get_library(self) -> List[PSNGame]:
        """Get purchased games from PSN library (PS4/PS5 only).

        Returns:
            List of PSN games with platform entitlements

        Raises:
            PSNTokenExpiredError: If token has expired
            PSNAPIError: If library cannot be retrieved
        """
        try:
            client = self.psnawp.me()
            purchased_games = client.purchased_games()

            games = []
            for game in purchased_games:
                # Detect which platforms user has entitlement for
                platforms = []
                if hasattr(game, 'has_ps5_entitlement') and game.has_ps5_entitlement:
                    platforms.append("playstation-5")
                if hasattr(game, 'has_ps4_entitlement') and game.has_ps4_entitlement:
                    platforms.append("playstation-4")

                # Fallback to PS5 if no platform info
                if not platforms:
                    platforms = ["playstation-5"]

                psn_game = PSNGame(
                    product_id=game.product_id,
                    name=game.name,
                    platforms=platforms,
                    metadata={
                        "product_id": game.product_id,
                        # Add other metadata as needed
                    }
                )
                games.append(psn_game)

            logger.info(f"Retrieved {len(games)} games from PSN library")
            return games

        except Exception as e:
            error_str = str(e).lower()
            if "expired" in error_str or "unauthorized" in error_str:
                raise PSNTokenExpiredError("NPSSO token has expired")
            raise PSNAPIError(f"Failed to retrieve PSN library: {e}")

    async def disconnect(self) -> None:
        """Disconnect PSN account.

        Note: PSNAWP is stateless, so this is a no-op.
        Actual credential cleanup happens in preferences.
        """
        # No-op for PSNAWP (stateless library)
        pass


def create_psn_service(npsso_token: str) -> PSNService:
    """Factory function to create a PSN service instance."""
    return PSNService(npsso_token)
```

## Sync Adapter Implementation

### PSN Sync Adapter (`app/worker/tasks/sync/adapters/psn.py`)

```python
"""PSN sync adapter for fetching user's PlayStation Network library.

Implements SyncSourceAdapter protocol to fetch games from PSN
and convert them to the standardized ExternalGame format.
"""

import logging
from typing import Optional, List, Dict
from datetime import datetime, timezone

from sqlmodel import Session

from app.models.user import User
from app.models.job import BackgroundJobSource
from app.services.psn import PSNService, PSNTokenExpiredError
from .base import ExternalGame

logger = logging.getLogger(__name__)


class PSNSyncAdapter:
    """Adapter for syncing games from PlayStation Network.

    Fetches the user's PSN purchased library and converts games to
    ExternalGame format for generic processing.
    """

    source = BackgroundJobSource.PSN

    async def fetch_games(self, user: User, session: Session) -> List[ExternalGame]:
        """Fetch all purchased games from user's PSN library.

        Args:
            user: The user whose PSN library to fetch
            session: SQLModel database session

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If PSN credentials are not configured
            PSNTokenExpiredError: If NPSSO token has expired
        """
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("PSN credentials not configured for this user")

        psn_service = PSNService(npsso_token=credentials["npsso_token"])

        try:
            psn_games = await psn_service.get_library()
        except PSNTokenExpiredError:
            # Mark token as expired in preferences
            self._mark_token_expired(user, session)
            raise

        logger.info(f"Fetched {len(psn_games)} games from PSN for user {user.id}")

        # Convert to ExternalGame objects
        # Create one ExternalGame per platform entitlement
        external_games = []
        for game in psn_games:
            for platform in game.platforms:
                external_games.append(
                    ExternalGame(
                        external_id=game.product_id,
                        title=game.name,
                        platform=platform,  # "playstation-4" or "playstation-5"
                        storefront="playstation-store",
                        metadata={
                            "product_id": game.product_id,
                            **game.metadata
                        },
                        playtime_hours=0  # PSN doesn't provide playtime in library
                    )
                )

        logger.info(f"Created {len(external_games)} ExternalGame objects from {len(psn_games)} PSN games")
        return external_games

    def get_credentials(self, user: User) -> Optional[Dict[str, str]]:
        """Extract PSN credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with npsso_token, or None if not configured
        """
        preferences = user.preferences or {}
        psn_config = preferences.get("psn", {})

        npsso_token = psn_config.get("npsso_token")
        is_verified = psn_config.get("is_verified", False)

        if not npsso_token or not is_verified:
            return None

        return {"npsso_token": npsso_token}

    def is_configured(self, user: User) -> bool:
        """Check if user has verified PSN credentials.

        Args:
            user: The user to check

        Returns:
            True if PSN credentials are configured and verified
        """
        return self.get_credentials(user) is not None

    def _mark_token_expired(self, user: User, session: Session) -> None:
        """Mark PSN token as expired in user preferences.

        Args:
            user: The user whose token expired
            session: SQLModel database session
        """
        preferences = user.preferences or {}
        if "psn" in preferences:
            preferences["psn"]["is_verified"] = False
            preferences["psn"]["token_expired_at"] = datetime.now(timezone.utc).isoformat()
            user.preferences = preferences
            session.commit()
            logger.warning(f"Marked PSN token as expired for user {user.id}")
```

### Integration Points

1. **Register adapter** in `get_sync_adapter()` function (`app/worker/tasks/sync/adapters/base.py`):
   ```python
   adapters = {
       "steam": SteamSyncAdapter,
       "epic": EpicSyncAdapter,
       "psn": PSNSyncAdapter,  # ADD THIS
   }
   ```

2. **Add enum value**: `SyncPlatform.PSN = "psn"` in `app/schemas/sync.py`

3. **Add job source**: `BackgroundJobSource.PSN = "psn"` in `app/models/job.py`

4. **No changes needed** to dispatch/process_item tasks - they're already generic

## API Endpoints

### New PSN Configuration Endpoints (`app/api/sync.py`)

```python
from pydantic import BaseModel, Field
from typing import Optional

# Schemas
class PSNConfigureRequest(BaseModel):
    """Request to configure PSN sync with NPSSO token."""
    npsso_token: str = Field(
        ...,
        min_length=64,
        max_length=64,
        description="64-character NPSSO token from PlayStation.com"
    )


class PSNConfigureResponse(BaseModel):
    """Response after configuring PSN sync."""
    success: bool
    online_id: str
    account_id: str
    region: str
    message: str


class PSNStatusResponse(BaseModel):
    """PSN connection status."""
    is_configured: bool
    online_id: Optional[str] = None
    account_id: Optional[str] = None
    region: Optional[str] = None
    token_expired: bool = False


# Endpoints
@router.post("/sync/psn/configure", response_model=PSNConfigureResponse)
async def configure_psn(
    request: PSNConfigureRequest,
    current_user: User = Depends(get_current_user),
    session: Session = Depends(get_session)
):
    """Configure PSN sync by verifying and storing NPSSO token.

    Steps:
    1. Verify NPSSO token with PSNAWP
    2. Fetch account information
    3. Store token and account info in user.preferences["psn"]
    4. Return account details
    """
    try:
        # Verify token and get account info
        psn_service = PSNService(npsso_token=request.npsso_token)
        account_info = await psn_service.get_account_info()

        # Store in preferences
        preferences = current_user.preferences or {}
        preferences["psn"] = {
            "npsso_token": request.npsso_token,
            "online_id": account_info.online_id,
            "account_id": account_info.account_id,
            "region": account_info.region,
            "is_verified": True
        }
        current_user.preferences = preferences
        session.commit()

        logger.info(f"PSN configured successfully for user {current_user.id}")

        return PSNConfigureResponse(
            success=True,
            online_id=account_info.online_id,
            account_id=account_info.account_id,
            region=account_info.region,
            message="PSN configured successfully"
        )

    except PSNAuthenticationError as e:
        logger.error(f"PSN authentication failed: {e}")
        raise HTTPException(
            status_code=400,
            detail=f"Invalid NPSSO token: {str(e)}"
        )
    except Exception as e:
        logger.error(f"Error configuring PSN: {e}")
        raise HTTPException(
            status_code=500,
            detail="Failed to configure PSN"
        )


@router.get("/sync/psn/status", response_model=PSNStatusResponse)
async def get_psn_status(
    current_user: User = Depends(get_current_user)
):
    """Get PSN connection status and account information."""
    preferences = current_user.preferences or {}
    psn_config = preferences.get("psn", {})

    return PSNStatusResponse(
        is_configured=psn_config.get("is_verified", False),
        online_id=psn_config.get("online_id"),
        account_id=psn_config.get("account_id"),
        region=psn_config.get("region"),
        token_expired=not psn_config.get("is_verified", False) and "token_expired_at" in psn_config
    )


@router.delete("/sync/psn/disconnect")
async def disconnect_psn(
    current_user: User = Depends(get_current_user),
    session: Session = Depends(get_session)
):
    """Disconnect PSN account by removing stored credentials."""
    preferences = current_user.preferences or {}
    if "psn" in preferences:
        del preferences["psn"]
        current_user.preferences = preferences
        session.commit()
        logger.info(f"PSN disconnected for user {current_user.id}")

    return {"success": True, "message": "PSN disconnected successfully"}
```

### Update Existing Sync Check Endpoint

Add PSN to the `_is_platform_configured()` helper in `app/api/sync.py`:

```python
def _is_platform_configured(user: User, platform: str) -> bool:
    """Check if platform credentials are configured."""
    preferences = user.preferences or {}

    if platform == "steam":
        steam_config = preferences.get("steam", {})
        return bool(
            steam_config.get("web_api_key")
            and steam_config.get("steam_id")
            and steam_config.get("is_verified", False)
        )
    elif platform == "epic":
        epic_config = preferences.get("epic", {})
        return bool(
            epic_config.get("is_verified", False)
            and epic_config.get("account_id")
        )
    elif platform == "psn":  # ADD THIS
        psn_config = preferences.get("psn", {})
        return bool(
            psn_config.get("npsso_token")
            and psn_config.get("is_verified", False)
        )

    return False
```

### Existing Endpoints (No Changes Needed)

- `POST /sync/psn` - Trigger sync (already generic)
- `GET /sync/config` - Get all sync configs including PSN
- `PUT /sync/config/psn` - Update PSN sync settings

## Deployment Requirements

### PSNAWP Installation

**Package**: `psnawp` (PyPI package name)

**Backend Dependencies** (`backend/pyproject.toml`):
```toml
[project]
dependencies = [
    # ... existing dependencies
    "psnawp>=2.1.0",
]
```

**Installation Command**:
```bash
cd backend
uv sync  # Will install psnawp from pyproject.toml
```

**Nix flake** (for development - optional):
```nix
# Already handled by uv sync in devShell
# No specific nix package needed
```

### No Additional Infrastructure

- No filesystem storage needed (unlike Epic/legendary)
- No additional volumes or config directories
- No environment variables needed
- Just the Python package dependency

## Testing Strategy

### Unit Tests

**`app/tests/test_psn_service.py`**:
- Mock PSNAWP library calls
- Test token verification (valid/invalid)
- Test account info fetching
- Test library fetching and platform detection
- Test error handling (invalid token, expired token, API errors)
- Test multi-platform game handling (PS4+PS5)

**`app/tests/test_sync_adapters.py`** (add PSN tests):
- Mock PSN service responses
- Verify ExternalGame conversion
- Test credential validation from preferences
- Test token expiration handling
- Test multi-platform game creates multiple ExternalGame objects

### Integration Tests

**`app/tests/test_api_psn_sync.py`**:
- PSN configure endpoint (valid/invalid tokens)
- PSN status endpoint
- PSN disconnect endpoint
- PSN sync trigger endpoint
- Error cases (not configured, expired token)

### Test Coverage Goals

- PSNService: >80% coverage
- PSNSyncAdapter: >80% coverage
- API endpoints: 100% coverage (all paths tested)
- Overall backend: Maintain >80% coverage

### Manual Testing Checklist

- [ ] PSNAWP installed and importable
- [ ] Configure PSN with valid NPSSO token
- [ ] Account info displays correctly
- [ ] Library sync pulls PSN games (PS4/PS5)
- [ ] Multi-platform games create multiple entries
- [ ] IGDB matching works for PSN titles
- [ ] Games appear in collection with PlayStation platforms
- [ ] Disconnect cleans up preferences
- [ ] Token expiration detected and handled gracefully
- [ ] Re-configuration works after expiration

### Error Handling

#### Token Validation Errors

```python
# Invalid token format (not 64 chars)
if len(npsso_token) != 64:
    raise HTTPException(
        status_code=400,
        detail="NPSSO token must be exactly 64 characters"
    )

# Invalid token (PSNAWP raises exception)
try:
    account_info = await psn_service.get_account_info()
except PSNAuthenticationError:
    raise HTTPException(
        status_code=400,
        detail="Invalid NPSSO token. Please obtain a new token from PlayStation.com"
    )
```

#### Token Expiration During Sync

```python
# In adapter.fetch_games()
try:
    psn_games = await psn_service.get_library()
except PSNTokenExpiredError:
    # Mark as expired in preferences
    preferences["psn"]["is_verified"] = False
    preferences["psn"]["token_expired_at"] = datetime.now(timezone.utc).isoformat()
    session.commit()
    # Job will fail with clear error message
    raise ValueError("PSN token expired. Please configure PSN with a new NPSSO token.")
```

#### API Rate Limiting

- PSNAWP handles rate limiting internally
- PSN API has rate limits but they're generous
- Add retry logic with exponential backoff if needed
- Log rate limit errors clearly

## Frontend Requirements

### Frontend Components & Pages

Following the Steam/Epic pattern for consistency:

#### Settings Page - PSN Configuration Card

**Location**: Settings page where Steam and Epic cards exist

**PSN Configuration Card**:
- Header: "PlayStation Network"
- Icon: PlayStation logo
- Configuration state display:
  - **Not configured**: Shows "Connect PSN" button with instructions
  - **Configured**: Shows online ID, account ID, region
  - **Token expired**: Shows warning banner with "Reconnect" button

**Configuration Flow**:
1. User clicks "Connect PSN" button
2. Modal/dialog opens with:
   - Instructions on how to get NPSSO token
   - Link to https://ca.account.sony.com/api/v1/ssocookie
   - Step-by-step guide (similar to Steam API key instructions)
   - Text input for 64-character NPSSO token
   - "Verify & Save" button
3. Backend verifies token and stores credentials
4. Card updates to show connected state

**Disconnect Flow**:
- "Disconnect" button in card
- Confirmation dialog
- Clears PSN credentials from preferences

#### Sync Management Page

**Location**: `/sync` or `/sync/psn` page (following existing pattern)

**PSN Sync Card/Section**:
- Shows sync status (last synced, sync in progress, errors)
- "Sync Now" button (only enabled if configured)
- Sync frequency settings (manual, hourly, daily, weekly)
- Auto-add toggle for matched games
- Token expiration warnings

**Token Expiration Handling**:
- If token expired during sync, show prominent warning
- "Update Token" button that opens configuration modal
- Clear messaging: "Your PSN token has expired. Please enter a new NPSSO token."

### Frontend API Integration

**Queries**:
```typescript
// Check PSN configuration status
GET /sync/psn/status
→ { is_configured, online_id, account_id, region, token_expired }

// Get sync status
GET /sync/config
→ { configs: [{ platform: "psn", frequency, auto_add, last_synced_at }] }
```

**Mutations**:
```typescript
// Configure PSN with NPSSO token
POST /sync/psn/configure
{ npsso_token: "..." }
→ { success, online_id, account_id, region, message }

// Trigger sync
POST /sync/psn
→ { job_id }

// Update sync settings
PUT /sync/config/psn
{ frequency, auto_add }

// Disconnect
DELETE /sync/psn/disconnect
→ { success, message }
```

### User Experience Flow

**First Time Setup**:
1. User navigates to Settings
2. Sees PSN card with "Not Connected" state
3. Clicks "Connect PSN"
4. Follows instructions to get NPSSO token from PlayStation.com
5. Pastes token into input field
6. Clicks "Verify & Save"
7. Card updates showing online ID and connection status
8. User can now trigger PSN sync

**Ongoing Sync**:
1. User navigates to Sync page
2. Clicks "Sync Now" for PSN
3. Job starts, UI shows progress
4. Games are fetched, matched with IGDB, added to collection
5. Multi-platform games (PS4+PS5) automatically get both platforms

**Token Expiration**:
1. User triggers sync (or auto-sync runs)
2. Backend detects expired token during sync
3. Job fails with "token_expired" error
4. Frontend shows warning: "PSN token expired"
5. User clicks "Update Token"
6. Configuration modal opens with new token input
7. User enters new NPSSO token
8. Can retry sync

### Documentation Requirements

**User-Facing Documentation**:
- How to obtain NPSSO token from PlayStation.com
- Screenshots of the token extraction process
- Security warning: treat NPSSO token like a password
- Token expiration notice (~2 months lifespan)
- Limitation: PS3 games must be added manually (API limitation)

**Developer Documentation**:
- PSNAWP library usage
- API endpoint documentation
- Frontend component integration guide

## Implementation Plan

### Phase 1: Backend Service Layer

**Tasks**:
1. Add `psnawp>=2.1.0` to `backend/pyproject.toml`
2. Run `uv sync` to install PSNAWP
3. Create `app/services/psn.py` with PSNService class
4. Implement authentication methods (verify_token, get_account_info)
5. Implement library fetching (get_library) with platform detection
6. Add custom exceptions (PSNAPIError, PSNAuthenticationError, PSNTokenExpiredError)
7. Write unit tests for PSNService (>80% coverage)

**Acceptance Criteria**:
- PSNAWP library installed and importable
- PSNService can verify NPSSO tokens
- PSNService can fetch account information
- PSNService can fetch purchased games with PS4/PS5 platform detection
- Multi-platform games return multiple platform entries
- Error handling works for invalid/expired tokens
- All tests pass

### Phase 2: Sync Adapter Implementation

**Tasks**:
1. Create `app/worker/tasks/sync/adapters/psn.py` with PSNSyncAdapter
2. Implement `fetch_games()` method following Steam pattern
3. Implement `get_credentials()` method reading from user.preferences
4. Implement `is_configured()` method
5. Add token expiration handling in adapter
6. Register PSNSyncAdapter in `get_sync_adapter()` function
7. Add `SyncPlatform.PSN` enum value
8. Add `BackgroundJobSource.PSN` enum value
9. Write adapter unit tests (>80% coverage)

**Acceptance Criteria**:
- Adapter follows Steam/Epic pattern exactly
- Converts PSN games to ExternalGame format correctly
- Multi-platform games create multiple ExternalGame objects (one per platform)
- Token expiration marks credentials as invalid
- Integrates with existing sync pipeline
- All tests pass

### Phase 3: API Endpoints

**Tasks**:
1. Add PSN schemas to `app/schemas/sync.py` (PSNConfigureRequest, PSNConfigureResponse, PSNStatusResponse)
2. Implement `POST /sync/psn/configure` endpoint
3. Implement `GET /sync/psn/status` endpoint
4. Implement `DELETE /sync/psn/disconnect` endpoint
5. Update `_is_platform_configured()` helper to include PSN
6. Write API integration tests (100% coverage)
7. Update OpenAPI documentation

**Acceptance Criteria**:
- All endpoints follow existing Steam/Epic patterns
- Token validation works correctly (64-character check)
- Account info stored in user.preferences["psn"]
- Disconnect clears credentials properly
- Error handling provides clear user messages
- All tests pass

### Phase 4: Frontend Implementation

**Tasks**:
1. Add PSN configuration card to Settings page (following Steam/Epic pattern)
2. Create NPSSO token input modal with instructions
3. Add PSN sync section to Sync page
4. Implement token expiration warnings and re-configuration flow
5. Add TypeScript types for PSN API responses
6. Integrate with existing sync hooks/queries
7. Add user documentation for obtaining NPSSO token

**Acceptance Criteria**:
- PSN card appears in Settings page
- Configuration flow works end-to-end
- Sync trigger works from Sync page
- Token expiration handled gracefully in UI
- Clear instructions for obtaining NPSSO token
- Consistent with Steam/Epic UI patterns

### Phase 5: Testing & Documentation

**Tasks**:
1. End-to-end testing with real PSN account (manual)
2. Test multi-platform games (PS4+PS5)
3. Test token expiration and re-configuration
4. Update PRD with PSN implementation status
5. Write user-facing documentation for PSN sync
6. Add NPSSO token security warnings to docs
7. Document PS3 limitation (API limitation)

**Acceptance Criteria**:
- Full sync flow works end-to-end
- Multi-platform detection works correctly
- Token expiration flow tested and working
- Documentation complete and accurate
- Security warnings present
- Zero breaking changes to existing sync functionality

### Phase 6: Deployment

**Tasks**:
1. Verify PSNAWP in production dependencies
2. Test deployment in staging environment
3. Update changelog with PSN sync feature
4. Deploy to production
5. Monitor for errors and performance issues

**Acceptance Criteria**:
- PSNAWP installs correctly in production
- No deployment issues
- PSN sync works in production environment
- No performance degradation
- Error monitoring in place

## Security Considerations

### Token Security

- NPSSO token stored in user.preferences (JSON field)
- Never expose token in API responses
- Clear token on user deletion
- Log security-relevant events (configure, disconnect)

### API Security

- All endpoints require authentication
- Rate limit configuration endpoints to prevent abuse
- Validate token format before passing to PSNAWP
- HTTPS only in production

## Risks & Mitigations

### Risk: PSNAWP Library Changes

**Impact**: Library updates or changes break integration
**Mitigation**:
- Pin PSNAWP version in dependencies (>=2.1.0)
- Monitor PSNAWP releases for breaking changes
- Add integration tests that verify PSNAWP API
- Document PSNAWP version requirements

### Risk: PSN API Changes

**Impact**: PSNAWP stops working if Sony changes their API
**Mitigation**:
- PSNAWP maintainers typically update quickly
- Our adapter is isolated, can be updated independently
- Graceful degradation if sync fails
- Clear error messages to users

### Risk: Token Expiration Confusion

**Impact**: Users don't understand why sync stopped working
**Mitigation**:
- Clear documentation about token lifespan
- Prominent warning when token expires
- Simple re-configuration flow
- Consider future enhancement: automatic token refresh if possible

### Risk: PS3 Limitation Complaints

**Impact**: Users expect PS3 games in sync
**Mitigation**:
- Clear documentation about PS3 limitation
- Explain it's a PSN API limitation, not Nexorious
- Provide guidance on manual PS3 game addition
- Consider future enhancement if PSN adds PS3 to purchased games API

## Future Enhancements

### Trophy Data Sync

PSNAWP supports fetching trophy data. Future enhancement could:
- Sync trophy progress
- Display trophy completion percentage
- Show platinum trophies earned

### Playtime Sync

If PSN exposes playtime data in future, add to `ExternalGame.playtime_hours`.

### PS3 Support

If PSN adds PS3 games to purchased games API, platform detection will automatically include them.

### Automatic Token Refresh

Research if PSN supports OAuth refresh tokens for automatic renewal.

## Success Criteria

- [ ] Users can configure PSN with NPSSO token
- [ ] PSN library syncs automatically on trigger
- [ ] Games appear in collection with PlayStation platforms (PS4/PS5)
- [ ] Multi-platform games get both platform associations
- [ ] Token expiration handled gracefully with clear user prompts
- [ ] All backend tests pass with >80% coverage
- [ ] Zero breaking changes to existing sync functionality
- [ ] Documentation complete and accurate
- [ ] Follows Steam/Epic patterns consistently

## References

- [PSNAWP on PyPI](https://pypi.org/project/psnawp/)
- [PSNAWP GitHub Repository](https://github.com/isFakeAccount/psnawp)
- [PSNAWP Documentation](https://psnawp.readthedocs.io/)
- [PlayStation Network API Documentation](https://andshrew.github.io/PlayStation-Trophies/)
- [How to Get NPSSO Token](https://ca.account.sony.com/api/v1/ssocookie)
- Existing Steam sync implementation: `app/worker/tasks/sync/adapters/steam.py`
- Existing Epic sync implementation: `app/worker/tasks/sync/adapters/epic.py`
- Sync adapter protocol: `app/worker/tasks/sync/adapters/base.py`
