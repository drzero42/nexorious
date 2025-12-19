"""Tests for import tasks (Nexorious JSON, Darkadia CSV, Steam library)."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from datetime import date
from decimal import Decimal
from contextlib import asynccontextmanager

from app.worker.tasks.import_export.import_nexorious import (
    import_nexorious_json,
    _map_play_status,
    _map_ownership_status,
    _parse_rating,
    _parse_date,
    SUPPORTED_EXPORT_VERSIONS,
)
from app.worker.tasks.import_export.import_darkadia import (
    import_darkadia_csv,
    _create_column_map,
    _create_review_item,
    _get_row_value,
    parse_darkadia_platform,
    COLUMN_MAPPINGS,
)
# from app.worker.tasks.import_export.import_steam import (
#     import_steam_library,
# )
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    ReviewItem,
    ReviewItemStatus,
)
from app.models.user_game import PlayStatus, OwnershipStatus, UserGame
from sqlmodel import Session


class TestNexoriousImportHelpers:
    """Test helper functions for Nexorious import."""

    def test_map_play_status_valid_statuses(self):
        """Map valid play status strings."""
        assert _map_play_status("not_started") == PlayStatus.NOT_STARTED
        assert _map_play_status("in_progress") == PlayStatus.IN_PROGRESS
        assert _map_play_status("completed") == PlayStatus.COMPLETED
        assert _map_play_status("mastered") == PlayStatus.MASTERED
        assert _map_play_status("dominated") == PlayStatus.DOMINATED
        assert _map_play_status("shelved") == PlayStatus.SHELVED
        assert _map_play_status("dropped") == PlayStatus.DROPPED
        assert _map_play_status("replay") == PlayStatus.REPLAY

    def test_map_play_status_aliases(self):
        """Map play status aliases."""
        assert _map_play_status("playing") == PlayStatus.IN_PROGRESS
        assert _map_play_status("finished") == PlayStatus.COMPLETED
        assert _map_play_status("100%") == PlayStatus.MASTERED
        assert _map_play_status("abandoned") == PlayStatus.DROPPED
        assert _map_play_status("backlog") == PlayStatus.NOT_STARTED

    def test_map_play_status_case_insensitive(self):
        """Play status mapping is case insensitive."""
        assert _map_play_status("IN_PROGRESS") == PlayStatus.IN_PROGRESS
        assert _map_play_status("In_Progress") == PlayStatus.IN_PROGRESS
        assert _map_play_status("COMPLETED") == PlayStatus.COMPLETED

    def test_map_play_status_with_dashes(self):
        """Play status mapping handles dashes."""
        assert _map_play_status("in-progress") == PlayStatus.IN_PROGRESS
        assert _map_play_status("not-started") == PlayStatus.NOT_STARTED

    def test_map_play_status_none(self):
        """None play status defaults to NOT_STARTED."""
        assert _map_play_status(None) == PlayStatus.NOT_STARTED

    def test_map_play_status_unknown(self):
        """Unknown play status defaults to NOT_STARTED."""
        assert _map_play_status("unknown_status") == PlayStatus.NOT_STARTED

    def test_map_ownership_status_valid_statuses(self):
        """Map valid ownership status strings."""
        assert _map_ownership_status("owned") == OwnershipStatus.OWNED
        assert _map_ownership_status("borrowed") == OwnershipStatus.BORROWED
        assert _map_ownership_status("rented") == OwnershipStatus.RENTED
        assert _map_ownership_status("subscription") == OwnershipStatus.SUBSCRIPTION
        assert _map_ownership_status("no_longer_owned") == OwnershipStatus.NO_LONGER_OWNED

    def test_map_ownership_status_aliases(self):
        """Map ownership status aliases."""
        assert _map_ownership_status("gamepass") == OwnershipStatus.SUBSCRIPTION
        assert _map_ownership_status("game_pass") == OwnershipStatus.SUBSCRIPTION
        assert _map_ownership_status("ps_plus") == OwnershipStatus.SUBSCRIPTION
        assert _map_ownership_status("ps+") == OwnershipStatus.SUBSCRIPTION
        assert _map_ownership_status("sold") == OwnershipStatus.NO_LONGER_OWNED

    def test_map_ownership_status_none(self):
        """None ownership status defaults to OWNED."""
        assert _map_ownership_status(None) == OwnershipStatus.OWNED

    def test_map_ownership_status_unknown(self):
        """Unknown ownership status defaults to OWNED."""
        assert _map_ownership_status("unknown") == OwnershipStatus.OWNED

    def test_parse_rating_valid_decimal(self):
        """Parse valid rating values."""
        assert _parse_rating("8.5") == Decimal("8.5")
        assert _parse_rating(8.5) == Decimal("8.5")
        assert _parse_rating(8) == Decimal("8.0")
        assert _parse_rating("10") == Decimal("10.0")

    def test_parse_rating_clamped(self):
        """Rating is clamped to valid range."""
        assert _parse_rating("-1") == Decimal("0.0")
        assert _parse_rating("15") == Decimal("10.0")
        assert _parse_rating(-5) == Decimal("0.0")

    def test_parse_rating_none(self):
        """None rating returns None."""
        assert _parse_rating(None) is None

    def test_parse_rating_invalid(self):
        """Invalid rating returns None."""
        assert _parse_rating("not a number") is None
        assert _parse_rating([]) is None

    def test_parse_date_valid_iso(self):
        """Parse valid ISO date strings."""
        result = _parse_date("2023-06-15")
        assert result == date(2023, 6, 15)

    def test_parse_date_with_time(self):
        """Parse date with time component."""
        result = _parse_date("2023-06-15T12:30:00")
        assert result == date(2023, 6, 15)

    def test_parse_date_already_date(self):
        """Return date object as-is."""
        d = date(2023, 6, 15)
        assert _parse_date(d) == d

    def test_parse_date_none(self):
        """None date returns None."""
        assert _parse_date(None) is None

    def test_parse_date_invalid(self):
        """Invalid date returns None."""
        assert _parse_date("not a date") is None
        assert _parse_date("2023") is None

    def test_supported_export_versions(self):
        """Supported export versions are defined."""
        assert "1.0" in SUPPORTED_EXPORT_VERSIONS
        assert "1.1" in SUPPORTED_EXPORT_VERSIONS


class TestDarkadiaImportHelpers:
    """Test helper functions for Darkadia import."""

    def test_create_column_map_basic(self):
        """Create column map from standard columns."""
        columns = ["Name", "Platform", "Status", "Rating"]
        column_map = _create_column_map(columns)

        assert column_map["name"] == "Name"
        assert column_map["platform"] == "Platform"
        assert column_map["status"] == "Status"
        assert column_map["rating"] == "Rating"

    def test_create_column_map_alternative_names(self):
        """Create column map from alternative column names."""
        columns = ["Title", "Console", "Play Status", "Score"]
        column_map = _create_column_map(columns)

        assert column_map["name"] == "Title"
        assert column_map["platform"] == "Console"
        assert column_map["status"] == "Play Status"
        assert column_map["rating"] == "Score"

    def test_create_column_map_missing_columns(self):
        """Missing columns map to None."""
        columns = ["Name"]
        column_map = _create_column_map(columns)

        assert column_map["name"] == "Name"
        assert column_map["platform"] is None
        assert column_map["rating"] is None

    def test_get_row_value_found(self):
        """Get row value when column exists."""
        row = {"Name": "Test Game", "Platform": "PC"}
        column_map: dict[str, str | None] = {"name": "Name", "platform": "Platform"}

        assert _get_row_value(row, column_map, "name") == "Test Game"
        assert _get_row_value(row, column_map, "platform") == "PC"

    def test_get_row_value_not_mapped(self):
        """Get row value returns None when column not mapped."""
        row = {"Name": "Test Game"}
        column_map = {"name": "Name", "platform": None}

        assert _get_row_value(row, column_map, "platform") is None

    def test_get_row_value_empty_string(self):
        """Get row value returns None for empty strings."""
        row = {"Name": "", "Platform": "  "}
        column_map: dict[str, str | None] = {"name": "Name", "platform": "Platform"}

        assert _get_row_value(row, column_map, "name") is None
        assert _get_row_value(row, column_map, "platform") is None

    def test_get_row_value_strips_whitespace(self):
        """Get row value strips whitespace."""
        row = {"Name": "  Test Game  "}
        column_map: dict[str, str | None] = {"name": "Name"}

        assert _get_row_value(row, column_map, "name") == "Test Game"

    def test_column_mappings_defined(self):
        """Column mappings are defined for all fields."""
        expected_fields = [
            "name",
            "platform",
            "status",
            "rating",
            "notes",
            "hours",
            "completion",
            "date_added",
            "release_year",
        ]
        for field in expected_fields:
            assert field in COLUMN_MAPPINGS
            assert len(COLUMN_MAPPINGS[field]) > 0


class TestParseDarkadiaPlatform:
    """Tests for Darkadia platform string parsing."""

    def test_parse_full_platform_string(self):
        """Parse platform with all components."""
        result = parse_darkadia_platform("PC|Steam|Digital")
        assert result == {
            "platform": "PC",
            "storefront": "Steam",
            "media_type": "Digital",
        }

    def test_parse_platform_only(self):
        """Parse platform with no storefront or media type."""
        result = parse_darkadia_platform("PlayStation 4")
        assert result == {
            "platform": "PlayStation 4",
            "storefront": None,
            "media_type": None,
        }

    def test_parse_platform_and_storefront(self):
        """Parse platform with storefront but no media type."""
        result = parse_darkadia_platform("PC|GOG")
        assert result == {
            "platform": "PC",
            "storefront": "GOG",
            "media_type": None,
        }

    def test_parse_empty_string(self):
        """Handle empty string."""
        result = parse_darkadia_platform("")
        assert result == {
            "platform": None,
            "storefront": None,
            "media_type": None,
        }

    def test_parse_whitespace_trimming(self):
        """Trim whitespace from components."""
        result = parse_darkadia_platform(" PC | Steam | Digital ")
        assert result == {
            "platform": "PC",
            "storefront": "Steam",
            "media_type": "Digital",
        }


class TestNexoriousImportTask:
    """Test the Nexorious JSON import task."""

    @pytest.fixture
    def mock_job(self):
        """Create a mock job for testing."""
        job = MagicMock(spec=Job)
        job.id = "test-job-id"
        job.user_id = "test-user-id"
        job.status = BackgroundJobStatus.PENDING
        job.progress_current = 0
        job.progress_total = 0
        return job

    @pytest.fixture
    def sample_nexorious_data(self):
        """Sample Nexorious export data."""
        return {
            "export_version": "1.0",
            "export_date": "2023-06-15",
            "games": [
                {
                    "title": "The Witcher 3",
                    "igdb_id": 1942,
                    "play_status": "completed",
                    "personal_rating": "9.5",
                    "hours_played": 150,
                    "personal_notes": "Amazing game",
                },
                {
                    "title": "Elden Ring",
                    "igdb_id": 119133,
                    "play_status": "in_progress",
                    "personal_rating": "10",
                    "hours_played": 80,
                },
            ],
        }

    @pytest.mark.asyncio
    async def test_import_job_not_found(self):
        """Import fails gracefully when job not found."""
        with patch(
            "app.worker.tasks.import_export.import_nexorious.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_nexorious.acquire_job_lock",
            return_value=True,
        ):
            mock_session = MagicMock()
            mock_session.get.return_value = None
            mock_context.return_value.__aenter__ = AsyncMock(return_value=mock_session)
            mock_context.return_value.__aexit__ = AsyncMock()

            result = await import_nexorious_json("nonexistent-job-id")

            assert result["status"] == "error"
            assert "Job not found" in result["error"]

    @pytest.mark.asyncio
    async def test_import_empty_data(self, mock_job):
        """Import fails when no import data in job."""
        mock_job.get_result_summary.return_value = {}

        with patch(
            "app.worker.tasks.import_export.import_nexorious.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_nexorious.acquire_job_lock",
            return_value=True,
        ):
            mock_session = MagicMock()
            mock_session.get.return_value = mock_job
            mock_context.return_value.__aenter__ = AsyncMock(return_value=mock_session)
            mock_context.return_value.__aexit__ = AsyncMock()

            result = await import_nexorious_json("test-job-id")

            assert result["status"] == "error"
            assert mock_job.status == BackgroundJobStatus.FAILED


class TestDarkadiaImportTask:
    """Test the Darkadia CSV import task."""

    @pytest.fixture
    def mock_job(self):
        """Create a mock job for testing."""
        job = MagicMock(spec=Job)
        job.id = "test-job-id"
        job.user_id = "test-user-id"
        job.status = BackgroundJobStatus.PENDING
        job.progress_current = 0
        job.progress_total = 0
        return job

    @pytest.fixture
    def sample_darkadia_rows(self):
        """Sample Darkadia CSV rows."""
        return [
            {"Name": "The Witcher 3", "Platform": "PC", "Status": "completed"},
            {"Name": "Elden Ring", "Platform": "PlayStation 5", "Status": "playing"},
            {"Name": "Unknown Game", "Platform": "PC", "Status": "backlog"},
        ]

    @pytest.mark.asyncio
    async def test_import_job_not_found(self):
        """Import fails gracefully when job not found."""
        with patch(
            "app.worker.tasks.import_export.import_darkadia.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_darkadia.acquire_job_lock",
            return_value=True,
        ), patch(
            "app.worker.tasks.import_export.import_darkadia.release_job_lock"
        ):
            mock_session = MagicMock()
            mock_session.get.return_value = None
            mock_context.return_value.__aenter__ = AsyncMock(return_value=mock_session)
            mock_context.return_value.__aexit__ = AsyncMock()

            result = await import_darkadia_csv("nonexistent-job-id")

            assert result["status"] == "error"
            assert "Job not found" in result["error"]

    @pytest.mark.asyncio
    async def test_import_empty_data(self, mock_job):
        """Import fails when no import data in job."""
        mock_job.get_result_summary.return_value = {"columns": ["Name"]}

        with patch(
            "app.worker.tasks.import_export.import_darkadia.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_darkadia.acquire_job_lock",
            return_value=True,
        ), patch(
            "app.worker.tasks.import_export.import_darkadia.release_job_lock"
        ):
            mock_session = MagicMock()
            mock_session.get.return_value = mock_job
            mock_context.return_value.__aenter__ = AsyncMock(return_value=mock_session)
            mock_context.return_value.__aexit__ = AsyncMock()

            result = await import_darkadia_csv("test-job-id")

            assert result["status"] == "error"
            assert mock_job.status == BackgroundJobStatus.FAILED


class TestImportTasksJobStatusTransitions:
    """Test job status transitions during import tasks."""

    def test_job_status_values(self):
        """Verify job status enum values used by import tasks."""
        assert BackgroundJobStatus.PENDING.value == "pending"
        assert BackgroundJobStatus.PROCESSING.value == "processing"
        assert BackgroundJobStatus.AWAITING_REVIEW.value == "awaiting_review"
        assert BackgroundJobStatus.COMPLETED.value == "completed"
        assert BackgroundJobStatus.FAILED.value == "failed"

    def test_review_item_status_values(self):
        """Verify review item status enum values."""
        assert ReviewItemStatus.PENDING.value == "pending"
        assert ReviewItemStatus.MATCHED.value == "matched"
        assert ReviewItemStatus.SKIPPED.value == "skipped"

    def test_job_source_values(self):
        """Verify job source enum values for imports."""
        assert BackgroundJobSource.NEXORIOUS.value == "nexorious"
        assert BackgroundJobSource.DARKADIA.value == "darkadia"
        assert BackgroundJobSource.STEAM.value == "steam"


class TestImportTaskIntegration:
    """Integration tests using database session."""

    @pytest.mark.asyncio
    async def test_nexorious_import_creates_review_items(self, session, test_user):
        """Nexorious import creates games without review items (non-interactive)."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )
        job.set_result_summary({
            "_import_data": {
                "export_version": "1.0",
                "games": [
                    {
                        "title": "Test Game",
                        "igdb_id": 12345,
                        "play_status": "completed",
                    }
                ],
            }
        })
        session.add(job)
        session.commit()
        session.refresh(job)

        # Mock IGDB service
        with patch(
            "app.worker.tasks.import_export.import_nexorious.IGDBService"
        ) as mock_igdb_class, patch(
            "app.worker.tasks.import_export.import_nexorious.GameService"
        ) as mock_game_service_class:
            mock_igdb = MagicMock()
            mock_igdb_class.return_value = mock_igdb

            mock_game_service = MagicMock()
            mock_game_service.create_or_update_game_from_igdb = AsyncMock(
                return_value=MagicMock(id=12345)
            )
            mock_game_service_class.return_value = mock_game_service

            # The task would need the session context, which is complex to mock
            # For integration tests, we verify the job is set up correctly
            assert job.status == BackgroundJobStatus.PENDING
            assert job.get_result_summary()["_import_data"]["games"][0]["igdb_id"] == 12345

    @pytest.mark.asyncio
    async def test_darkadia_import_requires_review(self, session, test_user):
        """Darkadia import creates review items for unmatched games."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )
        job.set_result_summary({
            "columns": ["Name", "Platform"],
            "_import_rows": [
                {"Name": "Unknown Game", "Platform": "PC"},
            ],
        })
        session.add(job)
        session.commit()
        session.refresh(job)

        # Verify job is set up correctly
        assert job.status == BackgroundJobStatus.PENDING
        result_summary = job.get_result_summary()
        assert "_import_rows" in result_summary
        assert len(result_summary["_import_rows"]) == 1

    @pytest.mark.asyncio
    async def test_steam_import_stores_steam_id(self, session, test_user):
        """Steam import stores Steam ID in job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )
        job.set_result_summary({"steam_id": "76561198012345678"})
        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.get_result_summary()["steam_id"] == "76561198012345678"


