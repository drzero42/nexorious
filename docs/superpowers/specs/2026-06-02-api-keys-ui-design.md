# API Keys management UI (#732)

## Summary

Add a web UI for users to manage their API keys (list, create, revoke) from the
profile page. **Frontend-only** — the backend CRUD surface already exists in
`internal/api/auth.go`, is wired in `router.go`, and is covered by `slumber.yaml`.
No model, migration, route, or slumber changes are needed.

## Backend (already done — for reference)

| Endpoint | Behaviour |
|---|---|
| `GET /api/auth/api-keys` | List. Returns `id, name, scopes, last_used_at, created_at, expires_at`. Filters `revoked_at IS NULL` only — so **expired-but-not-revoked** keys are still returned. |
| `POST /api/auth/api-keys` | Create. Body `{ name, scopes, expires_at }`. Returns `createAPIKeyResponse` including the raw `key` **exactly once**. |
| `DELETE /api/auth/api-keys/:id` | Revoke (soft — sets `revoked_at`). 404 if not found / already revoked. |

Server-side validation: `name` required; `scopes ∈ {read, write}` (defaults to
`write`); optional `expires_at` parsed as RFC3339 (400 otherwise).

## Structure decision

The issue (written before #514) suggested inlining an API Keys `Card` directly in
`profile.tsx`. Since then, #514 (notifications) established a cleaner precedent:
extract the area into `components/notifications/notifications-section.tsx` + a
separate dialog component + a `use-notifications.ts` hook + `api/notifications.ts`.

**#732 follows the #514 precedent.** `profile.tsx` stays thin and just renders
`<ApiKeysSection/>`.

## Files

### 1. `ui/frontend/src/api/auth.ts` (extend)

Add interfaces and three functions, mirroring the existing `api.get/post/delete`
style in this file:

```ts
export interface ApiKey {
  id: string;
  name: string;
  scopes: 'read' | 'write';
  last_used_at: string | null;
  created_at: string;
  expires_at: string | null;
}
export interface CreatedApiKey extends ApiKey {
  key: string; // raw value, shown exactly once
}

export function listApiKeys(): Promise<ApiKey[]>;            // GET    /auth/api-keys
export function createApiKey(body: {
  name: string;
  scopes: 'read' | 'write';
  expires_at: string | null;
}): Promise<CreatedApiKey>;                                   // POST   /auth/api-keys
export function revokeApiKey(id: string): Promise<void>;     // DELETE /auth/api-keys/:id
```

Note: `CreatedApiKey` extends `ApiKey` but the create response does not include
`last_used_at`; model `last_used_at` as optional/absent on the create path (it is
always `null` for a brand-new key) — only the list view reads it.

### 2. `ui/frontend/src/hooks/use-api-keys.ts` (new)

Mirror `use-notifications.ts`:

- `apiKeysKeys = { all: ['api-keys'] as const, list: () => [...apiKeysKeys.all, 'list'] as const }`
- `useApiKeys()` — list query
- `useCreateApiKey()` — mutation; invalidates the list `onSuccess`
- `useRevokeApiKey()` — mutation; invalidates the list `onSuccess`

Re-export from `ui/frontend/src/hooks/index.ts`.

### 3. `ui/frontend/src/components/api-keys/api-keys-section.tsx` (new)

`<ApiKeysSection/>` — a `Card`:

- Header: title + "New API key" button (opens the create dialog).
- Loading: `Skeleton` rows.
- Empty state: "No API keys yet".
- One row per key: **Name · scope `Badge` · Last used (relative; "Never used" if
  `last_used_at` is null) · Created · Expires**. If `expires_at` is in the past,
  render a distinct red **Expired** badge to prompt the user to revoke it.
- Per-row **Revoke** button → `AlertDialog` confirm → `useRevokeApiKey()` →
  success/error `toast`.

### 4. `ui/frontend/src/components/api-keys/create-api-key-dialog.tsx` (new)

A `Dialog` with two internal states:

**Form state:**
- Name — required `Input`.
- Scopes — `Select`: "Read & write" (`write`, default) / "Read only" (`read`).
- Expiry — `Select`: 30 days / 90 days / 365 days / Never.
- On submit: convert the preset to an RFC3339 string client-side (`null` for
  Never) via `expiryPresetToRFC3339`, call `useCreateApiKey()`. Surface backend
  400s (bad scope / missing name / bad RFC3339) as error `toast`s.

**Reveal state (on success):**
- Swap the dialog body to show the raw `key` (read-only) with a **Copy** button
  (`navigator.clipboard.writeText` + `toast`) and an unmissable warning:
  *"Copy this now — it won't be shown again."*
- On dialog close, the raw key is cleared from component state and removed from
  the DOM. The list refreshes underneath via the mutation's invalidation.

### 5. `ui/frontend/src/routes/_authenticated/profile.tsx` (edit)

Import and render `<ApiKeysSection/>` in the main column, between
`<NotificationsSection/>` and the Danger Zone `Card`.

## Logic to isolate

`expiryPresetToRFC3339(preset, now)` — pure helper (the only real logic). Takes a
preset (`'30' | '90' | '365' | 'never'`) and a base time; returns an RFC3339
string for day-presets and `null` for `'never'`. Accept `now` as a parameter so
the unit test is deterministic.

## Testing (per repo policy — only the non-trivial bits)

- **Unit:** `expiryPresetToRFC3339` — each day-preset → correct RFC3339 offset
  from a fixed `now`; `'never'` → `null`.
- **Component:** the "shown exactly once" reveal invariant — after a successful
  create the raw key is visible and the Copy button writes to the clipboard; after
  the dialog closes the key is no longer in the DOM. This is the security-relevant
  invariant worth pinning.

Straightforward wiring (list rows, query hooks, revoke confirm) does not need
dedicated tests.

## Out of scope

No backend changes, no new migration, no route changes, no `slumber.yaml` changes.
