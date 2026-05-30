# Issue #641 — Half-hour-granular playtime

**Date:** 2026-05-28
**Issue:** [#641](https://github.com/drzero42/nexorious/issues/641) — *feat: half-hour-granular playtime — capture Steam/PSN sub-hour precision and clean up hours display*
**Status:** Approved — ready for implementation plan
**Milestone:** 0.1.0

## Summary

Two coordinated changes to recorded playtime:

1. **Display** — a single shared formatter buckets recorded `hours_played` to half-hours
   below 10h and to whole hours from 10h up, eliminating float artifacts like
   `30.299999999999997h`.
2. **Capture** — Steam and PSN already carry sub-hour resolution that we currently throw
   away at ingestion. Promote the adapter's `PlaytimeHours int` to `float64` and stop
   truncating. GOG and Epic remain `0.0` (no source data).

The storage column `user_game_platforms.hours_played` is already `NUMERIC(10,2)`, so
fractional hours fit with no migration. Storage stays raw; the half-hour bucket is a
*display rule*, not a data rule.

## Problem

### Display

Recorded playtime renders raw float values: `{game.hours_played || 0}h`. Because
`game.hours_played` is now (post-#640) a `float64` sum of `NUMERIC(10,2)` per-platform
values, the natural float-summation artifacts surface in the UI — `30.299999999999997h` is
the canonical example. The averages in the dashboard are conventionally fractional
(`12.3h/game`) and are not the target of this fix.

### Capture

The four adapters write into the shared `ExternalGameEntry.PlaytimeHours int` field
(`internal/services/storefrontadapter/storefrontadapter.go:12`). The Steam and PSN paths
both have more precision than they pass through:

| Source | Raw shape | Current handling | Sub-hour data dropped? |
|---|---|---|---|
| Steam | `playtime_forever` in **minutes** (`steam/client.go:138`) | `g.PlaytimeForever / 60` — integer division (`steam/client.go:151`) | Yes — 90 min stored as `1`, not `1.5` |
| PSN | ISO 8601 duration `PTxHxMxS` (`psn/client.go:156`) | `parseDurationHours` returns the H component, M/S truncated (`psn/duration.go:14`) | Yes — minutes dropped |
| GOG | No playtime field (`gog/library.go:13`) | Always `0` | No |
| Epic | No playtime via Legendary | Always `0` (`epic/adapter.go:74`) | No |

So Steam and PSN already have the precision to make half-hour buckets meaningful; GOG and
Epic have nothing to capture.

## Approach

### Part 1 — Display formatter (frontend)

Add a single helper next to `formatTtb` in `ui/frontend/src/lib/game-utils.ts`:

```ts
export function formatHoursPlayed(hours: number | null | undefined): string {
  const h = hours ?? 0;
  const rounded = h < 10 ? Math.round(h * 2) / 2 : Math.round(h);
  return `${rounded}h`;
}
```

Rule: `h < 10` → nearest 0.5 (so `7.4 → 7.5`, `9.8 → 10`); `h ≥ 10` → nearest integer.
`null`/`undefined` → `"0h"`.

Apply at every recorded-playtime display site:

| File:line | Current code | New code |
|---|---|---|
| `components/games/game-card.tsx:149` | `{game.hours_played \|\| 0}h` | `{formatHoursPlayed(game.hours_played)}` |
| `components/games/game-list.tsx:180` | `{game.hours_played \|\| 0}h` | `{formatHoursPlayed(game.hours_played)}` |
| `routes/_authenticated/games/$id.index.tsx:395` | `{game.hours_played \|\| 0}h` | `{formatHoursPlayed(game.hours_played)}` |
| `routes/_authenticated/games/$id.index.tsx:410` | `{p.hours_played}h` | `{formatHoursPlayed(p.hours_played)}` |
| `components/games/game-edit-form.tsx:385` | `{totalHoursPlayed} hours total` | `{formatHoursPlayed(totalHoursPlayed)} total` |
| `components/dashboard/progress-statistics.tsx:109` | `{totalHoursPlayed.toLocaleString()}` | `{formatHoursPlayed(totalHoursPlayed)}` |
| `components/dashboard/progress-statistics.tsx:180` | `{totalHoursPlayed.toLocaleString()}` | `{formatHoursPlayed(totalHoursPlayed)}` |

Two notes on the dashboard sites:

- The bucketed value is always a small set of half-step numbers, so dropping
  `toLocaleString()` loses nothing (no thousand separators on `42.5` etc.). If we want
  separators for very large totals, the formatter can compose `Number(rounded).toLocaleString()`
  inside — we'll add this if it visibly hurts a real total in review; otherwise leave it.
- The "Time Investment" card's `totalHoursPlayed > 0` guard (`progress-statistics.tsx:171`)
  reads the raw number, so it keeps current behaviour.

The averages keep their `.toFixed(1)` calls — these are not recorded-playtime displays
(see [Out of scope](#out-of-scope)).

### Part 2 — Sub-hour capture (backend + manual entry)

**Adapter contract.** Promote the shared field type:

```go
// internal/services/storefrontadapter/storefrontadapter.go
type ExternalGameEntry struct {
    // ...
    PlaytimeHours float64 // 0 when the storefront does not provide playtime
    // ...
}
```

The worker's `upsertPlatforms` (`internal/worker/tasks/sync.go:195`) takes the value as
`float64` and writes it into the `NUMERIC(10,2)` column unchanged — bun will round the
literal to two decimals at insert time, so the canonical stored value for a Steam 90-min
game is `1.5` exactly, and for a PSN `PT340H46M13S` game is `340.77`.

**Steam** (`internal/services/steam/client.go`):

- `OwnedGame.PlaytimeHours` field (`:56`) becomes `float64`.
- The conversion at `:151` becomes `float64(g.PlaytimeForever) / 60.0`.
- The Steam `adapter.go:107` forwards the value unchanged.

**PSN** (`internal/services/psn/`):

- Rename `parseDurationHours` → `parseDurationFractionalHours` (`duration.go`) and return
  `float64 = H + M/60`. **Minutes only — seconds are dropped.** Rationale: the half-hour
  display bucket makes seconds invisible (PT340H46M13S → 340.77 → "341h"), the test
  fixtures stay readable, and we avoid pretending we have second-level fidelity end-to-end.
- `psn/client.go:217` switches to the new function.
- `psn/client.go:316` (the disc-game branch with `PlaytimeHours: 0`) and `:338` (the
  `PlaytimeHours int` field on the internal merge struct) become `0.0` / `float64`.
- The PSN `adapter.go:29` forwards the value unchanged.

**GOG** (`internal/services/gog/`) and **Epic** (`internal/services/epic/`):

- `gog/library.go:18` `PlaytimeHours int` → `float64`; literal `0` at `:129` → `0.0`.
- `gog/adapter.go:62` forwards unchanged.
- `epic/adapter.go:74` literal `0` → `0.0`.
- No behavioural change; the type ripple is the only edit.

**Manual edit form** (`ui/frontend/src/components/games/game-edit-form.tsx`):

- `<Input type="number" min="0" .../>` at `:482` gains `step="0.5"`.
- `parseInt(e.target.value) || 0` at `:489` becomes `parseFloat(e.target.value) || 0`.
- The `platformPlaytimes` state type stays `Record<string, number>`; the displayed
  fallback `p.hours_played` at `:485` is already typed `number`. No other change in the
  edit form's submit path — it already sends the value through the API as a number.

The `step="0.5"` is a browser hint, not a hard validator; users typing `1.23` will
have it accepted and stored as `1.23`, then displayed via the same formatter (so
"1h" or "1.5h" depending on the bucket). This is intentional — the formatter is the
single source of truth for presentation.

### Why store full precision instead of bucketing at ingest

- API consumers and the edit form see the underlying value, not a lossy approximation.
- The averages — which read the same column — keep meaningful precision.
- If the bucketing rule ever changes (quarter-hours, decimal-hours), it's a one-line
  formatter edit with no data migration.

## Testing

### Frontend

New file `ui/frontend/src/lib/game-utils.test.ts` (the module currently has no test).
Cover `formatHoursPlayed` only — `formatTtb` and `formatIgdbRating` are unrelated.

| Input | Expected | Why |
|---|---|---|
| `0` | `"0h"` | Zero baseline |
| `null` | `"0h"` | Null coalesces to 0 |
| `undefined` | `"0h"` | Same |
| `1.2` | `"1h"` | Rounds down to nearest 0.5 |
| `1.3` | `"1.5h"` | Rounds up to nearest 0.5 |
| `7.4` | `"7.5h"` | Issue's canonical case |
| `9.8` | `"10h"` | Boundary — bucket crosses 10 |
| `10` | `"10h"` | Boundary — exactly 10 → integer rule |
| `30.299999999999997` | `"30h"` | Issue's canonical float artifact |
| `134` | `"134h"` | Large integral value |

### Backend

- `internal/services/steam/client_test.go` — existing `120 min → 2` updates to
  `2.0`; add a `90 min → 1.5` case and a `45 min → 0.75` case to pin the float path.
- `internal/services/psn/duration_test.go` — if absent, create. Cover
  `PT2H30M → 2.5`, `PT0H → 0`, `PT1H59M → 1 + 59.0/60.0`, `PT340H46M13S → 340 + 46.0/60.0`,
  malformed input → 0. Compare with a small delta (`math.Abs(got-want) < 1e-9`) to keep
  expectations readable; do not use literal `340.7666...` in the source.
- `internal/services/psn/library_test.go` — the existing `expected PlaytimeHours=340`
  assertion at `:73` updates to `340 + 46.0/60.0` with the same delta comparison; the
  test fixture (`PT340H46M13S`) does not change.
- `internal/services/gog/library_test.go:179-180` — `!= 0` still type-checks; update the
  `%d` verb on `:180` to `%v` (or `%g`) to match the new `float64` type.
- `internal/services/epic/adapter_test.go:186-187` — already uses `%v`; no edit needed.
- `internal/services/psn/library_test.go:73-74,220-221,349-350` — three `%d` verbs to
  switch to `%v`; the `:220` assertion (`!= 0`) is unchanged; `:73` and `:349` are addressed
  below in the PSN test entry.
- `internal/worker/tasks/sync_test.go` — all `PlaytimeHours: <int>` literals stay valid
  Go float64 conversions, so most cases compile unchanged. Add **one** new case to the
  existing upsert table covering `PlaytimeHours: 1.5` to verify the fractional value
  reaches `user_game_platforms.hours_played` intact. This is the only spot where
  fractional behaviour can regress silently.

No new test scaffolding. The shared test DB pattern (`testDB` in `TestMain`) is reused.

## Out of scope

- **The averages.** `routes/_authenticated/dashboard.tsx:138` (avg hours/game),
  `components/dashboard/progress-statistics.tsx:186` (avg hours/game), and
  `:192` (avg completion time) keep their `.toFixed(1)` calls. Averages are inherently
  fractional; the half-hour bucket is wrong for them.
- **Storefront sort.** PR #640 added the game-level `hours_played` sum and the
  corresponding sort key. This issue displays that sum correctly via the formatter but
  does not touch the sort SQL or the aggregation pipeline.
- **Issue [#639](https://github.com/drzero42/nexorious/issues/639).** Broken
  `howlongtobeat_main` / `rating_average` sorts are a separate defect with a different
  root cause; conflating them with playtime precision would muddy the PR.
- **DB schema.** `NUMERIC(10,2)` already accommodates the new fractional values; no
  migration is added or modified.
- **API contract.** `hours_played` is already serialised as a JSON number; switching the
  Go type from `int` to `float64` does not change the wire format.

## Files touched

### Backend (Go)

Files that need actual source edits:

| File | Change |
|---|---|
| `internal/services/storefrontadapter/storefrontadapter.go` | `PlaytimeHours int` → `float64`; update field comment. |
| `internal/services/steam/client.go` | `OwnedGame.PlaytimeHours` → `float64`; conversion at `:151` becomes `float64(g.PlaytimeForever) / 60.0`. |
| `internal/services/psn/duration.go` | Replace `parseDurationHours` (int) with `parseDurationFractionalHours` returning `float64 = H + M/60`. |
| `internal/services/psn/client.go` | `:217` calls the new function; `:338` internal `PlaytimeHours int` → `float64`. |
| `internal/services/gog/library.go` | `PlaytimeHours int` → `float64`; update field comment. |
| `internal/worker/tasks/sync.go` | `upsertPlatforms` parameter `playtimeHours int` → `float64`. |

Files where the type ripple is invisible — no source edit needed, listed for the
reviewer's mental model:

- `internal/services/steam/adapter.go:107`, `internal/services/psn/adapter.go:29`,
  `internal/services/gog/adapter.go:62` — each does `PlaytimeHours: <src>.PlaytimeHours,`,
  which type-checks unchanged once both fields are `float64`.
- `internal/services/epic/adapter.go:74` (`PlaytimeHours: 0,`) and
  `internal/services/psn/client.go:316` (`PlaytimeHours: 0,`) — `0` is an untyped Go
  constant and remains valid for `float64`.

### Backend tests

| File | Change |
|---|---|
| `internal/services/steam/client_test.go` | Update `120 min` assertion to `2.0`; add `90 min → 1.5`, `45 min → 0.75`. |
| `internal/services/psn/duration_test.go` | Create or extend with fractional cases. |
| `internal/services/psn/library_test.go` | `:73` expectation becomes `340 + 46.0/60.0` with delta. |
| `internal/services/epic/adapter_test.go` | Type ripple on assertion. |
| `internal/services/gog/library_test.go` | Type ripple on assertion. |
| `internal/worker/tasks/sync_test.go` | Add one fractional-playtime upsert case. |

### Frontend

| File | Change |
|---|---|
| `ui/frontend/src/lib/game-utils.ts` | Add `formatHoursPlayed`. |
| `ui/frontend/src/lib/game-utils.test.ts` | New file — covers `formatHoursPlayed`. |
| `ui/frontend/src/components/games/game-card.tsx` | Use `formatHoursPlayed`. |
| `ui/frontend/src/components/games/game-list.tsx` | Use `formatHoursPlayed`. |
| `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` | Use `formatHoursPlayed` at the headline and per-platform sites. |
| `ui/frontend/src/components/games/game-edit-form.tsx` | Total uses `formatHoursPlayed`; per-platform input uses `step="0.5"` + `parseFloat`. |
| `ui/frontend/src/components/dashboard/progress-statistics.tsx` | Two `toLocaleString()` calls replaced with `formatHoursPlayed`. |

## Acceptance criteria (from issue)

- [ ] Recorded-playtime displays show half-hour buckets below 10h and whole hours at/above 10h; no float artifacts ever shown.
- [ ] Averages keep one-decimal precision (`dashboard.tsx`, `progress-statistics.tsx` averages).
- [ ] Steam playtime preserves sub-hour precision (90 min → `1.5h`); Steam adapter tests updated.
- [ ] PSN playtime preserves minutes (`PT2H30M` → `2.5h`); seconds intentionally dropped.
- [ ] GOG/Epic unchanged (still 0).
- [ ] Manual edit form accepts half-hour values.
- [ ] `formatHoursPlayed` has unit tests covering the documented boundary cases.
