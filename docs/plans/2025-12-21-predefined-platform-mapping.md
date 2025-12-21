# Predefined Platform/Storefront Auto-Matching Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add predefined mappings for common platform/storefront name variations so they auto-match during import without requiring user input.

**Architecture:** Consolidate the two separate mapping systems (`EXPLICIT_PLATFORM_MAPPINGS` in `platform_resolution/models.py` and `abbrev_map` in `review.py`) into a single source of truth. Add PlayStation Portable (PSP) as a new platform.

**Tech Stack:** Python, FastAPI, SQLModel, Alembic, pytest

---

### Task 1: Add PlayStation Portable (PSP) Platform to Seed Data

**Files:**
- Modify: `backend/app/seed_data/platforms.py`

**Step 1: Add PSP platform entry**

Add after the PlayStation Vita entry (around line 54):

```python
    {
        "name": "playstation-psp",
        "display_name": "PlayStation Portable (PSP)",
        "icon_url": "/static/logos/platforms/playstation-psp/playstation-psp-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
```

**Step 2: Verify syntax**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.seed_data.platforms import OFFICIAL_PLATFORMS; print(len(OFFICIAL_PLATFORMS))"`

Expected: `18` (one more than before)

**Step 3: Commit**

```bash
git add backend/app/seed_data/platforms.py
git commit -m "feat: add PlayStation Portable (PSP) platform to seed data"
```

---

### Task 2: Add PSP and Vita Platform-Storefront Associations

**Files:**
- Modify: `backend/app/seed_data/platform_storefront_associations.py`

**Step 1: Add PlayStation Vita associations**

Add after PlayStation 3 associations (around line 31):

```python
    # PlayStation Vita associations
    {"platform_name": "playstation-vita", "storefront_name": "playstation-store"},
    {"platform_name": "playstation-vita", "storefront_name": "physical"},

```

**Step 2: Add PlayStation Portable (PSP) associations**

Add after PlayStation Vita associations:

```python
    # PlayStation Portable (PSP) associations
    {"platform_name": "playstation-psp", "storefront_name": "playstation-store"},
    {"platform_name": "playstation-psp", "storefront_name": "physical"},

```

**Step 3: Verify syntax**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.seed_data.platform_storefront_associations import PLATFORM_STOREFRONT_ASSOCIATIONS; print(len(PLATFORM_STOREFRONT_ASSOCIATIONS))"`

Expected: `32` (4 more than before - 2 for Vita, 2 for PSP)

**Step 4: Commit**

```bash
git add backend/app/seed_data/platform_storefront_associations.py
git commit -m "feat: add PlayStation Vita and PSP storefront associations"
```

---

### Task 3: Expand Explicit Platform Mappings

**Files:**
- Modify: `backend/app/services/platform_resolution/models.py`

**Step 1: Add new mappings to EXPLICIT_PLATFORM_MAPPINGS**

Update the `EXPLICIT_PLATFORM_MAPPINGS` dict (starting at line 13) to include the new mappings:

```python
EXPLICIT_PLATFORM_MAPPINGS = {
    # Short forms that are too different for fuzzy matching
    'PC': 'PC (Windows)',
    'PS3': 'PlayStation 3',
    'PS4': 'PlayStation 4',
    'PS5': 'PlayStation 5',

    # Special cases with very different names
    'PlayStation Network (PS3)': 'PlayStation 3',
    'Xbox 360 Games Store': 'Xbox 360',

    # Darkadia-specific mappings
    'Wii': 'Nintendo Wii',
    'PlayStation Network (Vita)': 'PlayStation Vita',
    'PlayStation Network (PSP)': 'PlayStation Portable (PSP)',
}
```

**Step 2: Verify syntax**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.services.platform_resolution.models import EXPLICIT_PLATFORM_MAPPINGS; print(len(EXPLICIT_PLATFORM_MAPPINGS))"`

Expected: `9` (3 more than before)

**Step 3: Commit**

```bash
git add backend/app/services/platform_resolution/models.py
git commit -m "feat: add Wii, Vita, and PSP explicit platform mappings"
```

