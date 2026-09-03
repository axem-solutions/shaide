---
title: "Model deployment flow"
description: "How a model definition becomes a running inference service."
weight: 50
---

# Model deployment flow

This document describes how `app_serving` deploys LLM model serving infrastructure
using Pulumi. It covers the end-to-end flow, the role of each component, and the
differences between cloud (GCP) and on-prem air-gapped (RKE2) deployments.

---

## Overview

`app_serving` is a Pulumi Go program that deploys a full LLM inference stack onto a
Kubernetes cluster. It is model-agnostic: the models to deploy are selected via stack
config, and all model-specific settings live in per-model `values.yaml` files.

Multiple models can be deployed onto the same cluster from a single stack config by
listing them under `models.generative` and/or `models.embedder`. Each model gets its
own namespace and isolated set of resources, but shares the cluster-level credentials
(`kubeconfig`, `harborHostname`, `harborUser`, `harborToken`).

The stack deploys the following components **per model**, in order:

```
Namespace + Secrets
      ↓
llm-d-infra   (Istio Gateway + DestinationRule)
      ↓
GAIE          (Gateway API Inference Extension — EPP + InferencePool)
      ↓
ModelService  (vLLM decode/prefill pods + routing sidecar)
      ↓
      ├─ generative models → HTTPRoute        (binds Gateway listener → InferencePool)
      └─ embedder models   → EmbeddingService (ClusterIP Service straight to the decode pod, port 8200)
```

Traffic path at runtime:

```
Generative:
client → Gateway (infra-<slug>-inference-gateway)
       → HTTPRoute (llm-d-<slug>)
       → InferencePool (gaie-<slug>)  ← EPP picks the best pod
       → ModelService decode pod (ms-<slug>)

Embedding:
client → ms-<slug>-embeddings.<namespace>.svc.cluster.local:8200
       → ModelService decode pod directly — no Gateway, no GAIE/EPP
```

---

## Repository Layout

```
app_serving/
├── deployments/
│   ├── Pulumi.yaml                   # Stack descriptor (runtime: go, main: ..)
│   ├── Pulumi.<stack>.yaml           # Per-stack config (one file per cluster target)
│   └── models/
│       └── <category>/                     # generative or embedder
│           └── <ModelName>[-<target>]/      # One folder per model+cluster combination
│               ├── gaie-<slug>/            # slug is shared across variants of the same model
│               │   └── values.yaml         # GAIE (inferencepool) Helm values
│               └── ms-<slug>/
│                   └── values.yaml         # ModelService Helm values
└── main.go                           # Orchestration entry point; calls
                                       # github.com/axem-solutions/ai_platform/pkg/iac/serving
```

The implementation itself lives outside `app_serving/`, in the shared `pkg` module at
`pkg/iac/serving/`:

```
pkg/iac/serving/
├── serving.go                        # DeployAppServing — orchestration entry point
└── internal/
    ├── config/
    │   ├── config.go                 # Config loading and validation; returns a single Config
    │   │                              # (Config.Models holds one Model entry per enabled model)
    │   ├── input.go                  # Raw stack-config shape (stackInput, modelInput, ...)
    │   └── naming.go                 # Release/namespace/gateway name derivation
    ├── components/
    │   ├── llmd-infra/deploy.go      # llm-d-infra Helm chart
    │   ├── gaie/deploy.go            # inferencepool (GAIE) Helm chart
    │   ├── modelservice/deploy.go    # llm-d-modelservice Helm chart
    │   ├── httproute/deploy.go       # HTTPRoute custom resource
    │   └── embeddingservice/deploy.go # ClusterIP Service for embedder models
    └── platform/
        ├── secret.go                 # Harbor pull secret creation
        └── model_storage.go          # PVC + ORAS pull Job creation
```

---

## Step-by-Step Deployment Flow

### Step 1 — Run Pulumi

```bash
cd app_serving/deployments
pulumi up --stack <stack-name>
```

`Pulumi.yaml` declares `main: ..`, so Pulumi compiles the Go program from `app_serving/`
and runs it. The working directory for the binary is `app_serving/`.

