# Issue #689: Clear Library + Reset Database — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Clear Library" endpoint + UI for users and a "Reset Database" endpoint + UI for admins, both with typed-confirmation dialogs.

**Architecture:** Two new backend handlers (`HandleClearLibrary` on `UserGamesHandler`; new `AdminResetHandler`), each running deletions in a single transaction with River job cancellation. Frontend adds Danger Zone cards with `Dialog` + typed-input confirmation to the profile page and admin maintenance page.

**Tech Stack:** Go / Echo v5 / Bun ORM / PostgreSQL; React 19 / TanStack Router / shadcn Dialog / Sonner toasts

---

## File Map

| Action | Path |
|--------|------|
| Modify | `internal/api/user_games.go` — add `HandleClearLibrary` |
| Modify | `internal/api/user_games_test.go` — add `TestHandleClearLibrary` |
| Create | `internal/api/admin_reset.go` — `AdminResetHandler` + `HandleReset` |
| Create | `internal/api/admin_reset_test.go` — `TestHandleAdminReset` |
| Modify | `internal/api/router.go` — register both routes |
| Modify | `ui/frontend/src/api/games.ts` — add `clearLibrary()` |
| Modify | `ui/frontend/src/routes/_authenticated/profile.tsx` — Danger Zone card |
| Modify | `ui/frontend/src/api/admin.ts` — add `resetDatabase()` |
| Modify | `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` — Danger Zone card |
| Modify | `slumber.yaml` — add two new requests |

---

## Task 1: `HandleClearLibrary` — TDD backend

**Files:**
- Modify: `internal/api/user_games_test.go`
- Modify: `internal/api/user_games.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/api/user_games_test.go`:

