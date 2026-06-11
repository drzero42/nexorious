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
