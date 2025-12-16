"""Tests for the matching service."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.services.matching import (
    MatchingService,
    MatchResult,
    MatchStatus,
    MatchSource,
    IGDBCandidate,
    MatchRequest,
    BatchMatchResult,
)
from app.services.igdb.models import GameMetadata


class TestMatchModels:
    """Test matching service models."""

    def test_igdb_candidate_to_dict(self):
        """IGDBCandidate can be serialized to dict."""
        candidate = IGDBCandidate(
            igdb_id=123,
            name="Test Game",
            first_release_date="2023-01-15",
            cover_url="https://example.com/cover.jpg",
            summary="A test game",
            platforms=["PC", "PlayStation 5"],
            confidence_score=0.95,
        )

        result = candidate.to_dict()

        assert result["igdb_id"] == 123
        assert result["name"] == "Test Game"
        assert result["confidence_score"] == 0.95
        assert "PC" in result["platforms"]

    def test_match_result_is_matched_true(self):
        """MatchResult.is_matched returns True for MATCHED status."""
        result = MatchResult(
            source_title="Test",
            status=MatchStatus.MATCHED,
            igdb_id=123,
        )
        assert result.is_matched is True

    def test_match_result_is_matched_already_matched(self):
        """MatchResult.is_matched returns True for ALREADY_MATCHED status."""
        result = MatchResult(
            source_title="Test",
            status=MatchStatus.ALREADY_MATCHED,
        )
        assert result.is_matched is True

    def test_match_result_is_matched_false(self):
        """MatchResult.is_matched returns False for other statuses."""
        result = MatchResult(
            source_title="Test",
            status=MatchStatus.NEEDS_REVIEW,
        )
        assert result.is_matched is False

    def test_match_result_needs_review(self):
        """MatchResult.needs_review works correctly."""
        review_result = MatchResult(
            source_title="Test",
            status=MatchStatus.NEEDS_REVIEW,
        )
        matched_result = MatchResult(
            source_title="Test",
            status=MatchStatus.MATCHED,
        )

        assert review_result.needs_review is True
        assert matched_result.needs_review is False

    def test_match_result_to_dict(self):
        """MatchResult can be serialized to dict."""
        candidate = IGDBCandidate(igdb_id=123, name="Test Game")
        result = MatchResult(
            source_title="Test",
            status=MatchStatus.MATCHED,
            match_source=MatchSource.TITLE_SEARCH_AUTO,
            igdb_id=123,
            igdb_title="Test Game",
            confidence_score=0.9,
            candidates=[candidate],
        )

        data = result.to_dict()

        assert data["source_title"] == "Test"
        assert data["status"] == "matched"
        assert data["match_source"] == "title_search_auto"
        assert data["igdb_id"] == 123
        assert len(data["candidates"]) == 1

    def test_batch_match_result_add_result(self):
        """BatchMatchResult tracks results and counts correctly."""
        batch = BatchMatchResult()

        batch.add_result(MatchResult(source_title="A", status=MatchStatus.MATCHED))
        batch.add_result(MatchResult(source_title="B", status=MatchStatus.NEEDS_REVIEW))
        batch.add_result(MatchResult(source_title="C", status=MatchStatus.NO_MATCH))
        batch.add_result(MatchResult(source_title="D", status=MatchStatus.ERROR))
        batch.add_result(MatchResult(source_title="E", status=MatchStatus.ALREADY_MATCHED))

        assert batch.total_processed == 5
        assert batch.matched == 1
        assert batch.needs_review == 1
        assert batch.no_match == 1
        assert batch.errors == 1
        assert batch.already_matched == 1

    def test_batch_match_result_success_rate(self):
        """BatchMatchResult calculates success rate correctly."""
        batch = BatchMatchResult()

        batch.add_result(MatchResult(source_title="A", status=MatchStatus.MATCHED))
        batch.add_result(MatchResult(source_title="B", status=MatchStatus.MATCHED))
        batch.add_result(MatchResult(source_title="C", status=MatchStatus.ALREADY_MATCHED))
        batch.add_result(MatchResult(source_title="D", status=MatchStatus.NEEDS_REVIEW))

        # 3 matched out of 4
        assert batch.success_rate == 0.75

    def test_batch_match_result_success_rate_zero_processed(self):
        """BatchMatchResult returns 0 success rate when nothing processed."""
        batch = BatchMatchResult()
        assert batch.success_rate == 0.0

    def test_match_request_default_metadata(self):
        """MatchRequest initializes empty metadata dict."""
        request = MatchRequest(
            source_title="Test",
            source_platform="steam",
        )
        assert request.source_metadata == {}


class TestMatchingServiceIntegration:
    """Integration tests for MatchingService with mocked dependencies."""

    @pytest.fixture
    def mock_session(self):
        """Create a mock database session."""
        return MagicMock()

    @pytest.fixture
    def mock_igdb_service(self):
        """Create a mock IGDB service."""
        return MagicMock()

    @pytest.mark.asyncio
    async def test_match_by_igdb_id_success(self, mock_session, mock_igdb_service):
        """When IGDB ID is provided and valid, match succeeds."""
        # Mock IGDB service response
        mock_igdb_service.get_game_by_id = AsyncMock(
            return_value=GameMetadata(
                igdb_id=123,
                title="The Witcher 3",
            )
        )

        service = MatchingService(mock_session, mock_igdb_service)
        request = MatchRequest(
            source_title="Witcher 3",
            source_platform="nexorious",
            igdb_id=123,
        )

        # Mock the local DB lookup to return None (not found locally)
        with patch(
            "app.services.matching.service.lookup_by_igdb_id",
            new_callable=AsyncMock,
            return_value=None,
        ):
            result = await service.match_game(request)

        assert result.status == MatchStatus.MATCHED
        assert result.match_source == MatchSource.IGDB_ID_PROVIDED
        assert result.igdb_id == 123
        assert result.igdb_title == "The Witcher 3"

    @pytest.mark.asyncio
    async def test_match_by_title_auto_match(self, mock_session, mock_igdb_service):
        """High confidence title match results in auto-match."""
        # Mock IGDB search results
        mock_igdb_service.search_games = AsyncMock(
            return_value=[
                GameMetadata(igdb_id=456, title="Elden Ring"),
            ]
        )

        service = MatchingService(mock_session, mock_igdb_service, auto_match_threshold=0.85)
        request = MatchRequest(
            source_title="Elden Ring",
            source_platform="steam",
        )

        result = await service.match_game(request)

        assert result.status == MatchStatus.MATCHED
        assert result.match_source == MatchSource.TITLE_SEARCH_AUTO
        assert result.igdb_id == 456

    @pytest.mark.asyncio
    async def test_match_by_title_needs_review(self, mock_session, mock_igdb_service):
        """Low confidence title match needs review."""
        # Mock IGDB search results with a different title
        mock_igdb_service.search_games = AsyncMock(
            return_value=[
                GameMetadata(igdb_id=789, title="Some Very Different Game Name"),
            ]
        )

        service = MatchingService(mock_session, mock_igdb_service, auto_match_threshold=0.85)
        request = MatchRequest(
            source_title="Test Game",
            source_platform="darkadia",
        )

        result = await service.match_game(request)

        assert result.status == MatchStatus.NEEDS_REVIEW
        assert result.match_source == MatchSource.TITLE_SEARCH_REVIEW
        assert result.candidates is not None and len(result.candidates) > 0

    @pytest.mark.asyncio
    async def test_match_by_title_no_results(self, mock_session, mock_igdb_service):
        """No IGDB results returns NO_MATCH status."""
        mock_igdb_service.search_games = AsyncMock(return_value=[])

        service = MatchingService(mock_session, mock_igdb_service)
        request = MatchRequest(
            source_title="xyznonexistentgame123",
            source_platform="steam",
        )

        result = await service.match_game(request)

        assert result.status == MatchStatus.NO_MATCH

    @pytest.mark.asyncio
    async def test_match_batch(self, mock_session, mock_igdb_service):
        """Batch matching processes all requests."""
        # Mock different responses for different searches
        async def search_side_effect(query, limit=5, fuzzy_threshold=0.6):
            if "Elden" in query:
                return [GameMetadata(igdb_id=1, title="Elden Ring")]
            elif "Witcher" in query:
                return [GameMetadata(igdb_id=2, title="The Witcher 3")]
            return []

        mock_igdb_service.search_games = AsyncMock(side_effect=search_side_effect)

        service = MatchingService(mock_session, mock_igdb_service)
        requests = [
            MatchRequest(source_title="Elden Ring", source_platform="steam"),
            MatchRequest(source_title="The Witcher 3", source_platform="steam"),
            MatchRequest(source_title="Unknown Game", source_platform="steam"),
        ]

        batch_result = await service.match_batch(requests)

        assert batch_result.total_processed == 3
        assert batch_result.matched == 2
        assert batch_result.no_match == 1

    @pytest.mark.asyncio
    async def test_auto_match_threshold_setter(self, mock_session, mock_igdb_service):
        """Auto-match threshold can be changed."""
        service = MatchingService(mock_session, mock_igdb_service)

        assert service.auto_match_threshold == 0.85  # Default

        service.auto_match_threshold = 0.9
        assert service.auto_match_threshold == 0.9

    @pytest.mark.asyncio
    async def test_auto_match_threshold_validation(self, mock_session, mock_igdb_service):
        """Auto-match threshold validates range."""
        service = MatchingService(mock_session, mock_igdb_service)

        with pytest.raises(ValueError):
            service.auto_match_threshold = 1.5

        with pytest.raises(ValueError):
            service.auto_match_threshold = -0.1


class TestMatchSourceEnum:
    """Test MatchSource enum values."""

    def test_match_source_values(self):
        """MatchSource enum has expected values."""
        assert MatchSource.IGDB_ID_PROVIDED.value == "igdb_id_provided"
        assert MatchSource.PLATFORM_LOOKUP.value == "platform_lookup"
        assert MatchSource.TITLE_SEARCH_AUTO.value == "title_search_auto"
        assert MatchSource.TITLE_SEARCH_REVIEW.value == "title_search_review"
        assert MatchSource.MANUAL.value == "manual"


class TestMatchStatusEnum:
    """Test MatchStatus enum values."""

    def test_match_status_values(self):
        """MatchStatus enum has expected values."""
        assert MatchStatus.MATCHED.value == "matched"
        assert MatchStatus.NEEDS_REVIEW.value == "needs_review"
        assert MatchStatus.NO_MATCH.value == "no_match"
        assert MatchStatus.ALREADY_MATCHED.value == "already_matched"
        assert MatchStatus.ERROR.value == "error"
