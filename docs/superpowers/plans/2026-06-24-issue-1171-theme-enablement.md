# Theme Enablement (light/dark/system) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-user `theme` preference (System / Light / Dark), persisted server-side in `user_settings`, with a profile Select and a header quick-toggle, while keeping next-themes' `resolvedTheme` as the single source of truth.

**Architecture:** Mirror the existing `date_format` setting end-to-end: a new `theme` column on `user_settings`, exposed through `/api/settings` GET/PATCH and the frontend `Settings` type. next-themes (already wired at the app root) stays the runtime engine; a one-way `ThemeSync` component pushes the server value into next-themes inside the authenticated layout, and a shared `useThemePreference()` hook writes to both next-themes and the server. The provider, localStorage caching, OS tracking, and `.dark` CSS all already exist — this plan only adds the column, the API field, and the controls.

**Tech Stack:** Go 1.26 + Bun ORM + Echo v5 + PostgreSQL (backend); Vite + React 19 + TypeScript + TanStack Query + next-themes + Vitest (frontend).

## Global Constraints

- Migrations are new files only, named `YYYYMMDD<nnnnnn>_name.up.sql` / `.down.sql`; the next free name is `20260624000002_user_settings_theme` (latest existing is `20260624000001`). Discovered automatically — no registration code.
- The new column is `theme TEXT NOT NULL DEFAULT 'system'`. The three valid values are exactly `system`, `light`, `dark`.
- Bun sends every model column on INSERT, so **every** place that constructs `models.UserSettings{}` must set `Theme` explicitly (else it inserts `''`, violating the intended default). There are exactly two such sites: `internal/api/settings.go` and `internal/api/changelog.go`.
- Backend invalid-value rejection returns HTTP **422** (matches `deal_region` / `date_format`), surfaced verbatim.
- next-themes' `ThemeProvider` already lives at the app root (`ui/frontend/src/routes/__root.tsx`) — do **not** add a second one. The `.dark` token block already exists in `globals.css` — do not touch it.
- The authenticated `/api/settings` endpoint must not be called for anonymous visitors, so the server→client sync component mounts inside `_authenticated`, not at the root.
- Frontend quality gates that must stay green (run from `ui/frontend/`): `npm run check` (tsc + eslint), `npm run knip` (no dead exports), `npm run test`. After adding/removing a route, `npm run build` regenerates `routeTree.gen.ts` — not needed here (no route changes).
- After Go changes that remove callers, run `make deadcode` and reconcile new entries. (This plan only adds; deadcode is not expected to change.)

---

### Task 1: Backend — `theme` column, model, settings API, defaults

**Files:**
- Create: `internal/db/migrations/20260624000002_user_settings_theme.up.sql`
- Create: `internal/db/migrations/20260624000002_user_settings_theme.down.sql`
- Modify: `internal/db/models/models.go` (UserSettings struct, ~L250-258)
- Modify: `internal/api/settings.go` (whole handler)
- Modify: `internal/api/changelog.go:160-167` (setSeen construction)
- Test: `internal/api/settings_test.go` (extend existing test)

**Interfaces:**
- Produces: settings JSON now includes `"theme"` (string, one of `system|light|dark`); PATCH accepts optional `theme`, returns 422 on an invalid value; default is `"system"`.

- [ ] **Step 1: Write the migration pair**

`internal/db/migrations/20260624000002_user_settings_theme.up.sql`:
```sql
ALTER TABLE user_settings ADD COLUMN theme TEXT NOT NULL DEFAULT 'system';
```

`internal/db/migrations/20260624000002_user_settings_theme.down.sql`:
```sql
ALTER TABLE user_settings DROP COLUMN theme;
```

- [ ] **Step 2: Add the model field**

In `internal/db/models/models.go`, add `Theme` to `UserSettings` (place it right after `DateFormat`):
```go
	DealRegion               string    `bun:"deal_region,notnull"            json:"deal_region"`
	DateFormat               string    `bun:"date_format,notnull"            json:"date_format"`
	Theme                    string    `bun:"theme,notnull"                  json:"theme"`
	LastSeenChangelogVersion *string   `bun:"last_seen_changelog_version"    json:"last_seen_changelog_version,omitempty"`
```

