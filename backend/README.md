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
uv run python -m nexorious.main
```

Or use uvicorn directly:
```bash
uv run uvicorn nexorious.main:app --reload
```

## API Documentation

Once the server is running, you can access:
- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc
- Health check: http://localhost:8000/health

## Testing

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
uv run pytest nexorious/tests/test_business_logic.py
```

### Coverage Analysis

Run tests with coverage analysis:
```bash
# Basic coverage with terminal output
uv run pytest --cov=nexorious

# Coverage with missing lines highlighted
uv run pytest --cov=nexorious --cov-report=term-missing

# Generate HTML coverage report
uv run pytest --cov=nexorious --cov-report=html

# Both terminal and HTML reports
uv run pytest --cov=nexorious --cov-report=term-missing --cov-report=html
```

### Focused Coverage

Run coverage for specific modules:
```bash
# Business logic coverage
uv run pytest nexorious/tests/test_business_logic.py --cov=nexorious.api.games --cov=nexorious.services.igdb --cov=nexorious.services.storage --cov-report=term-missing

# API endpoints coverage
uv run pytest nexorious/tests/test_auth_register.py --cov=nexorious.api.auth --cov-report=term-missing
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

The application supports both SQLite and PostgreSQL databases. Configure the `DATABASE_URL` in your `.env` file:

- SQLite: `sqlite:///./nexorious.db`
- PostgreSQL: `postgresql://username:password@localhost:5432/nexorious`

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
uv run python -m nexorious.seed_data.cli
```

Check for potential conflicts before seeding:
```bash
uv run python -m nexorious.seed_data.cli --check-conflicts
```

Force seeding (skip conflict prompts):
```bash
uv run python -m nexorious.seed_data.cli --force
```

Load seed data with version tracking:
```bash
uv run python -m nexorious.seed_data.cli --version "2.0.0"
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