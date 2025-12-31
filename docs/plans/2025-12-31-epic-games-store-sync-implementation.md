# Epic Games Store Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Epic Games Store as a sync source using legendary CLI, enabling users to automatically import their Epic library into Nexorious collection.

**Architecture:** Server-side sync using legendary CLI as external subprocess. Follow existing Steam adapter pattern with `EpicService` for subprocess management, `EpicSyncAdapter` for sync protocol, and new API endpoints for authentication flow. Multi-user isolation via `XDG_CONFIG_HOME` per user.

**Tech Stack:** Python 3.13, FastAPI, legendary CLI (external), asyncio subprocess, existing sync infrastructure

---

## Task 1: Create Epic Service Foundation

**Files:**
- Create: `backend/app/services/epic.py`
- Create: `backend/app/tests/test_epic_service.py`

**Step 1: Write test for EpicService initialization**

Create `backend/app/tests/test_epic_service.py`:

```python
"""Tests for Epic Games Store service using legendary CLI."""

import pytest
from app.services.epic import EpicService


class TestEpicService:
    """Test EpicService initialization and config path setup."""

    def test_service_initialization(self):
        """Test EpicService creates with correct user config path."""
        user_id = "test-user-123"
        service = EpicService(user_id)

        assert service.user_id == user_id
        assert service.config_path == f"/var/lib/nexorious/legendary-configs/{user_id}"
```

**Step 2: Run test to verify it fails**

```bash
cd backend
uv run pytest app/tests/test_epic_service.py::TestEpicService::test_service_initialization -v
```

Expected: FAIL with "ModuleNotFoundError: No module named 'app.services.epic'"

**Step 3: Write minimal EpicService implementation**

Create `backend/app/services/epic.py`:

```python
"""
Epic Games Store service using legendary CLI.

Provides Epic authentication and library fetching via legendary subprocess calls.
All legendary commands run with isolated XDG_CONFIG_HOME per user.
"""

import logging
from typing import Optional

logger = logging.getLogger(__name__)


class EpicService:
    """Service for interacting with Epic Games Store via legendary CLI.

    Args:
        user_id: User's unique identifier for config isolation
    """

    def __init__(self, user_id: str):
        """Initialize Epic service with user-specific config path."""
        self.user_id = user_id
        self.config_path = f"/var/lib/nexorious/legendary-configs/{user_id}"
        logger.debug(f"EpicService initialized for user {user_id}")
```

**Step 4: Run test to verify it passes**

```bash
uv run pytest app/tests/test_epic_service.py::TestEpicService::test_service_initialization -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add app/services/epic.py app/tests/test_epic_service.py
git commit -m "feat: add EpicService foundation with config isolation"
```

---

## Task 2: Add Custom Exceptions

**Files:**
- Modify: `backend/app/services/epic.py`
- Modify: `backend/app/tests/test_epic_service.py`

**Step 1: Write test for custom exceptions**

Add to `backend/app/tests/test_epic_service.py`:

```python
from app.services.epic import (
    LegendaryNotFoundError,
    EpicAuthenticationError,
    EpicAuthExpiredError,
    EpicAPIError,
)


class TestEpicExceptions:
    """Test Epic service custom exceptions."""

    def test_legendary_not_found_error(self):
        """Test LegendaryNotFoundError can be raised and caught."""
        with pytest.raises(LegendaryNotFoundError) as exc_info:
            raise LegendaryNotFoundError("legendary not found")
        assert "legendary not found" in str(exc_info.value)

    def test_epic_authentication_error(self):
        """Test EpicAuthenticationError can be raised and caught."""
        with pytest.raises(EpicAuthenticationError) as exc_info:
            raise EpicAuthenticationError("auth failed")
        assert "auth failed" in str(exc_info.value)

    def test_epic_auth_expired_error(self):
        """Test EpicAuthExpiredError can be raised and caught."""
        with pytest.raises(EpicAuthExpiredError) as exc_info:
            raise EpicAuthExpiredError("auth expired")
        assert "auth expired" in str(exc_info.value)

    def test_epic_api_error(self):
        """Test EpicAPIError can be raised and caught."""
        with pytest.raises(EpicAPIError) as exc_info:
            raise EpicAPIError("api error")
        assert "api error" in str(exc_info.value)
```

**Step 2: Run test to verify it fails**

```bash
uv run pytest app/tests/test_epic_service.py::TestEpicExceptions -v
```

Expected: FAIL with "ImportError: cannot import name 'LegendaryNotFoundError'"

**Step 3: Implement custom exceptions**

Add to `backend/app/services/epic.py` (after imports):

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

**Step 4: Run test to verify it passes**

```bash
uv run pytest app/tests/test_epic_service.py::TestEpicExceptions -v
```

Expected: PASS (4 tests)

**Step 5: Commit**

```bash
git add app/services/epic.py app/tests/test_epic_service.py
git commit -m "feat: add Epic service custom exceptions"
```

---

## Task 3: Implement legendary Subprocess Runner

**Files:**
- Modify: `backend/app/services/epic.py`
- Modify: `backend/app/tests/test_epic_service.py`

**Step 1: Write test for subprocess runner**

Add to `backend/app/tests/test_epic_service.py`:

```python
import asyncio
from unittest.mock import AsyncMock, patch, MagicMock


class TestLegendarySubprocess:
    """Test legendary subprocess execution."""

    @pytest.mark.asyncio
    async def test_run_legendary_command_success(self):
        """Test successful legendary command execution."""
        service = EpicService("test-user")

        # Mock successful subprocess
        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.stdout = b'{"status": "ok"}'
        mock_process.stderr = b''

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process
            mock_process.communicate = AsyncMock(return_value=(b'{"status": "ok"}', b''))

            result = await service._run_legendary_command(["status", "--json"])

            assert result == {"stdout": '{"status": "ok"}', "stderr": "", "returncode": 0}

            # Verify XDG_CONFIG_HOME was set
            call_args = mock_exec.call_args
            assert "XDG_CONFIG_HOME" in call_args[1]["env"]
            assert call_args[1]["env"]["XDG_CONFIG_HOME"] == service.config_path

    @pytest.mark.asyncio
    async def test_run_legendary_command_not_found(self):
        """Test legendary not found error."""
        service = EpicService("test-user")

        with patch('asyncio.create_subprocess_exec', side_effect=FileNotFoundError()):
            with pytest.raises(LegendaryNotFoundError) as exc_info:
                await service._run_legendary_command(["status"])

            assert "legendary CLI not found" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_run_legendary_command_auth_expired(self):
        """Test detection of expired authentication."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 1
        mock_process.communicate = AsyncMock(
            return_value=(b'', b'You are not authenticated')
        )

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            with pytest.raises(EpicAuthExpiredError) as exc_info:
                await service._run_legendary_command(["list"])

            assert "authentication expired" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_run_legendary_command_generic_error(self):
        """Test generic command failure."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 1
        mock_process.communicate = AsyncMock(
            return_value=(b'', b'Some other error')
        )

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            with pytest.raises(EpicAPIError) as exc_info:
                await service._run_legendary_command(["list"])

            assert "legendary command failed" in str(exc_info.value)
```

**Step 2: Run test to verify it fails**

```bash
uv run pytest app/tests/test_epic_service.py::TestLegendarySubprocess -v
```

Expected: FAIL with "AttributeError: 'EpicService' object has no attribute '_run_legendary_command'"

**Step 3: Implement subprocess runner**

Add to `backend/app/services/epic.py` in EpicService class:

```python
import asyncio
import os
from typing import Dict, List


class EpicService:
    # ... existing __init__ ...

    async def _run_legendary_command(
        self, args: List[str], timeout: int = 60
    ) -> Dict[str, any]:
        """Run legendary CLI command with isolated config.

        Args:
            args: Command arguments (e.g., ["status", "--json"])
            timeout: Command timeout in seconds

        Returns:
            Dict with stdout, stderr, and returncode

        Raises:
            LegendaryNotFoundError: legendary CLI not found
            EpicAuthExpiredError: Authentication expired
            EpicAPIError: Command failed
        """
        # Set isolated config path via XDG_CONFIG_HOME
        env = os.environ.copy()
        env['XDG_CONFIG_HOME'] = self.config_path

        try:
            process = await asyncio.create_subprocess_exec(
                'legendary',
                *args,
                env=env,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )

            stdout, stderr = await asyncio.wait_for(
                process.communicate(), timeout=timeout
            )

            stdout_str = stdout.decode('utf-8')
            stderr_str = stderr.decode('utf-8')

            # Check for auth expiration
            if process.returncode != 0:
                stderr_lower = stderr_str.lower()
                if any(phrase in stderr_lower for phrase in [
                    'not authenticated',
                    'login',
                    'expired',
                    'authentication required'
                ]):
                    logger.warning(f"Epic authentication expired for user {self.user_id}")
                    raise EpicAuthExpiredError("Epic authentication expired")

                # Generic command failure
                logger.error(f"legendary command failed: {stderr_str}")
                raise EpicAPIError(f"legendary command failed: {stderr_str}")

            return {
                "stdout": stdout_str,
                "stderr": stderr_str,
                "returncode": process.returncode
            }

        except FileNotFoundError:
            logger.error("legendary CLI not found on system")
            raise LegendaryNotFoundError(
                "legendary CLI not found. Install with: pip install legendary-gl"
            )
        except asyncio.TimeoutError:
            logger.error(f"legendary command timed out after {timeout}s")
            raise EpicAPIError(f"legendary command timed out after {timeout}s")
```

**Step 4: Run test to verify it passes**

```bash
uv run pytest app/tests/test_epic_service.py::TestLegendarySubprocess -v
```

Expected: PASS (4 tests)

**Step 5: Commit**

```bash
git add app/services/epic.py app/tests/test_epic_service.py
git commit -m "feat: implement legendary subprocess runner with error handling"
```

---

## Task 4: Implement Authentication Methods

**Files:**
- Modify: `backend/app/services/epic.py`
- Modify: `backend/app/tests/test_epic_service.py`

**Step 1: Write tests for auth methods**

Add to `backend/app/tests/test_epic_service.py`:

```python
from pydantic import BaseModel


class TestEpicAuthentication:
    """Test Epic authentication flow."""

    @pytest.mark.asyncio
    async def test_start_device_auth_success(self):
        """Test starting device authentication flow."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.communicate = AsyncMock(return_value=(
            b'Please visit https://www.epicgames.com/activate\nand enter code: ABC123',
            b''
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            auth_url = await service.start_device_auth()

            assert "epicgames.com" in auth_url
            # Verify legendary auth was called
            assert mock_exec.call_args[0][1] == 'auth'

    @pytest.mark.asyncio
    async def test_complete_auth_success(self):
        """Test completing authentication with code."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.communicate = AsyncMock(return_value=(
            b'Successfully logged in as TestUser',
            b''
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            success = await service.complete_auth("ABC123")

            assert success is True
            # Verify legendary auth --code was called
            assert mock_exec.call_args[0][1] == 'auth'
            assert '--code' in mock_exec.call_args[0]
            assert 'ABC123' in mock_exec.call_args[0]

    @pytest.mark.asyncio
    async def test_complete_auth_invalid_code(self):
        """Test completing authentication with invalid code."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 1
        mock_process.communicate = AsyncMock(return_value=(
            b'',
            b'Invalid exchange code'
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            with pytest.raises(EpicAuthenticationError):
                await service.complete_auth("INVALID")

    @pytest.mark.asyncio
    async def test_verify_auth_authenticated(self):
        """Test verifying authentication when logged in."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.communicate = AsyncMock(return_value=(
            b'{"account": "test@example.com", "logged_in": true}',
            b''
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            is_authed = await service.verify_auth()

            assert is_authed is True

    @pytest.mark.asyncio
    async def test_verify_auth_not_authenticated(self):
        """Test verifying authentication when not logged in."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 1
        mock_process.communicate = AsyncMock(return_value=(
            b'',
            b'You are not authenticated'
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            is_authed = await service.verify_auth()

            assert is_authed is False

    @pytest.mark.asyncio
    async def test_get_account_info_success(self):
        """Test getting account information."""
        service = EpicService("test-user")

        mock_status = {
            "account": {"displayName": "TestUser", "id": "test123"}
        }

        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.communicate = AsyncMock(return_value=(
            b'{"account": {"displayName": "TestUser", "id": "test123"}}',
            b''
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            account_info = await service.get_account_info()

            assert account_info.display_name == "TestUser"
            assert account_info.account_id == "test123"

    @pytest.mark.asyncio
    async def test_disconnect_success(self):
        """Test disconnecting Epic account."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.communicate = AsyncMock(return_value=(
            b'Successfully logged out',
            b''
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            await service.disconnect()

            # Verify legendary auth --delete was called
            assert mock_exec.call_args[0][1] == 'auth'
            assert '--delete' in mock_exec.call_args[0]
```

