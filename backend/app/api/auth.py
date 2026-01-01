"""
Authentication endpoints for user registration, login, and session management.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Request, UploadFile, File
from sqlmodel import Session, select
from datetime import timedelta, datetime, timezone
import json
import uuid
from typing import Annotated
import logging
import tempfile
import shutil
from pathlib import Path

from ..core.database import get_session
from ..core.security import (
    verify_password, 
    get_password_hash, 
    create_access_token, 
    create_refresh_token,
    verify_token,
    get_current_user,
    get_current_admin_user,
    hash_token
)
from ..core.config import settings
from ..models.user import User, UserSession
from ..models.user_game import UserGame
from ..models.tag import Tag
from ..models.wishlist import Wishlist
from ..models.job import Job, BackgroundJobType
from ..schemas.auth import (
    UserRegisterRequest,
    UserLoginRequest,
    TokenResponse,
    RefreshTokenRequest,
    UserProfileResponse,
    UserUpdateRequest,
    ChangePasswordRequest,
    ChangeUsernameRequest,
    UsernameAvailabilityResponse,
    LogoutResponse,
    SetupStatusResponse,
    SetupRestoreResponse,
    InitialAdminSetupRequest,
    AdminUserCreateRequest,
    AdminUserUpdateRequest,
    AdminPasswordResetRequest,
    AdminUserResponse,
    UserDeletionImpactResponse
)
from ..schemas.common import SuccessResponse

router = APIRouter(prefix="/auth", tags=["Authentication"])
logger = logging.getLogger(__name__)


@router.get("/setup/status", response_model=SetupStatusResponse)
async def get_setup_status(session: Annotated[Session, Depends(get_session)]):
    """Check if initial admin setup is needed."""
    # Check if any users exist in the database
    user_count = session.exec(select(User)).first()
    return SetupStatusResponse(needs_setup=user_count is None)


@router.post("/setup/admin", response_model=UserProfileResponse, status_code=status.HTTP_201_CREATED)
async def create_initial_admin(
    admin_data: InitialAdminSetupRequest,
    session: Annotated[Session, Depends(get_session)]
):
    """Create initial admin user. Only works when no users exist."""
    
    # Check if any users already exist
    existing_user = session.exec(select(User)).first()
    if existing_user:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Initial admin setup is not needed. Users already exist."
        )
    
    # Check if username is already taken (shouldn't happen since no users exist, but for safety)
    username_check = session.exec(
        select(User).where(User.username == admin_data.username)
    ).first()
    
    if username_check:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Username already taken"
        )
    
    # Create initial admin user
    hashed_password = get_password_hash(admin_data.password)
    admin_user = User(
        username=admin_data.username,
        password_hash=hashed_password,
        is_admin=True  # First user is automatically admin
    )
    
    session.add(admin_user)
    session.commit()
    session.refresh(admin_user)
    
    # Load seed data automatically after creating initial admin
    try:
        from ..seed_data.seeder import seed_all_official_data
        
        logger.info("Loading seed data for initial setup...")
        seed_result = seed_all_official_data(session, version="1.0.0")
        logger.info(f"Seed data loaded successfully: {seed_result}")
    except Exception as e:
        logger.error(f"Error loading seed data during initial setup: {str(e)}")
        # Don't fail the admin creation if seed data fails
        # Admin can manually load seed data later
    
    return admin_user


@router.post("/setup/restore", response_model=SetupRestoreResponse)
async def restore_from_backup_setup(
    file: Annotated[UploadFile, File(description="Backup archive file (.tar.gz)")],
    session: Annotated[Session, Depends(get_session)]
):
    """Restore from backup during initial setup. Only works when no users exist."""
    from ..services.backup_service import backup_service

    # Check if any users already exist
    existing_user = session.exec(select(User)).first()
    if existing_user:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Setup has already been completed. Users already exist."
        )

    # Validate file format
    if not file.filename or not file.filename.endswith(".tar.gz"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid file format. Expected .tar.gz"
        )

    # Save uploaded file to temp location
    with tempfile.NamedTemporaryFile(delete=False, suffix=".tar.gz") as tmp:
        content = await file.read()
        tmp.write(content)
        tmp_path = Path(tmp.name)

    try:
        # Validate backup archive with checksum verification
        backup_service.validate_backup_archive(tmp_path, verify_checksums=True)

        # Close the session before restore - restore will terminate all DB connections
        session.close()

        # Move to backups dir with generated ID
        backup_id = backup_service.generate_backup_id()
        dest_path = backup_service.get_backup_path(backup_id)
        shutil.move(str(tmp_path), str(dest_path))

        # Restore without pre-restore backup (database is empty)
        # Use a placeholder admin_user_id since we don't have one yet
        backup_service.restore_backup(
            backup_id=backup_id,
            admin_user_id="setup-restore",
            admin_session_data=None,
            skip_prerestore=True,
        )

        return SetupRestoreResponse(
            success=True,
            message="Backup restored successfully. Please log in with your restored credentials."
        )

    except ValueError as e:
        # Clean up temp file if it still exists
        if tmp_path.exists():
            tmp_path.unlink()
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except RuntimeError as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )


@router.post("/register", response_model=UserProfileResponse, status_code=status.HTTP_201_CREATED)
async def register_user(
    user_data: UserRegisterRequest,
    session: Annotated[Session, Depends(get_session)]
):
    """Register a new user."""
    
    # Check if user already exists
    existing_user = session.exec(
        select(User).where(User.username == user_data.username)
    ).first()
    
    if existing_user:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Username already taken"
        )
    
    # Create new user
    hashed_password = get_password_hash(user_data.password)
    new_user = User(
        username=user_data.username,
        password_hash=hashed_password
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
    
    # Find user by username
    user = session.exec(
        select(User).where(User.username == user_data.username)
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


@router.post("/logout", response_model=LogoutResponse)
async def logout_user(
    refresh_data: RefreshTokenRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Logout user and invalidate refresh token."""
    
    logger.info(f"User logout initiated for user_id: {current_user.id}, username: {current_user.username}")
    
    try:
        # Verify refresh token matches the authenticated user
        payload = verify_token(refresh_data.refresh_token, token_type="refresh")
        token_user_id = payload.get("sub")
        
        # Ensure the refresh token belongs to the authenticated user
        if token_user_id != current_user.id:
            logger.warning(f"Logout attempt with mismatched refresh token. Authenticated user: {current_user.id}, token user: {token_user_id}")
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Invalid refresh token for authenticated user"
            )
        
        # Remove session record using both user ID and refresh token hash
        refresh_token_hash = hash_token(refresh_data.refresh_token)
        session_record = session.exec(
            select(UserSession).where(
                (UserSession.user_id == current_user.id) & 
                (UserSession.refresh_token_hash == refresh_token_hash)
            )
        ).first()
        
        if session_record:
            session.delete(session_record)
            session.commit()
            logger.info(f"Session successfully invalidated for user_id: {current_user.id}")
        else:
            logger.warning(f"Session not found for logout request. User: {current_user.id}, refresh_token_hash: {refresh_token_hash[:16]}...")
    
    except HTTPException:
        # Re-raise HTTP exceptions (like mismatched refresh token)
        raise
    except Exception as e:
        # Log other errors but still return success for security
        logger.error(f"Error during logout for user_id: {current_user.id}: {str(e)}")
    
    logger.info(f"Logout completed for user_id: {current_user.id}")
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
    
    # Check if new password is different from current password
    if password_data.current_password == password_data.new_password:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="New password must be different from current password"
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


