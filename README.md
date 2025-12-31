# Nexorious

A self-hostable web application for managing personal video game collections with comprehensive IGDB integration for tracking, organizing, and discovering games across multiple platforms and storefronts.

## Features

- **IGDB-Only Game Database**: All games sourced from the Internet Game Database (IGDB) with comprehensive metadata, cover art, ratings, and completion time estimates
- **Multi-Platform Game Tracking**: Support for Steam, Epic Games Store, PlayStation, Xbox, Nintendo, GOG, and physical media
- **Rich Game Discovery**: Search and import games from IGDB's extensive database with automatic metadata population
- **Progress Tracking**: Track play status, personal ratings, time played, and detailed notes for your IGDB-sourced games
- **Bulk Operations**: Import from CSV exports (Darkadia format) with intelligent conflict resolution
- **Admin Management**: User management, platform configuration, and system administration
- **Modern Tech Stack**: FastAPI backend with Next.js frontend

## Quick Start

### Prerequisites

- **Python 3.13+** with uv package manager
- **Node.js 18+** with npm
- **PostgreSQL 16+**
- **Nix** (optional, recommended for reproducible development environment)
- **IGDB API Credentials** (required) - See [IGDB Setup Guide](docs/igdb-setup.md)

### System Dependencies

- **PostgreSQL 16+** - Database server
- **legendary-gl** (for Epic Games Store sync)
  - Install: `pip install legendary-gl`
  - Required for Epic Games Store library sync
  - GPL3 licensed, used as external tool only

### Development Setup

#### Option 1: Using Nix (Recommended)

```bash
# Clone the repository
git clone https://github.com/your-username/nexorious.git
cd nexorious

# Enter development shell (includes all dependencies)
nix develop

# Setup backend
cd backend
uv sync
cd ..

# Setup frontend
cd frontend
npm install
cd ..
```

#### Option 2: Manual Setup

```bash
# Clone the repository
git clone https://github.com/your-username/nexorious.git
cd nexorious

# Setup backend
cd backend
uv sync  # Install Python dependencies
cd ..

# Setup frontend
cd frontend
npm install
cd ..
```

### Running in Development

#### Start Backend (Terminal 1)
```bash
cd backend
uv run python -m app.main
# Backend available at: http://localhost:8000
```

#### Start Frontend (Terminal 2)
```bash
cd frontend
npm run dev
# Frontend available at: http://localhost:3000
```

### Initial Setup

