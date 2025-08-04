"""
Steam Web API configuration endpoints for user Steam settings management.
"""

from fastapi import APIRouter, Depends, HTTPException, status
from sqlmodel import Session, select
from typing import Annotated, List
import json
import logging
from datetime import datetime, timezone
from rapidfuzz import fuzz

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.game import Game
from ..models.platform import Platform, Storefront
from ..models.user_game import UserGame, UserGamePlatform, OwnershipStatus
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
    SteamGameResponse,
    SteamLibraryImportRequest,
    SteamLibraryImportResponse,
    SteamGameImportResult
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
        return preferences.get("steam", {})
    except (json.JSONDecodeError, TypeError):
        return {}


def _update_user_steam_config(user: User, session: Session, steam_config: dict) -> None:
    """Update user's Steam configuration in preferences."""
    try:
        preferences = user.preferences.copy()
        preferences["steam"] = steam_config
        user.preferences_json = json.dumps(preferences)
        user.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(user)
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
    steam_config = _get_user_steam_config(current_user)
    
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
        
        _update_user_steam_config(current_user, session, steam_config)
        
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
                playtime_forever=game.playtime_forever,
                playtime_windows_forever=game.playtime_windows_forever,
                playtime_mac_forever=game.playtime_mac_forever,
                playtime_linux_forever=game.playtime_linux_forever,
                rtime_last_played=game.rtime_last_played,
                playtime_disconnected=game.playtime_disconnected,
                img_icon_url=game.img_icon_url,
                has_community_visible_stats=game.has_community_visible_stats
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


def _detect_platforms_from_steam_game(steam_game, platform_fallback: str = "pc-windows") -> List[str]:
    """Detect platforms based on Steam game playtime data."""
    detected_platforms = []
    
    # Check each platform based on playtime data
    if steam_game.playtime_windows_forever > 0:
        detected_platforms.append("pc-windows")
    
    if steam_game.playtime_mac_forever > 0:
        detected_platforms.append("pc-mac")  # Assuming this exists in seed data
    
    if steam_game.playtime_linux_forever > 0:
        detected_platforms.append("pc-linux")
    
    # If no platform-specific playtime, use fallback (most likely Windows)
    if not detected_platforms:
        detected_platforms.append(platform_fallback)
    
    return detected_platforms


def _find_best_game_match(steam_game_name: str, all_games: List[Game], fuzzy_threshold: float) -> tuple[Game | None, float]:
    """Find the best matching game using fuzzy string matching."""
    if not all_games:
        return None, 0.0
    
    best_match = None
    best_score = 0.0
    
    steam_name_lower = steam_game_name.lower().strip()
    
    for game in all_games:
        # Try exact match first
        if game.title.lower().strip() == steam_name_lower:
            return game, 1.0
        
        # Calculate fuzzy match scores
        ratio_score = fuzz.ratio(steam_name_lower, game.title.lower().strip()) / 100.0
        partial_score = fuzz.partial_ratio(steam_name_lower, game.title.lower().strip()) / 100.0
        token_sort_score = fuzz.token_sort_ratio(steam_name_lower, game.title.lower().strip()) / 100.0
        
        # Use the highest score
        max_score = max(ratio_score, partial_score, token_sort_score)
        
        if max_score > best_score:
            best_score = max_score
            best_match = game
    
    # Only return match if it meets the threshold
    if best_score >= fuzzy_threshold:
        return best_match, best_score
    
    return None, best_score


