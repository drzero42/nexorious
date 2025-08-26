"""
Darkadia Data Mapper

This module handles data transformation from Darkadia CSV format
to Nexorious data models, including platform/storefront mapping
and play status conversion.
"""

from typing import Dict, Any, List, Optional
from datetime import datetime, date
from enum import Enum

import pandas as pd
from rich.console import Console

console = Console()


class PlayStatus(Enum):
    """Play status enumeration matching Nexorious backend."""
    NOT_STARTED = "not_started"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    MASTERED = "mastered" 
    DOMINATED = "dominated"
    SHELVED = "shelved"
    DROPPED = "dropped"
    REPLAY = "replay"


class OwnershipStatus(Enum):
    """Ownership status enumeration matching Nexorious backend."""
    OWNED = "owned"
    NO_LONGER_OWNED = "no_longer_owned"
    WISHLIST = "wishlist"


class DarkadiaDataMapper:
    """Maps Darkadia CSV data to Nexorious data structures."""
    
    # Platform mapping from Darkadia names to Nexorious platform names
    PLATFORM_MAPPINGS = {
        'PC': 'PC (Windows)',
        'Mac': 'PC (Windows)',  # Map Mac to PC for now
        'Linux': 'PC (Windows)',  # Map Linux to PC for now
        'PlayStation 4': 'PlayStation 4',
        'PlayStation 5': 'PlayStation 5',
        'PlayStation Network (PS3)': 'PlayStation 3',
        'PS3': 'PlayStation 3',
        'Nintendo Switch': 'Nintendo Switch',
        'Xbox 360 Games Store': 'Xbox 360',
        'Xbox 360': 'Xbox 360',
        'Xbox One': 'Xbox One',
        'Xbox Series X/S': 'Xbox Series X/S',
        'Nintendo Wii': 'Nintendo Wii',
        'iOS': 'iOS',
        'Android': 'Android',
    }
    
    # Storefront mapping from Darkadia names to Nexorious storefront names
    STOREFRONT_MAPPINGS = {
        'Steam': 'Steam',
        'Epic Games Store': 'Epic Games Store',
        'Epic': 'Epic Games Store',
        'GOG': 'GOG',
        'Sony Entertainment Network': 'PlayStation Store',
        'PSN': 'PlayStation Store',
        'PlayStation Store': 'PlayStation Store',
        'Nintendo eShop': 'Nintendo eShop',
        'Microsoft Store': 'Microsoft Store',
        'Humble Bundle': 'Humble Bundle',
        'HB': 'Humble Bundle',
        'Itch.io': 'Itch.io',
        'Origin': 'Origin/EA App',
        'EA App': 'Origin/EA App',
        'Apple App Store': 'Apple App Store',
        'Google Play Store': 'Google Play Store',
        'Physical': 'Physical',
        'Other': 'Physical',  # Fallback to Physical for unknown sources
    }
    
    # Default storefront for each platform
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
    
    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        self.unknown_platforms: set = set()
        self.unknown_storefronts: set = set()
    
    def convert_to_nexorious_game(self, darkadia_game: Dict[str, Any]) -> Dict[str, Any]:
        """
        Convert a Darkadia game record to Nexorious game format.
        
        Args:
            darkadia_game: Game data from Darkadia CSV
            
        Returns:
            Dictionary in Nexorious game format
        """
        
        # Basic game information
        nexorious_game = {
            'title': self._clean_string(darkadia_game.get('Name', '')),
            'description': '',  # Darkadia doesn't have descriptions
            'personal_rating': self._convert_rating(darkadia_game.get('Rating')),
            'is_loved': bool(darkadia_game.get('Loved', False)),
            'personal_notes': self._clean_string(darkadia_game.get('Notes', '')),
            'acquired_date': self._convert_date(darkadia_game.get('Added')),
            'ownership_status': self._convert_ownership_status(darkadia_game),
            'play_status': self._convert_play_status(darkadia_game),
            'hours_played': 0,  # Darkadia doesn't track hours
            'platforms': self._convert_platforms(darkadia_game)
        }
        
        return nexorious_game
    
    def _convert_rating(self, rating: Any) -> Optional[float]:
        """Convert Darkadia rating (0-5) to Nexorious rating (0-5)."""
        if pd.isna(rating) or rating is None:
            return None
        
        try:
            rating_float = float(rating)
            # Ensure rating is within valid range
            if 0 <= rating_float <= 5:
                return rating_float
            else:
                if self.verbose:
                    console.print(f"[yellow]Invalid rating value: {rating_float}[/yellow]")
                return None
        except (ValueError, TypeError):
            if self.verbose:
                console.print(f"[yellow]Could not convert rating: {rating}[/yellow]")
            return None
    
    def _convert_date(self, date_value: Any) -> Optional[str]:
        """Convert date to ISO format string."""
        if pd.isna(date_value) or date_value is None:
            return None
        
        try:
            if isinstance(date_value, pd.Timestamp):
                return date_value.strftime('%Y-%m-%d')
            elif isinstance(date_value, (datetime, date)):
                return date_value.strftime('%Y-%m-%d')
            elif isinstance(date_value, str):
                # Try to parse string date
                parsed_date = pd.to_datetime(date_value, errors='coerce')
                if not pd.isna(parsed_date):
                    return parsed_date.strftime('%Y-%m-%d')
        except Exception:
            pass
        
        if self.verbose:
            console.print(f"[yellow]Could not convert date: {date_value}[/yellow]")
        return None
    
    def _convert_ownership_status(self, darkadia_game: Dict[str, Any]) -> str:
        """Convert Darkadia ownership status to Nexorious format."""
        owned = bool(darkadia_game.get('Owned', False))
        return OwnershipStatus.OWNED.value if owned else OwnershipStatus.NO_LONGER_OWNED.value
    
    def _convert_play_status(self, darkadia_game: Dict[str, Any]) -> str:
        """
        Convert Darkadia play status flags to Nexorious PlayStatus enum.
        
        Darkadia uses multiple boolean flags, we convert to single enum value
        using priority order: Dominated > Mastered > Finished > Playing > Shelved > Played > Not Started
        """
        
        # Check flags in priority order
        if bool(darkadia_game.get('Dominated', False)):
            return PlayStatus.DOMINATED.value
        elif bool(darkadia_game.get('Mastered', False)):
            return PlayStatus.MASTERED.value
        elif bool(darkadia_game.get('Finished', False)):
            return PlayStatus.COMPLETED.value
        elif bool(darkadia_game.get('Playing', False)):
            return PlayStatus.IN_PROGRESS.value
        elif bool(darkadia_game.get('Shelved', False)):
            return PlayStatus.SHELVED.value
        elif bool(darkadia_game.get('Played', False)):
            return PlayStatus.COMPLETED.value  # Fallback: generic "played" -> completed
        else:
            return PlayStatus.NOT_STARTED.value
    
    def _convert_platforms(self, darkadia_game: Dict[str, Any]) -> List[Dict[str, Any]]:
        """
        Convert platform information from Darkadia to Nexorious format.
        
        Returns list of platform associations with storefronts.
        """
        platforms = []
        
        # Get platform information from merged data
        platform_data = darkadia_game.get('_platforms', [])
        
        if not platform_data:
            # Fallback: try to extract from individual fields
            platform_name = darkadia_game.get('Copy platform', '').strip()
            if platform_name and platform_name != 'nan':
                platform_info = {
                    'platform': platform_name,
                    'storefront': darkadia_game.get('Copy source', '').strip(),
                    'storefront_other': darkadia_game.get('Copy source other', '').strip(),
                    'media': darkadia_game.get('Copy media', '').strip(),
                }
                platform_data = [platform_info]
        
        for platform_info in platform_data:
            converted_platform = self._convert_single_platform(platform_info)
            if converted_platform:
                platforms.append(converted_platform)
        
        return platforms
    
    def _convert_single_platform(self, platform_info: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """Convert a single platform entry to Nexorious format."""
        
        darkadia_platform = platform_info.get('platform', '').strip()
        if not darkadia_platform or darkadia_platform == 'nan':
            return None
        
        # Map platform name
        nexorious_platform = self._map_platform_name(darkadia_platform)
        if not nexorious_platform:
            return None
        
        # Map storefront
        darkadia_storefront = platform_info.get('storefront', '').strip()
        darkadia_storefront_other = platform_info.get('storefront_other', '').strip()
        
        # Use custom storefront if provided, otherwise use main storefront
        storefront_to_map = darkadia_storefront_other if darkadia_storefront_other else darkadia_storefront
        
        nexorious_storefront = self._map_storefront_name(storefront_to_map, nexorious_platform)
        
        # Determine if this is a physical copy
        media = platform_info.get('media', '').strip().lower()
        is_physical = media == 'physical'
        
        return {
            'platform_name': nexorious_platform,
            'storefront_name': nexorious_storefront,
            'is_available': True,  # Assume available by default
            'is_physical': is_physical,
            'metadata': {
                'original_platform': darkadia_platform,
                'original_storefront': storefront_to_map,
                'media_info': platform_info.get('media', ''),
                'label': platform_info.get('label', ''),
                'release': platform_info.get('release', ''),
                'purchase_date': self._convert_date(platform_info.get('purchase_date')),
                'box_info': platform_info.get('metadata', {})
            }
        }
    
    def _map_platform_name(self, darkadia_platform: str) -> Optional[str]:
        """Map Darkadia platform name to Nexorious platform name."""
        
        # Direct mapping
        if darkadia_platform in self.PLATFORM_MAPPINGS:
            return self.PLATFORM_MAPPINGS[darkadia_platform]
        
        # Fuzzy matching for common variations
        darkadia_lower = darkadia_platform.lower()
        
        # Common variations
        if 'playstation 4' in darkadia_lower or 'ps4' in darkadia_lower:
            return 'PlayStation 4'
        elif 'playstation 5' in darkadia_lower or 'ps5' in darkadia_lower:
            return 'PlayStation 5'
        elif 'playstation 3' in darkadia_lower or 'ps3' in darkadia_lower:
            return 'PlayStation 3'
        elif 'xbox 360' in darkadia_lower:
            return 'Xbox 360'
        elif 'xbox one' in darkadia_lower:
            return 'Xbox One'
        elif 'xbox series' in darkadia_lower:
            return 'Xbox Series X/S'
        elif 'nintendo switch' in darkadia_lower or 'switch' in darkadia_lower:
            return 'Nintendo Switch'
        elif 'pc' in darkadia_lower or 'windows' in darkadia_lower:
            return 'PC (Windows)'
        elif 'mac' in darkadia_lower or 'macos' in darkadia_lower:
            return 'PC (Windows)'  # Map to PC for now
        elif 'linux' in darkadia_lower:
            return 'PC (Windows)'  # Map to PC for now
        
        # Unknown platform
        self.unknown_platforms.add(darkadia_platform)
        if self.verbose:
            console.print(f"[yellow]Unknown platform: {darkadia_platform}[/yellow]")
        
        return None
    
    def _map_storefront_name(self, darkadia_storefront: str, platform_name: str) -> str:
        """Map Darkadia storefront name to Nexorious storefront name."""
        
        if not darkadia_storefront or darkadia_storefront == 'nan':
            # Use default storefront for platform
            return self.PLATFORM_DEFAULT_STOREFRONTS.get(platform_name, 'Physical')
        
        # Direct mapping
        if darkadia_storefront in self.STOREFRONT_MAPPINGS:
            return self.STOREFRONT_MAPPINGS[darkadia_storefront]
        
        # Fuzzy matching
        darkadia_lower = darkadia_storefront.lower()
        
        if 'steam' in darkadia_lower:
            return 'Steam'
        elif 'epic' in darkadia_lower:
            return 'Epic Games Store'
        elif 'gog' in darkadia_lower:
            return 'GOG'
        elif 'playstation' in darkadia_lower or 'psn' in darkadia_lower or 'sony' in darkadia_lower:
            return 'PlayStation Store'
        elif 'nintendo' in darkadia_lower or 'eshop' in darkadia_lower:
            return 'Nintendo eShop'
        elif 'microsoft' in darkadia_lower or 'xbox' in darkadia_lower:
            return 'Microsoft Store'
        elif 'humble' in darkadia_lower:
            return 'Humble Bundle'
        elif 'origin' in darkadia_lower or 'ea' in darkadia_lower:
            return 'Origin/EA App'
        elif 'apple' in darkadia_lower or 'app store' in darkadia_lower:
            return 'Apple App Store'
        elif 'google' in darkadia_lower or 'play store' in darkadia_lower:
            return 'Google Play Store'
        elif 'physical' in darkadia_lower:
            return 'Physical'
        
        # Unknown storefront - record it and use default
        self.unknown_storefronts.add(darkadia_storefront)
        if self.verbose:
            console.print(f"[yellow]Unknown storefront: {darkadia_storefront}[/yellow]")
        
        # Use default storefront for platform
        return self.PLATFORM_DEFAULT_STOREFRONTS.get(platform_name, 'Physical')
    
    def _clean_string(self, value: Any) -> str:
        """Clean and normalize string values."""
        if pd.isna(value) or value is None:
            return ''
        
        # Convert to string and strip whitespace
        cleaned = str(value).strip()
        
        # Remove pandas 'nan' artifacts
        if cleaned.lower() == 'nan':
            return ''
        
        return cleaned
    
    def get_mapping_summary(self) -> Dict[str, Any]:
        """Get summary of mapping results including unknown platforms/storefronts."""
        return {
            'unknown_platforms': list(self.unknown_platforms),
            'unknown_storefronts': list(self.unknown_storefronts),
            'unknown_platform_count': len(self.unknown_platforms),
            'unknown_storefront_count': len(self.unknown_storefronts)
        }