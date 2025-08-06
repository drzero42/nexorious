"""
Steam Web API configuration schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, field_validator
from typing import Optional, Dict, Any, List
from datetime import datetime
from ...models.steam_import import SteamImportJobStatus, SteamImportGameStatus


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
        
        if not v.startswith('7656119'):
            raise ValueError('Steam ID must be a valid 64-bit Steam ID starting with 7656119')
        
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
        
        if not v.startswith('7656119'):
            raise ValueError('Steam ID must be a valid 64-bit Steam ID starting with 7656119')
        
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


# Steam Import Schemas

class SteamImportJobCreateRequest(BaseModel):
    """Request schema for creating a Steam import job."""
    pass  # No additional parameters needed - uses user's Steam config


class SteamImportJobResponse(BaseModel):
    """Response schema for Steam import job creation."""
    id: str = Field(..., description="Import job ID")
    status: SteamImportJobStatus = Field(..., description="Current job status")
    total_games: int = Field(default=0, description="Total number of games in Steam library")
    processed_games: int = Field(default=0, description="Number of games processed so far")
    matched_games: int = Field(default=0, description="Number of games automatically matched")
    awaiting_review_games: int = Field(default=0, description="Number of games awaiting user review")
    skipped_games: int = Field(default=0, description="Number of games skipped by user")
    imported_games: int = Field(default=0, description="Number of new games imported")
    platform_added_games: int = Field(default=0, description="Number of games where Steam platform was added")
    error_message: Optional[str] = Field(None, description="Error message if job failed")
    created_at: datetime = Field(..., description="Job creation timestamp")
    updated_at: datetime = Field(..., description="Job last update timestamp")
    completed_at: Optional[datetime] = Field(None, description="Job completion timestamp")


class SteamImportGameResponse(BaseModel):
    """Response schema for individual Steam import game."""
    id: str = Field(..., description="Import game record ID")
    steam_appid: int = Field(..., description="Steam App ID")
    steam_name: str = Field(..., description="Game name from Steam")
    status: SteamImportGameStatus = Field(..., description="Current game status")
    matched_game_id: Optional[str] = Field(None, description="ID of matched game in database")
    user_decision: Optional[Dict[str, Any]] = Field(None, description="User's matching decision")
    error_message: Optional[str] = Field(None, description="Error message if import failed")
    created_at: datetime = Field(..., description="Record creation timestamp")
    updated_at: datetime = Field(..., description="Record last update timestamp")


class SteamImportJobStatusResponse(BaseModel):
    """Response schema for Steam import job status with detailed game information."""
    id: str = Field(..., description="Import job ID")
    status: SteamImportJobStatus = Field(..., description="Current job status")
    total_games: int = Field(default=0, description="Total number of games in Steam library")
    processed_games: int = Field(default=0, description="Number of games processed so far")
    matched_games: int = Field(default=0, description="Number of games automatically matched")
    awaiting_review_games: int = Field(default=0, description="Number of games awaiting user review")
    skipped_games: int = Field(default=0, description="Number of games skipped by user")
    imported_games: int = Field(default=0, description="Number of new games imported")
    platform_added_games: int = Field(default=0, description="Number of games where Steam platform was added")
    error_message: Optional[str] = Field(None, description="Error message if job failed")
    created_at: datetime = Field(..., description="Job creation timestamp")
    updated_at: datetime = Field(..., description="Job last update timestamp")
    completed_at: Optional[datetime] = Field(None, description="Job completion timestamp")
    games: List[SteamImportGameResponse] = Field(default=[], description="Individual game statuses")


class SteamImportUserDecisionRequest(BaseModel):
    """Request schema for submitting user decisions on games awaiting review."""
    decisions: Dict[str, Dict[str, Any]] = Field(
        ..., 
        description="Map of steam_appid (as string) to user decision. Decision format: {'action': 'import'|'skip', 'igdb_id': 'optional_igdb_id', 'notes': 'optional_notes'}"
    )


class SteamImportConfirmRequest(BaseModel):
    """Request schema for confirming final import execution."""
    pass  # No additional parameters needed - all decisions already submitted


