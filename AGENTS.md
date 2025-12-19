# AGENTS.md

Comprehensive guide for agentic coding agents working on the Nexorious game collection app (FastAPI + Next.js + TypeScript).

## Build/Lint/Test Commands

### Backend (Python 3.13 + FastAPI + SQLModel + pytest)
- Install: `cd backend && uv sync`
- Test all: `uv run pytest --cov=app --cov-report=term-missing` (>80% required)
- Test single: `uv run pytest app/tests/test_specific.py::test_function_name -v`
- Lint: `uv run ruff check .` (must pass)
- Dev server: `uv run python -m app.main`
- Migration: `uv run alembic revision --autogenerate -m "description"`

### Frontend (Next.js 16 + React 19 + TypeScript + Vitest)
- Install: `cd frontend && npm install`
- Test all: `npm run test` (>70% required)
- Test single: `npm run test game-card.test.tsx`
- Type check: `npm run check` (must pass - tsc && eslint)
- Dev server: `npm run dev` (port 3000)
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

### TypeScript Frontend (Next.js + React)
- **Imports**: External libraries → Internal (`@/...`) → Types (`type`, `interface`)
- **Naming**: camelCase variables/functions, PascalCase components, UPPER_CASE constants
- **React**: Functional components with hooks (useState, useEffect, custom hooks)
- **Props**: Interface-typed with destructuring: `function GameCard({ game, isLoading = false }: Props)`
- **State**: TanStack Query for server state, useState for local state
- **Styling**: Tailwind CSS classes, shadcn/ui components, responsive design patterns
- **Components**: Single responsibility, clear prop interfaces, accessible markup

### Testing Conventions
- **Backend**: `test_*.py` in `app/tests/`, async test functions, comprehensive coverage
- **Frontend**: `*.test.ts` or `*.test.tsx` alongside source files, component testing with @testing-library/react
- **Naming**: `test_should_create_user_when_valid_data()` (descriptive, behavioral)
- **Assertions**: Clear, specific error messages
- **Fixtures**: Reusable test data in `conftest.py`

### Quality Gates (Zero Tolerance)
- All tests must pass before committing
- Backend: >80% coverage, `uv run ruff check .` passes
- Frontend: >70% coverage, `npm run check` passes
- Fix failing tests immediately in same session
- Never commit secrets, always use proper error handling
