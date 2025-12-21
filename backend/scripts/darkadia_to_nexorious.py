#!/usr/bin/env -S uv run python
"""
Darkadia CSV to Nexorious JSON Converter.

Converts Darkadia CSV exports to Nexorious JSON format with IGDB enrichment.

Usage:
    export IGDB_CLIENT_ID="your_client_id"
    export IGDB_CLIENT_SECRET="your_client_secret"
    ./scripts/darkadia_to_nexorious.py input.csv output.json
"""

import argparse
import asyncio
import csv
import json
import os
import re
import sys
import unicodedata
from dataclasses import dataclass, field
from datetime import datetime, date, timezone
from pathlib import Path
from typing import Any, Optional

# Add backend directory to path so we can import app modules
_backend_dir = Path(__file__).resolve().parent.parent
if str(_backend_dir) not in sys.path:
    sys.path.insert(0, str(_backend_dir))


# =============================================================================
# Platform Mappings (Darkadia name -> Nexorious name)
# =============================================================================
# These map Darkadia platform names to Nexorious platform names.
# Will be expanded as we encounter new platforms during conversion.

PLATFORM_MAP: dict[str, str] = {
    # Direct matches (Darkadia name exactly matches or is close to Nexorious display_name)
    "PlayStation 5": "playstation-5",
    "PlayStation 4": "playstation-4",
    "PlayStation 3": "playstation-3",
    "PlayStation Vita": "playstation-vita",
    "PlayStation 2": "playstation-2",
    "PlayStation": "playstation",
    "Xbox Series X/S": "xbox-series",
    "Xbox One": "xbox-one",
    "Xbox 360": "xbox-360",
    "Nintendo Switch": "nintendo-switch",
    "Nintendo Wii U": "nintendo-wii-u",
    "Nintendo Wii": "nintendo-wii",
    "iOS": "ios",
    "Android": "android",
    "Mac": "mac",

    # Short forms and abbreviations
    "PC": "pc-windows",
    "Linux": "pc-linux",
    "PS5": "playstation-5",
    "PS4": "playstation-4",
    "PS3": "playstation-3",
    "PS2": "playstation-2",
    "PS1": "playstation",
    "PSP": "playstation-psp",
    "Vita": "playstation-vita",
    "Wii": "nintendo-wii",
    "Wii U": "nintendo-wii-u",
    "Switch": "nintendo-switch",

    # Special cases with very different names
    "PlayStation Network (PS3)": "playstation-3",
    "PlayStation Network (Vita)": "playstation-vita",
    "PlayStation Network (PSP)": "playstation-psp",
    "PlayStation Portable (PSP)": "playstation-psp",
    "Xbox 360 Games Store": "xbox-360",
}

# Valid Nexorious platform names (for validation)
VALID_PLATFORMS: set[str] = {
    "pc-windows", "pc-linux", "mac",
    "playstation", "playstation-2", "playstation-3", "playstation-4", "playstation-5",
    "playstation-vita", "playstation-psp",
    "xbox-360", "xbox-one", "xbox-series",
    "nintendo-wii", "nintendo-wii-u", "nintendo-switch", "nintendo-switch-2",
    "ios", "android",
}


# =============================================================================
# Storefront Mappings (Darkadia name -> Nexorious name)
# =============================================================================
# These map Darkadia storefront/source names to Nexorious storefront names.
# Will be expanded as we encounter new storefronts during conversion.

