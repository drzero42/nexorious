"""
Seeding functions for platforms and storefronts with conflict resolution.
"""

import uuid
from datetime import datetime, timezone
from typing import List, Dict, Any, Optional
from sqlmodel import Session, select
import logging

from ..models.platform import Platform, Storefront
from .platforms import OFFICIAL_PLATFORMS
from .storefronts import OFFICIAL_STOREFRONTS

logger = logging.getLogger(__name__)


def seed_platforms(session: Session, version: str = "1.0.0") -> int:
    """
    Seed official platforms with conflict resolution.
    
    Args:
        session: Database session
        version: Version string for tracking when platforms were added
        
    Returns:
        Number of platforms seeded (created or updated)
    """
    seeded_count = 0
    
    for platform_data in OFFICIAL_PLATFORMS:
        # Check if platform with same name already exists
        existing_platform = session.exec(
            select(Platform).where(Platform.name == platform_data["name"])
        ).first()
        
        if existing_platform:
            # If it's a custom platform, update it to official
            if existing_platform.source == "custom":
                logger.info(f"Converting custom platform '{platform_data['name']}' to official")
                existing_platform.source = "official"
                existing_platform.version_added = version
                existing_platform.updated_at = datetime.now(timezone.utc)
                
                # Preserve custom display name and icon if they exist
                if not existing_platform.display_name:
                    existing_platform.display_name = platform_data["display_name"]
                if not existing_platform.icon_url and platform_data.get("icon_url"):
                    existing_platform.icon_url = platform_data["icon_url"]
                
                session.add(existing_platform)
                seeded_count += 1
            else:
                logger.debug(f"Official platform '{platform_data['name']}' already exists, skipping")
        else:
            # Create new official platform
            logger.info(f"Creating new official platform '{platform_data['name']}'")
            new_platform = Platform(
                id=str(uuid.uuid4()),
                name=platform_data["name"],
                display_name=platform_data["display_name"],
                icon_url=platform_data.get("icon_url"),
                is_active=platform_data.get("is_active", True),
                source="official",
                version_added=version,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            session.add(new_platform)
            seeded_count += 1
    
    session.commit()
    logger.info(f"Seeded {seeded_count} platforms")
    return seeded_count


def seed_storefronts(session: Session, version: str = "1.0.0") -> int:
    """
    Seed official storefronts with conflict resolution.
    
    Args:
        session: Database session
        version: Version string for tracking when storefronts were added
        
    Returns:
        Number of storefronts seeded (created or updated)
    """
    seeded_count = 0
    
    for storefront_data in OFFICIAL_STOREFRONTS:
        # Check if storefront with same name already exists
        existing_storefront = session.exec(
            select(Storefront).where(Storefront.name == storefront_data["name"])
        ).first()
        
        if existing_storefront:
            # If it's a custom storefront, update it to official
            if existing_storefront.source == "custom":
                logger.info(f"Converting custom storefront '{storefront_data['name']}' to official")
                existing_storefront.source = "official"
                existing_storefront.version_added = version
                existing_storefront.updated_at = datetime.now(timezone.utc)
                
                # Preserve custom display name, icon, and base_url if they exist
                if not existing_storefront.display_name:
                    existing_storefront.display_name = storefront_data["display_name"]
                if not existing_storefront.icon_url and storefront_data.get("icon_url"):
                    existing_storefront.icon_url = storefront_data["icon_url"]
                if not existing_storefront.base_url and storefront_data.get("base_url"):
                    existing_storefront.base_url = storefront_data["base_url"]
                
                session.add(existing_storefront)
                seeded_count += 1
            else:
                logger.debug(f"Official storefront '{storefront_data['name']}' already exists, skipping")
        else:
            # Create new official storefront
            logger.info(f"Creating new official storefront '{storefront_data['name']}'")
            new_storefront = Storefront(
                id=str(uuid.uuid4()),
                name=storefront_data["name"],
                display_name=storefront_data["display_name"],
                icon_url=storefront_data.get("icon_url"),
                base_url=storefront_data.get("base_url"),
                is_active=storefront_data.get("is_active", True),
                source="official",
                version_added=version,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            session.add(new_storefront)
            seeded_count += 1
    
    session.commit()
    logger.info(f"Seeded {seeded_count} storefronts")
    return seeded_count


def seed_all_official_data(session: Session, version: str = "1.0.0") -> Dict[str, int]:
    """
    Seed all official platforms and storefronts.
    
    Args:
        session: Database session
        version: Version string for tracking when data was added
        
    Returns:
        Dictionary with counts of seeded platforms and storefronts
    """
    logger.info(f"Starting seeding of official data for version {version}")
    
    platform_count = seed_platforms(session, version)
    storefront_count = seed_storefronts(session, version)
    
    result = {
        "platforms": platform_count,
        "storefronts": storefront_count,
        "total": platform_count + storefront_count
    }
    
    logger.info(f"Completed seeding: {result}")
    return result


def get_seeding_conflicts(session: Session) -> Dict[str, List[str]]:
    """
    Check for potential conflicts with official data.
    
    Args:
        session: Database session
        
    Returns:
        Dictionary with lists of conflicting platform and storefront names
    """
    conflicts = {"platforms": [], "storefronts": []}
    
    # Check platform conflicts
    for platform_data in OFFICIAL_PLATFORMS:
        existing_platform = session.exec(
            select(Platform).where(
                Platform.name == platform_data["name"],
                Platform.source == "custom"
            )
        ).first()
        
        if existing_platform:
            conflicts["platforms"].append(platform_data["name"])
    
    # Check storefront conflicts
    for storefront_data in OFFICIAL_STOREFRONTS:
        existing_storefront = session.exec(
            select(Storefront).where(
                Storefront.name == storefront_data["name"],
                Storefront.source == "custom"
            )
        ).first()
        
        if existing_storefront:
            conflicts["storefronts"].append(storefront_data["name"])
    
    return conflicts