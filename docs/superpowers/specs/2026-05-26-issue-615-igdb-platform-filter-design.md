# Filter IGDB search by platform during sync matching

**Issue:** [#615](https://github.com/drzero42/nexorious/issues/615)
**Date:** 2026-05-26
**Branch:** `issue-615-igdb-platform-filter`

## Problem

When the sync worker finds a game on a storefront and queries IGDB for candidates, it sends only the title. IGDB returns matches across every platform — so a Steam library entry for *Tomb Raider* gets results that include PlayStation, Xbox, and Saturn releases. The auto-matcher then post-ranks by fuzzy title score with no platform awareness; ties and near-ties (Steam reissue vs PS1 original sharing the same name) push items into `pending_review`. There, the user picks from the same 10 unfiltered candidates.

We already know the storefront for every `external_game`, and we already know which platforms each storefront serves (`platform_storefronts`), and we already record which platforms the storefront reports for each specific game (`external_game_platforms`). That signal is currently discarded between sync and IGDB.

IGDB's Apicalypse query language supports `where platforms = (6,14,3)` — but only by numeric IGDB platform IDs, not by names. Our `platforms` table has had a nullable `igdb_platform_id` column since the original schema PR (#1) reserved for exactly this purpose, but no migration ever populated it. So the column exists, the data layer is wired through to the model, and nothing reads it.

## Goal

Reduce the IGDB candidate set for sync matching to only games released on the platforms the source storefront actually reported for that item. Apply this in both the auto-match path (worker) and the manual-match re-search path (pending_review UI). Leave the generic "add a game from IGDB" search ([igdb-search.tsx](../../ui/frontend/src/components/games/igdb-search.tsx)) untouched — that flow has no storefront context.

## Change

### Data — populate `platforms.igdb_platform_id`

Edit the existing baseline migration [internal/db/migrations/20260503000001_initial.up.sql](../../internal/db/migrations/20260503000001_initial.up.sql) to seed the column inline in the `INSERT INTO platforms` block (this repo follows a single-baseline-migration pattern — the recent commit `34fa6890` collapsed migrations and `9b4b8355` realigned the baseline, confirming direct edits to the initial migration are the project's convention):

```sql
INSERT INTO platforms (name, display_name, icon, igdb_platform_id, default_storefront) VALUES
    ('pc-windows',        'PC (Windows)',               'pc-windows-icon-light.svg',        6,   'steam'),
    ('playstation-5',     'PlayStation 5',              'playstation-5-icon-light.svg',     167, 'playstation-store'),
    ('playstation-4',     'PlayStation 4',              'playstation-4-icon-light.svg',     48,  'playstation-store'),
    ('playstation-3',     'PlayStation 3',              'playstation-3-icon-light.svg',     9,   'playstation-store'),
    ('playstation-vita',  'PlayStation Vita',           NULL,                               46,  'playstation-store'),
    ('playstation-psp',   'PlayStation Portable (PSP)', NULL,                               38,  'playstation-store'),
    ('xbox-series',       'Xbox Series X/S',            'xbox-series-icon-light.svg',       169, 'microsoft-store'),
    ('xbox-one',          'Xbox One',                   'xbox-one-icon-light.svg',          49,  'microsoft-store'),
    ('xbox-360',          'Xbox 360',                   'xbox-360-icon-light.svg',          12,  'microsoft-store'),
    ('nintendo-switch',   'Nintendo Switch',            'nintendo-switch-icon-light.svg',   130, 'nintendo-eshop'),
    ('nintendo-wii',      'Nintendo Wii',               'nintendo-wii-icon-light.svg',      5,   'nintendo-eshop'),
    ('ios',               'iOS',                        'ios-icon-light.svg',               39,  'apple-app-store'),
    ('android',           'Android',                    'android-icon-light.svg',           34,  'google-play-store'),
    ('playstation-2',     'PlayStation 2',              'playstation-2-icon-light.svg',     8,   'physical'),
    ('playstation',       'PlayStation',                'playstation-icon-light.svg',       7,   'physical'),
    ('nintendo-wii-u',    'Nintendo Wii U',             'nintendo-wii-u-icon-light.svg',    41,  'nintendo-eshop'),
    ('pc-linux',          'PC (Linux)',                 'pc-linux-icon-light.svg',          3,   'steam'),
    ('mac',               'Mac',                        'mac-icon-light.svg',               14,  'steam'),
    ('nintendo-switch-2', 'Nintendo Switch 2',          'nintendo-switch-2-icon-light.svg', 508, 'nintendo-eshop');
```

The numeric IDs are stable constants from IGDB's `/platforms` catalog and are public knowledge in their API docs. Any future platform row whose IGDB ID is unknown or absent simply leaves the column NULL, which silently opts that platform out of filtering (the resolver skips NULLs).

The `.down.sql` already drops the table — no edit needed.

### Resolution helper — `IGDBPlatformIDsForExternalGame`

New file [internal/services/platformresolution/igdb_ids.go](../../internal/services/platformresolution/igdb_ids.go):

```go
// IGDBPlatformIDsForExternalGame returns the IGDB numeric platform IDs for the
// platforms attached to this external_game. Platforms whose igdb_platform_id is
// NULL are silently skipped. Returns an empty slice (not an error) if the
// external_game has no platforms or no resolvable IDs. Returns an error only on
// DB failure.
func IGDBPlatformIDsForExternalGame(ctx context.Context, db *bun.DB, externalGameID string) ([]int, error)
```

One SQL query, with `DISTINCT` so two platform rows pointing at the same IGDB ID (shouldn't happen today, but cheap defence) collapse:

```sql
SELECT DISTINCT p.igdb_platform_id
FROM external_game_platforms egp
JOIN platforms p ON p.name = egp.platform
WHERE egp.external_game_id = $1 AND p.igdb_platform_id IS NOT NULL
```

This is the only new package surface. The package already exists for slug-translation logic; the new function fits its remit.

### IGDB client — `SearchGames` gains a `platformIDs []int` parameter

Signature change in [internal/services/igdb/igdb.go](../../internal/services/igdb/igdb.go):

```go
// Before
func (c *Client) SearchGames(ctx context.Context, query string, limit int) ([]GameMetadata, error)

// After
func (c *Client) SearchGames(ctx context.Context, query string, limit int, platformIDs []int) ([]GameMetadata, error)
```

`GetGameByID` and `FetchFullMetadata` are unchanged — they take an IGDB ID directly and don't need disambiguation.

Internal behaviour additions:

- New helper `buildPlatformsClause(platformIDs []int) (whereSuffix, searchTail string)` returns `("", "")` for empty input. For non-empty input it returns:
  - `whereSuffix` for queries that already have `where name = "..."` — `" & platforms = (6,14,3)"` joined via `&` (Apicalypse AND).
  - `searchTail` for `search "..."` queries (which take an optional separate `where`) — `" where platforms = (6,14,3);"`.
- Every IGDB query body built inside `SearchGames` uses the appropriate variant. Today that's the initial concurrent exact+fuzzy pair and an exact+fuzzy pair per keyword-expansion (two grammar shapes — `search "...";` and `where name = "...";` — applied at every call site).
- **Empty-result fallback**: after the existing rank+truncate, if `len(results) == 0 && len(platformIDs) > 0`, the function recurses once with `platformIDs = nil` and returns those results. This handles IGDB's incomplete platform tagging — legitimate Steam games occasionally lack a Windows platform tag in IGDB. One extra round-trip in the empty-set case only; the common case (filter returns matches) pays nothing extra.
- A `slog.Debug` line when the fallback fires so we can observe how often IGDB's platform data is the bottleneck.

### Sync worker — pass the filter through

[internal/worker/tasks/sync.go:402](../../internal/worker/tasks/sync.go#L402) before the existing `SearchGames` call:

```go
platformIDs, perErr := platformresolution.IGDBPlatformIDsForExternalGame(ctx, w.DB, eg.ID)
if perErr != nil {
    slog.Debug("igdb_match: platform resolution failed, falling back to unfiltered",
        "item_id", p.JobItemID, "external_game_id", eg.ID, "err", perErr)
    platformIDs = nil
}
candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10, platformIDs)
```

Resolution errors degrade gracefully to unfiltered search — the IGDB call still happens, the user still gets candidates. Never blocks the match pipeline. The error case is rare (DB connectivity) and is the same end state as "no platforms recorded for this EG", so log-and-continue is appropriate.

### HTTP handler — accept optional `external_game_id`

`IGDBSearchRequest` in [internal/api/games.go](../../internal/api/games.go):

```go
type IGDBSearchRequest struct {
    Query          string  `json:"query"`
    Limit          int     `json:"limit"`
    ExternalGameID *string `json:"external_game_id,omitempty"`  // new
}
```

`HandleSearchIGDB` verifies ownership, then resolves and forwards:

```go
var platformIDs []int
if req.ExternalGameID != nil && *req.ExternalGameID != "" {
    // Ownership check — the EG must belong to the authenticated user.
    var exists bool
    if err := h.db.NewRaw(
        `SELECT EXISTS(SELECT 1 FROM external_games WHERE id = ? AND user_id = ?)`,
        *req.ExternalGameID, userID,
    ).Scan(c.Request().Context(), &exists); err != nil {
        return c.JSON(http.StatusInternalServerError, map[string]string{"error": "ownership check failed"})
    }
    if !exists {
        return c.JSON(http.StatusForbidden, map[string]string{"error": "external_game not found or not owned by user"})
    }

    if ids, perErr := platformresolution.IGDBPlatformIDsForExternalGame(c.Request().Context(), h.db, *req.ExternalGameID); perErr == nil {
        platformIDs = ids
    } else {
        slog.Debug("HandleSearchIGDB: platform resolution failed, falling back to unfiltered",
            "external_game_id", *req.ExternalGameID, "err", perErr)
    }
}
results, err := h.igdb.SearchGames(c.Request().Context(), req.Query, req.Limit, platformIDs)
```

`userID` comes from the auth middleware (same source as other authenticated handlers in this file). `GamesHandler` already holds `*bun.DB` via `h.db` (used by `HandleGetGame` and `HandleImportFromIGDB`), so this adds no new dependency to the handler struct or its constructor.

The 403 path also covers "EG with that ID doesn't exist at all" — both cases collapse to the same response. We don't distinguish between them, which is the standard pattern for not leaking existence to non-owners.

### JobItem API — expose `external_game_id`

The frontend manual-match UI needs to know which external_game it's matching for, but `JobItem` API responses currently don't surface that field. Add it:

- [internal/api/jobs.go](../../internal/api/jobs.go) — `JobItem` response DTO gains `ExternalGameID *string \`json:"external_game_id"\``. The underlying model already has it ([internal/db/models/jobs.go:111](../../internal/db/models/jobs.go#L111)).
- [ui/frontend/src/api/jobs.ts](../../ui/frontend/src/api/jobs.ts) — `JobItemApiResponse` gains `external_game_id: string | null`; `transformJobItem` maps it to `externalGameId`.
- [ui/frontend/src/types/jobs.ts](../../ui/frontend/src/types/jobs.ts) — `JobItem` interface gains `externalGameId: string | null`.

### Frontend client — pass `externalGameId` through

`searchIGDB` in [ui/frontend/src/api/games.ts:506](../../ui/frontend/src/api/games.ts#L506):

```ts
export async function searchIGDB(
  query: string,
  limit?: number,
  externalGameId?: string,
): Promise<IGDBGameCandidate[]> {
  const body: { query: string; limit: number; external_game_id?: string } = {
    query,
    limit: limit ?? 10,
  };
  if (externalGameId) body.external_game_id = externalGameId;
  const response = await api.post<IGDBSearchApiResponse>('/games/search/igdb', body);
  return response.games.map(transformIGDBGameCandidate);
}
```

`useSearchIGDB` hook in [ui/frontend/src/hooks/use-games.ts](../../ui/frontend/src/hooks/use-games.ts) — accept an `externalGameId` option, include it in the query key so cached results don't bleed across external games sharing a title:

```ts
useSearchIGDB(query: string, options?: { limit?: number; externalGameId?: string })
// queryKey: ['games', 'searchIgdb', query, options?.externalGameId ?? null]
```

Call sites:

- [job-items-details.tsx:128](../../ui/frontend/src/components/jobs/job-items-details.tsx#L128) — pass `{ externalGameId: item.externalGameId ?? undefined }`.
- [igdb-search.tsx:140](../../ui/frontend/src/components/games/igdb-search.tsx#L140) — no change. Generic add-game UI stays unfiltered.

### All `SearchGames` call sites — three updates

Because Go has no optional parameters, every existing call to `SearchGames` must pass a fourth argument:

| Call site | Argument |
|---|---|
| [internal/worker/tasks/sync.go:402](../../internal/worker/tasks/sync.go#L402) | `platformIDs` (from resolver) |
| [internal/api/games.go:204](../../internal/api/games.go#L204) | `platformIDs` (from resolver, may be empty) |
| Test mocks and helpers (any direct call) | `nil` |

`nil` is byte-equivalent to today's behaviour inside `SearchGames` — `buildPlatformsClause` returns `("", "")`, the request bodies are unchanged.

## Why these choices

### Why filter per-game, not per-storefront

Two storefronts can sell the same game on different platform subsets — a Steam title might be Windows-only while another in the same library is Windows+Mac+Linux. Per-game filtering (using `external_game_platforms` for the specific item) gives a tighter IGDB filter than a flat storefront-union. The cost is one row in `external_game_platforms` already populated by the sync adapter, which is free.

### Why fall back to unfiltered when results are empty

IGDB's platform tagging is incomplete: there are real Steam games whose IGDB entry lacks a PC platform tag. A hard filter would silently zero out matches and route those items to `pending_review` with no suggestions, surfacing IGDB-data gaps as user friction. The fallback pays one extra round-trip in the empty case to preserve recall. The debug log makes it observable.

### Why seed `igdb_platform_id` directly in the baseline migration

The column has existed since #1 explicitly for this purpose (the original comment in the schema PR read `-- nullable; IGDB's numeric platform ID`). This repo's pattern is single-baseline-migration consolidation; the recent `34fa6890` and `9b4b8355` commits both edit the initial migration. A separate migration would fragment the baseline for ~19 hardcoded numeric constants. Direct seeding keeps the source of truth co-located with the platform rows themselves and makes admin hotfixes a one-line `UPDATE platforms SET igdb_platform_id = X WHERE name = 'slug'` in production.

### Why keep the IGDB client domain-free

The IGDB client takes `platformIDs []int` and nothing else. It does not import `bun.DB`, the `models` package, or `platformresolution`. Resolution lives one layer up, called by each caller before it invokes `SearchGames`. This preserves the existing layering: `igdb` is a thin HTTP wrapper, `platformresolution` is the domain-translation helper, and the callers compose them.

### Why the parameter is required (not variadic or options-struct)

Variadic `...int` would force callers to spread an existing slice (`SearchGames(ctx, q, lim, ids...)`), which obscures intent at the most common call site. An options struct is overkill for one knob. A required slice parameter with `nil` for unfiltered is the smallest delta — three call sites change by adding `, platformIDs` or `, nil`.

## Tests

| Layer | Test |
|---|---|
| Resolution helper | `igdb_ids_test.go` in `internal/services/platformresolution/` — returns IDs for an EG with two platforms; empty slice for EG with no platforms; empty slice for non-existent EG ID; NULL `igdb_platform_id` rows are skipped; duplicate IGDB IDs across platforms collapse via DISTINCT. Uses the shared package-level `testDB` (per [CLAUDE.md testing policy](../../CLAUDE.md)). |
| IGDB client | `igdb_test.go` — `SearchGames` with non-empty `platformIDs` puts `where platforms = (6,14,3)` (or the AND-joined variant) into every IGDB request body, asserted via `httptest` server captures; zero filtered results triggers exactly one unfiltered retry whose body has no platforms clause; empty `platformIDs` produces a body identical to today's behaviour (regression guard against accidental clause injection). |
| HTTP handler | `games_test.go` — `POST /api/games/search/igdb` with a self-owned `external_game_id` calls the IGDB mock with the expected IDs; without `external_game_id`, IDs are empty; with another user's EG returns 403; with a non-existent EG ID returns 403 (existence and ownership collapse to the same response). |
| Sync worker | `sync_test.go` — extend an existing IGDB-match test to assert the mock IGDB client received non-empty `platformIDs` for an EG with platforms. Existing tests must continue to pass with the new signature (mostly mechanical updates). |
| Frontend | `use-games.test.tsx` — `useSearchIGDB` forwards `externalGameId` to the API client and includes it in the query key; existing tests without the option still pass. `jobs.test.ts` — `transformJobItem` maps `external_game_id` to `externalGameId`. |

No coverage gate; tests are scoped to behaviours that have multiple meaningful paths (resolver edge cases, IGDB query-body construction, fallback trigger) or are security-adjacent (the `external_game_id` field — even though it's a user-owned ID, we should ensure the handler doesn't trust it cross-user; see Open Questions).

## Out of scope

- Filtering the generic "add a game from IGDB" search (`igdb-search.tsx`) — no storefront context exists there.
- Adding a platform picker UI to either search component.
- Score-weighting (soft filter) — the chosen design uses IGDB-side filtering with empty-result fallback, not post-rank weighting.
- Runtime discovery of IGDB platform IDs from IGDB's `/platforms` endpoint — overkill for ~19 stable constants.
- Tightening fuzzy-match scoring or thresholds — the goal is to shrink the candidate set, not change how candidates are ranked.

## Risk

- **Wrong IGDB platform ID seeded.** Mitigation: the IDs are stable public constants and easily verifiable by hitting IGDB's `/platforms` endpoint; an admin can hotfix a single row with a one-line UPDATE; the resolver skips NULLs so a single bad row degrades to that platform's items going unfiltered (same as today).
- **IGDB platform-tagging gaps cause many unnecessary fallback retries.** Mitigation: the debug log shows how often the fallback fires; if it's dominant for a storefront, we can later widen the filter (e.g. always include Linux+Windows for Steam regardless of per-game tagging) without changing the architecture.
- **Apicalypse syntax mistake** (e.g. wrong combinator in the AND-join). Mitigation: the IGDB client unit test asserts the exact query body shape for each query template.

### Why 403 on cross-user `external_game_id`

`HandleSearchIGDB` is authenticated, so the request body's `external_game_id` is user input that can name any row. Without the check, one user could pass another user's EG ID and learn something about that EG's platform set (the cross-user IGDB results would be filtered to platforms the other user's EG is tagged with). Small leak, but unnecessary. A strict 403 surfaces frontend bugs loudly rather than masking them, and collapsing "not found" and "not owned" into the same response avoids leaking existence to non-owners. One extra `SELECT EXISTS` per filtered search; negligible next to the IGDB round-trip.
