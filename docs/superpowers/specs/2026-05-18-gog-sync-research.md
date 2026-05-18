# GOG Sync — Research Notes

**Date:** 2026-05-18
**Status:** Research (pre-design)

This is a research dossier for adding GOG library sync to nexorious-go. It is not yet a design doc — the goal is to capture options, references, and a recommendation so a follow-up session can move directly to writing the design.

## Context

nexorious-go currently syncs Steam and PSN, and an Epic Games Store sync via the `legendary` Python CLI is in progress on `feat/epic-sync`. The user has asked whether GOG can be added next, citing Playnite as proof that it's possible.

## How Playnite does GOG

There are two Playnite plugins:

1. **GOG Library plugin** (bundled with Playnite) — reads from a locally installed GOG Galaxy client (its SQLite DB and Communication Service). Useless to us: requires GOG Galaxy installed on the user's machine, Windows-only in practice.

2. **playnite-gog-oss-plugin** by hawkeye116477 — <https://github.com/hawkeye116477/playnite-gog-oss-plugin>. Same author as the Epic/Legendary Playnite plugin our `feat/epic-sync` mirrors. Galaxy-free, uses two open-source CLI tools:
   - **gogdl** — <https://github.com/Heroic-Games-Launcher/heroic-gogdl>. Python CLI, the GOG equivalent of `legendary`. Handles OAuth login (browser handoff), library listing, install, cloud saves. There's also a third-party rewrite <https://github.com/fernandonr189/gogdl-cli> exposing commands like `gogdl games -l`.
   - **comet** — <https://github.com/imLinguin/comet>. Rust. Open-source reimplementation of GOG Galaxy's *Communication Service* so games using the Galaxy SDK (Witcher 3, Cyberpunk) can get achievements/cloud saves while running. **Not needed for library sync** — only relevant if we ever want to *launch* GOG games.

## The GOG API surface

Library data lives on a small set of `embed.gog.com` / `api.gog.com` HTTP+JSON endpoints. They are unofficial in the sense that GOG has never published an SDK for them, but they are stable: documented at <https://gogapidocs.readthedocs.io/> (originally written 2017, still accurate).

