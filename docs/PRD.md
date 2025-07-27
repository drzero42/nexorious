# Game Collection Management Service - Product Requirements Document

## Executive Summary

The Game Collection Management Service is a self-hostable web application designed to help users organize, track, and manage their personal video game collections across multiple platforms and storefronts. The service provides comprehensive collection management, progress tracking, and integration with major gaming platforms.

## Product Vision

To create the definitive self-hosted solution for personal game collection management that seamlessly integrates with existing gaming platforms while providing powerful organization, tracking, and discovery features.

## Target Users

- **Primary**: Gaming enthusiasts with large collections across multiple platforms
- **Secondary**: Casual gamers who want to organize their digital libraries
- **Tertiary**: Game collectors who mix physical and digital collections

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
- **User Story**: As a developer, I want a REST API so the frontend can interact with the backend and custom integrations can be built
- **Requirements**:
  - RESTful API with OpenAPI documentation
  - Authentication and authorization
  - CORS configuration for frontend access
- **Acceptance Criteria**:
  - API documentation is comprehensive and accurate
  - Authentication works with JWT tokens
  - Frontend can consume all necessary endpoints

#### 1.1 Initial Setup & User Authentication
**Priority**: P0 (Critical)
- **User Story**: As an administrator, I want to create the initial admin user on first startup and manage all subsequent user accounts

##### 1.1.1 First-Run Admin Setup
- **Requirements**:
  - On first startup when no users exist, display admin user creation screen
  - Require username and password for initial admin account
  - Automatically grant admin privileges to first user
  - Skip this screen if any users already exist in database
  - Automatically seed platform and storefront data on first startup
  - Ensure seed data is only loaded once per fresh installation
- **Acceptance Criteria**:
  - Application detects empty user table and shows setup screen
  - Admin user is created with is_admin=true flag
  - Setup screen is never shown again after first user creation
  - Platform and storefront seed data is automatically loaded on first startup
  - Seed data includes all major gaming platforms and storefronts
  - Subsequent startups do not duplicate or overwrite existing platform/storefront data

##### 1.1.2 User Authentication
- **Authentication Model**:
  - **Username**: Primary user identifier for login and display purposes
  - **Password**: Secure authentication credential
  - **Database ID**: Internal UUID primary key for database relationships
- **Requirements**:
  - Username-based login system only
  - No user self-registration capability
  - Password hashing with secure algorithm (bcrypt/scrypt)
  - JWT token-based session management
  - Session refresh token mechanism
- **Acceptance Criteria**:
  - Users can login with username and password
  - No registration option available to non-authenticated users
  - Username uniqueness is enforced across the system
  - Database maintains id as primary key for relationships

#### 1.2 Game Library Management
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to add games to my collection so I can track what I own
- **Backend Requirements**:
  - RESTful endpoints for CRUD operations on games
  - Game metadata storage with comprehensive fields
  - Multi-platform and multi-storefront association
  - Physical vs digital ownership tracking
  - Duplicate detection and prevention
  - IGDB integration for game lookup and metadata retrieval
- **Frontend Requirements**:
  - Game creation and editing forms
  - Game library list and grid views
  - Platform and storefront indicators
  - Search and filter interface
  - Bulk selection and operations
  - IGDB game search interface with candidate selection
  - Game metadata acceptance/confirmation screen
- **Game Addition Flow**:
  1. User searches for a game by title
  2. System queries IGDB API for matching games
  3. If multiple games found, present user with list of candidates showing:
     - Game title and release year
     - Cover art thumbnail
     - Platform information
     - Brief description
  4. User selects the correct game from the candidates
  5. System retrieves full metadata from IGDB for chosen game
  6. Present acceptance screen showing all retrieved information:
     - Complete game details (title, description, genre, developer, etc.)
     - Cover art
     - Release information
     - How Long to Beat estimates
     - Platforms available
  7. User confirms or edits information before final submission
  8. Game is added to database and user's collection
- **Acceptance Criteria**:
  - API endpoints handle all game management operations
  - Frontend forms validate input and provide feedback
  - Games display with all relevant metadata
  - Duplicate detection prevents redundant entries
  - Bulk operations work efficiently
  - IGDB search returns relevant game candidates
  - User can distinguish between similar games in candidate list
  - Metadata acceptance screen shows complete, accurate information
  - Users can modify auto-populated data before saving

