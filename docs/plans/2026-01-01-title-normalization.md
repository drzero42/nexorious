# Title Normalization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve auto-matching during sync by normalizing game titles before comparison.

**Architecture:** Create a `normalize_title()` function in `app/utils/normalize_title.py` that canonicalizes titles for comparison. Integrate it into fuzzy matching (scoring) and IGDB search (query construction). Normalization is internal only — never displayed or stored.

**Tech Stack:** Python, regex, pytest

---

## Task 1: Create normalize_title function with tests

**Files:**
- Create: `backend/app/utils/normalize_title.py`
- Create: `backend/app/tests/test_normalize_title.py`

**Step 1: Write the test file**

```python
"""Tests for title normalization utilities."""
import pytest
from app.utils.normalize_title import normalize_title


class TestNormalizeTitle:
    """Test cases for normalize_title function."""

    def test_removes_trademark_symbol(self):
        assert normalize_title("Doom™") == "doom"

    def test_removes_registered_symbol(self):
        assert normalize_title("The Witcher®") == "the witcher"

    def test_removes_colons(self):
        assert normalize_title("The Witcher 3: Wild Hunt") == "the witcher 3 wild hunt"

    def test_removes_apostrophes_straight(self):
        assert normalize_title("Assassin's Creed") == "assassins creed"

    def test_removes_apostrophes_curly(self):
        assert normalize_title("Assassin's Creed") == "assassins creed"

    def test_removes_year_in_parentheses(self):
        assert normalize_title("Resident Evil 4 (2023)") == "resident evil 4"

    def test_expands_goty(self):
        assert normalize_title("Fallout 3 GOTY") == "fallout 3 game of the year"

    def test_expands_goty_case_insensitive(self):
        assert normalize_title("Fallout 3 goty") == "fallout 3 game of the year"

    def test_removes_standalone_dashes(self):
        result = normalize_title("Wild Hunt - Game of the Year Edition")
        assert result == "wild hunt game of the year edition"

    def test_preserves_hyphens_in_words(self):
        assert normalize_title("Spider-Man") == "spider-man"

    def test_preserves_hyphens_in_words_with_numbers(self):
        assert normalize_title("Half-Life 2") == "half-life 2"

    def test_collapses_whitespace(self):
        assert normalize_title("Doom   Eternal") == "doom eternal"

    def test_trims_whitespace(self):
        assert normalize_title("  DOOM  ") == "doom"

    def test_lowercases(self):
        assert normalize_title("DOOM ETERNAL") == "doom eternal"

    def test_combined_normalizations(self):
        result = normalize_title("The Witcher® 3: Wild Hunt - GOTY Edition")
        assert result == "the witcher 3 wild hunt game of the year edition"

    def test_empty_string(self):
        assert normalize_title("") == ""

    def test_only_whitespace(self):
        assert normalize_title("   ") == ""

    def test_multiple_trademark_symbols(self):
        assert normalize_title("EA™ Sports™ FC™") == "ea sports fc"

    def test_year_at_end(self):
        assert normalize_title("FIFA (2023)") == "fifa"

    def test_year_in_middle(self):
        assert normalize_title("NBA 2K (2023) Edition") == "nba 2k edition"
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest app/tests/test_normalize_title.py -v`

Expected: FAIL with `ModuleNotFoundError: No module named 'app.utils.normalize_title'`

**Step 3: Write the implementation**

```python
"""
Title normalization utilities for improved matching.

Normalizes game titles to a canonical form for comparison during sync matching.
This normalization is internal only - never displayed to users or stored in the database.
"""
import re


def normalize_title(title: str) -> str:
    """
    Normalize a game title for matching purposes.

    Applies the following transformations in order:
    1. Expand GOTY -> Game of the Year (case-insensitive)
    2. Remove trademark symbols (TM, R)
    3. Remove apostrophes (straight and curly)
    4. Remove colons
    5. Remove standalone dashes ( - ) but preserve hyphens (Spider-Man)
    6. Remove year in parentheses, e.g., (2023)
    7. Collapse whitespace
    8. Lowercase and trim

    Args:
        title: The game title to normalize

    Returns:
        Normalized title string for comparison
    """
    result = title

    # 1. Expand GOTY -> Game of the Year (case-insensitive)
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

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest app/tests/test_normalize_title.py -v`

Expected: All tests PASS

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pyrefly check app/utils/normalize_title.py`

Expected: No errors

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend
git add app/utils/normalize_title.py app/tests/test_normalize_title.py
git commit -m "feat: add normalize_title function for sync matching"
```

---

## Task 2: Integrate normalize_title into fuzzy_match.py

**Files:**
- Modify: `backend/app/utils/fuzzy_match.py`
- Modify: `backend/app/tests/test_normalize_title.py` (add integration test)

**Step 1: Add integration test**

Add to `test_normalize_title.py`:

```python
class TestFuzzyMatchIntegration:
    """Test that normalization improves fuzzy matching."""

    def test_witcher_goty_matches_with_normalization(self):
        """Steam's GOTY should match IGDB's Game of the Year."""
        from app.utils.fuzzy_match import calculate_fuzzy_confidence

        steam_title = "The Witcher® 3: Wild Hunt - GOTY Edition"
        igdb_title = "The Witcher 3: Wild Hunt Game of the Year Edition"

        confidence = calculate_fuzzy_confidence(steam_title, igdb_title)
        # Should be high enough for auto-match (>= 0.85)
        assert confidence >= 0.85, f"Expected >= 0.85, got {confidence}"

    def test_resident_evil_year_suffix_matches(self):
        """Year suffix should not prevent matching."""
        from app.utils.fuzzy_match import calculate_fuzzy_confidence

        steam_title = "Resident Evil 4 (2023)"
        igdb_title = "Resident Evil 4"

        confidence = calculate_fuzzy_confidence(steam_title, igdb_title)
        assert confidence >= 0.85, f"Expected >= 0.85, got {confidence}"

    def test_assassins_creed_apostrophe_matches(self):
        """Different apostrophe styles should match."""
        from app.utils.fuzzy_match import calculate_fuzzy_confidence

        steam_title = "Assassin's Creed"
        igdb_title = "Assassin's Creed"

        confidence = calculate_fuzzy_confidence(steam_title, igdb_title)
        assert confidence >= 0.85, f"Expected >= 0.85, got {confidence}"
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest app/tests/test_normalize_title.py::TestFuzzyMatchIntegration -v`

Expected: FAIL (at least one test should fail because normalization isn't integrated yet)

**Step 3: Modify fuzzy_match.py to use normalize_title**

Replace lines 21-22 in `fuzzy_match.py`:

```python
    query_lower = query.lower().strip()
    title_lower = title.lower().strip()
```

With:

```python
    from app.utils.normalize_title import normalize_title

    query_normalized = normalize_title(query)
    title_normalized = normalize_title(title)
```

And update the comparisons (lines 25-29) to use the normalized versions:

```python
    # Different matching strategies
    exact_score = 1.0 if query_normalized == title_normalized else 0.0
    ratio_score = fuzz.ratio(query_normalized, title_normalized) / 100.0
    partial_score = fuzz.partial_ratio(query_normalized, title_normalized) / 100.0
    token_sort_score = fuzz.token_sort_ratio(query_normalized, title_normalized) / 100.0
    token_set_score = fuzz.token_set_ratio(query_normalized, title_normalized) / 100.0
```

**Step 4: Run integration tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest app/tests/test_normalize_title.py -v`

Expected: All tests PASS

**Step 5: Run existing fuzzy_match tests to ensure no regression**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest app/tests/test_fuzzy_match.py -v`

Expected: All tests PASS

**Step 6: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pyrefly check app/utils/fuzzy_match.py`

Expected: No errors

**Step 7: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend
git add app/utils/fuzzy_match.py app/tests/test_normalize_title.py
git commit -m "feat: integrate normalize_title into fuzzy matching"
```

---

## Task 3: Integrate normalize_title into IGDB search

**Files:**
- Modify: `backend/app/services/igdb/service.py`
- Modify: `backend/app/tests/test_normalize_title.py` (add search test)

**Step 1: Add test for normalized search query**

Add to `test_normalize_title.py`:

```python
class TestIGDBSearchNormalization:
    """Test that search queries are normalized before IGDB lookup."""

    def test_search_query_normalized(self):
        """Verify normalize_title is applied to search queries."""
        from app.utils.normalize_title import normalize_title

        # Simulate what service.py should do
        steam_title = "The Witcher® 3: Wild Hunt - GOTY Edition"
        normalized = normalize_title(steam_title)

        # The normalized query should be cleaner for IGDB search
        assert "®" not in normalized
        assert "GOTY" not in normalized
        assert "game of the year" in normalized
```

**Step 2: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest app/tests/test_normalize_title.py::TestIGDBSearchNormalization -v`

Expected: PASS (this just validates the function works correctly)

**Step 3: Modify service.py to normalize search query**

In `search_games` method (around line 154), after the empty query check:

Change line 168:
```python
            fuzzy_task = self._perform_single_search(query.strip(), limit * 2)
```

To:
```python
            from app.utils.normalize_title import normalize_title
            normalized_query = normalize_title(query)
            fuzzy_task = self._perform_single_search(normalized_query, limit * 2)
```

Also update line 169:
```python
            exact_task = self._search_by_exact_name(query.strip(), limit)
```

To:
```python
            exact_task = self._search_by_exact_name(normalized_query, limit)
```

**Step 4: Run IGDB service tests to ensure no regression**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest app/tests/test_igdb_service.py -v`

Expected: All tests PASS

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pyrefly check app/services/igdb/service.py`

Expected: No errors

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend
git add app/services/igdb/service.py app/tests/test_normalize_title.py
git commit -m "feat: normalize search queries before IGDB lookup"
```

---

## Task 4: Run full test suite and verify

**Files:**
- None (verification only)

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pytest --ignore=app/tests/test_sync_process_item.py -v`

Expected: All tests PASS (except pre-existing Epic service failures)

**Step 2: Run type checking on all modified files**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run pyrefly check app/utils/normalize_title.py app/utils/fuzzy_match.py app/services/igdb/service.py`

Expected: No errors

**Step 3: Run linting**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/title-normalization/backend && uv run ruff check app/utils/normalize_title.py app/utils/fuzzy_match.py app/services/igdb/service.py`

Expected: No errors

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Create normalize_title function with tests | `normalize_title.py`, `test_normalize_title.py` |
| 2 | Integrate into fuzzy_match.py | `fuzzy_match.py` |
| 3 | Integrate into IGDB service | `service.py` |
| 4 | Full test suite verification | (none) |
