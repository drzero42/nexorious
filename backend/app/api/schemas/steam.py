"""
Steam Web API configuration schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, field_validator
from typing import Optional, Dict, Any, List
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



