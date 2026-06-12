# Observability

Nexorious exposes both **metrics** (OTel → Prometheus) and **structured logs** (JSON,
see [logging-conventions.md](logging-conventions.md)). This page covers:

- **Metrics deployment** — a local dev stack for development and the Helm opt-ins
  (ServiceMonitor, Grafana dashboard) for production.
- **Log-based alerting** — ready-made alert rules for Grafana Loki and VictoriaLogs,
  delivered raw or via the chart's opt-in templates.

## Local dev stack

The repo ships a Docker Compose file that brings up a self-contained observability
environment built from your local source tree — no published image required.

**What it starts:**

| Container | Image | Purpose |
|---|---|---|
| `db` | postgres:18-alpine | Application database |
| `migrate` + `app` | built from `Dockerfile` | nexorious (your local checkout) |
| `otel-lgtm` | grafana/otel-lgtm | Grafana + Tempo + Prometheus + Loki in one container |

**Ports:**

| Service | URL |
|---|---|
| App | http://localhost:8000 |
| Grafana | http://localhost:3000 |
| OTLP gRPC | localhost:4317 |
| OTLP HTTP | localhost:4318 |

**Running it** (from the repo root):

```sh
cp deploy/docker/.env.dev.example deploy/docker/.env.dev
# set DB_ENCRYPTION_KEY (openssl rand -base64 32); IGDB_* optional
docker compose -f deploy/docker/docker-compose.dev.yml --env-file deploy/docker/.env.dev up --build
```

The app is built from the repo `Dockerfile`, which uses `COPY --chmod` and so
requires BuildKit. Modern Docker enables it by default; if the build fails with
`the --chmod option requires BuildKit`, prefix the command with `DOCKER_BUILDKIT=1`.

**How telemetry flows:**

- **Traces** are pushed from the app to otel-lgtm via OTLP
  (`OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-lgtm:4318`).
- **Metrics** are scraped from the app's `/metrics` endpoint by otel-lgtm's bundled
  Prometheus, configured at `deploy/docker/dev/prometheus.yaml`.

The Grafana dashboard **"Nexorious — Observability"** is auto-provisioned from
`deploy/observability/nexorious-dashboard.json` — the same file used in production.
Sync and job panels only show data once you trigger activity (e.g. run a sync). Open
Grafana at http://localhost:3000 (anonymous Admin, no login required).

**Teardown** (resets the database):

```sh
docker compose -f deploy/docker/docker-compose.dev.yml down -v
```

## Metrics in production (Helm)

Both objects are opt-in (default off) and independent of each other. The dashboard
panels and alert rules reference the OTel→Prometheus exporter metric names defined in
[`deploy/observability/prometheus-rules.yaml`](../deploy/observability/prometheus-rules.yaml).

### ServiceMonitor

Enable with one values key:

```yaml
serviceMonitor:
  main:
    enabled: true
```

This renders the bjw-s common native `ServiceMonitor` — a pure values toggle, no
custom template. It:

- Requires the **Prometheus Operator** (kube-prometheus-stack) CRDs in-cluster.
- Scrapes the nexorious Service `http` port (8000) at `/metrics` every 30 s.
- Sets `jobLabel: app.kubernetes.io/name`, so the generated `job` label is
  `nexorious` — which is why the metrics alert rule `up{job=~".*nexorious.*"}`
  matches the scraped target.

### Grafana dashboard

Enable with:

```yaml
dashboard:
  enabled: true
```

One toggle, two delivery modes via `dashboard.mode`:

| Mode | What is rendered | Discovery |
|---|---|---|
| `configmap` (default) | ConfigMap labeled `grafana_dashboard: "1"` | Grafana sidecar (kube-prometheus-stack default) auto-discovers it |
| `crd` | `GrafanaDashboard` CR (`grafana.integreatly.org/v1beta1`) | grafana-operator, selected by `dashboard.instanceSelector` |

Both modes render the **same JSON** — the single source of truth
`deploy/observability/nexorious-dashboard.json`, which is also what the dev stack
provisions automatically.

> **crd mode prerequisite:** `dashboard.mode: crd` requires the `GrafanaDashboard`
> CRD installed in-cluster, or `helm install`/`upgrade` fails on the unknown kind.
> `helm template`/`lint` are unaffected. This is why `configmap` is the default.

## Source of truth

Raw rules live under [`deploy/observability/`](../deploy/observability/) and are
the single source of truth (the Helm templates wrap these exact files):

| File | Backend | Loaded by |
|---|---|---|
| `loki-rules.yaml` | Grafana Loki | Loki ruler |
| `victorialogs-rules.yaml` | VictoriaLogs | vmalert (`type: vlogs`) |
| `alertmanager-route.example.yaml` | — | example Alertmanager route (not applied) |

Both backends define the same eight alerts across four categories, two-tier
severity (`critical` / `warning`):

