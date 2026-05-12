### Phase 4 — Sync Integrations
*Goal: automated library sync from external platforms.*

- `external_games` and `user_sync_configs` sync config API handlers (see `2026-05-10-sync-api-design.md`)
- Skip/un-skip endpoints via sync router (`GET/DELETE /api/sync/ignored`) operating on `external_games.is_skipped`
- `services/platform_resolution.go` — raw platform name → slug mapping
- `services/matching.go` — IGDB title matching using `FuzzyConfidence` (go-fuzzywuzzy)
- `services/steam/` — `SteamClient` interface implementation; direct HTTP to `api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002`
- `services/psn/` — `PSNClient` interface implementation using `github.com/sizovilya/go-psn-api`; calls `AuthWithNPSSO` then `GetProfile(ctx, "me")`
- Steam sync (dispatch + process)
- PSN sync
- Metadata refresh (dispatch + process)
- Remaining scheduler jobs (`CheckPendingSyncsTask` every 15 minutes, metadata refresh interval)
- Epic Games Store sync is **not in scope for Phase 4**; see Epic Games Store Sync section

**Checkpoint:** sync integrations work end-to-end.
