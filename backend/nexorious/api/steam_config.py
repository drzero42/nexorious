"""
Steam Web API configuration endpoints for user Steam settings management.
"""

from fastapi import APIRouter, Depends, HTTPException, status
from sqlmodel import Session, select
from typing import Annotated
import json
import logging
from datetime import datetime, timezone
from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..services.steam import create_steam_service, SteamAuthenticationError, SteamAPIError
from ..api.schemas.steam import (
    SteamConfigRequest,
    SteamConfigResponse,
    SteamVerificationRequest,
    SteamVerificationResponse,
    VanityUrlResolveRequest,
    VanityUrlResolveResponse,
    SteamUserInfoResponse,
    SteamLibraryResponse,
    SteamGameResponse
)
from ..api.schemas.common import SuccessResponse

router = APIRouter(prefix="/steam", tags=["Steam Configuration"])
logger = logging.getLogger(__name__)


def _mask_api_key(api_key: str) -> str:
    """Mask Steam API key for safe display (show first 8 and last 4 characters)."""
    if len(api_key) < 12:
        return "****"
    return f"{api_key[:8]}****{api_key[-4:]}"


def _get_user_steam_config(user: User) -> dict:
    """Get user's Steam configuration from preferences."""
    try:
        preferences = user.preferences
        steam_config = preferences.get("steam", {})
        logger.debug(f"Retrieved Steam config for user {user.id}: has_api_key={bool(steam_config.get('web_api_key'))}, has_steam_id={bool(steam_config.get('steam_id'))}, is_verified={steam_config.get('is_verified', False)}")
        return steam_config
    except (json.JSONDecodeError, TypeError) as e:
        logger.error(f"Error parsing preferences for user {user.id}: {str(e)}")
        return {}


def _update_user_steam_config(user: User, session: Session, steam_config: dict) -> None:
    """Update user's Steam configuration in preferences."""
    try:
        logger.debug(f"Starting Steam config update for user {user.id}")
        preferences = user.preferences.copy()
        
        preferences["steam"] = steam_config
        user.preferences_json = json.dumps(preferences)
        user.updated_at = datetime.now(timezone.utc)
        
        logger.debug(f"About to commit Steam config changes for user {user.id}")
        session.add(user)  # Explicitly add user to session
        session.commit()
        logger.debug(f"Database commit completed for user {user.id}")
        
        session.refresh(user)
        logger.debug(f"User refreshed from database for user {user.id}")
    except Exception as e:
        logger.error(f"Error updating Steam config for user {user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to update Steam configuration"
        )


@router.get("/config", response_model=SteamConfigResponse)
async def get_steam_config(
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get current user's Steam configuration (without exposing full API key)."""
    logger.debug(f"GET /api/steam/config called for user {current_user.id} ({current_user.username})")
    steam_config = _get_user_steam_config(current_user)
    logger.debug(f"Raw preferences_json for user {current_user.id}: {current_user.preferences_json[:100] + '...' if len(current_user.preferences_json) > 100 else current_user.preferences_json}")
    
    has_api_key = bool(steam_config.get("web_api_key"))
    api_key_masked = None
    
    if has_api_key:
        api_key_masked = _mask_api_key(steam_config["web_api_key"])
    
    return SteamConfigResponse(
        has_api_key=has_api_key,
        api_key_masked=api_key_masked,
        steam_id=steam_config.get("steam_id"),
        is_verified=steam_config.get("is_verified", False),
        configured_at=datetime.fromisoformat(steam_config["configured_at"]) if steam_config.get("configured_at") else None
    )


@router.put("/config", response_model=SteamConfigResponse)
async def set_steam_config(
    config_data: SteamConfigRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Set or update user's Steam Web API configuration."""
    logger.debug(f"Steam config save requested for user {current_user.id} ({current_user.username})")
    logger.debug(f"API key provided: {bool(config_data.web_api_key)}, Steam ID: {config_data.steam_id}")
    
    # Verify the API key is valid before saving
    try:
        steam_service = create_steam_service(config_data.web_api_key)
        is_valid = await steam_service.verify_api_key()
        
        if not is_valid:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Invalid Steam Web API key. Please check your key and try again."
            )
        
        # If Steam ID is provided, verify it exists and is accessible
        steam_user_info = None
        if config_data.steam_id:
            if not steam_service.validate_steam_id(config_data.steam_id):
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Invalid Steam ID format. Steam ID must be a 17-digit number."
                )
            
            steam_user_info = await steam_service.get_user_info(config_data.steam_id)
            if not steam_user_info:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Steam ID not found or profile is private. Please check your Steam ID and profile visibility."
                )
        
        # Save configuration to user preferences
        steam_config = {
            "web_api_key": config_data.web_api_key,
            "steam_id": config_data.steam_id,
            "is_verified": True,
            "configured_at": datetime.now(timezone.utc).isoformat()
        }
        
        logger.debug(f"Saving Steam config to database for user {current_user.id}")
        _update_user_steam_config(current_user, session, steam_config)
        logger.debug(f"Steam config saved successfully for user {current_user.id}")
        
        logger.info(f"Steam configuration updated for user {current_user.id}")
        
        return SteamConfigResponse(
            has_api_key=True,
            api_key_masked=_mask_api_key(config_data.web_api_key),
            steam_id=config_data.steam_id,
            is_verified=True,
            configured_at=datetime.now(timezone.utc)
        )
        
    except HTTPException:
        # Re-raise HTTPExceptions (like 400 Bad Request) without modification
        raise
    except SteamAuthenticationError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Steam authentication failed: {str(e)}"
        )
    except SteamAPIError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Steam API error: {str(e)}"
        )
    except Exception as e:
        logger.error(f"Error setting Steam config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to configure Steam settings"
        )