STOREFRONT_MAP: dict[str, str] = {
    # Direct matches
    "Steam": "steam",
    "Epic Games Store": "epic-games-store",
    "GOG": "gog",
    "GOG.com": "gog",
    "PlayStation Store": "playstation-store",
    "Microsoft Store": "microsoft-store",
    "Nintendo eShop": "nintendo-eshop",
    "Itch.io": "itch-io",
    "itch.io": "itch-io",
    "Humble Bundle": "humble-bundle",
    "Physical": "physical",
    "Uplay": "uplay",
    "UPlay": "uplay",
    "GamersGate": "gamersgate",

    # Short forms and abbreviations
    "PSN": "playstation-store",
    "HB": "humble-bundle",
    "Epic": "epic-games-store",
    "Origin": "origin-ea-app",
    "EA App": "origin-ea-app",
    "Origin/EA App": "origin-ea-app",
    "Google Play": "google-play-store",
    "Google Play Store": "google-play-store",
    "App Store": "apple-app-store",
    "Apple App Store": "apple-app-store",

    # Special cases
    "Sony Entertainment Network": "playstation-store",
    # Note: "Other" is NOT mapped here - it's handled specially in parsing:
    # When Copy platform is "Other", platform=PC and storefront comes from field 14

    # Epic Games Store variants
    "Epic Game Store": "epic-games-store",
    "Epic Gamestore": "epic-games-store",

    # Ubisoft variants
    "Ubisoft Club": "uplay",
    "Uplay (coupon w/ GTX 970)": "uplay",

    # Key resellers (keys are typically for Steam)
    "Fanatical": "steam",
    "Green Man Gaming": "steam",
    "GameBillet": "steam",
    "Gamebillet": "steam",
    "Gamesplanet": "steam",
    "Gamesplanet UK": "steam",
    "uk.gamesplanet.com": "steam",
    "Indie Gala": "steam",
    "WinGameStore": "steam",

    # Crowdfunding / backer keys (typically Steam keys)
    "Kickstarter": "steam",
    "Backer": "steam",

    # Defunct storefronts (map to empty, will be filtered out)
    "Telltale.com": "",
    "telltale.com": "",

    # Physical retail stores (Danish/Nordic)
    "Bilka": "physical",
    "Coolshop": "physical",
    "Coolshop.dk": "physical",
    "Elgiganten": "physical",
    "GameStop": "physical",
    "Gamestop": "physical",
    "Nintendopusheren": "physical",
    "Powerplay": "physical",
    "Proshop": "physical",
    "cdon.com": "physical",
}

# Valid Nexorious storefront names (for validation)
# Empty string "" is used for defunct storefronts - these copies will be skipped
VALID_STOREFRONTS: set[str] = {
    "steam", "epic-games-store", "gog", "playstation-store", "microsoft-store",
    "nintendo-eshop", "itch-io", "origin-ea-app", "apple-app-store",
    "google-play-store", "humble-bundle", "physical", "uplay", "gamersgate",
    "",  # Empty = defunct storefront, copy will be skipped
}


# =============================================================================
# Data Classes
# =============================================================================

@dataclass
class CopyData:
    """Data for a single game copy (platform/storefront combination)."""
    platform: str  # Original Darkadia platform name
    storefront: str  # Original Darkadia storefront name
    media_type: str  # Digital/Physical
    purchase_date: Optional[date] = None
    copy_label: str = ""
    csv_row_number: int = 0


@dataclass
class ConsolidatedGame:
    """Consolidated game data from potentially multiple CSV rows."""
    name: str
    copies: list[CopyData] = field(default_factory=list)

    # Consolidated base data (merged from all rows)
    added_date: Optional[date] = None
    loved: bool = False
    owned: bool = False
    played: bool = False
    playing: bool = False
    finished: bool = False
    mastered: bool = False
    dominated: bool = False
    shelved: bool = False
    rating: Optional[float] = None
    notes: str = ""

    # IGDB data (filled in later)
    igdb_id: Optional[int] = None
    igdb_title: Optional[str] = None
    release_year: Optional[int] = None

    # Tracking
    csv_row_numbers: list[int] = field(default_factory=list)


# =============================================================================
# CSV Parsing Functions
# =============================================================================

def parse_bool(value: str) -> bool:
    """Parse boolean from CSV (0/1 or empty)."""
    return value.strip() == "1"


def parse_date(value: str) -> Optional[date]:
    """Parse date from CSV (YYYY-MM-DD format)."""
    value = value.strip()
    if not value or value.lower() == "nan":
        return None
    try:
        return datetime.strptime(value, "%Y-%m-%d").date()
    except ValueError:
        return None


