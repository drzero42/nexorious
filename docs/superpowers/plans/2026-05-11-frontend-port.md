# Frontend Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the React SPA from the Python nexorious repository into `ui/frontend/` of the Go repository, wired against the Go backend.

**Architecture:** The React SPA is copied verbatim from `/home/abo/workspace/home/nexorious/frontend/`, placed under `ui/frontend/`, then a small set of targeted changes are applied: build plumbing (Makefile/embed), backend icon_url API responses, wishlist removal, logo URL path rewrites, and the new IGDB availability UI (health hook + banner + disabled states).

**Tech Stack:** Go/Echo (backend), React 19 + Vite 6 + TanStack Router/Query + Tailwind CSS v4 + shadcn/ui (frontend), Vitest + MSW (frontend tests), testcontainers (Go tests).

---

## File Map

### Created
| Path | Purpose |
|---|---|
| `ui/frontend/` | React SPA project root (copied from Python repo) |
| `ui/frontend/public/logos/` | Logo assets (moved from `ui/public/logos/`) |
| `ui/frontend/src/hooks/use-health-status.ts` | TanStack Query hook for `/health` endpoint |
| `ui/frontend/src/hooks/use-health-status.test.ts` | Tests for the health hook |
| `ui/frontend/src/routes/_authenticated.test.tsx` | Tests for IGDB banner in authenticated layout |
| `ui/frontend/src/routes/_authenticated/games/add.test.tsx` | Tests for IGDB disabled state on add game page |

### Modified
| Path | Change |
|---|---|
| `.gitignore` | Replace `ui/dist/` with `ui/frontend/dist/`, add `ui/frontend/node_modules/` and `ui/frontend/src/routeTree.gen.ts` |
| `Makefile` | Update `frontend` target to `cd ui/frontend` |
| `ui/ui.go` | Change embed path from `dist` to `frontend/dist` |
| `internal/api/router.go` | Change `fs.Sub(ui.UIBox, "dist")` → `fs.Sub(ui.UIBox, "frontend/dist")` |
| `internal/api/platforms.go` | Add DTO types + helpers; update all handlers to return `icon_url` |
| `internal/api/platforms_test.go` | Add icon_url assertion tests |
| `ui/frontend/package.json` | Rename `name` from `"frontend"` to `"nexorious-ui"` |
| `ui/frontend/src/types/admin.ts` | Remove `total_wishlist_items: number` |
| `ui/frontend/src/routes/_authenticated/admin/users/$id.tsx` | Remove wishlist row from deletion-impact dialog |
| `ui/frontend/src/api/admin.test.ts` | Remove `total_wishlist_items: 3` from fixture |
| `ui/frontend/src/types/import-export.ts` | Remove wishlist references from export description |
| `ui/frontend/src/types/sync.ts` | `/static/logos/` → `/logos/` in icon URL constants |
| `ui/frontend/src/routes/_authenticated/admin/platforms.tsx` | Update icon URL placeholder text |
| `ui/frontend/src/components/games/game-card.test.tsx` | Update 4 `/static/logos/` fixture paths |
| `ui/frontend/src/components/games/game-list.test.tsx` | Update 4 `/static/logos/` fixture paths |
| `ui/frontend/src/routes/_authenticated.tsx` | Export `AuthenticatedLayout`; add IGDB banner |
| `ui/frontend/src/routes/_authenticated/games/add.index.tsx` | Export `AddGamePage`; disable IGDB search when unconfigured |
| `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` | Disable IGDB refresh button when unconfigured |
| `ui/frontend/src/components/games/igdb-search.tsx` | Handle 503 errors with toast notification |

---

## Task 1: Copy frontend source files

**Files:**
- Create: `ui/frontend/` (all source files from Python frontend)

- [ ] **Step 1: Create the destination directory structure**

```bash
mkdir -p ui/frontend/public
```

- [ ] **Step 2: Copy all frontend source files**

```bash
cp -r /home/abo/workspace/home/nexorious/frontend/src ui/frontend/src
cp /home/abo/workspace/home/nexorious/frontend/index.html ui/frontend/index.html
cp /home/abo/workspace/home/nexorious/frontend/package.json ui/frontend/package.json
cp /home/abo/workspace/home/nexorious/frontend/tsconfig.json ui/frontend/tsconfig.json
cp /home/abo/workspace/home/nexorious/frontend/vite.config.ts ui/frontend/vite.config.ts
cp /home/abo/workspace/home/nexorious/frontend/vitest.config.ts ui/frontend/vitest.config.ts
cp /home/abo/workspace/home/nexorious/frontend/eslint.config.mjs ui/frontend/eslint.config.mjs
cp /home/abo/workspace/home/nexorious/frontend/postcss.config.mjs ui/frontend/postcss.config.mjs
cp /home/abo/workspace/home/nexorious/frontend/components.json ui/frontend/components.json
```

