# legendary-gl Container Image + Helm Chart Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `legendary-gl 0.20.34` to the container runtime stage and wire a single `nexorious.legendaryWorkDir` Helm value that, when non-empty, conditionally injects both the `LEGENDARY_WORK_DIR` env var and an emptyDir volume into the rendered manifests.

**Architecture:** Two independent file changes in one PR (issues #512 and #513). The Dockerfile change installs legendary via Alpine-native Python packages + `pip install --no-deps`. The Helm change adds a values-mutation helper (`nexorious.injectLegendaryIfEnabled`) called from `common.yaml` before `bjw-s.common.loader.all`, so bjw-s sees the injected persistence entry and env var when the value is set. When `legendaryWorkDir` is empty (the default), nothing extra is rendered.

**Tech Stack:** Docker multi-stage build (Alpine 3.23, apk, pip), Helm 3.19 + bjw-s common library 4.6.2, Sprig template functions (`set`, `dict`, `list`)

---

## File Map

| File | Change |
|------|--------|
| `Dockerfile` | Add legendary install to runtime stage `RUN` block (before addgroup/adduser) |
| `deploy/helm/values.yaml` | Add `nexorious.legendaryWorkDir: ""` with documentation comment |
| `deploy/helm/values.schema.json` | Add `legendaryWorkDir` string property under `nexorious` |
| `deploy/helm/templates/_helpers.tpl` | Add `nexorious.injectLegendaryIfEnabled` helper (appended) |
| `deploy/helm/templates/common.yaml` | Call injection helper before `bjw-s.common.loader.all` |

---

### Task 1: Create feature branch

**Files:** none

- [ ] **Step 1: Create and check out the branch**

```bash
git checkout -b feat/512-513-legendary-epic-sync
```

- [ ] **Step 2: Verify clean state**

```bash
git status
```
Expected: `nothing to commit, working tree clean`

---

### Task 2: Dockerfile — add legendary-gl to runtime stage

**Files:**
- Modify: `Dockerfile:30-34`

The runtime stage currently installs only `ca-certificates` and `postgresql18-client` before creating the nexorious user. legendary-gl's two actual runtime dependencies (`requests`, `filelock`) are available as Alpine packages; pip itself is only needed for the install and is removed immediately via a virtual package.

- [ ] **Step 1: Replace the runtime RUN block**

In `Dockerfile`, replace the existing runtime `RUN` block:

```dockerfile
RUN apk add --no-cache \
      ca-certificates \
      postgresql18-client \
 && addgroup -g 10001 -S nexorious \
 && adduser -u 10001 -S -G nexorious -h /app -s /sbin/nologin nexorious
```

with:

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

- [ ] **Step 2: Commit**

```bash
git add Dockerfile
git commit -m "feat(image): add legendary-gl 0.20.34 to runtime stage (#513)"
```

---

### Task 3: Helm — establish verification baseline

**Files:** none (observation only)

Run these checks before any Helm changes so you know what the failing state looks like.

- [ ] **Step 1: Confirm `legendaryWorkDir` is rejected by the schema**

```bash
helm lint deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  --set nexorious.legendaryWorkDir=/var/lib/legendary
```
Expected: error output containing `Additional property legendaryWorkDir is not allowed`

- [ ] **Step 2: Confirm default render has no legendary output**

```bash
helm template test deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  | grep -c 'LEGENDARY'
```
Expected: `0`

---

### Task 4: Helm — add legendaryWorkDir to values.yaml and schema

**Files:**
- Modify: `deploy/helm/values.yaml`
- Modify: `deploy/helm/values.schema.json`

- [ ] **Step 1: Add `legendaryWorkDir` to values.yaml**

In `deploy/helm/values.yaml`, after the `databaseUrl: ""` line (around line 35), insert:

```yaml
  # -- Legendary work directory. Set to a writable absolute path to enable
  # -- Epic Games Store sync (e.g. /var/lib/legendary). When non-empty, the
  # -- chart injects LEGENDARY_WORK_DIR on the main container and mounts an
  # -- emptyDir volume at this path. Leave empty to disable Epic sync.
  legendaryWorkDir: ""
```

- [ ] **Step 2: Add `legendaryWorkDir` to values.schema.json**

In `deploy/helm/values.schema.json`, inside the `nexorious.properties` object, after the `"databaseUrl"` property block and before `"secretKeyFrom"`, insert:

```json
        "legendaryWorkDir": {
          "type": "string",
          "description": "Legendary work directory path. Set to enable Epic Games Store sync."
        },
```

- [ ] **Step 3: Verify `helm lint` now accepts the value without rendering legendary resources**

```bash
helm lint deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  --set nexorious.legendaryWorkDir=/var/lib/legendary
```
Expected: `1 chart(s) linted, 0 chart(s) failed`

```bash
helm template test deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  --set nexorious.legendaryWorkDir=/var/lib/legendary \
  | grep -c 'LEGENDARY'
```
Expected: `0` — the schema accepts the value, but nothing is rendered yet (helper doesn't exist yet).

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/values.yaml deploy/helm/values.schema.json
git commit -m "feat(helm): add nexorious.legendaryWorkDir value and schema entry (#512)"
```

---

### Task 5: Helm — add injection helper to _helpers.tpl

**Files:**
- Modify: `deploy/helm/templates/_helpers.tpl` (append to end of file)

The helper mutates `.Values` in-place before bjw-s reads it. `set` on a Go map mutates the original because maps are reference types. The helper is a no-op when `legendaryWorkDir` is empty, so default installs are unaffected.

- [ ] **Step 1: Append the helper to `_helpers.tpl`**

Add the following to the end of `deploy/helm/templates/_helpers.tpl`:

```
{{/*
Inject legendary persistence entry and LEGENDARY_WORK_DIR env var when
nexorious.legendaryWorkDir is set. Call this BEFORE bjw-s.common.loader.all
so the loader sees the injected values.
*/}}
{{- define "nexorious.injectLegendaryIfEnabled" -}}
{{- if .Values.nexorious.legendaryWorkDir -}}
  {{- $_ := set .Values.persistence "legendary" (dict
      "enabled"        true
      "type"           "emptyDir"
      "advancedMounts" (dict
        "nexorious" (dict
          "main" (list (dict "path" .Values.nexorious.legendaryWorkDir))
        )
      )
  ) -}}
  {{- $_ = set .Values.controllers.nexorious.containers.main.env
      "LEGENDARY_WORK_DIR" .Values.nexorious.legendaryWorkDir -}}
{{- end -}}
{{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/templates/_helpers.tpl
git commit -m "feat(helm): add conditional legendary volume+env injection helper (#512)"
```

---

### Task 6: Helm — call injection helper from common.yaml

**Files:**
- Modify: `deploy/helm/templates/common.yaml`

`common.yaml` currently contains a single line. The injection helper must run before the bjw-s loader so the loader sees the mutated `.Values`.

- [ ] **Step 1: Edit `common.yaml`**

Replace the entire contents of `deploy/helm/templates/common.yaml` with:

```yaml
{{- include "nexorious.injectLegendaryIfEnabled" . -}}
{{- include "bjw-s.common.loader.all" . -}}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/templates/common.yaml
git commit -m "feat(helm): invoke legendary injection helper before bjw-s loader (#512)"
```

---

### Task 7: Verify Helm rendering

**Files:** none (verification only)

- [ ] **Step 1: Verify default install produces no legendary output**

```bash
helm template test deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  | grep -c 'LEGENDARY'
```
Expected: `0`

```bash
helm template test deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  | grep -c 'legendary'
```
Expected: `0`

- [ ] **Step 2: Verify `legendaryWorkDir` set renders the env var on the main container**

```bash
helm template test deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  --set nexorious.legendaryWorkDir=/var/lib/legendary \
  | grep -A1 'LEGENDARY_WORK_DIR'
```
Expected output includes:
```
        - name: LEGENDARY_WORK_DIR
          value: /var/lib/legendary
```

- [ ] **Step 3: Verify the emptyDir volume and mount appear**

```bash
helm template test deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  --set nexorious.legendaryWorkDir=/var/lib/legendary \
  | grep -A3 'legendary'
```
Expected: output shows a `legendary` volume entry (emptyDir) and a volumeMount with `mountPath: /var/lib/legendary`

- [ ] **Step 4: Verify the migrate initContainer does NOT receive the env var**

```bash
helm template test deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  --set nexorious.legendaryWorkDir=/var/lib/legendary \
  | grep -B10 'LEGENDARY_WORK_DIR' | grep 'migrate'
```
Expected: empty output (the `LEGENDARY_WORK_DIR` env var does not appear in a block that names the migrate container)

- [ ] **Step 5: Verify `helm lint` passes for both default and enabled cases**

```bash
helm lint deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass
```
Expected: `1 chart(s) linted, 0 chart(s) failed`

```bash
helm lint deploy/helm/ \
  --set nexorious.secretKey=test-key-not-change-me-ok \
  --set nexorious.igdbClientId=testid \
  --set nexorious.igdbClientSecret=testsecret \
  --set nexorious.postgresql.password=testpass \
  --set nexorious.legendaryWorkDir=/var/lib/legendary
```
Expected: `1 chart(s) linted, 0 chart(s) failed`

---

### Task 8: Open PR

**Files:** none

- [ ] **Step 1: Push branch and open PR**

```bash
git push -u origin feat/512-513-legendary-epic-sync
gh pr create \
  --title "feat: add legendary-gl to container image and expose via Helm chart" \
  --body "$(cat <<'EOF'
## Summary

- **#513 (image)**: Adds `legendary-gl 0.20.34` to the Alpine 3.23 runtime stage. Alpine's native `py3-requests` and `py3-filelock` cover legendary's two actual runtime deps; `pip install --no-deps` installs legendary itself without duplicates. `py3-pip` is removed immediately via `--virtual`. Estimated image growth: ~30 MB.
- **#512 (chart)**: Adds `nexorious.legendaryWorkDir` as a single Helm value. When non-empty, a `_helpers.tpl` mutation helper injects \`LEGENDARY_WORK_DIR\` onto the nexorious main container and a \`persistence.legendary\` emptyDir entry before bjw-s renders the Deployment. When empty (the default), nothing extra is rendered.

## Caveat

\`--no-deps\` relies on Alpine 3.23's \`py3-requests\` staying below 3.0. Both legendary upstream (last release Dec 2023) and the Alpine 3.23 line are static, so this is safe for the foreseeable future.

## Test plan

- [ ] `docker build --target runtime -t nexorious-test .` succeeds
- [ ] `docker run --rm nexorious-test legendary --version` → `legendary-gl version 0.20.34`
- [ ] `helm lint deploy/helm/` passes (default and with `legendaryWorkDir` set)
- [ ] `helm template` without `legendaryWorkDir` renders no `LEGENDARY_WORK_DIR` env var and no legendary volume
- [ ] `helm template --set nexorious.legendaryWorkDir=/var/lib/legendary` renders the env var on the nexorious main container and an emptyDir mount at `/var/lib/legendary`
- [ ] The migrate initContainer does not receive `LEGENDARY_WORK_DIR` or the volume mount

Closes #512
Closes #513

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
