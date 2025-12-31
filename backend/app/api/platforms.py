"""
Platform and storefront management endpoints (admin-only).
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query, Response, UploadFile, File, Request
from sqlmodel import Session, select, func, or_, desc, col, asc
from datetime import datetime, timezone
from typing import Annotated, Optional, List
import logging

from ..core.database import get_session
from ..core.security import get_current_admin_user, get_current_user
from ..models.user import User
from ..models.platform import Platform, Storefront, PlatformStorefront
from ..utils.sqlalchemy_typed import is_
from ..schemas.platform import (
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
    UpdatePlatformDefaultRequest,
    PlatformStorefrontsResponse,
    PlatformStorefrontAssociationResponse,
)
from ..schemas.common import SuccessResponse
from ..services.logo_service import LogoService, logo_service
from ..core.audit_logging import get_client_ip, audit

router = APIRouter(prefix="/platforms", tags=["Platforms & Storefronts"])
logger = logging.getLogger("uvicorn.error")


def get_logo_service() -> LogoService:
    """Dependency to get the logo service instance."""
    return logo_service


# Simple platform/storefront list endpoints for dropdowns
@router.get("/simple-list", response_model=List[str])
async def list_platform_names(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    active_only: bool = Query(default=True, description="Show only active platforms")
) -> List[str]:
    """Get simple list of platform names for dropdowns."""
    query = select(Platform.name)
    if active_only:
        query = query.where(Platform.is_active)
    
    # Order by display name for user-friendly sorting
    query = query.order_by(Platform.display_name)
    
    platform_names = session.exec(query).all()
    return list(platform_names)


@router.get("/storefronts/simple-list", response_model=List[str])
async def list_storefront_names(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    active_only: bool = Query(default=True, description="Show only active storefronts")
) -> List[str]:
    """Get simple list of storefront names for dropdowns."""
    query = select(Storefront.name)
    if active_only:
        query = query.where(Storefront.is_active)
    
    # Order by display name for user-friendly sorting
    query = query.order_by(Storefront.display_name)
    
    storefront_names = session.exec(query).all()
    return list(storefront_names)


# Platform endpoints
@router.get("/", response_model=PlatformListResponse)
async def list_platforms(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
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
    query = query.order_by(desc(Platform.source), Platform.display_name)
    
    # Get total count
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()
    
    # Apply pagination
    offset = (page - 1) * per_page
    platforms = session.exec(query.offset(offset).limit(per_page)).all()
    
    # Get associated storefronts for each platform
    platform_responses = []
    for platform in platforms:
        # Query associated storefronts for this platform
        storefront_query = (
            select(Storefront)
            .join(PlatformStorefront)
            .where(PlatformStorefront.platform == platform.name)
            .where(Storefront.is_active)
            .order_by(Storefront.display_name)
        )
        
        associated_storefronts = session.exec(storefront_query).all()
        
        # Create platform response with storefronts
        platform_dict = platform.model_dump()
        platform_dict['storefronts'] = associated_storefronts
        platform_responses.append(platform_dict)
    
    # Calculate pages
    pages = (total + per_page - 1) // per_page
    
    return PlatformListResponse(
        platforms=platform_responses,
        total=total,
        page=page,
        per_page=per_page,
        pages=pages
    )


@router.get("/{platform}", response_model=PlatformResponse)
async def get_platform(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get a specific platform by slug."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    return platform_obj


@router.get("/{platform}/storefronts", response_model=PlatformStorefrontsResponse)
async def get_platform_storefronts(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    active_only: bool = Query(default=True, description="Show only active storefronts")
):
    """Get all storefronts associated with a specific platform."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    query = (
        select(Storefront)
        .join(PlatformStorefront)
        .where(PlatformStorefront.platform == platform)
    )

    if active_only:
        query = query.where(Storefront.is_active)

    query = query.order_by(Storefront.display_name)

    storefronts = session.exec(query).all()

    return PlatformStorefrontsResponse(
        platform=platform_obj.name,
        platform_display_name=platform_obj.display_name,
        storefronts=[StorefrontResponse.model_validate(sf) for sf in storefronts],
        total_storefronts=len(storefronts)
    )


