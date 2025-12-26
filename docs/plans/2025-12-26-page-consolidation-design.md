# Page Consolidation Design

## Overview

This redesign consolidates four pages (Jobs, Review, Import/Export, Sync) into three focused pages:

1. **Import/Export** (`/import-export`) - Single-job-at-a-time import/export with inline progress and details
2. **Sync** (`/sync` + `/sync/[platform]`) - Platform sync overview and per-platform detail pages with configuration, progress tracking, and inline IGDB review
3. **Maintenance** (`/admin/maintenance`) - Admin-only page for seed data, cleanup, and IGDB data refresh

The standalone Jobs page (`/jobs`), Job detail page (`/jobs/[id]`), and Review page (`/review`) will be removed. Progress and review functionality moves inline to the relevant pages.

### Key Principles

- One active job per page per user (no queuing multiple imports)
- Progress shown inline with expandable item details
- Cancel removes the job immediately (no separate delete step)
- Historical job results retained for 7 days
- Review items handled inline where they originate (sync page only - imports from Nexorious JSON don't need review)

---

## Import/Export Page (`/import-export`)

### Page States

The page has three distinct states:

#### 1. Idle State (no active job)

- Shows import card (Nexorious JSON only) with file upload
- Shows export cards (JSON and CSV) with export buttons
- Shows collapsible "Recent Activity" section with jobs from last 7 days (imports and exports)

#### 2. Active Job State (import or export running)

- Import/export cards are disabled or hidden
- Progress overview card at top showing:
  - Job type and status badge
  - Progress bar with percentage
  - Summary counts: Completed, Pending, Failed
  - Cancel button (immediately removes the job)
- Expandable details section below grouped by status:
  - **Failed** (expanded by default if any) - shows game name + error message
  - **Completed** - shows game name + matched IGDB title
  - **Processing** - shows game name + spinner
  - **Pending** - shows game name

#### 3. Completed State (job just finished)

- Shows completion summary (success/partial/failed)
- Results remain visible until user dismisses or starts new job
- Option to download export file (for export jobs)
- "Start New Import/Export" button to return to idle state

### Recent Activity Section

Collapsible section showing jobs from last 7 days:

- Job type, source, date, duration
- Final counts (completed/failed)
- Expandable to see item details
- For exports: download link if file still available

---

## Sync Pages

### Sync Overview Page (`/sync`)

A dashboard showing all sync-capable storefronts at a glance.

#### Storefront Cards Grid

One card per storefront (Steam, future platforms), each showing:

- Storefront name and icon
- Connection status (configured/not configured)
- Last sync time
- Current status (idle, syncing with progress bar, items need review)
- "Sync Now" button (disabled if sync running)
- Link to storefront detail page

#### Items Needing Review Summary

- Badge/count on cards showing pending review items per storefront
- Quick link to that storefront's detail page

---

### Storefront Detail Page (`/sync/[platform]`)

#### Configuration Section

- Platform-specific settings (Steam ID, API keys, etc.)
- Scheduled sync configuration (frequency, time)
- Enable/disable auto-sync

#### Active Sync Progress (when running)

- Progress overview at top
- Expandable item details grouped by status
- Cancel button

#### Items Needing Review Section

- Expanded by default when items need attention
- Inline IGDB matching (candidates, search, skip)
- Filters by status

#### Recent Sync Activity

- Collapsible section with syncs from last 7 days
- Expandable details per sync

#### Manual Sync Button

- Available when no sync is running

### Sync Constraints

- Only one sync per platform can run at a time
- Multiple platforms could sync simultaneously (if supported in future)
- User can still configure other platforms while one is syncing

---

## Maintenance Page (`/admin/maintenance`)

Admin-only page under Administration section.

### Seed Data Section

- Card for loading/refreshing seed data
- Shows current state (platforms count, storefronts count, last loaded date)
- "Load Seed Data" button (idempotent - safe to run multiple times)
- Success/error toast notification on completion

### Database Cleanup Section

- Card for cleanup tasks
- Options:
  - Orphaned files cleanup (cover art not linked to any game)
  - Expired job data cleanup (manual trigger of TTL cleanup)
- Shows last run date for each task
- "Run Cleanup" buttons

### IGDB Data Refresh Section

- Card for refreshing IGDB metadata across collection
- Scope options:
  - All games
  - Games missing cover art
  - Games older than X days since last refresh
- Data to refresh (checkboxes):
  - Cover art
  - Time to beat
  - Platforms available
  - Description
- "Start Refresh" button
- Progress with expandable details when running

### Recent Maintenance Jobs

- Collapsible section showing maintenance jobs from last 7 days
- Type, date, duration, results summary

### Constraints

- One maintenance job of each type can run at a time
- Shows progress inline when running
- Cancel available for long-running jobs

---

## Removals & Navigation Changes

### Pages Removed

- `/jobs` - Jobs list page
- `/jobs/[id]` - Job detail page
- `/review` - Review queue page

### Navigation Updates

#### Manage Section (revised)

- Import / Export
- Sync
- Tags

(Jobs and Review removed from nav)

#### Administration Section (add)

- Admin Dashboard
- User Management
- Platforms
- **Maintenance** (new)

### Redirects

For backwards compatibility and bookmarks:

- `/jobs` → `/import-export`
- `/jobs/[id]` → `/import-export` (with toast: "Job details now shown inline")
- `/review` → `/sync` (with toast: "Review items now on Sync page")

---

## Backend Considerations

### Job Cancellation Simplification

Current behavior has cancel → cancelled state → delete. New behavior:

- Cancel immediately removes the job and all associated data
- No separate delete step needed
- Completed/failed jobs remain for 7 days TTL then auto-cleanup

### API Changes

May need new endpoints or modifications for:

- Getting active job for current user (per job type)
- Expanded job item details (not just counts)
- Streaming/polling for real-time progress updates

### Review Items

- Review items from imports should no longer be created (Nexorious JSON is pre-matched)
- Review items only created from sync operations
- Review endpoints may need filtering by source enforced
