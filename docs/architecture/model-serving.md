---
title: "Model serving"
description: "Per-model inference stacks built on vLLM and llm-d."
weight: 40
---

# Model serving

This is the application layer Pulumi project that deploys the distributed LLM model serving
infrastructure on Kubernetes. It follows [llm-d](https://llm-d.ai) v1.0: every model gets an
llm-d router (an Istio Gateway as the proxy and the llm-d endpoint picker, EPP) in front of
its vLLM model servers.

## Overview

Each enabled model gets its own namespace, `llm-d-<slug>`, holding:
- an **Istio inference Gateway** and the `llmd-gateway-<slug>` discovery Service
- an **llm-d router** release: the `InferencePool` and its endpoint picker (EPP)
- the **model server** Deployment running vLLM (or the inference simulator)
- an **HTTPRoute** binding the Gateway to the `InferencePool`
- for embedders, an **embedding Service** in front of the model servers
- when the model's weights come from Harbor, its **PVC** and the **ORAS pull Job** filling it

## Architecture

The components live under `pkg/iac/serving/internal/components/`:

1. **Gateway** (`gateway/deploy.go`): the model's Istio Gateway, `gateway-<slug>`
   - Gateway API `Gateway` of class `istio`, HTTP listener on port 80, labeled
     `istio.io/enable-inference-extproc: "true"` so Istio calls the EPP for its routes
   - Infrastructure `ConfigMap` making Istio's proxy Service `ClusterIP` and setting the
     proxy's log level and resources (from the llm-d v1.0 Istio gateway recipe)
   - `llmd-gateway-<slug>`: ExternalName Service for `gateway-<slug>-istio`, labeled for
     shaide-server discovery

2. **Router** (`router/`): the llm-d router, `router-<slug>`
   - Helm release of the vendored `llm-d-router-gateway` chart v1.0.0
     (`app_serving/charts/llm-d-router-gateway`, see `app_serving/charts/README.md`), used on
     every platform
   - Creates the `InferencePool` `router-<slug>` (port 8000, selecting the model server pods),
     the EPP Deployment and Service `router-<slug>-epp`, and the Istio `DestinationRule` for
     the Gateway's ext-proc calls
   - EPP image `llm-d-router-endpoint-picker:v1.0.0`, mirrored into Harbor by the installer
   - EPP configuration: llm-d's optimized baseline (`prefix-cache-affinity-filter` and
     `token-load-scorer`), as a `llm-d.ai/v1` `EndpointPickerConfig`
   - `failureMode: FailOpen`: if the EPP is unreachable, requests still reach a pod, only
     without load-aware placement
   - EPP resources: 1 CPU / 2Gi requested, 16Gi memory limit, CPU unlimited; a model under heavy
     load raises them with `router.resources`

3. **Model server** (`modelserver/`): the Deployment `model-<slug>`
   - Shaped like the llm-d v1.0 single-host decode recipe: container `modelserver`, port
     8000, liveness `/health`, readiness and startup `/v1/models` (the startup probe allows
     30 minutes for loading weights)
   - vLLM image `vllm/vllm-openai:v0.31.0` by default, mirrored into Harbor; a model may
     name its own image
   - `vllm serve <org>/<model> --port 8000 --served-model-name <model.name> --shutdown-timeout=45`,
     followed by the model's own `server.args`
   - Weights from the model's PVC, read offline from its Hugging Face cache
     (`HF_HUB_CACHE=/model-cache/hub`, `HF_HUB_OFFLINE=1`)
   - Graceful shutdown: the pod leaves the pool during a 15 s `preStop` sleep, then vLLM
     finishes in-flight requests for up to 45 s; the termination grace period is 75 s
   - Pod labels `llm-d.ai/model: model-<slug>` and `llm-d.ai/role: decode`, which the
     `InferencePool` and the embedding Service select on
   - One replica is replaced with the `Recreate` strategy (a rolling update would wait for the
     GPU the old pod holds); several replicas roll one at a time without extra GPUs

