# Backend Exception Handling Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Narrow overly broad `except Exception` handlers to catch specific exceptions, improving debuggability and preventing masked errors.

**Architecture:** Replace generic exception handlers with specific exception types in file upload validators, WebSocket handlers, platform schema validators, and storage service. Each change is isolated and can be tested independently.

**Tech Stack:** Python 3.13, FastAPI, Pydantic v2, SQLModel

**Related Issues:** nexorious-9q0, nexorious-3ey, nexorious-0qp, nexorious-ma1, nexorious-5ez

---

## Task 1: Narrow Platform Schema Validator Exceptions (nexorious-0qp)

**Files:**
- Modify: `backend/app/schemas/platform.py:37`, `68`, `115`, `146`
- Test: `backend/app/tests/test_platform_schemas.py` (create if needed)

**Step 1: Write failing test for icon URL validation**

Create `backend/app/tests/test_platform_schemas.py`:

```python
"""Tests for platform schema validators."""
import pytest
from pydantic import ValidationError

from app.schemas.platform import (
    PlatformCreateRequest,
    PlatformUpdateRequest,
    StorefrontCreateRequest,
    StorefrontUpdateRequest,
)


class TestPlatformIconUrlValidation:
    """Tests for icon_url field validation."""

    def test_valid_https_url(self):
        """Accept valid HTTPS URLs."""
        request = PlatformCreateRequest(
            name="test",
            display_name="Test",
            icon_url="https://example.com/icon.png",
        )
        assert request.icon_url == "https://example.com/icon.png"

    def test_valid_static_path(self):
        """Accept relative paths starting with /static/."""
        request = PlatformCreateRequest(
            name="test",
            display_name="Test",
            icon_url="/static/icons/platform.svg",
        )
        assert request.icon_url == "/static/icons/platform.svg"

    def test_invalid_url_raises_validation_error(self):
        """Invalid URLs should raise ValidationError, not generic Exception."""
        with pytest.raises(ValidationError) as exc_info:
            PlatformCreateRequest(
                name="test",
                display_name="Test",
                icon_url="not-a-valid-url",
            )
        # Verify it's a proper validation error, not a masked exception
        assert "icon_url" in str(exc_info.value).lower() or "url" in str(exc_info.value).lower()

    def test_none_url_allowed(self):
        """None should be accepted for optional icon_url."""
        request = PlatformCreateRequest(
            name="test",
            display_name="Test",
            icon_url=None,
        )
        assert request.icon_url is None

    def test_empty_string_becomes_none(self):
        """Empty string should become None."""
        request = PlatformCreateRequest(
            name="test",
            display_name="Test",
            icon_url="   ",
        )
        assert request.icon_url is None
```

