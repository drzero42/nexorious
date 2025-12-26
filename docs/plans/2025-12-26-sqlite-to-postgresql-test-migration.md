# SQLite to PostgreSQL Test Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate 8 test files from SQLite in-memory databases to PostgreSQL testcontainers.

**Architecture:** Leverage existing `integration_test_utils.py` fixtures that provide PostgreSQL testcontainer with transaction rollback isolation. Delete redundant tests, migrate unique coverage to integration tests, and convert remaining files to use shared fixtures.

**Tech Stack:** pytest, testcontainers-postgres, SQLModel, FastAPI TestClient

---

## Task 1: Add Malformed Auth Header Test

**Files:**
- Modify: `backend/app/tests/test_integration_auth.py`

**Step 1: Write the test**

Add to `TestAuthMeEndpoint` class:

```python
def test_get_me_malformed_header(self, client: TestClient):
    """Test GET /me with malformed authorization header (missing Bearer prefix)."""
    headers = {"Authorization": "invalid_header_format"}
    response = client.get("/api/auth/me", headers=headers)

    assert_api_error(response, 403, "Not authenticated")
```

**Step 2: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_auth.py::TestAuthMeEndpoint::test_get_me_malformed_header -v`

Expected: PASS

**Step 3: Commit**

```bash
git add backend/app/tests/test_integration_auth.py
git commit -m "test: add malformed auth header test to integration tests"
```

---

## Task 2: Add Bcrypt Format Validation Test

**Files:**
- Modify: `backend/app/tests/test_integration_auth.py`

**Step 1: Add import**

Add to imports section:

```python
from sqlmodel import Session, select
```

**Step 2: Write the test**

Add to `TestAuthRegisterEndpoint` class:

```python
def test_password_is_hashed_with_bcrypt(self, client: TestClient, session: Session):
    """Test that password is properly hashed using bcrypt."""
    user_data = create_test_user_data()
    response = client.post("/api/auth/register", json=user_data)

    assert_api_success(response, 201)

    # Verify password is hashed with bcrypt in database
    user = session.exec(select(User).where(User.username == user_data["username"])).first()
    assert user is not None
    assert user.password_hash != user_data["password"]
    assert len(user.password_hash) > 50  # Bcrypt hashes are long
    assert user.password_hash.startswith("$2b$")  # Bcrypt identifier
```

**Step 3: Add User import if needed**

Ensure this import exists:

```python
from ..models.user import User, UserSession
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_auth.py::TestAuthRegisterEndpoint::test_password_is_hashed_with_bcrypt -v`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/tests/test_integration_auth.py
git commit -m "test: add bcrypt password hashing verification test"
```

---

## Task 3: Add Field Length Validation Tests

**Files:**
- Modify: `backend/app/tests/test_integration_auth.py`

**Step 1: Write the tests**

Add new class after `TestAuthRegisterEndpoint`:

```python
class TestAuthRegisterValidation:
    """Test registration field validation."""

    def test_username_too_short(self, client: TestClient):
        """Test registration with username too short (less than 3 chars)."""
        register_data = {
            "username": "ab",
            "password": "password123"
        }
        response = client.post("/api/auth/register", json=register_data)

        assert_api_error(response, 422)
        result = response.json()
        assert any("username" in str(error).lower() for error in result["detail"])

    def test_password_too_short(self, client: TestClient):
        """Test registration with password too short (less than 8 chars)."""
        register_data = {
            "username": "testuser",
            "password": "short"
        }
        response = client.post("/api/auth/register", json=register_data)

        assert_api_error(response, 422)
        result = response.json()
        assert any("password" in str(error).lower() for error in result["detail"])

    def test_empty_required_fields(self, client: TestClient):
        """Test registration with empty required fields."""
        register_data = {
            "username": "",
            "password": ""
        }
        response = client.post("/api/auth/register", json=register_data)

        assert_api_error(response, 422)
```

**Step 2: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_auth.py::TestAuthRegisterValidation -v`

Expected: PASS (all 3 tests)

**Step 3: Commit**

```bash
git add backend/app/tests/test_integration_auth.py
git commit -m "test: add field length validation tests for registration"
```

---

## Task 4: Add Date Parsing Tests

**Files:**
- Modify: `backend/app/tests/test_integration_games.py`

**Step 1: Add import**

Add to imports:

```python
from datetime import date
from ..services.game_service import parse_date_string
```

**Step 2: Write the tests**

Add new class at end of file:

```python
class TestParseDateString:
    """Tests for parse_date_string utility function."""

    def test_parse_full_date(self):
        """Test parsing YYYY-MM-DD format."""
        result = parse_date_string("2015-05-19")
        assert result == date(2015, 5, 19)

    def test_parse_year_only(self):
        """Test parsing YYYY format."""
        result = parse_date_string("2015")
        assert result == date(2015, 1, 1)

    def test_parse_none(self):
        """Test parsing None returns None."""
        result = parse_date_string(None)
        assert result is None

    def test_parse_empty_string(self):
        """Test parsing empty string returns None."""
        result = parse_date_string("")
        assert result is None

    def test_parse_invalid_format(self):
        """Test parsing invalid format returns None."""
        result = parse_date_string("19-05-2015")
        assert result is None

    def test_parse_invalid_date(self):
        """Test parsing invalid date returns None."""
        result = parse_date_string("2015-13-45")
        assert result is None