- [ ] **Step 3: Verify the copy**

```bash
ls ui/frontend/src/routes/ | head -5
ls ui/frontend/src/types/
```

Expected: `_authenticated.tsx`, `_authenticated/`, etc. present; type files present.

- [ ] **Step 4: Move logos from `ui/public/logos/` into `ui/frontend/public/logos/`**

```bash
mv ui/public/logos ui/frontend/public/logos
rmdir ui/public
```

- [ ] **Step 5: Verify logo move**

```bash
ls ui/frontend/public/logos/
```

Expected: `platforms/` and `storefronts/` directories present.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/ && git commit -m "feat: copy React SPA source into ui/frontend/ and move logos"
```

---

## Task 2: Update .gitignore

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Replace `ui/dist/` and add frontend-specific entries**

Replace the line `ui/dist/` in `.gitignore` with:

```
ui/frontend/dist/
ui/frontend/node_modules/
ui/frontend/src/routeTree.gen.ts
```

The full `.gitignore` should look like:

```
# Go binary
/nexorious

# Frontend build output (populated by make frontend)
ui/frontend/dist/
ui/frontend/node_modules/
ui/frontend/src/routeTree.gen.ts

# devenv
.devenv/
.devenv.flake.nix

.env

# Worktrees
.worktrees/

storage
```

- [ ] **Step 2: Commit**

```bash
git add .gitignore && git commit -m "chore: update .gitignore for ui/frontend/ layout"
```

---

## Task 3: Update build infrastructure

**Files:**
- Modify: `Makefile`
- Modify: `ui/ui.go`
- Modify: `internal/api/router.go:332`

- [ ] **Step 1: Update `Makefile` frontend target**

In `Makefile`, change line 10 from:
```makefile
	cd ui && npm install && npm run build
```
to:
```makefile
	cd ui/frontend && npm install && npm run build
```

The full `Makefile`:
```makefile
.PHONY: all frontend build test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

all: frontend build

frontend:
	cd ui/frontend && npm install && npm run build

build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious

test:
	go test ./...
```

- [ ] **Step 2: Update `ui/ui.go` embed path**

Replace `ui/ui.go` entirely:

```go
package ui

import "embed"

//go:embed all:frontend/dist
var UIBox embed.FS

//go:embed all:migrate
var MigrateBox embed.FS

//go:embed db-error
var DBErrorBox embed.FS

//go:embed setup
var SetupBox embed.FS
```

- [ ] **Step 3: Update `spaHandler` in `internal/api/router.go`**

In `internal/api/router.go` at line 332, change:
```go
	fsys, err := fs.Sub(ui.UIBox, "dist")
```
to:
```go
	fsys, err := fs.Sub(ui.UIBox, "frontend/dist")
```

- [ ] **Step 4: Verify Go compiles (the SPA dist won't exist yet, so skip embed check)**

```bash
go build ./... 2>&1 | grep -v "no required module" | head -20
```

Note: `go build` will fail because `ui/frontend/dist/` doesn't exist yet (it's gitignored). That is expected at this stage — the build check is just to verify no syntax errors:

```bash
go vet ./... 2>&1 | head -10
```

Expected: no output (clean).

- [ ] **Step 5: Commit**

```bash
git add Makefile ui/ui.go internal/api/router.go && git commit -m "chore: update build to use ui/frontend/ layout"
```

---

## Task 4: Update `package.json` name and run npm install

**Files:**
- Modify: `ui/frontend/package.json`

- [ ] **Step 1: Update `name` field in `package.json`**

Open `ui/frontend/package.json` and change the `name` field:
```json
"name": "nexorious-ui",
```

- [ ] **Step 2: Run npm install to generate `node_modules` and `package-lock.json`**

```bash
cd ui/frontend && npm install
```

Expected: installation completes with no errors. `node_modules/` is created (gitignored).

- [ ] **Step 3: Verify TypeScript compiles (routeTree.gen.ts must exist first)**

```bash
cd ui/frontend && npm run build 2>&1 | tail -5
```

Expected: Vite builds successfully and outputs to `ui/frontend/dist/`. The first build also generates `src/routeTree.gen.ts`.

- [ ] **Step 4: Commit**

```bash
cd .. && git add ui/frontend/package.json && git commit -m "chore: rename frontend package to nexorious-ui"
```

---

## Task 5: Backend — add `icon_url` to platform/storefront responses (write failing tests)

**Files:**
- Modify: `internal/api/platforms_test.go`

- [ ] **Step 1: Add failing tests for `icon_url` in platform/storefront responses**

Append to `internal/api/platforms_test.go`:

```go
func TestListPlatforms_HasIconURL(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	token := loginAndGetToken(t, e, setupUser(t, db), "pass123")

	rec := getAuth(t, e, "/api/platforms", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var platforms []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &platforms); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var pcWindows map[string]any
	for _, p := range platforms {
		if p["name"] == "pc-windows" {
			pcWindows = p
			break
		}
	}
	if pcWindows == nil {
		t.Fatal("expected pc-windows platform in list")
	}

	iconURL, ok := pcWindows["icon_url"].(string)
	if !ok {
		t.Fatalf("expected icon_url string, got %T: %v", pcWindows["icon_url"], pcWindows["icon_url"])
	}
	want := "/logos/platforms/pc-windows/pc-windows-icon-light.svg"
	if iconURL != want {
		t.Fatalf("expected icon_url=%q, got %q", want, iconURL)
	}

	// Storefronts embedded in platform response also need icon_url
	storefronts, _ := pcWindows["storefronts"].([]any)
	for _, sfAny := range storefronts {
		sf, ok := sfAny.(map[string]any)
		if !ok {
			continue
		}
		if sf["icon"] != nil {
			if _, ok := sf["icon_url"].(string); !ok {
				t.Fatalf("storefront %v has icon but missing icon_url", sf["name"])
			}
		}
	}
}

