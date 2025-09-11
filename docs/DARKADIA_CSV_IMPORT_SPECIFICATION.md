# Darkadia CSV Import Data Processing Specification

## Table of Contents
1. [Overview](#overview)
2. [CSV Format Specification](#csv-format-specification)
3. [Copy Consolidation Process](#copy-consolidation-process)
4. [Data Transformation Requirements](#data-transformation-requirements)
5. [Platform/Storefront Resolution Framework](#platformstorefront-resolution-framework)
6. [Idempotent Import Process Design](#idempotent-import-process-design)
7. [Sync Process to User Collection](#sync-process-to-user-collection)
8. [Unsync and Reversibility Requirements](#unsync-and-reversibility-requirements)
9. [Data Integrity and State Management](#data-integrity-and-state-management)

---

## Overview

This specification defines the data processing requirements for importing Darkadia CSV game collection exports into the Nexorious game collection system. The specification is implementation-neutral and focuses on logical data transformation, consolidation, and synchronization processes.

### Key Principles
- **Data Preservation**: Original CSV data must never be lost during processing
- **Idempotent Operations**: Running the same import multiple times produces identical results
- **Reversible Sync**: Users can sync and unsync games without losing import data
- **Copy Tracking**: Individual game copies (platform/storefront combinations) are preserved
- **Resolution Separation**: Platform/storefront resolution is distinct from sync operations

---

## CSV Format Specification

### Field Structure
Darkadia CSV exports contain 29 fields representing game collection data with copy-specific information:

| Field # | Field Name | Type | Description | Required |
|---------|------------|------|-------------|----------|
| 1 | Name | String(500) | Game title | Yes |
| 2 | Added | Date (YYYY-MM-DD) | Date added to collection | No |
| 3 | Loved | Boolean (0/1) | Loved game flag | No |
| 4 | Owned | Boolean (0/1) | Currently owned | No |
| 5 | Played | Boolean (0/1) | Has been played | No |
| 6 | Playing | Boolean (0/1) | Currently playing | No |
| 7 | Finished | Boolean (0/1) | Main story completed | No |
| 8 | Mastered | Boolean (0/1) | Side quests completed | No |
| 9 | Dominated | Boolean (0/1) | 100% completion | No |
| 10 | Shelved | Boolean (0/1) | Temporarily paused | No |
| 11 | Rating | Decimal (0.0-5.0) | Personal rating | No |
| 12 | Copy label | String(200) | Edition/release identifier | No |
| 13 | Copy Release | String(200) | Specific release title | No |
| 14 | Copy platform | String(100) | Platform name for this copy | No |
| 15 | Copy media | String(50) | Media type (Digital/Physical) | No |
| 16 | Copy media other | String(100) | Custom media type | No |
| 17 | Copy source | String(100) | Store/source name | No |
| 18 | Copy source other | String(100) | Custom source name | No |
| 19 | Copy purchase date | Date (YYYY-MM-DD) | Purchase date | No |
| 20 | Copy box | String(50) | Box status | No |
| 21 | Copy box condition | String(50) | Box condition | No |
| 22 | Copy box notes | String(500) | Box notes | No |
| 23 | Copy manual | String(50) | Manual status | No |
| 24 | Copy manual condition | String(50) | Manual condition | No |
| 25 | Copy manual notes | String(500) | Manual notes | No |
| 26 | Copy complete | String(50) | Completeness status | No |
| 27 | Copy complete notes | String(500) | Completeness notes | No |
| 28 | Platforms | String(500) | Comma-separated platform list | No |
| 29 | Notes | String(2000) | Personal notes | No |

### Multi-Row Game Representation

#### Copy Patterns
Darkadia represents games with multiple platform/storefront combinations as multiple CSV rows:

**Pattern 1: Single Copy Game**
```csv
"Game Title",2024-01-15,1,1,1,0,1,0,0,0,4.5,"Steam Edition","Game Title",PC,Digital,,Steam,,2024-01-15,N/A,N/A,,N/A,N/A,,N/A,,"PC",""
```

**Pattern 2: Multi-Copy Game**
```csv
"Game Title",2024-01-15,1,1,1,0,1,0,0,0,4.5,"Steam Edition","Game Title",PC,Digital,,Steam,,2024-01-15,N/A,N/A,,N/A,N/A,,N/A,,"PC, PlayStation 4",""
"",2024-01-15,1,1,1,0,1,0,0,0,4.5,"PS4 Edition","Game Title","PlayStation 4",Digital,,"PlayStation Store",,2024-01-15,N/A,N/A,,N/A,N/A,,N/A,,"PC, PlayStation 4",""
```

**Pattern 3: Zero-Copy Game (Fallback)**
```csv
"Game Title",2024-01-15,1,1,1,0,1,0,0,0,4.5,"","","",,"","","",N/A,N/A,,N/A,N/A,,N/A,,"PC",""
```

#### Field Precedence Rules

**Copy-Specific Data (Higher Priority):**
- Copy platform
- Copy source / Copy source other
- Copy-specific metadata (box, manual, purchase date, etc.)

**General Game Data (Lower Priority):**
- Platforms field (comma-separated list)
- Base game metadata (rating, notes, play status flags)

### Data Validation Requirements

#### Critical Validations
- **Name field**: Must not be empty for valid game records
- **Boolean fields**: Must be 0, 1, or convertible to boolean
- **Rating field**: Must be 0.0-5.0 or empty/null
- **Date fields**: Must be valid YYYY-MM-DD format or empty/null

#### Data Cleaning Rules
- **Continuation rows**: Empty Name fields indicate continuation of previous game
- **Text normalization**: Trim whitespace, convert "nan" strings to empty
- **Boolean conversion**: Convert numeric 0/1 to proper boolean values
- **Date parsing**: Support multiple date formats with fallback to null

---

## Copy Consolidation Process

### Game Identification Logic

#### Primary Grouping Rule
CSV rows are grouped into games using **exact name matching** on the Name field after normalization:
- Trim whitespace
- Case-sensitive comparison
- Empty names inherit from previous row (continuation pattern)

#### Copy Detection Within Game Groups
Within each game group, individual copies are identified by presence of copy-specific data:

**Real Copy Criteria** (any of the following):
- Copy platform field has value
- Copy source field has value  
- Copy source other field has value
- Copy-specific metadata present (box, manual, purchase date)

**Fallback Copy Criteria**:
- No real copy data present
- Uses Platforms field as fallback platform source
- Single fallback copy created per unique platform in Platforms field

### Consolidation Data Structures

#### ConsolidatedGame Entity
```
ConsolidatedGame:
  - name: String (normalized game name)
  - base_data: Object (merged non-copy specific fields)
  - copies: Array<CopyData> (all identified copies)
  - csv_row_numbers: Array<Integer> (original CSV row tracking)
  - requires_platform_resolution: Boolean
  - requires_storefront_resolution: Boolean
```

#### CopyData Entity  
```
CopyData:
  - platform: String (platform name from copy or fallback)
  - storefront: String (storefront name from copy fields)
  - media_type: String (Digital/Physical/etc)
  - copy_identifier: String (unique identifier within game)
  - csv_row_number: Integer (original CSV row reference)
  - is_real_copy: Boolean (true for copy-specific, false for fallback)
  - physical_metadata: Object (box, manual, condition details)
  - purchase_date: Date
  - copy_label: String
  - copy_release: String
```

### Data Merging Rules

When consolidating multiple CSV rows for the same game:

#### Base Data Consolidation
- **Rating**: Use highest numerical value across all rows
- **Dates**: Use most recent date from Added field  
- **Boolean Flags**: Apply OR logic (true if any row is true)
- **Notes**: Concatenate unique values with " | " separator
- **Owned Status**: True if any row has Owned = 1

#### Play Status Derivation
Apply hierarchy to boolean flags (highest priority wins):
1. **Dominated** → PlayStatus.DOMINATED
2. **Mastered** → PlayStatus.MASTERED  
3. **Finished** → PlayStatus.COMPLETED
4. **Shelved** → PlayStatus.SHELVED
5. **Playing** → PlayStatus.IN_PROGRESS
6. **Played** → PlayStatus.COMPLETED
7. **Default** → PlayStatus.NOT_STARTED

#### Copy Preservation
- Each copy maintains independent metadata
- Copy identifiers ensure uniqueness within game
- Platform/storefront combinations tracked separately
- Original CSV row numbers preserved for audit

---

## Data Transformation Requirements

### Platform Name Mapping

#### Common Platform Transformations
```
CSV Platform Name → Normalized Platform Name
"PC" → "PC (Windows)"
"Mac" → "PC (Windows)"  
"Linux" → "PC (Windows)"
"PlayStation 4" → "PlayStation 4"
"PlayStation 5" → "PlayStation 5"
"PlayStation Network (PS3)" → "PlayStation 3"
"Nintendo Switch" → "Nintendo Switch"
"Xbox 360 Games Store" → "Xbox 360"
"Xbox One" → "Xbox One"
"Xbox Series X/S" → "Xbox Series X/S"
```

#### Platform Resolution Requirements
- **Automatic Mapping**: Apply known transformations
- **Unknown Platform Detection**: Flag unmapped platform names
- **User Resolution Required**: Present options for unknown platforms
- **Fallback Handling**: Use "Unknown Platform" category when resolution fails

### Storefront Name Mapping

#### Common Storefront Transformations
```
CSV Storefront Name → Normalized Storefront Name
"Steam" → "Steam"
"Epic Games Store" → "Epic Games Store"
"Epic" → "Epic Games Store" 
"GOG" → "GOG"
"Sony Entertainment Network" → "PlayStation Store"
"PSN" → "PlayStation Store"
"Nintendo eShop" → "Nintendo eShop"
"Microsoft Store" → "Microsoft Store"
"Humble Bundle" → "Humble Bundle"
"Physical" → "Physical"
"Other" → [Requires custom resolution]
```

#### Storefront Resolution Logic
1. **Check Copy source field** for standard storefront names
2. **Check Copy source other field** for custom storefront names  
3. **Apply platform context** - validate storefront compatibility with platform
4. **Default assignment** - use platform's default storefront if no explicit source
5. **Unknown handling** - flag for user resolution

### Data Type Transformations

#### Date Field Processing
**Priority Order:**
1. Copy purchase date (copy-specific)
2. Added date (general game data)  
3. Null if no valid dates found

**Date Format Handling:**
- Primary: YYYY-MM-DD
- Fallback: MM/DD/YYYY, DD/MM/YYYY
- Invalid dates convert to null

#### Rating Normalization
- **Input Range**: 0.0-5.0 (Darkadia scale)
- **Validation**: Reject values outside range
- **Null Handling**: Empty or invalid ratings become null
- **Precision**: Maintain decimal precision

#### Boolean Field Processing
- **Input Format**: 0/1 integers or boolean values
- **Conversion**: Convert to proper boolean type
- **Default**: False for null/invalid values

---

## Platform/Storefront Resolution Framework

### Resolution Categories

#### Automatic Resolution
**Criteria**: Platform/storefront name exactly matches known mapping table
**Process**: Apply transformation immediately without user input
**Examples**: "Steam" → Steam, "PlayStation 4" → PlayStation 4

#### Fuzzy Matching Resolution  
**Criteria**: Platform/storefront name similar to known entries (similarity threshold)
**Process**: Present best matches with confidence scores for user selection
**Examples**: "Playstation4" → PlayStation 4 (95% confidence)

#### Unknown Resolution Required
**Criteria**: No exact or fuzzy matches found in mapping tables
**Process**: Present options: create new platform/storefront, map to existing, skip
**Examples**: "Custom Gaming Platform" → requires user decision

### Resolution Data Requirements

#### Platform Resolution Data
```
PlatformResolution:
  - original_name: String (from CSV)
  - resolution_status: Enum (resolved, pending, ignored, conflict)
  - resolved_platform_id: String (mapped platform ID)
  - resolution_method: Enum (automatic, fuzzy, user, admin)
  - confidence_score: Float (0.0-1.0 for fuzzy matches)
  - alternatives: Array<ResolutionOption> (other possible matches)
```

#### Storefront Resolution Data
```
StorefrontResolution:
  - original_name: String (from Copy source/Copy source other)
  - platform_context: String (associated platform for validation)
  - resolution_status: Enum (resolved, pending, ignored, conflict)
  - resolved_storefront_id: String (mapped storefront ID)
  - resolution_method: Enum (automatic, fuzzy, user, default)
  - is_platform_compatible: Boolean (storefront available on platform)
```

### Resolution Precedence Rules

#### Copy Platform vs Platforms Field
**Rule**: Copy platform field takes absolute precedence over Platforms field
**Rationale**: Copy-specific data is more accurate than generic platform list
**Implementation**: Ignore Platforms field entirely if Copy platform has value

#### Storefront Source Priority
**Order**:
1. Copy source (if not "Other")
2. Copy source other (when Copy source = "Other")  
3. Platform default storefront (fallback)
4. Physical/Unknown (final fallback)

#### Resolution Conflict Handling
**Multiple Matches**: Present all options ranked by confidence
**Platform-Storefront Incompatibility**: Flag and require user resolution
**Missing Required Resolution**: Allow skip but track for later resolution

---

## Idempotent Import Process Design

### Core Idempotency Principle
Running the same CSV import operation multiple times with identical input must produce identical results in the system, regardless of:
- Previous import state
- Interruptions or failures
- System restarts
- Concurrent operations

### Three-Phase Import Architecture

#### Phase 1: CSV Parsing and Validation
**Input**: Raw CSV file
**Output**: Validated list of CSV row data
**Idempotency**: Same CSV always produces same parsed data
**Operations**:
- Parse CSV structure and validate format
- Apply data cleaning and normalization
- Detect and flag validation errors
- Generate consistent row ordering

#### Phase 2: Consolidation and Import Record Creation
**Input**: Validated CSV row data  
**Output**: Import records stored in staging tables
**Idempotency**: Same CSV data always creates identical import records
**Operations**:
- Apply copy consolidation logic to group related rows
- Transform data according to mapping rules
- Create import tracking records with consistent identifiers
- Preserve original CSV data for audit

#### Phase 3: Sync to User Collection
**Input**: Import records from staging tables
**Output**: User collection updated with new/modified games
**Idempotency**: Same import records always produce same collection state
**Operations**:
- Match games to IGDB database
- Create or update UserGame records
- Create UserGamePlatform associations
- Update import records with sync status

### Deduplication Strategy

#### CSV Internal Deduplication
**Game Level**: Group rows by exact name match
**Copy Level**: Identify distinct copies within same game
**Data Consistency**: Validate consistent base data across game copies

#### Database Deduplication
**Existing Game Detection**: Compare against user's current collection
**IGDB Matching**: Use IGDB ID as definitive game identifier
**Platform Association Deduplication**: Prevent duplicate platform/storefront combinations

### State Consistency Requirements

#### Import Batch Tracking
**Batch Identification**: Each import operation gets unique batch identifier
**Batch State**: Track overall import status (in_progress, completed, failed)
**Partial Recovery**: Support resuming from any phase boundary

#### Data Integrity Guarantees
**Atomicity**: Each phase completes fully or rolls back entirely
**Consistency**: Data relationships maintained across all operations
**Isolation**: Concurrent imports don't interfere with each other
**Durability**: Completed operations persist through system failures

---

## Sync Process to User Collection

### Sync State Logic

#### Sync State Definition
A Darkadia import record is considered "synced" when:
1. **IGDB Resolution**: Import game is matched to IGDB game database
2. **User Collection Presence**: A UserGame record exists for the user and IGDB game
3. **Platform Association**: A UserGamePlatform record exists matching:
   - User ID (importing user)
   - Game ID (resolved IGDB game)  
   - Platform ID (resolved platform)
   - Storefront ID (resolved storefront)

#### Sync State Detection Query
```sql
-- Pseudo-SQL for sync state detection
SELECT EXISTS (
  SELECT 1 
  FROM user_games ug
  JOIN user_game_platforms ugp ON ug.id = ugp.user_game_id
  WHERE ug.user_id = :importing_user_id
    AND ug.game_id = :resolved_igdb_game_id
    AND ugp.platform_id = :resolved_platform_id
    AND ugp.storefront_id = :resolved_storefront_id
) AS is_synced
```

### Resolution vs Sync Distinction

#### Platform/Storefront Resolution Status
**Purpose**: Track mapping of CSV platform/storefront names to database IDs
**Storage**: Import tracking tables with resolution metadata
**States**: `unresolved`, `resolved`, `pending_user_input`, `ignored`
**Independence**: Resolution status independent of sync status

#### Sync Status  
**Purpose**: Track presence of game in user's actual collection
**Detection**: Query-based verification of UserGamePlatform associations
**States**: `synced`, `not_synced` (calculated, not stored)
**Dependency**: Requires resolution to be complete before sync possible

### Sync Process Flow

#### Prerequisites for Sync
1. **IGDB Match**: Import game must be matched to IGDB database entry
2. **Platform Resolution**: Platform name resolved to database platform ID
3. **Storefront Resolution**: Storefront name resolved to database storefront ID (if applicable)
4. **No Conflicts**: No duplicate associations would be created

#### Sync Operations Sequence
1. **Create/Update UserGame**: 
   - Use consolidated base data (rating, notes, play status, dates)
   - Apply merge strategy if UserGame already exists
2. **Create UserGamePlatform Association**:
   - Link to resolved platform and storefront IDs
   - Include copy-specific metadata
   - Set association as available/active
3. **Update Import Tracking**:
   - Record successful sync timestamp
   - Link import record to created UserGamePlatform
   - Update sync status flags

### Data Mapping Specifications

#### ConsolidatedGame → UserGame Mapping
```
ConsolidatedGame.base_data.rating → UserGame.personal_rating
ConsolidatedGame.base_data.notes → UserGame.personal_notes  
ConsolidatedGame.base_data.play_status → UserGame.play_status
ConsolidatedGame.base_data.loved → UserGame.is_loved
ConsolidatedGame.base_data.added_date → UserGame.acquired_date
ConsolidatedGame.base_data.owned → UserGame.ownership_status
```

#### CopyData → UserGamePlatform Mapping
```
CopyData.resolved_platform_id → UserGamePlatform.platform_id
CopyData.resolved_storefront_id → UserGamePlatform.storefront_id  
CopyData.physical_metadata → UserGamePlatform.metadata (JSON)
CopyData.is_available → UserGamePlatform.is_available
CopyData.copy_identifier → UserGamePlatform.notes (for reference)
```

### Conflict Resolution During Sync

#### UserGame Conflicts
**Scenario**: User already has the game in collection with different data
**Resolution Options**:
- Preserve existing (no overwrite)
- Replace with CSV data (full overwrite) 
- Merge intelligently (combine data)
- Prompt user for decision

#### Platform Association Conflicts  
**Scenario**: User already has game on same platform/storefront
**Resolution**: Skip creating duplicate association, log as already synced

#### Data Validation Conflicts
**Scenario**: CSV data fails validation during sync
**Resolution**: Log error, continue with other copies, report validation failures

---

## Unsync and Reversibility Requirements

### Unsync Process Definition

#### Simple Unsync Operation
**Purpose**: Remove games from user collection while preserving all import data
**Scope**: Delete UserGame and UserGamePlatform records created during sync
**Preservation**: Keep all import tracking records intact for potential re-sync

#### Unsync Operation Steps
1. **Identify Target Associations**: Find UserGamePlatform records matching import data
2. **Remove Platform Associations**: Delete UserGamePlatform records
3. **Remove UserGame (Conditional)**: Delete UserGame only if no other platform associations remain
4. **Preserve Import Records**: Keep all import tracking data unchanged
5. **Update Sync Status**: Sync status automatically becomes "not synced"

### Reversibility Mechanism

#### Data Preservation Requirements
**Original CSV Data**: Complete CSV row data stored in import records
**Transformation Metadata**: Platform/storefront resolution data preserved  
**Copy Information**: Individual copy details and identifiers maintained
**Audit Information**: Import timestamps, batch IDs, operation history

#### Re-sync Capability
**Prerequisite**: Import records exist with resolved platform/storefront data
**Process**: Run standard sync operation using preserved import data
**Result**: Identical user collection state as original sync
**Limitations**: Requires resolution data to be intact

### Bulk Unsync Operations

#### Unsync All Games
**Scope**: Remove all games from user collection that originated from Darkadia imports
**Process**: Iterate through all import records, perform unsync operation for each
**Safety**: Preserve all import data for complete reversibility

#### Selective Unsync  
**By Import Batch**: Unsync all games from specific import operation
**By Date Range**: Unsync games imported within date range
**By Platform**: Unsync games from specific platform/storefront combinations
**By Resolution Status**: Unsync games with specific resolution characteristics

### Data Preservation Guarantees

#### CSV Data Persistence
**Guarantee**: Original CSV data never deleted unless user explicitly requests full reset
**Storage**: Raw CSV row data stored in JSON format in import records
**Access**: CSV data remains queryable and exportable after unsync

#### Resolution Data Persistence  
**Platform Mappings**: Resolved platform IDs and mapping metadata preserved
**Storefront Mappings**: Resolved storefront IDs and resolution confidence scores maintained
**User Decisions**: Manual resolution choices stored for consistency in re-imports

#### Audit Trail Preservation
**Import Operations**: Timestamps, batch IDs, file hashes maintained
**Sync/Unsync Events**: Operation history preserved for troubleshooting
**Data Lineage**: Traceability from CSV source to collection associations maintained

### Recovery Scenarios

#### Accidental Unsync Recovery
**Problem**: User accidentally removes games from collection
**Solution**: Re-run sync operation using preserved import records
**Result**: Collection restored to identical previous state

#### Partial Import Failure Recovery
**Problem**: Import process fails mid-operation  
**Solution**: Re-run import operation (idempotent design handles partial state)
**Result**: Import completes successfully regardless of previous failure point

#### Data Corruption Recovery
**Problem**: User collection data becomes inconsistent
**Solution**: Unsync all Darkadia games, then re-sync from preserved import data
**Result**: Clean, consistent collection state restored from source data

---

## Data Integrity and State Management

### Original Data Preservation

#### CSV Data Storage Requirements
**Format**: Complete CSV row stored as JSON object in import records
**Fields**: All 29 CSV fields preserved exactly as imported
**Encoding**: UTF-8 encoding maintained for international characters
**Structure**: Preserve original field names and data types

#### Data Immutability
**Rule**: Original CSV data fields never modified after initial import
**Exception**: Data cleaning operations applied during initial parsing only
**Audit**: Changes to derived/transformed data logged separately from originals

### Copy Tracking and Identification

#### Copy Identifier Generation
**Purpose**: Uniquely identify each copy within a game
**Components**: Platform name + Storefront name + Copy label + Row number
**Format**: Deterministic string generation for consistent identification
**Uniqueness**: Guaranteed unique within single game, may repeat across different games

#### Copy Metadata Structure
```json
{
  "copy_id": "pc_steam_deluxe_row_15",
  "platform": "PC (Windows)",
  "storefront": "Steam", 
  "media_type": "Digital",
  "copy_label": "Deluxe Edition",
  "copy_release": "Game of the Year Edition",
  "purchase_date": "2024-01-15",
  "physical_data": {
    "box_status": "N/A",
    "box_condition": null,
    "box_notes": null,
    "manual_status": "N/A", 
    "manual_condition": null,
    "manual_notes": null,
    "complete_status": "N/A",
    "complete_notes": null
  },
  "csv_row_number": 15,
  "is_real_copy": true
}
```

### State Management Architecture

#### Import Record States
**Parsed**: CSV data successfully parsed and validated
**Consolidated**: Multiple rows consolidated into game entities
**Platform Resolved**: Platform names mapped to database IDs
**Storefront Resolved**: Storefront names mapped to database IDs  
**IGDB Matched**: Game matched to IGDB database entry
**Ready for Sync**: All prerequisites met for user collection sync
**Synced**: Game present in user collection with platform associations

#### State Transition Rules
**Sequential Dependencies**: Later states require earlier states to be complete
**Parallel Resolution**: Platform and storefront resolution can occur independently
**State Persistence**: State information stored in import tracking records
**State Queries**: Current state determinable through database queries

### Error Handling and Validation

#### Data Validation Stages
**CSV Structure Validation**: Required fields, data types, format compliance
**Copy Consolidation Validation**: Consistent data across game copies
**Platform Resolution Validation**: Valid platform/storefront combinations
**IGDB Matching Validation**: Successful game identification
**Sync Operation Validation**: No conflicts with existing collection data

#### Error Recovery Strategies
**Partial Failure Tolerance**: Individual game failures don't block batch processing
**Error Reporting**: Detailed error messages with context and suggested resolutions
**Retry Mechanisms**: Temporary failures (network, API) can be retried
**Manual Intervention**: Complex errors flagged for user resolution

#### Data Consistency Checks
**Referential Integrity**: All foreign key relationships validated
**Business Rule Compliance**: Platform-storefront compatibility verified
**Duplicate Detection**: Prevent creation of duplicate associations
**State Consistency**: Verify state transitions follow defined rules

### Audit and Traceability Requirements

#### Operation Logging
**Import Operations**: Start time, completion time, record counts, error counts
**Resolution Operations**: Platform/storefront mappings applied, confidence scores
**Sync Operations**: Games synced, associations created, conflicts resolved
**Unsync Operations**: Associations removed, data preserved

#### Data Lineage Tracking
**Source Traceability**: Map user collection entries back to original CSV rows
**Transformation History**: Record all data transformations applied
**Resolution History**: Track platform/storefront resolution decisions
**Modification History**: Log any changes to imported data

#### Performance Monitoring
**Processing Metrics**: CSV parsing time, consolidation time, sync time
**Database Performance**: Query execution times, index utilization
**Error Rates**: Validation failures, resolution failures, sync failures
**User Experience**: Import completion times, resolution workflow efficiency

---

## Conclusion

This specification defines a comprehensive framework for processing Darkadia CSV game collection imports with focus on data integrity, reversibility, and user control. The design emphasizes:

- **Data Preservation**: Original CSV data is never lost
- **Flexible Resolution**: Platform and storefront mapping with user oversight
- **Reversible Operations**: Sync and unsync without data loss
- **Copy Granularity**: Individual game copies tracked and managed
- **Idempotent Processing**: Reliable, repeatable import operations

Implementation of this specification should result in a robust import system that handles the complexity of Darkadia's rich game collection data while maintaining the simplicity and reliability expected by users.