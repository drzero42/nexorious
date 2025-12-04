"""
Unit tests for GameService business logic.
Tests game import from IGDB, existing game handling, and error cases.
"""

import pytest
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool
from unittest.mock import Mock, AsyncMock, patch
from datetime import date

from ..models.game import Game
from ..services.game_service import GameService, GameNotFoundError, parse_date_string
from ..services.igdb import IGDBService, GameMetadata, TwitchAuthError, IGDBError


@pytest.fixture(name="service_session")
def service_session_fixture():
    """Create a test database session for service tests."""
    engine = create_engine(
        "sqlite:///:memory:",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
    )
    SQLModel.metadata.create_all(engine)
    with Session(engine) as session:
        yield session


@pytest.fixture(name="mock_igdb_service")
def mock_igdb_service_fixture():
    """Create a mock IGDB service."""
    mock_service = Mock(spec=IGDBService)
    mock_service.get_game_by_id = AsyncMock()
    mock_service.download_and_store_cover_art = AsyncMock()
    return mock_service


@pytest.fixture(name="game_service")
def game_service_fixture(service_session: Session, mock_igdb_service: Mock) -> GameService:
    """Create a GameService instance with mocked dependencies."""
    return GameService(service_session, mock_igdb_service)


@pytest.fixture(name="sample_game_metadata")
def sample_game_metadata_fixture() -> GameMetadata:
    """Create sample game metadata from IGDB."""
    return GameMetadata(
        igdb_id=12345,
        title="The Witcher 3: Wild Hunt",
        description="An open-world RPG game",
        genre="RPG",
        developer="CD Projekt Red",
        publisher="CD Projekt",
        release_date="2015-05-19",
        cover_art_url="https://images.igdb.com/cover.jpg",
        rating_average=92.5,
        rating_count=1000,
        estimated_playtime_hours=50,
        hastily=25,  # howlongtobeat_main
        normally=50,  # howlongtobeat_extra
        completely=100,  # howlongtobeat_completionist
        igdb_slug="the-witcher-3-wild-hunt",
        igdb_platform_ids=[6, 48, 49],
        platform_names=["PC", "PlayStation 4", "Xbox One"],
    )


@pytest.fixture(name="existing_game")
def existing_game_fixture(service_session: Session) -> Game:
    """Create an existing game in the database."""
    game = Game(
        id=12345,
        title="The Witcher 3: Wild Hunt",
        description="Original description",
        genre="RPG",
        developer="CD Projekt Red",
        publisher="CD Projekt",
        release_date=date(2015, 5, 19),
        cover_art_url="https://old-url.com/cover.jpg",
        rating_average=90.0,
        rating_count=500,
    )
    service_session.add(game)
    service_session.commit()
    service_session.refresh(game)
    return game


class TestParseDateString:
    """Tests for parse_date_string utility function."""

    def test_parse_full_date(self):
        """Test parsing YYYY-MM-DD format."""
        result = parse_date_string("2015-05-19")
        assert result == date(2015, 5, 19)

    def test_parse_year_only(self):
        """Test parsing YYYY format."""
        result = parse_date_string("2015")
        assert result == date(2015, 1, 1)

    def test_parse_none(self):
        """Test parsing None returns None."""
        result = parse_date_string(None)
        assert result is None

    def test_parse_empty_string(self):
        """Test parsing empty string returns None."""
        result = parse_date_string("")
        assert result is None

    def test_parse_invalid_format(self):
        """Test parsing invalid format returns None."""
        result = parse_date_string("19-05-2015")
        assert result is None

    def test_parse_invalid_date(self):
        """Test parsing invalid date returns None."""
        result = parse_date_string("2015-13-45")
        assert result is None


class TestGameNotFoundError:
    """Tests for GameNotFoundError exception."""

    def test_error_message(self):
        """Test error message contains IGDB ID."""
        error = GameNotFoundError(12345)
        assert "12345" in str(error)
        assert error.igdb_id == 12345


