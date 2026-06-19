# `nexctl` Phase 8 — `mcp` Command Group / MCP Server Design (Epic #1060, #518)

**Status:** Phase 8 (final) of the `nexctl` CLI epic (#1060). Phases 1–7 are
merged. This phase adds the MCP server hosted by `nexctl`. Implementation plan
lives under `docs/superpowers/plans/`.

**Issues:** #1060 (epic) and #518 (MCP). Both blockers are closed: #1094 →
`game stats`, #1095 → `game filters` (the two former "agent-only" tools now exist
as CLI commands and mirror for free). Prerequisite #1054 (tag assignment) is
closed.

## Problem

An AI agent (Claude Desktop, an IDE, Claude Code) on the user's machine should
be able to read and manage their collection — "what should I play next?", "mark
Elden Ring completed", "add Hollow Knight to my wishlist" — without a human
driving the CLI. There is no agent-facing surface today.

## Settled direction (recap from #518 / #1060)

These are decided upstream and are **not** re-opened here:

- **Hosted in `nexctl`**, not the server. Two subcommands: `nexctl mcp serve`
  (runs a local **stdio** MCP server) and `nexctl mcp config` (prints the
  paste-ready agent-config stanza for the active profile).
- **Over REST via `cliclient`.** The MCP server imports only the same layers the
  CLI does — it physically cannot bypass REST, the same compile-time boundary
  the epic prizes.
- **Tool surface is a pure mirror of the CLI command tree** (`game` / `pool` /
  `tag` / `sync`), **not** the bespoke ~12-tool facade in #518's stale body.
  Tool names follow the commands: `game_list`, `game_show`, `game_add`,
  `game_edit`, `game_acquire`, `game_rm`, `game_stats`, `game_filters`,
  `pool_*`, `tag_*`, `sync_*`. Platforms fold into `game_edit` exactly like the
  CLI.
- **Transport: stdio only** for v1. Streamable HTTP is a clean future add (the
  SDK binds transport last; no tool changes needed). Auth for stdio reuses the
  active CLI profile — no key to paste.
- **All tools registered in v1.** Scope-driven registration (read key → read
  tools only) is deferred; a read-scoped key's `403` surfaces as an actionable
  tool error instead.
- **Out of MCP v1:** `admin`, `import`, `export`, `backup`, `config` groups;
  Resources/Prompts primitives; OAuth.

## The one real architectural decision: where the shared orchestration lives

#518 assumed "consolidation, title→ID resolution, and concise projections live
in `cliclient` and are shared with the CLI." **They do not.** `cliclient`
exposes only thin per-endpoint methods (`ListUserGames`, `CreateUserGame`,
`UpdateProgress`, `AddPlatform`, `ReplaceTags`, …). The *consolidation* — IGDB
search→import→create for `game add`; the resolve-refs-or-filter → per-game
(platform/status/rating/tags) fan-out for `game edit`; ref disambiguation — all
lives in **`package main` under `cmd/nexctl/`** (`resolveUserGameRef`,
`resolveIGDBCandidate`, `gamesForRefsOrFilter`, `resolveGameIDs`, `editOne`,
`resolveStorefront`, `resolveExternalRef`, `resolvePoolRef`, `resolveTagRef`).

So "build the consolidation once, expose it as both commands and tools" requires
the orchestration to be callable from both the cobra commands **and** the MCP
tool handlers. Three options:

1. **MCP handlers re-implement orchestration against `cliclient`.** Rejected —
   this is exactly the drift the epic's "one mental model, can't drift" goal
   forbids. `game_add` and `game_edit` would have two independent copies of a
   non-trivial multi-call sequence.
2. **Extract orchestration into a new shared package** (e.g. `internal/cliop`).
   Cleanest separation, but a large refactor of Phases 2–4 across a package
   boundary, and most helpers are tiny.
3. **Keep the MCP server in `package main` (`cmd/nexctl/`)** and refactor the
   handful of orchestration helpers to drop their `*cobra.Command` dependency,
   so both the cobra command and the MCP handler call the *same* functions.
   **← recommended.**

**Decision: option 3.** The MCP server files (`mcp.go`, `mcp_game.go`,
`mcp_pool.go`, `mcp_tag.go`, `mcp_sync.go`) live in `package main` alongside the
commands and call the existing helpers directly. The refactor is mechanical and
small because the `*cobra.Command` parameter is used for only two things:

- **Interactivity gating** (`interactive(cmd)` → TTY + not `--json/--quiet/--yes`).
  Replace with an explicit `interactive bool`. The MCP caller always passes
  `false`.
- **Reading filter flags** in `gamesForRefsOrFilter` / `resolveGameIDs`. Replace
  the `cmd` param with the already-parsed values (a `gameFilter` struct / a
  `[]string` of refs), which the cobra command builds from flags and the MCP
  handler builds from its typed input.

`editOne` already takes a pure `editOpts` data struct — no change. Rendering
helpers (`printStatSection`, table writers, `cliui.EncodeJSON`) stay
CLI-only; the MCP layer has its own concise JSON projections (below).

