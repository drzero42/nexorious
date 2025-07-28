"""
Default platform-storefront mappings for seed data.

Maps platform names to their default storefronts as specified in the PRD.
"""

from typing import List, Dict

DEFAULT_PLATFORM_STOREFRONT_MAPPINGS: List[Dict[str, str]] = [
    {
        "platform_name": "pc-windows",
        "storefront_name": "steam"
    },
    {
        "platform_name": "playstation-5", 
        "storefront_name": "playstation-store"
    },
    {
        "platform_name": "playstation-4",
        "storefront_name": "playstation-store"
    },
    {
        "platform_name": "playstation-3",
        "storefront_name": "playstation-store"
    },
    {
        "platform_name": "xbox-series",
        "storefront_name": "microsoft-store"
    },
    {
        "platform_name": "xbox-one",
        "storefront_name": "microsoft-store"
    },
    {
        "platform_name": "xbox-360",
        "storefront_name": "microsoft-store"
    },
    {
        "platform_name": "nintendo-switch",
        "storefront_name": "nintendo-eshop"
    },
    {
        "platform_name": "nintendo-wii",
        "storefront_name": "nintendo-eshop"
    },
    {
        "platform_name": "ios",
        "storefront_name": "apple-app-store"
    },
    {
        "platform_name": "android",
        "storefront_name": "google-play-store"
    }
]