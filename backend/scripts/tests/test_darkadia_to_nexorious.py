"""Tests for the Darkadia to Nexorious CSV converter.

These tests verify the passthrough behavior for unmapped platforms and storefronts.
"""

import pytest
from typing import Optional

import sys
from pathlib import Path

# Add backend directory to path so we can import the module
_backend_dir = Path(__file__).resolve().parent.parent.parent
if str(_backend_dir) not in sys.path:
    sys.path.insert(0, str(_backend_dir))

from scripts.darkadia_to_nexorious import (
    ConsolidatedGame,
    CopyData,
    generate_nexorious_json,
    PLATFORM_MAP,
    STOREFRONT_MAP,
)


def make_test_game(
    name: str = "Test Game",
    platform: str = "PC",
    storefront: str = "Steam",
    igdb_id: int = 12345,
) -> ConsolidatedGame:
    """Helper to create a test game with a single copy."""
    game = ConsolidatedGame(name=name)
    game.igdb_id = igdb_id
    game.igdb_title = name
    game.copies.append(
        CopyData(
            platform=platform,
            storefront=storefront,
            media_type="Digital",
        )
    )
    return game


class TestUnmappedPlatformPassthrough:
    """Tests for unmapped platform passthrough behavior."""

    def test_unmapped_platform_passes_through(self):
        """Unmapped platform should be passed through as-is."""
        # "Unknown Console" is not in PLATFORM_MAP
        game = make_test_game(
            name="Test Game",
            platform="Unknown Console",
            storefront="Steam",
        )

        # Verify it's actually not in the map
        assert "Unknown Console" not in PLATFORM_MAP

        result = generate_nexorious_json([game])

        # The unmapped platform should be passed through as-is
        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["platform_name"] == "Unknown Console"

    def test_mapped_platform_still_works(self):
        """Mapped platforms should still be converted correctly."""
        game = make_test_game(
            name="Test Game",
            platform="PC",
            storefront="Steam",
        )

        # Verify it IS in the map
        assert "PC" in PLATFORM_MAP

        result = generate_nexorious_json([game])

        # The mapped platform should be converted
        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["platform_name"] == "pc-windows"


class TestUnmappedStorefrontPassthrough:
    """Tests for unmapped storefront passthrough behavior."""

    def test_unmapped_storefront_passes_through(self):
        """Unmapped storefront should be passed through as-is."""
        # "Unknown Store" is not in STOREFRONT_MAP
        game = make_test_game(
            name="Test Game",
            platform="PC",
            storefront="Unknown Store",
        )

        # Verify it's actually not in the map
        assert "Unknown Store" not in STOREFRONT_MAP

        result = generate_nexorious_json([game])

        # The unmapped storefront should be passed through as-is
        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["storefront_name"] == "Unknown Store"

    def test_mapped_storefront_still_works(self):
        """Mapped storefronts should still be converted correctly."""
        game = make_test_game(
            name="Test Game",
            platform="PC",
            storefront="Steam",
        )

        # Verify it IS in the map
        assert "Steam" in STOREFRONT_MAP

        result = generate_nexorious_json([game])

        # The mapped storefront should be converted
        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["storefront_name"] == "steam"


class TestEmptyValueHandling:
    """Tests for empty platform/storefront becoming None."""

    def test_empty_platform_becomes_none(self):
        """Empty platform string should become None."""
        game = make_test_game(
            name="Test Game",
            platform="",  # Empty platform
            storefront="Steam",
        )

        result = generate_nexorious_json([game])

        # Empty platform should become None
        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["platform_name"] is None

    def test_empty_storefront_becomes_none(self):
        """Empty storefront string should become None."""
        game = make_test_game(
            name="Test Game",
            platform="PC",
            storefront="",  # Empty storefront
        )

        result = generate_nexorious_json([game])

        # Empty storefront should become None
        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["storefront_name"] is None


class TestDefunctStorefrontHandling:
    """Tests for defunct storefronts (mapped to empty string)."""

    def test_defunct_storefront_becomes_none(self):
        """Storefronts mapped to '' should become None (defunct)."""
        # Telltale.com is mapped to "" in STOREFRONT_MAP (defunct)
        game = make_test_game(
            name="Test Game",
            platform="PC",
            storefront="Telltale.com",
        )

        # Verify it maps to empty string
        assert STOREFRONT_MAP.get("Telltale.com") == ""

        result = generate_nexorious_json([game])

        # Defunct storefront (mapped to "") should become None
        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["storefront_name"] is None


class TestCombinedScenarios:
    """Tests for combined unmapped platform AND storefront."""

    def test_both_unmapped_pass_through(self):
        """Both unmapped platform and storefront should pass through."""
        game = make_test_game(
            name="Test Game",
            platform="Weird Console",
            storefront="Local Store",
        )

        # Verify both are not in maps
        assert "Weird Console" not in PLATFORM_MAP
        assert "Local Store" not in STOREFRONT_MAP

        result = generate_nexorious_json([game])

        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["platform_name"] == "Weird Console"
        assert platforms[0]["storefront_name"] == "Local Store"

    def test_both_empty_become_none(self):
        """Both empty platform and storefront should become None."""
        game = make_test_game(
            name="Test Game",
            platform="",
            storefront="",
        )

        result = generate_nexorious_json([game])

        assert len(result["games"]) == 1
        platforms = result["games"][0]["platforms"]
        assert len(platforms) == 1
        assert platforms[0]["platform_name"] is None
        assert platforms[0]["storefront_name"] is None
