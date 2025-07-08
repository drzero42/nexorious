"""
Authentication endpoints for user registration, login, and session management.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Request
from sqlmodel import Session, select
from datetime import timedelta, datetime, timezone
import json
import uuid
from typing import Annotated

from ..core.database import get_session
from ..core.security import (
    verify_password, 
    get_password_hash, 
    create_access_token, 
    create_refresh_token,
    verify_token,
    create_user_session,
    invalidate_user_session,
    get_current_user,
    hash_token
)
from ..core.config import settings
from ..models.user import User, UserSession
from ..api.schemas.auth import (
    UserRegisterRequest,
    UserLoginRequest,
    TokenResponse,
    RefreshTokenRequest,
    UserProfileResponse,
    UserUpdateRequest,
    ChangePasswordRequest,
    LogoutResponse
)
from ..api.schemas.common import SuccessResponse

router = APIRouter(prefix="/auth", tags=["Authentication"])


@router.post("/register", response_model=UserProfileResponse, status_code=status.HTTP_201_CREATED)
async def register_user(
    user_data: UserRegisterRequest,
    session: Annotated[Session, Depends(get_session)]
):
    """Register a new user."""
    
    # Check if user already exists
    existing_user = session.exec(
        select(User).where(
            (User.email == user_data.email) | (User.username == user_data.username)
        )
    ).first()
    
    if existing_user:
        if existing_user.email == user_data.email:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Email already registered"
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Username already taken"
            )
    
    # Create new user
    hashed_password = get_password_hash(user_data.password)
    new_user = User(
        email=user_data.email,
        username=user_data.username,
        password_hash=hashed_password,
        first_name=user_data.first_name,
        last_name=user_data.last_name
    )
    
    session.add(new_user)
    session.commit()
    session.refresh(new_user)
    
    return new_user


@router.post("/login", response_model=TokenResponse)
async def login_user(
    user_data: UserLoginRequest,
    request: Request,
    session: Annotated[Session, Depends(get_session)]
):
    """Authenticate user and return JWT tokens."""
    
    # Find user by username or email
    user = session.exec(
        select(User).where(
            (User.username == user_data.username) | (User.email == user_data.username)
        )
    ).first()
    
    if not user or not verify_password(user_data.password, user.password_hash):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect username or password",
            headers={"WWW-Authenticate": "Bearer"}
        )
    
    if not user.is_active:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="User account is disabled"
        )
    
    # Create tokens
    access_token_expires = timedelta(minutes=settings.access_token_expire_minutes)
    access_token = create_access_token(
        data={"sub": user.id},
        expires_delta=access_token_expires
    )
    refresh_token = create_refresh_token(data={"sub": user.id})
    
    # Create session record
    session_id = str(uuid.uuid4())
    session_record = UserSession(
        id=session_id,
        user_id=user.id,
        token_hash=hash_token(access_token),
        refresh_token_hash=hash_token(refresh_token),
        expires_at=datetime.now(timezone.utc) + timedelta(days=settings.refresh_token_expire_days),
        user_agent=request.headers.get("User-Agent"),
        ip_address=request.client.host if request.client else None
    )
    
    session.add(session_record)
    session.commit()
    
    return TokenResponse(
        access_token=access_token,
        refresh_token=refresh_token,
        token_type="bearer",
        expires_in=int(access_token_expires.total_seconds())
    )


@router.post("/refresh", response_model=TokenResponse)
async def refresh_access_token(
    refresh_data: RefreshTokenRequest,
    session: Annotated[Session, Depends(get_session)]
):
    """Refresh an access token using a refresh token."""
    
    try:
        # Verify refresh token
        payload = verify_token(refresh_data.refresh_token, token_type="refresh")
        user_id = payload.get("sub")
        
        if not user_id:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid refresh token"
            )
        
        # Check if refresh token exists in database
        refresh_token_hash = hash_token(refresh_data.refresh_token)
        session_record = session.exec(
            select(UserSession).where(
                (UserSession.user_id == user_id) & 
                (UserSession.refresh_token_hash == refresh_token_hash) &
                (UserSession.expires_at > datetime.now(timezone.utc))
            )
        ).first()
        
        if not session_record:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid or expired refresh token"
            )
        
        # Get user
        user = session.get(User, user_id)
        if not user or not user.is_active:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="User not found or disabled"
            )
        
        # Create new access token
        access_token_expires = timedelta(minutes=settings.access_token_expire_minutes)
        new_access_token = create_access_token(
            data={"sub": user_id},
            expires_delta=access_token_expires
        )
        
        # Update session with new token hash
        session_record.token_hash = hash_token(new_access_token)
        session_record.updated_at = datetime.now(timezone.utc)
        session.commit()
        
        return TokenResponse(
            access_token=new_access_token,
            refresh_token=refresh_data.refresh_token,  # Keep same refresh token
            token_type="bearer",
            expires_in=int(access_token_expires.total_seconds())
        )
        
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid refresh token"
        )


@router.post("/logout", response_model=LogoutResponse)
async def logout_user(
    refresh_data: RefreshTokenRequest,
    session: Annotated[Session, Depends(get_session)]
):
    """Logout user and invalidate refresh token."""
    
    try:
        # Verify refresh token
        payload = verify_token(refresh_data.refresh_token, token_type="refresh")
        user_id = payload.get("sub")
        
        if user_id:
            # Remove session record
            refresh_token_hash = hash_token(refresh_data.refresh_token)
            session_record = session.exec(
                select(UserSession).where(
                    (UserSession.user_id == user_id) & 
                    (UserSession.refresh_token_hash == refresh_token_hash)
                )
            ).first()
            
            if session_record:
                session.delete(session_record)
                session.commit()
    
    except Exception:
        # Even if token verification fails, return success for security
        pass
    
    return LogoutResponse()


@router.get("/me", response_model=UserProfileResponse)
async def get_current_user_profile(
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get current user's profile information."""
    return current_user


@router.put("/me", response_model=UserProfileResponse)
async def update_user_profile(
    profile_data: UserUpdateRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Update current user's profile information."""
    
    # Update user fields
    if profile_data.first_name is not None:
        current_user.first_name = profile_data.first_name
    if profile_data.last_name is not None:
        current_user.last_name = profile_data.last_name
    if profile_data.preferences is not None:
        current_user.preferences_json = json.dumps(profile_data.preferences)
    
    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(current_user)
    
    return current_user


@router.put("/change-password", response_model=SuccessResponse)
async def change_password(
    password_data: ChangePasswordRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Change user's password."""
    
    # Verify current password
    if not verify_password(password_data.current_password, current_user.password_hash):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Current password is incorrect"
        )
    
    # Update password
    current_user.password_hash = get_password_hash(password_data.new_password)
    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()
    
    # Invalidate all existing sessions for security
    existing_sessions = session.exec(
        select(UserSession).where(UserSession.user_id == current_user.id)
    ).all()
    
    for user_session in existing_sessions:
        session.delete(user_session)
    
    session.commit()
    
    return SuccessResponse(message="Password changed successfully. Please log in again.")