# Loki + VictoriaLogs Alert Rules with Opt-in Helm Delivery (#908) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship log-based alert rules for both supported log backends (Grafana Loki ruler + VictoriaLogs/vmalert) as a single source of truth under `deploy/observability/`, wrapped by opt-in Helm templates (a labeled Loki `ConfigMap` and a `VMRule` CRD) that default off, plus an operator-facing guide with an example Alertmanager route.

**Architecture:** The raw rule files are the canonical artifacts (directly usable by non-Helm operators). The Helm chart reuses them verbatim via `.Files.Get` — Helm's `.Files` is **chart-scoped** and cannot read the sibling `deploy/observability/` directory, so the chart pulls the canonical files in through **symlinks inside `deploy/helm/files/`** (verified: Helm follows in-chart symlinks for both `helm template` from a directory and from a packaged `.tgz`). Rules filter on #907's stable structured fields (`level`, `category`, `source`, `msg`) — never free text where a `category` suffices. This is **infra + YAML + docs only**: no Go code, no schema migration, no behavior change.

**Tech Stack:** Grafana Loki ruler (LogQL), VictoriaLogs + vmalert + VictoriaMetrics Operator `VMRule` (LogsQL, group `type: vlogs`), Helm 3 (bjw-s common library chart), `helm lint --strict`, JSON Schema (draft-07) for `values.schema.json`.

---

## Context an implementer must know

### The structured-logging fields the rules target (all landed via #907/#924/#926)

nexorious emits JSON logs via `log/slog`. Confirmed field values (`docs/logging-conventions.md`, `internal/logging/`):

- `level`: `INFO` / `WARN` / `ERROR` / `DEBUG` (uppercase).
- `category` (set only on `warn`/`error` failure boundaries): `db`, `external_api`, `validation`, `auth`, `config`, `panic`.
- `source`: canonical storefront slug (`steam`, `playstation-store`, `gog`, `epic-games-store`, `humble-bundle`).
- `msg`: the human message.

Confirmed alert-relevant log lines that exist in the code today:

| Signal | Match | Source |
|---|---|---|
| DB connectivity lost / DBUnavailable | `msg="database unavailable"` (WARN, `category=db`) | `internal/migrate/migrator.go:353` |
| Migration failure | `category=db` + `level=ERROR` + `msg` starts `migrate:` | `internal/migrate/migrator.go:217,228,243,252` |
| River client init failure | `category=db` + `level=ERROR` + `msg` contains `river client init failed` | `cmd/nexorious/serve.go:248,347` |
| Recovered panic | `category=panic` (ERROR) | `internal/api/recover.go`, `internal/logging/error_handler.go` |
| Credential / auth failure | `category=auth` (WARN), `source=<slug>` | `cmd/nexorious/serve.go` decrypt warns, `internal/api/sync.go`, PSN client |
| Sync / external-API error | `category=external_api`, `source=<slug>` | `internal/worker/tasks/sync.go:258` etc. |
| Stuck job reaped | `msg` contains `marked stale` (WARN) | `internal/scheduler/stale_jobs.go:65,91` |
| Job enqueue/dispatch failure | `category=db` + `level=ERROR` + `msg` contains `enqueue` | `internal/scheduler/scheduler.go:219`, `internal/api/jobs.go:744` |

Some signals (DBUnavailable, migration-vs-runtime db error, stuck jobs, enqueue failures) share `category=db` and are only distinguishable by `msg`. Free-text `msg` matching is therefore **sanctioned for these**, scoped tight by `category`+`level`. Where a `category` alone is sufficient (panics, auth, external_api, generic error rate) the rules filter only on `category`/`level`. This rationale is documented in `docs/observability.md`.

### LogQL (Loki) — confirmed forms

- Rule file: `groups: → - name: / interval: / rules: → - alert: / expr: / for: / labels: / annotations:`.
- Parse + filter JSON: `{app="nexorious"} | json | category="db" | level="ERROR"`. String label filters use `=` / `=~` (regex). Chained `|` stages are ANDed.
- Threshold as instant vector: `sum(count_over_time({app="nexorious"} | json | category="db" [5m])) > 0`; per-source: `sum by (source) (...)`.
- `labels.severity` routes in Alertmanager; `annotations` (`summary`/`description`) support `{{ $value }}` / `{{ $labels.x }}`.
- The stream selector `{app="nexorious"}` depends on the operator's collector labels (promtail / grafana-alloy) — it is a documented default to adjust.

### LogsQL (VictoriaLogs/vmalert) — confirmed forms

