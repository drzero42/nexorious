"""
Import Mapping API endpoints for managing user platform/storefront mappings.

These mappings define how source strings from imports (e.g., "PC", "Steam")
should be mapped to actual Platform and Storefront entities.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select
from typing import Annotated, Optional
from datetime import datetime, timezone
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.user_import_mapping import UserImportMapping, ImportMappingType
from ..schemas.import_mapping import (
    ImportMappingResponse,
    ImportMappingListResponse,
    ImportMappingCreateRequest,
    ImportMappingUpdateRequest,
    BatchImportMappingRequest,
    BatchImportMappingResponse,
    MappingType,
)

router = APIRouter(prefix="/import-mappings", tags=["Import Mappings"])
logger = logging.getLogger(__name__)


def _to_response(mapping: UserImportMapping) -> ImportMappingResponse:
    """Convert a UserImportMapping model to response schema."""
    return ImportMappingResponse(
        id=mapping.id,
        user_id=mapping.user_id,
        import_source=mapping.import_source,
        mapping_type=MappingType(mapping.mapping_type.value),
        source_value=mapping.source_value,
        target_id=mapping.target_id,
        created_at=mapping.created_at,
        updated_at=mapping.updated_at,
    )


@router.get("/", response_model=ImportMappingListResponse)
async def list_import_mappings(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    import_source: Optional[str] = Query(
        default=None, description="Filter by import source"
    ),
    mapping_type: Optional[MappingType] = Query(
        default=None, description="Filter by mapping type"
    ),
):
    """
    List all import mappings for the current user.

    Optionally filter by import source and/or mapping type.
    """
    logger.debug(
        f"Listing import mappings for user {current_user.id}: "
        f"source={import_source}, type={mapping_type}"
    )

    query = select(UserImportMapping).where(
        UserImportMapping.user_id == current_user.id
    )

    if import_source:
        query = query.where(UserImportMapping.import_source == import_source)

    if mapping_type:
        query = query.where(
            UserImportMapping.mapping_type == ImportMappingType(mapping_type.value)
        )

    # Order by source_value for consistent ordering
    query = query.order_by(UserImportMapping.source_value)

    mappings = session.exec(query).all()

    return ImportMappingListResponse(
        items=[_to_response(m) for m in mappings],
        total=len(mappings),
    )


@router.get("/lookup", response_model=ImportMappingResponse)
async def lookup_import_mapping(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    import_source: str = Query(..., description="Import source"),
    mapping_type: MappingType = Query(..., description="Mapping type"),
    source_value: str = Query(..., description="Source value to look up"),
):
    """
    Look up a specific import mapping by source value.

    Returns the mapping if found, or 404 if not found.
    """
    mapping = session.exec(
        select(UserImportMapping).where(
            UserImportMapping.user_id == current_user.id,
            UserImportMapping.import_source == import_source,
            UserImportMapping.mapping_type == ImportMappingType(mapping_type.value),
            UserImportMapping.source_value == source_value,
        )
    ).first()

    if not mapping:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Mapping not found",
        )

    return _to_response(mapping)


@router.get("/{mapping_id}", response_model=ImportMappingResponse)
async def get_import_mapping(
    mapping_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Get a specific import mapping by ID.
    """
    mapping = session.get(UserImportMapping, mapping_id)

    if not mapping or mapping.user_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Mapping not found",
        )

    return _to_response(mapping)


@router.post("/", response_model=ImportMappingResponse, status_code=status.HTTP_201_CREATED)
async def create_import_mapping(
    request: ImportMappingCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Create a new import mapping.

    Returns 409 Conflict if a mapping already exists for this
    user/source/type/value combination.
    """
    logger.info(
        f"Creating import mapping for user {current_user.id}: "
        f"{request.import_source}/{request.mapping_type}/{request.source_value} -> {request.target_id}"
    )

    # Check for existing mapping
    existing = session.exec(
        select(UserImportMapping).where(
            UserImportMapping.user_id == current_user.id,
            UserImportMapping.import_source == request.import_source,
            UserImportMapping.mapping_type == ImportMappingType(request.mapping_type.value),
            UserImportMapping.source_value == request.source_value,
        )
    ).first()

    if existing:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Mapping already exists for this source value",
        )

    mapping = UserImportMapping(
        user_id=current_user.id,
        import_source=request.import_source,
        mapping_type=ImportMappingType(request.mapping_type.value),
        source_value=request.source_value,
        target_id=request.target_id,
    )

    session.add(mapping)
    session.commit()
    session.refresh(mapping)

    logger.info(f"Created import mapping {mapping.id}")

    return _to_response(mapping)


@router.put("/{mapping_id}", response_model=ImportMappingResponse)
async def update_import_mapping(
    mapping_id: str,
    request: ImportMappingUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Update an existing import mapping.

    Only the target_id can be updated.
    """
    mapping = session.get(UserImportMapping, mapping_id)

    if not mapping or mapping.user_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Mapping not found",
        )

    logger.info(
        f"Updating import mapping {mapping_id}: target_id {mapping.target_id} -> {request.target_id}"
    )

    mapping.target_id = request.target_id
    mapping.updated_at = datetime.now(timezone.utc)

    session.add(mapping)
    session.commit()
    session.refresh(mapping)

    return _to_response(mapping)


@router.delete("/{mapping_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_import_mapping(
    mapping_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Delete an import mapping.
    """
    mapping = session.get(UserImportMapping, mapping_id)

    if not mapping or mapping.user_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Mapping not found",
        )

    logger.info(f"Deleting import mapping {mapping_id}")

    session.delete(mapping)
    session.commit()


@router.post("/batch", response_model=BatchImportMappingResponse)
async def batch_import_mappings(
    request: BatchImportMappingRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Create or update multiple import mappings at once.

    This is an upsert operation - existing mappings will be updated,
    new mappings will be created.
    """
    logger.info(
        f"Batch import mappings for user {current_user.id}: "
        f"source={request.import_source}, count={len(request.mappings)}"
    )

    created = 0
    updated = 0

    for item in request.mappings:
        # Check for existing mapping
        existing = session.exec(
            select(UserImportMapping).where(
                UserImportMapping.user_id == current_user.id,
                UserImportMapping.import_source == request.import_source,
                UserImportMapping.mapping_type == ImportMappingType(item.mapping_type.value),
                UserImportMapping.source_value == item.source_value,
            )
        ).first()

        if existing:
            # Update existing
            if existing.target_id != item.target_id:
                existing.target_id = item.target_id
                existing.updated_at = datetime.now(timezone.utc)
                session.add(existing)
                updated += 1
        else:
            # Create new
            mapping = UserImportMapping(
                user_id=current_user.id,
                import_source=request.import_source,
                mapping_type=ImportMappingType(item.mapping_type.value),
                source_value=item.source_value,
                target_id=item.target_id,
            )
            session.add(mapping)
            created += 1

    session.commit()

    logger.info(f"Batch complete: {created} created, {updated} updated")

    return BatchImportMappingResponse(created=created, updated=updated)
