# Humble Bundle sync source — design

**Issue:** #766
**Date:** 2026-06-04
**Status:** Approved

## Summary

Add **Humble Bundle** as a sync source alongside Steam / PSN / Epic / GOG. It imports
**only DRM-free games you download directly from Humble** — explicitly **not** ebooks,
comics, audio, video, and **not** games for which you only receive a third-party (Steam)
key. The feature reuses the existing `storefrontadapter.Adapter` pattern and the whole
3-stage sync pipeline (dispatch → IGDB-match → user_game write).

Humble Bundle already exists in the app as a **display-only storefront** (seeded
`humble-bundle` row, logos, collection slug). This work promotes it to a real **sync
source**: new adapter + auth + thin API/UI wiring, plus the platform↔storefront
associations needed for both sync and manual tagging.

## Research — validated against a live library

The design was validated against a real account (138 orders; 25 fetched and analysed).
Findings that shaped the design — including corrections to the original issue's research:

- **Core filtering rule holds.** A subproduct is a game iff it has a download whose
  `platform ∈ {windows, mac, linux, android}` with a non-empty `download_struct`
  containing a `url.web`. Ebooks/audio/video/etc. are dropped by absence of those
  platforms.
- **Whitelist beats blacklist.** The live data contained `video` and `asmjs` platforms
  the original research never listed (and `comedy` is plausible but unseen). A whitelist
  of game platforms handles all current and future non-game platforms by exclusion.
- **Steam-key-only games are correctly excluded.** They typically have **no game
  subproduct at all** — only a `*_freegame_info` stub with empty `downloads` plus the key
  in `tpkd_dict` (e.g. Civ III Complete, ABZU, Batman: Arkham Origins). Never reading
  `tpkd_dict` drops them.
- **`machine_name` is always present** on game subproducts (0 missing across the sample),
  so it is a safe `ExternalID`.
- **🔴 Launcher binaries masquerade as games.** The `uplayclient` subproduct is a
  downloadable `UplayInstaller.exe` (`platform: windows`, real `download_struct`) titled
  *"Uplay Client (will download latest version)"*. It passes the naive filter and would
  import as a fake game. It appeared in 5 orders. A scan of the **full 138-order library**
  confirmed `uplayclient` is the **only** such launcher present. **Resolution: a
  single-entry machine_name blocklist** (see Filtering rule).
- **🟡 PC and Android editions are separate subproducts** with different `machine_name`s
  but the same `human_name` (e.g. `aquaria` win/mac/linux and `aquaria_android` android).
  The adapter yields both faithfully; the pipeline collapses them (see Dedup behavior).
- **Hash field is `md5`, not `sha1`** (cosmetic — unused).
- **Batch order endpoint is unreliable.** `GET /api/v1/orders?gamekeys=…` returned empty
  in testing; the per-order `GET /api/v1/order/{gamekey}?all_tpkds=true` works reliably
  and is what we use.

### Endpoints

- `GET /api/v1/user/order` → `[{ "gamekey": "…" }, …]` (used for auth verify + gamekey list)
- `GET /api/v1/order/{gamekey}?all_tpkds=true` → full order detail (per-order fetch)

### Auth

A session cookie `_simpleauth_sess`, sent as `Cookie: _simpleauth_sess=…` alongside the
header `X-Requested-By: hb_android_app`. There is no OAuth; programmatic login is gated by
reCAPTCHA + 2FA, so server-side login is not viable. The user copies `_simpleauth_sess`
from their browser devtools and pastes it into nexorious — the same shape as the PSN
NPSSO-token flow.

## Architecture

Mirror the existing `storefrontadapter.Adapter` pattern. New code is confined to one
service package plus thin API/UI wiring. Sync storefront id `humble` → collection slug
`humble-bundle` (mirrors `epic → epic-games-store`).

### Order detail structure (the two trees)

1. **`subproducts[]`** — what you got. Each has `machine_name`, `human_name`, `downloads[]`.
   Each download has `platform` and a `download_struct[]` of files (`url.web`, …).
   Subproducts may also be promo/info stubs (`*_crosspromo`, `*_freegame_info`) with empty
   `downloads`.
2. **`tpkd_dict.all_tpks[]`** — third-party keys (Steam etc.). **Intentionally never read.**

### Service package `internal/services/humble/`

