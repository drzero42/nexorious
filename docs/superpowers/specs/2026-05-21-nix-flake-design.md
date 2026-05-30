# Nix Flake — Package Build & NixOS Module Design

**Issue**: [#515](https://github.com/drzero42/nexorious/issues/515)
**Date**: 2026-05-21

## Summary

Add a `flake.nix` at the repository root that exposes:

1. **`packages.${system}.default`** — the `nexorious` binary (Go + embedded React SPA)
2. **`nixosModules.default`** — a NixOS module (`services.nexorious`) for running Nexorious as a systemd service with optional local PostgreSQL provisioning

The existing `devenv.nix` / `devenv.yaml` / `devenv.lock` development environment is untouched.

---

## File Layout

```
flake.nix          ← flake root: inputs, outputs, system matrix
nix/
  frontend.nix     ← buildNpmPackage: React SPA → dist/ contents
  package.nix      ← buildGoModule: Go binary with embedded frontend
  module.nix       ← NixOS module (services.nexorious)
```

---

## Flake Inputs

The flake uses a separate nixpkgs pin from devenv, targeting `nixos-unstable`. It does not reuse devenv's `cachix/devenv-nixpkgs` pin, since that pin is devenv-specific.

```nix
inputs = {
  nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
};
```

The flake supports a matrix of systems: `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`, `aarch64-darwin`. The NixOS module is system-independent (no `${system}` in its output path).

---

## Package Build (Approach A: two-derivation pipeline)

### Why two derivations

`ui/ui.go` embeds the React SPA via `//go:embed all:frontend/dist`. The Go build must see real built assets at `ui/frontend/dist/` — but this directory is gitignored (only `.gitkeep` is tracked). A two-derivation pipeline solves this:

1. Build the frontend independently → clean, cacheable npm derivation
2. Copy its output into place in a `preBuild` hook → Go sees the assets

This mirrors the standard nixpkgs pattern used by Gitea and Forgejo.

### `nix/frontend.nix`

Uses `buildNpmPackage` (compatible with lockfileVersion 3 / npm 10+).

- **src**: `ui/frontend/` subtree of the flake source
- **npmDepsHash**: SHA-256 hash of the npm dependency tree, computed from `package-lock.json`
- **buildPhase**: `npm run build` (runs `vite build && tsc --noEmit`)
- **installPhase**: copies `dist/` contents to `$out`
- **$out** is the dist directory itself (contains `index.html`, `assets/`, `logos/`, etc.)

`npmDepsHash` must be updated whenever `ui/frontend/package-lock.json` changes. Regenerate with:

```bash
nix run nixpkgs#prefetch-npm-deps ui/frontend/package-lock.json
```

### `nix/package.nix`

Uses `buildGoModule`.

- **src**: `self` (the flake source — git-tracked files only, no built artifacts)
- **vendorHash**: SHA-256 hash of the Go module graph (from `go.sum`)
- **preBuild**: copies `${nexorious-frontend}/*` into `ui/frontend/dist/` so the embed directive finds real assets
- **ldflags**: injects `-X main.version` (from `self.rev` or a version arg) and `-X main.commit`
- **CGO_ENABLED**: `0` (pure Go, matches Makefile and Dockerfile)
- **subPackages**: `["cmd/nexorious"]`

**Runtime dependencies** (wrapped into the binary's `PATH`):

| Package | Purpose |
|---|---|
| `legendary-gl` | GOG sync — needed to invoke `legendary` |
| `postgresql_18` | Provides `psql` and `pg_dump` for backup orchestration (mirrors `postgresql18-client` in the Dockerfile) |

`vendorHash` must be updated whenever `go.mod` / `go.sum` changes. Regenerate by setting it to `pkgs.lib.fakeHash`, running `nix build`, and copying the correct hash from the error output.

---

## NixOS Module (`nix/module.nix`)

### Documentation requirements

The module file opens with a comment block containing a minimal working example:

```nix
# Example — add to your NixOS configuration:
#
#   inputs.nexorious.url = "github:drzero42/nexorious";
#
#   { inputs, ... }: {
#     imports = [ inputs.nexorious.nixosModules.default ];
#
#     services.nexorious = {
#       enable = true;
#       environmentFile = "/run/secrets/nexorious.env";
#     };
#   }
#
# The environment file must contain at minimum:
#   SECRET_KEY=<random-secret-used-for-JWT-signing-and-encryption>
#
# Optional additions for IGDB metadata enrichment:
#   IGDB_CLIENT_ID=...
#   IGDB_CLIENT_SECRET=...
#
# When database.createLocally = true (the default), PostgreSQL is managed
# automatically and no database credentials are needed in the environment file.
```

Every option has a `description` string. Non-obvious options include an `example` value.

### Options

| Option | Type | Default | Description |
|---|---|---|---|
| `enable` | `bool` | `false` | Enable the Nexorious service |
| `package` | `package` | `pkgs.nexorious` | The Nexorious package to use |
| `port` | `port` | `8000` | TCP port to listen on |
| `database.createLocally` | `bool` | `true` | Create and manage a local PostgreSQL database. When `true`, `services.postgresql` is enabled, the `nexorious` DB user and database are provisioned, and `DATABASE_URL` is set to the Unix socket path (peer auth — no password needed in `environmentFile`). Set to `false` and supply credentials via `environmentFile` to use an external database. |
| `database.host` | `str` | `"localhost"` | Database host — used only when `createLocally = false` |
| `database.port` | `int` | `5432` | Database port — used only when `createLocally = false` |
| `database.name` | `str` | `"nexorious"` | Database name |
| `database.user` | `str` | `"nexorious"` | Database user |
| `storagePath` | `str` | `/var/lib/nexorious/storage` | Directory for uploads and stored data |
| `backupPath` | `str` | `/var/lib/nexorious/storage/backups` | Directory for database backups |
| `workerCount` | `int` | `4` | Number of River background workers |
| `logLevel` | `enum ["debug" "info" "warn" "error"]` | `"info"` | Log verbosity |
| `environmentFile` | `nullOr path` | `null` | Path to a file of `KEY=value` lines loaded by the systemd service. Must contain `SECRET_KEY`. May also contain `IGDB_CLIENT_ID`, `IGDB_CLIENT_SECRET`, and other sensitive vars. Compatible with sops-nix, agenix, and plain files. |
| `settings` | `attrsOf str` | `{}` | Escape hatch: additional environment variables merged into the service environment |

### Systemd service

- **User/group**: dedicated `nexorious` system user and group, created by the module
- **StateDirectory**: `"nexorious"` — systemd owns `/var/lib/nexorious`; `storagePath` and `backupPath` default to subdirectories of this
- **ExecStart**: `${cfg.package}/bin/nexorious serve`
- **EnvironmentFile**: set to `cfg.environmentFile` when non-null
- **Environment**: all non-secret settings translated to env vars (`PORT`, `LOG_LEVEL`, `STORAGE_PATH`, `BACKUP_PATH`, `WORKER_COUNT`, `LEGENDARY_WORK_DIR=/var/lib/nexorious/legendary`, plus `DATABASE_URL` when `createLocally = true`)
- **After / Requires**: `postgresql.service` when `database.createLocally = true`
- **Restart**: `on-failure`
- **LEGENDARY_WORK_DIR**: set to `/var/lib/nexorious/legendary` (a subdirectory of the `StateDirectory`) — not exposed as a user-facing option; always set internally so GOG sync has a writable, persistent home

### Local PostgreSQL wiring

When `database.createLocally = true`:

```nix
services.postgresql = {
  enable = true;
  package = pkgs.postgresql_18;
  ensureUsers = [{ name = "nexorious"; ensureDBOwnership = true; }];
  ensureDatabases = [ "nexorious" ];
};
```

`DATABASE_URL` is set to:

```
postgresql://nexorious@/nexorious?host=/run/postgresql&sslmode=disable
```

This uses the PostgreSQL Unix socket with peer authentication — no password is required or stored.

---

## Hash Update Workflow

Both hashes are intentionally opaque strings that must be updated alongside their respective lock files. The expected flow:

**npm deps changed** (`package-lock.json` updated):
```bash
nix run nixpkgs#prefetch-npm-deps ui/frontend/package-lock.json
# copy hash into nix/frontend.nix → npmDepsHash
```

**Go deps changed** (`go.mod` / `go.sum` updated):
Set `vendorHash = pkgs.lib.fakeHash;` in `nix/package.nix`, run `nix build`, copy the correct hash from the error output.

These steps are documented in a comment at the top of each respective Nix file.

---

## Out of Scope

- Publishing the package to nixpkgs (separate community contribution)
- A NixOS integration test (`nixosTest`) — useful but deferred
- Replacing or migrating `devenv.nix` to use the new flake
