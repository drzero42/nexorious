# Currently Playing Dashboard Section - Design Document

**Date:** 2025-12-31
**Feature:** Games in Progress on Dashboard
**Status:** Design Complete - Ready for Implementation

## Overview

Add a prominent "Currently Playing" section at the top of the dashboard that displays games with `IN_PROGRESS` or `REPLAY` status in a horizontal scrolling row. Each game shows cover art, title, and platform information, linking to the game's detail page.

## Key Requirements

- Display only when user has at least one active game (IN_PROGRESS or REPLAY status)
- Position at the very top of the dashboard, before all statistics cards
- Horizontal scroll layout with medium-sized cards (~160-180px wide)
- Show cover art, game title, and platform badge per game
- Click any card to navigate to that game's detail page
- Hide section completely if no active games exist

## Component Structure

### New Component: `CurrentlyPlayingSection.tsx`

**Location:** `frontend/src/components/dashboard/`

**Component Hierarchy:**
```
CurrentlyPlayingSection
├── Section header with title ("Currently Playing")
├── Horizontal scroll container
│   └── Game cards (mapped from active games)
│       ├── Cover art image (3/4 aspect ratio)
│       ├── Game title
│       └── Platform badge(s)
```

**Implementation Details:**
- Fetch active games using new `useActiveGames()` hook
- Card width: 160px (fixed to prevent layout shift)
- Cards maintain 3/4 aspect ratio for cover art
- Platform display: Show first platform, with "+X more" if multiple exist
- Scroll behavior: CSS `overflow-x: auto` with smooth scrolling
- Cards clickable via Next.js `Link` to `/games/[id]`

## Data Fetching

### New Hook: `useActiveGames()`

**Location:** `frontend/src/hooks/use-games.ts`

**Functionality:**
- Uses existing `getUserGames()` API function from `frontend/src/api/games.ts`
- Filters: `play_status` includes both `IN_PROGRESS` and `REPLAY`
- Returns first page only (no pagination needed for this section)
- TanStack Query caching with 5-minute stale time
- Auto-refetch when games updated elsewhere in app

**Query Key:**
```typescript
['user-games', 'active'] // Separate from main games list query
```

**Data:**
- Uses existing `UserGame` interface (no transformation needed)
- Platform display logic: First platform name, count remaining with "+X"
- Fallback: "Unknown Platform" if no platforms assigned

**Performance:**
- Typical return: 1-10 games (reasonable for most users)
- Cover art optimized via Next.js Image component
- Query cached across dashboard remounts

## Visual Design

### Section Header
- Title: "Currently Playing"
- Typography: `text-2xl font-semibold`
- Spacing: `margin-bottom` for separation from scroll container

### Horizontal Scroll Container
- CSS: `flex gap-4 overflow-x-auto pb-4 scroll-smooth`
- Hide scrollbar on desktop, show on mobile
- Right padding to show partial next card (encourages scrolling)

### Game Card Design

**Card Dimensions:**
- Width: 160px (fixed)
- Cover art: 160px × 213px (3/4 aspect ratio)
- Rounded corners: `rounded-lg`
- Hover: `hover:scale-105` with shadow increase
- Transition: `transition-transform duration-200`

**Card Layout:**
```
┌──────────────┐
│              │
│  Cover Art   │  ← 160×213px, object-cover, rounded-lg
│              │
└──────────────┘
  Game Title      ← text-sm font-medium, 2-line clamp
  Platform        ← text-xs text-muted-foreground, badge style
```

### Platform Badge
- Small rounded badge with `bg-secondary`
- Truncated text with ellipsis for long names
- Shows "+2 more" format if multiple platforms

### Responsive Behavior
- **Mobile:** 140px card width (~2.5 cards visible)
- **Tablet+:** 160px card width (~4-5 cards visible)
- **Desktop XL:** 160px card width (~6-7 cards visible)

## Dashboard Integration

### File: `frontend/src/app/(main)/dashboard/page.tsx`

**Changes:**
1. Import `CurrentlyPlayingSection` component
2. Place as **first element** in page layout (before statistics cards)
3. Conditional rendering: Only render if `useActiveGames()` returns non-empty array

**Example:**
```tsx
export default function DashboardPage() {
  return (
    <div className="container mx-auto p-6 space-y-6">
      <CurrentlyPlayingSection />  {/* New - shows only if active games exist */}

      {/* Existing dashboard content */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Stats cards... */}
      </div>
      {/* ... rest of dashboard */}
    </div>
  );
}
```

## Testing Requirements

### Component Tests: `CurrentlyPlayingSection.test.tsx`

**Test Cases:**
- ✓ Renders game cards with correct cover art URLs
- ✓ Displays game titles correctly
- ✓ Shows platform badges with correct text
- ✓ Hides section when no active games exist
- ✓ Links navigate to correct game detail pages (`/games/[id]`)
- ✓ Handles games with multiple platforms (shows "+X more")
- ✓ Handles missing cover art (shows fallback placeholder)
- ✓ Horizontal scroll works correctly

### Hook Tests: `use-games.test.ts` (additions)

**Test Cases:**
- ✓ `useActiveGames()` filters for IN_PROGRESS and REPLAY only
- ✓ Returns correct UserGame data structure
- ✓ Caches results appropriately
- ✓ Refetches on invalidation
- ✓ Handles empty results gracefully

### Integration Considerations
- Invalidate active games query when user updates any game status
- Use existing query invalidation patterns from bulk operations
- Ensure cover art URL resolution matches GameCard component helper
- Test with 1 game, 10 games, and 50+ games scenarios

### Accessibility
- ✓ Cards keyboard navigable with proper focus styles
- ✓ Cover images have meaningful alt text (game title)
- ✓ Scroll container supports keyboard arrow navigation
- ✓ Screen reader announces section and card count

## File Changes Summary

### New Files
- `frontend/src/components/dashboard/CurrentlyPlayingSection.tsx` - Main component
- `frontend/src/components/dashboard/CurrentlyPlayingSection.test.tsx` - Component tests

### Modified Files
- `frontend/src/hooks/use-games.ts` - Add `useActiveGames()` hook
- `frontend/src/hooks/use-games.test.ts` - Add hook tests
- `frontend/src/app/(main)/dashboard/page.tsx` - Integrate new section

## Success Criteria

- [ ] "Currently Playing" section appears at top of dashboard when active games exist
- [ ] Section hidden when no IN_PROGRESS or REPLAY games exist
- [ ] Each card displays cover art, title, and platform correctly
- [ ] Cards link to correct game detail pages
- [ ] Horizontal scrolling works smoothly on all devices
- [ ] All tests pass with >70% coverage
- [ ] Zero TypeScript errors
- [ ] Matches existing dashboard design patterns and styling

## Implementation Notes

- Reuse existing cover art URL resolution logic from GameCard component
- Follow existing shadcn/ui patterns for badges and cards
- Use Tailwind utilities for responsive breakpoints
- Leverage TanStack Query patterns established in codebase
- Consider adding subtle scroll indicators (shadows/gradients) at edges

## Future Enhancements (Out of Scope)

- Manual sorting/reordering of games in the section
- Progress bars based on HLTB data
- Quick actions (e.g., log hours, update status) on hover
- Configurable which statuses to include
- Animation when games enter/exit the section
