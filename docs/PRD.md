# Game Collection Management Service - Product Requirements Document

## Executive Summary

The Game Collection Management Service is a self-hostable web application designed to help users organize, track, and manage their personal video game collections across multiple platforms and storefronts. The service provides comprehensive collection management, progress tracking, and integration with major gaming platforms.

## Product Vision

To create the definitive self-hosted solution for personal game collection management that automatically syncs with gaming storefronts to give users a unified view of their digital libraries, with powerful backlog management, progress tracking, and organization features.

## Target Users

- **Primary**: Gaming enthusiasts with large collections across multiple platforms
- **Secondary**: Casual gamers who want to organize their digital libraries
- **Tertiary**: Game collectors with diverse acquisition sources

## Core Value Propositions

1. **Automatic Storefront Sync**: Connect Steam, Epic, PlayStation, and more - your games appear automatically
2. **Unified Collection View**: See all your games across every platform and storefront in one place
3. **Backlog Management**: Know what to play next and identify games you'll never touch
4. **Progress Tracking**: Track completion status from "Not Started" to "Dominated"
5. **Self-Hosted Privacy**: Complete control over your gaming data

## Success Metrics

- **Easy to deploy**: Simple deployment with minimal configuration
- **Easy to use**: Intuitive interface with comprehensive game management features
- **Secure by default**: JWT authentication with bcrypt password hashing

## Product Requirements

### Phase 1: Core Collection Management (MVP)

#### 1.0 API Development
**Priority**: P0 (Critical)
- RESTful API with OpenAPI documentation
- JWT authentication and authorization
- CORS configuration for frontend access

#### 1.1 Initial Setup & User Authentication
**Priority**: P0 (Critical)
- **User Story**: As an administrator, I want to create the initial admin user on first startup and manage all subsequent user accounts

##### 1.1.1 First-Run Admin Setup
- Display admin creation screen on first startup (empty user table)
- Automatically load platform/storefront seed data during admin creation
- Seed data function is idempotent and safe to run multiple times

##### 1.1.2 User Authentication
- Username-based login (no self-registration)
- Secure password hashing (bcrypt/scrypt)
- JWT tokens with refresh mechanism
- Username uniqueness enforced system-wide
- **UX Requirements**:
  - Username field must be focused by default on login form
  - Username field must be first in tab order for keyboard navigation

#### 1.2 Game Library Management
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to add games from IGDB to my collection so I can track what I own across all platforms and storefronts in a unified view, including games I've completed but no longer own
- **Backend Requirements**:
  - RESTful endpoints for CRUD operations on games
  - Game metadata storage with comprehensive fields including IGDB slug for proper link generation
  - **IGDB Data Tracking**: Games table must include a `last_updated` field that automatically tracks when IGDB metadata was last refreshed
  - Multi-platform and multi-storefront association (multiple storefronts per platform supported)
  - Ownership tracking through storefront associations
  - Support for games with no platform associations when ownership status indicates the game is no longer owned
  - **Automatic Ownership Status Management**: When the last platform is removed from an owned game, automatically change ownership status to "no_longer_owned"; when a platform is added to a "no_longer_owned" game, automatically change ownership status to "owned"
  - Duplicate detection and prevention at the game level (not platform level)
  - IGDB integration for game lookup and metadata retrieval with slug field storage
  - All games must be sourced from IGDB (no manual game creation)
- **Frontend Requirements**:
  - Game library list and grid views showing unified game cards
  - Each game appears once with all owned platforms/storefronts displayed as indicators/badges
  - Platform and storefront indicators clearly showing all ownership locations
  - Basic search interface for game titles with simple filtering options
  - Bulk selection and operations on unique games
  - **Multi-Selection UX Enhancement**: When games are selected for bulk operations, clicking anywhere on another game card should select that game rather than following the normal navigation link, providing a more intuitive multi-selection experience
  - IGDB game search interface with candidate selection
  - Game metadata acceptance/confirmation screen
  - Game editing interface for adding/removing platform and storefront ownership
  - IGDB rating display formatting: Convert IGDB ratings from integer (0-100) to decimal format (0.0-10.0) for proper user display
- **Game Addition Flow** (IGDB-Only):
  1. User searches for game by title using IGDB integration
  2. System presents IGDB game candidates with ownership status indicators
  3. User selects game and configures platforms/storefronts with automatic defaults
  4. System adds IGDB game to collection or updates existing entry to prevent duplicates
- **Acceptance Criteria**:
  - Games appear once in collection with platform/storefront indicators
  - IGDB integration provides accurate metadata and search
  - All games must be sourced from IGDB database
  - Duplicate detection prevents redundant entries
  - Bulk operations work on unified game view

#### 1.2.5 Game Editing & Platform Management
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to manage which platforms and storefronts I own games on and edit my personal data so I can keep my collection accurate
- **Backend Requirements**:
  - RESTful endpoints for updating platform/storefront associations and personal data
  - Platform/storefront addition and removal for existing games
  - **Automatic Ownership Status Transitions**: When platforms are added or removed, automatically update ownership status (remove last platform → "no_longer_owned", add platform to "no_longer_owned" game → "owned")
  - Validation to work with automatic ownership status transitions
  - Personal data editing (notes, ratings, progress, tags) separate from IGDB metadata
