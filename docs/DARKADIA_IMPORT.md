# Darkadia CSV Import Analysis and Implementation Guide

## Overview

This document provides a comprehensive analysis of the Darkadia CSV export format and defines the implementation strategy for importing Darkadia game collections into Nexorious. The import will be handled by a standalone Python script with configurable merge strategies.

## Darkadia CSV Format Analysis

### Sample Data Summary
- **Total Games**: 1,720 games in sample file
- **Total Rows**: 1,721 (including header)
- **Column Count**: 29 fields
- **Duplicate Handling**: Multiple rows per game for different platforms/storefronts
- **Date Format**: YYYY-MM-DD
- **Boolean Format**: 0/1 integers
- **Rating Scale**: 0-5 decimal scale

### Complete Field Analysis

| # | Field Name | Type | Description | Nexorious Mapping |
|---|------------|------|-------------|-------------------|
| 1 | Name | String | Game title | Game.title |
| 2 | Added | Date | Date added to collection | UserGame.acquired_date |
| 3 | Loved | Boolean | Loved game flag (0/1) | UserGame.is_loved |
| 4 | Owned | Boolean | Currently owned (0/1) | OwnershipStatus mapping |
| 5 | Played | Boolean | Has been played (0/1) | PlayStatus calculation |
| 6 | Playing | Boolean | Currently playing (0/1) | PlayStatus.IN_PROGRESS |
| 7 | Finished | Boolean | Main story completed (0/1) | PlayStatus.COMPLETED |
| 8 | Mastered | Boolean | Side quests completed (0/1) | PlayStatus.MASTERED |
| 9 | Dominated | Boolean | 100% completion (0/1) | PlayStatus.DOMINATED |
| 10 | Shelved | Boolean | Temporarily paused (0/1) | PlayStatus.SHELVED |
| 11 | Rating | Decimal | Personal rating 0-5 | UserGame.personal_rating |
| 12 | Copy label | String | Edition/release identifier | UserGamePlatform metadata |
| 13 | Copy Release | String | Specific release title | UserGamePlatform metadata |
| 14 | Copy platform | String | Platform name | Platform mapping |
| 15 | Copy media | String | Media type (Digital/Physical) | UserGamePlatform metadata |
| 16 | Copy media other | String | Custom media type | UserGamePlatform metadata |
| 17 | Copy source | String | Store/source name | Storefront mapping |
| 18 | Copy source other | String | Custom source name | Storefront mapping |
| 19 | Copy purchase date | Date | Purchase date | UserGame.acquired_date |
| 20 | Copy box | String | Box status | UserGamePlatform metadata |
| 21 | Copy box condition | String | Box condition | UserGamePlatform metadata |
| 22 | Copy box notes | String | Box notes | UserGamePlatform metadata |
| 23 | Copy manual | String | Manual status | UserGamePlatform metadata |
| 24 | Copy manual condition | String | Manual condition | UserGamePlatform metadata |
| 25 | Copy manual notes | String | Manual notes | UserGamePlatform metadata |
| 26 | Copy complete | String | Completeness status | UserGamePlatform metadata |
| 27 | Copy complete notes | String | Completeness notes | UserGamePlatform metadata |
| 28 | Platforms | String | Summary of all platforms | Multi-platform parsing |
| 29 | Notes | String | Personal notes | UserGame.personal_notes |

### Data Examples from Sample

#### Single Platform Game
```csv
"Yakuza 0",2019-12-25,0,1,1,0,1,0,0,0,4.0,Steam,"Yakuza 0",PC,Digital,,Steam,,2019-12-25,N/A,N/A,,N/A,N/A,,N/A,,PC,
```