#### 1.3 Platform & Storefront Tracking
**Priority**: P0 (Critical)
- **User Story**: As a user, I want to track which platforms I own games on so I know where to find them
- **Backend Requirements**:
  - Platform and storefront data models
  - API endpoints for managing platform associations
  - Availability status tracking
  - Platform-specific metadata storage
  - Admin-only access for platform/storefront management (create, update, delete)
  - Seed data management for initial platform and storefront population
  - Database migrations to populate seed data on fresh installations
- **Frontend Requirements**:
  - Platform selection interface
  - Storefront linking components
  - Availability status indicators
  - Platform filtering and sorting
  - Admin interface for platform/storefront management
- **Seed Data Content**:
  - **Platforms**: PC (Windows), PlayStation 5, PlayStation 4, PlayStation 3, Xbox Series X/S, Xbox One, Xbox 360, Nintendo Switch, Nintendo Wii, iOS, Android
  - **Storefronts**: Steam, Epic Games Store, GOG, PlayStation Store, Microsoft Store, Nintendo eShop, Itch.io, Origin/EA App, Apple App Store, Google Play Store, Humble Bundle
- **Acceptance Criteria**:
  - API supports multiple platforms per game
  - Frontend allows easy platform assignment
  - Storefront links are preserved and accessible
  - Ownership status is clearly indicated in UI
  - Only admin users can add, update, or remove platforms and storefronts
  - Regular users can only associate existing platforms/storefronts with their games
  - All seed data platforms and storefronts are available immediately after first startup
  - Seed data is populated automatically without admin intervention

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
  - API handles all progress tracking operations
  - Frontend provides intuitive status updates with clear completion level definitions
  - Time tracking accepts manual input
  - Notes support rich text formatting
  - Progress changes are reflected immediately
  - Completion levels provide meaningful progression tracking

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
- **Acceptance Criteria**:
  - Only admins can create new user accounts
  - Usernames must be unique across the system
  - Passwords are securely hashed before storage
  - Admin can reset any user's password
  - User deletion properly handles related data

##### 1.6.3 System Configuration
- **Requirements**:
  - Platform and storefront management (already defined in 1.3)
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

### Backend Stack
- **Framework**: FastAPI (Python)
- **Database**: PostgreSQL (production) / SQLite (single-instance, small deployments)
- **ORM**: SQLModel for database models and queries
- **Migrations**: Alembic for database schema migrations
- **Authentication**: JWT tokens with refresh mechanism
- **API Documentation**: OpenAPI/Swagger
- **Background Tasks**: Celery with Redis
- **File Storage**: Local filesystem with S3 compatibility
- **Testing**: Pytest for unit and integration tests

### Frontend Stack
- **Framework**: Svelte/SvelteKit
- **State Management**: Svelte stores
- **Styling**: Tailwind CSS
- **Build Tool**: Vite
- **Testing**: Vitest for unit tests, Playwright for E2E tests

### Infrastructure
- **Containerization**: Docker with multi-stage builds
- **Orchestration**: Docker Compose (local) / Kubernetes (production)
- **Monitoring**: Prometheus metrics, structured logging
- **Security**: Input validation, secure defaults
- **Backup**: Automated database backups with retention policies
- **CI/CD**: Automated testing pipeline on all code changes

## Risk Assessment

### Technical Risks
- **API Rate Limits**: Steam, IGDB, and other services may impose strict rate limits
  - *Mitigation*: Basic request throttling for MVP, with optional caching and advanced rate limiting in later phases
- **Data Migration**: Database schema changes could break existing installations
  - *Mitigation*: Comprehensive migration testing, rollback procedures
- **Storefront API Changes**: External APIs may change without notice
  - *Mitigation*: Abstraction layers, monitoring, and graceful error handling

### Product Risks
- **User Adoption**: Self-hosted software requires technical knowledge
  - *Mitigation*: Comprehensive documentation, Docker Compose simplification
- **Competition**: Existing services like HowLongToBeat, Backloggd
  - *Mitigation*: Focus on self-hosting, privacy, and comprehensive storefront support