- **`client.go`** — HTTP client + 200ms rate limiter (like Steam/PSN). Sends the cookie and
  `X-Requested-By` header.
  - `Verify(ctx, cookie)` → `GET /api/v1/user/order`; maps 401 → `ErrCredentials`.
  - `ListGamekeys(ctx, cookie)` → `[]string`.
  - `GetOrder(ctx, cookie, gamekey)` → one order detail. A single failing order is logged
    and skipped so one bad order doesn't sink the whole sync.
- **`models.go`** — minimal structs:
  - `Order{ Gamekey string; Subproducts []Subproduct }`
  - `Subproduct{ MachineName, HumanName string; Downloads []Download }`
  - `Download{ Platform, MachineName string; DownloadStruct []DownloadStruct }`
  - `DownloadStruct{ URL struct{ Web string } }`
  - `tpkd_dict` deliberately unmodeled.
- **`adapter.go`** — implements `GetLibrary`: list gamekeys → fetch each order → apply the
  filtering rule → yield `storefrontadapter.ExternalGameEntry` in batches.

### Filtering rule (the core)

Yield a subproduct as a game **iff all** of:

1. it has ≥1 `download` with `platform ∈ {windows, mac, linux, android}`, **and**
2. that download has a non-empty `download_struct` whose first entry has a non-empty
   `url.web`, **and**
3. its `machine_name` is **not** in the **launcher blocklist**: `{ "uplayclient" }`.
   A scan of the full 138-order test library found `uplayclient` to be the **only**
   launcher/non-game distributed with a game-platform download — so the blocklist contains
   exactly that one confirmed entry. It is a simple slice, trivially extended if another
   launcher is ever observed; we do **not** pre-populate it with unverified guesses.

Never read `tpkd_dict`.

Each yielded `ExternalGameEntry`:

- `ExternalID = subproduct.machine_name`
- `Title = subproduct.human_name`
- `Platforms` = canonical slugs mapped in-adapter: `windows→pc-windows`, `mac→mac`,
  `linux→pc-linux`, `android→android` (union across all qualifying downloads of the
  subproduct)
- `PlaytimeHours = 0` (Humble exposes no playtime)
- `OwnershipStatus = "owned"`

### Dedup behavior (why "report both" is safe)

PC and Android editions emit as two entries with different `ExternalID`s but the same
title. The existing pipeline collapses them, verified end-to-end in
`internal/worker/tasks/sync.go`:

1. Dispatch upserts `external_games` on `(user_id, storefront, external_id)` → two rows.
   The second same-title row gets `parent_id` set to the first.
2. Match: the child inherits the parent's `resolved_igdb_id`.
3. Write: `user_games` upserts on `(user_id, game_id)` → **one** row; `user_game_platforms`
   unions on `(user_game_id, platform, storefront)`.

Result: one library entry showing the union of all platforms — no duplicate, no clobber.
The adapter therefore stays simple and faithful.

### Authentication / credentials

- Stored as encrypted JSON `{ "session_cookie": "…" }` in
  `user_sync_configs.storefront_credentials` (AES-256-GCM, existing pattern).
- **Verify-on-save:** `PUT /api/sync/humble/connection` calls the order API once; a 401
  rejects the paste before storing.
