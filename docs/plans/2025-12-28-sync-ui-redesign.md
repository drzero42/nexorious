# Sync UI Redesign

## Problem Statement

The sync page has several defects:

1. **Games disappear after sync completes** - The review query is disabled when `activeJob` becomes null, causing cached data to vanish
2. **Job completes too early** - Backend marks job as COMPLETED when PENDING/PROCESSING items are done, ignoring PENDING_REVIEW items
3. **Review section never shows anything** - Items are shown in progress box during sync, but the separate review section's query timing is broken
4. **Can't act on items during sync** - Review widgets exist but aren't accessible in the progress box
5. **Recent activity lacks detail** - Only shows "last synced at X" with no useful information

## Design

### 1. Job Completion Logic

A sync job can only complete when ALL items have reached a terminal state:
- COMPLETED - Game was matched and added/confirmed
- SKIPPED - User chose to skip
- FAILED - Unrecoverable error

PENDING_REVIEW is NOT terminal - it blocks job completion. The job stays in RUNNING status until the user resolves all review items.

**Backend change:**
Modify `_check_and_update_job_completion` in `process_item.py`:
- Current: Job completes when `pending + processing == 0`
- New: Job completes when `pending + processing + pending_review == 0`

### 2. Progress Box as Single Sync UI

Remove the separate "Items Needing Review" section. The progress box becomes the complete sync interface with inline review capabilities.

```
┌─────────────────────────────────────────────────┐
│ Steam Sync                              Running │
│ ━━━━━━━━━━━━━━━━━━░░░░░░░░░░░░  45/100 items   │
├─────────────────────────────────────────────────┤
│ ▶ Completed (32)                                │
│ ▶ Skipped (5)                                   │
│ ▶ Failed (3)                                    │
│ ▼ Needs Review (5)  ← expanded by default       │
│   ┌─────────────────────────────────────────┐   │
│   │ "DOOM Eternal Deluxe Edition"           │   │
│   │ Suggested matches:                      │   │
│   │   ○ DOOM Eternal (IGDB: 103298)        │   │
│   │   ○ DOOM Eternal - Deluxe (IGDB: 10452)│   │
│   │ [Search] [Skip]                         │   │
│   └─────────────────────────────────────────┘   │
│   ┌─────────────────────────────────────────┐   │
│   │ "Half-Life 2 Episode One"               │   │
│   │ ...                                     │   │
│   └─────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```

**Key behaviors:**
- "Needs Review" section expanded by default when items exist
- Each review item shows inline: Steam title, suggested IGDB matches, search button, skip button
- User can resolve items while sync is still processing other games
- Once resolved, item moves to Completed or Skipped
- Job only finishes when "Needs Review" is empty

### 3. Recent Activity with Expandable Job Details

Each past sync job is an expandable card showing full results:

```
┌─────────────────────────────────────────────────┐
│ Recent Activity                                 │
├─────────────────────────────────────────────────┤
│ ▼ Dec 28, 2025 at 14:32 — 47 games processed   │
│   ┌─────────────────────────────────────────┐   │
│   │ ▶ Completed (42)                        │   │
│   │   • "DOOM Eternal Deluxe" → DOOM Eternal│   │
│   │     (IGDB: 103298) — Added              │   │
│   │   • "The Witcher 3" → The Witcher 3     │   │
│   │     (IGDB: 1942) — Already in library   │   │
│   ├─────────────────────────────────────────┤   │
│   │ ▶ Skipped (3)                           │   │
│   │   • "Steamworks Common Redistributables"│   │
│   │   • "Proton 8.0"                        │   │
│   ├─────────────────────────────────────────┤   │
│   │ ▶ Failed (2)                            │   │
│   │   • "Some Game" — IGDB API timeout      │   │
│   │   • "Another" — No matches found        │   │
│   └─────────────────────────────────────────┘   │
├─────────────────────────────────────────────────┤
│ ▶ Dec 25, 2025 at 10:15 — 12 games processed   │
└─────────────────────────────────────────────────┘
```

**Details per status:**

| Status | Information shown |
|--------|-------------------|
| Completed | Steam title → IGDB title (IGDB: ID) — Added/Already in library |
| Skipped | Steam title only |
| Failed | Steam title — Error reason |

## Implementation Changes

### Backend

1. **Job completion logic** (`backend/app/worker/tasks/sync/process_item.py`)
   - Modify `_check_and_update_job_completion` to treat PENDING_REVIEW as blocking
   - Job stays RUNNING until `pending + processing + pending_review == 0`

2. **Job item data** (verify schema stores required fields)
   - Original platform name (e.g., Steam title)
   - Matched IGDB title and ID
   - Whether game was "added" vs "already in library"
   - Error reason for failed items

### Frontend

1. **Sync page** (`frontend/src/app/(main)/sync/[platform]/page.tsx`)
   - Remove separate "Items Needing Review" section
   - Embed review widgets inline in progress box's "Needs Review" section
   - "Needs Review" expanded by default when items exist

2. **Query fix** (`frontend/src/hooks/use-sync.ts`, `frontend/src/hooks/use-jobs.ts`)
   - Keep job items query active as long as job exists
   - Fix `enabled: !!activeJob?.id` issue that causes data to vanish on completion

3. **Recent activity section**
   - Fetch past completed jobs for this platform
   - Render expandable cards with Completed/Skipped/Failed sections
   - Display appropriate details per status type