- Rule file / `VMRule` group: `- name: / type: vlogs / interval: / rules:`. `type: vlogs` selects LogsQL evaluation.
- **Exact whole-value equality** uses `:=` with a quoted value: `category:="db"`, `level:="ERROR"`, `category:="external_api"`. (`category:db` is a substring/word match — do **not** use it for equality; underscore is a word char so `external_api` is one token but still a contains-match without `:=`.)
- Threshold: `... | stats count() as logs | filter logs:>10`. Per-source: `... | stats by (source) count() as logs | filter logs:>3` (lines lacking `source` fall into an empty-string bucket — harmless).
- **Window:** vmalert auto-appends `_time:<group interval>`. An explicit `_time:5m` at the **start of the expr replaces** the auto-injected one — so set `interval: 1m` (eval cadence) and prefix `_time:5m`/`_time:10m`/`_time:15m` (window). Explicit `_time` disables vmalert replay/backfill — irrelevant here.
- `VMRule`: `apiVersion: operator.victoriametrics.com/v1beta1`, `kind: VMRule`, `spec.groups[]` mirrors the rules-file groups (incl. `type: vlogs`). The target `VMAlert`'s `spec.datasource.url` must point at VictoriaLogs; its `ruleSelector` must match the `VMRule`'s labels (or use `selectAllByDefault`).
- VictoriaLogs has no per-app stream selector in the JSON — if multiple apps ship to one VictoriaLogs, operators must prepend an app-scoping filter (e.g. `app:="nexorious"` if their collector adds it). Documented prominently.

### Severity & thresholds (decided with the user)

Two-tier (`severity: critical` / `severity: warning`), conservative documented defaults:

| Rule | Window | Threshold | for | severity |
|---|---|---|---|---|
| `NexoriousDBUnavailable` | 5m | `> 0` | 2m | critical |
| `NexoriousStartupFailure` (migration / river init) | 5m | `> 0` | 0s | critical |
| `NexoriousPanicRecovered` | 5m | `> 0` | 0s | critical |
| `NexoriousCredentialFailures` (by source) | 15m | `> 3` | 0s | warning |
| `NexoriousSyncErrors` (by source) | 15m | `> 3` | 0s | warning |
| `NexoriousStuckJobs` | 15m | `> 0` | 0s | warning |
| `NexoriousJobDispatchFailures` | 10m | `> 0` | 0s | warning |
| `NexoriousHighErrorRate` | 10m | `> 10` | 0s | warning |

### Helm wrapping mechanism (verified, not the issue's literal `.Files.Get deploy/observability/`)

`.Files` cannot read outside the chart. The canonical files live in `deploy/observability/`; the chart reaches them via in-chart symlinks `deploy/helm/files/loki-rules.yaml → ../../observability/loki-rules.yaml` (and likewise for victorialogs). `helm template`/`helm package` follow these symlinks (verified with helm v3.20.2: *"Contents of linked file included and used"*). The templates use `.Files.Get "files/loki-rules.yaml"`. No `tpl` is used — the rules are static so they stay valid for non-Helm users; operators edit the YAML directly to retune.

### Why no Go/TDD tests here

This change adds zero Go code. Verification is `helm lint --strict`, `helm template` render checks, and parsing the rendered/raw YAML with `python3 -c 'import yaml,sys; yaml.safe_load(...)'`. LogQL/LogsQL expression validity is checked by careful authoring against the confirmed forms above (no `cortextool`/`vmalert -dryRun` in the devenv — listed as optional manual checks in the doc).

---

## File map

| File | Change |
|---|---|
| `deploy/observability/loki-rules.yaml` | **Create** — canonical Loki ruler rules (LogQL) |
| `deploy/observability/victorialogs-rules.yaml` | **Create** — canonical vmalert rules (LogsQL, `type: vlogs`) |
| `deploy/observability/alertmanager-route.example.yaml` | **Create** — example Alertmanager route (documented, not applied) |
| `deploy/helm/files/loki-rules.yaml` | **Create** — symlink → `../../observability/loki-rules.yaml` |
| `deploy/helm/files/victorialogs-rules.yaml` | **Create** — symlink → `../../observability/victorialogs-rules.yaml` |
| `deploy/helm/templates/alerts-loki.yaml` | **Create** — gated Loki ruler `ConfigMap` |
| `deploy/helm/templates/alerts-victorialogs.yaml` | **Create** — gated `VMRule` |
| `deploy/helm/values.yaml` | **Modify** — add `alerts:` block (default off) |
| `deploy/helm/values.schema.json` | **Modify** — register `alerts` (`additionalProperties: false`) |
| `.github/workflows/test.yaml` | **Modify** — add `--set alerts.*` to the `helm lint --strict` step |
| `deploy/helm/README.md` | **Modify** — document the `alerts.*` values |
| `docs/observability.md` | **Create** — alerting guide + example route |
| `docs/maintenance.md` | **Modify** — cross-link the new alerting guide |

---

## Task 1: Canonical raw rule files in `deploy/observability/`

**Files:**
- Create: `deploy/observability/loki-rules.yaml`
- Create: `deploy/observability/victorialogs-rules.yaml`
- Create: `deploy/observability/alertmanager-route.example.yaml`

- [ ] **Step 1: Create the Loki rules file**

Create `deploy/observability/loki-rules.yaml`:

```yaml
# Nexorious — Grafana Loki ruler alerting rules (LogQL).
#
# SINGLE SOURCE OF TRUTH. Two ways to use it:
#   1. Non-Helm: drop this file into your Loki ruler's per-tenant rule directory
#      (e.g. /rules/fake/ when auth_enabled: false), then reload the ruler.
#   2. Helm: set alerts.enabled=true and alerts.loki.enabled=true — the chart
#      wraps this exact file into a ConfigMap the ruler sidecar discovers.
#
# ADJUST the stream selector {app="nexorious"} to match the labels your log
# collector (promtail / grafana-alloy) attaches to nexorious container logs.
# Thresholds and windows are conservative starting points — tune to your volume.
#
# Fields (nexorious structured logging — see docs/logging-conventions.md):
#   level    INFO | WARN | ERROR | DEBUG
#   category db | auth | external_api | validation | config | panic  (warn/error only)
#   source   steam | playstation-store | gog | epic-games-store | humble-bundle
#   msg      human message
groups:
  - name: nexorious-db-startup
    interval: 1m
    rules:
      - alert: NexoriousDBUnavailable
        expr: |
          sum(count_over_time({app="nexorious"} | json | msg="database unavailable" [5m])) > 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Nexorious lost its database connection"
          description: "{{ $value }} 'database unavailable' log line(s) in 5m — the app is in the DBUnavailable state and is not serving normally."

      - alert: NexoriousStartupFailure
        expr: |
          sum(count_over_time({app="nexorious"} | json | category="db" | level="ERROR" | msg=~"^(migrate:|serve: river client init failed)" [5m])) > 0
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Nexorious migration or River startup failure"
          description: "{{ $value }} migration/startup failure line(s) in 5m — migrations failed or the River client failed to initialize; the app cannot reach Ready."

  - name: nexorious-panics
    interval: 1m
    rules:
      - alert: NexoriousPanicRecovered
        expr: |
          sum(count_over_time({app="nexorious"} | json | category="panic" [5m])) > 0
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Nexorious recovered a panic"
          description: "{{ $value }} recovered panic(s) in 5m (category=panic) — an HTTP request or worker job hit an unhandled error."

  - name: nexorious-credentials
    interval: 1m
    rules:
      - alert: NexoriousCredentialFailures
        expr: |
          sum by (source) (count_over_time({app="nexorious"} | json | category="auth" [15m])) > 3
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Repeated credential/auth failures for {{ $labels.source }}"
          description: "{{ $value }} auth-category failures for source '{{ $labels.source }}' in 15m — stored credentials are likely expired or invalid."

  - name: nexorious-sync-jobs
    interval: 1m
    rules:
      - alert: NexoriousSyncErrors
        expr: |
          sum by (source) (count_over_time({app="nexorious"} | json | category="external_api" [15m])) > 3
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Elevated sync / external-API errors for {{ $labels.source }}"
          description: "{{ $value }} external_api failures for source '{{ $labels.source }}' in 15m."

      - alert: NexoriousStuckJobs
        expr: |
          sum(count_over_time({app="nexorious"} | json | msg=~"marked stale" [15m])) > 0
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Nexorious reaped stuck jobs"
          description: "{{ $value }} stale-job reaping event(s) in 15m — jobs were stuck and force-marked failed."

      - alert: NexoriousJobDispatchFailures
        expr: |
          sum(count_over_time({app="nexorious"} | json | category="db" | level="ERROR" | msg=~"enqueue.*failed" [10m])) > 0
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Nexorious job enqueue/dispatch failures"
          description: "{{ $value }} job enqueue/dispatch failure(s) in 10m — River jobs may not have been scheduled."

  - name: nexorious-generic
    interval: 1m
    rules:
      - alert: NexoriousHighErrorRate
        expr: |
          sum(count_over_time({app="nexorious"} | json | level="ERROR" [10m])) > 10
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Elevated nexorious error-log rate"
          description: "{{ $value }} ERROR-level log lines in 10m across all categories — investigate for a broader fault."
```

- [ ] **Step 2: Create the VictoriaLogs rules file**

Create `deploy/observability/victorialogs-rules.yaml`:

```yaml
# Nexorious — VictoriaLogs alerting rules for vmalert (LogsQL, type: vlogs).
#
# SINGLE SOURCE OF TRUTH. Two ways to use it:
#   1. Non-Helm: point a vmalert instance at VictoriaLogs and load this file:
#        vmalert -datasource.url=http://victorialogs:9428 \
#                -rule.defaultRuleType=vlogs \
#                -notifier.url=http://alertmanager:9093 \
#                -rule=/etc/vmalert/victorialogs-rules.yaml
#      (each group already sets type: vlogs, so -rule.defaultRuleType is optional.)
#   2. Helm: set alerts.enabled=true and alerts.victoriaLogs.enabled=true — the
#      chart wraps these groups into a VMRule (VictoriaMetrics Operator).
#
# SCOPING: VictoriaLogs has no per-app field in the JSON. If multiple apps ship
# logs to the same VictoriaLogs, PREPEND an app-scoping filter to every expr
# (e.g. `app:="nexorious"` if your collector adds an `app` field), or these rules
# will count other apps' logs too. Thresholds/windows are conservative — tune.
#
# Each group sets interval: 1m (eval cadence); the window is the explicit _time:
# filter at the start of each expr (it replaces vmalert's auto-injected _time).
#
# Fields: level (INFO/WARN/ERROR/DEBUG), category (db/auth/external_api/
# validation/config/panic), source (storefront slug), msg.
groups:
  - name: nexorious-db-startup
    type: vlogs
    interval: 1m
    rules:
      - alert: NexoriousDBUnavailable
        expr: '_time:5m msg:="database unavailable" | stats count() as logs | filter logs:>0'
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Nexorious lost its database connection"
          description: "{{ $value }} 'database unavailable' log line(s) in 5m — the app is in the DBUnavailable state and is not serving normally."

      - alert: NexoriousStartupFailure
        expr: '_time:5m category:="db" level:="ERROR" (msg:"migrate:" OR msg:"river client init failed") | stats count() as logs | filter logs:>0'
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Nexorious migration or River startup failure"
          description: "{{ $value }} migration/startup failure line(s) in 5m — migrations failed or the River client failed to initialize; the app cannot reach Ready."

  - name: nexorious-panics
    type: vlogs
    interval: 1m
    rules:
      - alert: NexoriousPanicRecovered
        expr: '_time:5m category:="panic" | stats count() as logs | filter logs:>0'
        for: 0s
        labels:
          severity: critical
        annotations:
          summary: "Nexorious recovered a panic"
          description: "{{ $value }} recovered panic(s) in 5m (category=panic) — an HTTP request or worker job hit an unhandled error."

  - name: nexorious-credentials
    type: vlogs
    interval: 1m
    rules:
      - alert: NexoriousCredentialFailures
        expr: '_time:15m category:="auth" | stats by (source) count() as logs | filter logs:>3'
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Repeated credential/auth failures"
          description: "{{ $value }} auth-category failures for source '{{ index .Labels \"source\" }}' in 15m — stored credentials are likely expired or invalid."

  - name: nexorious-sync-jobs
    type: vlogs
    interval: 1m
    rules:
      - alert: NexoriousSyncErrors
        expr: '_time:15m category:="external_api" | stats by (source) count() as logs | filter logs:>3'
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Elevated sync / external-API errors"
          description: "{{ $value }} external_api failures for source '{{ index .Labels \"source\" }}' in 15m."

      - alert: NexoriousStuckJobs
        expr: '_time:15m msg:"marked stale" | stats count() as logs | filter logs:>0'
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Nexorious reaped stuck jobs"
          description: "{{ $value }} stale-job reaping event(s) in 15m — jobs were stuck and force-marked failed."

      - alert: NexoriousJobDispatchFailures
        expr: '_time:10m category:="db" level:="ERROR" msg:"enqueue" | stats count() as logs | filter logs:>0'
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Nexorious job enqueue/dispatch failures"
          description: "{{ $value }} job enqueue/dispatch failure(s) in 10m — River jobs may not have been scheduled."

  - name: nexorious-generic
    type: vlogs
    interval: 1m
    rules:
      - alert: NexoriousHighErrorRate
        expr: '_time:10m level:="ERROR" | stats count() as logs | filter logs:>10'
        for: 0s
        labels:
          severity: warning
        annotations:
          summary: "Elevated nexorious error-log rate"
          description: "{{ $value }} ERROR-level log lines in 10m across all categories — investigate for a broader fault."
```

- [ ] **Step 3: Create the example Alertmanager route**

Create `deploy/observability/alertmanager-route.example.yaml`:

```yaml
# Example Alertmanager routing for nexorious alerts. NOT applied by the chart —
# Alertmanager config and receiver secrets are the operator's own infra. Copy the
# relevant bits into your Alertmanager config and wire your own receivers.
#
# Both backends label alerts with severity: critical | warning. Route on that.
route:
  receiver: nexorious-default
  group_by: ['alertname', 'source']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h
  routes:
    - matchers:
        - severity = "critical"
      receiver: nexorious-critical
      group_wait: 0s
      repeat_interval: 1h
    - matchers:
        - severity = "warning"
      receiver: nexorious-warning

receivers:
  - name: nexorious-default
  - name: nexorious-critical
    # Replace with your real pager/webhook integration, e.g.:
    # pagerduty_configs:
    #   - service_key: <key>
  - name: nexorious-warning
    # Replace with your real chat integration, e.g.:
    # slack_configs:
    #   - api_url: <webhook-url>
    #     channel: '#alerts'
```

- [ ] **Step 4: Verify all three files are valid YAML**

Run:
```bash
python3 - <<'PY'
import yaml
for f in ["deploy/observability/loki-rules.yaml",
          "deploy/observability/victorialogs-rules.yaml",
          "deploy/observability/alertmanager-route.example.yaml"]:
    with open(f) as fh:
        yaml.safe_load(fh)
    print("OK", f)
PY
```
Expected: three `OK` lines, no traceback.

- [ ] **Step 5: Sanity-check rule coverage by grep**

Run:
```bash
grep -h "alert:" deploy/observability/loki-rules.yaml deploy/observability/victorialogs-rules.yaml | sort | uniq -c
```
Expected: each of the 8 alert names appears exactly twice (once per backend): `NexoriousCredentialFailures`, `NexoriousDBUnavailable`, `NexoriousHighErrorRate`, `NexoriousJobDispatchFailures`, `NexoriousPanicRecovered`, `NexoriousStartupFailure`, `NexoriousStuckJobs`, `NexoriousSyncErrors`.

- [ ] **Step 6: Commit**

```bash
git add deploy/observability/
git commit -m "feat: add Loki + VictoriaLogs alert rules (source of truth)"
```

---

## Task 2: In-chart symlinks so Helm can read the canonical files

Helm's `.Files` is chart-scoped and cannot read `deploy/observability/` (a sibling of the chart). The chart pulls the canonical files in via symlinks under `deploy/helm/files/`; `helm template`/`package` follow in-chart symlinks (verified).

