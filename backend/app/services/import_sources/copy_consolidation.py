"""
Copy Consolidation System for Darkadia CSV Import

This module handles the consolidation of multiple CSV rows that represent
different copies of the same game, ensuring proper data transformation
and preventing duplicate game entries.
"""

import logging
import hashlib
import pandas as pd
from dataclasses import dataclass, field
from typing import Dict, List, Any, Optional
from datetime import datetime
from collections import defaultdict

from ...utils.data_extraction import safe_extract_string, safe_extract_date_string, safe_extract_numeric
from ...models.platform import Platform

logger = logging.getLogger(__name__)


@dataclass
class CopyData:
    """Represents a single copy of a game with its platform/storefront information."""
    
    platform: Optional[str]
    storefront: Optional[str] 
    storefront_other: Optional[str]
    media: str
    copy_identifier: str
    csv_row_number: int
    metadata: Dict[str, Any] = field(default_factory=dict)
    
    # Copy-specific fields
    label: str = ""
    release: str = ""
    purchase_date: Optional[str] = None
    
    # Physical copy metadata
    box: str = ""
    box_condition: str = ""
    box_notes: str = ""
    manual: str = ""
    manual_condition: str = ""
    manual_notes: str = ""
    complete: str = ""
    complete_notes: str = ""
    
    # Resolution flags
    is_real_copy: bool = True
    requires_storefront_resolution: bool = False


@dataclass
class ConsolidatedGame:
    """Represents a game with all its copies consolidated from multiple CSV rows."""
    
    name: str
    base_data: Dict[str, Any]  # Merged non-copy fields (ratings, notes, flags, etc.)
    copies: List[CopyData]     # All copies of this game
    csv_row_numbers: List[int] # Original CSV row numbers for tracking
    
    def has_real_copies(self) -> bool:
        """Check if game has any real copies (not just fallback platform data)."""
        return any(copy.is_real_copy for copy in self.copies)
    
    def get_copy_count(self) -> int:
        """Get total number of copies."""
        return len(self.copies)


