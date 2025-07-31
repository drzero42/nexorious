# Darkadia CSV Import Testing Guide

This directory contains comprehensive tests for the Darkadia CSV import system, ensuring reliable and idempotent data migration from Darkadia exports to Nexorious.

## Quick Start

### Prerequisites

1. Ensure you're in the backend directory:
   ```bash
   cd /path/to/nexorious/backend
   ```

2. Install development dependencies:
   ```bash
   uv sync --group dev
   ```

### Run All Tests

```bash
# Run all import script tests
uv run pytest scripts/tests/ -v

# Run all tests with coverage
uv run pytest scripts/tests/ --cov=scripts --cov-report=term-missing

# Generate HTML coverage report
uv run pytest scripts/tests/ --cov=scripts --cov-report=html
```

## Test Structure

### Test Files Overview

| Test File | Purpose | Test Count* |
|-----------|---------|-------------|
| `test_api_client.py` | Tests API client functionality, authentication, and network handling | 20+ |
| `test_darkadia_parser.py` | Tests CSV parsing, validation, and data extraction | 15+ |
| `test_data_mapping.py` | Tests data transformation from Darkadia to Nexorious format | 12+ |
| `test_merge_strategies.py` | Tests all three merge strategies and base functionality | 25+ |
| `test_idempotency.py` | Tests idempotent behavior and end-to-end workflows | 10+ |

*Test counts are approximate and may vary as tests are added/updated.

### Test Categories

#### Unit Tests
Test individual components in isolation:
- **API Client**: Authentication, HTTP requests, error handling
- **CSV Parser**: Data extraction, validation, duplicate detection
- **Data Mapping**: Format conversion, field transformation
- **Merge Strategies**: Conflict resolution, decision caching

#### Integration Tests
Test component interactions:
- **API Integration**: Real API workflows with mocked responses
- **End-to-End Workflows**: Complete import processes
- **Database Operations**: Platform/storefront management

#### Idempotency Tests
Test that operations can be safely repeated:
- **Multiple Import Runs**: Same CSV imported twice produces identical results
- **Platform Deduplication**: No duplicate platform associations
- **Decision Persistence**: Interactive decisions cached and reused

## Running Specific Tests

### By Test File

```bash
# Test API client only
uv run pytest scripts/tests/test_api_client.py -v

# Test merge strategies only
uv run pytest scripts/tests/test_merge_strategies.py -v

# Test idempotency only
uv run pytest scripts/tests/test_idempotency.py -v
```

### By Test Class

```bash
# Test specific merger class
uv run pytest scripts/tests/test_merge_strategies.py::TestInteractiveMerger -v

# Test API client authentication
uv run pytest scripts/tests/test_api_client.py::TestNexoriousAPIClient::test_authenticate_success -v

# Test idempotency scenarios
uv run pytest scripts/tests/test_idempotency.py::TestImportIdempotency -v
```

### By Test Method

```bash
# Test specific functionality
uv run pytest scripts/tests/test_merge_strategies.py::TestMergeStrategyBaseFunctionality::test_find_existing_game_exact_match -v

# Test decision caching
uv run pytest scripts/tests/test_merge_strategies.py::TestInteractiveMerger::test_decision_cache_loading -v
```

### By Pattern

```bash
# Run all tests containing "idempotent" in name
uv run pytest scripts/tests/ -k "idempotent" -v

# Run all tests for interactive merger
uv run pytest scripts/tests/ -k "interactive" -v

# Run all platform-related tests
uv run pytest scripts/tests/ -k "platform" -v
```

## Test Configuration Options

### Verbosity Levels

```bash
# Minimal output
uv run pytest scripts/tests/

# Verbose output (recommended)
uv run pytest scripts/tests/ -v

# Very verbose output
uv run pytest scripts/tests/ -vv

# Show local variables in tracebacks
uv run pytest scripts/tests/ -l
```

### Failure Handling

```bash
# Stop on first failure
uv run pytest scripts/tests/ -x

# Stop after N failures
uv run pytest scripts/tests/ --maxfail=3

# Show shorter tracebacks
uv run pytest scripts/tests/ --tb=short

# Show only failing line
uv run pytest scripts/tests/ --tb=line
```

### Parallel Execution

```bash
# Run tests in parallel (if pytest-xdist is installed)
uv run pytest scripts/tests/ -n auto

# Run with specific number of workers
uv run pytest scripts/tests/ -n 4
```

## Coverage Reports

### Terminal Coverage

```bash
# Basic coverage report
uv run pytest scripts/tests/ --cov=scripts

# Coverage with missing lines
uv run pytest scripts/tests/ --cov=scripts --cov-report=term-missing

# Coverage for specific module
uv run pytest scripts/tests/ --cov=scripts.darkadia.merge_strategies --cov-report=term-missing
```