func TestListStorefronts_HasIconURL(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	token := loginAndGetToken(t, e, setupUser(t, db), "pass123")

	rec := getAuth(t, e, "/api/platforms/storefronts", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var storefronts []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &storefronts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var steam map[string]any
	for _, s := range storefronts {
		if s["name"] == "steam" {
			steam = s
			break
		}
	}
	if steam == nil {
		t.Fatal("expected steam storefront in list")
	}

	iconURL, ok := steam["icon_url"].(string)
	if !ok {
		t.Fatalf("expected icon_url string, got %T: %v", steam["icon_url"], steam["icon_url"])
	}
	want := "/logos/storefronts/steam/steam-icon-light.svg"
	if iconURL != want {
		t.Fatalf("expected icon_url=%q, got %q", want, iconURL)
	}
}
```

- [ ] **Step 2: Run the new tests and confirm they fail**

```bash
go test ./internal/api/... -run "TestListPlatforms_HasIconURL|TestListStorefronts_HasIconURL" -v 2>&1 | tail -20
```

Expected: FAIL — `expected icon_url string, got <nil>`.

---

## Task 6: Backend — implement DTO types and update handlers

**Files:**
- Modify: `internal/api/platforms.go`

- [ ] **Step 1: Add `"fmt"` to the imports in `internal/api/platforms.go`**

Change the import block from:
```go
import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)
```
to:
```go
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)
```

- [ ] **Step 2: Add DTO types, helpers, and update `defaultStorefrontResponse` in `platforms.go`**

After the `PlatformsHandler` struct definition (after line 25), add the new types and helpers. Also update `defaultStorefrontResponse` (currently at line 114) to use `*storefrontResponse` instead of `*models.Storefront`.

Replace the `defaultStorefrontResponse` type and add new types/helpers. The new block to insert right after the `simpleItem` type (after line 50 in the current file):

```go
// storefrontResponse is the API response shape for a storefront, including icon_url.
type storefrontResponse struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	Icon        *string `json:"icon"`
	BaseUrl     *string `json:"base_url"`
	IconURL     *string `json:"icon_url"`
}

// platformResponse is the API response shape for a platform, including icon_url.
type platformResponse struct {
	Name              string               `json:"name"`
	DisplayName       string               `json:"display_name"`
	Icon              *string              `json:"icon"`
	IgdbPlatformID    *int32               `json:"igdb_platform_id"`
	DefaultStorefront *string              `json:"default_storefront"`
	Storefronts       []storefrontResponse `json:"storefronts,omitempty"`
	IconURL           *string              `json:"icon_url"`
}

func iconURL(category, name string, icon *string) *string {
	if icon == nil {
		return nil
	}
	u := fmt.Sprintf("/logos/%s/%s/%s", category, name, *icon)
	return &u
}

func toStorefrontResponse(sf models.Storefront) storefrontResponse {
	return storefrontResponse{
		Name:        sf.Name,
		DisplayName: sf.DisplayName,
		Icon:        sf.Icon,
		BaseUrl:     sf.BaseUrl,
		IconURL:     iconURL("storefronts", sf.Name, sf.Icon),
	}
}

