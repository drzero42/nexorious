# Native `.deb`/`.rpm` packages and a build-once-per-arch release pipeline

**Issue:** [#901](https://github.com/drzero42/nexorious/issues/901)
**Date:** 2026-06-10
**Status:** Approved design

## Summary

Produce native `.deb` and `.rpm` packages for releases, and restructure the
release build so the **same per-arch Go binary** is reused across the raw
release asset, the `.deb`, the `.rpm`, and the container image ("build once per
arch, package many"). Target architectures: **`amd64` + `arm64`**.

As part of this work the CI build topology is **simplified to release-only**:
the nightly/dev container-image and dev-chart machinery is removed entirely.
The project is small enough that artifacts only need to be produced when a
release is published.

## Motivation

- Self-hosters on Debian/Ubuntu and RHEL/Fedora/Rocky get a native install
  (`apt install` / `dnf install`) with a systemd service, dedicated user, and
  managed config — instead of hand-wiring a raw binary.
- Building the binary once and reusing it everywhere removes drift between
  artifacts and is the natural path to **multi-arch container images** (today's
  image is amd64-only).
- Dropping the nightly/dev build flow removes standing CI cost and a large
  amount of pruning/pre-check machinery that existed only to manage the churn
  of per-commit artifacts.

## Current state (verified)

- `Dockerfile` is a 3-stage build: frontend (node) → go → minimal alpine
  runtime. The runtime installs `ca-certificates`, `postgresql18-client`,
  `python3`/`py3-requests`/`py3-filelock`, pip-installs `legendary-gl`, and runs
  as a non-root `nexorious` user (uid/gid 10001). The binary is **not**
  self-contained at runtime — it shells out to `psql`/`pg_dump` and (for Epic
  sync) `legendary`. Build flags: `-trimpath -ldflags "-s -w -X main.version -X
  main.commit"`.
- `build-release-binaries.yaml` builds amd64+arm64 raw binaries + `sha256sums.txt`
  on `release: published` and uploads them. Build flags: `-ldflags "-X
  main.version -X main.commit"` (no `-trimpath`, no `-s -w` — **differs** from
  the Dockerfile).
- `build-push.yaml` builds/pushes the container image (amd64-only) + Helm chart
  on release/nightly/dispatch, prunes old dev images/charts, and advances the
  release branch. Contains: a `pre-check` job (skip-redundant-nightly), the
  image build, the chart push (release semver + moving `0.0.0-dev` dev tag), the
  release-branch advance, and two prune/cleanup jobs.
- App config is entirely **environment-variable based**
  (`internal/config/config.go`): `DATABASE_URL`, required `DB_ENCRYPTION_KEY`,
  `STORAGE_PATH` (default `./storage`), `BACKUP_PATH` (`./storage/backups`),
  `PORT` (8000), `LOG_LEVEL` (info), `WORKER_COUNT` (4), `LEGENDARY_WORK_DIR`, etc.
- `nexorious version` is a real subcommand (`cmd/nexorious/version.go`).
- Nothing user-facing references the dev/nightly artifacts: `docker-compose.yml`
  pins the release-please semver, the Helm docs use release charts, and the
  NixOS "bleeding edge" path builds from source via the flake. The only
  `dev`-tag references are in historical plan files (left as-is) and
  `CLAUDE.md`'s release-process section (updated).

## Design

### Unified build flags

All artifacts (raw binary, `.deb`, `.rpm`, container image) are produced from
the **same per-arch binary** built with:

```
CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build -trimpath \
  -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
  -o nexorious_<ver>_linux_<arch> ./cmd/nexorious
```

This adopts the Dockerfile's existing flags everywhere. Consequence: the raw
release binaries become **stripped** (`-s -w`). Go's `pclntab` is never
stripped, so panic stack traces still carry full function names and file:line —
only debugger-attach on the shipped binary is lost, which is immaterial for
release artifacts. `-trimpath` removes build-machine paths (reproducibility).

### Build topology: one release-only workflow

`build-push.yaml` is **deleted wholesale**, removing the nightly schedule, the
`workflow_dispatch` trigger, the `pre-check` job, the dev image tags (`dev`,
`main-YYYYMMDD-<sha>`), the dev chart tags (`0.0.0-dev`,
`0.0.0-dev-YYYYMMDD-<sha>`), and both prune/cleanup jobs. Its two release-time
duties (release chart push, release-branch advance) move into the new workflow.

`build-release-binaries.yaml` is replaced by `release-artifacts.yaml`, the only
build workflow:

```yaml
on:
  release:
    types: [published]
  pull_request:
    paths:
      - 'deploy/packaging/**'
      - 'Dockerfile'
      - '.github/workflows/release-artifacts.yaml'

jobs:
  build:            # ubuntu-latest
    # checkout (tag for release; PR head otherwise)
    # VERSION = tag minus 'v', or a dev version string for PRs
    # frontend built ONCE (npm install && npm run build)
    # per arch (amd64, arm64): go build with the unified flags above
    # nfpm ×4 → .deb/.rpm (deb amd64/arm64, rpm x86_64/aarch64)
    # sha256sums.txt over ALL artifacts (binaries + .deb + .rpm)
    # upload-artifact: dist/ + ci-binaries/nexorious-linux-{amd64,arm64}

  smoke-test:
    needs: build
    strategy:
      matrix:
        include:
          - { runner: ubuntu-latest,    image: debian:13,    pkg: deb, arch: amd64 }
          - { runner: ubuntu-latest,    image: rockylinux:9, pkg: rpm, arch: amd64 }
          - { runner: ubuntu-24.04-arm, image: debian:13,    pkg: deb, arch: arm64 }
          - { runner: ubuntu-24.04-arm, image: rockylinux:9, pkg: rpm, arch: arm64 }
    # install package → assert:
    #   `nexorious version` runs; `systemd-analyze verify` on the unit passes;
    #   nexorious user + /var/lib/nexorious exist; env conffile placed;
    #   DB_ENCRYPTION_KEY was generated (non-empty);
    #   then edit env + key, reinstall same package → file AND key preserved.

  image:
    needs: smoke-test
    # PRs: buildx --target runtime-ci, linux/amd64 only, NO push
    #      (validates the Dockerfile between releases, since the nightly
    #       build that used to do this no longer exists)
    # release: setup-qemu + buildx --platform linux/amd64,linux/arm64
    #          --target runtime-ci --build-context binaries=./ci-binaries
    #          --tag ghcr.io/drzero42/nexorious:X.Y.Z --tag …:latest --push

  upload:           # release only
    needs: smoke-test
    if: github.event_name == 'release'
    # gh release upload: binaries, .deb, .rpm, sha256sums.txt

  chart:            # release only — moved from build-push.yaml
    needs: image
    if: github.event_name == 'release'
    # helm dependency update / package --version X.Y.Z --app-version X.Y.Z
    # / push to oci://ghcr.io/drzero42/charts  (no moving 0.0.0-dev tag)

  release-branch:   # release only — moved from build-push.yaml
    needs: [upload, image]
    if: github.event_name == 'release'
    # fast-forward release branch + pin nix/release-version.txt
    # (logic unchanged; RELEASE_PLEASE_TOKEN)
```

Properties:

- **Build once per arch.** The binaries staged as
  `ci-binaries/nexorious-linux-{amd64,arm64}` feed the packages and the image;
  the same files are uploaded as raw assets. Identical bytes guaranteed.
- **Smoke tests gate everything.** Neither the release assets nor the image are
  published unless all four install tests pass. The repo is public, so free
  `ubuntu-24.04-arm` runners smoke-test arm64 packages natively; QEMU is needed
  only for the arm64 `apk add` layer of the image build (the Go compile is never
  emulated — binaries are prebuilt).
- **Image→chart ordering restored.** `chart` `needs: image` within one workflow
  (the previous cross-workflow draft had a publish race; this removes it).
- **Release branch advances only after artifacts exist** (`needs: [upload,
  image]`).
- Between releases the only CI exercising the Dockerfile is the PR-mode no-push
  image build (triggered on packaging-relevant path changes). The `runtime`
  source-build target is exercised by `make docker` locally and mirrors what
  `test.yaml` already compiles natively on every PR, so no full source-build
  image job is added to PR CI.

### One-time registry cleanup

A one-time scripted pass (`gh api` over
`/users/drzero42/packages/container/<name>/versions`) deletes every ghcr package
version that does not carry a release tag:

- `nexorious` image: keep only `X.Y.Z`-tagged versions (`latest` rides the
  newest of those); delete `dev`, `main-YYYYMMDD-<sha>`, legacy
  `YYYYMMDD-<sha>`, and untagged versions.
- `charts/nexorious`: keep only `X.Y.Z`; delete `0.0.0-dev` and
  `0.0.0-dev-YYYYMMDD-<sha>`.

Run **once during implementation, before the first multi-arch release**, with
the maintainer's `gh` auth, showing the kept/deleted lists before deleting. It
is deliberately **not** a recurring workflow: after the first multi-arch
release the image package contains *untagged* per-platform manifests referenced
by the multi-arch index, so a recurring "delete untagged" job would corrupt
released images. With the nightly flow gone, nothing non-release accumulates
again, so a one-time sweep is sufficient.

Rollout order: merge the PR (workflows replaced) → run the cleanup → next
release publishes the first multi-arch image into a clean registry.

### Dockerfile: one file, shared runtime layer, two final targets

Single root `Dockerfile`. The runtime setup is defined exactly once in a
`runtime-base` stage so the shipped image and the locally-built image cannot
drift.

```dockerfile
# syntax=docker/dockerfile:1.24

# Stage 1: build the React SPA (unchanged)
FROM docker.io/library/node:24-alpine AS frontend-build
WORKDIR /src
COPY ui/frontend/package.json ui/frontend/package-lock.json ./ui/frontend/
RUN cd ui/frontend && npm ci
COPY ui/frontend ./ui/frontend
RUN cd ui/frontend && npm run build && touch dist/.gitkeep

# Stage 2: build the Go binary (unchanged)
FROM docker.io/library/golang:1.26-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-build /src/ui/frontend/dist ./ui/frontend/dist
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/nexorious ./cmd/nexorious

# Shared runtime layer — defined exactly once
FROM docker.io/library/alpine:3.23 AS runtime-base
RUN apk add --no-cache \
      ca-certificates \
      postgresql18-client \
      python3 py3-requests py3-filelock \
 && apk add --no-cache --virtual .pip-tmp py3-pip \
 && pip install --no-cache-dir --break-system-packages --no-deps legendary-gl==0.20.34 \
 && apk del .pip-tmp \
 && addgroup -g 10001 -S nexorious \
 && adduser -u 10001 -S -G nexorious -h /app -s /sbin/nologin nexorious
WORKDIR /app
RUN mkdir -p /app/storage/backups && chown -R nexorious:nexorious /app
USER nexorious
EXPOSE 8000
ENTRYPOINT ["/app/nexorious"]
CMD ["serve"]

# Target: CI release image (prebuilt binary via buildx named context)
FROM runtime-base AS runtime-ci
ARG TARGETARCH
COPY --from=binaries --chown=nexorious:nexorious nexorious-linux-${TARGETARCH} /app/nexorious

# Target: full source build (LAST stage = default target)
FROM runtime-base AS runtime
COPY --from=go-build --chown=nexorious:nexorious /out/nexorious /app/nexorious
```

- `runtime` stays the **last** stage, so a plain `docker build .` still does a
  full source build. `make docker` gains an explicit `--target runtime`.
- `runtime-ci` consumes a buildx **named build context** (`--build-context
  binaries=./ci-binaries`) that only CI supplies; `TARGETARCH` selects the
  right prebuilt binary per platform, so one buildx invocation produces the
  multi-arch manifest. buildx only builds the target stage's dependency graph,
  so `frontend-build`/`go-build` are skipped for `runtime-ci`.

### Native packages (`deploy/packaging/`)

`deploy/` already houses per-method delivery artifacts (`deploy/docker/`,
`deploy/helm/`); packaging slots in as a sibling.

| Path | Purpose |
|---|---|
| `deploy/packaging/nfpm.yaml` | Single nfpm config for both `.deb`/`.rpm`; version/arch via env interpolation. |
| `deploy/packaging/nexorious.service` | systemd unit. |
| `deploy/packaging/nexorious.env` | env-file template → `/etc/nexorious/nexorious.env` conffile. |
| `deploy/packaging/scripts/preinstall.sh` | create user/group before files land. |
| `deploy/packaging/scripts/postinstall.sh` | generate key if absent, `daemon-reload`, print next steps / `try-restart`. |
| `deploy/packaging/scripts/preremove.sh` | stop/disable on final removal. |
| `deploy/packaging/scripts/postremove.sh` | purge cleanup; `daemon-reload`. |

#### `nfpm.yaml`

```yaml
name: nexorious
arch: ${NFPM_ARCH}            # amd64 | arm64 — nfpm maps to x86_64/aarch64 for rpm
platform: linux
version: ${NFPM_VERSION}
maintainer: Anders Bøgh Bruun <anders@boghbruun.dk>
description: Self-hosted game library manager.
homepage: https://github.com/drzero42/nexorious
license: MIT

overrides:
  deb:
    depends: [postgresql-client]     # Debian/Ubuntu meta-package → psql/pg_dump
  rpm:
    depends: [postgresql]            # RHEL/Fedora/Rocky client package → psql/pg_dump

contents:
  - src: ci-binaries/nexorious-linux-${NFPM_ARCH}
    dst: /usr/bin/nexorious
    file_info: { mode: 0755 }
  - src: deploy/packaging/nexorious.service
    dst: /usr/lib/systemd/system/nexorious.service
    file_info: { mode: 0644 }
  - src: deploy/packaging/nexorious.env
    dst: /etc/nexorious/nexorious.env
    type: config|noreplace           # deb conffile / rpm %config(noreplace)
    file_info: { mode: 0640, owner: root, group: nexorious }
  - dst: /var/lib/nexorious
    type: dir
    file_info: { mode: 0750, owner: nexorious, group: nexorious }

scripts:
  preinstall:  deploy/packaging/scripts/preinstall.sh
  postinstall: deploy/packaging/scripts/postinstall.sh
  preremove:   deploy/packaging/scripts/preremove.sh
  postremove:  deploy/packaging/scripts/postremove.sh
```

Package filenames follow each ecosystem's nfpm defaults (e.g.
`nexorious_X.Y.Z_amd64.deb`, `nexorious-X.Y.Z.x86_64.rpm`). `legendary` is **not**
a package dependency — it is a pip tool with no distro package, left as an
admin-managed install (Epic sync degrades gracefully without it).

#### `nexorious.service`

```ini
[Unit]
Description=Nexorious game library server
Documentation=https://github.com/drzero42/nexorious
After=network-online.target
Wants=network-online.target

[Service]
User=nexorious
Group=nexorious
EnvironmentFile=/etc/nexorious/nexorious.env
ExecStart=/usr/bin/nexorious serve
WorkingDirectory=/var/lib/nexorious
StateDirectory=nexorious
Restart=on-failure
RestartSec=5

# Hardening
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

`StateDirectory=nexorious` makes systemd guarantee `/var/lib/nexorious` exists
with correct ownership at each start. `ProtectSystem=full` (not `strict`)
because the app shells out to `psql`/`pg_dump`/`legendary`; tightening to
`strict` + `ReadWritePaths` is a non-breaking follow-up.

#### `nexorious.env` (conffile template)

```bash
# Nexorious configuration — read by the systemd service.
# After editing: systemctl restart nexorious

# REQUIRED: PostgreSQL connection string. Until set, Nexorious starts but
# idles on its "database unavailable" page.
# Example: postgres://nexorious:secret@localhost:5432/nexorious
DATABASE_URL=

# Auto-generated on first install. DO NOT change or lose this key —
# stored storefront credentials cannot be decrypted without it.
DB_ENCRYPTION_KEY=

STORAGE_PATH=/var/lib/nexorious
BACKUP_PATH=/var/lib/nexorious/backups

# Optional settings (defaults shown):
#PORT=8000
#LOG_LEVEL=info
#WORKER_COUNT=4
# IGDB metadata enrichment (https://api-docs.igdb.com):
#IGDB_CLIENT_ID=
#IGDB_CLIENT_SECRET=
```

#### Maintainer scripts

nfpm feeds the same script to both formats; deb passes words (`configure`,
`remove`, `purge`) and rpm passes counts (`0`/`1`/`2`). Each script cases on
`$1` to handle both.

- **preinstall** — create the `nexorious` system user/group if absent (`getent
  || useradd -r --home-dir /var/lib/nexorious --shell /usr/sbin/nologin`).
  Runs before files land so packaged ownership (conffile group, data dir)
  resolves on both dpkg and rpm.
- **postinstall** —
  1. If the `DB_ENCRYPTION_KEY=` line in the env file is empty, generate
     (`head -c32 /dev/urandom | base64`) and `sed` it in. Never touches a
     non-empty value → upgrade-safe by construction.
  2. `systemctl daemon-reload` (guarded: only if systemd is running).
  3. Fresh install: do **not** auto-enable/start; print next steps (edit the
     env file, then `systemctl enable --now nexorious`). One consistent
     behavior across both package families (matches RHEL convention); the app
     idles safely with a blank `DATABASE_URL` regardless.
  4. Upgrade: `systemctl try-restart nexorious` (restarts only if running).
- **preremove** — final removal (deb `remove` / rpm `0`): `systemctl stop` +
  `disable`. Upgrade: nothing.
- **postremove** —
  - deb `purge`: remove `/etc/nexorious`, delete `/var/lib/nexorious`
    **including backups**, remove the `nexorious` user and group,
    `daemon-reload`. (Full-purge semantics: a true clean uninstall.)
  - deb `remove` (no purge): `daemon-reload` only — config, key, data, user
    remain for reinstall.
  - rpm uninstall (`0`): `daemon-reload`; user and data stay (no purge concept;
    rpm preserves a modified conffile as `.rpmsave`).
  - any upgrade path: `daemon-reload` only.

### Dependencies

- **Hard dependency**: `postgresql-client` (deb) / `postgresql` (rpm) — provides
  `psql`/`pg_dump`.
- **`legendary`** (Epic sync) is not packaged — documented as an admin-managed
  install; Epic sync degrades gracefully when absent.

## Documentation (required deliverable)

**`docs/admin-guide.md`** (embedded and rendered in-app — every touched section
reviewed for coherence, not merely appended to):

- **`## Deploying Nexorious`** — new `### Native packages (Debian/Ubuntu,
  RHEL/Fedora/Rocky)` subsection: download `.deb`/`.rpm` from the Release,
  verify against `sha256sums.txt`, `apt install ./…deb` / `dnf install ./…rpm`,
  edit `/etc/nexorious/nexorious.env` (set `DATABASE_URL`), `systemctl enable
  --now nexorious`.
- **`### Single binary`** — reconciled with the package path: Debian/RHEL-family
  users pointed at the packages as the preferred native path; raw binary remains
  for other distros with the same runtime-dependency notes (`psql`/`pg_dump`
  required, `legendary` optional).
