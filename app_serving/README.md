# Application Layer for serving

This is the application layer Pulumi project that deploys the distributed LLM model serving infrastructure using the llm-d framework on Kubernetes.

## Overview

The app layer deploys a complete LLM serving stack including:
- **llm-d Infrastructure** components from the llm-d-infra Helm chart
- **GAIE (Gateway AI Engine)** for inference gateway
- **ModelService** for serving specific models
- **HTTPRoutes** for connecting generative model components
- **Embedding Services** for direct-backend access to embedding models

## Architecture

The deployment creates the following components:

1. **LLM-D Infrastructure** (`pkg/iac/serving/internal/components/llmd-infra/deploy.go`): Core llm-d components
   - Deployed via Helm chart from `../upstream/llm-d/llm-d-infra/charts/llm-d-infra`
   - Creates Gateway with Istio ingress configuration
   - Sets up TLS and traffic policies

2. **GAIE** (`pkg/iac/serving/internal/components/gaie/deploy.go`): AI inference engine gateway
   - InferencePool for model serving
   - Endpoint configurations

3. **Model Service** (`pkg/iac/serving/internal/components/modelservice/deploy.go`): Model-specific serving
   - Supports any model via convention-based discovery from `deployments/models/` directory
   - Configurable via the `models.generative` / `models.embedder` lists

4. **HTTPRoute** (`pkg/iac/serving/internal/components/httproute/deploy.go`): Traffic routing for generative models
   - Connects Gateway listeners to GAIE InferencePool
   - Created only for `generative` category models

5. **Embedding Service** (`pkg/iac/serving/internal/components/embeddingservice/deploy.go`): Direct-backend Service for embedder models
   - Created only for `embedder` category models; skipped for generative models
   - ClusterIP Service named `ms-<slug>-embeddings` in the model namespace
   - Selects decode pods via `llm-d.ai/model` and `llm-d.ai/role: decode` labels
   - Exposes port 8200 — the inference server port used by vLLM in pooling (embedding) mode
   - Reachable as `http://ms-<slug>-embeddings.<namespace>.svc.cluster.local:8200/v1/embeddings`

### Traffic flow

**Generative models:**
shaide server → Istio Gateway → GAIE InferencePool → ModelService decode pod (port 8000 via routing proxy)

**Embedding models:**
shaide server → `ms-<slug>-embeddings.<namespace>.svc.cluster.local:8200` → ModelService decode pod (port 8200 direct)

The Gateway/HTTPRoute path is not used for embedding models — the per-model gateway service returns route-level errors for embedding requests. Downstream apps (e.g. app-shaide) must use the direct ClusterIP Service DNS name for embeddings.

#### End-to-end call path for embeddings

```
app-shaide
    │
    │  POST /v1/embeddings
    ▼
ms-<slug>-embeddings.<namespace>.svc.cluster.local:8200
    │
    │  kube-proxy / iptables (ClusterIP)
    ▼
decode pod (vLLM --runner pooling, listening on :8200)
```

No Istio Gateway, no GAIE, no EPP scheduling — the request goes straight to the model server.

External ingress and TLS for generative models are handled by the cloud-specific infra layer (e.g., AWS Load Balancer Controller or GKE Gateway + Certificate Manager), keeping this app layer provider-agnostic.

## Configuration

The application is configured via Pulumi config values in `deployments/Pulumi.{stack}.yaml`:

### Required Configuration
- `cloudProvider`: Target platform — `cloud` (GCP/AWS/Azure) or `on-prem`. Determines which credentials are required and how the Kubernetes provider is configured.
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
- `releaseName`: Override auto-derived Helm release name (default: `infra-<slug>`).

### On-Prem / Air-Gap Configuration (optional)
These fields are only needed when deploying to an air-gapped RKE2 cluster. All are
absent from cloud stack configs.

