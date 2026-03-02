# Helm Chart Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a Helm chart at `deploy/helm/` that deploys Nexorious on Kubernetes using the bjw-s common library v4.6.2.

**Architecture:** Single application chart wrapping the bjw-s common library. All services (api, frontend, worker, scheduler, postgresql, nats) are defined as controllers in `values.yaml`. A custom `templates/credentials-secret.yaml` holds sensitive values. Connection string env vars use predictable in-cluster service names so they work out-of-the-box when the chart is installed as `helm install nexorious ./deploy/helm`.

**Tech Stack:** Helm 3, bjw-s/common library v4.6.2, Kubernetes >=1.28, PostgreSQL 18-alpine, NATS Alpine.

---

## Naming Convention Note

The bjw-s common library names resources as `<release-name>-<controller-name>`. So with `helm install nexorious ./deploy/helm`:
- PostgreSQL service: `nexorious-postgresql`
- NATS service: `nexorious-nats`
- API service: `nexorious-api`

The default values.yaml is pre-wired for the release name `nexorious`. Users using a different release name must update `DATABASE_URL`, `NATS_URL`, and `INTERNAL_API_URL` in their values override, OR set `global.fullnameOverride: nexorious`.

---

## Task 1: Create Chart Scaffolding

**Files:**
- Create: `deploy/helm/Chart.yaml`
- Create: `deploy/helm/.helmignore`
- Create: `deploy/helm/templates/` (directory — just write the first file in it)

**Step 1: Create the directory structure**

```bash
mkdir -p /home/abo/workspace/home/nexorious/deploy/helm/templates
```

**Step 2: Write `deploy/helm/Chart.yaml`**

```yaml
apiVersion: v2
name: nexorious
description: A self-hosted game collection management application
type: application
version: 0.1.0
appVersion: "latest"
kubeVersion: ">=1.28.0-0"
home: https://github.com/your-org/nexorious
sources:
  - https://github.com/your-org/nexorious
maintainers:
  - name: Nexorious
keywords:
  - games
  - collection
  - self-hosted
dependencies:
  - name: common
    repository: https://bjw-s-labs.github.io/helm-charts/
    version: 4.6.2
```

**Step 3: Write `deploy/helm/.helmignore`**

```
# Patterns to ignore when building packages.
.DS_Store
.git/
.gitignore
.bzr/
.bzrignore
.hg/
.hgignore
.svn/
*.swp
*.bak
*.tmp
*.orig
*~
.project
.idea/
*.tmproj
.vscode/
```

**Step 4: Run helm dependency update to fetch the common library**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm && helm dependency update
```

Expected: Creates `Chart.lock` and `charts/common-4.6.2.tgz`.

**Step 5: Verify**

```bash
ls /home/abo/workspace/home/nexorious/deploy/helm/charts/
```

Expected: `common-4.6.2.tgz`

**Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/Chart.yaml deploy/helm/.helmignore deploy/helm/Chart.lock deploy/helm/charts/
git commit -m "feat(helm): add chart scaffolding with bjw-s common library dependency"
```

---

## Task 2: Write `_helpers.tpl`

**Files:**
- Create: `deploy/helm/templates/_helpers.tpl`

**Step 1: Write the helpers template**

```
deploy/helm/templates/_helpers.tpl
```

```
{{/*
Expand the name of the chart.
*/}}
{{- define "nexorious.name" -}}
{{- include "bjw-s.common.lib.chart.names.name" . }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "nexorious.fullname" -}}
{{- include "bjw-s.common.lib.chart.names.fullname" . }}
{{- end }}

{{/*
Name of the credentials secret.
*/}}
{{- define "nexorious.credentialsSecretName" -}}
{{- include "bjw-s.common.lib.chart.names.fullname" . }}-credentials
{{- end }}

{{/*
Validate required values.
*/}}
{{- define "nexorious.validateValues" -}}
{{- if eq .Values.nexorious.secretKey "change-me-in-production" }}
  {{- fail "nexorious.secretKey must be set to a secure random value" }}
{{- end }}
{{- if eq .Values.nexorious.internalApiKey "change-me-in-production" }}
  {{- fail "nexorious.internalApiKey must be set to a secure random value" }}
{{- end }}
{{- if and (not (dig "postgresql" "enabled" true .Values.controllers)) (empty .Values.nexorious.databaseUrl) }}
  {{- fail "nexorious.databaseUrl must be set when the postgresql controller is disabled" }}
{{- end }}
{{- if and (not (dig "nats" "enabled" true .Values.controllers)) (empty .Values.nexorious.natsUrl) }}
  {{- fail "nexorious.natsUrl must be set when the nats controller is disabled" }}
{{- end }}
{{- end }}
```

