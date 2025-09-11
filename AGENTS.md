# AGENTS.md

Comprehensive guide for agentic coding agents working on the Nexorious game collection app (FastAPI + SvelteKit + TypeScript).

## Build/Lint/Test Commands

### Backend (Python 3.13 + FastAPI + SQLModel + pytest)
- Install: `cd backend && uv sync`
- Test all: `uv run pytest --cov=app --cov-report=term-missing` (>80% required)
- Test single: `uv run pytest app/tests/test_specific.py::test_function_name -v`
- Lint: `uv run ruff check .` (must pass)
- Dev server: `uv run python -m app.main`
- Migration: `uv run alembic revision --autogenerate -m "description"`

### Frontend (SvelteKit + TypeScript + Vitest)
- Install: `cd frontend && npm install`
- Test all: `npm run test` (>70% required)
- Test single: `npm run test GameCard.test.ts`
- Type check: `npm run check` (must pass)
- Dev server: `npm run dev`
- Build: `npm run build`

## Code Style Guidelines

### Python Backend
- **Imports**: Standard library → Third-party → Local (`from ..core.database import get_session`)
- **Naming**: snake_case functions/variables, PascalCase classes, UPPER_CASE constants
- **Type hints**: Always use (FastAPI dependency injection pattern: `Annotated[Session, Depends(get_session)]`)
- **SQLModel**: Read `docs/sqlmodel-computed-fields-guide.md` for computed fields
- **Pydantic**: Read `docs/pydantic-v2-best-practices.md` - use `model_validate()`, `model_dump()`, `@field_validator`
- **Error handling**: FastAPI HTTPException with proper status codes
- **Async**: Use `async def` for database operations and external APIs
- **Documentation**: Docstrings for classes/functions, type hints for all parameters

### TypeScript Frontend
- **Imports**: External libraries → Internal (`$lib/...`) → Types (`type`, `interface`)
- **Naming**: camelCase variables/functions, PascalCase components, UPPER_CASE constants
- **Svelte 5**: Read `docs/svelte5-syntax-guide.md` - use `$state()`, `$derived()`, `$effect()`, `$props()`
- **Props**: Interface-typed with default values: `let { game, isLoading = false }: Props = $props()`
- **State**: `let count = $state(0)` for reactivity, `$derived()` for computed values
- **Styling**: Tailwind classes, responsive design patterns
- **Components**: Single responsibility, clear prop interfaces, accessible markup

### Testing Conventions
- **Backend**: `test_*.py` in `app/tests/`, async test functions, comprehensive coverage
- **Frontend**: `*.test.ts` (not `+*.test.ts`), component testing with @testing-library/svelte
- **Naming**: `test_should_create_user_when_valid_data()` (descriptive, behavioral)
- **Assertions**: Clear, specific error messages
- **Fixtures**: Reusable test data in `conftest.py`

### Required Reading Before Coding
- `docs/PRD.md` - Product requirements
- `docs/pydantic-v2-best-practices.md` - Before any Pydantic models
- `docs/sqlmodel-computed-fields-guide.md` - Before SQLModel computed fields  
- `docs/svelte5-syntax-guide.md` - Before Svelte components
- `docs/alembic-migrations-guide.md` - Before database migrations

### Quality Gates (Zero Tolerance)
- All tests must pass before committing
- Backend: >80% coverage, `uv run ruff check .` passes
- Frontend: >70% coverage, `npm run check` passes
- Fix failing tests immediately in same session
- Never commit secrets, always use proper error handling