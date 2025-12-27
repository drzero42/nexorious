# Steam Sync Configuration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix Steam sync to require configuration before showing as connected, with proper credential verification UI.

**Architecture:** Backend adds verification endpoint and `is_configured` field to sync config response. Frontend adds `SteamConnectionCard` component with form, help accordions, and three-state badge logic.

**Tech Stack:** FastAPI, SQLModel, React, TanStack Query, shadcn/ui, Zod validation

---

## Task 1: Change Default `enabled` to `false`

**Files:**
- Modify: `backend/app/models/user_sync_config.py:53`
- Modify: `backend/app/api/sync.py:95-104` and `134-143`

**Step 1: Update model default**

In `backend/app/models/user_sync_config.py`, change line 53:

```python
enabled: bool = Field(default=False, description="Whether sync is enabled for this platform")
```

**Step 2: Update default config creation in get_sync_configs**

In `backend/app/api/sync.py`, change line 101:

```python
                enabled=False,
```

**Step 3: Update default config creation in get_sync_config**

In `backend/app/api/sync.py`, change line 140:

```python
        enabled=False,
```

**Step 4: Run backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/backend && uv run pytest app/tests/test_sync.py -v`

Expected: All tests pass (some may need updating if they expect `enabled=True`)

**Step 5: Commit**

```bash
git add backend/app/models/user_sync_config.py backend/app/api/sync.py
git commit -m "fix(sync): default enabled to false for new sync configs"
```

---

## Task 2: Add `is_configured` to Backend Schema and Response

**Files:**
- Modify: `backend/app/schemas/sync.py:31-48`
- Modify: `backend/app/api/sync.py:53-65` and add helper function

**Step 1: Add `is_configured` field to SyncConfigResponse**

In `backend/app/schemas/sync.py`, add field after line 48:

```python
class SyncConfigResponse(BaseModel):
    """Response model for a single sync configuration."""

    model_config = ConfigDict(from_attributes=True)

    id: str
    user_id: str
    platform: str
    frequency: SyncFrequency
    auto_add: bool = Field(
        description="If True, matched games are added automatically. If False, queued for review."
    )
    enabled: bool = Field(description="Whether sync is enabled for this platform")
    last_synced_at: Optional[datetime] = Field(
        default=None, description="Timestamp of last successful sync"
    )
    created_at: datetime
    updated_at: datetime
    is_configured: bool = Field(
        default=False,
        description="Whether platform credentials have been verified"
    )
```

**Step 2: Add helper function to check Steam configuration**

In `backend/app/api/sync.py`, add after the imports (around line 40):

```python
def _is_platform_configured(user: User, platform: str) -> bool:
    """Check if a platform has verified credentials configured."""
    if platform == "steam":
        preferences = user.preferences or {}
        steam_config = preferences.get("steam", {})
        return bool(
            steam_config.get("web_api_key")
            and steam_config.get("steam_id")
            and steam_config.get("is_verified", False)
        )
    # Other platforms not yet supported
    return False
```

**Step 3: Update `_config_to_response` to accept user parameter**

In `backend/app/api/sync.py`, modify the function:

```python
def _config_to_response(config: UserSyncConfig, user: User) -> SyncConfigResponse:
    """Convert UserSyncConfig model to response schema."""
    return SyncConfigResponse(
        id=config.id,
        user_id=config.user_id,
        platform=config.platform,
        frequency=FREQUENCY_MODEL_TO_SCHEMA[config.frequency],
        auto_add=config.auto_add,
        enabled=config.enabled,
        last_synced_at=config.last_synced_at,
        created_at=config.created_at,
        updated_at=config.updated_at,
        is_configured=_is_platform_configured(user, config.platform),
    )
```

**Step 4: Update all callers of `_config_to_response`**

Update `get_sync_configs` endpoint (around line 92):
```python
            configs_response.append(_config_to_response(config_map[platform.value], current_user))
```
And line 105:
```python
            configs_response.append(_config_to_response(default_config, current_user))
```

Update `get_sync_config` endpoint (around line 131):
```python
        return _config_to_response(config, current_user)
```
And line 144:
```python
    return _config_to_response(default_config, current_user)
```

Update `update_sync_config` endpoint (around line 198):
```python
    return _config_to_response(config, current_user)
```

**Step 5: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/backend && uv run pytest app/tests/test_sync.py -v`

Expected: PASS

**Step 6: Commit**

```bash
git add backend/app/schemas/sync.py backend/app/api/sync.py
git commit -m "feat(sync): add is_configured field to sync config response"
```

---

## Task 3: Add Steam Verify Endpoint

**Files:**
- Modify: `backend/app/api/sync.py` (add new endpoint)
- Modify: `backend/app/schemas/sync.py` (add request/response schemas)
- Create: `backend/app/tests/test_sync_steam_verify.py`

**Step 1: Add request/response schemas**

In `backend/app/schemas/sync.py`, add at the end:

