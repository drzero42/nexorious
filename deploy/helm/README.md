# nexorious Helm chart

Helm chart for [nexorious](https://github.com/drzero42/nexorious) (self-hosted game collection).

Built on the [bjw-s common library](https://bjw-s-labs.github.io/helm-charts/).
Ships an in-cluster PostgreSQL StatefulSet by default and a single Deployment
running the Go binary in two phases:

1. `migrate` initContainer — runs `/app/nexorious migrate` to apply any
   pending schema migrations.
2. `main` container — runs `/app/nexorious serve`; the Go binary embeds the
   React SPA and runs the API, workers, and scheduler in one process.

Image: `ghcr.io/drzero42/nexorious:<appVersion>`.

## Requirements

- Kubernetes 1.31+
- Helm 3.19+
- An [IGDB API client](https://api.igdb.com/v4/getting-started)

## Required values

These two values must be set or the chart will fail rendering:

| Value                         | Description                                          |
|-------------------------------|------------------------------------------------------|
| `nexorious.igdbClientId`      | IGDB OAuth client id                                 |
| `nexorious.igdbClientSecret`  | IGDB OAuth client secret                             |

Either can alternatively be supplied via an external Secret — see
[External secret refs](#external-secret-refs-from-pattern).

`nexorious.postgresql.password` is **optional**. When empty (the default),
the chart auto-generates a 32-character random password on first install
and reads it back from the existing managed Secret on subsequent renders
(via Helm's `lookup`), so the value stays stable across upgrades. Set it
explicitly to override, or use `nexorious.postgresql.passwordFrom` to
pull it from an external Secret.

> **Caveat:** `helm template` and `helm install --dry-run` can't `lookup`
> existing cluster state, so they emit a freshly generated password each
> time. Actual `helm install` / `helm upgrade` work correctly.

## Install

```sh
helm install nexorious oci://ghcr.io/drzero42/charts/nexorious \
  --version 0.1.0 \
  --set nexorious.igdbClientId="..." \
  --set nexorious.igdbClientSecret="..."
```

Upgrade:

```sh
helm upgrade nexorious oci://ghcr.io/drzero42/charts/nexorious --version 0.1.0 -f my-values.yaml
```

> **Note on `--set` for secrets:** the snippet above leaves the IGDB
> credentials in your shell history and process arglist.
> For anything beyond local experimentation, use a values file, `--set-file`,
> or — better — an external Secret referenced via the `*From` fields
> ([below](#external-secret-refs-from-pattern)). With a GitOps tool
> (Flux, Argo CD, etc.) you typically commit a `HelmRelease` /
> `Application` that references a pre-existing Secret and never pass
> secrets through Helm flags at all.

### Pinning the image tag

Each released chart pins the nexorious image to its own `appVersion`
(release-please keeps the chart version and `appVersion` in sync), so
installing chart `X.Y.Z` already deploys image `X.Y.Z` without any
extra configuration. Dev builds from `main` use `appVersion: dev`,
which is a moving target and unsuitable for real deployments — pin to
a release chart instead.

To run a *different* image release than the chart was built for,
override both the main container and the migrate initContainer to the
same tag (note: no `v` prefix — image tags are stripped of it):

```yaml
controllers:
  nexorious:
    initContainers:
      migrate:
        image:
          tag: 0.5.0   # match the release you intend to deploy
    containers:
      main:
        image:
          tag: 0.5.0
```

## Resources

The chart ships with conservative defaults for all containers
(nexorious main, migrate initContainer, and postgres). They are sized
for development and small homelab installs — operators should review
and override them for production. Defaults:

| Container             | requests        | limits    |
|-----------------------|-----------------|-----------|
| nexorious main        | 100m / 128Mi    | – / 128Mi |
| migrate initContainer | 100m / 128Mi    | – / 128Mi |
| postgres              | 100m / 256Mi    | – / 256Mi |

Override via standard bjw-s syntax, e.g.:

```yaml
controllers:
  nexorious:
    containers:
      main:
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            memory: 1Gi
```

## External secret refs (`*From` pattern)

Every credential and every DB connection field can come from an external
Secret instead of being inlined in `values.yaml`. When `*From.name`
and `*From.key` are both non-empty, the external Secret is used and the
inline value is ignored.

```yaml
nexorious:
  igdbClientIdFrom:
    name: my-existing-secret
    key: igdb-client-id
  igdbClientSecretFrom:
    name: my-existing-secret
    key: igdb-client-secret
```

### DB connection modes (mutually exclusive)

These configure how the **nexorious app** discovers its DSN:

| Mode             | Values                                                   |
|------------------|----------------------------------------------------------|
| Inline URL       | `nexorious.databaseUrl: postgresql://...`                |
| External URL ref | `nexorious.databaseUrlFrom: { name, key }`               |
| Individual keys  | `nexorious.dbHostFrom`, `dbPortFrom`, `dbUserFrom`, `dbPasswordFrom`, `dbNameFrom` (all five required together) |

If none are set, the chart auto-builds `DATABASE_URL` from
`nexorious.postgresql.{username,password,database}` pointing at the
in-cluster `postgresql` StatefulSet.

### In-cluster Postgres credentials

When the bundled Postgres pod is enabled, its own `POSTGRES_USER`,
`POSTGRES_PASSWORD`, and `POSTGRES_DB` env vars are sourced from the
managed credentials Secret by default (populated from the
`nexorious.postgresql.{username,password,database}` inline values). Each
can be redirected to an externally-managed Secret instead:

```yaml
nexorious:
  postgresql:
    usernameFrom: { name: pg-creds, key: user }
    passwordFrom: { name: pg-creds, key: password }
    databaseFrom: { name: pg-creds, key: dbname }
```

This is the path for external-secrets-operator, SealedSecrets, Vault,
etc. The `pg_isready` probes use `sh -c 'pg_isready -U "$POSTGRES_USER"'`
so they pick up the value from the env regardless of whether it came
from the inline value or an external Secret.

## In-cluster vs external PostgreSQL

The chart ships PostgreSQL in-cluster (`postgresql.enabled: true` by
default). The bundled Postgres password is auto-generated on first
install — see [Required values](#required-values).

### Bring your own PostgreSQL (e.g. CloudNativePG)

To use a database managed outside the chart, disable all three in-cluster
Postgres resources and point the nexorious app at the external DB. Three
typical styles:

**Inline DSN (quickest):**
```yaml
controllers:
  postgresql:
    enabled: false
service:
  postgresql:
    enabled: false
persistence:
  postgresql-data:
    enabled: false
nexorious:
  databaseUrl: "postgresql://user:pass@my-pg-host:5432/nexorious"
```

**Full DSN from an external Secret** (e.g. the `app` Secret a CloudNativePG
`Cluster` creates, which contains a `uri` key):
```yaml
controllers:
  postgresql:
    enabled: false
service:
  postgresql:
    enabled: false
persistence:
  postgresql-data:
    enabled: false
nexorious:
  databaseUrlFrom:
    name: my-cnpg-cluster-app
    key: uri
```

**Individual fields from an external Secret** (when you don't have a
ready-made DSN):
```yaml
controllers:
  postgresql:
    enabled: false
service:
  postgresql:
    enabled: false
persistence:
  postgresql-data:
    enabled: false
nexorious:
  dbHostFrom:     { name: my-db-creds, key: host }
  dbPortFrom:     { name: my-db-creds, key: port }
  dbUserFrom:     { name: my-db-creds, key: user }
  dbPasswordFrom: { name: my-db-creds, key: password }
  dbNameFrom:     { name: my-db-creds, key: dbname }
```

Then:
```sh
helm install nexorious oci://ghcr.io/drzero42/charts/nexorious \
  --version 0.1.0 -f my-values.yaml
```

The `migrate` initContainer applies migrations against the external
database before the main container starts serving — no extra wiring
needed.

## Storage class

By default each enabled PVC (`storage`, `postgresql-data`) is provisioned
without a `storageClassName`, so the cluster's default StorageClass is
used. To pin every PVC to a specific class in one place:

```yaml
global:
  storageClass: fast-ssd
```

The chart applies this to any PVC under `persistence:` that does not set
its own `storageClass` — per-PVC overrides win. Set to `-` to disable
dynamic provisioning entirely (you supply pre-bound PVs).

## Ingress

Disabled by default. To expose the app:

```yaml
ingress:
  nexorious:
    enabled: true
    className: nginx
    hosts:
      - host: nexorious.example.com
        paths:
          - path: /
            pathType: Prefix
            service:
              identifier: nexorious
              port: http
    tls:
      - hosts: [nexorious.example.com]
        secretName: nexorious-tls
```

The `nexorious` service serves both API routes (`/api/*`) and the embedded
SPA on port 8000 — no separate frontend service.

## Alerting (opt-in)

| Key | Default | Description |
|---|---|---|
| `alerts.enabled` | `false` | Master switch for log-based alert rules. |
| `alerts.loki.enabled` | `false` | Render the Loki ruler ConfigMap. |
| `alerts.loki.ruleLabel.key` / `.value` | `loki_rule` / `"1"` | Discovery label the ruler sidecar watches. |
| `alerts.victoriaLogs.enabled` | `false` | Render the VMRule (VictoriaMetrics Operator). |
| `alerts.victoriaLogs.ruleLabels` | `{}` | Extra labels so the VMAlert ruleSelector matches the VMRule. |

See [docs/observability.md](../../docs/observability.md) for full setup.

## Notes

- The `migrate` initContainer always runs before the main container; you do
  not need a separate one-off migration job on upgrades.
- The scheduler and background workers run inside the same `main`
  container, so the deployment is fixed at 1 replica in the default values
  (raise it only if you have made `internal/scheduler` leader-aware).
- All storage (cover art, uploads, backups) lives on the `storage` PVC
  mounted at `/app/storage`.