@router.post("/{platform}/storefronts/{storefront}", response_model=PlatformStorefrontAssociationResponse)
async def create_platform_storefront_association(
    platform: str,
    storefront: str,
    response: Response,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Create a platform-storefront association (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    if not platform_obj.is_active:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot create association with inactive platform"
        )

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    if not storefront_obj.is_active:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot create association with inactive storefront"
        )

    existing_association = session.exec(
        select(PlatformStorefront).where(
            PlatformStorefront.platform == platform,
            PlatformStorefront.storefront == storefront
        )
    ).first()

    if existing_association:
        response.status_code = status.HTTP_200_OK
        return PlatformStorefrontAssociationResponse(
            platform=platform_obj.name,
            platform_display_name=platform_obj.display_name,
            storefront=storefront_obj.name,
            storefront_display_name=storefront_obj.display_name,
            message="Association already exists"
        )

    new_association = PlatformStorefront(
        platform=platform,
        storefront=storefront
    )

    session.add(new_association)
    session.commit()

    response.status_code = status.HTTP_201_CREATED
    return PlatformStorefrontAssociationResponse(
        platform=platform_obj.name,
        platform_display_name=platform_obj.display_name,
        storefront=storefront_obj.name,
        storefront_display_name=storefront_obj.display_name,
        message="Association created successfully"
    )


@router.delete("/{platform}/storefronts/{storefront}", response_model=PlatformStorefrontAssociationResponse)
async def delete_platform_storefront_association(
    platform: str,
    storefront: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Remove a platform-storefront association (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )

    association = session.exec(
        select(PlatformStorefront).where(
            PlatformStorefront.platform == platform,
            PlatformStorefront.storefront == storefront
        )
    ).first()

    message = "Association removed successfully"
    if association:
        session.delete(association)
        session.commit()
    else:
        message = "Association does not exist"

    return PlatformStorefrontAssociationResponse(
        platform=platform_obj.name,
        platform_display_name=platform_obj.display_name,
        storefront=storefront_obj.name,
        storefront_display_name=storefront_obj.display_name,
        message=message
    )


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
    
    if platform_data.default_storefront:
        storefront = session.get(Storefront, platform_data.default_storefront)
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
        icon_url=platform_data.icon_url,
        is_active=platform_data.is_active if platform_data.is_active is not None else True,
        default_storefront=platform_data.default_storefront
    )
    
    session.add(new_platform)
    session.commit()
    session.refresh(new_platform)
    
    return new_platform


