# Epic Games Store Authentication Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable Epic Games credentials to be shared between API and worker containers via database storage

**Architecture:** Store legendary's user.json credentials in UserSyncConfig.platform_credentials field. On EpicService initialization, hydrate credentials from database to filesystem for legendary's use. After authentication, persist filesystem credentials back to database.

**Tech Stack:** FastAPI, SQLModel, legendary-gl, pytest

---

## Task 1: Add credential path helper method

**Files:**
- Modify: `backend/app/services/epic.py:45-68`
- Test: `backend/app/tests/test_epic_service.py`

**Step 1: Write the failing test**

Add to `backend/app/tests/test_epic_service.py` after line 36:

```python
@patch('app.services.epic.LegendaryCore')
def test_get_user_json_path(self, mock_legendary_core):
    """Test _get_user_json_path returns correct path."""
    service = EpicService("test-user-123")

    expected_path = "/var/lib/nexorious/legendary-configs/test-user-123/legendary/user.json"
    assert service._get_user_json_path() == expected_path
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicService::test_get_user_json_path -v`

Expected: FAIL with "AttributeError: 'EpicService' object has no attribute '_get_user_json_path'"

**Step 3: Write minimal implementation**

Add to `backend/app/services/epic.py` after line 67 (after `__init__` method):

```python
def _get_user_json_path(self) -> str:
    """Get path to legendary's user.json file."""
    return os.path.join(self.config_path, "legendary", "user.json")
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicService::test_get_user_json_path -v`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/services/epic.py backend/app/tests/test_epic_service.py
git commit -m "feat: add helper method to get legendary user.json path

Adds _get_user_json_path() method to EpicService for consistent
path construction to legendary's credential file."
```

---

## Task 2: Add load credentials from database method

**Files:**
- Modify: `backend/app/services/epic.py:7-13,70-71`
- Test: `backend/app/tests/test_epic_service.py`

**Step 1: Write failing tests for load_credentials_from_db**

Add to `backend/app/tests/test_epic_service.py` after the TestEpicService class (around line 36), create new test class:

```python
class TestEpicCredentialStorage:
    """Test Epic credential database storage."""

    @patch('app.services.epic.LegendaryCore')
    def test_load_credentials_from_db_success(self, mock_legendary_core, tmp_path):
        """Test loading credentials from database writes to filesystem."""
        import json
        from unittest.mock import MagicMock
        from app.models.user_sync_config import UserSyncConfig

        # Mock session and config
        mock_session = MagicMock()
        mock_config = UserSyncConfig(
            user_id="test-user",
            platform="epic",
            platform_credentials=json.dumps({
                "access_token": "test-token",
                "account_id": "test-account"
            })
        )
        mock_session.exec.return_value.first.return_value = mock_config

        # Use tmp_path for testing
        with patch.object(EpicService, '__init__', lambda self, user_id, session=None: None):
            service = EpicService.__new__(EpicService)
            service.user_id = "test-user"
            service.config_path = str(tmp_path)

            service._load_credentials_from_db(mock_session)

            # Verify file was created
            user_json_path = tmp_path / "legendary" / "user.json"
            assert user_json_path.exists()

            # Verify contents match
            with open(user_json_path, 'r') as f:
                loaded_creds = json.load(f)
            assert loaded_creds["access_token"] == "test-token"
            assert loaded_creds["account_id"] == "test-account"

    @patch('app.services.epic.LegendaryCore')
    def test_load_credentials_from_db_no_config(self, mock_legendary_core, tmp_path):
        """Test loading credentials when no config exists does nothing."""
        from unittest.mock import MagicMock

        # Mock session with no config
        mock_session = MagicMock()
        mock_session.exec.return_value.first.return_value = None

        with patch.object(EpicService, '__init__', lambda self, user_id, session=None: None):
            service = EpicService.__new__(EpicService)
            service.user_id = "test-user"
            service.config_path = str(tmp_path)

            # Should not raise, just do nothing
            service._load_credentials_from_db(mock_session)

            # Verify no file was created
            user_json_path = tmp_path / "legendary" / "user.json"
            assert not user_json_path.exists()

    @patch('app.services.epic.LegendaryCore')
    def test_load_credentials_from_db_empty_credentials(self, mock_legendary_core, tmp_path):
        """Test loading credentials when platform_credentials is None."""
        from unittest.mock import MagicMock
        from app.models.user_sync_config import UserSyncConfig

        # Mock session with empty credentials
        mock_session = MagicMock()
        mock_config = UserSyncConfig(
            user_id="test-user",
            platform="epic",
            platform_credentials=None
        )
        mock_session.exec.return_value.first.return_value = mock_config

        with patch.object(EpicService, '__init__', lambda self, user_id, session=None: None):
            service = EpicService.__new__(EpicService)
            service.user_id = "test-user"
            service.config_path = str(tmp_path)

            service._load_credentials_from_db(mock_session)

            # Verify no file was created
            user_json_path = tmp_path / "legendary" / "user.json"
            assert not user_json_path.exists()
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicCredentialStorage -v`

Expected: FAIL with "AttributeError: 'EpicService' object has no attribute '_load_credentials_from_db'"

**Step 3: Add required imports to epic.py**

Update imports in `backend/app/services/epic.py` (lines 7-13):

```python
import logging
import os
import json
from typing import Any, Dict, List, Optional