func toPlatformResponse(p models.Platform) platformResponse {
	resp := platformResponse{
		Name:              p.Name,
		DisplayName:       p.DisplayName,
		Icon:              p.Icon,
		IgdbPlatformID:    p.IgdbPlatformID,
		DefaultStorefront: p.DefaultStorefront,
		IconURL:           iconURL("platforms", p.Name, p.Icon),
	}
	for _, sf := range p.Storefronts {
		resp.Storefronts = append(resp.Storefronts, toStorefrontResponse(sf))
	}
	return resp
}
```

Also update `defaultStorefrontResponse` (find the existing definition and replace `*models.Storefront` with `*storefrontResponse`):

```go
type defaultStorefrontResponse struct {
	Platform            string              `json:"platform"`
	PlatformDisplayName string              `json:"platform_display_name"`
	DefaultStorefront   *storefrontResponse `json:"default_storefront"`
}
```

- [ ] **Step 3: Update all handlers to return DTOs**

Update `HandleListPlatforms` — change `return c.JSON(http.StatusOK, platforms)` to:
```go
	resp := make([]platformResponse, len(platforms))
	for i, p := range platforms {
		resp[i] = toPlatformResponse(p)
	}
	return c.JSON(http.StatusOK, resp)
```

Update `HandleGetPlatform` — change `return c.JSON(http.StatusOK, platform)` to:
```go
	return c.JSON(http.StatusOK, toPlatformResponse(platform))
```

Update `HandlePlatformStorefronts` — change `return c.JSON(http.StatusOK, storefronts)` to:
```go
	resp := make([]storefrontResponse, len(storefronts))
	for i, sf := range storefronts {
		resp[i] = toStorefrontResponse(sf)
	}
	return c.JSON(http.StatusOK, resp)
```

Update `HandleListStorefronts` — change `return c.JSON(http.StatusOK, storefronts)` to:
```go
	resp := make([]storefrontResponse, len(storefronts))
	for i, sf := range storefronts {
		resp[i] = toStorefrontResponse(sf)
	}
	return c.JSON(http.StatusOK, resp)
```

Update `HandleGetStorefront` — change `return c.JSON(http.StatusOK, sf)` to:
```go
	return c.JSON(http.StatusOK, toStorefrontResponse(sf))
```

Update `HandleDefaultStorefront` — where the storefront is set on the response, change:
```go
		if err == nil {
			resp.DefaultStorefront = &sf
		}
```
to:
```go
		if err == nil {
			sfResp := toStorefrontResponse(sf)
			resp.DefaultStorefront = &sfResp
		}
```

- [ ] **Step 4: Verify Go compiles**

```bash
go vet ./internal/api/...
```

Expected: no output.

- [ ] **Step 5: Run the new tests and confirm they pass**

```bash
go test ./internal/api/... -run "TestListPlatforms_HasIconURL|TestListStorefronts_HasIconURL" -v 2>&1 | tail -10
```

Expected: PASS for both tests.

- [ ] **Step 6: Run the full platform test suite to verify no regressions**

```bash
go test ./internal/api/... -run "TestListPlatforms|TestGetPlatform|TestPlatformStorefronts|TestDefaultStorefront|TestListStorefronts|TestGetStorefront|TestStorefrontSimpleList|TestPlatformSimpleList" -v 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/platforms.go internal/api/platforms_test.go && git commit -m "feat: add icon_url to platform and storefront API responses"
```

---

## Task 7: Frontend — fix `/static/logos/` → `/logos/` paths

**Files:**
- Modify: `ui/frontend/src/types/sync.ts`
- Modify: `ui/frontend/src/routes/_authenticated/admin/platforms.tsx`

- [ ] **Step 1: Update four icon URL constants in `sync.ts`**

In `ui/frontend/src/types/sync.ts`, replace all four `/static/logos/` occurrences with `/logos/`:

| Line | Old | New |
|---|---|---|
| 81 | `iconUrl: '/static/logos/storefronts/steam/steam-icon-light.svg'` | `iconUrl: '/logos/storefronts/steam/steam-icon-light.svg'` |
| 87 | `iconUrl: '/static/logos/storefronts/epic-games-store/epic-games-store-icon-light.svg'` | `iconUrl: '/logos/storefronts/epic-games-store/epic-games-store-icon-light.svg'` |
| 93 | `iconUrl: '/static/logos/storefronts/gog/gog-icon-light.svg'` | `iconUrl: '/logos/storefronts/gog/gog-icon-light.svg'` |
| 99 | `iconUrl: '/static/logos/storefronts/playstation-store/playstation-store-icon-light.svg'` | `iconUrl: '/logos/storefronts/playstation-store/playstation-store-icon-light.svg'` |

- [ ] **Step 2: Update placeholder text in `platforms.tsx`**

In `ui/frontend/src/routes/_authenticated/admin/platforms.tsx`:

Line 930: change `placeholder="/static/logos/platforms/example/icon.svg"` to:
```tsx
placeholder="/logos/platforms/example/icon.svg"
```

Line 1036: change `placeholder="/static/logos/storefronts/example/icon.svg"` to:
```tsx
placeholder="/logos/storefronts/example/icon.svg"
```

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/types/sync.ts ui/frontend/src/routes/_authenticated/admin/platforms.tsx
git commit -m "fix: update logo URL paths from /static/logos/ to /logos/"
```

