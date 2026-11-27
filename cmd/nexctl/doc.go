// Command nexctl is the Nexorious REST client. Every command calls the
// server's REST API through cliclient; operations shared with the MCP tools
// live in internal/clientops, and terminal helpers in internal/cliui.
//
// # Linking
//
// The MCP Go SDK (modelcontextprotocol/go-sdk) is a server-only dependency:
// nexctl never links it or internal/mcptools (TestNexctlDoesNotLinkMCPSDK).
//
// # Command groups
//
//   - account: login/logout/whoami/api-key. `login --scope read|write` picks
//     the stored key's scope (default write). With --profile <name>,
//     login/logout touch only that profile and never switch the current one
//     (clicfg.StoreProfile).
//   - profile.
//   - game: list/show/add/edit/acquire/rm/stats/filters.
//   - tag: list/create/rename/rm.
//   - pool: list/show/create/edit/rm/add/remove/queue/reorder.
//   - sync: status/connect/disconnect/config/run/review/resolve/skip/retry/reset.
//   - job: list/show/retry/cancel.
//   - import: sources/nexorious/csv/run/review/resolve/skip.
//   - export.
//   - backup: list/create/rm/download/restore/schedule [set].
//   - admin: user list/show/create/enable/disable/set-admin/passwd/rm, and reset.
//   - config: get/set, and notify channel/sub/test-url/events.
//   - mcp: config/serve.
//   - doctor: library-health smells (summary, --check, apply/ignore/restore/ignored).
//   - setup: admin/backups/restore, unauthenticated and pre-bootstrap.
//   - migrate: run/status, HTTP-driven against a running server.
//
// The persistent -y confirms destructive operations (job cancel, sync
// disconnect/reset/skip, import skip, and the rest) without a prompt.
//
// # game
//
// `game add` and `game edit` are multi-call orchestrations: IGDB search, then
// import, then create; and ordered platform/hours/status/fields/tags updates
// (clientops.EditOne). `game stats` is a read-only wrapper over
// GET /api/user-games/stats (CollectionStats): a sectioned table by default,
// --json for the raw record, -q for just total_games.
//
// `game filters` is read-only filter discovery (sectioned table, --json, or -q
// flat values). Play and ownership statuses come from internal/enum
// (AllPlayStatuses/AllOwnershipStatuses, stable enums imported directly);
// storefronts from GET /api/platforms/storefronts/simple-list; genres, game
// modes, themes and perspectives from GET /api/user-games/filter-options. Only
// server-owned values are fetched; tags and platforms have their own commands.
//
// # pool
//
// Pools key off user_game_id; game refs resolve through resolveUserGameRef
// (over clientops.FindUserGamesByRef). `pool queue` bulk-adds, then sets the
// declarative order. `pool create --filter` takes a raw JSON {"filters":[…]}
// string.
//
// # sync
//
// The storefront slug set comes from the server's registry through
// resolveStorefront; there is no hardcoded list. `sync connect` builds a
// per-storefront credential body, prompting for secrets without echo.
//
// `sync review` is the External game match review (rematch/skip) and runs only
// on a TTY; off a TTY use `sync resolve` / `sync skip`. With
// --only-this-edition, resolve and skip decide one edition alone (#1452): the
// request goes to the edition's id (from the list entry's editions), never the
// entry's. A title naming an entry with several editions, or several entries,
// is ambiguous, and the picker or error lists the editions (id, storefront id,
// platforms). `sync review` always decides the whole set. There is no unskip
// command.
//
// # import
//
// Imports are multipart uploads through cliclient's doBearerMultipart (the
// server caps them at 50 MB).
//
//   - `import nexorious` is the JSON round-trip.
//   - `import csv` with no flags runs auto mode: the server detects a preset by
//     header signature or builds a guessed mapping refined from the data,
//     enqueues, and nexctl prints what it chose. --inspect shows the headers,
//     the suggested mapping and the detected preset (the only way to list
//     presets, so they are never hardcoded). --preset names a preset; the
//     manual --*-col/--status-map/--rating-scale flags assemble the server's
//     mapping JSON. Preset and manual flags are mutually exclusive. The server
//     still answers format=generic without a mapping with 400 "missing mapping
//     field".
//   - `import run <source>` is validated against the registry by
//     resolveImportSource, like resolveStorefront. nexorious and csv are not
//     registry sources; they have their own endpoints.
//   - `import review`/`resolve`/`skip` is the import Job item match review: it
//     pages through every pending_review item and searches IGDB live, then
//     calls /api/job-items/:id/resolve or /skip. It is separate from sync
//     review.
//
// # export
//
// `export [--format json|csv] [--out FILE|-] [--no-wait]` starts an export Job,
// polls it until Job.IsTerminal, then streams the download to a path, stdout,
// or the default nexorious_export_<id>.<ext>. Export covers the whole library;
// the server has no filter, so neither has the command.
//
// # backup
//
// backup wraps the authenticated admin /api/admin/backups/* endpoints; the
// unauthenticated pre-bootstrap restore (/api/auth/setup/*) is the setup
// group. `create` is synchronous; `create --download [--out]` then downloads
// the new backup through the same saveBackup helper as `download`, and status
// lines go to stderr whenever stdout carries the archive, an id or JSON.
// `download` mirrors export's --out/-/default (backup-<id>.tar.gz). `restore`
// takes exactly one of <id> or --file (a multipart upload) behind a loud
// destructive confirmation. `schedule set` GETs the current BackupConfig and
// overlays only the flags the user changed (--frequency, --time, --day,
// --retention-mode, --retention-value), so it always PUTs a full valid struct.
//
// # admin
//
// admin (/api/auth/admin/*) needs an admin user's key. API keys carry no admin
// scope, so a non-admin key gets a 403, shown as the server sent it; nexctl
// has no admin gate of its own. `user rm` prints the deletion impact before
// confirming, and `admin reset` has the loudest confirmation. The server
// refuses self-deactivation, self-demotion and self-deletion (400); nexctl
// shows the refusal and never checks for it first.
//
// # config
//
// `config get/set` is user settings (--deal-region, validated by the server,
// whose 422 is shown). `config notify` nests channels, sub (list/set/reset
// subscriptions), test-url and events. Shoutrrr URLs are secrets: they are
// prompted without echo, never positional, and the API never returns them, so
// nexctl never prints them.
//
// # mcp
//
// mcp bridges stdio-only MCP clients to the server's POST /api/mcp (#1262).
// `mcp serve` is a plain JSON-RPC pump: it relays each stdin line to /api/mcp
// with the profile's key through cliclient.PostMCP and writes each reply as one
// stdout line. It reads nothing of the protocol but the id, so a server
// protocol upgrade never needs a nexctl upgrade. HTTP failures become JSON-RPC
// errors carrying the original id; a 404 or 405 means the server has no MCP
// endpoint (MCP_ENABLED=false, or too old). `mcp config` prints a stdio stanza
// and a native HTTP stanza (URL and Authorization header), with a stderr
// warning that the output contains the key.
//
// # doctor
//
// doctor is the front-end over the library-smells API (/api/library/smells,
// engine in internal/librarysmells). By default it prints a summary table
// (id, check, tier, fixable, count); --check <id> lists the flagged items;
// `apply <check> [refs…]` fixes an auto-fixable check (no refs means every
// flagged item; it confirms unless -y, and the server answers a check that is
// not fixable with 422); ignore/restore/ignored <check> manage per-item
// dismissals. Refs resolve through resolveUserGameRef. Whether a check is
// fixable is read from the API's auto_fixable, never hardcoded.
package main
