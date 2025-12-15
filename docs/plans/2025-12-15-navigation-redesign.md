# Navigation Redesign

## Problem

The sidebar navigation has 15+ items visible at once, making it crowded and hard to navigate. Technical and setup items are mixed with daily-use features.

## Solution

Reduce to 6 core navigation items by consolidating related features into hub pages.

## Navigation Structure

### Before (15+ items)
```
Dashboard
My Games
Add Game
Manage Tags
Background Jobs
Review Queue
Import Games
  Nexorious JSON
  Darkadia CSV
  Steam Library
Settings
  Sync Settings
Admin Dashboard      (admin only)
Manage Users         (admin only)
Manage Platforms     (admin only)
Profile
Logout
```

### After (6 items + admin)
```
Dashboard
My Games
Add Game
Import / Export (n)   <- badge shows pending import reviews
Sync (n)              <- badge shows pending sync reviews
Settings

Administration        (admin only)
  Admin Dashboard
  Manage Users
  Manage Platforms

User
  [Avatar] Username / Profile Settings
  Logout
```

## Page Definitions

### Import / Export (`/import-export`)
Consolidates all manual data transfer operations.

**Contents:**
- Import section
  - Nexorious JSON import
  - Darkadia CSV import
- Export section (new)
  - Export collection to JSON
- Recent import jobs list with status

**Badge behavior:**
- Shows count of pending reviews from imports
- Clicking badge navigates to `/review?source=import`

### Sync (`/sync`)
Automated service connections and synchronization.

**Contents:**
- Steam library sync (existing functionality from `/settings/sync`)
- Connection status and last sync time
- Future: GOG, Epic, other service integrations

**Badge behavior:**
- Shows count of pending reviews from sync operations
- Clicking badge navigates to `/review?source=sync`

### Settings (`/settings`)
Hub for all configuration and system tasks.

**Sections:**
- **Profile** - Username, email, password change (existing `/profile` content)
- **Tags** - Manage tags (existing `/tags` content)
- **Background Jobs** - View/manage jobs (existing `/jobs` content)
- **General** - Future app preferences

### Review (`/review`)
Existing page with added source filtering.

**Changes:**
- Add `?source=import|sync` query parameter support
- Add source filter toggle in UI
- Default view shows all pending reviews

## Route Changes

### New routes
| Route | Description |
|-------|-------------|
| `/import-export` | Consolidated import/export page |
| `/sync` | Sync management page |
| `/settings` | Settings hub with sections |

### Redirects
| Old Route | New Route | Notes |
|-----------|-----------|-------|
| `/import` | `/import-export` | |
| `/import/nexorious` | `/import-export` | Deep link to section if needed |
| `/import/darkadia` | `/import-export` | Deep link to section if needed |
| `/import/steam` | `/sync` | Steam is a sync, not import |
| `/settings/sync` | `/sync` | |
| `/tags` | `/settings#tags` | Or `/settings?section=tags` |
| `/jobs` | `/settings#jobs` | Or `/settings?section=jobs` |

## Badge Implementation

### Data requirements
The navigation needs access to review counts by source:
- `importReviewCount` - Reviews pending from import operations
- `syncReviewCount` - Reviews pending from sync operations

### Display rules
- Badge only appears when count > 0
- Each source shows only its own count
- Clicking the badge (not just the nav item) goes to filtered review page

### Visual states
```
No pending reviews:
  Import / Export
  Sync

Only sync reviews:
  Import / Export
  Sync (5)

Both have reviews:
  Import / Export (2)
  Sync (5)
```

## Mobile Navigation

Same structure as desktop - no separate mobile hierarchy.

```
[Hamburger] Nexorious                    [Avatar]

--- Slide-out menu ---
Dashboard
My Games
Add Game
Import / Export (2)
Sync (5)
Settings

Administration        (admin only)
  Admin Dashboard
  Manage Users
  Manage Platforms

Account
  [Avatar] Username / Profile Settings
  Sign out
```

## Implementation Notes

### Layout changes (`+layout.svelte`)
- Replace 15+ nav items with 6 core items
- Add badge components for Import/Export and Sync
- Fetch review counts on mount (or subscribe to store)
- Same changes for mobile menu

### New components needed
- `NavBadge.svelte` - Displays count badge on nav items
- Review count store or API endpoint by source

### Settings page structure
Consider tabs or accordion for sections:
- URL hash for direct section links (`/settings#tags`)
- Or query param (`/settings?section=tags`)
- Mobile-friendly vertical layout

## Out of Scope

- Export functionality implementation (separate task)
- Additional sync integrations beyond Steam
- Review page UI redesign (only adding source filter)
