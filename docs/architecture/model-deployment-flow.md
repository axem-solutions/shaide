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
Kubernetes cluster, following llm-d v1.0. It is model-agnostic: the models to deploy are
selected via stack config, and all model-specific settings live in per-model `model.yaml`
files.

Multiple models can be deployed onto the same cluster from a single stack config by
listing them under `models.generative` and/or `models.embedder`. Each model gets its
own namespace and isolated set of resources, but shares the cluster-level credentials
(`kubeconfig`, `harborHostname`, `harborUser`, `harborToken`).

The stack deploys the following components **per model**, in order:

```
Namespace + Harbor pull secret
      ↓
Model storage   (PVC + ORAS pull Job, when the model has a modelSource)
      ↓
Gateway         (Istio Gateway + its parameters ConfigMap + llmd-gateway-<slug> Service)
      ↓
Router          (llm-d-router-gateway chart: InferencePool + EPP + DestinationRule)
      ↓
Model server    (vLLM Deployment, waits for the ORAS Job)
      ↓
      ├─ HTTPRoute        (binds Gateway listener → InferencePool), every model
      └─ EmbeddingService (ClusterIP Service to the model server pods), embedder models
```

Traffic path at runtime:

```
Generative:
client → llmd-gateway-<slug> (ExternalName)
       → Gateway (gateway-<slug>, Service gateway-<slug>-istio)
       → HTTPRoute (llm-d-<slug>)
       → InferencePool (router-<slug>)  ← EPP router-<slug>-epp picks the best pod
       → model server pod (model-<slug>), port 8000

Embedding:
client → ms-<slug>-embeddings.<namespace>.svc.cluster.local:8200
       → model server pod (model-<slug>), port 8000
```

---

## Repository Layout

```
app_serving/
├── charts/
│   └── llm-d-router-gateway/         # Vendored llm-d router chart (v1.0.0)
├── deployments/
│   ├── Pulumi.yaml                   # Stack descriptor (runtime: go, main: ..)
│   ├── Pulumi.<stack>.yaml           # Per-stack config (one file per cluster target)
│   └── models/
│       └── <category>/                     # generative or embedder
│           └── <ModelName>[-<target>]/      # One folder per model+cluster combination
│               └── model.yaml              # Model definition (slug, pins, server, router)
└── main.go                           # Orchestration entry point; calls
                                       # github.com/axem-solutions/ai_platform/pkg/iac/serving
```

The implementation itself lives outside `app_serving/`, in the shared `pkg` module at
`pkg/iac/serving/`:

```
pkg/iac/serving/
├── serving.go                        # DeployAppServing — orchestration entry point
├── modeldef/                         # model.yaml schema, shared with the installer
└── internal/
    ├── config/
    │   ├── config.go                 # Config loading and validation; loads each model.yaml
    │   ├── input.go                  # Raw stack-config shape (stackInput, modelInput, ...)
    │   └── naming.go                 # Namespace/gateway/router/model server name derivation
    ├── components/
    │   ├── gateway/deploy.go         # Istio Gateway and discovery Service
    │   ├── router/                   # llm-d-router-gateway Helm release and its values
    │   ├── modelserver/              # Model server Deployment and runtime metadata
    │   ├── httproute/deploy.go       # HTTPRoute custom resource
    │   └── embeddingservice/deploy.go # ClusterIP Service for embedder models
    ├── legacy/                       # Removes the retired modelservice chart's pods
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

1. Reads `platform` — `gcp`, `aws`, `azure` or `on-prem`. Determines which credentials
   are required.
2. Reads `models.generative` and `models.embedder` lists. Each entry has:
   - `name` — folder name under `deployments/models/<category>/`
   - `enabled` — **boolean** (`true`/`false`); models with `enabled: false` are skipped
   - `nodeSelector` — per-model node targeting map
   - `nameSpace` (optional) — overrides auto-derived `llm-d-<slug>`
   - `releaseName` (optional) — overrides the auto-derived resource suffix (`infra-<slug>`)
3. Loads cluster-wide settings shared by all models:
   - `kubeconfig`, `harborHostname`, `harborUser`, `harborToken`
   - `modelServerImage`, `routerImage`, `orasImage`: the installer's Harbor mirrors, or upstream
4. For each enabled model, loads and validates
   `deployments/models/<category>/<name>/model.yaml` (`pkg/iac/serving/modeldef`).
5. Derives per-model resource names from the definition's slug (unless overridden):
   - Namespace: `llm-d-<slug>`
   - Gateway: `gateway-<slug>` (Istio Service `gateway-<slug>-istio`)
   - Router release and InferencePool: `router-<slug>`
   - Model server Deployment: `model-<slug>`
   - HTTPRoute name: `llm-d-<slug>`
6. Validates credentials per `platform`:
   - Cloud: no additional required credentials (Harbor credentials optional, needed when `modelSource` is set).
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

### Step 4 — Legacy cleanup and the per-model loop (`models/deploy.go`)

Before registering any resource (and not during a preview), the stack deletes the model
server Deployments the retired `llm-d-modelservice` chart left in each enabled model's
namespace and waits for their pods to terminate. Pulumi would otherwise create the new
model server before removing the old one, and on a GPU node the new pods would wait for
GPUs the old pods hold. On a stack deployed after the llm-d v1.0 move there is nothing to
delete.

Then, for each enabled model in the `models` list, steps 5–11 below are executed, each
using the model's `Model` entry (with its own `Namespace`, `ReleaseName`, `Definition`,
etc.).

---

### Step 5 — Namespace

Creates the `llm-d-<slug>` namespace. All resources for this model are isolated within it.

---

### Step 6 — Model Storage: PVC + ORAS Pull Job (optional)

When a model has a `modelSource` block in the stack config, Pulumi creates:

1. On on-prem with `hostpathNode` set: a static `PersistentVolume` (`<slug>-model-pv`) backed
   by `/var/lib/hostpath/models/<slug>` on the specified node. The directory must exist before
   `pulumi up`.
2. A `PersistentVolumeClaim` (`<slug>-model`) — RWO; `hostpath` StorageClass on-prem, cluster
   default on cloud.
3. A `batch/v1 Job` (`<slug>-model-pull`) — runs ORAS once to pull the model artifact
   from Harbor into the PVC. Writes `/model-cache/.pulled` on success for idempotency.
   `backoffLimit: 3`, `ttlSecondsAfterFinished: 86400` (auto-cleaned after 24 h).

The Job uses `harbor-creds` (mounted as `/root/.docker/config.json`) to authenticate
against Harbor. The ORAS image is pulled from where the installer mirrored it
(`app-serving:orasImage`, `<harbor>/services/oras-project/oras:v1.3.1`), or from `ghcr.io`
when no mirrored reference is set.

The model server Deployment (Step 10) depends on both the PVC and the Job, so the
inference pod never starts with an empty PVC.

See [Model storage](model-storage.md) for the full directory layout and recovery
procedure.

---

### Step 7 — Harbor Pull Secret

When `harborToken` is set (i.e. `HarborTokenSet` is true), creates a `harbor-creds`
`kubernetes.io/dockerconfigjson` secret in the model's namespace, authenticating
against Harbor with the robot account credentials (`harborUser` / `harborToken`).

This secret serves two purposes:
- **`imagePullSecrets`** — set on the model server and router pods, so cluster nodes (no
  internet) can pull the mirrored images from Harbor.
- **ORAS Job authentication** — mounted as `/root/.docker/config.json` in the ORAS pull
  Job so `oras pull` can authenticate against Harbor to fetch model artifacts.

---

### Step 8 — Gateway (`gateway/deploy.go`)

Creates, without any Helm chart:
- A Gateway API `Gateway` (`gateway-<slug>`) of class `istio` with an HTTP listener on
  port 80, labeled `istio.io/enable-inference-extproc: "true"`. Istio provisions its proxy
  Deployment and the Service `gateway-<slug>-istio`.
- The `ConfigMap` the Gateway's `infrastructure.parametersRef` names. It makes the
  Service `ClusterIP` and sets the istio-proxy log level and resources, following the
  llm-d v1.0 Istio gateway recipe.
- An `ExternalName` Service (`llmd-gateway-<slug>`) resolving to `gateway-<slug>-istio` —
  labeled so shaide-server discovers the model's endpoint by label selector.

---

### Step 9 — Router (`router/`)

**Chart source**: the `llm-d-router-gateway` chart v1.0.0, vendored unpacked in
`app_serving/charts/llm-d-router-gateway` and copied into the installer image. The same
copy is used on every platform, so neither the provisioner nor the cluster needs access to
a chart registry.

The values are built in Go (`router/values.go`):
- EPP image from `app-serving:routerImage`, split into the chart's registry, repository and
  tag (the chart's own default tag is `main`, so it is always set)
- `InferencePool` targeting port 8000 of the pods labeled `llm-d.ai/model: model-<slug>`,
  `llm-d.ai/role: decode`, with `failureMode: FailOpen`
- EPP configuration: llm-d's optimized baseline as a `llm-d.ai/v1` `EndpointPickerConfig`,
  or the model's own `router.pluginsConfig`
- Istio provider: a `DestinationRule` for the EPP Service (TLS SIMPLE with
  `insecureSkipVerify`, raised connection pool limits)
- `httpRoute.create: false` — the platform creates its own HTTPRoute (Step 11)
- With more than one router replica, a rolling update strategy that tolerates the
  leader-elected standbys, so the release does not wait forever for them to become ready

Deploys:
- The EPP Deployment and Service `router-<slug>-epp` — the Gateway calls it over ext-proc
  for every request, and it picks the model server pod.
- The `InferencePool` `router-<slug>` — the traffic target for the HTTPRoute.

---

### Step 10 — Model Server (`modelserver/`)

The Deployment `model-<slug>` is built from the Kubernetes API types
(`modelserver/deployment.go`) and applied as a manifest, so it is exactly what the unit
tests check. It follows the llm-d v1.0 single-host decode recipe:

- Container `modelserver`, port 8000 named `modelserver`; liveness `/health`, readiness and
  startup `/v1/models`. The startup probe allows 30 minutes.
- **vLLM** (`server.engine: vllm`, the default): `vllm serve <org>/<model> --port 8000
  --served-model-name <model.name> --shutdown-timeout=45` followed by `server.args`. The
  image is the model's `server.image` or the stack's `modelServerImage`.
- **Simulator** (`server.engine: sim`): the image's own entrypoint with
  `--model <model.name> --port 8000`. The image is the model's `server.image` or the
  stack's `simulatorImage`.
- **Weights**: with a `modelSource`, the model's PVC mounted read-only at `/model-cache`,
  `HF_HUB_CACHE=/model-cache/hub` and `HF_HUB_OFFLINE=1`; without one, an `emptyDir` sized
  to `model.size` that vLLM downloads into.
- **Placement**: the model's node affinity, the stack's GPU toleration plus the model's own
  tolerations. Replicas of a model with a PVC must share a node, because the PVC is
  `ReadWriteOnce`.
- **Graceful shutdown**: a 15 s `preStop` sleep (the pod leaves the pool first), vLLM's
  45 s drain, a 75 s termination grace period.
- **Rollout**: `Recreate` for one replica; with several replicas, a rolling update of one
  pod at a time that needs no spare GPU.
- **Runtime metadata** (`server.runtimeMetadata: true`): the platform's vLLM middleware,
  mounted from a content-addressed ConfigMap.
- Service links are disabled: they inject `VLLM_*`-looking variables that vLLM would read
  as its own settings.

**Inference server for CPU-only embedding models (on-prem)**:
The vLLM CUDA image cannot start on CPU-only nodes. For CPU-only verification use the
simulator, or a model with its own `server.image` (for example a vLLM CPU image).

**Image pull**:
- Cloud: images pulled from the Harbor mirrors the installer created, or from upstream
  registries on a direct `pulumi up`.
- On-prem: images pulled from Harbor using `harbor-creds`.

---

### Step 11 — HTTPRoute and Embedding Service

**Every model** (`httproute/deploy.go`): creates a `gateway.networking.k8s.io/v1
HTTPRoute` custom resource that:
- Binds to the Gateway created in Step 8.
- Routes all traffic (`PathPrefix: /`) to the `InferencePool` created in Step 9.
- Sets no request timeout, so long streamed generations are not cut off.

This completes the traffic chain. Inference requests arriving at the gateway are
forwarded to the EPP, which selects the model server pod by prefix-cache affinity and
token load.

**Embedder models** (`embeddingservice/deploy.go`): additionally creates a `ClusterIP`
Service (`ms-<slug>-embeddings`) on port 8200 that selects the model server pods directly
and forwards to their port 8000. shaide-server discovers and calls embedders through it.

---

## Stack Configurations

See [`app_serving/deployments/Pulumi.TEMPLATE.yaml`](https://github.com/axem-solutions/shaide/blob/main/app_serving/deployments/Pulumi.TEMPLATE.yaml) for the full,
up-to-date config reference with every key documented. The two examples below illustrate
the cloud vs. on-prem shape; copy the template to create a real stack (`cp
deployments/Pulumi.TEMPLATE.yaml deployments/Pulumi.<stack-name>.yaml`).

### Cloud example

```yaml
app-serving:platform: gcp
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
| Images | The installer's Harbor mirrors (`modelServerImage`, `routerImage`); upstream on a direct `pulumi up` |
| Helm charts | The vendored router chart in `app_serving/charts/` |
| Node targeting | Cloud node pool label (`nodegroup: <node-pool-name>`) |
| Namespace | `llm-d-<slug>` |

