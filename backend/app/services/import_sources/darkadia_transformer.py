"""
Enhanced Data Transformation Pipeline for Darkadia CSV Import

This module implements a multi-stage transformation pipeline with validation,
normalization, mapping, and persistence stages. Each stage handles specific
concerns with error recovery and comprehensive reporting.
"""

import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Dict, Any, List, Optional, Tuple
from datetime import datetime, timezone, date
from decimal import Decimal, InvalidOperation
import re

from ...models.user_game import PlayStatus
from ...security.csv_sanitizer import CSVSanitizer
from ...utils.fuzzy_match import calculate_fuzzy_confidence

logger = logging.getLogger(__name__)


@dataclass
class ValidationIssue:
    """Represents a validation issue found during transformation."""
    severity: str  # 'error', 'warning', 'info'
    field: str
    message: str
    original_value: Any
    corrected_value: Optional[Any] = None
    row_index: Optional[int] = None


@dataclass
class TransformationContext:
    """Context object to track transformation progress and issues."""
    total_rows: int = 0
    processed_rows: int = 0
    successful_rows: int = 0
    issues: List[ValidationIssue] = field(default_factory=list)
    unknown_platforms: set = field(default_factory=set)
    unknown_storefronts: set = field(default_factory=set)
    
    def add_issue(self, severity: str, field: str, message: str, 
                  original_value: Any, corrected_value: Any = None,
                  row_index: Optional[int] = None):
        """Add a validation issue to the context."""
        issue = ValidationIssue(
            severity=severity,
            field=field,
            message=message,
            original_value=original_value,
            corrected_value=corrected_value,
            row_index=row_index
        )
        self.issues.append(issue)
    
    def get_summary(self) -> Dict[str, Any]:
        """Get a summary of transformation results."""
        error_count = sum(1 for issue in self.issues if issue.severity == 'error')
        warning_count = sum(1 for issue in self.issues if issue.severity == 'warning')
        
        return {
            'total_rows': self.total_rows,
            'processed_rows': self.processed_rows,
            'successful_rows': self.successful_rows,
            'error_count': error_count,
            'warning_count': warning_count,
            'unknown_platforms': list(self.unknown_platforms),
            'unknown_storefronts': list(self.unknown_storefronts),
            'issues': [
                {
                    'severity': issue.severity,
                    'field': issue.field,
                    'message': issue.message,
                    'row_index': issue.row_index
                }
                for issue in self.issues
            ]
        }


class TransformationStage(ABC):
    """Base class for transformation pipeline stages."""
    
    @abstractmethod
    async def process(self, data: List[Dict[str, Any]], 
                     context: TransformationContext) -> List[Dict[str, Any]]:
        """Process the data through this stage."""
        pass
    
    @abstractmethod
    def get_stage_name(self) -> str:
        """Get the name of this stage for logging."""
        pass