---

### Task 4: Update _find_best_match to Use Centralized Mappings

**Files:**
- Modify: `backend/app/api/review.py`

**Step 1: Add import for explicit mappings**

Add to imports at the top of the file (around line 46):

```python
from ..services.platform_resolution.models import (
    EXPLICIT_PLATFORM_MAPPINGS,
    EXPLICIT_STOREFRONT_MAPPINGS,
)
```

**Step 2: Update _find_best_match function**

Replace the `_find_best_match` function (lines 1178-1212) with:

```python
def _find_best_match(
    original: str,
    candidates: list[tuple[str, str, str]],  # (id, name, display_name)
    explicit_mappings: dict[str, str] | None = None,
) -> Optional[tuple[str, str]]:
    """
    Find best matching platform/storefront for a string.

    Uses case-insensitive exact matching on name or display_name,
    then falls back to explicit mappings for known aliases.
    Returns (id, display_name) or None.

    Args:
        original: The original string to match
        candidates: List of (id, name, display_name) tuples
        explicit_mappings: Optional dict of explicit name mappings
    """
    original_lower = original.lower().strip()

    # First try exact match on name or display_name
    for id_, name, display_name in candidates:
        if name.lower() == original_lower or display_name.lower() == original_lower:
            return (id_, display_name)

    # Try explicit mappings if provided
    if explicit_mappings:
        # Check both original case and various case variants
        mapped_name = explicit_mappings.get(original) or explicit_mappings.get(original.strip())
        if mapped_name:
            mapped_lower = mapped_name.lower()
            for id_, name, display_name in candidates:
                if name.lower() == mapped_lower or display_name.lower() == mapped_lower:
                    return (id_, display_name)

    return None
```

**Step 3: Update platform matching call in get_platform_summary**

Find the platform matching call (around line 425) and update it:

```python
            suggested = _find_best_match(
                original,
                [(p.id, p.name, p.display_name) for p in all_platforms],
                EXPLICIT_PLATFORM_MAPPINGS,
            )
```

**Step 4: Update storefront matching call in get_platform_summary**

Find the storefront matching call (around line 448) and update it:

```python
            suggested = _find_best_match(
                original,
                [(s.id, s.name, s.display_name) for s in all_storefronts],
                EXPLICIT_STOREFRONT_MAPPINGS,
            )
```

