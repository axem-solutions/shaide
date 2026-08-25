# model-registry

Scripts and manifests for downloading AI models from HuggingFace and pushing them
to Harbor as OCI artifacts. Models are versioned, pinned to a specific commit hash,
and stored in HuggingFace hub cache layout so serving pods can load them directly
via `from_pretrained`.

The HF token stays on the provisioner — it never enters the Kubernetes cluster.

---

## Directory Structure

```
model-registry/
├── env-vars/
│   ├── example                  # Template — copy and fill in per deployment
│   └── <stack-name>             # Per-deployment credentials (gitignored)
├── models/
│   └── <stack-name>.yml         # Per-deployment model manifest
├── images/
│   ├── downloader/              # Dockerfile for huggingface-cli container
│   └── inferencer/              # Dockerfile + infer.py for model verification
├── documentation/
│   └── MODEL-REGISTRY-GUIDE.md  # HF vs Harbor comparison, cache layout, on-prem guide
├── logs/                        # Script logs (gitignored)
├── model-cache/                 # Downloaded model files (gitignored)
├── model-download.sh            # Download models from HuggingFace to local cache
├── model-upload.sh              # Push models from local cache to Harbor
├── model-sync.sh                # Download + upload in one step
├── model-verify.sh              # Verify models on provisioner via ORAS pull
└── model-verify-k8s.sh          # Verify models via Kubernetes Jobs inside the cluster
```

---

## Prerequisites

- Docker (runs `hf-downloader` and `oras` containers locally)
- `kubectl` configured for the target cluster
- Harbor deployed and accessible (see `infra/cloud-harbor`)
- HuggingFace token for downloading gated models

---

## Setup

### 1 — Create env file

Copy the example and fill in credentials for each deployment:

```bash
cp env-vars/example env-vars/<stack-name>
```

```bash
# env-vars/<stack-name>
HF_TOKEN=hf_...
HARBOR_USER='robot$k8s-harbor-sa'
HARBOR_PASSWORD=<robot-password>
```

Env files are gitignored. Only `env-vars/example` is committed.

### 2 — Create or update the models manifest

Create `models/<stack-name>.yml` declaring which models to sync:

```yaml
models:
  - id: "nomic-ai/nomic-embed-text-v1.5"
    harbor_project: ai-models
    harbor_name: nomic-embed-text-v1.5
    harbor_tag: "1.5.0"
    revision: "e5cf08aadaa33385f5990def41f7a23405aec398"
    dependencies:
      - id: "nomic-ai/nomic-bert-2048"
        revision: "7710840340a098cfb869c4f65e87cf2b1b70caca"
```

Always pin `revision` to a HuggingFace commit hash. Models and dependencies are
bundled into a single OCI artifact per model entry.

---

## Usage

### Download + upload in one step (recommended)

```bash
ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-sync.sh
```

Opens a `kubectl port-forward` to Harbor, downloads all models from HuggingFace,
and pushes them as OCI artifacts. Port-forward is closed on exit.

### Download only

```bash
ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-download.sh
```

Downloads models into `./model-cache/hub/`. Skips models already in cache.

### Upload only

```bash
ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-upload.sh
```

Pushes models from `./model-cache/hub/` to Harbor. Opens its own port-forward
if one is not already active. Skips models already present in Harbor.

### On-prem

Pass a custom kubeconfig for clusters not in the default context:

```bash
KUBECONFIG=~/.kube/rke2-cluster.yaml \
ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-sync.sh
```

---

## Verification

### On the provisioner (local)

Pulls each model artifact from Harbor to a temp directory and checks that all
expected hub directories are present. Cleans up after each check.

```bash
ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-verify.sh
```

### Inside the cluster (Kubernetes Jobs)

Creates a dedicated `model-verify` namespace and runs one Job per model. Each Job:
1. Pulls the OCI artifact from Harbor via ORAS (no port-forward — uses ClusterIP)
2. Checks expected hub directories are present
3. Validates `config.json`, tokenizer files, and safetensors binary format

