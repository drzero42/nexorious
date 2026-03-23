# Helm External Secret Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow any secret-valued Helm field to be sourced from an existing Kubernetes Secret via a `*From` pattern, supporting both DB URI mode and individual DB keys mode.

**Architecture:** Replace the single `envFrom: secretRef: nexorious-credentials` in each app container with individual `env.valueFrom.secretKeyRef` entries, where each secret name and key are resolved by helper templates that check whether a `*From` is configured. The managed Secret becomes conditional, omitting any field that is externalized.

**Tech Stack:** Helm 3, bjw-s common library v4.6.2, Go templates (`_helpers.tpl`), `helm lint`, `helm template`.

**Spec:** `docs/superpowers/specs/2026-03-23-helm-external-secret-support-design.md`

---

## File Map

| File | What changes |
|---|---|
| `deploy/helm/values.yaml` | Add 11 `*From` structs; replace `envFrom` with `env.valueFrom.secretKeyRef` in api/worker/scheduler |
| `deploy/helm/values.schema.json` | Add 11 `*From` object fields; remove `minLength: 1` from 4 inline credential fields |
| `deploy/helm/templates/_helpers.tpl` | Add 20 name/key helpers; extend `nexorious.validateValues` |
| `deploy/helm/templates/credentials-secret.yaml` | Conditionally omit externalized fields |

---

## Task 0: Create feature branch

- [ ] **Step 1: Ensure main is up to date and push any unpushed commits**

```bash
cd /home/abo/workspace/home/nexorious
git push
```

- [ ] **Step 2: Create and check out the feature branch**

```bash
cd /home/abo/workspace/home/nexorious
git checkout -b feat/helm-external-secret-support
```

- [ ] **Step 3: Verify you are on the new branch**

```bash
git branch --show-current
```

Expected: `feat/helm-external-secret-support`

---

## Task 1: Gate — verify bjw-s tpl rendering and `optional` passthrough

**Why first:** The entire env-injection approach depends on bjw-s rendering `valueFrom.secretKeyRef.name` and `.key` strings through `tpl`, and passing `optional: true` through to the rendered pod spec. If either doesn't work, stop here and redesign.

**Files:**
- Modify (temporarily): `deploy/helm/values.yaml`

- [ ] **Step 1: Add a test env var to the api container in values.yaml**

Under `controllers.api.containers.main.env`, add alongside the existing env vars:

```yaml
        env:
          CORS_ORIGINS: ""
          STORAGE_PATH: /app/storage
          LOG_LEVEL: INFO
          DEBUG: "false"
          _TPL_TEST:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.fullname" . }}-credentials'
                key: TEST_KEY
                optional: true
```

- [ ] **Step 2: Run helm template and verify the field renders**

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  2>&1 | grep -A 6 '_TPL_TEST'
```

Expected output (name must be `nexorious-credentials`, not the literal template string; `optional: true` must appear):
```yaml
        - name: _TPL_TEST
          valueFrom:
            secretKeyRef:
              name: nexorious-credentials
              key: TEST_KEY
              optional: true
```

**If the name shows as the raw template string** (e.g. `'{{ include "nexorious.fullname" . }}-credentials'`) or `optional: true` is absent: **STOP. The bjw-s tpl approach will not work.** Surface this to the user — the plan needs a redesign before proceeding.

- [ ] **Step 3: Remove the test env var from values.yaml**

Delete the `_TPL_TEST` block added in Step 1.

- [ ] **Step 4: No commit needed — the revert in Step 3 leaves the file unchanged**

---

## Task 2: Add `*From` fields to `values.yaml`

**Files:**
- Modify: `deploy/helm/values.yaml`

- [ ] **Step 1: Verify baseline lint passes before any changes**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```

Expected: `0 chart(s) failed`

- [ ] **Step 2: Add the 11 `*From` fields under `nexorious` in values.yaml**

Insert after `nexorious.internalApiUrl: ""` (before the `postgresql:` block):

```yaml
  # -- External secret references (*From pattern)
  # When *From.name and *From.key are both non-empty, the external secret is used
  # and the corresponding inline value is ignored.

  # Non-DB credentials
  secretKeyFrom:
    name: ""
    key: ""
  internalApiKeyFrom:
    name: ""
    key: ""
  igdbClientIdFrom:
    name: ""
    key: ""
  igdbClientSecretFrom:
    name: ""
    key: ""

  # Database: URI mode (single key containing full DATABASE_URL)
  # Mutually exclusive with databaseUrl and db*From.
  databaseUrlFrom:
    name: ""
    key: ""

  # Database: individual keys mode (DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME)
  # Mutually exclusive with databaseUrl and databaseUrlFrom.
  # No inline equivalents — individual keys always come from an external secret.
  dbHostFrom:
    name: ""
    key: ""
  dbPortFrom:
    name: ""
    key: ""
  dbUserFrom:
    name: ""
    key: ""
  dbPasswordFrom:
    name: ""
    key: ""
  dbNameFrom:
    name: ""
    key: ""
```

