# Native `.deb`/`.rpm` Packages and Release-Only Build Pipeline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce native `.deb`/`.rpm` packages (amd64 + arm64) and a multi-arch container image from one per-arch Go binary, driven by a single release-only CI workflow that replaces the nightly/dev build machinery.

**Architecture:** "Build once per arch, package many." A single CI workflow (`release-artifacts.yaml`) builds the frontend once, compiles one binary per arch with unified flags, then feeds those identical binaries into nfpm (packages), buildx (multi-arch image), and the raw release assets. A refactored single `Dockerfile` shares one `runtime-base` stage between a source-build target (`runtime`) and a prebuilt-binary target (`runtime-ci`). There are **no Go or frontend source changes** — the deliverables are packaging files, a CI workflow, and documentation.

**Tech Stack:** nfpm v2.46.3 (deb/rpm builder), Docker buildx named build-contexts, systemd unit + maintainer scripts (POSIX `sh`), GitHub Actions, Helm OCI push.

**Source spec:** `docs/superpowers/specs/2026-06-10-native-packages-design.md` (issue [#901](https://github.com/drzero42/nexorious/issues/901)). Read it before starting.

---

## Pre-flight

- [ ] **Confirm you are on the feature branch**

Run: `git branch --show-current`
Expected: `feat/native-packages-901` (this plan's branch already exists). If not, create it: `git checkout -b feat/native-packages-901`.

- [ ] **Confirm the toolchain is on PATH** (you must be inside `devenv shell`)

Run: `go version && docker version --format '{{.Server.Version}}' && helm version --short`
Expected: Go 1.26+, a Docker server version, a Helm version. Docker is required for the local package smoke test and the image build.

- [ ] **Install nfpm locally** (used for local package verification in Tasks 2–4)

Run: `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.46.3 && nfpm --version`
Expected: prints `nfpm version v2.46.3 ...`. `go install` drops the binary in `$(go env GOPATH)/bin`, which is on PATH in devenv.

---

## File Structure

Files created or modified by this plan:

| Path | Action | Responsibility |
|---|---|---|
| `Dockerfile` | Modify | Split runtime into shared `runtime-base` + `runtime` (source) + `runtime-ci` (prebuilt). |
| `Makefile` | Modify | `docker` target gains `--target runtime`. |
| `deploy/packaging/nfpm.yaml` | Create | Single nfpm config for both deb/rpm; version/arch via env. |
| `deploy/packaging/nexorious.service` | Create | systemd unit. |
| `deploy/packaging/nexorious.env` | Create | env-file template → `/etc/nexorious/nexorious.env` conffile. |
| `deploy/packaging/scripts/preinstall.sh` | Create | create user/group before files land. |
| `deploy/packaging/scripts/postinstall.sh` | Create | generate key if absent, daemon-reload, next-steps/try-restart. |
| `deploy/packaging/scripts/preremove.sh` | Create | stop/disable on final removal. |
| `deploy/packaging/scripts/postremove.sh` | Create | purge cleanup; daemon-reload. |
| `deploy/packaging/smoke-test.sh` | Create | install + assert + reinstall-preservation check (CI and local). |
| `.github/workflows/release-artifacts.yaml` | Create | the single release-only build workflow. |
| `.github/workflows/build-push.yaml` | Delete | nightly/dev image+chart machinery removed. |
| `.github/workflows/build-release-binaries.yaml` | Delete | superseded by `release-artifacts.yaml`. |
| `docs/admin-guide.md` | Modify | document native packages across all affected sections. |
| `README.md` | Modify | mention packages alongside other install methods. |
| `DEV.md` | Modify | release + Docker sections reflect the new pipeline. |
| `CLAUDE.md` | Modify | Release Process section names the new workflow; nightly/dev gone. |
| `scripts/registry-cleanup.sh` | Create | one-time, maintainer-run registry sweep (not wired to CI). |

---

## Task 1: Refactor the Dockerfile into shared `runtime-base` + two targets

**Files:**
- Modify: `Dockerfile`
- Modify: `Makefile` (the `docker` target)

The current `Dockerfile` ends in a single `runtime` stage that `COPY`s the freshly-built binary. We split the runtime setup into a `runtime-base` stage (defined once), then two final targets: `runtime` (source build, copies from `go-build`) and `runtime-ci` (copies a prebuilt binary from a buildx named context). `runtime` stays **last** so `docker build .` with no `--target` still does a full source build.

- [ ] **Step 1: Replace the runtime stage in `Dockerfile`**

Replace the entire current Stage 3 block (lines 28–47, from `# ─── Stage 3: minimal runtime` through the final `CMD ["serve"]`) with:

```dockerfile
# ─── Shared runtime layer (defined exactly once) ─────────────────────────────
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

# ─── Target: CI release image (prebuilt binary via buildx named context) ─────
FROM runtime-base AS runtime-ci
ARG TARGETARCH
COPY --from=binaries --chown=nexorious:nexorious nexorious-linux-${TARGETARCH} /app/nexorious

# ─── Target: full source build (LAST stage = default target) ─────────────────
FROM runtime-base AS runtime
COPY --from=go-build --chown=nexorious:nexorious /out/nexorious /app/nexorious
```

Leave Stage 1 (`frontend-build`) and Stage 2 (`go-build`) exactly as they are.

- [ ] **Step 2: Update the `docker` target in `Makefile`**

Find:

```make
docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t nexorious:local .
```

Replace with:

```make
docker:
	docker build --target runtime --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t nexorious:local .
```

- [ ] **Step 3: Verify the source build still works**

Run: `make docker`
Expected: build succeeds; ends with `naming to docker.io/library/nexorious:local`. Then confirm the binary runs:

Run: `docker run --rm nexorious:local version`
Expected: prints version info (e.g. `nexorious version main-... ...`) and exits 0.

- [ ] **Step 4: Verify the `runtime-ci` target builds from a prebuilt binary**

First produce a prebuilt binary the way CI will (frontend must be built so the embed succeeds):

Run:
```bash
make frontend
mkdir -p ci-binaries
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=citest -X main.commit=local" \
  -o ci-binaries/nexorious-linux-amd64 ./cmd/nexorious
```
Expected: produces `ci-binaries/nexorious-linux-amd64`.

Then build the CI target with a named build context and run it:

Run:
```bash
docker buildx build --target runtime-ci \
  --build-context binaries=./ci-binaries \
  --load -t nexorious:ci-test .
docker run --rm nexorious:ci-test version
```
Expected: build succeeds (note buildx skips `frontend-build`/`go-build` — only `runtime-base` + `runtime-ci` run); `docker run` prints `nexorious version citest ...`.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile Makefile
git commit -m "refactor: split Dockerfile into shared runtime-base with runtime and runtime-ci targets"
```

Note: `ci-binaries/` is a scratch dir — do not commit it. If it is not already ignored, you may delete it after this task (`rm -rf ci-binaries`); it gets rebuilt by later tasks.

---

## Task 2: nfpm config, systemd unit, and env conffile

**Files:**
- Create: `deploy/packaging/nfpm.yaml`
- Create: `deploy/packaging/nexorious.service`
- Create: `deploy/packaging/nexorious.env`

This task creates the package definition and the two files it installs that are not scripts. The maintainer scripts referenced by `nfpm.yaml` are created in Task 3; building a package will fail until they exist, so verification here is limited to YAML validity and is completed in Task 3.

- [ ] **Step 1: Create `deploy/packaging/nexorious.service`**

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

- [ ] **Step 2: Create `deploy/packaging/nexorious.env`**

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

> The `DB_ENCRYPTION_KEY=` line is intentionally a bare empty value with no trailing space — `postinstall.sh` keys off the exact pattern `^DB_ENCRYPTION_KEY=$` to decide whether to generate a key. Do not add a space or placeholder.

- [ ] **Step 3: Create `deploy/packaging/nfpm.yaml`**

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
    file_info:
      mode: 0755
  - src: deploy/packaging/nexorious.service
    dst: /usr/lib/systemd/system/nexorious.service
    file_info:
      mode: 0644
  - src: deploy/packaging/nexorious.env
    dst: /etc/nexorious/nexorious.env
    type: config|noreplace           # deb conffile / rpm %config(noreplace)
    file_info:
      mode: 0640
      owner: root
      group: nexorious
  - dst: /var/lib/nexorious
    type: dir
    file_info:
      mode: 0750
      owner: nexorious
      group: nexorious

scripts:
  preinstall:  deploy/packaging/scripts/preinstall.sh
  postinstall: deploy/packaging/scripts/postinstall.sh
  preremove:   deploy/packaging/scripts/preremove.sh
  postremove:  deploy/packaging/scripts/postremove.sh
```

- [ ] **Step 4: Validate the YAML files are syntactically well-formed**

Run: `yamllint deploy/packaging/nfpm.yaml`
Expected: no errors (warnings about line length or document-start are acceptable; fix any `error`-level finding). If `yamllint` is not on PATH, fall back to `yq '.' deploy/packaging/nfpm.yaml >/dev/null && echo OK`.

- [ ] **Step 5: Commit**

```bash
git add deploy/packaging/nfpm.yaml deploy/packaging/nexorious.service deploy/packaging/nexorious.env
git commit -m "feat: add nfpm config, systemd unit, and env conffile for native packages"
```

---

## Task 3: Maintainer scripts

**Files:**
- Create: `deploy/packaging/scripts/preinstall.sh`
- Create: `deploy/packaging/scripts/postinstall.sh`
- Create: `deploy/packaging/scripts/preremove.sh`
- Create: `deploy/packaging/scripts/postremove.sh`

nfpm hands the **same** script to both formats. dpkg passes words (`configure`, `remove`, `purge`, `upgrade`) as `$1`; rpm passes counts (`1` = install, `2` = upgrade, `0` = final removal). Each script cases on `$1` to cover both. All scripts are POSIX `sh` (run by `/bin/sh` on the target) and guard systemd calls on `/run/systemd/system` so they are safe inside containers and on non-systemd hosts.

- [ ] **Step 1: Create `deploy/packaging/scripts/preinstall.sh`**

```sh
#!/bin/sh
# Runs before files are unpacked, on both fresh install and upgrade.
# deb: $1 = install | upgrade        rpm: $1 = 1 (install) | 2 (upgrade)
# Create the nexorious system user/group if absent so packaged ownership
# (conffile group, data dir) resolves on both dpkg and rpm.
set -e

if ! getent group nexorious >/dev/null 2>&1; then
    groupadd --system nexorious
fi

if ! getent passwd nexorious >/dev/null 2>&1; then
    useradd --system --gid nexorious \
        --home-dir /var/lib/nexorious \
        --shell /usr/sbin/nologin \
        --comment "Nexorious service account" \
        nexorious
fi
```

- [ ] **Step 2: Create `deploy/packaging/scripts/postinstall.sh`**

```sh
#!/bin/sh
# Runs after files are unpacked.
# deb: $1 = configure, $2 = previous version (empty on fresh install)
# rpm: $1 = 1 (fresh install) | 2 (upgrade)
set -e

ENV_FILE=/etc/nexorious/nexorious.env

# 1. Generate DB_ENCRYPTION_KEY only if the line is present and empty.
#    A non-empty value is never touched -> upgrade-safe by construction.
if [ -f "$ENV_FILE" ] && grep -q '^DB_ENCRYPTION_KEY=$' "$ENV_FILE"; then
    KEY=$(head -c 32 /dev/urandom | base64)
    # '|' is a safe sed delimiter: it never appears in base64 output.
    sed -i "s|^DB_ENCRYPTION_KEY=\$|DB_ENCRYPTION_KEY=${KEY}|" "$ENV_FILE"
fi

# Determine whether this is an upgrade.
is_upgrade=0
case "$1" in
    configure) [ -n "${2:-}" ] && is_upgrade=1 ;;   # deb: $2 set => upgrade
    2)         is_upgrade=1 ;;                       # rpm upgrade
esac

# 2. daemon-reload (only when systemd is actually running).
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
    # 4. Upgrade: restart only if already running.
    if [ "$is_upgrade" = "1" ]; then
        systemctl try-restart nexorious.service || true
    fi
fi

# 3. Fresh install: do NOT auto-enable/start; print next steps.
if [ "$is_upgrade" = "0" ]; then
    cat <<'EOF'

Nexorious installed.

Next steps:
  1. Edit /etc/nexorious/nexorious.env and set DATABASE_URL.
     A DB_ENCRYPTION_KEY has been generated for you — do not change it.
  2. Enable and start the service:
       systemctl enable --now nexorious

EOF
fi
```

- [ ] **Step 3: Create `deploy/packaging/scripts/preremove.sh`**

```sh
#!/bin/sh
# Runs before files are removed.
# deb: $1 = remove (final) | upgrade | deconfigure ...
# rpm: $1 = 0 (final removal) | 1 (upgrade)
set -e

final_removal=0
case "$1" in
    remove) final_removal=1 ;;   # deb final removal
    0)      final_removal=1 ;;   # rpm final removal
esac

if [ "$final_removal" = "1" ] && [ -d /run/systemd/system ]; then
    systemctl stop nexorious.service || true
    systemctl disable nexorious.service || true
fi
```

- [ ] **Step 4: Create `deploy/packaging/scripts/postremove.sh`**

```sh
#!/bin/sh
# Runs after files are removed.
# deb: $1 = remove | purge | upgrade ...
# rpm: $1 = 0 (uninstall) | 1 (upgrade)
set -e

case "$1" in
    purge)
        # deb purge: true clean uninstall — remove config, ALL data
        # (including backups), and the service account.
        rm -rf /etc/nexorious
        rm -rf /var/lib/nexorious
        if getent passwd nexorious >/dev/null 2>&1; then
            userdel nexorious || true
        fi
        if getent group nexorious >/dev/null 2>&1; then
            groupdel nexorious || true
        fi
        ;;
esac

# deb remove (no purge), rpm uninstall, and any upgrade path: reload only.
# config, key, data, and user remain for a possible reinstall.
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
fi
```

- [ ] **Step 5: Verify a package builds and its contents/scripts are correct**

Build the amd64 binary the package needs, then build a `.deb` and inspect it:

```bash
make frontend
mkdir -p ci-binaries
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=0.0.0 -X main.commit=local" \
  -o ci-binaries/nexorious-linux-amd64 ./cmd/nexorious
NFPM_ARCH=amd64 NFPM_VERSION=0.0.0 nfpm package -f deploy/packaging/nfpm.yaml -p deb -t /tmp/
dpkg-deb -c /tmp/nexorious_0.0.0_amd64.deb
dpkg-deb -I /tmp/nexorious_0.0.0_amd64.deb
```
Expected: `dpkg-deb -c` lists `/usr/bin/nexorious` (mode 0755), `/usr/lib/systemd/system/nexorious.service`, `/etc/nexorious/nexorious.env`, and `/var/lib/nexorious/`. `dpkg-deb -I` shows `Depends: postgresql-client`, a `Conffiles:` line for the env file, and the four maintainer scripts (`preinst`, `postinst`, `prerm`, `postrm`).

> If `dpkg-deb` is unavailable on the host, skip to Task 4 — the containerized smoke test there covers contents end-to-end.

- [ ] **Step 6: Commit**

```bash
git add deploy/packaging/scripts/
git commit -m "feat: add deb/rpm maintainer scripts for native packages"
```

---

## Task 4: Smoke-test script and local install verification

**Files:**
- Create: `deploy/packaging/smoke-test.sh`

This script installs a package, asserts the package shape (binary runs, user/dir/conffile present, unit verifies, key generated non-empty), then reinstalls the same package and asserts the env edits **and** the generated key are preserved. CI runs it across the 4-way matrix; you run it locally for amd64 in throwaway containers.

- [ ] **Step 1: Create `deploy/packaging/smoke-test.sh`**

```bash
#!/usr/bin/env bash
# Install a nexorious package, assert its shape, then reinstall and assert
# that the env conffile edits and the generated key are preserved.
#
# Usage: smoke-test.sh <deb|rpm> <path-to-package>
# Intended to run as root inside a clean debian:13 or rockylinux:9 container.
set -euo pipefail

PKG_TYPE="$1"
PKG_PATH="$2"
# apt requires a path-like argument (leading ./ or /).
case "$PKG_PATH" in
    /*|./*) ;;
    *) PKG_PATH="./$PKG_PATH" ;;
esac

ENV_FILE=/etc/nexorious/nexorious.env
UNIT=/usr/lib/systemd/system/nexorious.service

install_systemd_and_pkg() {
    if [ "$PKG_TYPE" = "deb" ]; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update
        apt-get install -y systemd
        apt-get install -y "$PKG_PATH"
    else
        dnf install -y systemd
        dnf install -y "$PKG_PATH"
    fi
}

reinstall_pkg() {
    if [ "$PKG_TYPE" = "deb" ]; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get install -y --reinstall "$PKG_PATH"
    else
        dnf reinstall -y "$PKG_PATH"
    fi
}

echo "=== Installing package ==="
install_systemd_and_pkg

echo "=== Asserting package shape ==="
nexorious version
getent passwd nexorious
test -d /var/lib/nexorious
test -f "$ENV_FILE"
systemd-analyze verify "$UNIT"

KEY1=$(grep '^DB_ENCRYPTION_KEY=' "$ENV_FILE" | cut -d= -f2-)
if [ -z "$KEY1" ]; then
    echo "FAIL: DB_ENCRYPTION_KEY was not generated" >&2
    exit 1
fi
echo "Generated key present (len=${#KEY1})"

echo "=== Editing env file, then reinstalling ==="
sed -i 's|^DATABASE_URL=.*|DATABASE_URL=postgres://smoke@example|' "$ENV_FILE"
reinstall_pkg

echo "=== Asserting preservation ==="
KEY2=$(grep '^DB_ENCRYPTION_KEY=' "$ENV_FILE" | cut -d= -f2-)
if [ "$KEY1" != "$KEY2" ]; then
    echo "FAIL: key changed across reinstall ($KEY1 -> $KEY2)" >&2
    exit 1
fi
if ! grep -q '^DATABASE_URL=postgres://smoke@example' "$ENV_FILE"; then
    echo "FAIL: DATABASE_URL edit was not preserved across reinstall" >&2
    exit 1
fi

echo "=== SMOKE TEST PASSED ($PKG_TYPE) ==="
```

- [ ] **Step 2: Build both amd64 packages for local testing**

```bash
make frontend
mkdir -p ci-binaries dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=0.0.0 -X main.commit=local" \
  -o ci-binaries/nexorious-linux-amd64 ./cmd/nexorious
NFPM_ARCH=amd64 NFPM_VERSION=0.0.0 nfpm package -f deploy/packaging/nfpm.yaml -p deb -t dist/
NFPM_ARCH=amd64 NFPM_VERSION=0.0.0 nfpm package -f deploy/packaging/nfpm.yaml -p rpm -t dist/
ls dist/
```
Expected: `dist/` contains `nexorious_0.0.0_amd64.deb` and an rpm named `nexorious-0.0.0*.x86_64.rpm`.

- [ ] **Step 3: Run the deb smoke test in a Debian 13 container**

```bash
docker run --rm -v "$PWD:/work" -w /work debian:13 \
  bash deploy/packaging/smoke-test.sh deb "$(ls dist/nexorious_*_amd64.deb)"
```
Expected: ends with `=== SMOKE TEST PASSED (deb) ===`.

- [ ] **Step 4: Run the rpm smoke test in a Rocky Linux 9 container**

```bash
docker run --rm -v "$PWD:/work" -w /work rockylinux:9 \
  bash deploy/packaging/smoke-test.sh rpm "$(ls dist/nexorious-*.x86_64.rpm)"
```
Expected: ends with `=== SMOKE TEST PASSED (rpm) ===`.

> If either fails, debug the maintainer scripts (Task 3) — the failing assertion names the cause (key not generated, edit not preserved, unit verify failed). Re-run after fixing. Do not proceed until both pass.

- [ ] **Step 5: Clean up scratch artifacts and commit**

```bash
rm -rf ci-binaries dist
git add deploy/packaging/smoke-test.sh
git commit -m "test: add deb/rpm install smoke test for native packages"
```

---

## Task 5: The release-only workflow; delete the old workflows

**Files:**
- Create: `.github/workflows/release-artifacts.yaml`
- Delete: `.github/workflows/build-push.yaml`
- Delete: `.github/workflows/build-release-binaries.yaml`

This is the heart of the change: one workflow that builds once per arch and fans the binaries out to packages, image, and raw assets. It runs fully on `release: published`, and in a reduced PR mode (build + smoke-test + no-push image) on changes to packaging-relevant paths — which means **this PR triggers it**, giving real validation before merge.

- [ ] **Step 1: Create `.github/workflows/release-artifacts.yaml`**

```yaml
---
name: Release Artifacts

on:
  release:
    types: [published]
  pull_request:
    paths:
      - 'deploy/packaging/**'
      - 'Dockerfile'
      - '.github/workflows/release-artifacts.yaml'

permissions:
  contents: read

jobs:
  build:
    name: Build binaries and packages
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v6
        with:
          ref: >-
            ${{ github.event_name == 'release'
                && github.event.release.tag_name || github.sha }}

      - name: Set up Node.js
        uses: actions/setup-node@v6
        with:
          node-version: '24'

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod

      - name: Install nfpm
        run: |
          set -euo pipefail
          cd /tmp
          curl -sSL \
            https://github.com/goreleaser/nfpm/releases/download/v2.46.3/nfpm_2.46.3_Linux_x86_64.tar.gz \
            -o nfpm.tgz
          tar xzf nfpm.tgz nfpm
          sudo install -m0755 nfpm /usr/local/bin/nfpm
          nfpm --version

      - name: Compute versions
        id: versions
        run: |
          set -euo pipefail
          COMMIT=$(git rev-parse HEAD)
          if [[ "${{ github.event_name }}" == "release" ]]; then
            TAG="${{ github.event.release.tag_name }}"
            VERSION="${TAG#v}"
            PKG_VERSION="$VERSION"
          else
            VERSION="0.0.0-pr${{ github.event.pull_request.number }}"
            PKG_VERSION="0.0.0"
          fi
          echo "commit=$COMMIT"           >> "$GITHUB_OUTPUT"
          echo "version=$VERSION"         >> "$GITHUB_OUTPUT"
          echo "pkg_version=$PKG_VERSION" >> "$GITHUB_OUTPUT"

      - name: Build frontend
        run: cd ui/frontend && npm ci && npm run build

      - name: Build binaries (amd64, arm64)
        env:
          VERSION: ${{ steps.versions.outputs.version }}
          COMMIT: ${{ steps.versions.outputs.commit }}
          PKG_VERSION: ${{ steps.versions.outputs.pkg_version }}
        run: |
          set -euo pipefail
          mkdir -p ci-binaries dist
          for arch in amd64 arm64; do
            CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
              -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
              -o "ci-binaries/nexorious-linux-${arch}" ./cmd/nexorious
            cp "ci-binaries/nexorious-linux-${arch}" \
               "dist/nexorious_${PKG_VERSION}_linux_${arch}"
          done

      - name: Build packages (deb + rpm × amd64, arm64)
        env:
          NFPM_VERSION: ${{ steps.versions.outputs.pkg_version }}
        run: |
          set -euo pipefail
          for arch in amd64 arm64; do
            NFPM_ARCH="$arch" nfpm package -f deploy/packaging/nfpm.yaml -p deb -t dist/
            NFPM_ARCH="$arch" nfpm package -f deploy/packaging/nfpm.yaml -p rpm -t dist/
          done

      - name: Checksums over all artifacts
        run: cd dist && sha256sum nexorious* > sha256sums.txt

      - name: Upload dist artifact
        uses: actions/upload-artifact@v4
        with:
          name: dist
          path: dist/

      - name: Upload ci-binaries artifact
        uses: actions/upload-artifact@v4
        with:
          name: ci-binaries
          path: ci-binaries/

  smoke-test:
    name: Smoke ${{ matrix.pkg }} on ${{ matrix.image }} (${{ matrix.arch }})
    needs: build
    runs-on: ${{ matrix.runner }}
    container:
      image: ${{ matrix.image }}
    strategy:
      fail-fast: false
      matrix:
        include:
          - {runner: ubuntu-latest,    image: 'debian:13',    pkg: deb, arch: amd64}
          - {runner: ubuntu-latest,    image: 'rockylinux:9', pkg: rpm, arch: amd64}
          - {runner: ubuntu-24.04-arm, image: 'debian:13',    pkg: deb, arch: arm64}
          - {runner: ubuntu-24.04-arm, image: 'rockylinux:9', pkg: rpm, arch: arm64}
    steps:
      - name: Install base tools
        run: |
          if command -v apt-get >/dev/null; then
            apt-get update && apt-get install -y git ca-certificates curl
          else
            dnf install -y git ca-certificates curl
          fi

      - name: Checkout
        uses: actions/checkout@v6
        with:
          ref: >-
            ${{ github.event_name == 'release'
                && github.event.release.tag_name || github.sha }}

      - name: Download dist artifact
        uses: actions/download-artifact@v4
        with:
          name: dist
          path: dist

      - name: Run smoke test
        run: |
          set -euo pipefail
          case "${{ matrix.pkg }}-${{ matrix.arch }}" in
            deb-amd64) PKG=$(ls dist/nexorious_*_amd64.deb) ;;
            deb-arm64) PKG=$(ls dist/nexorious_*_arm64.deb) ;;
            rpm-amd64) PKG=$(ls dist/nexorious-*.x86_64.rpm) ;;
            rpm-arm64) PKG=$(ls dist/nexorious-*.aarch64.rpm) ;;
          esac
          bash deploy/packaging/smoke-test.sh "${{ matrix.pkg }}" "$PKG"

  image:
    name: Container image
    needs: smoke-test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - name: Checkout
        uses: actions/checkout@v6
        with:
          ref: >-
            ${{ github.event_name == 'release'
                && github.event.release.tag_name || github.sha }}

      - name: Download ci-binaries artifact
        uses: actions/download-artifact@v4
        with:
          name: ci-binaries
          path: ci-binaries

      - name: Compute version
        id: v
        run: |
          if [[ "${{ github.event_name }}" == "release" ]]; then
            TAG="${{ github.event.release.tag_name }}"
            echo "value=${TAG#v}" >> "$GITHUB_OUTPUT"
          else
            echo "value=0.0.0-pr${{ github.event.pull_request.number }}" >> "$GITHUB_OUTPUT"
          fi

      - name: Set up QEMU
        if: github.event_name == 'release'
        uses: docker/setup-qemu-action@v3

      - name: Set up Buildx
        uses: docker/setup-buildx-action@v4

      - name: Log in to ghcr.io
        if: github.event_name == 'release'
        uses: docker/login-action@v4
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build image (PR — amd64, no push)
        if: github.event_name != 'release'
        uses: docker/build-push-action@v7
        with:
          context: .
          build-contexts: |
            binaries=./ci-binaries
          target: runtime-ci
          platforms: linux/amd64
          push: false

      - name: Build and push image (release — multi-arch)
        if: github.event_name == 'release'
        uses: docker/build-push-action@v7
        with:
          context: .
          build-contexts: |
            binaries=./ci-binaries
          target: runtime-ci
          platforms: linux/amd64,linux/arm64
          push: true
          tags: |
            ghcr.io/drzero42/nexorious:${{ steps.v.outputs.value }}
            ghcr.io/drzero42/nexorious:latest

  upload:
    name: Upload release assets
    needs: smoke-test
    if: github.event_name == 'release'
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - name: Download dist artifact
        uses: actions/download-artifact@v4
        with:
          name: dist
          path: dist

      - name: Upload to release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAG_NAME: ${{ github.event.release.tag_name }}
          GH_REPO: ${{ github.repository }}
        run: gh release upload "$TAG_NAME" dist/* --clobber

  chart:
    name: Push Helm chart
    needs: image
    if: github.event_name == 'release'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - name: Checkout
        uses: actions/checkout@v6
        with:
          ref: ${{ github.event.release.tag_name }}

      - name: Compute chart version
        id: cv
        run: |
          V="${{ github.event.release.tag_name }}"
          V="${V#v}"
          echo "version=$V" >> "$GITHUB_OUTPUT"

      - name: Add bjw-s Helm repo
        run: helm repo add bjw-s https://bjw-s-labs.github.io/helm-charts/

      - name: Build Helm dependencies
        run: helm dependency update deploy/helm/

      - name: Package Helm chart
        id: pkg
        run: |
          PKG=$(helm package deploy/helm/ \
            --version "${{ steps.cv.outputs.version }}" \
            --app-version "${{ steps.cv.outputs.version }}" \
            | awk '{print $NF}')
          echo "path=$PKG" >> "$GITHUB_OUTPUT"

      - name: Log in to ghcr.io (Helm)
        run: |
          echo "${{ secrets.GITHUB_TOKEN }}" | \
            helm registry login ghcr.io \
              --username "${{ github.actor }}" \
              --password-stdin

      - name: Push Helm chart
        run: helm push "${{ steps.pkg.outputs.path }}" oci://ghcr.io/drzero42/charts

  release-branch:
    name: Advance release branch
    needs: [upload, image]
    if: github.event_name == 'release'
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - name: Fast-forward release branch and pin nix version
        env:
          GH_TOKEN: ${{ secrets.RELEASE_PLEASE_TOKEN }}
        run: |
          set -euo pipefail
          VERSION="${{ github.event.release.tag_name }}"
          VERSION="${VERSION#v}"
          RELEASE_SHA="${{ github.sha }}"

          # Fast-forward (or create) the release branch to the release commit.
          if gh api "repos/${{ github.repository }}/git/refs/heads/release" -q .ref 2>/dev/null; then
            gh api "repos/${{ github.repository }}/git/refs/heads/release" \
              -X PATCH --field sha="${RELEASE_SHA}" --field force=true
          else
            gh api "repos/${{ github.repository }}/git/refs" \
              -X POST --field ref="refs/heads/release" --field sha="${RELEASE_SHA}"
          fi

          # Add one commit on top of the release branch that creates
          # nix/release-version.txt. This file is absent from main, so
          # nix builds from main get "main-<shortrev>" while builds from
          # the release branch get the clean semver.
          BASE_TREE=$(gh api "repos/${{ github.repository }}/git/commits/${RELEASE_SHA}" -q .tree.sha)
          BLOB=$(gh api "repos/${{ github.repository }}/git/blobs" \
            -X POST --field content="${VERSION}" --field encoding="utf-8" -q .sha)
          NEW_TREE=$(gh api "repos/${{ github.repository }}/git/trees" \
            -X POST --input - <<EOF
          {"base_tree":"${BASE_TREE}","tree":[{"path":"nix/release-version.txt","mode":"100644","type":"blob","sha":"${BLOB}"}]}
          EOF
          )
          NEW_TREE_SHA=$(echo "${NEW_TREE}" | jq -r .sha)
          NEW_COMMIT=$(gh api "repos/${{ github.repository }}/git/commits" \
            -X POST --input - <<EOF
          {"message":"chore: pin nix version to ${VERSION}","tree":"${NEW_TREE_SHA}","parents":["${RELEASE_SHA}"]}
          EOF
          )
          NEW_COMMIT_SHA=$(echo "${NEW_COMMIT}" | jq -r .sha)
          gh api "repos/${{ github.repository }}/git/refs/heads/release" \
            -X PATCH --field sha="${NEW_COMMIT_SHA}" --field force=true
```

> The `release-branch` job is copied verbatim from the old `build-push.yaml` `update-release-branch` job (the heredoc indentation matters — keep it exactly as shown). The only changes vs. the original are the `needs:` (now `[upload, image]`) and the `if:` (now `github.event_name == 'release'`, since this workflow only triggers on `published`).

- [ ] **Step 2: Delete the two superseded workflows**

```bash
git rm .github/workflows/build-push.yaml .github/workflows/build-release-binaries.yaml
```

- [ ] **Step 3: Lint the new workflow**

Run: `yamllint .github/workflows/release-artifacts.yaml`
Expected: no `error`-level findings. If `actionlint` is available, also run `actionlint .github/workflows/release-artifacts.yaml` and resolve any reported issues.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release-artifacts.yaml
git commit -m "ci: replace nightly/dev build flow with release-only release-artifacts workflow"
```

> After this PR is opened, the workflow runs in PR mode: `build` → `smoke-test` (all 4) → `image` (amd64 no-push). Treat a green run here as the authoritative end-to-end verification — it exercises real arm64 packages on native runners, which you cannot do locally.

---

## Task 6: Documentation

**Files:**
- Modify: `docs/admin-guide.md`
- Modify: `README.md`
- Modify: `DEV.md`
- Modify: `CLAUDE.md`

`docs/admin-guide.md` is embedded and rendered in-app — every touched section must read coherently, not merely be appended to. No new embedded docs are added, so `docs/embed.go` and `ui/frontend/src/lib/doc-links.ts` stay untouched.

- [ ] **Step 1: Add a "Native packages" subsection to `docs/admin-guide.md` Deploying section**

In `docs/admin-guide.md`, immediately after the `### Docker Compose` subsection (i.e. before `### Kubernetes / Helm` at line ~26), insert:

```markdown
### Native packages (Debian/Ubuntu, RHEL/Fedora/Rocky)

For Debian/Ubuntu and RHEL/Fedora/Rocky hosts there are native `.deb` and `.rpm` packages on each [GitHub Release](https://github.com/drzero42/nexorious/releases), for `amd64` and `arm64`. They install the binary, a systemd service, a dedicated `nexorious` user, and a managed config file — no hand-wiring.

```bash
# Pick the file matching your distro and architecture from the Release, then:
# Verify it against the release checksums (optional but recommended):
sha256sum -c sha256sums.txt --ignore-missing

# Debian / Ubuntu:
sudo apt install ./nexorious_X.Y.Z_amd64.deb

# RHEL / Fedora / Rocky:
sudo dnf install ./nexorious-X.Y.Z.x86_64.rpm
```

The package depends on the PostgreSQL client tools (`postgresql-client` on Debian/Ubuntu, `postgresql` on RHEL-family), which your package manager pulls in automatically. It installs config to `/etc/nexorious/nexorious.env`, presets `STORAGE_PATH`/`BACKUP_PATH` to `/var/lib/nexorious`, and **auto-generates `DB_ENCRYPTION_KEY` on first install**.

After installing, set your database connection and start the service:

```bash
sudo nano /etc/nexorious/nexorious.env   # set DATABASE_URL
sudo systemctl enable --now nexorious
```

The service is not started automatically on install — it idles on the "database unavailable" page until `DATABASE_URL` is set, so you enable it once you've configured it (see [First run](#first-run)). Logs go to the journal: `journalctl -u nexorious -f`.
```

- [ ] **Step 2: Reconcile the `### Single binary` subsection**

In `docs/admin-guide.md`, replace the first paragraph of `### Single binary` (currently starting "You can also build the binary and run it directly…", lines ~99–106) with:

```markdown
On Debian/Ubuntu or RHEL/Fedora/Rocky, prefer the [native packages](#native-packages-debianubuntu-rhelfedorarocky) above — they wire up the service, user, and config for you. For other distributions, or just to try it out, you can build and run the binary directly next to a PostgreSQL instance. Build it (`make`), set at least the [database connection and `DB_ENCRYPTION_KEY`](#configuration), and start it:

```bash
export DATABASE_URL="postgres://user:password@host:5432/nexorious"
export DB_ENCRYPTION_KEY="$(openssl rand -base64 32)"
./nexorious serve
```
```

Leave the two paragraphs that follow it (the migration note and the `pg_dump`/`psql` note) unchanged.

- [ ] **Step 3: Update the `## Configuration` section for package installs**

In `docs/admin-guide.md`, at the end of the `### The essentials` list (after the `SESSION_COOKIE_SECURE` bullet at line ~121), add this bullet:

```markdown
- **Where config lives.** With the `.deb`/`.rpm` packages, configuration is the env file at `/etc/nexorious/nexorious.env` (read by the systemd service); the package presets `STORAGE_PATH` and `BACKUP_PATH` to `/var/lib/nexorious` and generates `DB_ENCRYPTION_KEY` for you on first install — never change that generated key afterwards. With Docker, Helm, or the raw binary, you supply the same variables your usual way.
```

- [ ] **Step 4: Update the `## First run` section for the package flow**

In `docs/admin-guide.md`, in `## First run`, append to the end of item 1 (after "Until the schema is up to date, the server gates every route other than the migration page.", line ~208) a new paragraph:

```markdown
   With a package install, the service idles on the database-unavailable page until you set `DATABASE_URL` in `/etc/nexorious/nexorious.env` and `systemctl enable --now nexorious`; from there apply migrations the same way — `nexorious migrate` on the host, or the `/migrate` browser flow.
```

- [ ] **Step 5: Update the `## Epic Games Store sync` section**

In `docs/admin-guide.md`, in the bulleted list under "Getting `legendary` in place depends on how you deploy:" (line ~234), add a third bullet after the "Single binary / from source" one:

```markdown
- **Native package (`.deb`/`.rpm`)** — `legendary` is **not** bundled in the packages (it is a pip tool with no distro package). Install it yourself (`pipx install legendary-gl` or `pip install --user legendary-gl`) so `legendary` is on the service's `PATH`, then set `LEGENDARY_WORK_DIR`. Epic sync degrades gracefully — everything else works — while it is absent.
```

- [ ] **Step 6: Update the `## Upgrades and versioning` section**

In `docs/admin-guide.md`, in `## Upgrades and versioning`, after the paragraph ending "…done." (the first paragraph, line ~319), insert:

```markdown
With the `.deb`/`.rpm` packages, a normal `apt upgrade` / `dnf upgrade` replaces the binary and preserves your `/etc/nexorious/nexorious.env` — the env file is a conffile (`%config(noreplace)` on rpm), so your settings and the generated `DB_ENCRYPTION_KEY` survive the upgrade, and the service is restarted only if it was already running. Removal differs by family: `apt purge` deletes `/var/lib/nexorious` **including backups** and removes the `nexorious` user (a true clean uninstall), while a plain `apt remove` and any `dnf` uninstall leave config, key, and data in place for a later reinstall (rpm keeps a modified env file as `nexorious.env.rpmsave`).
```

- [ ] **Step 7: Update the `## Monitoring and operations` section**

In `docs/admin-guide.md`, in `## Monitoring and operations`, replace the `- **Logs**` bullet (line ~313) with:

```markdown
- **Logs** — Nexorious logs to standard output; set `LOG_LEVEL=debug` when you need more detail. With the native packages it runs under systemd, so its output lands in the journal — `journalctl -u nexorious` (add `-f` to follow). In Docker or Kubernetes, collect stdout however you collect logs from anything else.
```

- [ ] **Step 8: Verify the admin guide still renders and links resolve**

Run: `grep -n "native-packages-debianubuntu-rhelfedorarocky" docs/admin-guide.md`
Expected: two matches — the anchor reference in `### Single binary` and (implicitly) the heading it points at. Confirm the heading `### Native packages (Debian/Ubuntu, RHEL/Fedora/Rocky)` exists with `grep -n "### Native packages" docs/admin-guide.md`.

- [ ] **Step 9: Update `README.md` Features list**

In `README.md`, replace the `- **Single Binary**: …` bullet (line ~48) with:

```markdown
- **Easy to self-host**: single Go binary with the React SPA embedded — run the container image, the Helm chart, native `.deb`/`.rpm` packages, or the raw binary
```

- [ ] **Step 10: Update `DEV.md` release and Docker sections**

In `DEV.md`, replace step 4 of `### Normal release` (line ~12) with:

```markdown
4. Merge the Release PR. CI (`release-artifacts.yaml`) then builds, for both amd64 and arm64, the raw binary, the `.deb`, the `.rpm`, and the multi-arch container image — all from the same per-arch binary — smoke-tests every package, publishes the GitHub Release assets, pushes the image and Helm chart, and advances the `release` branch. There is no nightly or dev build flow; artifacts are produced only when a release is published.
```

Then in `## Container Image (Docker)` (line ~244), replace the **Build** paragraph and code block (lines ~248–255) with:

```markdown
**Build** (full source build — the default `runtime` target):

```bash
make docker   # builds the runtime target, tags nexorious:local
```

Or directly, e.g. to pass explicit version metadata:

```bash
docker build --target runtime \
  --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" \
  -t nexorious:local .
```

The `Dockerfile` shares a single `runtime-base` stage between `runtime` (the full source build above) and `runtime-ci` (used only by CI, which copies a prebuilt per-arch binary from a buildx named context instead of compiling). Building `runtime-ci` locally requires that named context — see `.github/workflows/release-artifacts.yaml`.
```

- [ ] **Step 11: Update the Release Process section in `CLAUDE.md`**

In `CLAUDE.md`, in the `## Release Process` → "Cutting a release" / "Overrides" area, find the line describing the release build (step 4 mentions `build-push.yaml` pushing the image and chart) and update it so it reads:

```markdown
4. Merge the Release PR. release-please creates the `vX.Y.Z` tag and publishes a GitHub Release; `release-artifacts.yaml` then builds — for amd64 and arm64, from one per-arch binary — the raw binary, `.deb`, `.rpm`, and a multi-arch container image, smoke-tests the packages, uploads the release assets, pushes the image (semver tag + `latest`) and Helm chart, and advances the `release` branch. There is no nightly/dev build flow (`build-push.yaml` was removed); non-release artifacts no longer exist.
```

If any other sentence in `CLAUDE.md` names `build-push.yaml` or `build-release-binaries.yaml`, update it to `release-artifacts.yaml` (search: `grep -n "build-push\|build-release-binaries" CLAUDE.md`).

- [ ] **Step 12: Commit**

```bash
git add docs/admin-guide.md README.md DEV.md CLAUDE.md
git commit -m "docs: document native packages and the release-only build pipeline"
```

---

## Task 7: One-time registry cleanup (operational — maintainer-run)

**Files:**
- Create: `scripts/registry-cleanup.sh`

This is **not** a code change exercised by CI and **not** a recurring workflow. It is a one-shot sweep that deletes every ghcr package version that does not carry a release tag, run **once during rollout — after this PR merges, before the first multi-arch release**, with the maintainer's `gh` auth. It must show the kept/deleted lists before deleting, and must never run again: after the first multi-arch release the image package contains *untagged* per-platform manifests referenced by the multi-arch index, and a "delete untagged" pass would corrupt released images.

- [ ] **Step 1: Create `scripts/registry-cleanup.sh`**

```bash
#!/usr/bin/env bash
# ONE-TIME registry cleanup. Deletes every ghcr version of the nexorious image
# and charts/nexorious that does NOT carry an X.Y.Z release tag.
#
# Run ONCE during rollout (after the workflow PR merges, before the first
# multi-arch release) with the maintainer's `gh auth`. DO NOT wire to CI and
# DO NOT run after a multi-arch release exists — untagged per-platform
# manifests would be deleted, corrupting released images.
#
# Usage:
#   scripts/registry-cleanup.sh           # dry run: print keep/delete lists
#   scripts/registry-cleanup.sh --delete  # actually delete
set -euo pipefail

OWNER=drzero42
DELETE=0
[ "${1:-}" = "--delete" ] && DELETE=1

# A version is KEPT only if it carries a tag matching X.Y.Z (optional -suffix).
RELEASE_TAG_RE='^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$'

process_package() {
    local kind="$1" encoded="$2" label="$3"
    echo "=== ${label} ==="
    # List "<version-id> <comma-joined-tags>" for every version.
    gh api --paginate \
        "/users/${OWNER}/packages/${kind}/${encoded}/versions" \
        --jq '.[] | "\(.id) \((.metadata.container.tags // []) | join(","))"' \
    | while read -r id tags; do
        keep=0
        IFS=',' read -ra tag_arr <<< "$tags"
        for t in "${tag_arr[@]}"; do
            if [[ "$t" =~ $RELEASE_TAG_RE ]]; then keep=1; break; fi
        done
        if [ "$keep" = "1" ]; then
            echo "KEEP   $id  [$tags]"
        else
            echo "DELETE $id  [$tags]"
            if [ "$DELETE" = "1" ]; then
                gh api -X DELETE "/users/${OWNER}/packages/${kind}/${encoded}/versions/${id}"
            fi
        fi
    done
}

process_package container "nexorious"          "image: nexorious"
process_package container "charts%2Fnexorious" "chart: charts/nexorious"

if [ "$DELETE" = "0" ]; then
    echo
    echo "Dry run complete. Re-run with --delete to apply."
fi
```

- [ ] **Step 2: Commit the script (it is documented, auditable, and never auto-run)**

```bash
chmod +x scripts/registry-cleanup.sh
git add scripts/registry-cleanup.sh
git commit -m "chore: add one-time ghcr registry cleanup script for rollout"
```

- [ ] **Step 3: Record the rollout runbook (no code action — for the maintainer)**

The rollout order, to be performed by the maintainer with `gh` auth after this PR merges:

1. Merge this PR to `main` (workflows replaced).
2. Dry run: `scripts/registry-cleanup.sh` — review the `KEEP`/`DELETE` lists. Confirm every `X.Y.Z` release tag is under `KEEP` and only `dev`/`main-…`/`0.0.0-dev…`/untagged versions are under `DELETE`.
3. Apply: `scripts/registry-cleanup.sh --delete`.
4. Cut the next release as usual; it publishes the first multi-arch image into the cleaned registry.
5. **Do not run the script again** after step 4.

---

## Final verification

- [ ] **Step 1: Full local sanity pass**

Run:
```bash
git status            # expect: clean working tree, all task commits present
git log --oneline -8  # expect: the commits from Tasks 1,2,3,4,5,6,7
ls .github/workflows/ # expect: release-artifacts.yaml present; build-push.yaml and build-release-binaries.yaml gone
```

- [ ] **Step 2: Re-run the local amd64 smoke tests against a release-flavoured build**

This mirrors what CI does, with release-style flags:
```bash
make frontend
mkdir -p ci-binaries dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=9.9.9 -X main.commit=$(git rev-parse HEAD)" \
  -o ci-binaries/nexorious-linux-amd64 ./cmd/nexorious
NFPM_ARCH=amd64 NFPM_VERSION=9.9.9 nfpm package -f deploy/packaging/nfpm.yaml -p deb -t dist/
NFPM_ARCH=amd64 NFPM_VERSION=9.9.9 nfpm package -f deploy/packaging/nfpm.yaml -p rpm -t dist/
docker run --rm -v "$PWD:/work" -w /work debian:13 \
  bash deploy/packaging/smoke-test.sh deb "$(ls dist/nexorious_*_amd64.deb)"
docker run --rm -v "$PWD:/work" -w /work rockylinux:9 \
  bash deploy/packaging/smoke-test.sh rpm "$(ls dist/nexorious-*.x86_64.rpm)"
rm -rf ci-binaries dist
```
Expected: both end with `=== SMOKE TEST PASSED ... ===`.

- [ ] **Step 3: Push and open the PR; let CI validate arm64 + the image build**

```bash
git push -u origin feat/native-packages-901
gh pr create --fill
```
Then watch the `Release Artifacts` workflow on the PR (`gh run list --branch feat/native-packages-901`). It must be green: `build` → all four `smoke-test` legs (incl. native arm64) → `image` (PR no-push). This green run is the authoritative end-to-end check for the parts you cannot test locally.

> PR title must be a Conventional Commit so release-please parses it correctly — e.g. `feat: native .deb/.rpm packages and release-only build pipeline`. Use `Closes #901` in the PR body.

---

## Self-Review (performed against the spec)

**Spec coverage** — every spec section maps to a task:

- Unified build flags (`-trimpath -s -w`) → Task 1 (Dockerfile) + Task 5 (workflow `go build`).
- Build topology / `release-artifacts.yaml` (build, smoke-test, image, upload, chart, release-branch) → Task 5.
- Delete `build-push.yaml` + `build-release-binaries.yaml` → Task 5.
- Dockerfile shared `runtime-base` + `runtime`/`runtime-ci`; `make docker --target runtime` → Task 1.
- Native packages (`nfpm.yaml`, service, env, 4 scripts) → Tasks 2 + 3.
- Dependencies (`postgresql-client`/`postgresql`; legendary not packaged) → Task 2 (`overrides`) + Task 6 (Epic docs).
- Smoke tests (4-way matrix, version/user/dir/conffile/key/unit-verify + reinstall preservation) → Task 4 (`smoke-test.sh`) + Task 5 (matrix).
- One-time registry cleanup → Task 7.
- Documentation (admin-guide all sections, README, DEV, CLAUDE) → Task 6.

**Acceptance criteria** — each item in the spec's checklist is produced by Task 5 (artifacts/image/multi-arch/checksums/no tarballs), Task 1 (single Dockerfile/`make docker`), Tasks 2–4 (package shape, dependency, key generation/preservation, smoke test), Task 7 (registry cleanup), Task 6 (docs).

**Type/name consistency** — the binary staging name `ci-binaries/nexorious-linux-${ARCH}` is identical across the Dockerfile `runtime-ci` COPY (Task 1), `nfpm.yaml` `contents.src` (Task 2), and the workflow build/`build-contexts` (Task 5). The env file's `DB_ENCRYPTION_KEY=` empty-line pattern in Task 2 matches the `^DB_ENCRYPTION_KEY=$` grep/sed in Task 3's `postinstall.sh`. The buildx named context is `binaries` everywhere (`--from=binaries` in the Dockerfile, `build-contexts: binaries=./ci-binaries` in the workflow). nfpm pinned to v2.46.3 in both the local install (Pre-flight) and the CI install (Task 5).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-10-native-packages.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