### Auth
- OAuth2-style authorization-code flow.
- Public client_id: `46899977096215655`. (This is the same ID used by every third-party GOG client — Heroic, Lutris, gog-backup, gogdl. It's effectively the "Galaxy public client".)
- Flow:
  1. Open `https://login.gog.com/auth?client_id=46899977096215655&redirect_uri=...&response_type=code&layout=client2`
  2. User logs in in browser → redirected with `?code=...`
  3. `POST https://auth.gog.com/token` with `grant_type=authorization_code&code=...&client_id=...&client_secret=...&redirect_uri=...` → returns `access_token` (~1h lifetime) + `refresh_token` (long-lived).
  4. Refresh via `grant_type=refresh_token`.
- No headless username/password flow exists. CAPTCHA may be presented. For a web app the natural shape is: popup window → OAuth callback route. For headless/CLI usage the user pastes the redirected code (this is what `legendary auth --disable-webview` does and what `gogdl` falls back to).
- Reference: <https://gogapidocs.readthedocs.io/en/latest/auth.html>

### Owned-games endpoints
- `GET https://embed.gog.com/user/data/games` — JSON `{ "owned": [<id>, <id>, …] }`. ID list only.
- `GET https://embed.gog.com/account/getFilteredProducts?mediaType=1&page=N` — paginated rich game data: title, image, platforms, tags, release date, category, etc. This is what's needed for library sync.
- `GET https://api.gog.com/products/<id>?expand=description,screenshots,videos,related_products,changelog` — per-game detail.
- Reference: <https://gogapidocs.readthedocs.io/en/latest/account.html>

### Reference Go implementations (all open source)
- <https://pkg.go.dev/github.com/mscharley/gog-backup/pkg/gog> — clean Go client. Auth + library listing. Probably the closest model.
- <https://pkg.go.dev/github.com/habedi/gogg/client> — newer; has `RefreshToken` helper, `DownloadGameFiles`.
- <https://github.com/j05h/goggle> — opens a Chromium window for login, stores token in `~/.config/goggle/token.json`. Good UX reference.
- <https://pkg.go.dev/github.com/arelate/gog_auth> — auth flow only, including 2FA handling.

## Options

### Option A — Native Go client (recommended)
Hit `embed.gog.com` / `auth.gog.com` directly from Go. No subprocess, no bundled binaries, no Python runtime.

**Pros:**
- Mirrors how Steam/PSN are already implemented in `internal/services/` — same shape, same testing approach (httptest).
- No bundled binary in Kubernetes / devenv. No equivalent of `LEGENDARY_WORK_DIR`, snapshot/restore, embedded subprocess.
- Auth flow is plain OAuth2 — `golang.org/x/oauth2` handles it directly, with a custom endpoint.
- Library listing is ~2 endpoints. Estimated client size: a few hundred lines + tests.

**Cons:**
- We own the auth+endpoint code; if GOG changes something we have to fix it. (In practice they haven't changed in ~8 years.)
- If we later want install/download/cloud-save (we don't today), we'd either grow this client or shell out to gogdl after all.

### Option B — Shell out to gogdl
Same `legendary`-style pattern as `feat/epic-sync`.

**Pros:**
- gogdl owns the auth + library protocol; we get install/download/cloud-saves "for free" if we ever want them.

**Cons:**
- Python runtime dependency. gogdl is a PyInstaller'd binary on releases but still drags Python — adds packaging complexity to devenv.nix and Helm chart, similar to legendary.
- The snapshot/restore dance from epic-sync would need to be duplicated for gogdl's config dir.
- JSON-over-stdout parsing for what is fundamentally already an HTTP+JSON API.
- gogdl is "user-friendly CLI is not a goal" (per its README) — interface is intended for Heroic, not stable for third-party use.

### Option C — GOG Galaxy SDK / Comet
**Not applicable.** Galaxy SDK is for game developers shipping a game on GOG (achievements, leaderboards inside the game). Comet implements the SDK's runtime side so SDK-using games can run outside Galaxy. Neither speaks to library listing.

## Recommendation

**Option A.** Native Go client modeled after `internal/services/steam/`. Concretely:

- New package `internal/services/gog/` with:
  - `client.go` — HTTP client, token refresh, retry handling
  - `auth.go` — OAuth code-flow helpers (build authorize URL, exchange code, refresh)
  - `library.go` — `getFilteredProducts` paging, model mapping
- Reuse the same `BrowserAuthSession`-style pattern Steam/PSN use for the user-facing OAuth handoff.
- Tokens stored on the user record the same way Steam/PSN tokens are.
- River worker `worker/tasks/sync_gog.go` orchestrating job_items the same way the existing syncs do.
- Platform/storefront mapping: add `gog` → `gog-galaxy` (or similar) in `platformresolution`, with logos under `ui/frontend/public/logos/storefronts/gog/`.

This is a smaller change than epic-sync because there's no subprocess and no snapshot/restore — it slots into the existing Steam/PSN shape directly.

## Open questions for the design session

1. **Login UX.** Web flow (popup → callback route) is straightforward. Do we also need a "paste the code" fallback for headless/self-hosted deployments? Probably yes for symmetry with Epic.
2. **Token storage.** Steam uses a long-lived API key, PSN uses a refresh token. GOG matches PSN's shape — confirm we can reuse `UserSyncConfig` columns.
3. **Rate limiting.** GOG has no documented rate limit. `embed.gog.com` is friendly to ~1 req/s. Reuse `internal/ratelimit/` with conservative defaults?
4. **Game matching / metadata.** GOG returns its own product IDs and titles. We currently match Steam/PSN against IGDB — same approach should work for GOG. Need to confirm IGDB has good GOG ID coverage.
5. **Achievements & playtime.** GOG exposes achievements per-game (`GET /clients/<client_id>/users/<user_id>/achievements`) and last-played timestamps. Out of scope for v1?
6. **Risk.** GOG has not, in practice, blocked third-party clients using `46899977096215655` (Heroic, Lutris, gog-backup all use it openly). Reddit thread <https://www.reddit.com/r/gog/comments/1m84j87/> is the most recent confirmation hobby/personal use is fine. Worth a footnote, not a blocker.

## Caveats to flag in the design doc

- Unofficial API — no SLA. Mitigate by isolating all GOG HTTP knowledge in one package and keeping the model layer agnostic.
- Refresh token rotation behavior is not documented; some third-party clients re-store the refresh token after every refresh call. Follow that convention.
- `account/getFilteredProducts` returns at most ~50 items per page. Library sync must page.

## References (consolidated)

- Unofficial GOG API docs: <https://gogapidocs.readthedocs.io/>
- Playnite GOG OSS plugin: <https://github.com/hawkeye116477/playnite-gog-oss-plugin>
- gogdl (Heroic): <https://github.com/Heroic-Games-Launcher/heroic-gogdl>
- gogdl-cli (third-party rewrite): <https://github.com/fernandonr189/gogdl-cli>
- Comet (Galaxy Communication Service): <https://github.com/imLinguin/comet>
- Go references: <https://github.com/mscharley/gog-backup>, <https://github.com/habedi/gogg>, <https://github.com/j05h/goggle>, <https://pkg.go.dev/github.com/arelate/gog_auth>
- Heroic GOG architecture overview: <https://deepwiki.com/Heroic-Games-Launcher/HeroicGamesLauncher/3.2-gog>
- Reloaded-III GOG interaction notes (good summary of OAuth flow and endpoints): <https://reloaded-project.github.io/Reloaded-III/Server/Storage/Loadouts/Stores/GOG.html>
- Awesome GOG Galaxy index: <https://github.com/Mixaill/awesome-gog-galaxy>
