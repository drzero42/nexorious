# Game Collection Management Service - Product Requirements Document

## Executive Summary

The Game Collection Management Service is a self-hostable web application designed to help users organize, track, and manage their personal video game collections across multiple platforms and storefronts. The service provides comprehensive collection management, progress tracking, and integration with major gaming platforms.

## Product Vision

To create the definitive self-hosted solution for personal game collection management that seamlessly integrates with existing gaming platforms while providing powerful organization, tracking, and discovery features.

## Target Users

- **Primary**: Gaming enthusiasts with large collections across multiple platforms
- **Secondary**: Casual gamers who want to organize their digital libraries
- **Tertiary**: Game collectors with diverse acquisition sources

## Core Value Propositions

1. **Unified Collection View**: Consolidate games from all platforms in one place
2. **Progress Tracking**: Monitor gaming progress and completion status
3. **Self-Hosted Privacy**: Complete control over personal gaming data
4. **Storefront Integration**: Automatic import from major gaming storefronts
5. **Smart Organization**: Intelligent filtering, tagging, and recommendation systems

## Success Metrics

- **Easy to deploy and manage in self-hosted setups**: Users can successfully deploy with minimal configuration
- **Easy to use**: Intuitive interface that requires no learning curve for basic collection management
- **Secure by default**: Application follows security best practices without requiring additional configuration

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
- **User Story**: As a user, I want to add games to my collection so I can track what I own across all platforms and storefronts in a unified view
- **Backend Requirements**:
  - RESTful endpoints for CRUD operations on games
  - Game metadata storage with comprehensive fields
  - Multi-platform and multi-storefront association (multiple storefronts per platform supported)
  - Ownership tracking through storefront associations
  - Duplicate detection and prevention at the game level (not platform level)
  - IGDB integration for game lookup and metadata retrieval
- **Frontend Requirements**:
  - Game creation and editing forms with platform/storefront management
  - Game library list and grid views showing unified game cards
  - Each game appears once with all owned platforms/storefronts displayed as indicators/badges
  - Platform and storefront indicators clearly showing all ownership locations
  - Search and filter interface operating on unique games (not game-platform combinations)
  - Bulk selection and operations on unique games
  - IGDB game search interface with candidate selection
  - Game metadata acceptance/confirmation screen
  - Game editing interface for adding/removing platform and storefront ownership
- **Game Addition Flow**:
  1. User searches for game by title using IGDB integration
  2. System presents game candidates with ownership status indicators
  3. User selects game and configures platforms/storefronts with automatic defaults
  4. System adds to collection or updates existing entry to prevent duplicates
- **Acceptance Criteria**:
  - Games appear once in collection with platform/storefront indicators
  - IGDB integration provides accurate metadata and search
  - Duplicate detection prevents redundant entries
  - Bulk operations work on unified game view

#### 1.2.5 Game Editing & Platform Management
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to edit my games and manage which platforms and storefronts I own them on so I can keep my collection accurate
- **Backend Requirements**:
  - RESTful endpoints for updating game metadata and platform/storefront associations
  - Platform/storefront addition and removal for existing games
  - Validation to prevent removal of all platforms (orphaned games)
- **Frontend Requirements**:
  - Game editing form with metadata modification capabilities
  - Platform and storefront management interface within game editing
  - Add/remove platform associations with visual feedback
  - Add/remove storefront associations per platform
  - Confirmation dialogs for removing platform/storefront ownership
  - Bulk editing capabilities for multiple games
  - Visual indicators showing current ownership status during editing
- **Game Editing Flow**:
  1. User selects a game from their collection
  2. System displays game editing interface with current metadata and ownership
  3. User can modify game metadata (title, notes, ratings, etc.)
  4. User can add new platforms/storefronts to their ownership
  5. User can remove existing platforms/storefronts from their ownership
  6. System validates changes and prevents invalid states (e.g., no platforms)
  7. Changes are saved and reflected immediately in the collection view
  8. Game continues to appear once in collection with updated platform/storefront indicators
- **Acceptance Criteria**:
  - Platform/storefront ownership can be added/removed with validation
  - Bulk editing supported with immediate UI updates