@router.put("/{platform}", response_model=PlatformResponse)
async def update_platform(
    platform: str,
    platform_data: PlatformUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Update an existing platform (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    update_data = platform_data.model_dump(exclude_unset=True)

    if "default_storefront" in update_data and update_data["default_storefront"] is not None:
        storefront_slug = update_data["default_storefront"]
        storefront = session.get(Storefront, storefront_slug)
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

    if platform_obj.source == "official":
        platform_obj.source = "custom"

    for field, value in update_data.items():
        setattr(platform_obj, field, value)

    platform_obj.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(platform_obj)

    return platform_obj


@router.delete("/{platform}", response_model=SuccessResponse)
async def delete_platform(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Delete a platform (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    from ..models.user_game import UserGamePlatform

    usage_count = session.exec(
        select(func.count()).where(UserGamePlatform.platform == platform)
    ).one()

    if usage_count > 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot delete platform. It is referenced by {usage_count} user game entries."
        )

    session.delete(platform_obj)
    session.commit()

    return SuccessResponse(message="Platform deleted successfully")


@router.post("/{platform}/logo")
async def upload_platform_logo(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: str = Query(..., description="Logo theme: light or dark"),
    file: UploadFile = File(...)
):
    """Upload a logo for a platform (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    icon_url = await logo_service.upload_logo("platforms", platform_obj.name, theme, file)

    if theme == "light":
        platform_obj.icon_url = icon_url
        platform_obj.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(platform_obj)

    return {
        "message": f"Logo uploaded successfully for {platform_obj.display_name}",
        "theme": theme,
        "icon_url": icon_url,
        "platform": platform_obj
    }


@router.delete("/{platform}/logo")
async def delete_platform_logo(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: Optional[str] = Query(default=None, description="Logo theme to delete (light/dark), or all if not specified")
):
    """Delete logo(s) for a platform (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    deleted_files = logo_service.delete_logo("platforms", platform_obj.name, theme)

    if theme == "light" or theme is None:
        platform_obj.icon_url = None
        platform_obj.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(platform_obj)

    return {
        "message": f"Logo(s) deleted successfully for {platform_obj.display_name}",
        "deleted_files": len(deleted_files),
        "platform": platform_obj
    }


@router.get("/{platform}/logos")
async def list_platform_logos(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)]
):
    """List available logos for a platform (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    logos = logo_service.list_logos("platforms", platform_obj.name)

    return {
        "platform": platform_obj,
        "logos": logos
    }


@router.get("/{platform}/default-storefront", response_model=PlatformDefaultMapping)
async def get_platform_default_storefront(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get the default storefront for a specific platform."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    default_storefront = None
    if platform_obj.default_storefront:
        default_storefront = session.get(Storefront, platform_obj.default_storefront)

    return PlatformDefaultMapping(
        platform=platform_obj.name,
        platform_display_name=platform_obj.display_name,
        default_storefront=StorefrontResponse.model_validate(default_storefront) if default_storefront is not None else None
    )


@router.put("/{platform}/default-storefront", response_model=PlatformDefaultMapping)
async def update_platform_default_storefront(
    platform: str,
    update_data: UpdatePlatformDefaultRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Update the default storefront for a specific platform (admin only)."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    if update_data.storefront is not None:
        storefront = session.get(Storefront, update_data.storefront)
        if not storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Storefront not found"
            )

        if not storefront.is_active:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Cannot set inactive storefront as default"
            )

    platform_obj.default_storefront = update_data.storefront
    platform_obj.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(platform_obj)

    default_storefront = None
    if platform_obj.default_storefront:
        default_storefront = session.get(Storefront, platform_obj.default_storefront)

    return PlatformDefaultMapping(
        platform=platform_obj.name,
        platform_display_name=platform_obj.display_name,
        default_storefront=StorefrontResponse.model_validate(default_storefront) if default_storefront is not None else None
    )


# Storefront endpoints
@router.get("/storefronts/", response_model=StorefrontListResponse)
async def list_storefronts(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
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
    query = query.order_by(desc(Storefront.source), Storefront.display_name)
    
    # Get total count
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()
    
    # Apply pagination
    offset = (page - 1) * per_page
    storefronts = session.exec(query.offset(offset).limit(per_page)).all()
    
    # Calculate pages
    pages = (total + per_page - 1) // per_page
    
    return StorefrontListResponse(
        storefronts=[StorefrontResponse.model_validate(sf) for sf in storefronts],
        total=total,
        page=page,
        per_page=per_page,
        pages=pages
    )


@router.get("/storefronts/{storefront}", response_model=StorefrontResponse)
async def get_storefront(
    storefront: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get a specific storefront by slug."""

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )

    return storefront_obj


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
        icon_url=storefront_data.icon_url,
        base_url=str(storefront_data.base_url) if storefront_data.base_url else None,
        is_active=storefront_data.is_active if storefront_data.is_active is not None else True
    )
    
    session.add(new_storefront)
    session.commit()
    session.refresh(new_storefront)
    
    return new_storefront