**Cloud (internet access):**
```bash
ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-verify-k8s.sh
```

**On-prem (air-gapped):**

The cluster cannot reach external registries, so all images must come from Harbor.
The `hostpath` StorageClass requires a pre-existing PV on a specific node.

```bash
KUBECONFIG=~/.kube/rke2-cluster.yaml \
  ENV_FILE=rke2-cluster \
  MODELS_YML=rke2-cluster.yml \
  ORAS_CLUSTER_IMAGE=harbor.harbor.svc.cluster.local/images-infra/oras-project/oras:v1.3.1 \
  ALPINE_IMAGE=harbor.harbor.svc.cluster.local/images-infra/alpine:3.20 \
  INFER_IMAGE=harbor.harbor.svc.cluster.local/images-infra/python:3.12-slim \
  VERIFY_PVC_SIZE=10Gi \
  VERIFY_STORAGE_CLASS=hostpath \
  VERIFY_HOSTPATH_NODE=<node-hostname> \
  bash model-verify-k8s.sh
```

The script automatically creates and deletes the hostpath PV. The backing directory
(`/var/lib/hostpath/model-verify` by default) must exist on `VERIFY_HOSTPATH_NODE`
before running — managed via the `hostpath_dirs` Ansible role.

| Variable | Default | Description |
|---|---|---|
| `ORAS_CLUSTER_IMAGE` | same as `ORAS_IMAGE` | ORAS image used inside K8s Job (override for air-gap) |
| `ALPINE_IMAGE` | `alpine:3.20` | Alpine image for init/verify containers |
| `INFER_IMAGE` | `python:3.12-slim` | Python image for safetensors validation |
| `VERIFY_PVC_SIZE` | `60Gi` | PVC size (reduce for small disks) |
| `VERIFY_STORAGE_CLASS` | cluster default | StorageClass for the verification PVC |
| `VERIFY_HOSTPATH_NODE` | — | Required when `VERIFY_STORAGE_CLASS=hostpath` |
| `VERIFY_HOSTPATH_DIR` | `/var/lib/hostpath/model-verify` | Backing directory on the node |

The namespace is deleted on exit. Logs are written to `logs/`.

---

## How It Works

```
Provisioner
  model-download.sh
    └─ docker run hf-downloader
         └─ huggingface-cli download --cache-dir model-cache/hub/
              └─ models--<org>--<name>/
                   ├─ refs/main          (commit hash)
                   ├─ snapshots/<hash>/  (model files)
                   └─ blobs/             (content-addressed store)

  model-upload.sh
    └─ kubectl port-forward → harbor svc
    └─ docker run oras push
         └─ harbor.harbor:80/ai-models/<name>:<tag>
              └─ OCI artifact (model + dependencies bundled)

Kubernetes cluster
  ORAS Pull Job
    └─ oras pull harbor.harbor:80/ai-models/<name>:<tag>
         └─ writes to PV/PVC at /model-cache/hub/

  Serving Pod
    └─ mounts PV/PVC
    └─ from_pretrained("/model-cache/hub/models--<org>--<name>/...")
```

---

## Logs

All scripts write timestamped logs to `logs/`:

| File | Script |
|---|---|
| `model-download-<ts>.log` | `model-download.sh` |
| `model-upload-<ts>.log` | `model-upload.sh` |
| `model-sync-<ts>.log` | `model-sync.sh` |
| `model-verify-<ts>.log` | `model-verify.sh` |
| `model-verify-k8s-<ts>.log` | `model-verify-k8s.sh` |

---

## Further Reading

See `documentation/MODEL-REGISTRY-GUIDE.md` for:
- HuggingFace vs Harbor workload comparison with diagrams
- HF hub cache layout vs Harbor pull (symlinks, disk usage)
- On-prem hostPath storage and RWX considerations
- Deployment compatibility between HF and Harbor
