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

## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Auto-syncs to JSONL for version control
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**
```bash
bd ready --json
```

**Create new issues:**
```bash
bd create "Issue title" -t bug|feature|task -p 0-4 --json
bd create "Issue title" -p 1 --deps discovered-from:bd-123 --json
bd create "Subtask" --parent <epic-id> --json  # Hierarchical subtask (gets ID like epic-id.1)
```

**Claim and update:**
```bash
bd update bd-42 --status in_progress --json
bd update bd-42 --priority 1 --json
```

**Complete work:**
```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Create feature branch**: `git checkout -b <issue-id>-<description>` (e.g., `bd-42-fix-login`)
3. **Claim your task**: `bd update <id> --status in_progress`
4. **Work on it**: Implement, test, document
5. **Discover new work?** Create linked issue:
   - `bd create "Found bug" -p 1 --deps discovered-from:<parent-id>`
6. **Complete**: `bd close <id> --reason "Done"`
7. **Sync and push**: `bd sync && git push -u origin <branch-name>`
8. **Create PR**: `gh pr create --title "..." --body "Closes <issue-id>"`

### Branch Workflow (MANDATORY)

**AI agents MUST use branches when working on tasks. Never commit directly to main.**

#### Starting a Task
```bash
# Ensure on main and up to date
git checkout main && git pull origin main

# Create feature branch with issue ID
git checkout -b bd-42-fix-login-bug

# Claim the work
bd update bd-42 --status in_progress
```

#### Branch Naming
- Format: `<issue-id>-<short-kebab-case-description>`
- Examples: `bd-42-fix-login-bug`, `bd-55-add-dark-mode`

#### Completing a Task
```bash
# Run tests (must pass)
uv run pytest  # Backend
npm run check && npm run test  # Frontend

# Close issue and sync
bd close bd-42
bd sync

# Commit, push, and create PR
git add .
git commit -m "fix: resolve login bug (bd-42)"
git push -u origin bd-42-fix-login-bug
gh pr create --title "Fix login bug" --body "Closes bd-42"
```

#### Rules
- ✅ Create branch before ANY task work
- ✅ Name branches with beads issue ID
- ✅ One task per branch
- ✅ Create PRs for merging to main
- ❌ Never commit directly to main
- ❌ Never mix unrelated changes in one branch

### Auto-Sync

bd automatically syncs with git:
- Exports to `.beads/issues.jsonl` after changes (5s debounce)
- Imports from JSONL when newer (e.g., after `git pull`)
- No manual export/import needed!

### GitHub Copilot Integration

If using GitHub Copilot, also create `.github/copilot-instructions.md` for automatic instruction loading.
Run `bd onboard` to get the content, or see step 2 of the onboard instructions.

### MCP Server (Recommended)

If using Claude or MCP-compatible clients, install the beads MCP server:

```bash
pip install beads-mcp
```

Add to MCP config (e.g., `~/.config/claude/config.json`):
```json
{
  "beads": {
    "command": "beads-mcp",
    "args": []
  }
}
```

Then use `mcp__beads__*` functions instead of CLI commands.

### Managing AI-Generated Planning Documents

AI assistants often create planning and design documents during development:
- PLAN.md, IMPLEMENTATION.md, ARCHITECTURE.md
- DESIGN.md, CODEBASE_SUMMARY.md, INTEGRATION_PLAN.md
- TESTING_GUIDE.md, TECHNICAL_DESIGN.md, and similar files

**Best Practice: Use a dedicated directory for these ephemeral files**

**Recommended approach:**
- Create a `history/` directory in the project root
- Store ALL AI-generated planning/design docs in `history/`
- Keep the repository root clean and focused on permanent project files
- Only access `history/` when explicitly asked to review past planning

**Example .gitignore entry (optional):**
```
# AI planning documents (ephemeral)
history/
```

**Benefits:**
- ✅ Clean repository root
- ✅ Clear separation between ephemeral and permanent documentation
- ✅ Easy to exclude from version control if desired
- ✅ Preserves planning history for archeological research
- ✅ Reduces noise when browsing the project

### CLI Help

Run `bd <command> --help` to see all available flags for any command.
For example: `bd create --help` shows `--parent`, `--deps`, `--assignee`, etc.

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ✅ Store AI planning docs in `history/` directory
- ✅ Run `bd <cmd> --help` to discover available flags
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems
- ❌ Do NOT clutter repo root with planning documents

For more details, see README.md and QUICKSTART.md.
