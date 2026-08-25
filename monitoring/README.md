# Monitoring Pulumi Stack

Pulumi Go stack that deploys the logging and monitoring stack into Kubernetes.
This stack owns Loki (log aggregation), Grafana (visualization), Alloy (log collection),
and optionally Prometheus (metrics), backed by RustFS as the object storage backend for
long-term log retention.

## Scope

| Component  | Purpose                               | Namespace    |
|------------|---------------------------------------|--------------|
| Loki       | Log aggregation and querying          | `monitoring` |
| Grafana    | Dashboards and log/metric visualization | `monitoring` |
| Alloy      | Kubernetes pod log collection (DaemonSet) | `monitoring` |
| Prometheus | Metrics collection and querying (opt-in, annotation-based scraping) | `monitoring` |

RustFS is the S3-compatible object storage backend for Loki chunk and index storage.
It is deployed and managed by `app_shaide` — this stack references it but does not own it.

## Persistent Volume Requirements

| Component | Needs PV? | Size | Notes                                   |
|-----------|-----------|------|-----------------------------------------|
| Loki      | YES       | 10Gi | WAL + compactor working dir             |
| Grafana   | No        | —    | Stateless; dashboards as ConfigMaps     |
| Alloy     | No        | —    | Stateless DaemonSet, no local buffering |
| Prometheus | YES      | 10Gi | TSDB data directory                     |

Only Loki claims a PVC (`/var/loki`, `ReadWriteOnce`). On on-prem clusters set
`monitoring:lokiStorageClass: hostpath` in the stack YAML; on GCP use `standard` or
`premium-rwo`. Omit the key to fall back to the cluster's default StorageClass.

## Directory Structure

The Pulumi project itself (`monitoring/`) is a thin entrypoint; the deployment logic lives
in the shared package `pkg/iac/monitoring/`, which the installer also imports directly (via
`monitoring.DeployMonitoring`) so it can run as an inline Automation API program without
going through this Pulumi project.

```
monitoring/
├── charts/                         # Vendored Helm chart archives (offline deployments)
│   ├── loki-14.1.0.tgz
│   ├── grafana-12.3.2.tgz
│   ├── alloy-1.8.1.tgz
│   └── prometheus-29.21.0.tgz
├── deployments/                    # Pulumi stack definitions (one file per deployment target)
│   ├── Pulumi.yaml
│   └── Pulumi.kalmannemeth.yaml
├── main.go                         # Stack entrypoint — calls monitoring.DeployMonitoring
└── README.md

pkg/iac/monitoring/
├── monitoring.go                   # DeployMonitoring — K8s provider, namespace, component orchestration
└── internal/
    ├── components/
    │   ├── loki/
    │   │   └── deploy.go           # Loki Helm release + S3 config + bucket-creation Job
    │   ├── grafana/
    │   │   └── deploy.go           # Grafana Helm release + Loki datasource wiring
    │   ├── alloy/
    │   │   └── deploy.go           # Alloy Helm release + River config for pod log collection
    │   ├── dashboards/
    │   │   ├── deploy.go           # Registers each dashboard as a Grafana ConfigMap
    │   │   ├── platform.go         # AI Platform · Overview
    │   │   ├── errors.go           # AI Platform · Error Explorer
    │   │   ├── app_shaide.go       # app-shaide · Log Explorer
    │   │   ├── app_serving.go      # app-serving · Model Log Explorer
    │   │   ├── cluster_nodes.go    # Cluster · Node Metrics (if "prometheus" enabled)
    │   │   └── cluster_pods.go     # Cluster · Pod Resource Usage (if "prometheus" enabled)
    │   └── prometheus/
    │       └── deploy.go           # Prometheus Helm release (server + kube-state-metrics + node-exporter)
    └── config/
        └── config.go               # Config struct loaded from Pulumi stack yaml
```

## Deployment Flow

```
monitoring/main.go
  └── monitoring.DeployMonitoring(ctx, projectDir)   (pkg/iac/monitoring/monitoring.go)
        ├── Load config (internal/config/config.go)
        ├── Create Kubernetes provider
        ├── Create namespace (monitoring)
        ├── Deploy Loki       — if "loki"       in components (internal/components/loki/deploy.go)
        ├── Deploy Grafana    — if "grafana"    in components (internal/components/grafana/deploy.go)
        ├── Deploy Alloy      — if "alloy"      in components (internal/components/alloy/deploy.go)
        ├── Deploy Prometheus — if "prometheus" in components (internal/components/prometheus/deploy.go)
        └── Deploy Dashboards — if "dashboards" in components (internal/components/dashboards/deploy.go)
```