### Ambiguity resolution across the two front-ends

The ref-resolvers currently branch on `interactive(cmd)`: TTY → interactive
picker (which needs `cmd` for its I/O); off-TTY → error listing candidates. The
MCP front-end is **never** interactive and must hand the candidate list back to
the *agent* to choose, not emit a flat error string. So a resolver cannot be
both `cmd`-free *and* do the picking. The fix is to **extract the pure
match-finding core** (no `cmd`, no I/O) and let each front-end own its
disambiguation:

```go
// findUserGamesByRef returns every library game matching ref: a UUID yields the
// single fetched game; a title yields the search hits. No cmd, no picker.
func findUserGamesByRef(c *cliclient.Client, key, ref string) ([]cliclient.UserGame, error)
```

- **CLI** keeps `resolveUserGameRef(cmd, …)` as a thin wrapper: call the finder;
  one hit → use it; many → picker (interactive) or candidate-list error
  (off-TTY). Behavior unchanged, so existing tests still pass.
- **MCP** calls the finder directly: one hit → use it; many → return a
  structured tool result listing candidates (id + title + status) asking the
  agent to retry with an id. This is the agent-ergonomic disambiguation #518
  specifies.

Same extraction for the other ambiguous resolvers (`findIGDBCandidates`,
`gamesByFilter`, `findPoolsByRef`, `findExternalGamesByRef`); `resolveTagRef` and
`resolveStorefront` already take no `cmd` and are reused as-is. This is the one
match-finding core, exercised two ways — the no-drift guarantee.

## Tool surface (the mirror)

Each tool is one `mcp.AddTool` registration whose handler resolves the profile
(`currentProfile()`), constructs `cliclient.New(p.URL)`, calls the shared
orchestration with `interactive=false`, and projects the result to a concise
struct (full record available via `response_format: "detailed"` where a detail
view exists). Tools are grouped read vs write for the eventual scope-driven
registration.

### Read tools

| Tool | Orchestration / `cliclient` calls | Notes |
|---|---|---|
| `game_list` | `resolveTagID`→`ListUserGames` (+ `resolvePoolRef`) | All `game list` filters as typed fields; concise projection (title, id, status, rating, hours, platforms, tags); paginated. |
| `game_show` | `resolveUserGameRef`→`GetUserGame` | Full single-game detail incl. platform rows. Accepts title-or-id. |
| `game_stats` | `GetCollectionStats` | Distinct purpose; returns the stats record. |
| `game_filters` | `GetFilterOptions` + `ListStorefronts` + `enum.All*` | Facetable values so the agent can build a `game_list` filter in one call. |
| `pool_list` | `ListPools` | List pools. |
| `pool_show` | `resolvePoolRef`→`GetPool` | Pool detail incl. ordered queue + candidates. |
| `tag_list` | `ListTags` | List tag definitions. |
| `sync_status` | `ListSyncConfigs` / `GetSyncStatus` | All storefronts, or one with active-job + counts. |
| `sync_review` | `ListExternalGames` (filter `needs_review`) | **Read** listing of pending matches; resolution is the `sync_resolve`/`sync_skip` write tools (the interactive `sync review` loop has no MCP analogue). |

### Write tools

| Tool | Orchestration / `cliclient` calls | Notes |
|---|---|---|
| `game_add` | `resolveIGDBCandidate`→`ImportIGDBGame`→`CreateUserGame` | Title-or-igdb-id; ambiguous title → candidates back to agent. `wishlist: true` for wishlist adds. |
| `game_edit` | `gamesForRefsOrFilter`→`editOne` per game | The workhorse: status / rating / loved / notes / add-rm-platform / hours / tag / untag. One or many refs, or a `filter`. |
| `game_acquire` | `resolveUserGameRef`→`MoveToLibrary` | Promote wishlist → library with platform. |
| `game_rm` | `gamesForRefsOrFilter`→`DeleteUserGame` per game | One/many refs or a `filter`. |
| `pool_create` | `CreatePool` | `--filter` becomes a typed/raw JSON field. |
| `pool_edit` | `resolvePoolRef`→`UpdatePool` | name / color / filter / clear-filter. |
| `pool_rm` | `resolvePoolRef`→`DeletePool` | |
| `pool_add` | `resolvePoolRef`+`resolveGameIDs`→`AddPoolGame`/`BulkAddPoolGames` | |
| `pool_remove` | `resolvePoolRef`+`resolveGameIDs`→`RemovePoolGame` | |
| `pool_queue` | `resolvePoolRef`+`resolveGameIDs`→`BulkAddPoolGames`+`SetQueue` | Declarative ordered queue. |
| `pool_reorder` | `resolvePoolRef` ×N → `ReorderPools` | Reorder the pools themselves. |
| `tag_create` | `CreateTag` | |
| `tag_rename` | `resolveTagRef`→`UpdateTag` | |
| `tag_rm` | `resolveTagRef`→`DeleteTag` | |
| `sync_run` | `resolveStorefront`→`TriggerSync` | Enqueues; returns the job id; does not block. |
| `sync_resolve` | `resolveStorefront`+`resolveExternalRef`→`RematchExternalGame` | Non-interactive match resolution. |
| `sync_skip` | `resolveStorefront`+`resolveExternalRef`→`SkipExternalGame` | |
| `sync_retry` | `resolveStorefront`→`RetryFailedExternalGames` | |
| `sync_reset` | `resolveStorefront`→`ResetSyncData` | Destructive; the agent calls it explicitly. |
| `sync_disconnect` | `resolveStorefront`→`DisconnectStorefront` | No secrets. |

