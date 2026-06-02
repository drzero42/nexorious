# Admin Activity / Events View — Design

**Issue:** #734 (depends on #514, which landed the `events` table, `internal/notify`
registry, and formatters).

## Goal

Give admins a read-only view of what the system has been doing — sync runs,
imports/exports, scheduled backups, and maintenance tasks — without tailing logs.
This is a **read surface over the existing `events` table**: no new instrumentation,
no new event types, no new emit sites.

## Substrate (already exists, from #514)

- **`events` table:** `id, type, scope, actor_user_id, payload (jsonb), dedup_key,
  occurred_at`. Indexes: `events_occurred_at_idx`, `events_actor_user_id_idx`,
  unique `events_dedup_key_idx`. `actor_user_id` is `REFERENCES users(id) ON DELETE
  SET NULL` — admin/system events (backup, maintenance) carry no actor.
- **`internal/notify` registry:** `Registry() []EventTypeMeta{Type, Scope, Category,
  Label, DefaultOn}`, `Meta(type) (EventTypeMeta, bool)`, scope constants
  `ScopeUser` / `ScopeAdmin`.
- **`internal/notify` formatter:** `Format(eventType string, payload json.RawMessage)
  (title, body string)`.
- **Admin plumbing:** `adminGroup := e.Group("", auth.AuthMiddleware(db),
  auth.AdminMiddleware())` already in `router.go`.
- **Registry exposure:** `GET /api/notifications/event-types` already returns the full
  `notify.Registry()` to any authenticated user.

## Decisions on the issue's open questions

1. **Visibility — admin-only for the first cut.** Built so a future user self-view is a
   thin add-on (force `scope=user AND actor_user_id=<me>`, drop `AdminMiddleware`); no
   schema change needed to get there. Not in scope now.
2. **Pagination — keyset/cursor on `(occurred_at, id)` DESC.** The events table grows
   unbounded between prunes; an append-only, newest-first feed is the textbook keyset
   case (stable under concurrent inserts, no `COUNT`, no deep-offset cost). This
   deliberately diverges from the offset/`page` pattern in `HandleListJobs`, which is
   appropriate there (small, per-user table) but not here. Trade-off accepted: no total
   count and no jump-to-arbitrary-page — neither fits a skim-from-newest activity feed.
3. **Retention — out of scope.** The events-prune maintenance job from #514 owns the
   window (`NOTIFY_EVENTS_RETENTION_DAYS`). A later admin-settings control could surface
   it; this view does not own pruning.

## Backend

**New file `internal/api/events.go`** → `EventsHandler{db *bun.DB}`, registered on the
existing `adminGroup` in `router.go`.

### `GET /api/admin/events`

Query params (all AND-combined via Bun `.Where()` chaining):

| Param | Meaning |
|---|---|
| `limit` | page size, default 50, clamped to 200 |
| `before` | opaque cursor encoding `(occurred_at, id)`, base64'd |
| `type` | exact event type |
| `category` | registry category (e.g. `Sync`, `Backups`) |
| `scope` | `user` / `admin` |
| `user` | matches `actor_user_id` exactly OR username substring (via join) |
| `since` / `until` | optional RFC3339 range on `occurred_at` |

- `LEFT JOIN users u ON u.id = e.actor_user_id` to resolve `actor_username` (nullable).
- Order `occurred_at DESC, id DESC`. Fetch `LIMIT limit+1` to detect whether a further
  page exists; if the extra row comes back, drop it and emit `next_cursor` from the last
  returned row, else `next_cursor` is null.
- Keyset predicate when `before` is present: `WHERE (e.occurred_at, e.id) < (?, ?)`
  using the decoded cursor tuple (row-value comparison, matching the
  `occurred_at DESC, id DESC` order).

**Cursor encoding:** base64 of `<occurred_at RFC3339Nano>|<id>`. Decode validates both
parts; a malformed cursor returns `400`.