**Files:**
- Create: `deploy/helm/files/loki-rules.yaml` (symlink)
- Create: `deploy/helm/files/victorialogs-rules.yaml` (symlink)

- [ ] **Step 1: Create the symlinks**

Run (relative targets, so they resolve in any checkout):
```bash
mkdir -p deploy/helm/files
ln -s ../../observability/loki-rules.yaml deploy/helm/files/loki-rules.yaml
ln -s ../../observability/victorialogs-rules.yaml deploy/helm/files/victorialogs-rules.yaml
```

- [ ] **Step 2: Verify the symlinks resolve to the canonical files**

Run:
```bash
readlink deploy/helm/files/loki-rules.yaml
readlink deploy/helm/files/victorialogs-rules.yaml
head -1 deploy/helm/files/loki-rules.yaml
head -1 deploy/helm/files/victorialogs-rules.yaml
```
Expected: the two `readlink` outputs are `../../observability/loki-rules.yaml` and `../../observability/victorialogs-rules.yaml`; both `head -1` lines start with `# Nexorious —` (content resolves through the link).

- [ ] **Step 3: Verify git records them as symlinks (mode 120000)**

Run:
```bash
git add deploy/helm/files
git ls-files -s deploy/helm/files
```
Expected: both entries show mode `120000` (symlink), not `100644`.

- [ ] **Step 4: Commit**

```bash
git commit -m "build: symlink alert rules into the Helm chart for .Files.Get"
```

---

## Task 3: Add the `alerts:` values block (default off)

**Files:**
- Modify: `deploy/helm/values.yaml`

- [ ] **Step 1: Append the `alerts:` block**

Add to `deploy/helm/values.yaml` immediately **after** the `nexorious:` block ends (after its `postgresql:` sub-block, before the `# Default Pod options` divider — i.e. just before the line `# =============================================================================` that precedes `defaultPodOptions:`). Insert:

```yaml
# =============================================================================
# Alert rules (opt-in) — log-based alerting for Loki and/or VictoriaLogs
# =============================================================================
# Wraps the source-of-truth rules in deploy/observability/ into each backend's
# canonical object. All default OFF so a base install stays clean. See
# docs/observability.md for setup and an example Alertmanager route.
alerts:
  # -- Master switch. Must be true for ANY alert object to render.
  enabled: false

  loki:
    # -- Render the Loki ruler ConfigMap (requires alerts.enabled). Your Loki
    # -- ruler must run a sidecar (e.g. kiwigrid/k8s-sidecar) that discovers
    # -- rule ConfigMaps by the label below and loads them into its rule dir.
    enabled: false
    # -- Discovery label the ruler sidecar watches. Default matches the common
    # -- self-configured sidecar convention; the bundled grafana/loki v3 chart
    # -- uses {key: loki.grafana.com/rule, value: "true"} instead — set to match
    # -- your sidecar's label/labelValue.
    ruleLabel:
      key: "loki_rule"
      value: "1"

  victoriaLogs:
    # -- Render the VMRule (requires alerts.enabled and the VictoriaMetrics
    # -- Operator CRDs). The target VMAlert must point its datasource at
    # -- VictoriaLogs and select this VMRule (ruleSelector or selectAllByDefault).
    enabled: false
    # -- Extra labels on the VMRule so the target VMAlert's ruleSelector matches
    # -- it. Leave empty if the VMAlert uses selectAllByDefault: true.
    ruleLabels: {}
```

- [ ] **Step 2: Verify values.yaml still parses**