---

## Task 8: Frontend — remove wishlist references

**Files:**
- Modify: `ui/frontend/src/types/admin.ts`
- Modify: `ui/frontend/src/routes/_authenticated/admin/users/$id.tsx`
- Modify: `ui/frontend/src/api/admin.test.ts`
- Modify: `ui/frontend/src/types/import-export.ts`

- [ ] **Step 1: Remove `total_wishlist_items` from `admin.ts`**

In `ui/frontend/src/types/admin.ts`, in the `UserDeletionImpact` interface, remove the line:
```ts
  total_wishlist_items: number;
```

The updated interface:
```ts
export interface UserDeletionImpact {
  user_id: string;
  username: string;
  total_games: number;
  total_tags: number;
  total_import_jobs: number;
  total_sessions: number;
  warning: string;
}
```

- [ ] **Step 2: Remove wishlist row from deletion-impact dialog in `users/$id.tsx`**

In `ui/frontend/src/routes/_authenticated/admin/users/$id.tsx`, remove lines 573–578:
```tsx
                          <div className="flex justify-between">
                            <span>Wishlist items:</span>
                            <span className="font-medium text-destructive">
                              {deletionImpact.total_wishlist_items}
                            </span>
                          </div>
```

- [ ] **Step 3: Remove `total_wishlist_items` from admin test fixture**

In `ui/frontend/src/api/admin.test.ts` at line 390, remove:
```ts
        total_wishlist_items: 3,
```

The fixture block should look like:
```ts
      const mockImpact = {
        user_id: 'user-123',
        username: 'testuser',
        total_games: 10,
        total_tags: 5,
        total_import_jobs: 2,
        total_sessions: 1,
        warning: 'This action cannot be undone',
      };
```

- [ ] **Step 4: Remove wishlist copy from `import-export.ts`**

In `ui/frontend/src/types/import-export.ts`, update the JSON export description (lines 68–69):

Old:
```ts
      description: 'Export your entire game collection and wishlist to a JSON file for backup or transfer.',
      features: ['Complete collection and wishlist', 'Includes all metadata', 'Recommended for re-import'],
```

New:
```ts
      description: 'Export your entire game collection to a JSON file for backup or transfer.',
      features: ['Complete collection', 'Includes all metadata', 'Recommended for re-import'],
```

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/types/admin.ts \
  "ui/frontend/src/routes/_authenticated/admin/users/\$id.tsx" \
  ui/frontend/src/api/admin.test.ts \
  ui/frontend/src/types/import-export.ts
git commit -m "fix: remove wishlist references not present in Go backend"
```

---

## Task 9: Frontend — update test fixtures for logo URL paths

**Files:**
- Modify: `ui/frontend/src/components/games/game-card.test.tsx`
- Modify: `ui/frontend/src/components/games/game-list.test.tsx`

- [ ] **Step 1: Update `/static/logos/` → `/logos/` in `game-card.test.tsx`**

In `ui/frontend/src/components/games/game-card.test.tsx`, replace all occurrences of `/static/logos/` with `/logos/`. There are four occurrences at lines 135, 163, 180, and 424.

Run to confirm:
```bash
grep -n "static/logos" ui/frontend/src/components/games/game-card.test.tsx
```
Expected: no output (all replaced).

- [ ] **Step 2: Update `/static/logos/` → `/logos/` in `game-list.test.tsx`**

In `ui/frontend/src/components/games/game-list.test.tsx`, replace all occurrences of `/static/logos/` with `/logos/`. There are four occurrences at lines 343, 374, 391, and 464.

Run to confirm:
```bash
grep -n "static/logos" ui/frontend/src/components/games/game-list.test.tsx
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/games/game-card.test.tsx \
  ui/frontend/src/components/games/game-list.test.tsx
git commit -m "fix: update test fixture logo paths from /static/logos/ to /logos/"
```

---

## Task 10: Frontend — create `use-health-status.ts` (TDD)

**Files:**
- Create: `ui/frontend/src/hooks/use-health-status.test.ts`
- Create: `ui/frontend/src/hooks/use-health-status.ts`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/hooks/use-health-status.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { useHealthStatus } from './use-health-status';

describe('useHealthStatus', () => {
  it('returns igdb_configured: true from health endpoint', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({ status: 'ok', igdb_configured: true, backup_available: false })
      )
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.igdb_configured).toBe(true);
  });

  it('returns igdb_configured: false when IGDB is not configured', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({ status: 'ok', igdb_configured: false, backup_available: false })
      )
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.igdb_configured).toBe(false);
  });
});
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
cd ui/frontend && npm run test use-health-status.test.ts 2>&1 | tail -15
```