- **Frontend Requirements**:
  - Game editing form for personal data (notes, ratings, progress, tags)
  - Platform and storefront management interface within game editing
  - Add/remove platform associations with visual feedback
  - Add/remove storefront associations per platform
  - Allow removal of all platforms when ownership status is set to "no longer owned"
  - Confirmation dialogs for removing platform/storefront ownership
  - Bulk editing capabilities for multiple games
  - Visual indicators showing current ownership status during editing
  - Clear separation between IGDB metadata (read-only) and personal data (editable)
- **Game Editing Flow**:
  1. User selects a game from their collection
  2. System displays game editing interface with current personal data and ownership
  3. User can modify personal data (notes, ratings, progress, tags)
  4. User can add new platforms/storefronts to their ownership
  5. User can remove existing platforms/storefronts from their ownership
  6. System automatically updates ownership status based on platform changes (remove last platform → "no_longer_owned", add platform to "no_longer_owned" game → "owned") and validates all changes
  7. Changes are saved and reflected immediately in the collection view
  8. Game continues to appear once in collection with updated platform/storefront indicators
- **Acceptance Criteria**:
  - Platform/storefront ownership can be added/removed with automatic ownership status transitions
  - Personal data (notes, ratings, progress) can be edited freely
  - IGDB metadata (title, description, cover art) is read-only and cannot be manually edited
  - Ownership status automatically changes when last platform is removed or first platform is added
  - Bulk editing supported with immediate UI updates

#### 1.2.6 Automatic Game Cleanup
**Priority**: P1 (High)
- **User Story**: As a user, when I remove a game from my collection, I want the system to automatically clean up unreferenced game data so the database stays optimized without me needing to worry about whether other users still have the game
- **Backend Requirements**:
  - Games in the `games` table are automatically deleted when the last `user_games` association is removed
  - Games are automatically deleted when the last `wishlist` entry referencing them is removed
  - Automatic cleanup checks occur after any UserGame or Wishlist deletion operation
  - Admin-only direct game deletion endpoints remain available for emergency cleanup scenarios
  - All cleanup operations are performed within database transactions to ensure data consistency
- **Frontend Requirements**:
  - No changes needed - users continue to "remove games from collection" as they do now
  - Game deletion in the UI represents removing from user's collection, not system-wide deletion
  - Admin interface may show warnings that direct game deletion is rarely needed due to automatic cleanup
- **User Experience**:
  - Users only interact with their personal collection - they don't need to understand global game data management
  - Removing a game from collection appears instant to the user
  - System automatically handles database optimization in the background
- **Data Protection Requirements**:
  - Games are only deleted when NO user references exist (no user_games, no wishlist entries)
  - Cleanup operations are atomic to prevent data corruption during concurrent access
  - All associated data (aliases, metadata) is properly cleaned up when games are removed
- **Acceptance Criteria**:
  - When a user removes the last reference to a game, the game record is automatically deleted from the database
  - Games with any remaining user references (collections or wishlists) are preserved
  - No manual database maintenance required for game cleanup
  - Admin direct deletion endpoints remain functional for emergency scenarios
  - All cleanup operations maintain data integrity and referential consistency

#### 1.3 Platform & Storefront Tracking (Admin-Only Management)
**Priority**: P0 (Critical)
- **User Story**: As an administrator, I want to manage the available platforms and storefronts in the system so that users can accurately track their game ownership, while as a user, I want to associate my games with existing platforms and storefronts so I know where to find them
- **Backend Requirements**:
  - Simplified platform and storefront data models with minimal fields
  - Platform model: name, display_name, icon_url, default_storefront_id, source (official/custom)
  - Storefront model: name, display_name, icon_url, base_url, source (official/custom)
  - API endpoints for managing platform associations with support for multiple storefronts per platform
  - **ADMIN-ONLY ACCESS**: All platform/storefront management operations (create, update, delete) require admin privileges
  - **SECURITY NOTE**: Platform and storefront management is restricted to admins to maintain data consistency and prevent unauthorized system configuration changes
  - **Admin Editing Behavior**: When admin edits official platform/storefront, source changes to "custom"
  - Default storefront assignment for platforms (admin-only configuration)
  - API endpoints for managing platform default storefront relationships (admin-only)
  - Idempotent seed data function for platform and storefront population (admin-triggered)
  - Function automatically runs during initial admin account creation
  - Function can be manually triggered by admin users at any time
  - **Seed Data Behavior**: Only overwrites platforms/storefronts with source=official, preserves custom entries
- **Frontend Requirements**:
  - Platform selection interface (for users to associate games with existing platforms)
  - Multi-select storefront interface per platform (for users to associate games with existing storefronts)
  - Storefront linking components
  - Platform filtering and sorting
  - **ADMIN-ONLY**: Complete platform/storefront management interface (create, edit, delete platforms and storefronts)
  - **ADMIN-ONLY**: Interface for setting default storefronts per platform
  - **ADMIN-ONLY**: Manual seed data loading interface
  - Automatic default storefront selection when platform is chosen during game addition
  - Platform filtering during game addition and editing based on IGDB platform data
  - Primary platform list shows only platforms reported by IGDB for the selected game
  - "Others" expandable section contains all remaining platforms, collapsed by default
  - Users can still select any platform from the "Others" section to handle IGDB data inaccuracies
  - Clear visual distinction between admin-only management features and user association features