4. **HTTPRoute** (`httproute/deploy.go`): `llm-d-<slug>`, from the Gateway to the
   `InferencePool`, without route timeouts so long streamed responses are not cut off.
   Created for generative and embedder models.

5. **Embedding Service** (`embeddingservice/deploy.go`): embedder models only
   - ClusterIP Service `ms-<slug>-embeddings`, port 8200, forwarding to the model servers'
     port 8000
   - Reachable as `http://ms-<slug>-embeddings.<namespace>.svc.cluster.local:8200/v1/embeddings`

### Traffic flow

**Generative models:**
shaide-server → `llmd-gateway-<slug>` → Istio Gateway → `InferencePool` (the EPP picks the pod)
→ model server pod, port 8000

**Embedding models:**
shaide-server → `ms-<slug>-embeddings.<namespace>.svc.cluster.local:8200` → model server pod,
port 8000

The llm-d v1.0 router also parses `/v1/embeddings`, so embedders are routed through their
Gateway as well. shaide-server still uses the embedding Service.

#### End-to-end call path for generative requests

```
shaide-server
    │  POST /v1/chat/completions
    ▼
llmd-gateway-<slug>  (ExternalName → gateway-<slug>-istio)
    ▼
Istio Gateway ── ext-proc ──▶ router-<slug>-epp  (picks the pod)
    ▼
model-<slug> pod, vLLM on :8000
```

External ingress and TLS for generative models are handled by the cloud-specific infra layer
(e.g., AWS Load Balancer Controller or GKE Gateway + Certificate Manager), keeping this app
layer provider-agnostic.

### Router responses

The EPP can answer a request itself, without reaching a model server, and then sets the
`x-llm-d-request-dropped-reason` header. Values starting with `rejected-` mean no inference
work was done, so the request is safe to retry:

| Status | Reason | Meaning |
|---|---|---|
| 503 | `rejected-no-endpoints` | No model server pod is ready |
| 503 | `rejected-shutting-down` | The EPP is draining during a restart |
| 429 | `rejected-saturated` | Flow control found the pool saturated |

Flow control (queuing and the 429 responses) is an EPP feature gate the platform does not
enable.

## Configuration

The application is configured via Pulumi config values in `deployments/Pulumi.{stack}.yaml`:

### Required Configuration
- `platform`: Target platform: `gcp`, `aws`, `azure` or `on-prem`. Determines which credentials are required and how the Kubernetes provider is configured.
- `models`: Model list, split by category. Each entry names a folder under `deployments/models/<category>/` and specifies per-model settings:
  ```yaml
  app-serving:models:
    generative:
      - name: DeepSeek-Coder-V2-Lite-Instruct
        enabled: true          # must be boolean true/false, not a string
        nodeSelector:
          nodegroup: g2-standard-48-l4
    embedder:
      - name: BGE-M3
        enabled: true
        nodeSelector:
          nodegroup: g2-standard-48-l4
  ```
  All models share the same cluster credentials (`kubeconfig`, `harborHostname`, `harborUser`, `harborToken`). Each model gets its own namespace and set of resources derived from its slug.
- `harborToken`: Harbor robot account secret (encrypted). Required on on-prem; also required on cloud stacks that use `modelSource.harborRef`. Set via `pulumi config set --secret harborToken <secret>`.

### Optional Per-Model Configuration
These can be added inside a model entry to override auto-derived values:
- `nameSpace`: Override auto-derived namespace (default: `llm-d-<slug>`).
- `releaseName`: Override the auto-derived resource suffix: `infra-<suffix>` names the model's
  `gateway-<suffix>`, `router-<suffix>` and `model-<suffix>` (default: `infra-<slug>`).

