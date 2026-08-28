---
title: "Observability"
description: "What the platform collects, and how to reach the logs, metrics and dashboards."
weight: 55
---

# Observability

shaide ships a self-contained observability stack. Everything it collects stays in the
cluster — nothing is sent to an external service.

## What runs

| Component | Collects | Opt-in |
| --- | --- | --- |
| **Alloy** | Pod logs, as a DaemonSet on every node | No |
| **Loki** | Stores and indexes those logs | No |
| **Grafana** | Dashboards and queries | No |
| **Prometheus** | Metrics, via annotation-based scraping | **Yes** |

Enable them in the monitoring stack configuration:

```yaml
monitoring:components:
  - loki
  - grafana
  - alloy
  - dashboards
  # - prometheus   # uncomment to collect metrics
```

## What data is collected

### Logs

Container stdout/stderr from platform workloads. Alloy filters by namespace, so only
shaide's own components are collected — not everything running in the cluster.

Each log line is labelled, and those labels are what you query on:

| Label | Meaning | Example |
| --- | --- | --- |
| `namespace` | Kubernetes namespace | `app-shaide` |
| `pod` | Pod name | `shaide-server-0` |
| `container` | Container name | `shaide-server` |
| `app` | Application | `shaide-server` |
| `component` | Component within the app | `server`, `console` |
| `part_of` | Subsystem | `app-shaide`, `app-serving` |
| `platform` | Platform marker | `ai-platform` |
| `model_slug` | Model being served | `gpt-oss-20b` |
| `model_category` | Model type | `generative`, `embedder` |
| `nodegroup` | Node pool the pod runs on | `rtx6000pro-nodepool` |

The last three are what make model-level log filtering possible — you can isolate one
model's logs without knowing its pod names.

> `nodegroup` is absent on on-prem pods that use `workload: gpu` as their node selector
> key instead of `nodegroup`.

### Metrics

Only when Prometheus is enabled. Scraping is annotation-based, and the shipped dashboards
cover:

- **Node metrics** — CPU, memory, disk, network, load average, plus GPU utilisation and
  GPU memory where `dcgm-exporter` is present.
- **Pod metrics** — per-pod CPU and memory, restart counts, not-ready counts.

### What is not collected

Prompts, completions and model inputs are **not** collected by the observability stack. It
captures operational telemetry — log lines and resource metrics — not inference content.

## Retention

Logs are retained for **14 days** by default, then removed by Loki's compactor. Without
retention enabled, logs accumulate indefinitely — which matters most on on-prem
installations with fixed disk.

Loki claims a 10Gi volume for its write-ahead log and compactor working directory; the
logs themselves live in the platform's object storage. Prometheus, when enabled, claims a
further 10Gi for its time-series database.

## Accessing Grafana

Grafana is the entry point for both logs and metrics, with Loki pre-configured as its
default datasource.

```bash
kubectl port-forward -n monitoring svc/grafana 3000:80
```

Then open `http://localhost:3000`.

Where the control panel embeds Grafana, a bare port-forward shows *"Grafana has failed to
load its application files"* — expected, because Grafana is configured to serve from a
subpath that the control panel's proxy normally strips. Reach it through the control panel
instead, or see [Observability](../architecture/observability.md) for a local reverse-proxy
workaround.

## Dashboards

Shipped dashboards load automatically — no manual import.

**Logs** (always available):

| Dashboard | Shows |
| --- | --- |
| AI Platform · Overview | Log rate by subsystem, error-rate overview |
| AI Platform · Error Explorer | Error stream across the platform, filterable by namespace, component and model |
| app-shaide · Log Explorer | app-shaide logs by component |
| app-serving · Model Log Explorer | Model logs by category, model and nodegroup |

**Metrics** (only when Prometheus is enabled):

| Dashboard | Shows |
| --- | --- |
| Cluster · Node Metrics | Per-node CPU, memory, disk, network, GPU |
| Cluster · Pod Resource Usage | Per-pod CPU/memory, restarts, readiness |

Log dashboards are scoped to shaide's own workloads. Metrics dashboards are cluster-wide,
since node metrics have no per-workload label to filter on.

## Querying logs directly

Grafana's **Explore** view queries Loki with LogQL:

```logql
{app="shaide-server"}                          # all shaide-server logs
{component="console"}                          # control panel
{part_of="app-serving"}                        # all model serving
{model_slug="gpt-oss-20b"}                     # one specific model
{model_category="generative"}                  # all generative models
{nodegroup="rtx6000pro-nodepool"}              # everything on one GPU pool
{model_category="embedder"} |~ "(?i)error"     # embedder errors, case-insensitive
{namespace="app-shaide", component="server"}   # narrow to one component
```

Combine label selectors with `|~` for regex matching, or `|=` for a literal substring.

## Common tasks

**Why is a model failing?**

```logql
{model_slug="<model>"} |~ "(?i)error|panic|fatal"
```

**What changed across the platform in the last hour?** Open *AI Platform · Error Explorer*
and set the range to 1h.

**Is a GPU node saturated?** *Cluster · Node Metrics*, filtered to that node — requires
Prometheus.

## Troubleshooting

| Symptom | Cause |
| --- | --- |
| No logs in Grafana | Alloy not enabled, or the pod's namespace is outside the collection filter |
| Logs stop after ~14 days | Working as configured — that is the retention period |
| Metrics dashboards missing | Prometheus not enabled; they are skipped when there is no datasource to query |
| Grafana fails to load its files | Subpath configuration — reach it through the control panel |
| Loki pod pending | Its 10Gi PVC cannot bind — check the StorageClass |

Architecture and full configuration reference:
[Observability](../architecture/observability.md).