@router.delete("/config", response_model=SuccessResponse)
async def delete_steam_config(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Remove user's Steam Web API configuration."""
    logger.debug(f"DELETE /api/steam/config called for user {current_user.id} ({current_user.username})")
    
    try:
        # Update user preferences to remove Steam configuration
        preferences = current_user.preferences.copy()
        if "steam" in preferences:
            del preferences["steam"]
        
        current_user.preferences_json = json.dumps(preferences)
        current_user.updated_at = datetime.now(timezone.utc)
        session.commit()
        
        logger.info(f"Steam configuration removed for user {current_user.id}")
        
        return SuccessResponse(message="Steam configuration removed successfully")
        
    except Exception as e:
        logger.error(f"Error removing Steam config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to remove Steam configuration"
        )


@router.post("/verify", response_model=SteamVerificationResponse)
async def verify_steam_config(
    verification_data: SteamVerificationRequest,
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Verify Steam Web API key and optionally Steam ID without saving."""
    
    try:
        steam_service = create_steam_service(verification_data.web_api_key)
        
        # Verify API key
        is_valid = await steam_service.verify_api_key()
        
        if not is_valid:
            return SteamVerificationResponse(
                is_valid=False,
                error_message="Invalid Steam Web API key"
            )
        
        # If Steam ID is provided, verify it
        steam_user_info = None
        if verification_data.steam_id:
            if not steam_service.validate_steam_id(verification_data.steam_id):
                return SteamVerificationResponse(
                    is_valid=False,
                    error_message="Invalid Steam ID format"
                )
            
            user_info = await steam_service.get_user_info(verification_data.steam_id)
            if not user_info:
                return SteamVerificationResponse(
                    is_valid=False,
                    error_message="Steam ID not found or profile is private"
                )
            
            steam_user_info = {
                "steam_id": user_info.steam_id,
                "persona_name": user_info.persona_name,
                "profile_url": user_info.profile_url,
                "avatar": user_info.avatar,
                "avatar_medium": user_info.avatar_medium,
                "avatar_full": user_info.avatar_full
            }
        
        return SteamVerificationResponse(
            is_valid=True,
            steam_user_info=steam_user_info
        )
        
    except SteamAuthenticationError as e:
        return SteamVerificationResponse(
            is_valid=False,
            error_message=f"Authentication failed: {str(e)}"
        )
    except SteamAPIError as e:
        return SteamVerificationResponse(
            is_valid=False,
            error_message=f"Steam API error: {str(e)}"
        )
    except Exception as e:
        logger.error(f"Error verifying Steam config for user {current_user.id}: {str(e)}")
        return SteamVerificationResponse(
            is_valid=False,
            error_message="Verification failed due to an unexpected error"
        )


@router.post("/resolve-vanity", response_model=VanityUrlResolveResponse)
async def resolve_vanity_url(
    vanity_data: VanityUrlResolveRequest,
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Resolve Steam vanity URL to Steam ID."""
    
    # User needs to have Steam configured to use this endpoint
    steam_config = _get_user_steam_config(current_user)
    
    if not steam_config.get("web_api_key"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Please configure your Steam Web API key first"
        )
    
    try:
        steam_service = create_steam_service(steam_config["web_api_key"])
        steam_id = await steam_service.resolve_vanity_url(vanity_data.vanity_url)
        
        if steam_id:
            return VanityUrlResolveResponse(
                success=True,
                steam_id=steam_id
            )
        else:
            return VanityUrlResolveResponse(
                success=False,
                error_message="Vanity URL not found or user does not exist"
            )
            
    except Exception as e:
        logger.error(f"Error resolving vanity URL for user {current_user.id}: {str(e)}")
        return VanityUrlResolveResponse(
            success=False,
            error_message="Failed to resolve vanity URL"
        )


@router.get("/library", response_model=SteamLibraryResponse)
async def get_steam_library(
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get user's Steam library using their configured Steam settings."""
    
    steam_config = _get_user_steam_config(current_user)
    
    if not steam_config.get("web_api_key"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Please configure your Steam Web API key first"
        )
    
    if not steam_config.get("steam_id"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Please configure your Steam ID first"
        )
    
    try:
        steam_service = create_steam_service(steam_config["web_api_key"])
        
        # Get user info
        user_info = await steam_service.get_user_info(steam_config["steam_id"])
        if not user_info:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Steam profile not found or is private"
            )
        
        # Get owned games
        games = await steam_service.get_owned_games(steam_config["steam_id"])
        
        # Convert to response format
        game_responses = [
            SteamGameResponse(
                appid=game.appid,
                name=game.name,
                img_icon_url=game.img_icon_url
            )
            for game in games
        ]
        
        steam_user_response = SteamUserInfoResponse(
            steam_id=user_info.steam_id,
            persona_name=user_info.persona_name,
            profile_url=user_info.profile_url,
            avatar=user_info.avatar,
            avatar_medium=user_info.avatar_medium,
            avatar_full=user_info.avatar_full,
            persona_state=user_info.persona_state,
            community_visibility_state=user_info.community_visibility_state,
            profile_state=user_info.profile_state,
            last_logoff=user_info.last_logoff
        )
        
        logger.info(f"Retrieved Steam library for user {current_user.id}: {len(games)} games")
        
        return SteamLibraryResponse(
            total_games=len(games),
            games=game_responses,
            steam_user_info=steam_user_response
        )
        
    except SteamAuthenticationError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Steam authentication failed: {str(e)}"
        )
    except SteamAPIError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Steam API error: {str(e)}"
        )
    except Exception as e:
        logger.error(f"Error getting Steam library for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve Steam library"
        )






