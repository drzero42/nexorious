# Design: External Secret Support in Helm Chart

## Problem

The Helm chart currently requires all credentials to be provided as inline values, which are stored in a single chart-managed Kubernetes Secret (`nexorious-credentials`). This is incompatible with GitOps-friendly secret management tools (e.g. CloudNativePG, External Secrets Operator, Vault Agent) that provision secrets independently as Kubernetes Secrets.

Two roadmap items are addressed together:
- **External database secret support** (High) — support pointing the database connection to an externally-managed secret, via a full URI key or individual host/port/user/password/dbname keys.
- **External secret support for non-database credentials** (Medium) — extend the same mechanism to `secretKey`, `internalApiKey`, `igdbClientId`, and `igdbClientSecret`.

## Goals

- Allow any secret-valued Helm field to be sourced from a key in an existing Kubernetes Secret.
- Support two DB modes: URI (single key → `DATABASE_URL`) and individual keys (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`).
- Remain fully backward-compatible — existing deployments with inline values require no changes.
- Fail fast at `helm template` / `helm install` time with clear errors for misconfiguration.

## Non-Goals

- `NATS_URL` and `INTERNAL_API_URL` external secret support (separate roadmap item).
- Support for Kubernetes `ExternalSecret` CRDs — only native Kubernetes Secrets are referenced.

## Design

### `*From` Pattern

Each secret-valued field gains a companion `*From` struct:

```yaml
nexorious:
  # Non-DB credentials (each supports a *From variant)
  secretKey: "..."
  secretKeyFrom:
    name: ""   # Kubernetes Secret name
    key: ""    # key within that secret

  internalApiKey: "..."
  internalApiKeyFrom:
    name: ""
    key: ""

  igdbClientId: ""
  igdbClientIdFrom:
    name: ""
    key: ""

  igdbClientSecret: ""
  igdbClientSecretFrom:
    name: ""
    key: ""

  # DB: URI mode
  databaseUrl: ""
  databaseUrlFrom:
    name: ""
    key: ""

  # DB: individual keys mode (alternative to databaseUrl / databaseUrlFrom)
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

When `*From.name` and `*From.key` are both non-empty, the external secret is used and the corresponding inline value is ignored. When both fields of a `*From` are empty (the default), the inline value is used as before.

### DB Modes (Mutually Exclusive)

| Mode | How configured | Env vars injected |
|---|---|---|
| In-cluster postgresql | postgresql controller enabled, no DB override | `DATABASE_URL` (auto-built) |
| Direct URI | `databaseUrl` non-empty | `DATABASE_URL` |
| URI from external secret | `databaseUrlFrom.name` + `key` set | `DATABASE_URL` |
| Individual keys | any `db*From` set | `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` |

Configuring more than one DB mode is a validation error. In individual-keys mode `DATABASE_URL` is not injected at all (the backend uses individual vars when `DATABASE_URL` is absent).

### Env Var Injection

The three app containers (api, worker, scheduler) currently use `envFrom: secretRef: nexorious-credentials`. This is replaced with individual `env` entries using `valueFrom.secretKeyRef`, where the secret name and key are resolved by helper templates at render time.

Each credential has two helpers:
- `nexorious.<field>SecretName` — returns the external secret name if `*From` is configured, otherwise returns `nexorious-credentials`.
- `nexorious.<field>SecretKey` — returns the configured key if `*From` is configured, otherwise returns the default key name in the managed secret.

For individual DB vars (`DB_HOST` etc.), `optional: true` is set on the `secretKeyRef`. When not configured, the helpers return the managed secret name with a sentinel key (`_unused_db_host`, etc.) that does not exist in the managed secret — so `optional: true` causes the env var to be skipped entirely. The same `optional: true` treatment applies to `DATABASE_URL` so it is not injected in individual-keys mode (the managed secret omits the `DATABASE_URL` key when individual-keys mode is active).

**Note on bjw-s tpl rendering:** The bjw-s common library renders string values in container specs through `tpl` (evidenced by `tag: "{{ .Chart.AppVersion }}"` in the existing chart). The implementation must confirm that `env.*.valueFrom.secretKeyRef.name` and `.key` fields are also rendered through `tpl` via a test render before proceeding with the env var injection changes.

### Managed Secret (`credentials-secret.yaml`)

The managed secret becomes conditional — each field is omitted when the corresponding `*From` is configured:

- `SECRET_KEY` — omitted when `secretKeyFrom.name` is set
- `INTERNAL_API_KEY` — omitted when `internalApiKeyFrom.name` is set
- `IGDB_CLIENT_ID` — omitted when `igdbClientIdFrom.name` is set
- `IGDB_CLIENT_SECRET` — omitted when `igdbClientSecretFrom.name` is set
- `DATABASE_URL` — omitted when `databaseUrlFrom.name` is set OR when any `db*From.name` is set
- `POSTGRES_PASSWORD` — always present (read by the in-cluster PostgreSQL container directly)
- `NATS_URL` — always present
- `INTERNAL_API_URL` — always present

### Validation (`_helpers.tpl`)

The `nexorious.validateValues` helper is extended with:

1. **`*From` completeness:** For each `*From` field, if either `name` or `key` is set, both must be non-empty. Error example: `"nexorious.secretKeyFrom.key must be set when nexorious.secretKeyFrom.name is set"`.

2. **Non-DB credential bypass:** The existing "must not be default/empty" checks for `secretKey`, `internalApiKey`, `igdbClientId`, `igdbClientSecret` are skipped when the corresponding `*From` is configured.

3. **DB mode exclusivity:** Exactly one of `databaseUrl`, `databaseUrlFrom`, or the `db*From` group may be active. Using more than one is a hard failure: `"Only one DB mode may be configured: databaseUrl, databaseUrlFrom, or db*From"`.

4. **postgresql-disabled check:** The existing requirement that a DB connection is provided when the postgresql controller is disabled is extended to pass when any of the three DB modes is configured (not just `databaseUrl`).

### Schema (`values.schema.json`)

Each new `*From` field is added to the `nexorious` object as an optional object with two string properties:

```json
"secretKeyFrom": {
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "name": { "type": "string" },
    "key":  { "type": "string" }
  }
}
```

Same shape for all eleven `*From` fields. The `required` array on `nexorious` is unchanged — all `*From` fields are optional.

## Files Changed

| File | Change |
|---|---|
| `deploy/helm/values.yaml` | Add eleven `*From` fields; replace `envFrom` with individual `env.valueFrom.secretKeyRef` entries in api/worker/scheduler controllers |
| `deploy/helm/templates/_helpers.tpl` | Add ~22 secret-name/key helper templates; extend `nexorious.validateValues` |
| `deploy/helm/templates/credentials-secret.yaml` | Conditionally omit fields when `*From` is configured |
| `deploy/helm/values.schema.json` | Add eleven `*From` fields to `nexorious` schema object |

`values-dev.yaml` requires no changes.

## Risks and Edge Cases

- **tpl rendering in bjw-s:** If bjw-s does not render `valueFrom.secretKeyRef` fields through `tpl`, the approach requires a fallback (e.g. a separate template-generated env-var Secret that the pods reference via envFrom). Verify with `helm template` before implementing the controller changes.
- **`optional: true` in bjw-s:** Verify that bjw-s passes through the `optional` field on `secretKeyRef` structs correctly.
- **POSTGRES_PASSWORD with external DB:** When using an external DB and the in-cluster postgresql is disabled, `POSTGRES_PASSWORD` remains in the managed secret with its default value. This is harmless (the postgresql pod doesn't exist) but slightly untidy. Out of scope for this feature.
- **Empty `*From` structs in values:** Users may set `secretKeyFrom: {}` (empty object). The validation logic must treat this as "not configured" — only trigger external-secret mode when both `name` and `key` are non-empty.