Expected: FAIL — module not found `./use-health-status`.

- [ ] **Step 3: Implement `use-health-status.ts`**

Create `ui/frontend/src/hooks/use-health-status.ts`:

```ts
import { useQuery } from '@tanstack/react-query';

export interface HealthStatus {
  status: string;
  igdb_configured: boolean;
  backup_available: boolean;
}

export function useHealthStatus() {
  return useQuery<HealthStatus>({
    queryKey: ['health'],
    queryFn: () => fetch('/health').then((r) => r.json() as Promise<HealthStatus>),
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  });
}
```

Note: `/health` is served by the Go binary without an `/api` prefix. Using `fetch('/health')` directly (not `api.get`) avoids the `/api` base-URL prefix applied by the `apiCall` helper. In Vite dev mode, add `/health` to the proxy config in `vite.config.ts` if needed; in production (Go binary), it resolves directly.

- [ ] **Step 4: Run the test and confirm it passes**

```bash
cd ui/frontend && npm run test use-health-status.test.ts 2>&1 | tail -10
```

Expected: 2 tests PASS.

- [ ] **Step 5: Export the hook from `src/hooks/index.ts`**

Open `ui/frontend/src/hooks/index.ts` and add:
```ts
export { useHealthStatus } from './use-health-status';
export type { HealthStatus } from './use-health-status';
```

- [ ] **Step 6: Commit**

```bash
cd .. && git add ui/frontend/src/hooks/use-health-status.ts \
  ui/frontend/src/hooks/use-health-status.test.ts \
  ui/frontend/src/hooks/index.ts
git commit -m "feat: add useHealthStatus hook for IGDB availability"
```

---

## Task 11: Frontend — IGDB banner in authenticated layout (TDD)

**Files:**
- Create: `ui/frontend/src/routes/_authenticated.test.tsx`
- Modify: `ui/frontend/src/routes/_authenticated.tsx`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/routes/_authenticated.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AuthenticatedLayout } from './_authenticated';

const mockUseHealthStatus = vi.fn();
vi.mock('@/hooks/use-health-status', () => ({
  useHealthStatus: () => mockUseHealthStatus(),
}));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Outlet: () => null,
}));