class ValidationStage(TransformationStage):
    """Stage that validates and cleans input data."""
    
    # Date formats to try when parsing dates
    DATE_FORMATS = [
        '%Y-%m-%d',      # 2024-01-15
        '%m/%d/%Y',      # 01/15/2024
        '%d/%m/%Y',      # 15/01/2024
        '%Y-%m-%d %H:%M:%S',  # With time
        '%m/%d/%Y %H:%M:%S'
    ]
    
    # Valid boolean values
    BOOLEAN_TRUE_VALUES = {'1', 'true', 'True', 'TRUE', 'yes', 'Yes', 'YES', 'y', 'Y'}
    BOOLEAN_FALSE_VALUES = {'0', 'false', 'False', 'FALSE', 'no', 'No', 'NO', 'n', 'N', ''}
    
    def get_stage_name(self) -> str:
        return "Validation"
    
    async def process(self, data: List[Dict[str, Any]], 
                     context: TransformationContext) -> List[Dict[str, Any]]:
        """Validate and clean all data."""
        logger.info(f"Starting validation stage for {len(data)} rows")
        
        validated_data = []
        context.total_rows = len(data)
        
        for row_index, row in enumerate(data):
            try:
                # Sanitize all cell values first
                sanitized_row = {}
                for field, value in row.items():
                    sanitized_row[field] = CSVSanitizer.sanitize_cell(value)
                
                # Validate the row
                validated_row = await self._validate_row(sanitized_row, context, row_index)
                
                if validated_row is not None:
                    validated_data.append(validated_row)
                    context.successful_rows += 1
                
                context.processed_rows += 1
                
            except Exception as e:
                logger.error(f"Error validating row {row_index}: {e}")
                context.add_issue(
                    'error', 'row', f"Failed to validate row: {str(e)}", 
                    row, row_index=row_index
                )
        
        logger.info(f"Validation complete: {context.successful_rows}/{context.processed_rows} rows valid")
        return validated_data
    
    async def _validate_row(self, row: Dict[str, Any], 
                           context: TransformationContext, 
                           row_index: int) -> Optional[Dict[str, Any]]:
        """Validate a single row of data."""
        validated_row = row.copy()
        
        # Validate required fields
        name = validated_row.get('Name', '').strip()
        if not name:
            context.add_issue(
                'error', 'Name', 'Game name is required', 
                row.get('Name', ''), row_index=row_index
            )
            return None  # Skip rows without names
        
        # Validate and normalize boolean fields
        bool_fields = ['Loved', 'Owned', 'Played', 'Playing', 'Finished', 
                      'Mastered', 'Dominated', 'Shelved']
        
        for field in bool_fields:
            original_value = validated_row.get(field, '')
            validated_row[field] = self._validate_boolean(
                original_value, field, context, row_index
            )
        
        # Validate boolean flag combinations
        self._validate_flag_combinations(validated_row, context, row_index)
        
        # Validate rating
        validated_row['Rating'] = self._validate_rating(
            validated_row.get('Rating', ''), context, row_index
        )
        
        # Validate dates
        validated_row['Added'] = self._validate_date(
            validated_row.get('Added', ''), 'Added', context, row_index
        )
        validated_row['Copy purchase date'] = self._validate_date(
            validated_row.get('Copy purchase date', ''), 'Copy purchase date', 
            context, row_index
        )
        
        return validated_row
    
    def _validate_boolean(self, value: Any, field_name: str, 
                         context: TransformationContext, 
                         row_index: int) -> bool:
        """Validate and normalize boolean values."""
        if value is None or value == '':
            return False
        
        str_value = str(value).strip()
        
        if str_value in self.BOOLEAN_TRUE_VALUES:
            return True
        elif str_value in self.BOOLEAN_FALSE_VALUES:
            return False
        else:
            # Try to convert as number
            try:
                num_value = float(str_value)
                result = bool(num_value)
                if num_value not in [0, 1]:
                    context.add_issue(
                        'warning', field_name, 
                        f"Unusual boolean value converted: {str_value} -> {result}",
                        value, result, row_index
                    )
                return result
            except (ValueError, TypeError):
                context.add_issue(
                    'warning', field_name,
                    f"Invalid boolean value, defaulting to False: {str_value}",
                    value, False, row_index
                )
                return False
    
    def _validate_flag_combinations(self, row: Dict[str, Any], 
                                   context: TransformationContext, 
                                   row_index: int):
        """Validate boolean flag combinations for logical consistency."""
        flags = {
            'played': row.get('Played', False),
            'playing': row.get('Playing', False),
            'finished': row.get('Finished', False),
            'mastered': row.get('Mastered', False),
            'dominated': row.get('Dominated', False),
            'shelved': row.get('Shelved', False)
        }
        
        # Check for impossible combinations
        if flags['playing'] and flags['shelved']:
            context.add_issue(
                'warning', 'Playing/Shelved',
                'Playing and Shelved both true - setting Playing to False',
                f"Playing={flags['playing']}, Shelved={flags['shelved']}",
                'Playing=False, Shelved=True', row_index
            )
            row['Playing'] = False
        
        # Check hierarchy consistency (Dominated implies Mastered implies Finished)
        if flags['dominated'] and not flags['mastered']:
            context.add_issue(
                'warning', 'Dominated/Mastered',
                'Dominated without Mastered - setting Mastered to True',
                f"Dominated={flags['dominated']}, Mastered={flags['mastered']}",
                'Mastered=True', row_index
            )
            row['Mastered'] = True
        
        if flags['mastered'] and not flags['finished']:
            context.add_issue(
                'warning', 'Mastered/Finished',
                'Mastered without Finished - setting Finished to True',
                f"Mastered={flags['mastered']}, Finished={flags['finished']}",
                'Finished=True', row_index
            )
            row['Finished'] = True
        
        # Auto-set Dominated hierarchy if needed
        if flags['dominated'] and not flags['finished']:
            context.add_issue(
                'warning', 'Dominated/Finished',
                'Dominated without Finished - setting Finished to True',
                f"Dominated={flags['dominated']}, Finished={flags['finished']}",
                'Finished=True', row_index
            )
            row['Finished'] = True
    
    def _validate_rating(self, value: Any, context: TransformationContext, 
                        row_index: int) -> Optional[float]:
        """Validate rating values (0-5 scale)."""
        if not value or str(value).strip() == '':
            return None
        
        try:
            rating = float(str(value).strip())
            if 0.0 <= rating <= 5.0:
                return rating
            else:
                context.add_issue(
                    'warning', 'Rating',
                    f'Rating out of range (0-5): {rating}',
                    value, None, row_index
                )
                return None
        except (ValueError, TypeError):
            context.add_issue(
                'warning', 'Rating',
                f'Invalid rating value: {value}',
                value, None, row_index
            )
            return None
    
    def _validate_date(self, value: Any, field_name: str, 
                      context: TransformationContext, 
                      row_index: int) -> Optional[str]:
        """Validate and normalize date values to ISO format."""
        if not value or str(value).strip() == '':
            return None
        
        date_str = str(value).strip()
        
        # Try each format
        for date_format in self.DATE_FORMATS:
            try:
                parsed_date = datetime.strptime(date_str, date_format)
                return parsed_date.date().isoformat()
            except ValueError:
                continue
        
        # If no format worked, log warning and return None
        context.add_issue(
            'warning', field_name,
            f'Invalid date format: {date_str}',
            value, None, row_index
        )
        return None