---

### On-prem air-gap (RKE2) example

```yaml
app-serving:platform: on-prem
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
| Model PV | hostpath PV auto-created by Pulumi when `hostpathNode` is set; the directory must already exist on the node |
| GPU scheduling | The installer passes each model's placement: the node pools assigned to it in the `Assign node pools` stage, matched on the pool label the platform maintains (e.g. `kubernetes.azure.com/agentpool`, or `nodegroup` on-prem), one `nodeSelectorTerm` per label. Without a placement the stack falls back to `nodegroup=generative`. Models tolerate `nvidia.com/gpu=present:NoSchedule`; the stack writes the toleration as `gpuToleration`, applied to model pods and the ORAS pull Job |
| Images | Pulled from Harbor by cluster nodes (no internet) |
| ORAS image | `harbor.harbor.svc.cluster.local/services/oras-project/oras:v1.3.1`, mirrored by the installer |
| Helm charts | The vendored router chart in `app_serving/charts/`; nothing is downloaded |
| Node targeting | RKE2 node label (`workload: cpu` or `workload: gpu`) |
| Namespaces | One per model, derived from slug (e.g. `llm-d-<slug>`) |

**Before `pulumi up` — image preparation on provisioner laptop:**

The installer copies the images of supported models into Harbor in its
[`Populate Harbor` stage](../installation/installer-guide.md#populate-harbor-stage)
(see `installer/build/manifests/images.yaml`): vLLM, the llm-d router EPP, ORAS and Istio,
and the inference simulator when the `Simulator` model is served. A manual stack copies
the simulator image by hand:

```bash
skopeo copy docker://ghcr.io/llm-d/llm-d-inference-sim:v1.0.0 \
  docker://<harbor-host>/<project>/llm-d-inference-sim:v1.0.0