@router.put("/storefronts/{storefront}", response_model=StorefrontResponse)
async def update_storefront(
    storefront: str,
    storefront_data: StorefrontUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Update an existing storefront (admin only)."""

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )

    update_data = storefront_data.model_dump(exclude_unset=True)

    if storefront_obj.source == "official":
        storefront_obj.source = "custom"

    for field, value in update_data.items():
        if field == "base_url" and value:
            setattr(storefront_obj, field, str(value))
        else:
            setattr(storefront_obj, field, value)

    storefront_obj.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(storefront_obj)

    return storefront_obj


@router.delete("/storefronts/{storefront}", response_model=SuccessResponse)
async def delete_storefront(
    storefront: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Delete a storefront (admin only)."""

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )

    from ..models.user_game import UserGamePlatform

    usage_count = session.exec(
        select(func.count()).where(UserGamePlatform.storefront == storefront)
    ).one()

    if usage_count > 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot delete storefront. It is referenced by {usage_count} user game entries."
        )

    session.delete(storefront_obj)
    session.commit()

    return SuccessResponse(message="Storefront deleted successfully")


@router.post("/storefronts/{storefront}/logo")
async def upload_storefront_logo(
    storefront: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: str = Query(..., description="Logo theme: light or dark"),
    file: UploadFile = File(...)
):
    """Upload a logo for a storefront (admin only)."""

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )

    icon_url = await logo_service.upload_logo("storefronts", storefront_obj.name, theme, file)

    if theme == "light":
        storefront_obj.icon_url = icon_url
        storefront_obj.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(storefront_obj)

    return {
        "message": f"Logo uploaded successfully for {storefront_obj.display_name}",
        "theme": theme,
        "icon_url": icon_url,
        "storefront": storefront_obj
    }


@router.delete("/storefronts/{storefront}/logo")
async def delete_storefront_logo(
    storefront: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: Optional[str] = Query(default=None, description="Logo theme to delete (light/dark), or all if not specified")
):
    """Delete logo(s) for a storefront (admin only)."""

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )

    deleted_files = logo_service.delete_logo("storefronts", storefront_obj.name, theme)

    if theme == "light" or theme is None:
        storefront_obj.icon_url = None
        storefront_obj.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(storefront_obj)

    return {
        "message": f"Logo(s) deleted successfully for {storefront_obj.display_name}",
        "deleted_files": len(deleted_files),
        "storefront": storefront_obj
    }


@router.get("/storefronts/{storefront}/logos")
async def list_storefront_logos(
    storefront: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)]
):
    """List available logos for a storefront (admin only)."""

    storefront_obj = session.get(Storefront, storefront)
    if not storefront_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )

    logos = logo_service.list_logos("storefronts", storefront_obj.name)

    return {
        "storefront": storefront_obj,
        "logos": logos
    }