### Optional Images
The installer sets these to its Harbor mirrors; a direct `pulumi up` falls back to upstream.
- `modelServerImage`: vLLM image for models whose `model.yaml` names none (default: `docker.io/vllm/vllm-openai:v0.31.0`).
- `routerImage`: llm-d router EPP image (default: `ghcr.io/llm-d/llm-d-router-endpoint-picker:v1.0.0`).

### On-Prem / Air-Gap Configuration (optional)
These fields are only needed when deploying to an air-gapped RKE2 cluster. All are
absent from cloud stack configs.

- `kubeconfig`: Path to the RKE2 cluster kubeconfig on the provisioner laptop (e.g. `~/.kube/rke2-cluster.yaml`). Omit to fall back to `KUBECONFIG` env var or `~/.kube/config`.
- `harborHostname`: Internal Harbor registry hostname for containerd image pulls (e.g. `harbor.internal.lan`).
- `harborUser`: Harbor robot account name (e.g. `robot$k8s-puller`).
- `harborToken`: Harbor robot account secret (encrypted). Required for on-prem. Set via `pulumi config set --secret harborToken <secret>`. When set, a `harbor-creds` pull secret is created in each model namespace and used by the model server and router pods.

## Prerequisites

1. **Kubernetes Cluster**: GCP/AWS/Azure cloud cluster, or an on-prem RKE2 cluster
2. **Gateway Provider Prereqs**: Gateway API v1.6.2, Gateway API Inference Extension v1.6.2 and llm-d router v1.0.0 CRDs, and Istio 1.29 with the inference extension enabled, all installed via `infra/gateway-provider`
3. **GPU drivers**: the `vllm/vllm-openai:v0.31.0` image is built for CUDA 13.0, which needs NVIDIA driver 580 or newer. The installer checks the GPU nodes against the image before deploying. For older drivers use the `v0.31.0-cu129` tag.
4. **Harbor Registry**: Models stored as OCI artifacts in Harbor. The installer deploys Harbor when needed in its [`Discovery` stage](../installation/installer-guide.md#discovery-stage). Model artifacts are uploaded by the installer's [`Populate Harbor` stage](../installation/installer-guide.md#populate-harbor-stage).
5. **Go**: Go 1.26+
6. **Pulumi**: Pulumi CLI
7. **On-prem only**: Container images uploaded to Harbor by the installer's `Populate Harbor` stage. The `hostpath` StorageClass is used for model PVCs. When `modelSource.hostpathNode` is set, Pulumi creates the PV automatically, but the backing directory must already exist on that node.
8. **Real models only**: Model OCI artifacts pushed to Harbor by the installer before `pulumi up`. See [Model Weight Pre-loading](#model-weight-pre-loading) below.

## Deployment

1. **Set Configuration**:
   ```bash
   # Set Harbor robot token (encrypted)
   pulumi config set --secret harborToken <robot-secret>
   ```

2. **Deploy**:
   ```bash
   cd app_serving/deployments
   pulumi up
   ```

3. **Verify Deployment**:
   ```bash
   kubectl get pods -n llm-d-<slug>
   kubectl get gateway,httproute,inferencepool -n llm-d-<slug>
   ```

   A request through the Gateway, from inside the cluster:
   ```bash
   kubectl run curl-test -n llm-d-<slug> --rm -i --restart=Never --image=curlimages/curl -- \
     curl -sS http://gateway-<slug>-istio/v1/models
   ```

## Upgrading from the llm-d-infra / llm-d-modelservice charts

Stacks deployed before the llm-d v1.0 move ran the `llm-d-infra`, GAIE `inferencepool` and
`llm-d-modelservice` charts. The next `pulumi up`:

1. Deletes each enabled model's old model server Deployments
   (`llm-d.ai/model=ms-<slug>-llm-d-modelservice`) and waits for their pods to terminate, so
   their GPUs are free for the new model servers. This happens before Pulumi registers any
   resource, and not during a preview.
2. Creates the new Gateway, router and model server under their new names, and moves the
   HTTPRoute, the `llmd-gateway-<slug>` Service and the embedding Service to them in place.
3. Removes the old charts' remaining objects.

The model is unavailable from step 1 until its new model server is ready. The weights PVC is
kept, so nothing is downloaded again. Upgrade `infra/gateway-provider` first: the new router
needs its CRDs and Istio 1.29.

## Single-GPU Swaps

If you want to deploy a different model on the same GPU, first remove the current deployment and then deploy the new one:

```bash
cd app_serving/deployments
pulumi destroy
pulumi up
```

This is needed when the old model must be removed before the new pod can be scheduled on the same GPU.

## Supported Models

### Generative Models
- **Simulator**: llm-d inference simulator for testing; no GPU, no weights
- **DeepSeek-Coder-V2-Lite-Instruct**: Code generation model
- [**GLM-4.7-Flash**](https://huggingface.co/zai-org/GLM-4.7-Flash)
- [**GPT-OSS-20B**](https://huggingface.co/openai/gpt-oss-20b)
- [**Qwen3-Coder-30B-A3B-Instruct**](https://huggingface.co/Qwen/Qwen3-Coder-30B-A3B-Instruct)
- [**Devstral-Small-2-24B-Instruct-2512**](https://huggingface.co/mistralai/Devstral-Small-2-24B-Instruct-2512)

| Model                                  | Parameters | Tensor Type | Quantized                                                            |
|----------------------------------------|------------|-------------|----------------------------------------------------------------------|
| **GLM-4.7-Flash**                      | 31B        | BF16 / F32  |  [FP8](https://huggingface.co/unsloth/GLM-4.7-Flash-FP8-Dynamic)     |
| **Qwen3-Coder-30B-A3B-Instruct-FP8**   | 31B        | BF16        |  [FP8](https://huggingface.co/Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8) |
| **Devstral-Small-2-24B-Instruct-2512** | 24B        | BF16        |   -                                                                  |
| **Qwen3.5-27B-FP8**                    | 28B        |  BF16       | [FP8](https://huggingface.co/Qwen/Qwen3.5-27B-FP8)                   |

#### Benchmarks

| Benchmark                    | GLM-4.7-Flash | GPT-OSS-20B | Qwen3-Coder-30B-A3B-Instruct | Devstral-Small-2-24B-Instruct-2512 | Qwen3.5-27B |
|------------------------------|--------------:|------------:|-----------------------------:|-----------------------------------:|------------:|
| AIME 25                      |          91.6 |        91.7 |                            — |                                  — |           — |
| GPQA                         |          75.2 |        71.5 |                            — |                                  — |        85.5 |
| HLE                          |          14.4 |        10.9 |                            — |                                  — |           — |
| SWE-bench Verified           |          59.2 |        34.0 |                        51.6  |                              68.0% |        72.4 |
| LCB v6                       |         64.0  |        61.0 |                            — |                                  — |           — |
| τ²-Bench                     |          79.5 |        47.7 |                            — |                                  — |           — |
| BrowseComp                   |          42.8 |        28.3 |                            — |                                  — |        61.0 |
| Terminal-Bench               |             — |           — |                         31.3 |                              22.5% |        41.6 |
| SWE-bench Multilingual       |             — |           — |                         34.7 |                              55.7% |           — |

#### Error
This error applies to both **Qwen3.5-27B-FP8** and **GLM-4.7-Flash-FP8-Dynamic** when the installed `transformers` version does not yet recognize the model architecture.

```text
Value error, The checkpoint you are trying to load has model type `qwen3_5` but Transformers does not recognize this architecture. This could be because of an issue with the checkpoint, or because your version of Transformers is out of date. You can update Transformers with the command `pip install --upgrade transformers`.

If this does not work, and the checkpoint is very new, then there may not be a release version that supports this model yet.
In this case, you can get the most up-to-date code by installing Transformers from source with the command `pip install git+https://github.com/huggingface/transformers.git`
```

###  Embedder Models
- [**EmbeddingGemma-300M**](https://huggingface.co/google/embeddinggemma-300m)
- [**BGE-M3**](https://huggingface.co/BAAI/bge-m3)
- [**jina-embeddings-v5-text-small-retrieval**](https://huggingface.co/jinaai/jina-embeddings-v5-text-small-retrieval)
- [**nomic-embed-text-v1.5**](https://huggingface.co/nomic-ai/nomic-embed-text-v1.5)

| Model                                   | Parameters | Max context | Max-num-seqs |Max-num-batched-tokens | Output dimension |
|-----------------------------------------|------------|-------------|--------------|-----------------------| -----------------|
| EmbeddingGemma-300M                     | ~300M      | 2048        | 16           | 12288                 | 768              |
| BGE-M3                                  | ~568M      | 2048        | 16           | 8192                  | 1024             |
| Jina-Embeddings-v5-text-small-retrieval | ~677M      | 32K         | 16           | 12288                 | 1024             |
| Nomic-Embed-text-v1.5                   | ~137M      | 2048        | 8            | 8192                  | 768              |

[Embedding Leaderboard](https://huggingface.co/spaces/mteb/leaderboard)

Each entry below is a folder under `deployments/models/<category>/`. The same model may have
multiple folders for different cluster targets (e.g. `GPT-OSS-20B` for the cloud vs
`GPT-OSS-20B-rke2` for RKE2).

| Folder | Model | Target | Notes |
|--------|-------|--------|-------|
| `Simulator` | llm-d inference simulator | Any | No GPU or weights; the stack's `simulatorImage`; offered by the installer |
| `sim-local-emb` | llm-d inference simulator (embeddings) | Local cluster | No GPU required; ghcr.io image; manual stacks only |
| `sim-rke2-2` | llm-d inference simulator (embeddings) | RKE2 on-prem | Harbor image; manual stacks only |
| `DeepSeek-Coder-V2-Lite-Instruct` | DeepSeek Coder V2 Lite | GCP | 2 replicas × 2 GPUs (TP=2), real model weights |
| `GLM-4.7-Flash` | Z.ai GLM-4.7-Flash | Cloud | 4× GPU, real model weights |
| `GPT-OSS-20B` | GPT-OSS 20B | GCP / on-prem | 1× GPU, real model weights |
| `Qwen3-Coder-30B-A3B-Instruct` | Qwen3 Coder 30B A3B Instruct | Cloud | BF16, tuned for 1× 96 GB GPU; use FP8 or TP>1 on 48 GB GPUs |
| `nomic-embed-text-v1-5-rke2` | nomic-ai/nomic-embed-text-v1.5 | RKE2 on-prem | Model weights pre-loaded via Harbor + ORAS (`modelSource.hostpathNode`) |

## Adding a New Model

A model is a directory `deployments/models/<category>/<ModelName>/` holding one `model.yaml`.
The installer reads it to offer and download the model, the stack to deploy it; both parse it
with `pkg/iac/serving/modeldef`, which rejects unknown fields.

### `model.yaml`

```yaml
# The slug names the namespace, resources and pod labels.
slug: alma-pro

# Hugging Face commit the installer downloads; it also derives the Harbor tag.
revision: "0123456789abcdef0123456789abcdef01234567"
dependencies:                      # optional, further pinned repositories
  - {id: org/tokenizer, revision: "..."}

model:
  name: test-org/Test-Alma-V5-Pro  # Hugging Face repository and served model name
  size: 40Gi                       # model volume size

server:
  engine: vllm                     # vllm (default) or sim
  image: ""                        # optional; default: the stack's vLLM image
  replicas: 1                      # default 1
  args:                            # appended to the platform's vLLM arguments
    - "--max-model-len"
    - "32768"
  env: []                          # corev1.EnvVar list; replaces platform variables of the same name
  resources:
    limits: {nvidia.com/gpu: "1", memory: 40Gi, cpu: "16"}
    requests: {nvidia.com/gpu: "1", memory: 40Gi, cpu: "16"}
  volumes: []                      # corev1.Volume list, e.g. a memory-backed /dev/shm
  volumeMounts: []
  tolerations: []                  # added to the stack's GPU toleration
  startupProbe: null               # optional override of the 30 minute startup probe
  runtimeMetadata: false           # serve GET /v1/shaide/metadata (vLLM only)

router:                            # optional
  replicas: 1                      # more replicas run leader-elected (one active EPP)
  resources: null                  # optional override of the EPP resources
  pluginsConfig: ""                # optional complete llm-d.ai/v1 EndpointPickerConfig
```

The platform sets `--port`, `--served-model-name` and `--shutdown-timeout`; `server.args`
must not repeat them. The simulator (`engine: sim`) runs `server.image`, or the stack's `simulatorImage` when the
model names none; it is started with `--model <model.name> --port 8000`, needs no weights
or GPU, and so needs no `model.size`.

#### Replicas and the weights volume

The weights PVC is `ReadWriteOnce`, so all replicas of a model run on the same node: the
Deployment requires it, rather than leaving replicas on other nodes stuck attaching the
volume. Size the node for every replica's GPUs, as `DeepSeek-Coder-V2-Lite-Instruct` does
(two TP=2 replicas on a 4-GPU node).

#### Router tuning

The optimized baseline's `prefix-cache-affinity-filter` keys its saturation override off
`peakPrefillThroughput`, whose default is measured for Qwen3-32B on H100 (TP=2). For
multi-replica models on other hardware, measure it with llm-d's calibration recipe
(`guides/recipes/router/calibration` in the llm-d repository) and set it in
`router.pluginsConfig`.

### Naming flow

Everything flows from the slug:

| Resource | Naming pattern | Example (slug: `deepseek-coder`) |
|---|---|---|
| Namespace | `llm-d-<slug>` | `llm-d-deepseek-coder` |
| Gateway | `gateway-<slug>` | `gateway-deepseek-coder` |
| Gateway Service (Istio) | `gateway-<slug>-istio` | `gateway-deepseek-coder-istio` |
| Discovery Service | `llmd-gateway-<slug>` | `llmd-gateway-deepseek-coder` |
| Router release, `InferencePool` | `router-<slug>` | `router-deepseek-coder` |
| EPP Service | `router-<slug>-epp` | `router-deepseek-coder-epp` |
| Model server Deployment | `model-<slug>` | `model-deepseek-coder` |
| HTTPRoute | `llm-d-<slug>` | `llm-d-deepseek-coder` |
| Embedding Service | `ms-<slug>-embeddings` | `ms-deepseek-coder-embeddings` |
| Weights PVC | `<slug>-model` | `deepseek-coder-model` |

### Steps

1. **Create the model directory and its `model.yaml`**, starting from a similar model, e.g.
   `generative/GPT-OSS-20B/model.yaml` for a single-GPU vLLM model or
   `generative/Simulator/model.yaml` for a simulator.

2. **Add the model to the stack config file** under the appropriate category:

   ```yaml
   app-serving:models:
     generative:
       - name: Test-Alma-V5-Pro
         enabled: true
         nodeSelector:
           nodegroup: <node-pool-name>
         modelSource:
           harborRef: harbor.harbor.svc.cluster.local/ai-models/alma-pro:0123456789ab
           modelUri: hub/test-org/Test-Alma-V5-Pro
           storageSize: 40Gi
   ```

   Or create a new stack config from the template:

   ```bash
   cp Pulumi.TEMPLATE.yaml Pulumi.<stack-name>.yaml
   ```

3. **Create a Pulumi stack and set the secret** (new stacks only)

   ```bash
   cd app_serving/deployments
   pulumi stack init <stack-name>
   pulumi config set --secret harborToken <robot-secret>
   ```

4. **Preview and deploy**

   ```bash
   pulumi preview    # verify what will be created
   pulumi up         # deploy
   ```

The unit test `TestPackagedModelsBuild` in `pkg/iac/serving/internal/components/modelserver`
builds every packaged `model.yaml`, so a definition the stack cannot deploy fails in CI.

## Exposing Installed Models

app_serving does not export a model list. Instead, each model's routable Service is labeled
with `axem.dev/model-slug`, `axem.dev/model-category` (`generative`/`embedder`), and
`app.kubernetes.io/part-of=app-serving` (see `Model.MetaLabels()` in
`pkg/iac/serving/internal/config/naming.go`): the generative path's `llmd-gateway-<slug>`
ExternalName Service and the embedder path's `ms-<slug>-embeddings` ClusterIP Service.

app-shaide grants shaide-server cluster-wide read access to `services` and `namespaces` (see
`pkg/iac/shaide/internal/platform/k8s-rbac.go`), so it discovers this topology by label
selector at runtime and pairs each Service with the model-owned metadata (name, context size,
...) served by vLLM's own `/server-info` endpoint. `model.yaml` configures the model's
deployment; it is not read to describe the model. See
[Installed Model Discovery](application-layer.md#installed-model-discovery).

Two enabled models resolving to the same slug collide on the Kubernetes objects they would
create (same `llm-d-<slug>` namespace, same resource names), so the deploy fails at apply
time. Nothing in the stacks detects two different slugs serving the same model name; that
check belongs with shaide-server, which aggregates `/server-info` across models.

## Dependencies

The project uses:
- **Pulumi Kubernetes Provider** v4.25.0
- **llm-d** v1.0: the `llm-d-router-gateway` chart v1.0.0 (vendored under `app_serving/charts/`)
  and the `llm-d-router-endpoint-picker` v1.0.0 image
- **vLLM** `vllm/vllm-openai` v0.31.0 (the version llm-d v1.0's GPU recipes pin)
- **llm-d-inference-sim** v1.0.0 for the simulator models
- **Gateway API** v1.6.2 and **Gateway API Inference Extension** v1.6.2 CRDs, **Istio** 1.29,
  installed by `infra/gateway-provider`

## Model Weight Pre-loading

For deployments with real model weights (not sim), model OCI artifacts must be pushed to
Harbor **before** `pulumi up`. Pulumi creates a PVC and an ORAS pull Job that fetches the
artifact from Harbor at deploy time — no HuggingFace access is required from the cluster.

The installer downloads the selected models from Hugging Face and pushes them to Harbor
in its [`Populate Harbor` stage](../installation/installer-guide.md#populate-harbor-stage). The Hugging Face
commit is pinned by `revision` in each model's `model.yaml`, which also derives the Harbor
tag.

The `modelSource` block in the stack config triggers Pulumi to create the PVC and ORAS Job:

```yaml
modelSource:
  harborRef: harbor.harbor.svc.cluster.local/ai-models/<artifact>:<tag>
  modelUri: hub/<org>/<model>
  storageSize: <size>Gi
  hostpathNode: <node-hostname>   # on-prem only — node where the hostpath PV is created
```

On on-prem clusters, `hostpathNode` is required. Pulumi creates the PV automatically
pointing to `/var/lib/hostpath/models/<slug>` on the specified node. The directory
must exist on that node before `pulumi up`. Set `hostpathDir` to use a different
absolute path.

See [Model storage](model-storage.md) for full details on the artifact layout and
Job failure recovery.

## Troubleshooting

### Common Issues
1. **Missing `harborToken`**: Ensure `harborToken` is configured as an encrypted secret (`pulumi config set --secret harborToken <secret>`). Required for on-prem and for cloud stacks using `modelSource`.
2. **Missing models**: Ensure `models` (with `generative` and/or `embedder` lists) is set in the stack config.
3. **Namespace Issues**: Verify target namespace exists or will be created.
4. **Gateway Provider Dependencies**: Check that the Gateway API, Inference Extension and llm-d router CRDs and the Istio control plane are installed by the `gateway-provider` stack (`infra/gateway-provider`). The router release fails without the `InferencePool` and `InferenceObjective` CRDs.
5. **ORAS Job failed**: Check `kubectl logs -n <ns> job/<slug>-model-pull`. After fixing root cause, delete the Job and PVC manually then re-run `pulumi up`. See [Model storage](model-storage.md) for recovery steps.
6. **Pod starts with empty PVC**: The model server Deployment depends on the ORAS Job, so this only happens when the Job was deleted by hand. Delete the PVC too and re-run `pulumi up`.
7. **Replica stuck `Pending`**: all replicas of a model share the node holding its `ReadWriteOnce` weights volume. Check that node has free GPUs for every replica (`kubectl describe pod -n <ns> <pod>`).
8. **Requests fail with 503 and `x-llm-d-request-dropped-reason: rejected-no-endpoints`**: no model server pod is ready; check `kubectl get pods -n <ns> -l llm-d.ai/model=model-<slug>`.
9. **EPP pod `Pending`**: the router requests 1 CPU and 2Gi (more if the model sets `router.resources`); check the CPU nodes have that much free.
10. **Old pods do not terminate on upgrade**: the deploy waits up to 10 minutes for the retired chart's model server pods to go. Check `kubectl get pods -n <ns> -l llm-d.ai/model=ms-<slug>-llm-d-modelservice`.

## Development

`app_serving/` itself only contains `main.go` (a thin wrapper that calls
`serving.DeployAppServing(ctx, "", nil)`), `deployments/`, and `charts/`. The actual
implementation lives in the shared `pkg` module, at `pkg/iac/serving/`:

- `main.go`: Compiles the stack binary; delegates to `pkg/iac/serving.DeployAppServing`
- `pkg/iac/serving/modeldef/`: the `model.yaml` schema, shared with the installer
- `pkg/iac/serving/serving.go`: Orchestration — loops over all models in `config.Models`
- `pkg/iac/serving/internal/config/config.go`: Config loading and model discovery; loads each enabled model's `model.yaml`
- `pkg/iac/serving/internal/config/naming.go`: Resource names derived from the slug
- `pkg/iac/serving/internal/components/`: `gateway`, `router`, `modelserver`, `httproute`, `embeddingservice`
- `pkg/iac/serving/internal/legacy/`: removal of the retired modelservice chart's pods on upgrade
- `pkg/iac/serving/internal/platform/secret.go`: Harbor pull secret creation (`harbor-creds`)
- `pkg/iac/serving/internal/platform/model_storage.go`: PVC + ORAS pull Job creation for `modelSource`
- `deployments/models/`: Model definitions (one `model.yaml` per category + model + cluster target combination)
- `deployments/Pulumi.TEMPLATE.yaml`: Template for creating new stack configs

## State Management

Pulumi state is managed via the backend configured for the environment (S3 or GCS):
```bash
pulumi login <backend-url>
pulumi stack select <stack>
```

## Runtime metadata for Control Panel discovery

Models can opt into the dedicated read-only endpoint with `server.runtimeMetadata: true` in
their `model.yaml`. GPT-OSS-20B enables this by default. The platform mounts a small vLLM
middleware module from a ConfigMap, and shaide-server reads `GET /v1/shaide/metadata` through
the discovered model Service. The `model.yaml` configures vLLM; the endpoint reports the
running process's resolved settings. It does not read a YAML copy, run inference probes, or
enable vLLM development APIs.

See the [runtime metadata module](https://github.com/axem-solutions/shaide/blob/main/pkg/iac/serving/internal/components/modelserver/metadata/README.md)
for the schema, authentication behavior, missing-dimension handling, and tests.
