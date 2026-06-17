# User-Game Tag-Assignment Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a replace-set tag-assignment endpoint for user-games (`PUT /api/user-games/:id/tags`), wire the existing edit-form tag editor to it, and remove the dead frontend client code left over from the never-built API.

**Architecture:** A dedicated sub-resource accepts the complete desired set of tag *names*; the handler resolves/creates each within the caller's tags and reconciles the `user_game_tags` join table in one transaction. The find-or-create + reconcile logic moves into a shared `internal/usergame` helper consumed by both the new handler and the two import workers, so all write paths agree. The frontend edit form maps its selected tag IDs to names on save and calls the new endpoint; the dead `assign`/`remove`/`create-or-get`/`bulk` client functions and hooks are deleted.

**Tech Stack:** Go 1.26, Echo v5, Bun ORM (pgx/pgdriver), testcontainers-go; React 19 + TypeScript, TanStack Query, Vitest.

## Global Constraints

- Go: standard conventions; errors returned/wrapped, never discarded (`errcheck` runs with `check-blank` — no `_ =` discards in non-test code). `gosec` enabled.
- Echo v5 handler signature: `func (h *Handler) X(c *echo.Context) error` — note `*echo.Context` (pointer).
- Echo v5 route order: register static routes before parameterised ones; place the new `/:id/tags` route alongside the existing `/:id/platforms` routes.
- `internal/usergame` helpers accept `bun.IDB` (run inside a caller's transaction), mirroring `RemoveFromPoolsIfFinished` / `ClearWishlistOnAcquire`.
- Tag name validation mirrors `HandleCreateTag`: trim, reject empty, reject > 100 chars.
- `sql.ErrNoRows` is the not-found sentinel (Bun wraps pgx into it); `errors.Is(err, sql.ErrNoRows)` → 404.
- Frontend quality gates: `npm run check` (tsc + eslint, zero errors), `npm run knip` (zero unused), `npm run test` — all must pass. React Compiler bar: no warnings.
- After removing the unexported `findOrCreateTag` (a cross-package refactor), run `make deadcode` and reconcile new entries.

---

### Task 1: Shared tag reconcile helper + import-worker refactor

Extract the find-or-create logic into `internal/usergame` and add the replace-set reconcile, then point both import workers at it and delete the now-duplicated local `findOrCreateTag`.

**Files:**
- Create: `internal/usergame/tags.go`
- Modify: `internal/worker/tasks/import_item.go` (replace `findOrCreateTag` call at ~line 337, delete the func at ~line 388, add import)
- Modify: `internal/worker/tasks/import_pipeline.go` (replace `findOrCreateTag` call at ~line 280)

**Interfaces:**
- Produces:
  - `usergame.ResolveOrCreateTag(ctx context.Context, db bun.IDB, userID, name string, color *string) (string, error)` — returns the id of the caller's tag named `name` (case-insensitive), creating it with `color` if absent.
  - `usergame.ReplaceTags(ctx context.Context, db bun.IDB, userGameID, userID string, names []string) error` — resolves/creates each name (de-duped case-insensitively, trimmed; empty entries skipped) and reconciles `user_game_tags` for the game (insert missing, delete absent).

- [ ] **Step 1: Create the shared helper file**

Create `internal/usergame/tags.go`:

```go
package usergame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// ResolveOrCreateTag returns the id of the caller's tag named `name`, matching
// case-insensitively, creating the definition (with `color`) if it does not
// exist. Accepts bun.IDB so it runs inside a caller's transaction.
func ResolveOrCreateTag(ctx context.Context, db bun.IDB, userID, name string, color *string) (string, error) {
	var tag models.Tag
	err := db.NewSelect().Model(&tag).
		Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, name).
		Scan(ctx)
	if err == nil {
		return tag.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("select tag: %w", err)
	}

	now := time.Now().UTC()
	tag = models.Tag{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		Color:     color,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err = db.NewInsert().Model(&tag).Exec(ctx); err != nil {
		return "", fmt.Errorf("insert tag: %w", err)
	}
	return tag.ID, nil
}

// ReplaceTags sets the complete tag set on a user game to `names`. It resolves
// or creates each name within the caller's tags, then reconciles
// user_game_tags: inserting missing links and deleting links no longer present.
// Names are trimmed and de-duplicated case-insensitively; an empty slice clears
// all tags. Accepts bun.IDB so it runs inside a caller's transaction.
func ReplaceTags(ctx context.Context, db bun.IDB, userGameID, userID string, names []string) error {
	desired := map[string]bool{} // tag id -> wanted
	seen := map[string]bool{}     // lower(name) -> resolved
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		tagID, err := ResolveOrCreateTag(ctx, db, userID, name, nil)
		if err != nil {
			return err
		}
		desired[tagID] = true
	}

	var existing []models.UserGameTag
	if err := db.NewSelect().Model(&existing).
		Where("user_game_id = ?", userGameID).Scan(ctx); err != nil {
		return fmt.Errorf("select existing tags: %w", err)
	}
	existingIDs := map[string]bool{}
	for _, ugt := range existing {
		existingIDs[ugt.TagID] = true
	}

	var toDelete []string
	for id := range existingIDs {
		if !desired[id] {
			toDelete = append(toDelete, id)
		}
	}
	if len(toDelete) > 0 {
		if _, err := db.NewDelete().Model((*models.UserGameTag)(nil)).
			Where("user_game_id = ?", userGameID).
			Where("tag_id IN (?)", bun.In(toDelete)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete tag links: %w", err)
		}
	}

	now := time.Now().UTC()
	for id := range desired {
		if existingIDs[id] {
			continue
		}
		ugt := &models.UserGameTag{
			ID:         uuid.NewString(),
			UserGameID: userGameID,
			TagID:      id,
			CreatedAt:  now,
		}
		if _, err := db.NewInsert().Model(ugt).Exec(ctx); err != nil {
			return fmt.Errorf("insert tag link: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 2: Point `import_item.go` at the shared helper**

In `internal/worker/tasks/import_item.go`, change the call (currently `findOrCreateTag(ctx, w.DB, item.UserID, td.Name, td.Color)`):

```go
		tagID, err := usergame.ResolveOrCreateTag(ctx, w.DB, item.UserID, td.Name, td.Color)
```

Delete the entire local `func findOrCreateTag(...)` definition (~lines 388-415). Ensure the file imports `"github.com/drzero42/nexorious/internal/usergame"` (add it if not already present). After deleting `findOrCreateTag`, the imports `database/sql`, `errors`, and `uuid` may become unused **only if** nothing else in the file uses them — verify with the build in Step 5 and remove any now-unused import the compiler flags.

- [ ] **Step 3: Point `import_pipeline.go` at the shared helper**

In `internal/worker/tasks/import_pipeline.go`, change the call (currently `findOrCreateTag(ctx, w.DB, item.UserID, name, nil)`):

```go
		tagID, terr := usergame.ResolveOrCreateTag(ctx, w.DB, item.UserID, name, nil)
```

This file already imports `internal/usergame` (it calls `usergame.ClearWishlistOnAcquire`), so no import change is needed.

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: builds clean. Fix any unused-import errors the compiler reports in `import_item.go`.

- [ ] **Step 5: Run the import-worker tests (behavior unchanged)**

Run: `go test ./internal/worker/tasks/... -run Tag -v` then `go test ./internal/worker/tasks/...`
Expected: PASS. The worker tag-merge behavior is unchanged — `ResolveOrCreateTag` is a verbatim move of `findOrCreateTag`.

- [ ] **Step 6: Dead-code check**

Run: `make deadcode`
Expected: no *new* entries attributable to this change (the old `findOrCreateTag` is gone; `ResolveOrCreateTag`/`ReplaceTags` are reachable from the workers / the Task 2 handler). `ReplaceTags` will appear unreachable until Task 2 lands — note it and confirm it disappears after Task 2; do not delete it.

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/tags.go internal/worker/tasks/import_item.go internal/worker/tasks/import_pipeline.go
git commit -m "refactor: extract shared tag find-or-create into internal/usergame"
```

---

### Task 2: Backend replace-set endpoint

Add `PUT /api/user-games/:id/tags`, TDD.

**Files:**
- Create: `internal/api/user_game_tags_test.go`
- Modify: `internal/api/user_games.go` (add `replaceTagsRequest` type + `HandleReplaceTags`)
- Modify: `internal/api/router.go` (register the route near the `/:id/platforms` routes, ~line 377-381)

**Interfaces:**
- Consumes: `usergame.ReplaceTags` (Task 1); existing `toUserGameWithPlatformsResponse`, `models.UserGame`, auth/test helpers.
- Produces: `HandleReplaceTags(c *echo.Context) error` on `*UserGamesHandler`; route `PUT /api/user-games/:id/tags`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/user_game_tags_test.go`:

```go
package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReplaceUserGameTags(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "rtags")

	countLinks := func(t *testing.T, ugID string) int {
		t.Helper()
		n, err := testDB.NewSelect().Table("user_game_tags").
			Where("user_game_id = ?", ugID).Count(context.Background())
		if err != nil {
			t.Fatalf("count links: %v", err)
		}
		return n
	}
	newUG := func(t *testing.T, id, title string) {
		t.Helper()
		gameID := insertTestGame(t, testDB, title)
		insertTestUserGame(t, testDB, id, userID, int(gameID))
	}

	t.Run("creates links from names, auto-creating tags", func(t *testing.T) {
		newUG(t, "ug-rt-1", "RT Game 1")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-1/tags",
			map[string]any{"tags": []string{"RPG", "Backlog"}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-1"); got != 2 {
			t.Fatalf("expected 2 links, got %d", got)
		}
		n, err := testDB.NewSelect().Table("tags").Where("user_id = ?", userID).Count(context.Background())
		if err != nil {
			t.Fatalf("count tags: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 tag definitions auto-created, got %d", n)
		}
	})

	t.Run("replace with subset removes surplus, keeps rest", func(t *testing.T) {
		newUG(t, "ug-rt-2", "RT Game 2")
		putJSONAuth(t, e, "/api/user-games/ug-rt-2/tags", map[string]any{"tags": []string{"A", "B", "C"}}, token)
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-2/tags", map[string]any{"tags": []string{"B"}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-2"); got != 1 {
			t.Fatalf("expected 1 link, got %d", got)
		}
	})

	t.Run("empty set clears all tags", func(t *testing.T) {
		newUG(t, "ug-rt-3", "RT Game 3")
		putJSONAuth(t, e, "/api/user-games/ug-rt-3/tags", map[string]any{"tags": []string{"X"}}, token)
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-3/tags", map[string]any{"tags": []string{}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-3"); got != 0 {
			t.Fatalf("expected 0 links, got %d", got)
		}
	})

	t.Run("existing tag reused case-insensitively", func(t *testing.T) {
		newUG(t, "ug-rt-4", "RT Game 4")
		insertTag(t, testDB, "tag-existing-rt", userID, "Shooter", nil)
		putJSONAuth(t, e, "/api/user-games/ug-rt-4/tags", map[string]any{"tags": []string{"shooter"}}, token)
		var tagID string
		if err := testDB.NewSelect().Table("user_game_tags").Column("tag_id").
			Where("user_game_id = ?", "ug-rt-4").Scan(context.Background(), &tagID); err != nil {
			t.Fatalf("scan tag_id: %v", err)
		}
		if tagID != "tag-existing-rt" {
			t.Fatalf("expected reuse of tag-existing-rt, got %q", tagID)
		}
	})

	t.Run("duplicate names de-duped", func(t *testing.T) {
		newUG(t, "ug-rt-5", "RT Game 5")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-5/tags",
			map[string]any{"tags": []string{"Dup", "dup", "DUP"}}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if got := countLinks(t, "ug-rt-5"); got != 1 {
			t.Fatalf("expected 1 link, got %d", got)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		newUG(t, "ug-rt-6", "RT Game 6")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-6/tags", map[string]any{"tags": []string{"  "}}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("name over 100 chars rejected", func(t *testing.T) {
		newUG(t, "ug-rt-7", "RT Game 7")
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-7/tags",
			map[string]any{"tags": []string{strings.Repeat("x", 101)}}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("unknown user game returns 404", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/does-not-exist/tags", map[string]any{"tags": []string{"A"}}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("cannot tag another user's game", func(t *testing.T) {
		otherID, _ := setupUserGamesUser(t, testDB, e, "rtags-other")
		gameID := insertTestGame(t, testDB, "RT Game Other")
		insertTestUserGame(t, testDB, "ug-rt-other", otherID, int(gameID))
		rec := putJSONAuth(t, e, "/api/user-games/ug-rt-other/tags", map[string]any{"tags": []string{"A"}}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/user-games/ug-rt-1/tags",
			bytes.NewReader([]byte(`{"tags":["A"]}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/ -run TestReplaceUserGameTags -v`
Expected: FAIL — the route is unregistered, so requests 404 (and the unauthenticated subtest may pass by accident); the link-count and reuse subtests fail.

- [ ] **Step 3: Add the handler**

In `internal/api/user_games.go`, add (near `HandleUpdateUserGame`):

```go
// replaceTagsRequest is the body for PUT /api/user-games/:id/tags. The tags
// slice is the complete desired set of tag names.
type replaceTagsRequest struct {
	Tags []string `json:"tags"`
}

// HandleReplaceTags handles PUT /api/user-games/:id/tags. It validates the
// supplied tag names, then within one transaction verifies ownership of the
// user game and reconciles its tag set via usergame.ReplaceTags (resolving or
// creating each name within the caller's own tags). An empty or absent "tags"
// clears all tags. Returns the updated user game with its Tags relation.
func (h *UserGamesHandler) HandleReplaceTags(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")

	var req replaceTagsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	names := make([]string, 0, len(req.Tags))
	for _, raw := range req.Tags {
		name := strings.TrimSpace(raw)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tag name cannot be empty")
		}
		if len(name) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, "tag name must be 100 characters or less")
		}
		names = append(names, name)
	}

	ctx := context.Background()

	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		exists, existsErr := tx.NewSelect().Model((*models.UserGame)(nil)).
			Where("id = ? AND user_id = ?", id, userID).Exists(ctx)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return sql.ErrNoRows
		}
		return usergame.ReplaceTags(ctx, tx, id, userID, names)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	var ug models.UserGame
	if err := h.db.NewSelect().Model(&ug).
		Where("user_game.id = ?", id).
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		Scan(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(ug))
}
```

(`strings`, `errors`, `database/sql`, `context`, `bun`, `usergame`, `models`, `auth` are already imported by `user_games.go`.)

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, add alongside the other `/:id/...` user-games routes (after the platform routes, ~line 381):

```go
		userGamesGroup.PUT("/:id/tags", ugh.HandleReplaceTags)
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/api/ -run TestReplaceUserGameTags -v`
Expected: PASS (all subtests).

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/router.go internal/api/user_game_tags_test.go
git commit -m "feat: add tag-assignment endpoint for user-games (#1054)"
```

---

### Task 3: Frontend — wire the edit form to the new endpoint and remove dead code

Add the client function + hook, rewire `game-edit-form.tsx`, update its test, and delete the orphaned tag-assignment client code. Done as one task so the `knip` gate stays green (removing the dead exports and dropping their last consumer must land together).

**Files:**
- Modify: `ui/frontend/src/api/tags.ts` (add `replaceUserGameTags`; delete `createOrGetTag`, `assignTagsToGame`, `removeTagsFromGame`, `bulkAssignTags`, `bulkRemoveTags` and their orphaned types)
- Modify: `ui/frontend/src/hooks/use-tags.ts` (add `useReplaceUserGameTags`; delete `useCreateOrGetTag`, `useAssignTagsToGame`, `useRemoveTagsFromGame`)
- Modify: `ui/frontend/src/hooks/index.ts` (swap the re-exports)
- Modify: `ui/frontend/src/components/games/game-edit-form.tsx`
- Modify: `ui/frontend/src/components/games/game-edit-form.test.tsx`

**Interfaces:**
- Consumes: backend `PUT /api/user-games/:id/tags` (Task 2); existing `useCreateTag`, `useAllTags`, `TagSelector`, `api.put`.
- Produces:
  - `tagsApi.replaceUserGameTags(userGameId: string, tags: string[]): Promise<UserGame>`
  - `useReplaceUserGameTags()` → mutation with variables `{ userGameId: string; tags: string[] }`, result `UserGame`.

- [ ] **Step 1: Add the client function and remove the dead ones**

In `ui/frontend/src/api/tags.ts`:

1. Change the import to include `UserGame`:
```ts
import type { Tag, UserGame } from '@/types';
```
2. Add the new function (place it near the other user-game tag functions):
```ts
/**
 * Replace the complete tag set on a user game with the given tag names.
 * The backend resolves or creates each name within the user's tags and
 * reconciles the join table, returning the updated user game.
 */
export async function replaceUserGameTags(userGameId: string, tags: string[]): Promise<UserGame> {
  return api.put<UserGame>(`/user-games/${userGameId}/tags`, { tags });
}
```
3. Delete these functions entirely: `createOrGetTag`, `assignTagsToGame`, `removeTagsFromGame`, `bulkAssignTags`, `bulkRemoveTags`.
4. Delete these now-orphaned types: `TagAssignApiResponse`, `TagRemoveApiResponse`, `TagCreateOrGetResponse`, `TagAssignResponse`, `TagRemoveResponse` (and any `TagAssignResponse`/`TagRemoveResponse` import references). Leave `getTags`, `getAllTags`, `getTag`, `createTag`, `updateTag`, `deleteTag`, the `GetTagsParams`/`TagCreateData`/`TagUpdateData`/`TagsListResponse` exports intact.

- [ ] **Step 2: Add the hook and remove the dead ones**

In `ui/frontend/src/hooks/use-tags.ts`:

1. Add `UserGame` to the type import:
```ts
import type { Tag, UserGame } from '@/types';
```
2. Add the hook (replacing the deleted `useAssignTagsToGame`/`useRemoveTagsFromGame`/`useCreateOrGetTag` block):
```ts
export function useReplaceUserGameTags() {
  const queryClient = useQueryClient();

  return useMutation<UserGame, Error, { userGameId: string; tags: string[] }>({
    mutationFn: ({ userGameId, tags }) => tagsApi.replaceUserGameTags(userGameId, tags),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}
```
3. Delete `useCreateOrGetTag`, `useAssignTagsToGame`, `useRemoveTagsFromGame`. Keep `useCreateTag`.

- [ ] **Step 3: Fix the hook re-exports**

In `ui/frontend/src/hooks/index.ts`, replace the tag-hook re-export block with:
```ts
// Tag hooks
export {
  tagKeys,
  useTags,
  useAllTags,
  useTag,
  useCreateTag,
  useUpdateTag,
  useDeleteTag,
  useReplaceUserGameTags,
} from './use-tags';
```

- [ ] **Step 4: Rewire the edit form**

In `ui/frontend/src/components/games/game-edit-form.tsx`:

1. In the `@/hooks` import block, remove `useAssignTagsToGame`, `useRemoveTagsFromGame`, `useCreateOrGetTag` and add `useReplaceUserGameTags` and `useCreateTag`:
```ts
import {
  useUpdateUserGame,
  useAddPlatformToUserGame,
  useRemovePlatformFromUserGame,
  useUpdatePlatformAssociation,
  useReplaceUserGameTags,
  useAllPlatforms,
  useAllTags,
  useCreateTag,
  useSyncConfig,
} from '@/hooks';
```
2. Replace the three hook instances:
```ts
  const replaceTags = useReplaceUserGameTags();
  const createTag = useCreateTag();
```
(delete the old `const assignTags = useAssignTagsToGame();`, `const removeTags = useRemoveTagsFromGame();`, `const createOrGetTag = useCreateOrGetTag();`).
3. In the `isSaving`/pending expression that referenced `assignTags.isPending || removeTags.isPending`, replace those two terms with `replaceTags.isPending`.
4. Delete the now-unused `originalTagIds` memo (the `const originalTagIds = useMemo(...)` line). Add a name lookup memo near the other derived state:
```ts
  const tagNameById = useMemo(() => new Map(tags.map((t) => [t.id, t.name])), [tags]);
```
5. Replace the save handler's tag section (the `// 3. Handle tag changes` block computing `tagsToAdd`/`tagsToRemove` and the two `if` calls) with:
```ts
      // 3. Replace the full tag set (resolved by name on the backend).
      const tagNames = selectedTagIds
        .map((id) => tagNameById.get(id))
        .filter((name): name is string => name !== undefined);
      await replaceTags.mutateAsync({ userGameId: game.id, tags: tagNames });
```
6. Rewrite `handleCreateTag` to use the existing `createTag` mutation:
```ts
  const handleCreateTag = async (name: string) => {
    try {
      const created = await createTag.mutateAsync({ name });
      setSelectedTagIds((prev) => [...prev, created.id]);
      toast.success(`Tag "${name}" created`);
    } catch (error) {
      console.error('Failed to create tag:', error);
      toast.error('Failed to create tag');
    }
  };
```
(`useMemo` is already imported; `tags` comes from `useAllTags`.)

- [ ] **Step 5: Update the edit-form test**

In `ui/frontend/src/components/games/game-edit-form.test.tsx`:

1. In the `vi.hoisted` `hooks` object, replace `assignTags`, `removeTags`, `createOrGetTag` with:
```ts
  replaceTags: vi.fn(),
  createTag: vi.fn(),
```
2. Extend the hoisted `state` to carry tags (so `useAllTags` can return them):
```ts
const state = vi.hoisted(() => ({ platforms: [] as unknown[], tags: [] as { id: string; name: string }[] }));
```
3. In the `vi.mock('@/hooks', ...)` factory, remove `useAssignTagsToGame`, `useRemoveTagsFromGame`, `useCreateOrGetTag`; change `useAllTags` to read from state; add the two new hooks:
```ts
  useReplaceUserGameTags: () => ({ mutateAsync: hooks.replaceTags, isPending: false }),
  useAllTags: () => ({ data: state.tags, isLoading: false }),
  useCreateTag: () => ({ mutateAsync: hooks.createTag }),
```
4. Add a focused test that asserts the replace-set call on save (place it after the platform save tests):
```ts
  it('saves the selected tags as names via replace-set on save (#1054)', async () => {
    const user = userEvent.setup();
    state.tags = [{ id: 'tag-1', name: 'RPG' }];
    renderForm();

    await user.click(screen.getAllByRole('button', { name: /save changes/i })[0]);

    await waitFor(() => {
      expect(hooks.replaceTags).toHaveBeenCalledWith({
        userGameId: mockGame.id,
        tags: ['RPG'],
      });
    });
  });
```
   - Confirm `waitFor` is imported from `@testing-library/react` in this file; add it to the import if missing. Use the same render helper the surrounding tests use (e.g. `renderForm()`); if they call a differently-named helper or render inline, match that pattern.
   - `mockGame.tags` already contains a tag with `id: 'tag-1'`, so `selectedTagIds` initializes to `['tag-1']`; setting `state.tags` to the matching id makes the name resolve to `'RPG'`.

- [ ] **Step 6: Typecheck, dead-code, tests**

From `ui/frontend/`:
Run: `npm run check`  → expected: zero errors.
Run: `npm run knip`   → expected: zero findings (the new exports are now consumed; the deleted ones are gone).
Run: `npm run test game-edit-form` → expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/api/tags.ts ui/frontend/src/hooks/use-tags.ts ui/frontend/src/hooks/index.ts ui/frontend/src/components/games/game-edit-form.tsx ui/frontend/src/components/games/game-edit-form.test.tsx
git commit -m "feat: wire game edit form to tag-assignment endpoint and drop dead client code (#1054)"
```

---

## Final verification

- [ ] `go build ./...` and `go test ./internal/api/... ./internal/worker/tasks/... ./internal/usergame/...`
- [ ] From `ui/frontend/`: `npm run check && npm run knip && npm run test`
- [ ] `make deadcode` — no new entries (`ReplaceTags`/`ResolveOrCreateTag` are reachable from the handler and workers)
- [ ] Manual smoke (optional): start the server, open a game's edit page, add/remove/create tags, save — confirm tags persist and the prior 500 is gone.
- [ ] Open the PR with `Closes #1054` in the body; title `feat: add tag-assignment endpoint for user-games`.

## Spec coverage check

- Replace-set endpoint `PUT /api/user-games/:id/tags`, by-name, auto-create, ownership-safe, returns updated user-game → Task 2.
- Shared reconcile helper in `internal/usergame`, both import workers switched to it → Task 1.
- Frontend rewire + dead-code removal (`assign`/`remove`/`create-or-get`/`bulk`) → Task 3.
- Tests: add/remove/replace/clear, auto-create vs reuse, ownership/isolation, validation, 404/401 → Task 2; frontend replace-set-on-save → Task 3; worker behavior preserved → Task 1.