```python
class SteamVerifyRequest(BaseModel):
    """Request model for verifying Steam credentials."""

    steam_id: str = Field(
        description="Steam ID (17 digits starting with 7656119)"
    )
    web_api_key: str = Field(
        description="Steam Web API key (32 alphanumeric characters)"
    )


class SteamVerifyResponse(BaseModel):
    """Response model for Steam verification result."""

    valid: bool = Field(description="Whether the credentials are valid")
    steam_username: Optional[str] = Field(
        default=None,
        description="Steam display name if verification succeeded"
    )
    error: Optional[str] = Field(
        default=None,
        description="Error code if verification failed"
    )
```

**Step 2: Add verification endpoint**

In `backend/app/api/sync.py`, add after the `get_sync_status` endpoint (around line 302):

```python
from ..schemas.sync import SteamVerifyRequest, SteamVerifyResponse
from ..services.steam import SteamService, SteamAPIError, SteamAuthenticationError
import re


@router.post("/steam/verify", response_model=SteamVerifyResponse)
async def verify_steam_credentials(
    request: SteamVerifyRequest,
    current_user: Annotated[User, Depends(get_current_user)],
) -> SteamVerifyResponse:
    """
    Verify Steam credentials before saving them.

    Validates format and tests the credentials against Steam Web API.
    Returns the Steam username on success for user confirmation.
    """
    logger.info(f"Verifying Steam credentials for user {current_user.id}")

    # Validate Steam ID format (17 digits starting with 7656119)
    if not re.match(r"^7656119\d{10}$", request.steam_id):
        return SteamVerifyResponse(
            valid=False,
            error="invalid_steam_id"
        )

    # Validate API key format (32 alphanumeric characters)
    if not re.match(r"^[A-Fa-f0-9]{32}$", request.web_api_key):
        return SteamVerifyResponse(
            valid=False,
            error="invalid_api_key"
        )

    # Test credentials against Steam API
    try:
        steam_service = SteamService(request.web_api_key)

        # Verify API key is valid
        if not await steam_service.verify_api_key():
            return SteamVerifyResponse(
                valid=False,
                error="invalid_api_key"
            )

        # Get user info to verify Steam ID and get username
        user_info = await steam_service.get_user_info(request.steam_id)

        if not user_info:
            return SteamVerifyResponse(
                valid=False,
                error="invalid_steam_id"
            )

        # Check if profile is public (communityvisibilitystate 3 = public)
        if user_info.community_visibility_state != 3:
            return SteamVerifyResponse(
                valid=False,
                error="private_profile"
            )

        logger.info(
            f"Steam credentials verified for user {current_user.id}: "
            f"Steam username '{user_info.persona_name}'"
        )

        return SteamVerifyResponse(
            valid=True,
            steam_username=user_info.persona_name
        )

    except SteamAuthenticationError:
        return SteamVerifyResponse(
            valid=False,
            error="invalid_api_key"
        )
    except SteamAPIError as e:
        logger.error(f"Steam API error during verification: {str(e)}")
        if "rate limit" in str(e).lower():
            return SteamVerifyResponse(
                valid=False,
                error="rate_limited"
            )
        return SteamVerifyResponse(
            valid=False,
            error="network_error"
        )
    except Exception as e:
        logger.error(f"Unexpected error during Steam verification: {str(e)}")
        return SteamVerifyResponse(
            valid=False,
            error="network_error"
        )
```

**Step 3: Write tests**

Create `backend/app/tests/test_sync_steam_verify.py`:

```python
"""Tests for Steam credential verification endpoint."""

import pytest
from unittest.mock import AsyncMock, patch, MagicMock
from httpx import AsyncClient

from app.services.steam import SteamUserInfo, SteamAPIError, SteamAuthenticationError


@pytest.mark.asyncio
class TestSteamVerify:
    """Tests for POST /sync/steam/verify endpoint."""

    async def test_verify_invalid_steam_id_format(
        self, async_client: AsyncClient, auth_headers: dict
    ):
        """Test verification fails with invalid Steam ID format."""
        response = await async_client.post(
            "/sync/steam/verify",
            json={"steam_id": "12345", "web_api_key": "A" * 32},
            headers=auth_headers,
        )
        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_steam_id"

    async def test_verify_invalid_api_key_format(
        self, async_client: AsyncClient, auth_headers: dict
    ):
        """Test verification fails with invalid API key format."""
        response = await async_client.post(
            "/sync/steam/verify",
            json={"steam_id": "76561198012345678", "web_api_key": "short"},
            headers=auth_headers,
        )
        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_api_key"

    async def test_verify_success(
        self, async_client: AsyncClient, auth_headers: dict
    ):
        """Test successful verification returns username."""
        mock_user_info = SteamUserInfo(
            steam_id="76561198012345678",
            persona_name="TestPlayer",
            profile_url="https://steamcommunity.com/profiles/76561198012345678",
            avatar="",
            avatar_medium="",
            avatar_full="",
            persona_state=1,
            community_visibility_state=3,  # Public
        )

        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service.verify_api_key = AsyncMock(return_value=True)
            mock_service.get_user_info = AsyncMock(return_value=mock_user_info)
            mock_service_class.return_value = mock_service

            response = await async_client.post(
                "/sync/steam/verify",
                json={"steam_id": "76561198012345678", "web_api_key": "A" * 32},
                headers=auth_headers,
            )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is True
        assert data["steam_username"] == "TestPlayer"
        assert data["error"] is None

    async def test_verify_private_profile(
        self, async_client: AsyncClient, auth_headers: dict
    ):
        """Test verification fails for private profile."""
        mock_user_info = SteamUserInfo(
            steam_id="76561198012345678",
            persona_name="TestPlayer",
            profile_url="",
            avatar="",
            avatar_medium="",
            avatar_full="",
            persona_state=1,
            community_visibility_state=1,  # Private
        )

        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service.verify_api_key = AsyncMock(return_value=True)
            mock_service.get_user_info = AsyncMock(return_value=mock_user_info)
            mock_service_class.return_value = mock_service

            response = await async_client.post(
                "/sync/steam/verify",
                json={"steam_id": "76561198012345678", "web_api_key": "A" * 32},
                headers=auth_headers,
            )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "private_profile"

    async def test_verify_invalid_api_key_from_steam(
        self, async_client: AsyncClient, auth_headers: dict
    ):
        """Test verification fails when Steam rejects API key."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service.verify_api_key = AsyncMock(return_value=False)
            mock_service_class.return_value = mock_service

            response = await async_client.post(
                "/sync/steam/verify",
                json={"steam_id": "76561198012345678", "web_api_key": "A" * 32},
                headers=auth_headers,
            )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_api_key"

    async def test_verify_steam_id_not_found(
        self, async_client: AsyncClient, auth_headers: dict
    ):
        """Test verification fails when Steam ID not found."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service.verify_api_key = AsyncMock(return_value=True)
            mock_service.get_user_info = AsyncMock(return_value=None)
            mock_service_class.return_value = mock_service

            response = await async_client.post(
                "/sync/steam/verify",
                json={"steam_id": "76561198012345678", "web_api_key": "A" * 32},
                headers=auth_headers,
            )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_steam_id"

    async def test_verify_requires_auth(self, async_client: AsyncClient):
        """Test verification requires authentication."""
        response = await async_client.post(
            "/sync/steam/verify",
            json={"steam_id": "76561198012345678", "web_api_key": "A" * 32},
        )
        assert response.status_code == 401
```

**Step 4: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/backend && uv run pytest app/tests/test_sync_steam_verify.py -v`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/api/sync.py backend/app/schemas/sync.py backend/app/tests/test_sync_steam_verify.py
git commit -m "feat(sync): add Steam credential verification endpoint"
```

---

## Task 4: Add Steam Disconnect Endpoint

**Files:**
- Modify: `backend/app/api/sync.py` (add new endpoint)
- Modify: `backend/app/tests/test_sync_steam_verify.py` (add disconnect tests)

**Step 1: Add disconnect endpoint**

In `backend/app/api/sync.py`, add after the verify endpoint:

```python
@router.delete("/steam/connection", response_model=SuccessResponse)
async def disconnect_steam(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SuccessResponse:
    """
    Disconnect Steam integration.

    Clears Steam credentials from user preferences and disables sync.
    """
    logger.info(f"Disconnecting Steam for user {current_user.id}")

    # Clear Steam credentials from preferences
    preferences = current_user.preferences or {}
    if "steam" in preferences:
        del preferences["steam"]
        current_user.preferences_json = json.dumps(preferences)

    # Disable Steam sync config if it exists
    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == current_user.id,
        UserSyncConfig.platform == "steam",
    )
    config = session.exec(stmt).first()

    if config:
        config.enabled = False
        config.updated_at = datetime.now(timezone.utc)

    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()

    logger.info(f"Steam disconnected for user {current_user.id}")

    return SuccessResponse(
        success=True,
        message="Steam disconnected successfully"
    )
```

Also add the import at the top:
```python
import json
```

**Step 2: Add tests**

Add to `backend/app/tests/test_sync_steam_verify.py`:

```python
@pytest.mark.asyncio
class TestSteamDisconnect:
    """Tests for DELETE /sync/steam/connection endpoint."""

    async def test_disconnect_clears_credentials(
        self, async_client: AsyncClient, auth_headers: dict, test_user, db_session
    ):
        """Test disconnect clears Steam credentials from preferences."""
        # Set up Steam credentials
        test_user.preferences_json = '{"steam": {"web_api_key": "test", "steam_id": "123", "is_verified": true}}'
        db_session.commit()

        response = await async_client.delete(
            "/sync/steam/connection",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # Verify credentials were cleared
        db_session.refresh(test_user)
        preferences = test_user.preferences
        assert "steam" not in preferences

    async def test_disconnect_disables_sync(
        self, async_client: AsyncClient, auth_headers: dict, test_user, db_session
    ):
        """Test disconnect disables Steam sync config."""
        from app.models.user_sync_config import UserSyncConfig

        # Create enabled sync config
        config = UserSyncConfig(
            user_id=test_user.id,
            platform="steam",
            enabled=True,
        )
        db_session.add(config)
        db_session.commit()

        response = await async_client.delete(
            "/sync/steam/connection",
            headers=auth_headers,
        )

        assert response.status_code == 200

        # Verify sync was disabled
        db_session.refresh(config)
        assert config.enabled is False

    async def test_disconnect_requires_auth(self, async_client: AsyncClient):
        """Test disconnect requires authentication."""
        response = await async_client.delete("/sync/steam/connection")
        assert response.status_code == 401
```

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/backend && uv run pytest app/tests/test_sync_steam_verify.py -v`

Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/api/sync.py backend/app/tests/test_sync_steam_verify.py
git commit -m "feat(sync): add Steam disconnect endpoint"
```

