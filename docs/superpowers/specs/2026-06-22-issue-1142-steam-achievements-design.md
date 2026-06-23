# Steam achievement progress ("X of Y") on user-games

**Issue:** #1142
**Date:** 2026-06-22
**Status:** Approved (design)

## Summary

Surface per-storefront achievement completion as a compact "X of Y" count on each
user-game. v1 targets **Steam only** and stores **counts only** (unlocked + total,
not individual achievement names). A badge appears on the game card; a per-storefront
line appears on the game detail view. Refresh is folded into the existing Steam
library sync — no new worker or schedule.

## Motivation

Achievement completion is a glanceable signal of how far a player has gotten in a
game, complementary to play status and hours. Steam exposes it through an official
API we already authenticate against, so it is low-risk to add.

## Scope (v1)

- **Steam only.** GOG/PSN/Epic deferred (see Out of scope).
- **Counts only** — unlocked + total. No per-achievement names/descriptions/dates.
- **Display only** — no list filtering/sorting by completion.
- **Refresh** folded into the existing Steam library sync.

## Key design decisions

These were settled during brainstorming and depart from or sharpen the issue text:

1. **Two columns, both nullable, on _both_ platform tables.** Achievement counts
   follow the exact two-hop path that `hours_played` already takes:
   `external_game_platforms` (catalog) → `user_game_platforms` (library) via
   `usergame.Acquire`. The issue's migration section named only
   `user_game_platforms`; the fuller two-table carry is required because the sync
   worker reads counts back out of `external_game_platforms` when building the
   `usergame.PlatformInput` list. We add `achievements_unlocked` and
   `achievements_total` to both tables.

