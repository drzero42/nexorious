# `nexctl` Phase 7 — Packaging Design (Epic #1060)

**Status:** Phase 7 of the `nexctl` CLI epic (#1060). Phases 1–6 are merged (#1081, #1083, #1085, #1086, #1087, #1088, #1090). This phase ships `nexctl` through the existing release pipeline. Phase 8 (`mcp`, blocked on #518) remains.

## Problem

`nexctl` builds from `cmd/nexctl/` but is not produced by any release artifact. Users who want only the client must `go build` it themselves. The master CLI design (`2026-06-18-issue-1060-nexctl-cli-design.md`, §Packaging) calls for distributing `nexctl` **the same way as `nexorious`**: raw cross-platform binary, `.deb`, `.rpm`, and a nix package. Container image and Helm chart stay server-only.

## Scope decisions (settled with the user)

- **Separate `nexctl` package.** `nexctl` ships as its own `.deb`/`.rpm`/nix package — it is **not** bundled into the `nexorious` server package.
- **One PR for the whole phase.**
- **The flake exposes both** `packages.nexorious` and `packages.nexctl` (and both via `overlays.default`).
- **The `services.nexorious` NixOS module must not install `nexctl`.** The client stays opt-in: `nix/module.nix` is untouched and never references `nexctl`. Users who want the client add `packages.nexctl` to their own `environment.systemPackages`.

## What this mirrors (authority: `.github/workflows/release-artifacts.yaml`)

Both binaries share one repo version (release-please `packages: { "." }`; the `release-branch` job pins `nix/release-version.txt`). `nexctl`'s `cmd/nexctl/main.go` already declares `var version`/`var commit`, so the same `-ldflags "-s -w -X main.version=… -X main.commit=…"` injection works. amd64 + arm64, `CGO_ENABLED=0`, `-trimpath`, nfpm v2.46.3 — all identical to `nexorious`.

## Design

### 1. Binary + package build (`build` job, `release-artifacts.yaml`)

Add, alongside the existing `nexorious` steps:

- **Binaries:** for `arch in amd64 arm64`, `go build … -o ci-binaries/nexctl-linux-$arch ./cmd/nexctl`, then `cp` to `dist/nexctl_${PKG_VERSION}_linux_$arch` (raw release asset). The `ci-binaries/` copy is what nfpm consumes (parity with `nexorious`; no container uses it).
- **Packages:** `NFPM_ARCH=$arch nfpm package -f deploy/packaging/nfpm-nexctl.yaml -p deb -t dist/` and `-p rpm`, for both arches.
- **Checksums:** widen to `sha256sum nexorious* nexctl* > sha256sums.txt` (one combined file in `dist/`).
- **Build-provenance attestation:** add `dist/nexctl*` to the `subject-path` glob (per-artifact signing is genuinely needed).
- **SBOM:** **no separate `nexctl` SBOM.** The existing source SBOM documents the repository's Go dependency set, which is identical for both binaries; duplicating it under a `nexctl` name would falsely imply a distinct dependency set. The repo-wide source SBOM (`nexorious_<ver>_sbom.source.spdx.json`) covers `nexctl` too — noted here so the omission is intentional, not forgotten.
- **`upload` job:** already runs `gh release upload "$TAG" dist/* --clobber`, so `nexctl` assets ride along with no change.

### 2. nfpm config (`deploy/packaging/nfpm-nexctl.yaml`, new)

A minimal client package — no systemd unit, env conffile, data dir, maintainer scripts, or `postgresql` dependency (those are all server concerns):

```yaml
name: nexctl
arch: ${NFPM_ARCH}
platform: linux
version: ${NFPM_VERSION}
maintainer: Anders Bøgh Bruun <anders@boghbruun.dk>
description: Nexorious CLI client.
homepage: https://github.com/drzero42/nexorious
license: MIT
contents:
  - src: ci-binaries/nexctl-linux-${NFPM_ARCH}
    dst: /usr/bin/nexctl
    expand: true            # required for ${NFPM_ARCH} expansion in src
    file_info:
      mode: 0755
```

Default filenames: `nexctl_${ver}_amd64.deb` / `nexctl_${ver}_arm64.deb`, `nexctl-${ver}.x86_64.rpm` / `nexctl-${ver}.aarch64.rpm` — caught by the `nexctl*` checksum/attestation/upload globs.

### 3. Smoke test (`deploy/packaging/smoke-test-nexctl.sh` + `smoke-test-nexctl` job)

The server smoke test (`smoke-test.sh`) asserts systemd/user/conffile/data-dir — none of which `nexctl` has. A new minimal script `smoke-test-nexctl.sh <deb|rpm> <path>`:

1. Install the package (apt/dnf), matching the server script's install logic.
2. Assert `/usr/bin/nexctl` exists and is executable.
3. Assert `nexctl version` runs and prints the injected version.
4. Assert the package created **no** `nexorious` system user and **no** systemd unit (negative check — confirms the client package stays minimal and didn't accidentally inherit server content).
5. Reinstall the same package and re-assert `nexctl version` (trivial idempotency; no conffile to preserve).

A new `smoke-test-nexctl` job mirrors the existing 4-slot matrix (`debian:13` + `rockylinux/rockylinux:10` × amd64/arm64), `needs: build`. The `upload` job gains `needs: [smoke-test, smoke-test-nexctl]` so `nexctl` packages are validated before any release upload. The server-only `image` job keeps `needs: smoke-test` (unchanged).

### 4. nix (`nix/nexctl.nix`, new; wired in `flake.nix`)

```nix
{ buildGoModule, lib, src, version, commit }:
buildGoModule {
  pname = "nexctl";
  inherit version src;
  vendorHash = "<same as nix/package.nix — shared go.mod/go.sum>";
  subPackages = [ "cmd/nexctl" ];
  doCheck = false;            # tests need Docker (testcontainers); covered by CI
  ldflags = [ "-s" "-w" "-X main.version=${version}" "-X main.commit=${commit}" ];
  preBuild = "export CGO_ENABLED=0";
  meta = {
    description = "Nexorious CLI client";
    homepage = "https://github.com/drzero42/nexorious";
    license = lib.licenses.mit;
    mainProgram = "nexctl";
  };
}
```

Differs from `nix/package.nix`: **no** `nexorious-frontend` dependency, **no** frontend copy in `preBuild`, **no** `postInstall` PATH wrap (the client shells out to nothing). `vendorHash` is identical because both binaries vendor from the same `go.mod`/`go.sum` (do not invent a new hash — reuse the value verbatim; `nix build .#nexctl` confirms it).

`flake.nix`: add to the per-system `packages` set
```nix
nexctl = pkgs.callPackage ./nix/nexctl.nix {
  src = self; inherit version; commit = self.shortRev or "dirty";
};
```
`default = nexorious` stays. Extend `overlays.default` to expose `nexctl` as well (the user asked the flake to expose both). **`nix/module.nix` is not touched** — the NixOS service must not pull in the client.

## Out of scope

- Container image / Helm chart for `nexctl` (server-only, by design).
- Per-binary release versioning (release-please stays single-version `packages: { "." }`).
- Homebrew / scoop / Windows packaging (matches `nexorious`).
- Phase 8 `mcp` (blocked on #518).

## Verification

- Local: `go build ./cmd/nexctl`, `nix build .#nexctl` (validates `nexctl.nix` + vendorHash), `yq` structural lint of the workflow + nfpm config, `shellcheck deploy/packaging/smoke-test-nexctl.sh`.
- CI: editing `release-artifacts.yaml` + `deploy/packaging/**` triggers the workflow in **PR mode**, which actually builds the `nexctl` binary, produces the `.deb`/`.rpm`, and runs `smoke-test-nexctl` on all four matrix slots before merge.