**Step 2: Run test to verify it fails**

```bash
uv run pytest app/tests/test_epic_service.py::TestEpicAuthentication -v
```

Expected: FAIL with various "AttributeError" for missing methods

**Step 3: Add data models**

Add to `backend/app/services/epic.py` (after exceptions):

```python
import json
import re
from pydantic import BaseModel


class EpicAccountInfo(BaseModel):
    """Epic account information."""
    display_name: str
    account_id: str
```

**Step 4: Implement auth methods**

Add to `backend/app/services/epic.py` in EpicService class:

```python
    async def start_device_auth(self) -> str:
        """Start Epic device authentication flow.

        Returns:
            Authentication URL for user to visit

        Raises:
            EpicAPIError: Command failed
        """
        logger.info(f"Starting Epic device auth for user {self.user_id}")

        result = await self._run_legendary_command(["auth"], timeout=30)
        stdout = result["stdout"]

        # Extract URL from legendary output
        # legendary outputs: "Please visit https://... and enter code: ..."
        url_match = re.search(r'https://[^\s]+', stdout)
        if url_match:
            auth_url = url_match.group(0)
            logger.info(f"Generated Epic auth URL for user {self.user_id}")
            return auth_url

        # Fallback to generic Epic activate URL
        logger.warning("Could not extract URL from legendary output, using default")
        return "https://www.epicgames.com/activate"

    async def complete_auth(self, code: str) -> bool:
        """Complete authentication with authorization code.

        Args:
            code: Authorization code from Epic

        Returns:
            True if authentication successful

        Raises:
            EpicAuthenticationError: Invalid code or auth failed
        """
        logger.info(f"Completing Epic auth for user {self.user_id}")

        try:
            await self._run_legendary_command(
                ["auth", "--code", code], timeout=30
            )
            logger.info(f"Epic auth completed for user {self.user_id}")
            return True
        except EpicAPIError as e:
            # Re-raise as authentication error for invalid codes
            logger.error(f"Epic auth failed for user {self.user_id}: {e}")
            raise EpicAuthenticationError(f"Invalid authorization code: {e}")

    async def verify_auth(self) -> bool:
        """Verify if user is authenticated with Epic.

        Returns:
            True if authenticated, False otherwise
        """
        try:
            result = await self._run_legendary_command(
                ["status", "--json", "--offline"], timeout=10
            )

            # Parse JSON response
            status_data = json.loads(result["stdout"])

            # Check if logged in (structure may vary, be defensive)
            if isinstance(status_data, dict):
                return status_data.get("logged_in", False) or bool(status_data.get("account"))

            return False

        except (EpicAuthExpiredError, EpicAPIError, json.JSONDecodeError):
            # Any error means not authenticated
            return False

    async def get_account_info(self) -> EpicAccountInfo:
        """Get Epic account information.

        Returns:
            Epic account info with display name and ID

        Raises:
            EpicAuthExpiredError: Not authenticated
            EpicAPIError: Command failed
        """
        logger.info(f"Getting Epic account info for user {self.user_id}")

        result = await self._run_legendary_command(
            ["status", "--json", "--offline"], timeout=10
        )

        status_data = json.loads(result["stdout"])

        # Extract account info from status
        account = status_data.get("account", {})
        display_name = account.get("displayName", "Unknown")
        account_id = account.get("id", "")

        return EpicAccountInfo(
            display_name=display_name,
            account_id=account_id
        )

    async def disconnect(self) -> None:
        """Disconnect Epic account and remove authentication.

        Raises:
            EpicAPIError: Command failed
        """
        logger.info(f"Disconnecting Epic for user {self.user_id}")

        await self._run_legendary_command(["auth", "--delete"], timeout=10)

        logger.info(f"Epic disconnected for user {self.user_id}")
```

**Step 5: Run test to verify it passes**

```bash
uv run pytest app/tests/test_epic_service.py::TestEpicAuthentication -v
```

Expected: PASS (8 tests)

**Step 6: Commit**

```bash
git add app/services/epic.py app/tests/test_epic_service.py
git commit -m "feat: implement Epic authentication methods"
```

---

## Task 5: Implement Library Fetching

**Files:**
- Modify: `backend/app/services/epic.py`
- Modify: `backend/app/tests/test_epic_service.py`

**Step 1: Write tests for library fetching**

Add to `backend/app/tests/test_epic_service.py`:

```python
class TestEpicLibrary:
    """Test Epic library fetching."""

    @pytest.mark.asyncio
    async def test_get_library_success(self):
        """Test fetching Epic library."""
        service = EpicService("test-user")

        mock_games = [
            {"app_name": "Fortnite", "app_title": "Fortnite"},
            {"app_name": "RocketLeague", "app_title": "Rocket League"}
        ]

        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.communicate = AsyncMock(return_value=(
            json.dumps(mock_games).encode('utf-8'),
            b''
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            games = await service.get_library()

            assert len(games) == 2
            assert games[0].app_name == "Fortnite"
            assert games[0].title == "Fortnite"
            assert games[1].app_name == "RocketLeague"
            assert games[1].title == "Rocket League"

    @pytest.mark.asyncio
    async def test_get_library_empty(self):
        """Test fetching empty Epic library."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.communicate = AsyncMock(return_value=(
            b'[]',
            b''
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            games = await service.get_library()

            assert len(games) == 0

    @pytest.mark.asyncio
    async def test_get_library_auth_expired(self):
        """Test fetching library with expired auth."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 1
        mock_process.communicate = AsyncMock(return_value=(
            b'',
            b'You are not authenticated'
        ))

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            with pytest.raises(EpicAuthExpiredError):
                await service.get_library()
```

**Step 2: Run test to verify it fails**

```bash
uv run pytest app/tests/test_epic_service.py::TestEpicLibrary -v
```

Expected: FAIL with "AttributeError: 'EpicService' object has no attribute 'get_library'"

**Step 3: Add EpicGame model**

Add to `backend/app/services/epic.py` (with other models):

```python
class EpicGame(BaseModel):
    """Epic game from library."""
    app_name: str
    title: str
    metadata: Dict[str, any] = {}
```

**Step 4: Implement get_library method**

Add to `backend/app/services/epic.py` in EpicService class:

```python
from typing import List

    async def get_library(self) -> List[EpicGame]:
        """Fetch user's Epic Games library.

        Returns:
            List of Epic games

        Raises:
            EpicAuthExpiredError: Not authenticated
            EpicAPIError: Command failed
        """
        logger.info(f"Fetching Epic library for user {self.user_id}")

        result = await self._run_legendary_command(
            ["list", "--json"], timeout=60
        )

        # Parse JSON game list
        games_data = json.loads(result["stdout"])

        games = []
        for game_data in games_data:
            game = EpicGame(
                app_name=game_data.get("app_name", ""),
                title=game_data.get("app_title", game_data.get("app_name", "")),
                metadata=game_data
            )
            games.append(game)

        logger.info(f"Fetched {len(games)} games from Epic for user {self.user_id}")
        return games
```

**Step 5: Run test to verify it passes**

```bash
uv run pytest app/tests/test_epic_service.py::TestEpicLibrary -v
```

Expected: PASS (3 tests)

**Step 6: Run all Epic service tests**

```bash
uv run pytest app/tests/test_epic_service.py -v
```

Expected: All tests pass (19 total)

**Step 7: Commit**

```bash
git add app/services/epic.py app/tests/test_epic_service.py
git commit -m "feat: implement Epic library fetching"
```

---

## Task 6: Create Epic Sync Adapter

**Files:**
- Create: `backend/app/worker/tasks/sync/adapters/epic.py`
- Create: `backend/app/tests/test_sync_adapters_epic.py`
- Modify: `backend/app/worker/tasks/sync/adapters/base.py`

**Step 1: Write tests for Epic sync adapter**

Create `backend/app/tests/test_sync_adapters_epic.py`:

```python
"""Tests for Epic sync adapter."""

import pytest
from unittest.mock import AsyncMock, patch

from app.worker.tasks.sync.adapters.epic import EpicSyncAdapter
from app.worker.tasks/sync.adapters.base import ExternalGame
from app.models.user import User
from app.models.job import BackgroundJobSource
from app.services.epic import EpicGame


class TestEpicSyncAdapter:
    """Test Epic sync adapter implementation."""

    def test_adapter_source(self):
        """Test adapter has correct source."""
        adapter = EpicSyncAdapter()
        assert adapter.source == BackgroundJobSource.EPIC

    @pytest.mark.asyncio
    async def test_fetch_games_success(self):
        """Test fetching games from Epic."""
        adapter = EpicSyncAdapter()

        # Create mock user with Epic config
        user = User(
            id="test-user",
            username="testuser",
            hashed_password="hashed",
            preferences_json='{"epic": {"is_verified": true, "account_id": "123"}}'
        )

        # Mock Epic games
        mock_games = [
            EpicGame(app_name="Fortnite", title="Fortnite"),
            EpicGame(app_name="RocketLeague", title="Rocket League")
        ]

        with patch('app.worker.tasks.sync.adapters.epic.EpicService') as mock_service_class:
            mock_service = mock_service_class.return_value
            mock_service.get_library = AsyncMock(return_value=mock_games)

            external_games = await adapter.fetch_games(user)

            assert len(external_games) == 2
            assert all(isinstance(g, ExternalGame) for g in external_games)
            assert external_games[0].external_id == "Fortnite"
            assert external_games[0].title == "Fortnite"
            assert external_games[0].platform == "pc-windows"
            assert external_games[0].storefront == "epic"
            assert external_games[0].playtime_hours == 0

    @pytest.mark.asyncio
    async def test_fetch_games_not_configured(self):
        """Test fetching games when Epic not configured."""
        adapter = EpicSyncAdapter()

        # User without Epic config
        user = User(
            id="test-user",
            username="testuser",
            hashed_password="hashed",
            preferences_json='{}'
        )

        with pytest.raises(ValueError) as exc_info:
            await adapter.fetch_games(user)

        assert "not configured" in str(exc_info.value).lower()

    def test_get_credentials_configured(self):
        """Test getting credentials for configured user."""
        adapter = EpicSyncAdapter()

        user = User(
            id="test-user",
            username="testuser",
            hashed_password="hashed",
            preferences_json='{"epic": {"is_verified": true, "account_id": "123"}}'
        )

        creds = adapter.get_credentials(user)

        assert creds is not None
        assert creds["is_verified"] is True
        assert creds["account_id"] == "123"

    def test_get_credentials_not_configured(self):
        """Test getting credentials for unconfigured user."""
        adapter = EpicSyncAdapter()

        user = User(
            id="test-user",
            username="testuser",
            hashed_password="hashed",
            preferences_json='{}'
        )

        creds = adapter.get_credentials(user)

        assert creds is None

    def test_is_configured_true(self):
        """Test is_configured returns True for configured user."""
        adapter = EpicSyncAdapter()

        user = User(
            id="test-user",
            username="testuser",
            hashed_password="hashed",
            preferences_json='{"epic": {"is_verified": true}}'
        )

        assert adapter.is_configured(user) is True

    def test_is_configured_false(self):
        """Test is_configured returns False for unconfigured user."""
        adapter = EpicSyncAdapter()

        user = User(
            id="test-user",
            username="testuser",
            hashed_password="hashed",
            preferences_json='{}'
        )

        assert adapter.is_configured(user) is False
```

