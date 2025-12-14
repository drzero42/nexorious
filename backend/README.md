# Nexorious Backend

FastAPI backend for the Nexorious Game Collection Management Service.

## Setup

1. Create a virtual environment and install dependencies:
```bash
uv sync
```

2. Copy the environment file and configure it:
```bash
cp .env.example .env
```

3. Run database migrations:
```bash
uv run alembic upgrade head
```

4. Start the development server:
```bash
uv run python -m app.main
```

Or use uvicorn directly:
```bash
uv run uvicorn app.main:app --reload
```

## API Documentation

Once the server is running, you can access:
- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc
- Health check: http://localhost:8000/health

## Testing

Tests use **testcontainers** to spin up a real PostgreSQL database. This requires Docker or Podman to be running.

### Prerequisites

- Docker or Podman must be running
- For Podman users, ensure the socket is available at `/run/user/$UID/podman/podman.sock`

### Running Tests

Run the basic test suite:
```bash
uv run pytest
```

Run tests with verbose output:
```bash
uv run pytest -v
```

Run tests for a specific file:
```bash
uv run pytest app/tests/test_business_logic.py
```

### Coverage Analysis

Run tests with coverage analysis:
```bash
# Basic coverage with terminal output
uv run pytest --cov=app

# Coverage with missing lines highlighted
uv run pytest --cov=app --cov-report=term-missing

# Generate HTML coverage report
uv run pytest --cov=app --cov-report=html

# Both terminal and HTML reports
uv run pytest --cov=app --cov-report=term-missing --cov-report=html
```

### Focused Coverage

Run coverage for specific modules:
```bash
# Business logic coverage
uv run pytest app/tests/test_business_logic.py --cov=app.api.games --cov=app.services.igdb --cov=app.services.storage --cov-report=term-missing

# API endpoints coverage
uv run pytest app/tests/test_auth_register.py --cov=app.api.auth --cov-report=term-missing
```

### Coverage Reports

- **Terminal Report**: Shows coverage percentages and missing line numbers directly in the terminal
- **HTML Report**: Generates detailed HTML coverage reports in the `htmlcov/` directory
  - Open `htmlcov/index.html` in your browser for interactive coverage analysis
  - Shows line-by-line coverage with highlighting for covered/uncovered code

### Coverage Targets

The project aims for:
- **Business Logic**: >80% coverage for core business functions
- **API Endpoints**: >80% coverage for all REST endpoints
- **Models**: >90% coverage for data models and validation
- **Services**: >80% coverage for external service integrations

## Database

The application requires **PostgreSQL** (SQLite is no longer supported). Configure the `DATABASE_URL` in your `.env` file:

```bash
DATABASE_URL=postgresql://username:password@localhost:5432/nexorious
```

### PostgreSQL Setup

For local development, you can use Docker/Podman:

```bash
# Start PostgreSQL container
podman run -d \
  --name nexorious-db \
  -e POSTGRES_USER=nexorious \
  -e POSTGRES_PASSWORD=nexorious \
  -e POSTGRES_DB=nexorious \
  -p 5432:5432 \
  postgres:16-alpine
```

Or use the project's docker-compose setup which includes PostgreSQL.

### Breaking Change (v0.2.0)

**SQLite support has been removed.** If you were using SQLite, you must migrate to PostgreSQL:

1. Export your data from SQLite
2. Set up a PostgreSQL database
3. Import your data to PostgreSQL
4. Update your `DATABASE_URL` environment variable

## Configuration

### IGDB Rate Limiting

The application includes built-in rate limiting for IGDB API calls to respect their 4 requests per second limit. You can configure the rate limiting behavior via environment variables:

```bash
# IGDB rate limiting configuration
IGDB_REQUESTS_PER_SECOND=4.0      # Requests per second (default: 4.0)
IGDB_BURST_CAPACITY=8             # Maximum burst tokens (default: 8)
IGDB_BACKOFF_FACTOR=1.0          # Retry backoff factor in seconds (default: 1.0)
IGDB_MAX_RETRIES=3               # Maximum retry attempts (default: 3)
```

#### Rate Limiting Features

- **Token Bucket Algorithm**: Allows brief bursts while maintaining average rate
- **Automatic Retries**: Failed requests are retried with exponential backoff
- **Concurrent Safety**: Multiple simultaneous requests are properly queued
- **Monitoring**: Rate limiter status can be monitored via the IGDBService
- **Error Handling**: Rate limit violations are converted to user-friendly errors

#### Monitoring Rate Limits

You can monitor rate limiter status programmatically:

```python
# In your code
from app.services.igdb import IGDBService

async with IGDBService() as igdb:
    status = igdb.get_rate_limiter_status()
    print(f"Tokens available: {status['tokens_available']}")
    print(f"Utilization: {status['utilization']*100:.1f}%")
```

## Seed Data Management

The application includes official seed data for platforms and storefronts that provide the foundation for game collection management.

### What is Seed Data?

Seed data includes:
- **11 Official Platforms**: PC (Windows), PlayStation 5, PlayStation 4, PlayStation 3, Xbox Series X/S, Xbox One, Xbox 360, Nintendo Switch, Nintendo Wii, iOS, Android
- **12 Official Storefronts**: Steam, Epic Games Store, GOG, PlayStation Store, Microsoft Store, Nintendo eShop, Itch.io, Origin/EA App, Apple App Store, Google Play Store, Humble Bundle, Physical
- **Default Platform-Storefront Mappings**: Pre-configured associations (e.g., PC → Steam, PlayStation 5 → PlayStation Store)

### Automatic Loading

Seed data is **automatically loaded** when you create the initial admin user during first-time setup. No manual intervention is required for new installations.

### Manual Seed Data Management

For recovery, updates, or troubleshooting, you can manually manage seed data using the CLI tool:

#### Basic Commands

Load all official seed data:
```bash
uv run python -m app.seed_data.cli
```

Check for potential conflicts before seeding:
```bash
uv run python -m app.seed_data.cli --check-conflicts
```

Force seeding (skip conflict prompts):
```bash
uv run python -m app.seed_data.cli --force
```

Load seed data with version tracking:
```bash
uv run python -m app.seed_data.cli --version "2.0.0"
```

#### Conflict Resolution

The CLI tool intelligently handles conflicts with existing data:
- **Official entries**: Skipped (already exists)
- **Custom entries with same names**: Converted to official entries, preserving custom fields
- **Interactive prompts**: Asks for confirmation unless `--force` is used

#### When to Use Manual Seeding

- **Recovery**: After accidental deletion of platforms/storefronts
- **Updates**: When new official platforms/storefronts are added
- **Fresh installations**: If automatic loading failed during initial setup
- **Development**: When testing with clean database states

#### CLI Options Reference

| Option | Description |
|--------|-------------|
| `--help` | Show usage information |
| `--version VERSION` | Set version string for tracking (default: "1.0.0") |
| `--check-conflicts` | Check for conflicts without making changes |
| `--force` | Skip confirmation prompts and force seeding |

## Environment Variables

See `.env.example` for all available configuration options.