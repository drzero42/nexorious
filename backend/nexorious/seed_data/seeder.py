"""
Seeding functions for platforms and storefronts with conflict resolution.
"""

import uuid
from datetime import datetime, timezone
from typing import List, Dict, Any, Optional
from sqlmodel import Session, select
import logging

from ..models.platform import Platform, Storefront, PlatformStorefront
from .platforms import OFFICIAL_PLATFORMS
from .storefronts import OFFICIAL_STOREFRONTS
from .default_mappings import DEFAULT_PLATFORM_STOREFRONT_MAPPINGS
from .platform_storefront_associations import PLATFORM_STOREFRONT_ASSOCIATIONS

logger = logging.getLogger(__name__)


def seed_platforms(session: Session, version: str = "1.0.0", set_defaults: bool = True) -> int:
    """
    Seed official platforms with conflict resolution.
    
    Args:
        session: Database session
        version: Version string for tracking when platforms were added
        set_defaults: Whether to set default storefronts during platform creation
        
    Returns:
        Number of platforms seeded (created or updated)
    """
    seeded_count = 0
    
    for platform_data in OFFICIAL_PLATFORMS:
        # Check if platform with same name already exists
        existing_platform = session.exec(
            select(Platform).where(Platform.name == platform_data["name"])
        ).first()
        
        # Look up default storefront ID if specified and defaults should be set
        default_storefront_id = None
        if set_defaults and platform_data.get("default_storefront_name"):
            default_storefront = session.exec(
                select(Storefront).where(Storefront.name == platform_data["default_storefront_name"])
            ).first()
            
            if default_storefront:
                default_storefront_id = default_storefront.id
                logger.debug(f"Found default storefront '{platform_data['default_storefront_name']}' for platform '{platform_data['name']}'")
            else:
                logger.warning(f"Default storefront '{platform_data['default_storefront_name']}' not found for platform '{platform_data['name']}'")
        
        if existing_platform:
            # Only update if it's an official platform; preserve custom platforms
            if existing_platform.source == "official":
                # Check if any values actually need updating (excluding version)
                needs_update = False
                if existing_platform.display_name != platform_data["display_name"]:
                    existing_platform.display_name = platform_data["display_name"]
                    needs_update = True
                if existing_platform.icon_url != platform_data.get("icon_url"):
                    existing_platform.icon_url = platform_data.get("icon_url")
                    needs_update = True
                if default_storefront_id and existing_platform.default_storefront_id != default_storefront_id:
                    existing_platform.default_storefront_id = default_storefront_id
                    needs_update = True
                    logger.info(f"Updated default storefront for official platform '{platform_data['name']}'")
                
                if needs_update:
                    logger.info(f"Updating official platform '{platform_data['name']}' with seed data")
                    existing_platform.version_added = version  # Update version only when other changes are made
                    existing_platform.updated_at = datetime.now(timezone.utc)
                    session.add(existing_platform)
                    seeded_count += 1
                else:
                    logger.debug(f"Official platform '{platform_data['name']}' already up to date")
            else:
                logger.debug(f"Custom platform '{platform_data['name']}' exists, preserving it and skipping seed data")
        else:
            # Create new official platform
            logger.info(f"Creating new official platform '{platform_data['name']}'")
            new_platform = Platform(
                id=str(uuid.uuid4()),
                name=platform_data["name"],
                display_name=platform_data["display_name"],
                icon_url=platform_data.get("icon_url"),
                default_storefront_id=default_storefront_id,
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
            # Only update if it's an official storefront; preserve custom storefronts
            if existing_storefront.source == "official":
                # Check if any values actually need updating (excluding version)
                needs_update = False
                if existing_storefront.display_name != storefront_data["display_name"]:
                    existing_storefront.display_name = storefront_data["display_name"]
                    needs_update = True
                if existing_storefront.icon_url != storefront_data.get("icon_url"):
                    existing_storefront.icon_url = storefront_data.get("icon_url")
                    needs_update = True
                if existing_storefront.base_url != storefront_data.get("base_url"):
                    existing_storefront.base_url = storefront_data.get("base_url")
                    needs_update = True
                
                if needs_update:
                    logger.info(f"Updating official storefront '{storefront_data['name']}' with seed data")
                    existing_storefront.version_added = version  # Update version only when other changes are made
                    existing_storefront.updated_at = datetime.now(timezone.utc)
                    session.add(existing_storefront)
                    seeded_count += 1
                else:
                    logger.debug(f"Official storefront '{storefront_data['name']}' already up to date")
            else:
                logger.debug(f"Custom storefront '{storefront_data['name']}' exists, preserving it and skipping seed data")
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


def seed_default_platform_storefront_mappings(session: Session) -> int:
    """
    Set default storefront relationships for platforms.
    
    Args:
        session: Database session
        
    Returns:
        Number of platform-storefront mappings created
    """
    mapped_count = 0
    
    for mapping in DEFAULT_PLATFORM_STOREFRONT_MAPPINGS:
        platform_name = mapping["platform_name"]
        storefront_name = mapping["storefront_name"]
        
        # Find the platform
        platform = session.exec(
            select(Platform).where(Platform.name == platform_name)
        ).first()
        
        if not platform:
            logger.warning(f"Platform '{platform_name}' not found for default mapping")
            continue
            
        # Find the storefront
        storefront = session.exec(
            select(Storefront).where(Storefront.name == storefront_name)
        ).first()
        
        if not storefront:
            logger.warning(f"Storefront '{storefront_name}' not found for default mapping")
            continue
            
        # Set the default storefront if not already set
        if platform.default_storefront_id is None:
            logger.info(f"Setting default storefront for '{platform_name}' → '{storefront_name}'")
            platform.default_storefront_id = storefront.id
            platform.updated_at = datetime.now(timezone.utc)
            session.add(platform)
            mapped_count += 1
        else:
            logger.debug(f"Platform '{platform_name}' already has a default storefront, skipping")
    
    session.commit()
    logger.info(f"Created {mapped_count} default platform-storefront mappings")
    return mapped_count


def seed_platform_storefront_associations(session: Session) -> int:
    """
    Seed many-to-many platform-storefront associations.
    
    Args:
        session: Database session
        
    Returns:
        Number of platform-storefront associations created
    """
    association_count = 0
    
    for association in PLATFORM_STOREFRONT_ASSOCIATIONS:
        platform_name = association["platform_name"]
        storefront_name = association["storefront_name"]
        
        # Find the platform
        platform = session.exec(
            select(Platform).where(Platform.name == platform_name)
        ).first()
        
        if not platform:
            logger.warning(f"Platform '{platform_name}' not found for association")
            continue
            
        # Find the storefront
        storefront = session.exec(
            select(Storefront).where(Storefront.name == storefront_name)
        ).first()
        
        if not storefront:
            logger.warning(f"Storefront '{storefront_name}' not found for association")
            continue
            
        # Check if association already exists
        existing_association = session.exec(
            select(PlatformStorefront).where(
                PlatformStorefront.platform_id == platform.id,
                PlatformStorefront.storefront_id == storefront.id
            )
        ).first()
        
        if existing_association:
            logger.debug(f"Association '{platform_name}' → '{storefront_name}' already exists, skipping")
            continue
            
        # Create new association
        logger.info(f"Creating platform-storefront association '{platform_name}' → '{storefront_name}'")
        new_association = PlatformStorefront(
            platform_id=platform.id,
            storefront_id=storefront.id,
            created_at=datetime.now(timezone.utc)
        )
        session.add(new_association)
        association_count += 1
    
    session.commit()
    logger.info(f"Created {association_count} platform-storefront associations")
    return association_count


def seed_all_official_data(session: Session, version: str = "1.0.0") -> Dict[str, int]:
    """
    Seed all official platforms, storefronts, their default mappings, and many-to-many associations.
    
    Args:
        session: Database session
        version: Version string for tracking when data was added
        
    Returns:
        Dictionary with counts of seeded platforms, storefronts, mappings, and associations
    """
    logger.info(f"Starting seeding of official data for version {version}")
    
    # Seed storefronts first since platforms may reference them for default storefronts
    storefront_count = seed_storefronts(session, version)
    # Create platforms without defaults so mapping function can set them
    platform_count = seed_platforms(session, version, set_defaults=False)
    mapping_count = seed_default_platform_storefront_mappings(session)
    # Seed many-to-many platform-storefront associations
    association_count = seed_platform_storefront_associations(session)
    
    result = {
        "platforms": platform_count,
        "storefronts": storefront_count,
        "mappings": mapping_count,
        "associations": association_count,
        "total": platform_count + storefront_count + mapping_count + association_count
    }
    
    logger.info(f"Completed seeding: {result}")
    return result


def get_seeding_conflicts(session: Session) -> Dict[str, List[str]]:
    """
    Check for custom platforms/storefronts that will be preserved during seeding.
    
    Args:
        session: Database session
        
    Returns:
        Dictionary with lists of custom platform and storefront names that match official names
    """
    conflicts = {"platforms": [], "storefronts": []}
    
    # Check for custom platforms with official names (these will be preserved)
    for platform_data in OFFICIAL_PLATFORMS:
        existing_platform = session.exec(
            select(Platform).where(
                Platform.name == platform_data["name"],
                Platform.source == "custom"
            )
        ).first()
        
        if existing_platform:
            conflicts["platforms"].append(platform_data["name"])
    
    # Check for custom storefronts with official names (these will be preserved)
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