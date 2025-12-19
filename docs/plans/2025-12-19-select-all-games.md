# Select All Games Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a "select all" checkbox to the games page bulk actions bar with a three-state click cycle (none → all visible → all collection → none).

**Architecture:** New backend endpoint returns filtered game IDs only (lightweight). Frontend adds selection mode state and checkbox to BulkActions component. Checkbox cycles through states on click, fetching all IDs lazily when entering "all collection" mode.

**Tech Stack:** FastAPI (backend), React/TanStack Query (frontend), shadcn/ui Checkbox component

---

### Task 1: Backend - Add User Game IDs Endpoint

**Files:**
- Modify: `backend/app/api/user_games.py`
- Modify: `backend/app/schemas/user_game.py`
- Test: `backend/app/tests/test_integration_user_games.py`

**Step 1: Write the failing test**

Add to `backend/app/tests/test_integration_user_games.py`:

```python
class TestUserGameIdsEndpoint:
    """Test GET /api/user-games/ids endpoint."""

    def test_get_user_game_ids_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user game IDs retrieval."""
        response = client.get("/api/user-games/ids", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert "ids" in data
        assert isinstance(data["ids"], list)
        assert str(test_user_game.id) in data["ids"]

    def test_get_user_game_ids_without_auth(self, client: TestClient):
        """Test user game IDs without authentication."""
        response = client.get("/api/user-games/ids")

        assert_api_error(response, 403, "Not authenticated")

    def test_get_user_game_ids_with_filter(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test user game IDs with status filter."""
        response = client.get(
            f"/api/user-games/ids?play_status={test_user_game.play_status.value}",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        assert str(test_user_game.id) in data["ids"]
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_user_games.py::TestUserGameIdsEndpoint -v`
Expected: FAIL with "404 Not Found" (endpoint doesn't exist)

**Step 3: Add response schema**

Add to `backend/app/schemas/user_game.py` after `CollectionStatsResponse`:

```python
class UserGameIdsResponse(BaseModel):
    """Response schema for user game IDs list."""
    ids: List[str] = Field(..., description="List of user game IDs")
```

**Step 4: Add the endpoint**

Add to `backend/app/api/user_games.py` after the `list_user_games` endpoint (around line 260):

```python
@router.get("/ids", response_model=UserGameIdsResponse)
async def get_user_game_ids(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    play_status: Optional[PlayStatus] = Query(default=None, description="Filter by play status"),
    ownership_status: Optional[OwnershipStatus] = Query(default=None, description="Filter by ownership status"),
    is_loved: Optional[bool] = Query(default=None, description="Filter by loved status"),
    platform_id: Optional[str] = Query(default=None, description="Filter by platform"),
    storefront_id: Optional[str] = Query(default=None, description="Filter by storefront"),
    rating_min: Optional[float] = Query(default=None, ge=1, le=5, description="Minimum rating filter"),
    rating_max: Optional[float] = Query(default=None, ge=1, le=5, description="Maximum rating filter"),
    has_notes: Optional[bool] = Query(default=None, description="Filter by presence of notes"),
    q: Optional[str] = Query(default=None, description="Search in game titles and notes"),
):
    """Get all user game IDs matching filters (lightweight endpoint for bulk selection)."""

    # Build base query - only select IDs
    query = select(UserGame.id).where(UserGame.user_id == current_user.id)

    # Apply filters
    filters: List[Any] = []

    if play_status is not None:
        filters.append(UserGame.play_status == play_status)

    if ownership_status is not None:
        filters.append(UserGame.ownership_status == ownership_status)

    if is_loved is not None:
        filters.append(UserGame.is_loved == is_loved)

    if rating_min is not None:
        filters.append(col(UserGame.personal_rating) >= rating_min)

    if rating_max is not None:
        filters.append(col(UserGame.personal_rating) <= rating_max)

    if has_notes is not None:
        if has_notes:
            filters.append(is_not(col(UserGame.personal_notes), None))
            filters.append(UserGame.personal_notes != "")
        else:
            filters.append(or_(
                is_(col(UserGame.personal_notes), None),
                UserGame.personal_notes == ""
            ))

    if platform_id:
        query = query.join(UserGamePlatform).where(UserGamePlatform.platform_id == platform_id)

    if storefront_id:
        query = query.join(UserGamePlatform).where(UserGamePlatform.storefront_id == storefront_id)

    if filters:
        query = query.where(and_(*filters))

    if q:
        query = query.join(Game)
        query = query.where(or_(
            col(Game.title).icontains(q),
            and_(is_not(col(UserGame.personal_notes), None), col(UserGame.personal_notes).icontains(q))
        ))

    # Execute and return IDs only
    ids = session.exec(query).all()

    return UserGameIdsResponse(ids=[str(id) for id in ids])
```

**Step 5: Update schema import**

Update the import in `backend/app/api/user_games.py` to include `UserGameIdsResponse`:

```python
from ..schemas.user_game import (
    UserGameCreateRequest,
    UserGameUpdateRequest,
    ProgressUpdateRequest,
    UserGamePlatformCreateRequest,
    UserGameResponse,
    UserGameListResponse,
    UserGamePlatformResponse,
    BulkStatusUpdateRequest,
    BulkDeleteRequest,
    BulkAddPlatformRequest,
    BulkRemovePlatformRequest,
    CollectionStatsResponse,
    UserGameIdsResponse
)
```

**Step 6: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_user_games.py::TestUserGameIdsEndpoint -v`
Expected: PASS

**Step 7: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest`
Expected: All tests pass

**Step 8: Commit**

```bash
git add backend/app/api/user_games.py backend/app/schemas/user_game.py backend/app/tests/test_integration_user_games.py
git commit -m "$(cat <<'EOF'
feat(api): add /user-games/ids endpoint for bulk selection

Lightweight endpoint that returns only game IDs matching filters,
enabling efficient "select all" functionality in the frontend.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Frontend - Add API Function and Hook

**Files:**
- Modify: `frontend/src/api/games.ts`
- Modify: `frontend/src/hooks/use-games.ts`
- Modify: `frontend/src/hooks/index.ts`

**Step 1: Add API function**

Add to `frontend/src/api/games.ts` after `getCollectionStats` function:

```typescript
/**
 * Get all user game IDs matching filters (for bulk selection).
 */
export async function getUserGameIds(
  params?: GetUserGamesParams
): Promise<string[]> {
  const queryParams = buildUserGamesQueryParams(params);
  const response = await api.get<{ ids: string[] }>('/user-games/ids', {
    params: queryParams,
  });

  return response.ids;
}
```

**Step 2: Add hook**

Add to `frontend/src/hooks/use-games.ts` after `useCollectionStats`:

```typescript
/**
 * Hook to fetch all user game IDs matching filters.
 * Disabled by default - call refetch() to trigger.
 */
export function useUserGameIds(params?: GetUserGamesParams, options?: { enabled?: boolean }) {
  return useQuery<string[], Error>({
    queryKey: [...gameKeys.lists(), 'ids', params] as const,
    queryFn: () => gamesApi.getUserGameIds(params),
    enabled: options?.enabled ?? false,
  });
}
```

**Step 3: Export hook**

Update `frontend/src/hooks/index.ts` to include `useUserGameIds`:

```typescript
export {
  gameKeys,
  useUserGames,
  useUserGame,
  useUserGameIds,
  useSearchIGDB,
  // ... rest of exports
} from './use-games';
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: No TypeScript errors

**Step 5: Commit**

```bash
git add frontend/src/api/games.ts frontend/src/hooks/use-games.ts frontend/src/hooks/index.ts
git commit -m "$(cat <<'EOF'
feat(frontend): add useUserGameIds hook for bulk selection

Adds API function and React Query hook to fetch all game IDs
matching current filters, enabling "select all" functionality.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Frontend - Add SelectionMode Type

**Files:**
- Modify: `frontend/src/types/game.ts`

**Step 1: Add SelectionMode type**

Add to `frontend/src/types/game.ts` after the `PlayStatus` enum:

```typescript
/**
 * Selection mode for bulk game selection.
 * - manual: Individual games selected by clicking
 * - all-visible: All currently loaded games selected
 * - all-collection: All games in collection selected (fetched from API)
 */
export type SelectionMode = 'manual' | 'all-visible' | 'all-collection';
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: No TypeScript errors

**Step 3: Commit**

```bash
git add frontend/src/types/game.ts
git commit -m "$(cat <<'EOF'
feat(types): add SelectionMode type for bulk selection

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Frontend - Update BulkActions Component

**Files:**
- Modify: `frontend/src/components/games/bulk-actions.tsx`

**Step 1: Update props interface**

Replace the `BulkActionsProps` interface:

```typescript
export interface BulkActionsProps {
  selectedIds: Set<string>;
  onClearSelection: () => void;
  onSuccess?: () => void;
  // New props for select all
  selectionMode: SelectionMode;
  visibleCount: number;
  totalCount: number;
  onSelectAllClick: () => void;
}
```

**Step 2: Add imports**

Update imports at the top of the file:

```typescript
'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { useBulkUpdateUserGames, useBulkDeleteUserGames } from '@/hooks';
import { PlayStatus, SelectionMode } from '@/types';
import { Trash2, X } from 'lucide-react';
```

**Step 3: Update component signature and add checkbox logic**

Replace the component function:

```typescript
export function BulkActions({
  selectedIds,
  onClearSelection,
  onSuccess,
  selectionMode,
  visibleCount,
  totalCount,
  onSelectAllClick,
}: BulkActionsProps) {
  const bulkUpdate = useBulkUpdateUserGames();
  const bulkDelete = useBulkDeleteUserGames();
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

  // Don't render if no games exist
  if (totalCount === 0) {
    return null;
  }

  const handleStatusChange = async (status: PlayStatus) => {
    try {
      await bulkUpdate.mutateAsync({
        ids: Array.from(selectedIds),
        updates: { playStatus: status },
      });
      onClearSelection();
      onSuccess?.();
    } catch (error) {
      console.error('Failed to update games:', error);
    }
  };

  const handleDelete = async () => {
    try {
      await bulkDelete.mutateAsync(Array.from(selectedIds));
      setIsDeleteDialogOpen(false);
      onClearSelection();
      onSuccess?.();
    } catch (error) {
      console.error('Failed to delete games:', error);
    }
  };

  const isLoading = bulkUpdate.isPending || bulkDelete.isPending;
  const selectedCount = selectedIds.size;
  const hasSelection = selectedCount > 0;

  // Determine checkbox state
  const isAllVisibleSelected = selectionMode === 'all-visible' ||
    (selectionMode === 'manual' && selectedCount === visibleCount && visibleCount > 0);
  const isAllCollectionSelected = selectionMode === 'all-collection';
  const isIndeterminate = selectionMode === 'manual' && selectedCount > 0 && selectedCount < visibleCount;
  const isChecked = isAllVisibleSelected || isAllCollectionSelected;

  // Determine label text
  const getSelectionLabel = (): string => {
    if (isAllCollectionSelected) {
      return `All ${totalCount} in library selected`;
    }
    if (isAllVisibleSelected) {
      // Check if visible = total (collapsed state)
      if (visibleCount === totalCount) {
        return `All ${totalCount} game${totalCount !== 1 ? 's' : ''} selected`;
      }
      return `All ${visibleCount} visible selected`;
    }
    return 'Select all';
  };

  return (
    <div className="flex items-center gap-4 p-3 bg-muted rounded-lg">
      {/* Select all checkbox */}
      <div className="flex items-center gap-2">
        <Checkbox
          id="select-all"
          checked={isIndeterminate ? 'indeterminate' : isChecked}
          onCheckedChange={() => onSelectAllClick()}
          disabled={isLoading}
        />
        <label
          htmlFor="select-all"
          className="text-sm font-medium cursor-pointer select-none"
        >
          {getSelectionLabel()}
        </label>
      </div>

      {/* Show actions only when games are selected */}
      {hasSelection && (
        <>
          <div className="h-4 w-px bg-border" />

          <span className="text-sm text-muted-foreground">
            {selectedCount} game{selectedCount !== 1 ? 's' : ''} selected
          </span>

          {/* Bulk status change */}
          <Select
            onValueChange={(value) => handleStatusChange(value as PlayStatus)}
            disabled={isLoading}
          >
            <SelectTrigger className="w-40">
              <SelectValue placeholder="Change status" />
            </SelectTrigger>
            <SelectContent>
              {statusOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Delete button with confirmation */}
          <AlertDialog open={isDeleteDialogOpen} onOpenChange={setIsDeleteDialogOpen}>
            <AlertDialogTrigger asChild>
              <Button variant="destructive" size="sm" disabled={isLoading}>
                <Trash2 className="h-4 w-4 mr-1" />
                Delete
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete Games</AlertDialogTitle>
                <AlertDialogDescription>
                  Are you sure you want to delete {selectedCount} game
                  {selectedCount !== 1 ? 's' : ''}? This action cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={handleDelete} disabled={bulkDelete.isPending}>
                  Delete
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>

          {/* Clear selection */}
          <Button variant="ghost" size="sm" onClick={onClearSelection} disabled={isLoading}>
            <X className="h-4 w-4 mr-1" />
            Clear
          </Button>
        </>
      )}
    </div>
  );
}
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: TypeScript errors (games page not updated yet)

**Step 5: Commit (partial - component updated)**

```bash
git add frontend/src/components/games/bulk-actions.tsx
git commit -m "$(cat <<'EOF'
feat(bulk-actions): add select all checkbox with three-state cycle

Adds checkbox that cycles through: none -> all visible -> all collection -> none.
Shows dynamic label based on selection state.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Frontend - Update Games Page

**Files:**
- Modify: `frontend/src/app/(main)/games/page.tsx`

**Step 1: Update imports**

```typescript
'use client';

import { useState, useMemo, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { Plus } from 'lucide-react';
import { useUserGames, useUserGameIds } from '@/hooks';
import {
  GameFilters,
  GameGrid,
  GameList,
  BulkActions,
} from '@/components/games';
import { Button } from '@/components/ui/button';
import { useToast } from '@/hooks/use-toast';
import type { PlayStatus, UserGame, SelectionMode } from '@/types';
```

**Step 2: Update state and add selection logic**

Replace the component with:

```typescript
export default function GamesPage() {
  const router = useRouter();
  const { toast } = useToast();
  const [filters, setFilters] = useState<{
    search: string;
    status?: PlayStatus;
    platformId?: string;
  }>({ search: '' });
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [selectionMode, setSelectionMode] = useState<SelectionMode>('manual');

  // Build query params
  const queryParams = useMemo(
    () => ({
      search: filters.search || undefined,
      status: filters.status,
      platformId: filters.platformId,
      perPage: 50,
    }),
    [filters]
  );

  const { data, isLoading, refetch } = useUserGames(queryParams);
  const games = data?.items ?? [];
  const totalCount = data?.total ?? 0;
  const visibleCount = games.length;

  // Hook for fetching all IDs (disabled by default)
  const { refetch: fetchAllIds } = useUserGameIds(queryParams, { enabled: false });

  // Reset selection when filters change
  useEffect(() => {
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, [filters]);

  const handleSelectGame = useCallback((id: string) => {
    setSelectionMode('manual');
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, []);

  const handleSelectAllClick = useCallback(async () => {
    // Check if visible = total (collapsed state - only two states needed)
    const isCollapsedState = visibleCount === totalCount;

    if (selectionMode === 'manual' || (selectionMode === 'all-visible' && isCollapsedState)) {
      // Not all selected OR in collapsed state with all selected -> cycle appropriately
      if (selectedIds.size === 0 || selectionMode === 'manual') {
        // Select all visible
        const visibleIds = new Set(games.map((g) => g.id));
        setSelectedIds(visibleIds);
        setSelectionMode(isCollapsedState ? 'all-collection' : 'all-visible');
      } else if (isCollapsedState) {
        // In collapsed state, second click clears
        clearSelection();
      }
    } else if (selectionMode === 'all-visible') {
      // All visible selected -> select all in collection
      try {
        const result = await fetchAllIds();
        if (result.data) {
          setSelectedIds(new Set(result.data));
          setSelectionMode('all-collection');
        }
      } catch {
        toast({
          title: 'Error',
          description: 'Failed to select all games',
          variant: 'destructive',
        });
      }
    } else if (selectionMode === 'all-collection') {
      // All in collection selected -> clear
      clearSelection();
    }
  }, [selectionMode, selectedIds.size, games, visibleCount, totalCount, fetchAllIds, clearSelection, toast]);

  const handleClickGame = (game: UserGame) => {
    router.push(`/games/${game.id}`);
  };

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold">Game Library</h1>
          {data && (
            <span className="text-muted-foreground">
              {data.total} game{data.total !== 1 ? 's' : ''}
            </span>
          )}
        </div>
        <Button asChild>
          <Link href="/games/add">
            <Plus className="h-4 w-4 mr-2" />
            Add Game
          </Link>
        </Button>
      </div>

      {/* Filters */}
      <GameFilters
        filters={filters}
        onFiltersChange={setFilters}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
      />

      {/* Bulk actions */}
      <BulkActions
        selectedIds={selectedIds}
        onClearSelection={clearSelection}
        onSuccess={() => refetch()}
        selectionMode={selectionMode}
        visibleCount={visibleCount}
        totalCount={totalCount}
        onSelectAllClick={handleSelectAllClick}
      />

      {/* Games display */}
      {viewMode === 'grid' ? (
        <GameGrid
          games={games}
          isLoading={isLoading}
          selectedIds={selectedIds}
          onSelectGame={handleSelectGame}
          onClickGame={handleClickGame}
        />
      ) : (
        <GameList
          games={games}
          isLoading={isLoading}
          selectedIds={selectedIds}
          onSelectGame={handleSelectGame}
          onClickGame={handleClickGame}
        />
      )}
    </div>
  );
}
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: No TypeScript errors

**Step 4: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: All tests pass

**Step 5: Commit**

```bash
git add frontend/src/app/\(main\)/games/page.tsx
git commit -m "$(cat <<'EOF'
feat(games-page): integrate select all functionality

Adds selection mode state management with three-state cycle:
- manual: individual game selection
- all-visible: all loaded games selected
- all-collection: all games in library selected

Resets selection on filter changes.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Manual Testing

**Step 1: Start the development servers**

Run backend: `cd /home/abo/workspace/home/nexorious/backend && uv run python -m app.main`
Run frontend: `cd /home/abo/workspace/home/nexorious/frontend && npm run dev`

**Step 2: Test the feature**

1. Navigate to http://localhost:3000/games
2. Verify the "Select all" checkbox appears in the bulk actions bar
3. Click the checkbox - should select all visible games, label changes to "All X visible selected"
4. Click again - should select all games in collection, label changes to "All Y in library selected"
5. Click again - should clear selection
6. Apply a filter, verify selection resets
7. With filter active, test the cycle again
8. Manually select a game, verify checkbox shows indeterminate state
9. Test bulk actions (status change, delete) work with selection

**Step 3: Run full test suite**

Run backend: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest`
Run frontend: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`

Expected: All tests pass

---

### Task 7: Final Verification and Cleanup

**Step 1: Run all checks**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
cd /home/abo/workspace/home/nexorious/backend && uv run ruff check .
cd /home/abo/workspace/home/nexorious/backend && uv run pytest
cd /home/abo/workspace/home/nexorious/frontend && npm run check
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

**Step 2: Final commit if any fixes needed**

Only if there were fixes required during testing.
