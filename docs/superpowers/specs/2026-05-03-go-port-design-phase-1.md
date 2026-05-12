### Phase 1 — Infrastructure Skeleton
*Goal: a working binary that starts, runs migrations in the browser, serves the React SPA, and handles auth.*

- Project scaffolding: `go.mod`, directory structure, Makefile
- CLI flags: `--help`, `--version`, `--config`, `--migrate-only` (stdlib `flag`; build-time version injection via `-ldflags`)
- Config (`caarlos0/env`)
- Bun DB connection + initial schema migration (`00000000000001_initial.up.sql`) — full table list including all models
- Bun migrate + migration state machine + browser migration UI (SSE)
- Echo HTTP server: middleware stack, route zones, SPA fallback with `embed.FS`
- IGDB optional credentials: make `IGDB_CLIENT_ID`/`IGDB_CLIENT_SECRET` optional; validate at startup via Twitch token probe; IGDB endpoints return 503 when not configured; `GET /health` reports `igdb_configured` boolean
- Static file route: `/static/cover_art/*` (logos are frontend assets in `ui/public/logos/` — no Go route needed)
- JWT auth: login, refresh, logout; first-run setup flow (server-driven middleware gate, setup/admin); `needsSetup` flag cleared after first admin created
- `GET /api/auth/me` — current user profile; required in Phase 1 because the setup page writes tokens to `localStorage` then redirects to `/`, at which point the React SPA's `AuthProvider` immediately calls this endpoint to validate the token and populate the user object. Without it the SPA breaks on first load after setup. Implementation: verify JWT, query `users` table by `user_id` claim via `db.NewRaw(...)`, return profile. Auth queries use raw Bun SQL (not model-layer ORM) to keep auth isolated from the models package.
- Health/status endpoint
- `internal/filter/` package: query builder + criterion handlers

**Checkpoint:** binary starts, browser shows migration UI on first run, React app loads after migration, setup completes end-to-end (including SPA redirect), login works.
