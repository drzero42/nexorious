# Date-Format Preference + Locale-Aware Date Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop forcing US date format; add a per-user `date_format` preference (Auto / ISO / DD-MM / MM-DD) and route every frontend date render through one central, preference-aware util.

**Architecture:** Mirror the existing `deal_region` setting end-to-end: a new `date_format` column on `user_settings`, exposed through the `/api/settings` GET/PATCH handler and the frontend `Settings` type. A new leaf util `lib/format-date.ts` holds pure `formatDate`/`formatDateTime` functions taking an explicit preference; a `useDateFormat()` hook binds them to the value from `useSettings()` (React Query) so call sites stay reactive. All ~13 inline `toLocaleDateString`/`toLocaleString` helpers (including the 5 hardcoded `'en-US'` ones) are replaced with the hook.

**Tech Stack:** Go 1.26 + Bun ORM + Echo v5 + PostgreSQL (backend); Vite + React 19 + TypeScript + TanStack Query + Vitest (frontend).

## Global Constraints

- Migrations are new files only, named `YYYYMMDD<nnnnnn>_name.up.sql` / `.down.sql`; the next free name is `20260623000001_user_settings_date_format` (latest existing is `20260622000002`). Discovered automatically — no registration code.
- `user_settings` columns are NOT-NULL with defaults; the new column is `date_format TEXT NOT NULL DEFAULT 'auto'`. The four valid values are exactly `auto`, `iso`, `dmy`, `mdy`.
- Bun sends every model column on INSERT, so **every** place that constructs `models.UserSettings{}` must set `DateFormat` explicitly (else it inserts `''`, violating the intended default). There are two such sites: `internal/api/settings.go` and `internal/api/changelog.go`.
- Backend invalid-value rejection returns HTTP **422** (matches `deal_region`), surfaced verbatim.
- Date+time displays keep a **fixed 24-hour** time; only the date portion follows the preference. No 12h/24h toggle (out of scope).
- `formatRelativeTime` (in `types/jobs.ts`) keeps its "2h ago" logic; only its absolute-date fallback (for dates > 7 days old) routes through the new util.
- Frontend quality gates that must stay green: `npm run check` (tsc + eslint), `npm run knip` (no dead exports), `npm run test`. Run from `ui/frontend/`.
- After Go changes that remove callers, run `make deadcode` and reconcile new entries.

**Intentional behaviour change (call out in the PR):** Several admin/add-confirm screens currently render long-month dates ("Jun 23, 2026" / "June 23, 2026"). After consolidation they render numeric short dates ("06/23/2026" or the locale/preference equivalent). This is the deliberate consistency fix the issue asks for — there are only two output shapes now: date-only and date+24h-time.

---

### Task 1: Backend — `date_format` column, model, settings API, defaults

**Files:**
- Create: `internal/db/migrations/20260623000001_user_settings_date_format.up.sql`
- Create: `internal/db/migrations/20260623000001_user_settings_date_format.down.sql`
- Modify: `internal/db/models/models.go` (UserSettings struct, ~L249-258)
- Modify: `internal/api/settings.go` (whole handler)
- Modify: `internal/api/changelog.go:161-167` (setSeen construction)
- Test: `internal/api/settings_test.go` (extend existing test)

**Interfaces:**
- Produces: settings JSON now includes `"date_format"` (string, one of `auto|iso|dmy|mdy`); PATCH accepts optional `date_format`, returns 422 on an invalid value; default is `"auto"`.

- [ ] **Step 1: Write the migration pair**

`internal/db/migrations/20260623000001_user_settings_date_format.up.sql`:
```sql
ALTER TABLE user_settings ADD COLUMN date_format TEXT NOT NULL DEFAULT 'auto';
```

`internal/db/migrations/20260623000001_user_settings_date_format.down.sql`:
```sql
ALTER TABLE user_settings DROP COLUMN date_format;
```

- [ ] **Step 2: Add the model field**

In `internal/db/models/models.go`, add `DateFormat` to `UserSettings` (place it right after `DealRegion`):
```go
	DealRegion               string    `bun:"deal_region,notnull"            json:"deal_region"`
	DateFormat               string    `bun:"date_format,notnull"            json:"date_format"`
	LastSeenChangelogVersion *string   `bun:"last_seen_changelog_version"    json:"last_seen_changelog_version,omitempty"`
```

- [ ] **Step 3: Extend the settings handler**

In `internal/api/settings.go`:

Add the default const next to `defaultDealRegion`:
```go
const defaultDealRegion = "us"
const defaultDateFormat = "auto"

// validDateFormats is the closed set accepted by PATCH /api/settings.
var validDateFormats = map[string]bool{"auto": true, "iso": true, "dmy": true, "mdy": true}
```

Extend the response struct:
```go
type settingsResponse struct {
	DealRegion string `json:"deal_region"`
	DateFormat string `json:"date_format"`
}
```

In `HandleGet`, update both return sites:
```go
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, settingsResponse{DealRegion: defaultDealRegion, DateFormat: defaultDateFormat})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, settingsResponse{DealRegion: s.DealRegion, DateFormat: s.DateFormat})
```

Extend the request struct:
```go
type updateSettingsRequest struct {
	DealRegion *string `json:"deal_region"`
	DateFormat *string `json:"date_format"`
}
```

In `HandlePatch`, set the default on the fresh struct and handle the new field. Update the construction line:
```go
	s := models.UserSettings{UserID: userID, DealRegion: defaultDealRegion, DateFormat: defaultDateFormat, CreatedAt: now, UpdatedAt: now}
```

After the existing `if req.DealRegion != nil { ... }` block, add:
```go
	if req.DateFormat != nil {
		if !validDateFormats[*req.DateFormat] {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid date_format")
		}
		s.DateFormat = *req.DateFormat
	}
```

Add the column to the upsert `Set` chain (after the `deal_region` Set):
```go
	_, err = h.db.NewInsert().Model(&s).
		On("CONFLICT (user_id) DO UPDATE").
		Set("deal_region = EXCLUDED.deal_region").
		Set("date_format = EXCLUDED.date_format").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
```

Update the final PATCH return:
```go
	return c.JSON(http.StatusOK, settingsResponse{DealRegion: s.DealRegion, DateFormat: s.DateFormat})
```

- [ ] **Step 4: Fix the changelog row construction**

In `internal/api/changelog.go`, the `setSeen` upsert constructs a `models.UserSettings{}`. Add `DateFormat`:
```go
	s := models.UserSettings{
		UserID:                   userID,
		DealRegion:               defaultDealRegion,
		DateFormat:               defaultDateFormat,
		LastSeenChangelogVersion: &version,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
```
(`defaultDateFormat` is in the same `api` package, so it resolves.)

- [ ] **Step 5: Extend the settings test**

