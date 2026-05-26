# Design: Sync Activity Totals Reconciliation (Issue #619)

## Problem

After a sync, the Recent Activity card shows only library-delta events (`added`, `status_changed`, `removed`). Games that were matched but already in the library, and games the user skipped, produce no `sync_changes` row. The result is that a sync of 457 games may show only "382 added" with no accounting for the other 75 — making it appear the sync silently dropped games.

## Goal

Every game processed by a sync job has exactly one `sync_changes` row describing its outcome. The Recent Activity UI renders all five outcome buckets so that the counts reconcile to the total matched + skipped.

## Design

### Outcome taxonomy

| Outcome | `change_type` | Existing? |
|---|---|---|
| New `user_games` row | `added` | yes |
| Ownership rank upgrade on existing platform | `status_changed` | yes |
| Game no longer on storefront | `removed` | yes |
| Matched, `user_games` already existed, no ownership upgrade | `already_in_library` | **new** |
| User skipped the game | `skipped` | **new** |

No DB migration is required. `change_type` is a free-form `TEXT` column with no constraint.

---

## Backend changes

### 1. `internal/worker/tasks/sync.go` — `UserGameWorker.Work()`

#### 1a. `already_in_library` on the completed path

Add `var platformUpgraded bool` before the `egPlatforms` loop. Set it to `true` in the `newRank > existingRank` branch where `sync_changes('status_changed')` is already written.

After the loop, extend the existing post-loop block:

```go
if isNewRow.IsNew {
    // existing sync_changes('added') insert — unchanged
} else if !platformUpgraded {
    if _, err := w.DB.NewRaw(`
        INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
        VALUES (?, ?, ?, ?, 'already_in_library', ?, now())`,
        uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
    ).Exec(ctx); err != nil {
        slog.Error("user_game_write: insert sync_change (already_in_library)", "err", err)
    }
}
```

Games where `!isNewRow.IsNew && platformUpgraded` already have a `status_changed` row from inside the loop — no additional row needed.

#### 1b. `skipped` on the worker auto-skip path

At the `eg.IsSkipped` early-return (~line 537), before `syncMarkItemSkipped`, insert:

```go
if _, err := w.DB.NewRaw(`
    INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
    VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
    uuid.NewString(), item.JobID, item.UserID, eg.ID, eg.Title,
).Exec(ctx); err != nil {
    slog.Error("user_game_write: insert sync_change (skipped)", "err", err)
}
```

### 2. `internal/api/sync.go` — `HandleSkipGame`

The current ownership query fetches only `user_id`. Extend it to also select `title`:

```go
var ownerRow struct {
    UserID string `bun:"user_id"`
    Title  string `bun:"title"`
}
err := h.db.NewRaw(`SELECT user_id, title FROM external_games WHERE id = ?`, id).Scan(ctx, &ownerRow)
```

After the job_item is marked skipped (after the `UPDATE job_items SET status = 'skipped'` succeeds), insert:

```go
if _, err := h.db.NewRaw(`
    INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
    VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
    uuid.NewString(), jobItemRow.JobID, userID, id, ownerRow.Title,
).Exec(ctx); err != nil {
    slog.Error("sync: skip game: insert sync_change (skipped)", "err", err)
}
```

### 3. `internal/api/jobs.go` — `HandleRecentJobs`

Extend the local `jobWithChanges` struct:

```go
type jobWithChanges struct {
    models.Job
    Progress                map[string]any   `json:"progress"`
    AddedItems              []syncChangeItem `json:"added_items"`
    RemovedItems            []syncChangeItem `json:"removed_items"`
    StatusChangedItems      []syncChangeItem `json:"status_changed_items"`
    SkippedItems            []syncChangeItem `json:"skipped_items"`
    AlreadyInLibraryItems   []syncChangeItem `json:"already_in_library_items"`
}
```

Initialise the two new slices and handle them in the existing switch:

```go
skippedItems := []syncChangeItem{}
alreadyInLibraryItems := []syncChangeItem{}

// in the switch:
case "skipped":
    skippedItems = append(skippedItems, syncChangeItem{Title: sc.Title})
case "already_in_library":
    alreadyInLibraryItems = append(alreadyInLibraryItems, syncChangeItem{Title: sc.Title})
```

The SQL query already fetches all `sync_changes` rows for the job without filtering on `change_type` — no SQL change required.

---

## Frontend changes

### 4. `ui/frontend/src/types/jobs.ts`

Add two fields to `RecentJobDetail`:

```ts
skippedItems: SyncChangeItem[];
alreadyInLibraryItems: SyncChangeItem[];
```

### 5. `ui/frontend/src/api/jobs.ts`

Add to `RecentJobDetailApiResponse`:

```ts
skipped_items?: SyncChangeItemApiResponse[];
already_in_library_items?: SyncChangeItemApiResponse[];
```

In `transformRecentJob`:

```ts
skippedItems: (api.skipped_items ?? []).map(transformSyncChangeItem),
alreadyInLibraryItems: (api.already_in_library_items ?? []).map(transformSyncChangeItem),
```

### 6. `ui/frontend/src/components/sync/recent-activity.tsx`

Add `SkipForward` and `BookMarked` to the lucide-react import.

Add two new `SyncChangeList` calls in `JobCard`:

```tsx
<SyncChangeList
  items={job.alreadyInLibraryItems}
  label="Already in library"
  icon={<BookMarked className="h-4 w-4 text-muted-foreground" />}
/>
<SyncChangeList
  items={job.skippedItems}
  label="Skipped"
  icon={<SkipForward className="h-4 w-4 text-muted-foreground" />}
/>
```

Update `formatSummary` to include both new buckets:

```ts
if (job.alreadyInLibraryItems.length > 0)
  parts.push(`${job.alreadyInLibraryItems.length} already in library`);
if (job.skippedItems.length > 0)
  parts.push(`${job.skippedItems.length} skipped`);
```

---

## Tests

### Go

- `HandleSkipGame` API test: call `POST /api/sync/ignored/:id` for a game with an active job item; assert a `sync_changes` row with `change_type='skipped'` and the correct title is written to the DB.
- Worker auto-skip test: set `external_games.is_skipped=true` for a game in a job, dispatch `UserGameWorkerArgs`; assert a `sync_changes('skipped')` row is written.
- Worker `already_in_library` test: dispatch `UserGameWorkerArgs` for a game whose `user_games` row already exists with no ownership upgrade; assert a `sync_changes('already_in_library')` row is written (and no `added` or `status_changed` row).

### Frontend (Vitest)

- `formatSummary` unit test: verify skipped and already-in-library counts appear in the summary string.
- `JobCard` render test: verify the two new `SyncChangeList` sections render when the respective item arrays are non-empty, and are absent when empty.

---

## Backward compatibility

Old jobs that predate this fix have no `skipped_items` or `already_in_library_items` rows in `sync_changes`. The API initialises both as empty slices (`[]syncChangeItem{}`), and the frontend guards both `SyncChangeList` calls with `if (items.length === 0) return null` (already in the component). Old job cards render correctly with only the buckets they have data for.
