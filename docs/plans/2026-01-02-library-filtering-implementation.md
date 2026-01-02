# Library Filtering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add multi-select Storefront, Genre, and Tags filters to the Library page with URL-driven state management.

**Architecture:** Backend accepts multi-value query params with OR logic within each filter type, AND logic between types. Frontend uses URL params as single source of truth via Next.js `useSearchParams`. New `MultiSelectFilter` component handles checkbox dropdown UX.

**Tech Stack:** FastAPI (Python), Next.js 16, React 19, TanStack Query, TypeScript, Tailwind CSS, shadcn/ui

---

## Task 1: Backend - Add genres endpoint

**Files:**
- Modify: `backend/app/api/user_games.py:332-469` (add endpoint before stats)
- Modify: `backend/app/tests/test_integration_user_games.py` (add tests)

**Step 1: Write the failing test**

Add to `backend/app/tests/test_integration_user_games.py`:

```python
class TestUserGameGenres:
    """Tests for GET /user-games/genres endpoint."""

    def test_get_genres_returns_unique_parsed_genres(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test that genres endpoint returns unique parsed genres from collection."""
        response = client_with_mock_igdb.get("/api/user-games/genres", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert "genres" in data
        assert isinstance(data["genres"], list)
        # Genres should be sorted alphabetically
        assert data["genres"] == sorted(data["genres"])

    def test_get_genres_empty_collection(
        self, client: TestClient, auth_headers
    ):
        """Test genres endpoint with empty collection."""
        response = client.get("/api/user-games/genres", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert data["genres"] == []

    def test_get_genres_parses_comma_separated(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test that comma-separated genres are parsed into individual values."""
        response = client_with_mock_igdb.get("/api/user-games/genres", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        # Should not contain comma-separated strings
        for genre in data["genres"]:
            assert "," not in genre

    def test_get_genres_requires_auth(self, client: TestClient):
        """Test that genres endpoint requires authentication."""
        response = client.get("/api/user-games/genres")
        assert response.status_code == 401
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run pytest app/tests/test_integration_user_games.py::TestUserGameGenres -v`
Expected: FAIL with "404 Not Found" (endpoint doesn't exist)

**Step 3: Write minimal implementation**

Add to `backend/app/api/user_games.py` before the `get_collection_stats` endpoint (around line 332):

```python
@router.get("/genres", response_model=dict)
async def get_user_game_genres(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get unique genres from user's game collection."""

    # Query all genre strings from user's games
    genre_query = (
        select(col(Game.genre))
        .join(UserGame)
        .where(UserGame.user_id == current_user.id)
        .where(is_not(col(Game.genre), None))
        .distinct()
    )

    genre_strings = session.exec(genre_query).all()

    # Parse comma-separated genres into unique set
    unique_genres: set[str] = set()
    for genre_string in genre_strings:
        if genre_string:
            # Split by comma and strip whitespace
            for genre in genre_string.split(","):
                genre = genre.strip()
                if genre:
                    unique_genres.add(genre)

    # Return sorted list
    return {"genres": sorted(unique_genres)}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run pytest app/tests/test_integration_user_games.py::TestUserGameGenres -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add backend/app/api/user_games.py backend/app/tests/test_integration_user_games.py && git commit -m "feat(backend): add GET /user-games/genres endpoint"
```

---

## Task 2: Backend - Add multi-value filter support to list endpoint

**Files:**
- Modify: `backend/app/api/user_games.py:103-260` (list_user_games function)
- Modify: `backend/app/tests/test_integration_user_games.py` (add tests)

**Step 1: Write the failing test**

Add to `backend/app/tests/test_integration_user_games.py`:

```python
class TestMultiValueFilters:
    """Tests for multi-value filter parameters."""

    def test_filter_multiple_platforms(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test filtering by multiple platforms with OR logic."""
        # Filter by two platforms
        response = client_with_mock_igdb.get(
            "/api/user-games/?platform=windows&platform=playstation_5",
            headers=auth_headers
        )
        assert response.status_code == 200

    def test_filter_multiple_storefronts(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test filtering by multiple storefronts with OR logic."""
        response = client_with_mock_igdb.get(
            "/api/user-games/?storefront=steam&storefront=epic",
            headers=auth_headers
        )
        assert response.status_code == 200

    def test_filter_multiple_genres(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test filtering by multiple genres with OR logic."""
        response = client_with_mock_igdb.get(
            "/api/user-games/?genre=RPG&genre=Action",
            headers=auth_headers
        )
        assert response.status_code == 200

    def test_filter_multiple_tags(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test filtering by multiple tag IDs with OR logic."""
        response = client_with_mock_igdb.get(
            "/api/user-games/?tag=fake-tag-id-1&tag=fake-tag-id-2",
            headers=auth_headers
        )
        assert response.status_code == 200

    def test_combined_multi_value_filters(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test AND logic between different filter types."""
        response = client_with_mock_igdb.get(
            "/api/user-games/?platform=windows&genre=RPG&genre=Action",
            headers=auth_headers
        )
        assert response.status_code == 200
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run pytest app/tests/test_integration_user_games.py::TestMultiValueFilters -v`
Expected: FAIL (multi-value params not supported, genre/tag params don't exist)

**Step 3: Write minimal implementation**

Modify `backend/app/api/user_games.py` - update the `list_user_games` function signature and body:

```python
@router.get("/", response_model=UserGameListResponse)
async def list_user_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page"),
    limit: Optional[int] = Query(default=None, ge=1, le=100, description="Items per page (alias for per_page)"),
    play_status: Optional[PlayStatus] = Query(default=None, description="Filter by play status"),
    ownership_status: Optional[OwnershipStatus] = Query(default=None, description="Filter by ownership status"),
    is_loved: Optional[bool] = Query(default=None, description="Filter by loved status"),
    platform: Optional[List[str]] = Query(default=None, description="Filter by platform(s)"),
    storefront: Optional[List[str]] = Query(default=None, description="Filter by storefront(s)"),
    genre: Optional[List[str]] = Query(default=None, description="Filter by genre(s)"),
    tag: Optional[List[str]] = Query(default=None, description="Filter by tag ID(s)"),
    rating_min: Optional[float] = Query(default=None, ge=1, le=5, description="Minimum rating filter"),
    rating_max: Optional[float] = Query(default=None, ge=1, le=5, description="Maximum rating filter"),
    has_notes: Optional[bool] = Query(default=None, description="Filter by presence of notes"),
    q: Optional[str] = Query(default=None, description="Search in game titles and notes"),
    fuzzy_threshold: Optional[float] = Query(default=None, ge=0.0, le=1.0, description="Fuzzy matching threshold for title search (0.0-1.0)"),
    sort_by: Optional[str] = Query(default="title", description="Sort field"),
    sort_order: Optional[str] = Query(default="asc", pattern="^(asc|desc)$", description="Sort order")
):
    """List user's game collection with filtering and sorting."""

    # Handle limit parameter as alias for per_page
    if limit is not None:
        per_page = limit

    # Build base query
    query = select(UserGame).where(UserGame.user_id == current_user.id)

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

    # Platform filter (multi-value OR logic)
    if platform:
        query = query.join(UserGamePlatform).where(in_(col(UserGamePlatform.platform), platform))

    # Storefront filter (multi-value OR logic)
    if storefront:
        # Need to handle case where platform join already happened
        if not platform:
            query = query.join(UserGamePlatform)
        query = query.where(in_(col(UserGamePlatform.storefront), storefront))

    # Genre filter (multi-value OR logic with ILIKE)
    if genre:
        genre_conditions = [col(Game.genre).icontains(g) for g in genre]
        query = query.join(Game).where(or_(*genre_conditions))

    # Tag filter (multi-value OR logic with EXISTS)
    if tag:
        from ..models.tag import UserGameTag
        tag_subquery = (
            select(UserGameTag.user_game_id)
            .where(in_(col(UserGameTag.tag_id), tag))
        )
        filters.append(in_(col(UserGame.id), tag_subquery))

    # Apply filters
    if filters:
        query = query.where(and_(*filters))

    # ... rest of the function remains the same (search, sorting, pagination)
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run pytest app/tests/test_integration_user_games.py::TestMultiValueFilters -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add backend/app/api/user_games.py backend/app/tests/test_integration_user_games.py && git commit -m "feat(backend): add multi-value filter support for platform, storefront, genre, tag"
```

---

## Task 3: Backend - Update /ids endpoint with same filters

**Files:**
- Modify: `backend/app/api/user_games.py:263-329` (get_user_game_ids function)
- Modify: `backend/app/tests/test_integration_user_games.py` (add tests)

**Step 1: Write the failing test**

Add to `backend/app/tests/test_integration_user_games.py`:

```python
class TestUserGameIdsMultiValueFilters:
    """Tests for multi-value filters on /user-games/ids endpoint."""

    def test_ids_filter_multiple_genres(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test /ids endpoint with multiple genre filters."""
        response = client_with_mock_igdb.get(
            "/api/user-games/ids?genre=RPG&genre=Action",
            headers=auth_headers
        )
        assert response.status_code == 200
        data = response.json()
        assert "ids" in data
        assert isinstance(data["ids"], list)

    def test_ids_filter_multiple_tags(
        self, client_with_mock_igdb: TestClient, auth_headers
    ):
        """Test /ids endpoint with multiple tag filters."""
        response = client_with_mock_igdb.get(
            "/api/user-games/ids?tag=fake-id-1&tag=fake-id-2",
            headers=auth_headers
        )
        assert response.status_code == 200
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run pytest app/tests/test_integration_user_games.py::TestUserGameIdsMultiValueFilters -v`
Expected: FAIL (genre/tag params not supported on /ids)

**Step 3: Write minimal implementation**

Update `get_user_game_ids` function signature in `backend/app/api/user_games.py`:

```python
@router.get("/ids", response_model=UserGameIdsResponse)
async def get_user_game_ids(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    play_status: Optional[PlayStatus] = Query(default=None, description="Filter by play status"),
    ownership_status: Optional[OwnershipStatus] = Query(default=None, description="Filter by ownership status"),
    is_loved: Optional[bool] = Query(default=None, description="Filter by loved status"),
    platform: Optional[List[str]] = Query(default=None, description="Filter by platform(s)"),
    storefront: Optional[List[str]] = Query(default=None, description="Filter by storefront(s)"),
    genre: Optional[List[str]] = Query(default=None, description="Filter by genre(s)"),
    tag: Optional[List[str]] = Query(default=None, description="Filter by tag ID(s)"),
    rating_min: Optional[float] = Query(default=None, ge=1, le=5, description="Minimum rating filter"),
    rating_max: Optional[float] = Query(default=None, ge=1, le=5, description="Maximum rating filter"),
    has_notes: Optional[bool] = Query(default=None, description="Filter by presence of notes"),
    q: Optional[str] = Query(default=None, description="Search in game titles and notes"),
):
    """Get all user game IDs matching filters (lightweight endpoint for bulk selection)."""

    # Build base query - only select IDs
    query = select(UserGame.id).where(UserGame.user_id == current_user.id)

    # Apply filters (same logic as list_user_games)
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

    # Platform filter (multi-value OR logic)
    if platform:
        query = query.join(UserGamePlatform).where(in_(col(UserGamePlatform.platform), platform))

    # Storefront filter (multi-value OR logic)
    if storefront:
        if not platform:
            query = query.join(UserGamePlatform)
        query = query.where(in_(col(UserGamePlatform.storefront), storefront))

    # Genre filter (multi-value OR logic with ILIKE)
    if genre:
        genre_conditions = [col(Game.genre).icontains(g) for g in genre]
        query = query.join(Game).where(or_(*genre_conditions))

    # Tag filter (multi-value OR logic with EXISTS)
    if tag:
        from ..models.tag import UserGameTag
        tag_subquery = (
            select(UserGameTag.user_game_id)
            .where(in_(col(UserGameTag.tag_id), tag))
        )
        filters.append(in_(col(UserGame.id), tag_subquery))

    if filters:
        query = query.where(and_(*filters))

    if q:
        # Need Game join for search
        if not genre:
            query = query.join(Game)
        query = query.where(or_(
            col(Game.title).icontains(q),
            and_(is_not(col(UserGame.personal_notes), None), col(UserGame.personal_notes).icontains(q))
        ))

    # Execute and return IDs only
    ids = session.exec(query).all()

    return UserGameIdsResponse(ids=[str(id) for id in ids])
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run pytest app/tests/test_integration_user_games.py::TestUserGameIdsMultiValueFilters -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add backend/app/api/user_games.py backend/app/tests/test_integration_user_games.py && git commit -m "feat(backend): add multi-value filter support to /user-games/ids endpoint"
```

---

## Task 4: Frontend - Create MultiSelectFilter component

**Files:**
- Create: `frontend/src/components/ui/multi-select-filter.tsx`
- Create: `frontend/src/components/ui/multi-select-filter.test.tsx`

**Step 1: Write the failing test**

Create `frontend/src/components/ui/multi-select-filter.test.tsx`:

```typescript
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { MultiSelectFilter } from './multi-select-filter';

describe('MultiSelectFilter', () => {
  const defaultProps = {
    label: 'Genres',
    options: [
      { value: 'action', label: 'Action' },
      { value: 'rpg', label: 'RPG' },
      { value: 'adventure', label: 'Adventure' },
    ],
    selected: [] as string[],
    onChange: vi.fn(),
  };

  it('renders the label', () => {
    render(<MultiSelectFilter {...defaultProps} />);
    expect(screen.getByText('Genres')).toBeInTheDocument();
  });

  it('shows count badge when items are selected', () => {
    render(<MultiSelectFilter {...defaultProps} selected={['action', 'rpg']} />);
    expect(screen.getByText('Genres (2)')).toBeInTheDocument();
  });

  it('opens dropdown on click', () => {
    render(<MultiSelectFilter {...defaultProps} />);
    fireEvent.click(screen.getByRole('button'));
    expect(screen.getByRole('listbox')).toBeInTheDocument();
  });

  it('shows checkboxes for each option', () => {
    render(<MultiSelectFilter {...defaultProps} />);
    fireEvent.click(screen.getByRole('button'));
    expect(screen.getByLabelText('Action')).toBeInTheDocument();
    expect(screen.getByLabelText('RPG')).toBeInTheDocument();
    expect(screen.getByLabelText('Adventure')).toBeInTheDocument();
  });

  it('calls onChange when option is toggled', () => {
    const onChange = vi.fn();
    render(<MultiSelectFilter {...defaultProps} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button'));
    fireEvent.click(screen.getByLabelText('Action'));
    expect(onChange).toHaveBeenCalledWith(['action']);
  });

  it('removes value from selection when unchecked', () => {
    const onChange = vi.fn();
    render(
      <MultiSelectFilter
        {...defaultProps}
        selected={['action', 'rpg']}
        onChange={onChange}
      />
    );
    fireEvent.click(screen.getByRole('button'));
    fireEvent.click(screen.getByLabelText('Action'));
    expect(onChange).toHaveBeenCalledWith(['rpg']);
  });

  it('shows disabled state', () => {
    render(<MultiSelectFilter {...defaultProps} disabled />);
    expect(screen.getByRole('button')).toBeDisabled();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run test -- src/components/ui/multi-select-filter.test.tsx`
Expected: FAIL (component doesn't exist)

**Step 3: Write minimal implementation**

Create `frontend/src/components/ui/multi-select-filter.tsx`:

```typescript
'use client';

import { useState, useRef, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

export interface MultiSelectOption {
  value: string;
  label: string;
}

export interface MultiSelectFilterProps {
  label: string;
  options: MultiSelectOption[];
  selected: string[];
  onChange: (selected: string[]) => void;
  disabled?: boolean;
  className?: string;
}

export function MultiSelectFilter({
  label,
  options,
  selected,
  onChange,
  disabled = false,
  className,
}: MultiSelectFilterProps) {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(event.target as Node)
      ) {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Close on escape
  useEffect(() => {
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setIsOpen(false);
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, []);

  const handleToggle = (value: string) => {
    if (selected.includes(value)) {
      onChange(selected.filter((v) => v !== value));
    } else {
      onChange([...selected, value]);
    }
  };

  const displayLabel = selected.length > 0 ? `${label} (${selected.length})` : label;

  return (
    <div ref={containerRef} className={cn('relative', className)}>
      <Button
        variant="outline"
        role="button"
        onClick={() => setIsOpen(!isOpen)}
        disabled={disabled}
        className="w-40 justify-between"
      >
        <span className="truncate">{displayLabel}</span>
        <ChevronDown className={cn('h-4 w-4 transition-transform', isOpen && 'rotate-180')} />
      </Button>

      {isOpen && (
        <div
          role="listbox"
          className="absolute z-50 mt-1 w-56 rounded-md border bg-popover p-2 shadow-md"
        >
          <div className="max-h-60 overflow-y-auto space-y-1">
            {options.length === 0 ? (
              <div className="px-2 py-1.5 text-sm text-muted-foreground">
                No options available
              </div>
            ) : (
              options.map((option) => (
                <label
                  key={option.value}
                  className="flex items-center gap-2 px-2 py-1.5 rounded-sm hover:bg-accent cursor-pointer"
                >
                  <Checkbox
                    checked={selected.includes(option.value)}
                    onCheckedChange={() => handleToggle(option.value)}
                    aria-label={option.label}
                  />
                  <span className="text-sm">{option.label}</span>
                </label>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run test -- src/components/ui/multi-select-filter.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add frontend/src/components/ui/multi-select-filter.tsx frontend/src/components/ui/multi-select-filter.test.tsx && git commit -m "feat(frontend): create MultiSelectFilter component"
```

---

## Task 5: Frontend - Add useUserGameGenres hook

**Files:**
- Modify: `frontend/src/api/games.ts` (add API function)
- Modify: `frontend/src/hooks/use-games.ts` (add hook)
- Modify: `frontend/src/api/games.test.ts` (add tests)
- Modify: `frontend/src/hooks/use-games.test.tsx` (add tests)

**Step 1: Write the failing test**

Add to `frontend/src/api/games.test.ts`:

```typescript
describe('getUserGameGenres', () => {
  it('fetches unique genres from user collection', async () => {
    const genres = await gamesApi.getUserGameGenres();
    expect(Array.isArray(genres)).toBe(true);
  });
});
```

Add to `frontend/src/hooks/use-games.test.tsx`:

```typescript
describe('useUserGameGenres', () => {
  it('returns genres array', async () => {
    const { result } = renderHook(() => useUserGameGenres(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(Array.isArray(result.current.data)).toBe(true);
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run test -- -t "getUserGameGenres"`
Expected: FAIL (function doesn't exist)

**Step 3: Write minimal implementation**

Add to `frontend/src/api/games.ts`:

```typescript
/**
 * Get unique genres from user's game collection.
 */
export async function getUserGameGenres(): Promise<string[]> {
  const response = await api.get<{ genres: string[] }>('/user-games/genres');
  return response.genres;
}
```

Add to `frontend/src/hooks/use-games.ts`:

```typescript
/**
 * Hook to fetch unique genres from user's game collection.
 */
export function useUserGameGenres() {
  return useQuery<string[], Error>({
    queryKey: [...gameKeys.all, 'genres'] as const,
    queryFn: () => gamesApi.getUserGameGenres(),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}
```

Don't forget to export from `frontend/src/hooks/index.ts`.

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run test -- -t "getUserGameGenres"`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add frontend/src/api/games.ts frontend/src/api/games.test.ts frontend/src/hooks/use-games.ts frontend/src/hooks/use-games.test.tsx frontend/src/hooks/index.ts && git commit -m "feat(frontend): add useUserGameGenres hook and API function"
```

---

## Task 6: Frontend - Update API service for multi-value params

**Files:**
- Modify: `frontend/src/api/games.ts` (update GetUserGamesParams and buildUserGamesQueryParams)
- Modify: `frontend/src/api/games.test.ts` (add tests)

**Step 1: Write the failing test**

Add to `frontend/src/api/games.test.ts`:

```typescript
describe('buildUserGamesQueryParams with arrays', () => {
  it('handles multiple platform values', async () => {
    // This test validates that the API accepts array params
    const result = await gamesApi.getUserGames({
      platform: ['windows', 'playstation_5'],
    });
    expect(result).toBeDefined();
  });

  it('handles multiple genre values', async () => {
    const result = await gamesApi.getUserGames({
      genre: ['RPG', 'Action'],
    });
    expect(result).toBeDefined();
  });

  it('handles multiple tag values', async () => {
    const result = await gamesApi.getUserGames({
      tags: ['tag-id-1', 'tag-id-2'],
    });
    expect(result).toBeDefined();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run test -- -t "buildUserGamesQueryParams with arrays"`
Expected: FAIL (array params not supported)

**Step 3: Write minimal implementation**

Update `frontend/src/api/games.ts`:

```typescript
export interface GetUserGamesParams {
  status?: PlayStatus;
  ownershipStatus?: OwnershipStatus;
  platform?: string | string[];      // Changed to support array
  storefront?: string | string[];    // Changed to support array
  genre?: string[];                  // New
  tags?: string[];                   // New
  search?: string;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
  page?: number;
  perPage?: number;
  limit?: number;
  isLoved?: boolean;
  ratingMin?: number;
  ratingMax?: number;
  hasNotes?: boolean;
  fuzzyThreshold?: number;
}

function buildUserGamesQueryParams(
  params?: GetUserGamesParams
): URLSearchParams {
  const searchParams = new URLSearchParams();
  if (!params) return searchParams;

  // Single-value params
  if (params.status) searchParams.append('play_status', params.status);
  if (params.ownershipStatus) searchParams.append('ownership_status', params.ownershipStatus);
  if (params.search) searchParams.append('q', params.search);
  if (params.sortBy) searchParams.append('sort_by', params.sortBy);
  if (params.sortOrder) searchParams.append('sort_order', params.sortOrder);
  if (params.page) searchParams.append('page', String(params.page));
  if (params.perPage) searchParams.append('per_page', String(params.perPage));
  if (params.limit) searchParams.append('limit', String(params.limit));
  if (params.isLoved !== undefined) searchParams.append('is_loved', String(params.isLoved));
  if (params.ratingMin) searchParams.append('rating_min', String(params.ratingMin));
  if (params.ratingMax) searchParams.append('rating_max', String(params.ratingMax));
  if (params.hasNotes !== undefined) searchParams.append('has_notes', String(params.hasNotes));
  if (params.fuzzyThreshold) searchParams.append('fuzzy_threshold', String(params.fuzzyThreshold));

  // Multi-value params (append multiple times for same key)
  const platforms = Array.isArray(params.platform) ? params.platform : params.platform ? [params.platform] : [];
  platforms.forEach((p) => searchParams.append('platform', p));

  const storefronts = Array.isArray(params.storefront) ? params.storefront : params.storefront ? [params.storefront] : [];
  storefronts.forEach((s) => searchParams.append('storefront', s));

  params.genre?.forEach((g) => searchParams.append('genre', g));
  params.tags?.forEach((t) => searchParams.append('tag', t));

  return searchParams;
}
```

Update `getUserGames` and `getUserGameIds` to use the new URLSearchParams:

```typescript
export async function getUserGames(
  params?: GetUserGamesParams
): Promise<UserGamesListResponse> {
  const queryParams = buildUserGamesQueryParams(params);
  const response = await api.get<UserGameListApiResponse>('/user-games/', {
    params: Object.fromEntries(queryParams),
  });
  // ... transform response
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run test -- -t "buildUserGamesQueryParams with arrays"`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add frontend/src/api/games.ts frontend/src/api/games.test.ts && git commit -m "feat(frontend): update API service to support multi-value filter params"
```

---

## Task 7: Frontend - Update GameFilters component

**Files:**
- Modify: `frontend/src/components/games/game-filters.tsx`
- Modify: `frontend/src/components/games/game-filters.test.tsx` (if exists, otherwise create)

**Step 1: Update GameFiltersProps interface**

Update `frontend/src/components/games/game-filters.tsx`:

```typescript
export interface GameFiltersProps {
  filters: {
    search: string;
    status?: PlayStatus;
    platformId?: string;           // Keep for backwards compat
    platforms?: string[];          // New: multi-select
    storefronts?: string[];        // New
    genres?: string[];             // New
    tags?: string[];               // New
  };
  onFiltersChange: (filters: GameFiltersProps['filters']) => void;
  viewMode: 'grid' | 'list';
  onViewModeChange: (mode: 'grid' | 'list') => void;
  sortBy: SortField;
  sortOrder: SortOrder;
  onSortByChange: (sortBy: SortField) => void;
  onSortOrderToggle: () => void;
}
```

**Step 2: Add the new filter dropdowns**

Import and use MultiSelectFilter, useAllStorefronts, useUserGameGenres, useAllTags hooks.

```typescript
import { MultiSelectFilter } from '@/components/ui/multi-select-filter';
import { useAllPlatforms, useAllStorefronts, useUserGameGenres } from '@/hooks';
import { useAllTags } from '@/hooks/use-tags';

export function GameFilters({
  filters,
  onFiltersChange,
  // ...other props
}: GameFiltersProps) {
  const { data: platforms } = useAllPlatforms();
  const { data: storefronts } = useAllStorefronts();
  const { data: genres } = useUserGameGenres();
  const { data: tags } = useAllTags();

  const hasActiveFilters =
    filters.search ||
    filters.status ||
    filters.platformId ||
    (filters.platforms && filters.platforms.length > 0) ||
    (filters.storefronts && filters.storefronts.length > 0) ||
    (filters.genres && filters.genres.length > 0) ||
    (filters.tags && filters.tags.length > 0);

  const clearFilters = () => {
    onFiltersChange({ search: '' });
  };

  // Convert platforms to options
  const platformOptions = platforms?.map((p) => ({
    value: p.name,
    label: p.display_name,
  })) ?? [];

  const storefrontOptions = storefronts?.map((s) => ({
    value: s.name,
    label: s.display_name,
  })) ?? [];

  const genreOptions = genres?.map((g) => ({
    value: g,
    label: g,
  })) ?? [];

  const tagOptions = tags?.map((t) => ({
    value: t.id,
    label: t.name,
  })) ?? [];

  return (
    <div className="flex flex-wrap gap-4 items-center">
      {/* Search */}
      <Input ... />

      {/* Status filter */}
      <Select ... />

      {/* Platform filter - convert to multi-select */}
      <MultiSelectFilter
        label="Platforms"
        options={platformOptions}
        selected={filters.platforms ?? []}
        onChange={(platforms) => onFiltersChange({ ...filters, platforms, platformId: undefined })}
        disabled={platformOptions.length === 0}
      />

      {/* Storefront filter - NEW */}
      <MultiSelectFilter
        label="Storefronts"
        options={storefrontOptions}
        selected={filters.storefronts ?? []}
        onChange={(storefronts) => onFiltersChange({ ...filters, storefronts })}
        disabled={storefrontOptions.length === 0}
      />

      {/* Genre filter - NEW */}
      <MultiSelectFilter
        label="Genres"
        options={genreOptions}
        selected={filters.genres ?? []}
        onChange={(genres) => onFiltersChange({ ...filters, genres })}
        disabled={genreOptions.length === 0}
      />

      {/* Tags filter - NEW */}
      <MultiSelectFilter
        label="Tags"
        options={tagOptions}
        selected={filters.tags ?? []}
        onChange={(tags) => onFiltersChange({ ...filters, tags })}
        disabled={tagOptions.length === 0}
      />

      {/* Sort controls... */}
      {/* Clear filters... */}
      {/* View toggle... */}
    </div>
  );
}
```

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run check && npm run test`
Expected: PASS

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add frontend/src/components/games/game-filters.tsx && git commit -m "feat(frontend): add storefront, genre, and tags multi-select filters to GameFilters"
```

---

## Task 8: Frontend - Update Games page with URL state management

**Files:**
- Modify: `frontend/src/app/(main)/games/page.tsx`

**Step 1: Replace useState with URL params**

```typescript
'use client';

import { useMemo, useCallback } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
// ... other imports

type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date';
type SortOrder = 'asc' | 'desc';

export default function GamesPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // Read filters from URL
  const filters = useMemo(() => ({
    search: searchParams.get('q') ?? '',
    status: searchParams.get('status') as PlayStatus | undefined,
    platforms: searchParams.getAll('platform'),
    storefronts: searchParams.getAll('storefront'),
    genres: searchParams.getAll('genre'),
    tags: searchParams.getAll('tag'),
  }), [searchParams]);

  const sortBy = (searchParams.get('sort') as SortField) ?? 'title';
  const sortOrder = (searchParams.get('order') as SortOrder) ?? 'asc';
  const viewMode = (searchParams.get('view') as 'grid' | 'list') ?? 'grid';

  // Update URL params helper
  const updateParams = useCallback((updates: Record<string, string | string[] | undefined>) => {
    const params = new URLSearchParams(searchParams.toString());

    Object.entries(updates).forEach(([key, value]) => {
      // Remove existing values for this key
      params.delete(key);

      if (value === undefined || value === '' || (Array.isArray(value) && value.length === 0)) {
        // Don't add empty values
        return;
      }

      if (Array.isArray(value)) {
        value.forEach((v) => params.append(key, v));
      } else {
        params.set(key, value);
      }
    });

    router.replace(`/games?${params.toString()}`, { scroll: false });
  }, [router, searchParams]);

  // Filter change handler
  const handleFiltersChange = useCallback((newFilters: typeof filters) => {
    updateParams({
      q: newFilters.search || undefined,
      status: newFilters.status,
      platform: newFilters.platforms,
      storefront: newFilters.storefronts,
      genre: newFilters.genres,
      tag: newFilters.tags,
    });
    // Clear selection when filters change
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, [updateParams]);

  const handleSortByChange = useCallback((newSortBy: SortField) => {
    const option = SORT_OPTIONS.find((o) => o.value === newSortBy);
    const newOrder = option?.defaultOrder ?? 'asc';
    updateParams({ sort: newSortBy, order: newOrder });
  }, [updateParams]);

  const handleSortOrderToggle = useCallback(() => {
    updateParams({ order: sortOrder === 'asc' ? 'desc' : 'asc' });
  }, [sortOrder, updateParams]);

  const handleViewModeChange = useCallback((mode: 'grid' | 'list') => {
    updateParams({ view: mode });
  }, [updateParams]);

  // Build query params for API
  const queryParams = useMemo(() => ({
    search: filters.search || undefined,
    status: filters.status,
    platform: filters.platforms.length > 0 ? filters.platforms : undefined,
    storefront: filters.storefronts.length > 0 ? filters.storefronts : undefined,
    genre: filters.genres.length > 0 ? filters.genres : undefined,
    tags: filters.tags.length > 0 ? filters.tags : undefined,
    perPage: 50,
    sortBy,
    sortOrder,
  }), [filters, sortBy, sortOrder]);

  // ... rest of component (selection state, data fetching, rendering)
}
```

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run check && npm run test`
Expected: PASS

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering && git add frontend/src/app/\\(main\\)/games/page.tsx && git commit -m "feat(frontend): implement URL-driven state management for library filters"
```

---

## Task 9: Run full test suite and type checks

**Step 1: Run backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run pytest -v`
Expected: All tests pass

**Step 2: Run frontend type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run check`
Expected: No errors

**Step 3: Run frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run test`
Expected: All tests pass

**Step 4: Commit any fixes**

If any tests fail, fix them and commit.

---

## Task 10: Manual testing and final commit

**Step 1: Start the application**

```bash
# Terminal 1: Backend
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/backend && uv run python -m app.main

# Terminal 2: Frontend
cd /home/abo/workspace/home/nexorious/.worktrees/library-filtering/frontend && npm run dev
```

**Step 2: Test the filters manually**

1. Navigate to /games
2. Test each filter individually
3. Test filter combinations
4. Verify URL updates correctly
5. Verify page refresh preserves filters
6. Verify "Clear Filters" resets URL
7. Test empty states (no tags, no storefronts)

**Step 3: Final commit if needed**

Fix any issues found during manual testing and commit.

---

## Implementation Checklist Summary

### Backend
- [x] Task 1: Add `GET /api/user-games/genres` endpoint
- [x] Task 2: Add multi-value filter support to `GET /api/user-games/`
- [x] Task 3: Add multi-value filter support to `GET /api/user-games/ids`

### Frontend
- [x] Task 4: Create `MultiSelectFilter` component
- [x] Task 5: Add `useUserGameGenres` hook
- [x] Task 6: Update API service for multi-value params
- [x] Task 7: Update `GameFilters` component with new filters
- [x] Task 8: Update Games page with URL state management

### Verification
- [x] Task 9: Run full test suite
- [x] Task 10: Manual testing
