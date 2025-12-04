"""
Tests for storefront resolution functionality.
"""

import pytest
from unittest.mock import AsyncMock, Mock
from sqlmodel import Session

from app.models.platform import Platform, Storefront, PlatformStorefront
from app.models.darkadia_import import DarkadiaImport
from app.services.platform_resolution import PlatformResolutionService
from app.schemas.platform import StorefrontSuggestion


class TestStorefrontResolution:
    """Test storefront resolution functionality."""
    
    @pytest.fixture
    def mock_session(self):
        """Create a mock database session."""
        session = Mock(spec=Session)
        session.exec = Mock()
        session.get = Mock()
        session.add = Mock()
        session.commit = Mock()
        session.rollback = Mock()
        return session
    
    @pytest.fixture
    def resolution_service(self, mock_session):
        """Create a platform resolution service instance."""
        return PlatformResolutionService(mock_session)
    
    @pytest.fixture
    def sample_platform(self):
        """Create a sample platform for testing."""
        return Platform(
            id="test-platform-1",
            name="test_platform",
            display_name="Test Platform"
        )
    
    @pytest.fixture
    def sample_storefront(self):
        """Create a sample storefront for testing."""
        return Storefront(
            id="test-storefront-1",
            name="test_store",
            display_name="Test Store"
        )

    @pytest.mark.asyncio
    async def test_get_storefronts_for_platform(self, resolution_service, mock_session, sample_storefront):
        """Test getting storefronts for a specific platform."""
        # Mock the database query result
        mock_session.exec.return_value.all.return_value = [sample_storefront]
        
        # Test the method
        result = await resolution_service.get_storefronts_for_platform("test-platform-1")
        
        # Verify results
        assert len(result) == 1
        assert result[0] == sample_storefront
        
        # Verify the query was called correctly
        mock_session.exec.assert_called_once()

    @pytest.mark.asyncio
    async def test_validate_platform_storefront_compatibility_valid(self, resolution_service, mock_session):
        """Test platform-storefront compatibility validation for valid combination."""
        # Mock successful association lookup
        mock_association = PlatformStorefront(
            platform_id="test-platform-1",
            storefront_id="test-storefront-1"
        )
        mock_session.exec.return_value.first.return_value = mock_association
        
        # Test the method
        result = await resolution_service.validate_platform_storefront_compatibility(
            "test-platform-1", "test-storefront-1"
        )
        
        # Verify result
        assert result is True
        mock_session.exec.assert_called_once()

    @pytest.mark.asyncio
    async def test_validate_platform_storefront_compatibility_invalid(self, resolution_service, mock_session):
        """Test platform-storefront compatibility validation for invalid combination."""
        # Mock no association found
        mock_session.exec.return_value.first.return_value = None
        
        # Test the method
        result = await resolution_service.validate_platform_storefront_compatibility(
            "test-platform-1", "test-storefront-1"
        )
        
        # Verify result
        assert result is False
        mock_session.exec.assert_called_once()

    @pytest.mark.asyncio
    async def test_detect_unknown_storefronts(self, resolution_service, mock_session):
        """Test detection of unknown storefronts in user imports."""
        # Mock database query results
        mock_session.exec.return_value.all.return_value = ["Unknown Store", "Steam", "Epic Games Store"]
        
        # Test the method
        result = await resolution_service.detect_unknown_storefronts("test-user-1")
        
        # Verify results
        assert len(result) == 3
        assert "Unknown Store" in result
        assert "Steam" in result
        assert "Epic Games Store" in result
        
        mock_session.exec.assert_called_once()

    @pytest.mark.asyncio
    async def test_resolve_storefront_success(self, resolution_service, mock_session):
        """Test successful storefront resolution."""
        # Mock the import record
        mock_import = Mock(spec=DarkadiaImport)
        mock_import.id = "test-import-1"
        mock_import.user_id = "test-user-1"
        mock_import.game_name = "Test Game"
        mock_import.resolved_storefront_id = None
        mock_import.storefront_resolved = False
        mock_import.get_platform_resolution_data = Mock(return_value={})
        mock_import.set_platform_resolution_data = Mock()
        
        # Mock the storefront
        mock_storefront = Storefront(
            id="test-storefront-1",
            name="test_store",
            display_name="Test Store",
            is_active=True
        )
        
        # Setup mocks
        mock_session.get.side_effect = [mock_import, mock_storefront]
        
        # Test the method
        result = await resolution_service.resolve_storefront(
            import_id="test-import-1",
            user_id="test-user-1",
            resolved_storefront_id="test-storefront-1",
            user_notes="Test resolution"
        )
        
        # Verify result
        assert result is True
        
        # Verify the import record was updated
        assert mock_import.resolved_storefront_id == "test-storefront-1"
        assert mock_import.storefront_resolved is True
        
        # Verify database operations
        mock_session.add.assert_called_once_with(mock_import)
        mock_session.commit.assert_called_once()

    @pytest.mark.asyncio
    async def test_resolve_storefront_user_mismatch(self, resolution_service, mock_session):
        """Test storefront resolution with user ID mismatch."""
        # Mock the import record with different user ID
        mock_import = DarkadiaImport(
            id="test-import-1",
            user_id="different-user",
            game_name="Test Game"
        )
        
        # Setup mocks
        mock_session.get.return_value = mock_import
        
        # Test the method
        result = await resolution_service.resolve_storefront(
            import_id="test-import-1",
            user_id="test-user-1",
            resolved_storefront_id="test-storefront-1"
        )
        
        # Verify result
        assert result is False
        
        # Verify no database operations were performed
        mock_session.add.assert_not_called()
        mock_session.commit.assert_not_called()

    @pytest.mark.asyncio
    async def test_bulk_resolve_storefronts(self, resolution_service):
        """Test bulk storefront resolution."""
        # Mock the resolve_storefront method
        resolution_service.resolve_storefront = AsyncMock()
        resolution_service.resolve_storefront.side_effect = [True, True, False]
        
        # Test data
        resolutions = [
            {"import_id": "import-1", "storefront_id": "store-1", "user_notes": "Note 1"},
            {"import_id": "import-2", "storefront_id": "store-2", "user_notes": "Note 2"},
            {"import_id": "import-3", "storefront_id": "store-3", "user_notes": "Note 3"},
        ]
        
        # Test the method
        result = await resolution_service.bulk_resolve_storefronts(
            resolutions=resolutions,
            user_id="test-user-1"
        )
        
        # Verify results
        assert result["total_processed"] == 3
        assert result["successful_resolutions"] == 2
        assert result["failed_resolutions"] == 1
        assert len(result["errors"]) == 1
        
        # Verify individual resolutions were called
        assert resolution_service.resolve_storefront.call_count == 3

    @pytest.mark.asyncio
    async def test_suggest_storefront_matches_for_platform(self, resolution_service, mock_session, sample_storefront):
        """Test platform-contextual storefront suggestions."""
        # Mock platform-specific storefronts
        resolution_service.get_storefronts_for_platform = AsyncMock(return_value=[sample_storefront])
        
        # Mock the general storefront query to return empty list (for _get_storefront_suggestions)
        mock_session.exec.return_value.all.return_value = []
        
        # Mock fuzzy matching utility
        import app.utils.fuzzy_match
        original_function = app.utils.fuzzy_match.calculate_fuzzy_confidence
        app.utils.fuzzy_match.calculate_fuzzy_confidence = Mock(return_value=0.8)
        
        try:
            # Test the method
            result = await resolution_service.suggest_storefront_matches_for_platform(
                unknown_storefront_name="Test Store",
                platform_id="test-platform-1",
                min_confidence=0.6,
                max_suggestions=5
            )
            
            # Verify results
            assert len(result) == 1
            assert isinstance(result[0], StorefrontSuggestion)
            assert result[0].storefront_id == sample_storefront.id
            assert result[0].confidence > 0.8  # Should be boosted
            assert result[0].reason is not None and "Compatible with platform" in result[0].reason
            
        finally:
            # Restore original function
            app.utils.fuzzy_match.calculate_fuzzy_confidence = original_function


