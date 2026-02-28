"""Tests for ExternalGame model."""
from app.models.external_game import ExternalGame
from app.models.user_game import OwnershipStatus


class TestExternalGameStoreUrl:
    """Tests for ExternalGame.store_url computed property."""

    def test_steam_store_url(self):
        game = ExternalGame(
            user_id="u1", storefront="steam", external_id="730", title="CS2"
        )
        assert game.store_url == "https://store.steampowered.com/app/730"

    def test_unknown_storefront_returns_none(self):
        game = ExternalGame(
            user_id="u1", storefront="gog", external_id="abc", title="Some Game"
        )
        assert game.store_url is None

    def test_psn_store_url_returns_none_when_not_implemented(self):
        game = ExternalGame(
            user_id="u1", storefront="playstation-store", external_id="PPSA01234_00", title="Game"
        )
        assert game.store_url is None


class TestExternalGameDefaults:
    """Tests for ExternalGame field defaults."""

    def test_defaults(self):
        game = ExternalGame(
            user_id="u1", storefront="steam", external_id="123", title="Game"
        )
        assert game.is_skipped is False
        assert game.is_available is True
        assert game.is_subscription is False
        assert game.playtime_hours == 0
        assert game.resolved_igdb_id is None
        assert game.ownership_status is None
        assert game.platform is None


class TestUserGamePlatformNewFields:
    """Tests for new fields on UserGamePlatform."""

    def test_external_game_id_defaults_to_none(self):
        from app.models.user_game import UserGamePlatform
        ugp = UserGamePlatform(user_game_id="ug1", platform="pc-windows", storefront="steam")
        assert ugp.external_game_id is None

    def test_sync_from_source_defaults_to_true(self):
        from app.models.user_game import UserGamePlatform
        ugp = UserGamePlatform(user_game_id="ug1", platform="pc-windows", storefront="steam")
        assert ugp.sync_from_source is True