In `internal/api/settings_test.go`, after the existing invalid-region assertion (end of `TestSettings_GetDefaultAndPatch`), add:
```go
	// date_format defaults to "auto" on a fresh GET.
	rec = doGetSettings(t)
	got = decodeResp(t, rec)
	if got["date_format"] != "auto" {
		t.Fatalf("default date_format want auto, got %v", got["date_format"])
	}

	// PATCH date_format round-trips and does not disturb deal_region.
	rec = doPatchSettings(t, `{"date_format":"dmy"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH date_format want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got = decodeResp(t, rec)
	if got["date_format"] != "dmy" {
		t.Fatalf("PATCH response want date_format=dmy, got %v", got["date_format"])
	}
	if got["deal_region"] != "gb" {
		t.Fatalf("PATCH date_format must preserve deal_region=gb, got %v", got["deal_region"])
	}

	// GET reflects the persisted date_format.
	rec = doGetSettings(t)
	got = decodeResp(t, rec)
	if got["date_format"] != "dmy" {
		t.Fatalf("GET after PATCH want date_format=dmy, got %v", got["date_format"])
	}

	// Invalid date_format is rejected with 422.
	rec = doPatchSettings(t, `{"date_format":"bogus"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for invalid date_format, got %d: %s", rec.Code, rec.Body.String())
	}
```

- [ ] **Step 6: Run the backend test**

Run: `go test ./internal/api/... -run TestSettings_GetDefaultAndPatch -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrations/20260623000001_user_settings_date_format.up.sql \
        internal/db/migrations/20260623000001_user_settings_date_format.down.sql \
        internal/db/models/models.go internal/api/settings.go internal/api/changelog.go \
        internal/api/settings_test.go
git commit -m "feat: add date_format user setting (backend)"
```

---

### Task 2: `lib/format-date.ts` — pure preference-aware formatters

**Files:**
- Create: `ui/frontend/src/lib/format-date.ts`
- Test: `ui/frontend/src/lib/format-date.test.ts`

**Interfaces:**
- Produces:
  - `type DateFormatPref = 'auto' | 'iso' | 'dmy' | 'mdy'`
  - `formatDate(value: string | number | Date | null | undefined, pref?: DateFormatPref, nullLabel?: string): string`
  - `formatDateTime(value: string | number | Date | null | undefined, pref?: DateFormatPref, nullLabel?: string): string` — date portion follows `pref`, then a space, then 24-hour `HH:MM` (local time).
  - `DATE_FORMAT_OPTIONS: { value: DateFormatPref; label: string }[]` — for the profile select.

- [ ] **Step 1: Write the failing test**

`ui/frontend/src/lib/format-date.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import { formatDate, formatDateTime } from './format-date';

// Construct dates from LOCAL components so assertions are timezone-independent.
const d = new Date(2026, 5, 23, 14, 30, 0); // 2026-06-23 14:30 local
const single = new Date(2026, 0, 5, 9, 7, 0); // 2026-01-05 09:07 local

describe('formatDate', () => {
  it('iso → YYYY-MM-DD', () => {
    expect(formatDate(d, 'iso')).toBe('2026-06-23');
    expect(formatDate(single, 'iso')).toBe('2026-01-05');
  });
  it('dmy → DD-MM-YYYY', () => {
    expect(formatDate(d, 'dmy')).toBe('23-06-2026');
  });
  it('mdy → MM-DD-YYYY', () => {
    expect(formatDate(d, 'mdy')).toBe('06-23-2026');
  });
  it('auto returns a non-empty locale string', () => {
    expect(formatDate(d, 'auto')).not.toBe('');
    expect(formatDate(d, 'auto')).not.toBe('-');
  });
  it('defaults to auto when no pref given', () => {
    expect(formatDate(d)).toBe(formatDate(d, 'auto'));
  });
  it('returns the null label for missing/invalid input', () => {
    expect(formatDate(null, 'iso')).toBe('-');
    expect(formatDate(undefined, 'iso', 'Never')).toBe('Never');
    expect(formatDate('not-a-date', 'iso')).toBe('-');
  });
});

describe('formatDateTime', () => {
  it('appends fixed 24-hour time to the formatted date', () => {
    expect(formatDateTime(d, 'iso')).toBe('2026-06-23 14:30');
    expect(formatDateTime(single, 'iso')).toBe('2026-01-05 09:07');
  });
  it('honours pref for the date portion', () => {
    expect(formatDateTime(d, 'dmy')).toBe('23-06-2026 14:30');
  });
  it('returns the null label for missing input', () => {
    expect(formatDateTime(null, 'iso', 'Never')).toBe('Never');
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run (from `ui/frontend/`): `npm run test format-date.test.ts`
Expected: FAIL — `format-date.ts` does not exist.

- [ ] **Step 3: Write the implementation**

`ui/frontend/src/lib/format-date.ts`:
```ts
export type DateFormatPref = 'auto' | 'iso' | 'dmy' | 'mdy';

export const DATE_FORMAT_OPTIONS: { value: DateFormatPref; label: string }[] = [
  { value: 'auto', label: 'Auto (use browser locale)' },
  { value: 'iso', label: 'YYYY-MM-DD' },
  { value: 'dmy', label: 'DD-MM-YYYY' },
  { value: 'mdy', label: 'MM-DD-YYYY' },
];

function toDate(value: string | number | Date | null | undefined): Date | null {
  if (value === null || value === undefined || value === '') return null;
  const d = value instanceof Date ? value : new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/**
 * Format the date portion of `value` per the user's preference.
 * `auto` follows the browser locale (numeric short); the explicit prefs build
 * a fixed numeric order from LOCAL date components.
 */
export function formatDate(
  value: string | number | Date | null | undefined,
  pref: DateFormatPref = 'auto',
  nullLabel = '-',
): string {
  const d = toDate(value);
  if (!d) return nullLabel;

  if (pref === 'auto') {
    return d.toLocaleDateString(undefined, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  }

  const y = String(d.getFullYear());
  const m = pad(d.getMonth() + 1);
  const day = pad(d.getDate());
  switch (pref) {
    case 'iso':
      return `${y}-${m}-${day}`;
    case 'dmy':
      return `${day}-${m}-${y}`;
    case 'mdy':
      return `${m}-${day}-${y}`;
  }
}

/**
 * Format `value` as the preference-aware date plus a fixed 24-hour HH:MM
 * (local time), separated by a space.
 */
export function formatDateTime(
  value: string | number | Date | null | undefined,
  pref: DateFormatPref = 'auto',
  nullLabel = '-',
): string {
  const d = toDate(value);
  if (!d) return nullLabel;
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  return `${formatDate(d, pref)} ${time}`;
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run (from `ui/frontend/`): `npm run test format-date.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/lib/format-date.ts ui/frontend/src/lib/format-date.test.ts
git commit -m "feat: add preference-aware date formatting util"
```

---

### Task 3: Frontend settings plumbing — types, API transform, test mocks

**Files:**
- Modify: `ui/frontend/src/types/settings.ts`
- Modify: `ui/frontend/src/api/settings.ts`
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.test.tsx` (mock objects at ~L83 and ~L253)

**Interfaces:**
- Consumes: `DateFormatPref` from `@/lib/format-date` (Task 2).
- Produces: `Settings.dateFormat: DateFormatPref`; `getSettings()`/`updateSettings()` round-trip it via the `date_format` wire field.

- [ ] **Step 1: Extend the `Settings` type**

`ui/frontend/src/types/settings.ts`:
```ts
import type { DateFormatPref } from '@/lib/format-date';

export interface Settings {
  dealRegion: string;
  dateFormat: DateFormatPref;
}
```

- [ ] **Step 2: Extend the API transform + patch**

`ui/frontend/src/api/settings.ts`:
```ts
import { api } from './client';
import type { Settings } from '@/types/settings';
import type { DateFormatPref } from '@/lib/format-date';

interface SettingsApiResponse {
  deal_region: string;
  date_format: DateFormatPref;
}

function transform(r: SettingsApiResponse): Settings {
  return { dealRegion: r.deal_region, dateFormat: r.date_format };
}

export async function getSettings(): Promise<Settings> {
  return transform(await api.get<SettingsApiResponse>('/settings'));
}

export async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
  const body: Record<string, unknown> = {};
  if (patch.dealRegion !== undefined) body.deal_region = patch.dealRegion;
  if (patch.dateFormat !== undefined) body.date_format = patch.dateFormat;
  return transform(await api.patch<SettingsApiResponse>('/settings', body));
}
```

- [ ] **Step 3: Fix the test mocks that now miss a required field**

In `ui/frontend/src/routes/_authenticated/games/$id.index.test.tsx`, the two `useSettings` mocks return `data: { dealRegion: 'us' }`. Add the new field to both (lines ~83 and ~253):
```ts
      data: { dealRegion: 'us', dateFormat: 'auto' },
```

- [ ] **Step 4: Typecheck + run the affected test**

Run (from `ui/frontend/`):
```bash
npm run check
npm run test '$id.index.test.tsx'
```
Expected: tsc passes (no missing-property errors); the game-detail test passes.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/types/settings.ts ui/frontend/src/api/settings.ts \
        ui/frontend/src/routes/_authenticated/games/\$id.index.test.tsx
git commit -m "feat: plumb dateFormat through frontend settings types"
```

---

### Task 4: `useDateFormat()` hook

**Files:**
- Create: `ui/frontend/src/hooks/use-date-format.ts`
- Modify: `ui/frontend/src/hooks/index.ts` (add re-export)
- Modify: `ui/frontend/src/types/jobs.ts` (add `pref` param to `formatRelativeTime`)
- Test: `ui/frontend/src/hooks/use-date-format.test.tsx`

**Interfaces:**
- Consumes: `formatDate`/`formatDateTime`/`DateFormatPref` from `@/lib/format-date`; `formatRelativeTime` from `@/types/jobs`; `useSettings` from `./use-settings`.
- Produces: `useDateFormat(): { formatDate(value, nullLabel?): string; formatDateTime(value, nullLabel?): string; formatRelativeTime(value, nullLabel?): string }`, each bound to the current `settings.dateFormat` (defaulting to `'auto'`).

- [ ] **Step 1: Thread `pref` into `formatRelativeTime`**

In `ui/frontend/src/types/jobs.ts`, add the import at the top of the file (with the other imports):
```ts
import { formatDate, type DateFormatPref } from '@/lib/format-date';
```
Change the signature and the fallback line:
```ts
export function formatRelativeTime(
  dateStr: string | null,
  nullLabel = '-',
  pref: DateFormatPref = 'auto',
): string {
  if (!dateStr) return nullLabel;
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return formatDate(date, pref);
}
```
(The existing call sites that pass only `(dateStr)` or `(dateStr, 'Never')` keep working — `pref` defaults to `'auto'`. They are migrated to the hook in Task 7.)

- [ ] **Step 2: Write the hook**

`ui/frontend/src/hooks/use-date-format.ts`:
```ts
import { useMemo } from 'react';
import { useSettings } from './use-settings';
import {
  formatDate as fmtDate,
  formatDateTime as fmtDateTime,
  type DateFormatPref,
} from '@/lib/format-date';
import { formatRelativeTime as fmtRelative } from '@/types/jobs';

type DateValue = string | number | Date | null | undefined;

/**
 * Returns date formatters bound to the user's date_format preference
 * (from useSettings, defaulting to 'auto'). Use these instead of inline
 * toLocaleDateString/toLocaleString so all dates honour the preference.
 */
export function useDateFormat() {
  const { data } = useSettings();
  const pref: DateFormatPref = data?.dateFormat ?? 'auto';
  return useMemo(
    () => ({
      formatDate: (value: DateValue, nullLabel = '-') => fmtDate(value, pref, nullLabel),
      formatDateTime: (value: DateValue, nullLabel = '-') => fmtDateTime(value, pref, nullLabel),
      formatRelativeTime: (value: string | null, nullLabel = '-') =>
        fmtRelative(value, nullLabel, pref),
    }),
    [pref],
  );
}
```

- [ ] **Step 3: Re-export from the hooks barrel**

In `ui/frontend/src/hooks/index.ts`, add near the `use-settings` re-export:
```ts
export { useDateFormat } from './use-date-format';
```

- [ ] **Step 4: Write the hook test**

`ui/frontend/src/hooks/use-date-format.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useDateFormat } from './use-date-format';

const mockSettings = vi.fn();
vi.mock('./use-settings', () => ({
  useSettings: () => mockSettings(),
}));

describe('useDateFormat', () => {
  beforeEach(() => mockSettings.mockReset());

  it('binds formatters to the dmy preference', () => {
    mockSettings.mockReturnValue({ data: { dealRegion: 'us', dateFormat: 'dmy' } });
    const { result } = renderHook(() => useDateFormat());
    expect(result.current.formatDate(new Date(2026, 5, 23))).toBe('23-06-2026');
    expect(result.current.formatDateTime(new Date(2026, 5, 23, 14, 30))).toBe('23-06-2026 14:30');
  });

  it('falls back to auto when settings are not loaded', () => {
    mockSettings.mockReturnValue({ data: undefined });
    const { result } = renderHook(() => useDateFormat());
    expect(result.current.formatDate(null)).toBe('-');
    // auto path returns a non-empty locale string, not the iso literal
    expect(result.current.formatDate(new Date(2026, 5, 23))).not.toBe('2026-06-23');
  });
});
```

- [ ] **Step 5: Run the hook test + typecheck**

Run (from `ui/frontend/`):
```bash
npm run test use-date-format.test.tsx
npm run check
```
Expected: PASS; tsc clean.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/hooks/use-date-format.ts ui/frontend/src/hooks/index.ts \
        ui/frontend/src/hooks/use-date-format.test.tsx ui/frontend/src/types/jobs.ts
git commit -m "feat: add useDateFormat hook bound to user preference"
```

---

### Task 5: Profile Preferences — date format select

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/profile.tsx` (`PreferencesSection`, ~L62-97)

**Interfaces:**
- Consumes: `DATE_FORMAT_OPTIONS`/`DateFormatPref` from `@/lib/format-date`; `useSettings`/`useUpdateSettings` (already imported).

- [ ] **Step 1: Import the options**

In `ui/frontend/src/routes/_authenticated/profile.tsx`, add to the imports (next to `DEAL_REGIONS`):
```ts
import { DATE_FORMAT_OPTIONS, type DateFormatPref } from '@/lib/format-date';
```

- [ ] **Step 2: Add the control to `PreferencesSection`**

Inside the `<CardContent className="space-y-4">` of `PreferencesSection`, add a second block after the existing Deal-region `<div>`:
```tsx
        <div>
          <Label htmlFor="dateFormat">Date format</Label>
          <p className="mb-2 text-sm text-muted-foreground">
            How dates are displayed throughout the app. Auto follows your browser locale.
          </p>
          <Select
            value={settings?.dateFormat ?? 'auto'}
            onValueChange={(value) => updateSettings.mutate({ dateFormat: value as DateFormatPref })}
          >
            <SelectTrigger id="dateFormat" className="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DATE_FORMAT_OPTIONS.map((o) => (
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
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/profile.tsx
git commit -m "feat: add date format control to profile preferences"
```

---

### Task 6: Migrate admin date+time screens to `formatDateTime`

These five files each have ONE module-level helper (`formatDate`/`formatWhen`) that renders a date **with time**, called inside a single component. For each: delete the helper, call `const { formatDateTime } = useDateFormat();` at the top of the component, and replace each helper invocation with `formatDateTime(...)`.

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/admin/index.tsx` (delete `formatDate` L18-27; component `AdminDashboardPage` L46; call L204)
- Modify: `ui/frontend/src/routes/_authenticated/admin/users/index.tsx` (delete `formatDate` L37-46; component `AdminUsersPage` L90; calls L302, L342, L345)
- Modify: `ui/frontend/src/routes/_authenticated/admin/users/$id.tsx` (delete `formatDate` L40-49; component `EditUserPage` L90; calls L320, L371)
- Modify: `ui/frontend/src/routes/_authenticated/admin/activity/index.tsx` (delete `formatWhen` L38-47; component `AdminActivityPage` L48; call L272)
- Modify: `ui/frontend/src/routes/_authenticated/admin/backups.tsx` (delete `formatDate` L76-78; component `BackupPage` L113; calls L527, L608)

**Interfaces:**
- Consumes: `useDateFormat` from `@/hooks`.

- [ ] **Step 1: Migrate `admin/index.tsx`**

Add the import (with the other `@/hooks` imports; `admin/index.tsx` already imports from `@/hooks` — extend that line, or add a new import):
```ts
import { useDateFormat } from '@/hooks';
```
Delete the `function formatDate(dateString: string) {...}` block (L18-27). At the top of `AdminDashboardPage` (after `function AdminDashboardPage() {`), add:
```ts
  const { formatDateTime } = useDateFormat();
```
Replace the call at L204 `Created {formatDate(user.createdAt)}` → `Created {formatDateTime(user.createdAt)}`.

- [ ] **Step 2: Migrate `admin/users/index.tsx`**

Same pattern: add `import { useDateFormat } from '@/hooks';`, delete the `formatDate` helper (L37-46), add `const { formatDateTime } = useDateFormat();` at the top of `AdminUsersPage` (L90), and replace all three calls:
- L302 `formatDate(user.createdAt)` → `formatDateTime(user.createdAt)`
- L342 `formatDate(user.createdAt)` → `formatDateTime(user.createdAt)`
- L345 `formatDate(user.updatedAt)` → `formatDateTime(user.updatedAt)`

Verify with `grep -n 'formatDate(' src/routes/_authenticated/admin/users/index.tsx` afterwards — expect zero matches.

- [ ] **Step 3: Migrate `admin/users/$id.tsx`**

Add `import { useDateFormat } from '@/hooks';`, delete the `formatDate` helper (L40-49), add `const { formatDateTime } = useDateFormat();` at the top of `EditUserPage` (L90), and replace:
- L320 `formatDate(user.createdAt)` → `formatDateTime(user.createdAt)`
- L371 `formatDate(user.updatedAt)` → `formatDateTime(user.updatedAt)`

- [ ] **Step 4: Migrate `admin/activity/index.tsx`**

Add `import { useDateFormat } from '@/hooks';`, delete the `formatWhen` helper (L38-47), add `const { formatDateTime } = useDateFormat();` at the top of `AdminActivityPage` (L48), and replace L272 `formatWhen(e.occurredAt)` → `formatDateTime(e.occurredAt)`.

- [ ] **Step 5: Migrate `admin/backups.tsx`**

Add `import { useDateFormat } from '@/hooks';`, delete the `formatDate` helper (L76-78), add `const { formatDateTime } = useDateFormat();` at the top of `BackupPage` (L113), and replace:
- L527 `formatDate(backup.createdAt)` → `formatDateTime(backup.createdAt)`
- L608 `formatDate(restoreBackup.createdAt)` → `formatDateTime(restoreBackup.createdAt)`

- [ ] **Step 6: Verify no stragglers + typecheck**

Run (from `ui/frontend/`):
```bash
grep -rn "en-US\|function formatDate\|function formatWhen" src/routes/_authenticated/admin/ || echo "clean"
npm run check
```
Expected: no `en-US`/local-helper matches in `admin/`; tsc clean.

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/admin/
git commit -m "refactor: route admin date+time displays through useDateFormat"
```

---

### Task 7: Migrate date-only displays + `formatRelativeTime` callers to the hook

This sweeps the remaining sites: date-only renders become `formatDate`; the five `formatRelativeTime` import sites switch to the hook's bound version (so the >7-day fallback honours the preference).

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` (L388 release_date, L523 acquired_date — date only; component already uses `useSettings`)
- Modify: `ui/frontend/src/routes/_authenticated/games/add.confirm.tsx` (delete `formatReleaseDate` L50-60; replace its call with hook `formatDate`)
- Modify: `ui/frontend/src/components/notifications/notifications-section.tsx` (L144 created_at — date only)
- Modify: `ui/frontend/src/routes/_authenticated/tags.tsx` (L466 created_at — date only)
- Modify: `ui/frontend/src/components/jobs/job-card.tsx` (L23 import, L65 + L89 `formatRelativeTime` → hook)
- Modify: `ui/frontend/src/components/jobs/recent-activity.tsx` (L18 import, L228 → hook)
- Modify: `ui/frontend/src/components/sync/sync-service-card.tsx` (L9 import, L62 → hook)
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` (L66 import, L400 → hook)
- Modify: `ui/frontend/src/components/api-keys/api-keys-section.tsx` (L19 import, L80 relative, L82 + L84 created/expires — both kinds)

**Interfaces:**
- Consumes: `useDateFormat` from `@/hooks`.

- [ ] **Step 1: `games/$id.index.tsx` (date-only)**

This component already calls `useSettings()` (for deal links). Add `import { useDateFormat } from '@/hooks';` and, near the existing `const settings = ...`/`buildDealLinks` usage in the component, add:
```ts
  const { formatDate } = useDateFormat();
```
Replace:
- L388 `{new Date(game.game.release_date).toLocaleDateString()}` → `{formatDate(game.game.release_date)}`
- L523 `{new Date(p.acquired_date).toLocaleDateString()}` → `{formatDate(p.acquired_date)}`

- [ ] **Step 2: `games/add.confirm.tsx` (date-only, delete helper)**

Delete the `formatReleaseDate` helper (L50-60). Add `import { useDateFormat } from '@/hooks';`. In the component that renders the confirm view, add `const { formatDate } = useDateFormat();` at the top, and replace the single `formatReleaseDate(<arg>)` call site with `formatDate(<arg>)`. (Find it with `grep -n 'formatReleaseDate(' src/routes/_authenticated/games/add.confirm.tsx`. The old helper returned `null` for missing/invalid input; `formatDate` returns `'-'` instead — if the call site branches on a `null` return, pass `formatDate(arg, '')` and keep the surrounding conditional, or render `formatDate(arg)` directly where a string is always acceptable. Inspect the call site and preserve its empty-state behaviour.)

- [ ] **Step 3: `notifications-section.tsx` (date-only)**

Add `import { useDateFormat } from '@/hooks';`, add `const { formatDate } = useDateFormat();` at the top of the component, replace L144 `Added {new Date(ch.created_at).toLocaleDateString()}` → `Added {formatDate(ch.created_at)}`.

- [ ] **Step 4: `tags.tsx` (date-only)**

Add `import { useDateFormat } from '@/hooks';`, add `const { formatDate } = useDateFormat();` at the top of the component rendering L466, replace `Created {new Date(tag.created_at).toLocaleDateString()}` → `Created {formatDate(tag.created_at)}`.

- [ ] **Step 5: `job-card.tsx` (relative → hook)**

Remove `formatRelativeTime` from the `@/types/jobs` import (L23). Add `import { useDateFormat } from '@/hooks';`, add `const { formatRelativeTime } = useDateFormat();` at the top of the component. The calls at L65 `formatRelativeTime(job.createdAt)` and L89 `formatRelativeTime(job.startedAt || job.createdAt)` stay as-is (now resolved from the hook).

- [ ] **Step 6: `recent-activity.tsx` (relative → hook)**

Remove `formatRelativeTime` from the `@/types/jobs` import (L18), add `import { useDateFormat } from '@/hooks';` + `const { formatRelativeTime } = useDateFormat();` at the top of the component; L228 call stays.

- [ ] **Step 7: `sync-service-card.tsx` (relative → hook)**

Remove the `formatRelativeTime` import (L9), add `import { useDateFormat } from '@/hooks';` + `const { formatRelativeTime } = useDateFormat();` at the top of the component; L62 `formatRelativeTime(config.lastSyncedAt, 'Never')` stays.

- [ ] **Step 8: `sync/$storefront.tsx` (relative → hook)**

Remove the `formatRelativeTime` import (L66), add `import { useDateFormat } from '@/hooks';` + `const { formatRelativeTime } = useDateFormat();` at the top of the component; L400 `formatRelativeTime(config.lastSyncedAt, 'Never')` stays.

- [ ] **Step 9: `api-keys-section.tsx` (both kinds)**

Remove the `formatRelativeTime` import (L19). Add `import { useDateFormat } from '@/hooks';`, add `const { formatRelativeTime, formatDate } = useDateFormat();` at the top of the component. Replace:
- L80 `Last used ${formatRelativeTime(key.last_used_at)}` stays (now from the hook).
- L82 `Created ${new Date(key.created_at).toLocaleDateString()}` → `Created ${formatDate(key.created_at)}`
- L84 `Expires ${new Date(key.expires_at).toLocaleDateString()}` → `Expires ${formatDate(key.expires_at)}`

(These are inside a template literal; ensure the JS expression form `${formatDate(key.created_at)}` is used.)

- [ ] **Step 10: Verify the whole sweep + run gates**

Run (from `ui/frontend/`):
```bash
grep -rn "toLocaleDateString\|toLocaleString\|toLocaleTimeString" src/ | grep -v node_modules | grep -v '\.test\.'
```
Expected: ONLY `src/lib/format-date.ts` (the `auto` branch). No route/component matches remain.
```bash
npm run check
npm run knip
npm run test
```
Expected: all clean/green. (`knip` confirms no orphaned helper exports remain; the old per-file helpers were local, not exported, so the concern is mainly that `formatRelativeTime` is still imported somewhere — it is, by the hook.)

- [ ] **Step 11: Commit**

```bash
git add ui/frontend/src/
git commit -m "refactor: route remaining date displays through useDateFormat"
```

---

### Task 8: Build regen, docs, and final verification

**Files:**
- Modify: `ui/frontend/src/routeTree.gen.ts` only if a route file's structure changed (it did not — no routes added/removed, so this should be untouched; verify).

- [ ] **Step 1: Frontend full gate**

Run (from `ui/frontend/`):
```bash
npm run check && npm run knip && npm run test
```
Expected: all green.

- [ ] **Step 2: Backend full gate + deadcode**

Run (from repo root):
```bash
go test ./internal/api/... -run TestSettings -v
go build ./...
make deadcode
```
Expected: settings test passes; build clean; `make deadcode` reports no NEW entries attributable to this diff (the only Go symbol removed is none — `defaultDateFormat`/`validDateFormats` are all referenced).

- [ ] **Step 3: Acceptance-criteria spot check**

Confirm against the issue's acceptance criteria:
- `grep -rn "en-US" ui/frontend/src | grep -v node_modules` → no matches.
- `grep -rn "function formatDate\|function formatWhen\|function formatReleaseDate" ui/frontend/src | grep -v format-date` → no matches (all per-file helpers gone).
- Manual/logical check: with `dateFormat='auto'`, the `formatDate` `auto` branch uses `toLocaleDateString(undefined, …)` → browser locale; with an explicit pref the numeric order is fixed app-wide and persisted via the settings PATCH.

- [ ] **Step 4: Confirm routeTree is untouched**

Run: `git status --porcelain ui/frontend/src/routeTree.gen.ts`
Expected: no output (file unchanged — no routes were added or removed).

- [ ] **Step 5: Final commit (if anything pending) and open PR**

```bash
git status
# If clean, nothing to commit. Then push the branch and open the PR:
# Title: "feat: respect user locale for dates; add date-format preference"
# Body must include: "Closes #1155"
```

---

## Self-Review Notes

- **Spec coverage:** central util (Task 2) ✓; profile preference with Auto/ISO/DD-MM/MM-DD (Task 5, `DATE_FORMAT_OPTIONS`) ✓; migrate all ~13 sites incl. the 5 `en-US` ones (Tasks 6-7) ✓; backend `date_format` column + API + Settings type (Tasks 1, 3) ✓; fixed 24-hour time (Task 2 `formatDateTime`) ✓; `formatRelativeTime` keeps relative logic, only fallback routes through util (Task 4) ✓.
- **Acceptance criteria:** "no hardcoded locale literal" (Task 8 grep) ✓; "Auto honours browser locale" (Task 2 `auto` branch) ✓; "override changes app-wide + persists" (Tasks 1/3/4 + React Query cache update in `useUpdateSettings`) ✓; "all rendering through central util" (Task 8 grep) ✓.
- **Type consistency:** `DateFormatPref` defined once in `format-date.ts`, imported by `types/settings.ts`, `api/settings.ts`, `types/jobs.ts`, `use-date-format.ts`, `profile.tsx`. Hook returns `{ formatDate, formatDateTime, formatRelativeTime }` — names match every call site in Tasks 6-7. Backend `date_format` wire field ↔ `dateFormat` TS field mapped only in `api/settings.ts` `transform`.
- **Known edge to watch during execution:** `add.confirm.tsx` old helper returned `null` (callers may branch on it); Task 7 Step 2 explicitly says to inspect and preserve that empty-state behaviour rather than blindly substituting.
