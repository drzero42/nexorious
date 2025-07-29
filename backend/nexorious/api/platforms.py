"""
Platform and storefront management endpoints (admin-only).
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, func
from datetime import datetime, timezone
from typing import Annotated, Optional

from ..core.database import get_session
from ..core.security import get_current_admin_user
from ..models.user import User
from ..models.platform import Platform, Storefront
from ..api.schemas.platform import (
    PlatformCreateRequest,
    PlatformUpdateRequest,
    PlatformResponse,
    StorefrontCreateRequest,
    StorefrontUpdateRequest,
    StorefrontResponse,
    PlatformListResponse,
    StorefrontListResponse,
    PlatformStatsResponse,
    StorefrontStatsResponse,
    PlatformUsageStats,
    StorefrontUsageStats,
    SeedDataResponse,
    PlatformDefaultMapping,
    UpdatePlatformDefaultRequest
)
from ..api.schemas.common import SuccessResponse

router = APIRouter(prefix="/platforms", tags=["Platforms & Storefronts"])


# Platform endpoints
@router.get("/", response_model=PlatformListResponse)
async def list_platforms(
    session: Annotated[Session, Depends(get_session)],
    active_only: bool = Query(default=True, description="Show only active platforms"),
    source: Optional[str] = Query(default=None, description="Filter by source: 'official' or 'custom'"),
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page")
):
    """List all platforms."""
    
    query = select(Platform)
    if active_only:
        query = query.where(Platform.is_active)
    if source:
        query = query.where(Platform.source == source)
    
    # Order by source (official first) then by display name
    query = query.order_by(Platform.source.desc(), Platform.display_name)
    
    # Get total count
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()
    
    # Apply pagination
    offset = (page - 1) * per_page
    platforms = session.exec(query.offset(offset).limit(per_page)).all()
    
    # Calculate pages
    pages = (total + per_page - 1) // per_page
    
    return PlatformListResponse(
        platforms=platforms,
        total=total,
        page=page,
        per_page=per_page,
        pages=pages
    )


@router.get("/{platform_id}", response_model=PlatformResponse)
async def get_platform(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)]
):
    """Get a specific platform by ID."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    return platform


@router.post("/", response_model=PlatformResponse, status_code=status.HTTP_201_CREATED)
async def create_platform(
    platform_data: PlatformCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Create a new platform (admin only)."""
    
    # Check if platform name already exists
    existing_platform = session.exec(
        select(Platform).where(Platform.name == platform_data.name)
    ).first()
    
    if existing_platform:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Platform name already exists"
        )
    
    # Validate default_storefront_id if provided
    if platform_data.default_storefront_id:
        storefront = session.get(Storefront, platform_data.default_storefront_id)
        if not storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Default storefront not found"
            )
        if not storefront.is_active:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Cannot set inactive storefront as default"
            )
    
    new_platform = Platform(
        name=platform_data.name,
        display_name=platform_data.display_name,
        icon_url=str(platform_data.icon_url) if platform_data.icon_url else None,
        is_active=platform_data.is_active if platform_data.is_active is not None else True,
        default_storefront_id=platform_data.default_storefront_id
    )
    
    session.add(new_platform)
    session.commit()
    session.refresh(new_platform)
    
    return new_platform


@router.put("/{platform_id}", response_model=PlatformResponse)
async def update_platform(
    platform_id: str,
    platform_data: PlatformUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Update an existing platform (admin only)."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Update fields
    update_data = platform_data.model_dump(exclude_unset=True)
    
    # Validate default_storefront_id if provided
    if "default_storefront_id" in update_data and update_data["default_storefront_id"] is not None:
        storefront_id = update_data["default_storefront_id"]
        storefront = session.get(Storefront, storefront_id)
        if not storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Default storefront not found"
            )
        if not storefront.is_active:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Cannot set inactive storefront as default"
            )
    
    for field, value in update_data.items():
        if field == "icon_url" and value:
            setattr(platform, field, str(value))
        else:
            setattr(platform, field, value)
    
    platform.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(platform)
    
    return platform


@router.delete("/{platform_id}", response_model=SuccessResponse)
async def delete_platform(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Delete a platform (admin only)."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Check if platform is in use
    from ..models.user_game import UserGamePlatform
    
    usage_count = session.exec(
        select(func.count()).where(UserGamePlatform.platform_id == platform_id)
    ).one()
    
    if usage_count > 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot delete platform. It is referenced by {usage_count} user game entries."
        )
    
    session.delete(platform)
    session.commit()
    
    return SuccessResponse(message="Platform deleted successfully")


@router.get("/{platform_id}/default-storefront", response_model=PlatformDefaultMapping)
async def get_platform_default_storefront(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)]
):
    """Get the default storefront for a specific platform."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Get the default storefront if it exists
    default_storefront = None
    if platform.default_storefront_id:
        default_storefront = session.get(Storefront, platform.default_storefront_id)
    
    return PlatformDefaultMapping(
        platform_id=platform.id,
        platform_name=platform.name,
        platform_display_name=platform.display_name,
        default_storefront=default_storefront
    )


@router.put("/{platform_id}/default-storefront", response_model=PlatformDefaultMapping)
async def update_platform_default_storefront(
    platform_id: str,
    update_data: UpdatePlatformDefaultRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Update the default storefront for a specific platform (admin only)."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # If storefront_id is provided, validate that it exists
    if update_data.storefront_id is not None:
        storefront = session.get(Storefront, update_data.storefront_id)
        if not storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Storefront not found"
            )
        
        # Ensure storefront is active
        if not storefront.is_active:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Cannot set inactive storefront as default"
            )
    
    # Update the platform's default storefront
    platform.default_storefront_id = update_data.storefront_id
    platform.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(platform)
    
    # Get the updated default storefront for response
    default_storefront = None
    if platform.default_storefront_id:
        default_storefront = session.get(Storefront, platform.default_storefront_id) 
    
    return PlatformDefaultMapping(
        platform_id=platform.id,
        platform_name=platform.name,
        platform_display_name=platform.display_name,
        default_storefront=default_storefront
    )