class TestGameServiceCreateOrUpdateFromIGDB:
    """Tests for create_or_update_game_from_igdb method."""

    @pytest.mark.asyncio
    async def test_create_new_game(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
        service_session: Session,
    ):
        """Test creating a new game from IGDB metadata."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata
        mock_igdb_service.download_and_store_cover_art.return_value = "/local/cover.jpg"

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            download_cover_art=True,
        )

        assert result.id == 12345
        assert result.title == "The Witcher 3: Wild Hunt"
        assert result.description == "An open-world RPG game"
        assert result.genre == "RPG"
        assert result.developer == "CD Projekt Red"
        assert result.publisher == "CD Projekt"
        assert result.release_date == date(2015, 5, 19)
        assert result.cover_art_url == "/local/cover.jpg"
        assert result.howlongtobeat_main == 25
        assert result.howlongtobeat_extra == 50
        assert result.howlongtobeat_completionist == 100
        assert result.igdb_slug == "the-witcher-3-wild-hunt"

        # Verify game was persisted
        persisted_game = service_session.get(Game, 12345)
        assert persisted_game is not None
        assert persisted_game.title == "The Witcher 3: Wild Hunt"

    @pytest.mark.asyncio
    async def test_create_new_game_without_cover_art_download(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
    ):
        """Test creating a new game without downloading cover art."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            download_cover_art=False,
        )

        assert result.cover_art_url == "https://images.igdb.com/cover.jpg"
        mock_igdb_service.download_and_store_cover_art.assert_not_called()

    @pytest.mark.asyncio
    async def test_return_existing_game(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
        existing_game: Game,
    ):
        """Test returning an existing game without modification."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
        )

        assert result.id == existing_game.id
        # Should return existing game data, not IGDB metadata
        assert result.description == "Original description"
        assert result.cover_art_url == "https://old-url.com/cover.jpg"

    @pytest.mark.asyncio
    async def test_update_existing_game_with_overrides(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
        existing_game: Game,
        service_session: Session,
    ):
        """Test applying custom overrides to an existing game."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            custom_overrides={"description": "Custom description", "genre": "Action RPG"},
        )

        assert result.id == existing_game.id
        assert result.description == "Custom description"
        assert result.genre == "Action RPG"

        # Verify changes were persisted
        service_session.refresh(existing_game)
        assert existing_game.description == "Custom description"
        assert existing_game.genre == "Action RPG"

    @pytest.mark.asyncio
    async def test_create_new_game_with_overrides(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
    ):
        """Test creating a new game with custom overrides applied."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata
        mock_igdb_service.download_and_store_cover_art.return_value = None

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            custom_overrides={"title": "Custom Title", "description": "My description"},
            download_cover_art=True,
        )

        assert result.title == "Custom Title"
        assert result.description == "My description"
        # Other fields should come from IGDB
        assert result.developer == "CD Projekt Red"

    @pytest.mark.asyncio
    async def test_game_not_found_raises_error(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
    ):
        """Test that GameNotFoundError is raised when game not in IGDB."""
        mock_igdb_service.get_game_by_id.return_value = None

        with pytest.raises(GameNotFoundError) as exc_info:
            await game_service.create_or_update_game_from_igdb(igdb_id=99999)

        assert exc_info.value.igdb_id == 99999

    @pytest.mark.asyncio
    async def test_twitch_auth_error_propagates(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
    ):
        """Test that TwitchAuthError propagates from IGDB service."""
        mock_igdb_service.get_game_by_id.side_effect = TwitchAuthError("Auth failed")

        with pytest.raises(TwitchAuthError):
            await game_service.create_or_update_game_from_igdb(igdb_id=12345)

    @pytest.mark.asyncio
    async def test_igdb_error_propagates(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
    ):
        """Test that IGDBError propagates from IGDB service."""
        mock_igdb_service.get_game_by_id.side_effect = IGDBError("API error")

        with pytest.raises(IGDBError):
            await game_service.create_or_update_game_from_igdb(igdb_id=12345)

    @pytest.mark.asyncio
    async def test_cover_art_download_failure_does_not_fail_import(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
    ):
        """Test that cover art download failure doesn't fail the import."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata
        mock_igdb_service.download_and_store_cover_art.side_effect = Exception("Download failed")

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            download_cover_art=True,
        )

        # Import should succeed despite cover art failure
        assert result.id == 12345
        assert result.title == "The Witcher 3: Wild Hunt"
        # Cover art URL should be the original IGDB URL
        assert result.cover_art_url == "https://images.igdb.com/cover.jpg"

    @pytest.mark.asyncio
    async def test_null_override_values_ignored(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
        existing_game: Game,
    ):
        """Test that None values in overrides are ignored."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            custom_overrides={"description": None, "genre": "Action RPG"},
        )

        # Description should remain unchanged
        assert result.description == "Original description"
        # Genre should be updated
        assert result.genre == "Action RPG"

    @pytest.mark.asyncio
    async def test_platform_ids_serialized_to_json(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
        sample_game_metadata: GameMetadata,
    ):
        """Test that platform IDs are serialized to JSON."""
        mock_igdb_service.get_game_by_id.return_value = sample_game_metadata
        mock_igdb_service.download_and_store_cover_art.return_value = None

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            download_cover_art=True,
        )

        import json
        platform_ids = json.loads(result.igdb_platform_ids)
        platform_names = json.loads(result.igdb_platform_names)

        assert platform_ids == [6, 48, 49]
        assert platform_names == ["PC", "PlayStation 4", "Xbox One"]

    @pytest.mark.asyncio
    async def test_game_with_no_platform_data(
        self,
        game_service: GameService,
        mock_igdb_service: Mock,
    ):
        """Test handling game metadata with no platform data."""
        metadata = GameMetadata(
            igdb_id=12345,
            title="Test Game",
            igdb_platform_ids=None,
            platform_names=None,
        )
        mock_igdb_service.get_game_by_id.return_value = metadata
        mock_igdb_service.download_and_store_cover_art.return_value = None

        result = await game_service.create_or_update_game_from_igdb(
            igdb_id=12345,
            download_cover_art=False,
        )

        assert result.igdb_platform_ids is None
        assert result.igdb_platform_names is None
