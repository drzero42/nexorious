# IGDB ID Input Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow users to look up games directly by IGDB ID using `igdb:12345` format in search fields.

**Architecture:** Frontend detects `igdb:` prefix pattern, routes to a new backend endpoint that fetches a single game by ID. Same response format as search for seamless integration.

**Tech Stack:** FastAPI (backend), TanStack Query + React (frontend), pytest + Vitest (tests)

---

## Task 1: Backend - Add GET /games/igdb/{igdb_id} endpoint

**Files:**
- Modify: `backend/app/api/games.py` (add new endpoint after line 290)
- Modify: `backend/app/tests/test_integration_games.py` (add tests to TestIGDBIntegrationEndpoints class)

**Step 1: Write failing test for successful lookup**

Add to `backend/app/tests/test_integration_games.py` in the `TestIGDBIntegrationEndpoints` class:

```python
def test_igdb_lookup_by_id_success(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
    """Test successful IGDB lookup by ID."""
    response = client_with_mock_igdb.get("/api/games/igdb/12345", headers=auth_headers)

    assert_api_success(response, 200)
    data = response.json()
    assert "games" in data
    assert len(data["games"]) == 1
    assert data["games"][0]["igdb_id"] == 12345
    assert data["games"][0]["title"] == "Test Game"
    assert data["total"] == 1

def test_igdb_lookup_by_id_not_found(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
    """Test IGDB lookup by ID when game not found."""
    response = client_with_mock_igdb.get("/api/games/igdb/99999999", headers=auth_headers)

    assert_api_success(response, 200)
    data = response.json()
    assert data["games"] == []
    assert data["total"] == 0

def test_igdb_lookup_by_id_without_auth(self, client_with_mock_igdb: TestClient):
    """Test IGDB lookup by ID without authentication."""
    response = client_with_mock_igdb.get("/api/games/igdb/12345")

    assert_api_error(response, 403, "Not authenticated")
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/backend && uv run pytest app/tests/test_integration_games.py::TestIGDBIntegrationEndpoints::test_igdb_lookup_by_id_success -v`

