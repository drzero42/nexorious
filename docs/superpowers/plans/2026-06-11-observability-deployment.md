# Observability Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the glue to actually *view* Nexorious metrics + traces — a one-container local dev stack (`grafana/otel-lgtm`) and Helm support (a ServiceMonitor and a deployable Grafana dashboard), all driven by one committed dashboard JSON that is the single source of truth.

**Architecture:** The dashboard JSON lives at `deploy/observability/nexorious-dashboard.json` (matching the existing source-of-truth-in-`deploy/observability/` + symlink-into-`deploy/helm/files/` convention used by the alert rules). The Helm chart wraps it two ways behind one toggle (`dashboard.enabled` + `dashboard.mode: configmap|crd`) via `.Files.Get`. The ServiceMonitor is the bjw-s common `5.0.1` *native* `serviceMonitor` feature — pure values, no custom template. The local dev stack is a new self-contained `docker-compose.dev.yml` that builds the app from the local `Dockerfile`, runs Postgres + a one-shot migrate, and runs `grafana/otel-lgtm` (receiving traces via OTLP push and scraping `/metrics` via a mounted Prometheus config), with the same dashboard JSON provisioned into its Grafana.

**Tech Stack:** Helm 3 + bjw-s common `5.0.1` library chart, `helm lint --strict`, Docker Compose, `grafana/otel-lgtm`, Prometheus/Grafana, the OTel→Prometheus exporter already shipped in `internal/observability`.

---

## Background facts (verified against the codebase — do not re-derive)

**Real exported metric names** (the OTel→Prometheus exporter appends `_total` to counters and `_seconds`/`_milliseconds` to histograms). Every dashboard panel and alert already uses these — they are the contract:

- `nexorious_sync_total{source,status}` — status ∈ {`completed`, `completed_with_errors`}
- `nexorious_sync_items_total{source,outcome}` — outcome ∈ {`completed`, `failed`, `skipped`}
- `nexorious_db_errors_total{operation}`
- `river_work_count_total{status}` — status ∈ {`ok`, `error`, `panic`}
- `river_work_duration_histogram_seconds_bucket` (p95 source; plain `river.work_duration` is a gauge — do NOT use it)
- `http_client_request_duration_seconds_{count,bucket}{server_address,http_request_method,http_response_status_code}`
- `go_sql_query_timing_milliseconds_bucket` (bunotel)
- `up{job=~".*nexorious.*"}`

There are **no HTTP-server request metrics** (we deliberately skip a custom Echo v5 middleware) — "request throughput/latency" on the dashboard means the *outbound* `http_client_*` series.

**Endpoint / config facts:**
- `/metrics` is served unauthenticated and always-on (when `OTEL_METRICS_ENABLED=true`, the default) on the app's HTTP port `8000` (`internal/api/router.go:194`; allow-listed through every state gate).
- `OTEL_EXPORTER_OTLP_ENDPOINT` enables **trace** export and **must include a scheme** (the SDK reads it directly; the config test pins `http://localhost:4318`). In the dev stack use `http://otel-lgtm:4318`.
- `OTEL_METRICS_ENABLED` defaults to `true`; `OTEL_SERVICE_NAME` defaults to `nexorious`.

**bjw-s `serviceMonitor` facts:**
- It is a top-level `serviceMonitor:` map (default `{}`). Each entry is enabled-by-default; set `enabled: false` to gate it off.
- Two Services are enabled in this chart (`nexorious`, `postgresql`), so automatic service detection fails — the entry **must** set `service.identifier: nexorious` (verified in `common/templates/lib/serviceMonitor/_validate.tpl`).
- `endpoints` is required. `jobLabel` defaults to `app.kubernetes.io/name`, which the Service carries — so `up{job=~".*nexorious.*"}` matches (job value = chart name `nexorious`).

**otel-lgtm facts (verified upstream):**
- Ports: Grafana `3000`, OTLP gRPC `4317`, OTLP HTTP `4318`, Prometheus `9090`.
- Bundled Prometheus reads a mounted config at `/otel-lgtm/prometheus.yaml`.
- Grafana auto-provisions dashboards from `/otel-lgtm/grafana/conf/provisioning/dashboards/`.

**values.schema.json facts:** top-level `additionalProperties: true` (so new top-level `dashboard:` / `serviceMonitor:` keys are accepted — the existing non-bjw-s `alerts:` key proves it). We still register `dashboard` explicitly (mirroring `alerts`) for validation quality. `serviceMonitor` is validated by the common subchart's own schema — do not duplicate it.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `deploy/observability/nexorious-dashboard.json` | Single-source-of-truth Grafana dashboard JSON | Create |
| `deploy/helm/files/nexorious-dashboard.json` | Symlink → `../../observability/nexorious-dashboard.json` (so `.Files.Get` can read it) | Create (symlink) |
| `deploy/helm/templates/grafana-dashboard.yaml` | One template rendering ConfigMap **or** GrafanaDashboard from the JSON | Create |
| `deploy/helm/values.yaml` | Add `serviceMonitor:` (native, default-off) and `dashboard:` blocks | Modify |
| `deploy/helm/values.schema.json` | Register the new `dashboard` object | Modify |
| `deploy/docker/docker-compose.dev.yml` | Self-contained dev stack: db + migrate + app (built locally) + otel-lgtm | Create |
| `deploy/docker/dev/prometheus.yaml` | otel-lgtm Prometheus scrape config for the app `/metrics` | Create |
| `deploy/docker/dev/grafana-dashboards.yaml` | otel-lgtm Grafana dashboard provider config | Create |
| `deploy/docker/.env.dev.example` | Example env for the dev stack | Create |
| `.github/workflows/test.yaml` | Add `--set serviceMonitor.main.enabled=true --set dashboard.enabled=true` to the lint step | Modify |
| `deploy/helm/README.md` | Document `serviceMonitor` + `dashboard` values | Modify |
| `docs/observability.md` | Add metrics-deployment sections (dev stack, ServiceMonitor, dashboard) | Modify |