**Step 2: Run test to verify it passes (existing behavior)**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_platform_schemas.py -v`

Expected: PASS (existing code should work, we're just adding test coverage)

**Step 3: Update exception handling in PlatformCreateRequest**

Modify `backend/app/schemas/platform.py:37`:

Change:
```python
        except Exception:
            raise PydanticCustomError(
```

To:
```python
        except ValidationError:
            raise PydanticCustomError(
```

Add import at top of file (around line 5):
```python
from pydantic import ValidationError
```

**Step 4: Update exception handling in PlatformUpdateRequest**

Modify `backend/app/schemas/platform.py:68`:

Change:
```python
        except Exception:
            raise PydanticCustomError(
```

To:
```python
        except ValidationError:
            raise PydanticCustomError(
```

**Step 5: Update exception handling in StorefrontCreateRequest**

Modify `backend/app/schemas/platform.py:115`:

Change:
```python
        except Exception:
            raise PydanticCustomError(
```

To:
```python
        except ValidationError:
            raise PydanticCustomError(
```

**Step 6: Update exception handling in StorefrontUpdateRequest**

Modify `backend/app/schemas/platform.py:146`:

Change:
```python
        except Exception:
            raise PydanticCustomError(
```

To:
```python
        except ValidationError:
            raise PydanticCustomError(
```

**Step 7: Run tests to verify changes work**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_platform_schemas.py -v`

Expected: PASS

**Step 8: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 9: Commit**

```bash
git add backend/app/schemas/platform.py backend/app/tests/test_platform_schemas.py
git commit -m "refactor(schemas): narrow exception handling in platform validators

Replace generic 'except Exception' with 'except ValidationError' in
icon_url validators for Platform and Storefront schemas.

Closes: nexorious-0qp"
```

---

## Task 2: Narrow Storage URL Validator Exceptions (nexorious-ma1)

**Files:**
- Modify: `backend/app/services/storage.py:97`, `110`
- Test: `backend/app/tests/test_storage_service.py` (verify existing tests)

**Step 1: Review existing storage tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_storage*.py -v --collect-only 2>/dev/null || echo "No storage tests found"`

**Step 2: Update _is_valid_url exception handling**

Modify `backend/app/services/storage.py:97`:

Change:
```python
    def _is_valid_url(self, url: str) -> bool:
        """Validate URL format."""
        try:
            return url.startswith(('http://', 'https://')) and len(url) > 10
        except Exception:
            return False
```

To:
```python
    def _is_valid_url(self, url: str) -> bool:
        """Validate URL format."""
        try:
            return url.startswith(('http://', 'https://')) and len(url) > 10
        except (TypeError, AttributeError):
            # TypeError: url is not a string (e.g., None, int)
            # AttributeError: url doesn't have startswith method
            return False
```

**Step 3: Update _validate_image_file exception handling**

Modify `backend/app/services/storage.py:110`:

First, add import at top of file:
```python
from PIL import UnidentifiedImageError
```

Then change:
```python
        except Exception:
            return False
```

To:
```python
        except (OSError, UnidentifiedImageError):
            # OSError: file system errors, corrupted files
            # UnidentifiedImageError: PIL can't identify the image format
            return False
```

**Step 4: Run storage-related tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -k "storage" -v`

Expected: PASS

**Step 5: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 6: Commit**

```bash
git add backend/app/services/storage.py
git commit -m "refactor(storage): narrow exception handling in URL and image validators

Replace generic 'except Exception' with specific exceptions:
- _is_valid_url: TypeError, AttributeError
- _validate_image_file: OSError, UnidentifiedImageError

Closes: nexorious-ma1"
```

---

## Task 3: Narrow File Upload Validator Exceptions (nexorious-9q0)

**Files:**
- Modify: `backend/app/security/file_upload_validator.py:203`, `406`, `453`, `485`, `504`
- Test: `backend/app/tests/test_file_upload_validator.py` (verify existing tests)

**Step 1: Run existing file upload validator tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -k "file_upload" -v`

**Step 2: Update validate_file top-level exception (line 203)**

Modify `backend/app/security/file_upload_validator.py:203`:

Change:
```python
        except Exception as e:
            logger.error(f"File validation error for user {user_id}: {e}")
            result.errors.append("File validation failed due to internal error")
            return result
```

To:
```python
        except (OSError, IOError, UnicodeDecodeError, csv.Error) as e:
            logger.error(f"File validation error for user {user_id}: {e}")
            result.errors.append("File validation failed due to internal error")
            return result
```

**Step 3: Update _validate_csv_structure exception (line 406)**

Modify `backend/app/security/file_upload_validator.py:406`:

Change:
```python
        except Exception as e:
            errors.append(f"CSV validation error: {str(e)}")
            return False, 0, 0, errors
```

To:
```python
        except (UnicodeDecodeError, ValueError) as e:
            errors.append(f"CSV validation error: {str(e)}")
            return False, 0, 0, errors
```

**Step 4: Update _create_secure_temp_file exception (line 453)**

Modify `backend/app/security/file_upload_validator.py:453`:

Change:
```python
        except Exception as e:
            logger.error(f"Failed to create secure temp file for user {user_id}: {e}")
            raise RuntimeError(f"Temporary file creation failed: {str(e)}")
```

To:
```python
        except (OSError, IOError, PermissionError) as e:
            logger.error(f"Failed to create secure temp file for user {user_id}: {e}")
            raise RuntimeError(f"Temporary file creation failed: {str(e)}")
```

**Step 5: Update cleanup_temp_file exception (line 485)**

Modify `backend/app/security/file_upload_validator.py:485`:

Change:
```python
        except Exception as e:
            logger.error(f"Failed to cleanup temp file {file_path}: {e}")
            return False
```

To:
```python
        except (OSError, IOError, PermissionError) as e:
            logger.error(f"Failed to cleanup temp file {file_path}: {e}")
            return False
```

**Step 6: Update validate_file_permissions exception (line 504)**

Modify `backend/app/security/file_upload_validator.py:504`:

Change:
```python
        except Exception as e:
            logger.error(f"Failed to check file permissions for {file_path}: {e}")
            return False
```

To:
```python
        except (OSError, IOError) as e:
            logger.error(f"Failed to check file permissions for {file_path}: {e}")
            return False
```

**Step 7: Run file upload validator tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -k "file_upload or validator" -v`

Expected: PASS

**Step 8: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 9: Commit**

```bash
git add backend/app/security/file_upload_validator.py
git commit -m "refactor(security): narrow exception handling in file upload validators

Replace generic 'except Exception' with specific exceptions:
- validate_file: OSError, IOError, UnicodeDecodeError, csv.Error
- _validate_csv_structure: UnicodeDecodeError, ValueError
- _create_secure_temp_file: OSError, IOError, PermissionError
- cleanup_temp_file: OSError, IOError, PermissionError
- validate_file_permissions: OSError, IOError

Closes: nexorious-9q0"
```

---

## Task 4: Narrow WebSocket Rollback Exceptions (nexorious-3ey)

**Files:**
- Modify: `backend/app/api/websocket.py:221`, `368`, `373`
- Test: `backend/app/tests/test_websocket.py` (verify existing tests)

**Step 1: Run existing WebSocket tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -k "websocket" -v`

**Step 2: Update send message exception (line 221)**

Modify `backend/app/api/websocket.py:221`:

First, add imports at top of file:
```python
from starlette.websockets import WebSocketDisconnect
```

Change:
```python
    except Exception as e:
        logger.debug(f"Failed to send WebSocket message: {e}")
        return False
```

To:
```python
    except (WebSocketDisconnect, RuntimeError, ConnectionError) as e:
        # WebSocketDisconnect: client disconnected
        # RuntimeError: WebSocket not connected or already closed
        # ConnectionError: network connection issues
        logger.debug(f"Failed to send WebSocket message: {e}")
        return False
```

**Step 3: Update polling loop exception (line 368)**

Modify `backend/app/api/websocket.py:368`:

Change:
```python
            except Exception as e:
                logger.error(f"Error in WebSocket polling loop: {e}")
```

To:
```python
            except (SQLAlchemyError, WebSocketDisconnect, RuntimeError) as e:
                logger.error(f"Error in WebSocket polling loop: {e}")
```

Add import at top of file:
```python
from sqlalchemy.exc import SQLAlchemyError
```

**Step 4: Update rollback exception (line 373)**

Modify `backend/app/api/websocket.py:373`:

Change:
```python
                except Exception:
                    pass  # Ignore rollback errors
```

To:
```python
                except SQLAlchemyError:
                    pass  # Ignore rollback errors - session may already be invalid
```

**Step 5: Run WebSocket tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -k "websocket" -v`

Expected: PASS

**Step 6: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 7: Commit**

```bash
git add backend/app/api/websocket.py
git commit -m "refactor(api): narrow exception handling in WebSocket handlers

Replace generic 'except Exception' with specific exceptions:
- send_message: WebSocketDisconnect, RuntimeError, ConnectionError
- polling loop: SQLAlchemyError, WebSocketDisconnect, RuntimeError
- rollback: SQLAlchemyError

Closes: nexorious-3ey"
```

---

## Task 5: Close Migration Exception Issue (nexorious-5ez)

**Files:**
- None (no code changes needed)

**Step 1: Verify no broad exceptions in migrations**

Run: `cd /home/abo/workspace/home/nexorious/backend && grep -r "except Exception\|except:" app/alembic/ || echo "No broad exception handlers found"`

Expected: "No broad exception handlers found"

**Step 2: Close the beads issue**

Run: `bd close nexorious-5ez --reason="No action needed - migration files do not contain broad exception handlers"`

**Step 3: Sync beads**

Run: `bd sync`

---

## Final Verification

**Step 1: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

Expected: No new errors

**Step 2: Run full test suite with coverage**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing -q`

Expected: All tests pass, coverage >80%

**Step 3: Close remaining beads issues**

Run:
```bash
bd close nexorious-9q0 nexorious-3ey nexorious-0qp nexorious-ma1
bd sync
```
