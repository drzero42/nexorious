"""
Official storefront seed data.

Storefronts represent digital marketplaces where games are sold/distributed.
"""

from typing import List, Dict, Any

OFFICIAL_STOREFRONTS: List[Dict[str, Any]] = [
    {
        "name": "steam",
        "display_name": "Steam",
        "icon_url": "/static/logos/storefronts/steam/steam-icon-light.svg",
        "base_url": "https://store.steampowered.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "epic-games-store",
        "display_name": "Epic Games Store",
        "icon_url": "/static/logos/storefronts/epic-games-store/epic-games-store-icon-light.svg",
        "base_url": "https://store.epicgames.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "gog",
        "display_name": "GOG",
        "icon_url": "/static/logos/storefronts/gog/gog-icon-light.svg",
        "base_url": "https://www.gog.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "playstation-store",
        "display_name": "PlayStation Store",
        "icon_url": "/static/logos/storefronts/playstation-store/playstation-store-icon-light.svg",
        "base_url": "https://store.playstation.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "microsoft-store",
        "display_name": "Microsoft Store",
        "icon_url": "/static/logos/storefronts/microsoft-store/microsoft-store-icon-light.svg",
        "base_url": "https://www.microsoft.com/store",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "nintendo-eshop",
        "display_name": "Nintendo eShop",
        "icon_url": "/static/logos/storefronts/nintendo-eshop/nintendo-eshop-icon-light.svg",
        "base_url": "https://www.nintendo.com/us/store",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "itch-io",
        "display_name": "Itch.io",
        "icon_url": "/static/logos/storefronts/itch-io/itch-io-icon-light.svg",
        "base_url": "https://itch.io",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "origin-ea-app",
        "display_name": "Origin/EA App",
        "icon_url": "/static/logos/storefronts/origin-ea-app/origin-ea-app-icon-light.svg",
        "base_url": "https://www.ea.com/ea-app",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "apple-app-store",
        "display_name": "Apple App Store",
        "icon_url": "/static/logos/storefronts/apple-app-store/apple-app-store-icon-light.svg",
        "base_url": "https://apps.apple.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "google-play-store",
        "display_name": "Google Play Store",
        "icon_url": "/static/logos/storefronts/google-play-store/google-play-store-icon-light.svg",
        "base_url": "https://play.google.com/store",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "humble-bundle",
        "display_name": "Humble Bundle",
        "icon_url": "/static/logos/storefronts/humble-bundle/humble-bundle-icon-light.svg",
        "base_url": "https://www.humblebundle.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "physical",
        "display_name": "Physical",
        "icon_url": "/static/logos/storefronts/physical/physical-icon-light.svg",
        "base_url": None,
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "uplay",
        "display_name": "UPlay",
        "icon_url": "/static/logos/storefronts/uplay/uplay-icon-light.svg",
        "base_url": "https://store.ubi.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    },
    {
        "name": "gamersgate",
        "display_name": "GamersGate",
        "icon_url": "/static/logos/storefronts/gamersgate/gamersgate-icon-light.svg",
        "base_url": "https://www.gamersgate.com",
        "is_active": True,
        "source": "official",
        "version_added": "1.0.0"
    }
]