| Alert | Category | Severity | Fires when |
|---|---|---|---|
| `NexoriousDBUnavailable` | DB/startup | critical | any `database unavailable` in 5m (for 2m) |
| `NexoriousStartupFailure` | DB/startup | critical | any migration / River-init failure in 5m |
| `NexoriousPanicRecovered` | generic | critical | any recovered panic in 5m |
| `NexoriousCredentialFailures` | credentials | warning | > 3 auth failures per source in 15m |
| `NexoriousSyncErrors` | sync/job | warning | > 3 external-API failures per source in 15m |
| `NexoriousStuckJobs` | sync/job | warning | any stuck-job reaping in 15m |
| `NexoriousJobDispatchFailures` | sync/job | warning | any enqueue/dispatch failure in 10m |
| `NexoriousHighErrorRate` | generic | warning | > 10 ERROR lines in 10m |

Thresholds and windows are conservative starting points — **tune to your volume.**
Most rules filter on the low-cardinality `category` field; a few (DBUnavailable,
migration/startup, stuck jobs, enqueue failures) also match on `msg` because they
share `category=db` and are only distinguishable by message.

## Grafana Loki (LogQL → ruler → Alertmanager)

1. **Adjust the stream selector.** Every expr selects `{app="nexorious"}`. Change
   this to match the labels your collector (promtail / grafana-alloy) attaches to
   nexorious logs.
2. **Load the rules.** Drop `loki-rules.yaml` into the ruler's per-tenant rule
   directory (e.g. `/rules/fake/` when `auth_enabled: false`) and reload, or use
   the Helm delivery below.
3. The ruler forwards firing alerts to your Alertmanager.

## VictoriaLogs (LogsQL → vmalert → Alertmanager)

```
vmalert -datasource.url=http://victorialogs:9428 \
        -rule.defaultRuleType=vlogs \
        -notifier.url=http://alertmanager:9093 \
        -rule=/etc/vmalert/victorialogs-rules.yaml
```

- Point `-datasource.url` at the VictoriaLogs **base** URL (vmalert appends the
  LogsQL stats path). Use a vmalert instance dedicated to VictoriaLogs.
- **Scoping:** VictoriaLogs has no per-app field in the JSON. If multiple apps
  ship to the same VictoriaLogs, prepend an app-scoping filter (e.g.
  `app:="nexorious"`) to every expr, or the rules will count other apps' logs.

## Helm delivery (opt-in)

All alert objects default **off**. Enable per backend:

```yaml
alerts:
  enabled: true
  loki:
    enabled: true
    ruleLabel: { key: loki_rule, value: "1" }   # match your ruler sidecar
  victoriaLogs:
    enabled: true
    delivery: vmrule                            # vmrule | configmap
    ruleLabels: { project: nexorious }          # match your VMAlert ruleSelector
```

- **Loki** → a `ConfigMap` carrying `alerts.loki.ruleLabel` so the ruler sidecar
  (e.g. `kiwigrid/k8s-sidecar`) discovers it. The bundled grafana/loki v3 chart's
  built-in sidecar uses `{loki.grafana.com/rule: "true"}` instead — set
  `ruleLabel` to match your sidecar's `label`/`labelValue`.
- **VictoriaLogs** → chosen by `alerts.victoriaLogs.delivery`:
  - `vmrule` (default) → a `VMRule` (VictoriaMetrics Operator). The target
    `VMAlert` must point its datasource at VictoriaLogs and select this `VMRule`
    (via `ruleSelector` matching `alerts.victoriaLogs.ruleLabels`, or
    `selectAllByDefault`).
  - `configmap` → a plain `ConfigMap` (key `victorialogs-rules.yaml`) for setups
    **without** the operator. Unlike Loki's ruler, vmalert has no auto-discovery
    sidecar convention — mount this ConfigMap into your own vmalert pod and point
    `-rule` at the mounted file, e.g.:

    ```yaml
    # vmalert pod spec (excerpt)
    volumes:
      - name: nexorious-rules
        configMap:
          name: <release>-nexorious-victorialogs-alerts
    # container:
    #   args: ["-rule=/etc/vmalert/rules/victorialogs-rules.yaml", ...]
    #   volumeMounts:
    #     - name: nexorious-rules
    #       mountPath: /etc/vmalert/rules
    ```

    Trigger a reload after the ConfigMap updates via vmalert's `/-/reload`
    endpoint (commonly driven by a `configmap-reload` sidecar). `ruleLabels` are
    applied to the ConfigMap so a sidecar-based discovery setup can tag it.

## Alertmanager routing

Routing and receivers are your own infra (the chart ships no secrets). See
[`alertmanager-route.example.yaml`](../deploy/observability/alertmanager-route.example.yaml)
for a `severity`-based route splitting `critical` and `warning` to different
receivers.

## Validating rules before deploy (optional)

- Loki: `cortextool rules lint loki-rules.yaml` (if installed).
- VictoriaLogs: `vmalert -rule=victorialogs-rules.yaml -dryRun ...` (if installed).

These tools are not part of the dev environment; the CI gate validates the Helm
wrapping (`helm lint --strict`), not the LogQL/LogsQL semantics.
