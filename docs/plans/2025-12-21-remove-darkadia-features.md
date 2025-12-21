# Remove Darkadia Features Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove all Darkadia CSV import functionality from backend and frontend, keeping only Nexorious JSON import and the review system for sync operations.

**Architecture:** Delete Darkadia-specific models, services, endpoints, and UI components. Simplify the review system to only handle sync review (no platform/storefront mapping). Reset database migrations to a fresh initial state.

**Tech Stack:** FastAPI, SQLModel, Alembic, Next.js, React, TypeScript

---

## Task 1: Backend - Remove Darkadia Models

**Files:**
- Delete: `backend/app/models/darkadia_game.py`
- Delete: `backend/app/models/darkadia_import.py`
- Delete: `backend/app/models/user_import_mapping.py`
- Modify: `backend/app/models/__init__.py`

**Step 1: Delete model files**

```bash
rm backend/app/models/darkadia_game.py
rm backend/app/models/darkadia_import.py
rm backend/app/models/user_import_mapping.py
```

**Step 2: Update models __init__.py**

Remove imports and exports for:
- `DarkadiaGame`
- `DarkadiaImport`
- `UserImportMapping`
- `MappingType` (enum from user_import_mapping)

**Step 3: Run type check to find broken imports**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

Expected: Errors pointing to files that import deleted models (fix in later tasks)

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: remove Darkadia model files"
```

---

## Task 2: Backend - Remove Platform Resolution Service

**Files:**
- Delete: `backend/app/services/platform_resolution/` (entire directory)

**Step 1: Delete the directory**

```bash
rm -rf backend/app/services/platform_resolution/
```

**Step 2: Run type check to find broken imports**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

Expected: Errors in files that import from `platform_resolution` (fix in later tasks)

**Step 3: Commit**

```bash
git add -A && git commit -m "chore: remove platform resolution service"
```

---

## Task 3: Backend - Remove Darkadia Import Task

**Files:**
- Delete: `backend/app/worker/tasks/import_export/import_darkadia.py`
- Modify: `backend/app/worker/tasks/import_export/__init__.py`

**Step 1: Delete the task file**

```bash
rm backend/app/worker/tasks/import_export/import_darkadia.py
```

**Step 2: Update __init__.py**

Remove import and export for `import_darkadia_csv` task.

**Step 3: Commit**

```bash
git add -A && git commit -m "chore: remove Darkadia import task"
```

---

## Task 4: Backend - Remove Darkadia Schemas

**Files:**
- Delete: `backend/app/schemas/darkadia.py`
- Modify: `backend/app/schemas/__init__.py`

**Step 1: Delete schema file**

```bash
rm backend/app/schemas/darkadia.py
```

**Step 2: Update schemas __init__.py**

Remove all Darkadia-related imports and exports.

**Step 3: Commit**

```bash
git add -A && git commit -m "chore: remove Darkadia schemas"
```

---

## Task 5: Backend - Update Enums (Remove DARKADIA)

**Files:**
- Modify: `backend/app/models/job.py` (or wherever `BackgroundJobSource` is defined)

**Step 1: Find and update the enum**

```bash
cd /home/abo/workspace/home/nexorious/backend && grep -r "DARKADIA" --include="*.py" -l
```

Remove `DARKADIA` from `BackgroundJobSource` enum.

**Step 2: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

**Step 3: Commit**

```bash
git add -A && git commit -m "chore: remove DARKADIA from BackgroundJobSource enum"
```

---

## Task 6: Backend - Update Import Endpoints

**Files:**
- Modify: `backend/app/api/import_endpoints.py`

**Step 1: Remove Darkadia endpoint**

Remove the `POST /import/darkadia` endpoint and any related helper functions.

**Step 2: Remove unused imports**

Clean up imports that were only used by the Darkadia endpoint.

**Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

**Step 4: Commit**

```bash
git add -A && git commit -m "feat: remove Darkadia CSV import endpoint"
```

---

## Task 7: Backend - Simplify Review API (Remove Platform Mapping)

**Files:**
- Modify: `backend/app/api/review.py`
- Modify: `backend/app/schemas/review.py`

**Step 1: Remove platform-summary endpoint**

Remove `GET /review/platform-summary` endpoint.

**Step 2: Simplify finalize endpoint**

Remove `platform_mappings` and `storefront_mappings` parameters from `FinalizeImportRequest`.
The finalize endpoint should still work for sync review items (which don't need platform mapping).

**Step 3: Update review schemas**

Remove from `backend/app/schemas/review.py`:
- `PlatformMappingSuggestion`
- `PlatformSummaryResponse`
- Platform/storefront mapping fields from `FinalizeImportRequest`

**Step 4: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

**Step 5: Commit**

```bash
git add -A && git commit -m "feat: simplify review API, remove platform mapping"
```

---

## Task 8: Backend - Clean Up Remaining Imports

**Files:**
- Various files with broken imports

**Step 1: Run type check to find all errors**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

**Step 2: Fix each broken import**

Remove or update imports in any remaining files that reference deleted code.

**Step 3: Run tests**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest
```