**Step 2: Run test to verify it fails**

```bash
uv run pytest app/tests/test_sync_adapters_epic.py -v
```

Expected: FAIL with "ModuleNotFoundError: No module named 'app.worker.tasks.sync.adapters.epic'"

**Step 3: Implement Epic sync adapter**

Create `backend/app/worker/tasks/sync/adapters/epic.py`:

```python
"""Epic sync adapter for fetching user's Epic library.

Implements SyncSourceAdapter protocol to fetch games from Epic
and convert them to the standardized ExternalGame format.
"""

import logging
from typing import Optional, List, Dict

from app.models.user import User
from app.models.job import BackgroundJobSource
from app.services.epic import EpicService
from .base import ExternalGame

logger = logging.getLogger(__name__)


class EpicSyncAdapter:
    """Adapter for syncing games from Epic Games Store.

    Fetches the user's Epic library via legendary CLI and converts
    games to ExternalGame format for generic processing.
    """

    source = BackgroundJobSource.EPIC

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch all games from user's Epic library.

        Args:
            user: The user whose Epic library to fetch

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If Epic credentials are not configured
        """
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("Epic credentials not configured for this user")

        epic_service = EpicService(user.id)
        epic_games = await epic_service.get_library()

        logger.info(f"Fetched {len(epic_games)} games from Epic for user {user.id}")

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

    def get_credentials(self, user: User) -> Optional[Dict[str, any]]:
        """Extract Epic credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with Epic config, or None if not configured
        """
        preferences = user.preferences or {}
        epic_config = preferences.get("epic", {})

        is_verified = epic_config.get("is_verified", False)

        if not is_verified:
            return None

        return epic_config

    def is_configured(self, user: User) -> bool:
        """Check if user has verified Epic credentials.

        Args:
            user: The user to check

        Returns:
            True if Epic credentials are configured and verified
        """
        return self.get_credentials(user) is not None
```

**Step 4: Run test to verify it passes**

```bash
uv run pytest app/tests/test_sync_adapters_epic.py -v
```

Expected: PASS (7 tests)

**Step 5: Register adapter in base.py**

Modify `backend/app/worker/tasks/sync/adapters/base.py`:

Find the `get_sync_adapter` function and update:

```python
def get_sync_adapter(source: str) -> SyncSourceAdapter:
    """Get the appropriate adapter for a sync source.

    Args:
        source: Source identifier ("steam", "epic", "gog")

    Returns:
        Adapter instance for the specified source

    Raises:
        ValueError: If source is not supported
    """
    from .steam import SteamSyncAdapter
    from .epic import EpicSyncAdapter  # ADD THIS

    adapters = {
        "steam": SteamSyncAdapter,
        "epic": EpicSyncAdapter,  # ADD THIS
    }

    adapter_class = adapters.get(source.lower())
    if not adapter_class:
        raise ValueError(f"Unsupported sync source: {source}")

    return adapter_class()
```

**Step 6: Test adapter registration**

```bash
uv run pytest app/tests/test_sync_adapters.py -v -k "test_get_sync_adapter"
```

Expected: Tests pass including epic adapter

**Step 7: Commit**

```bash
git add app/worker/tasks/sync/adapters/epic.py app/worker/tasks/sync/adapters/base.py app/tests/test_sync_adapters_epic.py
git commit -m "feat: implement Epic sync adapter"
```

---

## Task 7: Add Epic API Schemas

**Files:**
- Modify: `backend/app/schemas/sync.py`
- Create: `backend/app/tests/test_schemas_epic.py`

**Step 1: Write tests for Epic schemas**

Create `backend/app/tests/test_schemas_epic.py`:

```python
"""Tests for Epic API schemas."""

from app.schemas.sync import (
    EpicAuthStartResponse,
    EpicAuthCompleteRequest,
    EpicAuthCompleteResponse,
    EpicAuthCheckResponse,
)


class TestEpicSchemas:
    """Test Epic API request/response schemas."""

    def test_epic_auth_start_response(self):
        """Test EpicAuthStartResponse schema."""
        response = EpicAuthStartResponse(
            auth_url="https://epicgames.com/activate",
            instructions="Login and enter code"
        )

        assert response.auth_url == "https://epicgames.com/activate"
        assert response.instructions == "Login and enter code"

    def test_epic_auth_complete_request(self):
        """Test EpicAuthCompleteRequest schema."""
        request = EpicAuthCompleteRequest(code="ABC123")

        assert request.code == "ABC123"

    def test_epic_auth_complete_response_success(self):
        """Test EpicAuthCompleteResponse success."""
        response = EpicAuthCompleteResponse(
            valid=True,
            display_name="TestUser"
        )

        assert response.valid is True
        assert response.display_name == "TestUser"
        assert response.error is None

    def test_epic_auth_complete_response_error(self):
        """Test EpicAuthCompleteResponse error."""
        response = EpicAuthCompleteResponse(
            valid=False,
            error="invalid_code"
        )

        assert response.valid is False
        assert response.error == "invalid_code"
        assert response.display_name is None

    def test_epic_auth_check_response(self):
        """Test EpicAuthCheckResponse schema."""
        response = EpicAuthCheckResponse(
            is_authenticated=True,
            display_name="TestUser"
        )

        assert response.is_authenticated is True
        assert response.display_name == "TestUser"
```