from legendary.core import LegendaryCore
from pydantic import BaseModel, Field
from sqlmodel import Session, select

from ..models.user_sync_config import UserSyncConfig
```

**Step 4: Write minimal implementation**

Add to `backend/app/services/epic.py` after the `_get_user_json_path` method (around line 71):

```python
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
```

**Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicCredentialStorage::test_load_credentials_from_db_success app/tests/test_epic_service.py::TestEpicCredentialStorage::test_load_credentials_from_db_no_config app/tests/test_epic_service.py::TestEpicCredentialStorage::test_load_credentials_from_db_empty_credentials -v`

Expected: PASS (all 3 tests)

**Step 6: Commit**

```bash
git add backend/app/services/epic.py backend/app/tests/test_epic_service.py
git commit -m "feat: add method to load Epic credentials from database

Implements _load_credentials_from_db() to hydrate legendary's user.json
from database storage, enabling credential sharing across containers."
```

---

## Task 3: Add save credentials to database method

**Files:**
- Modify: `backend/app/services/epic.py:~95`
- Test: `backend/app/tests/test_epic_service.py`

**Step 1: Write failing tests for save_credentials_to_db**

Add to `backend/app/tests/test_epic_service.py` in the TestEpicCredentialStorage class:

```python
@patch('app.services.epic.LegendaryCore')
def test_save_credentials_to_db_success(self, mock_legendary_core, tmp_path):
    """Test saving credentials from filesystem to database."""
    import json
    from unittest.mock import MagicMock
    from app.models.user_sync_config import UserSyncConfig
    from datetime import datetime, timezone

    # Create mock user.json file
    user_json_dir = tmp_path / "legendary"
    user_json_dir.mkdir(parents=True)
    user_json_path = user_json_dir / "user.json"

    credentials = {
        "access_token": "saved-token",
        "account_id": "saved-account"
    }
    with open(user_json_path, 'w') as f:
        json.dump(credentials, f)

    # Mock session and existing config
    mock_session = MagicMock()
    mock_config = UserSyncConfig(
        user_id="test-user",
        platform="epic",
        platform_credentials=None
    )
    mock_session.exec.return_value.first.return_value = mock_config

    with patch.object(EpicService, '__init__', lambda self, user_id, session=None: None):
        service = EpicService.__new__(EpicService)
        service.user_id = "test-user"
        service.config_path = str(tmp_path)

        service._save_credentials_to_db(mock_session)

        # Verify credentials were saved
        assert mock_config.platform_credentials is not None
        saved_creds = json.loads(mock_config.platform_credentials)
        assert saved_creds["access_token"] == "saved-token"
        assert saved_creds["account_id"] == "saved-account"

        # Verify commit was called
        mock_session.commit.assert_called_once()

@patch('app.services.epic.LegendaryCore')
def test_save_credentials_to_db_creates_config(self, mock_legendary_core, tmp_path):
    """Test saving credentials creates UserSyncConfig if none exists."""
    import json
    from unittest.mock import MagicMock
    from app.models.user_sync_config import UserSyncConfig

    # Create mock user.json file
    user_json_dir = tmp_path / "legendary"
    user_json_dir.mkdir(parents=True)
    user_json_path = user_json_dir / "user.json"

    credentials = {"access_token": "new-token"}
    with open(user_json_path, 'w') as f:
        json.dump(credentials, f)

    # Mock session with no existing config
    mock_session = MagicMock()
    mock_session.exec.return_value.first.return_value = None

    with patch.object(EpicService, '__init__', lambda self, user_id, session=None: None):
        service = EpicService.__new__(EpicService)
        service.user_id = "test-user"
        service.config_path = str(tmp_path)

        service._save_credentials_to_db(mock_session)

        # Verify new config was added
        mock_session.add.assert_called_once()
        added_config = mock_session.add.call_args[0][0]
        assert isinstance(added_config, UserSyncConfig)
        assert added_config.user_id == "test-user"
        assert added_config.platform == "epic"
        assert added_config.platform_credentials is not None

@patch('app.services.epic.LegendaryCore')
def test_save_credentials_to_db_no_file(self, mock_legendary_core, tmp_path):
    """Test saving credentials when user.json doesn't exist logs warning."""
    from unittest.mock import MagicMock

    mock_session = MagicMock()

    with patch.object(EpicService, '__init__', lambda self, user_id, session=None: None):
        service = EpicService.__new__(EpicService)
        service.user_id = "test-user"
        service.config_path = str(tmp_path)

        # Should not raise, just log warning
        service._save_credentials_to_db(mock_session)

        # Verify no database operations occurred
        mock_session.exec.assert_not_called()
        mock_session.add.assert_not_called()
        mock_session.commit.assert_not_called()
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicCredentialStorage::test_save_credentials_to_db_success app/tests/test_epic_service.py::TestEpicCredentialStorage::test_save_credentials_to_db_creates_config app/tests/test_epic_service.py::TestEpicCredentialStorage::test_save_credentials_to_db_no_file -v`