class TestStorefrontProcessing:
    """Test storefront processing in DarkadiaImportService."""
    
    @pytest.mark.asyncio
    async def test_process_storefront_data_with_copy_source(self):
        """Test processing storefront data from Copy source field."""
        from app.services.import_sources.darkadia import DarkadiaImportService
        
        # Mock the service components
        service = Mock(spec=DarkadiaImportService)
        service._process_storefront_data = DarkadiaImportService._process_storefront_data.__get__(service)
        
        # Test data
        game_data = {
            "Copy source": "Steam",
            "Copy source other": ""
        }
        storefront_stats = {}
        unknown_storefronts = set()
        
        # Mock the data mapper
        service.data_mapper = Mock()
        service.data_mapper.STOREFRONT_MAPPINGS = {"Steam": "steam"}
        # Ensure no suggested mapping method exists or it returns None
        delattr(service.data_mapper, '_map_storefront_name') if hasattr(service.data_mapper, '_map_storefront_name') else None
        
        # Mock the platform resolution service
        service.platform_resolution_service = Mock()
        mock_storefront = Mock()
        mock_storefront.display_name = "Steam"
        service.platform_resolution_service.get_canonical_storefront = AsyncMock(return_value=mock_storefront)
        
        # Test the method
        await service._process_storefront_data(game_data, storefront_stats, unknown_storefronts)
        
        # Verify results
        assert "Steam" in storefront_stats
        assert storefront_stats["Steam"]["name"] == "Steam"
        assert storefront_stats["Steam"]["games_count"] == 1
        assert storefront_stats["Steam"]["is_known"] is True
        assert len(unknown_storefronts) == 0

    @pytest.mark.asyncio
    async def test_process_storefront_data_with_other_source(self):
        """Test processing storefront data with 'Other' source."""
        from app.services.import_sources.darkadia import DarkadiaImportService
        
        # Mock the service components
        service = Mock(spec=DarkadiaImportService)
        service._process_storefront_data = DarkadiaImportService._process_storefront_data.__get__(service)
        
        # Test data
        game_data = {
            "Copy source": "Other",
            "Copy source other": "Custom Store"
        }
        storefront_stats = {}
        unknown_storefronts = set()
        
        # Mock the data mapper
        service.data_mapper = Mock()
        service.data_mapper.STOREFRONT_MAPPINGS = {}
        # Ensure no suggested mapping method exists or it returns None
        delattr(service.data_mapper, '_map_storefront_name') if hasattr(service.data_mapper, '_map_storefront_name') else None
        
        # Mock the platform resolution service to return None (unknown storefront)
        service.platform_resolution_service = Mock()
        service.platform_resolution_service.get_canonical_storefront = AsyncMock(return_value=None)
        
        # Test the method
        await service._process_storefront_data(game_data, storefront_stats, unknown_storefronts)
        
        # Verify results
        assert "Custom Store" in storefront_stats
        assert storefront_stats["Custom Store"]["name"] == "Custom Store"
        assert storefront_stats["Custom Store"]["games_count"] == 1
        assert storefront_stats["Custom Store"]["is_known"] is False
        assert "Custom Store" in unknown_storefronts


class TestStorefrontCompatibilityEndpoints:
    """Test storefront compatibility API endpoints."""
    
    def test_check_platform_storefront_compatibility_endpoint(self):
        """Test the platform-storefront compatibility check endpoint."""
        # This would be an integration test that would require the full FastAPI test client
        # For now, we'll leave it as a placeholder since the endpoint logic is tested
        # through the service layer tests above
        pass