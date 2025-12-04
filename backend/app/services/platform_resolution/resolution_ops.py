"""
Platform resolution operations module.

Handles platform and storefront resolution operations for CSV imports.
"""

import logging
from datetime import datetime, timezone
from typing import List, Optional, Dict, Any, Tuple

from sqlmodel import Session, select, and_, or_

from app.models.platform import Platform, Storefront, PlatformStorefront
from app.models.darkadia_import import DarkadiaImport
from app.utils.sqlalchemy_typed import is_, is_not
from app.schemas.platform import (
    PlatformResolutionData,
    PendingPlatformResolution,
    PlatformResolutionResult,
    BulkPlatformResolutionResponse
)

from .suggestions import (
    get_platform_suggestions,
    get_storefront_suggestions,
    suggest_storefront_matches_for_platform
)


logger = logging.getLogger(__name__)


async def get_pending_resolutions(
    session: Session,
    user_id: str,
    page: int = 1,
    per_page: int = 20
) -> Tuple[List[PendingPlatformResolution], int]:
    """
    Get pending platform and storefront resolutions for a user.

    Returns:
        Tuple of (pending_resolutions, total_count)
    """
    # Query DarkadiaImport records with unresolved platforms or storefronts
    query = select(DarkadiaImport).where(
        DarkadiaImport.user_id == user_id,
        and_(
            or_(
                # Unresolved platform
                and_(
                    not DarkadiaImport.platform_resolved,
                    is_not(DarkadiaImport.original_platform_name, None)
                ),
                # Unresolved storefront (has original storefront but no resolved storefront)
                and_(
                    is_not(DarkadiaImport.original_storefront_name, None),
                    is_(DarkadiaImport.resolved_storefront_id, None)
                )
            )
        )
    )

    # Get all import records
    all_imports = session.exec(query).all()

    # Group by combination of platform + storefront to avoid duplicates
    resolution_groups: Dict[str, List[DarkadiaImport]] = {}
    for import_record in all_imports:
        # Create a composite key from platform and storefront
        platform_part = import_record.original_platform_name or ""
        storefront_part = import_record.original_storefront_name or ""
        key = f"{platform_part.lower().strip()}|{storefront_part.lower().strip()}"

        if key not in resolution_groups:
            resolution_groups[key] = []
        resolution_groups[key].append(import_record)

    # Convert to pending resolutions
    pending_resolutions = []

    for resolution_key, import_records in resolution_groups.items():
        # Use the first import record as representative
        representative = import_records[0]

        # Skip if neither platform nor storefront needs resolution
        needs_platform_resolution = (
            representative.original_platform_name and
            not representative.platform_resolved
        )
        needs_storefront_resolution = (
            representative.original_storefront_name and
            not representative.resolved_storefront_id
        )

        if not (needs_platform_resolution or needs_storefront_resolution):
            continue

        # Get affected games
        affected_games = []
        for record in import_records:
            # Get game name from DarkadiaImport's game_name field
            game_name = record.game_name or "Unknown Game"
            if game_name not in affected_games:
                affected_games.append(game_name)

        # Get or create resolution data
        resolution_data = representative.get_platform_resolution_data()
        if not resolution_data or not resolution_data.get("status"):
            # Create initial resolution data
            resolution_data = {
                "status": "pending",
                "original_name": representative.original_platform_name,
                "original_storefront_name": representative.original_storefront_name,
                "suggestions": [],
                "storefront_suggestions": [],
                "resolved_platform_id": None,
                "resolved_storefront_id": None,
                "resolution_timestamp": None,
                "resolution_method": None,
                "user_notes": None,
                "needs_platform_resolution": needs_platform_resolution,
                "needs_storefront_resolution": needs_storefront_resolution
            }
            representative.set_platform_resolution_data(resolution_data)
            session.add(representative)
            session.commit()
        else:
            # Defensive code: ensure required fields exist in existing resolution data
            if 'original_name' not in resolution_data:
                resolution_data['original_name'] = representative.original_platform_name or ""
            if 'suggestions' not in resolution_data:
                resolution_data['suggestions'] = []
            if 'storefront_suggestions' not in resolution_data:
                resolution_data['storefront_suggestions'] = []
            if 'resolved_platform_id' not in resolution_data:
                resolution_data['resolved_platform_id'] = None
            if 'resolved_storefront_id' not in resolution_data:
                resolution_data['resolved_storefront_id'] = None
            if 'resolution_timestamp' not in resolution_data:
                resolution_data['resolution_timestamp'] = None
            if 'resolution_method' not in resolution_data:
                resolution_data['resolution_method'] = None
            if 'user_notes' not in resolution_data:
                resolution_data['user_notes'] = None

        pending_resolution = PendingPlatformResolution(
            import_id=representative.id,
            user_id=user_id,
            original_platform_name=representative.original_platform_name or "",
            original_storefront_name=representative.original_storefront_name or "",
            affected_games_count=len(affected_games),
            affected_games=affected_games[:10],  # Limit for display
            resolution_data=PlatformResolutionData(**resolution_data),
            created_at=representative.created_at
        )
        pending_resolutions.append(pending_resolution)

    total_count = len(pending_resolutions)

    # Apply pagination
    start_idx = (page - 1) * per_page
    end_idx = start_idx + per_page
    paginated_resolutions = pending_resolutions[start_idx:end_idx]

    return paginated_resolutions, total_count