Expected: FAIL with "AttributeError: 'EpicService' object has no attribute '_save_credentials_to_db'"

**Step 3: Write minimal implementation**

Add to `backend/app/services/epic.py` after the `_load_credentials_from_db` method:

```python
def _save_credentials_to_db(self, session: Session) -> None:
    """Read credentials from filesystem and save to database."""
    from datetime import datetime, timezone

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

**Step 4: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicCredentialStorage::test_save_credentials_to_db_success app/tests/test_epic_service.py::TestEpicCredentialStorage::test_save_credentials_to_db_creates_config app/tests/test_epic_service.py::TestEpicCredentialStorage::test_save_credentials_to_db_no_file -v`

Expected: PASS (all 3 tests)

**Step 5: Commit**

```bash
git add backend/app/services/epic.py backend/app/tests/test_epic_service.py
git commit -m "feat: add method to save Epic credentials to database

Implements _save_credentials_to_db() to persist legendary's user.json
to database after authentication completes."
```

---

## Task 4: Update EpicService __init__ to support session parameter

**Files:**
- Modify: `backend/app/services/epic.py:52-67`
- Test: `backend/app/tests/test_epic_service.py`

**Step 1: Write failing test**

Add to `backend/app/tests/test_epic_service.py` in TestEpicService class (after test_service_initialization):

```python
@patch('app.services.epic.LegendaryCore')
def test_service_initialization_with_session(self, mock_legendary_core, tmp_path):
    """Test EpicService loads credentials from database when session provided."""
    import json
    from unittest.mock import MagicMock
    from app.models.user_sync_config import UserSyncConfig

    # Mock session with credentials
    mock_session = MagicMock()
    mock_config = UserSyncConfig(
        user_id="test-user",
        platform="epic",
        platform_credentials=json.dumps({"access_token": "db-token"})
    )
    mock_session.exec.return_value.first.return_value = mock_config

    # Patch config_path to use tmp_path
    with patch.object(EpicService, 'config_path', str(tmp_path)):
        service = EpicService("test-user", session=mock_session)

        # Verify credentials were loaded to filesystem
        user_json_path = tmp_path / "legendary" / "user.json"
        assert user_json_path.exists()
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicService::test_service_initialization_with_session -v`