class MappingStage(TransformationStage):
    """Stage that maps platforms and storefronts to known values."""
    
    # Platform mappings with fuzzy matching support
    PLATFORM_MAPPINGS = {
        'PC': 'PC (Windows)',
        'Mac': 'PC (Windows)',
        'Linux': 'PC (Windows)',
        'PlayStation 4': 'PlayStation 4',
        'PlayStation 5': 'PlayStation 5',
        'PlayStation 3': 'PlayStation 3',
        'PS4': 'PlayStation 4',
        'PS5': 'PlayStation 5', 
        'PS3': 'PlayStation 3',
        'Nintendo Switch': 'Nintendo Switch',
        'Xbox 360': 'Xbox 360',
        'Xbox One': 'Xbox One',
        'Xbox Series X/S': 'Xbox Series X/S',
        'Nintendo Wii': 'Nintendo Wii',
        'iOS': 'iOS',
        'Android': 'Android',
    }
    
    # Storefront mappings
    STOREFRONT_MAPPINGS = {
        'Steam': 'Steam',
        'Epic Games Store': 'Epic Games Store',
        'Epic': 'Epic Games Store',
        'GOG': 'GOG',
        'PlayStation Store': 'PlayStation Store',
        'PSN': 'PlayStation Store',  # Add PSN mapping
        'Nintendo eShop': 'Nintendo eShop',
        'Microsoft Store': 'Microsoft Store',
        'Humble Bundle': 'Humble Bundle',
        'Origin': 'Origin/EA App',
        'EA App': 'Origin/EA App',
        'Apple App Store': 'Apple App Store',
        'Google Play Store': 'Google Play Store',
        'Physical': 'Physical',
        'Gamestop': 'Physical',
        'Best Buy': 'Physical',
        'Amazon': 'Physical',
        'Other': 'Physical',
    }
    
    # Default storefronts for platforms
    PLATFORM_DEFAULT_STOREFRONTS = {
        'PC (Windows)': 'Steam',
        'PlayStation 4': 'PlayStation Store',
        'PlayStation 5': 'PlayStation Store',
        'PlayStation 3': 'PlayStation Store',
        'Xbox Series X/S': 'Microsoft Store',
        'Xbox One': 'Microsoft Store',
        'Xbox 360': 'Microsoft Store',
        'Nintendo Switch': 'Nintendo eShop',
        'Nintendo Wii': 'Nintendo eShop',
        'iOS': 'Apple App Store',
        'Android': 'Google Play Store',
    }
    
    def get_stage_name(self) -> str:
        return "Mapping"
    
    async def process(self, data: List[Dict[str, Any]], 
                     context: TransformationContext) -> List[Dict[str, Any]]:
        """Map platform and storefront values."""
        logger.info(f"Starting mapping stage for {len(data)} rows")
        
        mapped_data = []
        
        for row_index, row in enumerate(data):
            try:
                mapped_row = await self._map_row(row, context, row_index)
                mapped_data.append(mapped_row)
            except Exception as e:
                logger.error(f"Error mapping row {row_index}: {e}")
                context.add_issue(
                    'error', 'mapping', f"Failed to map row: {str(e)}", 
                    row, row_index=row_index
                )
                # Still include the row with original values
                mapped_data.append(row)
        
        logger.info(f"Mapping complete for {len(mapped_data)} rows")
        return mapped_data
    
    async def _map_row(self, row: Dict[str, Any], 
                      context: TransformationContext, 
                      row_index: int) -> Dict[str, Any]:
        """Map platform and storefront values for a single row."""
        mapped_row = row.copy()
        
        # Map Copy platform
        original_platform = row.get('Copy platform', '').strip()
        if original_platform:
            mapped_platform = await self._map_platform(
                original_platform, context, row_index
            )
            mapped_row['_mapped_platform'] = mapped_platform
            mapped_row['_original_platform'] = original_platform
        
        # Map Copy source (with fallback to "Copy source other")
        original_source = row.get('Copy source', '').strip()
        other_source = row.get('Copy source other', '').strip()
        
        # Use "other" field if main source is generic
        if original_source in ['Other', 'other', ''] and other_source:
            source_to_map = other_source
        else:
            source_to_map = original_source
        
        # Always try to map storefront (even if empty, to get default)
        mapped_storefront = await self._map_storefront(
            source_to_map, 
            mapped_row.get('_mapped_platform', ''),
            context, 
            row_index
        )
        mapped_row['_mapped_storefront'] = mapped_storefront
        if source_to_map:  # Only set original if there was a value
            mapped_row['_original_storefront'] = source_to_map
        
        return mapped_row
    
    async def _map_platform(self, platform_name: str, 
                           context: TransformationContext,
                           row_index: int) -> Optional[str]:
        """Map platform name to known platform."""
        if not platform_name:
            return None
        
        # Direct mapping
        if platform_name in self.PLATFORM_MAPPINGS:
            return self.PLATFORM_MAPPINGS[platform_name]
        
        # Fuzzy matching against known platforms
        best_match = None
        best_confidence = 0.0
        
        for known_platform in self.PLATFORM_MAPPINGS.values():
            confidence = calculate_fuzzy_confidence(platform_name.lower(), known_platform.lower())
            if confidence > best_confidence and confidence >= 0.5:  # Lower threshold for fuzzy matching
                best_match = known_platform
                best_confidence = confidence
        
        if best_match:
            context.add_issue(
                'info', 'Copy platform',
                f'Fuzzy matched platform: {platform_name} -> {best_match} (confidence: {best_confidence:.2f})',
                platform_name, best_match, row_index
            )
            return best_match
        
        # Unknown platform
        context.unknown_platforms.add(platform_name)
        context.add_issue(
            'warning', 'Copy platform',
            f'Unknown platform: {platform_name}',
            platform_name, None, row_index
        )
        return None
    
    async def _map_storefront(self, storefront_name: str, platform_name: str,
                             context: TransformationContext,
                             row_index: int) -> str:
        """Map storefront name to known storefront."""
        if not storefront_name:
            # Use default storefront for platform
            return self.PLATFORM_DEFAULT_STOREFRONTS.get(platform_name, 'Physical')
        
        # Direct mapping
        if storefront_name in self.STOREFRONT_MAPPINGS:
            return self.STOREFRONT_MAPPINGS[storefront_name]
        
        # Fuzzy matching
        best_match = None
        best_confidence = 0.0
        
        for known_storefront in self.STOREFRONT_MAPPINGS.values():
            confidence = calculate_fuzzy_confidence(storefront_name.lower(), known_storefront.lower())
            if confidence > best_confidence and confidence >= 0.7:  # Slightly lower threshold for storefronts
                best_match = known_storefront
                best_confidence = confidence
        
        if best_match:
            context.add_issue(
                'info', 'Copy source',
                f'Fuzzy matched storefront: {storefront_name} -> {best_match} (confidence: {best_confidence:.2f})',
                storefront_name, best_match, row_index
            )
            return best_match
        
        # Unknown storefront - track and use default
        context.unknown_storefronts.add(storefront_name)
        context.add_issue(
            'warning', 'Copy source',
            f'Unknown storefront: {storefront_name}',
            storefront_name, None, row_index
        )
        
        # Use default storefront for platform
        return self.PLATFORM_DEFAULT_STOREFRONTS.get(platform_name, 'Physical')