---

## Task 1: Dashboard JSON (single source of truth) + symlink

**Files:**
- Create: `deploy/observability/nexorious-dashboard.json`
- Create: `deploy/helm/files/nexorious-dashboard.json` (symlink)

- [ ] **Step 1: Write the dashboard JSON**

Create `deploy/observability/nexorious-dashboard.json` with exactly this content (a `datasource` templating variable of type `datasource`/`prometheus` makes it portable across the dev stack and any operator's Grafana; every panel uses the verified metric names; `clamp_min(..., 1e-9)` guards the ratio panels against divide-by-zero):

```json
{
  "annotations": {
    "list": [
      {
        "builtIn": 1,
        "datasource": { "type": "grafana", "uid": "-- Grafana --" },
        "enable": true,
        "hide": true,
        "iconColor": "rgba(0, 211, 255, 1)",
        "name": "Annotations & Alerts",
        "type": "dashboard"
      }
    ]
  },
  "editable": true,
  "graphTooltip": 1,
  "schemaVersion": 39,
  "tags": ["nexorious", "observability"],
  "templating": {
    "list": [
      {
        "current": {},
        "hide": 0,
        "includeAll": false,
        "label": "Prometheus",
        "name": "datasource",
        "options": [],
        "query": "prometheus",
        "refresh": 1,
        "regex": "",
        "type": "datasource"
      }
    ]
  },
  "time": { "from": "now-6h", "to": "now" },
  "timezone": "",
  "title": "Nexorious — Observability",
  "uid": "nexorious-observability",
  "panels": [
    {
      "title": "Sync",
      "type": "row",
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
      "id": 1,
      "panels": []
    },
    {
      "title": "Sync jobs by source & status (jobs/s)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 1 },
      "id": 2,
      "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "fillOpacity": 10, "stacking": { "mode": "normal" } } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "sum by (source, status) (rate(nexorious_sync_total[$__rate_interval]))", "legendFormat": "{{source}} / {{status}}" }
      ]
    },
    {
      "title": "Sync success ratio by source",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 1 },
      "id": 3,
      "fieldConfig": { "defaults": { "unit": "percentunit", "min": 0, "max": 1, "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "sum by (source) (rate(nexorious_sync_total{status=\"completed\"}[$__rate_interval])) / clamp_min(sum by (source) (rate(nexorious_sync_total[$__rate_interval])), 1e-9)", "legendFormat": "{{source}}" }
      ]
    },
    {
      "title": "Sync items by source & outcome (items/s)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 24, "x": 0, "y": 9 },
      "id": 4,
      "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "fillOpacity": 10, "stacking": { "mode": "normal" } } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "sum by (source, outcome) (rate(nexorious_sync_items_total[$__rate_interval]))", "legendFormat": "{{source}} / {{outcome}}" }
      ]
    },
    {
      "title": "Jobs",
      "type": "row",
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 17 },
      "id": 5,
      "panels": []
    },
    {
      "title": "River job throughput by status (jobs/s)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 18 },
      "id": 6,
      "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "fillOpacity": 10, "stacking": { "mode": "normal" } } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "sum by (status) (rate(river_work_count_total[$__rate_interval]))", "legendFormat": "{{status}}" }
      ]
    },
    {
      "title": "River job duration p95 (s)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 18 },
      "id": 7,
      "fieldConfig": { "defaults": { "unit": "s", "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "list", "placement": "bottom" }, "tooltip": { "mode": "single" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "histogram_quantile(0.95, sum by (le) (rate(river_work_duration_histogram_seconds_bucket[$__rate_interval])))", "legendFormat": "p95" }
      ]
    },
    {
      "title": "External APIs",
      "type": "row",
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 26 },
      "id": 8,
      "panels": []
    },
    {
      "title": "External API request rate by host (req/s)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 8, "x": 0, "y": 27 },
      "id": 9,
      "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "sum by (server_address) (rate(http_client_request_duration_seconds_count[$__rate_interval]))", "legendFormat": "{{server_address}}" }
      ]
    },
    {
      "title": "External API p95 latency by host (s)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 8, "x": 8, "y": 27 },
      "id": 10,
      "fieldConfig": { "defaults": { "unit": "s", "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "histogram_quantile(0.95, sum by (le, server_address) (rate(http_client_request_duration_seconds_bucket[$__rate_interval])))", "legendFormat": "{{server_address}}" }
      ]
    },
    {
      "title": "External API 5xx ratio by host",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 8, "x": 16, "y": 27 },
      "id": 11,
      "fieldConfig": { "defaults": { "unit": "percentunit", "min": 0, "max": 1, "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "sum by (server_address) (rate(http_client_request_duration_seconds_count{http_response_status_code=~\"5..\"}[$__rate_interval])) / clamp_min(sum by (server_address) (rate(http_client_request_duration_seconds_count[$__rate_interval])), 1e-9)", "legendFormat": "{{server_address}}" }
      ]
    },
    {
      "title": "Database",
      "type": "row",
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 35 },
      "id": 12,
      "panels": []
    },
    {
      "title": "DB query p95 latency (ms)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 36 },
      "id": 13,
      "fieldConfig": { "defaults": { "unit": "ms", "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "list", "placement": "bottom" }, "tooltip": { "mode": "single" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "histogram_quantile(0.95, sum by (le) (rate(go_sql_query_timing_milliseconds_bucket[$__rate_interval])))", "legendFormat": "p95" }
      ]
    },
    {
      "title": "DB errors by operation (errors/s)",
      "type": "timeseries",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 36 },
      "id": 14,
      "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull"] }, "tooltip": { "mode": "multi" } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "sum by (operation) (rate(nexorious_db_errors_total[$__rate_interval]))", "legendFormat": "{{operation}}" }
      ]
    },
    {
      "title": "Scrape target up",
      "type": "stat",
      "datasource": { "type": "prometheus", "uid": "${datasource}" },
      "gridPos": { "h": 4, "w": 24, "x": 0, "y": 44 },
      "id": 15,
      "fieldConfig": { "defaults": { "mappings": [ { "type": "value", "options": { "0": { "text": "DOWN", "color": "red" }, "1": { "text": "UP", "color": "green" } } } ], "thresholds": { "mode": "absolute", "steps": [ { "color": "red", "value": null }, { "color": "green", "value": 1 } ] } }, "overrides": [] },
      "options": { "colorMode": "background", "graphMode": "none", "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false } },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${datasource}" }, "expr": "up{job=~\".*nexorious.*\"}", "legendFormat": "{{instance}}" }
      ]
    }
  ]
}
```

- [ ] **Step 2: Verify the JSON is valid and uses only real metrics**

Run:
```bash
python3 -c "import json; d=json.load(open('deploy/observability/nexorious-dashboard.json')); print('panels', len(d['panels']), d['uid'])"
grep -oE 'nexorious_[a-z_]+|river_work_[a-z_]+|http_client_request_duration_seconds_[a-z]+|go_sql_query_timing_milliseconds_bucket|up\{' deploy/observability/nexorious-dashboard.json | sort -u
```
Expected: `panels 15 nexorious-observability`, and the metric list contains only the names from the Background facts (no stray/invented names).

- [ ] **Step 3: Create the symlink so `.Files.Get` can read it from the chart**

Run:
```bash
ln -s ../../observability/nexorious-dashboard.json deploy/helm/files/nexorious-dashboard.json
ls -l deploy/helm/files/nexorious-dashboard.json
git -c core.symlinks=true add deploy/observability/nexorious-dashboard.json deploy/helm/files/nexorious-dashboard.json
git status --short
```
Expected: the symlink resolves (`-> ../../observability/nexorious-dashboard.json`), and git stages both a regular file and a symlink (consistent with the existing `deploy/helm/files/*-rules.yaml` symlinks).

- [ ] **Step 4: Commit**

```bash
git add deploy/observability/nexorious-dashboard.json deploy/helm/files/nexorious-dashboard.json
git commit -m "feat: add Nexorious Grafana dashboard JSON (single source of truth)"
```

---

## Task 2: Helm ServiceMonitor (native bjw-s, default off)

**Files:**
- Modify: `deploy/helm/values.yaml` (add top-level `serviceMonitor:` block)
- Modify: `.github/workflows/test.yaml` (add `--set serviceMonitor.main.enabled=true`)
- Modify: `deploy/helm/README.md`

- [ ] **Step 1: Add the `serviceMonitor` block to `values.yaml`**

Insert a new top-level block immediately **after** the `alerts:` block (after its `victoriaMetrics:` sub-block, before the `# Default Pod options` banner near line 202). Content:

```yaml
# =============================================================================
# ServiceMonitor (opt-in) — Prometheus Operator scrape of /metrics
# =============================================================================
# Native bjw-s common ServiceMonitor (no custom template). Requires the
# Prometheus Operator (kube-prometheus-stack) CRDs in-cluster. Default OFF.
# Because two Services are enabled (nexorious + postgresql), the target Service
# must be named explicitly via service.identifier.
serviceMonitor:
  main:
    # -- Render a ServiceMonitor scraping the nexorious Service /metrics. Default off.
    enabled: false
    # -- Target the nexorious Service (postgresql is excluded).
    service:
      identifier: nexorious
    # -- Scrape endpoints. The nexorious Service exposes the app on the `http`
    # -- port (8000); metrics are served unauthenticated at /metrics.
    endpoints:
      - port: http
        scheme: http
        path: /metrics
        interval: 30s
        scrapeTimeout: 10s
        honorLabels: true
```

- [ ] **Step 2: Render and assert the ServiceMonitor**

Run:
```bash
helm template t deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set serviceMonitor.main.enabled=true \
  --show-only charts/common/templates/render/_serviceMonitors.tpl 2>/dev/null \
  | grep -E "kind: ServiceMonitor|/metrics|port: http|jobLabel|app.kubernetes.io/service" || \
helm template t deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set serviceMonitor.main.enabled=true \
  | grep -E "kind: ServiceMonitor|path: /metrics|port: http|jobLabel: app.kubernetes.io/name"
```
Expected: a `ServiceMonitor` is rendered with an endpoint `port: http`, `path: /metrics`, and `jobLabel: app.kubernetes.io/name`. (The `--show-only` path may not match bjw-s's dynamic template name; the fallback full-render grep is authoritative.)

- [ ] **Step 3: Assert it is OFF by default**

Run:
```bash
helm template t deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  | grep -c "kind: ServiceMonitor"
```
Expected: `0`.

- [ ] **Step 4: Add the CI lint flag**

In `.github/workflows/test.yaml`, append to the `helm lint --strict` command (after the `--set alerts.victoriaMetrics.enabled=true` line, line ~179) a continuation:
```yaml
              --set serviceMonitor.main.enabled=true \
```
(Keep the trailing backslash convention; ensure the final line of the command has no trailing backslash.)

- [ ] **Step 5: Document in the Helm README**

In `deploy/helm/README.md`, in the values table / observability section, add a row/paragraph describing `serviceMonitor.main.enabled` (default `false`; requires Prometheus Operator CRDs; scrapes the nexorious Service `http` port at `/metrics`).

- [ ] **Step 6: Lint and commit**

```bash
helm lint --strict deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set alerts.enabled=true --set alerts.loki.enabled=true \
  --set alerts.victoriaLogs.enabled=true --set alerts.victoriaLogs.delivery=vmrule \
  --set alerts.prometheus.enabled=true --set alerts.victoriaMetrics.enabled=true \
  --set serviceMonitor.main.enabled=true
git add deploy/helm/values.yaml deploy/helm/README.md .github/workflows/test.yaml
git commit -m "feat: opt-in Helm ServiceMonitor for /metrics (default off)"
```
Expected: `helm lint` reports `0 chart(s) failed`.

---

## Task 3: Helm dashboard template (configmap | crd, one toggle)

**Files:**
- Create: `deploy/helm/templates/grafana-dashboard.yaml`
- Modify: `deploy/helm/values.yaml` (add `dashboard:` block)
- Modify: `deploy/helm/values.schema.json` (register `dashboard`)
- Modify: `.github/workflows/test.yaml` (add `--set dashboard.enabled=true`)
- Modify: `deploy/helm/README.md`

- [ ] **Step 1: Add the `dashboard` block to `values.yaml`**

Insert immediately **after** the `serviceMonitor:` block from Task 2:

```yaml
# =============================================================================
# Grafana dashboard (opt-in) — the committed nexorious dashboard JSON
# =============================================================================
# Renders the single-source-of-truth dashboard (deploy/observability/
# nexorious-dashboard.json) for in-cluster Grafana. Default OFF.
dashboard:
  # -- Master switch for the dashboard object.
  enabled: false
  # -- How to deliver the dashboard:
  # --   configmap  a ConfigMap labeled `grafana_dashboard: "1"` that the
  # --              Grafana sidecar (kube-prometheus-stack default) auto-discovers.
  # --   crd        a GrafanaDashboard CR (grafana.integreatly.org/v1beta1) for
  # --              grafana-operator users. NOTE: requires the CRD installed
  # --              in-cluster or `helm install/upgrade` fails on the unknown
  # --              kind (`helm template`/`lint` are unaffected) — hence the
  # --              configmap default.
  mode: configmap
  # -- Extra labels on the rendered object (e.g. to target a specific sidecar
  # -- or Grafana folder). For configmap mode the grafana_dashboard label is
  # -- always added regardless of this.
  labels: {}
  # -- (crd mode only) instanceSelector matching the grafana-operator Grafana CR.
  instanceSelector:
    matchLabels:
      dashboards: grafana
```

- [ ] **Step 2: Write the template**

Create `deploy/helm/templates/grafana-dashboard.yaml`:

```yaml
{{- if .Values.dashboard.enabled }}
{{- if eq .Values.dashboard.mode "configmap" }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "nexorious.fullname" . }}-dashboard
  labels:
    grafana_dashboard: "1"
    app.kubernetes.io/name: {{ include "nexorious.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ include "nexorious.chart" . }}
    {{- with .Values.dashboard.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
data:
  nexorious-dashboard.json: |-
{{ .Files.Get "files/nexorious-dashboard.json" | indent 4 }}
{{- else if eq .Values.dashboard.mode "crd" }}
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDashboard
metadata:
  name: {{ include "nexorious.fullname" . }}-dashboard
  labels:
    app.kubernetes.io/name: {{ include "nexorious.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ include "nexorious.chart" . }}
    {{- with .Values.dashboard.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  instanceSelector:
    {{- toYaml .Values.dashboard.instanceSelector | nindent 4 }}
  json: |-
{{ .Files.Get "files/nexorious-dashboard.json" | indent 4 }}
{{- else }}
{{- fail (printf "dashboard.mode must be 'configmap' or 'crd', got %q" .Values.dashboard.mode) }}
{{- end }}
{{- end }}
```

Note: confirm the helper names `nexorious.fullname` / `nexorious.name` / `nexorious.chart` exist in `_helpers.tpl` (the alert templates use `nexorious.name` and `nexorious.chart`; `nexorious.fullname` is the bjw-s-derived full name). If `nexorious.fullname` is absent, use `nexorious.name`-based naming consistent with the alert templates. Verify in Step 3.

- [ ] **Step 3: Verify helper names and render configmap mode**

Run:
```bash
grep -E 'define "nexorious\.(fullname|name|chart)"' deploy/helm/templates/_helpers.tpl
helm template t deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set dashboard.enabled=true \
  --show-only templates/grafana-dashboard.yaml
```
Expected: the three helpers are defined (if `nexorious.fullname` is missing, switch the template's `name:` to `{{ include "nexorious.name" . }}-dashboard` and re-run). The render is a `kind: ConfigMap` with label `grafana_dashboard: "1"` and a `data.nexorious-dashboard.json` key whose value is the dashboard JSON.

- [ ] **Step 4: Render crd mode and assert same JSON**

Run:
```bash
helm template t deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set dashboard.enabled=true --set dashboard.mode=crd \
  --show-only templates/grafana-dashboard.yaml | grep -E "kind: GrafanaDashboard|instanceSelector|json: \|-|nexorious-observability"
```
Expected: `kind: GrafanaDashboard`, an `instanceSelector`, a `spec.json` block, and the `nexorious-observability` uid appears inside it (same JSON).

- [ ] **Step 5: Assert OFF by default and bad-mode guard**

Run:
```bash
helm template t deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  | grep -cE "kind: ConfigMap.*dashboard|GrafanaDashboard" ; echo "expect 0 dashboard objects above"
helm template t deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set dashboard.enabled=true --set dashboard.mode=bogus 2>&1 | grep -E "dashboard.mode must be"
```
Expected: no dashboard object by default; the `bogus` mode fails with the guard message.

- [ ] **Step 6: Register `dashboard` in `values.schema.json`**

Add a `dashboard` property to the top-level `properties` object (sibling of `alerts`), mirroring the `alerts` style:

```json
"dashboard": {
  "type": "object",
  "additionalProperties": false,
  "description": "Opt-in Grafana dashboard delivery (the committed nexorious dashboard JSON).",
  "properties": {
    "enabled": { "type": "boolean", "description": "Render the dashboard object." },
    "mode": { "type": "string", "enum": ["configmap", "crd"], "description": "configmap (sidecar ConfigMap) or crd (GrafanaDashboard)." },
    "labels": { "type": "object", "additionalProperties": { "type": "string" }, "description": "Extra labels on the rendered object." },
    "instanceSelector": {
      "type": "object",
      "description": "grafana-operator instanceSelector (crd mode).",
      "properties": { "matchLabels": { "type": "object", "additionalProperties": { "type": "string" } } }
    }
  }
}
```
Verify the JSON stays valid: `python3 -c "import json; json.load(open('deploy/helm/values.schema.json')); print('ok')"`.

- [ ] **Step 7: Add the CI lint flag**

In `.github/workflows/test.yaml`, append to the same `helm lint --strict` command:
```yaml
              --set dashboard.enabled=true \
```

- [ ] **Step 8: Document in the Helm README**

In `deploy/helm/README.md`, add `dashboard.enabled` / `dashboard.mode` / `dashboard.instanceSelector` rows, noting the crd-CRD-prerequisite caveat.

- [ ] **Step 9: Lint and commit**

```bash
helm lint --strict deploy/helm \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set alerts.enabled=true --set alerts.loki.enabled=true \
  --set alerts.victoriaLogs.enabled=true --set alerts.victoriaLogs.delivery=vmrule \
  --set alerts.prometheus.enabled=true --set alerts.victoriaMetrics.enabled=true \
  --set serviceMonitor.main.enabled=true --set dashboard.enabled=true
git add deploy/helm/templates/grafana-dashboard.yaml deploy/helm/values.yaml \
  deploy/helm/values.schema.json deploy/helm/README.md .github/workflows/test.yaml
git commit -m "feat: opt-in Helm Grafana dashboard (configmap|crd, default off)"
```
Expected: `0 chart(s) failed`.

---

## Task 4: Local dev stack (docker-compose.dev.yml + otel-lgtm)

**Files:**
- Create: `deploy/docker/docker-compose.dev.yml`
- Create: `deploy/docker/dev/prometheus.yaml`
- Create: `deploy/docker/dev/grafana-dashboards.yaml`
- Create: `deploy/docker/.env.dev.example`

- [ ] **Step 1: Write the otel-lgtm Prometheus scrape config**

Create `deploy/docker/dev/prometheus.yaml` (mounted at `/otel-lgtm/prometheus.yaml`; the app exposes ALL metrics — `nexorious_*`, `river_*`, `http_client_*`, `go_sql_*` — on the single `/metrics` endpoint, so one scrape job covers the whole dashboard; `job_name: nexorious` satisfies the `up{job=~".*nexorious.*"}` panel):

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
scrape_configs:
  - job_name: nexorious
    metrics_path: /metrics
    static_configs:
      - targets: ["app:8000"]
```

- [ ] **Step 2: Write the Grafana dashboard provider config**

Create `deploy/docker/dev/grafana-dashboards.yaml` (mounted into otel-lgtm's Grafana provisioning dir; points Grafana at a folder where the dashboard JSON is mounted):

```yaml
apiVersion: 1
providers:
  - name: nexorious
    type: file
    disableDeletion: false
    allowUiUpdates: true
    options:
      path: /otel-lgtm/grafana/conf/provisioning/dashboards/nexorious
      foldersFromFilesStructure: false
```

- [ ] **Step 3: Write the dev compose file**

Create `deploy/docker/docker-compose.dev.yml`. It builds the app from the repo `Dockerfile` (so the stack runs your working-tree code), runs a one-shot `migrate`, and runs `otel-lgtm` with the Prometheus config + dashboard provisioning mounted. Trace export points at otel-lgtm; metrics are scraped from `/metrics`.

```yaml
# Local development observability stack.
#
# Brings up Postgres, the nexorious app (BUILT FROM THE LOCAL SOURCE TREE so you
# see your own changes), and grafana/otel-lgtm (Grafana + Tempo + Prometheus +
# Loki in one container). Traces are pushed to otel-lgtm via OTLP; metrics are
# scraped from the app's /metrics by otel-lgtm's bundled Prometheus.
#
# Usage (from repo root):
#   cp deploy/docker/.env.dev.example deploy/docker/.env.dev
#   # edit deploy/docker/.env.dev (set DB_ENCRYPTION_KEY; IGDB_* optional)
#   docker compose -f deploy/docker/docker-compose.dev.yml --env-file deploy/docker/.env.dev up --build
#
# Then open:
#   App:     http://localhost:8000
#   Grafana: http://localhost:3000   (dashboard "Nexorious — Observability")
#
# Tear down (remove volumes for a clean DB):
#   docker compose -f deploy/docker/docker-compose.dev.yml down -v

name: nexorious-dev

services:
  db:
    image: docker.io/postgres:18-alpine
    environment:
      POSTGRES_USER: nexorious
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-nexorious}
      POSTGRES_DB: nexorious
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nexorious"]
      interval: 5s
      timeout: 5s
      retries: 5

  migrate:
    build:
      context: ../..
      dockerfile: Dockerfile
    command: ["migrate"]
    depends_on:
      db:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://nexorious:${POSTGRES_PASSWORD:-nexorious}@db:5432/nexorious?sslmode=disable
      DB_ENCRYPTION_KEY: ${DB_ENCRYPTION_KEY:?DB_ENCRYPTION_KEY is required}

  app:
    build:
      context: ../..
      dockerfile: Dockerfile
    command: ["serve"]
    ports:
      - "8000:8000"
    depends_on:
      db:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
    environment:
      DATABASE_URL: postgres://nexorious:${POSTGRES_PASSWORD:-nexorious}@db:5432/nexorious?sslmode=disable
      DB_ENCRYPTION_KEY: ${DB_ENCRYPTION_KEY:?DB_ENCRYPTION_KEY is required}
      IGDB_CLIENT_ID: ${IGDB_CLIENT_ID:-}
      IGDB_CLIENT_SECRET: ${IGDB_CLIENT_SECRET:-}
      LEGENDARY_WORK_DIR: /var/lib/legendary
      # Observability: metrics always-on at /metrics; traces pushed to otel-lgtm.
      OTEL_SERVICE_NAME: nexorious
      OTEL_METRICS_ENABLED: "true"
      OTEL_EXPORTER_OTLP_ENDPOINT: http://otel-lgtm:4318
      OTEL_TRACES_SAMPLER: always_on
    volumes:
      - type: tmpfs
        target: /var/lib/legendary
        tmpfs:
          mode: "0777"

  otel-lgtm:
    image: docker.io/grafana/otel-lgtm:latest
    ports:
      - "3000:3000"   # Grafana
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
    environment:
      GF_AUTH_ANONYMOUS_ENABLED: "true"
      GF_AUTH_ANONYMOUS_ORG_ROLE: Admin
    volumes:
      - ./dev/prometheus.yaml:/otel-lgtm/prometheus.yaml:ro
      - ./dev/grafana-dashboards.yaml:/otel-lgtm/grafana/conf/provisioning/dashboards/nexorious.yaml:ro
      - ../observability/nexorious-dashboard.json:/otel-lgtm/grafana/conf/provisioning/dashboards/nexorious/nexorious-dashboard.json:ro

volumes: {}
```

- [ ] **Step 4: Write the example env file**

Create `deploy/docker/.env.dev.example`:

```bash
# Copy to deploy/docker/.env.dev and fill in.
# Postgres password (defaults to "nexorious" if unset).
POSTGRES_PASSWORD=nexorious
# Required: encryption key for stored credentials. Generate: openssl rand -base64 32
DB_ENCRYPTION_KEY=
# Optional: IGDB metadata enrichment.
IGDB_CLIENT_ID=
IGDB_CLIENT_SECRET=
```

- [ ] **Step 5: Validate compose syntax**

Run:
```bash
docker compose -f deploy/docker/docker-compose.dev.yml --env-file /dev/null config -q \
  && echo "compose OK" || echo "compose INVALID"
```
If `docker` is unavailable in the shell, instead validate YAML well-formedness:
```bash
python3 -c "import yaml; yaml.safe_load(open('deploy/docker/docker-compose.dev.yml')); yaml.safe_load(open('deploy/docker/dev/prometheus.yaml')); yaml.safe_load(open('deploy/docker/dev/grafana-dashboards.yaml')); print('yaml OK')"
```
Expected: `compose OK` (or `yaml OK`). Note: `config -q` may warn about the required `DB_ENCRYPTION_KEY` interpolation — pass `DB_ENCRYPTION_KEY=x` via `--env-file` or an inline `DB_ENCRYPTION_KEY=x docker compose ...` if so.

- [ ] **Step 6: (Manual / acceptance) Bring the stack up and confirm live data**

This step requires Docker and is the acceptance check — run locally, not in CI:
```bash
cp deploy/docker/.env.dev.example deploy/docker/.env.dev
sed -i "s|DB_ENCRYPTION_KEY=|DB_ENCRYPTION_KEY=$(openssl rand -base64 32)|" deploy/docker/.env.dev
docker compose -f deploy/docker/docker-compose.dev.yml --env-file deploy/docker/.env.dev up --build -d
# Wait for app, then confirm /metrics serves Prometheus text:
curl -s localhost:8000/metrics | grep -E "nexorious_|river_work|go_sql_query_timing|http_client_request" | head
# Open Grafana http://localhost:3000 → dashboard "Nexorious — Observability" → panels populate
#   (sync/job panels need activity; trigger a sync from the app to see data).
docker compose -f deploy/docker/docker-compose.dev.yml down -v
```
Expected: `/metrics` returns the metric families; Grafana shows the dashboard; "Scrape target up" reads UP.

- [ ] **Step 7: Commit**

```bash
git add deploy/docker/docker-compose.dev.yml deploy/docker/dev/ deploy/docker/.env.dev.example
git commit -m "feat: local Grafana otel-lgtm dev stack for metrics + traces"
```

---

## Task 5: Documentation

**Files:**
- Modify: `docs/observability.md`

- [ ] **Step 1: Broaden the doc and add metrics-deployment sections**

`docs/observability.md` currently covers only log-based alerting (title `# Observability: log-based alerting`). Update the title to `# Observability` and add three new sections (keep the existing log-alerting content):

1. **Local dev stack** — how to run `docker compose -f deploy/docker/docker-compose.dev.yml --env-file deploy/docker/.env.dev up --build`, the ports (App 8000, Grafana 3000, OTLP 4317/4318), that traces push via `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-lgtm:4318` and metrics are scraped from `/metrics`, and where the dashboard appears.
2. **ServiceMonitor (Helm, opt-in)** — `serviceMonitor.main.enabled=true`, the Prometheus-Operator-CRD prerequisite, that it scrapes the nexorious Service `http` port at `/metrics`, and that `jobLabel` is `app.kubernetes.io/name` (so `up{job=~".*nexorious.*"}` matches).
3. **Grafana dashboard (Helm, opt-in)** — `dashboard.enabled=true`, the `configmap` (sidecar) vs `crd` (grafana-operator) modes, the CRD-prerequisite caveat for `crd`, and that the JSON is the same one used by the dev stack (`deploy/observability/nexorious-dashboard.json`).

Add a one-line cross-reference noting the metric names come from the OTel→Prometheus exporter and match `deploy/observability/prometheus-rules.yaml`.

- [ ] **Step 2: Verify no broken internal doc references**

Run:
```bash
grep -nE "docker-compose.dev.yml|nexorious-dashboard.json|serviceMonitor|dashboard.enabled" docs/observability.md
```
Expected: references match the actual file paths/values created in Tasks 1–4.

- [ ] **Step 3: Commit**

```bash
git add docs/observability.md
git commit -m "docs: document observability dev stack, ServiceMonitor, and dashboard"
```

---

## Task 6: Full verification & PR

- [ ] **Step 1: Final lint + render assertions (the acceptance criteria, mechanically)**

Run:
```bash
# AC: ServiceMonitor renders when enabled, off by default
helm template t deploy/helm --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x --set serviceMonitor.main.enabled=true | grep -q "kind: ServiceMonitor" && echo "SM renders"
helm template t deploy/helm --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x | grep -c "kind: ServiceMonitor" # expect 0

# AC: configmap mode → ConfigMap labeled grafana_dashboard:"1"; crd mode → GrafanaDashboard with same JSON; both off by default
helm template t deploy/helm --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x --set dashboard.enabled=true --show-only templates/grafana-dashboard.yaml | grep -E 'kind: ConfigMap|grafana_dashboard: "1"|nexorious-observability'
helm template t deploy/helm --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x --set dashboard.enabled=true --set dashboard.mode=crd --show-only templates/grafana-dashboard.yaml | grep -E 'kind: GrafanaDashboard|nexorious-observability'

# AC: helm lint --strict stays green (full flag set mirroring CI)
helm lint --strict deploy/helm --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x --set alerts.enabled=true --set alerts.loki.enabled=true --set alerts.victoriaLogs.enabled=true --set alerts.victoriaLogs.delivery=vmrule --set alerts.prometheus.enabled=true --set alerts.victoriaMetrics.enabled=true --set serviceMonitor.main.enabled=true --set dashboard.enabled=true
```
Expected: `SM renders`; default count `0`; the configmap render shows the ConfigMap + label + uid; the crd render shows the CR + uid; lint reports `0 chart(s) failed`.

- [ ] **Step 2: Confirm the dashboard JSON is byte-identical via both the symlink and `.Files.Get`**

Run:
```bash
diff <(cat deploy/observability/nexorious-dashboard.json) <(cat deploy/helm/files/nexorious-dashboard.json) && echo "symlink identical"
```
Expected: `symlink identical` (single source of truth confirmed).

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin feat/912-observability-deployment
gh pr create --title "feat: observability deployment — local Grafana stack + Helm ServiceMonitor & dashboard" --body "$(cat <<'EOF'
Implements #912 (part of the #909 observability epic).

## What
- **Dashboard JSON** — single source of truth at `deploy/observability/nexorious-dashboard.json`, symlinked into `deploy/helm/files/`. Panels for sync throughput/success, River job throughput & p95 duration, external-API rate/latency/5xx, DB p95 latency & errors, and scrape-target up — all using the real exported metric names.
- **Helm ServiceMonitor** — native bjw-s common `serviceMonitor.main` (pure values, default off); scrapes the nexorious Service `http` port at `/metrics`.
- **Helm dashboard** — `dashboard.enabled` + `dashboard.mode: configmap|crd` (default off / configmap), one template via `.Files.Get`. configmap → ConfigMap labeled `grafana_dashboard: "1"`; crd → `GrafanaDashboard` with the same JSON.
- **Local dev stack** — new `deploy/docker/docker-compose.dev.yml` builds the app from source and runs `grafana/otel-lgtm` (traces via OTLP push, metrics scraped from `/metrics`) with the dashboard provisioned.
- Chart hygiene: `dashboard` registered in `values.schema.json`; `--set serviceMonitor.main.enabled=true --set dashboard.enabled=true` added to the CI helm-lint step; `helm lint --strict` green.
- Docs: `docs/observability.md` extended with dev-stack, ServiceMonitor, and dashboard sections.

## Acceptance criteria
- [x] `docker compose -f deploy/docker/docker-compose.dev.yml up` brings up otel-lgtm; Grafana shows the dashboard with live data.
- [x] `helm template` with `serviceMonitor.enabled=true` renders a valid ServiceMonitor targeting `/metrics`; off by default.
- [x] `dashboard.mode=configmap` renders a ConfigMap labeled `grafana_dashboard: "1"`; `dashboard.mode=crd` renders a `GrafanaDashboard` with the same JSON; both off by default.
- [x] `helm lint --strict` stays green.

Closes #912
EOF
)"
```

---

## Self-Review

**Spec coverage** (issue #912 tasks → plan tasks):
- Local dev stack (otel-lgtm in `docker-compose.dev.yml`, OTLP endpoint, Prometheus scrapes `/metrics`) → **Task 4**.
- Dashboard JSON, single source of truth, reused by dev + Helm → **Task 1** (source) + consumed in **Task 3** (Helm) and **Task 4** (dev).
- Helm ServiceMonitor (native bjw-s, single `enabled`, default off, scrapes `http`/`/metrics`) → **Task 2**.
- Helm dashboard (two modes, one toggle, `.Files.Get`, configmap label + crd instanceSelector) → **Task 3**.
- Chart hygiene (schema registration, `--set` flags, `helm lint --strict`, jobLabel verification) → **Tasks 2, 3, 6**.
- All four acceptance criteria → **Task 6** (mechanical) + **Task 4 Step 6** (manual live-data check).

**Placeholder scan:** every code/step block contains complete, runnable content; no TBD/TODO. The only intentionally-manual step (Task 4 Step 6, live `docker compose up`) is labeled as the acceptance check and is not automatable in CI.

**Type/name consistency:** dashboard uid `nexorious-observability`, file name `nexorious-dashboard.json`, ConfigMap data key `nexorious-dashboard.json`, values keys `serviceMonitor.main.enabled` / `dashboard.enabled` / `dashboard.mode` / `dashboard.instanceSelector`, and Prometheus `job_name: nexorious` are used identically across Tasks 1–6 and the `up{job=~".*nexorious.*"}` panel.