@router.get("/username/check/{username}", response_model=UsernameAvailabilityResponse)
async def check_username_availability(
    username: str,
    session: Annotated[Session, Depends(get_session)]
):
    """Check if a username is available."""
    
    # Validate username format (same as registration validation)
    if len(username) < 3 or len(username) > 100:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Username must be between 3 and 100 characters"
        )
    
    # Check if username exists
    existing_user = session.exec(
        select(User).where(User.username == username)
    ).first()
    
    return UsernameAvailabilityResponse(
        available=existing_user is None,
        username=username
    )


@router.put("/username", response_model=UserProfileResponse)
async def change_username(
    username_data: ChangeUsernameRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Change user's username."""
    
    # Check if the new username is the same as current
    if username_data.new_username == current_user.username:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="New username must be different from current username"
        )
    
    # Check if username is already taken
    existing_user = session.exec(
        select(User).where(User.username == username_data.new_username)
    ).first()
    
    if existing_user:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Username already taken"
        )
    
    # Update username
    current_user.username = username_data.new_username
    current_user.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(current_user)
    
    return current_user


# Admin-only endpoints

@router.post("/admin/users", response_model=AdminUserResponse, status_code=status.HTTP_201_CREATED)
async def admin_create_user(
    user_data: AdminUserCreateRequest,
    current_admin: Annotated[User, Depends(get_current_admin_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Admin endpoint to create a new user."""
    
    # Check if user already exists
    existing_user = session.exec(
        select(User).where(User.username == user_data.username)
    ).first()
    
    if existing_user:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Username already taken"
        )
    
    # Create new user
    hashed_password = get_password_hash(user_data.password)
    new_user = User(
        username=user_data.username,
        password_hash=hashed_password,
        is_admin=user_data.is_admin
    )
    
    session.add(new_user)
    session.commit()
    session.refresh(new_user)
    
    return new_user


@router.get("/admin/users", response_model=list[AdminUserResponse])
async def admin_list_users(
    current_admin: Annotated[User, Depends(get_current_admin_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Admin endpoint to list all users."""
    
    users = session.exec(select(User)).all()
    return users


@router.get("/admin/users/{user_id}", response_model=AdminUserResponse)
async def admin_get_user(
    user_id: str,
    current_admin: Annotated[User, Depends(get_current_admin_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Admin endpoint to get a specific user by ID."""
    
    user = session.get(User, user_id)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    return user


@router.put("/admin/users/{user_id}", response_model=AdminUserResponse)
async def admin_update_user(
    user_id: str,
    user_data: AdminUserUpdateRequest,
    current_admin: Annotated[User, Depends(get_current_admin_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Admin endpoint to update a user."""
    
    user = session.get(User, user_id)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Prevent admin from deactivating themselves
    if user.id == current_admin.id and user_data.is_active is False:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot deactivate your own account"
        )
    
    # Prevent admin from removing their own admin privileges
    if user.id == current_admin.id and user_data.is_admin is False:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot remove your own admin privileges"
        )
    
    # Check if new username is already taken (if username is being updated)
    if user_data.username and user_data.username != user.username:
        existing_user = session.exec(
            select(User).where(User.username == user_data.username)
        ).first()
        
        if existing_user:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Username already taken"
            )
    
    # Update user fields
    if user_data.username is not None:
        user.username = user_data.username
    if user_data.is_active is not None:
        user.is_active = user_data.is_active
    if user_data.is_admin is not None:
        user.is_admin = user_data.is_admin
    
    user.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(user)
    
    # If user was deactivated, invalidate all their sessions
    if user_data.is_active is False:
        existing_sessions = session.exec(
            select(UserSession).where(UserSession.user_id == user.id)
        ).all()
        
        for user_session in existing_sessions:
            session.delete(user_session)
        
        session.commit()
    
    return user


@router.put("/admin/users/{user_id}/password", response_model=SuccessResponse)
async def admin_reset_user_password(
    user_id: str,
    password_data: AdminPasswordResetRequest,
    current_admin: Annotated[User, Depends(get_current_admin_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Admin endpoint to reset a user's password."""
    
    user = session.get(User, user_id)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Update password
    user.password_hash = get_password_hash(password_data.new_password)
    user.updated_at = datetime.now(timezone.utc)
    session.commit()
    
    # Invalidate all existing sessions for security
    existing_sessions = session.exec(
        select(UserSession).where(UserSession.user_id == user.id)
    ).all()
    
    for user_session in existing_sessions:
        session.delete(user_session)
    
    session.commit()
    
    return SuccessResponse(message="Password reset successfully. User will need to log in again.")


@router.get("/admin/users/{user_id}/deletion-impact", response_model=UserDeletionImpactResponse)
async def admin_get_user_deletion_impact(
    user_id: str,
    current_admin: Annotated[User, Depends(get_current_admin_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Admin endpoint to preview the impact of deleting a user."""
    
    user = session.get(User, user_id)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Prevent admin from getting deletion impact for themselves
    if user.id == current_admin.id:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot delete your own account"
        )
    
    # Count user's data
    total_games = len(session.exec(select(UserGame).where(UserGame.user_id == user_id)).all())
    total_tags = len(session.exec(select(Tag).where(Tag.user_id == user_id)).all())
    total_wishlist_items = len(session.exec(select(Wishlist).where(Wishlist.user_id == user_id)).all())
    total_import_jobs = len(session.exec(select(Job).where(Job.user_id == user_id, Job.job_type == BackgroundJobType.IMPORT)).all())
    total_sessions = len(session.exec(select(UserSession).where(UserSession.user_id == user_id)).all())
    
    return UserDeletionImpactResponse(
        user_id=user.id,
        username=user.username,
        total_games=total_games,
        total_tags=total_tags,
        total_wishlist_items=total_wishlist_items,
        total_import_jobs=total_import_jobs,
        total_sessions=total_sessions
    )


@router.delete("/admin/users/{user_id}", response_model=SuccessResponse)
async def admin_delete_user(
    user_id: str,
    current_admin: Annotated[User, Depends(get_current_admin_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Admin endpoint to delete a user account."""
    
    user = session.get(User, user_id)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User not found"
        )
    
    # Prevent admin from deleting themselves
    if user.id == current_admin.id:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Cannot delete your own account"
        )
    
    # Manually delete related records first (since SQLModel doesn't handle cascade)
    
    # Delete user games (collection entries)
    user_games = session.exec(select(UserGame).where(UserGame.user_id == user_id)).all()
    for user_game in user_games:
        session.delete(user_game)
    
    # Delete user tags
    user_tags = session.exec(select(Tag).where(Tag.user_id == user_id)).all()
    for tag in user_tags:
        session.delete(tag)
    
    # Delete wishlist items
    wishlist_items = session.exec(select(Wishlist).where(Wishlist.user_id == user_id)).all()
    for wishlist_item in wishlist_items:
        session.delete(wishlist_item)
    
    # Delete import jobs (unified Job model filtered by IMPORT type)
    import_jobs = session.exec(select(Job).where(Job.user_id == user_id, Job.job_type == BackgroundJobType.IMPORT)).all()
    for import_job in import_jobs:
        session.delete(import_job)
    
    # Delete user sessions
    user_sessions = session.exec(select(UserSession).where(UserSession.user_id == user_id)).all()
    for user_session in user_sessions:
        session.delete(user_session)
    
    # Delete user
    session.delete(user)
    session.commit()
    
    return SuccessResponse(message="User and all associated data deleted successfully")