#### 1.3 Platform & Storefront Tracking (Admin-Only Management)
**Priority**: P0 (Critical)
- **User Story**: As an administrator, I want to manage the available platforms and storefronts in the system so that users can accurately track their game ownership, while as a user, I want to associate my games with existing platforms and storefronts so I know where to find them
- **Backend Requirements**:
  - Platform and storefront data models
  - API endpoints for managing platform associations with support for multiple storefronts per platform
  - Availability status tracking
  - Platform-specific metadata storage
  - **ADMIN-ONLY ACCESS**: All platform/storefront management operations (create, update, delete) require admin privileges
  - **SECURITY NOTE**: Platform and storefront management is restricted to admins to maintain data consistency and prevent unauthorized system configuration changes
  - Default storefront assignment for platforms (admin-only configuration)
  - API endpoints for managing platform default storefront relationships (admin-only)
  - Idempotent seed data function for platform and storefront population (admin-triggered)
  - Function automatically runs during initial admin account creation
  - Function can be manually triggered by admin users at any time
  - Function only adds missing default platforms/storefronts, never interferes with custom ones
- **Frontend Requirements**:
  - Platform selection interface (for users to associate games with existing platforms)
  - Multi-select storefront interface per platform (for users to associate games with existing storefronts)
  - Storefront linking components
  - Availability status indicators
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

### Phase 2: Data Integration & Import

#### 2.1 CSV Import
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to import my existing game collection from CSV so I don't have to manually enter everything
- **Requirements**:
  - Support Darkadia CSV export format
  - Generic CSV import with field mapping
  - Validation and error handling for import data
  - Progress tracking during import
- **Acceptance Criteria**:
  - CSV files are parsed correctly with proper error handling
  - Users can map CSV columns to database fields
  - Import progress is shown to user
  - Failed imports provide clear error messages

#### 2.2 Steam API Integration
**Priority**: P1 (High)
- **User Story**: As a user, I want to automatically import my Steam library so I don't have to manually add each game
- **Requirements**:
  - Steam Web API integration for library import
  - Automatic playtime import where available
  - Achievement data integration
  - Periodic sync to catch new purchases
- **Acceptance Criteria**:
  - Steam library import works with API key
  - Playtime data is accurately imported
  - Users can trigger manual sync
  - Import respects Steam privacy settings

#### 2.3 IGDB Metadata Integration
**Priority**: P1 (High)
- **User Story**: As a user, I want game metadata to be automatically populated so I don't have to enter descriptions and cover art manually
- **Requirements**:
  - IGDB API integration for game metadata
  - Automatic population of descriptions, release dates, genres, cover art
  - "How Long to Beat" completion time estimates integration
  - Fuzzy matching for game title lookups
  - Metadata refresh capabilities
- **Acceptance Criteria**:
  - Game metadata is automatically populated when adding games
  - Cover art is downloaded and stored locally
  - Completion time estimates are displayed for planning purposes
  - Users can manually trigger metadata refresh
  - Fuzzy matching handles slight title variations

### Phase 3: Discovery & Organization

#### 3.1 Search & Filtering
**Priority**: P1 (High)
- **User Story**: As a user, I want to search and filter my collection so I can find specific games quickly
- **Requirements**:
  - Full-text search across game titles and metadata
  - Advanced filtering by platform, status, rating, genre
  - Sorting by various criteria (alphabetical, rating, playtime, etc.)
  - Saved filter presets
- **Acceptance Criteria**:
  - Search returns relevant results quickly
  - Filters can be combined for complex queries
  - Sort options work correctly
  - Filter presets can be saved and reused

#### 3.2 Wishlist Management
**Priority**: P2 (Medium)
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

#### 3.3 Statistics Dashboard
**Priority**: P2 (Medium)
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

### Phase 4: User Experience & Interface

#### 4.1 Responsive Web Interface
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to access my collection from any device so I can manage it anywhere
- **Requirements**:
  - Responsive design that works on desktop, tablet, and mobile
  - Touch-friendly interface elements
- **Acceptance Criteria**:
  - Interface adapts to different screen sizes
  - Touch interactions work smoothly on mobile

#### 4.2 Keyboard Shortcuts
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

### Phase 5: Self-Hosting & Deployment