#### Multi-Platform Game (Abzû - 3 entries)
```csv
Abzû,2016-11-26,1,1,1,0,1,1,1,0,4.5,"HB / Steam",,PC,Digital,,"Humble Bundle",,2016-11-26,N/A,N/A,,N/A,N/A,,N/A,,"PC, PlayStation 4",
,2016-11-26,1,1,1,0,1,1,1,0,4.5,Epic,Abzu,PC,Digital,,Other,Epic,2019-09-09,N/A,N/A,,N/A,N/A,,N/A,,"PC, PlayStation 4",
,2016-11-26,1,1,1,0,1,1,1,0,4.5,PSN,"Abzû (PSN)","PlayStation 4",Digital,,"Sony Entertainment Network",,2021-03-27,N/A,N/A,,N/A,N/A,,N/A,,"PC, PlayStation 4",
```

#### Physical Game
```csv
"Zak McKracken and the Alien Mindbenders",2015-06-10,1,1,1,0,1,1,1,0,5.0,Physical,"Zak McKracken and the Alien Mindbenders",PC,Physical,,,,,N/A,N/A,,N/A,N/A,,N/A,,PC,"Floppy discs!"
```

## Data Transformation Rules

### Play Status Conversion Logic
The Darkadia format uses multiple boolean flags. We need to convert these to our single PlayStatus enum:

```python
def convert_play_status(row):
    """Convert Darkadia play status flags to Nexorious PlayStatus enum."""
    if row['Dominated'] == 1:
        return PlayStatus.DOMINATED
    elif row['Mastered'] == 1:
        return PlayStatus.MASTERED
    elif row['Finished'] == 1:
        return PlayStatus.COMPLETED
    elif row['Playing'] == 1:
        return PlayStatus.IN_PROGRESS
    elif row['Shelved'] == 1:
        return PlayStatus.SHELVED
    elif row['Played'] == 1:
        return PlayStatus.COMPLETED  # Fallback for generic "played"
    else:
        return PlayStatus.NOT_STARTED
```

### Ownership Status Conversion
```python
def convert_ownership_status(row):
    """Convert Darkadia ownership flag to OwnershipStatus enum."""
    return OwnershipStatus.OWNED if row['Owned'] == 1 else OwnershipStatus.NO_LONGER_OWNED
```

### Platform/Storefront Mapping

#### Common Platform Mappings
| Darkadia | Nexorious Platform | Default Storefront |
|----------|-------------------|-------------------|
| PC | PC (Windows) | Steam |
| PlayStation 4 | PlayStation 4 | PlayStation Store |
| PlayStation 5 | PlayStation 5 | PlayStation Store |
| PlayStation Network (PS3) | PlayStation 3 | PlayStation Store |
| Nintendo Switch | Nintendo Switch | Nintendo eShop |
| Xbox 360 Games Store | Xbox 360 | Microsoft Store |
| Mac | PC (Windows) | Steam |
| Linux | PC (Windows) | Steam |

#### Common Storefront Mappings
| Darkadia Source | Nexorious Storefront |
|----------------|---------------------|
| Steam | Steam |
| Epic Games Store | Epic Games Store |
| Epic | Epic Games Store |
| GOG | GOG |
| Sony Entertainment Network | PlayStation Store |
| PSN | PlayStation Store |
| Nintendo eShop | Nintendo eShop |
| Humble Bundle | Humble Bundle |
| Physical | Physical |
| Other | Physical (fallback) |

### Date Handling
- **Primary Date**: Use `Added` field as primary acquired date
- **Secondary Date**: Use `Copy purchase date` if `Added` is missing
- **Format**: YYYY-MM-DD (already compatible with our system)

### Notes Consolidation
- **Primary Notes**: Use `Notes` field
- **Additional Notes**: Append relevant copy notes if present
- **Format**: Combine with separator if multiple note sources exist

## Idempotent Operation Requirement

### Core Principle
The Darkadia CSV import script **MUST** be idempotent, meaning that running the same import command multiple times with the same CSV file should produce identical results in the user's game collection. This design eliminates the need for complex progress tracking and resume functionality.