- **Seed Data Content**:
  - **Platforms**: PC (Windows), PlayStation 5, PlayStation 4, PlayStation 3, Xbox Series X/S, Xbox One, Xbox 360, Nintendo Switch, Nintendo Wii, iOS, Android
  - **Storefronts**: Steam, Epic Games Store, GOG, PlayStation Store, Microsoft Store, Nintendo eShop, Itch.io, Origin/EA App, Apple App Store, Google Play Store, Humble Bundle, Physical
  - **Default Platform-Storefront Mappings**:
    - PC (Windows) → Steam
    - PlayStation 5 → PlayStation Store
    - PlayStation 4 → PlayStation Store
    - PlayStation 3 → PlayStation Store
    - Xbox Series X/S → Microsoft Store
    - Xbox One → Microsoft Store
    - Xbox 360 → Microsoft Store
    - Nintendo Switch → Nintendo eShop
    - Nintendo Wii → Nintendo eShop
    - iOS → Apple App Store
    - Android → Google Play Store
- **Acceptance Criteria**:
  - Only admins can create/modify platforms and storefronts
  - Users can only associate existing platforms/storefronts with games
  - Seed data loaded automatically during initial admin setup
  - Default storefront auto-selected when choosing platforms
  - Seed data loading is idempotent and admin-triggered

#### 1.3.5 IGDB Platform Filtering
**Priority**: P1 (High)
- **User Story**: As a user, I want to see only relevant platforms when adding or editing games so I can quickly find the platforms the game was actually released on
- **Requirements**:
  - During game addition and editing, filter platform list based on IGDB platform data
  - Show IGDB-reported platforms prominently in the main platform selection area
  - Provide an "Others" expandable section containing all remaining platforms
  - "Others" section should be collapsed by default but easily accessible
  - Maintain full platform selection capability for cases where IGDB data is incomplete or incorrect
  - Apply filtering consistently across both Add Game and Edit Game interfaces
- **Implementation Details**:
  - IGDB platform data should be retrieved and cached during game search/selection
  - Platform filtering should work with the existing default storefront selection logic
  - "Others" section should clearly indicate these are additional platforms not reported by IGDB
  - Users should be able to expand/collapse the "Others" section as needed
- **Acceptance Criteria**:
  - Add Game interface shows IGDB platforms first, others in collapsed section
  - Edit Game interface applies same filtering logic when modifying platform ownership
  - All platforms remain selectable despite filtering
  - "Others" section provides clear visual distinction from main platform list
  - Platform filtering integrates seamlessly with existing default storefront selection

#### 1.3.6 Platform-Storefront Associations
**Priority**: P1 (High)
- **User Story**: As a user, I want to see only relevant storefronts when selecting platforms so I can quickly find the storefronts that are actually available for each platform
- **Requirements**:
  - Each platform should have a list of associated/supported storefronts beyond just the default storefront
  - During game addition and editing, organize storefronts for each platform into two sections:
    - **Primary section**: Storefronts associated with the selected platform (shown by default)
    - **"Other" section**: Non-associated storefronts (collapsed by default, expandable)
  - This provides clearer organization and reduces choice paralysis while maintaining flexibility
  - Apply consistent storefront filtering across both Add Game and Edit Game interfaces
- **Implementation Details**:
  - Create many-to-many relationship between platforms and storefronts for associations
  - Update seed data with realistic platform-storefront associations:
    - PC (Windows) → Steam, Epic Games Store, GOG, Origin/EA App, Microsoft Store
    - PlayStation 5/4/3 → PlayStation Store, Physical
    - Xbox Series X/S/One/360 → Microsoft Store, Physical
    - Nintendo Switch/Wii → Nintendo eShop, Physical
    - iOS → Apple App Store, Epic Games Store
    - Android → Google Play Store, Epic Games Store
  - Admin interface for managing platform-storefront associations using simple checkboxes/buttons
  - Integration with existing default storefront selection logic
- **Acceptance Criteria**:
  - Game addition interface shows associated storefronts first, others in collapsed "Other" section
  - Game editing interface applies same storefront filtering logic
  - All storefronts remain selectable despite filtering for flexibility
  - "Other" section provides clear visual distinction from primary storefronts
  - Admin can manage platform-storefront associations through simple interface
  - Storefront filtering works seamlessly with existing default storefront selection

#### 1.3.7 Platform and Storefront Official Logos
**Priority**: P2 (Medium)
- **User Story**: As a user, I want platform and storefront badges to use official logos so I can quickly recognize them with familiar branding
- **Requirements**:
  - Replace placeholder text/icons with official platform and storefront logos
  - All logos must be used legally with proper attribution and compliance
  - Support high-resolution displays (2x assets for retina displays)
  - Consistent visual styling and sizing across all platforms and storefronts
  - Fallback to text-based badges when logos are unavailable
- **Legal and Attribution Requirements**:
  - All logos must be sourced legally (official press kits, public APIs, or permissive licenses)
  - Proper attribution must be included in README.md for all used logos
  - Compliance with each platform/storefront's brand guidelines and usage policies
  - Documentation of logo sources and permissions for audit purposes
- **Technical Implementation**:
  - Store logo files in organized directory structure under `frontend/static/logos/`
  - Use optimized formats (SVG preferred, PNG for raster graphics)
  - Implement responsive logo sizing for different screen sizes
  - Create logo management system in admin interface for future updates
  - Support both dark and light theme variants where available
- **Logo Sources and Attribution**:
  - Steam: Valve Corporation press kit/brand guidelines
  - Epic Games Store: Epic Games press resources  
  - PlayStation: Sony Interactive Entertainment press assets
  - Xbox: Microsoft press resources
  - Nintendo: Nintendo press materials
  - All other platforms and storefronts: respective official sources