---

## Task 5: Update Frontend Types

**Files:**
- Modify: `frontend/src/types/sync.ts`

**Step 1: Add `isConfigured` to SyncConfig interface**

In `frontend/src/types/sync.ts`, update the `SyncConfig` interface:

```typescript
export interface SyncConfig {
  id: string;
  userId: string;
  platform: SyncPlatform;
  frequency: SyncFrequency;
  autoAdd: boolean;
  enabled: boolean;
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
  isConfigured: boolean;
}
```

**Step 2: Add Steam verification types**

Add at the end of `frontend/src/types/sync.ts`:

```typescript
export interface SteamVerifyRequest {
  steamId: string;
  webApiKey: string;
}

export interface SteamVerifyResponse {
  valid: boolean;
  steamUsername: string | null;
  error: string | null;
}

export interface SteamConnectionInfo {
  configured: boolean;
  steamId: string | null;
  steamUsername: string | null;
}

// Error message mapping for Steam verification
export const STEAM_VERIFY_ERROR_MESSAGES: Record<string, string> = {
  invalid_api_key: 'Invalid API key. Please check and try again.',
  invalid_steam_id: 'Steam ID not found. Please verify the number.',
  private_profile: 'Your Steam profile or game details are set to private. Please make them public and try again.',
  rate_limited: 'Steam API rate limit reached. Please try again in a few minutes.',
  network_error: 'Could not connect to Steam. Please try again.',
};
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check`

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/types/sync.ts
git commit -m "feat(sync): add frontend types for Steam configuration"
```

---

## Task 6: Update Frontend API Client

**Files:**
- Modify: `frontend/src/api/sync.ts`

**Step 1: Update API response type to include is_configured**

In `frontend/src/api/sync.ts`, update `SyncConfigApiResponse`:

```typescript
interface SyncConfigApiResponse {
  id: string;
  user_id: string;
  platform: string;
  frequency: string;
  auto_add: boolean;
  enabled: boolean;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
  is_configured: boolean;
}
```

**Step 2: Update transform function**

Update `transformSyncConfig`:

```typescript
function transformSyncConfig(apiConfig: SyncConfigApiResponse): SyncConfig {
  return {
    id: apiConfig.id,
    userId: apiConfig.user_id,
    platform: apiConfig.platform as SyncPlatform,
    frequency: apiConfig.frequency as SyncFrequency,
    autoAdd: apiConfig.auto_add,
    enabled: apiConfig.enabled,
    lastSyncedAt: apiConfig.last_synced_at,
    createdAt: apiConfig.created_at,
    updatedAt: apiConfig.updated_at,
    isConfigured: apiConfig.is_configured,
  };
}
```

**Step 3: Add Steam API types and functions**

Add at the end of `frontend/src/api/sync.ts`:

```typescript
// Steam verification types
interface SteamVerifyApiRequest {
  steam_id: string;
  web_api_key: string;
}

interface SteamVerifyApiResponse {
  valid: boolean;
  steam_username: string | null;
  error: string | null;
}

// Steam verification functions
export async function verifySteamCredentials(
  steamId: string,
  webApiKey: string
): Promise<SteamVerifyResponse> {
  const response = await api.post<SteamVerifyApiResponse>('/sync/steam/verify', {
    steam_id: steamId,
    web_api_key: webApiKey,
  } as SteamVerifyApiRequest);

  return {
    valid: response.valid,
    steamUsername: response.steam_username,
    error: response.error,
  };
}

export async function disconnectSteam(): Promise<void> {
  await api.delete('/sync/steam/connection');
}
```

Also add the import at the top:

```typescript
import type { SteamVerifyResponse } from '@/types';
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/api/sync.ts
git commit -m "feat(sync): add Steam API client functions"
```

---

## Task 7: Add Frontend Hooks for Steam Configuration

**Files:**
- Modify: `frontend/src/hooks/use-sync.ts`

**Step 1: Add mutation hooks**

In `frontend/src/hooks/use-sync.ts`, add after `useUnignoreGame`:

```typescript
/**
 * Hook to verify Steam credentials before saving.
 */
