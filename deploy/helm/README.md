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

- Kubernetes 1.28+
- Helm 3.19+
- An [IGDB API client](https://api.igdb.com/v4/getting-started)

## Required values

These three values must be set or the chart will fail rendering:

| Value                         | Description                                          |
|-------------------------------|------------------------------------------------------|
| `nexorious.secretKey`         | JWT signing secret. `openssl rand -hex 32`           |
| `nexorious.igdbClientId`      | IGDB OAuth client id                                 |
| `nexorious.igdbClientSecret`  | IGDB OAuth client secret                             |

Additionally, set `nexorious.postgresql.password` (it has a placeholder
default `change-me-in-production` — the chart will warn if you forget).

## Install

```sh
helm install nexorious oci://ghcr.io/drzero42/charts/nexorious \
  --version 0.1.0 \
  --set nexorious.secretKey="$(openssl rand -hex 32)" \
  --set nexorious.igdbClientId="..." \
  --set nexorious.igdbClientSecret="..." \
  --set nexorious.postgresql.password="$(openssl rand -hex 24)"
```

Upgrade:

```sh
helm upgrade nexorious oci://ghcr.io/drzero42/charts/nexorious --version 0.1.0 -f my-values.yaml
```

### Pinning the image tag

By default the chart uses `appVersion: latest` for the nexorious image,
which is convenient during development but unsuitable for real
deployments — `latest` is a moving target and will silently roll on
every pod restart. For any non-dev environment, pin both the main
container and the migrate initContainer to the same release tag:

```yaml
controllers:
  nexorious:
    initContainers:
      migrate:
        image:
          tag: v0.5.0   # match the release you intend to deploy
    containers:
      main:
        image:
          tag: v0.5.0
```

## Resources

The chart ships with conservative defaults for all containers
(nexorious main, migrate initContainer, and postgres). They are sized
for development and small homelab installs — operators should review
and override them for production. Defaults:

| Container             | requests        | limits   |
|-----------------------|-----------------|----------|
| nexorious main        | 100m / 128Mi    | – / 512Mi |
| migrate initContainer | 100m / 128Mi    | – / 512Mi |
| postgres              | 100m / 256Mi    | – / 1Gi   |

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

Every non-DB credential and every DB connection field can come from an
external Secret instead of being inlined in `values.yaml`. When `*From.name`
and `*From.key` are both non-empty, the external Secret is used and the inline
value is ignored.

```yaml
nexorious:
  secretKeyFrom:
    name: my-existing-secret
    key: jwt-secret
  igdbClientIdFrom:
    name: my-existing-secret
    key: igdb-client-id
  igdbClientSecretFrom:
    name: my-existing-secret
    key: igdb-client-secret
```

### DB connection modes (mutually exclusive)

| Mode             | Values                                                   |
|------------------|----------------------------------------------------------|
| Inline URL       | `nexorious.databaseUrl: postgresql://...`                |
| External URL ref | `nexorious.databaseUrlFrom: { name, key }`               |
| Individual keys  | `nexorious.dbHostFrom`, `dbPortFrom`, `dbUserFrom`, `dbPasswordFrom`, `dbNameFrom` |

If none of those are set, the chart auto-builds `DATABASE_URL` from
`nexorious.postgresql.{username,password,database}` pointing at the
in-cluster `postgresql` StatefulSet.

## In-cluster vs external PostgreSQL

The chart ships PostgreSQL in-cluster (`postgresql.enabled: true` by
default). To bring your own database, disable all three resources and supply
a `DATABASE_URL`:

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

## Notes

- The `migrate` initContainer always runs before the main container; you do
  not need a separate one-off migration job on upgrades.
- The scheduler and background workers run inside the same `main`
  container, so the deployment is fixed at 1 replica in the default values
  (raise it only if you have made `internal/scheduler` leader-aware).
- All storage (cover art, uploads, backups) lives on the `storage` PVC
  mounted at `/app/storage`.