- **Acceptance Criteria**:
  - All platform and storefront badges display official logos instead of text
  - Logos are visually consistent in size and styling
  - High-resolution displays show crisp, clear logos
  - Attribution is properly documented in README.md
  - Logo usage complies with all brand guidelines
  - Admin interface allows for logo updates and management
  - Fallback system works when logos fail to load

#### 1.4 Progress Tracking
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to track my progress through games so I can see what I've completed
- **Backend Requirements**:
  - Progress tracking data model with status categories including completion levels
  - API endpoints for updating play status and completion
  - Time tracking with manual entry support
  - Personal notes storage and retrieval
- **Frontend Requirements**:
  - Status selection dropdown/buttons with completion level options
  - Time tracking input forms
  - Notes editor with rich text support
  - Progress visualization components
- **Play Status Categories**:
  - **Not Started**: Haven't begun playing
  - **In Progress**: Currently playing
  - **Completed**: Finished main story/campaign
  - **Mastered**: Completed main story plus all side quests and content
  - **Dominated**: 100% completion including all achievements/trophies
  - **Shelved**: Temporarily paused with intent to return
  - **Dropped**: Permanently abandoned
  - **Replay**: Playing again after previous completion
- **Acceptance Criteria**:
  - Status updates with completion levels (Not Started → Dominated)
  - Manual time tracking and rich text notes
  - Progress changes reflected immediately

#### 1.5 Personal Rating System
**Priority**: P1 (High)
- **User Story**: As a user, I want to rate games I've played so I can remember which ones I enjoyed
- **Backend Requirements**:
  - Rating system API with 1-5 stars
  - "Loved" designation endpoint
  - Custom tagging system with CRUD operations
  - Tag-based filtering and search
- **Frontend Requirements**:
  - Star rating component
  - Loved games toggle
  - Tag creation and assignment interface
  - Tag-based filtering controls
  - Rating display in game lists
- **Acceptance Criteria**:
  - API preserves ratings and loved status
  - Frontend star rating is interactive and responsive
  - Loved games have special visual treatment
  - Tags are searchable and filterable
  - Ratings display prominently in all views

#### 1.6 Admin User Management
**Priority**: P0 (Critical)
- **User Story**: As an administrator, I want to manage all users and system settings through a dedicated admin interface

##### 1.6.1 Admin Dashboard
- **Requirements**:
  - Dedicated admin section accessible only to users with is_admin=true
  - Navigation clearly indicates admin-only areas
  - Dashboard shows system statistics and user overview
- **Acceptance Criteria**:
  - Non-admin users cannot access admin routes
  - Admin UI is clearly distinguished from regular user interface
  - System health and statistics are displayed

##### 1.6.2 User Management
- **Backend Requirements**:
  - CRUD endpoints for user accounts (admin-only)
  - Username and password creation for new users
  - User activation/deactivation capabilities
  - Password reset functionality for any user
  - User role management (admin/regular user)
  - User activity monitoring
- **Frontend Requirements**:
  - User list with search and filtering
  - User creation form (username and password only)
  - User edit capabilities (username, active status, admin role)
  - Password reset interface
  - User deletion with data handling options
  - **UX Requirements**:
    - Username field must be focused by default on initial admin setup form
    - Username field must be first in tab order for keyboard navigation
- **Acceptance Criteria**:
  - Only admins can create new user accounts
  - Usernames must be unique across the system
  - Passwords are securely hashed before storage
  - Admin can reset any user's password
  - User deletion properly handles related data

##### 1.6.3 System Configuration
- **Requirements**:
  - Platform and storefront management (already defined in 1.3)
  - Seed data management functionality in admin interface
  - System-wide settings management
  - Import/export job monitoring
  - Database maintenance tools
- **Acceptance Criteria**:
  - All system configuration requires admin privileges
  - Configuration changes take effect immediately

### Phase 2: Storefront Sync

#### 2.1 Sync Architecture
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to connect my gaming accounts and have my games automatically appear in Nexorious
- **Requirements**:
  - Background worker system using NATS for job coordination
  - Per-user storefront configuration and credentials
  - Automatic IGDB matching for synced games
  - Manual matching interface for unmatched games
  - Ignore/skip functionality for unwanted games
  - Re-sync capability to pull new purchases
- **Acceptance Criteria**:
  - Users can connect multiple storefronts
  - Games are automatically matched with IGDB entries
  - Unmatched games can be manually matched or ignored
  - Sync status is clearly visible to users
  - Re-authentication prompts when credentials expire

#### 2.2 Steam Sync
**Priority**: P0 (Critical) - **IMPLEMENTED**
- **User Story**: As a user, I want my Steam library automatically synced to Nexorious
- **Integration**: Steam Web API
- **Platform/Storefront**: PC (Windows) / Steam
- **Features**:
  - Automatic library import via background worker
  - IGDB matching with manual override capability
  - Ignore/un-ignore workflow for unwanted games
  - Categorization: Matched, Unmatched, Ignored, In Sync
- **Acceptance Criteria**:
  - Steam configuration required before sync is available
  - Games properly matched with IGDB entries
  - Platform/storefront associations correctly set

#### 2.3 Epic Games Store Sync
**Priority**: P0 (Critical) - **IMPLEMENTED**
- **User Story**: As a user, I want my Epic Games library automatically synced to Nexorious
- **Integration**: Device code OAuth via legendary
- **Platform/Storefront**: PC (Windows) / Epic Games Store
- **Features**:
  - Multi-user config isolation via XDG_CONFIG_HOME
  - Auth expiration handling and re-authentication support
  - IGDB matching with manual override capability
