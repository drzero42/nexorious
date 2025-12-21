"""
Platform-storefront association seed data.

Defines the many-to-many relationships between platforms and storefronts
for realistic associations as specified in the PRD.
"""

from typing import List, Dict

PLATFORM_STOREFRONT_ASSOCIATIONS: List[Dict[str, str]] = [
    # PC (Windows) associations
    {"platform_name": "pc-windows", "storefront_name": "steam"},
    {"platform_name": "pc-windows", "storefront_name": "epic-games-store"},
    {"platform_name": "pc-windows", "storefront_name": "gog"},
    {"platform_name": "pc-windows", "storefront_name": "origin-ea-app"},
    {"platform_name": "pc-windows", "storefront_name": "microsoft-store"},
    {"platform_name": "pc-windows", "storefront_name": "itch-io"},
    {"platform_name": "pc-windows", "storefront_name": "gamersgate"},
    {"platform_name": "pc-windows", "storefront_name": "physical"},
    
    # PlayStation 5 associations
    {"platform_name": "playstation-5", "storefront_name": "playstation-store"},
    {"platform_name": "playstation-5", "storefront_name": "physical"},
    
    # PlayStation 4 associations
    {"platform_name": "playstation-4", "storefront_name": "playstation-store"},
    {"platform_name": "playstation-4", "storefront_name": "physical"},
    
    # PlayStation 3 associations
    {"platform_name": "playstation-3", "storefront_name": "playstation-store"},
    {"platform_name": "playstation-3", "storefront_name": "physical"},

    # PlayStation Vita associations
    {"platform_name": "playstation-vita", "storefront_name": "playstation-store"},
    {"platform_name": "playstation-vita", "storefront_name": "physical"},

    # PlayStation Portable (PSP) associations
    {"platform_name": "playstation-psp", "storefront_name": "playstation-store"},
    {"platform_name": "playstation-psp", "storefront_name": "physical"},

    # Xbox Series X/S associations
    {"platform_name": "xbox-series", "storefront_name": "microsoft-store"},
    {"platform_name": "xbox-series", "storefront_name": "physical"},
    
    # Xbox One associations
    {"platform_name": "xbox-one", "storefront_name": "microsoft-store"},
    {"platform_name": "xbox-one", "storefront_name": "physical"},
    
    # Xbox 360 associations
    {"platform_name": "xbox-360", "storefront_name": "microsoft-store"},
    {"platform_name": "xbox-360", "storefront_name": "physical"},
    
    # Nintendo Switch associations
    {"platform_name": "nintendo-switch", "storefront_name": "nintendo-eshop"},
    {"platform_name": "nintendo-switch", "storefront_name": "physical"},
    
    # Nintendo Wii associations
    {"platform_name": "nintendo-wii", "storefront_name": "nintendo-eshop"},
    {"platform_name": "nintendo-wii", "storefront_name": "physical"},
    
    # iOS associations
    {"platform_name": "ios", "storefront_name": "apple-app-store"},
    {"platform_name": "ios", "storefront_name": "epic-games-store"},
    
    # Android associations
    {"platform_name": "android", "storefront_name": "google-play-store"},
    {"platform_name": "android", "storefront_name": "epic-games-store"},
    
    # PC (Linux) associations
    {"platform_name": "pc-linux", "storefront_name": "steam"},
    {"platform_name": "pc-linux", "storefront_name": "gog"},
    {"platform_name": "pc-linux", "storefront_name": "humble-bundle"},
    
    # PlayStation 2 associations
    {"platform_name": "playstation-2", "storefront_name": "physical"},
    
    # PlayStation associations
    {"platform_name": "playstation", "storefront_name": "physical"},
    
    # Nintendo Wii U associations
    {"platform_name": "nintendo-wii-u", "storefront_name": "nintendo-eshop"},
    {"platform_name": "nintendo-wii-u", "storefront_name": "physical"},
    
    # Nintendo Switch 2 associations
    {"platform_name": "nintendo-switch-2", "storefront_name": "nintendo-eshop"},
    {"platform_name": "nintendo-switch-2", "storefront_name": "physical"},
]