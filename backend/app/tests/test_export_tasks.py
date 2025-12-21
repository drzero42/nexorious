"""Tests for export tasks."""

from datetime import datetime, timezone, date
from decimal import Decimal
import json
import csv

from sqlmodel import Session

from app.worker.tasks.import_export.export import (
    _get_exports_dir,
    _build_user_games_query,
    _user_game_to_export_data,
    _user_game_to_csv_row,
    _write_json_export,
    _write_csv_export,
    _calculate_export_stats,
    EXPORT_VERSION,
)
from app.schemas.export import (
    ExportGameData,
    ExportTagData,
    NexoriousExportData,
    CsvExportRow,
)
from app.models.game import Game
from app.models.user_game import UserGame, UserGamePlatform, OwnershipStatus, PlayStatus
from app.models.platform import Platform
from app.models.tag import Tag, UserGameTag


class TestExportHelpers:
    """Tests for export helper functions."""

    def test_get_exports_dir_creates_directory(self, tmp_path):
        """_get_exports_dir creates directory if it doesn't exist."""
        from unittest.mock import patch

        with patch("app.worker.tasks.import_export.export.settings") as mock_settings:
            mock_settings.storage_path = str(tmp_path)

            exports_dir = _get_exports_dir()

            assert exports_dir.exists()
            assert exports_dir.name == "exports"

    def test_build_user_games_query(
        self, session: Session, test_user, test_game
    ):
        """_build_user_games_query returns all games."""
        # Create owned game
        owned_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.COMPLETED,
        )
        session.add(owned_game)
        session.commit()

        games = _build_user_games_query(session, test_user.id)

        assert len(games) == 1
        assert games[0].id == owned_game.id


class TestUserGameToExportData:
    """Tests for converting UserGame to export format."""

    def test_basic_conversion(self, session: Session, test_user, test_game):
        """Convert basic UserGame to ExportGameData."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.COMPLETED,
            personal_rating=Decimal("4.5"),
            is_loved=True,
            hours_played=50,
            personal_notes="Great game!",
            acquired_date=date(2024, 1, 15),
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        export_data = _user_game_to_export_data(session, user_game)

        assert export_data.igdb_id == test_game.id
        assert export_data.title == test_game.title
        assert export_data.ownership_status == "owned"
        assert export_data.play_status == "completed"
        assert export_data.personal_rating == 4.5
        assert export_data.is_loved is True
        assert export_data.hours_played == 50
        assert export_data.personal_notes == "Great game!"
        assert export_data.acquired_date == date(2024, 1, 15)

    def test_conversion_with_platforms(
        self, session: Session, test_user, test_game, test_platform
    ):
        """Convert UserGame with platform associations."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.COMPLETED,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        # Add platform association
        platform_assoc = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=test_platform.id,
            store_game_id="12345",
            store_url="https://store.example.com/game",
            is_available=True,
        )
        session.add(platform_assoc)
        session.commit()
        session.refresh(user_game)

        export_data = _user_game_to_export_data(session, user_game)

        assert len(export_data.platforms) == 1
        platform_data = export_data.platforms[0]
        assert platform_data.platform_id == test_platform.id
        assert platform_data.platform_name == test_platform.name
        assert platform_data.store_game_id == "12345"
        assert platform_data.store_url == "https://store.example.com/game"
        assert platform_data.is_available is True

    def test_conversion_with_tags(
        self, session: Session, test_user, test_game
    ):
        """Convert UserGame with tags."""
        # Create tag
        tag = Tag(
            user_id=test_user.id,
            name="RPG",
            color="#FF5733",
        )
        session.add(tag)
        session.commit()

        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.COMPLETED,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        # Add tag association
        user_game_tag = UserGameTag(
            user_game_id=user_game.id,
            tag_id=tag.id,
        )
        session.add(user_game_tag)
        session.commit()
        session.refresh(user_game)

        export_data = _user_game_to_export_data(session, user_game)

        assert len(export_data.tags) == 1
        tag_data = export_data.tags[0]
        assert tag_data.name == "RPG"
        assert tag_data.color == "#FF5733"