- [ ] **Step 3: Extend the settings handler — defaults, validation, response/request**

In `internal/api/settings.go`:

Add the default const and valid-set next to the date-format ones:
```go
const defaultDealRegion = "us"
const defaultDateFormat = "auto"
const defaultTheme = "system"

// validDateFormats is the closed set accepted by PATCH /api/settings.
var validDateFormats = map[string]bool{"auto": true, "iso": true, "dmy": true, "mdy": true}

// validThemes is the closed set accepted by PATCH /api/settings.
var validThemes = map[string]bool{"system": true, "light": true, "dark": true}
```

Extend the response struct:
```go
type settingsResponse struct {
	DealRegion string `json:"deal_region"`
	DateFormat string `json:"date_format"`
	Theme      string `json:"theme"`
}
```

Extend the request struct:
```go
type updateSettingsRequest struct {
	DealRegion *string `json:"deal_region"`
	DateFormat *string `json:"date_format"`
	Theme      *string `json:"theme"`
}
```

In `HandleGet`, the no-row branch and the success branch both build a `settingsResponse` — add `Theme`:
```go
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, settingsResponse{DealRegion: defaultDealRegion, DateFormat: defaultDateFormat, Theme: defaultTheme})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, settingsResponse{DealRegion: s.DealRegion, DateFormat: s.DateFormat, Theme: s.Theme})
```

In `HandlePatch`, set the default on the constructed struct:
```go
	s := models.UserSettings{UserID: userID, DealRegion: defaultDealRegion, DateFormat: defaultDateFormat, Theme: defaultTheme, CreatedAt: now, UpdatedAt: now}
```

Add the validation block after the `DateFormat` one:
```go
	if req.Theme != nil {
		if !validThemes[*req.Theme] {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid theme")
		}
		s.Theme = *req.Theme
	}
```

Add the upsert `Set` and the response `Theme`:
```go
	_, err = h.db.NewInsert().Model(&s).
		On("CONFLICT (user_id) DO UPDATE").
		Set("deal_region = EXCLUDED.deal_region").
		Set("date_format = EXCLUDED.date_format").
		Set("theme = EXCLUDED.theme").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, settingsResponse{DealRegion: s.DealRegion, DateFormat: s.DateFormat, Theme: s.Theme})
```

- [ ] **Step 4: Set the default at the changelog construction site**

In `internal/api/changelog.go`, the `setSeen` `models.UserSettings{...}` literal must set `Theme`:
```go
	s := models.UserSettings{
		UserID:                   userID,
		DealRegion:               defaultDealRegion,
		DateFormat:               defaultDateFormat,
		Theme:                    defaultTheme,
		LastSeenChangelogVersion: &version,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
```

- [ ] **Step 5: Write/extend the failing test**

In `internal/api/settings_test.go`, follow the existing `date_format` assertions. Add a default-includes-theme assertion to the GET test, and a theme block to the PATCH test. Concretely, in the test that checks GET defaults, assert the body contains `"theme":"system"`. Add a new sub-test for PATCH theme:
```go
func TestSettingsPatchTheme(t *testing.T) {
	truncateAllTables(t)
	h, userID := newSettingsTestHandler(t) // use whatever the existing tests use to build handler + seed a user

	// valid theme persists
	rec := patchSettings(t, h, userID, `{"theme":"dark"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH theme: got %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"theme":"dark"`) {
		t.Fatalf("PATCH theme: body missing theme=dark: %s", rec.Body.String())
	}

	// invalid theme → 422
	rec = patchSettings(t, h, userID, `{"theme":"hotpink"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH invalid theme: got %d, want 422; body=%s", rec.Code, rec.Body.String())
	}

	// PATCH of deal_region preserves the previously-set theme
	rec = patchSettings(t, h, userID, `{"deal_region":"gb"}`)
	if !strings.Contains(rec.Body.String(), `"theme":"dark"`) {
		t.Fatalf("PATCH deal_region clobbered theme: %s", rec.Body.String())
	}
}
```
Adapt `newSettingsTestHandler` / `patchSettings` to the actual helpers in the file (read the existing `date_format` test in `settings_test.go` first and copy its exact construction — do not invent helpers).

- [ ] **Step 6: Run the test to verify it fails, then passes**

Run: `go test ./internal/api/... -run TestSettings -v`
Expected before Steps 1-4: compile/fail. After: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrations/20260624000002_user_settings_theme.up.sql internal/db/migrations/20260624000002_user_settings_theme.down.sql internal/db/models/models.go internal/api/settings.go internal/api/changelog.go internal/api/settings_test.go
git commit -m "feat: add theme preference column and /api/settings field"
```