**Step 2: Verify the file exists**

```bash
cat /home/abo/workspace/home/nexorious/deploy/helm/templates/_helpers.tpl
```

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/templates/_helpers.tpl
git commit -m "feat(helm): add _helpers.tpl with naming and validation helpers"
```

---

## Task 3: Write `credentials-secret.yaml`

This template creates a Kubernetes Secret containing all sensitive values. Controllers reference it via `envFrom`.

**Files:**
- Create: `deploy/helm/templates/credentials-secret.yaml`

**Step 1: Write the template**

```
deploy/helm/templates/credentials-secret.yaml
```

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
  SECRET_KEY: {{ .Values.nexorious.secretKey | quote }}
  INTERNAL_API_KEY: {{ .Values.nexorious.internalApiKey | quote }}
  IGDB_CLIENT_ID: {{ .Values.nexorious.igdbClientId | quote }}
  IGDB_CLIENT_SECRET: {{ .Values.nexorious.igdbClientSecret | quote }}
  POSTGRES_PASSWORD: {{ .Values.nexorious.postgresql.password | quote }}
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/templates/credentials-secret.yaml
git commit -m "feat(helm): add credentials secret template"
```

---

## Task 4: Write `NOTES.txt`

**Files:**
- Create: `deploy/helm/templates/NOTES.txt`

**Step 1: Write the template**

```
deploy/helm/templates/NOTES.txt
```

```
Thank you for installing {{ .Chart.Name }} {{ .Chart.Version }}!

{{- if eq .Values.nexorious.secretKey "change-me-in-production" }}

⚠️  WARNING: nexorious.secretKey is set to the default placeholder value.
   Set it to a secure random value:
   --set nexorious.secretKey=$(openssl rand -hex 32)

{{- end }}

{{- if eq .Values.nexorious.internalApiKey "change-me-in-production" }}

⚠️  WARNING: nexorious.internalApiKey is set to the default placeholder value.
   Set it to a secure random value:
   --set nexorious.internalApiKey=$(openssl rand -hex 32)

{{- end }}

{{- if eq .Values.nexorious.postgresql.password "change-me-in-production" }}

⚠️  WARNING: nexorious.postgresql.password is set to the default placeholder value.
   Set it before deploying to production.

{{- end }}

== Access ==

{{- if .Values.ingress.frontend.enabled }}
  Frontend: https://{{ (first .Values.ingress.frontend.hosts).host }}
{{- else }}
  Frontend is not exposed via ingress. Use port-forward to access:
  kubectl port-forward svc/{{ include "nexorious.fullname" . }}-frontend 3000:3000

  Then open: http://localhost:3000
{{- end }}

== Naming Note ==

This chart was designed to be installed as:
  helm install nexorious ./deploy/helm

If you used a different release name, update these values in your overrides:
  nexorious.databaseUrl
  nexorious.natsUrl
  nexorious.internalApiUrl
  controllers.frontend.containers.main.env.INTERNAL_API_URL
  controllers.frontend.containers.main.env.NEXT_PUBLIC_API_URL
  controllers.frontend.containers.main.env.NEXT_PUBLIC_STATIC_URL

OR set: global.fullnameOverride: nexorious
```