def parse_rating(value: str) -> Optional[float]:
    """Parse rating from CSV (0.0-5.0)."""
    value = value.strip()
    if not value or value.lower() == "nan":
        return None
    try:
        rating = float(value)
        if 0.0 <= rating <= 5.0:
            return rating
        return None
    except ValueError:
        return None


def parse_csv(filepath: str) -> list[ConsolidatedGame]:
    """
    Parse Darkadia CSV and consolidate multi-row games.

    Returns list of ConsolidatedGame objects.
    """
    games: dict[str, ConsolidatedGame] = {}
    current_game_name: Optional[str] = None

    with open(filepath, "r", encoding="utf-8") as f:
        reader = csv.reader(f)
        _header = next(reader)  # Skip header row

        for row_num, row in enumerate(reader, start=2):  # Start at 2 (1-indexed, after header)
            if len(row) < 29:
                print(f"Warning: Row {row_num} has fewer than 29 fields, skipping")
                continue

            # Extract fields (0-indexed)
            name = row[0].strip()
            added = row[1].strip()
            loved = row[2]
            owned = row[3]
            played = row[4]
            playing = row[5]
            finished = row[6]
            mastered = row[7]
            dominated = row[8]
            shelved = row[9]
            rating = row[10]
            copy_label = row[11].strip()
            _copy_release = row[12].strip()
            copy_platform = row[13].strip()
            copy_media = row[14].strip()
            copy_media_other = row[15].strip()
            copy_source = row[16].strip()
            copy_source_other = row[17].strip()
            copy_purchase_date = row[18].strip()
            # Rows 19-27: physical copy metadata (box, manual, etc.) - not used
            platforms_field = row[27].strip() if len(row) > 27 else ""
            notes = row[28].strip() if len(row) > 28 else ""

            # Determine game name (empty = continuation row)
            if name:
                current_game_name = name

            if not current_game_name:
                print(f"Warning: Row {row_num} has no game name and no previous game, skipping")
                continue

            # Get or create game entry
            if current_game_name not in games:
                games[current_game_name] = ConsolidatedGame(name=current_game_name)

            game = games[current_game_name]
            game.csv_row_numbers.append(row_num)

            # Merge base data (OR for booleans, highest for rating, concatenate notes)
            game.loved = game.loved or parse_bool(loved)
            game.owned = game.owned or parse_bool(owned)
            game.played = game.played or parse_bool(played)
            game.playing = game.playing or parse_bool(playing)
            game.finished = game.finished or parse_bool(finished)
            game.mastered = game.mastered or parse_bool(mastered)
            game.dominated = game.dominated or parse_bool(dominated)
            game.shelved = game.shelved or parse_bool(shelved)

            # Rating: use highest
            row_rating = parse_rating(rating)
            if row_rating is not None:
                if game.rating is None or row_rating > game.rating:
                    game.rating = row_rating

            # Added date: use most recent
            row_added = parse_date(added)
            if row_added is not None:
                if game.added_date is None or row_added > game.added_date:
                    game.added_date = row_added

            # Notes: concatenate unique
            if notes and notes not in game.notes:
                if game.notes:
                    game.notes += " | " + notes
                else:
                    game.notes = notes

            # Extract copy data
            # Note: Darkadia's "Copy platform" field (field 13) is confusingly named -
            # it actually contains storefront info, not platform info.
            # When it's "Other", field 14 (copy_media) contains the actual storefront,
            # and the platform defaults to PC.

            if copy_platform.lower() == "other":
                # "Other" means PC platform, and copy_media has the storefront
                platform = "PC"
                storefront = copy_media if copy_media else "Physical"
            else:
                # Normal case: copy_platform is the storefront, need to derive platform
                platform = copy_platform
                if not platform and platforms_field:
                    # Use first platform from comma-separated list as fallback
                    platform = platforms_field.split(",")[0].strip()

                # Determine storefront: copy_source, or copy_source_other if "Other"
                storefront = copy_source
                if storefront.lower() == "other" and copy_source_other:
                    storefront = copy_source_other
                elif not storefront:
                    storefront = ""

            # Determine media type (not used for platform/storefront, kept for reference)
            media_type = copy_media
            if media_type.lower() == "other" and copy_media_other:
                media_type = copy_media_other

            # Only add copy if we have platform info
            if platform:
                copy = CopyData(
                    platform=platform,
                    storefront=storefront if storefront else "Physical",  # Default to Physical
                    media_type=media_type if media_type else "Digital",
                    purchase_date=parse_date(copy_purchase_date),
                    copy_label=copy_label,
                    csv_row_number=row_num,
                )
                game.copies.append(copy)

    return list(games.values())


