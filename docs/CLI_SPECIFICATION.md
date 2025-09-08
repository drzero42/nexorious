# Nexorious CLI Client - Technical Specification

## Overview

This document specifies the technical requirements for implementing a command-line interface (CLI) client for the Nexorious Game Collection Management Service. The CLI client must provide full functionality equivalent to the web UI while following Unix/Linux standards for configuration and integration.

## Architecture Requirements

### Project Structure

The CLI must be implemented as a **completely separate project** from the backend and frontend:

```
nexorious/                      # Project root
├── backend/                    # FastAPI backend
├── frontend/                   # SvelteKit frontend  
├── cli/                        # ✅ Separate CLI project
│   ├── nexorious_cli/          # Python package
│   ├── pyproject.toml
│   └── tests/
└── docs/
```

- **Package name**: `nexorious-cli` (for pip install)
- **Command name**: `nexcmd` (what users type)
- **Communication**: HTTP API only (no direct imports between CLI and backend)

### High-Level Design

The CLI client shall be implemented as a Python application that communicates with the existing FastAPI backend through REST API calls. The client must maintain local configuration for authentication tokens and server settings following XDG Base Directory Specification.

```
┌─────────────────┐    HTTP/REST    ┌──────────────────┐
│   CLI Client    │ ◄────────────► │ FastAPI Backend  │
│                 │                 │                  │
│ - Command Parser│                 │ - Authentication │
│ - HTTP Client   │                 │ - Game API       │
│ - Token Manager │                 │ - Import API     │
│ - Config Manager│                 │ - Admin API      │
└─────────────────┘                 └──────────────────┘
```

### Core Components

The CLI client shall implement these core components:

1. **Command Parser** - Handles CLI argument parsing and command routing
2. **HTTP Client** - Manages API communication with automatic token refresh
3. **Authentication Manager** - Handles JWT token storage and refresh flow
4. **Configuration Manager** - Manages local configuration following XDG standards
5. **Output Formatter** - Provides multiple output formats (table, JSON, CSV)

## Authentication Requirements

### Initial Setup Flow

The CLI must support the same first-run admin setup as the web UI:

1. Check setup status via `/api/auth/setup/status`
2. Create initial admin user via `/api/auth/setup/admin` if needed
3. Store authentication session locally

### Authentication Flow

The CLI shall implement JWT-based authentication matching the web UI:

1. **Login** - Exchange credentials for access and refresh tokens via `/api/auth/login`
2. **Token Storage** - Store tokens securely in XDG_DATA_HOME with 600 permissions
3. **Auto-refresh** - Automatically refresh expired tokens via `/api/auth/refresh`
4. **Logout** - Invalidate tokens via `/api/auth/logout`

### Security Requirements

- Authentication tokens must be stored separately from user preferences
- Token files must have 600 permissions (owner read/write only)
- Authentication directories must have 700 permissions (owner access only)
- Token expiration must be validated before API requests
- Secure local authentication session must be supported

## Configuration System Requirements

### XDG Base Directory Compliance

The CLI must follow XDG Base Directory Specification:

```
$XDG_CONFIG_HOME/nexorious/          # User preferences
└── config.toml                      # Main configuration

$XDG_DATA_HOME/nexorious/           # Application data
└── auth/                           # Authentication sessions (mode 700)
    └── session.json                # (mode 600)

$XDG_STATE_HOME/nexorious/          # Logs and history
├── logs/
└── history/
```

### Configuration Requirements

1. **File Formats**
   - TOML for human-readable configuration files
   - JSON for structured authentication data

2. **Configuration Precedence** (highest to lowest)
   - Command-line arguments
   - Environment variables (`NEXORIOUS_*`)
   - Main configuration file
   - Built-in defaults

3. **Environment Variable Support**
   - All major settings configurable via environment variables
   - Boolean, integer, and string type conversion
   - Standard `NEXORIOUS_*` prefix

## Command Structure Requirements

### Global Options