@router.post("/import-library", response_model=SteamLibraryImportResponse)
async def import_steam_library(
    import_request: SteamLibraryImportRequest,
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
):
    """Import user's Steam library into their game collection."""
    
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
        
        # Get Steam library
        steam_games = await steam_service.get_owned_games(steam_config["steam_id"])
        logger.info(f"Retrieved {len(steam_games)} games from Steam for user {current_user.id}")
        
        # Get all games from database for fuzzy matching
        all_games = session.exec(select(Game)).all()
        logger.info(f"Loaded {len(all_games)} games from database for matching")
        
        # Get Steam storefront
        steam_storefront = session.exec(select(Storefront).where(Storefront.name == "steam")).first()
        if not steam_storefront:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Steam storefront not found in database. Please run seed data."
            )
        
        # Initialize counters and results
        imported_count = 0
        skipped_count = 0
        failed_count = 0
        no_match_count = 0
        platform_breakdown = {}
        results = []
        
        for steam_game in steam_games:
            try:
                # Detect platforms for this game
                detected_platforms = _detect_platforms_from_steam_game(
                    steam_game, import_request.platform_fallback
                )
                
                # Update platform breakdown
                for platform_name in detected_platforms:
                    platform_breakdown[platform_name] = platform_breakdown.get(platform_name, 0) + 1
                
                # Find matching game in database
                matched_game, match_score = _find_best_game_match(
                    steam_game.name, all_games, import_request.fuzzy_threshold
                )
                
                if not matched_game:
                    # No match found
                    no_match_count += 1
                    results.append(SteamGameImportResult(
                        steam_appid=steam_game.appid,
                        steam_name=steam_game.name,
                        status="no_match",
                        reason="No matching game found in IGDB database",
                        detected_platforms=detected_platforms,
                        match_score=match_score
                    ))
                    continue
                
                # Check if user already has this game
                existing_user_game = session.exec(
                    select(UserGame).where(
                        UserGame.user_id == current_user.id,
                        UserGame.game_id == matched_game.id
                    )
                ).first()
                
                if existing_user_game and import_request.merge_strategy == "skip":
                    # Game already exists and we're skipping
                    skipped_count += 1
                    results.append(SteamGameImportResult(
                        steam_appid=steam_game.appid,
                        steam_name=steam_game.name,
                        status="skipped",
                        reason="Game already in collection (merge_strategy=skip)",
                        matched_game_id=matched_game.id,
                        matched_game_title=matched_game.title,
                        detected_platforms=detected_platforms,
                        match_score=match_score
                    ))
                    continue
                
                # Add game to collection or add missing platforms
                if not existing_user_game:
                    # Create new user game
                    new_user_game = UserGame(
                        user_id=current_user.id,
                        game_id=matched_game.id,
                        ownership_status=OwnershipStatus.OWNED
                    )
                    session.add(new_user_game)
                    session.flush()  # Get the ID
                    target_user_game = new_user_game
                else:
                    # Use existing user game
                    target_user_game = existing_user_game
                
                # Add platform associations
                platforms_added = []
                for platform_name in detected_platforms:
                    # Find platform in database
                    platform = session.exec(select(Platform).where(Platform.name == platform_name)).first()
                    if not platform:
                        logger.warning(f"Platform '{platform_name}' not found in database, skipping")
                        continue
                    
                    # Check if platform association already exists
                    existing_platform = session.exec(
                        select(UserGamePlatform).where(
                            UserGamePlatform.user_game_id == target_user_game.id,
                            UserGamePlatform.platform_id == platform.id,
                            UserGamePlatform.storefront_id == steam_storefront.id
                        )
                    ).first()
                    
                    if not existing_platform:
                        # Add platform association
                        user_game_platform = UserGamePlatform(
                            user_game_id=target_user_game.id,
                            platform_id=platform.id,
                            storefront_id=steam_storefront.id
                        )
                        session.add(user_game_platform)
                        platforms_added.append(platform_name)
                
                if platforms_added or not existing_user_game:
                    imported_count += 1
                    status_msg = "imported" if not existing_user_game else "platforms_added"
                    reason_msg = f"Added platforms: {', '.join(platforms_added)}" if existing_user_game else "Game imported successfully"
                else:
                    skipped_count += 1
                    status_msg = "skipped"
                    reason_msg = "All platforms already exist in collection"
                
                results.append(SteamGameImportResult(
                    steam_appid=steam_game.appid,
                    steam_name=steam_game.name,
                    status=status_msg,
                    reason=reason_msg,
                    matched_game_id=matched_game.id,
                    matched_game_title=matched_game.title,
                    detected_platforms=detected_platforms,
                    match_score=match_score
                ))
                
            except Exception as e:
                failed_count += 1
                logger.error(f"Failed to import Steam game '{steam_game.name}' (AppID: {steam_game.appid}): {str(e)}")
                results.append(SteamGameImportResult(
                    steam_appid=steam_game.appid,
                    steam_name=steam_game.name,
                    status="failed",
                    reason=f"Import error: {str(e)}",
                    detected_platforms=detected_platforms if 'detected_platforms' in locals() else []
                ))
        
        # Commit all changes
        session.commit()
        
        # Generate summary
        summary = f"Imported {imported_count} games, skipped {skipped_count}, failed {failed_count}, no match {no_match_count} out of {len(steam_games)} Steam games"
        
        logger.info(f"Steam library import completed for user {current_user.id}: {summary}")
        
        return SteamLibraryImportResponse(
            total_games=len(steam_games),
            imported_count=imported_count,
            skipped_count=skipped_count,
            failed_count=failed_count,
            no_match_count=no_match_count,
            platform_breakdown=platform_breakdown,
            results=results,
            import_summary=summary
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
        logger.error(f"Error importing Steam library for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to import Steam library"
        )