**Step 2: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/templates/NOTES.txt
git commit -m "feat(helm): add NOTES.txt post-install instructions"
```

---

## Task 5: Write `values.yaml` — Infrastructure Controllers (PostgreSQL & NATS)

**Files:**
- Create: `deploy/helm/values.yaml` (initial, will be extended in Tasks 6-9)

**Step 1: Create `values.yaml` with the nexorious custom values block and infrastructure controllers**

Write the file `deploy/helm/values.yaml` with the following content:

```yaml
# =============================================================================
# Nexorious Helm Chart Values
# =============================================================================
# Based on the bjw-s common library: https://bjw-s-labs.github.io/helm-charts/
#
# Default release name assumed: nexorious
# Install with: helm install nexorious ./deploy/helm
#
# If using a different release name, set:
#   global.fullnameOverride: nexorious
# =============================================================================

# -- Custom Nexorious configuration (not part of the common library)
nexorious:
  # -- JWT signing secret key. REQUIRED. Generate with: openssl rand -hex 32
  secretKey: "change-me-in-production"

  # -- Internal worker-to-API authentication key. REQUIRED. Generate with: openssl rand -hex 32
  internalApiKey: "change-me-in-production"

  # -- IGDB API client ID (optional — IGDB features will be disabled if not set)
  igdbClientId: ""

  # -- IGDB API client secret (optional)
  igdbClientSecret: ""

  # -- Database URL. Leave empty to use the in-cluster PostgreSQL controller.
  # -- Set this when disabling the postgresql controller for an external database.
  # -- Example: postgresql://user:pass@my-postgres:5432/nexorious
  databaseUrl: ""

  # -- NATS URL. Leave empty to use the in-cluster NATS controller.
  # -- Set this when disabling the nats controller for an external NATS instance.
  # -- Example: nats://my-nats:4222
  natsUrl: ""

  # -- Internal API URL used by worker/scheduler to reach the api service.
  # -- Defaults to the in-cluster API service. Update if using a different release name.
  internalApiUrl: "http://nexorious-api:8000"

  postgresql:
    # -- PostgreSQL username
    username: nexorious
    # -- PostgreSQL password. REQUIRED. Change before deploying.
    password: "change-me-in-production"
    # -- PostgreSQL database name
    database: nexorious

# =============================================================================
# Controllers
# =============================================================================

controllers:
  # ---------------------------------------------------------------------------
  # PostgreSQL — in-cluster database (StatefulSet)
  # Disable and set nexorious.databaseUrl to use an external database.
  # ---------------------------------------------------------------------------
  postgresql:
    enabled: true
    type: statefulset
    containers:
      main:
        image:
          repository: postgres
          # -- PostgreSQL image tag. Change to use a different version.
          tag: 18-alpine
          pullPolicy: IfNotPresent
        env:
          POSTGRES_USER:
            valueFrom:
              secretKeyRef:
                name: nexorious-credentials
                key: POSTGRES_PASSWORD
                # We use a configMap for the username since it's not sensitive
          POSTGRES_DB: nexorious
          POSTGRES_PASSWORD:
            valueFrom:
              secretKeyRef:
                name: nexorious-credentials
                key: POSTGRES_PASSWORD
          PGDATA: /var/lib/postgresql/data/pgdata
        probes:
          liveness:
            enabled: true
            custom: true
            spec:
              exec:
                command:
                  - pg_isready
                  - -U
                  - nexorious
              initialDelaySeconds: 10
              periodSeconds: 10
              failureThreshold: 5
          readiness:
            enabled: true
            custom: true
            spec:
              exec:
                command:
                  - pg_isready
                  - -U
                  - nexorious
              initialDelaySeconds: 5
              periodSeconds: 5
              failureThreshold: 5
          startup:
            enabled: true
            custom: true
            spec:
              exec:
                command:
                  - pg_isready
                  - -U
                  - nexorious
              failureThreshold: 30
              periodSeconds: 5

  # ---------------------------------------------------------------------------
  # NATS — in-cluster message broker with JetStream (StatefulSet)
  # Disable and set nexorious.natsUrl to use an external NATS instance.
  # ---------------------------------------------------------------------------
  nats:
    enabled: true
    type: statefulset
    containers:
      main:
        image:
          repository: nats
          tag: alpine
          pullPolicy: IfNotPresent
        args:
          - --jetstream
          - --store_dir=/data
          - --http_port=8222
        probes:
          liveness:
            enabled: true
            custom: true
            spec:
              httpGet:
                path: /healthz
                port: 8222
              initialDelaySeconds: 10
              periodSeconds: 10
          readiness:
            enabled: true
            custom: true
            spec:
              httpGet:
                path: /healthz
                port: 8222
              initialDelaySeconds: 5
              periodSeconds: 5
          startup:
            enabled: true
            custom: true
            spec:
              httpGet:
                path: /healthz
                port: 8222
              failureThreshold: 30
              periodSeconds: 5
