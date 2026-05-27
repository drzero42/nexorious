# Design — Issue #613: make `hours_played` on `user_games` a calculated sum of platform hours

- **Issue:** #613 (`refactor: make hours_played on user_games a calculated sum of platform hours`)
- **Milestone:** 0.1.0
- **Date:** 2026-05-27
- **Branch:** `fix/613-calculated-hours-played`

## 1. Summary

`user_game_platforms.hours_played` is already the canonical, per-platform source of
truth for playtime. The data-model and sync halves of #613 are **already complete**:
there is no stored `user_games.hours_played` column, the `UserGame` model has no
`HoursPlayed` field, and sync workers write playtime only to the platform tables.

What is *not* done — and is the work in this spec — is that the API never exposes a
**calculated game-level `hours_played`**. As a result two things are broken today:

1. Every game-level "Xh" display (`game-card`, `game-list`, the game-detail headline)
   reads `game.hours_played`, which the API never sends, so it always renders **`0h`**
   (masked by a `|| 0` guard).
2. The UI offers a **"Hours Played" sort**, but the backend sort whitelist rejects it
   with **HTTP 400**.

This spec finishes the refactor: the API computes and returns `hours_played` as the sum
of a user-game's `user_game_platforms.hours_played` rows, exposes it as a working sort
field, and the frontend consumes it. It also includes an explicit **verification pass**
(audit + tests) proving the value is calculated, never stored, and that sync never
writes a game-level value.

## 2. Scope

**In scope**

- Verify (with tests) that no stored `user_games.hours_played` column exists and that
  sync writes playtime only to `user_game_platforms` / `external_game_platforms`.
- API: return a calculated game-level `hours_played` (sum of platform hours) on all
  user-game responses.
- API: support `sort_by=hours_played` on the user-games list.
- Frontend: consume the calculated value; remove the now-redundant game-level fallback
  in the edit form.
- Verify (with tests) that manual per-platform hours entry works for non-synced
  associations and is reflected in the calculated sum.

**Out of scope**

- The **write side is unchanged.** Manual hours entry stays available for non-synced
  platform associations; synced (Steam) associations remain locked in the UI
  (`disabled={isSteamSynced}`) and sync-owned (`hours_played = GREATEST(source, existing)`
  in `sync.go`). Whether users should be able to override hours on synced associations
  is explicitly deferred.
- The frontend also exposes `howlongtobeat_main` ("Time to Beat") and `rating_average`
  ("IGDB Rating") sorts that the backend's `allowedUserGameSortFields` whitelist does not
  include, so selecting them returns HTTP 400 — the same root cause as the `hours_played`
  sort bug (the sort menu drifted ahead of the whitelist). These are tracked separately
  in **#639** and are not fixed here. (Their fix is simpler than `hours_played`: both are
  `games`-table columns, so they only need whitelist entries + the existing games join,
  no aggregate subquery.)
- No database migration (there is no schema change).
- No new API route, so no `slumber.yaml` change.

## 3. Background — current state (evidence)

| Aspect | Issue's desired state | Current reality |
|---|---|---|
| Stored `user_games.hours_played` column | Remove / deprecate | Already absent — the single consolidated `20260503000001_initial.up.sql` defines `hours_played` only on `user_game_platforms` and `external_game_platforms`; the `UserGame` model has no `HoursPlayed` field |
| Canonical source = `user_game_platforms.hours_played` | Yes | Already the case |
| Sync (Stage 3) writes hours only to platform tables | Yes | `sync.go` writes `hours_played` only to `user_game_platforms` (line ~713) and `external_game_platforms` (line ~108); structurally cannot write a game-level value |
| User-entered playtime at platform level + UI | Yes, with UI changes | Edit form already has per-platform hours inputs and saves via `updatePlatformAssoc`; `HandleUpdatePlatform` / `HandleCreatePlatform` accept `hours_played` |
| Library `total_hours_played` stat = sum of platform hours | (implied) | Already `COALESCE(SUM(ugp.hours_played), 0)` in `HandleCollectionStats` |
| Export game-level total = sum of platform hours | (implied) | Already computed as `ugTotalHours` in `export.go` |
| **API returns calculated game-level `hours_played`** | **Yes** | **Missing** — `userGameWithPlatformsResponse` embeds `models.UserGame` (no such field) and computes nothing |

## 4. Verification / audit pass

Documented here and backed by tests (see §8):

- **No stored column.** A Go test queries `information_schema.columns` and asserts
  `user_games` has **no** column named `hours_played`. At compile time the `UserGame`
  model has no `HoursPlayed` field, so any code attempting to read/write one fails to
  build.
- **Sync writes only to platforms.** Confirmed by reading `sync.go`: playtime writes
  target `user_game_platforms` and `external_game_platforms` exclusively. A test mutates
  a platform's `hours_played`, re-reads the user-game via the API, and asserts the
  game-level value equals the new platform sum — proving the value is derived, not
  stored.
- **Manual entry is reflected.** A test sets `hours_played` on a non-synced platform via
  `PUT /api/user-games/:id/platforms/:platform_id` and asserts the calculated game-level
  `hours_played` includes it.

## 5. Backend — calculated response value (Approach 1)

