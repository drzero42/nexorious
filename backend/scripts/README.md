# Nexorious Import Scripts

This directory contains import scripts and utilities for migrating data into Nexorious from external sources.

## Darkadia CSV Import

The main import functionality for Darkadia CSV exports.

### Quick Start

```bash
# From the backend directory
cd /path/to/nexorious/backend

# Run import (example)
uv run python scripts/import_darkadia_csv.py path/to/export.csv --user-id USER_UUID --username admin --password your_password

# Interactive mode (asks for conflict resolution)
uv run python scripts/import_darkadia_csv.py export.csv --user-id USER_UUID --interactive --auth-token YOUR_JWT_TOKEN

# Overwrite mode (CSV data takes precedence)
uv run python scripts/import_darkadia_csv.py export.csv --user-id USER_UUID --overwrite --username admin --password your_password

# Preserve mode (never overwrite existing data)
uv run python scripts/import_darkadia_csv.py export.csv --user-id USER_UUID --preserve --auth-token YOUR_JWT_TOKEN

# Dry run (preview changes without making them)
uv run python scripts/import_darkadia_csv.py export.csv --user-id USER_UUID --dry-run --username admin --password your_password
```

### Features

- **Three Merge Strategies**:
  - **Interactive**: Ask user for conflict resolution (default)
  - **Overwrite**: CSV data always takes precedence
  - **Preserve**: Never overwrite existing data, only add new platforms

- **Idempotent Operations**: Safe to run multiple times - will not create duplicates

- **Decision Caching**: Interactive decisions are saved to `~/.nexorious/import_decisions.json` and reused

- **Platform Management**: Handles multiple platforms and storefronts per game

- **Robust Error Handling**: Comprehensive error reporting and retry logic

### Import Process

1. **Parse CSV**: Extract and validate game data from Darkadia export
2. **Group Duplicates**: Identify and merge duplicate entries within CSV
3. **Connect to API**: Authenticate with Nexorious backend
4. **Process Games**: Apply chosen merge strategy for each game
5. **Generate Report**: Provide detailed summary of import results

### Authentication

The script supports two authentication methods:

```bash
# Using username/password
--username admin --password your_password

# Using JWT token
--auth-token your_jwt_token
```

### Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `--user-id` | User ID for import (required) | - |
| `--api-base` | Backend API base URL | `http://localhost:8000` |
| `--interactive` | Interactive merge strategy | `true` |
| `--overwrite` | Overwrite merge strategy | `false` |
| `--preserve` | Preserve merge strategy | `false` |
| `--dry-run` | Preview changes without making them | `false` |
| `--batch-size` | Process N games at a time | `10` |
| `--verbose` | Enable verbose logging | `false` |

## Testing

Comprehensive test suite with >90% coverage.

```bash
# Run all import tests
uv run pytest scripts/tests/ -v

# Run with coverage report
uv run pytest scripts/tests/ --cov=scripts --cov-report=term-missing

# Test specific functionality
uv run pytest scripts/tests/test_idempotency.py -v
```

📖 **Complete Testing Guide**: See [`scripts/tests/README.md`](tests/README.md) for detailed testing documentation.

## Directory Structure

```
scripts/
├── README.md                     # This file
├── import_darkadia_csv.py       # Main import script
├── darkadia/                    # Darkadia import modules
│   ├── __init__.py
│   ├── api_client.py           # Nexorious API client
│   ├── mapper.py               # Data format conversion
│   ├── merge_strategies.py     # Conflict resolution strategies
│   └── parser.py               # CSV parsing and validation
└── tests/                      # Comprehensive test suite
    ├── README.md              # Testing documentation
    ├── test_api_client.py     # API client tests
    ├── test_darkadia_parser.py # CSV parser tests
    ├── test_data_mapping.py   # Data mapping tests
    ├── test_idempotency.py    # Idempotency tests
    ├── test_merge_strategies.py # Merge strategy tests
    └── sample_darkadia.csv    # Test data
```

## Architecture

The import system is designed for reliability and maintainability:

### Core Components

- **Parser** (`darkadia/parser.py`): CSV parsing and validation
- **Mapper** (`darkadia/mapper.py`): Data format transformation  
- **API Client** (`darkadia/api_client.py`): Backend communication
- **Merge Strategies** (`darkadia/merge_strategies.py`): Conflict resolution

### Design Principles

- **Idempotency**: All operations can be safely repeated
- **Error Recovery**: Robust handling of network and data errors
- **User Control**: Multiple merge strategies for different use cases
- **Transparency**: Detailed logging and reporting
- **Testability**: Comprehensive test coverage with isolated components

## Troubleshooting

### Common Issues

#### Authentication Errors
```bash
# Error: Invalid credentials
# Solution: Check username/password or JWT token
uv run python scripts/import_darkadia_csv.py --username admin --password correct_password

# Error: Token expired
# Solution: Re-authenticate or use username/password
```

#### Import Errors
```bash
# Error: Game not found
# Check: Game title spelling and IGDB availability

# Error: Platform not found  
# Check: Platform names match seed data (use --verbose for details)
```

#### Performance Issues
```bash
# For large imports, consider smaller batch sizes
--batch-size 5

# Use dry-run to test without API calls
--dry-run
```

### Getting Help

1. **Verbose Mode**: Add `--verbose` for detailed logging
2. **Dry Run**: Use `--dry-run` to preview without changes
3. **Test Data**: Use `scripts/tests/sample_darkadia.csv` for testing
4. **Logs**: Check console output for error details

## Development

### Adding New Import Sources

To add support for a new data source:

1. Create module in `scripts/new_source/`
2. Implement parser following `darkadia/parser.py` pattern
3. Create data mapper to Nexorious format
4. Add comprehensive tests
5. Update this documentation

### Contributing

1. **Code Style**: Follow existing patterns and naming conventions
2. **Testing**: Maintain >90% test coverage for new code
3. **Documentation**: Update README files for new features
4. **Idempotency**: Ensure all operations can be safely repeated

---

For detailed information about testing, see [`scripts/tests/README.md`](tests/README.md).

For project-wide documentation, see [`../CLAUDE.md`](../CLAUDE.md).