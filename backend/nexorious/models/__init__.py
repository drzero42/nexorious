"""
Database models for the Nexorious Game Collection Management Service.

This module contains all SQLModel definitions for the application.
"""

from .user import User, UserSession
from .platform import Platform, Storefront, PlatformStorefront
from .game import Game, GameAlias
from .user_game import UserGame, UserGamePlatform
from .tag import Tag, UserGameTag
from .wishlist import Wishlist
from .import_job import ImportJob
from .steam_import import SteamImportJob, SteamImportGame

__all__ = [
    "User",
    "UserSession",
    "Platform",
    "Storefront",
    "PlatformStorefront",
    "Game",
    "GameAlias",
    "UserGame",
    "UserGamePlatform",
    "Tag",
    "UserGameTag",
    "Wishlist",
    "ImportJob",
    "SteamImportJob",
    "SteamImportGame",
]