### HTML Coverage Report

```bash
# Generate HTML report
uv run pytest scripts/tests/ --cov=scripts --cov-report=html

# Open coverage report (the report will be in htmlcov/index.html)
# You can open it in your browser:
# file:///path/to/nexorious/backend/htmlcov/index.html
```

### Coverage Targets

The import system maintains high test coverage standards:
- **Overall Coverage**: >90%
- **Critical Modules**: >95%
  - `merge_strategies.py`
  - `api_client.py`
  - `parser.py`

## Test Data

### Sample CSV File

The test suite includes `sample_darkadia.csv` with representative data:
- Multiple games with different statuses
- Various platforms and storefronts
- Different rating and completion states
- Edge cases (missing data, special characters)

### Mock Data

Tests use comprehensive mock data including:
- **API Responses**: Realistic API response structures
- **Game Libraries**: Large collections for performance testing
- **Conflict Scenarios**: Various data conflicts for merge testing
- **Error Conditions**: Network failures, authentication errors

## Individual Test Module Guide

### test_api_client.py

Tests the Nexorious API client that communicates with the backend.

```bash
# Run all API client tests
uv run pytest scripts/tests/test_api_client.py -v

# Key test categories:
# - Authentication (login, token management)
# - Game operations (search, create, update)
# - Platform management (add, validate)
# - Error handling (network errors, API errors)
# - Retry logic (exponential backoff)
```

**Key Features Tested:**
- JWT authentication and token refresh
- Game search with fuzzy matching
- Platform/storefront validation
- HTTP error handling and retries
- Request caching for platforms/storefronts

### test_darkadia_parser.py

Tests CSV parsing and data extraction from Darkadia exports.

```bash
# Run parser tests
uv run pytest scripts/tests/test_darkadia_parser.py -v

# Key test categories:
# - CSV parsing (field extraction, validation)
# - Data cleaning (whitespace, empty values)
# - Duplicate detection (within CSV)
# - Error handling (malformed CSV, missing columns)
```

**Key Features Tested:**
- CSV field mapping and validation
- Data type conversion (dates, ratings, booleans)
- Duplicate game detection within CSV
- Handling of malformed or incomplete data

### test_data_mapping.py

Tests transformation from Darkadia format to Nexorious format.

```bash
# Run data mapping tests
uv run pytest scripts/tests/test_data_mapping.py -v

# Key test categories:
# - Field transformation (rating scales, date formats)
# - Status mapping (play status conversion)
# - Platform mapping (name normalization)
# - Default value handling
```

**Key Features Tested:**
- Rating conversion (text to numeric)
- Play status standardization
- Platform/storefront name mapping
- Date parsing and formatting
- Boolean field conversion

### test_merge_strategies.py

Tests all three merge strategies and their conflict resolution logic.

```bash
# Run merge strategy tests
uv run pytest scripts/tests/test_merge_strategies.py -v

# Test specific merger:
uv run pytest scripts/tests/test_merge_strategies.py::TestInteractiveMerger -v
uv run pytest scripts/tests/test_merge_strategies.py::TestOverwriteMerger -v
uv run pytest scripts/tests/test_merge_strategies.py::TestPreserveMerger -v
```

**Key Features Tested:**

#### Base Strategy Functionality:
- Enhanced game deduplication with tiered fuzzy matching
- Platform duplicate prevention
- Error recording and reporting

#### Interactive Merger:
- Decision caching to `~/.nexorious/import_decisions.json`
- Conflict signature generation for cache keys
- Batch decision application
- Intelligent data merging

#### Overwrite Merger:
- Always preferring CSV data
- Platform deduplication on updates
- Consistent behavior across runs

#### Preserve Merger:
- Never overwriting existing data
- Adding only new platforms
- Skipping when no new data

### test_idempotency.py

Tests that the import system behaves identically across multiple runs.

```bash
# Run idempotency tests
uv run pytest scripts/tests/test_idempotency.py -v

# Test specific scenarios:
uv run pytest scripts/tests/test_idempotency.py::TestImportIdempotency -v
uv run pytest scripts/tests/test_idempotency.py::TestScalabilityIdempotency -v
```

**Key Scenarios Tested:**
- **Complete Idempotency**: Same CSV imported twice produces identical results
- **Platform Deduplication**: No duplicate platform associations on re-runs
- **Decision Persistence**: Interactive decisions cached and reused
- **Interrupted Recovery**: Graceful handling of partial imports
- **Large Dataset Handling**: Performance with 100+ games
- **Duplicate Title Handling**: Games with identical names

## Performance Testing

### Large Dataset Testing

```bash
# Run scalability tests
uv run pytest scripts/tests/test_idempotency.py::TestScalabilityIdempotency -v

# Run with timing information
uv run pytest scripts/tests/ --durations=10
```

