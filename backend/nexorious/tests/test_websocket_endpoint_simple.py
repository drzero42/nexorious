"""
Simple WebSocket endpoint tests that verify endpoint configuration and basic functionality.
"""

import pytest
from fastapi.testclient import TestClient

from nexorious.main import app


class TestWebSocketEndpointConfiguration:
    """Test WebSocket endpoint configuration and basic setup."""
    
    def setup_method(self):
        """Set up test fixtures."""
        self.client = TestClient(app)
    
    def test_websocket_endpoint_exists(self):
        """Test that the WebSocket endpoint is properly configured in the app."""
        # Check that the WebSocket route exists in the app's routes
        websocket_routes = [route for route in app.routes if hasattr(route, 'path') and '/ws/' in route.path]
        assert len(websocket_routes) > 0, "WebSocket endpoint should be configured"
        
        # Find the Steam import WebSocket route
        steam_ws_routes = [route for route in websocket_routes if 'steam/import/ws' in route.path]
        assert len(steam_ws_routes) > 0, "Steam import WebSocket endpoint should be configured"
    
    def test_websocket_endpoint_requires_authentication(self):
        """Test that WebSocket endpoint requires authentication (rejects unauthenticated connections)."""
        job_id = "test_job_123"
        
        # Try to connect without token - should be rejected
        with pytest.raises(Exception):  # WebSocketDisconnect or similar
            with self.client.websocket_connect(f"/api/steam/import/ws/{job_id}"):
                pass  # Should not reach here
    
    def test_websocket_endpoint_rejects_invalid_token(self):
        """Test that WebSocket endpoint rejects invalid tokens."""
        job_id = "test_job_123"
        invalid_token = "invalid_token"
        
        # Try to connect with invalid token - should be rejected
        with pytest.raises(Exception):  # WebSocketDisconnect or similar
            with self.client.websocket_connect(f"/api/steam/import/ws/{job_id}?token={invalid_token}"):
                pass  # Should not reach here
    
    def test_websocket_manager_import(self):
        """Test that WebSocket manager can be imported successfully."""
        from nexorious.services.websocket_manager import get_websocket_manager, WebSocketConnectionManager
        
        manager = get_websocket_manager()
        assert isinstance(manager, WebSocketConnectionManager)
        assert hasattr(manager, 'authenticate_and_connect')
        assert hasattr(manager, 'send_to_job')
        assert hasattr(manager, 'disconnect')
    
    def test_websocket_event_types_import(self):
        """Test that WebSocket event types can be imported successfully."""
        from nexorious.services.websocket_manager import WebSocketEventType
        
        # Verify all expected event types exist
        expected_events = [
            'IMPORT_STATUS_CHANGE',
            'IMPORT_PROGRESS', 
            'GAME_MATCHED',
            'GAME_NEEDS_REVIEW',
            'GAME_IMPORTED',
            'PLATFORM_ADDED',
            'GAME_SKIPPED',
            'IMPORT_COMPLETE',
            'IMPORT_ERROR',
            'CONNECTION_STATUS',
            'HEARTBEAT'
        ]
        
        for event in expected_events:
            assert hasattr(WebSocketEventType, event), f"WebSocketEventType should have {event}"
    
    def test_steam_import_service_has_websocket_integration(self):
        """Test that Steam import service has WebSocket integration."""
        from nexorious.services.steam_import import SteamImportService
        from unittest.mock import Mock
        
        # Create a mock service instance
        mock_session = Mock()
        mock_igdb_service = Mock()
        service = SteamImportService(mock_session, mock_igdb_service)
        
        # Verify WebSocket manager is initialized
        assert hasattr(service, 'ws_manager')
        
        # Verify WebSocket event emission methods exist
        websocket_methods = [
            '_emit_status_change',
            '_emit_progress_update',
            '_emit_game_matched',
            '_emit_game_needs_review',
            '_emit_game_imported',
            '_emit_platform_added',
            '_emit_game_skipped',
            '_emit_import_complete',
            '_emit_import_error'
        ]
        
        for method in websocket_methods:
            assert hasattr(service, method), f"SteamImportService should have {method}"


class TestWebSocketEndpointBasicFunctionality:
    """Test basic WebSocket endpoint functionality without complex authentication mocking."""
    
    def test_websocket_endpoint_path_pattern(self):
        """Test that WebSocket endpoint uses correct path pattern."""
        from nexorious.api.steam_import import router
        
        # Find the WebSocket route in the router - it should be a WebSocketRoute
        websocket_routes = [route for route in router.routes if hasattr(route, 'path_regex') and 'ws' in str(route.path_regex.pattern)]
        assert len(websocket_routes) > 0, "Steam import router should have WebSocket route"
        
        # Verify the route pattern contains job_id parameter
        ws_route = websocket_routes[0]
        pattern = str(ws_route.path_regex.pattern)
        assert 'job_id' in pattern, "WebSocket route should have job_id parameter"
    
    def test_websocket_endpoint_import_success(self):
        """Test that WebSocket endpoint can be imported without errors."""
        try:
            from nexorious.api.steam_import import websocket_steam_import
            assert callable(websocket_steam_import), "WebSocket endpoint should be callable"
        except ImportError as e:
            pytest.fail(f"Failed to import WebSocket endpoint: {e}")
    
    def test_websocket_dependencies_available(self):
        """Test that all WebSocket dependencies are available."""
        try:
            from fastapi import WebSocket, Query
            from nexorious.core.database import get_session
            from nexorious.services.websocket_manager import get_websocket_manager
            
            # All imports should succeed
            assert WebSocket is not None
            assert Query is not None
            assert get_session is not None
            assert get_websocket_manager is not None
            
        except ImportError as e:
            pytest.fail(f"Failed to import WebSocket dependencies: {e}")