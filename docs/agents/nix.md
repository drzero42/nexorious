# Nix flake maintenance

The `nix/` directory contains the Nix package and NixOS module.

## Hashes

npm dependencies need no hash: `nix/frontend.nix` uses `importNpmLock`, which fetches each package from the `resolved` URL + `integrity` already in `ui/frontend/package-lock.json`.

One hash must be kept in sync with its lock file: **`goVendorHash` in `flake.nix`**. Update it after any `go.mod` / `go.sum` change:

```bash
./nix/update-go-vendor-hash.sh   # builds only .#nexctl.goModules and rewrites the line
```

Server (`nix/package.nix`) and client (`nix/nexctl.nix`) build from the same `go.mod`/`go.sum`, and `buildGoModule`'s `vendorHash` is independent of `subPackages`, so the two are always identical. They share **one** value — `goVendorHash` in `flake.nix`, passed to both via `callPackage`.

Renovate runs the same script on Go dependency PRs ([renovate.md](renovate.md)). The `Nix Build` workflow (`nix.yaml`) builds both `.#nexorious` and `.#nexctl` and **only verifies**: a stale hash fails it, and a hand-made `go.mod`/`go.sum` change needs the script run and `flake.nix` committed.

## Version and packages

The `version` field in `flake.nix` is managed by release-please (same as `Chart.yaml`). The flake exposes two packages, `nexorious` (server) and `nexctl` (CLI client); both are also in `overlays.default`. The `nexorious` NixOS module (`nix/module.nix`) installs only the server — `nexctl` is opt-in (add `packages.nexctl` to `environment.systemPackages`).