# =============================================================================
# Validation Functions
# =============================================================================

def validate_mappings(games: list[ConsolidatedGame]) -> tuple[set[str], set[str]]:
    """
    Validate all platforms and storefronts have mappings.

    Returns tuple of (unmapped_platforms, unmapped_storefronts).
    """
    unmapped_platforms: set[str] = set()
    unmapped_storefronts: set[str] = set()

    for game in games:
        for copy in game.copies:
            # Check platform mapping
            if copy.platform not in PLATFORM_MAP:
                unmapped_platforms.add(copy.platform)

            # Check storefront mapping
            if copy.storefront not in STOREFRONT_MAP:
                unmapped_storefronts.add(copy.storefront)

    return unmapped_platforms, unmapped_storefronts


# =============================================================================
# IGDB Integration Functions
# =============================================================================


def normalize_title(title: str) -> str:
    """
    Normalize a game title for comparison.

    - Lowercase
    - Remove punctuation and special characters
    - Normalize unicode (é -> e)
    - Collapse whitespace
    - Handle "The X" vs "X, The" patterns
    """
    if not title:
        return ""

    # Normalize unicode characters (é -> e, etc.)
    title = unicodedata.normalize('NFKD', title)
    title = ''.join(c for c in title if not unicodedata.combining(c))

    # Lowercase
    title = title.lower()

    # Handle "The X" -> "x the" and "X, The" -> "x the" for consistent comparison
    if title.startswith("the "):
        title = title[4:] + " the"
    elif title.endswith(", the"):
        title = title[:-5] + " the"

    # Remove all punctuation and special characters, keep only alphanumeric and spaces
    title = re.sub(r'[^\w\s]', '', title)

    # Collapse whitespace
    title = ' '.join(title.split())

    return title


def extract_year_from_date(iso_date: Optional[str]) -> Optional[int]:
    """Extract year from ISO date string (YYYY-MM-DD format)."""
    if not iso_date:
        return None
    try:
        return int(iso_date.split('-')[0])
    except (ValueError, IndexError, AttributeError):
        return None


# =============================================================================
# IGDB Cache Functions (for resumable lookups)
# =============================================================================

CACHE_FILE = Path("/tmp/darkadia_igdb_cache.json")


def load_igdb_cache() -> dict[str, Optional[int]]:
    """Load IGDB ID cache from temp file. Returns dict mapping game name -> igdb_id (or None if skipped)."""
    if CACHE_FILE.exists():
        try:
            with open(CACHE_FILE) as f:
                cache = json.load(f)
                print(f"Loaded {len(cache)} cached IGDB decisions from {CACHE_FILE}")
                return cache
        except (json.JSONDecodeError, IOError) as e:
            print(f"Warning: Could not load cache file: {e}")
    return {}


def save_igdb_cache(cache: dict[str, Optional[int]]) -> None:
    """Save IGDB ID cache to temp file."""
    try:
        with open(CACHE_FILE, "w") as f:
            json.dump(cache, f, indent=2)
    except IOError as e:
        print(f"Warning: Could not save cache file: {e}")


def clear_igdb_cache() -> None:
    """Delete the cache file."""
    if CACHE_FILE.exists():
        CACHE_FILE.unlink()
        print(f"Cleared cache file: {CACHE_FILE}")