- **Acceptance Criteria**:
  - Device code authentication flow works smoothly
  - Games properly matched with IGDB entries
  - Re-authentication prompts when tokens expire

#### 2.4 PlayStation Network Sync
**Priority**: P0 (Critical) - **IMPLEMENTED**
- **User Story**: As a user, I want my PlayStation library automatically synced to Nexorious
- **Integration**: NPSSO token authentication via PSNAWP library
- **Platform/Storefront**: PlayStation 4, PlayStation 5 / PlayStation Store
- **Features**:
  - Multi-platform support (PS4/PS5)
  - Token expiration handling
  - IGDB matching with manual override capability
- **Acceptance Criteria**:
  - NPSSO token authentication works correctly
  - Games properly categorized by platform (PS4 vs PS5)
  - Platform/storefront associations correctly set

#### 2.5 GOG Sync
**Priority**: P1 (High) - **PLANNED**
- **User Story**: As a user, I want my GOG library automatically synced to Nexorious
- **Integration**: lgogdownloader CLI tool
- **Platform/Storefront**: PC (Windows) / GOG
- **Acceptance Criteria**:
  - GOG authentication flow works smoothly
  - Games properly matched with IGDB entries

#### 2.6 Xbox Sync
**Priority**: P1 (High) - **PLANNED**
- **User Story**: As a user, I want my Xbox library automatically synced to Nexorious
- **Integration**: xbox-webapi-python library
- **Platform/Storefront**: Xbox Series X/S, Xbox One / Microsoft Store
- **Acceptance Criteria**:
  - Xbox authentication flow works smoothly
  - Games properly matched with IGDB entries
  - Multi-platform support (Series X/S, One)

#### 2.7 Humble Bundle Sync
**Priority**: P2 (Medium) - **PLANNED**
- **User Story**: As a user, I want my Humble Bundle library automatically synced to Nexorious
- **Platform/Storefront**: PC (Windows) / Humble Bundle
- **Acceptance Criteria**:
  - Humble Bundle authentication works correctly
  - Games properly matched with IGDB entries

#### 2.8 IGDB Metadata Integration
**Priority**: P1 (High)
- **User Story**: As a user, I want game metadata to be automatically populated so I don't have to enter descriptions and cover art manually
- **Requirements**:
  - IGDB API integration for game metadata including game slug for proper URL generation
  - Automatic population of descriptions, release dates, genres, cover art
  - "How Long to Beat" completion time estimates integration
  - Fuzzy matching for game title lookups
  - Metadata refresh capabilities
  - Storage of both IGDB ID (for display) and IGDB slug (for functional links)
- **Acceptance Criteria**:
  - Game metadata is automatically populated when adding games
  - Cover art is downloaded and stored locally
  - Completion time estimates are displayed for planning purposes
  - Users can manually trigger metadata refresh
  - Fuzzy matching handles slight title variations
  - IGDB links are functional using game slug while displaying game ID in frontend
  - Game slug is retrieved and stored during metadata population

#### 2.9 IGDB Game Data Update System
**Priority**: P1 (High)
- **User Story**: As a user, I want games imported from IGDB to stay up-to-date with the latest information while maintaining my personal data, and as an admin, I want to control when and how this data is refreshed across the system
- **Backend Requirements**:
  - All games are sourced from IGDB and have IGDB IDs
  - Admin-only interface for triggering IGDB data updates on individual games
  - **Last Updated Tracking**: Automatically update games table `last_updated` field whenever IGDB metadata is refreshed
  - Batch update capabilities for multiple games with filtering options
  - Manual refresh functionality that preserves user-added personal data (notes, ratings, progress)
  - Clear separation between IGDB metadata and user personal data
  - API endpoints for bulk IGDB refresh operations (admin-only access)
- **Frontend Requirements**:
  - Admin-only "Update from IGDB" buttons and interfaces for games
  - Bulk update interface for admins to refresh multiple games simultaneously
  - **Last Updated Display**: Show when game data was last updated from IGDB in game detail views
  - Clear visual distinction between IGDB metadata (read-only) and personal data fields (editable)
  - Game editing forms that show IGDB metadata as read-only for all users
  - Preservation of editability for personal data fields (notes, ratings, progress tracking)
- **Data Protection Requirements**:
  - IGDB metadata updates never overwrite user personal data
  - User progress, ratings, notes, and custom tags remain fully editable
  - Platform and storefront ownership associations remain user-controlled
  - Clear separation between official IGDB data and user-generated content
- **Acceptance Criteria**:
  - All games have IGDB metadata that is read-only for regular users
  - Only administrators can update IGDB game metadata through refresh operations
  - Games table `last_updated` field is automatically updated whenever IGDB metadata is refreshed
  - Frontend displays when game data was last updated from IGDB
  - User personal data (notes, ratings, progress) remains fully editable for all users
  - Batch update operations work efficiently for large collections without data loss
  - IGDB data updates maintain referential integrity and do not break existing user associations

### Phase 3: Backlog Management

#### 3.1 Backlog View
**Priority**: P1 (High)
- **User Story**: As a user, I want to see all my unfinished games so I can decide what to focus on
- **Requirements**:
  - View showing games not completed/mastered/dominated and not shelved
  - Filtering by platform, genre, time-to-beat
  - Sorting options (recently added, shortest, oldest, etc.)
  - Quick actions to shelve or drop games