### Decided — `sync_connect` is excluded from MCP v1

`sync connect` submits storefront **secrets** (Steam API key, PSN `npsso`, GOG
auth code, session cookies). Routing secrets through an LLM tool call means they
land in the agent's context/transcript. The CLI prompts for them no-echo
precisely to keep them off-screen. **Decision (confirmed): exclude
`sync_connect` from MCP v1.** A strict "pure mirror" would include it, but the
secret-handling trade-off argues against it; connecting a storefront is a
one-time setup step a human does in the CLI/UI, not an agent workflow. The agent
still gets `sync_run`/`status`/`review`/`resolve`/`skip`/`retry`/`reset`/
`disconnect`.

## Input / output schema design

- **Typed Go structs per tool**, registered via the generic `mcp.AddTool`, which
  infers the JSON Schema from the struct and `jsonschema:"…"` field tags,
  validates input, and marshals output. No hand-written schemas.
- **Enum constraints surfaced in the schema** for `play_status`
  (`not_started`, `in_progress`, `completed`, `mastered`, `dominated`,
  `shelved`, `dropped`, `replay`) and `ownership_status`, sourced from
  `internal/enum` so they cannot drift.
- **Concise by default, detailed on request.** List/detail projections carry
  human fields (title, status, platform names, tags) **plus the id** for
  chaining; `response_format: "detailed"` returns the full record where a detail
  endpoint exists. Pagination defaults steer the agent toward filters on large
  result sets.
- **Game reference fields accept title-or-id**; reads always return both so the
  agent can chain precisely.

## `nexctl mcp config`

Prints the stdio stanza for the active profile (a `--profile` flag selects
another). No URL/key — auth is the active profile:

```jsonc
{
  "mcpServers": {
    "nexorious": { "command": "nexctl", "args": ["mcp", "serve"] }
  }
}
```

## Errors

- A tool handler returns an ordinary Go `error` on failure; the SDK records it in
  the `CallToolResult` with `IsError: true`, so the agent sees the message as
  actionable text (not a swallowed code).
- Map `cliclient` HTTP errors to specific, corrective messages. In particular a
  **`403` from a read-scoped key on a write tool** returns "this profile's API
  key is read-only; mint a write-scoped key to modify the collection" — the v1
  substitute for scope-driven registration.

## Authentication & scope

- stdio reuses `currentProfile()` / `internal/clicfg`; the stored key is
  write-scoped by default (`cliclient.defaultScopes = "write"`).
- All tools are registered regardless of the key's scope in v1; read-only keys
  get the actionable `403` above. Scope-driven registration (querying the key's
  `Scopes` via `ListAPIKeys` and registering only read tools for a `read` key)
  is a documented future enhancement, deferred.

## Dependencies & packaging

- Adds `github.com/modelcontextprotocol/go-sdk` (target the latest stable,
  currently **v1.6.1**) to `go.mod` — a **`nexctl`-only** import, so the
  `nexorious` server binary does not link it.
- `go.mod`/`go.sum` change ⇒ update **`vendorHash` in BOTH `nix/package.nix`
  and `nix/nexctl.nix`** (shared value) per CLAUDE.md → Nix Flake Maintenance,
  and verify `nix build .#nexctl`.
- Register `newMCPCmd()` on the root in `cmd/nexctl/main.go`.
- No new release-artifacts wiring — `nexctl` packaging (Phase 7) already ships
  the binary; the new subcommand rides along. The repo-wide source SBOM already
  covers the new dependency.

## Testing

- **Orchestration-helper refactor:** the existing `cmd/nexctl` command tests must
  still pass after the `*cobra.Command` → explicit-param change (behavior
  unchanged); add a unit test for the new ambiguous-ref return path
  (`(nil, candidates, nil)`).
- **MCP tools:** drive the registered `mcp.Server` in-process with the SDK's
  client/in-memory transport over a stubbed `cliclient` HTTP server
  (`httptest`), asserting: schema generation for a representative tool, the
  read-vs-write split, a write tool's `403`→actionable-error mapping, and the
  ambiguous-ref candidate result. Avoid one test per trivial mirror; cover the
  workhorses (`game_add`, `game_edit`) and the cross-cutting behaviors.

## Out of scope (v1)

- Streamable HTTP transport / remote-agent hosting.
- Scope-driven tool registration (read key → read tools only).
- `admin` / `import` / `export` / `backup` / `config` tools.
- MCP Resources / Prompts; OAuth.
- `sync_connect` (excluded — secrets through an agent; see decision above).
- The eval harness (#1059) — build tools so it can drive them.