Expected: FAIL with "TypeError: __init__() got an unexpected keyword argument 'session'"

**Step 3: Update __init__ signature and implementation**

Modify `backend/app/services/epic.py` __init__ method (lines 52-67):

```python
def __init__(self, user_id: str, session: Optional[Session] = None):
    """Initialize Epic service with user-specific config path."""
    self.user_id = user_id
    self.config_path = f"/var/lib/nexorious/legendary-configs/{user_id}"

    # Load credentials from database if available
    if session:
        self._load_credentials_from_db(session)

    # Set environment variable for legendary to use custom config directory
    # legendary respects XDG_CONFIG_HOME for storing its config
    os.environ['XDG_CONFIG_HOME'] = self.config_path

    # Initialize legendary core with custom config
    try:
        self.core = LegendaryCore()
        logger.debug(f"EpicService initialized for user {user_id} with config at {self.config_path}")
    except Exception as e:
        logger.error(f"Failed to initialize LegendaryCore: {e}")
        raise EpicAPIError(f"Failed to initialize Epic service: {e}")
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py::TestEpicService::test_service_initialization_with_session -v`

Expected: PASS

**Step 5: Run all EpicService tests to ensure no regression**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py -v`

Expected: All tests PASS

**Step 6: Commit**

```bash
git add backend/app/services/epic.py backend/app/tests/test_epic_service.py
git commit -m "feat: add session parameter to EpicService __init__

Allows EpicService to load credentials from database on initialization
when session is provided. Maintains backward compatibility."
```

---

## Task 5: Update complete_epic_auth endpoint to save credentials

**Files:**
- Modify: `backend/app/api/sync.py:492-548`
- Test: `backend/app/tests/test_sync_api.py` (create if doesn't exist)

**Step 1: Write failing test**

Create or modify `backend/app/tests/test_sync_api.py`. Add test:

```python
"""Tests for sync API endpoints."""

import pytest
import json
from unittest.mock import MagicMock, patch, AsyncMock
from fastapi.testclient import TestClient

from app.main import app
from app.models.user_sync_config import UserSyncConfig


@pytest.fixture
def mock_epic_service():
    """Mock EpicService for testing."""
    with patch('app.api.sync.EpicService') as mock:
        mock_instance = MagicMock()
        mock_instance.complete_auth = AsyncMock(return_value=True)
        mock_instance.get_account_info = AsyncMock()
        mock_instance.get_account_info.return_value.display_name = "TestUser"
        mock_instance.get_account_info.return_value.account_id = "test-account-123"
        mock_instance._save_credentials_to_db = MagicMock()
        mock.return_value = mock_instance
        yield mock_instance


class TestEpicAuthEndpoints:
    """Test Epic authentication endpoints."""

    def test_complete_epic_auth_saves_to_db(self, mock_epic_service, client, db_session, test_user):
        """Test complete_epic_auth saves credentials to database."""
        # Authenticate as test user
        response = client.post(
            "/sync/epic/auth/complete",
            json={"code": "test-auth-code"},
            headers={"Authorization": f"Bearer {test_user.token}"}
        )

        assert response.status_code == 200
        assert response.json()["valid"] is True
        assert response.json()["display_name"] == "TestUser"

        # Verify _save_credentials_to_db was called
        mock_epic_service._save_credentials_to_db.assert_called_once()
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_sync_api.py::TestEpicAuthEndpoints::test_complete_epic_auth_saves_to_db -v`

Expected: FAIL (either test file doesn't exist or _save_credentials_to_db not called)

**Step 3: Update complete_epic_auth endpoint**

Modify `backend/app/api/sync.py` in the `complete_epic_auth` function (around line 506-524):

Find the section after `account_info = await epic_service.get_account_info()` and before updating user preferences. Add:

```python
# Save credentials to database
epic_service._save_credentials_to_db(session)
```

The complete section should look like:

```python
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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_sync_api.py::TestEpicAuthEndpoints::test_complete_epic_auth_saves_to_db -v`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/api/sync.py backend/app/tests/test_sync_api.py
git commit -m "feat: save Epic credentials to database after auth

Updates complete_epic_auth endpoint to persist credentials to database
immediately after successful authentication."
```

