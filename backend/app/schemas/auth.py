"""
Authentication-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, ConfigDict
from typing import Optional
from datetime import datetime


class UserLoginRequest(BaseModel):
    """Request schema for user login."""
    username: str = Field(..., description="Username")
    password: str = Field(..., description="Password")


class TokenResponse(BaseModel):
    """Response schema for token endpoints."""
    access_token: str = Field(..., description="JWT access token")
    refresh_token: str = Field(..., description="JWT refresh token")
    token_type: str = Field(default="bearer", description="Token type")
    expires_in: int = Field(..., description="Token expiration time in seconds")


class RefreshTokenRequest(BaseModel):
    """Request schema for token refresh."""
    refresh_token: str = Field(..., description="Refresh token")


class UserProfileResponse(BaseModel):
    """Response schema for user profile."""
    id: str
    username: str
    is_active: bool
    is_admin: bool
    preferences: dict
    created_at: datetime
    updated_at: datetime

    model_config = ConfigDict(from_attributes=True)


class UserUpdateRequest(BaseModel):
    """Request schema for updating user profile."""
    preferences: Optional[dict] = Field(None, description="User preferences as JSON")


class ChangePasswordRequest(BaseModel):
    """Request schema for changing password."""
    current_password: str = Field(..., description="Current password")
    new_password: str = Field(..., min_length=8, max_length=128, description="New password")


class ChangeUsernameRequest(BaseModel):
    """Request schema for changing username."""
    new_username: str = Field(..., min_length=3, max_length=100, description="New username")


class UsernameAvailabilityResponse(BaseModel):
    """Response schema for username availability check."""
    available: bool = Field(..., description="Whether the username is available")
    username: str = Field(..., description="Username that was checked")


class LogoutResponse(BaseModel):
    """Response schema for logout."""
    message: str = "Successfully logged out"


# Admin-specific schemas
class SetupStatusResponse(BaseModel):
    """Response schema for setup status check."""
    needs_setup: bool = Field(..., description="Whether initial admin setup is needed")


class InitialAdminSetupRequest(BaseModel):
    """Request schema for initial admin setup."""
    username: str = Field(..., min_length=3, max_length=100, description="Admin username")
    password: str = Field(..., min_length=8, max_length=128, description="Admin password")


class AdminUserCreateRequest(BaseModel):
    """Request schema for admin to create a new user."""
    username: str = Field(..., min_length=3, max_length=100, description="Username")
    password: str = Field(..., min_length=8, max_length=128, description="Password")
    is_admin: bool = Field(default=False, description="Whether user should have admin privileges")


class AdminUserUpdateRequest(BaseModel):
    """Request schema for admin to update a user."""
    username: Optional[str] = Field(None, min_length=3, max_length=100, description="New username")
    is_active: Optional[bool] = Field(None, description="Whether user is active")
    is_admin: Optional[bool] = Field(None, description="Whether user has admin privileges")


class AdminPasswordResetRequest(BaseModel):
    """Request schema for admin to reset a user's password."""
    new_password: str = Field(..., min_length=8, max_length=128, description="New password")


class AdminUserResponse(BaseModel):
    """Response schema for admin user management."""
    id: str
    username: str
    is_active: bool
    is_admin: bool
    created_at: datetime
    updated_at: datetime

    model_config = ConfigDict(from_attributes=True)


class UserDeletionImpactResponse(BaseModel):
    """Response schema for user deletion impact preview."""
    user_id: str = Field(..., description="User ID")
    username: str = Field(..., description="Username")
    total_games: int = Field(..., description="Number of games in user's collection")
    total_tags: int = Field(..., description="Number of user-created tags")
    total_wishlist_items: int = Field(..., description="Number of items in user's wishlist")
    total_import_jobs: int = Field(..., description="Number of import jobs")
    total_sessions: int = Field(..., description="Number of active sessions")
    warning: str = Field(default="All user data will be permanently deleted and cannot be recovered", description="Warning message")


class SetupRestoreResponse(BaseModel):
    """Response schema for setup restore."""
    success: bool = Field(..., description="Whether restore was successful")
    message: str = Field(..., description="Status message")