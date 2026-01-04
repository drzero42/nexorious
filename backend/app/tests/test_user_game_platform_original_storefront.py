"""Tests for original_storefront_name field on UserGamePlatform."""

from app.models.user_game import UserGamePlatform


def test_user_game_platform_has_original_storefront_name_field():
    """UserGamePlatform should have original_storefront_name field."""
    platform = UserGamePlatform(
        user_game_id="test-id",
        platform=None,
        storefront=None,
        original_platform_name="Unknown Platform",
        original_storefront_name="Unknown Storefront",
    )
    assert platform.original_storefront_name == "Unknown Storefront"


def test_user_game_platform_original_storefront_name_nullable():
    """original_storefront_name should be nullable."""
    platform = UserGamePlatform(
        user_game_id="test-id",
        platform="pc-windows",
        storefront="steam",
    )
    assert platform.original_storefront_name is None
