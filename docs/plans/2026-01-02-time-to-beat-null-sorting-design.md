# Time-to-Beat NULL Sorting Fix

## Problem

When sorting games by time-to-beat in descending order (longest first), games with NULL values appear at the beginning instead of the end. This makes the feature unusable for finding the longest games.

**Root cause:** PostgreSQL's default behavior places NULLs at the beginning for DESC sorts and at the end for ASC sorts. The backend has no explicit NULL handling.

## Solution

Add `.nulls_last()` to the SQLAlchemy sorting logic, ensuring NULL values always sort to the end regardless of sort direction.

## Implementation

**File:** `backend/app/api/user_games.py`

Change the sorting logic from:
```python
if sort_order == "desc":
    query = query.order_by(desc(col(sort_field)))
else:
    query = query.order_by(asc(col(sort_field)))
```

To:
```python
if sort_order == "desc":
    query = query.order_by(desc(col(sort_field)).nulls_last())
else:
    query = query.order_by(asc(col(sort_field)).nulls_last())
```

## Behavior After Change

| Sort Field | Direction | Result |
|------------|-----------|--------|
| time-to-beat | ASC | Shortest -> Longest -> NULLs |
| time-to-beat | DESC | Longest -> Shortest -> NULLs |
| title | ASC | A -> Z -> NULLs |
| title | DESC | Z -> A -> NULLs |

## Testing

Add test cases in `backend/app/tests/test_user_games.py`:

1. **Descending sort with NULLs** - Verify games with NULL `howlongtobeat_main` appear after games with values when sorting DESC
2. **Ascending sort with NULLs** - Verify NULL games still appear at end when sorting ASC

Test approach:
- Create 3 test games: low time (10h), high time (100h), NULL
- Sort DESC -> expect: 100h, 10h, NULL
- Sort ASC -> expect: 10h, 100h, NULL