Expected: FAIL with 404 (endpoint doesn't exist)

**Step 3: Implement the endpoint**

Add to `backend/app/api/games.py` after the `search_igdb` endpoint (around line 325):

```python
@router.get("/igdb/{igdb_id}", response_model=IGDBSearchResponse)
async def get_igdb_game_by_id(
    igdb_id: int,
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency),
):
    """Get a single game from IGDB by its ID.

    Returns the same response format as search for consistency.
    If the game is not found, returns an empty games list (not 404).
    """
    logger.info(
        f"IGDB lookup by ID from user {current_user.username}: igdb_id={igdb_id}"
    )

    try:
        game_metadata = await igdb_service.get_game_by_id(igdb_id)

        if game_metadata is None:
            logger.info(f"No game found in IGDB for ID {igdb_id}")
            return IGDBSearchResponse(games=[], total=0)

        # Use IGDB platform data if available, otherwise empty list
        platforms = game_metadata.platform_names if game_metadata.platform_names else []

        candidate = IGDBGameCandidate(
            igdb_id=game_metadata.igdb_id,
            igdb_slug=game_metadata.igdb_slug,
            title=game_metadata.title,
            release_date=parse_date_string(game_metadata.release_date),
            cover_art_url=game_metadata.cover_art_url,
            description=game_metadata.description,
            platforms=platforms,
            howlongtobeat_main=game_metadata.hastily,
            howlongtobeat_extra=game_metadata.normally,
            howlongtobeat_completionist=game_metadata.completely,
        )

        logger.info(f"Successfully fetched IGDB game {igdb_id}: {game_metadata.title}")
        return IGDBSearchResponse(games=[candidate], total=1)

    except IGDBNotConfiguredError as e:
        logger.warning(f"IGDB not configured during lookup for ID {igdb_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=str(e),
        )
    except TwitchAuthError as e:
        logger.error(f"Twitch authentication failed for IGDB lookup {igdb_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"IGDB authentication failed: {str(e)}",
        )
    except IGDBError as e:
        logger.error(f"IGDB API error during lookup for ID {igdb_id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail=f"IGDB API error: {str(e)}",
        )
    except Exception as e:
        logger.error(f"Unexpected error during IGDB lookup for ID {igdb_id}: {str(e)}", exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"An unexpected error occurred: {str(e)}",
        )
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/backend && uv run pytest app/tests/test_integration_games.py::TestIGDBIntegrationEndpoints::test_igdb_lookup_by_id_success app/tests/test_integration_games.py::TestIGDBIntegrationEndpoints::test_igdb_lookup_by_id_not_found app/tests/test_integration_games.py::TestIGDBIntegrationEndpoints::test_igdb_lookup_by_id_without_auth -v`

Expected: PASS

**Step 5: Run all IGDB integration tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/backend && uv run pytest app/tests/test_integration_games.py::TestIGDBIntegrationEndpoints -v`

Expected: All tests pass

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input
git add backend/app/api/games.py backend/app/tests/test_integration_games.py
git commit -m "feat(api): add GET /games/igdb/{igdb_id} endpoint for direct ID lookup"
```

---

## Task 2: Frontend - Add getGameByIGDBId API function

**Files:**
- Modify: `frontend/src/api/games.ts` (add new function after searchIGDB)
- Modify: `frontend/src/api/games.test.ts` (add tests)

**Step 1: Write failing test**

Add to `frontend/src/api/games.test.ts`:

```typescript
describe('getGameByIGDBId', () => {
  it('fetches game by IGDB ID successfully', async () => {
    server.use(
      http.get(`${API_URL}/games/igdb/12345`, () => {
        return HttpResponse.json({
          games: [mockIGDBGameApi],
          total: 1,
        });
      })
    );

    const result = await gamesApi.getGameByIGDBId(12345);

    expect(result).toHaveLength(1);
    expect(result[0].igdb_id).toBe(99999);
    expect(result[0].title).toBe('IGDB Game');
  });

  it('returns empty array when game not found', async () => {
    server.use(
      http.get(`${API_URL}/games/igdb/99999999`, () => {
        return HttpResponse.json({
          games: [],
          total: 0,
        });
      })
    );

    const result = await gamesApi.getGameByIGDBId(99999999);

    expect(result).toHaveLength(0);
  });
});
```

Also add import at top of test file:
```typescript
// In the imports, ensure getGameByIGDBId is imported
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/frontend && npm run test -- --run src/api/games.test.ts`

Expected: FAIL with "getGameByIGDBId is not a function"

**Step 3: Implement the function**

Add to `frontend/src/api/games.ts` after the `searchIGDB` function (around line 478):

```typescript
/**
 * Get a game from IGDB by its ID.
 * Returns the same format as searchIGDB for consistency.
 */
export async function getGameByIGDBId(igdbId: number): Promise<IGDBGameCandidate[]> {
  const response = await api.get<IGDBSearchApiResponse>(`/games/igdb/${igdbId}`);
  return response.games.map(transformIGDBGameCandidate);
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/frontend && npm run test -- --run src/api/games.test.ts`

Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input
git add frontend/src/api/games.ts frontend/src/api/games.test.ts
git commit -m "feat(api): add getGameByIGDBId function for direct ID lookup"
```

---

## Task 3: Frontend - Update useSearchIGDB hook with prefix detection

**Files:**
- Modify: `frontend/src/hooks/use-games.ts` (update useSearchIGDB hook)
- Modify: `frontend/src/hooks/use-games.test.tsx` (add tests for prefix detection)

**Step 1: Write failing tests**

Add to `frontend/src/hooks/use-games.test.tsx` in the `useSearchIGDB` describe block:

```typescript
it('detects igdb: prefix and calls lookup endpoint (lowercase)', async () => {
  server.use(
    http.get(`${API_URL}/games/igdb/12345`, () => {
      return HttpResponse.json({
        games: [mockIGDBGameApi],
        total: 1,
      });
    })
  );

  const { result } = renderHook(() => useSearchIGDB('igdb:12345'), {
    wrapper: QueryWrapper,
  });

  await waitFor(() => {
    expect(result.current.isSuccess).toBe(true);
  });

  expect(result.current.data).toHaveLength(1);
  expect(result.current.data?.[0].title).toBe('IGDB Game');
});

it('detects IGDB: prefix and calls lookup endpoint (uppercase)', async () => {
  server.use(
    http.get(`${API_URL}/games/igdb/99999`, () => {
      return HttpResponse.json({
        games: [mockIGDBGameApi],
        total: 1,
      });
    })
  );

  const { result } = renderHook(() => useSearchIGDB('IGDB:99999'), {
    wrapper: QueryWrapper,
  });

  await waitFor(() => {
    expect(result.current.isSuccess).toBe(true);
  });

  expect(result.current.data).toHaveLength(1);
});

it('does not apply 3-char minimum for IGDB ID lookup', async () => {
  server.use(
    http.get(`${API_URL}/games/igdb/1`, () => {
      return HttpResponse.json({
        games: [mockIGDBGameApi],
        total: 1,
      });
    })
  );

  const { result } = renderHook(() => useSearchIGDB('igdb:1'), {
    wrapper: QueryWrapper,
  });

  await waitFor(() => {
    expect(result.current.isSuccess).toBe(true);
  });

  expect(result.current.data).toHaveLength(1);
});

it('returns empty array when IGDB ID not found', async () => {
  server.use(
    http.get(`${API_URL}/games/igdb/99999999`, () => {
      return HttpResponse.json({
        games: [],
        total: 0,
      });
    })
  );

  const { result } = renderHook(() => useSearchIGDB('igdb:99999999'), {
    wrapper: QueryWrapper,
  });

  await waitFor(() => {
    expect(result.current.isSuccess).toBe(true);
  });

  expect(result.current.data).toHaveLength(0);
});

it('treats invalid igdb: format as regular search', async () => {
  const fetchSpy = vi.fn();

  server.use(
    http.post(`${API_URL}/games/search/igdb`, () => {
      fetchSpy();
      return HttpResponse.json({ games: [], total: 0 });
    })
  );

  // "igdb:abc" is not a valid ID format, should fall through to search
  const { result } = renderHook(() => useSearchIGDB('igdb:abc'), {
    wrapper: QueryWrapper,
  });

  await waitFor(() => {
    expect(result.current.isSuccess).toBe(true);
  });

  expect(fetchSpy).toHaveBeenCalled();
});
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/frontend && npm run test -- --run src/hooks/use-games.test.tsx`

Expected: FAIL (tests expecting GET /games/igdb/* will fail because hook still uses POST search)

**Step 3: Update the useSearchIGDB hook**

Replace the `useSearchIGDB` function in `frontend/src/hooks/use-games.ts`:

```typescript
/**
 * Regex to detect IGDB ID lookup format: igdb:12345 (case-insensitive)
 */
const IGDB_ID_PATTERN = /^igdb:(\d+)$/i;

/**
 * Parse IGDB ID from query if it matches the igdb:12345 format.
 * Returns the numeric ID or null if not a match.
 */
function parseIGDBIdFromQuery(query: string): number | null {
  const match = query.match(IGDB_ID_PATTERN);
  if (match) {
    return parseInt(match[1], 10);
  }
  return null;
}

/**
 * Hook to search IGDB for games.
 *
 * Supports two modes:
 * 1. Direct ID lookup: Use "igdb:12345" format (case-insensitive)
 * 2. Name search: Any other query (requires 3+ characters)
 */
export function useSearchIGDB(query: string, limit?: number) {
  const igdbId = parseIGDBIdFromQuery(query);
  const isIdLookup = igdbId !== null;

  return useQuery<IGDBGameCandidate[], Error>({
    queryKey: gameKeys.igdbSearch(query),
    queryFn: () => {
      if (isIdLookup) {
        return gamesApi.getGameByIGDBId(igdbId);
      }
      return gamesApi.searchIGDB(query, limit);
    },
    // ID lookup: always enabled (no min chars)
    // Name search: require 3+ characters
    enabled: isIdLookup || query.length >= 3,
  });
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/frontend && npm run test -- --run src/hooks/use-games.test.tsx`

Expected: PASS

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/frontend && npm run check`

Expected: No errors

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input
git add frontend/src/hooks/use-games.ts frontend/src/hooks/use-games.test.tsx
git commit -m "feat(hooks): add igdb: prefix detection to useSearchIGDB for direct ID lookup"
```

---

## Task 4: Final verification

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/backend && uv run pytest --ignore=app/tests/test_sync_process_item.py -q`

Expected: All tests pass (except pre-existing failures in test_epic_service.py)

**Step 2: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/frontend && npm run test`

Expected: All tests pass

**Step 3: Run frontend type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/igdb-id-input/frontend && npm run check`

Expected: No errors (only warnings)

**Step 4: Manual test (optional)**

If dev servers are running:
1. Go to `/games/add`
2. Type `igdb:1942` in the search box
3. Should see "The Legend of Zelda" or similar game result
4. Type `igdb:99999999999`
5. Should see "No games found" message

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Backend endpoint | `backend/app/api/games.py`, `backend/app/tests/test_integration_games.py` |
| 2 | Frontend API function | `frontend/src/api/games.ts`, `frontend/src/api/games.test.ts` |
| 3 | Hook prefix detection | `frontend/src/hooks/use-games.ts`, `frontend/src/hooks/use-games.test.tsx` |
| 4 | Final verification | All tests pass |