async def resolve_platform(
    session: Session,
    import_id: str,
    user_id: str,
    resolved_platform_id: Optional[str] = None,
    resolved_storefront_id: Optional[str] = None,
    user_notes: Optional[str] = None
) -> PlatformResolutionResult:
    """
    Resolve a platform for a specific import record.

    Args:
        session: Database session
        import_id: ID of the DarkadiaImport record
        user_id: ID of the user (for security)
        resolved_platform_id: Optional ID of platform to resolve to
        resolved_storefront_id: Optional ID of storefront to resolve to
        user_notes: Optional user notes

    Returns:
        PlatformResolutionResult with resolution details
    """
    # Get the import record
    import_record = session.get(DarkadiaImport, import_id)
    if not import_record:
        return PlatformResolutionResult(
            import_id=import_id,
            success=False,
            error_message="Import record not found"
        )

    # Verify user ownership
    if import_record.user_id != user_id:
        return PlatformResolutionResult(
            import_id=import_id,
            success=False,
            error_message="Access denied"
        )

    try:
        # Validate resolved platform if provided
        resolved_platform = None
        if resolved_platform_id:
            resolved_platform = session.get(Platform, resolved_platform_id)
            if not resolved_platform or not resolved_platform.is_active:
                return PlatformResolutionResult(
                    import_id=import_id,
                    success=False,
                    error_message="Invalid or inactive platform"
                )

        # Validate resolved storefront if provided
        resolved_storefront = None
        if resolved_storefront_id:
            resolved_storefront = session.get(Storefront, resolved_storefront_id)
            if not resolved_storefront or not resolved_storefront.is_active:
                return PlatformResolutionResult(
                    import_id=import_id,
                    success=False,
                    error_message="Invalid or inactive storefront"
                )

        # Update resolution data
        resolution_data = import_record.get_platform_resolution_data()
        if not resolution_data:
            resolution_data = {}

        resolution_data.update({
            "status": "resolved",
            "resolved_platform_id": resolved_platform_id,
            "resolved_storefront_id": resolved_storefront_id,
            "resolution_timestamp": datetime.now(timezone.utc).isoformat(),
            "resolution_method": "manual",
            "user_notes": user_notes
        })

        import_record.set_platform_resolution_data(resolution_data)
        import_record.platform_resolved = True
        import_record.updated_at = datetime.now(timezone.utc)

        session.add(import_record)
        session.commit()

        logger.info(f"Resolved platform for import {import_id}: platform={resolved_platform_id}, storefront={resolved_storefront_id}")

        return PlatformResolutionResult(
            import_id=import_id,
            success=True,
            resolved_platform=resolved_platform,
            resolved_storefront=resolved_storefront
        )

    except Exception as e:
        logger.error(f"Error resolving platform for import {import_id}: {str(e)}")
        return PlatformResolutionResult(
            import_id=import_id,
            success=False,
            error_message=f"Resolution failed: {str(e)}"
        )


async def bulk_resolve_platforms(
    session: Session,
    resolutions: List[Dict[str, Any]],
    user_id: str
) -> BulkPlatformResolutionResponse:
    """
    Resolve multiple platforms in a bulk operation.

    Args:
        session: Database session
        resolutions: List of resolution requests
        user_id: ID of the user (for security)

    Returns:
        BulkPlatformResolutionResponse with results
    """
    results = []
    successful_count = 0
    failed_count = 0
    errors = []

    for resolution_data in resolutions:
        try:
            import_id = resolution_data.get("import_id")
            if not import_id:
                raise ValueError("Missing import_id in resolution data")
            result = await resolve_platform(
                session=session,
                import_id=str(import_id),
                user_id=user_id,
                resolved_platform_id=resolution_data.get("resolved_platform_id"),
                resolved_storefront_id=resolution_data.get("resolved_storefront_id"),
                user_notes=resolution_data.get("user_notes")
            )

            results.append(result)

            if result.success:
                successful_count += 1
            else:
                failed_count += 1
                if result.error_message:
                    errors.append(f"Import {result.import_id}: {result.error_message}")

        except Exception as e:
            failed_count += 1
            error_msg = f"Import {resolution_data.get('import_id', 'unknown')}: {str(e)}"
            errors.append(error_msg)
            logger.error(f"Bulk resolution error: {error_msg}")

    return BulkPlatformResolutionResponse(
        total_processed=len(resolutions),
        successful_resolutions=successful_count,
        failed_resolutions=failed_count,
        results=results,
        errors=errors
    )


