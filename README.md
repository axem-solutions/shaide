# axem's AI platform

This project's goal is to provide a platform for running AI workloads on any Kubernetes
cluster, using the [llm-d](https://github.com/llm-d/llm-d) framework. For inference, the
vLLM engine is used. All infrastructure is written as Pulumi Go programs, deployable to
GCP, AWS, Azure, or an on-prem/air-gapped RKE2 cluster.

## Components

### Application layer

- **app_serving**: Per-model serving stack (llm-d-infra Gateway, GAIE, ModelService).
  Convention-based model discovery — drop a `gaie-<slug>`/`ms-<slug>` folder pair under
  `deployments/models/<category>/<model>/` and reference it in stack config. Supports
  both generative and embedder models, with optional Harbor-backed model-weight
  pre-loading via ORAS.
- **app_shaide**: Centralized router and application layer — the Shaide server
  (Rust/Axum, SQLite-backed), the control panel UI, an end-user webapp, RustFS
  (S3-compatible object storage), and Qdrant (vector DB for RAG). Exposed via
  LoadBalancer or, when a shared Gateway is available, ClusterIP + HTTPRoute.
- **app_mcp**: Deploys MCP (Model Context Protocol) server datasources into a shared
  gateway namespace, with per-datasource NetworkPolicies, optional internal-CA trust,
  and the RBAC Shaide needs to watch MCP pod state.

### Infrastructure layer

- **infra/aws**: EKS cluster with GPU node groups and the AWS Load Balancer Controller
  for public access.
- **infra/gcp**: GKE cluster with GPU node pools, Gateway API, and Certificate Manager
  for public HTTPS access.
- **infra/azure**: AKS cluster, built as sequential phases (bootstrap/team access,
  shared services, cluster, RBAC, optional workload identity) with a node-pool catalog
  driving new clusters without new code.
- **infra/on-prem**: Ansible-provisioned, air-gapped RKE2 clusters, plus the Pulumi
  stacks (hostPath StorageClass, Harbor, MetalLB, GPU Operator) that turn a bare RKE2
  cluster into a platform-ready one.
- **infra/gateway-provider**: Cluster-level Gateway API + GAIE CRDs, the Istio control
  plane, and the shared Gateway resource — deployed before `app_serving` on every cloud
  and on-prem.
- **infra/cloud-harbor**: Internal OCI registry (Harbor) used as the model and image
  registry across all deployment targets.
- **infra/model-registry**: Downloads models from Hugging Face and pushes them to
  Harbor as OCI artifacts.
- **infra/local-k8s**: Local Kubernetes cluster for development and testing.

### Platform tooling

- **installer**: Containerized terminal UI that installs or updates the platform from
  an offline bundle (images, manifests, Pulumi stack defaults) via the Pulumi Automation
  API — supports both on-prem and cloud targets.
- **monitoring**: Loki (log aggregation), Grafana (dashboards), and Alloy (log
  collection) stack, backed by RustFS for long-term retention. Prometheus support is
  planned but not yet implemented.
- **pkg**: Shared Go module — houses the actual Pulumi deployment logic for
  `app_serving`, `app_shaide`, `infra/cloud-harbor`, `infra/gateway-provider`, and the
  on-prem services stack, plus small cross-cutting helpers.
- **documentation**: mdBook technical reference — per-cloud architecture blueprints
  (AWS/GCP/Azure/on-prem), operational guides, and architecture decision records.

### Upstream references

- **llm-d**: The core LLM deployment framework, vendored as git submodules under
  `upstream/llm-d/`:
  - `llm-d`: Core framework and reference implementations for LLM deployment and serving
  - `llm-d-benchmark`: Tool for performance benchmarking
  - `llm-d-inference-scheduler`: Scheduler for prefill/decode tasks
  - `llm-d-inference-sim`: Simulation environment without actual model execution, for
    testing and development
  - `llm-d-infra`: Infrastructure components for llm-d
  - `llm-d-kv-cache`: Key-value cache implementation used by the serving stack
  - `llm-d-modelservice`: Helm chart reference and examples used by the serving stack
  - `llm-d-routing-sidecar`: Sidecar for routing requests to the appropriate model service
- **control_panel**: A React-based control panel for monitoring and managing the
  platform, deployed by `app_shaide`.
- **shaide_server**: The central router and API server, deployed by `app_shaide`.

## Supported models

See [`app_serving/README.md`](app_serving/README.md#supported-models) for the current
list of validated generative and embedder models.

## Architecture

### Deployment order (per cluster)

```
1. infra/gcp | infra/aws | infra/azure | infra/on-prem   — cluster provisioning
2. infra/cloud-harbor (or the on-prem services stack)    — internal OCI registry
3. infra/gateway-provider                                — Istio + Gateway API
4. app_serving                                           — per-model LLM serving
5. app_shaide                                            — application layer
6. app_mcp, monitoring                                   — optional
```

### Traffic flow

**Generative models:**
```
Public ingress (cloud LB / Gateway, or on-prem MetalLB)
  → app_shaide (shaide-server)
  → per-model Istio Gateway → GAIE EPP (InferencePool)
  → ModelService decode pod
```

**Embedding models** bypass the Gateway/GAIE path entirely — the inference gateway only
understands chat-completion requests:
```
app_shaide → ms-<slug>-embeddings ClusterIP Service → ModelService decode pod
```