- **Maintenance Burden**: Supporting multiple storefronts and integrations
  - *Mitigation*: Modular architecture, community contributions, automated testing

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

#### SQL Schema (Database Agnostic)

```sql
-- User Management
CREATE TABLE users (
    id VARCHAR(36) PRIMARY KEY,                    -- Internal UUID primary key for database relationships
    username VARCHAR(100) UNIQUE NOT NULL,        -- Primary user identifier for login and display
    password_hash VARCHAR(255) NOT NULL,          -- Secure password hash for authentication
    is_active BOOLEAN DEFAULT true,
    is_admin BOOLEAN DEFAULT false,
    preferences TEXT DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_sessions (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    refresh_token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_agent TEXT,
    ip_address VARCHAR(45)
);

-- Platform and Storefront Management
CREATE TABLE platforms (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    icon_url VARCHAR(500),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE storefronts (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    icon_url VARCHAR(500),
    base_url VARCHAR(500),
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Game Metadata
CREATE TABLE games (
    id VARCHAR(36) PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    genre VARCHAR(200),
    developer VARCHAR(200),
    publisher VARCHAR(200),
    release_date DATE,
    cover_art_url VARCHAR(500),
    rating_average DECIMAL(3,2),
    rating_count INTEGER DEFAULT 0,
    metadata TEXT DEFAULT '{}',
    estimated_playtime_hours INTEGER,
    howlongtobeat_main INTEGER,
    howlongtobeat_extra INTEGER,
    howlongtobeat_completionist INTEGER,
    igdb_id VARCHAR(50),
    is_verified BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE game_aliases (
    id VARCHAR(36) PRIMARY KEY,
    game_id VARCHAR(36) NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    alias_title VARCHAR(500) NOT NULL,
    source VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- User Game Collections
CREATE TABLE user_games (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id VARCHAR(36) NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    ownership_status VARCHAR(50) DEFAULT 'owned' CHECK (ownership_status IN ('owned', 'borrowed', 'rented', 'subscription')),
    is_physical BOOLEAN DEFAULT false,
    physical_location VARCHAR(200),
    personal_rating DECIMAL(2,1) CHECK (personal_rating >= 1 AND personal_rating <= 5),
    is_loved BOOLEAN DEFAULT false,
    play_status VARCHAR(50) DEFAULT 'not_started' CHECK (play_status IN ('not_started', 'in_progress', 'completed', 'mastered', 'dominated', 'shelved', 'dropped', 'replay')),
    hours_played INTEGER DEFAULT 0,
    personal_notes TEXT,
    acquired_date DATE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, game_id)
);

CREATE TABLE user_game_platforms (
    id VARCHAR(36) PRIMARY KEY,
    user_game_id VARCHAR(36) NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    platform_id VARCHAR(36) NOT NULL REFERENCES platforms(id) ON DELETE CASCADE,
    storefront_id VARCHAR(36) REFERENCES storefronts(id) ON DELETE SET NULL,
    store_game_id VARCHAR(200),
    store_url VARCHAR(500),
    is_available BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_game_id, platform_id)
);

-- Tagging System
CREATE TABLE tags (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    color VARCHAR(7) DEFAULT '#6B7280',
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);

CREATE TABLE user_game_tags (
    id VARCHAR(36) PRIMARY KEY,
    user_game_id VARCHAR(36) NOT NULL REFERENCES user_games(id) ON DELETE CASCADE,
    tag_id VARCHAR(36) NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_game_id, tag_id)
);

-- Wishlist Management
CREATE TABLE wishlists (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id VARCHAR(36) NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, game_id)
);

-- Import/Export Tracking
CREATE TABLE import_jobs (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    import_type VARCHAR(50) NOT NULL CHECK (import_type IN ('csv', 'steam', 'epic', 'gog', 'xbox', 'playstation')),
    status VARCHAR(50) DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    total_records INTEGER DEFAULT 0,
    processed_records INTEGER DEFAULT 0,
    failed_records INTEGER DEFAULT 0,
    error_log TEXT DEFAULT '[]',
    metadata TEXT DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

-- Indexes for Performance
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_token_hash ON user_sessions(token_hash);
CREATE INDEX idx_games_title ON games(title);
CREATE INDEX idx_games_igdb_id ON games(igdb_id);
CREATE INDEX idx_game_aliases_game_id ON game_aliases(game_id);
CREATE INDEX idx_game_aliases_title ON game_aliases(alias_title);
CREATE INDEX idx_user_games_user_id ON user_games(user_id);
CREATE INDEX idx_user_games_game_id ON user_games(game_id);
CREATE INDEX idx_user_games_play_status ON user_games(play_status);
CREATE INDEX idx_user_games_personal_rating ON user_games(personal_rating);
CREATE INDEX idx_user_games_is_loved ON user_games(is_loved);
CREATE INDEX idx_user_game_platforms_user_game_id ON user_game_platforms(user_game_id);
CREATE INDEX idx_user_game_platforms_platform_id ON user_game_platforms(platform_id);
CREATE INDEX idx_tags_user_id ON tags(user_id);
CREATE INDEX idx_user_game_tags_user_game_id ON user_game_tags(user_game_id);
CREATE INDEX idx_user_game_tags_tag_id ON user_game_tags(tag_id);
CREATE INDEX idx_wishlists_user_id ON wishlists(user_id);
CREATE INDEX idx_import_jobs_user_id ON import_jobs(user_id);
CREATE INDEX idx_import_jobs_status ON import_jobs(status);
```