**Step 2: Run test to verify it fails**

```bash
uv run pytest app/tests/test_schemas_epic.py -v
```

Expected: FAIL with ImportError for missing schemas

**Step 3: Add Epic schemas**

Add to `backend/app/schemas/sync.py` (at end of file):

```python
class EpicAuthStartResponse(BaseModel):
    """Response when starting Epic authentication."""
    auth_url: str = Field(description="URL for user to visit and authenticate")
    instructions: str = Field(description="Instructions for completing authentication")


class EpicAuthCompleteRequest(BaseModel):
    """Request to complete Epic authentication with code."""
    code: str = Field(description="Authorization code from Epic")


class EpicAuthCompleteResponse(BaseModel):
    """Response after completing Epic authentication."""
    valid: bool = Field(description="Whether authentication succeeded")
    display_name: Optional[str] = Field(
        default=None,
        description="Epic display name if authentication succeeded"
    )
    error: Optional[str] = Field(
        default=None,
        description="Error code if authentication failed"
    )


class EpicAuthCheckResponse(BaseModel):
    """Response for Epic authentication status check."""
    is_authenticated: bool = Field(description="Whether user is authenticated")
    display_name: Optional[str] = Field(
        default=None,
        description="Epic display name if authenticated"
    )
```

**Step 4: Run test to verify it passes**

```bash
uv run pytest app/tests/test_schemas_epic.py -v
```

Expected: PASS (5 tests)

**Step 5: Commit**

```bash
git add app/schemas/sync.py app/tests/test_schemas_epic.py
git commit -m "feat: add Epic API schemas"
```

---

## Task 8: Implement Epic API Endpoints

**Files:**
- Modify: `backend/app/api/sync.py`
- Create: `backend/app/tests/test_api_epic_sync.py`

**Step 1: Write tests for Epic endpoints**

Create `backend/app/tests/test_api_epic_sync.py`:

```python
"""Tests for Epic sync API endpoints."""

import pytest
import json
from unittest.mock import AsyncMock, patch
from fastapi import status

from app.services.epic import EpicAuthenticationError, EpicAPIError


@pytest.mark.asyncio
async def test_start_epic_auth_success(client, test_user, auth_headers):
    """Test starting Epic authentication."""

    with patch('app.api.sync.EpicService') as mock_service_class:
        mock_service = mock_service_class.return_value
        mock_service.start_device_auth = AsyncMock(
            return_value="https://epicgames.com/activate"
        )

        response = client.post(
            "/api/v1/sync/epic/auth/start",
            headers=auth_headers
        )

        assert response.status_code == status.HTTP_200_OK
        data = response.json()
        assert "auth_url" in data
        assert "epicgames.com" in data["auth_url"]
        assert "instructions" in data


@pytest.mark.asyncio
async def test_start_epic_auth_error(client, test_user, auth_headers):
    """Test starting Epic authentication with error."""

    with patch('app.api.sync.EpicService') as mock_service_class:
        mock_service = mock_service_class.return_value
        mock_service.start_device_auth = AsyncMock(
            side_effect=EpicAPIError("legendary not found")
        )

        response = client.post(
            "/api/v1/sync/epic/auth/start",
            headers=auth_headers
        )

        assert response.status_code == status.HTTP_500_INTERNAL_SERVER_ERROR


@pytest.mark.asyncio
async def test_complete_epic_auth_success(
    client, test_user, auth_headers, session
):
    """Test completing Epic authentication with valid code."""

    with patch('app.api.sync.EpicService') as mock_service_class:
        mock_service = mock_service_class.return_value
        mock_service.complete_auth = AsyncMock(return_value=True)
        mock_service.get_account_info = AsyncMock()
        mock_service.get_account_info.return_value.display_name = "TestUser"
        mock_service.get_account_info.return_value.account_id = "test123"

        response = client.post(
            "/api/v1/sync/epic/auth/complete",
            headers=auth_headers,
            json={"code": "ABC123"}
        )

        assert response.status_code == status.HTTP_200_OK
        data = response.json()
        assert data["valid"] is True
        assert data["display_name"] == "TestUser"

        # Verify preferences were updated
        session.refresh(test_user)
        prefs = test_user.preferences
        assert "epic" in prefs
        assert prefs["epic"]["is_verified"] is True
        assert prefs["epic"]["account_id"] == "test123"


@pytest.mark.asyncio
async def test_complete_epic_auth_invalid_code(client, test_user, auth_headers):
    """Test completing Epic authentication with invalid code."""

    with patch('app.api.sync.EpicService') as mock_service_class:
        mock_service = mock_service_class.return_value
        mock_service.complete_auth = AsyncMock(
            side_effect=EpicAuthenticationError("Invalid code")
        )

        response = client.post(
            "/api/v1/sync/epic/auth/complete",
            headers=auth_headers,
            json={"code": "INVALID"}
        )

        assert response.status_code == status.HTTP_200_OK
        data = response.json()
        assert data["valid"] is False
        assert data["error"] is not None


@pytest.mark.asyncio
async def test_check_epic_auth_authenticated(client, test_user, auth_headers):
    """Test checking Epic auth status when authenticated."""

    with patch('app.api.sync.EpicService') as mock_service_class:
        mock_service = mock_service_class.return_value
        mock_service.verify_auth = AsyncMock(return_value=True)
        mock_service.get_account_info = AsyncMock()
        mock_service.get_account_info.return_value.display_name = "TestUser"

        response = client.get(
            "/api/v1/sync/epic/auth/check",
            headers=auth_headers
        )

        assert response.status_code == status.HTTP_200_OK
        data = response.json()
        assert data["is_authenticated"] is True
        assert data["display_name"] == "TestUser"


@pytest.mark.asyncio
async def test_check_epic_auth_not_authenticated(client, test_user, auth_headers):
    """Test checking Epic auth status when not authenticated."""

    with patch('app.api.sync.EpicService') as mock_service_class:
        mock_service = mock_service_class.return_value
        mock_service.verify_auth = AsyncMock(return_value=False)

        response = client.get(
            "/api/v1/sync/epic/auth/check",
            headers=auth_headers
        )

        assert response.status_code == status.HTTP_200_OK
        data = response.json()
        assert data["is_authenticated"] is False
        assert data["display_name"] is None


@pytest.mark.asyncio
async def test_disconnect_epic_success(
    client, test_user, auth_headers, session
):
    """Test disconnecting Epic."""

    # Set up user with Epic config
    test_user.preferences_json = json.dumps({
        "epic": {"is_verified": True, "account_id": "123"}
    })
    session.commit()

    with patch('app.api.sync.EpicService') as mock_service_class:
        mock_service = mock_service_class.return_value
        mock_service.disconnect = AsyncMock()

        response = client.delete(
            "/api/v1/sync/epic/connection",
            headers=auth_headers
        )

        assert response.status_code == status.HTTP_200_OK
        data = response.json()
        assert data["success"] is True

        # Verify preferences were cleared
        session.refresh(test_user)
        prefs = test_user.preferences
        assert "epic" not in prefs
```