- **`## Configuration`** (*The essentials* and *Full reference*) — package
  installs keep config in `/etc/nexorious/nexorious.env`; the package presets
  `STORAGE_PATH`/`BACKUP_PATH` to `/var/lib/nexorious`; `DB_ENCRYPTION_KEY` is
  auto-generated on first install and must never be changed afterwards.
- **`## First run`** — package flow: the service idles on the
  database-unavailable page until `DATABASE_URL` is set; then the `/migrate`
  browser flow or `nexorious migrate` CLI.
- **`## Epic Games Store sync`** — `legendary` is not bundled in the packages;
  document the admin-managed install and graceful degradation.
- **`## Upgrades and versioning`** — `apt`/`dnf` upgrade preserves the env file
  and generated key (conffile / `%config(noreplace)`); service restarted if
  running. Note removal semantics (deb purge deletes data; plain remove and rpm
  uninstall do not).
- **`## Command-line tools`** and **`## Monitoring and operations`** —
  `systemctl`/`journalctl` notes for package installs (logs go to the journal).

**`README.md` / `DEV.md`** — install/release sections mention the packages
alongside existing methods.

**`CLAUDE.md`** — the Release Process section is updated: name
`release-artifacts.yaml` (replacing `build-push.yaml`) and the new artifact set;
note that nightly/dev builds no longer exist.

