# Enhanced CSV Import Error Reporting

## Overview

The Darkadia CSV import system has been enhanced with comprehensive error reporting capabilities to provide users with detailed, actionable information when import failures occur.

## Problem Statement

Previously, CSV import errors provided minimal context with generic messages like:
- "Failed to create: API error"
- "Failed to update: Unknown error"  
- "Game creation failed"

Users could not easily understand:
- What specifically went wrong
- Which CSV rows had issues
- How to fix the problems
- Whether errors were temporary or permanent

## Solution Implementation

### 1. Structured Error Recording (`merge_strategies.py`)

**Enhanced Error Context Structure:**
- **Error Categories**: Categorized errors into specific types (authentication, network, validation, etc.)
- **CSV Row Tracking**: Track which CSV rows correspond to errors
- **API Error Details**: Capture full API response data and parse meaningful information
- **Context Information**: Store operation context and additional debugging data
- **Timestamp Tracking**: Record when errors occurred

**New `ImportError` Class:**
```python
class ImportError:
    def __init__(
        self,
        category: ErrorCategory,
        message: str,
        game_title: str = "",
        csv_row: Optional[int] = None,
        csv_data: Optional[Dict[str, Any]] = None,
        api_error: Optional[APIException] = None,
        context: Optional[Dict[str, Any]] = None
    )
```

**Error Categories:**
- `AUTHENTICATION` - Login/permission issues
- `NETWORK` - Connectivity problems
- `API_VALIDATION` - Field validation errors
- `PLATFORM_MAPPING` - Platform/storefront mapping issues
- `GAME_CREATION` - Failed to create new games
- `GAME_UPDATE` - Failed to update existing games
- `CSV_DATA` - CSV format or content issues
- `IGDB_INTEGRATION` - IGDB API integration problems
- `UNEXPECTED` - Unknown or unexpected errors

### 2. Enhanced API Error Parsing (`api_client.py`)

**Improved `APIException` Class:**
- **Automatic Error Type Detection**: Determine error category from HTTP status codes
- **Validation Error Extraction**: Parse FastAPI validation errors with field details
- **User-Friendly Messages**: Convert technical API responses to readable messages
- **Troubleshooting Hints**: Provide specific guidance based on error types

**Key Features:**
```python
class APIException(Exception):
    def get_user_friendly_message(self) -> str:
        """Get a user-friendly error message."""
    
    def get_troubleshooting_hint(self) -> str:
        """Get troubleshooting hint based on error type."""
    
    def _extract_validation_errors(self) -> List[Dict[str, Any]]:
        """Extract detailed validation errors from response."""
```

### 3. Comprehensive Error Display (`import_darkadia_csv.py`)

**Enhanced Final Report Generation:**
- **Categorized Error Summary**: Group errors by type with counts and descriptions
- **Detailed Error Information**: Show complete context for each error
- **Specific Troubleshooting Guidance**: Provide targeted advice for each error category
- **Success Rate Analysis**: Calculate and display import success rates
- **Actionable Recommendations**: Suggest next steps based on error patterns

**Report Sections:**
1. **Import Summary Table** - Basic statistics
2. **Error Summary by Category** - Categorized error counts
3. **Detailed Error Information** - Full context for each error
4. **Troubleshooting Guide** - Category-specific guidance
5. **Next Steps & Recommendations** - Actionable next steps

### 4. Backward Compatibility

The enhancement maintains full backward compatibility:
- Existing `error_details` field preserved for legacy code
- All existing error recording methods still work
- Tests updated to account for new fields without breaking changes

## Sample Error Report Output

```
Error Summary by Category
┏━━━━━━━━━━━━━━━━━━┳━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ Category         ┃ Count ┃ Description                            ┃
┡━━━━━━━━━━━━━━━━━━╇━━━━━━━╇━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┩
│ Api Validation   │     2 │ API validation errors                  │
│ Game Creation    │     1 │ Failed to create new games             │
│ Platform Mapping │     3 │ Platform or storefront mapping issues  │
└──────────────────┴───────┴────────────────────────────────────────┘

Detailed Error Information

1. Api Validation Error
   Game: 'Super Mario Bros' | CSV Row: 1 | Error: Invalid game data | API Status: 422
   Validation Details:
     • title: field required
     • rating: ensure this value is less than or equal to 5
   CSV Context: Game 'Super Mario Bros'
   Operation: create_user_game

Platform Mapping Issues (3 errors):
  • Check platform and storefront names in your CSV
  • Verify platform/storefront combinations are valid
  • Some platforms may need to be created by an administrator
  Affected games: Doom, Quake, Wolfenstein
```