export function useVerifySteamCredentials() {
  return useMutation<
    SteamVerifyResponse,
    Error,
    { steamId: string; webApiKey: string }
  >({
    mutationFn: ({ steamId, webApiKey }) =>
      syncApi.verifySteamCredentials(steamId, webApiKey),
  });
}

/**
 * Hook to disconnect Steam integration.
 */
export function useDisconnectSteam() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, void>({
    mutationFn: () => syncApi.disconnectSteam(),
    onSuccess: () => {
      // Invalidate all sync-related queries
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
  });
}
```

Also add the import:

```typescript
import type { SteamVerifyResponse } from '@/types';
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check`

Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/hooks/use-sync.ts
git commit -m "feat(sync): add Steam configuration hooks"
```

---

## Task 8: Update Badge Logic in SyncServiceCard

**Files:**
- Modify: `frontend/src/components/sync/sync-service-card.tsx`

**Step 1: Update badge rendering**

In `frontend/src/components/sync/sync-service-card.tsx`, update the Badge component (around line 102-104):

```typescript
          <Badge
            variant={
              !config.isConfigured
                ? 'outline'
                : localEnabled
                  ? 'default'
                  : 'secondary'
            }
            className={
              !config.isConfigured
                ? 'bg-muted text-muted-foreground'
                : localEnabled
                  ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                  : 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
            }
          >
            {!config.isConfigured ? 'Not Configured' : localEnabled ? 'Enabled' : 'Disabled'}
          </Badge>
```

**Step 2: Disable controls when not configured**

Update the Switch for "Enable sync" (around line 112-116):

```typescript
          <Switch
            checked={localEnabled}
            onCheckedChange={handleEnabledChange}
            disabled={isUpdating || !config.isConfigured}
          />
```

Update the Select for frequency (around line 122-126):

```typescript
            disabled={!localEnabled || isUpdating || !config.isConfigured}
```

Update the Switch for auto-add (around line 143-147):

```typescript
            disabled={!localEnabled || isUpdating || !config.isConfigured}
```

Update the Sync Now button (around line 159-162):

```typescript
          disabled={!localEnabled || isCurrentlySyncing || !config.isConfigured}
```

**Step 3: Run type check and tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check && npm run test -- --run`

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/components/sync/sync-service-card.tsx
git commit -m "feat(sync): update badge states and disable controls when not configured"
```

---

## Task 9: Create SteamConnectionCard Component

**Files:**
- Create: `frontend/src/components/sync/steam-connection-card.tsx`

**Step 1: Create the component**

Create `frontend/src/components/sync/steam-connection-card.tsx`:

