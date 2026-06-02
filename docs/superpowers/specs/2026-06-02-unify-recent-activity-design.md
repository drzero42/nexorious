# Unify Recent Activity — one component, one endpoint, one `changes` table

**Issue:** #730 (follow-up to #670 / #722)
**Date:** 2026-06-02
**Status:** Approved — ready for implementation plan

## Goal

Collapse the two divergent **Recent Activity** implementations into a single
component, hook, and read path, and bring imports up to the same rich
per-outcome breakdown that sync already has. This is primarily a **DRY /
code-surface-reduction** pass: the codebase should be smaller and easier to
understand afterward, not larger.

## Current state (the duplication)

| | Sync | Import / Export / Maintenance |
|---|---|---|
| Component | `ui/frontend/src/components/sync/recent-activity.tsx` | `ui/frontend/src/components/jobs/recent-activity.tsx` |
| Hook | `useRecentJobs(source)` | `useJobs()` + client-side date/type/terminal filtering |
| Endpoint | `GET /api/jobs/recent/:source` (reads `sync_changes`) | generic `GET /api/jobs` list |
| Display | rich per-outcome breakdown (Added / Removed / Status changed / Already in library / Skipped) | aggregate completed/failed counts + expandable `JobItemsDetails` |

Two components, two hooks, two read paths, two looks — for one concept.

## Scope decision: keep two tables

