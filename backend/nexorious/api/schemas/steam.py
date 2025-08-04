"""
Steam Web API configuration schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, field_validator
from typing import Optional
from datetime import datetime


class SteamConfigRequest(BaseModel):
    """Request schema for setting Steam Web API configuration."""
    web_api_key: str = Field(..., min_length=32, max_length=32, description="Steam Web API key (32 characters)")
    steam_id: Optional[str] = Field(None, description="Optional Steam ID for validation (17-digit number)")
    
    @field_validator('web_api_key')
    @classmethod
    def validate_api_key_format(cls, v):
        """Validate Steam Web API key format."""
        if not v.isalnum():
            raise ValueError('Steam Web API key must contain only alphanumeric characters')
        return v
    
    @field_validator('steam_id')
    @classmethod
    def validate_steam_id_format(cls, v):
        """Validate Steam ID format if provided."""
        if v is None:
            return v
        
        if not v.isdigit():
            raise ValueError('Steam ID must contain only digits')
        
        if len(v) != 17:
            raise ValueError('Steam ID must be exactly 17 digits')
        
        if not v.startswith('765611979'):
            raise ValueError('Steam ID must be a valid 64-bit Steam ID starting with 765611979')
        
        return v


class SteamConfigResponse(BaseModel):
    """Response schema for Steam configuration."""
    has_api_key: bool = Field(..., description="Whether user has configured a Steam Web API key")
    api_key_masked: Optional[str] = Field(None, description="Masked API key (first 8 and last 4 characters)")
    steam_id: Optional[str] = Field(None, description="User's Steam ID if configured")
    is_verified: bool = Field(default=False, description="Whether the API key has been verified")
    configured_at: Optional[datetime] = Field(None, description="When the Steam configuration was last updated")


class SteamVerificationRequest(BaseModel):
    """Request schema for verifying Steam Web API key."""
    web_api_key: str = Field(..., min_length=32, max_length=32, description="Steam Web API key to verify")
    steam_id: Optional[str] = Field(None, description="Optional Steam ID to verify ownership")
    
    @field_validator('web_api_key')
    @classmethod
    def validate_api_key_format(cls, v):
        """Validate Steam Web API key format."""
        if not v.isalnum():
            raise ValueError('Steam Web API key must contain only alphanumeric characters')
        return v
    
    @field_validator('steam_id')
    @classmethod
    def validate_steam_id_format(cls, v):
        """Validate Steam ID format if provided."""
        if v is None:
            return v
        
        if not v.isdigit():
            raise ValueError('Steam ID must contain only digits')
        
        if len(v) != 17:
            raise ValueError('Steam ID must be exactly 17 digits')
        
        if not v.startswith('765611979'):
            raise ValueError('Steam ID must be a valid 64-bit Steam ID starting with 765611979')
        
        return v


class SteamVerificationResponse(BaseModel):
    """Response schema for Steam Web API key verification."""
    is_valid: bool = Field(..., description="Whether the Steam Web API key is valid")
    error_message: Optional[str] = Field(None, description="Error message if verification failed")
    steam_user_info: Optional[dict] = Field(None, description="Steam user information if Steam ID was provided and verified")


class SteamUserInfoResponse(BaseModel):
    """Response schema for Steam user information."""
    steam_id: str = Field(..., description="Steam ID")
    persona_name: str = Field(..., description="Steam display name")
    profile_url: str = Field(..., description="Steam profile URL")
    avatar: str = Field(..., description="Small avatar URL")
    avatar_medium: str = Field(..., description="Medium avatar URL")
    avatar_full: str = Field(..., description="Full avatar URL")
    persona_state: int = Field(..., description="Persona state (online status)")
    community_visibility_state: int = Field(..., description="Profile visibility state")
    profile_state: Optional[int] = Field(None, description="Whether user has configured their profile")
    last_logoff: Optional[int] = Field(None, description="Last logoff timestamp")


class SteamGameResponse(BaseModel):
    """Response schema for Steam game information."""
    appid: int = Field(..., description="Steam App ID")
    name: str = Field(..., description="Game name")
    playtime_forever: int = Field(..., description="Total playtime in minutes")
    playtime_windows_forever: int = Field(0, description="Windows playtime in minutes")
    playtime_mac_forever: int = Field(0, description="Mac playtime in minutes")
    playtime_linux_forever: int = Field(0, description="Linux playtime in minutes")
    rtime_last_played: Optional[int] = Field(None, description="Last played timestamp")
    playtime_disconnected: int = Field(0, description="Offline playtime in minutes")
    img_icon_url: Optional[str] = Field(None, description="Game icon URL")
    has_community_visible_stats: Optional[bool] = Field(None, description="Whether game has visible community stats")


class SteamLibraryResponse(BaseModel):
    """Response schema for Steam library information."""
    total_games: int = Field(..., description="Total number of games in library")
    games: list[SteamGameResponse] = Field(..., description="List of games in library")
    steam_user_info: SteamUserInfoResponse = Field(..., description="Steam user information")


class VanityUrlResolveRequest(BaseModel):
    """Request schema for resolving Steam vanity URL."""
    vanity_url: str = Field(..., min_length=3, max_length=32, description="Steam vanity URL (custom URL)")
    
    @field_validator('vanity_url')
    @classmethod
    def validate_vanity_url(cls, v):
        """Validate vanity URL format."""
        # Remove common URL prefixes if present
        if v.startswith('https://steamcommunity.com/id/'):
            v = v.replace('https://steamcommunity.com/id/', '')
        elif v.startswith('http://steamcommunity.com/id/'):
            v = v.replace('http://steamcommunity.com/id/', '')
        elif v.startswith('steamcommunity.com/id/'):
            v = v.replace('steamcommunity.com/id/', '')
        
        # Remove trailing slash
        v = v.rstrip('/')
        
        # Validate remaining vanity URL
        if not v.replace('_', '').replace('-', '').isalnum():
            raise ValueError('Vanity URL can only contain letters, numbers, underscores, and hyphens')
        
        return v


class VanityUrlResolveResponse(BaseModel):
    """Response schema for vanity URL resolution."""
    success: bool = Field(..., description="Whether the vanity URL was successfully resolved")
    steam_id: Optional[str] = Field(None, description="Resolved Steam ID if successful")
    error_message: Optional[str] = Field(None, description="Error message if resolution failed")


class SteamLibraryImportRequest(BaseModel):
    """Request schema for Steam library import."""
    fuzzy_threshold: float = Field(default=0.8, ge=0.0, le=1.0, description="Fuzzy matching threshold for game name matching (0.0-1.0)")
    merge_strategy: str = Field(default="skip", pattern="^(skip|add_platforms)$", description="How to handle games already in collection")
    platform_fallback: str = Field(default="pc-windows", description="Default platform when no Steam platform data available")


class SteamGameImportResult(BaseModel):
    """Result schema for individual Steam game import."""
    steam_appid: int = Field(..., description="Steam App ID")
    steam_name: str = Field(..., description="Game name from Steam")
    status: str = Field(..., description="Import status: 'imported', 'skipped', 'failed', 'no_match'")
    reason: Optional[str] = Field(None, description="Reason for skip/failure")
    matched_game_id: Optional[str] = Field(None, description="ID of matched game in database")
    matched_game_title: Optional[str] = Field(None, description="Title of matched game")
    detected_platforms: list[str] = Field(default_factory=list, description="Platforms detected from Steam data")
    match_score: Optional[float] = Field(None, description="Fuzzy match score if applicable")


class SteamLibraryImportResponse(BaseModel):
    """Response schema for Steam library import operation."""
    total_games: int = Field(..., description="Total games in Steam library")
    imported_count: int = Field(..., description="Number of games successfully imported")
    skipped_count: int = Field(..., description="Number of games skipped (already in collection)")
    failed_count: int = Field(..., description="Number of games that failed to import")
    no_match_count: int = Field(..., description="Number of games with no IGDB match found")
    platform_breakdown: dict[str, int] = Field(default_factory=dict, description="Count of games per detected platform")
    results: list[SteamGameImportResult] = Field(..., description="Detailed results for each game")
    import_summary: str = Field(..., description="Human-readable summary of import results")