### Memory Usage Testing

```bash
# Run with memory profiling (if memory-profiler is installed)
uv run pytest scripts/tests/ --profile-mem
```

## Mocking and Test Isolation

### API Mocking

Tests use comprehensive mocking to avoid external dependencies:
- **HTTP Requests**: All API calls are mocked with realistic responses
- **Authentication**: JWT tokens and login flows mocked
- **File System**: Decision cache files use temporary directories
- **Time**: Timestamps can be controlled for deterministic tests

### Database Independence

Tests run without requiring a live database:
- **SQLModel Operations**: Mocked at the API client level
- **Migration Independence**: No Alembic migrations required
- **Platform Data**: Seed data mocked in test fixtures

## Continuous Integration

### GitHub Actions Integration

Tests are designed to run in CI/CD environments:
- **No External Dependencies**: All services mocked
- **Deterministic Results**: Tests produce identical results across runs
- **Fast Execution**: Optimized for CI performance
- **Comprehensive Coverage**: Critical paths fully tested

### Pre-commit Integration

Consider running tests before commits:
```bash
# Quick smoke test before commit
uv run pytest scripts/tests/test_merge_strategies.py::TestMergeStrategyBaseFunctionality::test_find_existing_game_exact_match -v
```

## Troubleshooting

### Common Issues

#### ImportError: pandas/numpy dependency issues
```bash
# Issue: Numpy import errors from pandas
# Solution: Ensure you're using uv run and not direct python
uv run pytest scripts/tests/ -v  # ✓ Correct
python -m pytest scripts/tests/  # ✗ May fail
```

#### Path/Module Import Issues
```bash
# Issue: Cannot import scripts.darkadia modules  
# Solution: Run from backend directory
cd /path/to/nexorious/backend
uv run pytest scripts/tests/ -v
```

#### Cache/Temporary File Issues
```bash
# Issue: Decision cache conflicts between tests
# Solution: Tests use temporary directories automatically
# Manual cleanup if needed:
rm -rf ~/.nexorious/import_decisions.json
```

#### Performance Issues
```bash
# Issue: Tests running slowly
# Solutions:
# 1. Run specific test modules instead of all tests
uv run pytest scripts/tests/test_api_client.py -v

# 2. Use shorter test names/patterns
uv run pytest scripts/tests/ -k "not large_dataset" -v

# 3. Reduce verbosity
uv run pytest scripts/tests/ --tb=short
```

### Debugging Test Failures

#### Verbose Output
```bash
# Show detailed failure information
uv run pytest scripts/tests/test_merge_strategies.py::TestInteractiveMerger::test_decision_cache_loading -vv -s
```

#### Print Debugging
```bash
# Add print statements and use -s to see output
uv run pytest scripts/tests/ -s -v
```

#### Debugger Integration
```bash
# Drop into debugger on failure (if pdb is available)
uv run pytest scripts/tests/ --pdb
```

### Test Environment Validation

#### Check Dependencies
```bash
# Verify all test dependencies are installed
uv run python -c "import pytest, httpx, pandas, rich; print('All dependencies available')"
```

#### Validate Test Discovery
```bash
# Show which tests will be collected
uv run pytest scripts/tests/ --collect-only
```

## Writing New Tests

### Test Structure Guidelines

1. **Use descriptive test names**: `test_interactive_merger_caches_decision_across_runs`
2. **Follow AAA pattern**: Arrange, Act, Assert
3. **Use appropriate fixtures**: Leverage existing mock fixtures
4. **Test edge cases**: Empty data, network failures, malformed input
5. **Maintain idempotency**: Tests should not affect each other

### Example Test Template

```python
@pytest.mark.asyncio
async def test_new_functionality(mock_api_client, sample_data):
    """Test description explaining what this validates."""
    # Arrange
    merger = OverwriteMerger(mock_api_client, dry_run=False)
    mock_api_client.search_games.return_value = []
    
    # Act
    result = await merger.process_games(sample_data, 'user123')
    
    # Assert
    assert result['new_games'] == len(sample_data)
    assert result['errors'] == 0
    mock_api_client.create_user_game.assert_called()
```

### Test Coverage Requirements

- **New Code**: Must have >90% coverage
- **Critical Paths**: Must have 100% coverage
- **Error Handling**: All exception paths tested
- **Edge Cases**: Boundary conditions validated

## Related Documentation

- [Main Project Testing Documentation](../../README.md#testing)
- [Import Script Usage Guide](../README.md) (when available)
- [API Documentation](../../docs/api.md) (when available)
- [Development Setup](../../README.md#development-setup)

---

*Last Updated: $(date)*
*For issues with this documentation, please update this file or contact the development team.*