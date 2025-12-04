"""
Platform resolution suggestions module.

Handles fuzzy matching suggestions for platform and storefront names.
"""

import logging
from typing import List, Tuple

from sqlmodel import Session, select

from app.models.platform import Platform, Storefront, PlatformStorefront
from app.utils.fuzzy_match import calculate_fuzzy_confidence
from app.schemas.platform import PlatformSuggestion, StorefrontSuggestion

from .models import sanitize_platform_name


logger = logging.getLogger(__name__)


async def get_platform_suggestions(
    session: Session,
    unknown_name: str,
    min_confidence: float,
    max_suggestions: int
) -> List[PlatformSuggestion]:
    """Get fuzzy matching suggestions for platform names."""

    # Query all active platforms
    platforms = session.exec(
        select(Platform).where(Platform.is_active)
    ).all()

    suggestions: List[Tuple[float, PlatformSuggestion]] = []

    for platform in platforms:
        # Calculate confidence for platform name
        name_confidence = calculate_fuzzy_confidence(unknown_name, platform.name)
        display_confidence = calculate_fuzzy_confidence(unknown_name, platform.display_name)

        # Use the higher confidence score
        confidence = max(name_confidence, display_confidence)

        if confidence >= min_confidence:
            # Determine match type
            if confidence >= 0.95:
                match_type = "exact"
                reason = "Exact or near-exact match"
            elif confidence >= 0.8:
                match_type = "fuzzy"
                reason = "Strong similarity match"
            else:
                match_type = "partial"
                reason = "Partial similarity match"

            suggestion = PlatformSuggestion(
                platform_id=platform.id,
                platform_name=platform.name,
                platform_display_name=platform.display_name,
                confidence=confidence,
                match_type=match_type,
                reason=reason
            )
            suggestions.append((confidence, suggestion))

    # Sort by confidence (descending) and limit results
    suggestions.sort(key=lambda x: x[0], reverse=True)
    return [suggestion for _, suggestion in suggestions[:max_suggestions]]


async def get_storefront_suggestions(
    session: Session,
    unknown_name: str,
    min_confidence: float,
    max_suggestions: int
) -> List[StorefrontSuggestion]:
    """Get fuzzy matching suggestions for storefront names."""

    # Query all active storefronts
    storefronts = session.exec(
        select(Storefront).where(Storefront.is_active)
    ).all()

    suggestions: List[Tuple[float, StorefrontSuggestion]] = []

    for storefront in storefronts:
        # Calculate confidence for storefront name
        name_confidence = calculate_fuzzy_confidence(unknown_name, storefront.name)
        display_confidence = calculate_fuzzy_confidence(unknown_name, storefront.display_name)

        # Use the higher confidence score
        confidence = max(name_confidence, display_confidence)

        if confidence >= min_confidence:
            # Determine match type
            if confidence >= 0.95:
                match_type = "exact"
                reason = "Exact or near-exact match"
            elif confidence >= 0.8:
                match_type = "fuzzy"
                reason = "Strong similarity match"
            else:
                match_type = "partial"
                reason = "Partial similarity match"

            suggestion = StorefrontSuggestion(
                storefront_id=storefront.id,
                storefront_name=storefront.name,
                storefront_display_name=storefront.display_name,
                confidence=confidence,
                match_type=match_type,
                reason=reason
            )
            suggestions.append((confidence, suggestion))

    # Sort by confidence (descending) and limit results
    suggestions.sort(key=lambda x: x[0], reverse=True)
    return [suggestion for _, suggestion in suggestions[:max_suggestions]]


async def get_storefronts_for_platform(session: Session, platform_id: str) -> List[Storefront]:
    """
    Get all valid storefronts for a specific platform.

    Args:
        session: Database session
        platform_id: ID of the platform

    Returns:
        List of storefronts associated with the platform
    """
    storefronts = session.exec(
        select(Storefront)
        .join(PlatformStorefront, Storefront.id == PlatformStorefront.storefront_id)  # type: ignore[arg-type]
        .where(PlatformStorefront.platform_id == platform_id)
        .where(Storefront.is_active)
        .order_by(Storefront.display_name)
    ).all()

    return list(storefronts)


async def suggest_storefront_matches_for_platform(
    session: Session,
    unknown_storefront_name: str,
    platform_id: str,
    min_confidence: float = 0.6,
    max_suggestions: int = 5
) -> List[StorefrontSuggestion]:
    """
    Get platform-contextual storefront suggestions.

    Args:
        session: Database session
        unknown_storefront_name: The unknown storefront name
        platform_id: Platform context for suggestions
        min_confidence: Minimum confidence threshold
        max_suggestions: Maximum number of suggestions

    Returns:
        List of StorefrontSuggestion objects
    """
    clean_name = sanitize_platform_name(unknown_storefront_name)
    if not clean_name:
        return []

    # First, try to get platform-specific storefronts
    platform_storefronts = await get_storefronts_for_platform(session, platform_id)

    suggestions: List[Tuple[float, StorefrontSuggestion]] = []

    # Calculate confidence for platform-specific storefronts (higher priority)
    for storefront in platform_storefronts:
        name_confidence = calculate_fuzzy_confidence(clean_name, storefront.name)
        display_confidence = calculate_fuzzy_confidence(clean_name, storefront.display_name)
        confidence = max(name_confidence, display_confidence)

        if confidence >= min_confidence:
            # Boost confidence for platform-compatible storefronts
            boosted_confidence = min(1.0, confidence * 1.1)

            match_type = "exact" if boosted_confidence >= 0.95 else "fuzzy" if boosted_confidence >= 0.8 else "partial"
            reason = f"Compatible with platform - {match_type} match"

            suggestion = StorefrontSuggestion(
                storefront_id=storefront.id,
                storefront_name=storefront.name,
                storefront_display_name=storefront.display_name,
                confidence=boosted_confidence,
                match_type=match_type,
                reason=reason
            )
            suggestions.append((boosted_confidence, suggestion))

    # If we don't have enough platform-specific suggestions, add general suggestions
    if len(suggestions) < max_suggestions:
        general_suggestions = await get_storefront_suggestions(
            session, clean_name, min_confidence, max_suggestions * 2
        )

        # Filter out already suggested storefronts
        suggested_ids = {s[1].storefront_id for s in suggestions}

        for general_suggestion in general_suggestions:
            if general_suggestion.storefront_id not in suggested_ids:
                # Lower confidence for non-platform-specific suggestions
                adjusted_confidence = general_suggestion.confidence * 0.9
                general_suggestion.confidence = adjusted_confidence
                general_suggestion.reason = "General match - may require platform verification"
                suggestions.append((adjusted_confidence, general_suggestion))

                if len(suggestions) >= max_suggestions:
                    break

    # Sort by confidence and return
    suggestions.sort(key=lambda x: x[0], reverse=True)
    return [suggestion for _, suggestion in suggestions[:max_suggestions]]