```go
func TestHandleClearLibrary(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID, token := setupUserGamesUser(t, testDB, e, "clear")

	// Seed 3 games + user games.
	g1 := insertTestGame(t, testDB, "Clear Game 1")
	g2 := insertTestGame(t, testDB, "Clear Game 2")
	g3 := insertTestGame(t, testDB, "Clear Game 3")
	insertTestUserGame(t, testDB, "ug-cl-1", userID, int(g1))
	insertTestUserGame(t, testDB, "ug-cl-2", userID, int(g2))
	insertTestUserGame(t, testDB, "ug-cl-3", userID, int(g3))

	// Seed a job + job item + active river job.
	insertJob(t, testDB, "job-cl-1", userID, "sync", "steam", "processing")
	insertJobItem(t, testDB, "ji-cl-1", "job-cl-1", userID, "key-1", "Game 1", "pending")
	riverID := insertRiverJob(t, testDB, "sync_item", "available", "ji-cl-1")

	// Seed a sync config.
	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO user_sync_configs (id, user_id, storefront) VALUES (?, ?, 'steam')`,
		"sc-cl-1", userID,
	)
	if err != nil {
		t.Fatalf("seed sync_config: %v", err)
	}

	t.Run("deletes library and returns count", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}

		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["deleted"] != float64(3) {
			t.Errorf("deleted = %v, want 3", resp["deleted"])
		}
	})

	t.Run("clears user_games", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, userID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("user_games count = %d, want 0", count)
		}
	})

	t.Run("clears jobs", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ?`, userID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("jobs count = %d, want 0", count)
		}
	})

	t.Run("clears sync configs", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sync_configs WHERE user_id = ?`, userID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("sync_configs count = %d, want 0", count)
		}
	})

	t.Run("cancels active river jobs", func(t *testing.T) {
		var state string
		if err := testDB.NewRaw(`SELECT state FROM river_job WHERE id = ?`, riverID).
			Scan(context.Background(), &state); err != nil {
			t.Fatalf("river state: %v", err)
		}
		if state != "cancelled" {
			t.Errorf("river state = %q, want cancelled", state)
		}
	})

	t.Run("idempotent on empty library", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["deleted"] != float64(0) {
			t.Errorf("deleted = %v, want 0", resp["deleted"])
		}
	})

	t.Run("does not touch other users", func(t *testing.T) {
		otherID, _ := setupUserGamesUser(t, testDB, e, "clear-other")
		otherGame := insertTestGame(t, testDB, "Other User Game")
		insertTestUserGame(t, testDB, "ug-cl-other", otherID, int(otherGame))

		// Clear the original user again (already empty).
		rec := deleteAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}

		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, otherID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count other: %v", err)
		}
		if count != 1 {
			t.Errorf("other user_games count = %d, want 1", count)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/... -run TestHandleClearLibrary -v 2>&1 | tail -5
```

Expected: `FAIL` — method not found or route returns 404.

- [ ] **Step 3: Implement `HandleClearLibrary`**

Append to `internal/api/user_games.go` (before the last utility helpers section):

```go
// HandleClearLibrary handles DELETE /api/user-games.
// Removes all games, jobs, and sync configs for the authenticated user.
func (h *UserGamesHandler) HandleClearLibrary(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	ctx := context.Background()
	var deleted int64
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Cancel active River jobs whose items belong to this user.
		if _, err := tx.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = NOW()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE user_id = ?)`,
			userID,
		).Exec(ctx); err != nil {
			return err
		}
		// Delete jobs (cascades job_items + sync_changes).
		if _, err := tx.NewDelete().Model((*models.Job)(nil)).
			Where("user_id = ?", userID).Exec(ctx); err != nil {
			return err
		}
		// Delete sync configs.
		if _, err := tx.NewDelete().Model((*models.UserSyncConfig)(nil)).
			Where("user_id = ?", userID).Exec(ctx); err != nil {
			return err
		}
		// Delete user games (cascades user_game_platforms + user_game_tags).
		res, err := tx.NewDelete().Model((*models.UserGame)(nil)).
			Where("user_id = ?", userID).Exec(ctx)
		if err != nil {
			return err
		}
		deleted, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, map[string]any{"deleted": deleted})
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, find the `userGamesGroup` block. Add this line **before** the existing `userGamesGroup.DELETE("/bulk-delete", ...)` line:

```go
userGamesGroup.DELETE("", ugh.HandleClearLibrary)
```

The relevant block (around line 257–276) should look like:

```go
ugh := NewUserGamesHandler(db, cfg)
userGamesGroup.GET("", ugh.HandleListUserGames)
userGamesGroup.POST("", ugh.HandleCreateUserGame)
userGamesGroup.DELETE("", ugh.HandleClearLibrary)           // ← add this
userGamesGroup.DELETE("/bulk-delete", ugh.HandleBulkDelete)
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/api/... -run TestHandleClearLibrary -v 2>&1 | tail -10
```

Expected: all subtests `PASS`.

- [ ] **Step 6: Build check**

```bash
go build ./... 2>&1
```

Expected: no output (clean build).

- [ ] **Step 7: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add DELETE /api/user-games clear library endpoint"
```

---

## Task 2: `AdminResetHandler` — TDD backend

**Files:**
- Create: `internal/api/admin_reset.go`
- Create: `internal/api/admin_reset_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/admin_reset_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestHandleAdminReset(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	// Seed admin and two regular users.
	adminID, adminTok := setupAdminUser(t, testDB, e, "reset")
	user1ID, _ := setupRegularUser(t, testDB, e, "reset-u1")
	user2ID, _ := setupRegularUser(t, testDB, e, "reset-u2")

	// Seed games (catalog — must survive reset).
	g1 := insertTestGame(t, testDB, "Catalog Game 1")
	g2 := insertTestGame(t, testDB, "Catalog Game 2")

	// Seed user games for all three users.
	insertTestUserGame(t, testDB, "ug-r-admin", adminID, int(g1))
	insertTestUserGame(t, testDB, "ug-r-u1", user1ID, int(g1))
	insertTestUserGame(t, testDB, "ug-r-u2", user2ID, int(g2))

	// Seed jobs + job items + river jobs.
	insertJob(t, testDB, "job-r-admin", adminID, "sync", "steam", "processing")
	insertJobItem(t, testDB, "ji-r-admin", "job-r-admin", adminID, "k1", "t1", "pending")
	riverID := insertRiverJob(t, testDB, "sync_item", "available", "ji-r-admin")

	insertJob(t, testDB, "job-r-u1", user1ID, "import", "csv", "completed")

	// Seed sync configs.
	for _, row := range []struct{ id, uid string }{
		{"sc-r-admin", adminID},
		{"sc-r-u1", user1ID},
	} {
		if _, err := testDB.ExecContext(context.Background(),
			`INSERT INTO user_sync_configs (id, user_id, storefront) VALUES (?, ?, 'steam')`,
			row.id, row.uid,
		); err != nil {
			t.Fatalf("seed sync_config: %v", err)
		}
	}

	// Seed tags.
	insertTag(t, testDB, "tag-r-admin", adminID, "Admin Tag", nil)
	insertTag(t, testDB, "tag-r-u1", user1ID, "User1 Tag", nil)

	t.Run("admin can reset", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/auth/admin/reset", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// 3 user_games were seeded (admin + user1 + user2).
		if resp["deleted"] != float64(3) {
			t.Errorf("deleted = %v, want 3", resp["deleted"])
		}
	})

	t.Run("non-admin users are deleted", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM users WHERE NOT is_admin`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("non-admin user count = %d, want 0", count)
		}
	})

	t.Run("admin account is preserved", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM users WHERE id = ?`, adminID).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("admin count = %d, want 1", count)
		}
	})

	t.Run("all user_games are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_games`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("user_games count = %d, want 0", count)
		}
	})

	t.Run("all jobs are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("jobs count = %d, want 0", count)
		}
	})

	t.Run("all sync configs are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sync_configs`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("sync_configs count = %d, want 0", count)
		}
	})

	t.Run("all tags are cleared", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM tags`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("tags count = %d, want 0", count)
		}
	})

	t.Run("active river job is cancelled", func(t *testing.T) {
		var state string
		if err := testDB.NewRaw(`SELECT state FROM river_job WHERE id = ?`, riverID).
			Scan(context.Background(), &state); err != nil {
			t.Fatalf("river state: %v", err)
		}
		if state != "cancelled" {
			t.Errorf("river state = %q, want cancelled", state)
		}
	})

	t.Run("catalog games are preserved", func(t *testing.T) {
		var count int
		if err := testDB.NewRaw(`SELECT COUNT(*) FROM games`).
			Scan(context.Background(), &count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 2 {
			t.Errorf("games count = %d, want 2", count)
		}
	})

	t.Run("idempotent on empty state", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/auth/admin/reset", nil, adminTok)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["deleted"] != float64(0) {
			t.Errorf("deleted = %v, want 0", resp["deleted"])
		}
	})

	t.Run("non-admin gets 403", func(t *testing.T) {
		truncateAllTables(t)
		e2 := newTestEcho(t, testDB, cfg)
		_, regTok := setupRegularUser(t, testDB, e2, "reset-403")

		rec := postJSONAuth(t, e2, "/api/auth/admin/reset", nil, regTok)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/... -run TestHandleAdminReset -v 2>&1 | tail -5
```

Expected: `FAIL` — file does not compile or route not found.

- [ ] **Step 3: Create `internal/api/admin_reset.go`**

```go
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// AdminResetHandler exposes POST /api/auth/admin/reset.
type AdminResetHandler struct {
	db *bun.DB
}

// NewAdminResetHandler constructs an AdminResetHandler.
func NewAdminResetHandler(db *bun.DB) *AdminResetHandler {
	return &AdminResetHandler{db: db}
}

// HandleReset handles POST /api/auth/admin/reset.
// Truncates all user data (library, jobs, sync configs, tags, non-admin users)
// while preserving the admin account, catalog tables, and backup config.
func (h *AdminResetHandler) HandleReset(c *echo.Context) error {
	ctx := context.Background()
	var deleted int64
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Cancel all active River jobs.
		if _, err := tx.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = NOW()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')`).
			Exec(ctx); err != nil {
			return err
		}
		// Delete all user games (cascades user_game_platforms + user_game_tags).
		res, err := tx.NewRaw("DELETE FROM user_games").Exec(ctx)
		if err != nil {
			return err
		}
		deleted, err = res.RowsAffected()
		if err != nil {
			return err
		}
		// Delete all jobs (cascades job_items + sync_changes).
		if _, err := tx.NewRaw("DELETE FROM jobs").Exec(ctx); err != nil {
			return err
		}
		// Delete all sync configs.
		if _, err := tx.NewRaw("DELETE FROM user_sync_configs").Exec(ctx); err != nil {
			return err
		}
		// Delete all tags.
		if _, err := tx.NewRaw("DELETE FROM tags").Exec(ctx); err != nil {
			return err
		}
		// Delete non-admin users (cascades user_sessions + api_keys).
		if _, err := tx.NewRaw("DELETE FROM users WHERE NOT is_admin").Exec(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("admin reset: transaction failed", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, map[string]any{"deleted": deleted})
}
```

- [ ] **Step 4: Register the route**

In `internal/api/router.go`, find the `adminGroup` block (around line 311). After the existing admin backup and admin user routes, add:

```go
arh := NewAdminResetHandler(db)
adminGroup.POST("/api/auth/admin/reset", arh.HandleReset)
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/api/... -run TestHandleAdminReset -v 2>&1 | tail -15
```

Expected: all subtests `PASS`.

- [ ] **Step 6: Run full test suite**

```bash
go test -timeout 600s ./... 2>&1 | tail -20
```

Expected: `ok` for all packages.

- [ ] **Step 7: Commit**

```bash
git add internal/api/admin_reset.go internal/api/admin_reset_test.go internal/api/router.go
git commit -m "feat: add POST /api/auth/admin/reset endpoint"
```

---

## Task 3: Frontend — clearLibrary API + profile Danger Zone

**Files:**
- Modify: `ui/frontend/src/api/games.ts`
- Modify: `ui/frontend/src/routes/_authenticated/profile.tsx`

- [ ] **Step 1: Add `clearLibrary()` to `ui/frontend/src/api/games.ts`**

After the `deleteUserGame` function (around line 491), add:

```ts
/**
 * Remove all games from the authenticated user's library.
 */