### Benefits of Idempotent Design
- **Simple Recovery**: Users can safely re-run the same command if the import is interrupted by network issues, API errors, or system problems
- **No State Management**: No need to maintain progress files or intermediate state that could become corrupted
- **Predictable Behavior**: Users always know that re-running the import will not create duplicate games or corrupt existing data
- **Error Resilience**: Temporary failures (network timeouts, API rate limits) can be resolved by simply running the import again
- **Development Simplicity**: No complex resume logic, checkpoint management, or partial state recovery

### Implementation Requirements

#### Game Deduplication
- **Fuzzy Matching**: Use title-based fuzzy matching to detect existing games in the user's collection
- **Conflict Resolution**: Apply the selected merge strategy consistently across runs
- **Platform Handling**: Additive platform associations without creating duplicates

#### Merge Strategy Behavior
- **Interactive Mode**: 
  - Same conflicts should be resolvable the same way on re-runs
  - Batch decisions should apply consistently
  - Skip option should not affect subsequent runs
- **Overwrite Mode**: 
  - Always produces identical final state regardless of starting state
  - CSV data takes precedence consistently
- **Preserve Mode**: 
  - Only adds new games and platforms
  - Never modifies existing data, ensuring consistent results

#### Error Handling
- **Graceful Failures**: Individual game failures should not prevent processing of remaining games
- **Detailed Logging**: Log all operations for troubleshooting without affecting idempotency
- **Partial Success**: Successfully imported games remain imported on subsequent runs

### User Experience
When an import is interrupted, users simply re-run the exact same command:
```bash
# Original command
python import_darkadia_csv.py my_collection.csv --user-id 123 --overwrite

# After interruption, same command continues where it left off
python import_darkadia_csv.py my_collection.csv --user-id 123 --overwrite
```

The script will:
1. Parse the CSV file again
2. Detect which games are already in the collection  
3. Apply the merge strategy to any new or changed games
4. Skip games that are already correctly imported
5. Complete the import with the same final result

## Import Script Implementation Strategy

### Script Location and Structure
```
backend/scripts/import_darkadia_csv.py
```

### Command-Line Interface
```bash
python import_darkadia_csv.py [CSV_FILE] [OPTIONS]

Required Arguments:
  CSV_FILE              Path to Darkadia CSV export file

Options:
  --user-id USER_ID     User ID for import (required)
  --api-base URL        Backend API base URL (default: http://localhost:8000)
  
Merge Strategy (choose one):
  --interactive         Pause and ask user for conflict resolution (default)
  --overwrite          Always use CSV data, overwrite existing data
  --preserve           Never overwrite, only add new games/platforms
  
Additional Options:
  --dry-run            Preview changes without making them
  --batch-size N       Process N games at a time (default: 10)
  --auth-token TOKEN   API authentication token
  --username           Username for authentication (if no token provided)
  --password           Password for authentication (if no token provided)
  --verbose            Enable verbose logging

Note: The import is idempotent - you can safely re-run the same command multiple 
times. If interrupted, simply run the same command again to continue.
```

### Three Merge Strategies

#### 1. Interactive Mode (Default)
**Behavior:**
- Pause execution when conflicts are detected
- Display side-by-side comparison of existing vs CSV data
- Ask user to choose resolution strategy
- Allow batch decisions for similar conflicts
- **Idempotent**: Re-running applies same conflict resolutions consistently

**User Prompts:**
```
⚠️  Game Conflict: "The Witcher 3"
┌─────────────┬─────────────────┬─────────────────┐
│ Field       │ Existing Data   │ CSV Data        │
├─────────────┼─────────────────┼─────────────────┤
│ Rating      │ 3.0             │ 4.5             │
│ Play Status │ MASTERED        │ COMPLETED       │
│ Hours       │ 120             │ 85              │
│ Notes       │ "Great story"   │ "Amazing RPG"   │
└─────────────┴─────────────────┴─────────────────┘

Resolution Options:
  1) Keep existing data
  2) Use CSV data  
  3) Merge intelligently (max hours, highest status, combine notes)
  4) Skip this game
  5) Apply to all similar conflicts

Choice [1-5]: 
```

