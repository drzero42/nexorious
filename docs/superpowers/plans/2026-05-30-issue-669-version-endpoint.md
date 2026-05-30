# Version Endpoint and Web UI Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /api/version` (public, cached) and display the server version in the sidebar footer.

**Architecture:** `version` and `commit` build-time ldflags vars in `package main` are threaded into `api.New()` as two new positional `string` params (before the existing variadic `riverClient`); the handler closes over them and sets a `Cache-Control: public, max-age=3600` header. The React SPA fetches the version once via a new `useVersion` TanStack Query hook and renders it in the sidebar footer below the user menu.

**Tech Stack:** Go / Echo v5, React 19, TanStack Query, Tailwind CSS v4, Slumber

---

## File Map

| Action | File | What changes |
|--------|------|--------------|
| Modify | `internal/api/router.go` | Extend `New()` and `registerRoutes()` with `version, commit string`; add `/api/version` handler |
| Modify | `internal/api/router_test.go` | Add `TestVersionEndpoint`; update every `api.New()` call to include `"dev", "unknown"` |
| Modify | `internal/api/auth_test.go` | Update every `api.New()` call |
| Modify | `internal/api/backup_test.go` | Update `api.New()` call |
| Modify | `internal/api/games_test.go` | Update `api.New()` calls |
| Modify | `cmd/nexorious/serve.go` | Pass `version, commit` to `api.New()` |
| Create | `ui/frontend/src/hooks/use-version.ts` | New TanStack Query hook |
| Modify | `ui/frontend/src/hooks/index.ts` | Export `useVersion` and `VersionInfo` |
| Modify | `ui/frontend/src/components/navigation/sidebar.tsx` | Render version below user menu |
| Modify | `slumber.yaml` | Add `version` folder with `GET /api/version` request |

---

### Task 1: Backend — `/api/version` endpoint (TDD)

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/router_test.go`
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/backup_test.go`
- Modify: `internal/api/games_test.go`
- Modify: `cmd/nexorious/serve.go`

- [ ] **Step 1: Write the failing test**

Append this function at the bottom of `internal/api/router_test.go`:

```go
func TestVersionEndpoint(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "1.2.3", "abc1234")

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if cc != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q, want %q", cc, "public, max-age=3600")
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["version"] != "1.2.3" {
		t.Errorf("version = %q, want %q", body["version"], "1.2.3")
	}
	if body["commit"] != "abc1234" {
		t.Errorf("commit = %q, want %q", body["commit"], "abc1234")
	}
}
```

- [ ] **Step 2: Verify the test fails to compile**

```bash
go test ./internal/api/... -run TestVersionEndpoint -v
```

Expected: compilation error — `api.New` called with wrong number of arguments (10 vs 8). This is the expected "red" state.

- [ ] **Step 3: Update `api.New()` signature in `router.go`**

In `internal/api/router.go`, change the `New` function signature from:

```go
func New(encrypter *crypto.Encrypter, cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
```

to:

```go
func New(encrypter *crypto.Encrypter, cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, version, commit string, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
```

- [ ] **Step 4: Pass `version, commit` through to `registerRoutes` in `router.go`**

Change the `registerRoutes` function signature from:

```go
func registerRoutes(e *echo.Echo, encrypter *crypto.Encrypter, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient *river.Client[pgx.Tx]) {
```

to:

```go
func registerRoutes(e *echo.Echo, encrypter *crypto.Encrypter, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, version, commit string, riverClient *river.Client[pgx.Tx]) {
```

Update the call to `registerRoutes` inside `New()` — add `version, commit` before `rc`:

```go
registerRoutes(e, encrypter, cfg, mh, db, migrator, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, version, commit, rc)
```

- [ ] **Step 5: Add the `/api/version` handler in `registerRoutes`**

In `internal/api/router.go`, inside `registerRoutes`, add the following block immediately after the `/health` handler block (after the closing `})` of the health handler):

```go
	// Version — public, aggressively cached
	e.GET("/api/version", func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		return c.JSON(http.StatusOK, map[string]string{
			"version": version,
			"commit":  commit,
		})
	})
```

- [ ] **Step 6: Update all `api.New()` call sites in test files**

There are many existing test callsites that pass 8 positional args (no `riverClient`) or 9 (with `riverClient`). Insert `"dev", "unknown"` as the 9th and 10th args (i.e. before the optional `riverClient`).

**`internal/api/router_test.go`** — 18 calls, all have the same 8-arg pattern. Replace every occurrence of:

```go
api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil)
```
with:
```go
api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil, "dev", "unknown")
```

And replace:
```go
api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil)
```
with:
```go
api.New(testEncrypter, testCfg(), migrator, nil, "", nil, nil, nil, "dev", "unknown")
```

And replace the two `igdbClient` variants:
```go
api.New(testEncrypter, cfg, migrator, nil, "", igdbClient, nil, nil)
```
with:
```go
api.New(testEncrypter, cfg, migrator, nil, "", igdbClient, nil, nil, "dev", "unknown")
```

And:
```go
api.New(testEncrypter, testCfg(), migrator, nil, "", igdbClient, nil, nil)
```
with:
```go
api.New(testEncrypter, testCfg(), migrator, nil, "", igdbClient, nil, nil, "dev", "unknown")
```

**`internal/api/auth_test.go`** — 3 calls:

Replace:
```go
api.New(testEncrypter, cfg, m, db, "", nil, nil, nil)
```
with (two occurrences at lines 71 and 277):
```go
api.New(testEncrypter, cfg, m, db, "", nil, nil, nil, "dev", "unknown")
```

Replace (line 83, has trailing `rc`):
```go
api.New(testEncrypter, cfg, m, db, "", nil, nil, nil, rc)
```
with:
```go
api.New(testEncrypter, cfg, m, db, "", nil, nil, nil, "dev", "unknown", rc)
```

**`internal/api/backup_test.go`** — 1 call:

Replace:
```go
api.New(testEncrypter, cfg, m, db, "", nil, svc, nil)
```
with:
```go
api.New(testEncrypter, cfg, m, db, "", nil, svc, nil, "dev", "unknown")
```

**`internal/api/games_test.go`** — 2 calls:

Replace both:
```go
api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil)
```
with:
```go
api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil, "dev", "unknown")
```

- [ ] **Step 7: Wire `version, commit` into `api.New()` in `serve.go`**

In `cmd/nexorious/serve.go`, change:

```go
e := api.New(encrypter, cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, riverClient)
```

to:

```go
e := api.New(encrypter, cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, version, commit, riverClient)
```

- [ ] **Step 8: Run the version endpoint test**

```bash
go test ./internal/api/... -run TestVersionEndpoint -v
```

Expected output: `PASS`

- [ ] **Step 9: Run the full Go test suite**

```bash
go test -timeout 600s ./...
```

Expected: all pass, no compilation errors.

- [ ] **Step 10: Commit**

```bash
git add internal/api/router.go internal/api/router_test.go internal/api/auth_test.go internal/api/backup_test.go internal/api/games_test.go cmd/nexorious/serve.go
git commit -m "feat: add GET /api/version endpoint with 1h cache"
```

---

### Task 2: Slumber collection — add version request

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add the `version` folder to `slumber.yaml`**

In `slumber.yaml`, locate the `health:` folder (around line 290). Add a new `version:` folder immediately after the `health:` block (keeping alphabetical order is not required; placing it next to `health:` is logical):

```yaml
  version:
    name: Version
    requests:
      version:
        name: Get Version
        method: GET
        url: "{{base_url}}/api/version"
```

- [ ] **Step 2: Verify the collection loads**

```bash
slumber collection
```

Expected: no errors; the collection lists the new `version.version` request.

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "chore: add version request to slumber collection"
```

---

### Task 3: Frontend — `useVersion` hook

**Files:**
- Create: `ui/frontend/src/hooks/use-version.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

- [ ] **Step 1: Create `use-version.ts`**

Create `ui/frontend/src/hooks/use-version.ts` with this content:

```ts
import { useQuery } from '@tanstack/react-query';

export interface VersionInfo {
  version: string;
  commit: string;
}

export function useVersion() {
  return useQuery<VersionInfo>({
    queryKey: ['version'],
    queryFn: () => fetch('/api/version').then((r) => r.json() as Promise<VersionInfo>),
    staleTime: 60 * 60 * 1000,
    gcTime: 24 * 60 * 60 * 1000,
  });
}
```

- [ ] **Step 2: Export from `hooks/index.ts`**

In `ui/frontend/src/hooks/index.ts`, append these two lines at the end of the file (after the jobs hooks block):

```ts
// Version hooks
export { useVersion } from './use-version';
export type { VersionInfo } from './use-version';
```

- [ ] **Step 3: Run TypeScript type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/hooks/use-version.ts ui/frontend/src/hooks/index.ts
git commit -m "feat: add useVersion hook"
```

---

### Task 4: Frontend — sidebar version display

**Files:**
- Modify: `ui/frontend/src/components/navigation/sidebar.tsx`

- [ ] **Step 1: Update the sidebar to display the version**

In `ui/frontend/src/components/navigation/sidebar.tsx`:

1. Add `useVersion` to the import from `@/hooks`:

   Change:
   ```tsx
   import { useAuth } from '@/providers';
   ```
   to:
   ```tsx
   import { useVersion } from '@/hooks';
   import { useAuth } from '@/providers';
   ```

2. Inside the `Sidebar` function body, add a call to the hook (alongside the existing `useAuth` and `useNavItems` calls):

   ```tsx
   const { data: versionInfo } = useVersion();
   ```

3. After the closing `</div>` of the user menu section (the `<div className="p-4 border-t">` block), add a new version footer div. The complete bottom of the component should look like:

   ```tsx
       {/* User menu at bottom */}
       <div className="p-4 border-t">
         <DropdownMenu>
           {/* ... existing dropdown content unchanged ... */}
         </DropdownMenu>
       </div>

       {/* Version */}
       {versionInfo?.version && (
         <div className="px-4 pb-3 text-xs text-muted-foreground">
           v{versionInfo.version}
         </div>
       )}
     </aside>
   ```

   The `{versionInfo?.version && ...}` block goes between the closing `</div>` of the user menu section and the closing `</aside>` tag.

- [ ] **Step 2: Run TypeScript type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors.

- [ ] **Step 3: Run knip dead-code check**

```bash
cd ui/frontend && npm run knip
```

Expected: no unused exports.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/navigation/sidebar.tsx
git commit -m "feat: show server version in sidebar footer"
```