```

A manual stack without the installer also sets `modelServerImage`, `routerImage` and
`simulatorImage` to the Harbor copies of `docker.io/vllm/vllm-openai:v0.31.0`,
`ghcr.io/llm-d/llm-d-router-endpoint-picker:v1.0.0` and
`ghcr.io/llm-d/llm-d-inference-sim:v1.0.0`.

**Before `pulumi up` — model weight preparation (real models only, not sim):**

Sim models use a built-in fake model; they do not require this step. For real models,
weights must be pushed to Harbor before `pulumi up`. The ORAS pull Job created by Pulumi
will then fetch them from Harbor at deploy time.

The installer does this in its [`Populate Harbor` stage](../installation/installer-guide.md#populate-harbor-stage):
it downloads the pinned Hugging Face revision of each selected model and pushes it to
Harbor as `ai-models/<slug>:<first 12 characters of the revision>`.

After `pulumi up`, the ORAS pull Job runs automatically inside the cluster and populates
the PVC. The model server pods wait for the Job to complete before starting.

---

## Adding a New Model or Cluster Variant

The folder name under `models/` is the model entry referenced in `models.generative` /
`models.embedder`. The same model running on a different cluster gets its own folder with
a cluster-specific `model.yaml` (different tolerations, image, tuning) and, when both may
run on the same cluster, its own `slug` so Kubernetes resource names remain unique.

| Folder | Slug | Namespace | Target |
|--------|------|-----------|--------|
| `<ModelName>` | `<slug>` | `llm-d-<slug>` | Cloud |
| `<ModelName>-rke2` | `<slug>-rke2` | `llm-d-<slug>-rke2` | RKE2 on-prem |

1. Create the model folder and its definition:
   ```
   deployments/models/<category>/<ModelName>/   # or <ModelName>-<target> for a cluster variant
     model.yaml
   ```
   The `slug` is lowercase alphanumeric + `-`, max 47 characters. Each model deployed on
   the same cluster must have a **unique slug** to avoid namespace and resource name
   collisions. See [Adding a New Model](model-serving.md#adding-a-new-model) for the schema.

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
- Make sure the model's container images are in Harbor (see image preparation above).
- Set `harborHostname`, `harborUser`, `harborToken`, and `kubeconfig` in the stack config.

---

## Key Design Decisions

**Charts are vendored, images come from Harbor.**
The only Helm chart, the llm-d router, is committed unpacked under `app_serving/charts/`,
so a deploy needs no chart registry on any platform. The Gateway and the model server are
plain Kubernetes objects built in Go: the llm-d v1.0 project retired its `llm-d-infra` and
`llm-d-modelservice` charts in favour of Kustomize recipes, and shaide reproduces those
recipes in Go, where its own settings (placement, tolerations, weights volume, metadata)
are applied and unit tested.

**Container images are the air-gap boundary.**
Only container images are pulled by cluster nodes. On cloud clusters, nodes pull from
the Harbor mirrors or public registries. On the air-gapped RKE2 cluster, images must be
pre-loaded into Harbor; the `registries.yaml` on each node authenticates containerd
against Harbor.

**Model isolation via namespace.**
Each model gets its own namespace (`llm-d-<slug>`). Multiple models run concurrently
on the same cluster without interference — each has its own gateway, EPP, and model
server pods.

**Multiple models in one stack via `models`.**
A single `Pulumi.<stack>.yaml` can deploy multiple models onto the same cluster by
listing them under `models.generative` and/or `models.embedder`. All models share the
cluster-level credentials (`kubeconfig`, `harborHostname`, `harborUser`, `harborToken`),
which are defined once. `nodeSelector` is set per model, so different models can target
different node pools within the same cluster. This eliminates the need to duplicate
on-prem credentials across multiple stack files.

```yaml
# One stack file, two models, shared credentials, per-model nodeSelector
app-serving:platform: on-prem
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

**Model definition + platform policy.**
Model-specific settings (vLLM arguments, GPU resources, replicas, router tuning) live in
`model.yaml` and are owned by the model author. Cross-cutting settings (port, probes,
graceful shutdown, placement, GPU tolerations, weights volume, connection pool limits) are
applied in the Go components so they are consistent across all models. The platform
rejects a definition that repeats a flag it sets.
