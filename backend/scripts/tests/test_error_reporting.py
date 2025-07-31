"""
Tests for enhanced error reporting in the Darkadia CSV import system.

This module tests the new structured error reporting capabilities,
including error categorization, detailed messages, and troubleshooting guidance.
"""

import pytest
from unittest.mock import Mock, patch
from datetime import datetime
from pathlib import Path

from scripts.darkadia.merge_strategies import (
    ErrorCategory, ImportError, MergeStrategy, InteractiveMerger,
    OverwriteMerger, PreserveMerger
)
from scripts.darkadia.api_client import APIException, NexoriousAPIClient


class TestImportError:
    """Test the ImportError class for structured error reporting."""
    
    def test_import_error_creation(self):
        """Test basic ImportError creation with all fields."""
        api_error = APIException("Test API error", 400, {"detail": "Bad request"})
        
        error = ImportError(
            category=ErrorCategory.GAME_CREATION,
            message="Failed to create game",
            game_title="Test Game",
            csv_row=5,
            csv_data={"Name": "Test Game", "Status": "Completed"},
            api_error=api_error,
            context={"operation": "create_user_game"}
        )
        
        assert error.category == ErrorCategory.GAME_CREATION
        assert error.message == "Failed to create game"
        assert error.game_title == "Test Game"
        assert error.csv_row == 5
        assert error.api_error == api_error
        assert error.context["operation"] == "create_user_game"
    
    def test_import_error_to_dict(self):
        """Test conversion of ImportError to dictionary format."""
        api_error = APIException("Test error", 422, {
            "detail": [{"loc": ["field"], "msg": "validation failed"}]
        })
        
        error = ImportError(
            category=ErrorCategory.API_VALIDATION,
            message="Validation failed",
            game_title="Test Game",
            api_error=api_error
        )
        
        error_dict = error.to_dict()
        
        assert error_dict["category"] == "api_validation"
        assert error_dict["message"] == "Validation failed"
        assert error_dict["game_title"] == "Test Game"
        assert error_dict["api_status_code"] == 422
        assert "timestamp" in error_dict
    
    def test_import_error_detailed_message(self):
        """Test generation of detailed error messages."""
        api_error = APIException("Test error", 400, {"detail": "Invalid data"})
        
        error = ImportError(
            category=ErrorCategory.GAME_UPDATE,
            message="Update failed",
            game_title="Super Mario Bros",
            csv_row=10,
            api_error=api_error
        )
        
        detailed_msg = error.get_detailed_message()
        
        assert "Super Mario Bros" in detailed_msg
        assert "CSV Row: 10" in detailed_msg
        assert "Update failed" in detailed_msg
        assert "API Status: 400" in detailed_msg
        assert "Invalid data" in detailed_msg
    
    def test_import_error_minimal_creation(self):
        """Test ImportError creation with minimal required fields."""
        error = ImportError(
            category=ErrorCategory.UNEXPECTED,
            message="Something went wrong"
        )
        
        assert error.category == ErrorCategory.UNEXPECTED
        assert error.message == "Something went wrong"
        assert error.game_title == ""
        assert error.csv_row is None
        assert error.csv_data == {}
        assert error.api_error is None
        assert error.context == {}


