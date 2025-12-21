# Darkadia Cache Optimization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate redundant IGDB API calls when loading from cache by storing full game metadata instead of just IDs.

**Architecture:** Change cache format from `{game_name: igdb_id}` to `{game_name: {igdb_id, title, release_year}}`, with auto-migration of old integer entries on first access.

**Tech Stack:** Python, JSON, async/await

---

### Task 1: Update Cache Type Definitions and Load Function

**Files:**
- Modify: `backend/scripts/darkadia_to_nexorious.py:469-479`

**Step 1: Update load_igdb_cache return type and add migration detection**

Change the function to return the new format while detecting old entries:

```python
def load_igdb_cache() -> dict[str, Optional[dict[str, Any]]]:
    """Load IGDB ID cache from temp file. Returns dict mapping game name -> cache entry.

    Cache entry is either:
    - dict with {igdb_id, title, release_year} for matched games
    - int (legacy format, will be migrated on use)
    - None for skipped games
    """
    if CACHE_FILE.exists():
        try:
            with open(CACHE_FILE) as f:
                cache = json.load(f)
                # Count format types for info message
                new_format = sum(1 for v in cache.values() if isinstance(v, dict))
                old_format = sum(1 for v in cache.values() if isinstance(v, int))
                skipped = sum(1 for v in cache.values() if v is None)
                print(f"Loaded {len(cache)} cached IGDB decisions from {CACHE_FILE}")
                if old_format > 0:
                    print(f"  ({new_format} new format, {old_format} legacy entries to migrate, {skipped} skipped)")
                return cache
        except (json.JSONDecodeError, IOError) as e:
            print(f"Warning: Could not load cache file: {e}")
    return {}
```

**Step 2: Run script with existing cache to verify load works**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python scripts/darkadia_to_nexorious.py --help`
Expected: Help message displays without errors

**Step 3: Commit**

```bash
git add backend/scripts/darkadia_to_nexorious.py
git commit -m "feat(scripts): update cache load function for new format with migration detection"
```

---

### Task 2: Update Save Cache Function

**Files:**
- Modify: `backend/scripts/darkadia_to_nexorious.py:482-488`

**Step 1: Update save_igdb_cache type signature**

```python
def save_igdb_cache(cache: dict[str, Optional[dict[str, Any] | int]]) -> None:
    """Save IGDB ID cache to temp file."""
    try:
        with open(CACHE_FILE, "w") as f:
            json.dump(cache, f, indent=2)
    except IOError as e:
        print(f"Warning: Could not save cache file: {e}")
```

**Step 2: Commit**

```bash
git add backend/scripts/darkadia_to_nexorious.py
git commit -m "feat(scripts): update cache save function type for new format"
```

---

### Task 3: Update Cache Reading Logic in lookup_igdb_ids

**Files:**
- Modify: `backend/scripts/darkadia_to_nexorious.py:606-625`

**Step 1: Replace cache reading block with format-aware logic**

Replace lines 606-625 with:

```python
        # Check cache first
        if game.name in cache:
            cached_entry = cache[game.name]

            # Handle skipped games (None)
            if cached_entry is None:
                print("  -> From cache: Skipped")
                skipped += 1
                from_cache += 1
                continue

            # Handle new format (dict with full data)
            if isinstance(cached_entry, dict):
                game.igdb_id = cached_entry["igdb_id"]
                game.igdb_title = cached_entry["title"]
                game.release_year = cached_entry.get("release_year")
                print(f"  -> From cache: {game.igdb_title} ({game.release_year or '???'})")
                matched += 1
                from_cache += 1
                continue

            # Handle legacy format (int) - migrate by fetching details
            if isinstance(cached_entry, int):
                print(f"  -> Migrating legacy cache entry (ID: {cached_entry})...")
                game_details = await service.get_game_by_id(cached_entry)
                if game_details:
                    game.igdb_id = cached_entry
                    game.igdb_title = game_details.title
                    game.release_year = extract_year_from_date(game_details.release_date)
                    # Update cache to new format
                    cache[game.name] = {
                        "igdb_id": cached_entry,
                        "title": game_details.title,
                        "release_year": game.release_year,
                    }
                    save_igdb_cache(cache)
                    print(f"  -> Migrated: {game.igdb_title} ({game.release_year or '???'})")
                    matched += 1
                    from_cache += 1
                    continue
                else:
                    print(f"  -> Legacy ID {cached_entry} not found, re-searching...")