**Step 5: Verify syntax and type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/api/review.py`

Expected: No errors

**Step 6: Commit**

```bash
git add backend/app/api/review.py
git commit -m "refactor: use centralized explicit mappings in _find_best_match"
```

---

### Task 5: Write Tests for Explicit Mapping Resolution

**Files:**
- Modify: `backend/app/tests/test_review_api.py`

**Step 1: Add test for _find_best_match with explicit mappings**

Add new test class at the end of the file:

```python
class TestFindBestMatch:
    """Tests for the _find_best_match helper function."""

    def test_exact_match_on_display_name(self):
        """Test exact matching on display_name."""
        from app.api.review import _find_best_match

        candidates = [
            ("pc-windows", "pc-windows", "PC (Windows)"),
            ("playstation-5", "playstation-5", "PlayStation 5"),
        ]

        result = _find_best_match("PC (Windows)", candidates)
        assert result == ("pc-windows", "PC (Windows)")

    def test_exact_match_case_insensitive(self):
        """Test case-insensitive exact matching."""
        from app.api.review import _find_best_match

        candidates = [
            ("steam", "steam", "Steam"),
        ]

        result = _find_best_match("STEAM", candidates)
        assert result == ("steam", "Steam")

    def test_explicit_mapping_pc_to_pc_windows(self):
        """Test that 'PC' maps to 'PC (Windows)' via explicit mappings."""
        from app.api.review import _find_best_match
        from app.services.platform_resolution.models import EXPLICIT_PLATFORM_MAPPINGS

        candidates = [
            ("pc-windows", "pc-windows", "PC (Windows)"),
            ("pc-linux", "pc-linux", "PC (Linux)"),
        ]

        result = _find_best_match("PC", candidates, EXPLICIT_PLATFORM_MAPPINGS)
        assert result == ("pc-windows", "PC (Windows)")

    def test_explicit_mapping_wii(self):
        """Test that 'Wii' maps to 'Nintendo Wii' via explicit mappings."""
        from app.api.review import _find_best_match
        from app.services.platform_resolution.models import EXPLICIT_PLATFORM_MAPPINGS

        candidates = [
            ("nintendo-wii", "nintendo-wii", "Nintendo Wii"),
            ("nintendo-wii-u", "nintendo-wii-u", "Nintendo Wii U"),
        ]

        result = _find_best_match("Wii", candidates, EXPLICIT_PLATFORM_MAPPINGS)
        assert result == ("nintendo-wii", "Nintendo Wii")

    def test_explicit_mapping_playstation_network_vita(self):
        """Test that 'PlayStation Network (Vita)' maps to 'PlayStation Vita'."""
        from app.api.review import _find_best_match
        from app.services.platform_resolution.models import EXPLICIT_PLATFORM_MAPPINGS

        candidates = [
            ("playstation-vita", "playstation-vita", "PlayStation Vita"),
            ("playstation-4", "playstation-4", "PlayStation 4"),
        ]

        result = _find_best_match("PlayStation Network (Vita)", candidates, EXPLICIT_PLATFORM_MAPPINGS)
        assert result == ("playstation-vita", "PlayStation Vita")

    def test_explicit_mapping_playstation_network_psp(self):
        """Test that 'PlayStation Network (PSP)' maps to 'PlayStation Portable (PSP)'."""
        from app.api.review import _find_best_match
        from app.services.platform_resolution.models import EXPLICIT_PLATFORM_MAPPINGS

        candidates = [
            ("playstation-psp", "playstation-psp", "PlayStation Portable (PSP)"),
            ("playstation-vita", "playstation-vita", "PlayStation Vita"),
        ]

        result = _find_best_match("PlayStation Network (PSP)", candidates, EXPLICIT_PLATFORM_MAPPINGS)
        assert result == ("playstation-psp", "PlayStation Portable (PSP)")

    def test_no_match_returns_none(self):
        """Test that unrecognized strings return None."""
        from app.api.review import _find_best_match
        from app.services.platform_resolution.models import EXPLICIT_PLATFORM_MAPPINGS

        candidates = [
            ("pc-windows", "pc-windows", "PC (Windows)"),
        ]

        result = _find_best_match("Commodore 64", candidates, EXPLICIT_PLATFORM_MAPPINGS)
        assert result is None
```

**Step 2: Run the new tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_review_api.py::TestFindBestMatch -v`

Expected: All tests PASS

**Step 3: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_review_api.py -v`

Expected: All tests PASS

**Step 4: Commit**

```bash
git add backend/app/tests/test_review_api.py
git commit -m "test: add tests for explicit platform mapping resolution"
```

---

### Task 6: Run Database Seeder and Verify

**Step 1: Run the seeder to add new platform**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.seed_data.seeder import seed_database; from app.core.database import get_session_context; import asyncio; asyncio.run(seed_database())"`

Note: The seeder is idempotent and will add the new PSP platform if it doesn't exist.

**Step 2: Verify PSP platform exists**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "
from app.core.database import engine
from sqlmodel import Session, select
from app.models.platform import Platform
with Session(engine) as session:
    psp = session.exec(select(Platform).where(Platform.name == 'playstation-psp')).first()
    print(f'PSP Platform: {psp.display_name if psp else \"NOT FOUND\"}')"
`

Expected: `PSP Platform: PlayStation Portable (PSP)`

**Step 3: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`

Expected: All tests pass with >80% coverage

**Step 4: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

Expected: No errors

---

### Task 7: Final Commit and Cleanup

**Step 1: Verify all changes**

Run: `git status`

Expected: All changes committed, working tree clean

**Step 2: Squash commits if desired (optional)**

If you want a single commit for the feature:
```bash
git rebase -i HEAD~6
# Mark all but first as "squash"
```

Or leave as incremental commits for better history.