**Step 2: Run test to verify it fails**

```bash
uv run pytest app/tests/test_api_epic_sync.py -v
```

Expected: FAIL with 404 for missing endpoints

**Step 3: Implement Epic API endpoints**

Add to `backend/app/api/sync.py` (before the ignored games endpoints):

```python
from ..services.epic import EpicService, EpicAuthenticationError, EpicAPIError
from ..schemas.sync import (
    # ... existing imports ...
    EpicAuthStartResponse,
    EpicAuthCompleteRequest,
    EpicAuthCompleteResponse,
    EpicAuthCheckResponse,
)


@router.post("/epic/auth/start", response_model=EpicAuthStartResponse)
async def start_epic_auth(
    current_user: Annotated[User, Depends(get_current_user)],
) -> EpicAuthStartResponse:
    """
    Start Epic authentication flow.

    Initiates legendary auth and returns the authentication URL
    for user to complete login in browser.
    """
    logger.info(f"Starting Epic auth for user {current_user.id}")

    try:
        epic_service = EpicService(current_user.id)
        auth_url = await epic_service.start_device_auth()

        return EpicAuthStartResponse(
            auth_url=auth_url,
            instructions="Visit the URL above, login to Epic, and copy the authorization code"
        )
    except EpicAPIError as e:
        logger.error(f"Failed to start Epic auth: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )


@router.post("/epic/auth/complete", response_model=EpicAuthCompleteResponse)
async def complete_epic_auth(
    request: EpicAuthCompleteRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> EpicAuthCompleteResponse:
    """
    Complete Epic authentication with authorization code.

    Takes the code user copied from Epic and completes auth via legendary.
    """
    logger.info(f"Completing Epic auth for user {current_user.id}")

    epic_service = EpicService(current_user.id)

    try:
        # Complete auth with legendary
        await epic_service.complete_auth(request.code)

        # Get account info
        account_info = await epic_service.get_account_info()

        # Store in preferences
        preferences = current_user.preferences or {}
        preferences["epic"] = {
            "display_name": account_info.display_name,
            "account_id": account_info.account_id,
            "is_verified": True,
        }
        current_user.preferences_json = json.dumps(preferences)
        current_user.updated_at = datetime.now(timezone.utc)
        session.commit()

        logger.info(
            f"Epic auth completed for user {current_user.id}: "
            f"account '{account_info.display_name}'"
        )

        return EpicAuthCompleteResponse(
            valid=True,
            display_name=account_info.display_name
        )

    except EpicAuthenticationError as e:
        logger.warning(f"Epic auth failed for user {current_user.id}: {e}")
        return EpicAuthCompleteResponse(
            valid=False,
            error="invalid_code"
        )
    except EpicAPIError as e:
        logger.error(f"Epic API error during auth: {e}")
        return EpicAuthCompleteResponse(
            valid=False,
            error="network_error"
        )


@router.get("/epic/auth/check", response_model=EpicAuthCheckResponse)
async def check_epic_auth_status(
    current_user: Annotated[User, Depends(get_current_user)],
) -> EpicAuthCheckResponse:
    """
    Check if Epic authentication is still valid.

    Verifies legendary auth status without triggering new login.
    """
    logger.debug(f"Checking Epic auth status for user {current_user.id}")

    epic_service = EpicService(current_user.id)

    try:
        is_authenticated = await epic_service.verify_auth()

        if is_authenticated:
            account_info = await epic_service.get_account_info()
            return EpicAuthCheckResponse(
                is_authenticated=True,
                display_name=account_info.display_name
            )
        else:
            return EpicAuthCheckResponse(
                is_authenticated=False
            )

    except Exception as e:
        logger.error(f"Error checking Epic auth status: {e}")
        return EpicAuthCheckResponse(
            is_authenticated=False
        )


@router.delete("/epic/connection", response_model=SuccessResponse)
async def disconnect_epic(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> SuccessResponse:
    """
    Disconnect Epic integration.

    Clears Epic credentials and removes legendary auth.
    """
    logger.info(f"Disconnecting Epic for user {current_user.id}")

    epic_service = EpicService(current_user.id)

    try:
        await epic_service.disconnect()
    except EpicAPIError as e:
        # Log but don't fail - cleanup preferences anyway
        logger.warning(f"Error during Epic disconnect: {e}")

    # Clear from preferences
    preferences = current_user.preferences or {}
    if "epic" in preferences:
        del preferences["epic"]
        current_user.preferences_json = json.dumps(preferences)

    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()

    logger.info(f"Epic disconnected for user {current_user.id}")

    return SuccessResponse(
        success=True,
        message="Epic disconnected successfully"
    )
```

**Step 4: Run test to verify it passes**

```bash
uv run pytest app/tests/test_api_epic_sync.py -v
```

