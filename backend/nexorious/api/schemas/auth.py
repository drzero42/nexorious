"""
Authentication-related schemas for API requests and responses.
"""

from pydantic import BaseModel, EmailStr, Field, ConfigDict
from typing import Optional
from datetime import datetime


class UserRegisterRequest(BaseModel):
    """Request schema for user registration."""
    email: EmailStr = Field(..., description="User email address")
    username: str = Field(..., min_length=3, max_length=100, description="Username")
    password: str = Field(..., min_length=8, max_length=128, description="Password")
    first_name: Optional[str] = Field(None, max_length=100, description="First name")
    last_name: Optional[str] = Field(None, max_length=100, description="Last name")


class UserLoginRequest(BaseModel):
    """Request schema for user login."""
    username: str = Field(..., description="Username or email")
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
    email: EmailStr
    username: str
    first_name: Optional[str]
    last_name: Optional[str]
    is_active: bool
    is_admin: bool
    preferences: dict
    created_at: datetime
    updated_at: datetime

    model_config = ConfigDict(from_attributes=True)


class UserUpdateRequest(BaseModel):
    """Request schema for updating user profile."""
    first_name: Optional[str] = Field(None, max_length=100)
    last_name: Optional[str] = Field(None, max_length=100)
    preferences: Optional[dict] = Field(None, description="User preferences as JSON")


class ChangePasswordRequest(BaseModel):
    """Request schema for changing password."""
    current_password: str = Field(..., description="Current password")
    new_password: str = Field(..., min_length=8, max_length=128, description="New password")


class LogoutResponse(BaseModel):
    """Response schema for logout."""
    message: str = "Successfully logged out"