async def setup_igdb_service():
    """Initialize IGDB service with credentials from environment."""
    from app.services.igdb.service import IGDBService

    # Note: IGDBService reads credentials from app.core.config.settings
    # which are populated from environment variables (IGDB_CLIENT_ID, IGDB_CLIENT_SECRET)
    service = IGDBService()

    return service


async def search_igdb_interactive(
    service,
    game_name: str
) -> Optional[tuple[int, str, Optional[int]]]:
    """
    Search IGDB for a game with interactive selection.

    Returns tuple of (igdb_id, title, release_year) or None if skipped.
    """
    search_query = game_name

    while True:
        print(f'\nSearching IGDB for: "{search_query}"')

        results = await service.search_games(search_query)

        if not results:
            print("No results found.")
        else:
            print(f"\nResults ({len(results)} found):")
            for i, game in enumerate(results[:10], 1):  # Show max 10
                year = extract_year_from_date(game.release_date) or "???"
                platforms = ", ".join((game.platform_names or [])[:3])
                if len(game.platform_names or []) > 3:
                    platforms += "..."
                print(f"  {i}. {game.title} ({year}) - {platforms}")

        print("\nOptions:")
        if results:
            print(f"  [1-{min(len(results), 10)}] Select match")
        print("  [s] Enter new search query")
        print("  [i] Enter IGDB ID directly")
        print("  [x] Skip this game")

        choice = input("\nChoice: ").strip().lower()

        if choice == "x":
            return None
        elif choice == "s":
            search_query = input("New search query: ").strip()
            if not search_query:
                search_query = game_name
            continue
        elif choice == "i":
            igdb_id_str = input("Enter IGDB ID: ").strip()
            if igdb_id_str.isdigit():
                igdb_id = int(igdb_id_str)
                # Fetch the game details from IGDB to get title and year
                print(f"Looking up IGDB ID {igdb_id}...")
                game_details = await service.get_game_by_id(igdb_id)
                if game_details:
                    year = extract_year_from_date(game_details.release_date)
                    print(f"  Found: {game_details.title} ({year or '???'})")
                    confirm = input("Use this? [Y/n]: ").strip().lower()
                    if confirm in ("", "y", "yes"):
                        return (igdb_id, game_details.title, year)
                else:
                    print(f"  Could not find game with IGDB ID {igdb_id}")
            else:
                print("Invalid IGDB ID (must be a number)")
            continue
        elif choice.isdigit():
            idx = int(choice) - 1
            if 0 <= idx < min(len(results), 10):
                selected = results[idx]
                return (
                    selected.igdb_id,
                    selected.title,
                    extract_year_from_date(selected.release_date)
                )
            else:
                print("Invalid selection.")
        else:
            print("Invalid choice.")


async def lookup_igdb_ids(
    service,
    games: list[ConsolidatedGame]
) -> list[ConsolidatedGame]:
    """
    Look up IGDB IDs for all games with interactive resolution.

    Uses a cache file to remember decisions, allowing resumption after abort.
    Returns games with igdb_id, igdb_title, and release_year populated.
    """
    # Load existing cache
    cache = load_igdb_cache()

    matched = 0
    skipped = 0
    from_cache = 0

    for i, game in enumerate(games, 1):
        print(f"\n[{i}/{len(games)}] Processing: {game.name}")

        # Check cache first
        if game.name in cache:
            cached_id = cache[game.name]
            if cached_id is not None:
                # Fetch details for cached ID
                game_details = await service.get_game_by_id(cached_id)
                if game_details:
                    game.igdb_id = cached_id
                    game.igdb_title = game_details.title
                    game.release_year = extract_year_from_date(game_details.release_date)
                    print(f"  -> From cache: {game.igdb_title} ({game.release_year})")
                    matched += 1
                    from_cache += 1
                    continue
                else:
                    print(f"  -> Cache hit but IGDB ID {cached_id} not found, re-searching...")
            else:
                print("  -> From cache: Skipped")
                skipped += 1
                from_cache += 1
                continue

        # Try automatic match first (single high-confidence result)
        results = await service.search_games(game.name)

        # Check for exact name match using normalized titles
        exact_match = None
        game_normalized = normalize_title(game.name)
        for result in results:
            if normalize_title(result.title) == game_normalized:
                exact_match = result
                break

        if exact_match:
            game.igdb_id = exact_match.igdb_id
            game.igdb_title = exact_match.title
            game.release_year = extract_year_from_date(exact_match.release_date)
            print(f"  -> Auto-matched: {game.igdb_title} ({game.release_year})")
            matched += 1
            # Save to cache
            cache[game.name] = exact_match.igdb_id
            save_igdb_cache(cache)
        else:
            # Interactive selection needed
            result = await search_igdb_interactive(service, game.name)

            if result:
                game.igdb_id, game.igdb_title, game.release_year = result
                print(f"  -> Selected: {game.igdb_title} ({game.release_year})")
                matched += 1
                # Save to cache
                cache[game.name] = game.igdb_id
            else:
                print("  -> Skipped")
                skipped += 1
                # Save skip to cache (as None)
                cache[game.name] = None

            save_igdb_cache(cache)

    print(f"\n\nIGDB lookup complete: {matched} matched, {skipped} skipped ({from_cache} from cache)")

    # Remove skipped games
    return [g for g in games if g.igdb_id is not None]