export async function clearLibrary(): Promise<{ deleted: number }> {
  return api.delete<{ deleted: number }>('/user-games');
}
```

- [ ] **Step 2: Update imports in `profile.tsx`**

Replace the existing import block in `ui/frontend/src/routes/_authenticated/profile.tsx`:

```ts
import { useState, useEffect, useCallback, useMemo } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';
import { useCollectionStats } from '@/hooks';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from 'sonner';
import { Eye, EyeOff, Check, X, Loader2, AlertCircle, User, Trash2 } from 'lucide-react';
import * as authApi from '@/api/auth';
import * as gamesApi from '@/api/games';
```

- [ ] **Step 3: Add clear-library state inside `ProfilePage`**

At the top of the `ProfilePage` function body, after the existing state declarations, add:

```ts
const { data: stats } = useCollectionStats();
const navigate = useNavigate();

// Clear library state
const [isClearDialogOpen, setIsClearDialogOpen] = useState(false);
const [clearConfirmText, setClearConfirmText] = useState('');
const [isClearing, setIsClearing] = useState(false);

const handleClearLibrary = async () => {
  setIsClearing(true);
  try {
    const result = await gamesApi.clearLibrary();
    toast.success(`Cleared ${result.deleted} game${result.deleted === 1 ? '' : 's'} from your library`);
    setIsClearDialogOpen(false);
    setClearConfirmText('');
    void navigate({ to: '/games' });
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to clear library');
  } finally {
    setIsClearing(false);
  }
};
```

- [ ] **Step 4: Add the Danger Zone card + dialog to the JSX**

At the very end of the returned JSX in `ProfilePage`, after the closing `</div>` of the `grid` div, add:

```tsx
{/* Danger Zone */}
<Card className="border-red-200 dark:border-red-800">
  <CardHeader>
    <CardTitle className="flex items-center gap-2 text-red-600 dark:text-red-400">
      <Trash2 className="h-5 w-5" />
      Danger Zone
    </CardTitle>
    <CardDescription>These actions are permanent and cannot be undone.</CardDescription>
  </CardHeader>
  <CardContent>
    <div className="flex items-center justify-between">
      <div>
        <p className="font-medium">Clear Library</p>
        <p className="text-sm text-muted-foreground">
          Remove all games from your library.
        </p>
      </div>
      <Button variant="destructive" onClick={() => setIsClearDialogOpen(true)}>
        Clear Library
      </Button>
    </div>
  </CardContent>