# Storefront endpoints
@router.get("/storefronts/", response_model=StorefrontListResponse)
async def list_storefronts(
    session: Annotated[Session, Depends(get_session)],
    active_only: bool = Query(default=True, description="Show only active storefronts"),
    source: Optional[str] = Query(default=None, description="Filter by source: 'official' or 'custom'"),
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page")
):
    """List all storefronts."""
    
    query = select(Storefront)
    if active_only:
        query = query.where(Storefront.is_active)
    if source:
        query = query.where(Storefront.source == source)
    
    # Order by source (official first) then by display name
    query = query.order_by(Storefront.source.desc(), Storefront.display_name)
    
    # Get total count
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()
    
    # Apply pagination
    offset = (page - 1) * per_page
    storefronts = session.exec(query.offset(offset).limit(per_page)).all()
    
    # Calculate pages
    pages = (total + per_page - 1) // per_page
    
    return StorefrontListResponse(
        storefronts=storefronts,
        total=total,
        page=page,
        per_page=per_page,
        pages=pages
    )


@router.get("/storefronts/{storefront_id}", response_model=StorefrontResponse)
async def get_storefront(
    storefront_id: str,
    session: Annotated[Session, Depends(get_session)]
):
    """Get a specific storefront by ID."""
    
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    
    return storefront


