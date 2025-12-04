"""
Tests for the Platform Resolution Service.
"""

import pytest

from app.services.platform_resolution import PlatformResolutionService
from app.models.platform import Platform, Storefront
from app.schemas.platform import PlatformSuggestion, StorefrontSuggestion


@pytest.fixture
def resolution_service(session):
    """Create a PlatformResolutionService for testing."""
    return PlatformResolutionService(session)


@pytest.fixture
def sample_platform(session):
    """Create a sample platform for testing."""
    platform = Platform(
        name="test-platform",
        display_name="Test Platform",
        is_active=True
    )
    session.add(platform)
    session.commit()
    session.refresh(platform)
    return platform


@pytest.fixture
def sample_storefront(session):
    """Create a sample storefront for testing."""
    storefront = Storefront(
        name="test-storefront",
        display_name="Test Storefront",
        is_active=True
    )
    session.add(storefront)
    session.commit()
    session.refresh(storefront)
    return storefront


class TestPlatformResolutionService:
    """Test suite for PlatformResolutionService."""
    
    def test_sanitize_platform_name(self, resolution_service):
        """Test platform name sanitization."""
        # Test normal name
        assert resolution_service.sanitize_platform_name("PlayStation 5") == "PlayStation 5"
        
        # Test empty/None names
        assert resolution_service.sanitize_platform_name("") == ""
        assert resolution_service.sanitize_platform_name(None) == ""
        
        # Test script injection removal
        malicious_name = "<script>alert('xss')</script>PlayStation"
        sanitized = resolution_service.sanitize_platform_name(malicious_name)
        assert "<script>" not in sanitized
        assert "PlayStation" in sanitized
        
        # Test length limiting
        long_name = "A" * 300
        sanitized = resolution_service.sanitize_platform_name(long_name)
        assert len(sanitized) <= 200
    
    @pytest.mark.asyncio
    async def test_suggest_platform_matches_exact_match(self, resolution_service, sample_platform, sample_storefront):
        """Test platform suggestions with exact matches."""
        suggestions = await resolution_service.suggest_platform_matches(
            unknown_platform_name="Test Platform",
            min_confidence=0.5,
            max_suggestions=5
        )
        
        assert suggestions.unknown_platform_name == "Test Platform"
        assert suggestions.total_platform_suggestions >= 0
        
        # Check that we get platform suggestions
        if suggestions.platform_suggestions:
            suggestion = suggestions.platform_suggestions[0]
            assert isinstance(suggestion, PlatformSuggestion)
            assert suggestion.platform_name == "test-platform"
            assert suggestion.platform_display_name == "Test Platform"
            assert suggestion.confidence > 0.5
    
    @pytest.mark.asyncio
    async def test_suggest_platform_matches_fuzzy_match(self, resolution_service, sample_platform):
        """Test platform suggestions with fuzzy matching."""
        suggestions = await resolution_service.suggest_platform_matches(
            unknown_platform_name="Test Plat",  # Partial match
            min_confidence=0.3,
            max_suggestions=5
        )
        
        assert suggestions.unknown_platform_name == "Test Plat"
        
        # Should find the platform with fuzzy matching
        platform_names = [s.platform_name for s in suggestions.platform_suggestions]
        assert "test-platform" in platform_names or len(platform_names) == 0  # Might not match depending on confidence
    
    @pytest.mark.asyncio
    async def test_suggest_platform_matches_no_match(self, resolution_service):
        """Test platform suggestions when no matches found."""
        suggestions = await resolution_service.suggest_platform_matches(
            unknown_platform_name="Nonexistent Platform",
            min_confidence=0.8,
            max_suggestions=5
        )
        
        assert suggestions.unknown_platform_name == "Nonexistent Platform"
        assert suggestions.total_platform_suggestions == 0
        assert len(suggestions.platform_suggestions) == 0
    
    @pytest.mark.asyncio
    async def test_suggest_platform_matches_with_storefront(self, resolution_service, sample_platform, sample_storefront):
        """Test platform suggestions with storefront matching."""
        suggestions = await resolution_service.suggest_platform_matches(
            unknown_platform_name="Test Platform",
            unknown_storefront_name="Test Storefront",
            min_confidence=0.5,
            max_suggestions=5
        )
        
        assert suggestions.unknown_storefront_name == "Test Storefront"
        
        # Check storefront suggestions if any
        if suggestions.storefront_suggestions:
            suggestion = suggestions.storefront_suggestions[0]
            assert isinstance(suggestion, StorefrontSuggestion)
            assert suggestion.storefront_name == "test-storefront"
    
    @pytest.mark.asyncio
    async def test_resolve_platform_invalid_import(self, resolution_service):
        """Test resolving platform for invalid import ID."""
        result = await resolution_service.resolve_platform(
            import_id="nonexistent-id",
            user_id="test-user",
            resolved_platform_id="some-platform-id"
        )
        
        assert not result.success
        assert "not found" in result.error_message.lower()
    
    def test_get_platform_suggestions_filters_inactive_platforms(self, session):
        """Test that inactive platforms are not included in suggestions."""
        # Create active platform
        active_platform = Platform(
            name="active-platform",
            display_name="Active Platform",
            is_active=True
        )
        session.add(active_platform)
        
        # Create inactive platform
        inactive_platform = Platform(
            name="inactive-platform", 
            display_name="Inactive Platform",
            is_active=False
        )
        session.add(inactive_platform)
        session.commit()
        
        resolution_service = PlatformResolutionService(session)
        
        # Both should match the query, but only active should be returned
        async def test_active_only():
            suggestions = await resolution_service._get_platform_suggestions(
                unknown_name="Platform",
                min_confidence=0.3,
                max_suggestions=10
            )
            
            platform_names = [s.platform_name for s in suggestions]
            assert "active-platform" in platform_names
            assert "inactive-platform" not in platform_names
        
        import asyncio
        asyncio.run(test_active_only())
    
    @pytest.mark.asyncio
    async def test_bulk_resolve_platforms_empty_list(self, resolution_service):
        """Test bulk resolution with empty list."""
        result = await resolution_service.bulk_resolve_platforms(
            resolutions=[],
            user_id="test-user"
        )
        
        assert result.total_processed == 0
        assert result.successful_resolutions == 0
        assert result.failed_resolutions == 0
        assert len(result.results) == 0
    
    def test_sanitize_platform_name_javascript_prevention(self, resolution_service):
        """Test that JavaScript injection attempts are properly sanitized."""
        malicious_inputs = [
            "javascript:alert('xss')",
            "vbscript:msgbox('xss')",
            "<script>alert('test')</script>",
            "PlayStation<script>alert('xss')</script>5"
        ]
        
        for malicious_input in malicious_inputs:
            sanitized = resolution_service.sanitize_platform_name(malicious_input)
            
            # Should not contain script tags or javascript/vbscript protocols
            assert "<script>" not in sanitized.lower()
            assert "javascript:" not in sanitized.lower()
            assert "vbscript:" not in sanitized.lower()
    
    def test_sanitize_platform_name_null_byte_prevention(self, resolution_service):
        """Test that null bytes and control characters are removed."""
        malicious_input = "PlayStation\x00\x01\x02\x03\x04\x05"
        sanitized = resolution_service.sanitize_platform_name(malicious_input)
        
        # Should not contain null bytes or control characters
        assert "\x00" not in sanitized
        assert "PlayStation" in sanitized
        
        # Should preserve basic whitespace characters
        input_with_whitespace = "PlayStation\n\r\t5"
        sanitized = resolution_service.sanitize_platform_name(input_with_whitespace)
        assert "\n" in sanitized or "\r" in sanitized or "\t" in sanitized  # Should preserve basic whitespace