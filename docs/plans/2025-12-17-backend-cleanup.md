# Backend Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove unused code from the backend to reduce maintenance burden and improve code clarity.

**Architecture:** Simple deletions of unused functions. Each task is independent and safe to execute in any order.

**Tech Stack:** Python 3.13, SQLAlchemy, FastAPI

**Related Issues:** nexorious-2cc, nexorious-yiz

---

## Task 1: Remove Unused SQLAlchemy Wrapper Functions (nexorious-2cc)

**Files:**
- Modify: `backend/app/utils/sqlalchemy_typed.py:79-95`, `118-135`, `194-211`, `214-231`, `234-251`

**Step 1: Verify functions are unused**

Run:
```bash
cd /home/abo/workspace/home/nexorious/backend && grep -r "from.*sqlalchemy_typed import.*like\|from.*sqlalchemy_typed import.*not_in\|from.*sqlalchemy_typed import.*contains\|from.*sqlalchemy_typed import.*startswith\|from.*sqlalchemy_typed import.*endswith" app/ --include="*.py" | grep -v sqlalchemy_typed.py
```

Expected: No matches (functions are not imported anywhere)

Also verify with direct usage:
```bash
cd /home/abo/workspace/home/nexorious/backend && grep -r "sqlalchemy_typed\.like\|sqlalchemy_typed\.not_in\|sqlalchemy_typed\.contains\|sqlalchemy_typed\.startswith\|sqlalchemy_typed\.endswith" app/ --include="*.py"
```

Expected: No matches

**Step 2: Remove like() function (lines 79-95)**

Delete from `backend/app/utils/sqlalchemy_typed.py`:

```python
def like(column: Any, pattern: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.like() method.

    Performs case-sensitive LIKE comparison.

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        pattern: SQL LIKE pattern with % wildcards

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(like(User.name, '%John%'))  # WHERE name LIKE '%John%'
    """
    return column.like(pattern)  # type: ignore[no-any-return]
```

**Step 3: Remove not_in() function (lines 118-135)**

Delete from `backend/app/utils/sqlalchemy_typed.py`:

```python
def not_in(column: Any, values: Any) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.not_in() method.

    Checks if column value is not in a collection of values or subquery.

    Args:
        column: SQLAlchemy column or expression
        values: Collection of values (list, set, tuple) or a SQLAlchemy subquery

    Returns:
        SQLAlchemy column element representing the NOT IN comparison

    Example:
        query.where(not_in(User.status, ['deleted', 'banned']))
        # WHERE status NOT IN ('deleted', 'banned')
    """
    return column.not_in(values)  # type: ignore[no-any-return]
```

**Step 4: Remove contains() function (lines 194-211)**

Delete from `backend/app/utils/sqlalchemy_typed.py`:

```python
def contains(column: Any, value: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.contains() method.

    Checks if column contains a substring (translates to LIKE '%value%').

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        value: Substring to search for

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(contains(User.email, '@gmail.com'))
        # WHERE email LIKE '%@gmail.com%'
    """
    return column.contains(value)  # type: ignore[no-any-return]
```

**Step 5: Remove startswith() function (lines 214-231)**

Delete from `backend/app/utils/sqlalchemy_typed.py`:

```python
def startswith(column: Any, value: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.startswith() method.

    Checks if column starts with a substring (translates to LIKE 'value%').

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        value: Prefix to search for

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(startswith(User.username, 'admin'))
        # WHERE username LIKE 'admin%'
    """
    return column.startswith(value)  # type: ignore[no-any-return]
```

**Step 6: Remove endswith() function (lines 234-251)**

Delete from `backend/app/utils/sqlalchemy_typed.py`:

```python
def endswith(column: Any, value: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.endswith() method.

    Checks if column ends with a substring (translates to LIKE '%value').

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        value: Suffix to search for

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(endswith(User.email, '.com'))
        # WHERE email LIKE '%.com'
    """
    return column.endswith(value)  # type: ignore[no-any-return]
```

**Step 7: Run tests to verify nothing breaks**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 8: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

Expected: No new errors

**Step 9: Commit**

```bash
git add backend/app/utils/sqlalchemy_typed.py
git commit -m "cleanup(utils): remove unused SQLAlchemy wrapper functions

Remove unused functions from sqlalchemy_typed.py:
- like() - direct .like() calls used instead
- not_in() - direct .not_in() calls used instead
- contains() - never imported
- startswith() - never imported
- endswith() - never imported

Keeps: is_(), is_not(), ilike(), in_(), desc(), asc(), label()

Closes: nexorious-2cc"
```

---

## Task 2: Remove Unused get_user_agent() Function (nexorious-yiz)

**Files:**
- Modify: `backend/app/core/audit_logging.py:248-253`

**Step 1: Verify function is unused**

Run:
```bash
cd /home/abo/workspace/home/nexorious/backend && grep -r "get_user_agent" app/ --include="*.py" | grep -v "def get_user_agent"
```

Expected: No matches (function is defined but never called)

**Step 2: Remove get_user_agent() function**

Delete from `backend/app/core/audit_logging.py` (lines 248-253):

```python
def get_user_agent(request) -> Optional[str]:
    """Extract user agent from request."""
    if not hasattr(request, 'headers'):
        return None

    return request.headers.get("User-Agent")
```

**Step 3: Run tests to verify nothing breaks**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --tb=short -q`

Expected: All tests pass

**Step 4: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

Expected: No new errors

**Step 5: Commit**

```bash
git add backend/app/core/audit_logging.py
git commit -m "cleanup(audit): remove unused get_user_agent function

Remove orphaned get_user_agent() function from audit_logging.py.
Its companion get_client_ip() is used, but get_user_agent() was
never called anywhere in the codebase.

Closes: nexorious-yiz"
```

---

## Task 3: Close Issues and Sync

**Step 1: Close issues**

Run:
```bash
bd close nexorious-2cc nexorious-yiz
```

**Step 2: Sync beads**

Run: `bd sync`

---

## Final Verification

**Step 1: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing -q`

Expected: All tests pass, coverage >80%

**Step 2: Verify no regressions**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

Expected: No new errors

---

## Summary

| Task | Issue | Lines Removed | Risk |
|------|-------|---------------|------|
| Remove SQLAlchemy wrappers | nexorious-2cc | ~90 lines | Low |
| Remove get_user_agent | nexorious-yiz | ~6 lines | Low |

**Total cleanup:** ~96 lines of unused code removed.
