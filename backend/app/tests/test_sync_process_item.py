"""Tests for sync process_item task (Phases 4 and 5)."""
from unittest.mock import MagicMock
from app.models.user_game import OwnershipStatus, UserGamePlatform
from app.models.external_game import ExternalGame as ExternalGameModel


class TestOwnershipPrecedence:
    """Tests for _should_update_ownership helper."""

    def test_owned_not_overwritten_by_subscription(self):
        from app.worker.tasks.sync.process_item import _should_update_ownership
        assert _should_update_ownership(OwnershipStatus.OWNED, OwnershipStatus.SUBSCRIPTION) is False

    def test_subscription_overwritten_by_owned(self):
        from app.worker.tasks.sync.process_item import _should_update_ownership
        assert _should_update_ownership(OwnershipStatus.SUBSCRIPTION, OwnershipStatus.OWNED) is True

    def test_same_status_is_allowed(self):
        from app.worker.tasks.sync.process_item import _should_update_ownership
        assert _should_update_ownership(OwnershipStatus.OWNED, OwnershipStatus.OWNED) is True

    def test_no_longer_owned_overwritten_by_subscription(self):
        from app.worker.tasks.sync.process_item import _should_update_ownership
        assert _should_update_ownership(OwnershipStatus.NO_LONGER_OWNED, OwnershipStatus.SUBSCRIPTION) is True


class TestSyncToCollection:
    """Tests for Phase 5: _sync_external_game_to_collection."""

    def _make_eg(self, **kwargs) -> ExternalGameModel:
        defaults = dict(
            id="eg1", user_id="u1", storefront="steam",
            external_id="730", title="CS2",
            resolved_igdb_id=1234, playtime_hours=50,
            ownership_status=OwnershipStatus.OWNED,
            platform="pc-windows", is_available=True, is_skipped=False,
        )
        defaults.update(kwargs)
        return ExternalGameModel(**defaults)

    def test_links_existing_manual_entry(self):
        """Manual UserGamePlatform gets linked to ExternalGame and values updated."""
        from app.worker.tasks.sync.process_item import _sync_external_game_to_collection
        session = MagicMock()
        eg = self._make_eg()

        existing_ugp = MagicMock(spec=UserGamePlatform)
        existing_ugp.external_game_id = None
        existing_ugp.ownership_status = OwnershipStatus.OWNED
        existing_ugp.sync_from_source = True

        # First query (by external_game_id) returns None, second (manual lookup) returns ugp
        session.exec.return_value.first.side_effect = [None, existing_ugp]

        _sync_external_game_to_collection(session, eg)

        assert existing_ugp.external_game_id == "eg1"
        assert existing_ugp.hours_played == 50

    def test_respects_sync_from_source_false(self):
        """When sync_from_source=False, playtime on UserGamePlatform is not overwritten."""
        from app.worker.tasks.sync.process_item import _sync_external_game_to_collection
        session = MagicMock()
        eg = self._make_eg(playtime_hours=100)

        linked_ugp = MagicMock(spec=UserGamePlatform)
        linked_ugp.external_game_id = "eg1"
        linked_ugp.sync_from_source = False
        linked_ugp.hours_played = 42
        linked_ugp.ownership_status = OwnershipStatus.OWNED

        session.exec.return_value.first.return_value = linked_ugp

        _sync_external_game_to_collection(session, eg)

        assert linked_ugp.hours_played == 42

    def test_does_not_downgrade_owned_to_subscription(self):
        """Ownership precedence: OWNED is not overwritten by SUBSCRIPTION on sync."""
        from app.worker.tasks.sync.process_item import _sync_external_game_to_collection
        session = MagicMock()
        eg = self._make_eg(ownership_status=OwnershipStatus.SUBSCRIPTION)

        linked_ugp = MagicMock(spec=UserGamePlatform)
        linked_ugp.external_game_id = "eg1"
        linked_ugp.sync_from_source = True
        linked_ugp.hours_played = 10
        linked_ugp.ownership_status = OwnershipStatus.OWNED

        session.exec.return_value.first.return_value = linked_ugp

        _sync_external_game_to_collection(session, eg)

        assert linked_ugp.ownership_status == OwnershipStatus.OWNED

    def test_duplicate_external_id_does_not_re_link_already_claimed_ugp(self):
        """When the same game appears under two external IDs (e.g. Steam bundle + standalone),
        the second ExternalGame must not overwrite the UGP's existing external_game_id link."""
        from app.worker.tasks.sync.process_item import _sync_external_game_to_collection
        session = MagicMock()
        # eg2 has a different external_id but resolves to the same IGDB game as eg1
        eg = self._make_eg(id="eg2", external_id="9930", playtime_hours=60)

        already_claimed_ugp = MagicMock(spec=UserGamePlatform)
        already_claimed_ugp.external_game_id = "eg1"  # linked by the first ExternalGame
        already_claimed_ugp.sync_from_source = True
        already_claimed_ugp.hours_played = 50
        already_claimed_ugp.ownership_status = OwnershipStatus.OWNED

        # Lookup 1 (by eg.id="eg2") → None; Lookup 2 (by game identity) → already_claimed_ugp
        session.exec.return_value.first.side_effect = [None, already_claimed_ugp]

        _sync_external_game_to_collection(session, eg)

        # external_game_id must not be overwritten
        assert already_claimed_ugp.external_game_id == "eg1"
        # playtime should still be updated
        assert already_claimed_ugp.hours_played == 60