No new embedded docs: everything lands in the already-embedded admin guide, so
`docs/embed.go` and `ui/frontend/src/lib/doc-links.ts` are untouched.

## Testing / verification

No Go or frontend source changes — the test surface is the CI smoke-test matrix
(4 package installs + upgrade-preservation + `systemd-analyze verify` +
`nexorious version`) plus the PR-mode no-push image build. Existing Go/frontend
suites are unaffected.

## Scope amendments vs. issue #901

1. **Nightly/dev flow removed entirely** — `build-push.yaml` deleted; dev
   image/chart tags and prune jobs gone (issue said "unchanged"; the maintainer
   directed this cleanup). Chart release push + release-branch advance move into
   `release-artifacts.yaml`.
2. **One-time registry cleanup** of all non-release image/chart versions, run
   during implementation before the first multi-arch release.
3. **Full purge** on deb (data and backups deleted) rather than the issue's
   "considerate cleanup".
4. **No autostart** on install — printed instructions instead.
5. **Unified build flags** `-trimpath -s -w`: raw release binaries become
   stripped (they were not before).

## Acceptance criteria

- [ ] `release: published` produces, for both amd64 and arm64: a raw binary, a
  `.deb`, an `.rpm`, and a container image — all from the **same** per-arch
  binary.
- [ ] `.deb`/`.rpm` install a full system package (binary, systemd unit,
  `nexorious` user, preserved `/etc/nexorious/nexorious.env`, `/var/lib/nexorious`).