Each component is enabled by listing it in the `components` key of the stack YAML.
Omitting a component from the list skips its deployment without removing existing resources.
Note that `dashboards` is independent of `grafana` — listing `grafana` alone does not deploy
the dashboards; both must be listed to get dashboards loaded into Grafana.

## Components

### Loki

**Chart:** `grafana-community/loki` — [Helm chart source](https://github.com/grafana/loki/tree/main/production/helm/loki)

**Chart version:** 14.1.0 — **App version:** 3.7.2

Deployed in **Monolithic** mode (single binary, single replica). Uses RustFS as its
S3-compatible chunk and ruler storage backend.

**Deployment mode selection:**

| Mode | Scale ceiling | Replicas | Complexity | Status |
|---|---|---|---|---|
| **Monolithic** | ~20 GB/day | 1 (or 2 for HA) | minimal | **used here** |
| Simple Scalable | ~1 TB/day | 3+ targets + Nginx proxy | moderate | deprecated before Loki 4 |
| Microservices | unlimited | 13+ components | high | for massive multi-tenant |

Monolithic is the right choice for this stack: log volume from an internal company cluster
is well under 20 GB/day, a single RustFS backend requires no independent read/write scaling,
and Simple Scalable is being deprecated in Loki 4 — adding its complexity buys nothing here.
If volume grows past ~20 GB/day the migration path is directly to Microservices mode.

See [Loki deployment modes](https://grafana.com/docs/loki/latest/get-started/deployment-modes/) for details.

**Bucket creation:** A Kubernetes Job (`loki-create-bucket`) runs before the Loki Helm
release and creates the `loki-chunks` bucket in RustFS using `aws-cli`'s S3 API directly
(`head-bucket || create-bucket`), and retries up to 5 times if RustFS is not yet ready.
Pulumi awaits Job completion before deploying Loki.

An earlier version used `mc mb --ignore-existing`, which is documented as idempotent but
doesn't reliably no-op against RustFS's specific S3 API responses — the Job kept failing
with `BackoffLimitExceeded` even when the bucket already existed. `head-bucket` /
`create-bucket` are plain S3 API operations any S3-compatible backend implements the same
way, so this is portable across backends rather than depending on mc-specific behavior.

```
namespace → loki-create-bucket Job (completes) → loki Helm release
```

**Configuration:**

All Loki settings are set via Helm values in `pkg/iac/monitoring/internal/components/loki/deploy.go`.

**Retention**

Logs are retained for **14 days** (336 h) and deleted automatically by the compactor.
Without retention, logs accumulate in RustFS indefinitely. On on-prem minimal installations
RustFS uses hostPath storage strictly bound to a single node — a full disk brings down the
entire node, taking Shaide and everything running on it offline.

```
compactor.working_directory       /var/loki/compactor   local temp dir for compaction cycles
compactor.delete_request_store    s3                    delete requests survive pod restarts
compactor.retention_enabled       true                  activates the compactor delete loop
compactor.retention_delete_delay  2h                    grace period before chunks are removed
limits_config.retention_period    336h                  14 days, applied to all streams
```

**Ingestion limits**

Rate limits protect the node from log storms. A single misbehaving model or a runaway
debug-log loop can fill the disk far faster than the 14-day retention cycle can help.

```
limits_config.ingestion_rate_mb           16     MB/s sustained ingestion cap (all streams)
limits_config.ingestion_burst_size_mb     32     MB burst allowance
limits_config.per_stream_rate_limit       3MB    per log-stream sustained cap
limits_config.per_stream_rate_limit_burst 10MB   per log-stream burst allowance
```

**Query limits**

Without query limits, a single unbounded LogQL query across 14 days of logs can stall the
monolithic Loki process and block all other users.

```
limits_config.max_entries_limit_per_query  10000   max log lines returned per query
limits_config.max_query_length             336h    max time range (capped to retention period)
limits_config.max_query_parallelism        4       concurrent sub-queries on this single node
```

**Chunk compression**

Loki defaults to `snappy`. `zstd` reduces chunk size by ~30–40 % at the cost of slightly
more CPU — a worthwhile trade on a storage-constrained on-prem node.

```
chunk_store_config.chunk_encoding  zstd
```

**Persistence**

A 10 Gi PVC is claimed for `/var/loki` on the `singleBinary` pod. This covers the
compactor working directory (`/var/loki/compactor`) and the ingester WAL
(`/var/loki/wal`), ensuring both survive pod restarts. Uses the cluster's default
StorageClass — adjust `singleBinary.persistence.storageClass` in `deploy.go` if your
on-prem setup requires a specific class (e.g. `local-path`).

**Analytics**

`analytics.reporting_enabled: false` — disables the default Grafana Labs usage reporting.
Required for private on-prem installations.

**Helm pull:**

```bash
helm repo add grafana-community https://grafana-community.github.io/helm-charts
helm repo update

cd monitoring/charts/
helm pull grafana-community/loki --version 14.1.0
```

**Verify:**

```bash
$ helm search repo grafana-community/loki
NAME                      CHART VERSION    APP VERSION    DESCRIPTION
grafana-community/loki    14.1.0           3.7.2          Helm chart for Grafana Loki supporting monolith...
```

---

### Grafana

**Chart:** `grafana-community/grafana` — [Helm chart source](https://github.com/grafana/helm-charts/tree/main/charts/grafana)

**Chart version:** 12.3.2 — **App version:** 13.0.1-security-01

Loki is pre-configured as the default datasource via Helm values. No manual datasource
setup is required after deployment. The Loki datasource URL is wired to
`http://loki.monitoring.svc.cluster.local:3100`.

**Helm pull:**

```bash
helm repo add grafana-community https://grafana-community.github.io/helm-charts
helm repo update

cd monitoring/charts/
helm pull grafana-community/grafana --version 12.3.2
```

**Verify:**

```bash
$ helm search repo grafana-community/grafana
NAME                             CHART VERSION    APP VERSION           DESCRIPTION
grafana-community/grafana        12.3.2           13.0.1-security-01    The leading tool for querying and visualizing t...
```

---

### Alloy

**Chart:** `grafana/alloy` — [Helm chart source](https://github.com/grafana/alloy/tree/main/operations/helm/charts/alloy)

**Chart version:** 1.8.1 — **App version:** v1.16.1

Deployed as a **DaemonSet** — one pod per node. Collects logs from all Kubernetes pods
in the cluster via `loki.source.kubernetes` (Kubernetes API, no hostPath mount required).
Attaches `namespace`, `pod`, `container`, and `job` labels to every log stream.
RBAC (`ClusterRole` + `ClusterRoleBinding`) is created automatically by the chart.

**River config (inline in `alloy/deploy.go`):**

```alloy
discovery.kubernetes "pods" { role = "pod" }

discovery.relabel "pod_logs" {
  targets = discovery.kubernetes.pods.targets
  # promotes namespace / pod / container / job and AI Platform labels
  # drops any pod that does not carry axem.ai/platform=ai-platform
}

loki.source.kubernetes "pod_logs" {
  targets    = discovery.relabel.pod_logs.output
  forward_to = [loki.process.drop_noisy.receiver]
}

loki.process "drop_noisy" {
  forward_to = [loki.write.default.receiver]
  # drops Kubernetes probe lines (/healthz, /readyz, /metrics, ...)
  # drops empty and whitespace-only lines
}

loki.write "default" {
  endpoint { url = "http://loki.monitoring.svc.cluster.local:3100/loki/api/v1/push" }
}
```

**Namespace filtering**

Alloy runs as a DaemonSet with cluster-wide pod discovery, so without filtering it would
collect logs from every namespace — `kube-system`, `cert-manager`, `ingress-nginx`, the
monitoring stack itself. A `keep` rule in `discovery.relabel` drops any pod that does not
carry the `axem.ai/platform=ai-platform` label, scoping ingestion to AI Platform workloads
only. This reduces Loki storage consumption and eliminates noise in dashboards.

**Noisy log filtering**

A `loki.process "drop_noisy"` stage sits between the Kubernetes log source and the Loki
writer. It drops two categories before any bytes reach Loki:

- **Health probe lines** — Kubernetes liveness/readiness/startup probes and metrics scrape
  requests (`GET /healthz`, `GET /readyz`, `GET /metrics`, etc.) produce high-frequency
  repetitive lines with no diagnostic value.
- **Empty lines** — whitespace-only lines that some runtimes emit as log separators.

**Helm pull:**

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

cd monitoring/charts/
helm pull grafana/alloy --version 1.8.1
```

**Verify:**

```bash
$ helm search repo grafana/alloy
NAME           CHART VERSION    APP VERSION    DESCRIPTION
grafana/alloy  1.8.1            v1.16.1        Grafana Alloy
```

---

### Prometheus

**Chart:** `prometheus-community/prometheus` — [Helm chart source](https://github.com/prometheus-community/helm-charts/tree/main/charts/prometheus)

**Chart version:** 29.21.0 — **App version:** v3.13.2

Opt-in via the `prometheus` entry in `components` (not enabled by default in every stack).
Deployed as a single Prometheus server (Deployment, one replica) plus the chart's bundled
`kube-state-metrics` and `prometheus-node-exporter` subcharts, which give the server
something meaningful to scrape out of the box (Kubernetes object state + node/cgroup
metrics). `alertmanager` and `prometheus-pushgateway` are disabled — no alerting rules are
defined yet and nothing in this platform pushes ad-hoc metrics.

**Target discovery:** the chart ships several default scrape jobs (`server.scrapeConfigs` in
the chart's `values.yaml`), split between always-on cluster-wide jobs and opt-in
annotation-based jobs:

| Job | Scope | What it collects |
|---|---|---|
| `kubernetes-nodes` | always-on, cluster-wide | kubelet's own internal metrics |
| `kubernetes-nodes-cadvisor` | always-on, cluster-wide | **actual per-container/per-pod resource usage** (CPU, memory working set, network I/O) via the kubelet's built-in cAdvisor endpoint — no annotation needed |
| `kubernetes-api-servers` | always-on, cluster-wide | API server metrics |
| `kubernetes-pods` / `kubernetes-pods-slow` | opt-in | pods carrying a `prometheus.io/scrape: "true"` annotation (with optional `prometheus.io/port` and `prometheus.io/path` overrides) — for custom application metrics |
| `kubernetes-service-endpoints` / `-slow` | opt-in | Services carrying the same annotation |

`prometheus-node-exporter` (real host-level OS metrics: CPU, memory, disk, network) and
`kube-state-metrics` (Kubernetes *object state* for pods — phase, restarts, declared
resource requests/limits, not usage) are scraped as ordinary annotated Services created by
their own subcharts — no manual wiring needed.

Net effect: node hardware metrics, actual pod resource usage, and Kubernetes object state
are all collected automatically with zero per-workload configuration. Annotation-based
opt-in (`prometheus.io/scrape`) is only needed for custom application-level metrics beyond
generic resource usage — e.g. if `shaide-server` or a vLLM pod exposes its own `/metrics`
endpoint. This is a different model from Alloy's log collection, which harvests every pod
by default and relies on a relabel `keep` rule to scope down to AI Platform workloads.

**Persistence:** a 10 Gi PVC backs the TSDB data directory (`server.persistentVolume`),
mirroring Loki's PVC-per-component pattern. Uses the cluster's default StorageClass unless
`monitoring:prometheusStorageClass` is set.

**Retention:** `server.retention: 15d` — set explicitly in `deploy.go` rather than relying on
the chart default, for the same reason as Loki's retention setting: don't let TSDB data grow
unbounded on a size-constrained PVC.

**Helm pull:**

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

cd monitoring/charts/
helm pull prometheus-community/prometheus --version 29.21.0
```

**Verify:**

```bash
$ helm search repo prometheus-community/prometheus
NAME                                CHART VERSION    APP VERSION    DESCRIPTION
prometheus-community/prometheus     29.21.0          v3.13.2        Prometheus is a monitoring system and time seri...
```

**Grafana wiring:** when `prometheus` is enabled, `grafana/deploy.go` adds a `Prometheus`
datasource pointing at `http://prometheus-server.monitoring.svc.cluster.local` (default
Helm service, port 80) alongside the existing Loki datasource. No manual datasource setup
is needed.

---

### Dashboards

Grafana dashboards are defined as JSON constants in `pkg/iac/monitoring/internal/components/dashboards/` and
deployed as Kubernetes ConfigMaps with the `grafana_dashboard: "1"` label. The Grafana
sidecar watches for ConfigMaps with this label and loads them automatically — no manual
import is needed.

**Log dashboards** (always deployed when `dashboards` is enabled — use the Loki datasource):

| File | Dashboard | Purpose |
|---|---|---|
| `platform.go` | AI Platform · Overview | Platform-wide log rate split by subsystem; error rate overview |
| `errors.go` | AI Platform · Error Explorer | Cross-platform error stream filtered by namespace, component, and model |
| `app_shaide.go` | app-shaide · Log Explorer | app-shaide log rate and stream filtered by component |
| `app_serving.go` | app-serving · Model Log Explorer | Model log rate and stream filtered by category, model, and nodegroup |

These are scoped to the `platform="ai-platform"` stream label — only logs from AI Platform
workloads appear, consistent with the Alloy namespace filter.

**Metrics dashboards** (only deployed when `prometheus` is also enabled — use the Prometheus
datasource; skipped otherwise since Grafana has no Prometheus datasource to query):

| File | Dashboard | Purpose |
|---|---|---|
| `cluster_nodes.go` | Cluster · Node Metrics | Per-node CPU, memory, disk, network, load average (`node-exporter`), and GPU utilization/memory (`dcgm-exporter`, graceful no-data on non-GPU nodes) |
| `cluster_pods.go` | Cluster · Pod Resource Usage | Per-pod CPU/memory usage (cAdvisor), restart counts and not-ready count (`kube-state-metrics`), filtered by namespace/pod |

Unlike the log dashboards, these are cluster-wide by default (not scoped to
`platform="ai-platform"`) — node metrics have no per-workload label to filter by, and pod
metrics are useful across all namespaces for general cluster visibility. Use the `namespace`
and `pod` template variables in Cluster · Pod Resource Usage to narrow the view.

---

## Log Label Mapping

Alloy uses an allow-list approach: only explicitly mapped labels reach Loki as stream
labels. All `__meta_kubernetes_*` discovery labels are internal and dropped automatically
unless a `target_label` rule promotes them.

Rules that use `regex = "(.+)"` are **conditional** — the label is only set when the pod
actually carries it. Pods without `axem.ai/model-slug` (e.g. app_shaide components) produce
no `model_slug` label and no empty-value noise in Loki.

### Promoted Labels

| Loki label | Source | Set by | Example value |
|------------|--------|--------|---------------|
| `namespace` | `__meta_kubernetes_namespace` | Kubernetes | `"app-shaide"` |
| `pod` | `__meta_kubernetes_pod_name` | Kubernetes | `"shaide-server-0"` |
| `container` | `__meta_kubernetes_pod_container_name` | Kubernetes | `"shaide-server"` |
| `job` | `namespace/pod_name` | Alloy (composite) | `"app-shaide/shaide-server-0"` |
| `app` | `app.kubernetes.io/name` | app_shaide | `"shaide-server"` |
| `component` | `app.kubernetes.io/component` | app_shaide | `"server"`, `"console"` |
| `part_of` | `app.kubernetes.io/part-of` | app_shaide + app_serving | `"app-shaide"`, `"app-serving"` |
| `managed_by` | `app.kubernetes.io/managed-by` | app_shaide + app_serving | `"pulumi"` |
| `platform` | `axem.ai/platform` | app_shaide + app_serving | `"ai-platform"` |
| `model_slug` | `axem.ai/model-slug` | app_serving | `"gpt-oss-20b"` |
| `model_category` | `axem.ai/model-category` | app_serving | `"generative"`, `"embedder"` |
| `nodegroup` | `axem.ai/nodegroup` | app_serving | `"rtx6000pro-nodepool"` |

`nodegroup` is absent on on-prem pods that use `workload: gpu` as their nodeSelector key
instead of `nodegroup`.

### Intentionally NOT Promoted

| Pod label / metadata | Reason |
|----------------------|--------|
| `llm-d.ai/model` | High cardinality (per-release name), not needed for log filtering |
| `llm-d.ai/role` | decode/prefill distinction rarely needed in Loki |
| `llm-d.ai/inferenceServing` | Always `"true"` — no filter value |
| Pod UID, IP, node IP | High cardinality, not meaningfully filterable |
| Request IDs, user IDs, URL paths | From log message content — never promoted to stream labels |

### Example LogQL Queries

```logql
{app="shaide-server"}                                   # all shaide-server logs
{component="console"}                                   # control-panel logs
{part_of="app-serving"}                                 # all model-serving logs
{model_slug="gpt-oss-20b"}                             # specific model
{model_category="generative"}                           # all generative models
{nodegroup="rtx6000pro-nodepool"}                       # GPU nodegroup logs
{model_category="embedder"} |~ "(?i)error"             # embedder errors
{namespace="app-shaide", component="server"}            # app-shaide server component
```

---

## Object Storage Backend (RustFS)

Loki uses RustFS as its S3-compatible chunk and ruler storage backend.
RustFS is deployed by `app_shaide` in the `app-shaide` namespace and exposed at
`http://rustfs.app-shaide.svc.cluster.local:9000` (cluster-internal).

Loki is configured to point at RustFS via S3-compatible settings in the stack config:

```yaml
monitoring:s3Endpoint: http://rustfs.app-shaide.svc.cluster.local:9000
monitoring:s3BucketLoki: loki-chunks
monitoring:s3User: rustfsuser
monitoring:s3Password:
  secure: <encrypted>
```

The `loki-chunks` bucket is created automatically by the `loki-create-bucket` Job on
`pulumi up`. No manual bucket creation is needed.

For on-prem air-gapped clusters, the `amazon/aws-cli` image used by the Job must be
mirrored into Harbor and the `s3ClientImage` stack key set accordingly:

```yaml
monitoring:s3ClientImage: harbor.<host>/images/amazon/aws-cli:latest
```

**These stack `s3User`/`s3Password` values must match the actual RustFS credentials**
that `app_shaide` owns — this stack only *consumes* RustFS's S3 credentials, it doesn't
create or manage them. A mismatch here causes the `loki-create-bucket` Job to fail
authentication (distinct from the idempotency issue above — even a correctly-idempotent
Job still fails if the credentials themselves are wrong). To get the correct value:

```bash
cd app_shaide/deployments
pulumi stack select <same-stack-name>
pulumi config get --show-secrets s3Password
```

Then set the identical value on the `monitoring` stack — never commit the plaintext value
anywhere, only ever via `pulumi config set --secret`:

```bash
cd monitoring/deployments
pulumi stack select <same-stack-name>
pulumi config set --secret s3Password <value-from-app_shaide>
```

## Enabling Components

Components are opt-in via the `components` list in the stack YAML:

```yaml
monitoring:components:
  - loki
  - grafana
  - alloy
  - dashboards
  # - prometheus   # uncomment to enable
```

## Required Config Reference

Beyond `namespace` and `components`, each enabled Helm-backed component (`loki`,
`grafana`, `alloy`) requires its own chart version to be set explicitly — there is no
default:

```yaml
monitoring:lokiVersion: "14.1.0"
monitoring:grafanaVersion: "12.3.2"
monitoring:alloyVersion: "1.8.1"
```

The chart tarball path defaults to `charts/<component>-<version>.tgz` (e.g.
`charts/loki-14.1.0.tgz`) from this value; override with `lokiChartPath` /
`grafanaChartPath` / `alloyChartPath` if you keep charts elsewhere.

Two more keys are loaded by `config.go` but currently have **no effect** on anything
deployed — set them because `cloudProvider` is required (`pulumi up` fails without it),
but don't expect them to change behavior:
- `cloudProvider` — required; no component branches on it today
- `nodeSelector` — optional; no component applies it to any pod today

Only listed components are deployed. All config keys for enabled components must be present.

## Deploy

### Step 1 — Download Helm Charts (one-time, per machine)

Charts are pulled to a local `monitoring/charts/` directory so `pulumi up` works in
air-gapped/on-prem environments — the chart tarballs are read from disk at deploy time,
not fetched from a Helm repo on every run. Run this once per machine that will run
`pulumi up`; the `.tgz` files are git-ignored (`*.tgz` in `.gitignore`) and must **not**
be committed — download them fresh on each machine instead.

```bash
helm repo add grafana-community https://grafana-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

cd monitoring/charts/
helm pull grafana-community/loki    --version 14.1.0
helm pull grafana-community/grafana --version 12.3.2
helm pull grafana/alloy             --version 1.8.1
helm pull prometheus-community/prometheus --version 29.21.0
```

Expected result:

```
monitoring/charts/
├── loki-14.1.0.tgz
├── grafana-12.3.2.tgz
├── alloy-1.8.1.tgz
└── prometheus-29.21.0.tgz
```

### Step 2 — Set Secrets and Deploy (per cluster)

Prerequisites:
- `kubectl` context points at the target cluster
- `app_shaide` is running and RustFS is healthy in the `app-shaide` namespace

```bash
cd monitoring/deployments
pulumi stack select kalmannemeth

# First-time only — set encrypted secrets
# s3Password MUST exactly match app_shaide's own s3Password for this stack — RustFS is
# owned by app_shaide, not this stack; a mismatch fails loki-create-bucket's auth.
# Get the correct value with: (cd ../../app_shaide/deployments && pulumi stack select
# <same-stack> && pulumi config get --show-secrets s3Password) — see "Object Storage
# Backend (RustFS)" below for details.
pulumi config set --secret s3Password <value>
pulumi config set --secret grafanaAdminPassword <value>

pulumi up
```

### Execution Order

Pulumi resolves the dependency graph automatically:

```
namespace (monitoring)
  └── loki-create-bucket Job       ← creates loki-chunks bucket in RustFS
        └── loki Helm release      ← StatefulSet in monolithic mode
  └── grafana Helm release         ← Deployment, pre-wired Loki datasource
        └── dashboard ConfigMaps   ← picked up automatically by Grafana sidecar
  └── alloy Helm release           ← DaemonSet, streams pod logs to Loki
  └── prometheus Helm release      ← Deployment, if "prometheus" in components
```

Loki will not start until the bucket-creation Job completes successfully.
Grafana, Alloy, and Prometheus deploy in parallel with Loki (all depend only on the namespace).

## Accessing the Stack

### Grafana (primary interface)

Grafana is the main entry point for exploring logs and metrics. Loki is pre-configured as
its default datasource — no manual setup is needed after deployment.

```bash
kubectl port-forward -n monitoring svc/grafana 3000:80
```

**If your stack has the `grafana.ini` embedding block enabled** (see the `grafana.ini`
comments in `grafana/deploy.go` — this is opt-in per stack, for whichever clusters embed
Grafana inside the `app_shaide` control panel), opening `http://localhost:3000` directly
will show a *"Grafana has failed to load its application files"* error. This is expected, not a bug: that config sets `root_url` to a
subpath (`/control-panel/grafana/`, for embedding Grafana inside the `app_shaide` control
panel) with `serve_from_sub_path` deliberately left off, so Grafana only serves its static
assets at the bare, unprefixed path — matching what the control panel's own reverse proxy
strips before forwarding. A raw port-forward has no such proxy in front of it, so the
browser requests assets at the wrong (prefixed) path and gets 404s.

To view the dashboards directly anyway (e.g. local dev/debugging, independent of the
control panel), run a small local reverse proxy that replicates what the control panel's
proxy does — strips the prefix before forwarding to the port-forwarded Grafana:

```bash
cat > /tmp/grafana-proxy.conf <<'EOF'
events {}
http {
    server {
        listen 8080;
        location /control-panel/grafana/ {
            proxy_pass http://127.0.0.1:3000/;
            proxy_set_header Host $host;
        }
    }
}
EOF

docker run --rm --network host \
  -v /tmp/grafana-proxy.conf:/etc/nginx/nginx.conf:ro \
  nginx:alpine
```

Then open **http://localhost:8080/control-panel/grafana/** — not `http://localhost:3000`
directly, which will still show the failed-to-load error since it bypasses the proxy.
(`--network host` is Linux-only; on Docker Desktop for Mac/Windows use
`host.docker.internal` in `proxy_pass` plus an explicit `-p 8080:8080` instead.)

**If the embedding block is not active** for your stack, `http://localhost:3000` works
directly, no proxy needed.

Anonymous **Viewer** access is enabled on stacks with the embedding block active, so
dashboards are viewable without logging in. Admin login is still needed for anything that
changes state (editing dashboards, datasources, etc.):

| Field    | Value                                          |
|----------|------------------------------------------------|
| Username | `admin`                                        |
| Password | `pulumi config get grafanaAdminPassword`       |

To explore logs interactively: **Explore** (compass icon in the left sidebar) → select the
**Loki** datasource → write a LogQL query or use the label browser.

Pre-built dashboards are available under **Dashboards**:

| Dashboard | Purpose |
|---|---|
| AI Platform · Overview | Platform-wide log rate and error overview |
| AI Platform · Error Explorer | Filter errors by namespace, component, and model |
| app-shaide · Log Explorer | app-shaide logs filtered by component |
| app-serving · Model Log Explorer | Model logs filtered by category, model, and nodegroup |
| Cluster · Node Metrics *(if `prometheus` enabled)* | Per-node CPU, memory, disk, network, load, GPU utilization/memory |
| Cluster · Pod Resource Usage *(if `prometheus` enabled)* | Per-pod CPU/memory usage, restarts, not-ready count |

### Loki API (direct access)

Loki has no UI of its own. Port-forward its HTTP port for direct API access — useful for
debugging label cardinality, verifying ingestion, or running queries from the terminal.
Authentication is disabled (`auth_enabled: false`).

```bash
kubectl port-forward -n monitoring svc/loki 3100:3100
```

**Verify Loki is ready:**

```bash
curl http://localhost:3100/ready
```

**List all stream labels:**

```bash
curl http://localhost:3100/loki/api/v1/labels | jq .
```

**List values for a label:**

```bash
curl "http://localhost:3100/loki/api/v1/label/component/values" | jq .
```

**Run a LogQL query:**

```bash
curl -G http://localhost:3100/loki/api/v1/query \
  --data-urlencode 'query={platform="ai-platform"}' \
  --data-urlencode 'limit=10' | jq .
```

## Quick Checks

```bash
# Namespace and workloads
kubectl get pods -n monitoring -o wide

# Loki ready
kubectl rollout status statefulset/loki -n monitoring

# Grafana ready
kubectl rollout status deployment/grafana -n monitoring

# Alloy DaemonSet
kubectl rollout status daemonset/alloy -n monitoring

# Prometheus ready (if enabled)
kubectl rollout status deployment/prometheus-server -n monitoring

# Loki reachability (from inside cluster)
kubectl run loki-check -n monitoring --rm -i --restart=Never --image=curlimages/curl:8.5.0 -- \
  curl -sS http://loki.monitoring.svc.cluster.local:3100/ready

```

## After a Label or Dashboard Update

When label mappings or dashboards change after a `pulumi up`:

- **app_shaide pods** — pod template labels changed → Kubernetes triggers rolling restarts automatically as part of the StatefulSet/Deployment update
- **app_serving pods** — modelservice Helm values changed → Helm triggers pod rollouts automatically
- **Grafana dashboards** — ConfigMaps with `grafana_dashboard: "1"` are watched by the Grafana sidecar and picked up automatically without a restart

**Alloy** requires a manual restart. The Alloy Helm chart may or may not annotate the pod template with a config checksum, so changes to the River config are not guaranteed to trigger a rollout. To be safe:

```bash
kubectl rollout restart daemonset/alloy -n monitoring
kubectl rollout status daemonset/alloy -n monitoring
```

After the restart, new relabel rules are active on all incoming log streams. Existing streams already stored in Loki are not back-filled — new labels appear only on logs ingested after the restart.

## Troubleshooting

**Loki fails to write chunks:**
- Confirm RustFS is reachable: `kubectl exec -n monitoring <loki-pod> -- curl -sI http://rustfs.app-shaide.svc.cluster.local:9000`
- Verify the bucket exists: check `loki-create-bucket` Job logs with `kubectl logs -n monitoring job/loki-create-bucket`
- Check `s3Password` is set: `pulumi config get s3Password`
- If the Job logs show an auth/`403`/`SignatureDoesNotMatch` error rather than a bucket
  issue: `s3Password` (and `s3User`) on this stack must exactly match `app_shaide`'s own
  RustFS credentials for the same cluster — see "Object Storage Backend (RustFS)" above.
  A mismatch here is a distinct failure mode from a stale/misbehaving bucket-creation Job.

**Grafana shows no Loki datasource:**
- Confirm the Loki Service DNS resolves inside the cluster
- Check Grafana datasource config in the Helm values passed by `grafana/deploy.go`

**Alloy not shipping logs:**
- Check DaemonSet pod logs: `kubectl logs -n monitoring daemonset/alloy`
- Confirm Alloy can reach Loki: the River config targets `http://loki.monitoring.svc.cluster.local:3100`

**Expected logs not appearing in Loki:**
- The `drop_noisy` pipeline stage drops any line matching `/healthz`, `/readyz`, `/metrics`,
  `/ready`, or `/livez`, and any empty line. If your application legitimately logs these
  strings (e.g. an access log that includes the probe path), those lines are silently
  discarded. Adjust the `stage.drop` expressions in `alloy/deploy.go` if needed.

**Expected targets not appearing in Prometheus:**
- Node and pod resource-usage metrics (`kubernetes-nodes-cadvisor`, `kubernetes-nodes`) are
  cluster-wide and need no annotation — if they're missing, check kubelet RBAC/connectivity,
  not pod annotations.
- Custom application metrics only appear if the pod (or Service) carries a
  `prometheus.io/scrape: "true"` annotation — see the target-discovery table in the
  [Prometheus](#prometheus) section. A pod without that annotation is invisible to the
  `kubernetes-pods` job, by design.
- Check discovered targets: `kubectl port-forward -n monitoring svc/prometheus-server 9090:80`
  then open **http://localhost:9090/targets**.

**On-prem — charts unavailable:**
- Helm charts must be vendored into `charts/` before running `pulumi up`
- Verify archives: `ls monitoring/charts/`

## Mirror Images (on-prem)

On-prem clusters cannot pull from public registries. Mirror images into Harbor before deploying.

```bash
skopeo copy docker://grafana/loki:3.7.2 \
  oci-archive:infra/on-prem/ansible/artifacts/images/loki-3.7.2.tar

skopeo copy docker://grafana/grafana:13.0.1 \
  oci-archive:infra/on-prem/ansible/artifacts/images/grafana-13.0.1.tar

skopeo copy docker://grafana/alloy:v1.16.1 \
  oci-archive:infra/on-prem/ansible/artifacts/images/alloy-v1.16.1.tar

skopeo copy docker://amazon/aws-cli:latest \
  oci-archive:infra/on-prem/ansible/artifacts/images/aws-cli-latest.tar

# Prometheus (only if the "prometheus" component is enabled)
skopeo copy docker://quay.io/prometheus/prometheus:v3.13.2 \
  oci-archive:infra/on-prem/ansible/artifacts/images/prometheus-v3.13.2.tar

skopeo copy docker://registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.19.1 \
  oci-archive:infra/on-prem/ansible/artifacts/images/kube-state-metrics-v2.19.1.tar

skopeo copy docker://quay.io/prometheus/node-exporter:v1.12.1 \
  oci-archive:infra/on-prem/ansible/artifacts/images/node-exporter-v1.12.1.tar

skopeo copy docker://quay.io/prometheus-operator/prometheus-config-reloader:v0.93.0 \
  oci-archive:infra/on-prem/ansible/artifacts/images/prometheus-config-reloader-v0.93.0.tar
```

Then upload via the Harbor playbook:

```bash
cd infra/on-prem/ansible
ansible-playbook -i inventory-dev harbor_upload.yml
```

## Security Notes

- Sensitive values (`s3Password`, `grafanaAdminPassword`) must be stored as Pulumi secrets:
  `pulumi config set --secret <key> <value>`
- Avoid committing plaintext secrets into stack config files
- Grafana should not be exposed externally without authentication — use an HTTPRoute with
  auth middleware or restrict access to the internal network only

## Resource Ownership

This stack owns all resources in the `monitoring` namespace.
It does not own RustFS — that is managed by `app_shaide`.
Cluster-level routing and Gateways are managed by the infra stacks.
