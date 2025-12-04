"""
Platform resolution service module.

Main facade for all platform resolution operations.
"""

import logging
from typing import List, Optional, Dict, Any, Tuple

from sqlmodel import Session, select

from app.models.platform import Platform, Storefront
from app.schemas.platform import (
    PlatformSuggestionsResponse,
    PendingPlatformResolution,
    PlatformResolutionResult,
    BulkPlatformResolutionResponse,
    StorefrontSuggestion
)
from app.utils.fuzzy_match import calculate_fuzzy_confidence

from .models import (
    EXPLICIT_PLATFORM_MAPPINGS,
    EXPLICIT_STOREFRONT_MAPPINGS,
    sanitize_platform_name
)
from .suggestions import (
    get_platform_suggestions,
    get_storefront_suggestions,
    get_storefronts_for_platform
)
from .resolution_ops import (
    get_pending_resolutions,
    resolve_platform,
    bulk_resolve_platforms,
    populate_platform_suggestions,
    resolve_storefront,
    detect_unknown_storefronts,
    validate_platform_storefront_compatibility,
    auto_resolve_high_confidence_storefronts
)


logger = logging.getLogger(__name__)


class PlatformResolutionService:
    """Service for handling platform resolution during CSV imports."""

    def __init__(self, session: Session):
        self.session = session

    def sanitize_platform_name(self, name: str) -> str:
        """
        Sanitize platform name input to prevent injection attacks.
        Based on validate_platform_name from the spec but for suggestions only.
        """
        return sanitize_platform_name(name)

    # Expose constants as class attributes for backwards compatibility
    EXPLICIT_PLATFORM_MAPPINGS = EXPLICIT_PLATFORM_MAPPINGS
    EXPLICIT_STOREFRONT_MAPPINGS = EXPLICIT_STOREFRONT_MAPPINGS

    async def get_canonical_platform(self, platform_name: str) -> Optional[Platform]:
        """
        Get the canonical platform object for a given platform name using fuzzy matching and minimal explicit mappings.

        Args:
            platform_name: Platform name from CSV or other source

        Returns:
            Platform object if found, None if not found
        """
        if not platform_name:
            return None

        # Sanitize the input
        clean_name = self.sanitize_platform_name(platform_name)
        if not clean_name:
            return None

        # First try direct database lookup by display name (exact match)
        platform = self.session.exec(
            select(Platform).where(
                Platform.display_name == clean_name,
                Platform.is_active
            )
        ).first()

        if platform:
            return platform

        # Try explicit mapping for known edge cases
        explicit_mapping = self.EXPLICIT_PLATFORM_MAPPINGS.get(clean_name)
        if explicit_mapping:
            platform = self.session.exec(
                select(Platform).where(
                    Platform.display_name == explicit_mapping,
                    Platform.is_active
                )
            ).first()
            if platform:
                return platform

        # Use fuzzy matching to find best match
        suggestions = await get_platform_suggestions(
            self.session,
            clean_name,
            min_confidence=0.8,  # High confidence for automatic resolution
            max_suggestions=1
        )

        if suggestions and len(suggestions) > 0:
            best_match = suggestions[0]
            # Get the platform by ID from the suggestion
            platform = self.session.get(Platform, best_match.platform_id)
            if platform and platform.is_active:
                return platform

        # Platform not found
        return None

    async def get_canonical_storefront(self, storefront_name: str, platform: Optional[Platform] = None) -> Optional[Storefront]:
        """
        Get the canonical storefront object for a given storefront name using fuzzy matching and minimal explicit mappings.

        Args:
            storefront_name: Storefront name from CSV or other source
            platform: Optional platform for context-specific resolution

        Returns:
            Storefront object if found, None if not found
        """
        if not storefront_name:
            return None

        # Sanitize the input
        clean_name = self.sanitize_platform_name(storefront_name)
        if not clean_name:
            return None

        # First try direct database lookup by display name (exact match)
        storefront = self.session.exec(
            select(Storefront).where(
                Storefront.display_name == clean_name,
                Storefront.is_active
            )
        ).first()

        if storefront:
            return storefront

        # Try explicit mapping for known edge cases
        explicit_mapping = self.EXPLICIT_STOREFRONT_MAPPINGS.get(clean_name)
        if explicit_mapping:
            storefront = self.session.exec(
                select(Storefront).where(
                    Storefront.display_name == explicit_mapping,
                    Storefront.is_active
                )
            ).first()
            if storefront:
                return storefront

        # Use fuzzy matching to find best match
        suggestions = await get_storefront_suggestions(
            self.session,
            clean_name,
            min_confidence=0.8,  # High confidence for automatic resolution
            max_suggestions=1
        )

        if suggestions and len(suggestions) > 0:
            best_match = suggestions[0]
            # Get the storefront by ID from the suggestion
            storefront = self.session.get(Storefront, best_match.storefront_id)
            if storefront and storefront.is_active:
                return storefront

        # Storefront not found
        return None

    async def suggest_platform_matches(
        self,
        unknown_platform_name: str,
        unknown_storefront_name: Optional[str] = None,
        min_confidence: float = 0.6,
        max_suggestions: int = 5
    ) -> PlatformSuggestionsResponse:
        """
        Get fuzzy matching suggestions for unknown platform/storefront names.

        Args:
            unknown_platform_name: The unknown platform name to find matches for
            unknown_storefront_name: Optional unknown storefront name
            min_confidence: Minimum confidence threshold (0.0 to 1.0)
            max_suggestions: Maximum number of suggestions to return

        Returns:
            PlatformSuggestionsResponse with suggestions
        """
        # Sanitize inputs
        clean_platform_name = self.sanitize_platform_name(unknown_platform_name)
        clean_storefront_name = self.sanitize_platform_name(unknown_storefront_name) if unknown_storefront_name else None

        if not clean_platform_name:
            return PlatformSuggestionsResponse(
                unknown_platform_name=unknown_platform_name,
                unknown_storefront_name=unknown_storefront_name,
                platform_suggestions=[],
                storefront_suggestions=[],
                total_platform_suggestions=0,
                total_storefront_suggestions=0
            )

        # Get platform suggestions
        platform_suggestions = await get_platform_suggestions(
            self.session, clean_platform_name, min_confidence, max_suggestions
        )

        # Get storefront suggestions if storefront name provided
        storefront_suggestions = []
        if clean_storefront_name:
            storefront_suggestions = await get_storefront_suggestions(
                self.session, clean_storefront_name, min_confidence, max_suggestions
            )

        return PlatformSuggestionsResponse(
            unknown_platform_name=unknown_platform_name,
            unknown_storefront_name=unknown_storefront_name,
            platform_suggestions=platform_suggestions,
            storefront_suggestions=storefront_suggestions,
            total_platform_suggestions=len(platform_suggestions),
            total_storefront_suggestions=len(storefront_suggestions)
        )

    # Delegate to module functions
    async def _get_platform_suggestions(
        self,
        unknown_name: str,
        min_confidence: float,
        max_suggestions: int
    ):
        """Get fuzzy matching suggestions for platform names."""
        return await get_platform_suggestions(
            self.session, unknown_name, min_confidence, max_suggestions
        )

    async def _get_storefront_suggestions(
        self,
        unknown_name: str,
        min_confidence: float,
        max_suggestions: int
    ):
        """Get fuzzy matching suggestions for storefront names."""
        return await get_storefront_suggestions(
            self.session, unknown_name, min_confidence, max_suggestions
        )

    async def get_pending_resolutions(
        self,
        user_id: str,
        page: int = 1,
        per_page: int = 20
    ) -> Tuple[List[PendingPlatformResolution], int]:
        """
        Get pending platform and storefront resolutions for a user.

        Returns:
            Tuple of (pending_resolutions, total_count)
        """
        return await get_pending_resolutions(self.session, user_id, page, per_page)

    async def resolve_platform(
        self,
        import_id: str,
        user_id: str,
        resolved_platform_id: Optional[str] = None,
        resolved_storefront_id: Optional[str] = None,
        user_notes: Optional[str] = None
    ) -> PlatformResolutionResult:
        """
        Resolve a platform for a specific import record.

        Args:
            import_id: ID of the DarkadiaImport record
            user_id: ID of the user (for security)
            resolved_platform_id: Optional ID of platform to resolve to
            resolved_storefront_id: Optional ID of storefront to resolve to
            user_notes: Optional user notes

        Returns:
            PlatformResolutionResult with resolution details
        """
        return await resolve_platform(
            self.session, import_id, user_id,
            resolved_platform_id, resolved_storefront_id, user_notes
        )

    async def bulk_resolve_platforms(
        self,
        resolutions: List[Dict[str, Any]],
        user_id: str
    ) -> BulkPlatformResolutionResponse:
        """
        Resolve multiple platforms in a bulk operation.

        Args:
            resolutions: List of resolution requests
            user_id: ID of the user (for security)

        Returns:
            BulkPlatformResolutionResponse with results
        """
        return await bulk_resolve_platforms(self.session, resolutions, user_id)

    async def populate_platform_suggestions(
        self,
        import_id: str,
        user_id: str,
        min_confidence: float = 0.6
    ) -> bool:
        """
        Populate platform suggestions for an import record.

        Args:
            import_id: ID of the DarkadiaImport record
            user_id: ID of the user (for security)
            min_confidence: Minimum confidence for suggestions

        Returns:
            True if suggestions were populated successfully
        """
        return await populate_platform_suggestions(
            self.session, import_id, user_id, min_confidence
        )

    async def get_storefronts_for_platform(self, platform_id: str) -> List[Storefront]:
        """
        Get all valid storefronts for a specific platform.

        Args:
            platform_id: ID of the platform

        Returns:
            List of storefronts associated with the platform
        """
        return await get_storefronts_for_platform(self.session, platform_id)

    async def suggest_storefront_matches_for_platform(
        self,
        unknown_storefront_name: str,
        platform_id: str,
        min_confidence: float = 0.6,
        max_suggestions: int = 5
    ) -> List[StorefrontSuggestion]:
        """
        Get platform-contextual storefront suggestions.

        Args:
            unknown_storefront_name: The unknown storefront name
            platform_id: Platform context for suggestions
            min_confidence: Minimum confidence threshold
            max_suggestions: Maximum number of suggestions

        Returns:
            List of StorefrontSuggestion objects
        """
        # Implement locally to use self methods for testability
        clean_name = self.sanitize_platform_name(unknown_storefront_name)
        if not clean_name:
            return []

        # First, try to get platform-specific storefronts (uses self for testability)
        platform_storefronts = await self.get_storefronts_for_platform(platform_id)

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
            general_suggestions = await self._get_storefront_suggestions(
                clean_name, min_confidence, max_suggestions * 2
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

    async def validate_platform_storefront_compatibility(
        self,
        platform_id: str,
        storefront_id: str
    ) -> bool:
        """
        Check if a platform-storefront combination is valid.

        Args:
            platform_id: ID of the platform
            storefront_id: ID of the storefront

        Returns:
            True if compatible, False otherwise
        """
        return await validate_platform_storefront_compatibility(
            self.session, platform_id, storefront_id
        )

    async def detect_unknown_storefronts(
        self,
        user_id: str,
        batch_id: Optional[str] = None
    ) -> List[str]:
        """
        Detect unknown storefronts in user's imports.

        Args:
            user_id: ID of the user
            batch_id: Optional batch ID to limit search

        Returns:
            List of unknown storefront names
        """
        return await detect_unknown_storefronts(self.session, user_id, batch_id)

    async def resolve_storefront(
        self,
        import_id: str,
        user_id: str,
        resolved_storefront_id: str,
        user_notes: Optional[str] = None
    ) -> bool:
        """
        Resolve a storefront for a specific import record.

        Args:
            import_id: ID of the DarkadiaImport record
            user_id: ID of the user (for security)
            resolved_storefront_id: ID of storefront to resolve to
            user_notes: Optional user notes

        Returns:
            True if resolution was successful
        """
        return await resolve_storefront(
            self.session, import_id, user_id, resolved_storefront_id, user_notes
        )

    async def bulk_resolve_storefronts(
        self,
        resolutions: List[Dict[str, Any]],
        user_id: str
    ) -> Dict[str, Any]:
        """
        Resolve multiple storefronts in a bulk operation.

        Args:
            resolutions: List of {import_id, storefront_id, user_notes} dictionaries
            user_id: ID of the user

        Returns:
            Dictionary with success count, failed count, and errors
        """
        # Implement locally to use self.resolve_storefront for testability
        successful_count = 0
        failed_count = 0
        errors = []

        for resolution in resolutions:
            try:
                import_id = resolution.get("import_id")
                storefront_id = resolution.get("storefront_id")
                if not import_id or not storefront_id:
                    raise ValueError("Missing import_id or storefront_id in resolution data")
                success = await self.resolve_storefront(
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

    async def auto_resolve_high_confidence_storefronts(
        self,
        pending_resolutions: List[Any],
        confidence_threshold: float = 0.95
    ) -> List[Any]:
        """
        Automatically resolve storefronts with high confidence matches.

        Args:
            pending_resolutions: List of PendingStorefrontResolution objects
            confidence_threshold: Minimum confidence to auto-resolve (default 0.95)

        Returns:
            Updated list with auto-resolved items marked appropriately
        """
        return await auto_resolve_high_confidence_storefronts(
            self.session, pending_resolutions, confidence_threshold
        )


def create_platform_resolution_service(session: Session) -> PlatformResolutionService:
    """Factory function to create PlatformResolutionService."""
    return PlatformResolutionService(session)