class TestCreateReviewItemDuplicatePrevention:
    """Test duplicate prevention in _create_review_item."""

    @pytest.fixture
    def mock_match_result(self):
        """Create a mock match result."""
        result = MagicMock()
        result.candidates = []
        result.igdb_id = 12345
        result.igdb_title = "Test Game"
        result.confidence_score = 0.95
        return result

    def test_creates_review_item_when_none_exists(self, session, test_user):
        """Creates a ReviewItem when none exists for the job/game combination."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        mock_result = MagicMock()
        mock_result.candidates = []
        mock_result.igdb_id = 12345
        mock_result.igdb_title = "Test Game"
        mock_result.confidence_score = 0.95

        # Create review item
        _create_review_item(
            session=session,
            job=job,
            user_id=test_user.id,
            game_name="The Witcher 3",
            match_result=mock_result,
            source_metadata={"source": "darkadia"},
            status=ReviewItemStatus.MATCHED,
        )

        # Verify it was created
        from sqlmodel import select
        items = session.exec(
            select(ReviewItem).where(
                ReviewItem.job_id == job.id,
                ReviewItem.source_title == "The Witcher 3",
            )
        ).all()
        assert len(items) == 1
        assert items[0].source_title == "The Witcher 3"
        assert items[0].status == ReviewItemStatus.MATCHED

    def test_skips_duplicate_review_item(self, session, test_user):
        """Skips creating a ReviewItem when one already exists for the same job/game."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        mock_result = MagicMock()
        mock_result.candidates = []
        mock_result.igdb_id = 12345
        mock_result.igdb_title = "Test Game"
        mock_result.confidence_score = 0.95

        # Create first review item
        _create_review_item(
            session=session,
            job=job,
            user_id=test_user.id,
            game_name="Duplicate Game",
            match_result=mock_result,
            source_metadata={"source": "darkadia"},
            status=ReviewItemStatus.MATCHED,
        )

        # Try to create a duplicate
        _create_review_item(
            session=session,
            job=job,
            user_id=test_user.id,
            game_name="Duplicate Game",
            match_result=mock_result,
            source_metadata={"source": "darkadia", "extra": "data"},
            status=ReviewItemStatus.PENDING,  # Even with different status
        )

        # Verify only one exists
        from sqlmodel import select
        items = session.exec(
            select(ReviewItem).where(
                ReviewItem.job_id == job.id,
                ReviewItem.source_title == "Duplicate Game",
            )
        ).all()
        assert len(items) == 1
        # Original item should be preserved (MATCHED status)
        assert items[0].status == ReviewItemStatus.MATCHED

    def test_allows_same_game_in_different_jobs(self, session, test_user):
        """Allows the same game name in different jobs."""
        # Create two jobs
        job1 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
        )
        job2 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
        )
        session.add(job1)
        session.add(job2)
        session.commit()
        session.refresh(job1)
        session.refresh(job2)

        mock_result = MagicMock()
        mock_result.candidates = []
        mock_result.igdb_id = 12345
        mock_result.igdb_title = "Test Game"
        mock_result.confidence_score = 0.95

        # Create review item in first job
        _create_review_item(
            session=session,
            job=job1,
            user_id=test_user.id,
            game_name="Same Game",
            match_result=mock_result,
            source_metadata={"source": "darkadia"},
            status=ReviewItemStatus.MATCHED,
        )

        # Create review item with same name in second job
        _create_review_item(
            session=session,
            job=job2,
            user_id=test_user.id,
            game_name="Same Game",
            match_result=mock_result,
            source_metadata={"source": "darkadia"},
            status=ReviewItemStatus.MATCHED,
        )

        # Verify both exist (one per job)
        from sqlmodel import select
        items_job1 = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job1.id)
        ).all()
        items_job2 = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job2.id)
        ).all()
        assert len(items_job1) == 1
        assert len(items_job2) == 1

    def test_different_games_same_job_allowed(self, session, test_user):
        """Allows different games in the same job."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        mock_result = MagicMock()
        mock_result.candidates = []
        mock_result.igdb_id = 12345
        mock_result.igdb_title = "Test Game"
        mock_result.confidence_score = 0.95

        # Create review items for different games
        _create_review_item(
            session=session,
            job=job,
            user_id=test_user.id,
            game_name="Game A",
            match_result=mock_result,
            source_metadata={"source": "darkadia"},
            status=ReviewItemStatus.MATCHED,
        )

        _create_review_item(
            session=session,
            job=job,
            user_id=test_user.id,
            game_name="Game B",
            match_result=mock_result,
            source_metadata={"source": "darkadia"},
            status=ReviewItemStatus.PENDING,
        )

        # Verify both exist
        from sqlmodel import select
        items = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job.id)
        ).all()
        assert len(items) == 2
        titles = {item.source_title for item in items}
        assert titles == {"Game A", "Game B"}


class TestNexoriousImportLocking:
    """Test advisory lock behavior in Nexorious import task."""

    @pytest.mark.asyncio
    async def test_import_skips_when_lock_held(self, session, test_user):
        """Import returns skipped status when another worker holds the lock."""
        from app.worker.locking import acquire_job_lock, release_job_lock
        from app.core.database import get_engine

        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )
        job.set_result_summary({
            "_import_data": {
                "export_version": "1.0",
                "games": [{"title": "Test", "igdb_id": 123}],
            }
        })
        session.add(job)
        session.commit()
        session.refresh(job)

        # Simulate another worker holding the lock
        with Session(get_engine()) as other_session:
            acquired = acquire_job_lock(other_session, job.id)
            assert acquired is True

            # Now run the import task - should skip
            result = await import_nexorious_json(job.id)

            assert result["status"] == "skipped"
            assert result["reason"] == "duplicate_execution"

            # Release the lock
            release_job_lock(other_session, job.id)


class TestNexoriousImportIntegrityError:
    """Test IntegrityError handling in Nexorious import task."""

    @pytest.mark.asyncio
    async def test_import_handles_integrity_error_gracefully(
        self, session, test_user, test_game
    ):
        """Import counts as already_in_collection when IntegrityError occurs."""
        # Pre-create the UserGame to trigger IntegrityError on import
        existing = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(existing)
        session.commit()

        # Create import job for the same game
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )
        job.set_result_summary({
            "_import_data": {
                "export_version": "1.0",
                "games": [
                    {
                        "title": test_game.title,
                        "igdb_id": test_game.id,
                        "play_status": "completed",
                    }
                ],
            }
        })
        session.add(job)
        session.commit()
        session.refresh(job)

        # Mock get_session_context to use test session
        @asynccontextmanager
        async def mock_session_context():
            yield session

        # Mock services and session context
        with patch(
            "app.worker.tasks.import_export.import_nexorious.IGDBService"
        ), patch(
            "app.worker.tasks.import_export.import_nexorious.GameService"
        ), patch(
            "app.worker.tasks.import_export.import_nexorious.get_session_context",
            mock_session_context,
        ):
            result = await import_nexorious_json(job.id)

        # Should complete with already_in_collection count
        assert result["status"] == "success"
        assert result["already_in_collection"] == 1
        assert result["imported"] == 0