---

## Task 6: Update disconnect_epic endpoint to clear database credentials

**Files:**
- Modify: `backend/app/api/sync.py:583-616`
- Test: `backend/app/tests/test_sync_api.py`

**Step 1: Write failing test**

Add to `backend/app/tests/test_sync_api.py` in TestEpicAuthEndpoints class:

```python
def test_disconnect_epic_clears_db_credentials(self, client, db_session, test_user):
    """Test disconnect_epic clears platform_credentials from database."""
    from app.models.user_sync_config import UserSyncConfig

    # Set up user with Epic credentials in database
    config = UserSyncConfig(
        user_id=test_user.id,
        platform="epic",
        platform_credentials='{"access_token": "test-token"}'
    )
    db_session.add(config)
    db_session.commit()

    # Disconnect Epic
    with patch('app.api.sync.EpicService') as mock_service:
        mock_service.return_value.disconnect = AsyncMock()

        response = client.delete(
            "/sync/epic/connection",
            headers={"Authorization": f"Bearer {test_user.token}"}
        )

    assert response.status_code == 200

    # Verify credentials were cleared from database
    db_session.refresh(config)
    assert config.platform_credentials is None
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_sync_api.py::TestEpicAuthEndpoints::test_disconnect_epic_clears_db_credentials -v`

Expected: FAIL with "AssertionError: assert '{"access_token": "test-token"}' is None"

**Step 3: Update disconnect_epic endpoint**

Modify `backend/app/api/sync.py` in the `disconnect_epic` function (around line 595-610).

Add after the `epic_service.disconnect()` call and before clearing preferences:

```python
# Clear credentials from database
stmt = select(UserSyncConfig).where(
    UserSyncConfig.user_id == current_user.id,
    UserSyncConfig.platform == "epic"
)
config = session.exec(stmt).first()
if config:
    config.platform_credentials = None
    config.updated_at = datetime.now(timezone.utc)
```

The complete section should look like:

```python
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
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_sync_api.py::TestEpicAuthEndpoints::test_disconnect_epic_clears_db_credentials -v`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/api/sync.py backend/app/tests/test_sync_api.py
git commit -m "feat: clear database credentials on Epic disconnect

Updates disconnect_epic endpoint to clear platform_credentials field
from database when user disconnects Epic account."
```

---

## Task 7: Update worker Epic adapter to pass session

**Files:**
- Modify: `backend/app/worker/tasks/sync/adapters/epic.py:27-61`

**Step 1: Check current signature of fetch_games**

Read: `backend/app/worker/tasks/sync/adapters/epic.py`

Note: The signature needs to be updated to accept and pass session parameter.

**Step 2: Update fetch_games method signature**

Modify `backend/app/worker/tasks/sync/adapters/epic.py` fetch_games method (around line 27):

Change from:
```python
async def fetch_games(self, user: User) -> List[ExternalGame]:
```

To:
```python
async def fetch_games(self, user: User, session: Session) -> List[ExternalGame]:
```

**Step 3: Pass session to EpicService**

In the same method, update the EpicService initialization (around line 44):

Change from:
```python
epic_service = EpicService(user_id=user.id)
```

To:
```python
epic_service = EpicService(user_id=user.id, session=session)
```

**Step 4: Add required imports**

At the top of `backend/app/worker/tasks/sync/adapters/epic.py`, ensure Session is imported:

```python
from sqlmodel import Session
```

**Step 5: Verify no tests exist that need updating**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -k "epic" -k "adapter" -v`

Expected: Check if any adapter tests exist and need updating