class PersistenceStage(TransformationStage):
    """Stage that prepares data for database persistence."""
    
    def get_stage_name(self) -> str:
        return "Persistence"
    
    async def process(self, data: List[Dict[str, Any]], 
                     context: TransformationContext) -> List[Dict[str, Any]]:
        """Prepare data for database storage."""
        logger.info(f"Starting persistence stage for {len(data)} rows")
        
        prepared_data = []
        
        for row_index, row in enumerate(data):
            try:
                prepared_row = await self._prepare_row(row, context, row_index)
                prepared_data.append(prepared_row)
            except Exception as e:
                logger.error(f"Error preparing row {row_index}: {e}")
                context.add_issue(
                    'error', 'persistence', f"Failed to prepare row: {str(e)}", 
                    row, row_index=row_index
                )
                # Still include the row
                prepared_data.append(row)
        
        logger.info(f"Persistence preparation complete for {len(prepared_data)} rows")
        return prepared_data
    
    async def _prepare_row(self, row: Dict[str, Any], 
                          context: TransformationContext, 
                          row_index: int) -> Dict[str, Any]:
        """Prepare a single row for database storage."""
        prepared_row = row.copy()
        
        # Resolve play status from boolean flags
        play_status = self._resolve_play_status(row)
        prepared_row['_resolved_play_status'] = play_status
        
        # Extract physical copy metadata
        copy_metadata = self._extract_copy_metadata(row)
        if copy_metadata:
            prepared_row['_copy_metadata'] = copy_metadata
        
        # Ensure rating is properly formatted
        if prepared_row.get('Rating') is not None:
            try:
                prepared_row['Rating'] = float(prepared_row['Rating'])
            except (ValueError, TypeError):
                prepared_row['Rating'] = None
        
        return prepared_row
    
    def _resolve_play_status(self, row: Dict[str, Any]) -> str:
        """Resolve play status from Darkadia boolean flags."""
        flags = {
            'played': row.get('Played', False),
            'playing': row.get('Playing', False),
            'finished': row.get('Finished', False),
            'mastered': row.get('Mastered', False),
            'dominated': row.get('Dominated', False),
            'shelved': row.get('Shelved', False)
        }
        
        # Simple priority resolution
        if flags['dominated']:
            return PlayStatus.DOMINATED.value
        elif flags['mastered']:
            return PlayStatus.MASTERED.value
        elif flags['finished']:
            return PlayStatus.COMPLETED.value
        elif flags['shelved']:
            return PlayStatus.DROPPED.value  # Shelved = permanently abandoned
        elif flags['playing']:
            return PlayStatus.IN_PROGRESS.value
        elif flags['played']:
            return PlayStatus.SHELVED.value  # Played but not finished = paused/backlog
        else:
            return PlayStatus.NOT_STARTED.value
    
    def _extract_copy_metadata(self, row: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """Extract physical copy metadata from row."""
        copy_fields = [
            'Copy label', 'Copy Release', 'Copy platform', 'Copy media',
            'Copy source', 'Copy source other', 'Copy purchase date',
            'Copy box', 'Copy box condition', 'Copy box notes',
            'Copy manual', 'Copy manual condition', 'Copy manual notes',
            'Copy complete', 'Copy complete notes'
        ]
        
        metadata = {}
        for field in copy_fields:
            value = row.get(field, '')
            if value and str(value).strip():
                # Use simplified field name as key
                key = field.replace('Copy ', '').lower().replace(' ', '_')
                metadata[key] = str(value).strip()
        
        return metadata if metadata else None


class DarkadiaTransformationPipeline:
    """Main transformation pipeline for Darkadia CSV data."""
    
    def __init__(self):
        self.stages = [
            ValidationStage(),
            MappingStage(),
            PersistenceStage()
        ]
    
    async def transform(self, csv_data: List[Dict[str, Any]]) -> Tuple[List[Dict[str, Any]], TransformationContext]:
        """
        Transform CSV data through all pipeline stages.
        
        Args:
            csv_data: List of dictionaries representing CSV rows
            
        Returns:
            Tuple of (transformed_data, transformation_context)
        """
        if not csv_data:
            return [], TransformationContext()
        
        context = TransformationContext()
        current_data = csv_data
        
        logger.info(f"Starting transformation pipeline with {len(csv_data)} rows")
        
        for stage in self.stages:
            stage_name = stage.get_stage_name()
            logger.info(f"Processing stage: {stage_name}")
            
            try:
                current_data = await stage.process(current_data, context)
                logger.info(f"Completed stage: {stage_name}")
            except Exception as e:
                logger.error(f"Stage {stage_name} failed: {e}")
                context.add_issue('error', 'pipeline', f"Stage {stage_name} failed: {str(e)}", csv_data)
                break
        
        logger.info(f"Transformation pipeline complete. Summary: {context.get_summary()}")
        return current_data, context