```

**Step 2: Run helm template to verify the PostgreSQL and NATS sections render**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm template nexorious . | grep -A5 "kind: StatefulSet"
```

Expected: Two StatefulSet definitions (postgresql and nats).

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/values.yaml
git commit -m "feat(helm): add postgresql and nats controllers to values.yaml"
```

---

## Task 6: Add Application Controllers to `values.yaml`

**Files:**
- Modify: `deploy/helm/values.yaml`

Append the following to the `controllers:` section in `values.yaml` (after the `nats:` block):

**Step 1: Add the api, frontend, worker, and scheduler controllers**

Append to the `controllers:` block:

```yaml
  # ---------------------------------------------------------------------------
  # API — FastAPI backend (Deployment)
  # ---------------------------------------------------------------------------
  api:
    type: deployment
    replicas: 1
    containers:
      main:
        image:
          # -- API container image repository. Change to your actual image.
          repository: ghcr.io/your-org/nexorious-api
          tag: latest
          pullPolicy: IfNotPresent
        envFrom:
          - secretRef:
              name: nexorious-credentials
        env:
          # Connection strings — pre-wired for in-cluster services with release name "nexorious".
          # Update these if using a different release name or external services.
          DATABASE_URL: "postgresql://nexorious:$(POSTGRES_PASSWORD)@nexorious-postgresql:5432/nexorious"
          NATS_URL: "nats://nexorious-nats:4222"
          INTERNAL_API_URL: "http://nexorious-api:8000"
          CORS_ORIGINS: ""
          STORAGE_PATH: /app/storage
          LOG_LEVEL: INFO
          DEBUG: "false"
        probes:
          liveness:
            enabled: true
            custom: true
            spec:
              httpGet:
                path: /health
                port: 8000
              initialDelaySeconds: 15
              periodSeconds: 10
              failureThreshold: 3
          startup:
            enabled: true
            custom: true
            spec:
              httpGet:
                path: /health
                port: 8000
              failureThreshold: 30
              periodSeconds: 5

  # ---------------------------------------------------------------------------
  # Frontend — Next.js (Deployment)
  # ---------------------------------------------------------------------------
  frontend:
    type: deployment
    replicas: 1
    containers:
      main:
        image:
          # -- Frontend container image repository. Change to your actual image.
          repository: ghcr.io/your-org/nexorious-frontend
          tag: latest
          pullPolicy: IfNotPresent
        env:
          # Client-side URL (must be reachable from the user's browser)
          NEXT_PUBLIC_API_URL: "http://nexorious.example.com/api"
          NEXT_PUBLIC_STATIC_URL: "http://nexorious.example.com"
          # Server-side URL (container-to-container, not browser-facing)
          INTERNAL_API_URL: "http://nexorious-api:8000/api"
        probes:
          liveness:
            enabled: true
            custom: true
            spec:
              httpGet:
                path: /
                port: 3000
              initialDelaySeconds: 15
              periodSeconds: 10
              failureThreshold: 3

  # ---------------------------------------------------------------------------
  # Worker — Taskiq background worker (Deployment, horizontally scalable)
  # ---------------------------------------------------------------------------
  worker:
    type: deployment
    replicas: 1
    containers:
      main:
        image:
          # -- Same image as the api controller
          repository: ghcr.io/your-org/nexorious-api
          tag: latest
          pullPolicy: IfNotPresent
        command:
          - uv
          - run
          - taskiq
          - worker
          - --workers
          - "1"
          - --max-async-tasks
          - "5"
          - app.worker.broker:broker
          - app.worker.tasks
        envFrom:
          - secretRef:
              name: nexorious-credentials
        env:
          DATABASE_URL: "postgresql://nexorious:$(POSTGRES_PASSWORD)@nexorious-postgresql:5432/nexorious"
          NATS_URL: "nats://nexorious-nats:4222"
          INTERNAL_API_URL: "http://nexorious-api:8000"
          STORAGE_PATH: /app/storage
          TASKIQ_SKIP_TABLE_CREATION: "true"
          LOG_LEVEL: INFO

  # ---------------------------------------------------------------------------
  # Scheduler — Taskiq scheduler (Deployment, MUST remain at 1 replica)
  #
  # ⚠️  WARNING: Do NOT scale this above 1 replica.
  # The scheduler creates NATS JetStream tables on startup.
  # Multiple instances will conflict and cause data corruption.
  # ---------------------------------------------------------------------------
  scheduler:
    type: deployment
    replicas: 1
    containers:
      main:
        image:
          # -- Same image as the api controller
          repository: ghcr.io/your-org/nexorious-api
          tag: latest
          pullPolicy: IfNotPresent
        command:
          - sh
          - -c
          - |
            uv run python -c 'import asyncio; from app.worker.broker import broker; asyncio.run(broker.startup())' &&
            touch /tmp/taskiq_ready &&
            uv run taskiq scheduler app.worker.schedules:scheduler app.worker.tasks
        envFrom:
          - secretRef:
              name: nexorious-credentials
        env:
          DATABASE_URL: "postgresql://nexorious:$(POSTGRES_PASSWORD)@nexorious-postgresql:5432/nexorious"
          NATS_URL: "nats://nexorious-nats:4222"
          INTERNAL_API_URL: "http://nexorious-api:8000"
          LOG_LEVEL: INFO
        probes:
          liveness:
            enabled: true
            custom: true
            spec:
              exec:
                command:
                  - test
                  - -f
                  - /tmp/taskiq_ready
              initialDelaySeconds: 30
              periodSeconds: 10
              failureThreshold: 3
          startup:
            enabled: true
            custom: true
            spec:
              exec:
                command:
                  - test
                  - -f
                  - /tmp/taskiq_ready
              failureThreshold: 60
              periodSeconds: 5