# =============================================================================
# JSON Generation Functions
# =============================================================================

def derive_play_status(game: ConsolidatedGame) -> str:
    """Derive Nexorious play status from Darkadia boolean flags."""
    # Priority order (highest first)
    if game.dominated:
        return "dominated"
    if game.mastered:
        return "mastered"
    if game.finished:
        return "completed"
    if game.shelved:
        return "shelved"
    if game.playing:
        return "in_progress"
    if game.played:
        return "completed"  # played but not finished = completed (started at least)
    return "not_started"


def derive_ownership_status(game: ConsolidatedGame) -> str:
    """Derive Nexorious ownership status from Darkadia data."""
    if game.owned:
        return "owned"
    return "no_longer_owned"


def generate_nexorious_json(
    games: list[ConsolidatedGame],
    user_id: str = "darkadia-import"
) -> dict[str, Any]:
    """Generate Nexorious export JSON from consolidated games."""
    now = datetime.now(timezone.utc)

    # Calculate stats
    stats: dict[str, Any] = {
        "total_games": len(games),
        "by_play_status": {},
        "by_ownership_status": {},
        "games_with_ratings": 0,
        "games_with_notes": 0,
        "games_with_tags": 0,
        "loved_games": 0,
        "total_hours_played": 0,
    }

    exported_games = []

    for game in games:
        play_status = derive_play_status(game)
        ownership_status = derive_ownership_status(game)

        # Update stats
        stats["by_play_status"][play_status] = stats["by_play_status"].get(play_status, 0) + 1
        stats["by_ownership_status"][ownership_status] = stats["by_ownership_status"].get(ownership_status, 0) + 1
        if game.rating is not None:
            stats["games_with_ratings"] += 1
        if game.notes:
            stats["games_with_notes"] += 1
        if game.loved:
            stats["loved_games"] += 1

        # Build platform entries (skip copies with empty/defunct storefronts)
        platforms = []
        for copy in game.copies:
            platform_name = PLATFORM_MAP[copy.platform]
            storefront_name = STOREFRONT_MAP[copy.storefront]

            # Skip copies with empty storefront (defunct storefronts like Telltale.com)
            if not storefront_name:
                continue

            platforms.append({
                "platform_id": None,  # Will be resolved on import
                "platform_name": platform_name,
                "storefront_id": None,  # Will be resolved on import
                "storefront_name": storefront_name,
                "store_game_id": None,
                "store_url": None,
                "is_available": True,
            })

        # Build game entry
        game_data = {
            "igdb_id": game.igdb_id,
            "title": game.igdb_title or game.name,
            "release_year": game.release_year,
            "ownership_status": ownership_status,
            "play_status": play_status,
            "personal_rating": game.rating,
            "is_loved": game.loved,
            "hours_played": 0,  # Not tracked in Darkadia CSV
            "personal_notes": game.notes if game.notes else None,
            "acquired_date": game.added_date.isoformat() if game.added_date else None,
            "platforms": platforms,
            "tags": [],
            "created_at": now.isoformat(),
            "updated_at": now.isoformat(),
        }

        exported_games.append(game_data)

    return {
        "export_version": "1.1",
        "export_date": now.isoformat(),
        "user_id": user_id,
        "total_games": len(exported_games),
        "export_stats": stats,
        "games": exported_games,
    }


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Convert Darkadia CSV to Nexorious JSON format"
    )
    parser.add_argument("input_csv", help="Path to Darkadia CSV file")
    parser.add_argument("output_json", help="Path for output Nexorious JSON file")
    parser.add_argument(
        "--clear-cache",
        action="store_true",
        help="Clear the IGDB lookup cache before starting"
    )

    args = parser.parse_args()

    # Handle cache clearing
    if args.clear_cache:
        clear_igdb_cache()

    # Check environment variables
    client_id = os.environ.get("IGDB_CLIENT_ID")
    client_secret = os.environ.get("IGDB_CLIENT_SECRET")

    if not client_id or not client_secret:
        print("Error: IGDB_CLIENT_ID and IGDB_CLIENT_SECRET environment variables required")
        return 1

    # Check input file exists
    if not os.path.exists(args.input_csv):
        print(f"Error: Input file not found: {args.input_csv}")
        return 1

    # Step 1: Parse CSV
    print(f"Parsing {args.input_csv}...")
    games = parse_csv(args.input_csv)
    print(f"Found {len(games)} unique games")

    total_copies = sum(len(g.copies) for g in games)
    print(f"Total copies: {total_copies}")

    if not games:
        print("No games found in CSV.")
        return 1

    # Step 2: Validate mappings (fail fast)
    print("\nValidating platform and storefront mappings...")
    unmapped_platforms, unmapped_storefronts = validate_mappings(games)

    if unmapped_platforms or unmapped_storefronts:
        print("\nError: Unmapped values found. Add these to the mapping dictionaries:\n")

        if unmapped_platforms:
            print("Unmapped platforms:")
            for p in sorted(unmapped_platforms):
                print(f'    "{p}": "???",  # TODO: map to Nexorious platform')

        if unmapped_storefronts:
            print("\nUnmapped storefronts:")
            for s in sorted(unmapped_storefronts):
                print(f'    "{s}": "???",  # TODO: map to Nexorious storefront')

        return 1

    print("All mappings valid!")

    # Step 3: IGDB lookup
    print("\nStarting IGDB lookup...")
    try:
        games = asyncio.run(async_main(games))
    except Exception as e:
        print(f"\nError during IGDB lookup: {e}")
        print("Please check your IGDB credentials and network connection.")
        return 1

    if not games:
        print("No games remaining after IGDB lookup.")
        return 1

    # Step 4: Generate JSON
    print("\nGenerating Nexorious JSON...")
    output = generate_nexorious_json(games)

    # Step 5: Write output
    try:
        with open(args.output_json, "w", encoding="utf-8") as f:
            json.dump(output, f, indent=2)
    except IOError as e:
        print(f"\nError writing output file: {e}")
        return 1

    print(f"\nSuccess! Wrote {len(games)} games to {args.output_json}")
    print("\nSummary:")
    print(f"  Total games: {output['export_stats']['total_games']}")
    print(f"  Games with ratings: {output['export_stats']['games_with_ratings']}")
    print(f"  Games with notes: {output['export_stats']['games_with_notes']}")
    print(f"  Loved games: {output['export_stats']['loved_games']}")

    return 0


async def async_main(games: list[ConsolidatedGame]) -> list[ConsolidatedGame]:
    """Async entry point for IGDB operations."""
    service = await setup_igdb_service()
    return await lookup_igdb_ids(service, games)


if __name__ == "__main__":
    sys.exit(main())