class TestAPIException:
    """Test the enhanced APIException class."""
    
    def test_api_exception_error_type_determination(self):
        """Test automatic error type determination from status codes."""
        test_cases = [
            (400, "validation"),
            (401, "authentication"),
            (403, "authorization"),
            (404, "not_found"),
            (409, "conflict"),
            (422, "validation"),
            (450, "client_error"),
            (500, "server_error"),
            (None, "network")
        ]
        
        for status_code, expected_type in test_cases:
            exception = APIException("Test error", status_code)
            assert exception.error_type == expected_type
    
    def test_api_exception_validation_error_extraction(self):
        """Test extraction of FastAPI validation errors."""
        response_data = {
            "detail": [
                {
                    "loc": ["body", "game_title"],
                    "msg": "field required",
                    "type": "missing"
                },
                {
                    "loc": ["body", "rating"],
                    "msg": "ensure this value is less than or equal to 5",
                    "type": "value_error.number.not_le",
                    "input": 10
                }
            ]
        }
        
        exception = APIException("Validation failed", 422, response_data)
        
        assert len(exception.validation_errors) == 2
        assert exception.validation_errors[0]["field"] == "body.game_title"
        assert exception.validation_errors[0]["message"] == "field required"
        assert exception.validation_errors[1]["field"] == "body.rating"
        assert exception.validation_errors[1]["input"] == 10
    
    def test_api_exception_user_friendly_message(self):
        """Test generation of user-friendly error messages."""
        # Test with validation errors
        response_data = {
            "detail": [
                {"loc": ["title"], "msg": "field required"},
                {"loc": ["rating"], "msg": "invalid value"}
            ]
        }
        
        exception = APIException("Validation failed", 422, response_data)
        friendly_msg = exception.get_user_friendly_message()
        
        assert "Validation failed" in friendly_msg
        assert "title: field required" in friendly_msg
        assert "rating: invalid value" in friendly_msg
    
    def test_api_exception_troubleshooting_hints(self):
        """Test troubleshooting hint generation."""
        test_cases = [
            (401, "authentication", "Check your username and password"),
            (422, "validation", "Check the data you're trying to submit"),
            (404, "not_found", "The requested resource was not found"),
            (500, "server_error", "The server encountered an error")
        ]
        
        for status_code, error_type, expected_hint_part in test_cases:
            exception = APIException("Test error", status_code)
            hint = exception.get_troubleshooting_hint()
            assert expected_hint_part in hint
    
    def test_api_exception_conflict_detection_duplicate_title(self):
        """Test detection of duplicate title conflicts."""
        response_data = {
            "detail": "Game with title 'Control' already exists"
        }
        
        exception = APIException("Conflict error", 409, response_data)
        
        assert exception.conflict_details is not None
        assert exception.conflict_details['type'] == 'duplicate_title'
        assert exception.conflict_details['conflicting_title'] == 'Control'
        assert 'exact title already exists' in exception.conflict_details['reason']
        assert 'modify the title' in exception.conflict_details['recommendation']
    
    def test_api_exception_conflict_detection_duplicate_igdb_id(self):
        """Test detection of duplicate IGDB ID conflicts."""
        response_data = {
            "detail": "Game already exists in database",
            "igdb_id": 12345,
            "game_title": "Control"
        }
        
        exception = APIException("Conflict error", 409, response_data)
        
        assert exception.conflict_details is not None
        assert exception.conflict_details['type'] == 'duplicate_igdb_id'
        assert exception.conflict_details['conflicting_igdb_id'] == 12345
        assert exception.conflict_details['game_title'] == 'Control'
        assert 'IGDB ID already exists' in exception.conflict_details['reason']
        assert 'duplicate entry' in exception.conflict_details['recommendation']
    
    def test_api_exception_conflict_detection_generic(self):
        """Test detection of generic conflicts."""
        response_data = {
            "detail": "Resource already exists with these properties"
        }
        
        exception = APIException("Conflict error", 409, response_data)
        
        assert exception.conflict_details is not None
        assert exception.conflict_details['type'] == 'generic_conflict'
        assert exception.conflict_details['reason'] == "Resource already exists with these properties"
    
    def test_api_exception_conflict_user_friendly_message(self):
        """Test user-friendly messages for conflicts."""
        # Test duplicate title
        response_data = {"detail": "Game with title 'Super Mario Bros' already exists"}
        exception = APIException("Conflict", 409, response_data)
        
        friendly_msg = exception.get_user_friendly_message()
        assert "Duplicate game title: 'Super Mario Bros' already exists in your collection" in friendly_msg
        
        # Test duplicate IGDB ID
        response_data = {
            "detail": "Game already exists in database",
            "igdb_id": 98765
        }
        exception = APIException("Conflict", 409, response_data)
        
        friendly_msg = exception.get_user_friendly_message()
        assert "Duplicate game: IGDB ID 98765 already exists in the database" in friendly_msg
    
    def test_api_exception_conflict_troubleshooting_hints(self):
        """Test conflict-specific troubleshooting hints."""
        # Test duplicate title hint
        response_data = {"detail": "Game with title 'Zelda' already exists"}
        exception = APIException("Conflict", 409, response_data)
        
        hint = exception.get_troubleshooting_hint()
        assert "modify the title to distinguish different versions" in hint
        
        # Test duplicate IGDB ID hint
        response_data = {
            "detail": "Game already exists in database",
            "igdb_id": 555
        }
        exception = APIException("Conflict", 409, response_data)
        
        hint = exception.get_troubleshooting_hint()
        assert "duplicate entry - consider skipping this import" in hint