```

**Step 2: Verify all controllers render**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm template nexorious . | grep "kind: Deployment" | wc -l
```

Expected: 4 (api, frontend, worker, scheduler)

```bash
helm template nexorious . | grep "kind: StatefulSet" | wc -l
```

Expected: 2 (postgresql, nats)

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/values.yaml
git commit -m "feat(helm): add api, frontend, worker, scheduler controllers"
```

---

## Task 7: Add Services, Ingress, and Persistence to `values.yaml`

**Files:**
- Modify: `deploy/helm/values.yaml`

Append the following sections after the `controllers:` block:

**Step 1: Add services, ingress, and persistence**

```yaml
# =============================================================================
# Services
# =============================================================================

service:
  api:
    controller: api
    ports:
      http:
        port: 8000

  frontend:
    controller: frontend
    ports:
      http:
        port: 3000

  postgresql:
    controller: postgresql
    ports:
      postgresql:
        port: 5432

  nats:
    controller: nats
    ports:
      client:
        port: 4222
      monitoring:
        port: 8222

# =============================================================================
# Ingress
# =============================================================================

ingress:
  # -- Frontend ingress (disabled by default)
  # Enable and configure hosts to expose the frontend via an ingress controller.
  frontend:
    enabled: false
    # -- Ingress class name (e.g. nginx, traefik, etc.)
    className: ""
    annotations: {}
      # nginx.ingress.kubernetes.io/proxy-body-size: "50m"
    hosts:
      - host: nexorious.example.com
        paths:
          - path: /
            pathType: Prefix
            service:
              identifier: frontend
              port: http
    tls: []
    #  - secretName: nexorious-tls
    #    hosts:
    #      - nexorious.example.com