class CopyConsolidationProcessor:
    """Processor for consolidating multiple CSV rows representing copies of the same game."""
    
    def __init__(self, platform_resolution_service: Optional[Any] = None):
        self.processed_games = 0
        self.total_copies = 0
        self.consolidated_games = 0
        self.platform_resolution_service = platform_resolution_service
    
    async def consolidate_games(self, csv_data: List[Dict[str, Any]]) -> List[ConsolidatedGame]:
        """
        Consolidate CSV rows by grouping same games and merging their copy data.
        
        Args:
            csv_data: List of dictionaries representing CSV rows
            
        Returns:
            List of ConsolidatedGame objects with merged copy data
        """
        if not csv_data:
            return []
        
        logger.info(f"Starting copy consolidation for {len(csv_data)} rows")
        
        # Group rows by exact game name
        game_groups = self._group_by_exact_name(csv_data)
        
        # Process each group
        consolidated_games = []
        for game_name, rows in game_groups.items():
            try:
                consolidated_game = await self._merge_game_copies(game_name, rows)
                consolidated_games.append(consolidated_game)
                
                if len(rows) > 1:
                    self.consolidated_games += 1
                    logger.debug(f"Consolidated {len(rows)} rows for game: {game_name}")
                
            except Exception as e:
                logger.error(f"Error consolidating game '{game_name}': {str(e)}")
                # Skip this game rather than failing entire import
                continue
        
        self.processed_games = len(consolidated_games)
        self.total_copies = sum(game.get_copy_count() for game in consolidated_games)
        
        logger.info(f"Consolidation complete: {self.processed_games} games, "
                   f"{self.total_copies} total copies, {self.consolidated_games} games merged")
        
        return consolidated_games
    
    def _group_by_exact_name(self, csv_data: List[Dict[str, Any]]) -> Dict[str, List[Dict[str, Any]]]:
        """
        Group CSV rows by exact game name match.
        
        Args:
            csv_data: List of CSV row dictionaries
            
        Returns:
            Dictionary mapping game names to lists of rows
        """
        game_groups = defaultdict(list)
        
        for row in csv_data:
            game_name = safe_extract_string(row.get('Name'))
            if not game_name:
                logger.warning(f"Skipping row {row.get('_csv_row_number', '?')} with empty game name")
                continue
            
            game_groups[game_name].append(row)
        
        return dict(game_groups)
    
    async def _merge_game_copies(self, game_name: str, rows: List[Dict[str, Any]]) -> ConsolidatedGame:
        """
        Merge multiple rows of the same game into a ConsolidatedGame.
        
        Args:
            game_name: Name of the game
            rows: List of CSV rows for this game
            
        Returns:
            ConsolidatedGame with merged data
        """
        if not rows:
            raise ValueError(f"No rows provided for game: {game_name}")
        
        # Extract and merge base data (non-copy fields)
        base_data = self._merge_base_data(rows)
        
        # Extract copy data from each row
        copies = []
        csv_row_numbers = []
        
        for row in rows:
            csv_row_number = row.get('_csv_row_number', 0)
            csv_row_numbers.append(csv_row_number)
            
            # Extract copy-specific data
            copy_data_list = self._extract_copy_data(row, csv_row_number)
            copies.extend(copy_data_list)
        
        # Enhanced platform processing: Handle both real copies and uncovered platforms from Platforms field
        additional_platform_copies = await self._create_additional_platform_copies(copies, rows[0])
        copies.extend(additional_platform_copies)
        
        return ConsolidatedGame(
            name=game_name,
            base_data=base_data,
            copies=copies,
            csv_row_numbers=csv_row_numbers
        )
    
    def _merge_base_data(self, rows: List[Dict[str, Any]]) -> Dict[str, Any]:
        """
        Merge non-copy fields from multiple rows using priority rules.
        
        Args:
            rows: List of CSV rows for the same game
            
        Returns:
            Dictionary with merged base data
        """
        if not rows:
            return {}
        
        # Start with first row as base
        merged_data = rows[0].copy()
        
        # Remove CSV-specific metadata that shouldn't be merged
        merged_data.pop('_csv_row_number', None)
        
        # Apply merging rules for conflicting data
        
        # Rating: Take highest value
        ratings = [row.get('Rating') for row in rows 
                  if row.get('Rating') is not None and safe_extract_string(row.get('Rating'))]
        if ratings:
            try:
                numeric_ratings = [float(r) for r in ratings if r is not None and str(r).replace('.', '').isdigit()]
                if numeric_ratings:
                    merged_data['Rating'] = float(max(numeric_ratings))  # Ensure it's a Python float
            except (ValueError, TypeError):
                pass  # Keep original value
        
        # Dates: Take most recent (Added date)
        dates = []
        for row in rows:
            date_str = safe_extract_date_string(row.get('Added'))
            if date_str and date_str != 'nan':
                try:
                    # Try to parse date for comparison
                    if isinstance(date_str, str):
                        # Handle various date formats
                        for date_format in ['%Y-%m-%d', '%m/%d/%Y', '%d/%m/%Y']:
                            try:
                                parsed_date = datetime.strptime(date_str, date_format)
                                dates.append((parsed_date, date_str))
                                break
                            except ValueError:
                                continue
                except Exception:
                    pass  # Skip invalid dates
        
        if dates:
            # Use the most recent date string
            most_recent_date = max(dates, key=lambda x: x[0])[1]
            merged_data['Added'] = most_recent_date
        
        # Ensure Added field is always a string, not a Timestamp
        if 'Added' in merged_data:
            merged_data['Added'] = safe_extract_date_string(merged_data['Added'])
        
        # Boolean fields: OR logic (true if any row is true)
        boolean_fields = ['Loved', 'Owned', 'Played', 'Playing', 'Finished', 
                         'Mastered', 'Dominated', 'Shelved']
        
        for field_name in boolean_fields:
            values = []
            for row in rows:
                value = row.get(field_name, False)
                # Handle various boolean representations
                if isinstance(value, bool):
                    values.append(value)
                elif isinstance(value, (int, float)):
                    values.append(bool(value))
                elif isinstance(value, str):
                    values.append(value.lower() in ['true', '1', 'yes', 'y'])
                else:
                    values.append(False)
            
            merged_data[field_name] = any(values)
        
        # Notes: Concatenate unique non-empty values
        notes = []
        for row in rows:
            note = safe_extract_string(row.get('Notes'))
            if note and note not in notes and note != 'nan':
                notes.append(note)
        
        if notes:
            merged_data['Notes'] = ' | '.join(notes)
        elif not merged_data.get('Notes'):
            merged_data['Notes'] = ''
        
        # Final cleanup: ensure all values are JSON-serializable
        json_safe_data = {}
        for key, value in merged_data.items():
            # Check if this is a date-related field by name or type
            is_date_field = (
                'date' in key.lower() or 
                'added' in key.lower() or
                isinstance(value, (pd.Timestamp, datetime))
            )
            
            if is_date_field:
                # Date fields should be strings
                json_safe_data[key] = safe_extract_date_string(value)
            elif key == 'Rating':
                # Rating should be a Python float or None
                json_safe_data[key] = safe_extract_numeric(value)
            elif isinstance(value, pd.Timestamp) or pd.isna(value):
                # Any other pandas Timestamp or NA values should be converted to string
                json_safe_data[key] = safe_extract_string(value)
            else:
                # Keep other values as-is (they should already be JSON-serializable)
                json_safe_data[key] = value
        
        return json_safe_data
    
    def _extract_copy_data(self, row: Dict[str, Any], csv_row_number: int) -> List[CopyData]:
        """
        Extract copy-specific data from a CSV row following copy-based precedence rules.
        
        Args:
            row: CSV row dictionary
            csv_row_number: Original CSV row number
            
        Returns:
            List of CopyData objects (usually one, but can be multiple for fallback platforms)
        """
        copy_platform = safe_extract_string(row.get('Copy platform'))
        copy_source = safe_extract_string(row.get('Copy source'))
        copy_source_other = safe_extract_string(row.get('Copy source other'))
        
        # Rule 1: Has copy data (platform or storefront) - this is a real copy
        if copy_platform or copy_source or copy_source_other:
            
            # Determine final storefront (prefer Copy source over Copy source other)
            final_storefront = copy_source if copy_source else copy_source_other
            
            # Handle "Other" storefront case
            if copy_source == "Other" and copy_source_other:
                final_storefront = copy_source_other
            
            # Generate copy identifier
            copy_identifier = self._generate_copy_identifier(
                copy_platform, final_storefront, safe_extract_string(row.get('Copy media'))
            )
            
            # Determine if storefront resolution is required
            requires_storefront_resolution = bool(copy_platform and not final_storefront)
            
            copy_data = CopyData(
                platform=copy_platform if copy_platform else None,
                storefront=copy_source if copy_source and copy_source != "Other" else None,
                storefront_other=copy_source_other if copy_source_other else None,
                media=safe_extract_string(row.get('Copy media')),
                copy_identifier=copy_identifier,
                csv_row_number=csv_row_number,
                is_real_copy=True,
                requires_storefront_resolution=requires_storefront_resolution,
                
                # Copy-specific fields
                label=safe_extract_string(row.get('Copy label')),
                release=safe_extract_string(row.get('Copy Release')),
                purchase_date=safe_extract_date_string(row.get('Copy purchase date')),
                
                # Physical copy metadata
                box=safe_extract_string(row.get('Copy box')),
                box_condition=safe_extract_string(row.get('Copy box condition')),
                box_notes=safe_extract_string(row.get('Copy box notes')),
                manual=safe_extract_string(row.get('Copy manual')),
                manual_condition=safe_extract_string(row.get('Copy manual condition')),
                manual_notes=safe_extract_string(row.get('Copy manual notes')),
                complete=safe_extract_string(row.get('Copy complete')),
                complete_notes=safe_extract_string(row.get('Copy complete notes')),
            )
            
            return [copy_data]
        
        # Rule 2: No copy data - will be handled by fallback creation
        return []
    
    def _create_fallback_copies(self, row: Dict[str, Any]) -> List[CopyData]:
        """
        Create fallback copy data from generic Platforms field when no copy-specific data exists.
        
        Args:
            row: CSV row dictionary
            
        Returns:
            List of CopyData objects for fallback platforms
        """
        platforms_field = safe_extract_string(row.get('Platforms'))
        if not platforms_field:
            logger.warning(f"Row {row.get('_csv_row_number', '?')}: No platform data found for {safe_extract_string(row.get('Name'), 'Unknown Game')}")
            return []
        
        # Parse comma-separated platforms
        platform_names = [safe_extract_string(p) for p in platforms_field.split(',') if safe_extract_string(p)]
        
        fallback_copies = []
        for i, platform_name in enumerate(platform_names):
            copy_identifier = f"fallback:{platform_name}"
            
            fallback_copy = CopyData(
                platform=platform_name,
                storefront=None,
                storefront_other=None,
                media="Digital",  # Assume digital for fallback
                copy_identifier=copy_identifier,
                csv_row_number=row.get('_csv_row_number', 0),
                is_real_copy=False,
                requires_storefront_resolution=True  # Always true for fallback
            )
            
            fallback_copies.append(fallback_copy)
        
        return fallback_copies
    
    async def _create_additional_platform_copies(self, existing_copies: List[CopyData], row: Dict[str, Any]) -> List[CopyData]:
        """
        Create additional platform copies for uncovered platforms from the general Platforms field.
        
        This implements a hybrid approach:
        1. If no real copies exist, creates fallback copies from all platforms in Platforms field
        2. If real copies exist, only creates copies for platforms NOT covered by existing copies
        
        Args:
            existing_copies: List of already processed copies
            row: CSV row dictionary containing Platforms field
            
        Returns:
            List of additional CopyData objects for uncovered platforms
        """
        # Get platforms from the general Platforms field
        platforms_field = safe_extract_string(row.get('Platforms'))
        if not platforms_field:
            return []
        
        # Parse comma-separated platforms
        general_platforms = [safe_extract_string(p) for p in platforms_field.split(',') if safe_extract_string(p)]
        if not general_platforms:
            return []
        
        # Check if we have any real copies
        real_copies = [copy for copy in existing_copies if copy.is_real_copy]
        
        # If no real copies exist, create fallback copies from all general platforms
        if not real_copies:
            logger.debug(f"No real copies found for {safe_extract_string(row.get('Name'), 'Unknown Game')}, creating fallback copies from all platforms")
            return self._create_fallback_copies_from_platforms(general_platforms, row)
        
        # If real copies exist, find uncovered platforms using canonical platform resolution
        covered_platforms = await self._get_covered_platforms(real_copies)
        uncovered_platforms = await self._get_uncovered_platforms(general_platforms, covered_platforms)
        
        if uncovered_platforms:
            logger.debug(f"Found {len(uncovered_platforms)} uncovered platforms for {safe_extract_string(row.get('Name'), 'Unknown Game')}: {uncovered_platforms}")
            return self._create_fallback_copies_from_platforms(uncovered_platforms, row)
        
        return []
    
    async def _get_covered_platforms(self, real_copies: List[CopyData]) -> List[Platform]:
        """
        Get list of Platform objects that are already covered by real copies.
        
        Args:
            real_copies: List of real copy data objects
            
        Returns:
            List of Platform objects already covered
        """
        covered_platforms = []
        
        for copy in real_copies:
            if copy.platform:
                # Resolve the platform name to canonical Platform object
                platform = await self._resolve_platform_for_comparison(copy.platform)
                if platform and platform not in covered_platforms:
                    covered_platforms.append(platform)
        
        return covered_platforms
    
    async def _get_uncovered_platforms(self, general_platforms: List[str], covered_platforms: List[Platform]) -> List[str]:
        """
        Find platforms from the general Platforms field that are not covered by existing copies.
        
        Args:
            general_platforms: Platform names from the Platforms field
            covered_platforms: Platform objects already covered by copies
            
        Returns:
            List of platform names that need additional copies
        """
        uncovered_platforms = []
        
        for platform_name in general_platforms:
            # Resolve the platform name to canonical Platform object
            platform = await self._resolve_platform_for_comparison(platform_name)
            # Only consider it uncovered if we could resolve it and it's not already covered
            if platform and platform not in covered_platforms:
                uncovered_platforms.append(platform_name)
        
        return uncovered_platforms
    
    async def _resolve_platform_for_comparison(self, platform_name: str) -> Optional[Platform]:
        """
        Resolve platform name to canonical Platform object for comparison purposes.
        
        Args:
            platform_name: Original platform name
            
        Returns:
            Platform object if resolved, None if not found
        """
        if not platform_name or not self.platform_resolution_service:
            return None
        
        return await self.platform_resolution_service.get_canonical_platform(platform_name)
    
    def _create_fallback_copies_from_platforms(self, platform_names: List[str], row: Dict[str, Any]) -> List[CopyData]:
        """
        Create fallback copies from a list of platform names.
        
        Args:
            platform_names: List of platform names to create copies for
            row: CSV row dictionary for metadata
            
        Returns:
            List of CopyData objects for the platforms
        """
        fallback_copies = []
        csv_row_number = row.get('_csv_row_number', 0)
        
        for i, platform_name in enumerate(platform_names):
            copy_identifier = f"fallback:{platform_name}"
            
            fallback_copy = CopyData(
                platform=platform_name,
                storefront=None,
                storefront_other=None,
                media="Digital",  # Assume digital for fallback
                copy_identifier=copy_identifier,
                csv_row_number=csv_row_number,
                is_real_copy=False,
                requires_storefront_resolution=True  # Always true for fallback
            )
            
            fallback_copies.append(fallback_copy)
        
        return fallback_copies

    def _generate_copy_identifier(self, platform: str, storefront: str, media: str) -> str:
        """
        Generate a unique identifier for a copy.
        
        Args:
            platform: Platform name
            storefront: Storefront name
            media: Media type
            
        Returns:
            Unique copy identifier string
        """
        # Create a consistent identifier from the key components
        components = []
        
        if platform:
            components.append(f"plt:{platform}")
        if storefront:
            components.append(f"str:{storefront}")
        if media:
            components.append(f"med:{media}")
        
        if not components:
            components.append("unknown")
        
        # Create base identifier
        base_id = "|".join(components)
        
        # Add hash for uniqueness while keeping it readable
        hash_input = f"{platform}|{storefront}|{media}".encode('utf-8')
        hash_suffix = hashlib.md5(hash_input).hexdigest()[:8]
        
        return f"{base_id}#{hash_suffix}"
    
    def get_consolidation_stats(self) -> Dict[str, Any]:
        """Get statistics about the consolidation process."""
        return {
            'processed_games': self.processed_games,
            'total_copies': self.total_copies,
            'consolidated_games': self.consolidated_games,
            'average_copies_per_game': round(self.total_copies / self.processed_games, 2) if self.processed_games > 0 else 0
        }