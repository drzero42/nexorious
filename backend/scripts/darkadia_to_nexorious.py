#!/usr/bin/env python3
"""
Darkadia CSV to Nexorious JSON Converter.

Converts Darkadia CSV exports to Nexorious JSON format with IGDB enrichment.

Usage:
    export IGDB_CLIENT_ID="your_client_id"
    export IGDB_CLIENT_SECRET="your_client_secret"
    uv run python scripts/darkadia_to_nexorious.py input.csv output.json
"""

import argparse
import csv
import json
import os
import sys
from datetime import datetime, date
from typing import Optional


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
    "Other": "physical",  # Default fallback for "Other" source
}

# Valid Nexorious storefront names (for validation)
VALID_STOREFRONTS: set[str] = {
    "steam", "epic-games-store", "gog", "playstation-store", "microsoft-store",
    "nintendo-eshop", "itch-io", "origin-ea-app", "apple-app-store",
    "google-play-store", "humble-bundle", "physical", "uplay", "gamersgate",
}


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Convert Darkadia CSV to Nexorious JSON format"
    )
    parser.add_argument("input_csv", help="Path to Darkadia CSV file")
    parser.add_argument("output_json", help="Path for output Nexorious JSON file")

    args = parser.parse_args()

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

    print(f"Converting {args.input_csv} to {args.output_json}")
    print("(Implementation pending)")

    return 0


if __name__ == "__main__":
    sys.exit(main())