1. **First Run**: Navigate to the frontend URL - you'll be prompted to create an admin user
2. **Database**: Migrations run automatically on startup (see [Database Migrations](#database-migrations))
3. **API Documentation**: Available at http://localhost:8000/docs

## Production Deployment

### Database Migrations

**🚨 Important**: Nexorious now handles database migrations automatically during startup. You **do not** need to run manual migration commands.

#### How It Works

- **Automatic Execution**: When the application starts, it automatically runs `alembic upgrade head`
- **Error Handling**: If migrations fail, the application will not start and will log detailed error information
- **Logging**: Migration progress is logged at INFO level for monitoring
- **Database Support**: Works PostgreSQL

#### Migration Behavior

```bash
# ✅ Automatic - No manual intervention needed
# Migrations run during application startup

# ❌ Manual commands no longer required
# uv run alembic upgrade head  # NOT needed anymore
```

#### Environment Configuration

Ensure your environment variables are configured correctly:

```bash
# Required: Database connection
DATABASE_URL=postgresql://user:password@localhost:5432/nexorious

# Optional: Migration logging
LOG_LEVEL=INFO  # To see migration progress
```

#### Monitoring Migrations

Check application logs during startup to monitor migration progress:

```bash
# Look for these log messages:
# INFO - Starting database migrations...
# INFO - Database migrations completed successfully
```

#### Troubleshooting Migration Issues

If the application fails to start due to migration errors:

1. **Check Database Connection**:
   ```bash
   # Verify database is accessible
   psql $DATABASE_URL -c "SELECT 1;"
   ```

2. **Check Migration Logs**:
   ```bash
   # Look for detailed error messages in application logs
   # Error format: "Database migration failed: <error_details>"
   ```

3. **Manual Recovery** (if needed):
   ```bash
   cd backend
   # Check current migration status
   uv run alembic current
   
   # View migration history
   uv run alembic history
   
   # Manual upgrade (only if automatic failed)
   uv run alembic upgrade head
   ```

4. **Rollback Procedure**:
   ```bash
   cd backend
   # Rollback to previous migration
   uv run alembic downgrade -1
   
   # Or rollback to specific revision
   uv run alembic downgrade <revision_id>
   ```

### Docker/Podman Deployment

#### Podman Socket Setup (Required for Rootless Podman)

To use podman-compose or other tools that communicate via the Podman socket, you must enable the user-level Podman socket service:

```bash
# Enable and start the podman socket for your user
systemctl --user enable --now podman.socket

# Verify the socket is running
systemctl --user status podman.socket

# Check the socket path (typically /run/user/$(id -u)/podman/podman.sock)
podman info --format '{{.Host.RemoteSocket.Path}}'
```

**Note**: The `--user` flag is important - this runs the socket as your user, not as root. This is the recommended approach for rootless Podman.

If you need the socket to persist across reboots even when not logged in:

```bash
# Enable lingering for your user (allows user services to run without login)
loginctl enable-linger $USER
```

#### Running with Podman Compose

Once the socket is enabled, you can use podman-compose:

```bash
# Start all services
podman-compose up --build

# Stop services
podman-compose down

# Stop and reset database
podman-compose down -v

# Rebuild specific services
podman-compose build api
podman-compose build frontend
```

### Environment Variables

#### Backend Configuration

```bash
# Database
DATABASE_URL=postgresql://user:password@host:port/database

# Security
SECRET_KEY=your-secret-key-here
JWT_SECRET=your-jwt-secret-here

# IGDB Integration
IGDB_CLIENT_ID=your-igdb-client-id
IGDB_CLIENT_SECRET=your-igdb-client-secret

# Application
APP_NAME="Nexorious"
LOG_LEVEL=INFO
DEBUG=false

# CORS (for frontend integration)
CORS_ORIGINS=http://localhost:5173,http://localhost:3000,http://localhost:4173

# Storage
STORAGE_PATH=/path/to/storage  # For cover art and uploads
BACKUP_PATH=/path/to/backups   # For backup files (default: storage/backups)
```

#### Frontend Configuration

```bash
# API Connection
NEXT_PUBLIC_API_URL=http://localhost:8000/api
NEXT_PUBLIC_STATIC_URL=http://localhost:8000
```

### Production Checklist

- [ ] Database configured (PostgreSQL recommended)
- [ ] Environment variables set
- [ ] Storage directory writable
- [ ] IGDB API credentials configured
- [ ] CORS origins configured for frontend domain
- [ ] Secret keys generated (use secure random values)
- [ ] Backup procedures in place

## Data Import

### CSV Import (Darkadia Format)

Import your existing game collection from Darkadia CSV exports:

```bash
cd backend

# Interactive mode (recommended for first import)
uv run python scripts/import_darkadia_csv.py export.csv --username admin --password your_password

# Available strategies:
# --interactive  # Ask for conflict resolution (default)
# --overwrite    # CSV data takes precedence  
# --preserve     # Never overwrite existing data

# See scripts/README.md for complete documentation
```

**Features**:
- **Idempotent**: Safe to run multiple times
- **Conflict Resolution**: Multiple merge strategies
- **Progress Tracking**: Detailed reporting
- **Error Recovery**: Robust error handling

### Steam Library Import

Import your Steam game library directly into Nexorious with intelligent platform detection and automatic game matching.

#### Prerequisites

1. **Steam Web API Key**: Obtain from [Valve Developer Portal](https://steamcommunity.com/dev/apikey)
   - Register with your Steam account
   - Provide a domain name (can be localhost for personal use)
   - Copy the generated 32-character API key

2. **Steam ID**: Your 17-digit Steam ID (e.g., `76561197960435530`)
   - Find via [SteamID.io](https://steamid.io/) or similar services
   - Enter your Steam profile URL or custom URL

3. **Steam Profile Settings**: Ensure your game library is visible
   - Steam → Profile → Edit Profile → Privacy Settings
   - Game details: Set to "Public" or "Friends Only"

#### Setup Process

1. **Configure Steam Integration** (via API or future frontend):
   ```bash
   # Example API call to configure Steam settings
   POST /api/steam/config
   {
     "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
     "steam_id": "76561197960435530"
   }
   ```

2. **Verify Configuration**:
   ```bash
   # Test your Steam configuration
   POST /api/steam/verify
   {
     "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
     "steam_id": "76561197960435530"
   }
   ```

#### Import Your Library

```bash
# Import Steam library with default settings
POST /api/steam/import-library
{
  "fuzzy_threshold": 0.8,
  "merge_strategy": "skip",
  "platform_fallback": "pc-windows"
}
```

**Import Options**:
- `fuzzy_threshold` (0.0-1.0): Game name matching sensitivity (0.8 recommended)
- `merge_strategy`: 
  - `"skip"`: Skip games already in your collection
  - `"add_platforms"`: Add missing platforms to existing games
- `platform_fallback`: Default platform when Steam provides no platform data

#### What Gets Imported

✅ **Imported Data**:
- Game names and metadata (matched with IGDB database)
- Platform detection (Windows/Mac/Linux based on Steam playtime data)
- Steam storefront assignment (always "steam")
- Ownership status (set to "owned")

❌ **Not Imported**:
- Playtime hours (Steam data is unreliable)
- Play status (completion status)
- Personal ratings or notes
- Achievement data

#### Platform Detection

Nexorious intelligently detects which platforms you've used for each game:

- **Windows**: Games with `playtime_windows_forever > 0`
- **Mac**: Games with `playtime_mac_forever > 0` 
- **Linux**: Games with `playtime_linux_forever > 0`
- **Multi-Platform**: Games can be assigned to multiple platforms
- **Default Fallback**: Games with no platform data default to Windows

#### Import Results

Example import response:
```json
{
  "total_games": 150,
  "imported_count": 120,
  "skipped_count": 10,
  "failed_count": 5,
  "no_match_count": 15,
  "platform_breakdown": {
    "pc-windows": 140,
    "pc-linux": 25,
    "pc-mac": 8
  },
  "import_summary": "Imported 120 games, skipped 10, failed 5, no match 15 out of 150 Steam games"
}
```

**Result Categories**:
- **Imported**: Successfully added to your collection
- **Skipped**: Already in collection (with skip strategy)
- **Failed**: Import errors (network, database issues)
- **No Match**: Steam game name couldn't be matched with IGDB database

#### Troubleshooting

**Common Issues**:

1. **"No matching game found"**: 
   - Steam game names don't always match IGDB exactly
   - Lower `fuzzy_threshold` to 0.6-0.7 for more matches
   - Some Steam games aren't in IGDB database

2. **"Steam profile not found or is private"**:
   - Check Steam privacy settings
   - Ensure game library is set to Public or Friends Only
   - Verify correct Steam ID format

3. **"Invalid Steam Web API key"**:
   - Regenerate API key from Valve Developer Portal
   - Ensure 32-character alphanumeric format
   - Check for extra spaces or characters

4. **Import appears incomplete**:
   - Check `no_match_count` - these games couldn't be matched
   - Review `failed_count` - these had import errors
   - Re-run import safely (it's idempotent)

**Re-running Imports**:
- Safe to run multiple times
- Use `"add_platforms"` strategy to add missing platforms
- Previously imported games won't be duplicated

#### API Endpoints Reference

- **Configuration**: 
  - `GET /api/steam/config` - View current configuration
  - `PUT /api/steam/config` - Set Steam API key and Steam ID
  - `DELETE /api/steam/config` - Remove configuration

- **Verification**:
  - `POST /api/steam/verify` - Test Steam credentials without saving

- **Library Operations**:
  - `GET /api/steam/library` - View Steam library (raw data)
  - `POST /api/steam/import-library` - Import library into collection

- **Utilities**:
  - `POST /api/steam/resolve-vanity` - Convert custom URL to Steam ID

For complete API documentation, see [Swagger UI](http://localhost:8000/docs) after starting the backend.

## API Documentation

- **Swagger UI**: http://localhost:8000/docs
- **ReDoc**: http://localhost:8000/redoc
- **OpenAPI Spec**: http://localhost:8000/openapi.json

### IGDB-Only System

**Important**: This API exclusively supports games sourced from the Internet Game Database (IGDB). All games must have valid IGDB IDs and metadata. Manual game creation is not supported.

**Key API Features**:
- **Game Discovery**: Search IGDB database and import games with complete metadata
- **Metadata Management**: Refresh and update game information from IGDB
- **Authentication Required**: All game-related endpoints require authentication tokens
- **Rich Data**: All games include cover art, ratings, genre information, and completion time estimates

## Testing

### Backend Testing

```bash
cd backend

# Run all tests
uv run pytest

# With coverage (target >80%)
uv run pytest --cov=nexorious --cov-report=term-missing

# HTML coverage report
uv run pytest --cov=nexorious --cov-report=html

# Import script tests
uv run pytest scripts/tests/ -v
```

### Frontend Testing

```bash
cd frontend

# Run all tests  
npm run test

# With coverage (target >70%)
npm run test:coverage

# Type checking
npm run check
```

## Development

### Project Structure

```
nexorious/
├── backend/           # FastAPI Python backend
│   ├── app/           # Main application package
│   ├── alembic/       # Database migrations
│   ├── scripts/       # Import scripts and utilities
│   └── tests/         # Backend test suite
├── frontend/          # Next.js TypeScript frontend
│   ├── src/
│   │   ├── app/       # App Router pages and layouts
│   │   ├── api/       # API service layer
│   │   ├── components/# Reusable UI components
│   │   ├── hooks/     # TanStack Query hooks
│   │   ├── providers/ # React context providers
│   │   ├── types/     # TypeScript type definitions
│   │   └── lib/       # Utilities and configuration
│   └── public/        # Static assets
├── docs/              # Project documentation
│   ├── PRD.md         # Product Requirements
│   └── TASK_BREAKDOWN.md  # Development tasks
└── storage/           # Runtime file storage
```

### Key Commands

```bash
# Backend development
cd backend
uv run python -m app.main       # Start server
uv run pytest                    # Run tests
uv run alembic revision --autogenerate -m "description"  # New migration

# Frontend development
cd frontend
npm run dev      # Start dev server (port 3000)
npm run build    # Production build
npm run check    # Type checking (tsc && eslint)
npm run test     # Run tests

# Tests
cd frontend && npm run test      # Frontend tests
cd backend && uv run pytest      # Backend tests
```

### Development Guidelines

- **Database Changes**: Always create migrations for schema changes
- **Testing**: Maintain >80% backend and >70% frontend coverage
- **Code Style**: Use existing patterns and conventions
- **Automatic Migrations**: Database migrations run automatically on startup

## Contributing

1. **Read Documentation**: Review `docs/PRD.md` and `docs/TASK_BREAKDOWN.md`
2. **Create Branch**: Use descriptive branch names (`task-X.Y.Z-description`)
3. **Follow Standards**: Maintain test coverage and code quality
4. **Update Documentation**: Keep README and task breakdown current

## Support

- **Documentation**: See `docs/` directory for detailed specifications
- **Import Help**: See `backend/scripts/README.md` for CSV import guide
- **Testing Guide**: See `backend/scripts/tests/README.md` for testing details
- **Issues**: Report bugs and feature requests via GitHub issues

## Trademarks and Copyright

All mentioned trademarks, brand names, and logos for gaming platforms and storefronts (including but not limited to PlayStation, Xbox, Nintendo, Steam, Epic Games Store, GOG, Apple App Store, Google Play Store, and others) are the property of their respective owners. These trademarks are used solely for identification and compatibility purposes.

The use of these trademarks and brand names does not imply any affiliation, endorsement, or partnership with the respective companies. All rights to these trademarks remain with their original owners.

The logos and icons used in this application are sourced from SVG Repo and other public repositories under various open-source licenses (MIT, CC0, Logo License, etc.).

## License

MIT License - see LICENSE file for details.

---

**Self-hosted game collection management made simple.**