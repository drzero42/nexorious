"""
Official platform seed data.

Platforms represent hardware/operating systems that games run on.
"""

from typing import List, Dict, Any

OFFICIAL_PLATFORMS: List[Dict[str, Any]] = [
    {
        "name": "pc-windows",
        "display_name": "PC (Windows)",
        "icon_url": "/static/logos/platforms/pc-windows/pc-windows-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "steam"
    },
    {
        "name": "playstation-5",
        "display_name": "PlayStation 5",
        "icon_url": "/static/logos/platforms/playstation-5/playstation-5-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "playstation-4",
        "display_name": "PlayStation 4",
        "icon_url": "/static/logos/platforms/playstation-4/playstation-4-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "playstation-3",
        "display_name": "PlayStation 3",
        "icon_url": "/static/logos/platforms/playstation-3/playstation-3-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "playstation-vita",
        "display_name": "PlayStation Vita",
        "icon_url": "/static/logos/platforms/playstation-vita/playstation-vita-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "playstation-psp",
        "display_name": "PlayStation Portable (PSP)",
        "icon_url": "/static/logos/platforms/playstation-psp/playstation-psp-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "playstation-store"
    },
    {
        "name": "xbox-series",
        "display_name": "Xbox Series X/S",
        "icon_url": "/static/logos/platforms/xbox-series/xbox-series-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "microsoft-store"
    },
    {
        "name": "xbox-one",
        "display_name": "Xbox One",
        "icon_url": "/static/logos/platforms/xbox-one/xbox-one-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "microsoft-store"
    },
    {
        "name": "xbox-360",
        "display_name": "Xbox 360",
        "icon_url": "/static/logos/platforms/xbox-360/xbox-360-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "microsoft-store"
    },
    {
        "name": "nintendo-switch",
        "display_name": "Nintendo Switch",
        "icon_url": "/static/logos/platforms/nintendo-switch/nintendo-switch-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "nintendo-eshop"
    },
    {
        "name": "nintendo-wii",
        "display_name": "Nintendo Wii",
        "icon_url": "/static/logos/platforms/nintendo-wii/nintendo-wii-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "nintendo-eshop"
    },
    {
        "name": "ios",
        "display_name": "iOS",
        "icon_url": "/static/logos/platforms/ios/ios-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "apple-app-store"
    },
    {
        "name": "android",
        "display_name": "Android",
        "icon_url": "/static/logos/platforms/android/android-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "google-play-store"
    },
    {
        "name": "playstation-2",
        "display_name": "PlayStation 2",
        "icon_url": "/static/logos/platforms/playstation-2/playstation-2-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "physical"
    },
    {
        "name": "playstation",
        "display_name": "PlayStation",
        "icon_url": "/static/logos/platforms/playstation/playstation-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "physical"
    },
    {
        "name": "nintendo-wii-u",
        "display_name": "Nintendo Wii U",
        "icon_url": "/static/logos/platforms/nintendo-wii-u/nintendo-wii-u-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "nintendo-eshop"
    },
    {
        "name": "pc-linux",
        "display_name": "PC (Linux)",
        "icon_url": "/static/logos/platforms/pc-linux/pc-linux-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "steam"
    },
    {
        "name": "mac",
        "display_name": "Mac",
        "icon_url": "/static/logos/platforms/mac/mac-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "steam"
    },
    {
        "name": "nintendo-switch-2",
        "display_name": "Nintendo Switch 2",
        "icon_url": "/static/logos/platforms/nintendo-switch-2/nintendo-switch-2-icon-light.svg",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0",
        "default_storefront_name": "nintendo-eshop"
    }
]