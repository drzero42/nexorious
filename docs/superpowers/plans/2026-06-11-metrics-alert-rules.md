# Metrics-based Alert Rules (#913) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add metrics-based alert rules (PromQL) over the OTel metrics exposed at `/metrics` by #910, with opt-in Helm delivery as `PrometheusRule` and `VMRule`, plus a new `nexorious_db_errors_total` counter so DB-error alerting has a real signal.

**Architecture:** Mirror the already-shipped log-based alerting from #908 exactly: a single raw-YAML source of truth in `deploy/observability/prometheus-rules.yaml`, symlinked into `deploy/helm/files/`, wrapped into the two CRD kinds by `templates/alerts-*.yaml` via `.Files.Get`, all gated behind the existing `alerts.*` Helm values block (default off). The DB-error signal that bunotel does not emit is added as a tiny custom bun `QueryHook` in `internal/observability` that increments an OTel counter.

**Tech Stack:** Go (OTel SDK, bun `QueryHook`), Helm (bjw-s common chart), PromQL/MetricsQL, Prometheus Operator (`PrometheusRule`) + VictoriaMetrics Operator (`VMRule`).

---

## Background: ground-truth metric names (verified by reading the wired instrumentation)

The issue's example PromQL is partly wrong. These are the **actual** series the running app exports (OTel→Prometheus exporter rendering — counters get `_total`, dots→`_`, histogram unit `s`→`_seconds`). Names marked "validate" depend on the exporter's unit-suffix rendering and should be confirmed against a live `/metrics` scrape, but the rules ship as documented, tunable starting points (same posture as #908).

| Series | Labels (values) | Source |
|---|---|---|
| `nexorious_sync_total` | `source`, `status` ∈ {`completed`, `completed_with_errors`} | `internal/observability/observability.go` `RecordSyncOutcome` |
| `nexorious_sync_items_total` | `source`, `outcome` ∈ {`completed`, `failed`, `skipped`} | `RecordSyncItems` |
| `nexorious_db_errors_total` | `operation` (SELECT/INSERT/…) | **NEW — Task 1** |
| `river_work_count_total` | `status` ∈ {`ok`, `error`, `panic`} | `otelriver` `river.work_count` |
| `river_work_duration_histogram_seconds_bucket` / `_count` / `_sum` | `status` | `otelriver` `river.work_duration_histogram` (NOTE: plain `river.work_duration` is a **gauge**, unusable for p95) |
| `http_client_request_duration_seconds_bucket` / `_count` / `_sum` | `server_address`, `http_request_method`, `http_response_status_code` | `otelhttp` client transport |
| `go_sql_query_timing_milliseconds_bucket` / `_count` / `_sum` (validate suffix) | `db_operation`, `db_sql_table`, `db_system` | `bunotel` query timing (no error/status label — hence Task 1) |
| `db_client_connections_*` (validate) | pool gauges | `otelsql.ReportDBStatsMetrics` (via bunotel `Init`) |
| `up` | `job`, `instance` | Prometheus scrape target health |

Key corrections vs. the issue text:
- `nexorious_sync_total{status="error"}` does **not** exist → error signal is the `completed_with_errors` ratio + `nexorious_sync_items_total{outcome="failed"}`.
- `river.work_duration` p95 → must use the `_histogram` series; work status is `ok|error|panic`.
- bunotel emits **no DB error/status metric** → Task 1 adds `nexorious_db_errors_total`.

---

## File Structure

- **Create** `internal/observability/dbhook.go` — bun `QueryHook` that increments `nexorious_db_errors_total` on real DB errors.
- **Create** `internal/observability/dbhook_test.go` — unit test using an OTel manual reader.
- **Modify** `internal/observability/observability.go` — declare the `dbErrors` counter, create it in `initInstruments`, add `RecordDBError`.
- **Modify** `cmd/nexorious/serve.go` — register the new hook alongside the bunotel hook.
- **Create** `deploy/observability/prometheus-rules.yaml` — single source-of-truth PromQL rule groups.
- **Create** `deploy/helm/files/prometheus-rules.yaml` — symlink → `../../observability/prometheus-rules.yaml`.
- **Create** `deploy/helm/templates/alerts-prometheus.yaml` — `PrometheusRule` CRD, gated, default off.
- **Create** `deploy/helm/templates/alerts-victoriametrics.yaml` — `VMRule` CRD (prometheus-type groups), gated, default off.
- **Modify** `deploy/helm/values.yaml` — extend `alerts:` with `prometheus` + `victoriaMetrics` sub-blocks.
- **Modify** `deploy/helm/values.schema.json` — register the two new sub-blocks (`additionalProperties: false`).
- **Modify** `.github/workflows/test.yaml` — add `--set alerts.prometheus.enabled=true --set alerts.victoriaMetrics.enabled=true` to the lint step.
- **Modify** `deploy/observability/alertmanager-route.example.yaml` — add the metrics alert names/severities to the documented example route.

---

## Task 1: `nexorious_db_errors_total` counter + bun QueryHook

**Files:**
- Modify: `internal/observability/observability.go` (instrument vars ~line 42; `initInstruments` ~line 166; add `RecordDBError` after `RecordSyncItems` ~line 209)
- Create: `internal/observability/dbhook.go`
- Create: `internal/observability/dbhook_test.go`
- Modify: `cmd/nexorious/serve.go:97-100` (after the bunotel `AddQueryHook`)

- [ ] **Step 1: Write the failing test**

Create `internal/observability/dbhook_test.go`:

```go
package observability

import (
	"context"
	"database/sql"
	"testing"

	"github.com/uptrace/bun"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// collectDBErrors returns the summed value of nexorious_db_errors across all
// attribute sets, or 0 if the instrument has not recorded anything.
func collectDBErrors(t *testing.T, reader sdkmetric.Reader) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "nexorious_db_errors" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("nexorious_db_errors is %T, want Sum[int64]", m.Data)
			}
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	return total
}

func TestDBErrorHook(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	initInstruments(mp)

	hook := NewDBErrorHook()
	ctx := context.Background()

	// A real, non-ErrNoRows error must increment the counter.
	e := &bun.QueryEvent{Query: "SELECT 1", Err: sql.ErrConnDone}
	hook.AfterQuery(ctx, e)
	if got := collectDBErrors(t, reader); got != 1 {
		t.Fatalf("after real error: got %d, want 1", got)
	}

	// sql.ErrNoRows is "not found", not a failure — must NOT increment.
	noRows := &bun.QueryEvent{Query: "SELECT 1", Err: sql.ErrNoRows}
	hook.AfterQuery(ctx, noRows)
	if got := collectDBErrors(t, reader); got != 1 {
		t.Fatalf("after ErrNoRows: got %d, want 1 (unchanged)", got)
	}

	// A successful query (no error) must NOT increment.
	ok := &bun.QueryEvent{Query: "SELECT 1", Err: nil}
	hook.AfterQuery(ctx, ok)
	if got := collectDBErrors(t, reader); got != 1 {
		t.Fatalf("after success: got %d, want 1 (unchanged)", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/observability/ -run TestDBErrorHook -v`
Expected: FAIL — `NewDBErrorHook` and the `dbErrors` instrument are undefined (compile error).

- [ ] **Step 3: Declare the counter and `RecordDBError`**

In `internal/observability/observability.go`, add `dbErrors` to the instrument var block (after `syncItemsTotal otelmetric.Int64Counter`, ~line 43):

```go
	syncTotal      otelmetric.Int64Counter
	syncItemsTotal otelmetric.Int64Counter
	dbErrors       otelmetric.Int64Counter
```

In `initInstruments`, after the `syncItemsTotal` block (~line 182), create it:

```go
	dbErrors, err = m.Int64Counter(
		"nexorious_db_errors",
		otelmetric.WithDescription("Count of failed database queries by SQL operation (excludes ErrNoRows / context cancellation)."),
	)
	if err != nil {
		slog.Error("observability: failed to create nexorious_db_errors counter", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
	}
```

After `RecordSyncItems` (~line 209), add the recorder:

```go
// RecordDBError records one failed database query, labeled by SQL operation
// (SELECT/INSERT/…). Operation cardinality is bounded; never label by table or
// full query text. A nil instrument (metrics disabled) is a no-op.
func RecordDBError(ctx context.Context, operation string) {
	if dbErrors == nil {
		return
	}
	dbErrors.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("operation", operation),
	))
}
```

- [ ] **Step 4: Write the hook**

Create `internal/observability/dbhook.go`:

```go
package observability

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"
)

// dbErrorHook is a bun.QueryHook that counts failed queries into
// nexorious_db_errors_total. bunotel emits query timing but no error signal, so
// this fills the gap for DB-error alerting (#913). It is intentionally minimal:
// no spans, no extra labels beyond the SQL operation.
type dbErrorHook struct{}

// NewDBErrorHook returns a bun.QueryHook recording DB query failures as the
// nexorious_db_errors_total metric. Register it alongside the bunotel hook.
func NewDBErrorHook() bun.QueryHook { return dbErrorHook{} }

func (dbErrorHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (dbErrorHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	if event.Err == nil {
		return
	}
	// ErrNoRows is an expected "not found" result, not a failure; context
	// cancellation is shutdown/timeout noise, not a DB fault.
	if errors.Is(event.Err, sql.ErrNoRows) || errors.Is(event.Err, context.Canceled) {
		return
	}
	RecordDBError(ctx, event.Operation())
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/observability/ -run TestDBErrorHook -v`
Expected: PASS.

- [ ] **Step 6: Wire the hook in serve.go**

In `cmd/nexorious/serve.go`, immediately after the existing bunotel `db.AddQueryHook(...)` call (ends ~line 100), add:

```go
	// Counts failed queries into nexorious_db_errors_total — bunotel records
	// timing but no error signal (#913). No-op when metrics are disabled.
	db.AddQueryHook(observability.NewDBErrorHook())
```

- [ ] **Step 7: Build + commit**

Run: `go build ./...`
Expected: no errors.

```bash
git add internal/observability/observability.go internal/observability/dbhook.go internal/observability/dbhook_test.go cmd/nexorious/serve.go
git commit -m "feat: add nexorious_db_errors_total counter and bun query-error hook"
```

---

## Task 2: source-of-truth PromQL rules

**Files:**
- Create: `deploy/observability/prometheus-rules.yaml`
- Create: `deploy/helm/files/prometheus-rules.yaml` (symlink)

- [ ] **Step 1: Write the rules file**

Create `deploy/observability/prometheus-rules.yaml`. This file is the **single source of truth** for both the `PrometheusRule` and `VMRule` Helm templates (Tasks 3–4) and is directly usable without Helm.

```yaml
# Nexorious — Prometheus / VictoriaMetrics alerting rules (PromQL / MetricsQL).
#
# SINGLE SOURCE OF TRUTH. Three ways to use it:
#   1. Non-Helm Prometheus: reference this file from prometheus.yml
#        rule_files: [ /etc/prometheus/rules/prometheus-rules.yaml ]
#      then reload (POST /-/reload or SIGHUP).
#   2. Non-Helm vmalert: vmalert -rule=/etc/vmalert/prometheus-rules.yaml
#      (groups are prometheus-type — the vmalert default).
#   3. Helm: set alerts.enabled=true and alerts.prometheus.enabled=true (renders
#      a PrometheusRule) and/or alerts.victoriaMetrics.enabled=true (a VMRule).
#      Both templates wrap THIS exact file via .Files.Get.
#
# METRIC NAMES come from the OTel→Prometheus exporter (#910): monotonic counters
# carry _total; histogram seconds carry _seconds_bucket/_count/_sum. If you run
# multiple apps in one Prometheus, scope each expr by the job/namespace label
# your scrape config adds. Thresholds/windows are conservative starting points —
# tune to your traffic. A couple of names (go_sql_query_timing_*,
# db_client_connections_*) depend on the exporter's unit rendering — confirm
# against your live /metrics and adjust if your build labels them differently.
#
# Known coverage gap (by design, see #909): there are NO HTTP-server request
# metrics (we skip a custom echo v5 middleware), so server-side 5xx/latency
# alerts are out of scope — coverage is sync / jobs / external APIs / DB /
# target-up.
groups:
  - name: nexorious-sync
    rules:
      - alert: NexoriousSyncErrorRatioHigh
        # Share of completed sync jobs that finished with item errors, per source.
        expr: |
          sum by (source) (rate(nexorious_sync_total{status="completed_with_errors"}[15m]))
            /
          sum by (source) (rate(nexorious_sync_total[15m]))
            > 0.5
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "High sync error ratio for {{ $labels.source }}"
          description: "Over 50% of {{ $labels.source }} sync jobs in the last 15m finished with errors ({{ $value | humanizePercentage }})."

      - alert: NexoriousSyncItemFailuresHigh
        # Share of synced items that failed, per source.
        expr: |
          sum by (source) (rate(nexorious_sync_items_total{outcome="failed"}[30m]))
            /
          sum by (source) (rate(nexorious_sync_items_total[30m]))
            > 0.2
        for: 30m
        labels:
          severity: warning
        annotations:
          summary: "High per-item sync failure rate for {{ $labels.source }}"
          description: "Over 20% of {{ $labels.source }} synced items failed in the last 30m ({{ $value | humanizePercentage }})."

  - name: nexorious-jobs
    rules:
      - alert: NexoriousJobErrorRatioHigh
        # River worker error+panic share across all job kinds.
        expr: |
          sum (rate(river_work_count_total{status=~"error|panic"}[10m]))
            /
          sum (rate(river_work_count_total[10m]))
            > 0.1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Elevated River job failure rate"
          description: "Over 10% of River jobs failed or panicked in the last 10m ({{ $value | humanizePercentage }})."

      - alert: NexoriousJobPanics
        expr: increase(river_work_count_total{status="panic"}[10m]) > 0
        for: 0m
        labels:
          severity: critical
        annotations:
          summary: "River worker panic"
          description: "{{ $value }} River job panic(s) in the last 10m — a worker hit an unrecovered panic."

      - alert: NexoriousJobDurationP95High
        expr: |
          histogram_quantile(0.95,
            sum by (le) (rate(river_work_duration_histogram_seconds_bucket[10m]))
          ) > 300
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "River job p95 duration over 5m"
          description: "p95 job work duration has exceeded 300s for 15m — workers may be slow or stuck."

  - name: nexorious-external-api
    rules:
      - alert: NexoriousExternalAPIErrorRatioHigh
        # 5xx share of outbound HTTP client calls, per upstream host.
        expr: |
          sum by (server_address) (rate(http_client_request_duration_seconds_count{http_response_status_code=~"5.."}[10m]))
            /
          sum by (server_address) (rate(http_client_request_duration_seconds_count[10m]))
            > 0.2
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High 5xx rate calling {{ $labels.server_address }}"
          description: "Over 20% of outbound requests to {{ $labels.server_address }} returned 5xx in the last 10m ({{ $value | humanizePercentage }})."

      - alert: NexoriousExternalAPILatencyP95High
        expr: |
          histogram_quantile(0.95,
            sum by (le, server_address) (rate(http_client_request_duration_seconds_bucket[10m]))
          ) > 10
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "External API p95 latency high for {{ $labels.server_address }}"
          description: "p95 request duration to {{ $labels.server_address }} has exceeded 10s for 15m."

  - name: nexorious-database
    rules:
      - alert: NexoriousDBErrorsHigh
        expr: sum (rate(nexorious_db_errors_total[10m])) > 0.1
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Elevated database error rate"
          description: "Database queries are failing at {{ $value | printf \"%.2f\" }}/s (excluding not-found) over the last 10m."

      - alert: NexoriousDBLatencyP95High
        # bunotel records query timing in milliseconds; threshold is 500ms.
        expr: |
          histogram_quantile(0.95,
            sum by (le) (rate(go_sql_query_timing_milliseconds_bucket[10m]))
          ) > 500
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Database query p95 latency over 500ms"
          description: "p95 query timing has exceeded 500ms for 15m — the database or connection pool may be saturated."

  - name: nexorious-target
    rules:
      - alert: NexoriousTargetDown
        # Adjust the job matcher to your ServiceMonitor's generated job label.
        expr: up{job=~".*nexorious.*"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Nexorious scrape target down"
          description: "Prometheus has failed to scrape the nexorious /metrics endpoint for 5m — the instance may be down or unreachable."
```

- [ ] **Step 2: Create the Helm symlink (matches the #908 pattern)**

Run:

```bash
cd deploy/helm/files
ln -s ../../observability/prometheus-rules.yaml prometheus-rules.yaml
cd -
ls -l deploy/helm/files/prometheus-rules.yaml
```

Expected: symlink `prometheus-rules.yaml -> ../../observability/prometheus-rules.yaml` (same form as the existing `loki-rules.yaml` / `victorialogs-rules.yaml` symlinks).

- [ ] **Step 3: Sanity-check the YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('deploy/observability/prometheus-rules.yaml'))" && echo OK`
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
git add deploy/observability/prometheus-rules.yaml deploy/helm/files/prometheus-rules.yaml
git commit -m "feat: add metrics-based alert rules source-of-truth (#913)"
```

---

## Task 3: PrometheusRule Helm template

**Files:**
- Create: `deploy/helm/templates/alerts-prometheus.yaml`

- [ ] **Step 1: Write the template**

Create `deploy/helm/templates/alerts-prometheus.yaml` (mirrors `alerts-victorialogs.yaml`, distinct `-metrics-alerts` name to avoid colliding with the log-based VMRule):

```yaml
{{- if and .Values.alerts.enabled .Values.alerts.prometheus.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: {{ include "nexorious.fullname" . }}-metrics-alerts
  labels:
    app.kubernetes.io/name: {{ include "nexorious.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ include "nexorious.chart" . }}
    {{- with .Values.alerts.prometheus.ruleLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
{{ .Files.Get "files/prometheus-rules.yaml" | indent 2 }}
{{- end }}
```

- [ ] **Step 2: Render to verify (off by default)**

Run:
```bash
helm template deploy/helm/ \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  | grep -c 'kind: PrometheusRule'
```
Expected: `0` (default off).

- [ ] **Step 3: Render with the toggle on**

Run:
```bash
helm template deploy/helm/ \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.prometheus.enabled=true \
  | grep -A2 'kind: PrometheusRule'
```
Expected: a `PrometheusRule` named `<release>-nexorious-metrics-alerts` with a `spec:` containing `groups:`.

(Note: this step depends on Task 5 adding `alerts.prometheus` to `values.yaml`; if running tasks in order, defer the assertion until after Task 5. The template itself is committed now.)

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/templates/alerts-prometheus.yaml
git commit -m "feat: add PrometheusRule Helm template for metrics alerts (#913)"
```

---

## Task 4: VMRule Helm template

**Files:**
- Create: `deploy/helm/templates/alerts-victoriametrics.yaml`

- [ ] **Step 1: Write the template**

Create `deploy/helm/templates/alerts-victoriametrics.yaml`. Same source file; VMRule groups default to prometheus type, so no `type:` is needed:

```yaml
{{- if and .Values.alerts.enabled .Values.alerts.victoriaMetrics.enabled }}
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: {{ include "nexorious.fullname" . }}-metrics-alerts
  labels:
    app.kubernetes.io/name: {{ include "nexorious.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ include "nexorious.chart" . }}
    {{- with .Values.alerts.victoriaMetrics.ruleLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
{{ .Files.Get "files/prometheus-rules.yaml" | indent 2 }}
{{- end }}
```

- [ ] **Step 2: Render with the toggle on (defer assertion until after Task 5 if running in order)**

Run:
```bash
helm template deploy/helm/ \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.victoriaMetrics.enabled=true \
  | grep -A2 'kind: VMRule'
```
Expected: a `VMRule` named `<release>-nexorious-metrics-alerts` whose `spec:` contains `groups:`.

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/templates/alerts-victoriametrics.yaml
git commit -m "feat: add VMRule Helm template for metrics alerts (#913)"
```

---

## Task 5: values.yaml + values.schema.json

**Files:**
- Modify: `deploy/helm/values.yaml` (inside the `alerts:` block, after the `victoriaLogs:` sub-block, ~line 181)
- Modify: `deploy/helm/values.schema.json` (inside `alerts.properties`, after `victoriaLogs`, ~line 240)

- [ ] **Step 1: Extend values.yaml**

In `deploy/helm/values.yaml`, append two sub-blocks inside `alerts:` (after the `victoriaLogs:` block, keeping the same indentation as `loki:` / `victoriaLogs:`):

```yaml
  prometheus:
    # -- Render a PrometheusRule (requires alerts.enabled) for the
    # -- Prometheus Operator / kube-prometheus-stack. The rules query the OTel
    # -- metrics at /metrics (#910). Default off.
    enabled: false
    # -- Extra labels on the PrometheusRule so your Prometheus's ruleSelector
    # -- matches it (e.g. {release: kube-prometheus-stack}). Leave empty if the
    # -- Prometheus selects all PrometheusRules.
    ruleLabels: {}

  victoriaMetrics:
    # -- Render a VMRule (requires alerts.enabled) consumed by the
    # -- VictoriaMetrics Operator (rendered into vmalert). Same rules as the
    # -- PrometheusRule above. Default off.
    enabled: false
    # -- Extra labels on the VMRule so the target VMAlert's ruleSelector matches
    # -- it (leave empty if the VMAlert uses selectAllByDefault: true).
    ruleLabels: {}
```

- [ ] **Step 2: Extend values.schema.json**

In `deploy/helm/values.schema.json`, inside `alerts.properties` (after the `victoriaLogs` property object closes), add:

```json
        "prometheus": {
          "type": "object",
          "additionalProperties": false,
          "description": "PrometheusRule (Prometheus Operator) delivery for metrics-based alerts.",
          "properties": {
            "enabled": {
              "type": "boolean",
              "description": "Render the PrometheusRule (requires alerts.enabled)."
            },
            "ruleLabels": {
              "type": "object",
              "additionalProperties": { "type": "string" },
              "description": "Extra labels on the PrometheusRule for ruleSelector matching."
            }
          }
        },
        "victoriaMetrics": {
          "type": "object",
          "additionalProperties": false,
          "description": "VMRule (VictoriaMetrics Operator) delivery for metrics-based alerts.",
          "properties": {
            "enabled": {
              "type": "boolean",
              "description": "Render the VMRule (requires alerts.enabled)."
            },
            "ruleLabels": {
              "type": "object",
              "additionalProperties": { "type": "string" },
              "description": "Extra labels on the VMRule for VMAlert ruleSelector matching."
            }
          }
        }
```

(Insert as new sibling properties; ensure the preceding `victoriaLogs` property object is followed by a comma.)

- [ ] **Step 3: Validate schema JSON parses**

Run: `python3 -c "import json; json.load(open('deploy/helm/values.schema.json'))" && echo OK`
Expected: `OK`.

- [ ] **Step 4: Verify both templates now render**

Run:
```bash
helm template deploy/helm/ \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.prometheus.enabled=true --set alerts.victoriaMetrics.enabled=true \
  | grep -E 'kind: (PrometheusRule|VMRule)'
```
Expected: both `kind: PrometheusRule` and `kind: VMRule` appear.

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/values.yaml deploy/helm/values.schema.json
git commit -m "feat: add alerts.prometheus + alerts.victoriaMetrics values + schema (#913)"
```

---

## Task 6: CI lint flags

**Files:**
- Modify: `.github/workflows/test.yaml:174-177` (the `helm lint --strict` invocation)

- [ ] **Step 1: Add the new toggles to the lint step**

In `.github/workflows/test.yaml`, add two `--set` lines to the `helm lint --strict` command (inside the existing `for vl_delivery` loop, after `--set alerts.victoriaLogs.delivery=$vl_delivery`):

```yaml
              --set alerts.victoriaLogs.delivery=$vl_delivery \
              --set alerts.prometheus.enabled=true \
              --set alerts.victoriaMetrics.enabled=true
```

- [ ] **Step 2: Run helm lint locally to mirror CI**

Run:
```bash
helm dependency build deploy/helm/ 2>/dev/null || true
helm lint --strict deploy/helm/ \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set alerts.enabled=true --set alerts.loki.enabled=true \
  --set alerts.victoriaLogs.enabled=true --set alerts.victoriaLogs.delivery=vmrule \
  --set alerts.prometheus.enabled=true --set alerts.victoriaMetrics.enabled=true
```
Expected: `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/test.yaml
git commit -m "ci: lint metrics-alert toggles in helm lint matrix (#913)"
```

---

## Task 7: Alertmanager route example

**Files:**
- Modify: `deploy/observability/alertmanager-route.example.yaml`

- [ ] **Step 1: Read the existing example**

Run: `cat deploy/observability/alertmanager-route.example.yaml`
Note the existing structure (it documents routing for #908's log-based alerts).

- [ ] **Step 2: Extend it with the metrics alerts**

Add a commented section (matching the file's existing style/voice) noting the new metrics alert names and that they share the same `severity` label convention (`warning` / `critical`) as the log-based rules, so an operator's existing severity-based routes already cover them. List the new alert names for discoverability:

```yaml
# --- Metrics-based alerts (#913, deploy/observability/prometheus-rules.yaml) ---
# These reuse the same severity labels (warning/critical) as the log-based rules
# above, so a severity-based route needs no changes. Alert names:
#   NexoriousSyncErrorRatioHigh, NexoriousSyncItemFailuresHigh,
#   NexoriousJobErrorRatioHigh, NexoriousJobPanics, NexoriousJobDurationP95High,
#   NexoriousExternalAPIErrorRatioHigh, NexoriousExternalAPILatencyP95High,
#   NexoriousDBErrorsHigh, NexoriousDBLatencyP95High, NexoriousTargetDown
```

(Adapt indentation/placement to the actual file; if it is pure YAML rather than commented prose, add the names as a comment block at the end.)

- [ ] **Step 3: Commit**

```bash
git add deploy/observability/alertmanager-route.example.yaml
git commit -m "docs: document metrics alerts in Alertmanager route example (#913)"
```

---

## Task 8: Full verification

- [ ] **Step 1: Go build + targeted test**

Run: `go build ./... && go test ./internal/observability/ -v`
Expected: build clean; all observability tests pass.

- [ ] **Step 2: Go lint the changed packages**

Run: `golangci-lint run ./internal/observability/... ./cmd/nexorious/...`
Expected: no findings.

- [ ] **Step 3: helm lint (both VictoriaLogs delivery modes, mirroring CI)**

Run:
```bash
for vl in vmrule configmap; do
  helm lint --strict deploy/helm/ \
    --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
    --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
    --set alerts.enabled=true --set alerts.loki.enabled=true \
    --set alerts.victoriaLogs.enabled=true --set alerts.victoriaLogs.delivery=$vl \
    --set alerts.prometheus.enabled=true --set alerts.victoriaMetrics.enabled=true || exit 1
done
echo "ALL GREEN"
```
Expected: `ALL GREEN`.

- [ ] **Step 4: Confirm default-off**

Run:
```bash
helm template deploy/helm/ --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  | grep -cE 'kind: (PrometheusRule|VMRule)'
```
Expected: `0`.

- [ ] **Step 5: promtool check (if available) — validate rule syntax**

Run: `command -v promtool >/dev/null && promtool check rules deploy/observability/prometheus-rules.yaml || echo "promtool not installed — skipping (CI/helm lint still gate structure)"`
Expected: `SUCCESS` or the skip message. (promtool is not a project dependency; this is best-effort.)

- [ ] **Step 6: Push (pre-push hooks run full suites) and open the PR**

```bash
git push -u origin feat/913-metrics-alert-rules
```

Then open a PR titled `feat: metrics-based alert rules (PrometheusRule + VMRule) (#913)` with body `Closes #913`, summarizing: the new `nexorious_db_errors_total` counter + hook, the source-of-truth rules, the two opt-in CRD templates, and the metric-name corrections vs. the issue.

---

## Self-Review notes

- **Spec coverage:** Part 1 rules → Task 2 (all categories: sync, jobs, external, DB, target-up; DB error-rate now backed by Task 1's real counter). Part 2 Helm delivery → Tasks 3–5 (`PrometheusRule` + `VMRule`, default off, composing with #908's `alerts.*`). schema + CI → Tasks 5–6. Alertmanager example → Task 7. Acceptance criteria (helm lint green, raw YAML usable, default off) → Task 8.
- **Out of scope (honored):** no Alertmanager receivers/secrets (only the example route); no HTTP-server request alerts (no such metrics); no log-based rules (those are #908).
- **Known caveat surfaced in-file:** a few exporter-rendered names (`go_sql_query_timing_milliseconds_*`, `db_client_connections_*`) depend on unit rendering and are flagged for live-scrape confirmation — acceptance is helm-lint-based, not live-data-based.