Expected: Some tests will fail (Darkadia-specific tests)

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: fix broken imports after Darkadia removal"
```

---

## Task 9: Backend - Remove Darkadia Tests

**Files:**
- Delete/Modify: `backend/app/tests/test_import_endpoints.py` (remove Darkadia tests)
- Delete/Modify: `backend/app/tests/test_import_tasks.py` (remove Darkadia tests)
- Delete/Modify: `backend/app/tests/test_review_api.py` (remove platform mapping tests)
- Delete/Modify: `backend/app/tests/test_user_import_mapping.py` (delete entirely if only Darkadia)
- Delete/Modify: `backend/app/tests/test_import_mapping_api.py` (delete entirely if only Darkadia)

**Step 1: Identify Darkadia-specific test files**

```bash
cd /home/abo/workspace/home/nexorious/backend && grep -r "darkadia\|Darkadia\|DARKADIA" app/tests/ --include="*.py" -l
```

**Step 2: Remove Darkadia tests**

Delete entire test files if they're Darkadia-only, or remove specific test classes/functions.

**Step 3: Run all tests**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest
```

Expected: All tests pass

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: remove Darkadia-related tests"
```

---

## Task 10: Backend - Reset Database Migrations

**Files:**
- Delete: `backend/alembic/versions/*.py` (all migration files)
- Create: New initial migration

**Step 1: Delete all existing migrations**

```bash
rm backend/alembic/versions/*.py
```

**Step 2: Generate fresh initial migration**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run alembic revision --autogenerate -m "initial schema"
```

**Step 3: Review generated migration**

Ensure the migration does NOT include:
- `darkadia_games` table
- `darkadia_imports` table
- `user_import_mappings` table

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: reset migrations to fresh initial schema"
```

---

## Task 11: Frontend - Remove Import Mapping Page

**Files:**
- Delete: `frontend/src/app/(main)/import/mapping/page.tsx`
- Delete: `frontend/src/app/(main)/import/mapping/page.test.tsx`

**Step 1: Delete the page and test**

```bash
rm frontend/src/app/\(main\)/import/mapping/page.tsx
rm frontend/src/app/\(main\)/import/mapping/page.test.tsx
rmdir frontend/src/app/\(main\)/import/mapping/
rmdir frontend/src/app/\(main\)/import/ 2>/dev/null || true
```

**Step 2: Commit**

```bash
git add -A && git commit -m "chore: remove Darkadia import mapping page"
```

---

## Task 12: Frontend - Remove Mapping Section Component

**Files:**
- Delete: `frontend/src/components/import/mapping-section.tsx`
- Delete: `frontend/src/components/import/mapping-section.test.tsx`

**Step 1: Delete the component and test**

```bash
rm frontend/src/components/import/mapping-section.tsx
rm frontend/src/components/import/mapping-section.test.tsx
rmdir frontend/src/components/import/ 2>/dev/null || true
```

**Step 2: Commit**

```bash
git add -A && git commit -m "chore: remove import mapping section component"
```

---

## Task 13: Frontend - Remove Import Mapping Context

**Files:**
- Delete: `frontend/src/contexts/import-mapping-context.tsx`
- Modify: `frontend/src/contexts/index.ts` (if exists)

**Step 1: Delete the context**

```bash
rm frontend/src/contexts/import-mapping-context.tsx
```

**Step 2: Update context exports if needed**

Remove export from index file if it exists.

**Step 3: Commit**

```bash
git add -A && git commit -m "chore: remove import mapping context"
```

---

## Task 14: Frontend - Update Types (Remove DARKADIA)

**Files:**
- Modify: `frontend/src/types/import-export.ts`

**Step 1: Update ImportSource enum**

Remove `DARKADIA = 'darkadia'` from the enum.

**Step 2: Update getImportSourceDisplayInfo**

Remove the `[ImportSource.DARKADIA]` entry from the info object.

**Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: Errors in files that use `ImportSource.DARKADIA`

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: remove DARKADIA from ImportSource enum"
```

---

## Task 15: Frontend - Update Import/Export API

**Files:**
- Modify: `frontend/src/api/import-export.ts`
- Modify: `frontend/src/api/import-export.test.ts`

**Step 1: Remove importDarkadiaCsv function**

Delete the `importDarkadiaCsv()` function.

**Step 2: Update tests**

Remove any tests for the deleted function.

**Step 3: Run tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: remove Darkadia import API function"
```

---

## Task 16: Frontend - Update Import/Export Page

**Files:**
- Modify: `frontend/src/app/(main)/import-export/page.tsx`

**Step 1: Remove Darkadia import card**

Remove the `ImportCard` for `ImportSource.DARKADIA`.

**Step 2: Remove useImportDarkadia hook usage**

Remove import and usage of `useImportDarkadia`.

**Step 3: Update handleImportFile**

Remove the Darkadia-specific routing logic to `/import/mapping`.

**Step 4: Update info alert**

Remove the paragraph about Darkadia CSV imports.

**Step 5: Run type check and tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test
```

**Step 6: Commit**

```bash
git add -A && git commit -m "feat: remove Darkadia import from import/export page"
```

---

## Task 17: Frontend - Update Review Hooks

**Files:**
- Modify: `frontend/src/hooks/use-review.ts`

**Step 1: Remove usePlatformSummary hook**

Delete the hook and its related types.

**Step 2: Simplify useFinalizeImport**

Remove platform_mappings and storefront_mappings parameters if present.

**Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: remove platform mapping from review hooks"
```

---

## Task 18: Frontend - Update Review API

**Files:**
- Modify: `frontend/src/api/review.ts`

**Step 1: Remove getPlatformSummary function**

Delete the function.

**Step 2: Simplify finalizeImport**

Remove platform_mappings and storefront_mappings parameters.

**Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: remove platform mapping from review API"
```

---

## Task 19: Frontend - Update Review Page

**Files:**
- Modify: `frontend/src/app/(main)/review/page.tsx`

**Step 1: Remove platform mapping UI**

Remove any mapping-section component usage or platform/storefront mapping UI elements.

**Step 2: Simplify finalize logic**

Update finalize to not pass mapping parameters.

**Step 3: Run type check and tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test
```

**Step 4: Commit**

```bash
git add -A && git commit -m "feat: simplify review page, remove platform mapping"
```

---

## Task 20: Frontend - Remove Darkadia Hooks

**Files:**
- Modify: `frontend/src/hooks/index.ts` (or wherever hooks are exported)
- Delete/Modify: Hook file containing `useImportDarkadia`

**Step 1: Find and remove useImportDarkadia**

```bash
cd /home/abo/workspace/home/nexorious/frontend && grep -r "useImportDarkadia" --include="*.ts" --include="*.tsx" -l
```

Remove the hook definition and exports.

**Step 2: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

**Step 3: Commit**

```bash
git add -A && git commit -m "chore: remove useImportDarkadia hook"
```

---

## Task 21: Frontend - Clean Up Remaining Imports

**Files:**
- Various files with broken imports

**Step 1: Run full check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

**Step 2: Fix any remaining errors**

Remove unused imports and fix any type errors.

**Step 3: Run all tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

**Step 4: Commit**

```bash
git add -A && git commit -m "chore: clean up imports after Darkadia removal"
```

---

## Task 22: Final Verification

**Step 1: Run backend tests with coverage**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing
```

Expected: All tests pass, coverage >80%

**Step 2: Run frontend checks and tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test
```

Expected: All checks pass, all tests pass

**Step 3: Start backend and verify endpoints**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run python -m app.main &
curl http://localhost:8000/docs  # Verify no Darkadia endpoints
```

**Step 4: Verify conversion script still works**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run python scripts/darkadia_to_nexorious.py --help
```

Expected: Script runs without import errors

**Step 5: Final commit if any changes**

```bash
git add -A && git commit -m "chore: final cleanup after Darkadia feature removal"
```

---

## Summary

After completing all tasks:

**Removed:**
- 3 models (DarkadiaGame, DarkadiaImport, UserImportMapping)
- 1 service directory (platform_resolution/)
- 1 background task (import_darkadia.py)
- 1 schema file (darkadia.py)
- 1 enum value (DARKADIA from BackgroundJobSource)
- 1 API endpoint (POST /import/darkadia)
- 1 API endpoint (GET /review/platform-summary)
- 1 frontend page (/import/mapping)
- 1 frontend component (mapping-section)
- 1 frontend context (import-mapping-context)
- Multiple hooks and API functions

**Kept:**
- Review system (for sync operations)
- Job system
- IGDB matching service
- Nexorious JSON import/export
- darkadia_to_nexorious.py conversion script
