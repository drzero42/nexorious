"""
Platform and storefront management endpoints (admin-only).
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query, Response, UploadFile, File, Request
from sqlmodel import Session, select, func, or_, desc
from datetime import datetime, timezone
from typing import Annotated, Optional
import logging

from ..core.database import get_session
from ..core.security import get_current_admin_user, get_current_user
from ..models.user import User
from ..models.platform import Platform, Storefront, PlatformStorefront
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
    UpdatePlatformDefaultRequest,
    PlatformStorefrontsResponse,
    PlatformStorefrontAssociationResponse,
    # Platform Resolution schemas
    PlatformSuggestionsRequest,
    PlatformSuggestionsResponse,
    PlatformResolutionRequest,
    BulkPlatformResolutionRequest,
    BulkPlatformResolutionResponse,
    PendingResolutionsListResponse,
    # Storefront Resolution schemas
    StorefrontSuggestionsRequest,
    StorefrontSuggestionsResponse,
    StorefrontResolutionData,
    PendingStorefrontResolution,
    StorefrontResolutionRequest,
    BulkStorefrontResolutionRequest,
    StorefrontResolutionResult,
    BulkStorefrontResolutionResponse,
    PendingStorefrontsListResponse,
    StorefrontCompatibilityRequest,
    StorefrontCompatibilityResponse
)
from ..api.schemas.common import SuccessResponse
from ..services.logo_service import LogoService, logo_service
from ..services.platform_resolution import create_platform_resolution_service, PlatformResolutionService
from ..core.audit_logging import get_client_ip, audit

router = APIRouter(prefix="/platforms", tags=["Platforms & Storefronts"])
logger = logging.getLogger("uvicorn.error")


def get_logo_service() -> LogoService:
    """Dependency to get the logo service instance."""
    return logo_service


def get_platform_resolution_service(session: Annotated[Session, Depends(get_session)]):
    """Dependency to get platform resolution service instance."""
    return create_platform_resolution_service(session)


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
            .where(PlatformStorefront.platform_id == platform.id)
            .where(Storefront.is_active == True)  # Only include active storefronts
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


@router.get("/{platform_id}", response_model=PlatformResponse)
async def get_platform(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get a specific platform by ID."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    return platform


@router.get("/{platform_id}/storefronts", response_model=PlatformStorefrontsResponse)
async def get_platform_storefronts(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    active_only: bool = Query(default=True, description="Show only active storefronts")
):
    """Get all storefronts associated with a specific platform."""
    
    # First, verify the platform exists
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Query for associated storefronts via the junction table
    query = (
        select(Storefront)
        .join(PlatformStorefront)
        .where(PlatformStorefront.platform_id == platform_id)
    )
    
    # Filter by active status if requested
    if active_only:
        query = query.where(Storefront.is_active)
    
    # Order by storefront display name
    query = query.order_by(Storefront.display_name)
    
    storefronts = session.exec(query).all()
    
    return PlatformStorefrontsResponse(
        platform_id=platform.id,
        platform_name=platform.name,
        platform_display_name=platform.display_name,
        storefronts=[StorefrontResponse.model_validate(sf) for sf in storefronts],
        total_storefronts=len(storefronts)
    )


@router.post("/{platform_id}/storefronts/{storefront_id}", response_model=PlatformStorefrontAssociationResponse)
async def create_platform_storefront_association(
    platform_id: str,
    storefront_id: str,
    response: Response,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Create a platform-storefront association (admin only)."""
    
    # Verify platform exists and is active
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    if not platform.is_active:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot create association with inactive platform"
        )
    
    # Verify storefront exists and is active
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    if not storefront.is_active:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot create association with inactive storefront"
        )
    
    # Check if association already exists
    existing_association = session.exec(
        select(PlatformStorefront).where(
            PlatformStorefront.platform_id == platform_id,
            PlatformStorefront.storefront_id == storefront_id
        )
    ).first()
    
    if existing_association:
        # Association already exists, return 200 with appropriate message
        response.status_code = status.HTTP_200_OK
        return PlatformStorefrontAssociationResponse(
            platform_id=platform.id,
            platform_name=platform.name,
            platform_display_name=platform.display_name,
            storefront_id=storefront.id,
            storefront_name=storefront.name,
            storefront_display_name=storefront.display_name,
            message="Association already exists"
        )
    
    # Create new association
    new_association = PlatformStorefront(
        platform_id=platform_id,
        storefront_id=storefront_id
    )
    
    session.add(new_association)
    session.commit()
    
    # Set 201 status code for successful creation
    response.status_code = status.HTTP_201_CREATED
    return PlatformStorefrontAssociationResponse(
        platform_id=platform.id,
        platform_name=platform.name,
        platform_display_name=platform.display_name,
        storefront_id=storefront.id,
        storefront_name=storefront.name,
        storefront_display_name=storefront.display_name,
        message="Association created successfully"
    )