Run:
```bash
python3 -c 'import yaml; yaml.safe_load(open("deploy/helm/values.yaml")); print("OK")'
```
Expected: `OK`.

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/values.yaml
git commit -m "feat: add opt-in alerts.* values (default off)"
```

---

## Task 4: Loki ruler `ConfigMap` template

**Files:**
- Create: `deploy/helm/templates/alerts-loki.yaml`

- [ ] **Step 1: Create the template**

Create `deploy/helm/templates/alerts-loki.yaml`:

```yaml
{{- if and .Values.alerts.enabled .Values.alerts.loki.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "nexorious.fullname" . }}-loki-alerts
  labels:
    app.kubernetes.io/name: {{ include "nexorious.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ include "nexorious.chart" . }}
    {{ .Values.alerts.loki.ruleLabel.key }}: {{ .Values.alerts.loki.ruleLabel.value | quote }}
data:
  nexorious-rules.yaml: |
{{ .Files.Get "files/loki-rules.yaml" | indent 4 }}
{{- end }}
```

- [ ] **Step 2: Render it and confirm the ConfigMap appears with the rules and label**

Run:
```bash
helm template t deploy/helm/ \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.loki.enabled=true \
  --show-only templates/alerts-loki.yaml
```
Expected: a `kind: ConfigMap` named `t-loki-alerts` (or `t-nexorious-loki-alerts` depending on fullname), carrying the label `loki_rule: "1"`, with `data.nexorious-rules.yaml` containing the `groups:` rules. (Helm logs the symlink-followed line to stderr — that is expected.)

- [ ] **Step 3: Confirm it does NOT render when alerts are off (default)**

Run:
```bash
helm template t deploy/helm/ \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --show-only templates/alerts-loki.yaml 2>&1 | tail -3
```
Expected: helm errors with "could not find template ... in chart" / empty output — i.e. nothing rendered (the gate holds; this non-zero exit is expected and fine).

- [ ] **Step 4: Confirm the embedded rules parse as YAML**

Run:
```bash
helm template t deploy/helm/ \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.loki.enabled=true \
  --show-only templates/alerts-loki.yaml 2>/dev/null \
  | python3 -c 'import yaml,sys; d=yaml.safe_load(sys.stdin); yaml.safe_load(d["data"]["nexorious-rules.yaml"]); print("rules parse OK")'
```
Expected: `rules parse OK`.

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/templates/alerts-loki.yaml
git commit -m "feat: add gated Loki ruler ConfigMap template"
```

---

## Task 5: `VMRule` template

**Files:**
- Create: `deploy/helm/templates/alerts-victorialogs.yaml`

- [ ] **Step 1: Create the template**

Create `deploy/helm/templates/alerts-victorialogs.yaml`:

```yaml
{{- if and .Values.alerts.enabled .Values.alerts.victoriaLogs.enabled }}
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: {{ include "nexorious.fullname" . }}-alerts
  labels:
    app.kubernetes.io/name: {{ include "nexorious.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
    helm.sh/chart: {{ include "nexorious.chart" . }}
    {{- with .Values.alerts.victoriaLogs.ruleLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
{{ .Files.Get "files/victorialogs-rules.yaml" | indent 2 }}
{{- end }}
```

(The canonical `victorialogs-rules.yaml` starts with `groups:`; indented 2 it sits correctly under `spec:`. The leading `#` comment lines become indented YAML comments — valid. Each group carries `type: vlogs`, which the operator passes through to the generated vmalert rules file.)

- [ ] **Step 2: Render it and confirm the VMRule appears**

Run:
```bash
helm template t deploy/helm/ \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.victoriaLogs.enabled=true \
  --set alerts.victoriaLogs.ruleLabels.project=nexorious \
  --show-only templates/alerts-victorialogs.yaml
```
Expected: `apiVersion: operator.victoriametrics.com/v1beta1`, `kind: VMRule`, metadata label `project: nexorious`, and `spec.groups` containing the vlogs rule groups (each with `type: vlogs`).

- [ ] **Step 3: Confirm the rendered VMRule is valid YAML and `spec.groups` is a list**

Run:
```bash
helm template t deploy/helm/ \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.victoriaLogs.enabled=true \
  --show-only templates/alerts-victorialogs.yaml 2>/dev/null \
  | python3 -c 'import yaml,sys; d=yaml.safe_load(sys.stdin); assert isinstance(d["spec"]["groups"], list) and d["spec"]["groups"][0]["type"]=="vlogs"; print("VMRule spec OK,", len(d["spec"]["groups"]), "groups")'
```
Expected: `VMRule spec OK, 5 groups`.

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/templates/alerts-victorialogs.yaml
git commit -m "feat: add gated VMRule template for VictoriaLogs alerts"
```

---

## Task 6: Register `alerts` in `values.schema.json` + update the CI lint step

**Files:**
- Modify: `deploy/helm/values.schema.json`
- Modify: `.github/workflows/test.yaml`

- [ ] **Step 1: Add the `alerts` property to the schema**

In `deploy/helm/values.schema.json`, the top-level `properties` object already contains `global` and `nexorious`. Add a sibling `alerts` property (insert after the `nexorious` property's closing brace, still inside top-level `properties`):

```json
    "alerts": {
      "type": "object",
      "additionalProperties": false,
      "description": "Opt-in log-based alert rules (Loki ruler ConfigMap + VictoriaLogs VMRule). Default off.",
      "properties": {
        "enabled": {
          "type": "boolean",
          "description": "Master switch. Must be true for any alert object to render."
        },
        "loki": {
          "type": "object",
          "additionalProperties": false,
          "description": "Loki ruler ConfigMap delivery.",
          "properties": {
            "enabled": {
              "type": "boolean",
              "description": "Render the Loki ruler ConfigMap (requires alerts.enabled)."
            },
            "ruleLabel": {
              "type": "object",
              "additionalProperties": false,
              "description": "Discovery label the Loki ruler sidecar watches.",
              "properties": {
                "key": { "type": "string" },
                "value": { "type": "string" }
              }
            }
          }
        },
        "victoriaLogs": {
          "type": "object",
          "additionalProperties": false,
          "description": "VictoriaLogs VMRule delivery (VictoriaMetrics Operator).",
          "properties": {
            "enabled": {
              "type": "boolean",
              "description": "Render the VMRule (requires alerts.enabled and the Operator CRDs)."
            },
            "ruleLabels": {
              "type": "object",
              "additionalProperties": { "type": "string" },
              "description": "Extra labels on the VMRule so the target VMAlert's ruleSelector matches it."
            }
          }
        }
      }
    },
```

(Take care with the trailing comma: the property inserted before `alerts` must end with `},` and `alerts` itself ends with `},` if another property follows, or no comma if it is last in `properties`. Match the existing file's structure exactly.)

- [ ] **Step 2: Verify the schema is valid JSON**

Run:
```bash
python3 -c 'import json; json.load(open("deploy/helm/values.schema.json")); print("schema JSON OK")'
```
Expected: `schema JSON OK`.

- [ ] **Step 3: Add the `--set alerts.*` flags to the helm lint step**

In `.github/workflows/test.yaml`, the `helm lint --strict` invocation currently is:

```yaml
          helm lint --strict deploy/helm/ \
            --set nexorious.dbEncryptionKey=x \
            --set nexorious.postgresql.password=x \
            --set nexorious.igdbClientId=x \
            --set nexorious.igdbClientSecret=x
