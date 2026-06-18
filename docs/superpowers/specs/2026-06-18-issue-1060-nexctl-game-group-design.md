# `nexctl` Phase 2 — `game` Command Group Design (Epic #1060)

**Status:** Phase 2 of the `nexctl` CLI epic (#1060). Phase 1 (scaffold + account/profile) merged in PR #1081. This phase adds the full `game` command group in one PR.

**Builds on:** the merged `internal/cliui` (TTY/prompt/confirm/JSON helpers, `EncodeJSON`), `internal/cliclient` (HTTP client), `cmd/nexctl` root + global flags (`--profile`/`--json`/`-q`/`-y`) and helpers (`resolveProfile`, `profileName`, `flagBool`).

## Problem

`nexctl` can authenticate and manage profiles, but cannot yet touch the collection. This phase makes the daily-driver collection workflows reachable from the terminal: browse/search the library, add games (with IGDB lookup), edit them (status, rating, loved, notes, playtime, platforms, tags), promote wishlist→library, and remove them.

## Command surface (this phase)

```
game  list   [--status --ownership --tag --platform --storefront --genre
              --wishlist --loved --has-notes --rating-min/max --hours-min/max
              --pool --sort --order --limit --page]   [--json|-q]
      show   <ref>                                    [--json]
      add    <title> | --igdb-id <id>
             [--status --platform --storefront --wishlist --rating --loved --notes]
      edit   <ref…> | --filter <list-flags>
             [--status --rating --loved/--no-loved --notes
              --add-platform p[/storefront] --rm-platform p --hours N [--platform p]
              --tag NAME (repeatable) --untag NAME (repeatable)]
      acquire <ref>  --platform p[/storefront] [--ownership]   # wishlist → library
      rm     <ref…> | --filter <list-flags>            [-y]
```

`game` is registered on the `nexctl` root (alongside `account`, `profile`, `version`).

## Architecture

Pure REST client over the bearer key from the active profile. Every command resolves the profile via the existing `resolveProfile(cmd)`. All new endpoint calls live in `internal/cliclient`; the `cmd/nexctl` command files orchestrate them. The client must remain server-package-free (import boundary unchanged).

`add` and `edit` are **multi-call orchestrations** (client-side; no cross-call transaction — sequential, fail-fast per game with a clear error naming the step).

### New `cliclient` methods (the client currently has none for games)

- **IGDB:** `SearchIGDB(key, query, limit)`, `GetIGDBGame(key, igdbID)`, `ImportIGDBGame(key, igdbID)`.
- **Read:** `ListUserGames(key, params url.Values)`, `GetUserGame(key, id)`.
- **Mutate:** `CreateUserGame`, `MoveToLibrary`, `UpdateUserGame` (partial field map), `UpdateProgress`, `AddPlatform`, `UpdatePlatform`, `DeletePlatform`, `ReplaceTags`, `DeleteUserGame`.
- **Reference:** `ListTags(key)` (to resolve `--tag NAME`→id for the list filter).

Response decode types (subset of the API's `userGameWithPlatformsResponse`): `UserGame{ID, GameID, PlayStatus *string, PersonalRating *int, IsLoved, IsWishlisted, PersonalNotes *string, Game *{ID,Title}, HoursPlayed float64, Platforms []UserGamePlatform, Tags []Tag}`; `UserGamePlatform{ID, Platform *string, Storefront *string, HoursPlayed *float64, OwnershipStatus *string}`; `Tag{ID, Name, Color *string}`; `IGDBCandidate{IgdbID int, Title, ReleaseDate, … , UserGameID *string}`.

## Resolution rules

### Library reference (`show`, `edit`, `acquire`, `rm`)
`<ref>` resolves against the **library**:
- If `ref` parses as a UUID → use directly (no lookup).
- Else treat as a title query → `ListUserGames(q=ref)`. 0 matches → error `no game matching "<ref>"`. 1 → use it. Many → on a TTY, an interactive numbered picker (id + title + status); off-TTY (or `--json`/`-q`/`--yes`) → error listing the candidate ids+titles so the user can re-run with an id.

### IGDB reference (`add`)
- `--igdb-id N` → `GetIGDBGame(N)` (skip search).
- Else `<title>` → `SearchIGDB(title, limit 10)`. 0 → error. 1 → use. Many → TTY picker (title + year + "(in library)" marker via `UserGameID`); off-TTY → error listing candidates with their igdb ids.

## Command behaviour

- **`list`** — maps flags to query params (`play_status`, `ownership_status`, `is_loved`, `has_notes`, `rating_min/max`, `time_to_beat_min/max`, `platform`, `storefront`, `genre`, `wishlist`, `sort_by`, `sort_order`, `per_page`/`page`). `--tag NAME` is resolved to a tag UUID via `ListTags` (case-insensitive; unknown name → error). `--pool` accepts a pool UUID passthrough (name resolution arrives with the pool group). Default output: a table (ID, TITLE, STATUS, RATING, HOURS, PLATFORMS, TAGS). `--json` emits the raw list; `-q` emits bare user-game ids.
- **`show <ref>`** — detail view of one user-game (human or `--json`).
- **`add`** — resolve IGDB candidate → `ImportIGDBGame(igdbID)` (pull metadata into the local DB) → `CreateUserGame{game_id, play_status (--status, default not_started), is_wishlisted (--wishlist), personal_rating (--rating), is_loved (--loved), personal_notes (--notes), platforms}`. `--wishlist` and `--platform` are mutually exclusive (client-side guard, mirrors the server's 422). With `--platform p[/storefront]`, one platform row is created (ownership `owned`).
- **`edit`** — resolve ref(s) or `--filter`; per game, apply in this order, each a separate REST call, fail-fast with a step-naming error: (1) `--add-platform`/`--rm-platform` (rm resolves the platform_id from the game's current platforms by slug); (2) `--hours N` → `UpdatePlatform` on the target platform (`--platform` selects it; if the game has exactly one platform, default to it; otherwise error asking which); (3) `--status` → `UpdateProgress`; (4) `--rating`/`--loved`/`--no-loved`/`--notes` → `UpdateUserGame` (partial field map); (5) `--tag`/`--untag` → fetch current tags, compute `current ∪ added \ removed`, `ReplaceTags`. Prints a per-game summary.
- **`acquire <ref>`** — resolve a (wishlist) library ref → `MoveToLibrary{platforms:[{platform (--platform, required), storefront, ownership_status (--ownership, default owned)}]}`.
- **`rm`** — resolve ref(s) or `--filter` → confirm (unless `-y`/non-TTY with `-y`) → `DeleteUserGame` each. `--filter` performs a bulk delete of every matched game (always confirmed unless `-y`).

## Cross-cutting conventions (unchanged from Phase 1)

- Output: human table/detail by default; `--json`; `-q` bare ids/values. Use `cliui.EncodeJSON`.
- Interactivity: pickers only on a TTY and never when `--json`/`-q`/`--yes` is set; otherwise error with candidates.
- Destructive ops (`rm`, bulk `edit`/`rm` via `--filter`) confirm on a TTY; `--yes` skips.
- No secrets in output (N/A here — game data carries none).

## Out of scope (later phases)

- `pool` / `tag` management groups, `sync`, `job`, `import`/`export`, `backup`, `admin`, `config`, `mcp` — later phases of #1060.
- `--pool <name>` resolution (pool group); for now `--pool` takes a UUID.
- Shell completions; release packaging (Phase 7).
- Top-level workflow shortcuts (`add`, `played`, `play-next`, `search`) — can wrap these commands later.
