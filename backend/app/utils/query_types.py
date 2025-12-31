"""
TypedDict classes for aggregate query results.

These types provide type safety when using SQLModel's .mappings() method
to access aggregate query results as dictionaries instead of positional tuples.

Usage:
    from sqlalchemy import RowMapping
    from app.utils.query_types import PlatformUsageRow

    results: Sequence[RowMapping] = session.exec(query).mappings().all()
    for row in results:
        # Type-safe access via dictionary keys
        platform: str = row["platform"]
        usage_count: int = row["usage_count"]
"""

from typing import Optional, TypedDict


class PlatformUsageRow(TypedDict):
    """Row type for platform usage statistics query in platforms.py."""

    platform: str
    platform_name: str
    platform_display_name: str
    usage_count: int


class StorefrontUsageRow(TypedDict):
    """Row type for storefront usage statistics query in platforms.py."""

    storefront: str
    storefront_name: str
    storefront_display_name: str
    usage_count: int


class PlatformCountRow(TypedDict):
    """Row type for platform count query in user_games.py."""

    display_name: str
    count: int


class RatingCountRow(TypedDict):
    """Row type for rating distribution query in user_games.py."""

    personal_rating: Optional[float]
    count: int


class GenreCountRow(TypedDict):
    """Row type for genre statistics query in user_games.py."""

    genre: Optional[str]
    count: int


class PlatformMappingRow(TypedDict):
    """Row type for platform mapping query in darkadia.py."""

    original_platform_name: Optional[str]
    platform_name: Optional[str]
    game_count: int


class StorefrontMappingRow(TypedDict):
    """Row type for storefront mapping query in darkadia.py."""

    original_storefront_name: Optional[str]
    storefront_name: Optional[str]
    game_count: int
