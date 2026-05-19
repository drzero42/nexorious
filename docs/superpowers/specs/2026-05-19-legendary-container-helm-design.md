# Design: legendary-gl in container image + Helm chart wiring

**Date:** 2026-05-19
**Issues:** #512 (Helm chart), #513 (Dockerfile)
**Status:** Approved

## Problem

Epic Games Store sync requires the `legendary` CLI in the production container image and a writable working directory exposed to the binary. Both are absent today:

- The runtime image (`alpine:3.23`) has no `legendary` binary; every Epic sync attempt fails with `epic: legendary not found in PATH`.
- The Helm chart sets neither `LEGENDARY_WORK_DIR` nor a volume at that path; the backend short-circuits with `disabled: true` even after the image is fixed.

Both must land together in a single PR before Epic sync works in-cluster.

## Approach

Two independent file changes, shipped in one PR:

1. **Dockerfile** — add `legendary-gl==0.20.34` to the runtime stage using Alpine's native Python packages for deps and a `--no-deps` pip install for legendary itself.
2. **Helm chart** — add a single `nexorious.legendaryWorkDir` value that, when non-empty, causes the chart to render both `LEGENDARY_WORK_DIR` on the main container and an `emptyDir` volume mounted at that path.

## Change 1 — Dockerfile (issue #513)

### Runtime stage additions

```dockerfile
RUN apk add --no-cache \
      ca-certificates \
      postgresql18-client \
      python3 py3-requests py3-filelock \
 && apk add --no-cache --virtual .pip-tmp py3-pip \
 && pip install --no-cache-dir --break-system-packages --no-deps legendary-gl==0.20.34 \
 && apk del .pip-tmp \
 && addgroup -g 10001 -S nexorious \
 && adduser -u 10001 -S -G nexorious -h /app -s /sbin/nologin nexorious
```

The `addgroup`/`adduser` lines are already in the current `RUN` block and remain unchanged; the legendary installation is inserted before them.

### Why this approach

Alpine's package index (`v3.23/main`) ships `py3-requests` (2.33.1) and `py3-filelock` — the only two runtime deps legendary actually imports. `pip install --no-deps` installs legendary without duplicating those packages. `py3-pip` is a build-time dependency removed via `--virtual` immediately after install.

Alternatives ruled out:
- **`apk add legendary`** — not in any Alpine 3.23 repo (main, community, edge).
- **GitHub release binary** — PyInstaller bundle built against glibc; Alpine is musl. No upstream `SHA256SUMS`.
- **`pip install legendary-gl` with full deps** — works but duplicates copies of `requests`/`filelock`; ~55–60 MB growth vs. ~30 MB.
- **`uv tool install`** — uv itself is ~35 MB on Alpine; multi-stage venv copy is brittle.

### Pin rationale

`legendary-gl==0.20.34` is the final upstream release (Dec 2023). `--no-deps` relies on Alpine 3.23's `py3-requests` staying below 3.0. Both upstream and the Alpine 3.23 line are effectively frozen, so this is safe for the foreseeable future. Document the caveat in the PR description.

### Expected image growth

~30 MB.

## Change 2 — Helm chart (issue #512)

### values.yaml

Add `legendaryWorkDir` under `nexorious:`:

```yaml
nexorious:
  # -- Legendary work directory. Set to a writable path to enable Epic Games
  # -- Store sync. When non-empty, renders LEGENDARY_WORK_DIR on the main
  # -- container and mounts an emptyDir volume at this path.
  # -- Leave empty (the default) to leave Epic sync disabled.
  legendaryWorkDir: ""
```

### values.schema.json

Add `legendaryWorkDir` as an optional `string` property inside the `nexorious` object. The `nexorious` object already has `"additionalProperties": false`, so the field must be declared or `helm lint` will fail.

```json
"legendaryWorkDir": {
  "type": "string",
  "description": "Legendary work directory path. Set to enable Epic Games Store sync."
}
```

### Conditional rendering contract

When `nexorious.legendaryWorkDir` is **non-empty**:
- `LEGENDARY_WORK_DIR: <legendaryWorkDir>` is added to `controllers.nexorious.containers.main.env`
- An `emptyDir` volume is created and mounted at `<legendaryWorkDir>` on the `nexorious` main container

When `nexorious.legendaryWorkDir` is **empty** (the default):
- No `LEGENDARY_WORK_DIR` env var is rendered
- No volume or volumeMount is rendered

The `migrate` initContainer is **not** given the env var or volume mount in either case — migrations do not invoke sync workers.

### Implementation mechanism

The conditional logic lives in `_helpers.tpl`. Because bjw-s processes string values in `values.yaml` through `tpl`, the env var entry can reference a helper. For the volume/volumeMount, the chart uses bjw-s's `controllers.nexorious.pod.volumes` and `controllers.nexorious.containers.main.volumeMounts` raw pass-through fields (or an equivalent mechanism) to conditionally inject the emptyDir and its mount. The exact template approach is left to the implementation plan; the contract above is what must be satisfied.

### Security constraints (unchanged)

- `readOnlyRootFilesystem: true` on both the main container and the migrate initContainer stays as-is. The emptyDir is a separate writable volume, not the root filesystem.
- `runAsUser: 10001`, `fsGroup: 10001`, `fsGroupChangePolicy: OnRootMismatch` from `defaultPodOptions` apply to the new volume automatically. The nexorious user can write to it.
- The migrate initContainer does not receive `LEGENDARY_WORK_DIR` and does not need a volume mount.

### Operator experience

To enable Epic sync on a new install:
```yaml
nexorious:
  legendaryWorkDir: /var/lib/legendary
```

To disable: leave `legendaryWorkDir: ""` (or omit it entirely — the default is empty).

## Files to change

| File | Change |
|------|--------|
| `Dockerfile` | Add legendary install to runtime stage `RUN` block |
| `deploy/helm/values.yaml` | Add `nexorious.legendaryWorkDir: ""` |
| `deploy/helm/values.schema.json` | Add `legendaryWorkDir` string property under `nexorious` |
| `deploy/helm/templates/_helpers.tpl` | Add conditional env-var and volume helpers |
| `deploy/helm/templates/common.yaml` | Reference new helpers if needed |

## Acceptance criteria

- `docker run --rm <image> legendary --version` → `0.20.34`
- `helm install ... --set nexorious.legendaryWorkDir=/var/lib/legendary` produces a pod with `LEGENDARY_WORK_DIR=/var/lib/legendary` and a writable emptyDir at `/var/lib/legendary`
- Default install (no `legendaryWorkDir` set) produces no env var and no volume
- Epic sync page shows "Not Configured" (not an error) for an unauthenticated user on a cluster with `legendaryWorkDir` set
- Operator can opt out by leaving `legendaryWorkDir` empty
- `helm lint` passes
- Image size growth documented in the PR (~30 MB)