---

### Task 2: Frontend — `theme` in Settings type, api, and `lib/theme.ts`

**Files:**
- Create: `ui/frontend/src/lib/theme.ts`
- Modify: `ui/frontend/src/types/settings.ts`
- Modify: `ui/frontend/src/api/settings.ts`
- Test: `ui/frontend/src/lib/theme.test.ts` (optional, only if non-trivial — skip if it would only assert the constant array)

**Interfaces:**
- Produces: `type ThemePref = 'system' | 'light' | 'dark'`; `THEME_OPTIONS: { value: ThemePref; label: string; icon: LucideIcon }[]`; `Settings.theme: ThemePref`; `getSettings()`/`updateSettings()` round-trip `theme`.

- [ ] **Step 1: Create `lib/theme.ts`**

```ts
import { Monitor, Sun, Moon, type LucideIcon } from 'lucide-react';

export type ThemePref = 'system' | 'light' | 'dark';

export interface ThemeOption {
  value: ThemePref;
  label: string;
  icon: LucideIcon;
}

export const THEME_OPTIONS: ThemeOption[] = [
  { value: 'system', label: 'System', icon: Monitor },
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
];
```

- [ ] **Step 2: Add `theme` to the Settings type**

In `ui/frontend/src/types/settings.ts`:
```ts
import type { DateFormatPref } from '@/lib/format-date';
import type { ThemePref } from '@/lib/theme';

export interface Settings {
  dealRegion: string;
  dateFormat: DateFormatPref;
  theme: ThemePref;
}
```

- [ ] **Step 3: Round-trip `theme` in the settings api**

In `ui/frontend/src/api/settings.ts`:
```ts
import { api } from './client';
import type { Settings } from '@/types/settings';
import type { DateFormatPref } from '@/lib/format-date';
import type { ThemePref } from '@/lib/theme';

interface SettingsApiResponse {
  deal_region: string;
  date_format: DateFormatPref;
  theme: ThemePref;
}

function transform(r: SettingsApiResponse): Settings {
  return { dealRegion: r.deal_region, dateFormat: r.date_format, theme: r.theme };
}

export async function getSettings(): Promise<Settings> {
  return transform(await api.get<SettingsApiResponse>('/settings'));
}

export async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
  const body: Record<string, unknown> = {};
  if (patch.dealRegion !== undefined) body.deal_region = patch.dealRegion;
  if (patch.dateFormat !== undefined) body.date_format = patch.dateFormat;
  if (patch.theme !== undefined) body.theme = patch.theme;
  return transform(await api.patch<SettingsApiResponse>('/settings', body));
}
```

- [ ] **Step 4: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS (no type/lint errors). knip will flag `THEME_OPTIONS`/`ThemePref` as unused until Tasks 3-5 consume them — that's fine within this task; the full `npm run knip` gate is satisfied once Tasks 3-5 land. If running knip standalone now, expect those two unused-export findings and resolve them by completing Tasks 3-5 before the final gate.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/lib/theme.ts ui/frontend/src/types/settings.ts ui/frontend/src/api/settings.ts
git commit -m "feat: add theme to frontend Settings type and api"
```

---

### Task 3: Frontend — `useThemePreference` hook + `ThemeSync` server→client bridge

**Files:**
- Create: `ui/frontend/src/hooks/use-theme-preference.ts`
- Modify: `ui/frontend/src/hooks/index.ts` (export the hook)
- Create: `ui/frontend/src/components/theme/theme-sync.tsx`
- Modify: `ui/frontend/src/routes/_authenticated.tsx` (mount `<ThemeSync />`)
- Test: `ui/frontend/src/components/theme/theme-sync.test.tsx`

**Interfaces:**
- Consumes: `useSettings`/`useUpdateSettings` from `@/hooks`; `useTheme` from `next-themes`; `ThemePref` from `@/lib/theme`.
- Produces: `useThemePreference(): { pref: ThemePref; setPref: (t: ThemePref) => void }`; `<ThemeSync />` (renders null, pushes server theme into next-themes).

- [ ] **Step 1: Create the `useThemePreference` hook**

`ui/frontend/src/hooks/use-theme-preference.ts`:
```ts
import { useTheme } from 'next-themes';
import { useSettings, useUpdateSettings } from '@/hooks';
import type { ThemePref } from '@/lib/theme';