```

**Step 2: Commit**

```bash
git add backend/scripts/darkadia_to_nexorious.py
git commit -m "feat(scripts): add format-aware cache reading with legacy migration"
```

---

### Task 4: Update Cache Writing Logic for New Matches

**Files:**
- Modify: `backend/scripts/darkadia_to_nexorious.py:644-646` and `backend/scripts/darkadia_to_nexorious.py:656-661`

**Step 1: Update auto-match cache save (around line 645)**

Change:
```python
            cache[game.name] = exact_match.igdb_id
```

To:
```python
            cache[game.name] = {
                "igdb_id": exact_match.igdb_id,
                "title": exact_match.title,
                "release_year": game.release_year,
            }
```

**Step 2: Update interactive selection cache save (around line 656)**

Change:
```python
                cache[game.name] = game.igdb_id
```

To:
```python
                cache[game.name] = {
                    "igdb_id": game.igdb_id,
                    "title": game.igdb_title,
                    "release_year": game.release_year,
                }
```

**Step 3: Commit**

```bash
git add backend/scripts/darkadia_to_nexorious.py
git commit -m "feat(scripts): save full game metadata to cache instead of just ID"
```

---

### Task 5: Manual Integration Test

**Step 1: Create a test cache file with mixed formats**

```bash
cat > /tmp/darkadia_igdb_cache_test.json << 'EOF'
{
  "Test Game New Format": {"igdb_id": 12345, "title": "Test Game Official", "release_year": 2020},
  "Test Game Legacy": 67890,
  "Test Game Skipped": null
}
EOF
```

**Step 2: Backup real cache and use test cache**

```bash
mv /tmp/darkadia_igdb_cache.json /tmp/darkadia_igdb_cache.json.backup 2>/dev/null || true
cp /tmp/darkadia_igdb_cache_test.json /tmp/darkadia_igdb_cache.json
```

**Step 3: Run script with dry run (will fail at IGDB but shows cache loading)**

```bash
cd /home/abo/workspace/home/nexorious/backend
IGDB_CLIENT_ID=test IGDB_CLIENT_SECRET=test uv run python -c "
import sys
sys.path.insert(0, '.')
from scripts.darkadia_to_nexorious import load_igdb_cache
cache = load_igdb_cache()
print('Cache contents:')
for k, v in cache.items():
    print(f'  {k}: {type(v).__name__} = {v}')
"
```

Expected output:
```
Loaded 3 cached IGDB decisions from /tmp/darkadia_igdb_cache.json
  (1 new format, 1 legacy entries to migrate, 1 skipped)
Cache contents:
  Test Game New Format: dict = {'igdb_id': 12345, 'title': 'Test Game Official', 'release_year': 2020}
  Test Game Legacy: int = 67890
  Test Game Skipped: NoneType = None
```

**Step 4: Restore real cache**

```bash
mv /tmp/darkadia_igdb_cache.json.backup /tmp/darkadia_igdb_cache.json 2>/dev/null || true
rm /tmp/darkadia_igdb_cache_test.json
```

**Step 5: Final commit**

```bash
git add backend/scripts/darkadia_to_nexorious.py
git commit -m "feat(scripts): complete cache optimization - store full metadata

Eliminates redundant IGDB API calls when loading from cache by storing
full game metadata (igdb_id, title, release_year) instead of just IDs.

- New format: {game_name: {igdb_id, title, release_year}}
- Auto-migrates legacy integer entries on first access
- Skipped games remain as null

Performance impact: N cached games now requires 0 API calls
(previously required N calls to fetch title/year)."
```

---

Plan complete and saved to `docs/plans/2025-12-21-darkadia-cache-optimization.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

Which approach?