```

**Step 3: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_games.py::TestParseDateString -v`

Expected: PASS (all 6 tests)

**Step 4: Commit**

```bash
git add backend/app/tests/test_integration_games.py
git commit -m "test: add parse_date_string utility tests"
```

---

## Task 5: Delete Redundant Test Files

**Files:**
- Delete: `backend/app/tests/test_igdb_endpoints.py`
- Delete: `backend/app/tests/test_igdb_platform_data.py`
- Delete: `backend/app/tests/test_auth_me.py`
- Delete: `backend/app/tests/test_auth_register.py`
- Delete: `backend/app/tests/test_tag_service.py`

**Step 1: Verify tests pass before deletion**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_auth.py app/tests/test_integration_games.py -v`

Expected: PASS

**Step 2: Delete redundant files**

```bash
cd /home/abo/workspace/home/nexorious/backend
rm app/tests/test_igdb_endpoints.py
rm app/tests/test_igdb_platform_data.py
rm app/tests/test_auth_me.py
rm app/tests/test_auth_register.py
rm app/tests/test_tag_service.py
```

**Step 3: Run full test suite to confirm no breakage**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -v`

Expected: PASS (fewer tests but no failures)

**Step 4: Commit**

```bash
git add -A
git commit -m "test: remove redundant SQLite-based test files

Coverage now provided by PostgreSQL integration tests:
- test_igdb_endpoints.py -> test_integration_games.py
- test_igdb_platform_data.py -> test_integration_games.py
- test_auth_me.py -> test_integration_auth.py
- test_auth_register.py -> test_integration_auth.py
- test_tag_service.py -> test_integration_tags.py"
```

---

## Task 6: Convert test_tag_models.py to PostgreSQL

**Files:**
- Modify: `backend/app/tests/test_tag_models.py`

**Step 1: Replace SQLite fixtures with PostgreSQL fixtures**

Replace the entire fixture section (lines 20-86) with:

```python
@pytest.fixture(name="model_session")
def model_session_fixture(db_session):
    """Use the shared PostgreSQL test session."""
    return db_session


@pytest.fixture(name="test_user_for_model")
def test_user_for_model_fixture(model_session: Session) -> User:
    """Create a test user for model tests."""
    user = User(
        username="test_user",
        password_hash="$2b$12$test_hash",
        is_active=True,
        is_admin=False
    )
    model_session.add(user)
    model_session.commit()
    model_session.refresh(user)
    return user


@pytest.fixture(name="second_test_user")
def second_test_user_fixture(model_session: Session) -> User:
    """Create a second test user for isolation tests."""
    user = User(
        username="second_user",
        password_hash="$2b$12$test_hash_2",
        is_active=True,
        is_admin=False
    )
    model_session.add(user)
    model_session.commit()
    model_session.refresh(user)
    return user


@pytest.fixture(name="test_game_for_model")
def test_game_for_model_fixture(model_session: Session) -> Game:
    """Create a test game for model tests."""
    game = create_test_game(title="Test Game", igdb_id=1001)
    model_session.add(game)
    model_session.commit()
    model_session.refresh(game)
    return game


@pytest.fixture(name="test_user_game_for_model")
def test_user_game_for_model_fixture(model_session: Session, test_user_for_model: User, test_game_for_model: Game) -> UserGame:
    """Create a test user game for model tests."""
    user_game = UserGame(
        user_id=test_user_for_model.id,
        game_id=test_game_for_model.id,
        ownership_status="owned",
        play_status="not_started"
    )
    model_session.add(user_game)
    model_session.commit()
    model_session.refresh(user_game)
    return user_game
```

**Step 2: Remove SQLite imports**

Remove these lines from imports:

```python
from sqlmodel import Session, SQLModel, create_engine, select, and_
from sqlmodel.pool import StaticPool
```

Replace with:

```python
from sqlmodel import Session, select, and_
```

**Step 3: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_tag_models.py -v`

Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/tests/test_tag_models.py
git commit -m "test: convert test_tag_models.py to PostgreSQL testcontainer"
```

---

## Task 7: Convert test_game_service.py to PostgreSQL

**Files:**
- Modify: `backend/app/tests/test_game_service.py`

