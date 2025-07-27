"""
Official storefront seed data.

Storefronts represent digital marketplaces where games are sold/distributed.
"""

from typing import List, Dict, Any

OFFICIAL_STOREFRONTS: List[Dict[str, Any]] = [
    {
        "name": "steam",
        "display_name": "Steam",
        "icon_url": None,
        "base_url": "https://store.steampowered.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "epic-games-store",
        "display_name": "Epic Games Store",
        "icon_url": None,
        "base_url": "https://store.epicgames.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "gog",
        "display_name": "GOG",
        "icon_url": None,
        "base_url": "https://www.gog.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "playstation-store",
        "display_name": "PlayStation Store",
        "icon_url": None,
        "base_url": "https://store.playstation.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "microsoft-store",
        "display_name": "Microsoft Store",
        "icon_url": None,
        "base_url": "https://www.microsoft.com/store",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "nintendo-eshop",
        "display_name": "Nintendo eShop",
        "icon_url": None,
        "base_url": "https://www.nintendo.com/us/store",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "itch-io",
        "display_name": "Itch.io",
        "icon_url": None,
        "base_url": "https://itch.io",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "origin-ea-app",
        "display_name": "Origin/EA App",
        "icon_url": None,
        "base_url": "https://www.ea.com/ea-app",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "apple-app-store",
        "display_name": "Apple App Store",
        "icon_url": None,
        "base_url": "https://apps.apple.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "google-play-store",
        "display_name": "Google Play Store",
        "icon_url": None,
        "base_url": "https://play.google.com/store",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "humble-bundle",
        "display_name": "Humble Bundle",
        "icon_url": None,
        "base_url": "https://www.humblebundle.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    }
]