- [ ] `postgresql-client`/`postgresql` is a declared hard dependency;
  `legendary` is documented as an optional admin install.
- [ ] First install auto-generates `DB_ENCRYPTION_KEY`; upgrades preserve the
  env file and key.
- [ ] Container release image is a multi-arch (amd64+arm64) manifest.
- [ ] `sha256sums.txt` covers the `.deb`/`.rpm`; no non-source tarballs are produced.
- [ ] Single root `Dockerfile` with shared `runtime-base` + `runtime`/`runtime-ci`
  targets; `make docker` still works.
- [ ] CI package-install smoke test (Debian + Rocky, amd64 + arm64) passes,
  including the upgrade-preservation check.
- [ ] `build-push.yaml` is removed; the nightly/dev image and dev chart flows no
  longer exist; release chart push + release-branch advance run from
  `release-artifacts.yaml`.
- [ ] One-time registry cleanup of non-release image/chart versions completed.
- [ ] `docs/admin-guide.md` updated across all affected sections; README/DEV.md
  and CLAUDE.md updated to match.

## Out of scope (follow-up issues)

- Hosted signed apt/dnf repository (GPG keys, repo metadata/indexing, hosting).
- GPG signing of the packages themselves.
- 32-bit `armhf`/armv7 builds.
- Tightening systemd hardening to `ProtectSystem=strict` with explicit
  `ReadWritePaths`.
