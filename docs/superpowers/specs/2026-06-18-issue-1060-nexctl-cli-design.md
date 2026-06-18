# `nexctl` CLI Client — Design (Epic #1060)

**Status:** Epic. Delivered in phases / multiple PRs. This document is the
overall design; each phase gets its own implementation plan under
`docs/superpowers/plans/`.

**Issue:** #1060. **Prerequisite #1054** (tag-assignment endpoint) is **closed/done**.
**Cross-ref #518** (MCP) is still **open** — the `mcp` command group depends on it.

## Problem

Everyday Nexorious workflows (managing the collection, pools, tags, sync,
import/export, backups, settings, admin users) are only reachable from the
browser. There is no terminal-first way to drive a remote/self-hosted instance.

Today a handful of *client* commands (`login`, `logout`, `whoami`, `api-key`)
live inside the heavyweight `nexorious` **server** binary (`cmd/nexorious/`),
which also embeds the React SPA, migrations, River workers, and DB drivers. A
client-only user has to download the whole server to run `login`.

## Goals & principles

- **Browser parity** for everyday use — daily-driver workflows reachable from the CLI.
- **Human-first, not a 1:1 REST mirror.** Related operations consolidate into
  coherent commands with rich flags (the human counterpart to the MCP
  consolidation in #518).
- **Scriptable.** Everything works non-interactively; interactivity is a TTY
  convenience, never a requirement.
- **Not a TUI.** Transient per-operation pickers are fine; no persistent
  full-screen dashboard.

## Distribution: a separate `nexctl` binary

Ship the client as a **separate binary** `cmd/nexctl`, in this repo, distinct
from `nexorious`.

- **Why separate:** client-only users shouldn't download the server binary; and
  a client binary that imports **only** `internal/clicfg` + `internal/cliclient`
  (+ the new shared CLI helpers) **physically cannot** bypass REST — it enforces
  the "browser-equivalent over the same API" boundary at compile time.
- **Same repo / module / version.** Released by the same release-please flow. No
  second repo, no independent version. Build version/commit injected via the
  same `-ldflags -X main.version=… -X main.commit=…` mechanism as `nexorious`.
- **What lives where:**
  - `nexorious` keeps **server + local ops**: `serve`, `migrate`, `setup`,
    `reset-password`, `version`.
  - `nexctl` gets **all remote-client commands**: account (`login`/`logout`/
    `whoami`/`api-key`/`passwd`), `profile`, the full browser-parity surface
    below, and the `mcp` group (per #518).
- **No back-compat aliases.** Nexorious has no external users yet
  ([[project_no_backcompat_solo_user]]); the four existing client commands are
  **moved out** of `nexorious`, not left behind as deprecated shims.

### Packaging (later phase)

`nexctl` is distributed the same way as `nexorious`: **raw cross-platform
binary, `.deb`, `.rpm`, and a nix package** (no Homebrew/scoop). Container image
and Helm chart remain server-only.

- `.github/workflows/release-artifacts.yaml` gains a `nexctl` build/package
  matrix (per-arch binary → `.deb`/`.rpm` → release assets), mirroring the
  existing `nexorious` lanes.
- Add a `nix/` package for `nexctl` alongside `nix/package.nix`; update
  `vendorHash` per CLAUDE.md → Nix Flake Maintenance.

## Architecture

`nexctl` is a **REST client over HTTP**, authenticated via a stored profile
(`internal/clicfg`: server URL + API key), exactly like today's `login`/
`api-key` commands and like the browser. It does **not** call Go services
directly.

- Consolidated commands may issue **several REST calls** under one invocation
  (e.g. `game edit` = update + progress + platform + tags). That orchestration
  lives client-side in `internal/cliclient`.
- **Profiles** support multiple servers (`--profile`, `nexctl profile …`),
  reusing the existing `clicfg` store/format (already multi-profile capable).

### Shared CLI packages

To let both binaries share the login bootstrap and prompt helpers without
`cmd/nexctl` importing server code, extract two small `internal/` packages:

- **`internal/cliui`** — front-end-agnostic terminal helpers: TTY detection,
  text/secret prompts, confirm, `--json` encoding, `-q`/quiet value emission,
  `FirstNonEmpty`.
- **`internal/cliauth`** — the API-key bootstrap orchestration
  (`LoginAndStoreKey`) and `DefaultServerURL`, built on `cliclient` + `clicfg`.
  Used by both `nexctl account login` and `nexorious setup --login`.

`cmd/nexorious/setup.go` is repointed at these packages; the old in-`package main`
helpers in `cmd/nexorious` are removed.

## Cross-cutting conventions

- **Output:** human-readable tables / detail views by default; global `--json`
  for machine-readable output; `-q`/`--quiet` for bare ids/values to pipe.
- **Interactivity:** on a TTY, rich interactive selection for ambiguous/
  multi-step flows (IGDB match choice, game disambiguation, sync match review).
  Every prompt is **flag-overridable** and **auto-skipped** when stdout isn't a
  TTY or `--json`/`--quiet`/`--yes` is set.
- **Destructive ops** (`rm`, `reset`, `restore`, filtered bulk) confirm on a
  TTY; `--yes` skips.
- **Game references** accept a title (interactive disambiguation on TTY;
  error-with-candidates off-TTY) or an id.
- **Profile selection:** global `--profile <name>` overrides the active profile
  for any command; absent, the `current` profile from `clicfg` is used.
- **Top-level convenience aliases:** the two most-reached-for auth commands are
  also registered at the root — `nexctl login` / `nexctl logout` behave
  identically to `nexctl account login` / `nexctl account logout`. (Cobra has no
  cross-level alias, so these are thin second registrations of the same run
  functions; both appear in help, which is intentional.)

## Command surface (whole epic)

```
account  login | logout | whoami | api-key generate|list|revoke | passwd
         # `login`/`logout` also registered top-level as `nexctl login`/`logout`
profile  list | use <name> | add <name> [--url] | rm <name>
game     list | show <ref> | add <title|--igdb-id> | edit <ref…|--filter> | acquire <ref> | rm <ref…|--filter>
pool     list | show <ref> | create | edit | rm | add | remove | queue | reorder
tag      list | create | rename | rm                         # assignment via `game edit --tag` (#1054, done)
sync     setup | connect | disconnect | run | status | review | resolve | skip | retry | reset
job      list | show | retry | cancel
import   <source> <file> [--map …]
export   [--format json|csv] [--out FILE] [--filter …]
backup   create | restore <file> | list | schedule get|set
config   get|set ;  config notify set|test|clear
admin    user list|create|disable|enable|rm|reset-password ;  admin reset
platform list                                                # reference lookup (read-only)
mcp      config | serve                                       # per #518
```

Top-level workflow shortcuts (later phase): `add`, `played`, `play-next`,
`search`.

### Consolidation decisions (from the issue, settled)

- **No separate `bulk` verb** — `game edit` / `game rm` take explicit refs *or*
  a `--filter` selector (with confirm).
- **Platform rows fold into `game edit`** (`--add-platform`/`--rm-platform`/
  `--hours`), not a `game platform …` sub-group.
- **Wishlist is not its own group** — `game list --wishlist`,
  `game add --wishlist`, `game acquire`.
- **Sync match review** consolidates resolve/skip/rematch into interactive
  `sync review`, with non-interactive `sync resolve` / `sync skip`.

### Driven-from-registry lists (don't hardcode)

- **Sync storefronts** from the live sync registry: `steam`, `psn`/
  `playstation-store`, `gog`, `epic-games-store`, `humble-bundle`.
- **Import sources** from the `importsource`/`csvmap` registries: `vglist`
  (native) plus the CSV presets (generic CSV, Completionator, Grouvee, Darkadia,
  IGDB-id).

## Scope

**In:** collection (full), pools, tags, sync (incl. match review), jobs,
import/export, backup, account, profiles, admin (users), settings — including
the shoutrrr notification-delivery URL (`config notify …`).

**Out:** the notification **inbox/feed** (doesn't fit the CLI). In-app help/docs
viewing (use the web UI). No persistent TUI.

## Phasing

Delivered as multiple PRs by command group. Each phase produces a working,
testable increment.

1. **Scaffold + account/profile** (this epic's first plan) — `cmd/nexctl`,
   cobra root, global flags (`--profile`/`--json`/`-q`/`-y`), TTY-aware helper,
   shared `cliui`/`cliauth` extraction, **move** account commands out of
   `nexorious`, add `profile` management. *(`account passwd` deferred — see below.)*
2. **game** group.
3. **pool** + **tag** groups.
4. **sync** + **job** groups (incl. interactive match review).
5. **import** / **export**.
6. **backup** / **admin** / **config**.
7. **packaging** — release-artifacts matrix + nix package for `nexctl`.
8. **mcp** (`config` + `serve`) — per #518, once it lands.

### Deferred within Phase 1

- **`account passwd`** — `PUT /api/auth/change-password` exists server-side but
  `cliclient` has no method for it yet. Folded into a later account-polish step
  or the `config`/`account` phase to keep the scaffold a clean move + profiles.

## Out of scope / follow-ups

- Notification inbox/feed in the CLI.
- Shell completions (bash/zsh/fish) — once the command tree stabilises.
- A man-page / `docs` build for `nexctl`.