@router.delete("/{platform_id}/storefronts/{storefront_id}", response_model=PlatformStorefrontAssociationResponse)
async def delete_platform_storefront_association(
    platform_id: str,
    storefront_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Remove a platform-storefront association (admin only)."""
    
    # Verify platform exists
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Verify storefront exists
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    
    # Find the association
    association = session.exec(
        select(PlatformStorefront).where(
            PlatformStorefront.platform_id == platform_id,
            PlatformStorefront.storefront_id == storefront_id
        )
    ).first()
    
    message = "Association removed successfully"
    if association:
        # Remove the association
        session.delete(association)
        session.commit()
    else:
        # Association doesn't exist, but return success anyway (idempotent operation)
        message = "Association does not exist"
    
    return PlatformStorefrontAssociationResponse(
        platform_id=platform.id,
        platform_name=platform.name,
        platform_display_name=platform.display_name,
        storefront_id=storefront.id,
        storefront_name=storefront.name,
        storefront_display_name=storefront.display_name,
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
        icon_url=platform_data.icon_url,
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
    
    # Change source to "custom" when admin edits official platform
    if platform.source == "official":
        platform.source = "custom"
    
    for field, value in update_data.items():
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


# Platform logo management endpoints
@router.post("/{platform_id}/logo")
async def upload_platform_logo(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: str = Query(..., description="Logo theme: light or dark"),
    file: UploadFile = File(...)
):
    """Upload a logo for a platform (admin only)."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Upload the logo file
    icon_url = await logo_service.upload_logo("platforms", platform.name, theme, file)
    
    # Update platform's icon_url to point to the new logo
    # We'll use the light theme as the default icon_url
    if theme == "light":
        platform.icon_url = icon_url
        platform.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(platform)
    
    return {
        "message": f"Logo uploaded successfully for {platform.display_name}",
        "theme": theme,
        "icon_url": icon_url,
        "platform": platform
    }


@router.delete("/{platform_id}/logo")
async def delete_platform_logo(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: Optional[str] = Query(default=None, description="Logo theme to delete (light/dark), or all if not specified")
):
    """Delete logo(s) for a platform (admin only)."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Delete logo file(s)
    deleted_files = logo_service.delete_logo("platforms", platform.name, theme)
    
    # If we deleted the light theme (which is the default), clear the icon_url
    if theme == "light" or theme is None:
        platform.icon_url = None
        platform.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(platform)
    
    return {
        "message": f"Logo(s) deleted successfully for {platform.display_name}",
        "deleted_files": len(deleted_files),
        "platform": platform
    }


@router.get("/{platform_id}/logos")
async def list_platform_logos(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)]
):
    """List available logos for a platform (admin only)."""
    
    platform = session.get(Platform, platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    logos = logo_service.list_logos("platforms", platform.name)
    
    return {
        "platform": platform,
        "logos": logos
    }


@router.get("/{platform_id}/default-storefront", response_model=PlatformDefaultMapping)
async def get_platform_default_storefront(
    platform_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
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
        default_storefront=StorefrontResponse.model_validate(default_storefront) if default_storefront is not None else None
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
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
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
        icon_url=storefront_data.icon_url,
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
    
    # Change source to "custom" when admin edits official storefront
    if storefront.source == "official":
        storefront.source = "custom"
    
    for field, value in update_data.items():
        if field == "base_url" and value:
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


# Storefront logo management endpoints
@router.post("/storefronts/{storefront_id}/logo")
async def upload_storefront_logo(
    storefront_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: str = Query(..., description="Logo theme: light or dark"),
    file: UploadFile = File(...)
):
    """Upload a logo for a storefront (admin only)."""
    
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    
    # Upload the logo file
    icon_url = await logo_service.upload_logo("storefronts", storefront.name, theme, file)
    
    # Update storefront's icon_url to point to the new logo
    # We'll use the light theme as the default icon_url
    if theme == "light":
        storefront.icon_url = icon_url
        storefront.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(storefront)
    
    return {
        "message": f"Logo uploaded successfully for {storefront.display_name}",
        "theme": theme,
        "icon_url": icon_url,
        "storefront": storefront
    }


@router.delete("/storefronts/{storefront_id}/logo")
async def delete_storefront_logo(
    storefront_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)],
    theme: Optional[str] = Query(default=None, description="Logo theme to delete (light/dark), or all if not specified")
):
    """Delete logo(s) for a storefront (admin only)."""
    
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    
    # Delete logo file(s)
    deleted_files = logo_service.delete_logo("storefronts", storefront.name, theme)
    
    # If we deleted the light theme (which is the default), clear the icon_url
    if theme == "light" or theme is None:
        storefront.icon_url = None
        storefront.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(storefront)
    
    return {
        "message": f"Logo(s) deleted successfully for {storefront.display_name}",
        "deleted_files": len(deleted_files),
        "storefront": storefront
    }


@router.get("/storefronts/{storefront_id}/logos")
async def list_storefront_logos(
    storefront_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
    logo_service: Annotated[LogoService, Depends(get_logo_service)]
):
    """List available logos for a storefront (admin only)."""
    
    storefront = session.get(Storefront, storefront_id)
    if not storefront:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Storefront not found"
        )
    
    logos = logo_service.list_logos("storefronts", storefront.name)
    
    return {
        "storefront": storefront,
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
    # Try to find the storefront by name (case-insensitive)
    storefront = session.exec(
        select(Storefront).where(
            or_(
                func.lower(Storefront.name) == storefront_name.lower(),
                func.lower(Storefront.display_name) == storefront_name.lower()
            )
        )
    ).first()
    
    if storefront:
        # Get platforms associated with this storefront
        platform_storefronts = session.exec(
            select(PlatformStorefront)
            .where(PlatformStorefront.storefront_id == storefront.id)
            .order_by(PlatformStorefront.created_at.asc())  # Oldest association first
        ).all()
        
        if platform_storefronts:
            # Get the first associated platform
            platform = session.get(Platform, platform_storefronts[0].platform_id)
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
            .order_by(Platform.name.desc())  # PS5 > PS4 > PS3
        ).first()
        if ps_platform:
            return ps_platform.name
    
    # Xbox storefronts
    if 'xbox' in storefront_lower or 'microsoft' in storefront_lower:
        # Find the latest Xbox platform
        xbox_platform = session.exec(
            select(Platform)
            .where(func.lower(Platform.name).like('%xbox%'))
            .order_by(Platform.name.desc())
        ).first()
        if xbox_platform:
            return xbox_platform.name
    
    # Nintendo storefronts
    if 'nintendo' in storefront_lower or 'eshop' in storefront_lower:
        # Find the latest Nintendo platform
        nintendo_platform = session.exec(
            select(Platform)
            .where(func.lower(Platform.name).like('%nintendo%'))
            .order_by(Platform.name.desc())
        ).first()
        if nintendo_platform:
            return nintendo_platform.name
    
    # Default fallback
    logger.debug(f"No specific platform found for storefront '{storefront_name}', defaulting to 'PC (Windows)'")
    return "PC (Windows)"


# Platform Resolution endpoints
@router.post("/resolution/suggestions", response_model=PlatformSuggestionsResponse)
async def get_platform_suggestions(
    request: PlatformSuggestionsRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    http_request: Request,
    resolution_service: Annotated[PlatformResolutionService, Depends(get_platform_resolution_service)],
    session: Annotated[Session, Depends(get_session)]
) -> PlatformSuggestionsResponse:
    """
    Get fuzzy matching suggestions for unknown platform and storefront names.
    
    This endpoint helps users resolve unknown platforms found during CSV imports
    by providing ranked suggestions based on fuzzy matching against existing
    platforms and storefronts.
    """
    # Add comprehensive debug logging
    logger.debug(f"=== Platform Suggestions Request ===")
    logger.debug(f"Request type: {type(request)}")
    logger.debug(f"Request data: {request.model_dump() if hasattr(request, 'model_dump') else request}")
    logger.debug(f"HTTP Request type: {type(http_request)}")
    logger.debug(f"HTTP Request available: {http_request is not None}")
    
    client_ip = get_client_ip(http_request) if http_request else None
    logger.debug(f"Client IP: {client_ip}")
    
    try:
        # Handle empty platform names by getting default platform for storefront
        platform_name = request.unknown_platform_name
        if not platform_name and request.unknown_storefront_name:
            platform_name = get_default_platform_for_storefront(session, request.unknown_storefront_name)
            logger.debug(f"Resolved empty platform name to '{platform_name}' for storefront '{request.unknown_storefront_name}'")
        elif not platform_name:
            # No platform name and no storefront name - use default fallback
            platform_name = "PC (Windows)"
            logger.debug(f"Using fallback platform name '{platform_name}' for empty platform and storefront names")
        
        # Input validation and sanitization is already handled in the service
        suggestions = await resolution_service.suggest_platform_matches(
            unknown_platform_name=platform_name,
            unknown_storefront_name=request.unknown_storefront_name,
            min_confidence=request.min_confidence,
            max_suggestions=request.max_suggestions
        )
        
        # Audit log successful suggestion request
        audit.log_platform_suggestion(
            user_id=current_user.id,
            unknown_platform_name=platform_name,
            suggestions_count=suggestions.total_platform_suggestions,
            request_ip=client_ip,
            min_confidence=request.min_confidence
        )
        
        return suggestions
    except Exception as e:
        logger.error(f"Error in platform suggestions: {type(e).__name__}: {str(e)}")
        logger.error(f"Full exception: {e}", exc_info=True)
        
        # Audit log failure
        audit.log_invalid_input(
            user_id=current_user.id,
            operation_name="platform_suggestions",
            invalid_input=platform_name if 'platform_name' in locals() else request.unknown_platform_name,
            request_ip=client_ip
        )
        
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get platform suggestions"
        )


@router.get("/resolution/pending", response_model=PendingResolutionsListResponse)
async def get_pending_platform_resolutions(
    current_user: Annotated[User, Depends(get_current_user)],
    resolution_service = Depends(get_platform_resolution_service),
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page")
) -> PendingResolutionsListResponse:
    """
    Get pending platform resolutions for the current user.
    
    Returns a paginated list of unresolved platforms from CSV imports
    that require user attention for proper platform mapping.
    """
    try:
        pending_resolutions, total = await resolution_service.get_pending_resolutions(
            user_id=current_user.id,
            page=page,
            per_page=per_page
        )
        
        # Calculate pages
        pages = (total + per_page - 1) // per_page
        
        return PendingResolutionsListResponse(
            pending_resolutions=pending_resolutions,
            total=total,
            page=page,
            per_page=per_page,
            pages=pages
        )
    except Exception as e:
        logger.error(f"Error getting pending resolutions for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get pending platform resolutions"
        )


@router.post("/resolution/resolve")
async def resolve_platform(
    request: PlatformResolutionRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    http_request: Request,
    resolution_service = Depends(get_platform_resolution_service)
):
    """
    Resolve a single platform mapping.
    
    Maps an unknown platform from a CSV import to an existing platform
    and optionally to a storefront. This updates the import record
    to mark the platform as resolved.
    """
    client_ip = get_client_ip(http_request) if http_request else None
    
    try:
        result = await resolution_service.resolve_platform(
            import_id=request.import_id,
            user_id=current_user.id,
            resolved_platform_id=request.resolved_platform_id,
            resolved_storefront_id=request.resolved_storefront_id,
            user_notes=request.user_notes
        )
        
        # Get original platform name for audit logging
        original_platform_name = "unknown"
        if hasattr(result, 'resolved_platform') and result.resolved_platform:
            original_platform_name = result.resolved_platform.display_name
        
        # Audit log the resolution attempt
        audit.log_platform_resolution(
            user_id=current_user.id,
            import_id=request.import_id,
            original_platform_name=original_platform_name,
            resolved_platform_id=request.resolved_platform_id,
            resolved_storefront_id=request.resolved_storefront_id,
            success=result.success,
            error_message=result.error_message,
            request_ip=client_ip
        )
        
        if not result.success:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=result.error_message or "Failed to resolve platform"
            )
        
        return result
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error resolving platform for user {current_user.id}: {str(e)}")
        
        # Audit log the failure
        audit.log_platform_resolution(
            user_id=current_user.id,
            import_id=request.import_id,
            original_platform_name="unknown",
            resolved_platform_id=request.resolved_platform_id,
            resolved_storefront_id=request.resolved_storefront_id,
            success=False,
            error_message=str(e),
            request_ip=client_ip
        )
        
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to resolve platform"
        )


@router.post("/resolution/bulk-resolve", response_model=BulkPlatformResolutionResponse)
async def bulk_resolve_platforms(
    request: BulkPlatformResolutionRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    http_request: Request,
    resolution_service = Depends(get_platform_resolution_service)
) -> BulkPlatformResolutionResponse:
    """
    Resolve multiple platform mappings in a single operation.
    
    Efficiently handles multiple platform resolutions at once,
    useful for users who have many unknown platforms to resolve
    from their CSV imports.
    """
    client_ip = get_client_ip(http_request) if http_request else None
    
    try:
        # Convert request to dict format expected by service
        resolutions_data = []
        for resolution in request.resolutions:
            resolutions_data.append({
                "import_id": resolution.import_id,
                "resolved_platform_id": resolution.resolved_platform_id,
                "resolved_storefront_id": resolution.resolved_storefront_id,
                "user_notes": resolution.user_notes
            })
        
        result = await resolution_service.bulk_resolve_platforms(
            resolutions=resolutions_data,
            user_id=current_user.id
        )
        
        # Audit log bulk resolution
        audit.log_bulk_resolution(
            user_id=current_user.id,
            total_processed=result.total_processed,
            successful_count=result.successful_resolutions,
            failed_count=result.failed_resolutions,
            request_ip=client_ip
        )
        
        return result
    except Exception as e:
        logger.error(f"Error bulk resolving platforms for user {current_user.id}: {str(e)}")
        
        # Audit log failure
        audit.log_bulk_resolution(
            user_id=current_user.id,
            total_processed=len(request.resolutions),
            successful_count=0,
            failed_count=len(request.resolutions),
            request_ip=client_ip
        )
        
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to bulk resolve platforms"
        )


@router.post("/resolution/populate-suggestions/{import_id}")
async def populate_platform_suggestions(
    import_id: str,
    current_user: Annotated[User, Depends(get_current_user)],
    resolution_service = Depends(get_platform_resolution_service),
    min_confidence: float = Query(default=0.6, ge=0.0, le=1.0, description="Minimum confidence for suggestions")
):
    """
    Populate platform suggestions for a specific import record.
    
    This endpoint can be used to generate and cache platform suggestions
    for import records that don't have suggestions yet, or to regenerate
    suggestions with different confidence thresholds.
    """
    try:
        success = await resolution_service.populate_platform_suggestions(
            import_id=import_id,
            user_id=current_user.id,
            min_confidence=min_confidence
        )
        
        if not success:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Import record not found or no platform name to resolve"
            )
        
        return {"message": "Platform suggestions populated successfully", "success": True}
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error populating suggestions for import {import_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to populate platform suggestions"
        )


# Storefront Resolution Endpoints

@router.post("/resolution/storefront-suggestions", response_model=StorefrontSuggestionsResponse)
async def get_storefront_suggestions(
    request: StorefrontSuggestionsRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    resolution_service: Annotated[PlatformResolutionService, Depends(get_platform_resolution_service)],
    session: Annotated[Session, Depends(get_session)],
    http_request: Request
):
    """
    Get platform-contextual suggestions for unknown storefronts.
    
    This endpoint provides fuzzy matching suggestions for unknown storefront names,
    optionally filtered by platform compatibility. When a platform_id is provided,
    suggestions will be prioritized based on platform-storefront associations.
    """
    try:
        suggestions = []
        platform = None
        
        if request.platform_id:
            # Get platform-contextual suggestions
            platform = session.get(Platform, request.platform_id)
            if platform:
                suggestions = await resolution_service.suggest_storefront_matches_for_platform(
                    unknown_storefront_name=request.unknown_storefront_name,
                    platform_id=request.platform_id,
                    min_confidence=request.min_confidence,
                    max_suggestions=request.max_suggestions
                )
        
        if not suggestions:
            # Fall back to general storefront suggestions
            suggestions_response = await resolution_service.suggest_platform_matches(
                unknown_platform_name="",  # Empty platform name
                unknown_storefront_name=request.unknown_storefront_name,
                min_confidence=request.min_confidence,
                max_suggestions=request.max_suggestions
            )
            suggestions = suggestions_response.storefront_suggestions
        
        return StorefrontSuggestionsResponse(
            unknown_storefront_name=request.unknown_storefront_name,
            platform_id=request.platform_id,
            platform_name=platform.display_name if platform else None,
            storefront_suggestions=suggestions,
            total_suggestions=len(suggestions),
            is_platform_contextual=bool(request.platform_id and platform)
        )
    except Exception as e:
        logger.error(f"Error getting storefront suggestions: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get storefront suggestions"
        )


@router.get("/resolution/pending-storefronts", response_model=PendingStorefrontsListResponse)
async def get_pending_storefront_resolutions(
    current_user: Annotated[User, Depends(get_current_user)],
    resolution_service: Annotated[PlatformResolutionService, Depends(get_platform_resolution_service)],
    session: Annotated[Session, Depends(get_session)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page")
):
    """
    Get pending storefront resolutions for the current user.
    
    Returns a paginated list of unresolved storefronts that need manual resolution.
    Each entry includes the original storefront name, affected games, and platform context.
    """
    try:
        # Get unknown storefronts from user's imports
        unknown_storefronts = await resolution_service.detect_unknown_storefronts(
            user_id=current_user.id
        )
        
        # Convert to pending storefront resolutions format
        pending_storefronts = []
        
        for storefront_name in unknown_storefronts:
            # Get imports with this storefront name to build context
            from sqlmodel import select, and_
            from ..models.darkadia_import import DarkadiaImport
            
            imports = session.exec(
                select(DarkadiaImport).where(
                    and_(
                        DarkadiaImport.user_id == current_user.id,
                        DarkadiaImport.original_storefront_name == storefront_name,
                        DarkadiaImport.storefront_resolved == False
                    )
                ).limit(10)  # Limit for performance
            ).all()
            
            if imports:
                affected_games = list(set(imp.game_name for imp in imports if imp.game_name))
                platform_context = imports[0].original_platform_name if imports else None
                
                # Create resolution data for this storefront
                resolution_data = StorefrontResolutionData(
                    status="pending",
                    original_name=storefront_name,
                    suggestions=[],  # Will be populated on demand
                    platform_context=platform_context
                )
                
                pending_storefront = PendingStorefrontResolution(
                    import_id=imports[0].id,  # Use first import as representative
                    user_id=current_user.id,
                    original_storefront_name=storefront_name,
                    original_platform_name=platform_context,
                    affected_games_count=len(affected_games),
                    affected_games=affected_games[:5],  # Limit for display
                    platform_context=platform_context,
                    resolution_data=resolution_data,
                    created_at=imports[0].created_at
                )
                pending_storefronts.append(pending_storefront)
        
        # Apply auto-resolution for high confidence matches
        pending_storefronts = await resolution_service.auto_resolve_high_confidence_storefronts(
            pending_storefronts,
            confidence_threshold=0.95
        )
        
        # Apply pagination
        total = len(pending_storefronts)
        start_idx = (page - 1) * per_page
        end_idx = start_idx + per_page
        paginated_storefronts = pending_storefronts[start_idx:end_idx]
        
        pages = (total + per_page - 1) // per_page
        
        return PendingStorefrontsListResponse(
            pending_resolutions=paginated_storefronts,
            total=total,
            page=page,
            per_page=per_page,
            pages=pages
        )
    except Exception as e:
        logger.error(f"Error getting pending storefront resolutions: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to get pending storefront resolutions"
        )


@router.post("/resolution/resolve-storefront", response_model=StorefrontResolutionResult)
async def resolve_storefront(
    resolution_request: StorefrontResolutionRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    resolution_service: Annotated[PlatformResolutionService, Depends(get_platform_resolution_service)],
    session: Annotated[Session, Depends(get_session)],
    http_request: Request
):
    """
    Resolve a single storefront for an import record.
    
    This endpoint resolves an unknown storefront by mapping it to an existing 
    storefront in the database. The resolution is stored and tracked for the user.
    """
    try:
        success = await resolution_service.resolve_storefront(
            import_id=resolution_request.import_id,
            user_id=current_user.id,
            resolved_storefront_id=resolution_request.resolved_storefront_id,
            user_notes=resolution_request.user_notes
        )
        
        if not success:
            return StorefrontResolutionResult(
                import_id=resolution_request.import_id,
                success=False,
                error_message="Failed to resolve storefront - import not found or access denied"
            )
        
        # Get the resolved storefront details for response
        resolved_storefront = session.get(Storefront, resolution_request.resolved_storefront_id)
        
        return StorefrontResolutionResult(
            import_id=resolution_request.import_id,
            success=True,
            resolved_storefront=resolved_storefront,
            error_message=None
        )
    except Exception as e:
        logger.error(f"Error resolving storefront: {str(e)}")
        return StorefrontResolutionResult(
            import_id=resolution_request.import_id,
            success=False,
            error_message=f"Resolution failed: {str(e)}"
        )


@router.post("/resolution/bulk-resolve-storefronts", response_model=BulkStorefrontResolutionResponse)
async def bulk_resolve_storefronts(
    bulk_request: BulkStorefrontResolutionRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    resolution_service: Annotated[PlatformResolutionService, Depends(get_platform_resolution_service)],
    session: Annotated[Session, Depends(get_session)],
    http_request: Request
):
    """
    Resolve multiple storefronts in a single bulk operation.
    
    This endpoint allows efficient resolution of multiple unknown storefronts
    at once, providing detailed results for each resolution attempt.
    """
    try:
        # Convert request to format expected by service
        resolutions_data = [
            {
                "import_id": res.import_id,
                "storefront_id": res.resolved_storefront_id,
                "user_notes": res.user_notes
            }
            for res in bulk_request.resolutions
        ]
        
        # Call the bulk resolution service
        bulk_result = await resolution_service.bulk_resolve_storefronts(
            resolutions=resolutions_data,
            user_id=current_user.id
        )
        
        # Convert results to proper response format
        results = []
        for resolution, resolution_data in zip(bulk_request.resolutions, resolutions_data):
            if resolution_data.get("import_id") in [r.split(":")[0] for r in bulk_result.get("errors", [])]:
                # This resolution failed
                error_msg = next((e for e in bulk_result["errors"] if e.startswith(resolution_data["import_id"])), "Unknown error")
                result = StorefrontResolutionResult(
                    import_id=resolution.import_id,
                    success=False,
                    error_message=error_msg
                )
            else:
                # This resolution succeeded
                resolved_storefront = session.get(Storefront, resolution.resolved_storefront_id)
                result = StorefrontResolutionResult(
                    import_id=resolution.import_id,
                    success=True,
                    resolved_storefront=resolved_storefront
                )
            results.append(result)
        
        return BulkStorefrontResolutionResponse(
            total_processed=bulk_result["total_processed"],
            successful_resolutions=bulk_result["successful_resolutions"],
            failed_resolutions=bulk_result["failed_resolutions"],
            results=results,
            errors=bulk_result["errors"]
        )
    except Exception as e:
        logger.error(f"Error in bulk storefront resolution: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to perform bulk storefront resolution"
        )


@router.post("/compatibility/check", response_model=StorefrontCompatibilityResponse)
async def check_platform_storefront_compatibility(
    compatibility_request: StorefrontCompatibilityRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    resolution_service: Annotated[PlatformResolutionService, Depends(get_platform_resolution_service)],
    session: Annotated[Session, Depends(get_session)]
):
    """
    Check if a platform-storefront combination is valid.
    
    This endpoint validates whether a given storefront is compatible with
    a specific platform based on the platform-storefront association table.
    """
    try:
        # Get platform and storefront details
        platform = session.get(Platform, compatibility_request.platform_id)
        storefront = session.get(Storefront, compatibility_request.storefront_id)
        
        if not platform:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Platform not found"
            )
        
        if not storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Storefront not found"
            )
        
        # Check compatibility
        is_compatible = await resolution_service.validate_platform_storefront_compatibility(
            platform_id=compatibility_request.platform_id,
            storefront_id=compatibility_request.storefront_id
        )
        
        message = (
            f"{storefront.display_name} is compatible with {platform.display_name}"
            if is_compatible
            else f"{storefront.display_name} is not associated with {platform.display_name}"
        )
        
        return StorefrontCompatibilityResponse(
            platform_id=platform.id,
            platform_name=platform.display_name,
            storefront_id=storefront.id,
            storefront_name=storefront.display_name,
            is_compatible=is_compatible,
            message=message
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error checking platform-storefront compatibility: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to check compatibility"
        )