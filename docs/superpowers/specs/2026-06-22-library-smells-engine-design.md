# Library Smells — detection engine + REST API (backbone)

**Issue:** [#1144](https://github.com/drzero42/nexorious/issues/1144) — backbone of the Library Smells epic [#1143](https://github.com/drzero42/nexorious/issues/1143).
**Status:** design proposed, awaiting approval before an implementation plan.
**Lands first.** The Library Health web UI ([#1145](https://github.com/drzero42/nexorious/issues/1145)) and the `nexctl doctor` CLI + MCP tools ([#1146](https://github.com/drzero42/nexorious/issues/1146)) both consume this engine + API and are blocked on it.

## Summary

A "library smell" is a data-quality issue in a user's collection — accumulated from imports
and manual adds. This issue builds the **shared backbone**: a `internal/librarysmells` package
with a detector registry of 10 checks, a `smell_ignores` table for per-game dismissals, and
a REST API (summary → per-check listing → apply → ignore/restore/list-dismissed).

The concept, two-tier model, per-check fixability, and full rules table are settled in the epic
[#1143](https://github.com/drzero42/nexorious/issues/1143). This spec settles the **implementation
shape** on top of it: the detector contract, check identifiers, the SQL predicates against the
*actual* schema, the auto-fix routing through `internal/usergame`, and the API surface.

## Decisions taken into this spec

- **Check IDs are slugs**, not numbers. The epic's table numbers (1–11, with #11 sitting between
  #6 and #7) are display-order artifacts; the stable identifier used in the `:checkID` URL path
  and the `smell_ignores.check_id` column is a kebab-case slug (e.g. `storefront-less-platform`).
- **Two planning artifacts:** this design spec, then an implementation plan, then execute.

## Schema corrections vs. the epic (verified against the code)

The epic's rules table is mostly accurate, but three points were verified and corrected against
`internal/db/models/models.go` + the baseline migration:

1. **`acquired_date` is per-platform**, on `user_game_platforms` (nullable `date`), *not* on
   `user_games`. Check #6 therefore operates on each platform row, comparing its `acquired_date`
   against `now()` and against the game's `games.release_date`.
2. **No clear-wishlist mutation is exported** from `internal/usergame` — only the private
   `clearWishlistOnAcquire`. Check #4's auto-fix requires **adding** an exported bulk mutation
   there (see *Auto-fix routing*).
3. **Check #10's finished set ≠ `enum.FinishedPlayStatuses`.** #10 fires for
   `{completed, mastered, dominated}`; `enum.FinishedPlayStatuses` also includes `dropped`. #10
   uses its own explicit three-status set, not that slice.

Relevant column types (all confirmed):
`user_games.play_status *text` (nullable), `.is_wishlisted bool NOT NULL`, `.personal_rating *int`;
`user_game_platforms.platform *text`, `.storefront *text`, `.ownership_status *text`,
`.hours_played *numeric`, `.acquired_date *date`;
`games.howlongtobeat_main *numeric`, `.release_date *date`;
`platforms.default_storefront *text`; `platform_storefronts(platform, storefront)` composite PK.

## The 10 checks (slug, tier, fix, predicate)

> **Epic check #3 "storefront without a platform" is dropped.** It required `platform IS NULL`,
> but `user_game_platforms.platform` is `NOT NULL` in the schema (the baseline migration; the Bun
> model's `*string` is defensive typing only). A platform row therefore always has a platform, so
> #3 is unreachable by construction. The epic's rules table assumed the column was nullable. The
> remaining 10 checks stand. (Filed back on epic #1143.)

`hours(ug)` = `COALESCE(SUM(hours_played), 0)` over that game's `user_game_platforms` rows.
Tier `inconsistency` = epic Tier 1; `nudge` = epic Tier 2.

| Slug | Tier | Auto-fix | Predicate (per the rules table, against real columns) |
|---|---|---|---|
| `storefront-less-platform` | inconsistency | ❌ deep-link | a `user_game_platforms` row with `storefront IS NULL`. Context carries the platform row + the platform's `default_storefront` as a pre-filled suggestion. |
| `orphan-game` | inconsistency | ❌ deep-link | a `user_games` row with **zero** `user_game_platforms` rows **and `is_wishlisted = false`** (a wishlisted game legitimately has no platforms — that is the wishlist state, not an orphan). The mirror of `wishlisted-yet-owned`: a non-wishlisted game should have ≥1 platform row. |
| `wishlisted-yet-owned` | inconsistency | ✅ clear `is_wishlisted` | `is_wishlisted = true` **and** ≥1 `user_game_platforms` row exists (see *“owned” semantics*). |
| `missing-ownership-status` | inconsistency | ❌ deep-link | a platform row with `ownership_status IS NULL`. |
| `impossible-acquired-date` | inconsistency | ❌ deep-link | a platform row whose `acquired_date > now()::date`, **or** `acquired_date < games.release_date` (only when both dates are non-null). |
| `invalid-storefront-for-platform` | inconsistency | ❌ deep-link | a platform row with **both** `platform` and `storefront` non-null whose `(platform, storefront)` pair is absent from `platform_storefronts`. (Null platform/storefront is covered by the two checks above, not here — no double-flag.) |
| `beat-but-not-marked` | nudge | ✅ → `completed` | `games.howlongtobeat_main IS NOT NULL AND hours(ug) >= howlongtobeat_main AND play_status IN (not_started, in_progress)`. |
| `played-but-not-started` | nudge | ✅ → `in_progress` | `play_status = not_started AND hours(ug) >= 0.5` **and not** already matched by `beat-but-not-marked` (i.e. NOT(`howlongtobeat_main IS NOT NULL AND hours(ug) >= howlongtobeat_main`)). Enforces the #7→#8 precedence. |
| `in-progress-untouched` | nudge | ✅ → `not_started` | `play_status = in_progress AND hours(ug) = 0` (0 or NULL hours). |
| `unrated-after-finishing` | nudge | ❌ deep-link | `play_status IN (completed, mastered, dominated) AND personal_rating IS NULL`. |

Checks depending on `howlongtobeat_main` are **silent when it is NULL** — no false positives. Only
`beat-but-not-marked` (and the negative guard in `played-but-not-started`) reads HLTB.

### "Owned" semantics for `wishlisted-yet-owned`

The epic phrases #4 as "while an owned platform row exists". We define **owned = the game has ≥1
`user_game_platforms` row** (i.e. it is in the library at all), matching the existing
`clearWishlistOnAcquire` invariant (`EXISTS (SELECT 1 FROM user_game_platforms …)`). This keeps the
detector and the auto-fix consistent with the mutation that already enforces "a wishlisted game with
platforms gets its wishlist flag cleared". We deliberately do **not** require
`ownership_status = 'owned'` — a `no_longer_owned`/`borrowed` row still means the game is in the
library, and ownership-status gaps are their own check (#5).

## Package: `internal/librarysmells`

A detector registry. Each check is a value in a registry keyed by slug. A struct-with-funcs shape
(not an interface per detector) keeps all 11 declarations terse and in one place, which is what the
"detector registry" framing wants:

```go
type Tier string

const (
    TierInconsistency Tier = "inconsistency"
    TierNudge         Tier = "nudge"
)

// Check is one library-smell detector.
type Check struct {
    ID          string // stable slug, used in URLs and smell_ignores.check_id
    Title       string // human title, e.g. "Storefront-less platform"
    Description string // one-line explanation surfaced in the UI
    Tier        Tier
    AutoFixable bool   // true ⇒ Apply is non-nil

    // Detect returns the flagged items for userID, already excluding rows
    // dismissed via smell_ignores for this check. Read-only.
    Detect func(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error)

    // Apply performs the one-click fix for the given user_game IDs. Non-nil only
    // when AutoFixable. It re-validates each ID against Detect's predicate and
    // mutates only the still-flagged subset, routing through internal/usergame.
    // Returns (applied, skipped).
    Apply func(ctx context.Context, db *bun.DB, userID string, userGameIDs []string) (applied, skipped int, err error)
}

// FlaggedItem is one flagged game for one check, with check-specific context.
type FlaggedItem struct {
    UserGameID string  `json:"user_game_id"`
    GameID     int32   `json:"game_id"`
    Title      string  `json:"title"`            // game title for display + deep-link
    CoverArtURL *string `json:"cover_art_url,omitempty"`

    // Context (all omitempty; only the fields a given check needs are set):
    PlatformRowID       *string `json:"platform_row_id,omitempty"`       // offending user_game_platforms.id
    Platform            *string `json:"platform,omitempty"`
    Storefront          *string `json:"storefront,omitempty"`
    SuggestedStorefront *string `json:"suggested_storefront,omitempty"`  // #1: platform.default_storefront
    SuggestedStatus     *string `json:"suggested_status,omitempty"`      // #7/#8/#9
    Detail              *string `json:"detail,omitempty"`                // e.g. "acquired 2031-04-01 (future)"
}
```

- `Registry() []Check` returns the 10 checks in epic display order. `Lookup(slug) (Check, bool)`
  resolves the URL param. The HTTP layer 404s an unknown slug.
- Each detector's `Detect` is a single Bun query that **also** anti-joins `smell_ignores`
  (`NOT EXISTS (SELECT 1 FROM smell_ignores si WHERE si.user_id = ? AND si.user_game_id = ug.id AND si.check_id = ?)`).
  Platform-level checks return one `FlaggedItem` **per offending platform row** (so a game with two
  storefront-less rows flags twice), but the ignore is keyed by `(user_game, check)` — dismissing
  silences the whole game for that check (matches the migration's grain).
- On-demand evaluation. Solo user, small libraries, ~11 queries — no caching, no materialization.

## Migration: `smell_ignores`

Pair `internal/db/migrations/20260622000001_create_smell_ignores.{up,down}.sql` (confirm the running
number against the latest migration at implementation time — currently `20260621000001`).

| column | type | notes |
|---|---|---|
| `id` | TEXT PK | uuid, generated in the handler |
| `user_id` | TEXT NOT NULL → `users(id)` ON DELETE CASCADE | |
| `user_game_id` | TEXT NOT NULL → `user_games(id)` ON DELETE CASCADE | a deleted game drops its ignores |
| `check_id` | TEXT NOT NULL | the check slug; validated against the registry at the API layer (not an FK) |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |

- Unique `(user_id, user_game_id, check_id)` — one dismissal per game per check. Ignore-inserts use
  `ON CONFLICT DO NOTHING` (idempotent).
- A new `SmellIgnore` Bun model in `internal/db/models/`.

## API surface

Group `e.Group("/api/library/smells", auth.AuthMiddleware(db))`, user-scoped via
`auth.UserIDFromContext(c)`. Echo v5 route order: the static collection route is registered before
the `:checkID` param route; the `/apply`, `/ignore`, `/ignored` segments are children of `:checkID`
so there is no sibling static-vs-param collision.

| Method | Path | Behaviour |
|---|---|---|
| `GET` | `/api/library/smells` | **Summary.** Array of `{id, title, description, tier, auto_fixable, count}` for all 10 checks. `count` is the flagged count *after* ignores. (Runs every detector; counts only.) |
| `GET` | `/api/library/smells/:checkID` | **Per-check listing**, paginated. 404 if `checkID` is not a registry slug. Honors ignores. Response envelope `{items: FlaggedItem[], total, page, per_page, pages}` (mirrors `UserGameListResponse`; `page` 1-indexed, `per_page` 1–200 default 25). |
| `POST` | `/api/library/smells/:checkID/apply` | Body `{user_game_ids: [...]}`. **422** if the check is not auto-fixable. Re-validates against the predicate, applies the fix to the still-flagged subset via `internal/usergame`, returns `{applied, skipped}`. |
| `POST` | `/api/library/smells/:checkID/ignore` | Body `{user_game_ids: [...]}`. Inserts `smell_ignores` rows (`ON CONFLICT DO NOTHING`); each game must belong to the user (else skipped). Returns `{ignored}`. |
| `DELETE` | `/api/library/smells/:checkID/ignore` | Body `{user_game_ids: [...]}`. **Restore** — deletes the matching `smell_ignores` rows. Returns `{restored}`. |
| `GET` | `/api/library/smells/:checkID/ignored` | **List dismissed** items for one check, paginated: `{items: [{user_game_id, title, created_at}], total, page, per_page, pages}`. Driven purely by `smell_ignores` (an item appears here even if it would no longer flag). |

All bodies validate `user_game_ids` is a non-empty array; unknown JSON keys are tolerated per
existing handler style (no strict decode on these). The handler caps the id-list length defensively.

## Auto-fix routing (all through `internal/usergame`)

The auto-fix set is **`wishlisted-yet-owned`, `beat-but-not-marked`, `played-but-not-started`,
`in-progress-untouched`** — no hand-chained writes.

| Check | Routes to | Notes |
|---|---|---|
| `wishlisted-yet-owned` | **new** `usergame.ClearWishlist(ctx, db, userID, userGameIDs []string) (int, error)` | Adds an exported bulk mutation mirroring `clearWishlistOnAcquire`: `UPDATE user_games SET is_wishlisted=false, updated_at=now() WHERE id = ANY(?) AND user_id = ? AND is_wishlisted AND EXISTS (platform row)`, in its own `RunInTx`, returning the rows affected. The `EXISTS` guard makes it the bulk, user-scoped, exported counterpart of the private helper. |
| `beat-but-not-marked` | `usergame.SetPlayStatusBulk(…, PlayStatus: "completed")` | `removeFromPoolsIfFinished` fires (completed is finished) — desirable. |
| `played-but-not-started` | `usergame.SetPlayStatusBulk(…, PlayStatus: "in_progress")` | |
| `in-progress-untouched` | `usergame.SetPlayStatusBulk(…, PlayStatus: "not_started")` | `removeFromPoolsIfFinished` is a no-op; `promoteToInProgressIfPlayed` is *not* called by `SetPlayStatusBulk`, and the item only flags at 0 hours anyway, so it stays `not_started`. |

`Apply` re-validates first: it calls the check's `Detect`, intersects the flagged `user_game_id` set
with the requested ids, mutates only the intersection, and reports `applied` = mutated, `skipped` =
requested − flagged. This makes apply safe against a stale client and idempotent.

## Test plan

Per the repo's "test non-trivial / real-bug-catching logic" policy. All fixtures use **seeded**
platform/storefront names (`pc-windows`, `steam`, …) to satisfy the FK constraints.

- **Per detector (11):** seed a flagged row, a clean row, and an ignored row; assert it fires on the
  flagged row, not on the clean row, and respects the ignore. Use the shared `testDB` +
  `truncateAllTables`.
- **Precedence:** a `not_started` game past `howlongtobeat_main` flags `beat-but-not-marked`, **not**
  `played-but-not-started`.
- **`orphan-game` wishlist guard:** a wishlisted game with zero platform rows does **not** flag; a
  non-wishlisted game with zero platform rows does.
- **HLTB-NULL silence:** a heavily-played game with `howlongtobeat_main IS NULL` does not flag
  `beat-but-not-marked`.
- **#6 date logic:** future `acquired_date` flags; `acquired_date` before `release_date` flags;
  `acquired_date` after release and not future does not; NULL `release_date` suppresses the
  "before release" arm.
- **#11:** an invalid `(platform, storefront)` pair flags; a valid seeded pair does not; a NULL
  platform/storefront does not flag here (a NULL storefront is covered by #1; platform is never NULL).
- **Apply path (the 4 auto-fix checks):** assert the `internal/usergame` mutation is invoked and the
  invariants hold (e.g. `beat-but-not-marked` → `completed` removes the game from pools); assert
  apply skips an id that is no longer flagged (TOCTOU); assert `played-but-not-started` does not
  fire on a game already matched by `beat-but-not-marked`.
- **`usergame.ClearWishlist`:** clears only wishlisted rows that have a platform row and belong to
  the user; idempotent; returns the count.
- **Ignore/restore:** ignore removes an item from the listing and the summary count; restore brings
  it back; the dismissed listing shows ignored items even when they no longer flag; ignore is
  per-(game,check) and does not silence other checks.
- **API:** 404 for an unknown `checkID`; 422 applying a non-auto-fixable check; pagination envelope
  shape; auth required.

## Out of scope (this issue)

The Library Health web page ([#1145](https://github.com/drzero42/nexorious/issues/1145)) and the
`nexctl doctor` CLI + MCP tools ([#1146](https://github.com/drzero42/nexorious/issues/1146)). No
caching/materialization. No global "fix everything" — apply is always scoped to one check.