vi.mock('@/components/route-guard', () => ({
  RouteGuard: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('@/components/navigation', () => ({
  Sidebar: () => null,
  MobileNav: () => null,
}));

describe('AuthenticatedLayout IGDB banner', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows IGDB warning banner when igdb_configured is false', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_configured: false } });
    render(<AuthenticatedLayout />);
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText(/IGDB is not configured/)).toBeInTheDocument();
  });

  it('does not show IGDB banner when igdb_configured is true', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_configured: true } });
    render(<AuthenticatedLayout />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('does not show IGDB banner while health data is loading', () => {
    mockUseHealthStatus.mockReturnValue({ data: undefined });
    render(<AuthenticatedLayout />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
cd ui/frontend && npm run test _authenticated.test.tsx 2>&1 | tail -15
```

Expected: FAIL — `AuthenticatedLayout` is not an export (currently unexported), and no `role="alert"` element exists.

- [ ] **Step 3: Update `_authenticated.tsx` — export component and add IGDB banner**

Replace `ui/frontend/src/routes/_authenticated.tsx` with:

```tsx
import { createFileRoute, Outlet } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import { Sidebar, MobileNav } from '@/components/navigation';
import { useHealthStatus } from '@/hooks/use-health-status';

export const Route = createFileRoute('/_authenticated')({
  component: AuthenticatedLayout,
});

export function AuthenticatedLayout() {
  const { data: health } = useHealthStatus();

  return (
    <RouteGuard>
      <div className="flex min-h-screen flex-col md:flex-row">
        <MobileNav />
        <Sidebar />
        <div className="flex-1 flex flex-col md:ml-64">
          {health?.igdb_configured === false && (
            <div
              role="alert"
              className="bg-amber-50 border-b border-amber-200 px-6 py-3 text-sm text-amber-800 dark:bg-amber-950 dark:border-amber-800 dark:text-amber-200"
            >
              <strong>IGDB is not configured</strong> — game search and scheduled metadata refresh
              are unavailable. An administrator needs to set{' '}
              <code className="font-mono">IGDB_CLIENT_ID</code> and{' '}
              <code className="font-mono">IGDB_CLIENT_SECRET</code>.
            </div>
          )}
          <main className="flex-1 p-6 overflow-auto">
            <Outlet />
          </main>
        </div>
      </div>
    </RouteGuard>
  );
}
```

- [ ] **Step 4: Run the test and confirm it passes**

```bash
cd ui/frontend && npm run test _authenticated.test.tsx 2>&1 | tail -10
```

Expected: 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
cd .. && git add ui/frontend/src/routes/_authenticated.tsx \
  ui/frontend/src/routes/_authenticated.test.tsx
git commit -m "feat: add IGDB unavailability banner to authenticated layout"
```

---

## Task 12: Frontend — IGDB disabled states and 503 handling (TDD)

**Files:**
- Create: `ui/frontend/src/routes/_authenticated/games/add.test.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/games/add.index.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx`
- Modify: `ui/frontend/src/components/games/igdb-search.tsx`

- [ ] **Step 1: Write the failing test for `add.index.tsx`**

Create `ui/frontend/src/routes/_authenticated/games/add.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AddGamePage } from './add.index';

const mockUseHealthStatus = vi.fn();
vi.mock('@/hooks/use-health-status', () => ({
  useHealthStatus: () => mockUseHealthStatus(),
}));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
  useNavigate: () => vi.fn(),
}));

vi.mock('@/components/games/igdb-search', () => ({
  IGDBSearch: ({ disabled }: { disabled?: boolean }) => (
    <div data-testid="igdb-search" aria-disabled={String(disabled ?? false)} />
  ),
}));

describe('AddGamePage IGDB disabled state', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('disables IGDB search when igdb_configured is false', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_configured: false } });
    render(<AddGamePage />);
    expect(screen.getByTestId('igdb-search')).toHaveAttribute('aria-disabled', 'true');
  });

  it('enables IGDB search when igdb_configured is true', () => {
    mockUseHealthStatus.mockReturnValue({ data: { igdb_configured: true } });
    render(<AddGamePage />);
    expect(screen.getByTestId('igdb-search')).toHaveAttribute('aria-disabled', 'false');
  });
});
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
cd ui/frontend && npm run test add.test.tsx 2>&1 | tail -15
```

Expected: FAIL — `AddGamePage` is not an export and IGDB search is never disabled.

- [ ] **Step 3: Update `add.index.tsx` — export component, add IGDB disabled state**

Replace `ui/frontend/src/routes/_authenticated/games/add.index.tsx` with:

```tsx
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { ArrowLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { IGDBSearch } from '@/components/games/igdb-search';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useHealthStatus } from '@/hooks/use-health-status';
import type { IGDBGameCandidate } from '@/types';

export const SELECTED_GAME_STORAGE_KEY = 'nexorious_selected_game';

export const Route = createFileRoute('/_authenticated/games/add/')({
  component: AddGamePage,
});

export function AddGamePage() {
  const navigate = useNavigate();
  const { data: health } = useHealthStatus();
  const igdbUnavailable = health?.igdb_configured === false;

  const handleGameSelect = (game: IGDBGameCandidate) => {
    sessionStorage.setItem(SELECTED_GAME_STORAGE_KEY, JSON.stringify(game));
    navigate({ to: '/games/add/confirm', search: { igdb_id: String(game.igdb_id) } });
  };

  const search = (
    <IGDBSearch
      onSelect={handleGameSelect}
      autoFocus
      placeholder="Search for a game to add..."
      disabled={igdbUnavailable}
    />
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link to="/games">
            <ArrowLeft className="h-4 w-4" />
            <span className="sr-only">Back to library</span>
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold">Add Game</h1>
          <p className="text-muted-foreground">
            Search IGDB to find and add a game to your library
          </p>
        </div>
      </div>

      <div className="max-w-2xl">
        {igdbUnavailable ? (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div>{search}</div>
              </TooltipTrigger>
              <TooltipContent>IGDB not configured</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        ) : (
          search
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run the test and confirm it passes**

```bash
cd ui/frontend && npm run test add.test.tsx 2>&1 | tail -10
```

Expected: 2 tests PASS.

- [ ] **Step 5: Update `maintenance.tsx` — disable IGDB refresh button when unconfigured**

In `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx`:

1. Add to imports (after existing imports):
```tsx
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useHealthStatus } from '@/hooks/use-health-status';
```

2. Add inside `MaintenancePage` function body (before the return), after existing hooks:
```tsx
  const { data: health } = useHealthStatus();
  const igdbUnavailable = health?.igdb_configured === false;
```

3. Find the IGDB Data Refresh `<Button>` (currently line 244) and replace the button with a conditional wrapper. Replace:
```tsx
            <Button onClick={handleStartMetadataRefresh} disabled={isRefreshLoading} className="w-full">
              {isRefreshLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Starting...
                </>
              ) : (
                <>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Refresh All Game Metadata
                </>
              )}
            </Button>
```

with:
```tsx
            {igdbUnavailable ? (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <div className="w-full">
                      <Button disabled className="w-full">
                        <RefreshCw className="mr-2 h-4 w-4" />
                        Refresh All Game Metadata
                      </Button>
                    </div>
                  </TooltipTrigger>
                  <TooltipContent>IGDB not configured</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            ) : (
              <Button onClick={handleStartMetadataRefresh} disabled={isRefreshLoading} className="w-full">
                {isRefreshLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Starting...
                  </>
                ) : (
                  <>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    Refresh All Game Metadata
                  </>
                )}
              </Button>
            )}
```

- [ ] **Step 6: Add 503 error handling to `igdb-search.tsx`**

In `ui/frontend/src/components/games/igdb-search.tsx`:

1. Add to imports:
```tsx
import { toast } from 'sonner';
import { ApiErrorException } from '@/api/client';
```

2. In the `IGDBSearch` function body, change the `useSearchIGDB` destructure to include `error`:
```tsx
  const { data: results, isLoading, isFetching, error } = useSearchIGDB(debouncedQuery);
```

3. Add a `useEffect` right after that line to handle 503:
```tsx
  React.useEffect(() => {
    if (error instanceof ApiErrorException && error.status === 503) {
      toast.error('IGDB is currently unavailable');
    }
  }, [error]);
```

- [ ] **Step 7: Commit**

```bash
cd .. && git add \
  "ui/frontend/src/routes/_authenticated/games/add.index.tsx" \
  "ui/frontend/src/routes/_authenticated/games/add.test.tsx" \
  "ui/frontend/src/routes/_authenticated/admin/maintenance.tsx" \
  "ui/frontend/src/components/games/igdb-search.tsx"
git commit -m "feat: disable IGDB UI when unconfigured; handle 503 in search"
```

---

## Task 13: Final verification

**Files:** none

- [ ] **Step 1: Run the full frontend type check and test suite**

```bash
cd ui/frontend && npm run check && npm run test 2>&1 | tail -30
```

Expected: zero TypeScript errors; all tests pass.

- [ ] **Step 2: Run the full Go test suite**

```bash
go test -timeout 300s ./... 2>&1 | tail -20
```

Expected: all packages PASS.

- [ ] **Step 3: Do a full production build to verify embedding works**

```bash
make 2>&1 | tail -10
```

Expected: frontend builds into `ui/frontend/dist/`, then Go binary compiles successfully embedding the dist.

- [ ] **Step 4: Confirm no `/static/logos/` references remain in the frontend source**

```bash
grep -r "static/logos" ui/frontend/src/ --include="*.ts" --include="*.tsx"
```

Expected: no output.

- [ ] **Step 5: Confirm no `total_wishlist_items` references remain in the frontend source**

```bash
grep -r "total_wishlist_items" ui/frontend/src/
```

Expected: no output.

---

## Self-review against spec

### Spec coverage check

| Spec section | Covered by task |
|---|---|
| File copy (src/, index.html, configs) | Task 1 |
| Exclude public/ SVGs, keep logos | Task 1 (move logos, not copy public/) |
| Logo move ui/public → ui/frontend/public | Task 1 step 4 |
| Makefile `frontend` target | Task 3 |
| ui/ui.go embed path | Task 3 |
| router.go spaHandler path | Task 3 |
| package.json name → nexorious-ui | Task 4 |
| .gitignore entries | Task 2 |
| Backend icon_url (platforms + storefronts) | Tasks 5–6 |
| Frontend /static/logos/ → /logos/ in sync.ts | Task 7 |
| Frontend placeholder text in platforms.tsx | Task 7 |
| Wishlist removal (admin.ts, $id.tsx, admin.test.ts) | Task 8 |
| Import-export wishlist copy removal | Task 8 |
| game-card.test.tsx logo paths | Task 9 |
| game-list.test.tsx logo paths | Task 9 |
| useHealthStatus hook | Task 10 |
| IGDB banner in _authenticated.tsx | Task 11 |
| Add game IGDB disabled state | Task 12 |
| Maintenance IGDB button disabled | Task 12 |
| 503 handling in igdb-search.tsx | Task 12 |
| use-health-status.test.ts | Task 10 |
| _authenticated.test.tsx | Task 11 |
| add.test.tsx | Task 12 |
| npm run check && npm run test gate | Task 13 |

### Gaps found

- **`src/routeTree.gen.ts` must exist before type-checking.** Task 4 step 3 runs `npm run build` which triggers the TanStack Router Vite plugin to generate it. This is done before any type-checking steps.
- **`defaultStorefrontResponse` type.** Updated in Task 6 step 2 to use `*storefrontResponse`.
- **`use-health-status` export from `hooks/index.ts`.** Added in Task 10 step 5.
