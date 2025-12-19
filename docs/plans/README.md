# Implementation Plans

This directory contains detailed implementation plans for Nexorious features and improvements.

## Plan Coverage by Issue

### Backend Issues

| Issue ID | Title | Plan |
|----------|-------|------|
| nexorious-9q0 | Narrow exception handling in file upload validators | [backend-exception-handling.md](2025-12-17-backend-exception-handling.md) |
| nexorious-3ey | Narrow exception handling in WebSocket rollback | [backend-exception-handling.md](2025-12-17-backend-exception-handling.md) |
| nexorious-0qp | Narrow exception handling in platform schema validators | [backend-exception-handling.md](2025-12-17-backend-exception-handling.md) |
| nexorious-ma1 | Narrow exception handling in storage URL validator | [backend-exception-handling.md](2025-12-17-backend-exception-handling.md) |
| nexorious-5ez | Narrow exception handling in database migrations | [backend-exception-handling.md](2025-12-17-backend-exception-handling.md) |
| nexorious-rx0 | Research: Evaluate switching from Pyrefly to Pyright | [backend-architecture-refactoring.md](2025-12-17-backend-architecture-refactoring.md) |
| nexorious-9mu | Fix layering violations | [backend-architecture-refactoring.md](2025-12-17-backend-architecture-refactoring.md) |
| nexorious-8wy | Break down monolithic API files | [backend-architecture-refactoring.md](2025-12-17-backend-architecture-refactoring.md) |
| nexorious-2cc | Remove unused SQLAlchemy wrapper functions | [backend-cleanup.md](2025-12-17-backend-cleanup.md) |
| nexorious-yiz | Remove unused get_user_agent() | [backend-cleanup.md](2025-12-17-backend-cleanup.md) |

### Frontend (Svelte) Issues

| Issue ID | Title | Plan |
|----------|-------|------|
| nexorious-548 | Frontend Anti-Pattern Refactoring Epic | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-ayw | Consolidate store creation patterns | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-9xc | Replace TypeScript any types | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-qpk | Break down large components | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-lgs | Refactor event listener cleanup | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-60w | Improve timer/interval cleanup | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-ife | Fix manual reactivity hacks | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-idn | Extract duplicated component utilities | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |
| nexorious-5hn | Replace hardcoded timeout values | [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) |

### Frontend Cleanup Issues

| Issue ID | Title | Plan |
|----------|-------|------|
| nexorious-e80 | Remove unused import-helpers functions | [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) |
| nexorious-sh6 | Remove unused createCompactPlatformDisplay | [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) |
| nexorious-5oo | Remove unused steam-utils.ts | [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) |
| nexorious-857 | Remove unused components | [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) |
| nexorious-b3h | Remove commented-out code blocks | [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) |
| nexorious-dr3 | Clean up empty handleTagChange | [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) |
| nexorious-chc | Update frontend documentation comments | [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) |

### Frontend Test Issues

| Issue ID | Title | Plan |
|----------|-------|------|
| nexorious-j3x | Frontend Test Anti-Pattern Cleanup Epic | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-z9m | Tests verify snapshot objects with excessive toMatchObject/toEqual | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-f21 | Missing assertions in tests | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-0kh | Complex test setup with excessive beforeEach mocking | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-f9s | Tests use container.querySelector instead of testing-library queries | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-ofl | Tests use fireEvent instead of userEvent | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-71i | Duplicate test coverage across multiple test files | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-6t9 | Tests using getAllByText for assertions | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |
| nexorious-7ju | Excessive CSS class testing | [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) |

### Frontend-Next Issues

| Issue ID | Title | Plan |
|----------|-------|------|
| nexorious-jjda | Phase 6: Post-MVP Feature Migration | [nextjs-frontend-implementation.md](2025-12-16-nextjs-frontend-implementation.md) |
| nexorious-4z7f | Add tests for QueryProvider | [frontend-next-queryprovider-tests.md](2025-12-19-frontend-next-queryprovider-tests.md) |

### Feature Issues

| Issue ID | Title | Plan |
|----------|-------|------|
| nexorious-0ppx | Add ability to unmatch/rematch resolved review items | [unmatch-rematch-review-items.md](2025-12-19-unmatch-rematch-review-items.md) |

---

## All Plans

### Architecture & Design
- [background-task-system-design.md](2025-12-13-background-task-system-design.md) - Background task processing with taskiq
- [sync-import-export-separation-design.md](2025-12-13-sync-import-export-separation-design.md) - Task separation architecture
- [import-sync-refactor-design.md](2025-12-15-import-sync-refactor-design.md) - Import/sync architecture refactor
- [nextjs-frontend-rewrite-design.md](2025-12-16-nextjs-frontend-rewrite-design.md) - Next.js frontend design
- [navigation-redesign.md](2025-12-15-navigation-redesign.md) - Navigation structure simplification

### Implementation Plans
- [export-to-file-design.md](2025-12-13-export-to-file-design.md) - Manual export functionality
- [darkadia-worker-migration-design.md](2025-12-15-darkadia-worker-migration-design.md) - Darkadia import migration
- [darkadia-worker-migration-plan.md](2025-12-15-darkadia-worker-migration-plan.md) - Darkadia implementation steps
- [import-sync-refactor-implementation.md](2025-12-15-import-sync-refactor-implementation.md) - Import/sync implementation
- [nextjs-frontend-implementation.md](2025-12-16-nextjs-frontend-implementation.md) - Next.js frontend implementation
- [game-edit-form-implementation.md](2025-12-17-game-edit-form-implementation.md) - Game edit form feature
- [setup-page-route-guard-integration.md](2025-12-18-setup-page-route-guard-integration.md) - Setup page integration
- [sync-page-frontend-next.md](2025-12-18-sync-page-frontend-next.md) - Sync page implementation

### Refactoring & Cleanup
- [backend-exception-handling.md](2025-12-17-backend-exception-handling.md) - Exception handling improvements
- [backend-architecture-refactoring.md](2025-12-17-backend-architecture-refactoring.md) - Backend architecture improvements
- [backend-cleanup.md](2025-12-17-backend-cleanup.md) - Backend unused code removal
- [frontend-antipattern-refactoring.md](2025-12-19-frontend-antipattern-refactoring.md) - Frontend anti-pattern fixes
- [frontend-cleanup-tasks.md](2025-12-19-frontend-cleanup-tasks.md) - Frontend cleanup tasks
- [frontend-test-antipattern-cleanup.md](2025-12-19-frontend-test-antipattern-cleanup.md) - Test anti-pattern fixes

### Features
- [unmatch-rematch-review-items.md](2025-12-19-unmatch-rematch-review-items.md) - Unmatch/rematch review items
- [frontend-next-queryprovider-tests.md](2025-12-19-frontend-next-queryprovider-tests.md) - QueryProvider tests
