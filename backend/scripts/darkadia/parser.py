"""
Darkadia CSV Parser

This module handles parsing and validation of Darkadia CSV export files,
including duplicate detection and data consolidation.
"""

import asyncio
from pathlib import Path
from typing import List, Dict, Any, Optional, Set
from datetime import datetime, date
import uuid

import pandas as pd
from rich.console import Console
from rich.progress import Progress, SpinnerColumn, TextColumn, BarColumn
from rapidfuzz import fuzz

from .mapper import DarkadiaDataMapper

console = Console()


def safe_strip(value, default=''):
    """
    Safely strip a value, handling pandas NaN and None values.
    
    Args:
        value: Value to strip (can be string, float NaN, None, etc.)
        default: Default value to return for invalid inputs
        
    Returns:
        Stripped string or default value
    """
    if pd.isna(value) or value is None:
        return default
    return str(value).strip()


class DarkadiaCSVParser:
    """Parser for Darkadia CSV export files."""
    
    # Expected columns in Darkadia CSV format
    EXPECTED_COLUMNS = [
        'Name', 'Added', 'Loved', 'Owned', 'Played', 'Playing', 'Finished',
        'Mastered', 'Dominated', 'Shelved', 'Rating', 'Copy label', 'Copy Release',
        'Copy platform', 'Copy media', 'Copy media other', 'Copy source',
        'Copy source other', 'Copy purchase date', 'Copy box', 'Copy box condition',
        'Copy box notes', 'Copy manual', 'Copy manual condition', 'Copy manual notes',
        'Copy complete', 'Copy complete notes', 'Platforms', 'Notes'
    ]
    
    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        self.mapper = DarkadiaDataMapper()
        self.validation_errors: List[str] = []
        self.warnings: List[str] = []
    
    async def parse_csv(self, csv_file: Path) -> List[Dict[str, Any]]:
        """
        Parse Darkadia CSV file and return list of game data.
        
        Args:
            csv_file: Path to the CSV file
            
        Returns:
            List of dictionaries containing game data
            
        Raises:
            ValueError: If CSV format is invalid
            FileNotFoundError: If CSV file doesn't exist
        """
        
        if not csv_file.exists():
            raise FileNotFoundError(f"CSV file not found: {csv_file}")
        
        console.print(f"Reading CSV file: {csv_file}")
        
        try:
            # Read CSV with pandas
            df = pd.read_csv(csv_file, encoding='utf-8')
            
            if self.verbose:
                console.print(f"CSV shape: {df.shape}")
                console.print(f"Columns found: {list(df.columns)}")
            
            # Validate CSV structure
            await self._validate_csv_structure(df)
            
            # Clean and process data
            df = await self._clean_dataframe(df)
            
            # Replace any remaining NaN values with empty strings before dict conversion
            # This prevents float NaN from appearing in the dictionaries
            df = df.fillna('')
            
            # Convert to list of dictionaries
            games_data = df.to_dict('records')
            
            console.print(f"Successfully parsed {len(games_data)} rows")
            
            if self.validation_errors:
                console.print(f"[yellow]Found {len(self.validation_errors)} validation errors[/yellow]")
                if self.verbose:
                    for error in self.validation_errors[:5]:  # Show first 5 errors
                        console.print(f"  • {error}")
                    if len(self.validation_errors) > 5:
                        console.print(f"  ... and {len(self.validation_errors) - 5} more")
            
            return games_data
            
        except pd.errors.EmptyDataError:
            raise ValueError("CSV file is empty")
        except pd.errors.ParserError as e:
            raise ValueError(f"CSV parsing error: {str(e)}")
        except Exception as e:
            raise ValueError(f"Error reading CSV file: {str(e)}")
    
    async def _validate_csv_structure(self, df: pd.DataFrame):
        """Validate that the CSV has the expected structure."""
        
        # Check if DataFrame is empty
        if df.empty:
            raise ValueError("CSV file contains no data")
        
        # Check for critical columns (flexible - allow missing optional columns)
        critical_columns = ['Name']  # Only Name is truly critical
        missing_critical = [col for col in critical_columns if col not in df.columns]
        
        if missing_critical:
            raise ValueError(f"Missing critical columns: {missing_critical}")
        
        # Warn about missing optional columns
        missing_optional = [col for col in self.EXPECTED_COLUMNS if col not in df.columns]
        if missing_optional and self.verbose:
            console.print(f"[yellow]Missing optional columns: {missing_optional}[/yellow]")
        
        # Check for completely empty Name column
        if df['Name'].isna().all():
            raise ValueError("All game names are empty")
        
        console.print("✓ CSV structure validation passed")
    
    async def _clean_dataframe(self, df: pd.DataFrame) -> pd.DataFrame:
        """Clean and normalize the CSV data."""
        
        # Create a copy to avoid modifying original
        df = df.copy()
        
        # Remove rows where Name is empty (these are continuation rows)
        # but first, let's handle multi-row games properly
        df = await self._handle_continuation_rows(df)
        
        # Normalize text fields
        text_columns = ['Name', 'Copy label', 'Copy Release', 'Copy platform', 
                       'Copy source', 'Notes']
        for col in text_columns:
            if col in df.columns:
                df[col] = df[col].astype(str).str.strip()
                df[col] = df[col].replace('nan', '')  # pandas converts None to 'nan' string
        
        # Normalize boolean columns (convert to proper booleans)
        boolean_columns = ['Loved', 'Owned', 'Played', 'Playing', 'Finished', 
                          'Mastered', 'Dominated', 'Shelved']
        for col in boolean_columns:
            if col in df.columns:
                df[col] = pd.to_numeric(df[col], errors='coerce').fillna(0).astype(bool)
        
        # Clean numeric columns
        if 'Rating' in df.columns:
            df['Rating'] = pd.to_numeric(df['Rating'], errors='coerce')
            # Validate rating range
            invalid_ratings = df[(df['Rating'] < 0) | (df['Rating'] > 5)].index
            if len(invalid_ratings) > 0:
                self.validation_errors.append(f"Found {len(invalid_ratings)} invalid ratings (outside 0-5 range)")
                df.loc[invalid_ratings, 'Rating'] = None
        
        # Clean date columns
        date_columns = ['Added', 'Copy purchase date']
        for col in date_columns:
            if col in df.columns:
                df[col] = pd.to_datetime(df[col], errors='coerce', format='%Y-%m-%d')
        
        console.print("✓ Data cleaning completed")
        return df
    
    async def _handle_continuation_rows(self, df: pd.DataFrame) -> pd.DataFrame:
        """
        Handle multi-row games in Darkadia format.
        
        In Darkadia CSV, games with multiple platforms appear as multiple rows,
        where the first row has the game name and subsequent rows are empty in the Name column.
        """
        
        # Forward-fill the Name column to handle continuation rows
        df['Name'] = df['Name'].replace('', pd.NA)  # Convert empty strings to NaN
        df['Name'] = df['Name'].fillna(method='ffill')
        
        # Remove rows that are completely empty (all NaN except potentially Name)
        df = df.dropna(how='all', subset=[col for col in df.columns if col != 'Name'])
        
        return df
    
    async def group_duplicates(self, games_data: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """
        Group duplicate games and merge their platform data.
        
        Args:
            games_data: List of game dictionaries from CSV
            
        Returns:
            List of unique games with merged platform data
        """
        
        console.print("Grouping duplicate games...")
        
        # Group games by normalized name
        game_groups: Dict[str, List[Dict[str, Any]]] = {}
        
        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            transient=True
        ) as progress:
            task = progress.add_task("Processing games...", total=len(games_data))
            
            for game_data in games_data:
                # Skip rows with empty names
                if not game_data.get('Name') or pd.isna(game_data['Name']):
                    progress.update(task, advance=1)
                    continue
                
                # Normalize game name for grouping
                normalized_name = self._normalize_game_name(game_data['Name'])
                
                if normalized_name not in game_groups:
                    game_groups[normalized_name] = []
                
                game_groups[normalized_name].append(game_data)
                progress.update(task, advance=1)
        
        # Merge duplicate groups
        unique_games = []
        
        for normalized_name, group in game_groups.items():
            if len(group) == 1:
                # Single game, no merging needed
                unique_games.append(group[0])
            else:
                # Multiple rows for same game - merge them
                merged_game = await self._merge_game_group(group)
                unique_games.append(merged_game)
                
                if self.verbose:
                    console.print(f"Merged {len(group)} rows for game: {group[0]['Name']}")
        
        console.print(f"✓ Grouped into {len(unique_games)} unique games")
        return unique_games
    
    def _normalize_game_name(self, name: str) -> str:
        """Normalize game name for duplicate detection."""
        if pd.isna(name) or not name:
            return ""
        
        # Convert to lowercase and remove extra whitespace
        normalized = str(name).lower().strip()
        
        # Remove common punctuation and special characters
        import re
        normalized = re.sub(r'[^\w\s]', '', normalized)
        normalized = re.sub(r'\s+', ' ', normalized)
        
        return normalized
    
    async def _merge_game_group(self, group: List[Dict[str, Any]]) -> Dict[str, Any]:
        """
        Merge multiple rows of the same game into a single game record.
        
        Args:
            group: List of game dictionaries representing the same game
            
        Returns:
            Merged game dictionary
        """
        
        # Use first row as base
        merged = group[0].copy()
        
        # Collect all platform information from all rows
        platforms = []
        
        for row in group:
            platform_info = self._extract_platform_info(row)
            if platform_info:
                platforms.append(platform_info)
        
        # Store platform information in merged record
        merged['_platforms'] = platforms
        
        # For conflicting data, use majority rule or most recent
        # This is a simplified approach - more sophisticated logic could be added
        
        # Use the highest rating if multiple ratings exist
        ratings = [row.get('Rating') for row in group if row.get('Rating') and not pd.isna(row.get('Rating'))]
        if ratings:
            merged['Rating'] = max(ratings)
        
        # Use the most recent date
        dates = [row.get('Added') for row in group if row.get('Added') and not pd.isna(row.get('Added'))]
        if dates:
            merged['Added'] = max(dates)
        
        # Combine notes
        notes = [safe_strip(note) for row in group 
                for note in [row.get('Notes')] 
                if note is not None and safe_strip(note)]
        if notes:
            merged['Notes'] = ' | '.join(set(notes))  # Remove duplicates
        
        # For boolean fields, use OR logic (true if any row is true)
        boolean_fields = ['Loved', 'Owned', 'Played', 'Playing', 'Finished', 'Mastered', 'Dominated', 'Shelved']
        for field in boolean_fields:
            values = [bool(row.get(field, False)) for row in group]
            merged[field] = any(values)
        
        return merged
    
    def _extract_platform_info(self, row: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """Extract platform information from a CSV row."""
        
        platform = row.get('Copy platform', '').strip()
        if not platform or platform == 'nan':
            return None
        
        return {
            'platform': platform,
            'storefront': row.get('Copy source', '').strip(),
            'storefront_other': row.get('Copy source other', '').strip(),
            'media': row.get('Copy media', '').strip(),
            'media_other': row.get('Copy media other', '').strip(),
            'label': row.get('Copy label', '').strip(),
            'release': row.get('Copy Release', '').strip(),
            'purchase_date': row.get('Copy purchase date'),
            'metadata': {
                'box': row.get('Copy box', '').strip(),
                'box_condition': row.get('Copy box condition', '').strip(),
                'box_notes': row.get('Copy box notes', '').strip(),
                'manual': row.get('Copy manual', '').strip(),
                'manual_condition': row.get('Copy manual condition', '').strip(),
                'manual_notes': row.get('Copy manual notes', '').strip(),
                'complete': row.get('Copy complete', '').strip(),
                'complete_notes': row.get('Copy complete notes', '').strip(),
            }
        }
    
    def get_validation_summary(self) -> Dict[str, Any]:
        """Get summary of validation results."""
        return {
            'errors': self.validation_errors,
            'warnings': self.warnings,
            'error_count': len(self.validation_errors),
            'warning_count': len(self.warnings)
        }