```typescript
'use client';

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Loader2, Check, ExternalLink } from 'lucide-react';
import { useVerifySteamCredentials, useDisconnectSteam } from '@/hooks';
import { useUpdateProfile } from '@/hooks/use-auth';
import { STEAM_VERIFY_ERROR_MESSAGES } from '@/types';

const steamCredentialsSchema = z.object({
  steamId: z
    .string()
    .min(17, 'Steam ID must be 17 digits')
    .max(17, 'Steam ID must be 17 digits')
    .regex(/^7656119\d{10}$/, 'Invalid Steam ID format'),
  webApiKey: z
    .string()
    .length(32, 'API key must be 32 characters')
    .regex(/^[A-Fa-f0-9]{32}$/, 'Invalid API key format'),
});

type SteamCredentialsForm = z.infer<typeof steamCredentialsSchema>;

interface SteamConnectionCardProps {
  isConfigured: boolean;
  enabled: boolean;
  steamId?: string;
  steamUsername?: string;
  onConnectionChange: () => void;
}

export function SteamConnectionCard({
  isConfigured,
  enabled,
  steamId,
  steamUsername,
  onConnectionChange,
}: SteamConnectionCardProps) {
  const [verifiedUsername, setVerifiedUsername] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<SteamCredentialsForm>({
    resolver: zodResolver(steamCredentialsSchema),
  });

  const verifyMutation = useVerifySteamCredentials();
  const disconnectMutation = useDisconnectSteam();
  const updateProfileMutation = useUpdateProfile();

  const isVerifying = verifyMutation.isPending || updateProfileMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;

  const onSubmit = async (data: SteamCredentialsForm) => {
    try {
      // First verify credentials with Steam API
      const result = await verifyMutation.mutateAsync({
        steamId: data.steamId,
        webApiKey: data.webApiKey,
      });

      if (!result.valid) {
        const errorMessage = result.error
          ? STEAM_VERIFY_ERROR_MESSAGES[result.error] || 'Verification failed'
          : 'Verification failed';

        if (result.error === 'invalid_steam_id') {
          setError('steamId', { message: errorMessage });
        } else if (result.error === 'invalid_api_key') {
          setError('webApiKey', { message: errorMessage });
        } else {
          toast.error(errorMessage);
        }
        return;
      }

      setVerifiedUsername(result.steamUsername);

      // Save credentials to user preferences
      await updateProfileMutation.mutateAsync({
        preferences: {
          steam: {
            steam_id: data.steamId,
            web_api_key: data.webApiKey,
            is_verified: true,
            username: result.steamUsername,
          },
        },
      });

      toast.success('Steam connected successfully');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to connect Steam');
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Steam disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect Steam');
    }
  };

  const getBadgeState = () => {
    if (!isConfigured) return { label: 'Not Configured', className: 'bg-muted text-muted-foreground' };
    if (enabled) return { label: 'Enabled', className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' };
    return { label: 'Disabled', className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400' };
  };

  const badgeState = getBadgeState();

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Steam Connection</CardTitle>
            <CardDescription>
              {isConfigured
                ? 'Your Steam account is connected'
                : 'Connect your Steam account to sync your game library'}
            </CardDescription>
          </div>
          <Badge variant="outline" className={badgeState.className}>
            {badgeState.label}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        {isConfigured ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
              <Check className="h-5 w-5 text-green-600" />
              <div>
                <p className="font-medium">
                  Connected as {steamUsername || verifiedUsername}
                </p>
                {steamId && (
                  <p className="text-sm text-muted-foreground">{steamId}</p>
                )}
              </div>
            </div>

            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="outline" disabled={isDisconnecting}>
                  {isDisconnecting ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Disconnecting...
                    </>
                  ) : (
                    'Disconnect'
                  )}
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Disconnect Steam?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Your sync settings will be preserved but syncing will stop until you reconnect.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleDisconnect}>
                    Disconnect
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="steamId">Steam ID</Label>
              <Input
                id="steamId"
                placeholder="76561198012345678"
                {...register('steamId')}
                disabled={isVerifying}
              />
              {errors.steamId && (
                <p className="text-sm text-destructive">{errors.steamId.message}</p>
              )}

              <Accordion type="single" collapsible className="w-full">
                <AccordionItem value="steam-id-help" className="border-none">
                  <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                    How do I find my Steam ID?
                  </AccordionTrigger>
                  <AccordionContent className="text-sm text-muted-foreground">
                    <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                      <p className="font-medium text-foreground">
                        Your Steam ID is a 17-digit number that uniquely identifies your account.
                      </p>
                      <ol className="list-inside list-decimal space-y-1">
                        <li>Open Steam and go to your <strong>Profile</strong></li>
                        <li>Look at the URL:
                          <ul className="ml-4 list-inside list-disc">
                            <li>If it shows <code>steamcommunity.com/profiles/76561198...</code>, that number is your Steam ID</li>
                            <li>If it shows <code>steamcommunity.com/id/customname/</code>, you have a custom URL</li>
                          </ul>
                        </li>
                        <li>
                          <strong>If you have a custom URL:</strong> Go to{' '}
                          <a
                            href="https://steamid.io"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline"
                          >
                            steamid.io <ExternalLink className="inline h-3 w-3" />
                          </a>
                          , paste your profile URL, and copy the <strong>steamID64</strong> value
                        </li>
                      </ol>
                      <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                        <strong>Important:</strong> Your Steam profile must be set to <strong>Public</strong> for sync to work.
                        <ol className="ml-4 mt-1 list-inside list-decimal">
                          <li>Go to Steam → Settings → Privacy Settings</li>
                          <li>Set "My profile" to Public</li>
                          <li>Set "Game details" to Public</li>
                        </ol>
                      </div>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </div>

            <div className="space-y-2">
              <Label htmlFor="webApiKey">Steam Web API Key</Label>
              <Input
                id="webApiKey"
                type="password"
                placeholder="••••••••••••••••••••••••••••••••"
                {...register('webApiKey')}
                disabled={isVerifying}
              />
              {errors.webApiKey && (
                <p className="text-sm text-destructive">{errors.webApiKey.message}</p>
              )}

              <Accordion type="single" collapsible className="w-full">
                <AccordionItem value="api-key-help" className="border-none">
                  <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                    How do I get an API key?
                  </AccordionTrigger>
                  <AccordionContent className="text-sm text-muted-foreground">
                    <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                      <p className="font-medium text-foreground">
                        A Steam Web API key allows Nexorious to read your game library.
                      </p>
                      <ol className="list-inside list-decimal space-y-1">
                        <li>
                          Go to{' '}
                          <a
                            href="https://steamcommunity.com/dev/apikey"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline"
                          >
                            Steam Web API Key Registration <ExternalLink className="inline h-3 w-3" />
                          </a>
                        </li>
                        <li>Sign in with your Steam account if prompted</li>
                        <li>Enter a domain name (you can use <code>localhost</code> or any domain)</li>
                        <li>Click <strong>Register</strong> and copy the 32-character key</li>
                      </ol>
                      <p className="mt-2 text-xs">
                        <strong>Note:</strong> Keep your API key private. It's stored securely and only used to sync your library.
                      </p>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </div>

            <Button type="submit" disabled={isVerifying} className="w-full">
              {isVerifying ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Verifying...
                </>
              ) : (
                'Verify & Connect'
              )}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
```

**Step 2: Export from index**