/**
 * Reads the user's theme preference (server-backed) and exposes a setter that
 * updates both next-themes (instant) and the server (persisted). Server is the
 * source of truth; ThemeSync reconciles on load.
 */
export function useThemePreference(): { pref: ThemePref; setPref: (t: ThemePref) => void } {
  const { setTheme } = useTheme();
  const { data } = useSettings();
  const update = useUpdateSettings();

  const pref: ThemePref = data?.theme ?? 'system';
  const setPref = (t: ThemePref) => {
    setTheme(t);
    update.mutate({ theme: t });
  };
  return { pref, setPref };
}
```

Note: `useSettings`/`useUpdateSettings` are re-exported from `@/hooks` (see `hooks/index.ts`). Importing from `@/hooks` inside another hook in the same barrel is the existing pattern (`use-date-format.ts` imports `useSettings` from `./use-settings`). To avoid a circular-import smell, import directly from the leaf modules instead:
```ts
import { useSettings, useUpdateSettings } from './use-settings';
```
(Confirm `useUpdateSettings` is exported from `./use-settings` — it is, see that file.)

- [ ] **Step 2: Export the hook from the barrel**

In `ui/frontend/src/hooks/index.ts`, alongside the `useDateFormat` export:
```ts
export { useThemePreference } from './use-theme-preference';
```

- [ ] **Step 3: Create the `ThemeSync` component**

`ui/frontend/src/components/theme/theme-sync.tsx`:
```tsx
import { useEffect } from 'react';
import { useTheme } from 'next-themes';
import { useSettings } from '@/hooks';

/**
 * One-way bridge: pushes the server-stored theme preference into next-themes
 * once settings load (and whenever it changes), so the server is the source of
 * truth. Renders nothing. Mounted inside the authenticated layout so the
 * authenticated /api/settings endpoint is never hit for anonymous visitors.
 */
export function ThemeSync() {
  const { data } = useSettings();
  const { theme, setTheme } = useTheme();

  useEffect(() => {
    if (data?.theme && data.theme !== theme) {
      setTheme(data.theme);
    }
  }, [data?.theme, theme, setTheme]);

  return null;
}
```

- [ ] **Step 4: Mount `<ThemeSync />` in the authenticated layout**

In `ui/frontend/src/routes/_authenticated.tsx`, add the import and render it once inside `RouteGuard` (it renders null, so placement is cosmetic — put it just inside the outer `<div>`):
```tsx
import { ThemeSync } from '@/components/theme/theme-sync';
```
```tsx
    <RouteGuard>
      <ThemeSync />
      <div className="flex h-screen flex-col md:flex-row">
```

- [ ] **Step 5: Write the `ThemeSync` test**

`ui/frontend/src/components/theme/theme-sync.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/react';
import { ThemeSync } from './theme-sync';

const setTheme = vi.fn();
let mockTheme = 'system';
let mockSettings: { theme?: string } | undefined = undefined;

vi.mock('next-themes', () => ({
  useTheme: () => ({ theme: mockTheme, setTheme }),
}));
vi.mock('@/hooks', () => ({
  useSettings: () => ({ data: mockSettings }),
}));

