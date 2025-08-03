# Nexorious

A self-hostable web application for managing personal video game collections with comprehensive IGDB integration for tracking, organizing, and discovering games across multiple platforms and storefronts.

## Features

- **IGDB-Only Game Database**: All games sourced from the Internet Game Database (IGDB) with comprehensive metadata, cover art, ratings, and completion time estimates
- **Multi-Platform Game Tracking**: Support for Steam, Epic Games Store, PlayStation, Xbox, Nintendo, GOG, and physical media
- **Rich Game Discovery**: Search and import games from IGDB's extensive database with automatic metadata population
- **Progress Tracking**: Track play status, personal ratings, time played, and detailed notes for your IGDB-sourced games
- **Bulk Operations**: Import from CSV exports (Darkadia format) with intelligent conflict resolution
- **Admin Management**: User management, platform configuration, and system administration
- **Modern Tech Stack**: FastAPI backend with SvelteKit frontend

## Quick Start

### Prerequisites

- **Python 3.13+** with uv package manager
- **Node.js 18+** with npm
- **PostgreSQL 13+** (production) or SQLite (development)
- **Nix** (optional, recommended for reproducible development environment)

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
uv run python -m nexorious.main
# Backend available at: http://localhost:8000
```

#### Start Frontend (Terminal 2)
```bash
cd frontend
npm run dev
# Frontend available at: http://localhost:5173
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
- **Database Support**: Works with both PostgreSQL (production) and SQLite (development)

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
# Or for SQLite (development)
DATABASE_URL=sqlite:///./nexorious.db

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

### Docker Deployment (Coming Soon)

Docker support is planned for Phase 5. For now, deploy using standard Python/Node.js deployment methods.

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
CORS_ORIGINS=http://localhost:5173,http://localhost:4173

# Storage
STORAGE_PATH=/path/to/storage  # For cover art and uploads
```

#### Frontend Configuration

```bash
# API Connection
PUBLIC_API_URL=http://localhost:8000  # Backend URL
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
│   ├── nexorious/     # Main application package
│   ├── alembic/       # Database migrations
│   ├── scripts/       # Import scripts and utilities
│   └── tests/         # Backend test suite
├── frontend/          # SvelteKit TypeScript frontend
│   ├── src/           # Application source code
│   └── tests/         # Frontend test suite
├── docs/              # Project documentation
│   ├── PRD.md         # Product Requirements
│   └── TASK_BREAKDOWN.md  # Development tasks
└── storage/           # Runtime file storage
```

### Key Commands

```bash
# Backend development
cd backend
uv run python -m nexorious.main  # Start server
uv run pytest                    # Run tests
uv run alembic revision --autogenerate -m "description"  # New migration

# Frontend development  
cd frontend
npm run dev      # Start dev server
npm run check    # Type checking
npm run build    # Production build

# Both
npm run test     # Frontend tests
uv run pytest   # Backend tests
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

The logos and icons used in this application are sourced from SVG Repo and other public repositories under various open-source licenses (MIT, CC0, Logo License, etc.). See individual SOURCE.txt files in the `frontend/static/logos/` directory for specific license information and attribution.

## License

MIT License - see LICENSE file for details.

---

**Self-hosted game collection management made simple.**