2. **No `achievements_synced_at` column (YAGNI).** The issue proposed it, but its
   only functional use would be skip-if-recent, which we are not doing in v1 (see
   #4). It can be added later if a freshness/skip feature is ever built.

3. **`success:false` from Steam → leave both counts `NULL`.** Steam's
   `GetPlayerAchievements` returns `success:false` for *both* "profile is private"
   and "game has no achievements," distinguished only by a brittle, localized error
   string. Because the display outcome is identical in both cases (no badge), we
   collapse both to `NULL` rather than parse the error text to synthesize a
   `total = 0`. This deliberately simplifies the issue's stated NULL-vs-0
   distinction. Net effect: `NULL` means "no badge" for any reason (not fetched,
   private, or genuinely no achievements); a non-NULL `total` only ever comes from a
   `success:true` response.

4. **API call volume bounded by the `playtime_forever > 0` gate only.** No
   skip-if-recent guard. Every played Steam game gets one `GetPlayerAchievements`
   call per sync. Unplayed games are skipped (0% anyway).

5. **Card badge: highest progress wins.** When multiple platform rows carry
   achievement data, the card shows the badge from the row with the highest
   `unlocked / total` ratio. In v1 only Steam rows carry data, so this effectively
   shows Steam; the rule is chosen to be forward-compatible with future storefronts.
   The badge is hidden when no row has `total > 0`.

## Architecture

### Data flow (mirrors `hours_played`)

```
GetOwnedGames ─┐
               ├─► ExternalGameEntry ─► upsertPlatforms ─► external_game_platforms
GetPlayer-     ┘   (.AchievementsUnlocked,                 (.achievements_unlocked,
Achievements        .AchievementsTotal)                     .achievements_total)
                                                                     │
                                                                     ▼
                                          read back ─► usergame.PlatformInput
                                                       ─► usergame.Acquire
                                                       ─► user_game_platforms
                                                          (.achievements_unlocked,
                                                           .achievements_total)
                                                                     │
                                                                     ▼
                                          userGamePlatformResponse (JSON) ─► UI
```

### 1. Database migration

New pair `internal/db/migrations/20260622000002_add_achievement_counts.up.sql` /
`.down.sql` (next running number after the `20260622000001_create_smell_ignores`
migration):

```sql
-- up
ALTER TABLE external_game_platforms
  ADD COLUMN achievements_unlocked integer,
  ADD COLUMN achievements_total    integer;

ALTER TABLE user_game_platforms
  ADD COLUMN achievements_unlocked integer,
  ADD COLUMN achievements_total    integer;
```

```sql
-- down
ALTER TABLE user_game_platforms
  DROP COLUMN achievements_unlocked,
  DROP COLUMN achievements_total;

ALTER TABLE external_game_platforms
  DROP COLUMN achievements_unlocked,
  DROP COLUMN achievements_total;
```

Both columns are nullable (no `NOT NULL`, no default) on both tables. Note this
diverges from `external_game_platforms.hours_played`, which is `NOT NULL DEFAULT 0`;
nullability is meaningful for achievements (NULL = not fetched), so we keep them
nullable on both tables for a single consistent representation.

### 2. Model structs (`internal/db/models/models.go`)

- `UserGamePlatform`: add `AchievementsUnlocked *int \`bun:"achievements_unlocked" json:"achievements_unlocked"\`` and `AchievementsTotal *int \`bun:"achievements_total" json:"achievements_total"\``.
- `ExternalGamePlatform`: add the same two `*int` fields (no `json` consumer, but kept consistent).

### 3. Steam client (`internal/services/steam/client.go`)

Add `GetPlayerAchievements(ctx, apiKey, steamID string, appID int) (unlocked, total int, ok bool, err error)`:

- Endpoint: `{ownedGamesBase}/ISteamUserStats/GetPlayerAchievements/v0001/?appid={appID}&key={apiKey}&steamid={steamID}&format=json` (web API, same base as `GetOwnedGames`).
- Response shape:
  ```json
  { "playerstats": {
      "success": true,
      "achievements": [ { "apiname": "...", "achieved": 1 }, ... ] } }
  ```
- `success:false` → return `ok=false`, no error (caller leaves NULL). A genuine
  transport/HTTP error → return `err` (caller logs, leaves NULL, sync continues).
- `success:true` → `total = len(achievements)`, `unlocked = count(achieved == 1)`,
  `ok = true`.

Add a dedicated `achievementsLimiter *rate.Limiter` to `Client` (the existing
`limiter` guards only the store-API `GetAppDetailsPlatforms`; this is a separate
per-game web-API call). Mirror the existing 200 ms cadence; `Wait(ctx)` before each
call.

### 4. Steam adapter (`internal/services/steam/adapter.go`)

In the per-game build loop (where `ExternalGameEntry` is assembled, ~line 105):

- When `og.PlaytimeForever > 0` (equivalently `PlaytimeHours > 0`), call
  `GetPlayerAchievements`. On `ok`, set `entry.AchievementsUnlocked` /
  `entry.AchievementsTotal` to the fetched values; otherwise leave them nil.
- On a transport error, log at WARN and continue (never fail the sync). Reuse the
  adapter's existing 429 backoff posture for the new call.

### 5. `ExternalGameEntry` (`internal/services/storefrontadapter/storefrontadapter.go`)

Add `AchievementsUnlocked *int` and `AchievementsTotal *int` (nil when the source
provides nothing — every non-Steam adapter leaves them nil).

### 6. Sync worker (`internal/worker/tasks/sync.go`)

- `upsertPlatforms` signature gains the two counts; it writes them to
  `external_game_platforms`, attached to **platform index 0 only** (same wrinkle as
  `hours_played` — Steam achievements are account-wide per app, so index 0 is the
  natural carrier). `ON CONFLICT` update sets the columns from `EXCLUDED`.
- The read-back loop (~line 728) copies `egp.AchievementsUnlocked` /
  `egp.AchievementsTotal` into `usergame.PlatformInput`.

### 7. `usergame` mutation boundary

- `usergame.PlatformInput` (`internal/usergame/types.go`): add the two `*int` fields.
- `usergame.Acquire` INSERT/UPDATE (`acquire.go`), `AddPlatform` (`acquire.go`), and
  `UpdatePlatform` (`platform.go`) persist the two columns to `user_game_platforms`.
  All achievement writes route through this boundary — no hand-chained SQL elsewhere.

### 8. REST API (`internal/api/user_games.go`)

- `userGamePlatformResponse`: add `AchievementsUnlocked *int \`json:"achievements_unlocked"\`` and `AchievementsTotal *int \`json:"achievements_total"\``.
- `toUserGamePlatformResponse`: copy the two fields from the model.

### 9. Frontend

- `ui/frontend/src/types/game.ts` — `UserGamePlatform`: add
  `achievements_unlocked?: number | null` and `achievements_total?: number | null`.
- `ui/frontend/src/api/games.ts` — `UserGamePlatformApiResponse` interface +
  `transformUserGamePlatform`: carry the two fields through.
- `ui/frontend/src/components/games/game-card.tsx` — compute the
  highest-`unlocked/total`-ratio platform row; render one compact badge
  (e.g. `🏆 42/78`) near the hours/platform icons. Hide when no row has `total > 0`.
- `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` — in the existing
  "Platforms & Ownership" rows, add a per-storefront achievement line for rows where
  `total > 0`.

## Error handling

| Condition | Result |
|---|---|
| Private Steam profile | `success:false` → counts `NULL`, no badge, sync succeeds |
| Game has zero achievements | `success:false` → counts `NULL`, no badge, sync succeeds |
| Player unlocked 0 of N | `success:true` → `unlocked=0, total=N`, badge shows `0/N` |
| Steam API transport error | logged WARN, counts `NULL`, sync continues |
| `playtime_forever == 0` | call skipped, counts `NULL` |
| Manually-added (non-synced) platform row | counts `NULL` (never written) |

## Testing

- **Count derivation**: parse a `GetPlayerAchievements` `success:true` payload →
  `unlocked = sum(achieved)`, `total = len`. Include an all-locked (`unlocked=0`)
  case.
- **`success:false` → `NULL`**: both the private-profile and no-achievements
  responses leave counts nil (no error raised).
- **Playtime gate**: `playtime_forever == 0` makes no achievements call.
- **Plumbing**: counts survive the `external_game_platforms → user_game_platforms`
  hop through `usergame.Acquire` (DB-backed test, mirroring the existing
  `TestAcquire_*HoursPlayed*` tests).
- **Frontend**: highest-ratio platform selection; badge hidden when all rows are
  `NULL`/`0`; badge renders `unlocked/total` otherwise.

## Out of scope / future

- **GOG** — feasible via the undocumented
  `gameplay.gog.com/clients/{id}/users/{id}/achievements` endpoint; unofficial, can
  break.
- **PSN** — feasible via trophy-summary endpoints, but the current Go lib likely
  doesn't expose trophies and trophy titles key off `npCommunicationId`, a different
  ID space than the synced `titleId` (non-trivial matching).
- **Epic** — effectively infeasible: EOS achievements need per-game deployment
  credentials Legendary doesn't expose.
- Full achievement detail (names/descriptions/unlock dates) and per-game list views.
- List filtering/sorting by completion %.
- `achievements_synced_at` + skip-if-recent freshness logic.