We considered serving Recent Activity from the existing append-only `events`
table (#514), which already backs the admin `/admin/activity` log view (#747).
**Rejected** — they are two different kinds of data:

| | `events` (audit / notify) | per-item change rows (Recent Activity) |
|---|---|---|
| Grain | per job (~6 rows / sync) | per game (~200–300 rows / sync) |
| Identity | detail inside `payload` JSONB; no `job_id`/`game_id` columns | `job_id`, `external_game_id`, `change_type` as real columns |
| Retention | 90-day prune | permanent (cascades only on job delete) |
| Purpose | timeline + notification delivery (dedup-key fire-once) | per-game outcome lists, grouped by `change_type` |

A single physical table cannot cleanly be both a coarse 90-day audit/notify log
and a permanent high-volume per-item detail store. We therefore **keep two
tables**; the `events` table and `/admin/activity` are left untouched. This
issue consolidates the *Recent Activity* surface only.

## Design

### 1. Data model — rename `sync_changes` → `changes`

Once non-sync workers write to it, the name `sync_changes` is dishonest. Rename
the table (and its model); columns are unchanged.

Table `changes` (formerly `sync_changes`):

```
id               TEXT PRIMARY KEY
job_id           TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE
user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE
external_game_id TEXT REFERENCES external_games(id) ON DELETE SET NULL
change_type      TEXT NOT NULL
title            TEXT NOT NULL
old_status       TEXT
new_status       TEXT
created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
```

- **Bun model:** `SyncChange` → `JobChange` (`bun:"table:changes"`) in
  `internal/db/models/models.go`.
- **Migration:** `20260602000002_rename_sync_changes_to_changes.up.sql`
  ```sql
  ALTER TABLE sync_changes RENAME TO changes;
  ALTER INDEX sync_changes_job_id_idx     RENAME TO changes_job_id_idx;
  ALTER INDEX sync_changes_user_id_idx    RENAME TO changes_user_id_idx;
  ALTER INDEX sync_changes_created_at_idx RENAME TO changes_created_at_idx;
  ```
  `.down.sql` reverses each statement. A table rename preserves all existing
  rows — no backfill.

### 2. Writers

**Sync** (`internal/worker/tasks/sync.go`) — repoint the existing ~6 raw
`INSERT INTO sync_changes ...` statements to `INSERT INTO changes ...`. The
`change_type` vocabulary is unchanged: `added`, `removed`, `status_changed`,
`skipped`, `already_in_library`. Pure rename, no behavioural change.

**Import** (`internal/worker/tasks/import_item.go`) — *new*. The worker already
computes `alreadyExists` (whether a `user_game` already existed) and, when it
does, merges any new platform/storefront pairs and tags into the existing entry
(it tracks an `existingPlatforms` set and inserts only the missing pairs). After
the item is successfully processed, insert one `changes` row of the appropriate
type:

- `added` — the game was new to the library (`alreadyExists` is false).
- `updated` — the game already existed **and** at least one new
  platform/storefront pair (or tag) was merged into it this import.
- `already_in_library` — the game already existed and nothing new was merged
  (every platform/tag was already present).

The worker therefore needs to track whether the merge inserted anything (e.g. a
count of newly-inserted `user_game_platforms` / `user_game_tags`) to choose
between `updated` and `already_in_library`.

Use the matched `external_game_id` and the game title. `old_status`/`new_status`
stay null for import rows. Failures are **not** change rows — they remain on
`job_items` (status `failed`, `error_message`) and surface via the job's
progress counts, exactly as sync does today. The insert is best-effort and
logged on error (same pattern as the sync writers); a failed change-row insert
must not fail the import item.

**Export** (`internal/worker/tasks/export.go`) — writes **no** change rows.
Export has no meaningful per-item outcome; it renders via the counts fallback.

**Metadata refresh** — untouched. Out of scope; keeps the counts fallback.

### 3. API — one read path

Replace `GET /api/jobs/recent/:source` with `GET /api/jobs/recent` taking
optional query filters:

- `source` — single value (e.g. `steam`), AND-combined when present.
- `jobType` — one or more values, comma-separated or repeated (e.g.
  `jobType=import,export`).
- `daysBack` — integer window (default 7); filters to
  `created_at >= now() - daysBack`.
- `limit` — max jobs (default 5).

If neither `source` nor `jobType` is supplied, no type/source narrowing is
applied (still scoped to the authenticated user, the `daysBack` window, and
terminal statuses). The handler keeps the existing behaviour: fetch the most
recent terminal (`completed`/`failed`) jobs for the user matching the filters,
compute each job's progress counts, and attach the job's `changes` rows grouped
by `change_type`.

Response shape adds an `updated_items` bucket to today's sync endpoint shape,
with **empty arrays** when a job has no rows of that type:

```json
{
  "jobs": [
    {
      "id": "...", "job_type": "import", "source": "nexorious",
      "status": "completed",
      "progress": { "completed": 120, "failed": 2, "total": 122, ... },
      "added_items": [ { "title": "Game A" } ],
      "removed_items": [],
      "status_changed_items": [],
      "updated_items": [ { "title": "Game C" } ],
      "skipped_items": [],
      "already_in_library_items": [ { "title": "Game B" } ]
    }
  ]
}
```

Query change in the handler: the `changes` lookup is `WHERE job_id = ?` against
the renamed table, with a new `updated` → `updated_items` grouping case added to
the existing `change_type` switch (otherwise unchanged).

**Callers:** Sync page → `?source=steam`; Import/Export page →
`?jobType=import,export`; Maintenance page → `?jobType=metadata_refresh`.

**Routing:** register `GET /api/jobs/recent` (static) before any parameterised
`/api/jobs/:id`-style routes to respect Echo v5 route ordering.

**slumber.yaml:** update the existing `jobs/` recent request to the new
query-param form.

### 4. Frontend — one component

A single `RecentActivity` lives in `ui/frontend/src/components/jobs/` and
consumes a single `useRecentJobs(filters)` hook. Props:

```ts
interface RecentActivityProps {
  source?: string;            // sync: storefront
  jobTypes?: JobType[];       // import/export, maintenance
  excludeJobIds?: string[];   // e.g. the currently-displayed job
  daysBack?: number;          // default 7 (applied server-side)
  limit?: number;             // default 5
}
```

The `daysBack` window is applied **server-side**: the recent endpoint accepts a
`daysBack` query param (default 7) and filters jobs to
`created_at >= now() - daysBack` in SQL. The old client-side date filtering is
removed.

Per job, the card expands to:

- **Rich breakdown** when the job has change rows (sync, import) — the
  per-outcome collapsible `SyncChangeList` lists, moved over from the sync
  component. The component renders one list per non-empty bucket; labels:
  `added` → "Added to library", `updated` → "Updated", `removed` → "Removed
  from storefront", `status_changed` → "Status changed", `already_in_library` →
  "Already in library", `skipped` → "Skipped". Import emits only `added`,
  `updated`, and `already_in_library`, so only those lists render for imports.
- **Counts fallback** otherwise (export, metadata-refresh) — the existing
  aggregate completed/failed counts + `JobItemsDetails`, kept and reused.

The component decides per job which mode to use based on whether any grouped
change array is non-empty.

**Deleted:** `ui/frontend/src/components/sync/recent-activity.tsx`, its export
from `components/sync/index.ts`, and the `useJobs`-plus-client-side-filtering
path that the old jobs component used. The sync route imports the unified
component instead.

## Error handling

- Change-row inserts (sync + import) are best-effort: log on failure, never fail
  the job item. Matches the existing sync writers.
- The recent endpoint already tolerates a failed `changes` query by returning
  the job with empty change arrays (`slog.Error` + `allChanges = nil`); preserve
  that.
- Malformed `jobType` values → ignore unknown types (filter to the known set);
  no 400 needed.

## Testing

- **Migration round-trip** — apply up + down; confirm the table and indexes are
  renamed and existing rows survive (covered by the package's shared-container
  migration run; add an explicit assertion if convenient).
- **Import writer** (`internal/worker/tasks`) — a new `user_game` produces an
  `added` row; an existing game that gains a new platform/tag produces `updated`;
  an existing game with nothing new merged produces `already_in_library`; a
  failed item produces **no** change row and is marked `failed` on `job_items`.
  This is non-obvious per-item logic and a plausible bug site, so it warrants a
  test.
- **Recent endpoint** (`internal/api`) — filtering by `source`, by single
  `jobType`, by multiple `jobType`s, and by `daysBack` (a job outside the window
  is excluded); a job with no change rows returns empty arrays; grouping by
  `change_type` (including `updated`) is correct. Auth gating unchanged.
- **Frontend** — `recent-activity.test.tsx`: rich breakdown renders when change
  arrays are populated; counts fallback renders when they are empty; the deleted
  sync component's test is removed/folded in.

## Net effect

Two divergent components, two hooks, and two read paths collapse to one each
(net code deleted). Imports gain the rich breakdown via a small, isolated writer
addition. The `events` / `/admin/activity` system is untouched.

## Out of scope

- Per-item `changes` rows for export and metadata-refresh (they keep the counts
  fallback).
- Any change to the `events` table, its prune job, or `/admin/activity`.
- A finer-grained import taxonomy beyond `added` / `updated` /
  `already_in_library` (e.g. distinguishing ownership-status upgrades the way
  sync's `status_changed` does, or recording *which* platform was added).