class TestMergeStrategyErrorRecording:
    """Test error recording in merge strategies."""
    
    @pytest.fixture
    def mock_api_client(self):
        """Create a mock API client."""
        return Mock(spec=NexoriousAPIClient)
    
    def test_structured_error_recording(self, mock_api_client):
        """Test that structured errors are recorded correctly."""
        merger = OverwriteMerger(mock_api_client, dry_run=False)
        merger.current_csv_row = 5
        
        csv_data = {"Name": "Test Game", "Status": "Completed"}
        api_error = APIException("Create failed", 400, {"detail": "Invalid data"})
        
        merger._record_error(
            category=ErrorCategory.GAME_CREATION,
            message="Failed to create game",
            game_title="Test Game",
            csv_data=csv_data,
            api_error=api_error,
            context={"operation": "create_user_game"}
        )
        
        assert merger.results["errors"] == 1
        assert len(merger.results["structured_errors"]) == 1
        assert len(merger.results["error_details"]) == 1  # Backward compatibility
        
        structured_error = merger.results["structured_errors"][0]
        assert structured_error.category == ErrorCategory.GAME_CREATION
        assert structured_error.message == "Failed to create game"
        assert structured_error.game_title == "Test Game"
        assert structured_error.csv_row == 5
        assert structured_error.csv_data == csv_data
        assert structured_error.api_error == api_error
        assert structured_error.context["operation"] == "create_user_game"
    
    def test_csv_row_tracking(self, mock_api_client):
        """Test that CSV row numbers are tracked correctly."""
        merger = InteractiveMerger(Mock(), mock_api_client, dry_run=False)
        
        # Test different row numbers
        test_rows = [1, 5, 100]
        
        for row_num in test_rows:
            merger.current_csv_row = row_num
            merger._record_error(
                category=ErrorCategory.CSV_DATA,
                message="Test error",
                game_title=f"Game {row_num}"
            )
        
        assert len(merger.results["structured_errors"]) == len(test_rows)
        
        for i, expected_row in enumerate(test_rows):
            assert merger.results["structured_errors"][i].csv_row == expected_row
            assert merger.results["structured_errors"][i].game_title == f"Game {expected_row}"
    
    def test_backward_compatibility_error_recording(self, mock_api_client):
        """Test that legacy error recording still works."""
        merger = PreserveMerger(mock_api_client, dry_run=False)
        
        merger._record_simple_error("Old style error", "Test Game")
        
        assert merger.results["errors"] == 1
        assert len(merger.results["error_details"]) == 1
        assert len(merger.results["structured_errors"]) == 1
        
        # Should default to UNEXPECTED category
        structured_error = merger.results["structured_errors"][0]
        assert structured_error.category == ErrorCategory.UNEXPECTED
        assert structured_error.message == "Old style error"
        assert structured_error.game_title == "Test Game"


class TestErrorReportingIntegration:
    """Test integration of error reporting with import functions."""
    
    def test_generate_final_report_with_structured_errors(self):
        """Test that the enhanced report generator works with structured errors."""
        # Import here to avoid circular imports
        from scripts.import_darkadia_csv import generate_final_report
        
        # Create mock structured errors
        api_error = APIException("Validation failed", 422, {
            "detail": [{"loc": ["title"], "msg": "field required"}]
        })
        
        error1 = ImportError(
            category=ErrorCategory.GAME_CREATION,
            message="Failed to create game",
            game_title="Super Mario Bros",
            csv_row=1,
            api_error=api_error
        )
        
        error2 = ImportError(
            category=ErrorCategory.PLATFORM_MAPPING,
            message="Invalid platform",
            game_title="Zelda",
            csv_row=2
        )
        
        results = {
            'total_processed': 10,
            'new_games': 7,
            'updated_games': 1,
            'skipped_games': 0,
            'errors': 2,
            'structured_errors': [error1, error2],
            'error_details': ["Legacy error 1", "Legacy error 2"]
        }
        
        csv_file = Path("test.csv")
        merge_strategy = "overwrite"
        
        # Should not raise an exception - this is the main test
        try:
            generate_final_report(results, csv_file, merge_strategy)
            test_passed = True
        except Exception as e:
            test_passed = False
            pytest.fail(f"generate_final_report raised an exception: {e}")
        
        assert test_passed
    
    def test_generate_final_report_no_errors(self):
        """Test report generation when there are no errors."""
        from scripts.import_darkadia_csv import generate_final_report
        
        results = {
            'total_processed': 5,
            'new_games': 5,
            'updated_games': 0,
            'skipped_games': 0,
            'errors': 0,
            'structured_errors': [],
            'error_details': []
        }
        
        csv_file = Path("test.csv")
        merge_strategy = "interactive"
        
        # Should not raise an exception - this is the main test
        try:
            generate_final_report(results, csv_file, merge_strategy)
            test_passed = True
        except Exception as e:
            test_passed = False
            pytest.fail(f"generate_final_report raised an exception: {e}")
        
        assert test_passed
    
    def test_generate_final_report_with_conflict_details(self):
        """Test that conflict details are properly displayed in the final report."""
        from scripts.import_darkadia_csv import generate_final_report
        
        # Create a 409 conflict error with detailed information
        conflict_api_error = APIException(
            "Game creation failed", 
            409, 
            {
                "detail": "Game with title 'Control' already exists",
                "game_title": "Control"
            }
        )
        
        conflict_error = ImportError(
            category=ErrorCategory.GAME_CREATION,
            message="Failed to create duplicate game",
            game_title="Control",
            csv_row=15,
            csv_data={"Name": "Control", "Platform": "PC"},
            api_error=conflict_api_error,
            context={"operation": "create_user_game"}
        )
        
        results = {
            'total_processed': 1,
            'new_games': 0,
            'updated_games': 0,
            'skipped_games': 0,
            'errors': 1,
            'structured_errors': [conflict_error],
            'error_details': [conflict_error.get_detailed_message()]
        }
        
        csv_file = Path("test_conflicts.csv")
        merge_strategy = "overwrite"
        
        # Should not raise an exception and should display conflict details
        try:
            generate_final_report(results, csv_file, merge_strategy)
            test_passed = True
        except Exception as e:
            test_passed = False
            pytest.fail(f"generate_final_report with conflicts raised an exception: {e}")
        
        assert test_passed


if __name__ == "__main__":
    pytest.main([__file__])