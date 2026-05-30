# Issue #620 — Surface backend `error` detail in API toasts

**Date:** 2026-05-27
**Issue:** [#620](https://github.com/drzero42/nexorious/issues/620) — *auth: 401 toast loses backend error detail; refresh-retry race lacks regression test*
**Status:** Approved — ready for implementation plan
**Milestone:** 0.1.0

## Summary

Issue #620 raises two independent items. This spec covers **item 1 only**; item 2 is
deliberately dropped (see [Scope decision](#scope-decision)).

**Item 1:** the frontend's API error handler never reads the backend's standard `error`
response key, so almost every backend 4xx/5xx renders as a generic
`HTTP <status>: <statusText>` toast. The fix adds an `error` branch to a single function.

## Problem

`handleApiError` in `ui/frontend/src/api/client.ts` extracts the user-visible message from
the response body in this order:

```ts
if (typeof details.detail === 'string')  errorMessage = details.detail;
else if (typeof details.message === 'string') errorMessage = details.message;
// else: HTTP <status>: <statusText>
```

It has **no branch for `details.error`**. An audit of `internal/api/*` and adjacent backend
packages shows the response-body keys actually in use:

| Key       | Occurrences | Nature |
|-----------|-------------|--------|
| `error`   | 99          | The de-facto standard for **error** bodies (incl. the auth middleware in `internal/auth/jwt.go`). |
| `message` | 13          | Almost entirely **success** messages (e.g. "Backup created successfully", "Successfully logged out"); also Echo's default HTTP error handler. |
| `detail`  | 1           | Single use — `internal/middleware/maintenance.go` ("Restore in progress"). |

So the handler reads the two rarely-used keys and ignores the dominant one. The auth 401 in
the issue title is just where the symptom was noticed; the defect is **API-wide**: not-found,
conflict, bad-request, and every other `error`-keyed response falls through to the generic
`HTTP <status>` fallback.

**Impact:** users get an uninformative toast (e.g. `HTTP 401: Unauthorized`,
`HTTP 409: Conflict`); diagnosing the real cause requires opening the network tab.

## Approach

Frontend-only, additive. The issue's "Option 2" (align the backend on a single key) is a
no-op — the backend is already consistent on `error`; the frontend is the side that is out of
sync. The auth middleware (`internal/auth/jwt.go`) already emits `error`, so no backend change
is warranted.

### Change

A single function changes: `handleApiError` in `ui/frontend/src/api/client.ts`. Add a
`typeof details.error === 'string'` branch. This is the sole error-extraction point —
`apiCall`, `apiUploadFile`, and `apiDownloadFile` all funnel through it, so one edit fixes
every request path.

**Precedence:** `detail` → `error` → `message` → default.

```ts
if (typeof details.detail === 'string') {
  errorMessage = details.detail;
} else if (typeof details.error === 'string') {
  errorMessage = details.error;
} else if (typeof details.message === 'string') {
  errorMessage = details.message;
}
```

- `detail` stays first (unchanged — the maintenance "Restore in progress" body).
- `error` is inserted next (the API-wide standard for real errors).
- `message` stays last (Echo's default error handler + legacy).

Precedence is academic in practice: no single backend body carries more than one of these
keys. This order is the least surprising, preserves all current behavior, and only *adds*
`error` handling. **No branches are removed and no backend code changes** — zero risk to
existing endpoints.

## Testing

Add one case to the existing `error handling` describe block in
`ui/frontend/src/api/client.test.ts`:

- Mock a target endpoint to return a 4xx with body `{ error: "<message>" }`.
- Assert the thrown `ApiErrorException.message` equals `<message>` (not the
  `HTTP <status>: <statusText>` fallback).

This documents the fix and pins the regression. The existing `detail` / `message` /
non-ok-response tests stay green and are not modified.

This satisfies the repo testing policy: the change has a clear behavioral edge case (a
previously-unhandled body shape), and the test would have failed on the pre-fix code.

## Scope decision

**Item 2 of #620 (refresh-and-retry regression test) is dropped.**

Issue [#625](https://github.com/drzero42/nexorious/issues/625) ("Replace JWT auth with server-side sessions
and API keys", milestone 0.2.0) removes the entire refresh mechanism that item 2 would test:
`refreshTokensFn`, `refreshPromiseRef`, and `setAuthHandlers` in `AuthProvider`, plus the
refresh-on-401 retry in `client.ts` (new behavior: on 401, redirect to login, no retry). A
regression test guarding "the retry uses the new token after refresh" would test code that
#625 deletes, so it would be removed one milestone later. Its only value is the interim window,
and only if #625 slips.

By contrast, **item 1 survives #625**: #625 keeps `client.ts` and `handleApiError` (it only
strips token/refresh logic), and the backend keeps returning `error` bodies. Fixing item 1 now
also means #625 inherits a correct `handleApiError`.

A comment recording this decision and its reasoning will be posted on issue #620.

## Out of scope

- Item 2 (refresh-retry regression test) — see [Scope decision](#scope-decision).
- Backend key alignment (issue's "Option 2") — backend is already consistent on `error`.
- Any change to `detail` / `message` handling — preserved as-is for backward compatibility
  (maintenance middleware and Echo's default error handler).

## Files touched

| File | Change |
|------|--------|
| `ui/frontend/src/api/client.ts` | Add `error` branch to `handleApiError`. |
| `ui/frontend/src/api/client.test.ts` | Add a test asserting `{ error: ... }` bodies surface in the toast message. |