**Step 6: Commit**

```bash
git add backend/app/worker/tasks/sync/adapters/epic.py
git commit -m "feat: pass database session to EpicService in worker

Updates Epic sync adapter to pass session parameter, enabling worker
to load credentials from database."
```

---

## Task 8: Run full test suite and verify coverage

**Files:**
- N/A (testing only)

**Step 1: Run all Epic-related tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_epic_service.py -v`

Expected: All tests PASS

**Step 2: Run API tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_sync_api.py -v`

Expected: All tests PASS

**Step 3: Run full test suite with coverage**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`

Expected:
- All tests PASS
- Coverage >80%
- New methods have 100% coverage

**Step 4: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

Expected: No type errors

**Step 5: Document test results**

If any tests fail, fix them before proceeding. If coverage is low, add missing tests.

---

## Task 9: Manual integration testing

**Files:**
- N/A (manual testing)

**Step 1: Start development environment**

Run: `cd /home/abo/workspace/home/nexorious && podman-compose up --build`

Expected: API and worker containers start successfully

**Step 2: Test Epic authentication flow**

1. Navigate to frontend Epic sync settings
2. Click "Connect Epic Games"
3. Complete authentication flow
4. Verify success message

**Step 3: Verify database storage**

Run: Check database for credentials:
```sql
SELECT user_id, platform,
       CASE WHEN platform_credentials IS NULL THEN 'NULL' ELSE 'SET' END as creds_status
FROM user_sync_configs
WHERE platform = 'epic';
```

Expected: Credentials should be SET for authenticated user

**Step 4: Test worker credential access**

1. Trigger Epic library sync
2. Check worker logs for "Loaded Epic credentials from database"
3. Verify sync completes successfully

**Step 5: Test disconnect flow**

1. Disconnect Epic account
2. Verify database credentials cleared
3. Verify filesystem credentials removed

---

## Task 10: Update documentation and final commit

**Files:**
- Modify: `docs/plans/2026-01-01-epic-auth-improvements-design.md`

**Step 1: Update design document status**

Update `backend/docs/plans/2026-01-01-epic-auth-improvements-design.md` header:

Change:
```markdown
**Status:** Approved for Implementation
```

To:
```markdown
**Status:** Implemented
**Implementation Date:** 2026-01-01
```

**Step 2: Add implementation notes section**

Add to end of design document:

```markdown
## Implementation Notes

**Completed:** 2026-01-01

**Changes from design:**
- None - implementation followed design exactly

**Test Coverage:**
- EpicService credential methods: 100%
- API endpoints: 100%
- Overall backend: >80%

**Known Issues:**
- None identified during implementation

**Future Enhancements:**
- Add automatic token refresh persistence to database
- Add encryption for platform_credentials field
- Add migration script for existing filesystem credentials
```

**Step 3: Final commit**

```bash
git add docs/plans/2026-01-01-epic-auth-improvements-design.md
git commit -m "docs: mark Epic auth improvements as implemented

Updates design document to reflect completed implementation with
notes on test coverage and future enhancements."
```

**Step 4: Create summary commit**

```bash
git log --oneline | head -n 10
```

Review commits to ensure clean history.

---

## Success Criteria Checklist

Before considering this implementation complete, verify:

- [ ] All unit tests pass
- [ ] Test coverage >80% overall, 100% for new methods
- [ ] Type checking passes with zero errors
- [ ] API container can authenticate Epic account
- [ ] Worker container can access credentials from database
- [ ] Disconnect clears database credentials
- [ ] No regression in existing Epic sync functionality
- [ ] Manual integration testing completed successfully
- [ ] Documentation updated

---

## Rollback Plan

If critical issues are discovered:

1. Revert commits in reverse order
2. Existing filesystem-based auth will continue working
3. No database migration to rollback (field already existed)

## References

- Design Document: `docs/plans/2026-01-01-epic-auth-improvements-design.md`
- Epic Service: `backend/app/services/epic.py`
- Sync API: `backend/app/api/sync.py`
- Worker Adapter: `backend/app/worker/tasks/sync/adapters/epic.py`