async def populate_platform_suggestions(
    session: Session,
    import_id: str,
    user_id: str,
    min_confidence: float = 0.6
) -> bool:
    """
    Populate platform suggestions for an import record.

    Args:
        session: Database session
        import_id: ID of the DarkadiaImport record
        user_id: ID of the user (for security)
        min_confidence: Minimum confidence for suggestions

    Returns:
        True if suggestions were populated successfully
    """
    from .service import PlatformResolutionService

    # Get the import record
    import_record = session.get(DarkadiaImport, import_id)
    if not import_record or import_record.user_id != user_id:
        return False

    if not import_record.original_platform_name:
        return False

    try:
        # Use service to get suggestions
        service = PlatformResolutionService(session)
        suggestions_response = await service.suggest_platform_matches(
            unknown_platform_name=import_record.original_platform_name,
            min_confidence=min_confidence,
            max_suggestions=5
        )

        # Update resolution data with suggestions
        resolution_data: Dict[str, Any] = import_record.get_platform_resolution_data() or {}
        if not resolution_data:
            resolution_data = {
                "status": "pending",
                "original_name": import_record.original_platform_name
            }

        resolution_data.update({
            "status": "suggested" if suggestions_response.platform_suggestions else "pending",
            "suggestions": [s.model_dump() for s in suggestions_response.platform_suggestions],
            "storefront_suggestions": [s.model_dump() for s in suggestions_response.storefront_suggestions]
        })

        import_record.set_platform_resolution_data(resolution_data)
        import_record.updated_at = datetime.now(timezone.utc)

        session.add(import_record)
        session.commit()

        return True

    except Exception as e:
        logger.error(f"Error populating suggestions for import {import_id}: {str(e)}")
        return False


async def resolve_storefront(
    session: Session,
    import_id: str,
    user_id: str,
    resolved_storefront_id: str,
    user_notes: Optional[str] = None
) -> bool:
    """
    Resolve a storefront for a specific import record.

    Args:
        session: Database session
        import_id: ID of the DarkadiaImport record
        user_id: ID of the user (for security)
        resolved_storefront_id: ID of storefront to resolve to
        user_notes: Optional user notes

    Returns:
        True if resolution was successful
    """
    # Get the import record
    import_record = session.get(DarkadiaImport, import_id)
    if not import_record or import_record.user_id != user_id:
        return False

    # Validate storefront exists and is active
    storefront = session.get(Storefront, resolved_storefront_id)
    if not storefront or not storefront.is_active:
        return False

    try:
        # Update the import record
        import_record.resolved_storefront_id = resolved_storefront_id
        import_record.storefront_resolved = True
        import_record.updated_at = datetime.now(timezone.utc)

        # Update resolution data
        resolution_data = import_record.get_platform_resolution_data()
        resolution_data.update({
            "storefront_status": "resolved",
            "resolved_storefront_id": resolved_storefront_id,
            "storefront_resolution_timestamp": datetime.now(timezone.utc).isoformat(),
            "storefront_resolution_method": "manual",
            "storefront_user_notes": user_notes
        })
        import_record.set_platform_resolution_data(resolution_data)

        session.add(import_record)
        session.commit()

        logger.info(f"Resolved storefront for import {import_id}: storefront={resolved_storefront_id}")
        return True

    except Exception as e:
        logger.error(f"Error resolving storefront for import {import_id}: {str(e)}")
        session.rollback()
        return False


async def bulk_resolve_storefronts(
    session: Session,
    resolutions: List[Dict[str, Any]],
    user_id: str
) -> Dict[str, Any]:
    """
    Resolve multiple storefronts in a bulk operation.

    Args:
        session: Database session
        resolutions: List of {import_id, storefront_id, user_notes} dictionaries
        user_id: ID of the user

    Returns:
        Dictionary with success count, failed count, and errors
    """
    successful_count = 0
    failed_count = 0
    errors = []

    for resolution in resolutions:
        try:
            import_id = resolution.get("import_id")
            storefront_id = resolution.get("storefront_id")
            if not import_id or not storefront_id:
                raise ValueError("Missing import_id or storefront_id in resolution data")
            success = await resolve_storefront(
                session=session,
                import_id=str(import_id),
                user_id=user_id,
                resolved_storefront_id=str(storefront_id),
                user_notes=resolution.get("user_notes")
            )

            if success:
                successful_count += 1
            else:
                failed_count += 1
                errors.append(f"Failed to resolve storefront for import {resolution.get('import_id')}")

        except Exception as e:
            failed_count += 1
            errors.append(f"Error resolving import {resolution.get('import_id')}: {str(e)}")

    return {
        "total_processed": len(resolutions),
        "successful_resolutions": successful_count,
        "failed_resolutions": failed_count,
        "errors": errors
    }