#### Key Schema Features

- **UUID Primary Keys**: All tables use VARCHAR(36) to store UUIDs for better distribution and security (generated by application)
- **Comprehensive User Management**: User accounts, sessions, and preferences with clear identifier roles:
  - **id**: Primary key for database relationships and internal references
  - **username**: Primary user identifier for login authentication and display
  - **is_admin**: Boolean flag for administrative privileges
- **Flexible Game Metadata**: Support for multiple data sources with JSON text fields for extensibility
- **Multi-Platform Support**: Games can exist on multiple platforms
- **Progress Tracking**: Detailed play status with completion levels (Completed, Mastered, Dominated) and time logging
- **Tagging System**: User-defined tags with color coding for organization
- **Wishlist Management**: Simple wishlist with dynamic price comparison links
- **Import/Export Jobs**: Tracking for batch operations and data migrations
- **Performance Indexes**: Strategic indexing for common query patterns including username-based lookups
- **Data Integrity**: Foreign key constraints and check constraints for data validation
- **Timestamp Management**: Created and updated timestamps handled by SQLModel in the application layer

#### Database Compatibility Notes

- **Data Types**: Uses standard SQL data types compatible with both SQLite and PostgreSQL
- **UUIDs**: Stored as VARCHAR(36) and generated by the application layer
- **JSON Fields**: Stored as TEXT with JSON serialization handled by SQLModel
- **Timestamps**: SQLModel will automatically manage created_at and updated_at fields
- **Full-Text Search**: Will be implemented at the application layer using SQLModel queries
- **No Database-Specific Features**: Avoids triggers, stored procedures, or PostgreSQL-specific functions

### C. Deployment Configurations
- Docker Compose examples
- Kubernetes manifests
- Environment variable reference
- Reverse proxy configurations

### D. Community Guidelines
- Contribution guidelines
- Code of conduct
- Issue reporting templates
- Feature request process

### E. Operational Procedures
- Admin user recovery procedures
- Database-level password reset instructions
- User account management best practices
- Security incident response procedures

#### Database Password Reset Procedure

When an administrator needs to reset a user's password directly in the database:

1. **Generate a new password hash** using the same algorithm as the application (bcrypt/scrypt)
2. **Connect to the database** using appropriate credentials
3. **Execute the update query**:
   ```sql
   UPDATE users 
   SET password_hash = 'new_hash_value', 
       updated_at = CURRENT_TIMESTAMP 
   WHERE username = 'target_username';
   ```
4. **Verify the update** was successful
5. **Communicate the temporary password** to the user through a secure channel
6. **Require password change** on next login (if implemented)

**Security Notes**:
- Never store plaintext passwords
- Use the application's password hashing function when possible
- Document all manual password resets for audit purposes
- Consider implementing an in-app admin password reset feature to avoid direct database access