All commands shall support these global options:

```bash
--server URL        # Server URL (env: NEXORIOUS_SERVER)
--config DIR        # Config directory path (env: NEXORIOUS_CONFIG_DIR)
--format FORMAT     # Output format: table, json, csv (env: NEXORIOUS_FORMAT)
--verbose, -v       # Verbose output (env: NEXORIOUS_VERBOSE)
--quiet, -q         # Quiet output (env: NEXORIOUS_QUIET)
--no-color          # Disable colored output
--help, -h          # Show help
--version           # Show version
```

### Command Groups

The CLI shall implement these primary command groups:

#### 1. Authentication Commands (`auth`)
- `login [USERNAME]` - Login to server
- `logout` - Logout from server
- `whoami` - Show current user info  
- `setup` - Initial admin setup
- `config show/set/get` - Configuration management

#### 2. Games Commands (`games`) - Global Game Database Operations
- `search QUERY` - Search IGDB for games
- `list [OPTIONS]` - List games in global database
- `show GAME_ID` - Show game details from global database
- `import IGDB_ID` - Import game from IGDB to global database
- `aliases list/create/delete GAME_ID` - Game alias management
- `metadata refresh/populate/status/bulk` - Metadata operations (admin)
- `cover-art download/bulk-download` - Cover art operations (admin)

#### 3. User Games Commands (`usergames`) - Personal Collection Management
- `add GAME_ID` - Add game from global database to personal collection
- `list [OPTIONS]` - List personal game collection
- `show USER_GAME_ID` - Show collection item details
- `update USER_GAME_ID` - Update collection item (status, rating, notes)
- `progress USER_GAME_ID` - Update play progress
- `remove USER_GAME_ID` - Remove from collection
- `platforms list/add/update/remove USER_GAME_ID` - Platform management for collection items
- `bulk-update/bulk-delete/bulk-add-platforms/bulk-remove-platforms` - Bulk collection operations
- `stats` - Personal collection statistics

#### 4. Tags Commands (`tags`)
- `list/create/show/update/delete` - Tag CRUD operations
- `assign/remove USER_GAME_ID TAG_IDS` - Tag assignment
- `bulk-assign/bulk-remove` - Bulk tag operations
- `stats` - Tag usage statistics
- `create-or-get NAME` - Create or get existing tag

#### 5. Wishlist Commands (`wishlist`)
- `list [OPTIONS]` - List wishlist
- `add/remove GAME_ID` - Wishlist management
- `clear` - Clear wishlist

#### 6. Import Commands (`import`)
- `steam/csv/darkadia [OPTIONS]` - Import from sources
- `status/history/sources` - Import tracking
- `jobs list/show/cancel` - Job management
- `batch start/status/next/cancel` - Batch operations
- `resolve-suggestions/pending-resolutions` - Platform resolution
- `resolve/bulk-resolve` - Conflict resolution
- `compatibility-check` - Platform compatibility

#### 7. Admin Commands (`admin`)
- `users list/create/show/update/delete/reset-password` - User management
- `platforms list/create/show/update/delete` - Platform management
- `storefronts list/create/show/update/delete` - Storefront management
- `platforms associate/dissociate` - Platform associations
- `platforms/storefronts upload-logo/delete-logo/list-logos` - Logo management
- `seed-data` - Load seed data
- `stats platforms/storefronts` - Usage statistics

## API Integration Requirements

### HTTP Client Requirements

The HTTP client must:
- Support async operations using `httpx`
- Handle automatic token refresh on 401 responses
- Implement proper timeout handling (configurable, default 30s)
- Support file uploads for CSV imports and logo management
- Handle pagination automatically where supported
- Implement retry logic for transient failures

### Endpoint Coverage

The CLI must support all existing backend API endpoints with clear command-to-API mapping:

#### Command to API Endpoint Mapping
- `auth` commands → `/api/auth/*` (authentication, setup, admin user management)
- `games` commands → `/api/games/*` (IGDB search, global database, metadata operations)
- `usergames` commands → `/api/user-games/*` (personal collection management)
- `tags` commands → `/api/tags/*` (tag management and assignment)
- `wishlist` commands → `/api/wishlist/*` (wishlist management)
- `import` commands → `/api/import/*` (import operations and platform resolution)
- `admin` commands → Multiple API prefixes:
  - User management: `/api/auth/admin/*`
  - Platform/storefront management: `/api/platforms/*` (with admin permissions)
  - Game metadata operations: `/api/games/*` (with admin permissions)

### Error Handling

- HTTP 401: Attempt token refresh, retry once, then prompt for login
- HTTP 403: Show access denied message
- HTTP 404: Show not found message
- HTTP 409: Show conflict/duplicate message
- HTTP 422: Show validation errors clearly
- Network errors: Show connection issues with retry suggestions
- JSON decode errors: Show invalid response message

## Output Requirements

### Format Support

The CLI must support multiple output formats:

1. **Table Format** (default)
   - Human-readable tabular output using tabulate or rich
   - Color support (respects `--no-color` and `NO_COLOR` env var)
   - Pagination support for large datasets
   - Column width optimization

2. **JSON Format**
   - Machine-readable structured output
   - Pretty-printed with proper indentation
   - Complete data preservation

3. **CSV Format**  
   - Comma-separated values for data analysis
   - Proper escaping of special characters
   - Header row included

### Pagination

Commands returning large datasets must support:
- `--page` and `--per-page` options
- Display of current page and total pages
- Navigation hints for next/previous pages

## Security Requirements

### Authentication Security
- Secure token storage with proper file permissions
- Automatic token validation and refresh
- Secure local session management
- Secure credential prompting (password masking)

### Input Validation
- All user inputs must be validated before API calls
- File path validation for uploads
- ID validation for entity references
- Prevention of command injection

### Output Security
- No sensitive information in verbose logs
- Error messages must not expose internal details
- Credential masking in configuration display

## Performance Requirements

### Response Time
- Local operations (config, help): < 100ms
- Authentication operations: < 2s
- Standard API operations: < 5s
- Bulk operations: Progress indication required

### Resource Usage
- Memory usage should remain reasonable for large collections
- Network requests should be optimized (avoid unnecessary calls)

## Compatibility Requirements

### Platform Support
- Linux (primary target)
- macOS (with XDG directory adaptation)
- Windows (with appropriate directory mapping)

### Python Requirements
- Python 3.9+ support
- Standard library usage preferred where possible
- Minimal external dependencies

### API Compatibility
- Must work with current backend API version
- Forward compatibility for minor API changes
- Graceful degradation for missing features

## Testing Requirements

### Test Coverage
- Unit tests for core functionality
- Integration tests with backend API
- Configuration system tests
- Authentication flow tests
- Command parsing tests

### Test Environment
- Mock backend responses for unit tests
- Test database for integration tests
- Automated CI/CD pipeline integration

## Documentation Requirements

### User Documentation
- Command reference with examples
- Configuration guide
- Installation instructions
- Troubleshooting guide

### Developer Documentation
- API integration guide
- Extension points for new commands
- Configuration system documentation
- Testing procedures

## Installation and Distribution

### Project Structure
- CLI must be a separate project in `cli/` directory (not inside backend)
- Completely independent codebase from backend and frontend
- HTTP-only communication with backend (no direct imports)

### Package Requirements
- Package name: `nexorious-cli` (installable via pip)
- Command name: `nexcmd` (what users actually type)
- Entry point script registration via pyproject.toml
- Independent dependency management (separate from backend dependencies)
- Version management

### Distribution
- PyPI package distribution as `nexorious-cli`
- GitHub releases with binaries
- Package manager integration (future)
- Completely separate deployment from backend

This specification defines the requirements for a professional-grade CLI client that provides complete feature parity with the web UI while following Unix/Linux standards and best practices for command-line applications.