```

Change it to also enable the alert objects so lint renders and schema-validates them:

```yaml
          helm lint --strict deploy/helm/ \
            --set nexorious.dbEncryptionKey=x \
            --set nexorious.postgresql.password=x \
            --set nexorious.igdbClientId=x \
            --set nexorious.igdbClientSecret=x \
            --set alerts.enabled=true \
            --set alerts.loki.enabled=true \
            --set alerts.victoriaLogs.enabled=true
```

- [ ] **Step 4: Run `helm lint --strict` locally exactly as CI will**

Run:
```bash
helm dependency build deploy/helm/ 2>/dev/null || true
helm lint --strict deploy/helm/ \
  --set nexorious.dbEncryptionKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set alerts.enabled=true \
  --set alerts.loki.enabled=true \
  --set alerts.victoriaLogs.enabled=true
```
Expected: `1 chart(s) linted, 0 chart(s) failed`. (The symlink-followed stderr line is fine.) If it reports a values-schema violation, fix the `alerts` schema block to match the `values.yaml` defaults from Task 3.

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/values.schema.json .github/workflows/test.yaml
git commit -m "ci: register alerts schema and lint the alert templates"
```

---

## Task 7: Operator-facing docs (`docs/observability.md`) + cross-links

**Files:**
- Create: `docs/observability.md`
- Modify: `docs/maintenance.md`
- Modify: `deploy/helm/README.md`

- [ ] **Step 1: Write `docs/observability.md`**

Create `docs/observability.md`:

```markdown
# Observability: log-based alerting

Nexorious emits structured JSON logs (see [logging-conventions.md](logging-conventions.md))
with stable `level`, `category`, `source`, and `msg` fields. This page covers the
ready-made **log-based alert rules** for the two supported log backends and how to
deliver them — raw for non-Helm users, or via the chart's opt-in templates.

> Metrics-based alerts (OTel metrics) are tracked separately and compose with the
> same `alerts.*` Helm namespace; this page is logs only.

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
    ruleLabels: { project: nexorious }          # match your VMAlert ruleSelector
```

- **Loki** → a `ConfigMap` carrying `alerts.loki.ruleLabel` so the ruler sidecar
  (e.g. `kiwigrid/k8s-sidecar`) discovers it. The bundled grafana/loki v3 chart's
  built-in sidecar uses `{loki.grafana.com/rule: "true"}` instead — set
  `ruleLabel` to match your sidecar's `label`/`labelValue`.
- **VictoriaLogs** → a `VMRule` (VictoriaMetrics Operator). The target `VMAlert`
  must point its datasource at VictoriaLogs and select this `VMRule` (via
  `ruleSelector` matching `alerts.victoriaLogs.ruleLabels`, or `selectAllByDefault`).

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
```

- [ ] **Step 2: Cross-link from `docs/maintenance.md`**

Open `docs/maintenance.md`, and near its top (after the intro paragraph / first heading) add a one-line pointer:

```markdown
> For log-based alerting on these maintenance and sync failures, see
> [observability.md](observability.md).
```

(Place it where it reads naturally — e.g. immediately under the top-level title or intro. Verify the surrounding markdown still renders by reading the edited region.)

- [ ] **Step 3: Document `alerts.*` in `deploy/helm/README.md`**

Open `deploy/helm/README.md`. Find where values are documented (a values table or
a "Configuration" / "Values" section). Add an `alerts` row/subsection consistent
with the existing format, for example:

```markdown
### Alerting (opt-in)

| Key | Default | Description |
|---|---|---|
| `alerts.enabled` | `false` | Master switch for log-based alert rules. |
| `alerts.loki.enabled` | `false` | Render the Loki ruler ConfigMap. |
| `alerts.loki.ruleLabel.key` / `.value` | `loki_rule` / `"1"` | Discovery label the ruler sidecar watches. |
| `alerts.victoriaLogs.enabled` | `false` | Render the VMRule (VictoriaMetrics Operator). |
| `alerts.victoriaLogs.ruleLabels` | `{}` | Extra labels so the VMAlert ruleSelector matches the VMRule. |

See [docs/observability.md](../../docs/observability.md) for full setup.
```

(If `README.md` has no values table and only prose, add a short "Alerting (opt-in)"
prose paragraph linking to `docs/observability.md` instead — match the file's style.)

- [ ] **Step 4: Verify the new doc is valid markdown / links resolve relatively**

Run:
```bash
test -f docs/observability.md && echo "doc exists"
grep -n "observability.md" docs/maintenance.md
grep -n "alerts.enabled" deploy/helm/README.md
```
Expected: `doc exists`; the maintenance cross-link matches; the README mentions `alerts.enabled`.

- [ ] **Step 5: Commit**

```bash
git add docs/observability.md docs/maintenance.md deploy/helm/README.md
git commit -m "docs: add observability/alerting guide and cross-links"
```

---

## Task 8: Final verification before the PR

- [ ] **Step 1: Full `helm lint --strict` (as CI runs it)**

Run:
```bash
helm dependency build deploy/helm/ 2>/dev/null || true
helm lint --strict deploy/helm/ \
  --set nexorious.dbEncryptionKey=x \
  --set nexorious.postgresql.password=x \
  --set nexorious.igdbClientId=x \
  --set nexorious.igdbClientSecret=x \
  --set alerts.enabled=true \
  --set alerts.loki.enabled=true \
  --set alerts.victoriaLogs.enabled=true