#### 2. Overwrite Mode (`--overwrite`)
**Behavior:**
- CSV data always takes precedence
- Update all existing game fields with CSV values
- Add new platforms/storefronts (additive)
- Fast, fully automated processing
- No user interaction required
- **Idempotent**: Always produces same final state regardless of re-runs

**Conflict Resolution:**
- Rating: Use CSV rating
- Play Status: Use CSV status
- Notes: Replace with CSV notes
- Dates: Use CSV dates
- Platforms: Merge (existing + CSV platforms)

#### 3. Preserve Mode (`--preserve`)
**Behavior:**
- Never overwrite existing game data
- Only create new games not in collection
- Only add new platforms to existing games
- Skip games that already exist with data
- Safe mode to prevent data loss
- **Idempotent**: Re-running only adds new games/platforms, never duplicates

**Conflict Resolution:**
- Rating: Keep existing, ignore CSV
- Play Status: Keep existing, ignore CSV  
- Notes: Keep existing, ignore CSV
- Dates: Keep existing, ignore CSV
- Platforms: Add new platforms only

### Game Deduplication Strategy

#### Within CSV File
1. **Group by Game Name**: Combine multiple rows with same name
2. **Merge Platform Data**: Create list of all platforms/storefronts
3. **Consolidate Metadata**: Use consistent values across rows
4. **Validate Consistency**: Warn if ratings/status differ between rows

#### Against Existing Database
1. **Exact Match**: Compare normalized game titles
2. **Fuzzy Match**: Use rapidfuzz for similar titles (>85% similarity)
3. **User Confirmation**: Ask user to confirm fuzzy matches in interactive mode
4. **IGDB Lookup**: Use IGDB ID if available for definitive matching

### API Integration Approach

#### Required Endpoints
```python
# Authentication
POST /api/auth/login

# Game Operations  
GET /api/games/search?q={title}       # Find existing games
POST /api/user-games                  # Create new user game
PUT /api/user-games/{id}              # Update existing user game
GET /api/user-games/{id}              # Get user game details

# Platform Operations
GET /api/platforms                    # Get all platforms
GET /api/platforms/{id}/storefronts   # Get platform storefronts
POST /api/user-games/{id}/platforms   # Add platform to user game

# IGDB Integration
GET /api/games/igdb/search?q={title}  # Search IGDB for game
POST /api/games/igdb/import           # Import game from IGDB
```

#### Authentication Strategy
```python
import httpx

class NexoriousAPI:
    def __init__(self, base_url: str, auth_token: str):
        self.base_url = base_url
        self.client = httpx.Client(
            headers={"Authorization": f"Bearer {auth_token}"}
        )
    
    def authenticate(self, username: str, password: str) -> str:
        """Authenticate and return token."""
        response = self.client.post(f"{self.base_url}/api/auth/login", 
                                   json={"username": username, "password": password})
        return response.json()["access_token"]
```

### Error Handling Scenarios

#### Data Validation Errors
- **Missing required fields**: Game name, user ID
- **Invalid dates**: Malformed date strings
- **Invalid ratings**: Outside 0-5 range
- **Unknown platforms**: Platforms not in seed data

#### API Errors
- **Authentication failures**: Invalid credentials or expired tokens
- **Network issues**: Connection timeouts or server errors
- **Rate limiting**: Too many requests per second
- **Data conflicts**: Concurrent modifications

#### Recovery Strategies
- **Retry logic**: Exponential backoff for network errors
- **Progress saving**: Checkpoint after each successful batch  
- **Error logging**: Detailed logs for manual review
- **Partial success**: Continue processing after non-fatal errors

### Progress Tracking and Reporting

#### Console Output
```
Darkadia CSV Import Progress
============================

Phase 1: Parsing CSV file... ✓ (1,720 games found)
Phase 2: Grouping duplicates... ✓ (892 unique games)  
Phase 3: Processing games... [████████████████████████████████████████] 100%

Game Processing Summary:
  • New games created: 742
  • Existing games updated: 150  
  • Games skipped: 0
  • Errors encountered: 3

Platform Associations:
  • PC: 1,245 games
  • PlayStation 4: 387 games
  • Nintendo Switch: 156 games
  • Xbox 360: 89 games
  • PlayStation 3: 67 games

Processing completed in 4m 32s
```