Add to `frontend/src/components/sync/index.ts` (create if doesn't exist):

```typescript
export { SyncServiceCard } from './sync-service-card';
export { SteamConnectionCard } from './steam-connection-card';
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check`

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/components/sync/
git commit -m "feat(sync): add SteamConnectionCard component"
```

---

## Task 10: Check for useUpdateProfile Hook

**Files:**
- Check: `frontend/src/hooks/use-auth.ts`
- Possibly modify if hook doesn't exist

**Step 1: Check if useUpdateProfile exists**

Read `frontend/src/hooks/use-auth.ts` and check if `useUpdateProfile` hook exists.

If it doesn't exist, add it:

```typescript
/**
 * Hook to update user profile.
 */
export function useUpdateProfile() {
  const queryClient = useQueryClient();

  return useMutation<User, Error, { preferences?: Record<string, unknown> }>({
    mutationFn: (data) => authApi.updateProfile(data),
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.me(), user);
    },
  });
}
```

And ensure the API function exists in `frontend/src/api/auth.ts`:

```typescript
export async function updateProfile(data: { preferences?: Record<string, unknown> }): Promise<User> {
  return api.put<User>('/auth/me', data);
}
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check`

Expected: PASS

**Step 3: Commit if changes were made**

```bash
git add frontend/src/hooks/use-auth.ts frontend/src/api/auth.ts
git commit -m "feat(auth): add useUpdateProfile hook"
```

---

## Task 11: Update Steam Sync Detail Page

**Files:**
- Modify: `frontend/src/app/(main)/sync/[platform]/page.tsx`

**Step 1: Import SteamConnectionCard**

Add import at top:

```typescript
import { SteamConnectionCard } from '@/components/sync/steam-connection-card';
```

**Step 2: Get Steam credentials from user preferences**

We need to access user preferences. Add the useCurrentUser hook:

```typescript
import { useCurrentUser } from '@/hooks/use-auth';
```

Inside the component, add:

```typescript
  const { data: currentUser } = useCurrentUser();

  // Extract Steam credentials from user preferences
  const steamPrefs = currentUser?.preferences?.steam as {
    steam_id?: string;
    username?: string;
  } | undefined;
```

**Step 3: Add SteamConnectionCard to the page**

After the "Platform Header" Card and before "Active Sync Progress", add:

```typescript
      {/* Steam Connection Card - only show for Steam platform */}
      {platform === SyncPlatform.STEAM && (
        <SteamConnectionCard
          isConfigured={config.isConfigured}
          enabled={effectiveEnabled}
          steamId={steamPrefs?.steam_id}
          steamUsername={steamPrefs?.username}
          onConnectionChange={() => {
            // Invalidate queries to refresh data
            queryClient.invalidateQueries({ queryKey: syncKeys.config(platform) });
            queryClient.invalidateQueries({ queryKey: ['auth', 'me'] });
          }}
        />
      )}
```

Also import queryClient and syncKeys:

```typescript
import { useQueryClient } from '@tanstack/react-query';
import { syncKeys } from '@/hooks';
```

And inside component:

```typescript
  const queryClient = useQueryClient();
```

**Step 4: Update badge in Platform Header**

Update the Badge in the Platform Header (around line 380):

```typescript
              <Badge
                variant="outline"
                className={
                  !config.isConfigured
                    ? 'bg-muted text-muted-foreground'
                    : effectiveEnabled
                      ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                      : 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
                }
              >
                {!config.isConfigured ? 'Not Configured' : effectiveEnabled ? 'Enabled' : 'Disabled'}
              </Badge>
```

**Step 5: Disable controls when not configured**

Update the "Sync Now" button in the header (around line 346):

```typescript
        <Button onClick={handleTriggerSync} disabled={!effectiveEnabled || isSyncing || !config.isConfigured}>
```

Update the Switch in Configuration section (around line 419):

```typescript
            <Switch
              checked={effectiveEnabled}
              onCheckedChange={handleEnabledChange}
              disabled={isUpdating || !config.isConfigured}
            />
```

Update the Select for frequency (around line 434):

```typescript
            disabled={!effectiveEnabled || isUpdating || !config.isConfigured}
```

Update the auto-add Switch (around line 460):

```typescript
            disabled={!effectiveEnabled || isUpdating || !config.isConfigured}
```

**Step 6: Run type check and tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check && npm run test -- --run`

Expected: PASS

**Step 7: Commit**

```bash
git add frontend/src/app/(main)/sync/[platform]/page.tsx
git commit -m "feat(sync): integrate SteamConnectionCard into Steam sync page"
```

---

## Task 12: Write Frontend Tests

**Files:**
- Create: `frontend/src/components/sync/steam-connection-card.test.tsx`

**Step 1: Create test file**

