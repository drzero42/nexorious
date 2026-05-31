# Issue #697 – Per-Route Page Titles

## Problem

On mobile Firefox, navigating to certain routes (e.g. Add Game) causes the browser tab title to revert to the full URL instead of "Nexorious". The root cause is that `document.title` is never set programmatically — the app relies solely on the static `<title>Nexorious</title>` in `index.html`. Desktop browsers tolerate this; mobile Firefox does not.

## Solution

Use TanStack Router v1's first-class `head` + `HeadContent` mechanism to set `<title>` on every client-side navigation. Each content route declares its own title string; the root route provides a "Nexorious" fallback. TanStack Router deduplicates and writes the most-specific title to the DOM on every navigation.

## Architecture

### Root route (`ui/frontend/src/routes/__root.tsx`)

- Add `head: () => ({ meta: [{ title: 'Nexorious' }] })` as the fallback.
- Render `<HeadContent />` once at the top of `RootComponent`, before `<Outlet />`.

The existing static `<title>Nexorious</title>` in `index.html` is kept as a pre-JS fallback and does not conflict.

### Per-route `head` functions

Every content route adds:

```tsx
head: () => ({ meta: [{ title: 'Page Name | Nexorious' }] }),
```

Layout-only routes (`add.tsx`, `$id.tsx`) are pure `<Outlet />` pass-throughs and require no `head`. Redirect routes (`review.tsx`, `jobs/index.tsx`, `jobs/$id.tsx`) immediately navigate away and fall through to the root default.

### Dynamic storefront title

`sync/$storefront.tsx` receives `params.storefront` (e.g. `steam`, `psn`, `gog`, `epic`). The `head` function title-cases this value:

```tsx
head: ({ params }) => ({
  meta: [{ title: `${params.storefront.charAt(0).toUpperCase() + params.storefront.slice(1)} Sync | Nexorious` }],
}),
```

### Game detail pages

`games/$id.index.tsx` and `games/$id.edit.tsx` use static titles (`Game Details | Nexorious`, `Edit Game | Nexorious`). The game name lives in a React hook, not a route loader, so dynamic titles would require adding a loader — not worth it for this fix.

## Route → Title Map

| Route file | `<title>` value |
|---|---|
| `__root.tsx` (default) | `Nexorious` |
| `_public/login.tsx` | `Login \| Nexorious` |
| `_authenticated/dashboard.tsx` | `Dashboard \| Nexorious` |
| `_authenticated/games/index.tsx` | `Library \| Nexorious` |
| `_authenticated/games/add.index.tsx` | `Add Game \| Nexorious` |
| `_authenticated/games/add.confirm.tsx` | `Add Game \| Nexorious` |
| `_authenticated/games/$id.index.tsx` | `Game Details \| Nexorious` |
| `_authenticated/games/$id.edit.tsx` | `Edit Game \| Nexorious` |
| `_authenticated/import-export.tsx` | `Import & Export \| Nexorious` |
| `_authenticated/tags.tsx` | `Tags \| Nexorious` |
| `_authenticated/profile.tsx` | `Profile \| Nexorious` |
| `_authenticated/sync/index.tsx` | `Sync \| Nexorious` |
| `_authenticated/sync/$storefront.tsx` | `{Storefront} Sync \| Nexorious` |
| `_authenticated/admin/index.tsx` | `Admin \| Nexorious` |
| `_authenticated/admin/backups.tsx` | `Backups \| Nexorious` |
| `_authenticated/admin/maintenance.tsx` | `Maintenance \| Nexorious` |
| `_authenticated/admin/users/index.tsx` | `Users \| Nexorious` |
| `_authenticated/admin/users/new.tsx` | `New User \| Nexorious` |

## Testing

No automated tests are added. Title management is browser-visible behaviour, not business logic. Existing frontend tests are unaffected (none check `document.title`). The fix is verified by navigating between routes in a browser and confirming the tab title updates correctly.
