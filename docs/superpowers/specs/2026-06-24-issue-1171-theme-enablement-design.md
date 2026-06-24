# Theme Enablement (light/dark/system) — Design

**Issue:** #1171 — Theme enablement: user-selectable light/dark/system theme
**Sibling issues:** #1172 (surface platform/storefront icons), #1173 (source missing icons)
**Date:** 2026-06-24

## Problem

`next-themes` is already wired into the app (`__root.tsx` wraps everything in
`<ThemeProvider attribute="class" defaultTheme="system" enableSystem>`), and the
dark token block already exists in `globals.css` (`@custom-variant dark` + a full
`.dark {}` palette). But there is **no user-facing control** to pick a theme, so
the app is effectively stuck on the system default with no way to override it, and
no signal for theme-aware assets (the icon work in #1172/#1173) to switch on.

So despite how the issue frames it ("add a theme provider at the app root"), the
provider, localStorage persistence, live OS tracking, and `.dark` styles all
**already exist**. The only missing pieces are: a way to choose a theme, and
persisting that choice server-side so it syncs across devices.

## Goal

Let the user choose **System / Light / Dark**, persist the choice in
`user_settings` (server-side, cross-device), and keep `resolvedTheme` from
next-themes as the single source of truth the rest of the UI reads.

## Decisions (from brainstorm with the user)

1. **Persistence: server-side `user_settings.theme`** (not localStorage-only).
   Cross-device sync is wanted. Mirrors the existing `deal_region` / `date_format`
   columns end-to-end.
2. **Control surfaces: a profile Select _and_ a header quick-toggle.** The Select
   sits in the profile Preferences card next to deal-region/date-format; the
   quick-toggle is an icon dropdown in the sidebar (desktop) and the mobile top bar.
3. **Dark-mode polish is out of scope.** Ship the working toggle now; the full
   per-page dark-mode contrast audit is a separate follow-up issue.

## Architecture

next-themes is inherently localStorage-based; the server is the source of truth.
We reconcile them without replacing next-themes:

- **next-themes stays the runtime engine** — it owns applying the `.dark` class and
  live-tracking `prefers-color-scheme`. Provider stays at the app root (unchanged),
  so unauthenticated pages still render with the system default.
- **`user_settings.theme` is the source of truth.** A one-way `ThemeSync`
  component, mounted only inside the authenticated layout, reads `useSettings()`
  and pushes the server value into next-themes via `setTheme()` whenever they
  differ. (It lives in the authenticated zone so the authenticated `/api/settings`
  endpoint is never hit for anonymous visitors.)
- **Writes go to both.** A shared `useThemePreference()` hook exposes `setPref(t)`,
  which calls `setTheme(t)` (instant UI) and `updateSettings.mutate({ theme: t })`
  (persist). next-themes' localStorage acts as a first-paint cache to minimise the
  flash of the wrong theme on cold load.

**Known minor flash:** a brand-new device with no localStorage whose server value
is non-system will paint the system theme for one frame, then snap to the stored
preference once `/api/settings` resolves. Acceptable — the issue explicitly flags
this trade-off for the server-persisted option.

**No loop:** `setPref` updates the React Query cache via the existing
`useUpdateSettings` `onSuccess` (`setQueryData`), but by then `theme` already equals
the server value, so `ThemeSync`'s effect is a no-op.

## Data model

New column, mirroring `date_format`:

```sql
ALTER TABLE user_settings ADD COLUMN theme TEXT NOT NULL DEFAULT 'system';
```

Valid values: exactly `system`, `light`, `dark`. Default `system`. Invalid value on
PATCH → **422** (matches `deal_region` / `date_format`).

Because Bun sends every column on INSERT, **both** sites that construct
`models.UserSettings{}` (`internal/api/settings.go`, `internal/api/changelog.go`)
must set `Theme: defaultTheme` explicitly, or they insert `''` and violate the
intended default.

## API

`/api/settings` GET/PATCH gains a `theme` field alongside `deal_region` /
`date_format`. GET returns `"system"` when no row exists. PATCH accepts optional
`theme`, validated against the closed set.

## Frontend

- `lib/theme.ts` — `type ThemePref = 'system' | 'light' | 'dark'` and a
  `THEME_OPTIONS` array (`{ value, label, icon }`, icons Monitor/Sun/Moon).
- `Settings` type + `api/settings.ts` transform/patch gain `theme`.
- `hooks/use-theme-preference.ts` — `useThemePreference()` → `{ pref, setPref }`.
- `components/theme/theme-sync.tsx` — `<ThemeSync />`, mounted once in
  `AuthenticatedLayout`.
- `components/theme/theme-toggle.tsx` — `<ThemeToggle />` icon dropdown, used in
  `Sidebar` (desktop footer) and `MobileNav` (top bar).
- `profile.tsx` `PreferencesSection` — a Theme `<Select>` using `THEME_OPTIONS`.

## Acceptance criteria (from the issue) → how met

- Switch System / Light / Dark from the UI → profile Select + header toggle.
- Choice persists across reloads → server `user_settings.theme` + next-themes cache.
- "System" tracks the OS live → next-themes `enableSystem` (unchanged).
- `resolvedTheme` available to components → next-themes provider (unchanged).

## Out of scope → follow-up

- Full dark-mode contrast/legibility audit across every page (separate issue).
- The black `*-icon-light.svg` assets and theme-aware icon selection (#1172/#1173).

## Testing

- **Backend** (`internal/api/settings_test.go`, extend): GET default includes
  `theme:"system"`; PATCH valid theme persists; PATCH invalid theme → 422; PATCH of
  one field preserves the others.
- **Frontend** (`components/theme/theme-sync.test.tsx`): `ThemeSync` pushes the
  server value into `setTheme` when they differ, and is a no-op when they match
  (the one-way-sync invariant).