</Card>

<Dialog
  open={isClearDialogOpen}
  onOpenChange={(open) => {
    if (!open) {
      setIsClearDialogOpen(false);
      setClearConfirmText('');
    }
  }}
>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>Clear Library</DialogTitle>
      <DialogDescription>
        This will permanently remove all{' '}
        <strong>{stats?.totalGames ?? '?'} game{stats?.totalGames === 1 ? '' : 's'}</strong>{' '}
        from your library. This cannot be undone.
      </DialogDescription>
    </DialogHeader>
    <div className="space-y-2">
      <p className="text-sm text-muted-foreground">
        Type <strong>DELETE</strong> to confirm:
      </p>
      <Input
        value={clearConfirmText}
        onChange={(e) => setClearConfirmText(e.target.value)}
        placeholder="Type DELETE to confirm"
        autoComplete="off"
      />
    </div>
    <DialogFooter>
      <Button
        variant="outline"
        onClick={() => {
          setIsClearDialogOpen(false);
          setClearConfirmText('');
        }}
      >
        Cancel
      </Button>
      <Button
        variant="destructive"
        disabled={clearConfirmText !== 'DELETE' || isClearing}
        onClick={handleClearLibrary}
      >
        {isClearing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        Clear Library
      </Button>
    </DialogFooter>
  </DialogContent>
</Dialog>
```

- [ ] **Step 5: Typecheck and build**

```bash
cd ui/frontend && npm run check 2>&1 | tail -10
```

Expected: no errors.

```bash
npm run build 2>&1 | tail -5
```

Expected: `✓ built in ...`

- [ ] **Step 6: Commit**

```bash
cd ../.. && git add ui/frontend/src/api/games.ts ui/frontend/src/routes/_authenticated/profile.tsx
git commit -m "feat: add clear library Danger Zone to profile page"
```

---

## Task 4: Frontend — resetDatabase API + maintenance Danger Zone

**Files:**
- Modify: `ui/frontend/src/api/admin.ts`
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx`

- [ ] **Step 1: Add `resetDatabase()` to `ui/frontend/src/api/admin.ts`**

At the end of `ui/frontend/src/api/admin.ts`, add:

```ts
/**
 * Reset the database to post-admin-creation state (admin only).
 */
export async function resetDatabase(): Promise<{ deleted: number }> {
  return api.post<{ deleted: number }>('/auth/admin/reset', {});
}
```

- [ ] **Step 2: Update imports in `maintenance.tsx`**

Replace the existing import block in `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx`:

```ts
import { useState, useEffect } from 'react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';
import { useActiveJob, useCancelJob } from '@/hooks';
import { JobType } from '@/types';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { JobProgressCard, JobItemsDetails, RecentActivity } from '@/components/jobs';
import { toast } from 'sonner';
import { RefreshCw, Loader2, RotateCcw, AlertTriangle } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { useHealthStatus } from '@/hooks/use-health-status';
import * as adminApi from '@/api/admin';
```

- [ ] **Step 3: Add reset state inside `MaintenancePage`**

At the top of the `MaintenancePage` function body, after the existing state and hooks, add:

```ts
// Reset database state: 0 = closed, 1 = first confirm dialog, 2 = typed confirm dialog.
const [resetStep, setResetStep] = useState(0);
const [resetConfirmText, setResetConfirmText] = useState('');
const [isResetting, setIsResetting] = useState(false);

const handleReset = async () => {
  setIsResetting(true);
  try {
    await adminApi.resetDatabase();
    toast.success('Database reset complete');
    setResetStep(0);
    setResetConfirmText('');
    void navigate({ to: '/dashboard' });
  } catch (err) {
    toast.error(err instanceof Error ? err.message : 'Failed to reset database');
  } finally {
    setIsResetting(false);
  }
};
```

- [ ] **Step 4: Add Danger Zone card + dialogs to the JSX**

At the very end of the returned JSX in `MaintenancePage`, after the last section (the `RecentActivity` block or the IGDB card), add:

```tsx
{/* Danger Zone */}
<Card className="border-red-200 dark:border-red-800">
  <CardHeader>
    <CardTitle className="flex items-center gap-2 text-red-600 dark:text-red-400">
      <AlertTriangle className="h-5 w-5" />
      Danger Zone
    </CardTitle>
    <CardDescription>
      These actions are permanent and cannot be undone.
    </CardDescription>
  </CardHeader>
  <CardContent>
    <div className="flex items-center justify-between">
      <div>
        <p className="font-medium">Reset Database</p>
        <p className="text-sm text-muted-foreground">
          Delete all users, libraries, sync configs, jobs, and tags.
        </p>
      </div>
      <Button variant="destructive" onClick={() => setResetStep(1)}>
        Reset Database
      </Button>
    </div>
  </CardContent>
</Card>

{/* Step 1: First confirmation */}
<Dialog open={resetStep === 1} onOpenChange={(open) => { if (!open) setResetStep(0); }}>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>Reset Database</DialogTitle>
      <DialogDescription>
        Are you sure? This will delete all users (except you), all libraries, all sync
        configs, jobs, and tags. This cannot be undone.
      </DialogDescription>
    </DialogHeader>
    <DialogFooter>
      <Button variant="outline" onClick={() => setResetStep(0)}>
        Cancel
      </Button>
      <Button variant="destructive" onClick={() => setResetStep(2)}>
        Yes, continue
      </Button>
    </DialogFooter>
  </DialogContent>
</Dialog>

{/* Step 2: Typed confirmation */}
<Dialog
  open={resetStep === 2}
  onOpenChange={(open) => {
    if (!open) {
      setResetStep(0);
      setResetConfirmText('');
    }
  }}
>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>Last Chance</DialogTitle>
      <DialogDescription>
        Type <strong>RESET</strong> to confirm. This action is irreversible.
      </DialogDescription>
    </DialogHeader>
    <div className="space-y-2">
      <Input
        value={resetConfirmText}
        onChange={(e) => setResetConfirmText(e.target.value)}
        placeholder="Type RESET to confirm"
        autoComplete="off"
      />
    </div>
    <DialogFooter>
      <Button
        variant="outline"
        onClick={() => {
          setResetStep(0);
          setResetConfirmText('');
        }}
      >
        Cancel
      </Button>
      <Button
        variant="destructive"
        disabled={resetConfirmText !== 'RESET' || isResetting}
        onClick={handleReset}
      >
        {isResetting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        Reset Database
      </Button>
    </DialogFooter>
  </DialogContent>
</Dialog>
```

- [ ] **Step 5: Typecheck and build**

```bash
cd ui/frontend && npm run check 2>&1 | tail -10
```

Expected: no errors.

```bash
npm run build 2>&1 | tail -5
```

Expected: `✓ built in ...`

- [ ] **Step 6: Commit**

```bash
cd ../.. && git add ui/frontend/src/api/admin.ts ui/frontend/src/routes/_authenticated/admin/maintenance.tsx
git commit -m "feat: add reset database Danger Zone to admin maintenance page"
```

---

## Task 5: Slumber collection entries

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add `clear_library` to the `user_games` folder**

In `slumber.yaml`, find the `user_games:` → `requests:` block. After the last existing request in that block, add:

```yaml
      clear_library:
        name: Clear Library
        method: DELETE
        url: "{{base_url}}/api/user-games"
        $ref: "#/.authenticated"
```

- [ ] **Step 2: Add `reset_database` to the `admin_users` folder**

In `slumber.yaml`, find the `admin_users:` → `requests:` block. After the `delete_user` entry, add:

```yaml
      reset_database:
        name: Reset Database
        method: POST
        url: "{{base_url}}/api/auth/admin/reset"
        $ref: "#/.authenticated"
        body:
          type: json
          data: {}
```

- [ ] **Step 3: Verify the collection loads**

```bash
slumber collection 2>&1 | tail -5
```

Expected: no errors, collection lists all requests including the two new ones.

- [ ] **Step 4: Commit**

```bash
git add slumber.yaml
git commit -m "chore: add clear library and reset database to slumber collection"
```