**Step 1: Replace SQLite fixtures with PostgreSQL fixtures**

Replace the `service_session_fixture` (lines 18-28) with:

```python
@pytest.fixture(name="service_session")
def service_session_fixture(db_session):
    """Use the shared PostgreSQL test session."""
    return db_session
```

**Step 2: Remove SQLite imports**

Remove these lines from imports:

```python
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool
```

Replace with:

```python
from sqlmodel import Session
```

**Step 3: Update race condition test to use db_session**

Replace the `test_race_condition_handling` method (lines 389-451) - it creates its own SQLite engine. Update to use PostgreSQL:

```python
@pytest.mark.asyncio
async def test_race_condition_handling(
    self,
    mock_igdb_service: Mock,
    sample_game_metadata: GameMetadata,
    db_session: Session,
):
    """Test that race condition (duplicate key) is handled gracefully.

    Simulates the scenario where another process creates the same game
    between our existence check and our INSERT.
    """
    mock_igdb_service.get_game_by_id.return_value = sample_game_metadata
    mock_igdb_service.download_and_store_cover_art.return_value = None

    game_service = GameService(db_session, mock_igdb_service)

    # Create the game that will be "found" after rollback
    existing_game = Game(
        id=12345,
        title="The Witcher 3: Wild Hunt",
        description="Created by another process",
    )

    # Track call count to simulate race condition
    original_commit = db_session.commit
    commit_call_count = [0]

    def mock_commit():
        commit_call_count[0] += 1
        if commit_call_count[0] == 1:
            # First commit (after adding new_game) - simulate IntegrityError
            db_session.rollback()
            db_session.add(existing_game)
            original_commit()
            raise IntegrityError(
                "INSERT INTO games",
                params={},
                orig=Exception("duplicate key value violates unique constraint")
            )
        else:
            original_commit()

    with patch.object(db_session, 'commit', side_effect=mock_commit):
        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            download_cover_art=False,
        )

    assert result.id == 12345
    assert result.description == "Created by another process"
```

**Step 4: Update second race condition test similarly**

Replace `test_race_condition_reraises_if_game_not_found` (lines 453-493):

```python
@pytest.mark.asyncio
async def test_race_condition_reraises_if_game_not_found(
    self,
    mock_igdb_service: Mock,
    sample_game_metadata: GameMetadata,
    db_session: Session,
):
    """Test that IntegrityError is re-raised if game still not found after rollback."""
    mock_igdb_service.get_game_by_id.return_value = sample_game_metadata
    mock_igdb_service.download_and_store_cover_art.return_value = None

    game_service = GameService(db_session, mock_igdb_service)

    def mock_commit():
        db_session.rollback()
        raise IntegrityError(
            "INSERT INTO games",
            params={},
            orig=Exception("some other constraint violation")
        )

    with patch.object(db_session, 'commit', side_effect=mock_commit):
        with pytest.raises(IntegrityError):
            await game_service.create_or_update_game_from_igdb(
                igdb_id=12345,
                download_cover_art=False,
            )
```

**Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_game_service.py -v`

Expected: PASS

**Step 6: Commit**

```bash
git add backend/app/tests/test_game_service.py
git commit -m "test: convert test_game_service.py to PostgreSQL testcontainer"
```

---

## Task 8: Convert test_cover_art.py to PostgreSQL

**Files:**
- Modify: `backend/app/tests/test_cover_art.py`

**Step 1: Replace SQLite client fixture with PostgreSQL**

Replace the `client` fixture (lines 37-57) with:

```python
@pytest.fixture
def client(db_session):
    """Create test client using PostgreSQL session."""
    def get_test_session():
        yield db_session

    app.dependency_overrides[get_session] = get_test_session

    client = TestClient(app)
    yield client

    app.dependency_overrides.clear()
```

**Step 2: Remove SQLite imports**

Remove these lines from imports:

```python
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool
```

Add:

```python
from sqlmodel import Session
```

**Step 3: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_cover_art.py -v`

Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/tests/test_cover_art.py
git commit -m "test: convert test_cover_art.py to PostgreSQL testcontainer"
```

---

## Task 9: Final Verification

**Step 1: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -v`

Expected: PASS (all tests)

**Step 2: Run coverage check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`

Expected: >80% coverage maintained

**Step 3: Verify no SQLite references remain**

Run: `grep -r "sqlite" backend/app/tests/`

Expected: No results (or only in comments)

**Step 4: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

Expected: No errors

**Step 5: Commit final state if any cleanup needed**

```bash
git add -A
git commit -m "test: complete SQLite to PostgreSQL migration"
```

---

## Success Criteria

- [ ] All tests pass using PostgreSQL testcontainer
- [ ] No SQLite references remain in test files
- [ ] Test coverage maintained at >80%
- [ ] All unique test cases preserved
- [ ] Type checking passes
