# Wishlist for games the user wants but doesn't own (#867)

**Status:** design
**Issue:** #867

## Problem

There is no way to record "I want this game." Ownership status
(`owned`/`borrowed`/`rented`/`subscription`/`no_longer_owned`) is structurally
**per-platform** on `user_game_platforms` and only describes games the user
already has. A wishlist is a different concept: a **game-level, platform-agnostic
"want"** — a natural surface for deal-site links and a future input to a
"what to play/buy next" feature.

This feature also requires a small, durable place to store a per-user **deal
region** (which psprices storefront region to deep-link into). The project
previously had a generic `users.preferences` TEXT blob and **deliberately
removed it** (migration `20260604000002_drop_users_preferences`, issues
#797/#798) because it was untyped and never consumed. We reintroduce per-user
preferences the *right* way rather than resurrecting that blob.

## Scope

In scope:

1. **`user_games` wishlist state** — an explicit `is_wishlisted` boolean.
2. **Wishlist API** — create as wishlist, list/exclude wishlist, an
   auto-maintained invariant (shared platform-insert helper that auto-promotes),
   and a dedicated **move-to-library** endpoint.
3. **`/wishlist` page** + nav entry, reusing the existing grid/card components.
4. **Add-game destination toggle** (Library | Wishlist).
5. **Game detail page** wishlist section: deal links + "Move to library".
6. **Deal links** — client-side deep links to isthereanydeal.com (PC) and
   psprices.com (console), search-by-title only, no scraping.
7. **`user_settings` foundation** — a new typed per-user settings table, an
   `/api/settings` resource, and a profile-page "Preferences" section, seeded
   with one setting: `deal_region`.

Out of scope (per issue):

- The upcoming **"what to play next"** feature. We only ensure wishlist data
  exists and is queryable; no coupling is designed here.
- **Live deal/price data**, price-drop alerts, per-platform wishlist
  granularity.
- Any preference beyond `deal_region`. The settings table is built to extend
  (one typed column per future preference) but we add exactly one now.

## Design

### 1. Data model

**Migration `20260608000003_add_wishlist_to_user_games`:**

```sql
ALTER TABLE user_games
  ADD COLUMN is_wishlisted BOOLEAN NOT NULL DEFAULT false;

-- Wishlist queries are "is_wishlisted = true"; a partial index keeps that scan
-- cheap and small (most rows are library entries).
CREATE INDEX idx_user_games_wishlisted
  ON user_games (user_id)
  WHERE is_wishlisted;
```

`.down.sql` drops the index then the column.

Model (`internal/db/models/models.go`, `UserGame`): add
`IsWishlisted bool \`bun:"is_wishlisted,notnull"\``.

**Invariant.** A wishlist entry is a `user_games` row with `is_wishlisted=true`
and **zero** `user_game_platforms`. A library entry has `is_wishlisted=false`.
A game is one or the other, never both. `play_status` on a wishlist entry stays
at its `not_started` default and is **not surfaced** in wishlist UI. Notes,
personal rating, loved, and tags are all usable on a wishlist entry and "carry
over" to the library automatically — move-to-library mutates the same row, so
there is no copy step.

The invariant is maintained by a **shared insertion helper** (below), not by a
DB constraint or trigger. The rule spans two tables (`user_games` +
`user_game_platforms`), which a Postgres `CHECK` cannot express. A trigger was
considered and rejected: this codebase deliberately keeps behaviour in Go
(no triggers exist today; even auth uses raw Go SQL), the app is the sole writer
of its schema, and a trigger's hidden cross-table mutation would be invisible at
the call site and would leave the in-memory Bun model stale after insert. The
helper keeps enforcement discoverable in Go and — as a side benefit — creates
the platform-insert chokepoint the code currently lacks.

**Auto-promote on platform-attach.** The invariant is *maintained*, not just
guarded: whenever a `user_game_platform` row is inserted for a wishlisted entry,
`is_wishlisted` is cleared in the same transaction — the game is automatically
promoted to the library. This is the correct behaviour for **sync** (a
wishlisted game that turns up in a storefront was clearly acquired), and it
applies uniformly to **every** insert path (sync, import, Darkadia, bulk-add,
manual single-attach, create). There is therefore **no** `422` on attach; the
only contradictory request still rejected is *create* with `is_wishlisted=true`
*and* a non-empty `platforms` array.

### 2. Wishlist API (`internal/api/user_games.go`)

**Create — `POST /api/user-games`.** Add optional `is_wishlisted bool` to
`createUserGameRequest`.

- `is_wishlisted=true`: require `platforms` empty; otherwise `422`. Create the
  `user_games` row with `is_wishlisted=true` and no platform rows.
- `is_wishlisted` false/omitted: current behaviour, unchanged.

**Shared platform-insert helper (new chokepoint).** There is currently no
shared function for inserting `user_game_platforms`; 6 independent sites build
the insert inline (4 Bun, 2 raw SQL). Introduce one helper that all platform
inserts flow through. Its contract: within a transaction, insert the platform
row(s) for a given `user_game_id` (preserving the existing
`ON CONFLICT (user_game_id, platform, storefront) DO NOTHING` semantics) **and**
clear `is_wishlisted` on that parent `user_games` row in the same transaction.
The helper lives in a package importable by both `internal/api` and
`internal/worker/tasks` (e.g. a new `internal/usergame` package) to avoid an
import cycle; exact signature/location is an implementation-plan detail.

Refactor all 6 insert sites onto it:

1. `HandleCreateUserGame` (`user_games.go:447`) — batch insert for a new entry.
2. `HandleCreatePlatform` (`user_games.go:1017`) — single manual attach.
3. `HandleBulkAddPlatforms` (`user_games.go:832`, raw SQL) — calls the helper
   per affected `user_game_id` inside its existing loop.
4. Sync `UserGameWorker` (`worker/tasks/sync.go:754`, raw SQL) — the
   insert-new-platforms branch (the update-existing branch already implies the
   game had platforms, so it cannot be wishlisted).
5. Import `ImportItemWorker` (`worker/tasks/import_item.go:275`).
6. Darkadia `DarkadiaFinalizeWorker` (`worker/tasks/darkadia.go:223`).

A future 7th insert site is expected to use the helper; this is enforced by
convention + review, and the single helper makes omission far less likely than
6 scattered clears would.

**Move to library — `POST /api/user-games/:id/move-to-library`** (new).
Request body carries one or more platforms with ownership/hours/acquired (same
shape the add/create flow already uses). In a single transaction:

1. Load the entry; `422` if it is not the caller's, or `is_wishlisted=false`,
   or the platforms array is empty.
2. Insert the `user_game_platforms` rows **via the shared helper** (which clears
   `is_wishlisted` as part of the same transaction), including the play-status
   auto-promotion when hours are provided.

The endpoint exists for the wishlist→library UX (collect ownership in the
wishlist context); mechanically it is the helper plus the not-wishlisted/empty
guards. Register the static `move-to-library` sub-route before any parameterised
`:something` route on the same path segment (Echo v5 route-order gotcha).

**List — `GET /api/user-games`.** Add an `ApplyWishlist` filter in
`internal/filter/`. The handler **defaults to excluding wishlisted entries**
(`is_wishlisted=false`) and accepts a query param (e.g. `wishlist=true`) that
flips it to return **only** wishlisted entries. The filter is applied to both
the count query and the row query so pagination is correct. The default-exclude
behaviour means the existing library list automatically drops wishlist entries
with no frontend change.

**Dashboard / stats.** Audit dashboard count queries and exclude wishlisted
entries from "library" totals (a wishlisted game is not an owned game). Adjust
the affected queries; if a "wishlist count" stat is trivial to surface it may be
added, but that is optional and not required by this spec.

### 3. `user_settings` foundation

App-level preferences do **not** live on the `users` table — `internal/auth`
keeps the user/auth layer isolated and accesses it via raw SQL, not Bun models.
Preferences are a feature concern and get their own Bun-managed table.

**Migration `20260608000004_create_user_settings`:**

```sql
CREATE TABLE user_settings (
  user_id    TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  deal_region TEXT NOT NULL DEFAULT 'us',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

(`user_id` type matches the existing `users.id` column type — confirm and match
during implementation.) The row is 1:1 with `users`, created **lazily**: a read
for a user with no row returns defaults; a write upserts.

`deal_region` is a typed column, not a generic key/value bag — this is the
deliberate fix for what killed the old blob: a typed column forces the
reader + writer + UI to be wired together, gives a DB default, and is queryable.
Validation against the psprices region allowlist (below) is done in the API layer
rather than as a 76-value DB `CHECK`, which would be brittle as psprices adds
regions.

**API — new `internal/api/settings.go`:**

- `GET /api/settings` → `{ "deal_region": "us" }` (defaults if no row).
- `PATCH /api/settings` → accepts `{ "deal_region": "<code>" }`; validate the
  code against the allowlist (`422` otherwise); upsert; return the updated
  settings.

Both are auth-gated and operate on the current user. They are a standalone app
resource — **not** folded into `/api/auth/me`, keeping auth isolated.

**Region allowlist.** The set of psprices region codes (ISO 3166-1 alpha-2),
scraped from psprices' region switcher, lives in one place on the backend (a Go
`map[string]bool` / slice) and is the validation source of truth. The current
list (76 codes):

```
ae ar at au be bg bh bo br ca ch cl cn co cr cy cz de dk ec es fi fr gb ge gr
gt hk hn hr hu id ie il in iq is it jp kr kw kz lb lu mt mx my ni nl no nz om
pa pe ph pk pl pt py qa ro ru sa se sg si sk sv th tr tw ua us uy vn za
```

The frontend region `<Select>` is populated from this list (served via a small
constant or a `GET` of allowed regions — implementation choice; a shared static
list is fine).

### 4. Frontend

**Nav + route.** Add a "Wishlist" entry in
`ui/frontend/src/components/navigation/nav-items.tsx` and a new
`_authenticated/wishlist` route. The page reuses `GameGrid`/`GameList` and the
existing list hook with `wishlist=true`. Cards render without play-status badges
or platform chips (wishlist entries have neither). The page links each card to
the game detail page.

**Add-game destination toggle.** On the add flow (`games/add.confirm.tsx` and
its form), add a **Library | Wishlist** choice. Choosing **Wishlist**:

- hides/suppresses the platform/storefront/ownership inputs, and
- posts `is_wishlisted=true` with no platforms.

Choosing **Library** is the current behaviour.

**Game detail page (`games/$id.index.tsx`).** When `is_wishlisted`:

- show a **wishlist section** with the two deal links and a **"Move to library"**
  action; and
- do **not** offer platform/storefront attachment.

"Move to library" opens the existing `platform-detail-fields` UI (platform +
storefront + ownership/hours/acquired) and, on submit, calls
`POST /api/user-games/:id/move-to-library`. On success the entry becomes a normal
library entry and the page renders the standard library view.

**Profile "Preferences" section.** Add a section to
`_authenticated/profile.tsx` with a `deal_region` `<Select>`, backed by a
`useSettings` TanStack Query hook (`GET /api/settings`) and a mutation
(`PATCH /api/settings`). Region options come from the allowlist.

**Types.** Extend the user-game type with `isWishlisted`; add a `Settings`
type (`{ dealRegion: string }`). Follow the existing API-transform conventions.

### 5. Deal links (client-side)

Pure URL construction from the game title; both links always shown; the user
picks the ecosystem at click time. No network calls, no scraping.

- **PC (IsThereAnyDeal):**
  `https://isthereanydeal.com/search/?q=${encodeURIComponent(title)}`
  (verified: the server 302-redirects this to the title-filtered results page;
  the redirect target varies by query. ITAD pricing region is tied to the user's
  own ITAD account, so no region param is needed here.)
- **Console (PSprices):**
  `https://psprices.com/region-${dealRegion}/games/?q=${encodeURIComponent(title)}`
  (verified: title-filtered listing; a nonsense query yields "Nothing found".
  `dealRegion` comes from `user_settings`, default `us`.)

A small pure helper (e.g. `deal-links.ts`) builds the two URLs from
`(title, dealRegion)` and is unit-tested.

## Testing (TDD)

**Backend (`internal/api`, `internal/filter`):**

- Create wishlist entry: succeeds with empty platforms; `422` when platforms are
  provided.
- Shared helper auto-promote: inserting a platform for a wishlisted entry clears
  `is_wishlisted` (test the helper directly), and the conflict path is a no-op
  on an already-library entry.
- Auto-promote via sync: a wishlisted entry that the sync `UserGameWorker`
  attaches a platform to ends up with `is_wishlisted=false` and leaves the
  wishlist (regression test for the #867 edge case). Spot-check at least one of
  the import/Darkadia paths too.
- Move-to-library: happy path attaches platforms + clears `is_wishlisted`;
  `422` when entry isn't wishlisted; `422` when platforms empty; `404`/forbidden
  for another user's entry.
- List: default excludes wishlisted; `wishlist=true` returns only wishlisted;
  counts/pagination respect the filter.
- Settings: `GET` returns defaults when no row; `PATCH` upserts and round-trips;
  invalid `deal_region` → `422`.

**Frontend:**

- `deal-links.ts`: builds correct ITAD + psprices URLs, encodes titles, honours
  `dealRegion`.
- Add destination toggle: choosing Wishlist suppresses platform inputs and posts
  `is_wishlisted=true` with no platforms.
- Detail wishlist section renders deal links + move-to-library for a wishlisted
  entry and not for a library entry.
- Settings/preferences section: loads current region, saves a change.

## Acceptance

- A user can add a game **to the wishlist** from the add page; it appears on
  `/wishlist` and is absent from the main library.
- A wishlist entry has no platforms and shows ITAD + psprices deal links that
  deep-link a title search; the psprices link uses the user's configured region.
- "Move to library" attaches platform(s)/ownership and clears the wishlist
  state in one step; notes/rating/loved/tags are preserved; the game leaves
  `/wishlist` and appears in the library.
- A wishlisted game that later appears in a storefront sync is automatically
  promoted to the library (its `is_wishlisted` is cleared and it leaves
  `/wishlist`) — the same auto-promote holds for import/Darkadia and any manual
  platform attach.
- The API rejects creating a wishlist entry that also carries platforms.
- A user can set their deal region in profile preferences; it persists across
  sessions and is validated against the psprices region allowlist.
