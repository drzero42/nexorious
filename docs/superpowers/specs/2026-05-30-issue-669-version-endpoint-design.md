# Design: Version Endpoint and Web UI Display (Issue #669)

## Overview

Expose the running server version in the web UI so users can see what version they are running without leaving the browser. A new `GET /api/version` endpoint serves the version and commit hash with aggressive caching headers; the React SPA fetches it once and displays the version string in the sidebar footer.

## Backend

### New endpoint: `GET /api/version`

- **Public** — no authentication required (consistent with `/health`)
- **Response body:**
  ```json
  { "version": "0.3.1", "commit": "abc1234" }
  ```
- **Cache-Control:** `public, max-age=3600` (1 hour). The version is static for the lifetime of a running server process; on deploy, the new process serves the updated version.
- **Route placement:** Registered alongside `/health`, before the `if db != nil` block. The endpoint is available in Ready state; during DB-unavailable or migration-pending states the gate middleware redirects it like any other `/api/*` route. No gate allowlist exemption is needed — the sidebar that displays the version is only rendered in the authenticated (Ready) layout, so the redirect paths are never reached in practice.

### Signature change to `api.New()`

```go
func New(..., version, commit string) *echo.Echo
```

`version` and `commit` are the ldflags-injected build-time vars from `package main`. They flow through `New` → `registerRoutes` and are closed over in the handler. This follows the same pattern as `backup.NewService(..., version)`.

### Handler (inline in `router.go`)

```go
e.GET("/api/version", func(c *echo.Context) error {
    c.Response().Header().Set("Cache-Control", "public, max-age=3600")
    return c.JSON(http.StatusOK, map[string]string{
        "version": version,
        "commit":  commit,
    })
})
```

### Wire-up in `serve.go`

The `api.New(...)` call in `runServe` gains `version, commit` as the last two arguments.

### Slumber collection

Add a new `version/` folder in `slumber.yaml` with `GET {{base_url}}/api/version` (no auth required).

## Frontend

### New hook: `use-version.ts`

Location: `ui/frontend/src/hooks/use-version.ts`

```ts
export interface VersionInfo {
  version: string;
  commit: string;
}

export function useVersion() {
  return useQuery<VersionInfo>({
    queryKey: ['version'],
    queryFn: () => fetch('/api/version').then(r => r.json()),
    staleTime: 60 * 60 * 1000,   // 1 hour — matches server Cache-Control
    gcTime: 24 * 60 * 60 * 1000, // 24 hours
  });
}
```

Export `useVersion` from `ui/frontend/src/hooks/index.ts`.

### Sidebar footer

Location: `ui/frontend/src/components/navigation/sidebar.tsx`

Below the existing user `DropdownMenu` block, add:

```tsx
{version?.version && (
  <div className="px-2 pb-1 text-xs text-muted-foreground">
    v{version.version}
  </div>
)}
```

Only the semver version string is shown in the UI (not the commit hash). The commit hash is available via the API for power users and ops scripts.

## Testing

### Go

Add a test in `internal/api/router_test.go`:
- `GET /api/version` returns 200
- Response body contains correct `version` and `commit` fields
- Response includes `Cache-Control: public, max-age=3600` header

### Frontend

No new frontend test required. `useVersion` is a thin TanStack Query wrapper with no business logic. The existing `use-health-status.test.ts` serves as the pattern if a test is ever desired.

## Out of Scope

- No mobile nav version display (sidebar only; mobile nav is a hamburger menu)
- No `build_date` or other build metadata fields
- No version display on the `/migrate`, `/db-error`, or `/setup` pages
