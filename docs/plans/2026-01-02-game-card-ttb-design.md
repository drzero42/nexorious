# Game Card Time-to-Beat Display

## Overview

Add time-to-beat estimates to game cards on the /games page, showing main story, extras, and completionist times in a compact inline format.

## Design

### Display Format

```
⏱ 12h / 25h / 40h
```

Three values separated by slashes:
- Main story completion
- Main + side content (extras)
- 100% completion (completionist)

### Data Source

Uses existing fields from the Game model (already available in API responses):
- `howlongtobeat_main`
- `howlongtobeat_extra`
- `howlongtobeat_completionist`

### Display Rules

| Scenario | Behavior |
|----------|----------|
| All values null | Hide row entirely |
| Any value exists | Show row with available data |
| Individual value missing | Show "—" for that value |
| Zero hours | Show "0h" (valid for short games) |

Examples:
- Full data: `⏱ 12h / 25h / 40h`
- Partial data: `⏱ 12h / — / 40h`
- Single value: `⏱ 12h / — / —`

### Visual Styling

- **Placement**: Below "Hours played" row at bottom of card
- **Text**: `text-xs text-muted-foreground` (matches hours played)
- **Icon**: `Timer` from Lucide (differentiates from Clock used for hours played)

### Card Layout (after change)

```
┌─────────────────────┐
│ [Cover Image]       │
│ [Status] [♥]        │
├─────────────────────┤
│ Game Title          │
│ Platform, Platform  │
│ ★★★★☆ 4.0          │
│ 🕐 15h played       │
│ ⏱ 12h / 25h / 40h  │  ← NEW
└─────────────────────┘
```

## Implementation

### File Changed

`frontend/src/components/games/game-card.tsx`

### Changes

1. Import `Timer` icon from `lucide-react`
2. Add helper function:
   ```typescript
   const formatTtb = (hours: number | null | undefined) =>
     hours != null ? `${hours}h` : '—'
   ```
3. Add conditional row after hours played section

### No Backend Changes

Data already flows from IGDB → backend → API → frontend. No API or model changes needed.

### No New Tests

Simple display-only change with no logic complexity. Type system ensures correct field types.
