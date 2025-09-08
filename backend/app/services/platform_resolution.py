"""
Platform Resolution Service

Handles platform and storefront resolution for CSV imports, providing fuzzy matching
suggestions and resolution tracking for unknown platforms.
"""

import logging
from typing import List, Optional, Dict, Any, Tuple
from datetime import datetime, timezone
from sqlmodel import Session, select, and_, or_
import re

from ..models.platform import Platform, Storefront, PlatformStorefront
from ..models.darkadia_import import DarkadiaImport
from ..utils.fuzzy_match import calculate_fuzzy_confidence
from ..api.schemas.platform import (
    PlatformSuggestion,
    StorefrontSuggestion,
    PlatformResolutionData,
    PendingPlatformResolution,
    PlatformSuggestionsResponse,
    PlatformResolutionResult,
    BulkPlatformResolutionResponse
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
        if not name:
            return ""
        
        # Strip whitespace and limit length
        sanitized = str(name).strip()[:200]
        
        # Remove null bytes and control characters except basic whitespace
        sanitized = ''.join(char for char in sanitized if ord(char) >= 32 or char in '\n\r\t')
        
        # Remove potential script injection patterns
        script_patterns = [
            re.compile(r'<script.*?</script>', re.IGNORECASE | re.DOTALL),
            re.compile(r'javascript:', re.IGNORECASE),
            re.compile(r'vbscript:', re.IGNORECASE),
        ]
        
        for pattern in script_patterns:
            sanitized = pattern.sub('', sanitized)
        
        return sanitized
    
    # Minimal explicit mappings for cases where fuzzy matching fails
    EXPLICIT_PLATFORM_MAPPINGS = {
        # Short forms that are too different for fuzzy matching
        'PC': 'PC (Windows)',
        'PS3': 'PlayStation 3',
        'PS4': 'PlayStation 4', 
        'PS5': 'PlayStation 5',
        
        # Special cases with very different names
        'PlayStation Network (PS3)': 'PlayStation 3',
        'Xbox 360 Games Store': 'Xbox 360',
    }
    
    # Minimal explicit storefront mappings
    EXPLICIT_STOREFRONT_MAPPINGS = {
        # Short forms and abbreviations  
        'PSN': 'PlayStation Store',
        'HB': 'Humble Bundle',
        'Epic': 'Epic Games Store',
        
        # Special cases
        'Other': 'Physical',
        'Sony Entertainment Network': 'PlayStation Store',
    }
    
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
                Platform.is_active == True
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
                    Platform.is_active == True
                )
            ).first()
            if platform:
                return platform
        
        # Use fuzzy matching to find best match
        suggestions = await self._get_platform_suggestions(
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
                Storefront.is_active == True
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
                    Storefront.is_active == True
                )
            ).first()
            if storefront:
                return storefront
        
        # Use fuzzy matching to find best match
        suggestions = await self._get_storefront_suggestions(
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
        platform_suggestions = await self._get_platform_suggestions(
            clean_platform_name, min_confidence, max_suggestions
        )
        
        # Get storefront suggestions if storefront name provided
        storefront_suggestions = []
        if clean_storefront_name:
            storefront_suggestions = await self._get_storefront_suggestions(
                clean_storefront_name, min_confidence, max_suggestions
            )
        
        return PlatformSuggestionsResponse(
            unknown_platform_name=unknown_platform_name,
            unknown_storefront_name=unknown_storefront_name,
            platform_suggestions=platform_suggestions,
            storefront_suggestions=storefront_suggestions,
            total_platform_suggestions=len(platform_suggestions),
            total_storefront_suggestions=len(storefront_suggestions)
        )
    
    async def _get_platform_suggestions(
        self,
        unknown_name: str,
        min_confidence: float,
        max_suggestions: int
    ) -> List[PlatformSuggestion]:
        """Get fuzzy matching suggestions for platform names."""
        
        # Query all active platforms
        platforms = self.session.exec(
            select(Platform).where(Platform.is_active)
        ).all()
        
        suggestions = []
        
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
    
    async def _get_storefront_suggestions(
        self,
        unknown_name: str,
        min_confidence: float,
        max_suggestions: int
    ) -> List[StorefrontSuggestion]:
        """Get fuzzy matching suggestions for storefront names."""
        
        # Query all active storefronts
        storefronts = self.session.exec(
            select(Storefront).where(Storefront.is_active)
        ).all()
        
        suggestions = []
        
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
        # Query DarkadiaImport records with unresolved platforms or storefronts
        query = select(DarkadiaImport).where(
            DarkadiaImport.user_id == user_id,
            and_(
                or_(
                    # Unresolved platform
                    and_(
                        not DarkadiaImport.platform_resolved,
                        DarkadiaImport.original_platform_name.isnot(None)
                    ),
                    # Unresolved storefront (has original storefront but no resolved storefront)
                    and_(
                        DarkadiaImport.original_storefront_name.isnot(None),
                        DarkadiaImport.resolved_storefront_id.is_(None)
                    )
                )
            )
        )
        
        # Get all import records
        all_imports = self.session.exec(query).all()
        
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
                self.session.add(representative)
                self.session.commit()
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
        # Get the import record
        import_record = self.session.get(DarkadiaImport, import_id)
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
                resolved_platform = self.session.get(Platform, resolved_platform_id)
                if not resolved_platform or not resolved_platform.is_active:
                    return PlatformResolutionResult(
                        import_id=import_id,
                        success=False,
                        error_message="Invalid or inactive platform"
                    )
            
            # Validate resolved storefront if provided
            resolved_storefront = None
            if resolved_storefront_id:
                resolved_storefront = self.session.get(Storefront, resolved_storefront_id)
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
            
            self.session.add(import_record)
            self.session.commit()
            
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
        results = []
        successful_count = 0
        failed_count = 0
        errors = []
        
        for resolution_data in resolutions:
            try:
                result = await self.resolve_platform(
                    import_id=resolution_data.get("import_id"),
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
        # Get the import record
        import_record = self.session.get(DarkadiaImport, import_id)
        if not import_record or import_record.user_id != user_id:
            return False
        
        if not import_record.original_platform_name:
            return False
        
        try:
            # Get suggestions
            suggestions_response = await self.suggest_platform_matches(
                unknown_platform_name=import_record.original_platform_name,
                min_confidence=min_confidence,
                max_suggestions=5
            )
            
            # Update resolution data with suggestions
            resolution_data = import_record.get_platform_resolution_data()
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
            
            self.session.add(import_record)
            self.session.commit()
            
            return True
            
        except Exception as e:
            logger.error(f"Error populating suggestions for import {import_id}: {str(e)}")
            return False

    async def get_storefronts_for_platform(self, platform_id: str) -> List[Storefront]:
        """
        Get all valid storefronts for a specific platform.
        
        Args:
            platform_id: ID of the platform
            
        Returns:
            List of storefronts associated with the platform
        """
        storefronts = self.session.exec(
            select(Storefront)
            .join(PlatformStorefront, Storefront.id == PlatformStorefront.storefront_id)
            .where(PlatformStorefront.platform_id == platform_id)
            .where(Storefront.is_active)
            .order_by(Storefront.display_name)
        ).all()
        
        return storefronts

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
        clean_name = self.sanitize_platform_name(unknown_storefront_name)
        if not clean_name:
            return []
        
        # First, try to get platform-specific storefronts
        platform_storefronts = await self.get_storefronts_for_platform(platform_id)
        
        suggestions = []
        
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
        association = self.session.exec(
            select(PlatformStorefront).where(
                and_(
                    PlatformStorefront.platform_id == platform_id,
                    PlatformStorefront.storefront_id == storefront_id
                )
            )
        ).first()
        
        return association is not None

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
        query = select(DarkadiaImport.original_storefront_name).where(
            and_(
                DarkadiaImport.user_id == user_id,
                DarkadiaImport.original_storefront_name.isnot(None),
                or_(
                    not DarkadiaImport.storefront_resolved,
                    DarkadiaImport.requires_storefront_resolution.is_(True)
                )
            )
        )
        
        if batch_id:
            query = query.where(DarkadiaImport.batch_id == batch_id)
        
        results = self.session.exec(query.distinct()).all()
        return [name for name in results if name and name.strip()]

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
        # Get the import record
        import_record = self.session.get(DarkadiaImport, import_id)
        if not import_record or import_record.user_id != user_id:
            return False
        
        # Validate storefront exists and is active
        storefront = self.session.get(Storefront, resolved_storefront_id)
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
            
            self.session.add(import_record)
            self.session.commit()
            
            logger.info(f"Resolved storefront for import {import_id}: storefront={resolved_storefront_id}")
            return True
            
        except Exception as e:
            logger.error(f"Error resolving storefront for import {import_id}: {str(e)}")
            self.session.rollback()
            return False

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
        successful_count = 0
        failed_count = 0
        errors = []
        
        for resolution in resolutions:
            try:
                success = await self.resolve_storefront(
                    import_id=resolution.get("import_id"),
                    user_id=user_id,
                    resolved_storefront_id=resolution.get("storefront_id"),
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
        updated_resolutions = []
        
        for resolution in pending_resolutions:
            try:
                # Get storefront suggestions for this resolution
                suggestions = await self.suggest_storefront_matches_for_platform(
                    unknown_storefront_name=resolution.original_storefront_name,
                    platform_id=resolution.platform_id if hasattr(resolution, 'platform_id') else None,
                    min_confidence=confidence_threshold,
                    max_suggestions=1  # We only need the best match
                )
                
                if suggestions and suggestions[0].confidence >= confidence_threshold:
                    best_suggestion = suggestions[0]
                    
                    # Auto-resolve this storefront
                    success = await self.resolve_storefront(
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
                    general_suggestions = await self._get_storefront_suggestions(
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


def create_platform_resolution_service(session: Session) -> PlatformResolutionService:
    """Factory function to create PlatformResolutionService."""
    return PlatformResolutionService(session)