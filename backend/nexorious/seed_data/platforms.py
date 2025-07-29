"""
Official platform seed data.

Platforms represent hardware/operating systems that games run on.
"""

from typing import List, Dict, Any

OFFICIAL_PLATFORMS: List[Dict[str, Any]] = [
    {
        "name": "pc-windows",
        "display_name": "PC (Windows)",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "steam"
    },
    {
        "name": "playstation-5",
        "display_name": "PlayStation 5",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "playstation-4",
        "display_name": "PlayStation 4",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "playstation-3",
        "display_name": "PlayStation 3",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "xbox-series",
        "display_name": "Xbox Series X/S",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "microsoft-store"
    },
    {
        "name": "xbox-one",
        "display_name": "Xbox One",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "microsoft-store"
    },
    {
        "name": "xbox-360",
        "display_name": "Xbox 360",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "microsoft-store"
    },
    {
        "name": "nintendo-switch",
        "display_name": "Nintendo Switch",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "nintendo-eshop"
    },
    {
        "name": "nintendo-wii",
        "display_name": "Nintendo Wii",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "nintendo-eshop"
    },
    {
        "name": "ios",
        "display_name": "iOS",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "apple-app-store"
    },
    {
        "name": "android",
        "display_name": "Android",
        "icon_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "google-play-store"
    }
]