# =============================================================================
# Persistence
# =============================================================================

persistence:
  # -- Main storage volume for cover art, uploads, and backups.
  # Shared between the api and worker controllers.
  # By default, backups are stored under /app/storage/backups on this same PVC.
  # To use separate storage for backups (e.g. NFS), add another persistence entry:
  #
  #   backups:
  #     enabled: true
  #     type: nfs
  #     server: nas.local
  #     path: /backups/nexorious
  #     advancedMounts:
  #       api:
  #         main:
  #           - path: /app/storage/backups
  #       worker:
  #         main:
  #           - path: /app/storage/backups
  storage:
    enabled: true
    type: persistentVolumeClaim
    accessMode: ReadWriteOnce
    size: 5Gi
    advancedMounts:
      api:
        main:
          - path: /app/storage
      worker:
        main:
          - path: /app/storage

  # -- PostgreSQL data volume
  postgresql-data:
    enabled: true
    type: persistentVolumeClaim
    accessMode: ReadWriteOnce
    size: 8Gi
    advancedMounts:
      postgresql:
        main:
          - path: /var/lib/postgresql/data

  # -- NATS JetStream data volume
  nats-data:
    enabled: true
    type: persistentVolumeClaim
    accessMode: ReadWriteOnce
    size: 1Gi
    advancedMounts:
      nats:
        main:
          - path: /data
```

**Step 2: Verify services and persistence render**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm template nexorious . | grep "kind: Service" | wc -l
```

Expected: 4 (api, frontend, postgresql, nats)

```bash
helm template nexorious . | grep "kind: PersistentVolumeClaim" | wc -l
```

Expected: 3 (storage, postgresql-data, nats-data)

**Step 3: Run helm lint**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm lint .
```

Expected: `0 chart(s) failed`

**Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/values.yaml
git commit -m "feat(helm): add services, ingress, and persistence to values.yaml"
```

---

## Task 8: Fix the DATABASE_URL env var pattern

The `DATABASE_URL` env var uses `$(POSTGRES_PASSWORD)` which relies on Kubernetes env var substitution. This requires `POSTGRES_PASSWORD` to be defined before `DATABASE_URL` in the container env. Verify this works OR switch to a direct string pattern.

**Step 1: Check if the env var order is correct in the rendered template**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm template nexorious . | grep -A 30 "name: nexorious-api" | grep -A 20 "env:"
```

Check that `POSTGRES_PASSWORD` (from `envFrom` secret) appears before `DATABASE_URL` in the container spec. If the order is wrong, update `values.yaml` to use a fixed DATABASE_URL string instead of the `$(POSTGRES_PASSWORD)` substitution pattern:

Replace the DATABASE_URL in api, worker, and scheduler controllers with:

```yaml
DATABASE_URL: "postgresql://nexorious:change-me-in-production@nexorious-postgresql:5432/nexorious"
```

And add a note in the values.yaml comment:

```yaml
# NOTE: Update the password in DATABASE_URL to match nexorious.postgresql.password
```

**Step 2: Verify the full template renders cleanly**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm template nexorious . > /tmp/nexorious-rendered.yaml && echo "Render OK" && wc -l /tmp/nexorious-rendered.yaml
```

Expected: "Render OK" and a reasonable number of lines (>200).

