# Title Normalization for Improved Sync Matching

## Overview

Add a `normalize_title()` function that transforms game titles into a canonical form for better matching. This normalization applies at two points:

1. **Search time** — before querying IGDB, to get better search results
2. **Scoring time** — when comparing query vs candidate titles in `fuzzy_match.py`

The current matching pipeline stays intact. We're only changing how strings are compared, not the matching strategies or confidence thresholds.

## Normalization Rules (Applied in Order)

1. **Expand GOTY** → Game of the Year (case-insensitive)
2. **Remove trademark symbols** — ™ and ®
3. **Remove apostrophes** — straight (`'`) and curly (`'` `'`)
4. **Remove colons** — `:`
5. **Remove standalone dashes** — ` - ` only (preserves hyphens like "Spider-Man")
6. **Remove year in parentheses** — e.g., `(2023)`, `(1998)`
7. **Collapse whitespace** — multiple spaces → single space
8. **Lowercase and trim**

## Important: Normalization is Internal Only

Normalization is **only used for comparison logic** — never displayed to users or stored in the database.

| Context | Title Shown/Stored |
|---------|-------------------|
| Sync review UI | Original title from Steam/Epic/GOG |
| Match candidates | Original title from IGDB |
| Database (UserGame, Game) | Original IGDB title |
| Logs | Original titles |

The normalized form exists only transiently during:
1. IGDB search query construction
2. Confidence score calculation

Users never see "assassins creed" — they see "Assassin's Creed".

## Implementation Location

**New file:** `backend/app/utils/normalize_title.py`

A single `normalize_title(title: str) -> str` function that applies all rules in sequence. Keeping it separate from `fuzzy_match.py` makes it reusable and testable in isolation.

**Integration points:**

| File | Change |
|------|--------|
| `app/utils/fuzzy_match.py` | Call `normalize_title()` on both query and candidate before scoring |
| `app/services/igdb/service.py` | Call `normalize_title()` on query before IGDB search |

**No changes to:**
- Matching strategies or confidence thresholds
- Database models or stored titles (normalization is runtime-only)
- API contracts

## Implementation Details

```python
import re

def normalize_title(title: str) -> str:
    """Normalize a game title for matching purposes."""
    result = title

    # 1. Expand GOTY → Game of the Year (case-insensitive)
    result = re.sub(r'\bGOTY\b', 'Game of the Year', result, flags=re.IGNORECASE)

    # 2. Remove trademark symbols
    result = re.sub(r'[™®]', '', result)

    # 3. Remove apostrophes (straight and curly)
    result = re.sub(r"[''']", '', result)

    # 4. Remove colons
    result = result.replace(':', '')

    # 5. Remove standalone dashes (separators, not hyphens)
    result = re.sub(r' - ', ' ', result)

    # 6. Remove year in parentheses, e.g., (2023)
    result = re.sub(r'\(\d{4}\)', '', result)

    # 7. Collapse whitespace
    result = re.sub(r'\s+', ' ', result)

    # 8. Lowercase and trim
    result = result.lower().strip()

    return result
```

## Testing Strategy

**Unit tests for `normalize_title()`** in `backend/app/tests/test_normalize_title.py`:

| Input | Expected Output |
|-------|-----------------|
| `Doom™` | `doom` |
| `The Witcher 3: Wild Hunt` | `the witcher 3 wild hunt` |
| `Assassin's Creed` | `assassins creed` |
| `Resident Evil 4 (2023)` | `resident evil 4` |
| `Fallout 3 GOTY` | `fallout 3 game of the year` |
| `Wild Hunt - Game of the Year Edition` | `wild hunt game of the year edition` |
| `Spider-Man` | `spider-man` (hyphen preserved) |
| `Half-Life 2` | `half-life 2` (hyphen preserved) |
| `  DOOM  ` | `doom` (trimmed) |
| `Assassin's Creed` | `assassins creed` (curly apostrophe) |

**Integration test:** Verify that a Steam title like `The Witcher® 3: Wild Hunt - GOTY Edition` matches IGDB's `The Witcher 3: Wild Hunt Game of the Year Edition` with high confidence.

## Summary

**Files to create/modify:**
- Create: `backend/app/utils/normalize_title.py`
- Create: `backend/app/tests/test_normalize_title.py`
- Modify: `backend/app/utils/fuzzy_match.py`
- Modify: `backend/app/services/igdb/service.py`

**What stays the same:**
- Matching strategies and 0.85 confidence threshold
- Database storage (titles stored as-is)
- API contracts