- [ ] **Step 3: Lint still passes (schema will reject unknown fields until Task 3)**

Run lint — it will fail with a schema error because the new fields aren't in the schema yet. This is expected and confirms the schema is enforced:

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```

Expected: failure mentioning `Additional property ... is not allowed` — confirms schema validation is active.

---

## Task 3: Update `values.schema.json`

**Files:**
- Modify: `deploy/helm/values.schema.json`

- [ ] **Step 1: Remove `minLength: 1` from the four inline credential fields**

In `values.schema.json`, change `secretKey`, `internalApiKey`, `igdbClientId`, `igdbClientSecret` properties — remove the `"minLength": 1` line from each. The fields remain `"type": "string"`. Runtime validation in `_helpers.tpl` takes over completeness checking.

Before (example for `secretKey`):
```json
"secretKey": {
  "type": "string",
  "minLength": 1,
  "description": "JWT signing secret key. Generate with: openssl rand -hex 32"
},
```

After:
```json
"secretKey": {
  "type": "string",
  "description": "JWT signing secret key. Generate with: openssl rand -hex 32"
},
```

Apply the same change to `internalApiKey`, `igdbClientId`, `igdbClientSecret`.

- [ ] **Step 2: Add the 11 `*From` fields to the schema**

Inside the `nexorious.properties` object (after the `postgresql` block), add:

```json
"secretKeyFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"internalApiKeyFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"igdbClientIdFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"igdbClientSecretFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"databaseUrlFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"dbHostFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"dbPortFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"dbUserFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"dbPasswordFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
},
"dbNameFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
}
```

- [ ] **Step 3: Verify lint passes with defaults**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```

Expected: `0 chart(s) failed`

- [ ] **Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/values.yaml deploy/helm/values.schema.json
git commit -m "feat(helm): add *From fields to values and schema"
```

---

## Task 4: Add non-DB credential helper templates

**Files:**
- Modify: `deploy/helm/templates/_helpers.tpl`

These helpers follow a strict pattern: if `*From.name` is set, return the external secret name/key; otherwise return the managed secret name and default key.

- [ ] **Step 1: Add the 8 helpers for non-DB credentials to `_helpers.tpl`**

Append after the existing `nexorious.internalApiUrl` helper:

```
{{/*
Helpers for resolving secret sources via the *From pattern.
Each credential has two helpers: SecretName and SecretKey.
When *From.name is non-empty, the external secret is used.
Otherwise, falls back to the managed nexorious-credentials secret.
*/}}

