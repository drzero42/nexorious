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
computes `alreadyExists` (whether a `user_game` already existed). After the item
is successfully processed, insert one `changes` row:

- `already_in_library` when `alreadyExists` is true,
- `added` otherwise.

Use the matched `external_game_id` and the game title. Failures are **not**
change rows — they remain on `job_items` (status `failed`, `error_message`) and
surface via the job's progress counts, exactly as sync does today. The insert is
best-effort and logged on error (same pattern as the sync writers); a failed
change-row insert must not fail the import item.

**Export** (`internal/worker/tasks/export.go`) — writes **no** change rows.
Export has no meaningful per-item outcome; it renders via the counts fallback.

**Metadata refresh** — untouched. Out of scope; keeps the counts fallback.

### 3. API — one read path

Replace `GET /api/jobs/recent/:source` with `GET /api/jobs/recent` taking
optional query filters:

- `source` — single value (e.g. `steam`), AND-combined when present.
- `jobType` — one or more values, comma-separated or repeated (e.g.
  `jobType=import,export`).

If neither is supplied, no type/source narrowing is applied (still scoped to the
authenticated user and terminal statuses). The handler keeps the existing
behaviour: fetch the N most recent terminal (`completed`/`failed`) jobs for the
user matching the filters, compute each job's progress counts, and attach the
job's `changes` rows grouped by `change_type`.

Response shape is unchanged from today's sync endpoint, with **empty arrays**
when a job has no `changes` rows:

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
      "skipped_items": [],
      "already_in_library_items": [ { "title": "Game B" } ]
    }
  ]
}
```

Query change in the handler: the `changes` lookup is `WHERE job_id = ?` against
the renamed table (the per-job grouping logic is otherwise unchanged).

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
  daysBack?: number;          // default 7 (applied server-side or client-side)
  limit?: number;             // default 5
}
```

Per job, the card expands to:

- **Rich breakdown** when the job has change rows (sync, import) — the
  per-outcome collapsible `SyncChangeList` lists, moved over from the sync
  component. Existing change-type labels map cleanly; import only emits `added`
  + `already_in_library`, so only those two lists render.
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
  `added` row; an existing one produces `already_in_library`; a failed item
  produces **no** change row and is marked `failed` on `job_items`. This is
  non-obvious per-item logic and a plausible bug site, so it warrants a test.
- **Recent endpoint** (`internal/api`) — filtering by `source`, by single
  `jobType`, by multiple `jobType`s; a job with no change rows returns empty
  arrays; grouping by `change_type` is correct. Auth gating unchanged.
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
- A per-item taxonomy for ownership-status changes on import merges (an import
  that adds a new platform/storefront to an existing game is classified
  `already_in_library`, not `status_changed`).