```
Expected: `1 chart(s) linted, 0 chart(s) failed`.

- [ ] **Step 2: Confirm a default install renders NO alert objects**

Run:
```bash
helm template t deploy/helm/ \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x 2>/dev/null \
  | grep -E "kind: (ConfigMap|VMRule)" | grep -iE "alert|rule" || echo "no alert objects (correct)"
```
Expected: `no alert objects (correct)`.

- [ ] **Step 3: Confirm both objects render and parse when enabled**

Run:
```bash
helm template t deploy/helm/ \
  --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x \
  --set nexorious.dbEncryptionKey=x --set nexorious.postgresql.password=x \
  --set alerts.enabled=true --set alerts.loki.enabled=true \
  --set alerts.victoriaLogs.enabled=true 2>/dev/null \
  | python3 -c 'import yaml,sys; docs=[d for d in yaml.safe_load_all(sys.stdin) if d]; kinds=[d["kind"] for d in docs]; assert "ConfigMap" in kinds and "VMRule" in kinds, kinds; print("rendered kinds OK:", sorted(set(kinds)))'
```
Expected: a list including `ConfigMap` and `VMRule`.

- [ ] **Step 4: Confirm symlinks are committed as symlinks**

Run:
```bash
git ls-files -s deploy/helm/files | awk '{print $1, $4}'
```
Expected: both files listed with mode `120000`.

- [ ] **Step 5: Acceptance-criteria self-check (map each box in #908)**

  - [ ] LogQL rules covering all four categories, with labels/annotations → `loki-rules.yaml` (Task 1).
  - [ ] LogsQL rules covering the same four, with labels/annotations → `victorialogs-rules.yaml` (Task 1).
  - [ ] `deploy/observability/` raw YAML for both backends → Task 1.
  - [ ] Opt-in chart templates (`VMRule` + labeled `ConfigMap`), default off → Tasks 4, 5 (gated by Task 3 values).
  - [ ] `values.schema.json` + `test.yaml` lint step updated; `helm lint --strict` passes → Task 6.
  - [ ] Example Alertmanager route documented → `alertmanager-route.example.yaml` + `docs/observability.md` (Tasks 1, 7).

- [ ] **Step 6: Push and open the PR**

```bash
git push -u origin feat/log-alert-rules-908
gh pr create --title "feat: add Loki + VictoriaLogs alert rules with opt-in Helm delivery" --body "$(cat <<'EOF'
Closes #908

Adds log-based alert rules for both supported log backends, built on #907's
structured `category`/`source`/`level` fields:

- **Source of truth** under `deploy/observability/`: `loki-rules.yaml` (LogQL),
  `victorialogs-rules.yaml` (LogsQL, `type: vlogs`), and an example Alertmanager
  route. Directly usable by non-Helm operators.
- **Opt-in Helm delivery** (default off): a labeled Loki ruler `ConfigMap` and a
  `VMRule` (VictoriaMetrics Operator), gated behind `alerts.enabled` +
  `alerts.loki.enabled` / `alerts.victoriaLogs.enabled`. The chart reuses the
  canonical files verbatim via in-chart symlinks (`.Files` is chart-scoped and
  can't read the sibling dir).
- Eight alerts across the four categories (DB/startup, credentials, sync/job,
  generic), two-tier severity (critical/warning), conservative documented
  thresholds.
- `values.schema.json` registers the `alerts` block (`additionalProperties:
  false`); the `helm lint --strict` CI step now renders the alert templates.
- `docs/observability.md` documents setup for both backends and the example
  Alertmanager route; cross-linked from `docs/maintenance.md`.

Composes with the metrics-based alerts (#913) on the same `alerts.*` namespace.
EOF
)"
```

---

## Self-review notes

- **Spec coverage:** all six acceptance-criteria boxes map to tasks (Step 5 above).
- **Deviation from the issue's literal mechanism:** the issue says the chart wraps
  the source-of-truth via `.Files.Get` from `deploy/observability/`. Helm's
  `.Files` is chart-scoped, so that is infeasible as written; the plan uses
  in-chart symlinks (verified) to keep `deploy/observability/` canonical while the
  templates `.Files.Get "files/<name>.yaml"`. `tpl` is intentionally not used —
  the rules stay static so they remain valid for non-Helm users.
- **Type consistency:** values keys (`alerts.enabled`, `alerts.loki.enabled`,
  `alerts.loki.ruleLabel.{key,value}`, `alerts.victoriaLogs.enabled`,
  `alerts.victoriaLogs.ruleLabels`) are identical across `values.yaml`,
  `values.schema.json`, both templates, the CI `--set` flags, and the docs.
- **Out-of-scope finding to surface (do not fix here):** the Loki stream selector
  `{app="nexorious"}` and the VictoriaLogs app-scoping are operator-environment
  dependent; the rules ship a documented default. No code change needed.
```