- On later expiry the adapter returns `ErrCredentials` → `credentials_error=true` → UI
  prompts re-paste (identical lifecycle to PSN's NPSSO token).

### Wiring

- **Adapter factory** (`cmd/nexorious/serve.go`): `case "humble"` → decrypt creds →
  unmarshal `{ session_cookie }` → `humble.NewAdapter(humble.NewClient(), cookie)`. Return
  `tasks.ErrCredentials` on unmarshal failure.
- **`platformresolution.StorefrontToCollectionSlug`**: add `"humble" → "humble-bundle"`.
- **API** (`internal/api/sync.go` + `router.go`):
  - add `"humble"` to `supportedStorefronts` (feeds `validStorefronts`) and a
    `storefrontDisplayName` case (`"Humble Bundle"`).
  - routes a single `connection` resource with three verbs:
    `GET /humble/connection` (status), `PUT /humble/connection` (create-or-replace the
    connection: submit + verify creds), `DELETE /humble/connection` (disconnect). **PUT**
    is the canonical method here: the connection is a singleton sub-resource at a fixed,
    client-known URI (one per user+storefront), and establishing it is an idempotent
    create-or-replace of the representation at that URI. These are registered as static
    routes before the parameterised `:storefront` routes (Echo v5 ordering); three methods
    on one static path is fine. This RESTful single-resource shape is the convention #817 is
    updated to standardize the four existing storefronts onto, so Humble conforms from day
    one and is excluded from that cleanup. The generic `:storefront` routes (sync trigger,
    status, external-games, config) then work unchanged.
  - register static `humble` routes before the parameterised `:storefront` routes
    (Echo v5 route-order gotcha).
- **Job source enum** (`internal/db/models/jobs.go`): `JobSourceHumble = "humble"`.
- **Migration** `20260604000003_humble_platform_storefronts.{up,down}.sql`:
  - up: insert `(pc-windows, humble-bundle)`, `(mac, humble-bundle)`,
    `(android, humble-bundle)` into `platform_storefronts`
    (`pc-linux ↔ humble-bundle` already seeded).
  - down: delete those three rows.
  - These associations are load-bearing for **both** the adapter's platform resolution and
    **manual tagging** (the `platform_storefronts` join controls which storefronts are
    selectable for a platform in the UI) — notably enabling Humble Bundle as the storefront
    for Android games.
- **Frontend** (`ui/frontend/`):
  - `SyncStorefront.HUMBLE = 'humble'` in `types/sync.ts` (short id, not the slug), added to
    `SUPPORTED_SYNC_STOREFRONTS` and `getStorefrontDisplayInfo` (iconUrl
    `/logos/storefronts/humble-bundle/humble-bundle-icon-light.svg`).
  - `api/sync.ts`: `connectHumble`/`getHumbleConnection`/`disconnectHumble` (+ request/
    response types), mirroring PSN.
  - `hooks/use-sync.ts` (+ `hooks/index.ts`): `useConnectHumble`/`useHumbleStatus`/
    `useDisconnectHumble` and a `syncKeys` entry.
  - `components/sync/humble-connection-card.tsx` (+ `components/sync/index.ts` export):
    cookie textarea + step-by-step "how to copy `_simpleauth_sess` from devtools" help,
    connect/validate, status, disconnect — built from the shared connection components and
    the PSN card as template.
  - wire into `routes/_authenticated/sync/index.tsx` and `.../sync/$storefront.tsx`
    (status fetch, credentials-error derivation, conditional card render).
  - logos already present at `public/logos/storefronts/humble-bundle/` (light + dark).

### Error handling

- 401 / invalid cookie → `ErrCredentials` → `credentials_error`.
- HTTP 429 / 5xx → surfaced as a job error; River retries via `MaxAttempts`.
- A single failing order is logged and skipped so one bad order doesn't fail the sync.

## Testing

Adapter table tests over **fixtures derived from real order JSON** (download URLs and any
key material scrubbed to placeholders):

- ebook-only excluded; audio excluded; video/asmjs excluded
- Steam-key-only (entry only in `tpkd_dict`, no game subproduct / empty `download_struct`)
  excluded
- promo/info stub (`*_freegame_info`, empty `downloads`) excluded
- **launcher blocklist:** `uplayclient` excluded even though it has a real windows download
- windows game included; **android-only game included**
- game with both a direct download and a Steam key included (it is downloadable)
- dedup / faithful emission by `machine_name` (incl. separate PC + Android editions)

Plus: platform-slug mapping, and `Verify` mapping 401 → `ErrCredentials`. Thin CRUD
handlers / struct accessors get no tests (per repo test policy).

## Out of scope (v1)

- **Humble Trove / "Humble Games Collection"** — separate DRM-free subscription catalog
  with its own fetch path; deferred (requires an active Humble Choice subscription).
- Surfacing / redeeming Steam (or other third-party) keys.
- Any download management.

## References

- Playnite stock Humble Library plugin (DRM-free only):
  https://github.com/JosefNemec/PlayniteExtensions/tree/master/source/Libraries/HumbleLibrary
- Hayden Schiff — Humble Bundle API docs: https://www.schiff.io/projects/humble-bundle-api/
- Hayden Schiff — reverse-engineering the Humble Bundle API:
  https://www.schiff.io/blog/2017/07/21/reverse-engineering-humble-bundle-api/
- FailSpy/humble-steam-key-redeemer (shows `tpkd_dict.all_tpks`):
  https://github.com/FailSpy/humble-steam-key-redeemer
- MestreLion/humblebundle (download_struct / platform filtering):
  https://github.com/MestreLion/humblebundle