#### Final Report
```
Import Summary Report
====================
Source: darkadia_export_20250730.csv
Mode: Interactive
Date: 2025-01-15 14:32:15

Statistics:
  Total CSV rows processed: 1,720
  Unique games identified: 892
  New games added: 742
  Existing games updated: 150
  Games skipped: 0
  Errors: 3

Error Details:
  1. "Unknown Game Title" - IGDB lookup failed
  2. "Invalid Date Format" - Row 1,245  
  3. "Platform mapping failed" - Unknown platform "Atari 2600"

Recommendations:
  - Review error log at: /tmp/darkadia_import_errors.log
  - Consider manual addition of failed games
  - Update platform mappings for future imports
```

### File Structure and Dependencies

#### Script Dependencies
```python
# Add to backend/pyproject.toml
dependencies = [
    # ... existing dependencies
    "pandas>=2.0.0",      # CSV processing
    "rapidfuzz>=3.10.0",  # Already present - fuzzy matching
    "httpx>=0.28.0",      # Already present - API calls  
    "rich>=13.0.0",       # Console formatting
    "click>=8.0.0",       # Command-line interface
]
```

#### Import Script Structure
```
backend/scripts/
├── __init__.py
├── import_darkadia_csv.py          # Main script
├── darkadia/
│   ├── __init__.py
│   ├── parser.py                   # CSV parsing logic
│   ├── mapper.py                   # Data transformation
│   ├── api_client.py               # Nexorious API client
│   └── merge_strategies.py         # Conflict resolution
└── tests/
    ├── test_darkadia_parser.py
    ├── test_data_mapping.py
    └── sample_darkadia.csv
```

### Testing Strategy

#### Unit Tests
- **CSV Parsing**: Test field extraction and validation
- **Data Transformation**: Test status/platform mapping
- **Merge Logic**: Test conflict resolution strategies
- **API Integration**: Mock API responses and test calls

#### Integration Tests  
- **End-to-End**: Complete import workflow with test data
- **Error Scenarios**: Network failures, invalid data, conflicts
- **Performance**: Large CSV file processing (1000+ games)

#### Test Data
- **Sample CSV**: Subset of Darkadia export for testing
- **Mock Responses**: API responses for various scenarios
- **Edge Cases**: Invalid data, missing fields, conflicts

## Implementation Timeline

### Phase 1: Core Infrastructure (2 days)
- Create documentation file ✓
- Set up script structure and dependencies
- Implement CSV parsing and validation
- Create basic API client

### Phase 2: Data Processing (2 days)  
- Implement game deduplication logic
- Create platform/storefront mapping
- Build data transformation functions
- Add basic merge strategies

### Phase 3: User Interface (1 day)
- Implement interactive mode prompts
- Add command-line argument parsing
- Create progress tracking and reporting
- Add error handling and logging

### Phase 4: Testing and Refinement (1 day)
- Write unit and integration tests
- Performance testing with large files
- Error scenario testing
- Documentation updates

**Total Estimated Time**: 6 days

## Future Enhancements

### Advanced Features
- **Web Interface**: Optional web-based import wizard
- **Multiple Format Support**: Support other collection formats
- **Automatic Updates**: Periodic re-import of updated CSV files
- **Data Validation**: Enhanced duplicate detection and fuzzy matching

### Performance Optimizations
- **Batch Processing**: Optimize API calls with batch endpoints
- **Parallel Processing**: Multi-threaded processing for large files
- **Caching**: Cache platform/storefront mappings
- **Progress Persistence**: Resume from any point in large imports

This comprehensive analysis provides the foundation for implementing a robust, user-friendly Darkadia CSV import solution that preserves data integrity while offering flexibility in how conflicts are resolved.