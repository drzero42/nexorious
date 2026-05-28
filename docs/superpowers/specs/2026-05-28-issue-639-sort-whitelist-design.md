# Issue #639 — Fix "Time to Beat" and "IGDB Rating" sort options

## Problem

The user-games list sort menu offers two options that the backend whitelist does
not accept:

- `howlongtobeat_main` ("Time to Beat")
- `rating_average` ("IGDB Rating")

The frontend forwards the chosen sort verbatim
(`ui/frontend/src/api/games.ts:388`), so selecting either option hits the `!ok`
branch in `internal/api/user_games.go:182` and returns
`400 "invalid sort_by field"` instead of a sorted list. The dropdown is defined
in `ui/frontend/src/components/games/game-filters.tsx:44,48`; the whitelist that
drifted behind it lives in `internal/api/user_games.go:133-149`.

Root cause: the sort menu drifted ahead of the backend whitelist. The
sibling `hours_played` drift is being fixed separately (#613, merged as
4beb49e2).

## Fix

Both columns live on `games` (`rating_average NUMERIC(5,2)`,
`howlongtobeat_main NUMERIC(6,2)`, both nullable). They sort through the same
machinery as `title` / `release_date` — a `LEFT JOIN games AS g` applied to
both phases of the two-phase list query. The only nuance is NULL ordering (see
below).

### Backend changes (`internal/api/user_games.go`)

1. Extend `allowedUserGameSortFields`:

   ```go
   "howlongtobeat_main": "g.howlongtobeat_main",
   "rating_average":     "g.rating_average",
   ```

2. Extend `sortFieldsRequiringGamesJoin` so the existing
   `LEFT JOIN games AS g ON g.id = ug.game_id` is applied in both the ID-phase
   (around `:225`) and the model-phase (around `:313`):

   ```go
   "howlongtobeat_main": true,
   "rating_average":     true,
   ```

3. Introduce a per-field flag controlling NULL ordering — only the two new
   sorts opt in:

   ```go
   var sortFieldsNullsLast = map[string]bool{
       "howlongtobeat_main": true,
       "rating_average":     true,
   }
   ```

4. Compute the order expression once after `sortCol`/`sortOrder` are resolved,
   and use it in both phases of the list query (replacing the inline
   `sortCol + " " + sortOrder` at `:270` and `:318`):

   ```go
   orderExpr := sortCol + " " + sortOrder
   if sortFieldsNullsLast[sortBy] {
       orderExpr += " NULLS LAST"
   }
   ```

No other handler changes; no new joins; no schema change; no frontend change.

### NULL ordering

For games with no IGDB rating or no HowLongToBeat estimate, the corresponding
column is `NULL`. We want those games to sink to the bottom regardless of sort
direction — "no data" is not the same as "lowest" or "highest", and floating
them to the top under DESC (the PostgreSQL default) is the more confusing of
the two possible defaults.

So both new sorts emit `ORDER BY <col> <dir> NULLS LAST`.

The `release_date` sort is intentionally left untouched: it currently uses
PostgreSQL defaults (NULLs last ASC, NULLs first DESC), and changing that is
beyond the scope of this fix. The `sortFieldsNullsLast` flag is the clean
opt-in for any future sort that wants the same "no-data sinks" behavior.

### Why a flag rather than a global change

Applying `NULLS LAST` to every sort would alter the behavior of the existing
`release_date` sort (DESC currently surfaces NULL-date games first). That is a
user-visible behavior change beyond the scope of issue #639. The per-field flag
keeps the fix surgical.

## Tests

Add `TestListUserGamesSortByGameNumerics` in
`internal/api/user_games_test.go`, modeled on `TestListUserGamesSortByHours`
(`:1207`). Structure:

- Insert three user-games via `insertTestGame` + `insertTestUserGame`. Label
  them `ug-low`, `ug-high`, `ug-null` so the expected orderings are obvious in
  test failures.
- Set the column values on the underlying `games` rows with raw
  `UPDATE games SET rating_average = ..., howlongtobeat_main = ... WHERE id = ...`,
  mirroring how the hours test uses raw UPDATEs for
  `user_game_platforms.hours_played`. The fixture sets both columns on the
  same three games with parallel low/high/null mapping — `ug-low` gets the low
  value for both columns, `ug-high` the high value for both, `ug-null` is left
  with both columns NULL. This means both columns produce the same expected
  ordering, and the four sub-tests share one setup.
- Two sub-`t.Run` blocks per column (one for `rating_average`, one for
  `howlongtobeat_main`), each asserting:
  - `GET /api/user-games?sort_by=<field>&sort_order=desc` returns 200 and
    order `[high, low, null]` — NULL sinks, not floats.
  - `GET /api/user-games?sort_by=<field>&sort_order=asc` returns 200 and
    order `[low, high, null]` — NULL still sinks.

The 200 status assertion is the regression guard against the original 400.

The two columns share a near-identical fixture, so the test sets both columns
on the same three games and runs the four (column × direction) sub-tests
against that single fixture.

## Out of scope

- `hours_played` sort — fixed in #613.
- `release_date` NULL ordering — unchanged; would be a user-visible behavior
  change beyond this issue.
- Any frontend change — the dropdown already sends the correct values.
- Sort whitelist extensibility / config-driven schema — the existing two-map
  pattern (column expression + join flags) plus the new `nullsLast` flag is
  sufficient for the foreseeable list of sortable game columns.

## Related

- #613 — sibling fix for the `hours_played` sort (already merged).