#### 5.1 Container Deployment
**Priority**: P0 (Critical)
- **User Story**: As a system administrator, I want to deploy the service using containers so it's easy to manage
- **Requirements**:
  - Docker images for backend and frontend
  - Docker Compose configuration for single-machine deployment
  - Environment variable configuration
  - Health check endpoints
- **Acceptance Criteria**:
  - Containers start successfully with docker-compose up
  - Environment variables configure all necessary settings
  - Health checks report service status accurately
  - Logs are structured and useful for debugging

#### 5.2 Database Support
**Priority**: P0 (Critical)
- **User Story**: As a user, I want flexible database options so I can choose what works best for my setup
- **Requirements**:
  - PostgreSQL support for production deployments
  - SQLite support for single-instance, small deployments
  - SQLModel ORM for type-safe database operations
  - Alembic for automatic database migrations
  - Automatic timestamp management via SQLModel for created_at and updated_at fields
  - Backup and restore capabilities
- **Implementation Details**:
  - SQLModel will handle automatic population of created_at timestamps on record creation
  - SQLModel will handle automatic updates of updated_at timestamps on record modification
  - Database-agnostic schema design avoiding PostgreSQL-specific features
- **Acceptance Criteria**:
  - Both database types work without configuration changes
  - SQLModel provides consistent API across database types
  - Alembic migrations run automatically on startup
  - Timestamp fields are automatically managed by the application layer
  - Backup tools preserve all user data
  - Restore process is reliable and documented

#### 5.3 Kubernetes Support
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

#### 5.4 Testing & Quality Assurance
**Priority**: P0 (Critical)
- **User Story**: As a developer, I want comprehensive testing to ensure the application works correctly and reliably
- **Backend Testing Requirements**:
  - Unit tests for all business logic with >80% code coverage
  - Integration tests for all API endpoints
  - Database tests verifying SQLModel operations on both PostgreSQL and SQLite
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
  - Database migration tests for both PostgreSQL and SQLite
- **Acceptance Criteria**:
  - Backend test coverage exceeds 80%
  - Frontend test coverage exceeds 70%
  - All tests pass on both PostgreSQL and SQLite databases
  - CI/CD pipeline blocks deployments if tests fail
  - All API endpoints have corresponding integration tests

### Phase 6: Advanced Features

#### 6.1 Enhanced Storefront Integration
**Priority**: P2 (Medium)
- **User Story**: As a user, I want integration with more storefronts so I can import all my games automatically
- **Requirements**:
  - Epic Games Store integration
  - GOG integration
  - PlayStation Store integration
  - Xbox Marketplace integration
- **Acceptance Criteria**:
  - Each storefront integration imports library correctly
  - Authentication flows work smoothly
  - Sync can be triggered manually or automatically
  - Error handling provides clear user feedback

## Technical Architecture

### Backend
- **FastAPI** (Python) with SQLModel ORM
- **PostgreSQL** (production) / **SQLite** (development)
- **JWT authentication** with refresh tokens
- **IGDB API** integration for game metadata

### Frontend
- **SvelteKit** with TypeScript
- **Tailwind CSS** for styling
- **Svelte stores** for state management

### Deployment
- **Docker** containers with Docker Compose
- **Kubernetes** support with Helm charts
- Automated testing and CI/CD pipelines

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
- Users can add their first game within 2 minutes
- CSV import works on first try for standard formats
- Core features are discoverable without documentation
- Interface works seamlessly on desktop and mobile
- Admin interface provides clear user management capabilities

## Appendices

### A. API Integration Details
- Steam Web API requirements and limitations
- IGDB API authentication and rate limits
- Epic Games Store integration possibilities
- PlayStation Store and Xbox Marketplace API availability

### B. Database Schema Design
- Core entity relationships
- Indexing strategy for performance
- Migration strategy for schema changes
- Backup and restore procedures

#### Database Schema

Complete database schema and models are implemented using SQLModel and can be found in the codebase at:
- `backend/nexorious/models/` - SQLModel definitions
- `backend/alembic/versions/` - Database migration files

Key architectural decisions:
- UUID primary keys for security and distribution
- Support for both PostgreSQL (production) and SQLite (development)
- Multi-platform game ownership tracking
- User-defined tagging and rating systems

### C. Additional Documentation
- Deployment configurations in `/docs/deployment/`
- API integration details in `/docs/integrations/`
- Community guidelines in repository root