describe('ThemeSync', () => {
  beforeEach(() => {
    setTheme.mockClear();
    mockTheme = 'system';
    mockSettings = undefined;
  });

  it('pushes the server theme into next-themes when they differ', () => {
    mockSettings = { theme: 'dark' };
    render(<ThemeSync />);
    expect(setTheme).toHaveBeenCalledWith('dark');
  });

  it('is a no-op when server and next-themes already match', () => {
    mockTheme = 'dark';
    mockSettings = { theme: 'dark' };
    render(<ThemeSync />);
    expect(setTheme).not.toHaveBeenCalled();
  });

  it('does nothing before settings load', () => {
    mockSettings = undefined;
    render(<ThemeSync />);
    expect(setTheme).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 6: Run the test**

Run (from `ui/frontend/`): `npm run test theme-sync`
Expected: 3 passing.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/hooks/use-theme-preference.ts ui/frontend/src/hooks/index.ts ui/frontend/src/components/theme/theme-sync.tsx ui/frontend/src/components/theme/theme-sync.test.tsx ui/frontend/src/routes/_authenticated.tsx
git commit -m "feat: sync server theme preference into next-themes"
```

---

### Task 4: Frontend — profile Theme select

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/profile.tsx` (`PreferencesSection`, ~L63-130)

**Interfaces:**
- Consumes: `useThemePreference` from `@/hooks`; `THEME_OPTIONS` from `@/lib/theme`.

- [ ] **Step 1: Add the imports**

In `profile.tsx`, add to the existing import block:
```ts
import { useCollectionStats, gameKeys, useSettings, useUpdateSettings, useThemePreference } from '@/hooks';
import { THEME_OPTIONS, type ThemePref } from '@/lib/theme';
```
(`useSettings`/`useUpdateSettings` are already imported on that line — just add `useThemePreference`.)

- [ ] **Step 2: Add the Theme select to `PreferencesSection`**

Inside `PreferencesSection`, add the hook near the top:
```tsx
function PreferencesSection() {
  const { data: settings } = useSettings();
  const updateSettings = useUpdateSettings();
  const { pref: themePref, setPref: setThemePref } = useThemePreference();
```

Then add a new `<div>` block inside the `CardContent` (after the date-format block):
```tsx
        <div>
          <Label htmlFor="theme">Theme</Label>
          <p className="mb-2 text-sm text-muted-foreground">
            System follows your operating system's light/dark setting.
          </p>
          <Select
            value={themePref}
            onValueChange={(value) => setThemePref(value as ThemePref)}
          >
            <SelectTrigger id="theme" className="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {THEME_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
```

- [ ] **Step 3: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/profile.tsx
git commit -m "feat: add theme select to profile preferences"
```

---

### Task 5: Frontend — `ThemeToggle` quick-toggle in sidebar + mobile nav

**Files:**
- Create: `ui/frontend/src/components/theme/theme-toggle.tsx`
- Modify: `ui/frontend/src/components/navigation/sidebar.tsx`
- Modify: `ui/frontend/src/components/navigation/mobile-nav.tsx`

**Interfaces:**
- Consumes: `useThemePreference` from `@/hooks`; `THEME_OPTIONS` from `@/lib/theme`; `DropdownMenu*` from `@/components/ui/dropdown-menu`; `Button` from `@/components/ui/button`.
- Produces: `<ThemeToggle />` — an icon-button dropdown letting the user pick System/Light/Dark.

- [ ] **Step 1: Create the `ThemeToggle` component**

`ui/frontend/src/components/theme/theme-toggle.tsx`:
```tsx
import { Sun, Moon, Monitor } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useThemePreference } from '@/hooks';
import { THEME_OPTIONS, type ThemePref } from '@/lib/theme';

const ICONS: Record<ThemePref, typeof Sun> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
};

export function ThemeToggle() {
  const { pref, setPref } = useThemePreference();
  const CurrentIcon = ICONS[pref];

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Change theme">
          <CurrentIcon className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        {THEME_OPTIONS.map((o) => {
          const Icon = o.icon;
          return (
            <DropdownMenuItem
              key={o.value}
              onClick={() => setPref(o.value)}
              className="cursor-pointer"
            >
              <Icon className="mr-2 h-4 w-4" />
              <span>{o.label}</span>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
```

- [ ] **Step 2: Place `ThemeToggle` in the desktop sidebar footer**

In `ui/frontend/src/components/navigation/sidebar.tsx`, import it and put it in the bottom "User menu" footer as a row alongside the user dropdown. Add the import:
```ts
import { ThemeToggle } from '@/components/theme/theme-toggle';
```
Change the footer `<div className="p-4 border-t">` so the user button and toggle share a row:
```tsx
      {/* User menu at bottom */}
      <div className="p-4 border-t flex items-center gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="flex-1 justify-between">
```
(only the wrapper `<div>` className and the trigger `Button` className change: add `flex items-center gap-2` to the div, and change the trigger Button from `w-full` to `flex-1`.)

Then add the toggle as the last child of that footer `<div>`, after the closing `</DropdownMenu>`:
```tsx
        </DropdownMenu>
        <ThemeToggle />
      </div>
```

- [ ] **Step 3: Place `ThemeToggle` in the mobile top bar**

In `ui/frontend/src/components/navigation/mobile-nav.tsx`, import it and add it to the right-hand cluster next to the avatar. Add the import:
```ts
import { ThemeToggle } from '@/components/theme/theme-toggle';
```
Wrap the right-hand avatar in a flex row with the toggle. Replace:
```tsx
      {/* Avatar on right */}
      <Link to="/profile">
        <Avatar className="h-8 w-8">
          <AvatarFallback>{user?.username?.charAt(0).toUpperCase()}</AvatarFallback>
        </Avatar>
      </Link>
```
with:
```tsx
      {/* Right-hand controls */}
      <div className="flex items-center gap-1">
        <ThemeToggle />
        <Link to="/profile">
          <Avatar className="h-8 w-8">
            <AvatarFallback>{user?.username?.charAt(0).toUpperCase()}</AvatarFallback>
          </Avatar>
        </Link>
      </div>
```

- [ ] **Step 4: Verify gates green**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all pass; knip reports no unused exports (THEME_OPTIONS, ThemePref, useThemePreference, ThemeToggle, ThemeSync all now consumed).

- [ ] **Step 5: Manual smoke (optional but recommended)**

Build the frontend and confirm: profile Theme select switches the app between light/dark instantly; the sidebar/mobile toggle does the same; reload preserves the choice; with "System", flipping the OS appearance flips the app live.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/components/theme/theme-toggle.tsx ui/frontend/src/components/navigation/sidebar.tsx ui/frontend/src/components/navigation/mobile-nav.tsx
git commit -m "feat: add theme quick-toggle to sidebar and mobile nav"
```

---

### Task 6: Commit the design + plan docs

**Files:**
- Create: `docs/superpowers/specs/2026-06-24-issue-1171-theme-enablement-design.md` (already written)
- Create: `docs/superpowers/plans/2026-06-24-issue-1171-theme-enablement.md` (this file)

- [ ] **Step 1: Commit the docs** (do this first, before Task 1, per the project's plan-on-branch rule)

```bash
git add docs/superpowers/specs/2026-06-24-issue-1171-theme-enablement-design.md docs/superpowers/plans/2026-06-24-issue-1171-theme-enablement.md
git commit -m "docs: spec + plan for theme enablement (#1171)"
```

---

## Self-Review

**Spec coverage:**
- Switch System/Light/Dark from UI → Task 4 (profile select) + Task 5 (toggle). ✓
- Persists across reloads → Task 1 (server column) + Task 3 (sync). ✓
- "System" tracks OS live → next-themes `enableSystem`, unchanged (noted in plan). ✓
- `resolvedTheme` available → next-themes provider, unchanged. ✓
- Server-side persistence decision → Task 1 + Task 2 + Task 3. ✓
- Profile select + header toggle decision → Tasks 4 + 5. ✓
- Dark-mode audit out of scope → not in any task; follow-up issue filed separately. ✓

**Placeholder scan:** Task 1 Step 5 intentionally defers to "read the existing `date_format` test and copy its exact helpers" because the test file's helper names weren't read during planning — this is a directed instruction, not a vague placeholder. All code steps show concrete code.

**Type consistency:** `ThemePref` ('system'|'light'|'dark') used identically in `lib/theme.ts`, `types/settings.ts`, `api/settings.ts`, `use-theme-preference.ts`, `theme-toggle.tsx`, and `profile.tsx`. `setPref`/`pref` names consistent across hook + consumers. Backend `theme` column / `validThemes` / `defaultTheme` consistent across migration, model, handler, changelog.

## Follow-up issue to file (post-merge or alongside)

Full dark-mode contrast/legibility audit across all pages now that dark mode is reachable — label `enhancement`. (The black `*-icon-light.svg` assets are already tracked by #1172/#1173.)
