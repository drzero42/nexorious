# Filter/Sort UI Separation Design

## Overview

Reorganize the /games page filter and sort controls from a single confusing row into two distinct labeled rows for better UX and responsive behavior.

## Problem

- All controls (filters + sorting + view toggle) on one line
- Users can't quickly distinguish filters from sorting
- Horizontal overflow on smaller screens causes awkward wrapping

## Solution

Split into two labeled rows:
1. **Sort row (top):** Sort field, sort order, view toggle
2. **Filters row (bottom):** Search, status, platforms, storefronts, genres, tags, clear button

## Design Specification

### Layout Structure

```
Sort by:  [Date Added ▾] [↓]                    [Grid] [List]
Filters:  [Search...] [Status ▾] [Platforms ▾] [Storefronts ▾] [Genres ▾] [Tags ▾] [Clear]
```

### Component Changes

**File:** `frontend/src/components/games/game-filters.tsx`

**Outer container:**
```tsx
<div className="flex flex-col gap-3">
```

**Sort row (top):**
```tsx
<div className="flex flex-wrap gap-4 items-center">
  <span className="text-sm text-muted-foreground w-14">Sort by:</span>
  {/* Sort dropdown (w-40) */}
  {/* Sort order toggle button */}
  <div className="flex-1" />  {/* spacer */}
  {/* View toggle buttons */}
</div>
```

**Filters row (bottom):**
```tsx
<div className="flex flex-wrap gap-4 items-center">
  <span className="text-sm text-muted-foreground w-14">Filters:</span>
  {/* Search input: w-full sm:w-64 */}
  {/* Status dropdown (w-40) */}
  {/* Platforms multi-select */}
  {/* Storefronts multi-select */}
  {/* Genres multi-select */}
  {/* Tags multi-select */}
  {/* Clear filters button (conditional) */}
</div>
```

### Label Styling

- `text-sm` — smaller than controls
- `text-muted-foreground` — subtle gray from shadcn theme
- `w-14` — fixed width (~56px) for alignment between rows

### Responsive Behavior

**Desktop (1024px+):**
- Both rows display on single lines
- View toggle right-aligned on sort row

**Tablet (768px - 1023px):**
- Filter controls may wrap to additional lines
- Sort row stays on one line (fewer controls)

**Mobile (<768px):**
- Filter controls wrap across multiple lines
- Search input takes full width (`w-full sm:w-64`)
- Natural flow, no hidden controls

### Changes Summary

| Element | Current | New |
|---------|---------|-----|
| Container | Single flex row | Vertical stack (`flex-col gap-3`) |
| Labels | None | "Sort by:" and "Filters:" with `w-14` |
| Search width | `w-64` | `w-full sm:w-64` |
| Row order | Mixed | Sort top, filters bottom |

### No Changes To

- Props or state management
- URL parameter handling
- Child components (MultiSelectFilter, Select, etc.)
- Parent page component (`games/page.tsx`)