---

### Step 2 — Config Loading (`config.go`)

`appConfig.Load(ctx)` reads the stack config (`Pulumi.<stack>.yaml`) and returns
a `Config` struct containing one `Model` entry per enabled model.

1. Reads `cloudProvider` — must be `cloud` or `on-prem`. Determines which credentials
   are required.
2. Reads `models.generative` and `models.embedder` lists. Each entry has:
   - `name` — folder name under `deployments/models/<category>/`
   - `enabled` — **boolean** (`true`/`false`); models with `enabled: false` are skipped
   - `nodeSelector` — per-model node targeting map
   - `nameSpace` (optional) — overrides auto-derived `llm-d-<slug>`
   - `releaseName` (optional) — overrides auto-derived `infra-<slug>`
3. Loads cluster-wide credentials shared by all models:
   - `kubeconfig`, `harborHostname`, `harborUser`, `harborToken`
4. For each enabled model, scans `deployments/models/<category>/<name>/` for exactly one
   `gaie-<slug>/` and one `ms-<slug>/` subdirectory. Both must share the same `<slug>`.
5. Derives per-model resource names from the slug (unless overridden):
   - Namespace: `llm-d-<slug>`
   - Infra release: `infra-<slug>`
   - GAIE release: `gaie-<slug>`
   - ModelService release: `ms-<slug>`
   - Gateway name: `infra-<slug>-inference-gateway`
   - HTTPRoute name: `llm-d-<slug>`
6. Validates credentials per `cloudProvider`:
   - `cloud`: no additional required credentials (Harbor credentials optional, needed when `modelSource` is set).
   - `on-prem`: `kubeconfig`, `harborHostname`, `harborUser`, `harborToken` **required**.

---

### Step 3 — Kubernetes Provider (`main.go`)

```go
kubernetes.NewProvider(ctx, "app-serving-k8s", &kubernetes.ProviderArgs{
    Kubeconfig: ...,  // set only when kubeconfig is in stack config
})
```

A **single provider** is created for the entire stack and shared by all models. This
means all models in a stack target the same cluster.

- **Cloud stacks**: no `kubeconfig` in config → provider reads `KUBECONFIG` env var
  or `~/.kube/config` (GKE cluster).
- **RKE2 stack**: `kubeconfig: ~/.kube/rke2-cluster.yaml` → provider explicitly
  targets the on-prem cluster.

---

### Step 4 — Per-Model Deployment Loop (`main.go`)

For each enabled model in the `models` list, steps 5–11 below are executed
in sequence, each using the model's `Model` entry (with its own `Namespace`,
`ReleaseName`, `GaieValuesPath`, etc.).

---

### Step 5 — Namespace

Creates the `llm-d-<slug>` namespace. All resources for this model are isolated within it.

---

### Step 6 — Model Storage: PVC + ORAS Pull Job (optional)

When a model has a `modelSource` block in the stack config, Pulumi creates:

1. On on-prem with `hostpathNode` set: a static `PersistentVolume` (`<slug>-model-pv`) backed
   by `/var/lib/hostpath/models/<slug>` on the specified node. The directory must exist before
   `pulumi up` (managed by the `hostpath_dirs` Ansible role).
2. A `PersistentVolumeClaim` (`<slug>-model`) — RWO; `hostpath` StorageClass on-prem, cluster
   default on cloud.
3. A `batch/v1 Job` (`<slug>-model-pull`) — runs ORAS once to pull the model artifact
   from Harbor into the PVC. Writes `/model-cache/.pulled` on success for idempotency.
   `backoffLimit: 3`, `ttlSecondsAfterFinished: 86400` (auto-cleaned after 24 h).

The Job uses `harbor-creds` (mounted as `/root/.docker/config.json`) to authenticate
against Harbor. The ORAS image is pulled from `ghcr.io` on cloud or from
`<harborHostname>/images-infra/oras-project/oras:v1.3.1` on air-gapped on-prem.

The ModelService chart (Step 10) depends on both the PVC and the Job, so the inference
pod never starts with an empty PVC.