**Step 3: Commit any changes**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/values.yaml
git commit -m "feat(helm): fix DATABASE_URL env var construction"
```

---

## Task 9: Write `values.schema.json`

A minimal JSON schema to validate required user-facing values and catch obvious mistakes.

**Files:**
- Create: `deploy/helm/values.schema.json`

**Step 1: Write the schema**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "nexorious": {
      "type": "object",
      "properties": {
        "secretKey": {
          "type": "string",
          "minLength": 1,
          "description": "JWT signing secret key. Generate with: openssl rand -hex 32"
        },
        "internalApiKey": {
          "type": "string",
          "minLength": 1,
          "description": "Internal worker-to-API auth key. Generate with: openssl rand -hex 32"
        },
        "igdbClientId": {
          "type": "string"
        },
        "igdbClientSecret": {
          "type": "string"
        },
        "databaseUrl": {
          "type": "string",
          "description": "External database URL. Leave empty to use in-cluster PostgreSQL."
        },
        "natsUrl": {
          "type": "string",
          "description": "External NATS URL. Leave empty to use in-cluster NATS."
        },
        "internalApiUrl": {
          "type": "string",
          "description": "Internal URL for the API service (used by worker/scheduler)."
        },
        "postgresql": {
          "type": "object",
          "properties": {
            "username": {"type": "string"},
            "password": {"type": "string", "minLength": 1},
            "database": {"type": "string"}
          },
          "required": ["password"]
        }
      },
      "required": ["secretKey", "internalApiKey"]
    }
  }
}
```

**Step 2: Validate the schema works with helm lint**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm lint .
```

Expected: `0 chart(s) failed`

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/values.schema.json
git commit -m "feat(helm): add values.schema.json for input validation"
```

---

## Task 10: Full Integration Test

**Step 1: Run a full helm template dry-run with overrides that simulate a real install**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm template nexorious . \
  --set nexorious.secretKey="$(openssl rand -hex 32)" \
  --set nexorious.internalApiKey="$(openssl rand -hex 32)" \
  --set nexorious.postgresql.password="supersecret" \
  --set ingress.frontend.enabled=true \
  --set ingress.frontend.hosts[0].host=nexorious.example.com \
  > /tmp/nexorious-full.yaml && echo "Template OK"
```

Expected: "Template OK" with no errors.

**Step 2: Verify all expected resource types are present**

```bash
grep "^kind:" /tmp/nexorious-full.yaml | sort | uniq -c
```

Expected output (approximately):
```
      4 kind: Deployment
      1 kind: Ingress
      1 kind: PersistentVolumeClaim  (x3 actually — run without sort | uniq -c to see all)
      1 kind: Secret
      4 kind: Service
      2 kind: StatefulSet
```

**Step 3: Run helm lint with strict mode**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm lint . --strict
```

Expected: `0 chart(s) failed`

**Step 4: Verify validation fails correctly when required values are missing**

```bash
cd /home/abo/workspace/home/nexorious/deploy/helm
helm template nexorious . 2>&1 | head -5
```

Expected: An error about `nexorious.secretKey` being set to the default placeholder value (from the `_helpers.tpl` validation).

**Step 5: Final commit**

```bash
cd /home/abo/workspace/home/nexorious
git add deploy/helm/
git commit -m "feat(helm): complete Nexorious Helm chart implementation"
```

---

## Notes for Implementer

### Image Names
The `values.yaml` uses placeholder image names (`ghcr.io/your-org/nexorious-api`, `ghcr.io/your-org/nexorious-frontend`). Update these to your actual published image references before using in production.

### PostgreSQL POSTGRES_USER Fix
The postgresql controller's `POSTGRES_USER` env var currently references `POSTGRES_PASSWORD` by mistake (copy error in the plan). Fix it to:
```yaml
POSTGRES_USER: nexorious
```
(plain string, not a secretKeyRef)

### External Services
To use an external PostgreSQL:
```yaml
controllers:
  postgresql:
    enabled: false
nexorious:
  databaseUrl: "postgresql://user:pass@my-postgres:5432/nexorious"
```

To use external NATS:
```yaml
controllers:
  nats:
    enabled: false
nexorious:
  natsUrl: "nats://my-nats:4222"
```

### Backup Storage Override
To mount backups from NFS:
```yaml
persistence:
  backups:
    enabled: true
    type: nfs
    server: nas.local
    path: /backups/nexorious
    advancedMounts:
      api:
        main:
          - path: /app/storage/backups
      worker:
        main:
          - path: /app/storage/backups
```
