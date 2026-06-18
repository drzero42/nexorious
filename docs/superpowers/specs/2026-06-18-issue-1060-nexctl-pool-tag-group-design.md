# `nexctl` Phase 3 — `pool` + `tag` Command Groups Design (Epic #1060)

**Status:** Phase 3 of the `nexctl` CLI epic (#1060). Phases 1 (#1081) and 2 (#1083) merged. This phase adds the `tag` and `pool` command groups in one PR.

**Builds on:** merged `internal/cliui`, `internal/cliclient` (incl. `doBearer`, `ListTags`, `UserGame`), and `cmd/nexctl` root + helpers (`resolveProfile`, `flagBool`, `interactive`, `resolveUserGameRef`, render helpers).

## Problem

`nexctl` can manage the collection but not the two organizing layers: **tags** (labels assigned to games) and **pools** (play-planning groups with an ordered queue + candidate set + an optional saved filter). This phase makes both manageable from the terminal, and unblocks name-based `--tag`/`--pool` resolution that Phase 2 deferred.

## Command surface (this phase)

```
tag   list                              [--json|-q]
      create <name> [--color]
      rename <ref> <new-name>
      rm <ref>                          [-y]

pool  list                              [--json|-q]
      show <ref>                        [--json]
      create <name> [--color --filter <json>]
      edit <ref> [--name --color --filter <json>|--clear-filter]
      rm <ref>                          [-y]
      add <pool-ref> <game-ref…>        # add games as candidates
      remove <pool-ref> <game-ref…>
      queue <pool-ref> [game-ref…]      # declarative ordered queue (no games = clear)
      reorder <pool-ref…>               # set the order of the pools themselves
```

Both groups register on the `nexctl` root.

## Architecture

Pure REST client over the bearer key (`resolveProfile`). New `internal/cliclient` methods; `cmd/nexctl/{tag,pool}*.go` orchestrate them.

**Reference resolution:**
- **Tag ref** (`tag rename/rm <ref>`): UUID → used directly; else name → `ListTags` case-insensitive match (0 → error; 1 → use; many is impossible — names are unique per user, but guard anyway). Helper `resolveTagRef`.
- **Pool ref** (`pool show/edit/rm/add/remove/queue/reorder`): UUID → used directly; else name → `ListPools` case-insensitive match (0/1/many → error/use/TTY-pick-or-candidate-error, mirroring `resolveUserGameRef`). Helper `resolvePoolRef`.
- **Game refs** in `pool add/remove/queue` (`<game-ref…>`): each resolved via the existing `resolveUserGameRef` → `user_game_id` (pools reference games by user-game id).

### New `cliclient` methods

- **Tag:** `CreateTag(name, *color)`, `UpdateTag(id, *name, *color)`, `DeleteTag(id)`. Extend the existing `Tag` struct with `GameCount int64 json:"game_count"` (populated only by the list endpoint; harmless elsewhere).
- **Pool read:** `ListPools()` → `[]PoolListItem{ID,Name,Color,Position,HasFilter,QueueCount,CandidateCount}`; `GetPool(id)` → `PoolDetail{Pool + Queue []UserGame + Candidates []UserGame}`.
- **Pool mutate:** `CreatePool(name,*color,filter json.RawMessage)`, `UpdatePool(id, fields map[string]any)`, `DeletePool(id)`, `AddPoolGame(id,userGameID)`, `BulkAddPoolGames(id,[]userGameID) (added int64)`, `RemovePoolGame(id,userGameID)`, `SetQueue(id,[]userGameID)`, `ReorderPools([]poolID)`.
- Types: `Pool{ID,Name,Color,Position,Filter json.RawMessage,HasFilter,...}`, `PoolListItem`, `PoolDetail`.

## Command behaviour

- **`tag list`** — table (ID/NAME/COLOR/GAMES via `game_count`); `--json` raw; `-q` bare ids.
- **`tag create <name> [--color]`** — `CreateTag`; 409 surfaces as "tag already exists".
- **`tag rename <ref> <new>`** — `resolveTagRef` → `UpdateTag(name=new)`.
- **`tag rm <ref>`** — confirm unless `-y` → `DeleteTag`.
- **`pool list`** — table (ID/NAME/POS/QUEUE/CANDIDATES/FILTER); `--json`; `-q` ids.
- **`pool show <ref>`** — meta + an ordered `QUEUE` list (position, title, status) and a `CANDIDATES` list; `--json` raw detail.
- **`pool create <name> [--color --filter <json>]`** — `--filter` is a raw JSON string of shape `{"filters":[{play_status,genre,tag,platform,storefront,rating_min/max,is_loved,…}]}` (tag values are UUIDs); passed through as `json.RawMessage`, validated server-side (400 on bad shape). Omitted → manual pool.
- **`pool edit <ref>`** — partial: `--name`/`--color`/`--filter <json>`; `--clear-filter` sends `filter:null`; `--color ""`-style clear via a `--clear-color` is out of scope (use edit with a new color). At least one change required.
- **`pool rm <ref>`** — confirm unless `-y` → `DeletePool`.
- **`pool add <pool> <game…>`** — resolve pool + each game ref; 1 game → `AddPoolGame`, >1 → `BulkAddPoolGames` (reports added count). Games join as candidates.
- **`pool remove <pool> <game…>`** — resolve, `RemovePoolGame` each.
- **`pool queue <pool> [game…]`** — resolve pool + game refs; **bulk-add** the resolved ids first (idempotent, so non-members are added), then `SetQueue(ids)` in the given order. No game refs → `SetQueue([])` clears the queue (all members become candidates). This makes `queue` a one-shot "these games, in this order."
- **`pool reorder <pool…>`** — resolve each pool ref → `ReorderPools(ids)` (positions assigned by argument order). Non-interactive (no TUI). List pools in the desired order.

## Cross-cutting conventions (unchanged)

Human table/detail default; `--json`; `-q` bare ids. Confirms on destructive ops (`tag rm`, `pool rm`) unless `-y`/non-TTY. No interactive reorder editor (not a TUI).

## Out of scope (later phases)

- `sync`/`job` (Phase 4), `import`/`export` (5), `backup`/`admin`/`config` (6), packaging (7), `mcp` (8, blocked on #518).
- A structured `--filter` DSL (raw JSON only this phase).
- Resolving `pool create --filter` tag **names** → UUIDs client-side (the JSON carries UUIDs); a future nicety.
- Interactive queue/reorder editors.

## Follow-up enabled by this phase

Once `tag`/`pool` exist, `game list --tag NAME` already resolves names (Phase 2), and `game list --pool` can be upgraded from UUID-passthrough to name resolution via `resolvePoolRef` — a small follow-up, not required here.