See [MODEL_STORAGE.md](model-storage.md) for the full directory layout and recovery
procedure.

---

### Step 7 — Harbor Pull Secret

When `harborToken` is set (i.e. `HarborTokenSet` is true), creates a `harbor-creds`
`kubernetes.io/dockerconfigjson` secret in the model's namespace, authenticating
against Harbor with the robot account credentials (`harborUser` / `harborToken`).

This secret serves two purposes:
- **`imagePullSecrets`** — referenced by pods in on-prem `values.yaml` files so cluster
  nodes (no internet) can pull container images from Harbor.
- **ORAS Job authentication** — mounted as `/root/.docker/config.json` in the ORAS pull
  Job so `oras pull` can authenticate against Harbor to fetch model artifacts.

---

### Step 8 — llm-d-infra Helm Chart (`llmd-infra/deploy.go`)

**Chart source**: local git submodule at
`upstream/llm-d/llm-d-infra/charts/llm-d-infra` (initialized on the provisioner
laptop, which has internet access).

Deploys:
- An Istio `Gateway` resource — the cluster entry point for inference traffic for this model.
- A `DestinationRule` for the GAIE EPP service (TLS settings — one-way SIMPLE mode with
  `insecureSkipVerify`, not mutual TLS).

Additionally creates two plain Kubernetes resources:
- An `ExternalName` Service (`llmd-gateway-<slug>`) resolving to the gateway's in-cluster
  FQDN — used by `app_shaide` to reach the inference gateway.
