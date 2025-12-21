"""
Database models for the Nexorious Game Collection Management Service.

This module contains all SQLModel definitions for the application.
"""

from .user import User, UserSession
from .platform import Platform, Storefront, PlatformStorefront
from .game import Game
from .user_game import UserGame, UserGamePlatform
from .tag import Tag, UserGameTag
from .wishlist import Wishlist
from .job import (
    Job,
    ReviewItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    ImportJobSubtype,
    ReviewItemStatus,
)
from .user_sync_config import UserSyncConfig, SyncFrequency
from .ignored_external_game import IgnoredExternalGame

__all__ = [
    "User",
    "UserSession",
    "Platform",
    "Storefront",
    "PlatformStorefront",
    "Game",
    "UserGame",
    "UserGamePlatform",
    "Tag",
    "UserGameTag",
    "Wishlist",
    "Job",
    "ReviewItem",
    "BackgroundJobType",
    "BackgroundJobSource",
    "BackgroundJobStatus",
    "BackgroundJobPriority",
    "ImportJobSubtype",
    "ReviewItemStatus",
    "UserSyncConfig",
    "SyncFrequency",
    "IgnoredExternalGame",
]