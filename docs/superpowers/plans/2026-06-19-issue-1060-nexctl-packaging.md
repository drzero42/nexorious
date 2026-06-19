# `nexctl` Phase 7 — Packaging Implementation Plan (Epic #1060)

Design: [../specs/2026-06-19-issue-1060-nexctl-packaging-design.md](../specs/2026-06-19-issue-1060-nexctl-packaging-design.md)

One PR, whole phase. Infra change (nfpm/nix/workflow/shell) — no Go logic. Implemented by the controller directly (small, exhaustively specced), then independent review + whole-branch review; merge on explicit user instruction.

## Tasks

- **P1 — nfpm config + smoke script.** New `deploy/packaging/nfpm-nexctl.yaml` (minimal client package: binary → `/usr/bin/nexctl`, no systemd/env/data-dir/scripts/deps). New `deploy/packaging/smoke-test-nexctl.sh` (install → assert `/usr/bin/nexctl` + `nexctl version` → assert no `nexorious` user/unit → reinstall idempotency). `shellcheck` clean; `yq` validates the nfpm YAML.
- **P2 — release-artifacts.yaml wiring.** In `build`: add nexctl binary loop (`ci-binaries/nexctl-linux-$arch` + `dist/nexctl_${PKG_VERSION}_linux_$arch`), nexctl package loop (`nfpm -f nfpm-nexctl.yaml`), widen checksums to `nexorious* nexctl*`, add `dist/nexctl*` to attestation subject-path. Add `smoke-test-nexctl` job (4-slot matrix, `needs: build`). `upload` gains `needs: [smoke-test, smoke-test-nexctl]`. `image` unchanged. `yq` validates structure; depends on P1 file paths.
- **P3 — nix.** New `nix/nexctl.nix` (buildGoModule, `subPackages = ["cmd/nexctl"]`, reuse `vendorHash`, no frontend, no PATH wrap). `flake.nix`: add `packages.nexctl`, extend `overlays.default` with `nexctl`; `default` stays `nexorious`; **`nix/module.nix` untouched**. Verify `nix build .#nexctl` and `nix flake check`.
- **T-docs — finalize.** Commit design + plan. Update `CLAUDE.md` Release Process section (note `nexctl` ships as a separate binary/.deb/.rpm/nix package; nix flake exposes both, module installs only the server). Whole-branch review.

## Verification gates (per task + final)

`go build ./cmd/nexctl`; `nix build .#nexctl` + `nix flake check`; `shellcheck deploy/packaging/smoke-test-nexctl.sh`; `yq` lint of `release-artifacts.yaml` + `nfpm-nexctl.yaml`. Final proof is CI PR-mode actually building + smoke-testing the nexctl packages on the PR.
