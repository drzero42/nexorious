# Helm Chart Design

**Date:** 2026-03-02
**Status:** Approved

## Overview

Create a Helm chart at `deploy/helm/` to deploy Nexorious on Kubernetes. The chart wraps the [bjw-s common library chart](https://github.com/bjw-s-labs/helm-charts/tree/main/charts/library/common), the same library that powers `app-template`. This gives users a fully opinionated default deployment while retaining the full flexibility of the common library for customization.

## Chart Location & Structure

```
deploy/helm/
├── Chart.yaml              # Chart metadata + common library dependency
├── values.yaml             # Nexorious-opinionated default values
├── values.schema.json      # JSON schema for values validation
└── templates/
    ├── NOTES.txt           # Post-install instructions
    └── _helpers.tpl        # Template helpers (DATABASE_URL, NATS_URL)
```

**Key decisions:**
- Single chart, no Nexorious-specific sub-charts
- Only one Helm dependency: the bjw-s common library
- PostgreSQL and NATS are deployed as controllers within the chart (not as sub-chart dependencies)

## Controllers

All controllers are defined under the common library's `controllers` key. Six controllers total:

| Controller    | Type         | Image                                    | Replicas       |
|---------------|--------------|------------------------------------------|----------------|
| `api`         | Deployment   | `ghcr.io/your-org/nexorious-api`         | 1 (scalable)   |
| `frontend`    | Deployment   | `ghcr.io/your-org/nexorious-frontend`    | 1 (scalable)   |
| `worker`      | Deployment   | same as `api` (different command)        | 1 (scalable)   |
| `scheduler`   | Deployment   | same as `api` (different command)        | **1 (locked)** |
| `postgresql`  | StatefulSet  | `postgres:18-alpine` (configurable)      | 1              |
| `nats`        | StatefulSet  | `nats:alpine`                            | 1              |

### api

- **Port:** 8000
- **Probes:** `startupProbe` + `livenessProbe` on `GET /health`
- **Env vars:** `DATABASE_URL`, `NATS_URL`, `SECRET_KEY`, `IGDB_CLIENT_ID`, `IGDB_CLIENT_SECRET`, `CORS_ORIGINS`, `STORAGE_PATH=/app/storage`, `INTERNAL_API_KEY`, `INTERNAL_API_URL`
- **Volumes:** storage PVC mounted at `/app/storage`

### frontend

- **Port:** 3000
- **Probes:** `livenessProbe` on `GET /`
- **Env vars:** `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_STATIC_URL`, `INTERNAL_API_URL`
- **Volumes:** none

### worker

- **Command:** `uv run taskiq worker --workers 1 --max-async-tasks 5 app.worker.broker:broker app.worker.tasks`
- **Env vars:** same as `api` plus `TASKIQ_SKIP_TABLE_CREATION=true`
- **Volumes:** storage PVC mounted at `/app/storage` (same PVC as api)
- **Scaling:** Users scale horizontally by increasing replica count

### scheduler

- **Command:** the scheduler init + run command from docker-compose
- **Replicas:** Hardcoded to `1` — must never be scaled. Documented prominently. Scheduler creates NATS JetStream tables; multiple instances would conflict.
- **Env vars:** `DATABASE_URL`, `NATS_URL`, `SECRET_KEY`, `INTERNAL_API_KEY`, `INTERNAL_API_URL`
- **Volumes:** none

### postgresql

- **Image:** `postgres:18-alpine` (tag configurable via `controllers.postgresql.containers.main.image.tag`)
- **Enabled by default:** yes — set `controllers.postgresql.enabled: false` to use an external PostgreSQL instance
- **Env vars:** `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- **Volumes:** PVC mounted at `/var/lib/postgresql/data`
- **Health check:** `pg_isready`

### nats

- **Image:** `nats:alpine`
- **Enabled by default:** yes — set `controllers.nats.enabled: false` to use an external NATS instance
- **Command:** `--jetstream --store_dir=/data --http_port=8222`
- **Volumes:** PVC mounted at `/data`
- **Health check:** HTTP check on port 8222 (`/healthz`)
- **Ports:** 4222 (client), 8222 (monitoring)

## Services

| Service      | Type      | Port | Target           |
|--------------|-----------|------|------------------|
| `api`        | ClusterIP | 8000 | api controller   |
| `frontend`   | ClusterIP | 3000 | frontend controller |
| `postgresql` | ClusterIP | 5432 | postgresql controller |
| `nats`       | ClusterIP | 4222, 8222 | nats controller |

`worker` and `scheduler` receive no traffic — no services defined for them.

## Ingress

- **Disabled by default**
- One optional ingress resource targeting the `frontend` service on port 3000
- Users enable via `ingress.main.enabled: true` and configure `ingress.main.hosts`
- Supports `ingressClassName` and TLS via the common library's ingress primitives
- The API is not directly exposed via ingress — the frontend proxies API calls via `INTERNAL_API_URL` server-side and `NEXT_PUBLIC_API_URL` client-side

## Persistence

| Name              | Default Size | Mount Path                      | Controllers        |
|-------------------|--------------|---------------------------------|--------------------|
| `storage`         | 5Gi          | `/app/storage`                  | api, worker        |
| `postgresql-data` | 8Gi          | `/var/lib/postgresql/data`      | postgresql         |
| `nats-data`       | 1Gi          | `/data`                         | nats               |

**Backups override pattern:** The `storage` PVC covers `/app/storage/backups` by default. Users who want backups on separate storage (e.g., NFS) add a second persistence entry in their values:

```yaml
persistence:
  backups:
    enabled: true
    type: nfs          # or pvc, hostPath, etc.
    server: nas.local
    path: /backups/nexorious
    globalMounts:
      - path: /app/storage/backups
```

This naturally overrides the backups sub-path via Kubernetes mount precedence without any chart modifications.

## Secrets & Configuration

Sensitive values are configured via the common library's `secrets` key or directly in `env` blocks:

| Variable             | Description                                      |
|----------------------|--------------------------------------------------|
| `SECRET_KEY`         | JWT signing key — **must be set by user**        |
| `INTERNAL_API_KEY`   | Worker-to-API auth key — **must be set by user** |
| `IGDB_CLIENT_ID`     | IGDB API client ID (optional, degrades gracefully) |
| `IGDB_CLIENT_SECRET` | IGDB API client secret                           |
| `POSTGRES_PASSWORD`  | PostgreSQL password — **must be set by user**    |

All secrets default to placeholder values that emit warnings via NOTES.txt.

## Template Helpers (`_helpers.tpl`)

Two computed values resolve connection URLs based on whether in-cluster controllers are enabled:

**`nexorious.databaseUrl`**
- If `controllers.postgresql.enabled`: builds `postgresql://nexorious:{{ password }}@{{ release-name }}-postgresql:5432/nexorious`
- Else: uses `externalPostgresql.url` from values

**`nexorious.natsUrl`**
- If `controllers.nats.enabled`: builds `nats://{{ release-name }}-nats:4222`
- Else: uses `externalNats.url` from values

These helpers are referenced in the `env` blocks of api, worker, and scheduler controllers.

## External Services (when in-cluster controllers disabled)

When users disable the in-cluster PostgreSQL or NATS, they supply connection details:

```yaml
# External PostgreSQL
controllers:
  postgresql:
    enabled: false
externalPostgresql:
  url: "postgresql://user:pass@my-postgres:5432/nexorious"

# External NATS
controllers:
  nats:
    enabled: false
externalNats:
  url: "nats://my-nats:4222"
```

## Non-Goals

- No cert-manager integration (users bring their own TLS solution)
- No HorizontalPodAutoscaler (users add this via `rawResources` if needed)
- No NetworkPolicy (users add via common library's `networkpolicies` key if needed)
- No multi-replica PostgreSQL or NATS HA (single-instance, self-hosted scope)