The chosen approach computes the response value in Go from the already-eager-loaded
platforms — mirroring the proven `export.go` summation — and uses SQL only for sorting.

- Add a field to the response DTO:
  ```go
  // userGameWithPlatformsResponse
  HoursPlayed float64 `json:"hours_played"`
  ```
- In `toUserGameWithPlatformsResponse`, sum the platform hours (nil-safe):
  ```go
  var total float64
  for _, p := range ug.Platforms {
      if p.HoursPlayed != nil {
          total += *p.HoursPlayed
      }
  }
  resp.HoursPlayed = total
  ```
- Because **every** user-game endpoint (list, single GET, create, update, progress)
  serializes through `toUserGameWithPlatformsResponse`, all of them gain the field with
  no per-handler change.
- **Semantics:** non-nullable `float64`, `0` when there are no platform hours — matches
  the existing frontend type `hours_played: number` and the `|| 0` display guards. No
  extra database round-trip (platforms are already loaded for the response).

There is no JSON key collision: the embedded `models.UserGame` has no `hours_played`
field, and Go's encoding promotes the wrapper's field at the top level (the same way the
wrapper already overrides `platforms`).

## 6. Backend — sort by `hours_played`

The list handler (`HandleListUserGames`) runs a **two-phase** query: a paginated ID
query aliased `ug`, then a model fetch aliased `user_game` that re-applies the sort.
Game-table sorts (`title`, `release_date`) already work by adding the *same join* in both
phases and ordering by a join-stable alias (`g.title`). The hours sort mirrors that
pattern exactly, which sidesteps the `ug` vs `user_game` base-alias difference.

- Add to `allowedUserGameSortFields`:
  ```go
  "hours_played": "hp.total",
  ```
- Add a new set mirroring `sortFieldsRequiringGamesJoin`:
  ```go
  var sortFieldsRequiringHoursJoin = map[string]bool{
      "hours_played": true,
  }
  ```
- In **both** phases add the identical pre-aggregated join (one row per `user_game_id`,
  so it is safe under `DISTINCT` and cannot multiply rows):
  ```sql
  LEFT JOIN (
      SELECT user_game_id, COALESCE(SUM(hours_played), 0) AS total
      FROM user_game_platforms
      GROUP BY user_game_id
  ) hp ON hp.user_game_id = <ug | user_game>.id
  ```
  - Phase 1 (ID query, alias `ug`): register via
    `fb.AddJoin("hp", "LEFT JOIN (...) hp ON hp.user_game_id = ug.id")`.
  - Phase 2 (model fetch, alias `user_game`): add via
    `q.Join("LEFT JOIN (...) hp ON hp.user_game_id = user_game.id")`, gated on
    `sortFieldsRequiringHoursJoin[sortBy]`, exactly like the existing games-join branch.
- The `ORDER BY hp.total` expression is identical in both phases (alias `hp` is the
  join's own alias, independent of the base table alias), so the existing
  `sortCol + " " + sortOrder` machinery works unchanged.

`sort_by=hours_played` will then return 200 and order correctly; it no longer 400s.

## 7. Frontend

- **No type change.** `UserGame.hours_played: number` already exists and is now
  populated by the API. The `game-card`, `game-list`, and game-detail headline displays
  (`game.hours_played || 0`) become correct automatically.
- **Simplify the edit form.** In `game-edit-form.tsx`, the `totalHoursPlayed` memo
  currently reads `platformHours > 0 ? platformHours : game.hours_played`. Since the
  platform sum *is* `game.hours_played`, drop the redundant fallback and compute the
  total from the platform playtimes directly.
- **Sort.** The "Hours Played" option already exists in `games/index.tsx` /
  `game-filters.tsx`; it starts working once the backend accepts the field. No frontend
  change required beyond confirming it sends `sort_by=hours_played`.
- **Write side unchanged** (per §2): the per-platform hours input keeps its current
  behavior, including `disabled={isSteamSynced}` for synced associations.

## 8. Testing

**Go (API)**

- Game-level sum across multiple platforms — e.g. `10 + 25.5 → 35.5` — on both
  `GET /api/user-games` (list) and `GET /api/user-games/:id` (single).
- `hours_played == 0` when a user-game has no platforms, or all platform hours are NULL.
- `sort_by=hours_played` with `sort_order=asc` and `desc` orders user-games by their
  summed hours; assert it returns 200 (regression guard against the previous 400).
- Audit assertions from §4: `information_schema` has no `user_games.hours_played`
  column; mutating a platform's hours changes the calculated game-level value; manual
  entry on a non-synced platform is included in the sum.

**Frontend**

- Update MSW handlers/mocks (`test/mocks/handlers.ts`) so user-game fixtures include a
  game-level `hours_played` equal to the platform sum.
- Verify `game-card` / `game-list` / game-detail render the summed hours.

The mechanical gates (build, lint, typecheck, full suites at push) run via the existing
hooks; targeted tests above are run during development.

## 9. Release note

Although the issue title uses `refactor:`, the change repairs user-visible behavior
(game-level hours rendering as `0h`; the Hours Played sort returning 400). The PR should
be titled with **`fix:`** so release-please cuts a patch release. (Use `refactor:` only
if no release is desired.)