Create `frontend/src/components/sync/steam-connection-card.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SteamConnectionCard } from './steam-connection-card';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

// Mock the hooks
vi.mock('@/hooks', () => ({
  useVerifySteamCredentials: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
  useDisconnectSteam: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
}));

vi.mock('@/hooks/use-auth', () => ({
  useUpdateProfile: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
}));

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe('SteamConnectionCard', () => {
  const defaultProps = {
    isConfigured: false,
    enabled: false,
    onConnectionChange: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders "Not Configured" badge when not configured', () => {
    render(<SteamConnectionCard {...defaultProps} />, { wrapper: createWrapper() });
    expect(screen.getByText('Not Configured')).toBeInTheDocument();
  });

  it('renders "Enabled" badge when configured and enabled', () => {
    render(
      <SteamConnectionCard
        {...defaultProps}
        isConfigured={true}
        enabled={true}
        steamUsername="TestPlayer"
        steamId="76561198012345678"
      />,
      { wrapper: createWrapper() }
    );
    expect(screen.getByText('Enabled')).toBeInTheDocument();
  });

  it('renders "Disabled" badge when configured but not enabled', () => {
    render(
      <SteamConnectionCard
        {...defaultProps}
        isConfigured={true}
        enabled={false}
        steamUsername="TestPlayer"
      />,
      { wrapper: createWrapper() }
    );
    expect(screen.getByText('Disabled')).toBeInTheDocument();
  });

  it('shows connection form when not configured', () => {
    render(<SteamConnectionCard {...defaultProps} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText('Steam ID')).toBeInTheDocument();
    expect(screen.getByLabelText('Steam Web API Key')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /verify & connect/i })).toBeInTheDocument();
  });

  it('shows connected state when configured', () => {
    render(
      <SteamConnectionCard
        {...defaultProps}
        isConfigured={true}
        enabled={true}
        steamUsername="TestPlayer"
        steamId="76561198012345678"
      />,
      { wrapper: createWrapper() }
    );
    expect(screen.getByText(/connected as testplayer/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /disconnect/i })).toBeInTheDocument();
  });

  it('shows help accordions', async () => {
    render(<SteamConnectionCard {...defaultProps} />, { wrapper: createWrapper() });

    const steamIdHelp = screen.getByText('How do I find my Steam ID?');
    expect(steamIdHelp).toBeInTheDocument();

    const apiKeyHelp = screen.getByText('How do I get an API key?');
    expect(apiKeyHelp).toBeInTheDocument();
  });

  it('validates Steam ID format', async () => {
    const user = userEvent.setup();
    render(<SteamConnectionCard {...defaultProps} />, { wrapper: createWrapper() });

    const steamIdInput = screen.getByLabelText('Steam ID');
    await user.type(steamIdInput, '12345');

    const submitButton = screen.getByRole('button', { name: /verify & connect/i });
    await user.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/steam id must be 17 digits/i)).toBeInTheDocument();
    });
  });

  it('validates API key format', async () => {
    const user = userEvent.setup();
    render(<SteamConnectionCard {...defaultProps} />, { wrapper: createWrapper() });

    const steamIdInput = screen.getByLabelText('Steam ID');
    const apiKeyInput = screen.getByLabelText('Steam Web API Key');

    await user.type(steamIdInput, '76561198012345678');
    await user.type(apiKeyInput, 'short');

    const submitButton = screen.getByRole('button', { name: /verify & connect/i });
    await user.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/api key must be 32 characters/i)).toBeInTheDocument();
    });
  });
});
```

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run test -- --run src/components/sync/steam-connection-card.test.tsx`

Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/components/sync/steam-connection-card.test.tsx
git commit -m "test(sync): add SteamConnectionCard tests"
```

---

## Task 13: Run Full Test Suite and Type Checks

**Files:** None (verification task)

**Step 1: Run backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/backend && uv run pytest --cov=app --cov-report=term-missing`

Expected: All tests pass with >80% coverage

**Step 2: Run backend type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/backend && uv run pyrefly check`

Expected: No errors

**Step 3: Run frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run test -- --run`

Expected: All tests pass

**Step 4: Run frontend type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/steam-sync-config/frontend && npm run check`

Expected: No errors

---

## Task 14: Final Commit and Summary

**Step 1: Verify all changes are committed**

Run: `git status`

Expected: Clean working directory

**Step 2: Create summary commit if needed**

If there are any uncommitted changes:

```bash
git add -A
git commit -m "chore: cleanup and finalize Steam sync configuration feature"
```

---

## Summary of Changes

### Backend
- Changed default `enabled` to `false` for new sync configs
- Added `is_configured` field to `SyncConfigResponse`
- Added `POST /sync/steam/verify` endpoint for credential verification
- Added `DELETE /sync/steam/connection` endpoint for disconnecting Steam

### Frontend
- Added `isConfigured` to `SyncConfig` type
- Added Steam verification types and error messages
- Added `verifySteamCredentials` and `disconnectSteam` API functions
- Added `useVerifySteamCredentials` and `useDisconnectSteam` hooks
- Updated badge logic to show three states: "Not Configured", "Enabled", "Disabled"
- Created `SteamConnectionCard` component with form and help accordions
- Integrated `SteamConnectionCard` into Steam sync detail page
- Disabled sync controls when Steam is not configured