## Testing

Comprehensive test suite added in `test_error_reporting.py`:
- **13 test cases** covering all new functionality
- **Error categorization testing**
- **API error parsing validation**
- **Report generation verification**
- **Backward compatibility confirmation**

All existing tests continue to pass with 90 total test cases.

## Benefits

### For Users
- **Clear Understanding**: Know exactly what went wrong and where
- **Easy Location**: CSV row numbers help find problematic data quickly
- **Actionable Guidance**: Specific troubleshooting steps for each error type
- **Success Visibility**: See what worked alongside what failed
- **Time Savings**: Faster problem resolution with detailed context

### For Developers
- **Debugging Support**: Rich error context for troubleshooting
- **Monitoring**: Categorized errors enable better system monitoring
- **Extensibility**: Easy to add new error categories and guidance
- **Maintainability**: Structured error handling improves code quality

## Usage

The enhanced error reporting is automatically active for all CSV imports:

```bash
# Standard import with enhanced error reporting
uv run python scripts/import_darkadia_csv.py sample.csv --username admin --password secret

# All merge strategies benefit from enhanced reporting
uv run python scripts/import_darkadia_csv.py sample.csv --overwrite --username admin --password secret
```

No changes needed to existing import workflows - the enhanced reporting is transparent to users.

## 409 Conflict Enhancement (Latest Update)

### Problem Addressed
Users reported that 409 Conflict errors only showed HTTP status codes without explaining why the conflict occurred or how to resolve it.

### Enhanced 409 Conflict Detection

**Before:**
```
Game: 'Control' | CSV Row: 15 | Error: Failed to create game | API Status: 409 | Details: Game already exists in database
```

**After:**
```
Game: 'Control' | CSV Row: 15 | Error: Failed to create duplicate game | API Status: 409
Conflict Details:
  • Reason: A game with this exact title already exists in the database
  • Existing title: 'Control'
  • Recommendation: Check if this is the same game or modify the title to distinguish different versions
```

### Conflict Types Detected

1. **Duplicate Title Conflicts**
   - Detects when a game with the same title already exists
   - Extracts the conflicting title from the API response
   - Provides guidance on title modification for different editions

2. **Duplicate IGDB ID Conflicts**
   - Identifies when the same IGDB game already exists in the database
   - Shows the conflicting IGDB ID for reference
   - Recommends skipping the import for true duplicates

3. **Generic Conflicts**
   - Handles other types of resource conflicts
   - Preserves the original error message
   - Provides general conflict resolution guidance

### Implementation Details

**Enhanced APIException Class:**
- `_extract_conflict_details()`: Parses 409 responses for specific conflict information
- `conflict_details` property: Stores structured conflict information
- Enhanced user-friendly messages with conflict-specific explanations
- Targeted troubleshooting hints based on conflict type

**Improved Error Display:**
- Dedicated "Conflict Details" section in error reports
- Shows specific reasons, existing data, and actionable recommendations
- Examples of affected games in troubleshooting guides
- Special handling in the troubleshooting guide for duplicate game conflicts

### Testing Coverage

Added 6 new test cases specifically for 409 conflict scenarios:
- Duplicate title detection and parsing
- Duplicate IGDB ID detection and parsing
- Generic conflict handling
- User-friendly message generation
- Conflict-specific troubleshooting hints
- Integration with enhanced error reporting display

## Future Enhancements

Potential future improvements:
- **Export Failed Rows**: Option to export failed CSV rows for correction
- **Interactive Error Resolution**: Fix errors directly in the import interface
- **Error Trend Analysis**: Track error patterns over time
- **Custom Troubleshooting**: User-defined error resolution guidance
- **Existing Game Details**: Show more information about conflicting games (platforms, status, etc.)
- **Smart Duplicate Detection**: Suggest merge operations for similar games

## Impact

This enhancement transforms the CSV import experience from frustrating error-hunting to efficient problem-solving with clear, actionable information that helps users successfully import their game collections.