- **Acceptance Criteria**:
  - Backlog view accurately shows unfinished games
  - Filters and sorting work correctly
  - Quick actions update game status immediately

#### 3.2 Next Up / Choose Next Game
**Priority**: P1 (High)
- **User Story**: As a user, I want help deciding what game to play next based on my preferences
- **Requirements**:
  - Filter by genre, platform, estimated play time
  - Consider wishlist games (for purchase decisions)
  - Time-to-beat integration from IGDB
  - Randomizer option for the indecisive
  - "Start playing" action that changes status to In Progress
- **Acceptance Criteria**:
  - Filtering produces relevant game suggestions
  - Time-to-beat data is displayed and filterable
  - "Start playing" updates status correctly

#### 3.3 Collection Cleanup
**Priority**: P2 (Medium)
- **User Story**: As a user, I want to identify games I'll never play so I can clean up my backlog
- **Requirements**:
  - Surface games that have been "Not Started" for extended periods
  - Bulk "Drop" or "Shelve" operations
  - Statistics on backlog size and growth over time
- **Acceptance Criteria**:
  - Long-untouched games are identified
  - Bulk operations work correctly
  - Backlog statistics are accurate

### Phase 4: Discovery & Organization

#### 4.1 Advanced Search & Filtering
**Priority**: P1 (High) - **PARTIALLY IMPLEMENTED**
- **User Story**: As a user, I want advanced search and filtering capabilities so I can quickly find specific games in large collections using complex criteria
- **Requirements**:
  - Full-text search across all game metadata (basic search implemented)
  - Multi-criteria filtering system supporting platform, genre, status, rating, tags, release date (basic filtering implemented)
  - Advanced search with multiple parameters and boolean operators (future enhancement)
  - Saved filter presets for frequently used search patterns (future enhancement)
  - Advanced sorting options (basic sorting implemented)
  - Search result relevance scoring and intelligent ranking (future enhancement)
  - Real-time search suggestions and autocomplete functionality (future enhancement)
  - Complex boolean search queries with parentheses support (future enhancement)
  - Advanced bulk operations on filtered search results (bulk operations implemented)
  - Export search results to various formats (future enhancement)
- **Acceptance Criteria**:
  - Search returns relevant results quickly with sub-second response times
  - Filters can be combined for complex queries with multiple criteria
  - Advanced sort options work correctly across all metadata fields
  - Filter presets can be saved, shared, and reused across sessions
  - Boolean search operators work correctly for complex queries
  - Bulk operations can be performed on filtered results efficiently
  - Search autocomplete provides relevant suggestions as user types

#### 4.2 Wishlist Management
**Priority**: P2 (Medium) - **IMPLEMENTED**
- **User Story**: As a user, I want to maintain a wishlist so I can track games I want to purchase
- **Requirements**:
  - Simple add/remove games from wishlist functionality
  - Display wishlist with game information
  - Generate price comparison links on-the-fly for IsThereAnyDeal.com and PSPrices.com
  - Move games from wishlist to owned collection
- **Price Comparison Integration**:
  - **IsThereAnyDeal.com**: Generate search URLs using game titles for PC game price tracking
  - **PSPrices.com**: Generate search URLs for PlayStation game price tracking
  - Links are dynamically generated in the frontend using game title
  - No stored price data or tracking - purely external link generation
- **Acceptance Criteria**:
  - Wishlist is separate from owned collection
  - Users can easily add/remove games from wishlist
  - Price comparison links are automatically generated and functional
  - Games can be easily moved from wishlist to owned collection
  - External links open in new tabs/windows

#### 4.3 Statistics Dashboard
**Priority**: P2 (Medium) - **NOT IMPLEMENTED**
- **User Story**: As a user, I want to see statistics about my collection so I can understand my gaming habits
- **Requirements**:
  - Collection size by platform and genre
  - Completion rates and progress statistics
  - "Pile of Shame" count (owned games with 'not_started' status)
  - Most played games and genres
  - Monthly/yearly gaming activity
- **Acceptance Criteria**:
  - Dashboard loads quickly with visual charts
  - Statistics are accurate and update in real-time
  - Charts are responsive and mobile-friendly
  - "Pile of Shame" metric is prominently displayed with actionable insights

### Phase 5: User Experience & Interface

#### 5.1 Responsive Web Interface
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to access my collection from any device so I can manage it anywhere
- **Requirements**:
  - Responsive design that works on desktop, tablet, and mobile
  - Touch-friendly interface elements
- **Acceptance Criteria**:
  - Interface adapts to different screen sizes
  - Touch interactions work smoothly on mobile

#### 5.2 Keyboard Shortcuts
**Priority**: P2 (Medium)
- **User Story**: As a power user, I want keyboard shortcuts so I can navigate quickly
- **Requirements**:
  - Navigation shortcuts (search, add game, etc.)
  - Status change shortcuts
  - Bulk operation shortcuts
  - Customizable key bindings
- **Acceptance Criteria**:
  - Shortcuts work consistently across browsers
  - Help overlay shows available shortcuts
  - Shortcuts don't conflict with browser defaults

### Phase 6: Self-Hosting & Deployment

#### 6.1 Container Deployment
**Priority**: P0 (Critical) - **PENDING**
- **User Story**: As a system administrator, I want to deploy the service using containers so it's easy to manage
- **Requirements**:
  - Docker images for backend and frontend (pending)
  - Docker Compose configuration for single-machine deployment (pending)
  - Environment variable configuration (partially implemented)
  - Health check endpoints (implemented)