{{- define "nexorious.secretKeySecretName" -}}
{{- if and .Values.nexorious.secretKeyFrom .Values.nexorious.secretKeyFrom.name -}}
{{- .Values.nexorious.secretKeyFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.secretKeySecretKey" -}}
{{- if and .Values.nexorious.secretKeyFrom .Values.nexorious.secretKeyFrom.key -}}
{{- .Values.nexorious.secretKeyFrom.key -}}
{{- else -}}
SECRET_KEY
{{- end -}}
{{- end }}

{{- define "nexorious.internalApiKeySecretName" -}}
{{- if and .Values.nexorious.internalApiKeyFrom .Values.nexorious.internalApiKeyFrom.name -}}
{{- .Values.nexorious.internalApiKeyFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.internalApiKeySecretKey" -}}
{{- if and .Values.nexorious.internalApiKeyFrom .Values.nexorious.internalApiKeyFrom.key -}}
{{- .Values.nexorious.internalApiKeyFrom.key -}}
{{- else -}}
INTERNAL_API_KEY
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientIdSecretName" -}}
{{- if and .Values.nexorious.igdbClientIdFrom .Values.nexorious.igdbClientIdFrom.name -}}
{{- .Values.nexorious.igdbClientIdFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientIdSecretKey" -}}
{{- if and .Values.nexorious.igdbClientIdFrom .Values.nexorious.igdbClientIdFrom.key -}}
{{- .Values.nexorious.igdbClientIdFrom.key -}}
{{- else -}}
IGDB_CLIENT_ID
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientSecretSecretName" -}}
{{- if and .Values.nexorious.igdbClientSecretFrom .Values.nexorious.igdbClientSecretFrom.name -}}
{{- .Values.nexorious.igdbClientSecretFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.igdbClientSecretSecretKey" -}}
{{- if and .Values.nexorious.igdbClientSecretFrom .Values.nexorious.igdbClientSecretFrom.key -}}
{{- .Values.nexorious.igdbClientSecretFrom.key -}}
{{- else -}}
IGDB_CLIENT_SECRET
{{- end -}}
{{- end }}
```

- [ ] **Step 2: Verify helm template renders the helpers correctly for the default case**

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  2>&1 | grep -c 'nexorious-credentials'
```

Expected: a non-zero count (the managed secret is still referenced multiple times in the rendered output).

- [ ] **Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/templates/_helpers.tpl
git commit -m "feat(helm): add non-DB credential *From helper templates"
```

---

## Task 5: Add DB credential helper templates

**Files:**
- Modify: `deploy/helm/templates/_helpers.tpl`

`DATABASE_URL` falls back to the managed secret (which conditionally omits the key in individual-keys mode — handled in Task 7). Individual DB var helpers fall back to the managed secret name with a sentinel key `_nexorious_unused`; `optional: true` on those env vars means they are silently skipped when the key doesn't exist.

- [ ] **Step 1: Add the 12 DB credential helpers**

Append to `_helpers.tpl`:

```
{{- define "nexorious.databaseUrlSecretName" -}}
{{- if and .Values.nexorious.databaseUrlFrom .Values.nexorious.databaseUrlFrom.name -}}
{{- .Values.nexorious.databaseUrlFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.databaseUrlSecretKey" -}}
{{- if and .Values.nexorious.databaseUrlFrom .Values.nexorious.databaseUrlFrom.key -}}
{{- .Values.nexorious.databaseUrlFrom.key -}}
{{- else -}}
DATABASE_URL
{{- end -}}
{{- end }}

{{- define "nexorious.dbHostSecretName" -}}
{{- if and .Values.nexorious.dbHostFrom .Values.nexorious.dbHostFrom.name -}}
{{- .Values.nexorious.dbHostFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbHostSecretKey" -}}
{{- if and .Values.nexorious.dbHostFrom .Values.nexorious.dbHostFrom.key -}}
{{- .Values.nexorious.dbHostFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbPortSecretName" -}}
{{- if and .Values.nexorious.dbPortFrom .Values.nexorious.dbPortFrom.name -}}
{{- .Values.nexorious.dbPortFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbPortSecretKey" -}}
{{- if and .Values.nexorious.dbPortFrom .Values.nexorious.dbPortFrom.key -}}
{{- .Values.nexorious.dbPortFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbUserSecretName" -}}
{{- if and .Values.nexorious.dbUserFrom .Values.nexorious.dbUserFrom.name -}}
{{- .Values.nexorious.dbUserFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbUserSecretKey" -}}
{{- if and .Values.nexorious.dbUserFrom .Values.nexorious.dbUserFrom.key -}}
{{- .Values.nexorious.dbUserFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbPasswordSecretName" -}}
{{- if and .Values.nexorious.dbPasswordFrom .Values.nexorious.dbPasswordFrom.name -}}
{{- .Values.nexorious.dbPasswordFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbPasswordSecretKey" -}}
{{- if and .Values.nexorious.dbPasswordFrom .Values.nexorious.dbPasswordFrom.key -}}
{{- .Values.nexorious.dbPasswordFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}

{{- define "nexorious.dbNameSecretName" -}}
{{- if and .Values.nexorious.dbNameFrom .Values.nexorious.dbNameFrom.name -}}
{{- .Values.nexorious.dbNameFrom.name -}}
{{- else -}}
{{- include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbNameSecretKey" -}}
{{- if and .Values.nexorious.dbNameFrom .Values.nexorious.dbNameFrom.key -}}
{{- .Values.nexorious.dbNameFrom.key -}}
{{- else -}}
_nexorious_unused
{{- end -}}
{{- end }}
```

- [ ] **Step 2: Lint passes**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```

Expected: `0 chart(s) failed`

- [ ] **Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/templates/_helpers.tpl
git commit -m "feat(helm): add DB credential *From helper templates"
```

---

## Task 6: Extend validation in `_helpers.tpl`

**Files:**
- Modify: `deploy/helm/templates/_helpers.tpl`

- [ ] **Step 1: Replace the existing `nexorious.validateValues` define block in `_helpers.tpl` with two top-level blocks**

**Important:** Helm does not support nested `define` blocks. The `_validateFrom` helper must be a **separate** top-level block placed BEFORE `validateValues` in the file. Replace the single existing `nexorious.validateValues` define block with these two consecutive blocks:

```
{{/*
Check *From completeness — both name and key must be set together.
Call as: include "nexorious._validateFrom" (list "nexorious.secretKeyFrom" .Values.nexorious.secretKeyFrom)
*/}}
{{- define "nexorious._validateFrom" -}}
  {{- $label := index . 0 -}}
  {{- $from := index . 1 -}}
  {{- if and $from (or $from.name $from.key) -}}
    {{- if empty $from.name -}}
      {{- fail (printf "%s.name must be set when %s.key is set" $label $label) -}}
    {{- end -}}
    {{- if empty $from.key -}}
      {{- fail (printf "%s.key must be set when %s.name is set" $label $label) -}}
    {{- end -}}
  {{- end -}}
{{- end }}

{{/*
Validate required values.
*/}}
{{- define "nexorious.validateValues" -}}

{{/* Validate *From completeness for all fields */}}
{{- include "nexorious._validateFrom" (list "nexorious.secretKeyFrom" .Values.nexorious.secretKeyFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.internalApiKeyFrom" .Values.nexorious.internalApiKeyFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.igdbClientIdFrom" .Values.nexorious.igdbClientIdFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.igdbClientSecretFrom" .Values.nexorious.igdbClientSecretFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.databaseUrlFrom" .Values.nexorious.databaseUrlFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.dbHostFrom" .Values.nexorious.dbHostFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.dbPortFrom" .Values.nexorious.dbPortFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.dbUserFrom" .Values.nexorious.dbUserFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.dbPasswordFrom" .Values.nexorious.dbPasswordFrom) }}
{{- include "nexorious._validateFrom" (list "nexorious.dbNameFrom" .Values.nexorious.dbNameFrom) }}

{{/* Non-DB credentials: skip inline checks when *From is configured */}}
{{- if not (and .Values.nexorious.secretKeyFrom .Values.nexorious.secretKeyFrom.name) }}
  {{- if eq .Values.nexorious.secretKey "change-me-in-production" }}
    {{- fail "nexorious.secretKey must be set to a secure random value" }}
  {{- end }}
{{- end }}
{{- if not (and .Values.nexorious.internalApiKeyFrom .Values.nexorious.internalApiKeyFrom.name) }}
  {{- if eq .Values.nexorious.internalApiKey "change-me-in-production" }}
    {{- fail "nexorious.internalApiKey must be set to a secure random value" }}
  {{- end }}
{{- end }}
{{- if not (and .Values.nexorious.igdbClientIdFrom .Values.nexorious.igdbClientIdFrom.name) }}
  {{- if empty .Values.nexorious.igdbClientId }}
    {{- fail "nexorious.igdbClientId is required. Nexorious will not function without valid IGDB credentials." }}
  {{- end }}
{{- end }}
{{- if not (and .Values.nexorious.igdbClientSecretFrom .Values.nexorious.igdbClientSecretFrom.name) }}
  {{- if empty .Values.nexorious.igdbClientSecret }}
    {{- fail "nexorious.igdbClientSecret is required. Nexorious will not function without valid IGDB credentials." }}
  {{- end }}
{{- end }}

{{/* DB mode exclusivity: at most one of databaseUrl, databaseUrlFrom, or db*From may be active */}}
{{- $uriDirect := not (empty .Values.nexorious.databaseUrl) }}
{{- $uriFrom := and .Values.nexorious.databaseUrlFrom .Values.nexorious.databaseUrlFrom.name | default false }}
{{- $indivFrom := or
    (and .Values.nexorious.dbHostFrom .Values.nexorious.dbHostFrom.name)
    (and .Values.nexorious.dbPortFrom .Values.nexorious.dbPortFrom.name)
    (and .Values.nexorious.dbUserFrom .Values.nexorious.dbUserFrom.name)
    (and .Values.nexorious.dbPasswordFrom .Values.nexorious.dbPasswordFrom.name)
    (and .Values.nexorious.dbNameFrom .Values.nexorious.dbNameFrom.name) }}
{{- $dbModeCount := add (ternary 1 0 $uriDirect) (ternary 1 0 (not (not $uriFrom))) (ternary 1 0 (not (not $indivFrom))) }}
{{- if gt $dbModeCount 1 }}
  {{- fail "Only one DB mode may be configured: nexorious.databaseUrl, nexorious.databaseUrlFrom, or nexorious.db*From" }}
{{- end }}

{{/* postgresql-disabled: require some DB mode when postgresql controller is disabled */}}
{{- if not (dig "postgresql" "enabled" true .Values.controllers) }}
  {{- if not (or $uriDirect (not (not $uriFrom)) (not (not $indivFrom))) }}
    {{- fail "A database connection must be configured when the postgresql controller is disabled: set nexorious.databaseUrl, nexorious.databaseUrlFrom, or nexorious.db*From" }}
  {{- end }}
{{- end }}

{{/* nats-disabled: require natsUrl when nats controller is disabled */}}
{{- if and (not (dig "nats" "enabled" true .Values.controllers)) (empty .Values.nexorious.natsUrl) }}
  {{- fail "nexorious.natsUrl must be set when the nats controller is disabled" }}
{{- end }}

{{- end }}
```

- [ ] **Step 2: Verify lint passes with defaults**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```

Expected: `0 chart(s) failed`

- [ ] **Step 3: Verify validation catches missing *From.key**

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set nexorious.secretKeyFrom.name=my-secret \
  2>&1 | grep -i "key must be set"
```

Expected: output containing `nexorious.secretKeyFrom.key must be set when nexorious.secretKeyFrom.name is set`

- [ ] **Step 4: Verify validation catches mixed DB modes**

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set nexorious.databaseUrl=postgresql://x:x@host/db \
  --set nexorious.databaseUrlFrom.name=my-secret \
  --set nexorious.databaseUrlFrom.key=uri \
  2>&1 | grep -i "Only one DB mode"
```

Expected: output containing `Only one DB mode may be configured`

- [ ] **Step 5: Verify bypass works — igdbClientId not required when igdbClientIdFrom is set**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientIdFrom.name=my-secret \
  --set nexorious.igdbClientIdFrom.key=client_id \
  --set nexorious.igdbClientSecretFrom.name=my-secret \
  --set nexorious.igdbClientSecretFrom.key=client_secret
```

Expected: `0 chart(s) failed` (no error about missing igdbClientId/igdbClientSecret)

- [ ] **Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/templates/_helpers.tpl
git commit -m "feat(helm): extend validation for *From pattern and DB mode exclusivity"
```

---

## Task 7: Update `credentials-secret.yaml` — conditional fields

**Files:**
- Modify: `deploy/helm/templates/credentials-secret.yaml`

- [ ] **Step 1: Replace the stringData block with the conditional version**

Replace the entire file content with:

```yaml
{{- include "nexorious.validateValues" . }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "nexorious.credentialsSecretName" . }}
  labels:
    app.kubernetes.io/name: {{ include "nexorious.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
stringData:
  {{- if not (and .Values.nexorious.secretKeyFrom .Values.nexorious.secretKeyFrom.name) }}
  SECRET_KEY: {{ .Values.nexorious.secretKey | quote }}
  {{- end }}
  {{- if not (and .Values.nexorious.internalApiKeyFrom .Values.nexorious.internalApiKeyFrom.name) }}
  INTERNAL_API_KEY: {{ .Values.nexorious.internalApiKey | quote }}
  {{- end }}
  {{- if not (and .Values.nexorious.igdbClientIdFrom .Values.nexorious.igdbClientIdFrom.name) }}
  IGDB_CLIENT_ID: {{ .Values.nexorious.igdbClientId | quote }}
  {{- end }}
  {{- if not (and .Values.nexorious.igdbClientSecretFrom .Values.nexorious.igdbClientSecretFrom.name) }}
  IGDB_CLIENT_SECRET: {{ .Values.nexorious.igdbClientSecret | quote }}
  {{- end }}
  {{- $databaseUrlExternalized := and .Values.nexorious.databaseUrlFrom .Values.nexorious.databaseUrlFrom.name }}
  {{- $indivKeysMode := or
      (and .Values.nexorious.dbHostFrom .Values.nexorious.dbHostFrom.name)
      (and .Values.nexorious.dbPortFrom .Values.nexorious.dbPortFrom.name)
      (and .Values.nexorious.dbUserFrom .Values.nexorious.dbUserFrom.name)
      (and .Values.nexorious.dbPasswordFrom .Values.nexorious.dbPasswordFrom.name)
      (and .Values.nexorious.dbNameFrom .Values.nexorious.dbNameFrom.name) }}
  {{- if not (or $databaseUrlExternalized $indivKeysMode) }}
  DATABASE_URL: {{ include "nexorious.databaseUrl" . | quote }}
  {{- end }}
  POSTGRES_PASSWORD: {{ .Values.nexorious.postgresql.password | quote }}
  NATS_URL: {{ include "nexorious.natsUrl" . | quote }}
  INTERNAL_API_URL: {{ include "nexorious.internalApiUrl" . | quote }}
```

- [ ] **Step 2: Verify the managed secret omits DATABASE_URL in URI-from mode**

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set nexorious.databaseUrlFrom.name=db-secret \
  --set nexorious.databaseUrlFrom.key=uri \
  --set 'controllers.postgresql.enabled=false' \
  --set 'service.postgresql.enabled=false' \
  --set 'persistence.postgresql-data.enabled=false' \
  2>&1 | grep -A 20 'kind: Secret' | grep 'DATABASE_URL'
```

Expected: no output (DATABASE_URL is absent from the managed secret)

- [ ] **Step 3: Verify SECRET_KEY is absent when secretKeyFrom is set**

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKeyFrom.name=my-secret \
  --set nexorious.secretKeyFrom.key=sk \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  2>&1 | grep -A 20 'kind: Secret' | grep 'SECRET_KEY'
```

Expected: no output

- [ ] **Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/templates/credentials-secret.yaml
git commit -m "feat(helm): conditionally omit externalized fields from managed secret"
```

---

## Task 8: Replace `envFrom` with individual `env` entries in controllers

**Files:**
- Modify: `deploy/helm/values.yaml`

This is the largest change. Replace `envFrom: secretRef: nexorious-credentials` in all three app containers (api, worker, scheduler) with individual `env.valueFrom.secretKeyRef` entries referencing the helper templates from Tasks 4 and 5.

- [ ] **Step 1: Update the api container**

In `controllers.api.containers.main`, remove the `envFrom` block:
```yaml
        envFrom:
          - secretRef:
              name: nexorious-credentials
```

And expand the `env` block to include credential entries alongside the existing non-secret vars:

```yaml
        env:
          SECRET_KEY:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.secretKeySecretName" . }}'
                key: '{{ include "nexorious.secretKeySecretKey" . }}'
          INTERNAL_API_KEY:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.internalApiKeySecretName" . }}'
                key: '{{ include "nexorious.internalApiKeySecretKey" . }}'
          IGDB_CLIENT_ID:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.igdbClientIdSecretName" . }}'
                key: '{{ include "nexorious.igdbClientIdSecretKey" . }}'
          IGDB_CLIENT_SECRET:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.igdbClientSecretSecretName" . }}'
                key: '{{ include "nexorious.igdbClientSecretSecretKey" . }}'
          DATABASE_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.databaseUrlSecretName" . }}'
                key: '{{ include "nexorious.databaseUrlSecretKey" . }}'
                optional: true
          DB_HOST:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbHostSecretName" . }}'
                key: '{{ include "nexorious.dbHostSecretKey" . }}'
                optional: true
          DB_PORT:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbPortSecretName" . }}'
                key: '{{ include "nexorious.dbPortSecretKey" . }}'
                optional: true
          DB_USER:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbUserSecretName" . }}'
                key: '{{ include "nexorious.dbUserSecretKey" . }}'
                optional: true
          DB_PASSWORD:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbPasswordSecretName" . }}'
                key: '{{ include "nexorious.dbPasswordSecretKey" . }}'
                optional: true
          DB_NAME:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbNameSecretName" . }}'
                key: '{{ include "nexorious.dbNameSecretKey" . }}'
                optional: true
          NATS_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.fullname" . }}-credentials'
                key: NATS_URL
          INTERNAL_API_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.fullname" . }}-credentials'
                key: INTERNAL_API_URL
          CORS_ORIGINS: ""
          STORAGE_PATH: /app/storage
          LOG_LEVEL: INFO
          DEBUG: "false"
```

- [ ] **Step 2: Update the worker container**

Remove `envFrom` from `controllers.worker.containers.main` and add the same credential env block (minus `CORS_ORIGINS` and `DEBUG`; keep `STORAGE_PATH`, `TASKIQ_SKIP_TABLE_CREATION`, `LOG_LEVEL`):

```yaml
        env:
          SECRET_KEY:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.secretKeySecretName" . }}'
                key: '{{ include "nexorious.secretKeySecretKey" . }}'
          INTERNAL_API_KEY:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.internalApiKeySecretName" . }}'
                key: '{{ include "nexorious.internalApiKeySecretKey" . }}'
          IGDB_CLIENT_ID:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.igdbClientIdSecretName" . }}'
                key: '{{ include "nexorious.igdbClientIdSecretKey" . }}'
          IGDB_CLIENT_SECRET:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.igdbClientSecretSecretName" . }}'
                key: '{{ include "nexorious.igdbClientSecretSecretKey" . }}'
          DATABASE_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.databaseUrlSecretName" . }}'
                key: '{{ include "nexorious.databaseUrlSecretKey" . }}'
                optional: true
          DB_HOST:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbHostSecretName" . }}'
                key: '{{ include "nexorious.dbHostSecretKey" . }}'
                optional: true
          DB_PORT:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbPortSecretName" . }}'
                key: '{{ include "nexorious.dbPortSecretKey" . }}'
                optional: true
          DB_USER:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbUserSecretName" . }}'
                key: '{{ include "nexorious.dbUserSecretKey" . }}'
                optional: true
          DB_PASSWORD:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbPasswordSecretName" . }}'
                key: '{{ include "nexorious.dbPasswordSecretKey" . }}'
                optional: true
          DB_NAME:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbNameSecretName" . }}'
                key: '{{ include "nexorious.dbNameSecretKey" . }}'
                optional: true
          NATS_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.fullname" . }}-credentials'
                key: NATS_URL
          INTERNAL_API_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.fullname" . }}-credentials'
                key: INTERNAL_API_URL
          STORAGE_PATH: /app/storage
          TASKIQ_SKIP_TABLE_CREATION: "true"
          LOG_LEVEL: INFO
```

- [ ] **Step 3: Update the scheduler container**

Remove `envFrom` from `controllers.scheduler.containers.main` and add the same credential block (keep `LOG_LEVEL`):

```yaml
        env:
          SECRET_KEY:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.secretKeySecretName" . }}'
                key: '{{ include "nexorious.secretKeySecretKey" . }}'
          INTERNAL_API_KEY:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.internalApiKeySecretName" . }}'
                key: '{{ include "nexorious.internalApiKeySecretKey" . }}'
          IGDB_CLIENT_ID:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.igdbClientIdSecretName" . }}'
                key: '{{ include "nexorious.igdbClientIdSecretKey" . }}'
          IGDB_CLIENT_SECRET:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.igdbClientSecretSecretName" . }}'
                key: '{{ include "nexorious.igdbClientSecretSecretKey" . }}'
          DATABASE_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.databaseUrlSecretName" . }}'
                key: '{{ include "nexorious.databaseUrlSecretKey" . }}'
                optional: true
          DB_HOST:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbHostSecretName" . }}'
                key: '{{ include "nexorious.dbHostSecretKey" . }}'
                optional: true
          DB_PORT:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbPortSecretName" . }}'
                key: '{{ include "nexorious.dbPortSecretKey" . }}'
                optional: true
          DB_USER:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbUserSecretName" . }}'
                key: '{{ include "nexorious.dbUserSecretKey" . }}'
                optional: true
          DB_PASSWORD:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbPasswordSecretName" . }}'
                key: '{{ include "nexorious.dbPasswordSecretKey" . }}'
                optional: true
          DB_NAME:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbNameSecretName" . }}'
                key: '{{ include "nexorious.dbNameSecretKey" . }}'
                optional: true
          NATS_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.fullname" . }}-credentials'
                key: NATS_URL
          INTERNAL_API_URL:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.fullname" . }}-credentials'
                key: INTERNAL_API_URL
          LOG_LEVEL: INFO
```

- [ ] **Step 4: Lint passes**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```

Expected: `0 chart(s) failed`

- [ ] **Step 5: Verify SECRET_KEY env var references the managed secret in default mode**

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  2>&1 | grep -A 10 'name: SECRET_KEY'
```

Expected: contains `name: nexorious-credentials` and `key: SECRET_KEY`

- [ ] **Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/values.yaml
git commit -m "feat(helm): replace envFrom with per-credential env.valueFrom.secretKeyRef"
```

---

## Task 9: Full render and lint verification

- [ ] **Step 1: Default mode (in-cluster postgresql) — full lint**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x
```

Expected: `0 chart(s) failed`

- [ ] **Step 2: URI-from mode — SECRET_KEY and DATABASE_URL from external secrets**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKeyFrom.name=app-secret \
  --set nexorious.secretKeyFrom.key=sk \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set nexorious.databaseUrlFrom.name=db-secret \
  --set nexorious.databaseUrlFrom.key=uri \
  --set 'controllers.postgresql.enabled=false' \
  --set 'service.postgresql.enabled=false' \
  --set 'persistence.postgresql-data.enabled=false'
```

Expected: `0 chart(s) failed`

Verify SECRET_KEY references external secret:

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKeyFrom.name=app-secret \
  --set nexorious.secretKeyFrom.key=sk \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set nexorious.databaseUrlFrom.name=db-secret \
  --set nexorious.databaseUrlFrom.key=uri \
  --set 'controllers.postgresql.enabled=false' \
  --set 'service.postgresql.enabled=false' \
  --set 'persistence.postgresql-data.enabled=false' \
  2>&1 | grep -A 10 'name: SECRET_KEY'
```

Expected: `name: app-secret` and `key: sk`

- [ ] **Step 3: Individual keys mode**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set nexorious.dbHostFrom.name=cnpg-secret \
  --set nexorious.dbHostFrom.key=host \
  --set nexorious.dbPortFrom.name=cnpg-secret \
  --set nexorious.dbPortFrom.key=port \
  --set nexorious.dbUserFrom.name=cnpg-secret \
  --set nexorious.dbUserFrom.key=username \
  --set nexorious.dbPasswordFrom.name=cnpg-secret \
  --set nexorious.dbPasswordFrom.key=password \
  --set nexorious.dbNameFrom.name=cnpg-secret \
  --set nexorious.dbNameFrom.key=dbname \
  --set 'controllers.postgresql.enabled=false' \
  --set 'service.postgresql.enabled=false' \
  --set 'persistence.postgresql-data.enabled=false'
```

Expected: `0 chart(s) failed`

Verify DB_HOST references external secret and DATABASE_URL has `optional: true`:

```bash
cd /home/abo/workspace/home/nexorious
helm template nexorious deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set nexorious.dbHostFrom.name=cnpg-secret \
  --set nexorious.dbHostFrom.key=host \
  --set nexorious.dbPortFrom.name=cnpg-secret \
  --set nexorious.dbPortFrom.key=port \
  --set nexorious.dbUserFrom.name=cnpg-secret \
  --set nexorious.dbUserFrom.key=username \
  --set nexorious.dbPasswordFrom.name=cnpg-secret \
  --set nexorious.dbPasswordFrom.key=password \
  --set nexorious.dbNameFrom.name=cnpg-secret \
  --set nexorious.dbNameFrom.key=dbname \
  --set 'controllers.postgresql.enabled=false' \
  --set 'service.postgresql.enabled=false' \
  --set 'persistence.postgresql-data.enabled=false' \
  2>&1 | grep -A 10 'name: DB_HOST'
```

Expected: `name: cnpg-secret` and `key: host`

- [ ] **Step 4: All-external mode (all credentials from external secrets)**

```bash
cd /home/abo/workspace/home/nexorious
helm lint --strict deploy/helm/ \
  --set nexorious.secretKeyFrom.name=app-secret \
  --set nexorious.secretKeyFrom.key=sk \
  --set nexorious.internalApiKeyFrom.name=app-secret \
  --set nexorious.internalApiKeyFrom.key=iak \
  --set nexorious.igdbClientIdFrom.name=igdb-secret \
  --set nexorious.igdbClientIdFrom.key=client_id \
  --set nexorious.igdbClientSecretFrom.name=igdb-secret \
  --set nexorious.igdbClientSecretFrom.key=client_secret \
  --set nexorious.databaseUrlFrom.name=db-secret \
  --set nexorious.databaseUrlFrom.key=uri \
  --set nexorious.postgresql.password=x \
  --set 'controllers.postgresql.enabled=false' \
  --set 'service.postgresql.enabled=false' \
  --set 'persistence.postgresql-data.enabled=false'
```

Expected: `0 chart(s) failed`

- [ ] **Step 5: Remove completed items from PRD roadmap**

In `docs/PRD.md`, remove the following two roadmap items:
- `#### External database secret support in Helm chart`
- `#### External secret support for non-database credentials`

- [ ] **Step 6: Final commit**

```bash
cd /home/abo/workspace/home/nexorious
git add docs/PRD.md
git commit -m "docs: remove completed external secret support items from roadmap"
```

- [ ] **Step 7: Push branch and open PR for review**

```bash
cd /home/abo/workspace/home/nexorious
git push -u origin feat/helm-external-secret-support
gh pr create --title "feat(helm): external secret support via *From pattern" --body "$(cat <<'EOF'
## Summary
- Adds `*From` companion fields for all secret-valued Helm values (`secretKey`, `internalApiKey`, `igdbClientId`, `igdbClientSecret`, `databaseUrl`, and individual DB keys)
- Replaces `envFrom: secretRef` with per-credential `env.valueFrom.secretKeyRef` in api/worker/scheduler containers
- Supports DB URI mode (`databaseUrlFrom`) and individual keys mode (`db*From`) as mutually exclusive options
- Managed secret conditionally omits externalized fields

## Test plan
- [ ] `helm lint --strict` passes in default mode (in-cluster postgresql)
- [ ] `helm lint --strict` passes with all credentials externalized
- [ ] `helm lint --strict` passes with URI-from DB mode (postgresql disabled)
- [ ] `helm lint --strict` passes with individual-keys DB mode (postgresql disabled)
- [ ] `helm template` confirms credential env vars reference external secrets when `*From` is configured
- [ ] `helm template` confirms individual DB vars are absent when not in individual-keys mode
- [ ] Validation errors fire correctly for partial `*From`, mixed DB modes, and missing DB connection

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

**Do not merge** — wait for user review of the PR diff before merging.