- `kubeconfig`: Path to the RKE2 cluster kubeconfig on the provisioner laptop (e.g. `~/.kube/rke2-cluster.yaml`). Omit to fall back to `KUBECONFIG` env var or `~/.kube/config`.
- `harborHostname`: Internal Harbor registry hostname for containerd image pulls (e.g. `harbor.internal.lan`).
- `harborUser`: Harbor robot account name (e.g. `robot$k8s-puller`).
- `harborToken`: Harbor robot account secret (encrypted). Required for on-prem. Set via `pulumi config set --secret harborToken <secret>`. When set, a `harbor-creds` pull secret is created in each model namespace.

## Prerequisites

1. **Kubernetes Cluster**: GCP/AWS/Azure cloud cluster, or an on-prem RKE2 cluster provisioned via `infra/on-prem`
2. **Gateway Provider Prereqs**: Gateway API + GAIE CRDs and a Gateway implementation (Istio) installed via `infra/gateway-provider`
3. **Harbor Registry**: Models stored as OCI artifacts in Harbor. On cloud, Harbor is deployed by `infra/cloud-harbor`; on on-prem by `infra/on-prem/pulumi/services`. Model artifacts uploaded via `infra/model-registry/model-sync.sh`.
4. **Go**: Go 1.23+ with toolchain 1.24.5+
5. **Pulumi**: Pulumi CLI
6. **On-prem only**: Harbor registry deployed and configured, images uploaded via `infra/on-prem/ansible/harbor_upload.yml`. The `hostpath` StorageClass is used for model PVCs — PVs are created automatically by Pulumi when `modelSource.hostpathNode` is set, but the backing directory must exist on the node first (managed via the `hostpath_dirs` Ansible role).
7. **Real models only**: Model OCI artifacts pushed to Harbor via `infra/model-registry/model-sync.sh` before `pulumi up`. See [Model Weight Pre-loading](#model-weight-pre-loading) below.

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
   kubectl get gateway -n llm-d-<slug>
   kubectl get httproute -n llm-d-<slug>
   ```

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
- **sim**: Simulation model for testing (GCP)
- **sim-rke2-1 / sim-rke2-2**: Simulation model for testing (RKE2 on-prem, Harbor images + `imagePullSecrets`)
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

Each entry below is a folder under `deployments/models/`. The same model may have
multiple folders for different cluster targets (e.g. `sim` for GCP vs `sim-rke2-1` for RKE2).

| Folder | Model | Target | Notes |
|--------|-------|--------|-------|
| `sim` | llm-d inference simulator | GCP | No GPU required; for testing |
| `sim-rke2-1` | llm-d inference simulator | RKE2 on-prem | Harbor images, `imagePullSecrets` |
| `sim-rke2-2` | llm-d inference simulator | RKE2 on-prem | Harbor images, `imagePullSecrets` |
| `DeepSeek-Coder-V2-Lite-Instruct` | DeepSeek Coder V2 Lite | GCP | 2× GPU, real model weights |
| `GLM-4.7-Flash` | Z.ai GLM-4.7-Flash | Cloud | 4× GPU, real model weights |
| `GPT-OSS-20B` | GPT-OSS 20B | GCP / on-prem | 1× GPU, real model weights |
| `Qwen3-Coder-30B-A3B-Instruct` | Qwen3 Coder 30B A3B Instruct | Cloud | BF16, tuned for 1× 96 GB GPU; use FP8 or TP>1 on 48 GB GPUs |
| `nomic-embed-text-v1-5-rke2` | nomic-ai/nomic-embed-text-v1.5 | RKE2 on-prem | CPU-only test via infinity; model weights pre-loaded via Harbor + ORAS (`modelSource.hostpathNode`) |

## Adding a New Model

The code uses convention-based model discovery via `resolveModelPaths()`. Users create `deployments/models/<category>/<modelName>/gaie-<slug>/` and `ms-<slug>/` folders, and the code auto-derives `namespace`, release names, and values paths from the slug.

### How it works

Paths are auto-discovered by `resolveModelPaths()` from the folder structure.
Given a model entry `DeepSeek-Coder-V2-Lite-Instruct` in `models.generative`:
```
→ scans deployments/models/generative/DeepSeek-Coder-V2-Lite-Instruct/
→ finds gaie-deepseek-coder/ → gaieValuesPath = models/.../gaie-deepseek-coder/values.yaml
→ finds ms-deepseek-coder/   → modelValuesPath = models/.../ms-deepseek-coder/values.yaml
```

### The Workflow

1. User creates folders:
```
deployments/models/generative/Test-Alma-V5-Pro/
  gaie-alma-pro/values.yaml
  ms-alma-pro/values.yaml
```

2. User adds the model to `models` in `Pulumi.<stack-name>.yaml` under the appropriate category:
```yaml
config:
  app-serving:cloudProvider: cloud
  app-serving:models:
    generative:
      - name: Test-Alma-V5-Pro
        enabled: true
        nodeSelector:
          nodegroup: <node-pool-name>
        modelSource:
          harborRef: harbor.harbor.svc.cluster.local/ai-models/test-alma-v5-pro:1.0.0
          modelUri: hub/test-org/Test-Alma-V5-Pro
          storageSize: 10Gi
  app-serving:harborHostname: harbor.harbor.svc.cluster.local
  app-serving:harborUser: robot$k8s-puller
  app-serving:harborToken:
    secure: v1:...
```

3. Validation. The code catches mismatches immediately.
In case the user mistypes subfolder names, for example:
```
deployments/models/Test-Alma-V5-Pro/
  gaie-aLMa-pro/values.yaml
  ms-aMLa-pro/values.yaml
```
Error message:
```
Error: slug mismatch: gaie-aLMa-pro vs ms-aMLa-pro in "models/Test-Alma-V5-Pro"
```
It also verifies both `values.yaml` files actually exist.

4. Code scans, extracts slug `alma-pro` from folder names, derives everything else.

### Naming flow

```
models entry: "DeepSeek-Coder-V2-Lite-Instruct"
     │
     ▼
deployments/models/generative/DeepSeek-Coder-V2-Lite-Instruct/gaie-deepseek-coder/
                                             │
                                        slug = "deepseek-coder"
                                             │
              ┌──────────────────────────────┼──────────────────────────────┐
              ▼                              ▼                              ▼
    ns: llm-d-deepseek-coder    release: infra-deepseek-coder    gaie: gaie-deepseek-coder
                                                                 ms:   ms-deepseek-coder
```

Everything flows from the slug. The slug flows from the folder name the user created.

What gets auto-derived from the slug:

| Resource | Naming pattern | Example (slug: `deepseek-coder`) |
|---|---|---|
| Namespace | `llm-d-<slug>` | `llm-d-deepseek-coder` |
| Infra release | `infra-<slug>` | `infra-deepseek-coder` |
| GAIE release | `gaie-<slug>` | `gaie-deepseek-coder` |
| Model service release | `ms-<slug>` | `ms-deepseek-coder` |
| Gateway | `infra-<slug>-inference-gateway` | `infra-deepseek-coder-inference-gateway` |
| HTTPRoute | `llm-d-<slug>` | `llm-d-deepseek-coder` |

### Steps

#### Required Steps

1. **Create the model directory**

```bash
mkdir -p models/<category>/<ModelName>/gaie-<slug>/
mkdir -p models/<category>/<ModelName>/ms-<slug>/
```

- `<category>` — `generative` or `embedder`
- `<ModelName>` — the exact model name (e.g. `DeepSeek-Coder-V2-Lite-Instruct`)
- `<slug>` — a short, lowercase identifier (e.g. `deepseek-coder`)
- Both subdirectories **must use the same slug**

Example:
```bash
mkdir -p models/generative/Test-Alma-V5-Pro/gaie-alma-pro/
mkdir -p models/generative/Test-Alma-V5-Pro/ms-alma-pro/
```

2. **Add Helm values files**

Create `values.yaml` in each subdirectory:

```
deployments/models/generative/Test-Alma-V5-Pro/
  gaie-alma-pro/values.yaml    # GAIE InferencePool values
  ms-alma-pro/values.yaml      # llm-d-modelservice values
```

Use an existing model as reference for the values structure:
- Cloud target: copy from `deployments/models/generative/sim/` (external image registries)
- On-prem target: copy from `deployments/models/generative/sim-rke2-1/` (Harbor image references, `imagePullSecrets`)

3. **Add the model to the stack config file**

Add the model to `models` in `Pulumi.<stack-name>.yaml` under the appropriate category (`generative` or `embedder`):

```yaml
app-serving:models:
  generative:
    - name: ExistingModel
      enabled: true
      nodeSelector:
        nodegroup: <node-pool-name>
    - name: Test-Alma-V5-Pro
      enabled: true
      nodeSelector:
        nodegroup: <node-pool-name>
```

Or create a new stack config from the template:

```bash
cp Pulumi.TEMPLATE.yaml Pulumi.<stack-name>.yaml
```

4. **Create a Pulumi stack and set the secret** (new stacks only)

```bash
cd app_serving/deployments
pulumi stack init <stack-name>
pulumi config set --secret harborToken <robot-secret>
```

5. **Preview and deploy**

```bash
pulumi preview    # verify what will be created
pulumi up         # deploy
```

#### Optional per-model overrides

These can be added inside a model entry to override auto-derived values:

```yaml
app-serving:models:
  generative:
    - name: Test-Alma-V5-Pro
      enabled: true
      nodeSelector:
        nodegroup: <node-pool-name>
      nameSpace: custom-namespace       # overrides llm-d-<slug>
      releaseName: custom-release-name  # overrides infra-<slug>
```

#### Validation

The code validates that:
- A `gaie-*` subdirectory exists under `deployments/models/<category>/<ModelName>/`
- A `ms-*` subdirectory exists under `deployments/models/<category>/<ModelName>/`
- Both use the **same slug** (e.g. `gaie-alma-pro` and `ms-alma-pro`)
- Both `values.yaml` files exist

Mismatches (e.g. `gaie-alma-pro` + `ms-amla-pro`) will produce a clear error at `pulumi preview` time.

## Exposing Installed Models

app_serving does not export a model list. Instead, each model's routable
Service is labeled with `axem.dev/model-slug`, `axem.dev/model-category`
(`generative`/`embedder`), and `app.kubernetes.io/part-of=app-serving` (see
`Model.MetaLabels()` in `internal/config/naming.go`) — the generative path's
`llmd-gateway-<slug>` ExternalName Service and the embedder path's
`ms-<slug>-embeddings` ClusterIP Service.

app-shaide grants shaide-server cluster-wide read access to `services` and
`namespaces` (see `pkg/iac/shaide/internal/platform/k8s-rbac.go`) so it can
discover this topology by label selector at runtime and pair each Service
with the model-owned metadata (name, context size, ...) served by vLLM's own
`/server-info` endpoint — rather than app_serving parsing `values.yaml` to
rebuild state vLLM already owns. `values.yaml` still configures the model's
actual deployment; it is no longer a source consumed to describe it.

Two enabled models resolving to the same slug collide on the Kubernetes
objects they'd create (same `llm-d-<slug>` namespace, same release names),
so the deploy fails — just at apply time against the cluster, not at plan
time.

Note this is a different check from the one that used to exist:
`BuildModelsJSON` rejected duplicate *served* model names (`modelArtifacts.name`
in `ms-<slug>/values.yaml` — what a client puts in the OpenAI `model`
field), which is orthogonal to the slug. Two different slugs deploying the
same underlying model under the same served name will both come up fine
today — nothing in this repo detects that anymore. Catching it belongs
with whatever aggregates `/server-info` across models (shaide-server), the
same place that now owns served-name resolution in the first place.

## Dependencies

The project uses:
- **Pulumi Kubernetes Provider** v4.25.0
- **llm-d framework** (local dependency via `../upstream/llm-d/llm-d-infra/`)
- **llm-d-infra** chart v1.4.0
- **llm-d-modelservice** chart v0.4.12
- **Gateway API Inference Extension** chart v1.2.0
- **Helm** for component packaging

## Model Weight Pre-loading

For deployments with real model weights (not sim), model OCI artifacts must be pushed to
Harbor **before** `pulumi up`. Pulumi creates a PVC and an ORAS pull Job that fetches the
artifact from Harbor at deploy time — no HuggingFace access is required from the cluster.

Use the scripts in `infra/model-registry/`:

```bash
cd infra/model-registry

# Build downloader image once
docker build -t hf-downloader:local downloader/

# Download from HuggingFace + push to Harbor
HF_TOKEN=<your-token> HARBOR_HOST=localhost:5000 CACHE_DIR=/tmp/hf-cache ./model-sync.sh
```

Model definitions (HF repo ID, Harbor project, artifact name, tag) live in
`infra/model-registry/models.yml`.

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
must exist before `pulumi up` — add it to `hostpath_pv_dirs` in the node's
`inventory-dev/host_vars/<node>.yml` and run:

```bash
cd infra/on-prem/ansible
ansible-playbook setup/hostpath_dirs.yml -i inventory-dev --limit <node>
```

See [MODEL_STORAGE.md](MODEL_STORAGE.md) for full details on the artifact layout and
Job failure recovery.

## Troubleshooting

### Common Issues
1. **Missing `harborToken`**: Ensure `harborToken` is configured as an encrypted secret (`pulumi config set --secret harborToken <secret>`). Required for on-prem and for cloud stacks using `modelSource`.
2. **Missing models**: Ensure `models` (with `generative` and/or `embedder` lists) is set in the stack config.
3. **Namespace Issues**: Verify target namespace exists or will be created.
4. **Gateway Provider Dependencies**: Check that Gateway CRDs and Istio control plane are installed via `infra/gateway-provider`.
5. **ORAS Job failed**: Check `kubectl logs -n <ns> job/<slug>-model-pull`. After fixing root cause, delete the Job and PVC manually then re-run `pulumi up`. See [MODEL_STORAGE.md](MODEL_STORAGE.md) for recovery steps.
6. **Pod starts with empty PVC**: The ORAS Job may have not finished or the ModelService chart was deployed without `DependsOn` the Job. Ensure the Job completed successfully before the pod starts.
7. **Port conflict in decode pod**: The routing-proxy init container binds port 8000. The inference server (`vllmServe` or `custom` modelCommand) must listen on port **8200** for decode pods. With `modelCommand: vllmServe` the chart handles this automatically; with `modelCommand: custom` set `--port 8200` explicitly.
8. **Embeddings endpoint via gateway returns 400**: The inference gateway (EPP) only validates chat-completion requests. For embedding models, access the inference server directly via pod port-forward on port 8200 or add a separate ClusterIP service pointing to port 8200.

## Development

`app_serving/` itself only contains `main.go` (a thin wrapper that calls
`serving.DeployAppServing(ctx, "", nil)`), `deployments/`, and `charts/`. The actual
implementation lives in the shared `pkg` module, at `pkg/iac/serving/`:

- `main.go`: Compiles the stack binary; delegates to `pkg/iac/serving.DeployAppServing`
- `pkg/iac/serving/serving.go`: Orchestration — loops over all models in `config.Models`
- `pkg/iac/serving/internal/config/config.go`: Config loading and convention-based model discovery; returns a single `Config` containing one `Model` entry per enabled model
- `pkg/iac/serving/internal/config/naming.go`: Release name derivation from slug, plus `ChatCompletionEndpoint()`/`EmbeddingURL()` and `MetaLabels()` (the labels shaide-server discovers a model's Service by)
- `pkg/iac/serving/internal/components/*/deploy.go`: Component-specific installation functions (llmd-infra, gaie, modelservice, httproute, embeddingservice)
- `pkg/iac/serving/internal/platform/secret.go`: Harbor pull secret creation (`harbor-creds`)
- `pkg/iac/serving/internal/platform/model_storage.go`: PVC + ORAS pull Job creation for `modelSource`
- `deployments/models/`: Model-specific configurations (one directory per category + model + cluster target combination)
- `deployments/Pulumi.TEMPLATE.yaml`: Template for creating new stack configs

## State Management

Pulumi state is managed via the backend configured for the environment (S3 or GCS):
```bash
pulumi login <backend-url>
pulumi stack select <stack>
```