# Statistics endpoints
@router.get("/stats", response_model=PlatformStatsResponse)
async def get_platform_usage_stats(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Get platform usage statistics (admin only)."""

    from ..models.user_game import UserGamePlatform

    query = (
        select(
            col(Platform.name).label("platform"),
            col(Platform.display_name).label("platform_display_name"),
            func.count(col(UserGamePlatform.id)).label("usage_count")
        )
        .select_from(Platform)
        .outerjoin(UserGamePlatform)
        .group_by(Platform.name, Platform.display_name)
        .order_by(func.count(col(UserGamePlatform.id)).desc(), Platform.display_name)
    )

    results = session.execute(query).mappings().all()

    platform_stats = [
        PlatformUsageStats(
            platform=row["platform"],
            platform_display_name=row["platform_display_name"],
            usage_count=row["usage_count"]
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

    query = (
        select(
            col(Storefront.name).label("storefront"),
            col(Storefront.display_name).label("storefront_display_name"),
            func.count(col(UserGamePlatform.id)).label("usage_count")
        )
        .select_from(Storefront)
        .outerjoin(UserGamePlatform)
        .group_by(Storefront.name, Storefront.display_name)
        .order_by(func.count(col(UserGamePlatform.id)).desc(), Storefront.display_name)
    )

    results = session.execute(query).mappings().all()

    storefront_stats = [
        StorefrontUsageStats(
            storefront=row["storefront"],
            storefront_display_name=row["storefront_display_name"],
            usage_count=row["usage_count"]
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
    version: Annotated[str, Query(description="Version string for tracking when data was added")] = "1.0.0"
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
        message = "No changes made. All official platforms, storefronts, and associations are already up to date."
    else:
        message = f"Successfully loaded seed data: {result['platforms']} platforms, {result['storefronts']} storefronts, {result['associations']} associations."
    
    return SeedDataResponse(
        platforms_added=result["platforms"],
        storefronts_added=result["storefronts"],
        mappings_created=0,  # No longer creating separate mappings
        total_changes=result["total"],
        message=message
    )


def get_default_platform_for_storefront(session: Session, storefront_name: str) -> str:
    """
    Get the default platform for a given storefront name.
    Returns the first associated platform or a sensible default.
    """
    storefront = session.exec(
        select(Storefront).where(
            or_(
                func.lower(Storefront.name) == storefront_name.lower(),
                func.lower(Storefront.display_name) == storefront_name.lower()
            )
        )
    ).first()

    if storefront:
        platform_storefronts = session.exec(
            select(PlatformStorefront)
            .where(PlatformStorefront.storefront == storefront.name)
            .order_by(asc(PlatformStorefront.created_at))
        ).all()

        if platform_storefronts:
            platform = session.get(Platform, platform_storefronts[0].platform)
            if platform:
                logger.debug(f"Found platform '{platform.name}' for storefront '{storefront_name}'")
                return platform.name
    
    # Fallback: Check for common patterns in storefront name
    storefront_lower = storefront_name.lower()
    
    # PC storefronts
    pc_keywords = ['steam', 'epic', 'gog', 'origin', 'uplay', 'ubisoft', 
                   'humble', 'itch', 'gamersgate', 'battle.net']
    if any(keyword in storefront_lower for keyword in pc_keywords):
        # Try to find PC (Windows) platform
        pc_platform = session.exec(
            select(Platform).where(
                or_(
                    func.lower(Platform.name) == 'pc (windows)',
                    func.lower(Platform.name) == 'pc-windows',
                    func.lower(Platform.name) == 'windows'
                )
            )
        ).first()
        if pc_platform:
            return pc_platform.name
        return "PC (Windows)"
    
    # PlayStation storefronts
    if 'playstation' in storefront_lower or 'sony' in storefront_lower:
        # Find the latest PlayStation platform
        ps_platform = session.exec(
            select(Platform)
            .where(func.lower(Platform.name).like('%playstation%'))
            .order_by(desc(Platform.name))  # PS5 > PS4 > PS3
        ).first()
        if ps_platform:
            return ps_platform.name
    
    # Xbox storefronts
    if 'xbox' in storefront_lower or 'microsoft' in storefront_lower:
        # Find the latest Xbox platform
        xbox_platform = session.exec(
            select(Platform)
            .where(func.lower(Platform.name).like('%xbox%'))
            .order_by(desc(Platform.name))
        ).first()
        if xbox_platform:
            return xbox_platform.name
    
    # Nintendo storefronts
    if 'nintendo' in storefront_lower or 'eshop' in storefront_lower:
        # Find the latest Nintendo platform
        nintendo_platform = session.exec(
            select(Platform)
            .where(func.lower(Platform.name).like('%nintendo%'))
            .order_by(desc(Platform.name))
        ).first()
        if nintendo_platform:
            return nintendo_platform.name
    
    # Default fallback
    logger.debug(f"No specific platform found for storefront '{storefront_name}', defaulting to 'PC (Windows)'")
    return "PC (Windows)"