class TestUserGameToCsvRow:
    """Tests for converting UserGame to CSV row format."""

    def test_basic_csv_conversion(self, session: Session, test_user, test_game):
        """Convert basic UserGame to CsvExportRow."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.COMPLETED,
            personal_rating=Decimal("4.5"),
            hours_played=50,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        csv_row = _user_game_to_csv_row(session, user_game)

        assert csv_row.igdb_id == test_game.id
        assert csv_row.title == test_game.title
        assert csv_row.ownership_status == "owned"
        assert csv_row.play_status == "completed"
        assert csv_row.personal_rating == 4.5
        assert csv_row.hours_played == 50

    def test_csv_conversion_with_platforms_comma_separated(
        self, session: Session, test_user, test_game
    ):
        """CSV row contains comma-separated platform names."""
        # Create two platforms
        platform1 = Platform(id="test-pc", name="PC", display_name="PC", is_active=True)
        platform2 = Platform(id="test-ps5", name="PlayStation 5", display_name="PlayStation 5", is_active=True)
        session.add(platform1)
        session.add(platform2)
        session.commit()

        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.COMPLETED,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        # Add platform associations
        for platform in [platform1, platform2]:
            assoc = UserGamePlatform(
                user_game_id=user_game.id,
                platform_id=platform.id,
            )
            session.add(assoc)
        session.commit()
        session.refresh(user_game)

        csv_row = _user_game_to_csv_row(session, user_game)

        # Platform names should be comma-separated and sorted
        assert "PC" in csv_row.platforms
        assert "PlayStation 5" in csv_row.platforms


class TestWriteExports:
    """Tests for writing export files."""

    def test_write_json_export(self, tmp_path):
        """Write JSON export to file."""
        export_data = NexoriousExportData(
            export_version="1.0",
            export_date=datetime(2024, 1, 15, 10, 30, 0, tzinfo=timezone.utc),
            user_id="test-user-id",
            total_games=1,
            export_stats={"by_play_status": {"completed": 1}},
            games=[
                ExportGameData(
                    igdb_id=1942,
                    title="The Witcher 3",
                    ownership_status="owned",
                    play_status="completed",
                    created_at=datetime(2024, 1, 1, tzinfo=timezone.utc),
                    updated_at=datetime(2024, 1, 15, tzinfo=timezone.utc),
                )
            ],
        )

        file_path = tmp_path / "test_export.json"
        file_size = _write_json_export(export_data, file_path)

        assert file_path.exists()
        assert file_size > 0

        # Verify JSON content
        with open(file_path) as f:
            data = json.load(f)
        assert data["export_version"] == "1.0"
        assert data["total_games"] == 1
        assert len(data["games"]) == 1
        assert data["games"][0]["igdb_id"] == 1942

    def test_write_csv_export(self, tmp_path):
        """Write CSV export to file."""
        rows = [
            CsvExportRow(
                igdb_id=1942,
                title="The Witcher 3",
                ownership_status="owned",
                play_status="completed",
                personal_rating=4.5,
                is_loved=True,
                hours_played=100,
                platforms="PC, PlayStation 5",
                storefronts="Steam",
                tags="RPG, Open World",
                created_at="2024-01-01T00:00:00+00:00",
                updated_at="2024-01-15T00:00:00+00:00",
            ),
        ]

        file_path = tmp_path / "test_export.csv"
        file_size = _write_csv_export(rows, file_path)

        assert file_path.exists()
        assert file_size > 0

        # Verify CSV content
        with open(file_path) as f:
            reader = csv.DictReader(f)
            csv_rows = list(reader)

        assert len(csv_rows) == 1
        assert csv_rows[0]["igdb_id"] == "1942"
        assert csv_rows[0]["title"] == "The Witcher 3"
        assert csv_rows[0]["platforms"] == "PC, PlayStation 5"

    def test_write_csv_export_empty(self, tmp_path):
        """Write empty CSV export."""
        file_path = tmp_path / "empty_export.csv"
        file_size = _write_csv_export([], file_path)

        assert file_path.exists()
        assert file_size == 0


class TestCalculateExportStats:
    """Tests for export statistics calculation."""

    def test_calculate_stats_basic(self):
        """Calculate basic export statistics."""
        games = [
            ExportGameData(
                igdb_id=1,
                title="Game 1",
                ownership_status="owned",
                play_status="completed",
                personal_rating=4.5,
                is_loved=True,
                hours_played=50,
                personal_notes="Good game",
                tags=[ExportTagData(name="RPG")],
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            ),
            ExportGameData(
                igdb_id=2,
                title="Game 2",
                ownership_status="owned",
                play_status="in_progress",
                hours_played=20,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            ),
            ExportGameData(
                igdb_id=3,
                title="Game 3",
                ownership_status="subscription",
                play_status="completed",
                personal_rating=3.5,
                hours_played=30,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc),
            ),
        ]

        stats = _calculate_export_stats(games)

        assert stats["total_games"] == 3
        assert stats["by_play_status"]["completed"] == 2
        assert stats["by_play_status"]["in_progress"] == 1
        assert stats["by_ownership_status"]["owned"] == 2
        assert stats["by_ownership_status"]["subscription"] == 1
        assert stats["games_with_ratings"] == 2
        assert stats["games_with_notes"] == 1
        assert stats["games_with_tags"] == 1
        assert stats["loved_games"] == 1
        assert stats["total_hours_played"] == 100

    def test_calculate_stats_empty(self):
        """Calculate stats for empty game list."""
        stats = _calculate_export_stats([])

        assert stats["total_games"] == 0
        assert stats["games_with_ratings"] == 0
        assert stats["loved_games"] == 0
        assert stats["total_hours_played"] == 0


class TestExportVersionConstant:
    """Tests for export version constant."""

    def test_export_version_format(self):
        """Export version follows semantic versioning."""
        assert EXPORT_VERSION == "1.1"
        # Version should be a valid semver
        parts = EXPORT_VERSION.split(".")
        assert len(parts) >= 2
        assert all(p.isdigit() for p in parts)
