# Issue #671 — Move Sync Schedule into the Storefront Header Card

## Problem

The sync schedule setting (sync frequency) is buried inside the collapsible connection section on the storefront detail page. Users have to click the "Connected" badge to expand the section before they can find or change the schedule. This location is counter-intuitive: the connection section is for configuring credentials, not for routine scheduling.

## Scope

**In scope:** Storefront detail page only (`ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`).

**Out of scope:** The sync index page (`/sync`) and its `SyncServiceCard` components are not changed.

## Design

### Layout change in the Platform Header Card

The header card's right column currently contains only the connection status badge. It becomes a vertically-stacked flex column (`flex-col items-end gap-2`) containing:

1. **Connection status badge** (Connected / Credentials Error / Not Configured) — existing, unchanged, still clickable to toggle the collapsible connection section.
2. **Sync frequency Select** — rendered only when `config.isConfigured` is true. Uses the existing `effectiveFrequency` value, `handleFrequencyChange` handler, and `isUpdating` disabled state. Width `w-[140px]`, size `sm`.

### Removal

The standalone "Sync Frequency" `Card` that currently lives inside the `<Collapsible>` section (rendered when `config.isConfigured`) is removed entirely.

### What does not change

- The collapsible connection section and all storefront-specific connection cards are untouched.
- `handleFrequencyChange`, `effectiveFrequency`, and `localFrequency` state are untouched.
- When `config.isConfigured` is false, no frequency dropdown is shown (same behaviour as today).
- The sync index page is untouched.

## Affected Files

- `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` — only file changed.

## Testing

Visual verification: load `/sync/steam` (or any configured storefront) and confirm:
- The sync frequency dropdown appears in the header card's right column, below the badge.
- Changing the frequency saves successfully (toast confirms, value persists on refresh).
- When not configured, no frequency dropdown appears in the header.
- Expanding/collapsing the connection section no longer reveals a separate schedule card.
