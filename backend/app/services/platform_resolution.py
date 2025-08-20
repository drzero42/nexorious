"""
Platform Resolution Service

Handles platform and storefront resolution for CSV imports, providing fuzzy matching
suggestions and resolution tracking for unknown platforms.
"""

import logging
from typing import List, Optional, Dict, Any, Tuple
from datetime import datetime, timezone
from sqlmodel import Session, select
import json
import re

from ..models.platform import Platform, Storefront
from ..models.darkadia_import import DarkadiaImport
from ..models.user import User
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
            select(Platform).where(Platform.is_active == True)
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
            select(Storefront).where(Storefront.is_active == True)
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
        Get pending platform resolutions for a user.
        
        Returns:
            Tuple of (pending_resolutions, total_count)
        """
        # Query DarkadiaImport records with unresolved platforms
        query = select(DarkadiaImport).where(
            DarkadiaImport.user_id == user_id,
            DarkadiaImport.platform_resolved == False,
            DarkadiaImport.original_platform_name.isnot(None)
        )
        
        # Get total count
        all_imports = self.session.exec(query).all()
        
        # Group by original platform name to avoid duplicates
        platform_groups: Dict[str, List[DarkadiaImport]] = {}
        for import_record in all_imports:
            if import_record.original_platform_name:
                key = import_record.original_platform_name.lower().strip()
                if key not in platform_groups:
                    platform_groups[key] = []
                platform_groups[key].append(import_record)
        
        # Convert to pending resolutions
        pending_resolutions = []
        
        for platform_name, import_records in platform_groups.items():
            # Use the first import record as representative
            representative = import_records[0]
            
            # Get affected games
            affected_games = []
            for record in import_records:
                csv_data = record.get_original_csv_data()
                game_name = csv_data.get("Name", "Unknown Game")
                if game_name not in affected_games:
                    affected_games.append(game_name)
            
            # Get or create resolution data
            resolution_data = representative.get_platform_resolution_data()
            if not resolution_data or not resolution_data.get("status"):
                # Create initial resolution data
                resolution_data = {
                    "status": "pending",
                    "original_name": representative.original_platform_name,
                    "suggestions": [],
                    "storefront_suggestions": [],
                    "resolved_platform_id": None,
                    "resolved_storefront_id": None,
                    "resolution_timestamp": None,
                    "resolution_method": None,
                    "user_notes": None
                }
                representative.set_platform_resolution_data(resolution_data)
                self.session.add(representative)
                self.session.commit()
            
            pending_resolution = PendingPlatformResolution(
                import_id=representative.id,
                user_id=user_id,
                original_platform_name=representative.original_platform_name,
                original_storefront_name=None,  # TODO: Add storefront tracking
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


def create_platform_resolution_service(session: Session) -> PlatformResolutionService:
    """Factory function to create PlatformResolutionService."""
    return PlatformResolutionService(session)