**Response:**
```json
{
  "events": [
    {
      "id": "…",
      "type": "sync.completed",
      "category": "Sync",
      "scope": "user",
      "occurred_at": "2026-06-02T10:00:00Z",
      "actor_user_id": "…",        // null for system/admin events
      "actor_username": "alice",   // null for system/admin events
      "title": "Sync completed",   // from notify.Format
      "body": "…",                 // from notify.Format (may be multi-line)
      "payload": { }               // raw jsonb, for the expandable detail
    }
  ],
  "next_cursor": "…"               // null when exhausted
}
```

`title`/`body` come from `notify.Format(type, payload)`; `category`/`scope` from
`notify.Meta(type)` (fall back gracefully if a type is unknown — empty category/scope,
title defaults handled by `Format`). Raw `payload` is retained for the UI detail row.

**No `/api/admin/events/meta` endpoint.** The frontend reuses
`GET /api/notifications/event-types` (already returns the registry with
type/scope/category/label) and derives the category/scope filter options client-side.

## Migration

New migration pair (next in sequence after `20260601000004_*`):

`internal/db/migrations/20260602000001_events_keyset_index.up.sql`
```sql
CREATE INDEX events_occurred_at_id_idx ON events (occurred_at DESC, id DESC);
```

`internal/db/migrations/20260602000001_events_keyset_index.down.sql`
```sql
DROP INDEX IF EXISTS events_occurred_at_id_idx;
```

The existing `events_actor_user_id_idx` covers the user filter. No `type`/`scope`
indexes until filtering proves hot.

## Frontend

- **Route** `ui/frontend/src/routes/_authenticated/admin/activity/index.tsx`, admin-gated
  with the established guard: `if (!currentUser?.isAdmin) navigate({ to: '/dashboard' })`.
  Add a nav entry from the admin dashboard (`admin/index.tsx`).
- **`src/api/events.ts`** — `eventsApi.list(params)` wrapping
  `api.get('/admin/events', { params })`; a query-key factory like `jobsKeys`; TS types
  for the event DTO and the list response.
- **`useAdminEvents`** hook using `useInfiniteQuery`, `getNextPageParam → next_cursor`.
- **Table** (reuse shadcn `Table` from `admin/users`): columns
  - **Type** — `label` + category badge
  - **When** — relative time, absolute timestamp on hover
  - **User** — `actor_username`, or `—` for system/admin events
  - **Detail** — `title`/`body` summary; row expands to full formatted body +
    collapsible raw `payload`. `body` can be multi-line; truncate in the row, show full
    in the expanded detail.
- **Filter bar** — category/type Select + scope Select (options driven by
  `event-types`), a user search input, optional date range. Filters are server-side
  query params; changing any filter resets the cursor and refetches from the top.
  Controlled Selects use `useState` (not RHF `watch()`), per the project's React
  Compiler convention — this is a filter bar, not a form submission.

## Testing

- **Backend (`internal/api/events_test.go`, shared `testDB`, `truncateAllTables(t)`):**
  - Keyset paging correctness: cursor returns strictly older rows, no duplicates or
    skips across pages; `next_cursor` null on the final page.
  - Each filter: `type`, `category`, `scope`, `user` (by id and by username substring),
    `since`/`until` range.
  - `actor_username` join resolves for user events and is null for system/admin events
    (backup, maintenance).
  - Admin-only gating: 401 unauthenticated, 403 for a non-admin user.
  - Cursor encode/decode round-trip; malformed cursor → 400.
- **Frontend:** filter → query-param mapping; infinite-scroll page accumulation;
  expand/collapse of the detail row.

## Slumber

Add an `admin/events/` folder (alphabetical order) with the `GET /api/admin/events`
request, using the bearer-auth block. Run `slumber collection` to verify it loads.

## Non-goals

- No new event types or emit sites — purely read.
- No retention/pruning control (owned by the #514 maintenance job).
- No user self-view (`My activity`) — future thin add-on.
- No total-count or jump-to-page (consequence of keyset; not wanted for a feed).
