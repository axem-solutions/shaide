---
title: "ADR-001: Centralized logging"
description: "Decision record for the centralized logging architecture."
weight: 10
---

# ADR-001: Centralized logging

**Status:** Review

---

## Context

The AI Platform runs workloads across multiple Kubernetes clusters (GCP GKE, AWS EKS, on-prem RKE2).
Each cluster produces logs from platform components (Shaide Server, Control Panel, RustFS, Qdrant, MCP Servers)
and from customer workloads. Without centralized aggregation, operators must `kubectl logs` into individual pods
to investigate incidents — there is no cross-service correlation, no historical retention, and no unified
interface.

The following requirements drive this decision:

- Log data must be retained beyond the pod lifecycle.
- Logs from all namespaces on a cluster must be collected without modifying application code.
- A query and visualization interface must be available for operational use.
- The solution must be deployable in air-gapped environments where no external internet access is available.
- Object storage for log retention must reuse existing cluster infrastructure — no additional stateful dependencies.
- Credentials must not be embedded in infrastructure manifests.

---

## Decisions

### 1. Log backend: Loki

**Grafana Loki** is selected as the log aggregation and query backend.

Loki indexes only log metadata (labels) rather than full log content, which significantly reduces storage
and indexing overhead compared to Elasticsearch-based stacks. Its label model aligns naturally with
Kubernetes metadata (namespace, pod, container). The Loki query language (LogQL) is purpose-built for
log filtering and pattern analysis.

Loki exposes a Prometheus-compatible push API, which makes it compatible with any OpenTelemetry-aware
or Prometheus-ecosystem collector without vendor lock-in.

### 2. Log collector: Kubernetes DaemonSet collector

A DaemonSet-based log collector is deployed on every cluster node to tail container log files from
`/var/log/pods/` and forward them to Loki. **Grafana Alloy** (the OpenTelemetry-native successor to
Promtail) is the preferred collector. Promtail remains a valid alternative for clusters where Alloy
is not yet available.

No application-side changes are required. The collector attaches Kubernetes metadata labels
(namespace, pod name, container name, node) to each log stream automatically.

### 3. Object storage backend: RustFS

**RustFS** is selected as the object storage backend for Loki chunk and index storage.

RustFS is already deployed and managed by the `app_shaide` Pulumi project in the `app-shaide`
namespace. It exposes an S3-compatible API at `rustfs.app-shaide.svc.cluster.local:9000`.
Reusing RustFS eliminates the need for a separate object storage deployment, avoids additional
persistent volume requirements, and keeps all object storage concerns under a single operational
boundary.

Loki is configured in **single-binary mode** for initial deployment, writing chunks and the TSDB index
to RustFS via the S3-compatible API. The RustFS bucket for Loki (`loki-chunks`) must be created
before the first `pulumi up`.

Credentials for RustFS are provided at deploy time as Pulumi secrets and are never stored in
plaintext in stack config files.

### 4. Visualization: Grafana

**Grafana** is deployed as part of the observability stack and configured with Loki as a built-in
datasource. Grafana provides the query UI (Explore), pre-built Kubernetes log dashboards, and
alerting integration.

Grafana does not replace the existing GPU monitoring dashboards. It complements them by adding
a log-oriented view alongside metric-oriented views.

### 5. Infrastructure ownership: `observability` Pulumi project

All logging infrastructure (Loki, Grafana, collector DaemonSet) is owned and deployed by a new
**`observability` Pulumi project** located at `observability/` in the repository root.

This follows the existing project-per-concern pattern used across the platform
(`app_shaide`, `app_serving`, `infra/gcp`, `infra/on-prem`). Each stack config file
(`deployments/Pulumi.<stack>.yaml`) holds the authoritative configuration for a given
deployment target, including RustFS connection settings, chart versions, and secrets references.

The `observability` project is independently deployable — it has no Pulumi StackReference dependency
on `app_shaide`. The RustFS endpoint and credentials are provided as explicit config values rather
than resolved from a stack output, keeping the dependency relationship operational rather than
infrastructural.

### 6. Existing monitoring stays in `monitoring/`

The existing `monitoring/` directory at the repository root contains GPU monitoring configuration
(NVIDIA DCGM Exporter, kube-prometheus-stack Helm values) managed via plain Helm commands.
It is **not moved under `infra/`** and is **not converted to a Pulumi project**.

Reasons:
- It is deployed manually via `helm install` and has no Pulumi lifecycle.
- Moving it would require migrating operational runbooks with no functional benefit.
- GPU monitoring and log aggregation are independent concerns with different deployment cadences.

The `monitoring/` directory continues to own GPU metrics. The `observability/` project owns log
aggregation and general-purpose dashboards. These are complementary and do not overlap.

---

## Consequences

**Positive:**
- Centralized log retention across all namespaces, surviving pod restarts and rollouts.
- No new object storage backend required — RustFS reuse reduces operational surface.
- Air-gap compatible: charts can be vendored to `observability/charts/`, images mirrored to Harbor.
- Follows existing Pulumi project conventions — operators already familiar with the pattern.
- Loki's label-based model maps directly to Kubernetes metadata without custom pipeline configuration.

**Negative / trade-offs:**
- RustFS bucket for Loki must be provisioned before first deployment — `observability` has
  no mechanism to create buckets in a foreign Pulumi project's resource.
- Single-binary Loki mode has scalability limits. If log volume grows significantly, migration to
  Loki microservices mode (with separate ingester, querier, compactor components) will be required.
  This is a known architectural boundary, not an operational surprise.
- Grafana is a new workload on the cluster. Resource requests and limits must be tuned per cluster
  to avoid contention on smaller node pools.