- **Acceptance Criteria**:
  - Containers start successfully with docker-compose up
  - Environment variables configure all necessary settings
  - Health checks report service status accurately
  - Logs are structured and useful for debugging

#### 6.2 Database Support
**Priority**: P0 (Critical) - **IMPLEMENTED**
- **User Story**: As a user, I want reliable database operations for my collection data
- **Requirements**:
  - PostgreSQL database
  - SQLModel ORM for type-safe database operations
  - Alembic for automatic database migrations
  - Automatic timestamp management via SQLModel for created_at and updated_at fields
- **Implementation Details**:
  - SQLModel handles automatic population of created_at timestamps on record creation
  - SQLModel handles automatic updates of updated_at timestamps on record modification
- **Acceptance Criteria**:
  - SQLModel provides consistent API
  - Alembic migrations run automatically on startup
  - Timestamp fields are automatically managed by the application layer

#### 6.2.1 Automatic Database Migrations
**Priority**: P0 (Critical)
- **User Story**: As a user deploying the application, I want database migrations to run automatically at startup so I don't need to manually execute migration commands
- **Requirements**:
  - Backend automatically runs pending Alembic migrations during application startup
  - Migration process is logged with clear status messages
  - Application startup fails gracefully if migrations fail
  - No manual intervention required for database schema updates
- **Implementation Details**:
  - Replace `create_db_and_tables()` with `run_alembic_migrations()` in startup lifespan
  - Use Alembic's programmatic API to execute migrations
  - Maintain backward compatibility with existing deployments
- **Acceptance Criteria**:
  - Fresh deployments automatically create database schema via migrations
  - Existing deployments automatically apply new migrations on startup
  - Migration failures prevent application startup with clear error messages
  - No manual migration commands required for normal deployment workflow

#### 6.3 Kubernetes Support
**Priority**: P0 (Critical)
- **User Story**: As a DevOps engineer, I want to deploy on Kubernetes so I can scale and manage the service in my cluster
- **Requirements**:
  - Kubernetes manifests for all components
  - Helm chart for easy deployment
  - Horizontal Pod Autoscaling configuration
  - Persistent volume support
- **Acceptance Criteria**:
  - Helm chart deploys successfully
  - Service scales based on load
  - Data persists across pod restarts
  - Configuration is externalized via ConfigMaps

#### 6.4 Testing & Quality Assurance
**Priority**: P0 (Critical)
- **User Story**: As a developer, I want comprehensive testing to ensure the application works correctly and reliably
- **Backend Testing Requirements**:
  - Unit tests for all business logic with >80% code coverage
  - Integration tests for all API endpoints
  - Database tests verifying SQLModel operations on PostgreSQL
  - Authentication and authorization tests
  - External API integration tests with mocked responses
  - Performance tests for critical operations
- **Frontend Testing Requirements**:
  - Unit tests for all components and stores
  - Integration tests for user workflows
  - End-to-end tests for critical user journeys
  - Visual regression tests for UI consistency
  - Accessibility tests (WCAG compliance)
  - Cross-browser compatibility tests
- **Test Automation Requirements**:
  - All tests must run automatically on every code commit
  - Pull requests cannot be merged without passing tests
  - Test execution in CI/CD pipeline before deployment
  - Automated test reports with coverage metrics
  - Database migration tests for PostgreSQL
- **Acceptance Criteria**:
  - Backend test coverage exceeds 80%
  - Frontend test coverage exceeds 70%
  - All tests pass on PostgreSQL
  - CI/CD pipeline blocks deployments if tests fail
  - All API endpoints have corresponding integration tests

#### 6.5 Backup System
**Priority**: P1 (High)
- **User Story**: As a user, I want my collection data backed up automatically
- **Requirements**:
  - Scheduled backups via NATS worker
  - Export collection to Nexorious-native format
  - User-triggered manual backups
  - Restore functionality from backup files
  - Backup retention policies
- **Acceptance Criteria**:
  - Backups run on schedule without user intervention
  - Manual backup triggers work correctly
  - Restore process recovers data accurately
  - Old backups are cleaned up per retention policy

### Phase 7: Future Enhancements

#### 7.1 Achievements & Trophies Tracking
**Priority**: P3 (Low)
- **User Story**: As a user, I want to see my achievement progress for games that support it
- **Requirements**:
  - Pull achievement data from supported storefronts (Steam initially)
  - Display completion percentage per game
  - Optional detailed achievement lists
  - Integration with "Dominated" play status (100% achievements)
- **Acceptance Criteria**:
  - Achievement data syncs from supported storefronts
  - Completion percentage displays on game cards
  - Achievement progress is tracked over time

#### 7.2 Notifications
**Priority**: P3 (Low)
- **User Story**: As a user, I want to be notified about important events in my collection
- **Requirements**:
  - Support multiple notification services (Telegram, Pushover, etc.)
  - Use notification aggregation library to minimize code
  - Configurable notification preferences per event type
  - Event types: re-authentication needed, new games synced, sync failures
- **Acceptance Criteria**:
  - Users can configure notification destinations
  - Notifications are sent for configured events
  - Users can enable/disable specific notification types

#### 7.3 Collection Anomaly Detection
**Priority**: P3 (Low)
- **User Story**: As a user, I want to identify potential data issues in my collection
- **Requirements**:
  - Flag games with platform/storefront combinations not in IGDB
  - Identify potential duplicates or mismatches
  - Suggest corrections based on IGDB data
  - User-facing view for reviewing and fixing anomalies in their own collection