@router.post("/storefronts/", response_model=StorefrontResponse, status_code=status.HTTP_201_CREATED)
async def create_storefront(
    storefront_data: StorefrontCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Create a new storefront (admin only)."""
    
    # Check if storefront name already exists
    existing_storefront = session.exec(
        select(Storefront).where(Storefront.name == storefront_data.name)
    ).first()
    
    if existing_storefront:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Storefront name already exists"
        )
    
    new_storefront = Storefront(
        name=storefront_data.name,
        display_name=storefront_data.display_name,
        icon_url=str(storefront_data.icon_url) if storefront_data.icon_url else None,
        base_url=str(storefront_data.base_url) if storefront_data.base_url else None,
        is_active=storefront_data.is_active if storefront_data.is_active is not None else True
    )
    
    session.add(new_storefront)
    session.commit()
    session.refresh(new_storefront)
    
    return new_storefront


@router.put("/storefronts/{storefront_id}", response_model=StorefrontResponse)
async def update_storefront(
    storefront_id: str,
    storefront_data: StorefrontUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Update an existing storefront (admin only)."""
    
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    
    # Update fields
    update_data = storefront_data.model_dump(exclude_unset=True)
    
    for field, value in update_data.items():
        if field in ["icon_url", "base_url"] and value:
            setattr(storefront, field, str(value))
        else:
            setattr(storefront, field, value)
    
    storefront.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(storefront)
    
    return storefront


@router.delete("/storefronts/{storefront_id}", response_model=SuccessResponse)
async def delete_storefront(
    storefront_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Delete a storefront (admin only)."""
    
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    
    # Check if storefront is in use
    from ..models.user_game import UserGamePlatform
    
    usage_count = session.exec(
        select(func.count()).where(UserGamePlatform.storefront_id == storefront_id)
    ).one()
    
    if usage_count > 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot delete storefront. It is referenced by {usage_count} user game entries."
        )
    
    session.delete(storefront)
    session.commit()
    
    return SuccessResponse(message="Storefront deleted successfully")


# Statistics endpoints
@router.get("/stats", response_model=PlatformStatsResponse)
async def get_platform_usage_stats(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Get platform usage statistics (admin only)."""
    
    from ..models.user_game import UserGamePlatform
    
    # Query platform usage statistics
    query = (
        select(
            Platform.id.label("platform_id"),
            Platform.name.label("platform_name"),
            Platform.display_name.label("platform_display_name"),
            func.count(UserGamePlatform.id).label("usage_count")
        )
        .select_from(Platform)
        .outerjoin(UserGamePlatform, Platform.id == UserGamePlatform.platform_id)
        .group_by(Platform.id, Platform.name, Platform.display_name)
        .order_by(func.count(UserGamePlatform.id).desc(), Platform.display_name)
    )
    
    results = session.exec(query).all()
    
    platform_stats = [
        PlatformUsageStats(
            platform_id=row.platform_id,
            platform_name=row.platform_name,
            platform_display_name=row.platform_display_name,
            usage_count=row.usage_count
        )
        for row in results
    ]
    
    total_platforms = len(platform_stats)
    total_usage = sum(stat.usage_count for stat in platform_stats)
    
    return PlatformStatsResponse(
        platforms=platform_stats,
        total_platforms=total_platforms,
        total_usage=total_usage
    )


@router.get("/storefronts/stats", response_model=StorefrontStatsResponse)
async def get_storefront_usage_stats(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Get storefront usage statistics (admin only)."""
    
    from ..models.user_game import UserGamePlatform
    
    # Query storefront usage statistics
    query = (
        select(
            Storefront.id.label("storefront_id"),
            Storefront.name.label("storefront_name"),
            Storefront.display_name.label("storefront_display_name"),
            func.count(UserGamePlatform.id).label("usage_count")
        )
        .select_from(Storefront)
        .outerjoin(UserGamePlatform, Storefront.id == UserGamePlatform.storefront_id)
        .group_by(Storefront.id, Storefront.name, Storefront.display_name)
        .order_by(func.count(UserGamePlatform.id).desc(), Storefront.display_name)
    )
    
    results = session.exec(query).all()
    
    storefront_stats = [
        StorefrontUsageStats(
            storefront_id=row.storefront_id,
            storefront_name=row.storefront_name,
            storefront_display_name=row.storefront_display_name,
            usage_count=row.usage_count
        )
        for row in results
    ]
    
    total_storefronts = len(storefront_stats)
    total_usage = sum(stat.usage_count for stat in storefront_stats)
    
    return StorefrontStatsResponse(
        storefronts=storefront_stats,
        total_storefronts=total_storefronts,
        total_usage=total_usage
    )


# Seed data endpoint
@router.post("/seed", response_model=SeedDataResponse)
async def seed_platforms_and_storefronts(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    version: Optional[str] = Query(default="1.0.0", description="Version string for tracking when data was added")
):
    """
    Load official platforms, storefronts, and their default mappings into the database.
    
    This operation is idempotent - it can be safely run multiple times.
    Only official data will be added, and existing custom data will be preserved.
    
    Admin access required.
    """
    from ..seed_data.seeder import seed_all_official_data
    
    # Load seed data
    result = seed_all_official_data(session, version)
    
    # Create response message
    if result["total"] == 0:
        message = "No changes made. All official platforms, storefronts, and mappings are already up to date."
    else:
        message = f"Successfully loaded seed data: {result['platforms']} platforms, {result['storefronts']} storefronts, {result['mappings']} default mappings."
    
    return SeedDataResponse(
        platforms_added=result["platforms"],
        storefronts_added=result["storefronts"],
        mappings_created=result["mappings"],
        total_changes=result["total"],
        message=message
    )