async def detect_unknown_storefronts(
    session: Session,
    user_id: str,
    batch_id: Optional[str] = None
) -> List[str]:
    """
    Detect unknown storefronts in user's imports.

    Args:
        session: Database session
        user_id: ID of the user
        batch_id: Optional batch ID to limit search

    Returns:
        List of unknown storefront names
    """
    query = select(DarkadiaImport.original_storefront_name).where(
        and_(
            DarkadiaImport.user_id == user_id,
            is_not(DarkadiaImport.original_storefront_name, None),
            or_(
                not DarkadiaImport.storefront_resolved,
                is_(DarkadiaImport.requires_storefront_resolution, True)
            )
        )
    )

    if batch_id:
        query = query.where(DarkadiaImport.batch_id == batch_id)

    results = session.exec(query.distinct()).all()
    return [name for name in results if name and name.strip()]


async def validate_platform_storefront_compatibility(
    session: Session,
    platform_id: str,
    storefront_id: str
) -> bool:
    """
    Check if a platform-storefront combination is valid.

    Args:
        session: Database session
        platform_id: ID of the platform
        storefront_id: ID of the storefront

    Returns:
        True if compatible, False otherwise
    """
    association = session.exec(
        select(PlatformStorefront).where(
            and_(
                PlatformStorefront.platform_id == platform_id,
                PlatformStorefront.storefront_id == storefront_id
            )
        )
    ).first()

    return association is not None


async def auto_resolve_high_confidence_storefronts(
    session: Session,
    pending_resolutions: List[Any],
    confidence_threshold: float = 0.95
) -> List[Any]:
    """
    Automatically resolve storefronts with high confidence matches.

    Args:
        session: Database session
        pending_resolutions: List of PendingStorefrontResolution objects
        confidence_threshold: Minimum confidence to auto-resolve (default 0.95)

    Returns:
        Updated list with auto-resolved items marked appropriately
    """
    updated_resolutions = []

    for resolution in pending_resolutions:
        try:
            # Get storefront suggestions for this resolution
            platform_id = getattr(resolution, 'platform_id', None)
            if not platform_id:
                # Skip if no platform_id available
                updated_resolutions.append(resolution)
                continue
            suggestions = await suggest_storefront_matches_for_platform(
                session=session,
                unknown_storefront_name=resolution.original_storefront_name,
                platform_id=str(platform_id),
                min_confidence=confidence_threshold,
                max_suggestions=1  # We only need the best match
            )

            if suggestions and suggestions[0].confidence >= confidence_threshold:
                best_suggestion = suggestions[0]

                # Auto-resolve this storefront
                success = await resolve_storefront(
                    session=session,
                    import_id=resolution.import_id,
                    user_id=resolution.user_id,
                    resolved_storefront_id=best_suggestion.storefront_id,
                    user_notes=f"Auto-resolved: {best_suggestion.reason} (confidence: {int(best_suggestion.confidence * 100)}%)"
                )

                if success:
                    # Update resolution data to show it was auto-resolved
                    resolution.resolution_data.status = "resolved"
                    resolution.resolution_data.resolved_storefront_id = best_suggestion.storefront_id
                    resolution.resolution_data.resolution_method = "auto"
                    resolution.resolution_data.resolution_timestamp = datetime.now(timezone.utc).isoformat()
                    resolution.resolution_data.user_notes = f"Auto-resolved: {best_suggestion.reason} (confidence: {int(best_suggestion.confidence * 100)}%)"

                    logger.info(f"Auto-resolved storefront '{resolution.original_storefront_name}' to '{best_suggestion.storefront_display_name}' with confidence {best_suggestion.confidence:.2f}")
                    continue  # Skip adding to updated_resolutions since it's resolved

            # If not auto-resolved, populate suggestions for manual resolution
            if not hasattr(resolution.resolution_data, 'suggestions') or not resolution.resolution_data.suggestions:
                general_suggestions = await get_storefront_suggestions(
                    session=session,
                    unknown_name=resolution.original_storefront_name,
                    min_confidence=0.6,
                    max_suggestions=5
                )

                resolution.resolution_data.suggestions = [s.model_dump() for s in general_suggestions]
                resolution.resolution_data.status = "suggested" if general_suggestions else "pending"

            updated_resolutions.append(resolution)

        except Exception as e:
            logger.error(f"Error in auto-resolution for storefront '{resolution.original_storefront_name}': {str(e)}")
            # On error, keep the resolution in the list for manual handling
            updated_resolutions.append(resolution)

    return updated_resolutions