- **Acceptance Criteria**:
  - Anomalies are automatically detected
  - Users can review flagged items
  - Corrections can be applied easily

### Phase 8: Performance Optimization

#### 8.1 IGDB Search Performance Optimization
**Priority**: P2 (Medium)
- **User Story**: As a user, I want faster search results when looking for games to add so I can quickly find and add games to my collection
- **Performance Issue**: Current IGDB search fetches time-to-beat data for every search result, requiring additional API calls that slow down search responses and consume IGDB API quota unnecessarily
- **Requirements**:
  - Remove time-to-beat data fetching from IGDB search result display
  - Defer time-to-beat data fetching until game is actually imported/added to database
  - Maintain full functionality - time-to-beat data still available in game details and import confirmation
  - Reduce IGDB API calls by 50-80% during search operations
  - Significantly improve search response times
  - Better utilize IGDB rate limits for core functionality
- **Technical Approach**:
  - Backend: Remove `_get_time_to_beat_data()` calls from `IGDBService.search_games()` method
  - Backend: Keep time-to-beat fetching in `get_game_by_id()` for actual game imports
  - Frontend: Remove time-to-beat display from search result cards
  - Frontend: Maintain time-to-beat display in import confirmation and game detail views
  - Update API schemas to reflect optional time-to-beat data in search results
- **Success Criteria**:
  - Search results load significantly faster (target: <2 seconds vs current 5-10 seconds for 10 results)
  - Reduced IGDB API rate limit consumption during search operations
  - No loss of user functionality - time-to-beat data accessible when needed
  - Time-to-beat data still displays correctly in game details and during import flow
- **Acceptance Criteria**:
  - IGDB search results display without time-to-beat information
  - Search response times improve measurably
  - Game import flow still retrieves and displays time-to-beat data
  - No breaking changes to existing API contracts
  - All existing functionality preserved in game detail views

## Additional Implemented Features

### Advanced Tagging System
- Color-coded tags with user customization
- Bulk tag operations across multiple games
- Tag-based filtering and organization

### Logo Management
- Official platform and storefront logos
- Admin logo upload and management system
- Legal attribution and compliance tracking

### Security Framework
- File upload security measures
- Memory-safe processing for large operations

## Technical Architecture

### Backend
- **FastAPI** (Python) with SQLModel ORM
- **PostgreSQL** database
- **JWT authentication** with refresh tokens
- **IGDB API** integration for all game metadata
- **NATS** for job queue and worker coordination

### Background Workers
- Storefront sync jobs (Steam, Epic, PSN, GOG, Xbox)
- IGDB metadata refresh and maintenance
- Import/export operations
- Scheduled backups
- Maintenance jobs (cleanup, orphaned data)

### Frontend
- **Next.js 16** with React 19 and TypeScript
- **Tailwind CSS** for styling
- **shadcn/ui** for accessible components
- **TanStack Query** for server state
- **TipTap** rich text editor for notes

### Deployment
- **Docker** containers with Docker Compose
- **Kubernetes** support with Helm charts (future)
- Comprehensive testing framework (implemented)

## Risk Assessment

### Technical Risks
- **API Rate Limits**: External service limits (Steam, IGDB)
- **Data Migration**: Schema changes in existing installations
- **API Changes**: External services may change without notice

### Product Risks
- **User Adoption**: Self-hosting requires technical knowledge
- **Maintenance**: Supporting multiple platform integrations
- **Competition**: Existing services (HowLongToBeat, Backloggd)

## Success Criteria

### Technical Success
- < 2 second page load times
- Zero data loss during migrations
- All automated tests pass on every deployment
- >80% backend code coverage, >70% frontend code coverage

### Deployment Success
- Single-command deployment with Docker Compose
- Clear documentation with step-by-step setup guides
- Automatic database migrations work reliably
- Troubleshooting guides for common issues
- All tests pass in CI/CD pipeline before deployment

### User Experience Success
- Initial admin setup completes successfully on first run
- Admin-created users can login with username successfully on first attempt
- Users can connect a storefront and sync games within 5 minutes
- Storefront sync successfully imports games on first try
- Core features are discoverable without documentation
- Interface works seamlessly on desktop and mobile
- Admin interface provides clear user management capabilities

## Appendices

### A. API Integration Details
- Steam Web API requirements and limitations
- IGDB API authentication and rate limits
- Epic Games Store integration via legendary
- PlayStation Network integration via PSNAWP
- GOG integration via lgogdownloader (planned)
- Xbox integration via xbox-webapi-python (planned)
- Humble Bundle integration (planned)

### B. Database Schema Design
- Core entity relationships
- Indexing strategy for performance
- Migration strategy for schema changes
- Backup and restore procedures

#### Database Schema

Complete database schema and models are implemented using SQLModel and can be found in the codebase at:
- `backend/app/models/` - SQLModel definitions
- `backend/alembic/versions/` - Database migration files

Key architectural decisions:
- UUID primary keys for security and distribution
- Support for PostgreSQL
- Multi-platform game ownership tracking
- User-defined tagging and rating systems
- IGDB integration with both ID (for display) and slug (for functional links) storage
- **IGDB Data Freshness Tracking**: Games table includes `last_updated` field to track when IGDB metadata was last refreshed

### C. Additional Documentation
- Deployment configurations in `/docs/deployment/`
- API integration details in `/docs/integrations/`
- Community guidelines in repository root