- A `ConfigMap` (`gateway-defaults-<slug>`) that injects a hard `nodeAffinity` (built from the
  stack's `nodeSelector` config) into the Istio Gateway pod via the `istio` GatewayClass
  defaults mechanism.

---

### Step 9 — GAIE Helm Chart (`gaie/deploy.go`)

**Chart source**: Pulumi calls Helm on the provisioner laptop, which downloads the
chart from `oci://registry.k8s.io/gateway-api-inference-extension/charts/inferencepool`
at version `v1.2.0`. The provisioner has internet; the cluster nodes do not need it.

Values are merged from two sources:
1. `deployments/models/<category>/<modelName>/gaie-<slug>/values.yaml` — model-specific
   settings (EPP image, InferencePool label selectors).
2. Programmatic values in `gaie/deploy.go` — DestinationRule connection pool settings,
   Istio provider config.

Deploys:
- The EPP (Endpoint Picker Policy) pod — watches InferencePod labels and routes
  requests to the best available ModelService pod.
- An `InferencePool` custom resource — the traffic target for the HTTPRoute.

**Image pull**:
- Cloud: EPP image pulled from `registry.k8s.io/gateway-api-inference-extension/epp:v1.2.0`
  directly by cluster nodes (internet access available).
- On-prem: EPP image pulled from `harbor.harbor.svc.cluster.local/images-shaide/gateway-api-inference-extension/epp:v1.2.0`
  (Harbor) using the `harbor-creds` pull secret.

---

### Step 10 — ModelService Helm Chart (`modelservice/deploy.go`)

**Chart source**: Pulumi calls Helm on the provisioner laptop, which downloads the
chart from `https://llm-d-incubation.github.io/llm-d-modelservice/` at version
`v0.4.12`.

Values are merged from two sources:
1. `deployments/models/<category>/<modelName>/ms-<slug>/values.yaml` — model-specific
   settings (model URI, container images, GPU resources, inference server args).
2. Programmatic values in `modelservice/deploy.go` — a hard `nodeAffinity` (built from the
   stack's `nodeSelector` config) and, when the stack sets `gpuToleration`, a matching
   toleration applied to both decode and prefill pods. The
   toleration's key/value/effect are read from stack config, not hardcoded — they must match
   whatever taint is actually applied to the GPU node.

Deploys per-model decode and prefill pods. Each pod contains two containers:
- **Inference server container** — serves the model. Supported servers:
  - `modelCommand: vllmServe` — runs vLLM; chart auto-generates `--model`, `--port`, and
    parallelism args. The decode pod listens on port **8200** (routing-proxy takes 8000).
  - `modelCommand: custom` — user provides `command` and `args` directly. The chart
    still mounts the model PVC and injects `HF_HUB_CACHE` (from `pvc+hf://` URI), but
    does not generate any inference args. The inference server **must listen on port 8200**
    for decode pods.
  - `modelCommand: imageDefault` — uses the container image's default entrypoint with
    chart-generated args.

**Inference server for CPU-only embedding models (on-prem)**:
For CPU-only verification of embedding models, `michaelf34/infinity` can be used instead
of vLLM via `modelCommand: custom`. Infinity does not require CUDA libraries at startup
and resolves model weights from the HF cache via `HF_HUB_CACHE` (injected by the chart).
The vLLM CUDA image (`llm-d-cuda`) cannot start on CPU-only nodes even with `--device cpu`.

**Image pull**:
- Cloud: images pulled from `ghcr.io/llm-d/...` directly.
- On-prem: images pulled from `harbor.harbor.svc.cluster.local/images-shaide/llm-d/...` using `harbor-creds`.

**Embeddings endpoint**:
The inference gateway (EPP) validates all requests as chat-completions and rejects
`/v1/embeddings` or `/embeddings` calls with a 400 error. For embedding models, bypass
the gateway and access the inference server directly on port 8200 (pod port-forward or
a dedicated ClusterIP service).

---

### Step 11 — HTTPRoute or Embedding Service

**Generative models** (`httproute/deploy.go`): creates a `gateway.networking.k8s.io/v1
HTTPRoute` custom resource that:
- Binds to the Istio Gateway created in Step 8.
- Routes all traffic (`PathPrefix: /`) to the `InferencePool` created in Step 9.

This completes the traffic chain. Inference requests arriving at the gateway are
forwarded to the EPP, which selects the optimal decode pod based on KV-cache state.

**Embedder models** (`embeddingservice/deploy.go`): creates a `ClusterIP` Service
(`ms-<slug>-embeddings`) that selects the decode pod directly on port 8200, bypassing
the Gateway/GAIE/EPP path entirely — the EPP only validates chat-completion requests
and rejects `/v1/embeddings`. `main.go` picks one branch or the other based on
`model.IsEmbedder`; a model never gets both.

---

## Stack Configurations

See [`app_serving/deployments/Pulumi.TEMPLATE.yaml`](https://github.com/axem-solutions/shaide/blob/main/app_serving/deployments/Pulumi.TEMPLATE.yaml) for the full,
up-to-date config reference with every key documented. The two examples below illustrate
the cloud vs. on-prem shape; copy the template to create a real stack (`cp
deployments/Pulumi.TEMPLATE.yaml deployments/Pulumi.<stack-name>.yaml`).

### Cloud example

```yaml
app-serving:cloudProvider: cloud
app-serving:models:
  generative:
    - name: <modelName>
      enabled: true
      nodeSelector:
        nodegroup: <node-pool-name>
      modelSource:
        harborRef: harbor.harbor.svc.cluster.local/ai-models/<model>:<tag>
        modelUri: hub/<org>/<model-name>
        storageSize: 40Gi
        storageClass: <storage-class>   # optional; cloud only
app-serving:harborHostname: harbor.harbor.svc.cluster.local
app-serving:harborUser: robot$k8s-puller
app-serving:harborToken:
  secure: <encrypted>
```

| Aspect | Detail |
|--------|--------|
| Cluster | GKE/EKS/AKS; provider reads `KUBECONFIG` env |
| Harbor pull secret | Created (`harbor-creds`) when `harborToken` is set |
| Model weights | Pulled from Harbor by ORAS Job into PVC before inference pod starts |
| Images | Pulled from `ghcr.io` and `registry.k8s.io` by cluster nodes (internet) |
| Helm charts | Downloaded from OCI / HTTP by Pulumi on provisioner laptop |
| Node targeting | Cloud node pool label (`nodegroup: <node-pool-name>`) |
| Namespace | `llm-d-<slug>` |

---

### On-prem air-gap (RKE2) example

```yaml
app-serving:cloudProvider: on-prem
app-serving:models:
  generative:
    - name: <modelName>
      enabled: true
      nodeSelector:
        workload: gpu
      modelSource:
        harborRef: harbor.harbor.svc.cluster.local/ai-models/<model>:<tag>
        modelUri: hub/<org>/<model-name>
        storageSize: 40Gi
        hostpathNode: <node-hostname>
        hostpathDir: /var/lib/hostpath/models/<slug>   # optional; default shown
app-serving:gpuToleration:      # optional; only if the GPU node carries a taint
  key: dedicated
  operator: Equal
  value: gpu
  effect: NoSchedule
app-serving:kubeconfig:     ~/.kube/rke2-cluster.yaml
app-serving:harborHostname: harbor.harbor.svc.cluster.local
app-serving:harborUser:     robot$k8s-puller
app-serving:harborToken:
  secure: <encrypted robot secret>
```

| Aspect | Detail |
|--------|--------|
| Cluster | RKE2; provider uses `~/.kube/rke2-cluster.yaml` |
| Harbor pull secret | Created as `harbor-creds` in each model namespace |
| Model weights | Pulled from Harbor by ORAS Job into PVC (models with `modelSource`) |
| Model PV | hostpath PV auto-created by Pulumi when `hostpathNode` is set; directory managed by `hostpath_dirs` Ansible role |
| GPU scheduling | `gpuToleration` (optional) applied stack-wide to model pods and the ORAS pull Job |
| Images | Pulled from `harbor.harbor.svc.cluster.local/images-shaide/...` by cluster nodes (no internet) |
| ORAS image | `harbor.harbor.svc.cluster.local/images-infra/oras-project/oras:v1.3.1` (air-gapped) |
| Helm charts | Downloaded from OCI / HTTP by Pulumi on provisioner laptop (has internet) |
| Node targeting | RKE2 node label (`workload: cpu` or `workload: gpu`) |
| Namespaces | One per model, derived from slug (e.g. `llm-d-<slug>`) |

**Before `pulumi up` — image preparation on provisioner laptop:**

The full list of images and their skopeo download commands is maintained in
`infra/on-prem/ansible/group_vars/all/images.yaml`. Run after adding any new entry:

```bash
# Download image archives (see images.yaml for full list and download commands)
skopeo copy docker://ghcr.io/llm-d/llm-d-inference-sim:v0.7.1 \
  oci-archive:infra/on-prem/ansible/artifacts/images/llm-d-inference-sim-v0.7.1.tar

skopeo copy docker://registry.k8s.io/gateway-api-inference-extension/epp:v1.2.0 \
  oci-archive:infra/on-prem/ansible/artifacts/images/epp-v1.2.0.tar

# Upload all archives to Harbor
cd infra/on-prem/ansible
ansible-playbook -i inventory/hosts.yml harbor_upload.yml
```

**Before `pulumi up` — model weight preparation (real models only, not sim):**

Sim models use a built-in fake model; they do not require this step. For real models,
weights must be pushed to Harbor before `pulumi up`. The ORAS pull Job created by Pulumi
will then fetch them from Harbor at deploy time.

Use the scripts in `infra/model-registry/` on a machine with internet access:

```bash
cd infra/model-registry

# Build downloader image once
docker build -t hf-downloader:local downloader/

# Download from HuggingFace + push to Harbor in one step
HF_TOKEN=<your-token> HARBOR_HOST=localhost:5000 CACHE_DIR=/tmp/hf-cache ./model-sync.sh
```

Model definitions live in `infra/model-registry/models.yaml`. Each entry maps one model
to a Harbor project, artifact name, and tag.

After `pulumi up`, the ORAS pull Job runs automatically inside the cluster and populates
the PVC. The ModelService pod waits for the Job to complete before starting.

---

## Adding a New Model or Cluster Variant

The folder name under `models/` is the model entry referenced in `models.generative` /
`models.embedder`.
The same model running on a different cluster gets its own folder with cluster-specific
`values.yaml` files (different image registries, `imagePullSecrets`, etc.) while
keeping a unique `<slug>` in the subfolder names so Kubernetes resource names remain
unique within a cluster.

| Folder | Slug | Namespace | Target |
|--------|------|-----------|--------|
| `<ModelName>` | `<slug>` | `llm-d-<slug>` | Cloud |
| `<ModelName>-rke2` | `<slug>-rke2` | `llm-d-<slug>-rke2` | RKE2 on-prem |

1. Create the model folder:
   ```
   deployments/models/<category>/<ModelName>/   # or <ModelName>-<target> for a cluster variant
     gaie-<slug>/values.yaml
     ms-<slug>/values.yaml
   ```
   Both subdirectories must use the same `<slug>` (lowercase alphanumeric + `-`,
   max 47 characters). Each model deployed on the same cluster must have a **unique slug**
   to avoid namespace and resource name collisions.

2. Add the model to the stack config under the appropriate category:
   ```yaml
   app-serving:models:
     generative:             # or embedder:
       - name: ExistingModel
         enabled: true
         nodeSelector:
           <key>: <value>
       - name: NewModel
         enabled: true
         nodeSelector:
           <key>: <value>
   ```

   Or create a new stack config for a new cluster target:
   ```bash
   cd deployments
   pulumi stack init <stack-name>
   pulumi config set --secret harborToken <robot-secret>
   ```

3. Deploy:
   ```bash
   pulumi up --stack <stack-name>
   ```

For on-prem air-gap deployments, additionally:
- Upload model container images to Harbor (see `infra/on-prem/ansible/group_vars/all/images.yaml`).
- Set `harborHostname`, `harborUser`, `harborToken`, and `kubeconfig` in the stack config.
- Use Harbor image references in the model's `values.yaml` files.

---

## Key Design Decisions

**Helm charts downloaded by the provisioner, not the cluster.**
Both cloud and on-prem stacks pull Helm charts from OCI/HTTP registries at `pulumi up`
time. This happens on the provisioner laptop, which always has internet access. Cluster
nodes never need to fetch chart content.

**Container images are the air-gap boundary.**
Only container images are pulled by cluster nodes. On cloud clusters, nodes pull from
public registries directly. On the air-gapped RKE2 cluster, images must be pre-loaded
into Harbor; the `registries.yaml` on each node authenticates containerd against Harbor.

**Model isolation via namespace.**
Each model gets its own namespace (`llm-d-<slug>`). Multiple models run concurrently
on the same cluster without interference — each has its own gateway, EPP, and model
service pods.

**Multiple models in one stack via `models`.**
A single `Pulumi.<stack>.yaml` can deploy multiple models onto the same cluster by
listing them under `models.generative` and/or `models.embedder`. All models share the
cluster-level credentials (`kubeconfig`, `harborHostname`, `harborUser`, `harborToken`),
which are defined once. `nodeSelector` is set per model, so different models can target
different node pools within the same cluster. This eliminates the need to duplicate
on-prem credentials across multiple stack files.

```yaml
# One stack file, two models, shared credentials, per-model nodeSelector
app-serving:cloudProvider: on-prem
app-serving:models:
  generative:
    - name: <ModelA>
      enabled: true
      nodeSelector:
        workload: gpu
  embedder:
    - name: <ModelB>
      enabled: true
      nodeSelector:
        workload: gpu
      modelSource:
        harborRef: harbor.harbor.svc.cluster.local/ai-models/<model>:<tag>
        modelUri: hub/<org>/<model-name>
        storageSize: 5Gi
        hostpathNode: <node-hostname>
app-serving:kubeconfig:     ~/.kube/rke2-cluster.yaml
app-serving:harborHostname: harbor.harbor.svc.cluster.local
app-serving:harborUser:     robot$k8s-puller
app-serving:harborToken:
  secure: <encrypted>
```

**Values merge: file + programmatic.**
Model-specific settings (images, model URI, GPU resources) live in `values.yaml` files
and are owned by the model author. Cross-cutting infrastructure settings (nodeSelector,
GPU tolerations, connection pool limits) are applied programmatically in the Go
components so they are consistent across all models.
