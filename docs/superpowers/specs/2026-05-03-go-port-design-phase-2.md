### Phase 2 — Core Game API
*Goal: full read/write game collection functionality via the existing React UI.*

- Bun model structs (`internal/db/models/`) for all Phase 2 tables: games, user_games, user_game_platforms, platforms, storefronts, platform_storefronts, tags, user_game_tags
- Games API (`/api/games`, `/api/games/:id`, search, IGDB import, metadata endpoints)
- User games API (list with dynamic filtering via `internal/filter/` criteria functions, sort, CRUD, platform associations)
- IGDB result ranking: `go-fuzzywuzzy` + `NormalizeTitle`; local list search uses `ILIKE` only
- Platforms and tags read endpoints (JWT required; read-only — no write/admin endpoints; see static platforms spec)
- User-games filter-options / genres / stats (`GET /api/user-games/stats`) / ids endpoints
  - Note: `GET /api/platforms/stats` and `GET /api/platforms/storefronts/stats` are **not implemented** — cancelled per the static platforms spec
- IGDB service (rate-limited HTTP client, cover art storage)
- Remaining auth profile endpoints: `PUT /api/auth/me`, `PUT /api/auth/change-password`, `GET /api/auth/username/check/:username`, `PUT /api/auth/username` (`GET /api/auth/me` is Phase 1 — see above)

**Checkpoint:** React frontend fully usable for browsing and managing game collection.