Expected: PASS (7 tests)

**Step 5: Test Epic sync trigger (already works!)**

```bash
uv run pytest app/tests/test_sync_api.py -v -k "epic"
```

Expected: Existing generic sync endpoints work with Epic

**Step 6: Commit**

```bash
git add app/api/sync.py app/tests/test_api_epic_sync.py
git commit -m "feat: implement Epic API endpoints"
```

---

## Task 9: Update Deployment Configuration

**Files:**
- Modify: `backend/Dockerfile`
- Modify: `docker-compose.yml`
- Modify: `README.md`
- Modify: `flake.nix`

**Step 1: Update Dockerfile**

Modify `backend/Dockerfile` to add legendary installation:

Find the dependencies installation section and add:

```dockerfile
# Install legendary-gl for Epic Games Store sync
RUN pip install legendary-gl>=0.20.34
```

**Step 2: Update docker-compose.yml**

Add volume for legendary configs:

```yaml
services:
  api:
    volumes:
      # ... existing volumes ...
      - legendary-configs:/var/lib/nexorious/legendary-configs

volumes:
  # ... existing volumes ...
  legendary-configs:
```

**Step 3: Update README.md**

Add legendary to system requirements section:

```markdown
### System Dependencies

- PostgreSQL 13+
- legendary-gl (for Epic Games Store sync)
  - Install: `pip install legendary-gl`
  - Required for Epic Games Store library sync
  - GPL3 licensed, used as external tool only
```

**Step 4: Update flake.nix**

Add legendary to development environment:

Find the `packages` section and add:

```nix
packages = with pkgs; [
  # ... existing packages ...
  python313Packages.legendary-gl  # Epic Games Store sync
];
```

**Step 5: Commit**

```bash
git add Dockerfile docker-compose.yml README.md flake.nix
git commit -m "feat: add legendary deployment configuration"
```

---

## Task 10: Run Full Test Suite

**Files:**
- None (verification step)

**Step 1: Run all backend tests**

```bash
cd backend
uv run pytest --cov=app --cov-report=term-missing -v
```

Expected: All tests pass, coverage >80%

**Step 2: Run type checking**

```bash
uv run pyrefly check
```

Expected: No type errors

**Step 3: Run linting**

```bash
uv run ruff check .
```

Expected: No errors

**Step 4: If any issues, fix them**

Fix any test failures, type errors, or lint errors before continuing.

**Step 5: Commit any fixes**

```bash
git add .
git commit -m "fix: address test/type/lint issues"
```

---

## Task 11: Integration Testing

**Files:**
- None (manual verification)

**Step 1: Test Epic auth flow (if legendary available)**

```bash
# Start backend
uv run python -m app.main
```

In another terminal:
```bash
# Test auth start
curl -X POST http://localhost:8000/api/v1/sync/epic/auth/start \
  -H "Authorization: Bearer YOUR_TOKEN"

# Copy URL and authenticate with Epic (manual step)

# Test auth complete
curl -X POST http://localhost:8000/api/v1/sync/epic/auth/complete \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"code": "YOUR_CODE"}'

# Test sync trigger
curl -X POST http://localhost:8000/api/v1/sync/epic \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Step 2: Verify sync pipeline**

Check that:
- [ ] Auth flow completes successfully
- [ ] User preferences store Epic config
- [ ] Sync job is created
- [ ] Games are fetched from Epic
- [ ] IGDB matching works
- [ ] Games appear in collection with Epic storefront

**Step 3: Document any issues**

Create issues for any bugs found during testing.

---

## Task 12: Update PRD

**Files:**
- Modify: `docs/PRD.md`

**Step 1: Update PRD status**

Find section 6.1 Enhanced Storefront Integration and update:

```markdown
#### 6.1 Enhanced Storefront Integration
**Priority**: P2 (Medium) - **PARTIALLY IMPLEMENTED**

**Implemented:**
- Epic Games Store integration ✓
  - Server-side sync using legendary CLI
  - Device code OAuth authentication
  - Library import with IGDB matching
  - Multi-user config isolation
  - Auth expiration handling

**Pending:**
- GOG integration
- PlayStation Store integration
- Xbox Marketplace integration
```

**Step 2: Commit PRD update**

```bash
git add docs/PRD.md
git commit -m "docs: update PRD with Epic Games Store implementation status"
```

---

## Final Steps

**Step 1: Merge to main**

Use the `superpowers:finishing-a-development-branch` skill to review and merge.

**Step 2: Cleanup worktree**

After merge, remove the worktree:

```bash
cd /home/abo/workspace/home/nexorious
git worktree remove .worktrees/epic-games-store-sync
```

---

## Testing Checklist

- [x] All unit tests pass (EpicService, EpicSyncAdapter, API endpoints)
- [x] Integration tests pass (sync pipeline with Epic adapter)
- [x] Type checking passes (pyrefly)
- [x] Linting passes (ruff)
- [x] Backend coverage >80%
- [ ] Manual Epic auth flow tested (requires legendary + Epic account)
- [ ] Manual sync tested end-to-end (requires legendary + Epic account)
- [ ] Multi-user isolation verified
- [ ] Auth expiration handling verified

## Documentation Checklist

- [x] README updated with legendary requirement
- [x] Deployment docs updated (Dockerfile, docker-compose)
- [x] Development environment updated (flake.nix)
- [x] PRD updated with implementation status
- [ ] User-facing Epic sync documentation (future task)
- [ ] Admin guide for legendary troubleshooting (future task)

## Success Criteria

✓ Users can authenticate with Epic Games Store via device code flow
✓ Epic library syncs automatically on trigger
✓ Games appear in collection with Epic storefront badge
✓ Auth expiration handled gracefully with user prompts
✓ Multi-user isolation works correctly (separate legendary configs)
✓ All tests pass with >80% coverage
✓ legendary remains external dependency (no GPL3 contamination)
✓ Zero breaking changes to existing sync functionality
